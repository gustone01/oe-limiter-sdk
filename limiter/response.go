package limiter

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func rateLimitedResponse(req *http.Request, retryAfter time.Duration, rejectReason string) *http.Response {
	h := make(http.Header)
	retrySec := 0
	if retryAfter > 0 {
		retrySec = int(retryAfter.Seconds())
		if retrySec < 1 {
			retrySec = 1
		}
		h.Set("Retry-After", strconv.Itoa(retrySec))
	}
	if rejectReason != "" {
		h.Set(HeaderRejectReason, rejectReason)
	}
	h.Set("Content-Type", "application/json; charset=utf-8")

	body := fmt.Sprintf(`{"code":-429,"reason":%q,"retry_after":%d}`, rejectReason, retrySec)

	return &http.Response{
		StatusCode:    http.StatusTooManyRequests,
		Status:        StatusRateLimited,
		Header:        h,
		Body:          io.NopCloser(strings.NewReader(body)),
		ContentLength: int64(len(body)),
		Request:       req,
	}
}
