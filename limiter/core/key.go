package core

import "fmt"

// HashTagKey 生成 Redis Cluster 兼容的限流 Key。
// 通过 {prefix:path} hash tag 保证同一接口的多维度 Key 落在同一 slot，
// 使 AllowMultiWindow Lua 脚本可跨 Key 原子执行。
//
//	HashTagKey("oe:limit", "/open_api/v3.0/foo", "qps")
//	→ "{oe:limit:/open_api/v3.0/foo}:qps"
func HashTagKey(prefix, path, dimension string) string {
	return fmt.Sprintf("{%s:%s}:%s", prefix, path, dimension)
}
