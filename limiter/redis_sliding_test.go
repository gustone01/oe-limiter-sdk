package limiter

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestRedisLimiter_SlidingDoubleCheck(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	lim := NewRedisLimiter(rdb)
	ctx := context.Background()

	req := AllowRequest{
		ServiceKey: "oe:limit:event:/a", GlobalKey: "oe:limit:ALL:/a",
		ServiceLimit: 2, GlobalLimit: 100, WindowSec: 1, WindowMS: 1000, Mode: LimiterModeSliding,
	}
	for i := 0; i < 2; i++ {
		ok, err := lim.AllowDoubleCheck(ctx, req)
		if err != nil || !ok {
			t.Fatalf("i=%d ok=%v err=%v", i, ok, err)
		}
	}
	ok, err := lim.AllowDoubleCheck(ctx, req)
	if err != nil || ok {
		t.Fatalf("third should fail ok=%v err=%v", ok, err)
	}
}
