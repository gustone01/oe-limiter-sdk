package limiter

import "testing"

func TestMatchRuleLongestPrefix(t *testing.T) {
	rm := &RuleManager{}
	rm.cache.Store("event", []Rule{
		{APIPathPrefix: "/open_api/", QPSLimit: 500, Enabled: true},
		{APIPathPrefix: "/open_api/v3.0/event/track/", QPSLimit: 200, Enabled: true},
	})

	rule, ok := rm.matchRule("event", "/open_api/v3.0/event/track/abc")
	if !ok {
		t.Fatal("expected match")
	}
	if rule.QPSLimit != 200 {
		t.Fatalf("got qps %d, want 200", rule.QPSLimit)
	}
}
