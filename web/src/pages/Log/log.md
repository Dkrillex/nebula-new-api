# Nebula Lab 日志界面计费流程梳理

本文档汇总 `web/src/pages/Log/index.jsx` 所在日志页面的完整计费逻辑，便于在其他模块复用或二次集成。

---

## 1. 页面结构与职责

- **入口组件**：`web/src/pages/Log/index.jsx`  
  - 负责渲染日志页面框架，核心内容来自 `UsageLogsTable` 组件。
- **核心组件目录**：`web/src/components/table/usage-logs/`  
  - `index.jsx`：组合筛选、操作、表格、列选择和用户信息等模块。  
  - `UsageLogsTable.jsx`：渲染 `CardTable` 表格。  
  - `UsageLogsColumnDefs.jsx`：定义列与计费详情展示逻辑。  
  - `modals` 子目录：列选择、用户详情等弹层。  
  - `UsageLogsFilters.jsx`、`UsageLogsActions.jsx`：提供筛选和操作按钮。
- **数据 Hook**：`web/src/hooks/usage-logs/useUsageLogsData.jsx`  
  - 负责数据获取、状态管理、统计计算、展开行内容等。
- **渲染工具**：`web/src/helpers/render.jsx`  
  - 提供 `renderModelPrice`、`renderModelPriceSimple`、`renderQuota` 等计费展示函数。

---

## 2. 前端数据流

### 2.1 数据加载

1. `useLogsData` 在初始化时读取本地分页、列配置并调用 `loadLogs`。
2. `loadLogs` 根据用户角色调用不同接口：  
   - 管理员：`GET /api/log/?...`  
   - 普通用户：`GET /api/log/self/?...`
3. 接口返回 `items/page/page_size/total`，`setLogsFormat` 负责格式化结果。

### 2.2 数据格式化逻辑

- 统一添加：
  - `timestamp2string`：把 `created_at` 转为可读时间。
  - `key`：使用日志 `id`。
- 解析 `other` JSON，构造展开行数据 `expandData`，包含：
  - 渠道信息（管理员可见）。
  - 语音/文字细节、缓存信息。
  - `renderLogContent` / `renderClaudeLogContent` 生成的长文本说明。
  - 计费过程：根据模型类型选择 `renderModelPrice`、`renderClaudeModelPrice`、`renderAudioModelPrice` 等函数生成详细描述。
  - Reasoning Effort、模型映射等额外字段。

### 2.3 统计数据

- `handleEyeClick` 触发统计查询：
  - 管理员：`GET /api/log/stat?...`
  - 普通用户：`GET /api/log/self/stat?...`
- 返回值包含 `quota`（额度）和 `token`（Token 数），用于统计弹层展示。

---

## 3. 表格列与计费展示

### 3.1 列定义要点（`UsageLogsColumnDefs.jsx`）

- `COLUMN_KEYS` 定义列标识，含 `TIME、CHANNEL、USER、TOKEN、TYPE、MODEL、PROMPT、COMPLETION、COST、DETAILS` 等。
- 渲染函数示例：
  - `renderQuota`：根据本地配置（USD/CNY/自定义/Tokens）格式化额度。
  - `renderModelName`：支持模型映射 Tooltip 展示。
  - `renderUseTime` 等：用 Tag 样式展示耗时、首字时间、流式标记。
  - `renderGroup`、`renderType`：使用 Tag 和颜色区分。

### 3.2 计费详情列（`DETAILS`）

- 核心分支逻辑：
  1. **图像 Token 表计费**：存在 `other.image_token_pricing`，使用 `renderImageTokenPricing`，并提供 Tooltip 展示质量、尺寸、输入/输出图片等。
  2. **视频按秒计费**：存在 `other.video_seconds` 与 `other.video_price_per_second`，调用 `renderVideoPerSecondPrice`。
  3. **常规模型**：
     - 若 `other.claude` 为真，调用 `renderModelPriceSimple(..., provider='claude')`。
     - 否则默认 `renderModelPriceSimple(..., provider='openai')`。
- 渲染结果通过 `Typography.Paragraph` 展示，支持折叠和 Tooltip。

### 3.3 展开行计费过程

- `useLogsData` 中在 `setLogsFormat` 构建展开数据：
  - 图像/音频/缓存信息。
  - `renderModelPrice` 或 `renderAudioModelPrice` 等函数生成的多段文本，包含：
    - 模型倍率、分组倍率、用户分组倍率。
    - 输入/输出 Token 数与倍率计算。
    - 缓存命中、Web/File 搜索、按次附加费用等。
    - 合计 Token 与折算金额。

