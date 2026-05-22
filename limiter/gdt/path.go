package gdt

import "regexp"

var (
	// versionPrefixRE 匹配腾讯广告路径中的版本前缀 /vX.Y/，如 /v3.0/、/v1.1/
	versionPrefixRE = regexp.MustCompile(`^/v\d+\.\d+/`)
	// digitSegmentRE 匹配 5 位及以上纯数字路径段
	digitSegmentRE = regexp.MustCompile(`/\d{5,}`)
)

// NormalizePath 归一化腾讯广告 API 路径：
//  1. 去掉版本前缀 /vX.Y/ → /（如 /v3.0/videos/get → /videos/get）
//  2. 将 5 位+纯数字路径段替换为 /{id}
func NormalizePath(raw string) string {
	s := versionPrefixRE.ReplaceAllString(raw, "/")
	return digitSegmentRE.ReplaceAllString(s, "/{id}")
}
