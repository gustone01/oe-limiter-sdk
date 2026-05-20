package limiter

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"192.168.10.236/gustone/oe-limiter-sdk/model"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// RuleManager 管理限流规则的三级存储与同步：
//   - L3 MySQL：持久化配置
//   - L2 Redis Pub/Sub：多实例间规则变更通知
//   - L1 sync.Map：进程内高速缓存
type RuleManager struct {
	db      *gorm.DB
	rdb     *redis.Client
	opts    Options
	limiter *RedisLimiter

	cache          sync.Map // key=serviceName，value=[]Rule
	discoveredApis sync.Map // key=serviceName:path，记录已触发过自动发现的接口

	pubsub    *redis.PubSub
	stop      chan struct{} // 关闭时通知后台 goroutine 退出
	closeOnce sync.Once
}

// NewRuleManager 创建规则管理器：自动建表（可关闭）→ 加载规则 → 订阅 Redis 变更。
func NewRuleManager(db *gorm.DB, rdb *redis.Client, opts ...ManagerOption) (*RuleManager, error) {
	o := Options{}
	for _, fn := range opts {
		fn(&o)
	}
	o.applyDefaults()

	// SDK 内置建表：oe_rate_limit_rules + oe_rate_limit_pending
	if !o.SkipAutoMigrate {
		if err := AutoMigrate(db); err != nil {
			return nil, fmt.Errorf("auto migrate: %w", err)
		}
	}

	rm := &RuleManager{
		db:      db,
		rdb:     rdb,
		opts:    o,
		limiter: NewRedisLimiter(rdb),
		stop:    make(chan struct{}),
	}
	if err := rm.reload(context.Background()); err != nil {
		return nil, fmt.Errorf("load rules: %w", err)
	}
	rm.startSubscriber()
	return rm, nil
}

// Limiter 返回关联的 Redis 限流器，可在自定义逻辑中直接调用 AllowDoubleCheck。
func (rm *RuleManager) Limiter() *RedisLimiter {
	return rm.limiter
}

// WindowSec 返回当前配置的限流计数窗口长度（秒）。
func (rm *RuleManager) WindowSec() int {
	return rm.opts.WindowSec
}

// WindowMS 返回滑动窗口长度（毫秒）。
func (rm *RuleManager) WindowMS() int {
	return rm.opts.WindowMS
}

// LimiterMode 返回限流算法模式。
func (rm *RuleManager) LimiterMode() string {
	return rm.opts.LimiterMode
}

// Close 停止 Redis Pub/Sub 订阅并释放资源，进程退出前建议调用；可安全重复调用。
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

// Reload 主动从 MySQL 重新加载全部启用规则到本地缓存（一般由 Pub/Sub 自动触发，也可手动调用）。
func (rm *RuleManager) Reload(ctx context.Context) error {
	return rm.reload(ctx)
}

// PublishRuleUpdate 在管理后台增删改规则后调用，向 Redis 发布消息，通知所有实例刷新缓存。
func (rm *RuleManager) PublishRuleUpdate(ctx context.Context) error {
	return rm.rdb.Publish(ctx, rm.opts.PubSubChannel, "reload").Err()
}

// reload 从数据库查询 enabled=1 的规则，按 service_name 分组写入本地 sync.Map。
func (rm *RuleManager) reload(ctx context.Context) error {
	var rows []model.RateLimitRule
	// 只加载启用中的规则，禁用规则不参与限流
	if err := rm.db.WithContext(ctx).
		Where("enabled = ?", 1).
		Find(&rows).Error; err != nil {
		return err
	}
	// 按服务名分组，便于 matchRule 时 O(n) 扫描该服务下所有前缀
	grouped := make(map[string][]Rule)
	for _, row := range rows {
		svc := strings.TrimSpace(row.ServiceName)
		prefix := row.APIPathPrefix
		grouped[svc] = append(grouped[svc], Rule{
			ServiceName:   svc,
			APIPathPrefix: prefix,
			QPSLimit:      row.QPSLimit,
			IsShared:      row.IsShared == 1,
			Enabled:       row.Enabled == 1,
		})
	}
	// 整组替换 L1 缓存；Pub/Sub 触发时所有实例都会执行这一步
	for svc, rules := range grouped {
		rm.cache.Store(svc, rules)
	}
	// 删除已无启用规则的服务，避免禁用/删光后仍命中旧缓存
	rm.cache.Range(func(key, _ any) bool {
		svc := key.(string)
		if _, ok := grouped[svc]; !ok {
			rm.cache.Delete(svc)
		}
		return true
	})
	return nil
}

