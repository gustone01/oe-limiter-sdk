-- =============================================================================
-- oe-limiter-sdk 数据库初始化脚本
-- 与 model.RateLimitRule / model.RateLimitPending（gorm.Model）字段对齐
--
-- 使用方式：
--   mysql -u user -p your_db < schema.sql
-- 或在代码中：limiter.AutoMigrate(db)
-- =============================================================================

-- -----------------------------------------------------------------------------
-- 1. 正式限流规则表（管理后台配置 / 审核通过后写入）
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `oe_rate_limit_rules` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `service_name` varchar(32) NOT NULL COMMENT '服务标识: click,data,event,gy; ALL=全局共享',
  `api_path_prefix` varchar(128) NOT NULL COMMENT 'API路径前缀，最长前缀匹配（归一化后，数字段替换为{id}）',
  `qps_limit` int NOT NULL DEFAULT 10 COMMENT 'QPS配额（滑动窗口秒级计数，建议留频控后台配额10%-20%余量）',
  `is_shared` tinyint(1) NOT NULL DEFAULT 0 COMMENT '是否共享池: 1共享(ALL全局) 0独占(单服务)',
  `enabled` tinyint(1) NOT NULL DEFAULT 1 COMMENT '是否启用: 1启用 0禁用(禁用后不加载到缓存)',
  `created_at` datetime(3) NULL DEFAULT NULL,
  `updated_at` datetime(3) NULL DEFAULT NULL,
  `deleted_at` datetime(3) NULL DEFAULT NULL COMMENT '软删除，gorm.Model',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_service_api` (`service_name`, `api_path_prefix`),
  KEY `idx_path` (`api_path_prefix`),
  KEY `idx_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='限流正式规则（滑动窗口QPS + 40110开发者频控封禁）';

-- -----------------------------------------------------------------------------
-- 2. 自动发现待审核表（SDK 发现未配置接口时写入，status=0 待审核）
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `oe_rate_limit_pending` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `service_name` varchar(32) NOT NULL COMMENT '服务标识: click,data,event,gy',
  `api_path_prefix` varchar(128) NOT NULL COMMENT '归一化路径，如 /open_api/v3.0/foo/{id}',
  `suggested_qps` int NOT NULL DEFAULT 5 COMMENT '建议QPS（审核通过时可覆盖，建议参考频控后台配额）',
  `status` tinyint NOT NULL DEFAULT 0 COMMENT '0待审核 1已通过(写入rules表) 2已拒绝',
  `remark` varchar(255) NOT NULL DEFAULT '' COMMENT '审核备注（拒绝原因等）',
  `discovered_at` datetime(3) NOT NULL COMMENT '首次自动发现时间',
  `reviewed_at` datetime(3) NULL DEFAULT NULL COMMENT '审核完成时间',
  `created_at` datetime(3) NULL DEFAULT NULL,
  `updated_at` datetime(3) NULL DEFAULT NULL,
  `deleted_at` datetime(3) NULL DEFAULT NULL COMMENT '软删除，gorm.Model',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_pending_service_api` (`service_name`, `api_path_prefix`),
  KEY `idx_status` (`status`),
  KEY `idx_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='自动发现待审核（SDK首次访问未配置接口时自动写入）';

-- -----------------------------------------------------------------------------
-- 3. 示例数据（可按环境删减）
-- -----------------------------------------------------------------------------
INSERT INTO `oe_rate_limit_rules` (`service_name`, `api_path_prefix`, `qps_limit`, `is_shared`, `enabled`, `created_at`, `updated_at`)
VALUES
  ('event', '/open_api/v3.0/event/track/', 200, 0, 1, NOW(3), NOW(3)),
  ('data',  '/open_api/v3.0/report/get/',   50,  0, 1, NOW(3), NOW(3)),
  ('click', '/open_api/v3.0/tools/click_track/', 30, 0, 1, NOW(3), NOW(3)),
  ('gy',    '/open_api/v3.0/advertiser/update/', 10, 0, 1, NOW(3), NOW(3)),
  ('ALL',   '/open_api/',                   500, 1, 1, NOW(3), NOW(3))
ON DUPLICATE KEY UPDATE
  `qps_limit` = VALUES(`qps_limit`),
  `is_shared` = VALUES(`is_shared`),
  `enabled`   = VALUES(`enabled`),
  `updated_at` = NOW(3);

-- -----------------------------------------------------------------------------
-- 4. 旧库升级（若表已存在但缺少字段，按需执行）
-- -----------------------------------------------------------------------------
-- ALTER TABLE `oe_rate_limit_rules`
--   MODIFY COLUMN `id` bigint unsigned NOT NULL AUTO_INCREMENT,
--   MODIFY COLUMN `created_at` datetime(3) NULL DEFAULT NULL,
--   MODIFY COLUMN `updated_at` datetime(3) NULL DEFAULT NULL,
--   MODIFY COLUMN `deleted_at` datetime(3) NULL DEFAULT NULL,
--   ADD KEY `idx_deleted_at` (`deleted_at`);

-- CREATE TABLE IF NOT EXISTS `oe_rate_limit_pending` (...);  -- 见上方完整建表

-- ALTER TABLE `oe_rate_limit_pending`
--   ADD COLUMN `deleted_at` datetime(3) NULL DEFAULT NULL AFTER `updated_at`,
--   ADD KEY `idx_deleted_at` (`deleted_at`);
