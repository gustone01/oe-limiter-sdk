package gdt_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gustone01/oe-limiter-sdk/limiter/core"
	"github.com/gustone01/oe-limiter-sdk/limiter/gdt"
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
	if err := gdt.AutoMigrate(db); err != nil {
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

func TestTransport_QPMLimit429(t *testing.T) {
	db := testDB(t)
	rdb, cleanup := testRedis(t)
	defer cleanup()

	_ = db.Create(&model.GdtRateLimitRule{APIPathPrefix: "/videos/get", QPMLimit: 2, QPDLimit: 0, Enabled: 1})

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	tp, err := gdt.NewTransport(db, rdb,
		gdt.WithPubSubChannel("ch:"+t.Name()),
		gdt.WithSkipAutoMigrate(),
		gdt.WithBaseTransport(upstream.Client().Transport),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer tp.Close()
	client := &http.Client{Transport: tp}
	url := upstream.URL + "/v3.0/videos/get"

	for i := 0; i < 2; i++ {
		resp, err := client.Get(url)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("request %d: want 200, got %d", i+1, resp.StatusCode)
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

func TestTransport_QPDLimit429(t *testing.T) {
	db := testDB(t)
	rdb, cleanup := testRedis(t)
	defer cleanup()

	_ = db.Create(&model.GdtRateLimitRule{APIPathPrefix: "/reports/get", QPMLimit: 0, QPDLimit: 2, Enabled: 1})

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	tp, err := gdt.NewTransport(db, rdb,
		gdt.WithPubSubChannel("ch:"+t.Name()),
		gdt.WithSkipAutoMigrate(),
		gdt.WithBaseTransport(upstream.Client().Transport),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer tp.Close()
	client := &http.Client{Transport: tp}
	url := upstream.URL + "/v3.0/reports/get"

	for i := 0; i < 2; i++ {
		resp, err := client.Get(url)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("request %d: want 200, got %d", i+1, resp.StatusCode)
		}
	}

	resp, err := client.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("want 429 (QPD limit), got %d", resp.StatusCode)
	}
}

func TestTransport_NoLimitWhenZero(t *testing.T) {
	db := testDB(t)
	rdb, cleanup := testRedis(t)
	defer cleanup()

	// QPM=0 QPD=0 → 不限制
	_ = db.Create(&model.GdtRateLimitRule{APIPathPrefix: "/unlimited/api", QPMLimit: 0, QPDLimit: 0, Enabled: 1})

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	tp, err := gdt.NewTransport(db, rdb,
		gdt.WithPubSubChannel("ch:"+t.Name()),
		gdt.WithSkipAutoMigrate(),
		gdt.WithBaseTransport(upstream.Client().Transport),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer tp.Close()
	client := &http.Client{Transport: tp}

	for i := 0; i < 10; i++ {
		resp, err := client.Get(upstream.URL + "/v3.0/unlimited/api")
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("request %d: want 200, got %d", i+1, resp.StatusCode)
		}
	}
}

func TestUnmatchedUnlimited_SkipsFallbackLimit(t *testing.T) {
	db := testDB(t)
	rdb, cleanup := testRedis(t)
	defer cleanup()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	tp, err := gdt.NewTransport(db, rdb,
		gdt.WithPubSubChannel("ch:"+t.Name()),
		gdt.WithSkipAutoMigrate(),
		gdt.WithFallbackQPM(1),
		gdt.WithUnmatchedUnlimited(),
		gdt.WithBaseTransport(upstream.Client().Transport),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer tp.Close()
	client := &http.Client{Transport: tp}
	url := upstream.URL + "/v3.0/unknown/api"

	for i := 0; i < 5; i++ {
		resp, err := client.Get(url)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("request %d: want 200 (unlimited), got %d", i+1, resp.StatusCode)
		}
	}

	var n int64
	db.Model(&model.GdtRateLimitPending{}).Where("status = ?", model.PendingStatusPending).Count(&n)
	if n != 1 {
		t.Fatalf("pending=%d want 1", n)
	}
}

func TestUnmatchedDefault_UsesFallbackLimit(t *testing.T) {
	db := testDB(t)
	rdb, cleanup := testRedis(t)
	defer cleanup()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	tp, err := gdt.NewTransport(db, rdb,
		gdt.WithPubSubChannel("ch:"+t.Name()),
		gdt.WithSkipAutoMigrate(),
		gdt.WithFallbackQPM(1),
		gdt.WithBaseTransport(upstream.Client().Transport),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer tp.Close()
	client := &http.Client{Transport: tp}
	url := upstream.URL + "/v3.0/unknown/fallback"

	resp, err := client.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("first request: want 200, got %d", resp.StatusCode)
	}

	resp, err = client.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("second request: want 429 (fallback limit), got %d", resp.StatusCode)
	}
}

func TestUnmatchedUnlimited_WithDisablePendingSave_NoPending(t *testing.T) {
	db := testDB(t)
	rdb, cleanup := testRedis(t)
	defer cleanup()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	tp, err := gdt.NewTransport(db, rdb,
		gdt.WithPubSubChannel("ch:"+t.Name()),
		gdt.WithSkipAutoMigrate(),
		gdt.WithFallbackQPM(1),
		gdt.WithUnmatchedUnlimited(),
		gdt.WithDisablePendingSave(),
		gdt.WithBaseTransport(upstream.Client().Transport),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer tp.Close()
	client := &http.Client{Transport: tp}
	url := upstream.URL + "/v3.0/no_pending/api"

	for i := 0; i < 3; i++ {
		resp, err := client.Get(url)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("request %d: want 200, got %d", i+1, resp.StatusCode)
		}
	}

	var n int64
	db.Model(&model.GdtRateLimitPending{}).Count(&n)
	if n != 0 {
		t.Fatalf("pending=%d want 0 (DisablePendingSave)", n)
	}
}

func TestAutoDiscover_WritePending(t *testing.T) {
	db := testDB(t)
	rdb, cleanup := testRedis(t)
	defer cleanup()

	rm, err := gdt.NewRuleManager(db, rdb,
		gdt.WithPubSubChannel("ch:"+t.Name()),
		gdt.WithSkipAutoMigrate(),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer rm.Close()

	_ = rm.GetRule("/new/api")

	var n int64
	db.Model(&model.GdtRateLimitPending{}).Where("status = ?", model.PendingStatusPending).Count(&n)
	if n != 1 {
		t.Fatalf("pending=%d want 1", n)
	}
}

func TestNormalizePath(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"/v3.0/videos/get", "/videos/get"},
		{"/v1.1/images/add", "/images/add"},
		{"/videos/get", "/videos/get"},
		{"/v3.0/ad/12345/detail", "/ad/{id}/detail"},
	}
	for _, tc := range cases {
		got := gdt.NormalizePath(tc.in)
		if got != tc.want {
			t.Errorf("NormalizePath(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestApprovePending(t *testing.T) {
	db := testDB(t)
	rdb, cleanup := testRedis(t)
	defer cleanup()

	rm, err := gdt.NewRuleManager(db, rdb,
		gdt.WithPubSubChannel("ch:"+t.Name()),
		gdt.WithSkipAutoMigrate(),
	)
	if err != nil {
		t.Fatal(err)
	}

	_ = rm.GetRule("/some/api")
	list, _ := rm.ListPending(context.Background())
	if err := rm.ApprovePendingAndReload(context.Background(), list[0].ID, 500, 10000); err != nil {
		t.Fatal(err)
	}
	_ = rm.Close()

	var rules int64
	db.Model(&model.GdtRateLimitRule{}).Count(&rules)
	if rules != 1 {
		t.Fatalf("rules=%d want 1", rules)
	}
}
