package limiter

import "regexp"

// digitSegmentRE 匹配路径中 5 位及以上的纯数字段（动态 ID），
// 短数字段（如版本号 /2/、/3/）保持不变。
var digitSegmentRE = regexp.MustCompile(`/\d{5,}`)

// NormalizePath 将 API 路径中的动态数字 ID 统一替换为 /{id}。
//
// 例如 /open_api/v3.0/event/track/12345 → /open_api/v3.0/event/track/{id}，
// 而版本号 /open_api/2/customer_center/... 中的 /2 不会被替换。
func NormalizePath(apiPath string) string {
	return digitSegmentRE.ReplaceAllString(apiPath, "/{id}")
}
