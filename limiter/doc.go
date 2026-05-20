// Package limiter 提供基于 MySQL + Redis + 本地缓存的分布式 HTTP 出站限流能力。
//
// 典型用法：用 NewTransport 包装 http.Client，在调用开放平台 API 前自动做 QPS 校验。
//
// 限流分两条路径：
//   - 请求路径：滑动窗口（默认，对齐平台「当前时刻倒推 1 秒」）+ 已有 40110 封禁则直接 429；
//   - 响应路径：解析 40110（开发者 QPM 惩罚）与 X-RateLimit-RetryIn，写入 Redis 接口级封禁。
//
// 规则在 MySQL 中配置，通过 Redis Pub/Sub 在多实例间同步，本地 sync.Map 做热缓存。
package limiter
