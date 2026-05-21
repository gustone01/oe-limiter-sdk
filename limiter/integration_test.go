// 集成测试：SQLite(内存) + miniredis，无需真实 MySQL/Redis。
package limiter_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"192.168.10.236/gustone/oe-limiter-sdk/limiter"
	"192.168.10.236/gustone/oe-limiter-sdk/model"

	"github.com/alicebob/miniredis/v2"
	"github.com/glebarez/sqlite"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=private", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := limiter.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	return db
}

func testRedis(t *testing.T) (*redis.Client, func()) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return rdb, func() { _ = rdb.Close(); mr.Close() }
}

func TestTransport_RateLimit429(t *testing.T) {
	db := testDB(t)
	rdb, cleanup := testRedis(t)
	defer cleanup()

	_ = db.Create(&model.RateLimitRule{
		ServiceName: "event", APIPathPrefix: "/open_api/v3.0/event/track/", QPSLimit: 2, Enabled: 1,
	})

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	tp, err := limiter.NewTransport("event", db, rdb, limiter.WithPubSubChannel("ch:"+t.Name()))
	if err != nil {
		t.Fatal(err)
	}
	defer tp.Close()
	client := &http.Client{Transport: tp}
	url := upstream.URL + "/open_api/v3.0/event/track/1"

	for i := 0; i < 2; i++ {
		resp, err := client.Get(url)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("want 200, got %d", resp.StatusCode)
		}
	}
	resp, err := client.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("want 429, got %d", resp.StatusCode)
	}
	if reason := resp.Header.Get(limiter.HeaderRejectReason); reason != limiter.RejectReasonLocal {
		t.Fatalf("reject reason: want %q, got %q", limiter.RejectReasonLocal, reason)
	}
}

func TestAutoDiscover_WritePending(t *testing.T) {
	db := testDB(t)
	rdb, cleanup := testRedis(t)
	defer cleanup()

	rm, err := limiter.NewRuleManager(db, rdb, limiter.WithPubSubChannel("ch:"+t.Name()))
	if err != nil {
		t.Fatal(err)
	}
	defer rm.Close()

	_ = rm.GetRule("event", "/open_api/v3.0/new/{id}")

	var n int64
	db.Model(&model.RateLimitPending{}).Where("status = ?", model.PendingStatusPending).Count(&n)
	if n != 1 {
		t.Fatalf("pending=%d want 1", n)
	}
}

func TestApprovePending(t *testing.T) {
	db := testDB(t)
	rdb, cleanup := testRedis(t)
	defer cleanup()

	rm, err := limiter.NewRuleManager(db, rdb, limiter.WithPubSubChannel("ch:"+t.Name()))
	if err != nil {
		t.Fatal(err)
	}

	_ = rm.GetRule("data", "/open_api/v3.0/report/{id}")
	list, _ := rm.ListPending(context.Background())
	if err := rm.ApprovePendingAndReload(context.Background(), list[0].ID, 50); err != nil {
		t.Fatal(err)
	}
	// 先关闭 subscriber，再查 DB，避免 subscriber 异步 reload 访问已关闭的内存 DB
	_ = rm.Close()
	var rules int64
	db.Model(&model.RateLimitRule{}).Count(&rules)
	if rules != 1 {
		t.Fatalf("rules=%d want 1", rules)
	}
}

func TestTransport_GlobalLimit429(t *testing.T) {
	db := testDB(t)
	rdb, cleanup := testRedis(t)
	defer cleanup()

	_ = db.Create(&model.RateLimitRule{
		ServiceName: "event", APIPathPrefix: "/open_api/", QPSLimit: 1000, Enabled: 1,
	})
	_ = db.Create(&model.RateLimitRule{
		ServiceName: "ALL", APIPathPrefix: "/open_api/", QPSLimit: 1, IsShared: 1, Enabled: 1,
	})

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	tp, err := limiter.NewTransport("event", db, rdb, limiter.WithPubSubChannel("ch:"+t.Name()))
	if err != nil {
		t.Fatal(err)
	}
	defer tp.Close()

	client := &http.Client{Transport: tp}
	url := upstream.URL + "/open_api/v3.0/event/track/1"

	resp, err := client.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("first request want 200, got %d", resp.StatusCode)
	}

	resp, err = client.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("second request want 429 (global limit), got %d", resp.StatusCode)
	}
}

