package core_test

import (
	"testing"
	"time"

	"192.168.10.236/gustone/oe-limiter-sdk/limiter/core"
)

func TestSlidingWindowLimiter_Basic(t *testing.T) {
	lim := core.NewSlidingWindowLimiter(time.Second, 3)
	for i := 0; i < 3; i++ {
		if !lim.Allow() {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
	if lim.Allow() {
		t.Fatal("4th request should be rejected")
	}
}

func TestSlidingWindowLimiter_WindowExpiry(t *testing.T) {
	lim := core.NewSlidingWindowLimiter(50*time.Millisecond, 1)
	if !lim.Allow() {
		t.Fatal("first should be allowed")
	}
	if lim.Allow() {
		t.Fatal("second should be rejected")
	}
	time.Sleep(60 * time.Millisecond)
	if !lim.Allow() {
		t.Fatal("after window should be allowed again")
	}
}
