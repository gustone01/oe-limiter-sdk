package gdt

// Rule 从数据库加载到内存后的腾讯广告规则快照。
type Rule struct {
	APIPathPrefix string
	QPMLimit      int
	QPDLimit      int
	Enabled       bool
}

// DefaultRule 返回兜底规则。
func DefaultRule(fallbackQPM, fallbackQPD int) Rule {
	return Rule{QPMLimit: fallbackQPM, QPDLimit: fallbackQPD, Enabled: true}
}
