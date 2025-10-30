-- 一次性历史数据清理：将 Veo 成功任务中形如 data:... 的大字段清空或标记为 expired
-- SQLite 兼容写法（根据实际数据库类型调整）

-- 方案A：置为短标记
UPDATE tasks
SET fail_reason = 'expired'
WHERE status = 'SUCCESS'
  AND LOWER(model_name) LIKE '%veo%'
  AND fail_reason LIKE 'data:%;base64,%';

-- 如需直接清空，请使用：
-- UPDATE tasks
-- SET fail_reason = ''
-- WHERE status = 'SUCCESS'
--   AND LOWER(model_name) LIKE '%veo%'
--   AND fail_reason LIKE 'data:%;base64,%';


