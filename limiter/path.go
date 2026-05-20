package limiter

import "regexp"

// digitSegmentRE 匹配路径中的数字段，如 /123、/456。
var digitSegmentRE = regexp.MustCompile(`/\d+`)

// NormalizePath 将 API 路径中的动态数字 ID 统一替换为 /{id}。
//
// 例如 /open_api/v3.0/event/track/12345 → /open_api/v3.0/event/track/{id}，
// 这样同一条限流规则可覆盖所有 ID，而无需为每个 ID 单独配置。
func NormalizePath(apiPath string) string {
	// 连续数字段统一为 /{id}，使一条规则覆盖所有动态 ID
	return digitSegmentRE.ReplaceAllString(apiPath, "/{id}")
}
