package oe

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/gustone01/oe-limiter-sdk/limiter/core"
	"github.com/gustone01/oe-limiter-sdk/model"

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
	if o.UnmatchedUnlimited && o.DisablePendingSave {
		log.Printf("[oe-limiter] WARNING: UnmatchedUnlimited + DisablePendingSave 同时启用，未匹配接口将既无限流也无记录")
	}

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

// GetRule 根据 API 路径获取限流规则（最长前缀匹配）。
// 未匹配时触发自动发现；若 UnmatchedUnlimited 为 true 则返回不限流规则，否则返回兜底规则。
func (rm *RuleManager) GetRule(apiPath string) Rule {
	if rule, ok := rm.matchRule(apiPath); ok {
		return rule
	}
	rm.handleAutoDiscover(apiPath)
	return rm.unmatchedRule()
}

// unmatchedRule 返回未匹配到任何规则时应使用的 Rule。
func (rm *RuleManager) unmatchedRule() Rule {
	if rm.opts.UnmatchedUnlimited {
		return UnlimitedRule()
	}
	return DefaultRule(rm.opts.FallbackQPS)
}

// reload 从数据库加载已启用的规则并刷新本地缓存。
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
	log.Printf("[oe-limiter] loaded %d rules", len(rules))
	return nil
}

// matchRule 在有序规则列表中执行最长前缀匹配。
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

// startSubscriber 启动 Redis Pub/Sub 订阅，收到消息后自动 reload 规则。
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

// handleAutoDiscover 处理未匹配规则的接口：去重后写入待审核表并触发回调。
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

// createTempRuleInRedis 在 Redis 中标记已发现的接口，24h 过期，用于跨实例去重。
func (rm *RuleManager) createTempRuleInRedis(ctx context.Context, apiPath string) {
	_ = rm.rdb.Set(ctx, "oe:limit:discovered:"+apiPath, time.Now().Unix(), 24*time.Hour).Err()
}