// startSubscriber 后台监听 Redis 频道，收到规则变更消息后自动 reload。
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
				// 管理后台 PublishRuleUpdate 后，各实例收到消息即全量刷新本地规则
				if err := rm.reload(context.Background()); err != nil {
					log.Printf("[oe-limiter] reload rules failed: %v", err)
				}
			}
		}
	}()
}

// GetRule 根据服务名和 API 路径获取限流规则（最长前缀匹配）。
//
// 若 MySQL 中无匹配规则，会触发自动发现（写待审核表 + 日志 + OnDiscover），
// 并返回兜底 QPS（见 PendingSuggestedQPS）。
func (rm *RuleManager) GetRule(serviceName, apiPath string) Rule {
	if rule, ok := rm.matchRule(serviceName, apiPath); ok {
		return rule
	}
	rm.handleAutoDiscover(serviceName, apiPath)
	return Rule{QPSLimit: rm.opts.PendingSuggestedQPS, Enabled: true}
}

// matchRule 在本地缓存中按最长路径前缀匹配规则，不触发自动发现。
//
// 例如同时存在 /open_api/ 与 /open_api/v3.0/event/track/ 时，后者更具体，优先命中。
func (rm *RuleManager) matchRule(serviceName, apiPath string) (Rule, bool) {
	val, ok := rm.cache.Load(serviceName)
	if !ok {
		return Rule{}, false
	}
	rules := val.([]Rule)
	var best Rule
	var bestLen int
	for _, r := range rules {
		if !r.Enabled {
			continue
		}
		// 请求路径必须以规则前缀开头，例如规则 /open_api/ 可匹配 /open_api/v3.0/xxx
		if !strings.HasPrefix(apiPath, r.APIPathPrefix) {
			continue
		}
		// 多条规则同时命中时，取最长前缀（最具体的那条）
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

// handleAutoDiscover 处理首次出现的未配置接口：写待审核表、打日志、异步回调告警。
func (rm *RuleManager) handleAutoDiscover(serviceName, apiPath string) {
	key := fmt.Sprintf("%s:%s", serviceName, apiPath)
	// LoadOrStore 保证同一接口只处理一次，避免日志风暴和重复插入
	if _, loaded := rm.discoveredApis.LoadOrStore(key, struct{}{}); loaded {
		return
	}
	log.Printf("[AUTO-DISCOVER] 发现新接口 Service=%s Path=%s", serviceName, apiPath)
	// 写入 MySQL 待审核表 oe_rate_limit_pending
	if err := rm.savePending(context.Background(), serviceName, apiPath); err != nil {
		log.Printf("[AUTO-DISCOVER] save pending failed: %v", err)
	}
	rm.createTempRuleInRedis(context.Background(), key)
	if rm.opts.OnDiscover != nil {
		go rm.opts.OnDiscover(serviceName, apiPath)
	}
}

// createTempRuleInRedis 在 Redis 记录该接口的发现时间（24 小时过期），便于运维排查；正式规则仍需写入 MySQL。
func (rm *RuleManager) createTempRuleInRedis(ctx context.Context, cacheKey string) {
	_ = rm.rdb.Set(ctx, "oe:limit:discovered:"+cacheKey, time.Now().Unix(), 24*time.Hour).Err()
}
