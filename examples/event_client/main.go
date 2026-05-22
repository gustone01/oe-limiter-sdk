// 巨量引擎服务接入 oe-limiter-sdk 示例。
//
// 环境变量：MYSQL_DSN、REDIS_ADDR、API_URL（可选）
package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/gustone01/oe-limiter-sdk/limiter/oe"

	"github.com/redis/go-redis/v9"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func main() {
	dsn := os.Getenv("MYSQL_DSN")
	if dsn == "" {
		log.Fatal("请设置 MYSQL_DSN，例如: user:pass@tcp(127.0.0.1:3306)/db?charset=utf8mb4&parseTime=True")
	}
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "127.0.0.1:6379"
	}

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatal(err)
	}

	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		log.Fatal(err)
	}

	transport, err := oe.NewTransport(db, rdb,
		oe.WithOnDiscover(func(path string) {
			log.Printf("[AUTO-DISCOVER] path=%s → 已写入 oe_rate_limit_pending", path)
		}),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer transport.Close()

	apiURL := os.Getenv("API_URL")
	if apiURL == "" {
		apiURL = "https://httpbin.org/get"
	}

	resp, err := (&http.Client{Transport: transport}).Get(apiURL)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("status=%d\n%s\n", resp.StatusCode, body)
}