---

## 4. 后端计费计算关键路径

### 4.1 Relay 层（`controller/relay.go`）

- `Relay` 按请求类型转发至对应 Helper（文本、图片、音频、文件等）。
- 请求上下文通过 `relaycommon.RelayInfo` 传递，包含用户、模型、分组等关键信息。

### 4.2 价格判定（`relay/helper/price.go`）

`ModelPriceHelper` 计算当前请求的计费策略，优先级如下：

1. **图像 Token 表定价**：`ratio_setting.GetImageTokenPricing`，设置 `UseImageTokenPricing=true`。
2. **按张计费**：`ratio_setting.GetImageModelPricePerImage`，设置 `UsePrice=true` 并给出预扣额度。
3. **按次计费**：`ratio_setting.GetModelPrice(UsePerCall=false)`。
4. **按 Token 倍率计费**：`ratio_setting.GetModelRatio`，结合 `CompletionRatio`、`GroupRatio`、`CacheRatio` 等计算预扣 Tokens。
5. **视频按秒计费**：`ratio_setting.GetVideoModelPricePerSecond` 或分辨率价格（如 `wan2.5` 系列）。

函数返回 `types.PriceData`，写入 `RelayInfo`，供后续扣费与记录使用。

### 4.3 额度计算（`service/quota.go`）

- `calculateAudioQuota`、`calculateQuota` 等函数负责将 Token 数与倍率转换为最终扣除的 Quota。
- 处理要点：
  - 按价格计费时：`quota = modelPrice × QuotaPerUnit × groupRatio`。
  - 按倍率计费时：根据输入/输出文本、音频、缓存 Token，乘以模型倍率、完成倍率、分组倍率、用户分组倍率等。
  - 向上取整 / 保底：确保倍率或结果为 0 时使用默认值。
  - 支持 Web 搜索、文件检索、按次附加费用等额外项。

### 4.4 日志记录（简述）

- Relay 在请求完成后根据 `PriceData` 与实际消耗生成日志。
- 日志写入数据库 `logs` 表，关键字段：
  - `type`：1=充值、2=消费、5=错误等。
  - `quota`：扣除的额度（整数）。
  - `prompt_tokens` / `completion_tokens`。
  - `model_name`、`channel`、`token_name`。
  - `other`：JSON 字符串，包含详细计费参数（倍率、价格、缓存、附加功能等）。

---

## 5. `other` 字段结构（常见字段示例）

```json
{
  "model_ratio": 0.5,
  "completion_ratio": 2,
  "group_ratio": 1,
  "user_group_ratio": 0.8,
  "cache_tokens": 150,
  "cache_ratio": 0.5,
  "model_price": 0.002,
  "per_call_price": 0.01,
  "per_call_image_multiplier": 2,
  "video_seconds": 12,
  "video_price_per_second": 0.02,
  "image_token_pricing": {
    "input_text_price": 0.000005,
    "input_image_price": 0.000012,
    "output_image_price": 0.00004
  },
  "input_text_tokens": 800,
  "input_image_tokens": 500,
  "output_tokens": 1200,
  "input_images_count": 2,
  "output_images_count": 1,
  "claude": false,
  "audio": false,
  "web_search": true,
  "web_search_call_count": 2,
  "web_search_price": 0.001,
  "file_search": false,
  "is_model_mapped": true,
  "upstream_model_name": "gpt-4o",
  "admin_info": {
    "is_multi_key": true,
    "multi_key_index": 1,
    "use_channel": [12, 13, 2]
  },
  "reasoning_effort": "medium"
}
```

> 实际字段会按功能不同组合，前端通过 `getLogOther` 做安全解析后再渲染。

---

## 6. 在其他模块复用的建议

### 6.1 轻量复用（仅展示计费结果）

1. 引入渲染函数：
   ```javascript
   import { renderModelPriceSimple, renderModelPrice, renderQuota } from 'web/src/helpers/render';
   ```
2. 根据日志对象（或其它 API 返回）里的字段调用：
   - 表格列展示额度：`renderQuota(log.quota, 6)`
   - 简化计费详情：`renderModelPriceSimple(...)`
   - 展开行或详情页面：`renderModelPrice(...)`
3. 确保准备好 `model_ratio/group_ratio/cache_tokens` 等参数（通常从后端 `other` 字段解析）。

### 6.2 完整复用（含筛选、分页、统计）

