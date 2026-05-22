package oe

import "regexp"

// digitSegmentRE 匹配路径中 5 位及以上的纯数字段，替换为 {id}。
// 仅替换长数字段以避免误伤版本号（如 /v2/、/v3.0/）。
var digitSegmentRE = regexp.MustCompile(`/\d{5,}`)

// NormalizePath 归一化巨量引擎 API 路径：将 5 位+纯数字路径段替换为 /{id}。
//
//	/open_api/v3.0/event/track/1234567890 → /open_api/v3.0/event/track/{id}
//	/open_api/2/customer_center/advertiser/list/ → 不变（2 不足 5 位）
func NormalizePath(raw string) string {
	return digitSegmentRE.ReplaceAllString(raw, "/{id}")
}
