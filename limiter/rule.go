package limiter

// Rule 从数据库加载到内存后的规则快照，供限流逻辑快速读取。
type Rule struct {
	// ServiceName 服务名，与 model.RateLimitRule.ServiceName 一致。
	ServiceName string
	// APIPathPrefix 路径前缀，用于最长前缀匹配。
	APIPathPrefix string
	// QPSLimit 该规则下的每秒配额上限。
	QPSLimit int
	// IsShared 是否为共享池规则（通常 service_name=ALL 时为 true）。
	IsShared bool
	// Enabled 规则是否生效。
	Enabled bool
}

// DefaultRule 返回自动发现新接口时使用的兜底规则（默认 5 QPS）。
func DefaultRule() Rule {
	return Rule{QPSLimit: defaultPendingSuggested, Enabled: true}
}
