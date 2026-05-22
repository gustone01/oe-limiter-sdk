package oe

// Rule 从数据库加载到内存后的巨量引擎规则快照。
type Rule struct {
	// APIPathPrefix 归一化后的 API 路径前缀，用于最长前缀匹配。
	APIPathPrefix string
	// QPSLimit 每秒允许的最大请求数（QPM=QPSLimit*100 自动派生）。
	QPSLimit int
	// Enabled 规则是否启用。
	Enabled bool
}

// DefaultRule 返回兜底规则。
func DefaultRule(fallbackQPS int) Rule {
	return Rule{QPSLimit: fallbackQPS, Enabled: true}
}
