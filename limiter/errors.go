package limiter

import "errors"

// StatusRateLimited SDK 生成的 429 响应的 Status 文本。
// 业务端可通过此常量区分 SDK 限流与平台原始 429（"429 Too Many Requests"）：
//
//	if httpResp.Status == limiter.StatusRateLimited { ... }
const StatusRateLimited = "429 Rate Limited"

// RejectReason 标识 SDK 生成的 429 响应来源。
//
// 获取方式：
//   - Header: resp.Header.Get(limiter.HeaderRejectReason)
//   - Body JSON: {"reason":"local","retry_after":0} 或 {"reason":"platform_40110","retry_after":300}
const (
	RejectReasonLocal   = "local"          // 本地滑动窗口 QPS 超限
	RejectReasonPenalty = "platform_40110" // 平台 40110 封禁期拦截
)

// HeaderRejectReason SDK 在 429 响应中写入拒绝原因的自定义 Header 名。
const HeaderRejectReason = "X-Limiter-Reject-Reason"

// 哨兵错误，业务端可用 errors.Is 判断。
var (
	ErrPendingAlreadyReviewed = errors.New("pending record already reviewed")
	ErrPendingNotPending      = errors.New("pending record is not in pending status")
)
