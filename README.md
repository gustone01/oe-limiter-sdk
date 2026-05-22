# oe-limiter-sdk

多平台分布式 API 限流 SDK（Go），通过包装 `http.RoundTripper` 对出站 HTTP 请求限流。

## 安装

```bash
go env -w GOPRIVATE=github.com/gustone01/*
go get github.com/gustone01/oe-limiter-sdk@latest
```

## 快速开始

```go
// 巨量引擎
transport, _ := oe.NewTransport(db, rdb)
client := &http.Client{Transport: transport}

// 腾讯广告
transport, _ := gdt.NewTransport(db, rdb)
client := &http.Client{Transport: transport}
```
