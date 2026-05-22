package core

import "errors"

// StatusRateLimited SDK 生成的 429 响应的 Status 文本。
// 业务端可通过此常量区分 SDK 限流与平台原始 429（"429 Too Many Requests"）：
//
//	if httpResp.Status == core.StatusRateLimited { ... }
const StatusRateLimited = "429 Rate Limited"

// 哨兵错误，业务端可用 errors.Is 判断。
var (
	ErrPendingAlreadyReviewed = errors.New("pending record already reviewed")
	ErrPendingNotPending      = errors.New("pending record is not in pending status")
)
