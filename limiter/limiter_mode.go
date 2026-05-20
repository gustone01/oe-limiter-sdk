package limiter

// 限流计数算法：默认滑动窗口，与巨量开放平台「当前时刻倒推 1 秒」统计方式对齐。
const (
	LimiterModeSliding = "sliding" // 滑动窗口
	LimiterModeFixed   = "fixed"   // 固定窗口
)
