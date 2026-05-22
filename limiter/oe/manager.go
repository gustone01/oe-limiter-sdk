package oe

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"192.168.10.236/gustone/oe-limiter-sdk/limiter/core"
	"192.168.10.236/gustone/oe-limiter-sdk/model"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// RuleManager 管理巨量引擎限流规则的三级存储与同步。
type RuleManager struct {
	db      *gorm.DB
	rdb     *redis.Client
	opts    Options
	limiter *core.RedisLimiter

	cache          sync.Map // key=路径前缀, value=Rule
	rules          []Rule   // 有序规则列表，供最长前缀匹配
	rulesMu        sync.RWMutex
	discoveredApis sync.Map

	pubsub    *redis.PubSub
	stop      chan struct{}
	closeOnce sync.Once
}

// NewRuleManager 创建巨量引擎规则管理器。
func NewRuleManager(db *gorm.DB, rdb *redis.Client, opts ...Option) (*RuleManager, error) {
	o := Options{}
	for _, fn := range opts {
		fn(&o)
	}
	o.applyDefaults()

	if !o.SkipAutoMigrate {
		if err := AutoMigrate(db); err != nil {
			return nil, fmt.Errorf("auto migrate: %w", err)
		}
	}

	rm := &RuleManager{
		db:      db,
		rdb:     rdb,
		opts:    o,
		limiter: core.NewRedisLimiter(rdb),
		stop:    make(chan struct{}),
	}
	if err := rm.reload(context.Background()); err != nil {
		return nil, fmt.Errorf("load rules: %w", err)
	}
	rm.startSubscriber()
	return rm, nil
}

// Close 停止 Pub/Sub 订阅并释放资源，可安全重复调用。
func (rm *RuleManager) Close() error {
	var err error
	rm.closeOnce.Do(func() {
		close(rm.stop)
		if rm.pubsub != nil {
			err = rm.pubsub.Close()
		}
	})
	return err
}

// Reload 主动从 MySQL 重新加载规则。
func (rm *RuleManager) Reload(ctx context.Context) error {
	return rm.reload(ctx)
}

// PublishRuleUpdate 通知所有实例刷新缓存。
func (rm *RuleManager) PublishRuleUpdate(ctx context.Context) error {
	return rm.rdb.Publish(ctx, rm.opts.PubSubChannel, "reload").Err()
}

// GetRule 根据 API 路径获取限流规则（最长前缀匹配），未匹配则触发自动发现并返回兜底规则。
func (rm *RuleManager) GetRule(apiPath string) Rule {
	if rule, ok := rm.matchRule(apiPath); ok {
		return rule
	}
	rm.handleAutoDiscover(apiPath)
	return DefaultRule(rm.opts.FallbackQPS)
}

func (rm *RuleManager) reload(ctx context.Context) error {
	var rows []model.OeRateLimitRule
	if err := rm.db.WithContext(ctx).Where("enabled = ?", 1).Find(&rows).Error; err != nil {
		return err
	}

	rules := make([]Rule, 0, len(rows))
	for _, row := range rows {
		rules = append(rules, Rule{
			APIPathPrefix: row.APIPathPrefix,
			QPSLimit:      row.QPSLimit,
			Enabled:       row.Enabled == 1,
		})
	}

	rm.rulesMu.Lock()
	rm.rules = rules
	rm.rulesMu.Unlock()
	return nil
}

func (rm *RuleManager) matchRule(apiPath string) (Rule, bool) {
	rm.rulesMu.RLock()
	defer rm.rulesMu.RUnlock()

	var best Rule
	var bestLen int
	for _, r := range rm.rules {
		if !r.Enabled {
			continue
		}
		if !strings.HasPrefix(apiPath, r.APIPathPrefix) {
			continue
		}
		if len(r.APIPathPrefix) > bestLen {
			bestLen = len(r.APIPathPrefix)
			best = r
		}
	}
	if bestLen == 0 {
		return Rule{}, false
	}
	return best, true
}

func (rm *RuleManager) startSubscriber() {
	rm.pubsub = rm.rdb.Subscribe(context.Background(), rm.opts.PubSubChannel)
	go func() {
		ch := rm.pubsub.Channel()
		for {
			select {
			case <-rm.stop:
				return
			case msg, ok := <-ch:
				if !ok {
					return
				}
				if msg == nil {
					continue
				}
				if err := rm.reload(context.Background()); err != nil {
					log.Printf("[oe-limiter] reload rules failed: %v", err)
				}
			}
		}
	}()
}

func (rm *RuleManager) handleAutoDiscover(apiPath string) {
	if _, loaded := rm.discoveredApis.LoadOrStore(apiPath, struct{}{}); loaded {
		return
	}
	log.Printf("[AUTO-DISCOVER] 发现新接口 Path=%s", apiPath)
	if err := rm.savePending(context.Background(), apiPath); err != nil {
		log.Printf("[AUTO-DISCOVER] save pending failed: %v", err)
	}
	rm.createTempRuleInRedis(context.Background(), apiPath)
	if rm.opts.OnDiscover != nil {
		go rm.opts.OnDiscover(apiPath)
	}
}

func (rm *RuleManager) createTempRuleInRedis(ctx context.Context, apiPath string) {
	_ = rm.rdb.Set(ctx, "oe:limit:discovered:"+apiPath, time.Now().Unix(), 24*time.Hour).Err()
}
