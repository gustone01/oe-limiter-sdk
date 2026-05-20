package limiter

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestApplyPlatformPenaltyFromResponse_40110(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	body := `{"code":40110,"message":"too many requests"}`
	h := make(http.Header)
	h.Set("X-RateLimit-RetryIn", "30")
	h.Set("X-RateLimit-Dimension", "developer_qpm")
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     h,
	}

	code, applied, ttl, err := ApplyPlatformPenaltyFromResponse(
		context.Background(), rdb, "/open_api/foo", resp, 0,
	)
	if err != nil || !applied || code != 40110 || ttl != 30*time.Second {
		t.Fatalf("applied=%v code=%d ttl=%v err=%v", applied, code, ttl, err)
	}

	// 封禁 Key 是接口级，不含 service
	key := PenaltyKey("/open_api/foo")
	blocked, retry, err := CheckPenalty(context.Background(), rdb, key)
	if err != nil || !blocked || retry <= 0 {
		t.Fatalf("blocked=%v retry=%v err=%v", blocked, retry, err)
	}
}

func TestPenaltyKey_CrossServiceBlocking(t *testing.T) {
	mr, _ := miniredis.Run()
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	apiPath := "/open_api/v2/report/get"
	key := PenaltyKey(apiPath)
	_ = SetPenalty(context.Background(), rdb, key, 5*time.Minute, "40110")

	// 任何 service 检查同路径都应被拦截
	for _, svc := range []string{"event", "data", "click"} {
		blocked, _, err := CheckPenalty(context.Background(), rdb, PenaltyKey(apiPath))
		if err != nil || !blocked {
			t.Fatalf("service=%s should be blocked on penalty key, blocked=%v err=%v", svc, blocked, err)
		}
	}
}

func TestApplyPlatformPenalty_Ignores40100And40130(t *testing.T) {
	mr, _ := miniredis.Run()
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	for _, jsonBody := range []string{
		`{"code":40100}`,
		`{"code":40130}`,
	} {
		h := make(http.Header)
		h.Set("X-RateLimit-RetryIn", "60")
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(jsonBody)),
			Header:     h,
		}

		code, applied, _, _ := ApplyPlatformPenaltyFromResponse(
			context.Background(), rdb, "/open_api/bar", resp, 0,
		)
		if applied || code != 0 {
			t.Fatalf("body=%s should NOT trigger penalty, got applied=%v code=%d", jsonBody, applied, code)
		}
	}
}

func TestCheckPenalty_RequestPathBlocks(t *testing.T) {
	mr, _ := miniredis.Run()
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	key := PenaltyKey("/x")
	_ = SetPenalty(context.Background(), rdb, key, 2*time.Second, "40110")

	blocked, _, _ := CheckPenalty(context.Background(), rdb, key)
	if !blocked {
		t.Fatal("expected penalty block on request path")
	}
}
