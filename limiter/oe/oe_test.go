package oe_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gustone01/oe-limiter-sdk/limiter/core"
	"github.com/gustone01/oe-limiter-sdk/limiter/oe"
	"github.com/gustone01/oe-limiter-sdk/model"

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
	if err := oe.AutoMigrate(db); err != nil {
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

	_ = db.Create(&model.OeRateLimitRule{APIPathPrefix: "/open_api/v3.0/event/track/", QPSLimit: 2, Enabled: 1})

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	tp, err := oe.NewTransport(db, rdb,
		oe.WithPubSubChannel("ch:"+t.Name()),
		oe.WithSkipAutoMigrate(),
		oe.WithBaseTransport(upstream.Client().Transport),
	)
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
	if resp.Status != core.StatusRateLimited {
		t.Fatalf("want status %q, got %q", core.StatusRateLimited, resp.Status)
	}
}

func TestAutoDiscover_WritePending(t *testing.T) {
	db := testDB(t)
	rdb, cleanup := testRedis(t)
	defer cleanup()

	rm, err := oe.NewRuleManager(db, rdb,
		oe.WithPubSubChannel("ch:"+t.Name()),
		oe.WithSkipAutoMigrate(),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer rm.Close()

	_ = rm.GetRule("/open_api/v3.0/new/{id}")

	var n int64
	db.Model(&model.OeRateLimitPending{}).Where("status = ?", model.PendingStatusPending).Count(&n)
	if n != 1 {
		t.Fatalf("pending=%d want 1", n)
	}
}

func TestApprovePending(t *testing.T) {
	db := testDB(t)
	rdb, cleanup := testRedis(t)
	defer cleanup()

	rm, err := oe.NewRuleManager(db, rdb,
		oe.WithPubSubChannel("ch:"+t.Name()),
		oe.WithSkipAutoMigrate(),
	)
	if err != nil {
		t.Fatal(err)
	}

	_ = rm.GetRule("/open_api/v3.0/report/{id}")
	list, _ := rm.ListPending(context.Background())
	// 使用 ApprovePending 而非 ApprovePendingAndReload，避免 Pub/Sub 异步 reload 与 SQLite 内存 DB 竞态
	if err := rm.ApprovePending(context.Background(), list[0].ID, 50); err != nil {
		t.Fatal(err)
	}
	_ = rm.Close()

	var rules int64
	db.Model(&model.OeRateLimitRule{}).Count(&rules)
	if rules != 1 {
		t.Fatalf("rules=%d want 1", rules)
	}
}

func TestApprovePending_SentinelErrors(t *testing.T) {
	db := testDB(t)
	rdb, cleanup := testRedis(t)
	defer cleanup()

	rm, err := oe.NewRuleManager(db, rdb,
		oe.WithPubSubChannel("ch:"+t.Name()),
		oe.WithSkipAutoMigrate(),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer rm.Close()

	_ = rm.GetRule("/open_api/v3.0/sentinel/test")
	list, _ := rm.ListPending(context.Background())
	pid := list[0].ID

	if err := rm.ApprovePending(context.Background(), pid, 100); err != nil {
		t.Fatal(err)
	}
	err = rm.ApprovePending(context.Background(), pid, 100)
	if !errors.Is(err, core.ErrPendingNotPending) {
		t.Fatalf("want ErrPendingNotPending, got %v", err)
	}
}

func TestRuleManager_CloseIdempotent(t *testing.T) {
	db := testDB(t)
	rdb, cleanup := testRedis(t)
	defer cleanup()

	rm, err := oe.NewRuleManager(db, rdb,
		oe.WithPubSubChannel("ch:"+t.Name()),
		oe.WithSkipAutoMigrate(),
	)
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

func TestNormalizePath(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"/open_api/v3.0/event/track/1234567890", "/open_api/v3.0/event/track/{id}"},
		{"/open_api/2/customer_center/advertiser/list/", "/open_api/2/customer_center/advertiser/list/"},
		{"/a/1/b/2/c", "/a/1/b/2/c"},
		{"/open_api/v3.0/foo/12345/bar", "/open_api/v3.0/foo/{id}/bar"},
	}
	for _, tc := range cases {
		got := oe.NormalizePath(tc.in)
		if got != tc.want {
			t.Errorf("NormalizePath(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
