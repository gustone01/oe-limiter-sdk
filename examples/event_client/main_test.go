package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"192.168.10.236/gustone/oe-limiter-sdk/limiter"
	"192.168.10.236/gustone/oe-limiter-sdk/model"

	"github.com/alicebob/miniredis/v2"
	"github.com/glebarez/sqlite"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

func TestEventClient(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open("file:event_test?mode=memory&cache=private"), &gorm.Config{})
	_ = limiter.AutoMigrate(db)
	_ = db.Create(&model.RateLimitRule{ServiceName: "event", APIPathPrefix: "/open_api/", QPSLimit: 100, Enabled: 1})

	mr, _ := miniredis.Run()
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	tp, err := limiter.NewTransport("event", db, rdb)
	if err != nil {
		t.Fatal(err)
	}
	defer tp.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }))
	defer srv.Close()

	resp, err := (&http.Client{Transport: tp}).Get(srv.URL + "/open_api/v3.0/event/track/1")
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("status=%v err=%v", resp.StatusCode, err)
	}
}
