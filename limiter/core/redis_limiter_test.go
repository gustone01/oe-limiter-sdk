package core_test

import (
	"context"
	"testing"

	"github.com/gustone01/oe-limiter-sdk/limiter/core"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func setup(t *testing.T) (*core.RedisLimiter, func()) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return core.NewRedisLimiter(rdb), func() { _ = rdb.Close(); mr.Close() }
}

func TestAllowMultiWindow_SingleSliding(t *testing.T) {
	lim, cleanup := setup(t)
	defer cleanup()
	ctx := context.Background()

	checks := []core.WindowCheck{{
		Key: "{test}:qps", Limit: 2, WindowMS: 1000, Type: core.WindowTypeSliding,
	}}

	for i := 0; i < 2; i++ {
		ok, err := lim.AllowMultiWindow(ctx, checks)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}

	ok, err := lim.AllowMultiWindow(ctx, checks)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("3rd request should be rejected")
	}
}

func TestAllowMultiWindow_SingleCounter(t *testing.T) {
	lim, cleanup := setup(t)
	defer cleanup()
	ctx := context.Background()

	checks := []core.WindowCheck{{
		Key: "{test}:qpd", Limit: 3, WindowMS: 86400000, Type: core.WindowTypeCounter,
	}}

	for i := 0; i < 3; i++ {
		ok, err := lim.AllowMultiWindow(ctx, checks)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}

	ok, err := lim.AllowMultiWindow(ctx, checks)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("4th request should be rejected")
	}
}

func TestAllowMultiWindow_MixedSlidingAndCounter(t *testing.T) {
	lim, cleanup := setup(t)
	defer cleanup()
	ctx := context.Background()

	checks := []core.WindowCheck{
		{Key: "{test}:qpm", Limit: 5, WindowMS: 60000, Type: core.WindowTypeSliding},
		{Key: "{test}:qpd", Limit: 3, WindowMS: 86400000, Type: core.WindowTypeCounter},
	}

	for i := 0; i < 3; i++ {
		ok, err := lim.AllowMultiWindow(ctx, checks)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}

	// QPD=3 已满，即使 QPM 还有余量也应拒绝
	ok, err := lim.AllowMultiWindow(ctx, checks)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("should be rejected by QPD limit")
	}
}

func TestAllowMultiWindow_EmptyChecks(t *testing.T) {
	lim, cleanup := setup(t)
	defer cleanup()

	ok, err := lim.AllowMultiWindow(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("empty checks should always allow")
	}
}

func TestAllowMultiWindow_DoubleSliding(t *testing.T) {
	lim, cleanup := setup(t)
	defer cleanup()
	ctx := context.Background()

	checks := []core.WindowCheck{
		{Key: "{test}:qps", Limit: 10, WindowMS: 1000, Type: core.WindowTypeSliding},
		{Key: "{test}:qpm", Limit: 2, WindowMS: 60000, Type: core.WindowTypeSliding},
	}

	for i := 0; i < 2; i++ {
		ok, err := lim.AllowMultiWindow(ctx, checks)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}

	// QPM=2 已满
	ok, err := lim.AllowMultiWindow(ctx, checks)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("should be rejected by QPM limit")
	}
}
