package core

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// RateLimitedResponse 生成 SDK 限流拦截的 429 响应（未发起真实请求）。
func RateLimitedResponse(req *http.Request, retryAfter time.Duration) *http.Response {
	h := make(http.Header)
	retrySec := 0
	if retryAfter > 0 {
		retrySec = int(retryAfter.Seconds())
		if retrySec < 1 {
			retrySec = 1
		}
		h.Set("Retry-After", strconv.Itoa(retrySec))
	}
	h.Set("Content-Type", "application/json; charset=utf-8")

	body := fmt.Sprintf(`{"code":-429,"message":"rate limited","retry_after":%d}`, retrySec)

	return &http.Response{
		StatusCode:    http.StatusTooManyRequests,
		Status:        StatusRateLimited,
		Header:        h,
		Body:          io.NopCloser(strings.NewReader(body)),
		ContentLength: int64(len(body)),
		Request:       req,
	}
}
