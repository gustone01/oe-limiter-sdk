# oe-limiter-sdk 开发指南

多平台分布式 API 限流 SDK：支持巨量引擎（QPS+QPM）和腾讯广告（QPM+QPD），MySQL 持久化规则 + Redis Pub/Sub 多实例同步 + 本地缓存，通过包装 `http.RoundTripper` 对出站 HTTP 请求限流。

## 架构

```
limiter/
├── core/          通用底层（AllowMultiWindow Lua、Response、Errors、SlidingWindow）
├── oe/            巨量引擎（QPS 滑动窗口 + QPM=QPS*100 自动派生）
└── gdt/           腾讯广告（QPM 滑动窗口 + QPD 计数器）

model/             GORM 数据模型（oe_rate_limit_* + gdt_rate_limit_*）
```

## 功能

- 三级存储：L1 本地缓存 / L2 Redis Pub/Sub / L3 MySQL
- **多维度限流**：单个原子 Lua 脚本同时检查多个维度（QPS/QPM/QPD）
- **混合算法**：QPS/QPM 使用 ZSET 滑动窗口（对齐平台统计），QPD 使用 INCR 计数器（内存友好）
- **Redis Cluster 兼容**：Hash Tag 保证同一接口的多维度 Key 落在同一 slot
- API 路径归一化（巨量：保留版本号去长数字段；腾讯：去版本前缀）
- 最长前缀匹配规则
- 未配置接口自动发现 → 写入待审核表 + 可选回调告警
- **单机滑动窗口工具类**：`core.SlidingWindowLimiter` 可用于不依赖 Redis 的单进程场景

### 两平台对比

| | 巨量引擎 (oe) | 腾讯广告 (gdt) |
|------|--------------|---------------|
| 限流维度 | QPS + QPM | QPM + QPD |
| QPM 来源 | QPS * 100 自动派生 | 独立配置 |
| QPD 算法 | 无 | INCR 计数器 |
| 路径归一化 | 保留版本号，去 5 位+数字 | 去 `/vX.Y/` 前缀 |
| 规则表 | `oe_rate_limit_rules` | `gdt_rate_limit_rules` |

## 数据库

业务服务**无需手动建表**，首次调用 `NewTransport` 时 SDK 自动创建。

生产环境若由 DBA 统一执行 DDL + 种子数据：

```bash
mysql -u user -p your_db < sql/schema.sql
mysql -u user -p your_db < sql/seed_oe_rules.sql
mysql -u user -p your_db < sql/seed_gdt_rules.sql
```

关闭自动建表：

```go
oe.NewTransport(db, rdb, oe.WithSkipAutoMigrate())
```

### 巨量引擎表（320 条规则）

| api_path_prefix | qps_limit |
|-----------------|-----------|
| /open_api/v3.0/event/track/ | 200 |
| /open_api/v3.0/report/get/ | 50 |
| /open_api/ | 500 |

完整规则见 [sql/seed_oe_rules.sql](../sql/seed_oe_rules.sql)。

### 腾讯广告表（858 条规则）

| api_path_prefix | qpm_limit | qpd_limit |
|-----------------|-----------|-----------|
| /adgroups/get | 2000 | 0 |
| /custom_audience_files/add | 50 | 7000 |
| /wechat_channels_authorization/get | 1000 | 1440000 |

完整规则见 [sql/seed_gdt_rules.sql](../sql/seed_gdt_rules.sql)。

## 使用

### 巨量引擎

```go
import "github.com/gustone01/oe-limiter-sdk/limiter/oe"

transport, err := oe.NewTransport(db, rdb,
    oe.WithOnDiscover(func(path string) {
        log.Printf("发现未配置接口: %s", path)
    }),
)
if err != nil {
    panic(err)
}
defer transport.Close()

client := &http.Client{Transport: transport}
resp, _ := client.Get("https://api.example.com/open_api/v3.0/event/track/123")
```

### 腾讯广告

```go
import "github.com/gustone01/oe-limiter-sdk/limiter/gdt"

transport, err := gdt.NewTransport(db, rdb,
    gdt.WithFallbackQPM(100),
    gdt.WithFallbackQPD(10000),
)
if err != nil {
    panic(err)
}
defer transport.Close()

client := &http.Client{Transport: transport}
resp, _ := client.Get("https://api.e.qq.com/v3.0/videos/get?...")
```

### 可选配置

#### 巨量引擎 (oe)

| 选项 | 默认值 | 说明 |
|------|--------|------|
| `WithFallbackQPS` | `5` | 未匹配规则时的兜底 QPS |
| `WithPubSubChannel` | `oe:limit:rules:updated` | 规则变更广播频道 |
| `WithOnDiscover` | -- | 发现未配置接口时的回调 |
| `WithSkipAutoMigrate` | `false` | 跳过自动建表 |

#### 腾讯广告 (gdt)

| 选项 | 默认值 | 说明 |
|------|--------|------|
| `WithFallbackQPM` | `100` | 未匹配规则时的兜底 QPM |
| `WithFallbackQPD` | `0` | 未匹配规则时的兜底 QPD（0=不限） |
| `WithPubSubChannel` | `gdt:limit:rules:updated` | 规则变更广播频道 |

### 管理后台

审核通过（写入正式表并广播刷新）：

```go
rm := transport.Manager()
_ = rm.ApprovePendingAndReload(ctx, pendingID, 200) // 200 为最终 QPS
```

查询待审核列表：

```go
list, _ := rm.ListPending(ctx)
```

规则变更后通知所有实例：

```go
_ = rm.PublishRuleUpdate(ctx)
```

### 单机滑动窗口（不依赖 Redis）

```go
import "github.com/gustone01/oe-limiter-sdk/limiter/core"

lim := core.NewSlidingWindowLimiter(time.Second, 100) // 1 秒窗口，100 QPS
if lim.Allow() {
    // 放行
}
```

## 429 处理建议

SDK 在本地限流超限时返回 `429`（`Status: "429 Rate Limited"`），此时请求未发起。

```go
import "github.com/gustone01/oe-limiter-sdk/limiter/core"

if resp.Status == core.StatusRateLimited {
    // SDK 限流拦截，退避后重试
}
```

通用实践：

- 打点 `rate_limit_blocked_total` 监控拦截率
- 指数退避 + jitter 重试
- 写操作可写入 MQ 异步削峰

## 测试

| 文件 | 说明 |
|------|------|
| [limiter/core/redis_limiter_test.go](../limiter/core/redis_limiter_test.go) | AllowMultiWindow Lua 单测 |
| [limiter/core/sliding_window_test.go](../limiter/core/sliding_window_test.go) | 进程内滑动窗口单测 |
| [limiter/oe/oe_test.go](../limiter/oe/oe_test.go) | 巨量引擎集成测试 |
| [limiter/gdt/gdt_test.go](../limiter/gdt/gdt_test.go) | 腾讯广告集成测试 |

```bash
go test ./limiter/... -v
```
