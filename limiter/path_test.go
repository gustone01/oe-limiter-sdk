package limiter

import "testing"

func TestNormalizePath(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"/open_api/v3.0/event/track/12345", "/open_api/v3.0/event/track/{id}"},
		{"/open_api/v3.0/report/get/", "/open_api/v3.0/report/get/"},
		{"/a/1/b/2/c", "/a/{id}/b/{id}/c"},
	}
	for _, tt := range tests {
		if got := NormalizePath(tt.in); got != tt.want {
			t.Errorf("NormalizePath(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
