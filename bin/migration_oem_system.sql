-- ============================================
-- OEM贴牌白标系统 - Go网关数据库扩展脚本
-- ============================================
-- 创建日期: 2025-01-XX
-- 说明: 此脚本用于扩展Go网关数据库中的users和logs表
-- ============================================

-- ============================================
-- 1. 扩展 users 表
-- ============================================
-- 添加 system_code 字段
ALTER TABLE users 
ADD COLUMN system_code VARCHAR(32) DEFAULT 'nebula' COMMENT '所属系统代码';

-- 添加索引（MySQL语法）
-- 如果索引已存在，此语句会报错，可以忽略
CREATE INDEX idx_system_code ON users(system_code);

-- ============================================
-- 2. 扩展 logs 表
-- ============================================
-- 添加 system_code 字段
ALTER TABLE logs 
ADD COLUMN system_code VARCHAR(32) COMMENT '系统代码';

-- 添加价格链条字段
ALTER TABLE logs 
ADD COLUMN official_quota BIGINT COMMENT '官方价格quota',
ADD COLUMN cost_quota BIGINT COMMENT '平台成本quota',
ADD COLUMN system_quota BIGINT COMMENT '系统销售价quota',
ADD COLUMN user_quota BIGINT COMMENT '用户支付价quota',
ADD COLUMN platform_profit BIGINT COMMENT '平台利润（system_quota - cost_quota）',
ADD COLUMN oem_subsidy BIGINT COMMENT 'OEM补贴（system_quota - user_quota，负数表示补贴）';

-- 添加索引（MySQL语法）
-- 如果索引已存在，此语句会报错，可以忽略
CREATE INDEX idx_system_code ON logs(system_code);

-- ============================================
-- 3. 数据迁移 - 标记现有用户为nebula系统
-- ============================================
UPDATE users SET system_code = 'nebula' WHERE system_code IS NULL OR system_code = '';

-- ============================================
-- 4. 数据迁移 - 标记现有日志为nebula系统
-- ============================================
UPDATE logs SET system_code = 'nebula' WHERE system_code IS NULL OR system_code = '';

-- ============================================
-- 5. 初始化系统账户
-- ============================================
-- 注意：需要确保这些用户ID没有被占用
-- 如果已存在，请手动更新system_account_user_id
-- MySQL语法
INSERT INTO users (id, username, password, quota, `group`, system_code, role, status, display_name) VALUES
(10001, 'nebula_system_account', '', 0, 'system', 'nebula', 1, 1, 'Nebula系统账户'),
(10002, 'xiaomai_system_account', '', 1000000000, 'system', 'xiaomai', 1, 1, 'Xiaomai系统账户')
ON DUPLICATE KEY UPDATE
    username = VALUES(username),
    `group` = VALUES(`group`),
    system_code = VALUES(system_code),
    display_name = VALUES(display_name);

-- PostgreSQL/SQLite语法（如果使用PostgreSQL或SQLite，请使用以下语句）
-- INSERT INTO users (id, username, password, quota, "group", system_code, role, status, display_name) VALUES
-- (10001, 'nebula_system_account', '', 0, 'system', 'nebula', 1, 1, 'Nebula系统账户'),
-- (10002, 'xiaomai_system_account', '', 1000000000, 'system', 'xiaomai', 1, 1, 'Xiaomai系统账户')
-- ON CONFLICT(id) DO UPDATE SET
--     username = EXCLUDED.username,
--     "group" = EXCLUDED."group",
--     system_code = EXCLUDED.system_code,
--     display_name = EXCLUDED.display_name;

-- ============================================
-- 6. 初始化 GroupRatio 配置（options表）
-- ============================================
-- Nebula系统的GroupRatio配置
-- MySQL语法
INSERT INTO options (`key`, value) VALUES 
('GroupRatio_nebula', '{"default": 1.0, "vip": 1.0, "premium": 1.0}')
ON DUPLICATE KEY UPDATE value = VALUES(value);

-- Xiaomai系统的GroupRatio配置
INSERT INTO options (`key`, value) VALUES 
('GroupRatio_xiaomai', '{"default": 1.125, "vip": 0.875, "discount": 0.375}')
ON DUPLICATE KEY UPDATE value = VALUES(value);

-- PostgreSQL/SQLite语法（如果使用PostgreSQL或SQLite，请使用以下语句）
-- INSERT INTO options ("key", value) VALUES 
-- ('GroupRatio_nebula', '{"default": 1.0, "vip": 1.0, "premium": 1.0}')
-- ON CONFLICT("key") DO UPDATE SET value = EXCLUDED.value;
-- 
-- INSERT INTO options ("key", value) VALUES 
-- ('GroupRatio_xiaomai', '{"default": 1.125, "vip": 0.875, "discount": 0.375}')
-- ON CONFLICT("key") DO UPDATE SET value = EXCLUDED.value;

-- ============================================
-- 脚本执行完成
-- ============================================
-- 说明：
-- 1. 如果使用MySQL，请将 ON CONFLICT 替换为 ON DUPLICATE KEY UPDATE
-- 2. 如果使用PostgreSQL，保持 ON CONFLICT 语法
-- 3. 如果使用SQLite，保持 ON CONFLICT 语法
-- ============================================

