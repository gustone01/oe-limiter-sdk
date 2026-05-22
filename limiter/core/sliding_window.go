package core

import (
	"sync"
	"time"
)

// TimeSlice 毫秒级时间片。
type TimeSlice struct {
	Timestamp int64
	Count     int64
}

// SlidingWindowLimiter 进程内滑动窗口限流器，适用于单进程/单机场景或单元测试。
// SDK 的分布式限流走 RedisLimiter（Lua 脚本），此类型作为工具类导出供
// 业务侧在不依赖 Redis 的场景下独立使用。
type SlidingWindowLimiter struct {
	windowMS    int64
	maxRequests int64
	slices      []*TimeSlice
	mu          sync.Mutex
}

// NewSlidingWindowLimiter 创建滑动窗口限流器。
// window 通常为 1s，maxRequests 为窗口内允许次数（QPS）。
func NewSlidingWindowLimiter(window time.Duration, maxRequests int64) *SlidingWindowLimiter {
	ms := window.Milliseconds()
	if ms < 1 {
		ms = 1000
	}
	if maxRequests < 1 {
		maxRequests = 1
	}
	return &SlidingWindowLimiter{
		windowMS:    ms,
		maxRequests: maxRequests,
	}
}

// Allow 当前时刻往前 window 内请求数未达上限则放行并计数。
func (l *SlidingWindowLimiter) Allow() bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now().UnixMilli()
	cutoff := now - l.windowMS

	kept := l.slices[:0]
	var total int64
	for _, s := range l.slices {
		if s.Timestamp < cutoff {
			continue
		}
		kept = append(kept, s)
		total += s.Count
	}
	l.slices = kept

	if total >= l.maxRequests {
		return false
	}

	n := len(l.slices)
	if n > 0 && l.slices[n-1].Timestamp == now {
		l.slices[n-1].Count++
	} else {
		l.slices = append(l.slices, &TimeSlice{Timestamp: now, Count: 1})
	}
	return true
}
