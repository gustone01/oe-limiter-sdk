package limiter

import (
	"net/http"
	"strconv"
	"time"
)

func rateLimitedResponse(req *http.Request, retryAfter time.Duration, status string) *http.Response {
	h := make(http.Header)
	if retryAfter > 0 {
		sec := int(retryAfter.Seconds())
		if sec < 1 {
			sec = 1
		}
		h.Set("Retry-After", strconv.Itoa(sec))
	}
	if status == "" {
		status = "429 Rate Limited"
	}
	return &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Status:     status,
		Body:       http.NoBody,
		Header:     h,
		Request:    req,
	}
}