func TestTransport_40110PenaltyBlocksSubsequent(t *testing.T) {
	db := testDB(t)
	rdb, cleanup := testRedis(t)
	defer cleanup()

	_ = db.Create(&model.RateLimitRule{
		ServiceName: "event", APIPathPrefix: "/open_api/", QPSLimit: 1000, Enabled: 1,
	})

	callCount := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		callCount++
		if callCount == 2 {
			w.Header().Set("X-RateLimit-RetryIn", "300")
			w.Header().Set("X-RateLimit-Dimension", "developer_qpm")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"code":40110,"message":"too many requests"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"code":0}`))
	}))
	defer upstream.Close()

	var penaltyCode int
	tp, err := limiter.NewTransport("event", db, rdb,
		limiter.WithPubSubChannel("ch:"+t.Name()),
		limiter.WithBaseTransport(upstream.Client().Transport),
		limiter.WithOnPlatformRateLimit(func(svc, path string, code int, wait time.Duration) {
			penaltyCode = code
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer tp.Close()

	client := &http.Client{Transport: tp}

	// 第 1 次：正常
	resp, err := client.Get(upstream.URL + "/open_api/v3.0/foo")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("req1: want 200, got %d", resp.StatusCode)
	}

	// 第 2 次：upstream 返回 40110
	resp, err = client.Get(upstream.URL + "/open_api/v3.0/foo")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("req2: want 200 (platform returns 200+code), got %d", resp.StatusCode)
	}
	if penaltyCode != 40110 {
		t.Fatalf("penalty callback: want 40110, got %d", penaltyCode)
	}

	// 第 3 次：应被 SDK 封禁拦截，不发起真实请求
	resp, err = client.Get(upstream.URL + "/open_api/v3.0/foo")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 429 {
		t.Fatalf("req3: want 429 (penalty block), got %d", resp.StatusCode)
	}
	if reason := resp.Header.Get(limiter.HeaderRejectReason); reason != limiter.RejectReasonPenalty {
		t.Fatalf("reject reason: want %q, got %q", limiter.RejectReasonPenalty, reason)
	}
	if callCount != 2 {
		t.Fatalf("upstream should only receive 2 calls, got %d", callCount)
	}

	// 另一个 service（data）访问相同路径，也应被封禁（接口级 Key）
	tp2 := limiter.NewTransportWithManager("data", tp.Manager(),
		limiter.WithBaseTransport(upstream.Client().Transport),
	)
	client2 := &http.Client{Transport: tp2}
	resp, err = client2.Get(upstream.URL + "/open_api/v3.0/foo")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 429 {
		t.Fatalf("cross-service req: want 429 (penalty block), got %d", resp.StatusCode)
	}
}

func TestApprovePending_SentinelErrors(t *testing.T) {
	db := testDB(t)
	rdb, cleanup := testRedis(t)
	defer cleanup()

	rm, err := limiter.NewRuleManager(db, rdb, limiter.WithPubSubChannel("ch:"+t.Name()))
	if err != nil {
		t.Fatal(err)
	}
	defer rm.Close()

	_ = rm.GetRule("event", "/open_api/v3.0/sentinel/test")
	list, _ := rm.ListPending(context.Background())
	if len(list) == 0 {
		t.Fatal("expected pending record")
	}
	pid := list[0].ID

	// 先通过审核（不触发 PubSub 以避免 SQLite 并发竞态）
	if err := rm.ApprovePending(context.Background(), pid, 100); err != nil {
		t.Fatal(err)
	}
	// 再次审核同一条 → ErrPendingNotPending
	err = rm.ApprovePending(context.Background(), pid, 100)
	if !errors.Is(err, limiter.ErrPendingNotPending) {
		t.Fatalf("want ErrPendingNotPending, got %v", err)
	}
}

func TestRuleManager_CloseIdempotent(t *testing.T) {
	db := testDB(t)
	rdb, cleanup := testRedis(t)
	defer cleanup()

	rm, err := limiter.NewRuleManager(db, rdb, limiter.WithPubSubChannel("ch:"+t.Name()))
	if err != nil {
		t.Fatal(err)
	}
	if err := rm.Close(); err != nil {
		t.Fatal(err)
	}
	if err := rm.Close(); err != nil {
		t.Fatal(err)
	}
}
