package limiter

import (
	"net/http"
	"strconv"
	"time"
)

func rateLimitedResponse(req *http.Request, retryAfter time.Duration, rejectReason string) *http.Response {
	h := make(http.Header)
	if retryAfter > 0 {
		sec := int(retryAfter.Seconds())
		if sec < 1 {
			sec = 1
		}
		h.Set("Retry-After", strconv.Itoa(sec))
	}
	if rejectReason != "" {
		h.Set(HeaderRejectReason, rejectReason)
	}
	return &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Status:     "429 Rate Limited",
		Body:       http.NoBody,
		Header:     h,
		Request:    req,
	}
}
