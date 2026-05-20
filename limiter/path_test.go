package limiter

import "testing"

func TestNormalizePath(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"/open_api/v3.0/event/track/12345", "/open_api/v3.0/event/track/{id}"},
		{"/open_api/v3.0/report/get/", "/open_api/v3.0/report/get/"},
		// 版本号（短数字段）不替换
		{"/open_api/2/customer_center/advertiser/list/", "/open_api/2/customer_center/advertiser/list/"},
		{"/open_api/2/ad/get/7890123456", "/open_api/2/ad/get/{id}"},
		// 短数字段保持不变
		{"/a/1/b/22/c/333/d/4444/e/55555", "/a/1/b/22/c/333/d/4444/e/{id}"},
	}
	for _, tt := range tests {
		if got := NormalizePath(tt.in); got != tt.want {
			t.Errorf("NormalizePath(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
