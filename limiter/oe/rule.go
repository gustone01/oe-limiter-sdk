package oe

// Rule 从数据库加载到内存后的巨量引擎规则快照。
type Rule struct {
	APIPathPrefix string
	QPSLimit      int
	Enabled       bool
}

// DefaultRule 返回兜底规则。
func DefaultRule(fallbackQPS int) Rule {
	return Rule{QPSLimit: fallbackQPS, Enabled: true}
}
