package gdt

// Rule 从数据库加载到内存后的腾讯广告规则快照。
type Rule struct {
	// APIPathPrefix 归一化后的 API 路径前缀（去版本号），用于最长前缀匹配。
	APIPathPrefix string
	// QPMLimit 每分钟允许的最大请求数（滑动窗口），0 表示不限。
	QPMLimit int
	// QPDLimit 每天允许的最大请求数（INCR 计数器），0 表示不限。
	QPDLimit int
	// Enabled 规则是否启用。
	Enabled bool
}

// DefaultRule 返回兜底规则。
func DefaultRule(fallbackQPM, fallbackQPD int) Rule {
	return Rule{QPMLimit: fallbackQPM, QPDLimit: fallbackQPD, Enabled: true}
}

// UnlimitedRule 返回不限流规则。
// QPMLimit=0 + QPDLimit=0 + Enabled=true 利用 transport 层 "if limit > 0" 条件跳过限流。
func UnlimitedRule() Rule {
	return Rule{QPMLimit: 0, QPDLimit: 0, Enabled: true}
}