- 拷贝下列组件与 Hook：
  - `web/src/components/table/usage-logs/` 全目录。
  - `web/src/hooks/usage-logs/useUsageLogsData.jsx`。
  - 相关公共组件：`CardTable`、`CardPro`、`UsageLogsFilters` 等（位于 `web/src/components/common/ui/` 和 `web/src/components/table/`）。
- 确保同时引入：
  - 权限检测：`helpers/utils` 内的 `isAdmin`。
  - API 封装：`helpers/api`（在 `helpers/utils.jsx` 内导出 `API` 实例）。
  - 国际化 `t` 函数：依赖 `useTranslation`。
  - 本地存储列配置：`localStorage` key 需保持一致或自定义。

### 6.3 后端依赖

- 确保目标系统也实现以下接口或兼容数据返回格式：
  - `GET /api/log/`、`GET /api/log/self/`
  - `GET /api/log/stat`、`GET /api/log/self/stat`
  - 日志记录结构包含 `quota/prompt_tokens/completion_tokens/other` 等字段。
- 若后端不同，可在前端解析层中写适配器，把返回结构转换为 `UsageLogsTable` 期望的数据格式。

---

## 7. 排错与扩展提示

- **额度显示为 0 或 NaN**：检查本地 `localStorage.quota_per_unit` 与 `quota_display_type` 是否设置正确；确认后端返回的 `quota` 为整数。
- **展开行无“计费过程”**：`other` 字段可能缺失关键倍率信息，需要后端生成时补齐。
- **图像/视频计费未生效**：确认 `ratio_setting` 中是否配置了对应模型的价格表。
- **用户/渠道列为空**：普通用户默认隐藏，管理员需确认 `isAdmin()` 返回正确且列配置未关闭。
- **多 Key 渠道信息缺失**：`admin_info.is_multi_key` 字段需由后端在日志 `other` 中写入。

---

## 8. 字段与函数对照表

| 功能 | 前端函数 | 来源文件 |
| ---- | -------- | -------- |
| 额度格式化 | `renderQuota` | `web/src/helpers/render.jsx` |
| 模型计费详情 | `renderModelPrice` / `renderModelPriceSimple` | `web/src/helpers/render.jsx` |
| Claude 计费 | `renderClaudeModelPrice` / `renderClaudeModelPriceSimple` | `render.jsx` |
| 图像计费 | `renderImageTokenPricing` | `render.jsx` |
| 视频按秒计费 | `renderVideoPerSecondPrice` | `render.jsx` |
| 音频计费 | `renderAudioModelPrice` | `render.jsx` |
| 日志详情渲染 | `renderLogContent` / `renderClaudeLogContent` | `render.jsx` |
| 列定义入口 | `getLogsColumns` | `UsageLogsColumnDefs.jsx` |
| 数据加载 | `loadLogs` | `useUsageLogsData.jsx` |
| 统计查询 | `getLogStat` / `getLogSelfStat` | `useUsageLogsData.jsx` |
| 价格判定 | `ModelPriceHelper` | `relay/helper/price.go` |
| 额度计算 | `calculateQuota` / `calculateAudioQuota` | `service/quota.go` |

---

## 9. 集成步骤速览

1. **明确需求**：是展示现有日志，还是在新场景复用计费展示逻辑。
2. **准备数据**：确保后端返回包含 `prompt_tokens/completion_tokens/quota/other` 等字段。
3. **解析 `other`**：统一用 `getLogOther`（位于 `helpers/utils.jsx`）安全解析。
4. **选择展示模式**：
   - 简单模式：直接调用 `renderModelPriceSimple` + `renderQuota`。
   - 完整模式：复用 `UsageLogsTable` 全套组件。
5. **调整列和权限**：根据角色控制列显隐、渠道信息展示等。
6. **测试场景**：
   - 普通模型按倍率计费。
   - 按次计费模型。
   - 图像 Token 表计费。
   - 视频按秒计费。
   - 含缓存、Web 搜索、文件搜索等附加费用的场景。
7. **验证统计**：检查 `stat.quota` 与 `stat.token` 是否符合预期。

---

通过以上梳理，可以快速定位日志计费流程中的关键节点，并在其他页面或服务中复用计费展示逻辑。若需进一步扩展（如新增计费类型、接入新的后端接口），建议从 `render.jsx` 的渲染函数和 `relay/helper/price.go` 的价格判定逻辑入手。随后同步更新前端 `UsageLogsColumnDefs.jsx` 的分支，以确保展示与计算保持一致。

