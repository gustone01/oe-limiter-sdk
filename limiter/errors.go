package limiter

import "errors"

// RejectReason 标识 SDK 生成的 429 响应来源，业务端通过 Header 获取：
//
//	resp.Header.Get("X-Limiter-Reject-Reason")
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
