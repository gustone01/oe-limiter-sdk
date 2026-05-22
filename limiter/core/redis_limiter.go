package core

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

var memberSeq uint64

// allowMultiWindowLua 多维度原子限流脚本（ZSET 滑动窗口 + INCR 计数器混合）。
//
// KEYS[1..N]  各维度 Key（通过 hash tag 保证同一 Redis Cluster slot）
// ARGV 每组 3 个: (limit, windowMs, type)  type="sliding"|"counter"
// ARGV[3N+1]  nowMs
// ARGV[3N+2]  member（仅 sliding 使用，保证高并发唯一性）
const allowMultiWindowLua = `
local n = #KEYS
local now = tonumber(ARGV[n*3 + 1])
local member = ARGV[n*3 + 2]

for i = 1, n do
  local limit  = tonumber(ARGV[(i-1)*3 + 1])
  local window = tonumber(ARGV[(i-1)*3 + 2])
  local wtype  = ARGV[(i-1)*3 + 3]
  if wtype == "sliding" then
    redis.call('ZREMRANGEBYSCORE', KEYS[i], 0, now - window)
    if redis.call('ZCARD', KEYS[i]) >= limit then return 0 end
  else
    local cnt = tonumber(redis.call('GET', KEYS[i]) or "0")
    if cnt >= limit then return 0 end
  end
end

for i = 1, n do
  local window = tonumber(ARGV[(i-1)*3 + 2])
  local wtype  = ARGV[(i-1)*3 + 3]
  if wtype == "sliding" then
    redis.call('ZADD', KEYS[i], now, member .. ':' .. i)
    redis.call('PEXPIRE', KEYS[i], window)
  else
    redis.call('INCR', KEYS[i])
    if redis.call('PTTL', KEYS[i]) < 0 then
      redis.call('PEXPIRE', KEYS[i], window)
    end
  end
end
return 1
`

const (
	WindowTypeSliding = "sliding" // ZSET 滑动窗口，适用于 QPS/QPM（短窗口）
	WindowTypeCounter = "counter" // INCR 计数器，适用于 QPD（长窗口，内存友好）
)

// WindowCheck 描述一个维度的限流检查参数。
type WindowCheck struct {
	Key      string // 带 hash tag 的 Redis Key
	Limit    int    // 配额上限
	WindowMS int64  // 窗口长度（毫秒）
	Type     string // WindowTypeSliding 或 WindowTypeCounter
}

// RedisLimiter 封装 Redis + Lua，在分布式环境下对请求做原子多维度限流。
type RedisLimiter struct {
	rdb    *redis.Client
	script *redis.Script
}

// NewRedisLimiter 创建基于 Redis 的多维度限流器。
func NewRedisLimiter(rdb *redis.Client) *RedisLimiter {
	return &RedisLimiter{
		rdb:    rdb,
		script: redis.NewScript(allowMultiWindowLua),
	}
}

// AllowMultiWindow 原子执行多维度限流检查，任一维度超限则返回 false。
// checks 中各 Key 应使用 HashTagKey 生成以保证 Redis Cluster 兼容。
func (l *RedisLimiter) AllowMultiWindow(ctx context.Context, checks []WindowCheck) (bool, error) {
	if len(checks) == 0 {
		return true, nil
	}

	now := time.Now().UnixMilli()
	seq := atomic.AddUint64(&memberSeq, 1)
	member := fmt.Sprintf("%d-%d", now, seq)

	keys := make([]string, len(checks))
	args := make([]interface{}, 0, len(checks)*3+2)
	for i, c := range checks {
		keys[i] = c.Key
		args = append(args, c.Limit, c.WindowMS, c.Type)
	}
	args = append(args, now, member)

	n, err := l.script.Run(ctx, l.rdb, keys, args...).Int()
	if err != nil {
		return false, err
	}
	return n == 1, nil
}
