-- 正式环境：为 one_api 库添加 Seedream 5.0 渠道（供后续由运维/管理员执行）
-- 使用前请将 YOUR_VOLCENGINE_ARK_API_KEY 替换为火山方舟 Ark 的正式 API Key
-- 执行后需重启 Go 服务以使渠道缓存生效

-- 1) 插入渠道（type=45 为 VolcEngine）
INSERT INTO channels (
  type, `key`, name, status, created_time, test_time, `group`, models
) VALUES (
  45,
  'YOUR_VOLCENGINE_ARK_API_KEY',
  'Seedream 5.0',
  1,
  UNIX_TIMESTAMP(),
  0,
  'default',
  'doubao-seedream-5-0-260128'
);

-- 2) 插入能力
SET @channel_id = LAST_INSERT_ID();
INSERT INTO abilities (`group`, model, channel_id, enabled, priority, weight)
VALUES ('default', 'doubao-seedream-5-0-260128', @channel_id, 1, 0, 0);
