package limiter

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// PlatformCodeDeveloper 开发者频控惩罚错误码。
// 滑动窗口统计 QPM 超限后，平台返回该 code 并持续惩罚 5 分钟。
const PlatformCodeDeveloper = 40110

const (
	defaultPenaltyTTL = 5 * time.Minute // 平台 40110 惩罚默认 5 分钟
	penaltyKeyPrefix  = "oe:limit:penalty:dev:"
)

// PenaltyKey 40110 开发者频控封禁 Key —— 接口级，不区分 service。
// 同一开发者账户下所有 service 共享 QPM 配额，触发后统一拦截。
func PenaltyKey(apiPath string) string {
	return penaltyKeyPrefix + apiPath
}

// CheckPenalty 请求路径：是否处于平台惩罚封禁期。
func CheckPenalty(ctx context.Context, rdb *redis.Client, key string) (blocked bool, retryAfter time.Duration, err error) {
	if rdb == nil {
		return false, 0, nil
	}
	ttl, err := rdb.TTL(ctx, key).Result()
	if err != nil {
		return false, 0, err
	}
	if ttl <= 0 {
		return false, 0, nil
	}
	return true, ttl, nil
}

// SetPenalty 响应路径：记录平台惩罚，在 TTL 内请求路径直接拒绝出站。
func SetPenalty(ctx context.Context, rdb *redis.Client, key string, ttl time.Duration, reason string) error {
	if rdb == nil || ttl <= 0 {
		return nil
	}
	return rdb.Set(ctx, key, reason, ttl).Err()
}

type platformBody struct {
	Code int `json:"code"`
}

// ApplyPlatformPenaltyFromResponse 在收到开放平台响应后解析 40110 频控并写入封禁状态。
// 应在 RoundTrip 返回后调用；会读取并还原 Response.Body。
// 写入的封禁 Key 为接口级（不含 serviceName），因为 40110 对同一开发者账户下所有 service 生效。
func ApplyPlatformPenaltyFromResponse(
	ctx context.Context,
	rdb *redis.Client,
	apiPath string,
	resp *http.Response,
	defaultTTL time.Duration,
) (code int, applied bool, retryAfter time.Duration, err error) {
	if rdb == nil || resp == nil {
		return 0, false, 0, nil
	}
	if defaultTTL <= 0 {
		defaultTTL = defaultPenaltyTTL
	}

	code, retryAfter = parsePlatformRateLimit(resp)
	if code == 0 {
		return 0, false, 0, nil
	}
	ttl := retryAfter
	if ttl <= 0 {
		ttl = defaultTTL
	}
	key := PenaltyKey(apiPath)
	if err := SetPenalty(ctx, rdb, key, ttl, "40110"); err != nil {
		return code, false, 0, err
	}
	return code, true, ttl, nil
}

// parsePlatformRateLimit 从响应头与 JSON body 解析 40110 开发者频控。
// 只有存在 X-RateLimit-* 头或 HTTP 429 时才读取 body，避免正常请求多余的 IO。
// 仅识别 40110，其他 code 忽略。
func parsePlatformRateLimit(resp *http.Response) (code int, retryAfter time.Duration) {
	hasRetryIn := false
	if ra := resp.Header.Get("X-RateLimit-RetryIn"); ra != "" {
		if sec, err := strconv.Atoi(strings.TrimSpace(ra)); err == nil && sec > 0 {
			retryAfter = time.Duration(sec) * time.Second
			hasRetryIn = true
		}
	}

	hasDim := resp.Header.Get("X-RateLimit-Dimension") != ""
	is429 := resp.StatusCode == http.StatusTooManyRequests

	if !hasRetryIn && !hasDim && !is429 {
		return 0, 0
	}

	bodyCode, _ := peekJSONCode(resp)
	if bodyCode != PlatformCodeDeveloper {
		return 0, 0
	}
	return bodyCode, retryAfter
}

func peekJSONCode(resp *http.Response) (int, error) {
	if resp.Body == nil {
		return 0, nil
	}
	const maxPeek = 64 << 10
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxPeek))
	if err != nil {
		return 0, err
	}
	_ = resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(body))

	var v platformBody
	if err := json.Unmarshal(body, &v); err != nil {
		// 非 JSON 响应（如 HTML 502）不视为错误，仅无法提取 code
		return 0, nil
	}
	return v.Code, nil
}
