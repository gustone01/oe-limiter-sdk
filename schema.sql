-- =============================================================================
-- oe-limiter-sdk 数据库初始化脚本
-- 支持巨量引擎 (oe) 和腾讯广告 (gdt) 两套限流表
--
-- 使用方式：
--   mysql -u user -p your_db < schema.sql
-- 或在代码中：oe.AutoMigrate(db) / gdt.AutoMigrate(db)
-- =============================================================================

-- =============================================================================
-- 一、巨量引擎（Ocean Engine）
-- =============================================================================

-- 1. 正式限流规则表
CREATE TABLE IF NOT EXISTS `oe_rate_limit_rules` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `api_path_prefix` varchar(128) NOT NULL COMMENT 'API路径前缀，最长前缀匹配（归一化后）',
  `qps_limit` int NOT NULL DEFAULT 10 COMMENT 'QPS配额（滑动窗口秒级，QPM=QPS*100自动派生，建议留10%-20%余量）',
  `enabled` tinyint(1) NOT NULL DEFAULT 1 COMMENT '1启用 0禁用',
  `created_at` datetime(3) NULL DEFAULT NULL,
  `updated_at` datetime(3) NULL DEFAULT NULL,
  `deleted_at` datetime(3) NULL DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_api_path` (`api_path_prefix`),
  KEY `idx_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='巨量引擎限流规则（QPS滑动窗口）';

-- 2. 自动发现待审核表
CREATE TABLE IF NOT EXISTS `oe_rate_limit_pending` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `api_path_prefix` varchar(128) NOT NULL COMMENT '归一化路径',
  `suggested_qps` int NOT NULL DEFAULT 5 COMMENT '建议QPS',
  `status` tinyint NOT NULL DEFAULT 0 COMMENT '0待审核 1已通过 2已拒绝',
  `remark` varchar(255) NOT NULL DEFAULT '',
  `discovered_at` datetime(3) NOT NULL COMMENT '首次发现时间',
  `reviewed_at` datetime(3) NULL DEFAULT NULL,
  `created_at` datetime(3) NULL DEFAULT NULL,
  `updated_at` datetime(3) NULL DEFAULT NULL,
  `deleted_at` datetime(3) NULL DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_api_path` (`api_path_prefix`),
  KEY `idx_status` (`status`),
  KEY `idx_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='巨量引擎自动发现待审核';

-- 3. 巨量引擎示例数据
-- INSERT INTO `oe_rate_limit_rules` (`api_path_prefix`, `qps_limit`, `enabled`, `created_at`, `updated_at`)
-- VALUES
--   ('/open_api/v3.0/event/track/', 200, 1, NOW(3), NOW(3)),
--   ('/open_api/v3.0/report/get/',   50, 1, NOW(3), NOW(3)),
--   ('/open_api/v3.0/tools/click_track/', 30, 1, NOW(3), NOW(3)),
--   ('/open_api/v3.0/advertiser/update/', 10, 1, NOW(3), NOW(3)),
--   ('/open_api/', 500, 1, NOW(3), NOW(3))
-- ON DUPLICATE KEY UPDATE
--   `qps_limit` = VALUES(`qps_limit`),
--   `enabled`   = VALUES(`enabled`),
--   `updated_at` = NOW(3);

-- =============================================================================
-- 二、腾讯广告（GDT / Tencent Ads）
-- =============================================================================

-- 4. 正式限流规则表
CREATE TABLE IF NOT EXISTS `gdt_rate_limit_rules` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `api_path_prefix` varchar(128) NOT NULL COMMENT 'API路径前缀（去版本号归一化后，如 /videos/get）',
  `qpm_limit` int NOT NULL DEFAULT 0 COMMENT 'QPM配额（滑动窗口分钟级，0=不限）',
  `qpd_limit` int NOT NULL DEFAULT 0 COMMENT 'QPD配额（计数器日级，0=不限）',
  `enabled` tinyint(1) NOT NULL DEFAULT 1 COMMENT '1启用 0禁用',
  `created_at` datetime(3) NULL DEFAULT NULL,
  `updated_at` datetime(3) NULL DEFAULT NULL,
  `deleted_at` datetime(3) NULL DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_api_path` (`api_path_prefix`),
  KEY `idx_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='腾讯广告限流规则（QPM滑动窗口+QPD计数器）';

-- 5. 自动发现待审核表
CREATE TABLE IF NOT EXISTS `gdt_rate_limit_pending` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `api_path_prefix` varchar(128) NOT NULL COMMENT '归一化路径（去版本号）',
  `suggested_qpm` int NOT NULL DEFAULT 100 COMMENT '建议QPM',
  `status` tinyint NOT NULL DEFAULT 0 COMMENT '0待审核 1已通过 2已拒绝',
  `remark` varchar(255) NOT NULL DEFAULT '',
  `discovered_at` datetime(3) NOT NULL COMMENT '首次发现时间',
  `reviewed_at` datetime(3) NULL DEFAULT NULL,
  `created_at` datetime(3) NULL DEFAULT NULL,
  `updated_at` datetime(3) NULL DEFAULT NULL,
  `deleted_at` datetime(3) NULL DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_api_path` (`api_path_prefix`),
  KEY `idx_status` (`status`),
  KEY `idx_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='腾讯广告自动发现待审核';

-- =============================================================================
-- 三、旧库升级（从含 service_name 版本迁移）
-- =============================================================================
-- 如果从旧版升级，需要执行以下操作：
--
-- ALTER TABLE `oe_rate_limit_rules`
--   DROP INDEX `uk_service_api`,
--   DROP COLUMN `service_name`,
--   DROP COLUMN `is_shared`,
--   ADD UNIQUE KEY `uk_api_path` (`api_path_prefix`);
--
-- ALTER TABLE `oe_rate_limit_pending`
--   DROP INDEX `uk_pending_service_api`,
--   DROP COLUMN `service_name`,
--   ADD UNIQUE KEY `uk_api_path` (`api_path_prefix`);
