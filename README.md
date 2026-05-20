# oe-limiter-sdk

多服务架构下的分布式 API 限流 SDK：MySQL 持久化规则 + Redis Pub/Sub 多实例同步 + 本地 `sync.Map` 缓存，通过包装 `http.RoundTripper` 对出站 HTTP 请求限流。

- **Module**：`192.168.10.236/gustone/oe-limiter-sdk`
- **仓库**：`http://192.168.10.236:3000/gustone/oe-limiter-sdk.git`

## 快速初始化

```powershell
# 1. 配置私有 Go 模块 + 拉依赖
.\scripts\setup.ps1

# 2. 建表（MySQL）
mysql -u user -p your_db < schema.sql

# 3. 跑测试
go test ./... -v

# 4. 本地示例（需 MySQL/Redis，参考 .env.example）
cd examples/event_client
go run .
```

## 功能

- 三级存储：L1 本地缓存 / L2 Redis Pub/Sub / L3 MySQL
- 服务专属 + 全局 `ALL` 双重 QPS 校验（Redis Lua 原子脚本）
- **滑动窗口限流（默认）**：ZSET + 原子递增序号保证高并发 member 唯一，对齐平台「当前时刻倒推 1 秒」统计
- **平台 40110 惩罚封禁**：响应路径识别开发者频控 `40110`，按 `X-RateLimit-RetryIn` 写入 Redis **接口级**封禁 Key（同路径所有 service 共享）；后续请求路径直接拦截（写入失败仅日志降级，不阻断正常响应）
- **响应路径按需解析**：仅在 HTTP 429 / `X-RateLimit-RetryIn` / `X-RateLimit-Dimension` 头存在时才读取 body，正常请求零额外 IO
- API 路径数字段归一化（`/123` → `/{id}`）
- 最长前缀匹配规则
- 未配置接口自动发现 → 写入 **`oe_rate_limit_pending` 待审核表**（兜底 QPS）+ 可选回调告警
- **单机滑动窗口工具类**：`SlidingWindowLimiter` 可用于不依赖 Redis 的单进程限流场景（SDK 内部不使用）

### 与巨量开放平台频控文档的关系

