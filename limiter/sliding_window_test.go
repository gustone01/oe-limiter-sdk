package limiter

import (
	"testing"
	"time"
)

func TestSlidingWindowLimiter_BurstAtBoundary(t *testing.T) {
	lim := NewSlidingWindowLimiter(time.Second, 3)
	for i := 0; i < 3; i++ {
		if !lim.Allow() {
			t.Fatalf("request %d should pass", i+1)
		}
	}
	if lim.Allow() {
		t.Fatal("4th request in same second should be denied")
	}
}

func TestSlidingWindowLimiter_WindowSlides(t *testing.T) {
	lim := NewSlidingWindowLimiter(200*time.Millisecond, 2)
	if !lim.Allow() || !lim.Allow() {
		t.Fatal("first two should pass")
	}
	if lim.Allow() {
		t.Fatal("third should fail")
	}
	time.Sleep(220 * time.Millisecond)
	if !lim.Allow() {
		t.Fatal("after window slide should pass again")
	}
}
