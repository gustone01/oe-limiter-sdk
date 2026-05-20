package limiter

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

var memberSeq uint64

// doubleCheckLua Redis Lua 脚本：在同一原子操作中完成「服务专属 + 全局」双重计数校验。
//
// KEYS[1] 服务专属计数 Key，KEYS[2] 全局计数 Key；
// ARGV[1] 服务 QPS 上限，ARGV[2] 全局 QPS 上限，ARGV[3] 窗口秒数。
// 返回值：1=允许通过并已 INCR，0=任一配额已满需拒绝。
const doubleCheckLua = `
-- 固定窗口限流：window 秒内计数不超过 limit 即放行
local serviceLimit = tonumber(ARGV[1])
local globalLimit = tonumber(ARGV[2])
local window = tonumber(ARGV[3])

-- 第一关：本服务（如 event）专属配额
local serviceCount = redis.call('GET', KEYS[1])
if not serviceCount then serviceCount = 0 else serviceCount = tonumber(serviceCount) end
if serviceCount >= serviceLimit then
  return 0  -- 服务配额已满
end

-- 第二关：全局 ALL 共享配额（防止多服务合计打爆平台）
local globalCount = redis.call('GET', KEYS[2])
if not globalCount then globalCount = 0 else globalCount = tonumber(globalCount) end
if globalCount >= globalLimit then
  return 0  -- 全局配额已满
end

-- 两关都通过：服务计数和全局计数各 +1
redis.call('INCR', KEYS[1])
redis.call('INCR', KEYS[2])

-- 窗口内首次请求时设置过期，形成「每 window 秒重置」的固定窗口
if serviceCount == 0 then
  redis.call('EXPIRE', KEYS[1], window)
end
if globalCount == 0 then
  redis.call('EXPIRE', KEYS[2], window)
end
return 1  -- 允许
`

// slidingDoubleCheckLua 滑动窗口：统计 [now-windowMs, now] 内请求数，与平台「倒推 1 秒」一致。
// KEYS[1]/[2] 为 ZSET；ARGV: serviceLimit, globalLimit, windowMs, nowMs, member
const slidingDoubleCheckLua = `
local window = tonumber(ARGV[3])
local now = tonumber(ARGV[4])
local member = ARGV[5]

local function count_ok(key, limit)
  redis.call('ZREMRANGEBYSCORE', key, 0, now - window)
  return redis.call('ZCARD', key) < limit
end

local sLimit = tonumber(ARGV[1])
local gLimit = tonumber(ARGV[2])

if not count_ok(KEYS[1], sLimit) then
  return 0
end
if not count_ok(KEYS[2], gLimit) then
  return 0
end

redis.call('ZADD', KEYS[1], now, member)
redis.call('ZADD', KEYS[2], now, member .. ':g')
redis.call('PEXPIRE', KEYS[1], window)
redis.call('PEXPIRE', KEYS[2], window)
return 1
`

// RedisLimiter 封装 Redis + Lua，在分布式环境下对请求做原子计数与放行判断。
type RedisLimiter struct {
	rdb            *redis.Client
	fixedScript    *redis.Script
	slidingScript  *redis.Script
}

// NewRedisLimiter 创建基于 Redis 的限流计数器。
func NewRedisLimiter(rdb *redis.Client) *RedisLimiter {
	return &RedisLimiter{
		rdb:           rdb,
		fixedScript:   redis.NewScript(doubleCheckLua),
		slidingScript: redis.NewScript(slidingDoubleCheckLua),
	}
}

// AllowRequest 封装双重限流所需的全部参数。
type AllowRequest struct {
	ServiceKey   string
	GlobalKey    string
	ServiceLimit int
	GlobalLimit  int
	WindowSec    int
	WindowMS     int
	Mode         string
}

// AllowDoubleCheck 执行双重限流：先检查服务专属配额，再检查全局（ALL）配额。
// 返回 true 表示允许本次请求并已累加计数。
func (l *RedisLimiter) AllowDoubleCheck(ctx context.Context, r AllowRequest) (bool, error) {
	if r.Mode == LimiterModeFixed {
		n, err := l.fixedScript.Run(ctx, l.rdb, []string{r.ServiceKey, r.GlobalKey},
			r.ServiceLimit, r.GlobalLimit, r.WindowSec,
		).Int()
		if err != nil {
			return false, err
		}
		return n == 1, nil
	}
	ms := int64(r.WindowMS)
	if ms < 1000 {
		ms = 1000
	}
	return l.allowSliding(ctx, r.ServiceKey, r.GlobalKey, r.ServiceLimit, r.GlobalLimit, ms)
}

func slidingZKey(key string) string {
	const p = "oe:limit:"
	if strings.HasPrefix(key, p) {
		return "oe:limit:sw:" + key[len(p):]
	}
	return "oe:limit:sw:" + key
}

func (l *RedisLimiter) allowSliding(
	ctx context.Context,
	serviceKey, globalKey string,
	serviceLimit, globalLimit int,
	windowMS int64,
) (bool, error) {
	now := time.Now().UnixMilli()
	seq := atomic.AddUint64(&memberSeq, 1)
	member := fmt.Sprintf("%d-%d", now, seq)
	n, err := l.slidingScript.Run(ctx, l.rdb, []string{slidingZKey(serviceKey), slidingZKey(globalKey)},
		serviceLimit, globalLimit, windowMS, now, member,
	).Int()
	if err != nil {
		return false, err
	}
	return n == 1, nil
}

// RedisKey 生成 Redis 中存储请求计数的 Key，格式：oe:limit:{服务名}:{归一化路径}。
func RedisKey(serviceName, apiPath string) string {
	return "oe:limit:" + serviceName + ":" + apiPath
}
