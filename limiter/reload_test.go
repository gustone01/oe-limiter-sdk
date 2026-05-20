package limiter

import (
	"context"
	"fmt"
	"testing"

	"192.168.10.236/gustone/oe-limiter-sdk/model"

	"github.com/alicebob/miniredis/v2"
	"github.com/glebarez/sqlite"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

func reloadTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=private", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestReload_RemovesDisabledServiceCache(t *testing.T) {
	db := reloadTestDB(t)
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	row := model.RateLimitRule{
		ServiceName: "event", APIPathPrefix: "/open_api/", QPSLimit: 100, Enabled: 1,
	}
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}

	rm, err := NewRuleManager(db, rdb, WithPubSubChannel("ch:"+t.Name()))
	if err != nil {
		t.Fatal(err)
	}
	defer rm.Close()

	if rule, ok := rm.matchRule("event", "/open_api/v3.0/foo"); !ok || rule.QPSLimit != 100 {
		t.Fatalf("before disable: ok=%v qps=%d", ok, rule.QPSLimit)
	}

	if err := db.Model(&row).Update("enabled", 0).Error; err != nil {
		t.Fatal(err)
	}
	if err := rm.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}

	if _, ok := rm.matchRule("event", "/open_api/v3.0/foo"); ok {
		t.Fatal("disabled service rules should be removed from cache")
	}
}

func TestTransport_CloseIdempotent(t *testing.T) {
	db := reloadTestDB(t)
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	tp, err := NewTransport("event", db, rdb, WithPubSubChannel("ch:"+t.Name()))
	if err != nil {
		t.Fatal(err)
	}
	if err := tp.Close(); err != nil {
		t.Fatal(err)
	}
	if err := tp.Close(); err != nil {
		t.Fatal(err)
	}
}