| 官方文档 | 说明 |
|----------|------|
| [频控限制说明](https://open.oceanengine.com/labels/7/docs/1696710758159360) | 三层频控：客户 `40130`、开发者 `40110`、接口总 `40100`；**本 SDK 仅处理 `40110`** |
| [基础实践：频控限流器开发](https://open.oceanengine.com/labels/7/docs/1696710758840332) | 滑动窗口 + MQ 削峰；本 SDK 实现滑动窗口与封禁逻辑，MQ 由业务侧组合 |
| [频控管理后台](https://open.oceanengine.com/developer/admin/qps) | **各接口 QPS 配额以此为准**，写入 `oe_rate_limit_rules` 时宜留 10%～20% 余量 |

> **为什么只处理 40110？**
>
> - `40110`（开发者频控）由 QPM 超限触发，带完整的 `X-RateLimit-RetryIn` / `X-RateLimit-Dimension` 头，SDK 可精确封禁 + 自动恢复。
> - `40100`（接口总频控）由系统动态调整、非固定值，非单个开发者可控，触发后短暂重试即可，无需持久封禁。
> - `40130`（客户频控）按 `advertiser_id` 维度限制，需业务侧按广告主粒度处理，SDK 层统一封禁反而会误伤其他广告主的请求。

### 限流路径设计（请求 vs 响应）

```
出站请求
  → [请求路径] 40110 接口级封禁检查（Redis Key: oe:limit:penalty:dev:{path}）
  → [请求路径] 滑动窗口 QPS（Redis ZSET 原子计数，自限避免打满配额）
  → 调用开放平台 API
  → [响应路径] 仅当 429 / X-RateLimit-* 头存在时解析 body，识别 40110 写入封禁 TTL
  → [响应路径] 封禁写入失败仅日志降级，不影响本次已成功的响应
  → 返回业务
```

| 能力 | 路径 | 说明 |
|------|------|------|
| 滑动窗口 QPS | **请求**（出站前） | 默认开启；`WithLimiterMode(limiter.LimiterModeSliding)` |
| 固定窗口 QPS | **请求** | 旧算法：`WithLimiterMode(limiter.LimiterModeFixed)` |
| 40110 开发者封禁 | **响应写入 / 请求检查** | 收到 `40110` 后按 `RetryIn` 写入接口级封禁，期间同路径所有 service 直接 `429` |

## 安装

```bash
go env -w GOPRIVATE=192.168.10.236
# 将 module 路径映射到 Gitea 实际地址（含 3000 端口）
git config --global url."http://192.168.10.236:3000/".insteadOf "https://192.168.10.236/"
go get 192.168.10.236/gustone/oe-limiter-sdk@latest
```

私有仓库需配置 HTTP 凭据（示例 `~/.netrc`）：

```text
machine 192.168.10.236
login your_username
password your_token
```

## 数据库（SDK 自动建表）

业务服务 **无需手动建表**。首次调用 `NewTransport` / `NewRuleManager` 时，SDK 会自动创建/更新：

- `oe_rate_limit_rules` — 正式限流规则
- `oe_rate_limit_pending` — 自动发现待审核

```go
// 内部自动执行 AutoMigrate，业务代码只要：
transport, err := limiter.NewTransport("event", db, rdb)
```

生产环境若由 DBA 统一执行 [schema.sql](./schema.sql)，可关闭自动建表：

```go
limiter.NewTransport("event", db, rdb, limiter.WithSkipAutoMigrate())
```

示例数据：

| service_name | api_path_prefix | qps_limit | is_shared |
|--------------|-----------------|-----------|-----------|
| event | /open_api/v3.0/event/track/ | 200 | 0 |
| data | /open_api/v3.0/report/get/ | 50 | 0 |
| click | /open_api/v3.0/tools/click_track/ | 30 | 0 |
| gy | /open_api/v3.0/advertiser/update/ | 10 | 0 |
| ALL | /open_api/ | 500 | 1 |

### 待审核表（自动发现）

| 表名 | 说明 |
|------|------|
| **`oe_rate_limit_pending`** | 自动发现的新接口，`status=0` 待审核 |

| 字段 | 说明 |
|------|------|
| service_name | 服务名，如 event |
| api_path_prefix | 归一化路径，如 `/open_api/v3.0/foo/{id}` |
| suggested_qps | 建议配额，默认 5 |
| status | 0 待审核 / 1 已通过 / 2 已拒绝 |

审核通过（写入正式表并广播刷新）：

```go
_ = rm.ApprovePendingAndReload(ctx, pendingID, 200) // 200 为最终 QPS，传 0 则用 suggested_qps
```

拒绝：

```go
_ = rm.RejectPending(ctx, pendingID, "路径不需要限流")
```

查询待审核列表：

```go
list, _ := rm.ListPending(ctx)
```

## 使用

```go
import (
    "net/http"
    "time"

    "192.168.10.236/gustone/oe-limiter-sdk/limiter"
    "github.com/redis/go-redis/v9"
    "gorm.io/gorm"
)

func main() {
    transport, err := limiter.NewTransport("event", db, rdb,
        // 默认已是滑动窗口；可选显式指定：
        // limiter.WithLimiterMode(limiter.LimiterModeSliding),
        // limiter.WithWindowMS(1000),
        limiter.WithOnDiscover(func(svc, path string) {
            // 告警：发现未配置接口
        }),
        limiter.WithOnPlatformRateLimit(func(svc, path string, code int, wait time.Duration) {
            // 响应路径：平台返回 40110 后触发，可打点/告警
        }),
    )
    if err != nil {
        panic(err)
    }

    defer transport.Close()

    client := &http.Client{Transport: transport}
    _, _ = client.Get("https://api.example.com/open_api/v3.0/event/track/1")
}
```

### 可选配置

| 选项 | 默认值 | 说明 |
|------|--------|------|
| `WithLimiterMode` | `sliding` | `sliding` 滑动窗口（推荐）/ `fixed` 固定秒窗口 |
| `WithWindowMS` | `1000` | 滑动窗口长度（毫秒） |
| `WithDisablePlatformPenalty` | `false` | 为 `true` 时不解析平台频控、不写封禁 Key |
| `WithPlatformPenaltyTTL` | `5m` | 40110 未返回 `X-RateLimit-RetryIn` 时的默认封禁时长 |
| `WithOnPlatformRateLimit` | — | 平台惩罚触发时的回调 |

### 直接使用 RedisLimiter（高级用法）

如需在自定义逻辑中直接调用限流判断，可通过 `AllowRequest` 结构体传参：

```go
ok, err := rm.Limiter().AllowDoubleCheck(ctx, limiter.AllowRequest{
    ServiceKey:   limiter.RedisKey("event", "/open_api/v3.0/event/track/"),
    GlobalKey:    limiter.RedisKey("ALL", "/open_api/v3.0/event/track/"),
    ServiceLimit: 200,
    GlobalLimit:  500,
    WindowSec:    1,
    WindowMS:     1000,
    Mode:         limiter.LimiterModeSliding,
})
```

### 单机滑动窗口（不依赖 Redis）

`SlidingWindowLimiter` 是进程内工具类，适用于单元测试或无 Redis 的单机场景：

```go
lim := limiter.NewSlidingWindowLimiter(time.Second, 100) // 1 秒窗口，100 QPS
if lim.Allow() {
    // 放行
}
```

### MQ 消费者场景（不经过 HTTP RoundTripper）

批量/报表等异步任务可在消费协程内复用同一套限流：

```go
import "context"

tp, _ := limiter.NewTransport("event", db, rdb)
defer tp.Close()

for msg := range queue {
    ok, err := tp.Allow(context.Background(), msg.APIPath)
    if err != nil || !ok {
        // 重新入队或写入死信
        continue
    }
    // CallTargetAPI(msg)
}
```

`Allow` 同样走请求路径的滑动窗口；平台封禁需在 HTTP 调用后由业务解析响应，或统一走 `http.Client{Transport: tp}`。

管理后台更新规则后，发布刷新通知：

```go
rm, _ := limiter.NewRuleManager(db, rdb)
_ = rm.PublishRuleUpdate(ctx)
```

多个服务共享同一 `RuleManager` 时：

```go
rm, _ := limiter.NewRuleManager(db, rdb)
eventTP := limiter.NewTransportWithManager("event", rm)
dataTP := limiter.NewTransportWithManager("data", rm)
```

## 429 处理建议

SDK 可能返回两类 `429`（均带 `Retry-After` 时建议遵守）：

| 来源 | 含义 | 建议 |
|------|------|------|
| 本地滑动窗口 | 自限 QPS，未发起平台请求 | 退避后重试；检查 `oe_rate_limit_rules` 是否高于[频控后台](https://open.oceanengine.com/developer/admin/qps)配额 |
| 40110 接口级封禁 | 上次响应触发开发者频控惩罚，请求路径拦截 | 等待 `Retry-After` / 封禁 TTL 结束后再打（通常 5 分钟） |

通用实践：

- 打点 `rate_limit_blocked_total`（区分 `local` / `platform_penalty`）
- 指数退避 + jitter；尊重 `X-RateLimit-RetryIn`
- 写操作可写入 MQ 异步削峰（参见官方基础实践文档）
- 封禁写入 Redis 失败时 SDK 仅打日志（`[oe-limiter] apply 40110 penalty failed`），不影响本次已成功的业务响应

## 监控建议

| 指标 | 阈值 | 动作 |
|------|------|------|
| 本地限流拦截率 | >5% | 检查 QPS 配置或下调 `qps_limit` |
| 40110 封禁触发次数 | 持续升高 | 降频、错峰；查频控后台配额 |
| 429 错误率 | >1% | 下调配额或扩容 |
| Redis 命中率 | <90% | 检查路径匹配 |

## 其他服务如何接入与测试

### 1. 引入依赖

在 **event / click / data / gy** 等业务服务的 `go.mod` 中：

```bash
go env -w GOPRIVATE=192.168.10.236
go get 192.168.10.236/gustone/oe-limiter-sdk@latest
```

本地联调 SDK 源码（未发布版本）：

```go
// go.mod
require 192.168.10.236/gustone/oe-limiter-sdk v0.0.0

replace 192.168.10.236/gustone/oe-limiter-sdk => ../oe-limiter-sdk
```

### 2. 业务代码接入（与生产一致）

```go
transport, _ := limiter.NewTransport("event", db, rdb)
defer transport.Close()
client := &http.Client{Transport: transport}
resp, _ := client.Get("https://开放平台/open_api/v3.0/event/track/123")
```

完整可运行示例见：[examples/event_client](./examples/event_client/main.go)

### 3. 业务侧单测（推荐，无需真实 MySQL/Redis）

使用 **SQLite 内存库 + miniredis**，参考：

| 文件 | 说明 |
|------|------|
| [limiter/integration_test.go](./limiter/integration_test.go) | 集成测试：限流 / 自动发现 / 审核 / 全局限流 / 40110 端到端封禁（含跨 service 拦截）/ Close 幂等 |
| [limiter/redis_sliding_test.go](./limiter/redis_sliding_test.go) | Redis ZSET 滑动窗口双重限流单测 |
| [limiter/sliding_window_test.go](./limiter/sliding_window_test.go) | 进程内滑动窗口单测 |
| [limiter/penalty_test.go](./limiter/penalty_test.go) | 40110 封禁单测（写入/检查/跨 service/忽略 40100&40130） |
| [limiter/reload_test.go](./limiter/reload_test.go) | 规则禁用后缓存清除 / Transport Close 幂等 |
| [examples/event_client/main_test.go](./examples/event_client/main_test.go) | 业务服务可复制的最小测试模板 |

在业务仓库执行：

```bash
go test ./... -v
```

在 SDK 仓库执行全部测试：

```bash
go test ./... -v
```

### 4. 连真实环境冒烟

```bash
cd examples/event_client
set MYSQL_DSN=user:pass@tcp(127.0.0.1:3306)/db?charset=utf8mb4&parseTime=True
set REDIS_ADDR=127.0.0.1:6379
go run .
```

## 开发

```bash
go test ./... -v
# 含 examples：
cd examples/event_client && go test ./...
```

或使用 Makefile：`make test`
