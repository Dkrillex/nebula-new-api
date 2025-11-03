# 后端代码目录

本文档用于快速定位后端Go代码位置。后端代码主要分为以下几个模块：

## 目录结构概览

```
├── router/          # 路由配置
├── controller/      # 控制器（HTTP处理器）
├── service/         # 业务逻辑服务层
├── model/           # 数据模型和数据库操作
├── dto/             # 数据传输对象
├── relay/           # 中继转发逻辑
├── middleware/      # 中间件
├── setting/         # 配置管理
├── types/           # 类型定义
├── constant/        # 常量定义
└── common/          # 通用工具函数
```

## 路由模块 (router/)

路由配置目录，负责将HTTP请求路由到对应的控制器。

| 文件 | 说明 | 主要路由 |
|-----|------|---------|
| `router/main.go` | 路由主入口，聚合所有路由 | - |
| `router/web-router.go` | Web前端静态资源路由 | 静态文件服务 |
| `router/api-router.go` | Web API路由配置 | `/api/*` |
| `router/relay-router.go` | 中继转发路由配置 | `/v1/*`, `/mj/*`, `/suno/*` |
| `router/dashboard.go` | 仪表盘路由 | - |
| `router/video-router.go` | 视频相关路由 | `/api/video/*` |

### API路由组织 (api-router.go)
- `/api/user/*` - 用户相关API
- `/api/channel/*` - 渠道管理API
- `/api/token/*` - Token管理API
- `/api/redemption/*` - 兑换码API
- `/api/log/*` - 日志API
- `/api/models/*` - 模型管理API
- `/api/vendors/*` - 供应商API
- `/api/task/*` - 任务API
- `/api/mj/*` - Midjourney API
- `/api/sync/system/*` - 系统同步API（使用access_token认证）

### 中继路由组织 (relay-router.go)
- `/v1/models` - 模型列表
- `/v1/chat/completions` - 聊天补全
- `/v1/completions` - 文本补全
- `/v1/images/generations` - 图像生成
- `/v1/embeddings` - 嵌入向量
- `/v1/audio/*` - 音频处理
- `/v1/rerank` - 重排序
- `/mj/*` - Midjourney相关
- `/suno/*` - Suno音频生成

## 控制器模块 (controller/)

HTTP请求处理器，处理具体的API请求。

### 核心控制器

| 文件 | 主要功能 | 相关API |
|-----|---------|---------|
| `controller/user.go` | 用户管理（注册、登录、CRUD） | `/api/user/*` |
| `controller/channel.go` | 渠道管理 | `/api/channel/*` |
| `controller/token.go` | Token管理 | `/api/token/*` |
| `controller/redemption.go` | 兑换码管理 | `/api/redemption/*` |
| `controller/log.go` | 日志查询 | `/api/log/*` |
| `controller/model.go` | 模型管理 | `/api/models/*` |
| `controller/model_meta.go` | 模型元数据管理 | `/api/models/*` |
| `controller/vendor_meta.go` | 供应商管理 | `/api/vendors/*` |

### 中继相关

| 文件 | 主要功能 | 相关路由 |
|-----|---------|---------|
| `controller/relay.go` | 中继转发入口 | `/v1/*` |
| `controller/midjourney.go` | Midjourney处理 | `/mj/*` |
| `controller/task.go` | 任务处理 | `/api/task/*` |
| `controller/task_video.go` | 视频任务处理 | `/api/video/*` |
| `controller/swag_video.go` | Swag视频处理 | - |

### 认证和授权

| 文件 | 主要功能 |
|-----|---------|
| `controller/setup.go` | 系统初始化 |
| `controller/github.go` | GitHub OAuth |
| `controller/oidc.go` | OIDC认证 |
| `controller/linuxdo.go` | LinuxDo OAuth |
| `controller/wechat.go` | 微信认证 |
| `controller/telegram.go` | Telegram认证 |
| `controller/passkey.go` | Passkey认证 |
| `controller/twofa.go` | 双因素认证 |
| `controller/secure_verification.go` | 安全验证 |

### 支付和充值

| 文件 | 主要功能 |
|-----|---------|
| `controller/topup.go` | 充值管理 |
| `controller/topup_stripe.go` | Stripe支付 |
| `controller/billing.go` | 账单管理 |
| `controller/channel-billing.go` | 渠道账单 |

### 设置和配置

| 文件 | 主要功能 |
|-----|---------|
| `controller/option.go` | 系统选项配置 |
| `controller/ratio_config.go` | 费率配置 |
| `controller/ratio_sync.go` | 费率同步 |
| `controller/misc.go` | 杂项配置 |

### 其他功能

| 文件 | 主要功能 |
|-----|---------|
| `controller/playground.go` | Playground API |
| `controller/pricing.go` | 定价信息 |
| `controller/image.go` | 图像处理 |
| `controller/sync.go` | 系统同步 |
| `controller/model_sync.go` | 模型同步 |
| `controller/missing_models.go` | 缺失模型检测 |
| `controller/video_download.go` | 视频下载 |
| `controller/video_proxy.go` | 视频代理 |
| `controller/uptime_kuma.go` | Uptime Kuma集成 |
| `controller/channel-test.go` | 渠道测试 |
| `controller/prefill_group.go` | 预填充组管理 |
| `controller/group.go` | 用户组管理 |
| `controller/usedata.go` | 使用数据 |
| `controller/console_migrate.go` | 控制台迁移 |

## 服务层 (service/)

业务逻辑服务层，封装复杂的业务逻辑。

| 文件 | 主要功能 |
|-----|---------|
| `service/quota.go` | 配额管理服务 |
| `service/pre_consume_quota.go` | 预消费配额 |
| `service/token_counter.go` | Token计数服务 |
| `service/channel.go` | 渠道服务 |
| `service/convert.go` | 数据转换服务 |
| `service/image.go` | 图像处理服务 |
| `service/audio.go` | 音频处理服务 |
| `service/midjourney.go` | Midjourney服务 |
| `service/task.go` | 任务服务 |
| `service/file_decoder.go` | 文件解码服务 |
| `service/user_notify.go` | 用户通知服务 |
| `service/webhook.go` | Webhook服务 |
| `service/sensitive.go` | 敏感词检测 |
| `service/notify-limit.go` | 通知限流 |
| `service/error.go` | 错误处理服务 |
| `service/download.go` | 下载服务 |
| `service/http_client.go` | HTTP客户端 |
| `service/http.go` | HTTP工具 |
| `service/str.go` | 字符串处理 |
| `service/log_info_generate.go` | 日志信息生成 |
| `service/usage_helpr.go` | 使用数据帮助 |
| `service/epay.go` | 易支付服务 |
| `service/passkey/` | Passkey相关服务 |

## 数据模型 (model/)

数据库模型定义和ORM操作。

| 文件 | 主要功能 | 数据表/模型 |
|-----|---------|-----------|
| `model/main.go` | 模型初始化 | - |
| `model/user.go` | 用户模型 | User |
| `model/token.go` | Token模型 | Token |
| `model/channel.go` | 渠道模型 | Channel |
| `model/log.go` | 日志模型 | Log |
| `model/model_meta.go` | 模型元数据 | Model |
| `model/vendor_meta.go` | 供应商元数据 | Vendor |
| `model/pricing.go` | 定价模型 | Pricing |
| `model/option.go` | 选项模型 | Option |
| `model/redemption.go` | 兑换码模型 | Redemption |
| `model/topup.go` | 充值模型 | TopUp |
| `model/midjourney.go` | Midjourney模型 | Midjourney |
| `model/task.go` | 任务模型 | Task |
| `model/ability.go` | 能力模型 | Ability |
| `model/twofa.go` | 双因素认证模型 | TwoFA |
| `model/passkey.go` | Passkey模型 | Passkey |
| `model/prefill_group.go` | 预填充组模型 | PrefillGroup |
| `model/user_cache.go` | 用户缓存 | - |
| `model/channel_cache.go` | 渠道缓存 | - |
| `model/token_cache.go` | Token缓存 | - |
| `model/usedata.go` | 使用数据模型 | - |
| `model/utils.go` | 模型工具函数 | - |
| `model/veo_cleanup.go` | VEO清理 | - |
| `model/missing_models.go` | 缺失模型 | - |
| `model/model_extra.go` | 模型扩展 | - |
| `model/pricing_default.go` | 默认定价 | - |
| `model/pricing_refresh.go` | 定价刷新 | - |
| `model/setup.go` | 初始化设置 | - |

## 中继模块 (relay/)

中继转发核心逻辑，处理各种AI服务的请求转发。

### 核心文件

| 文件 | 主要功能 |
|-----|---------|
| `relay/compatible_handler.go` | 兼容性处理器 |
| `relay/image_handler.go` | 图像处理 |
| `relay/embedding_handler.go` | 嵌入向量处理 |
| `relay/audio_handler.go` | 音频处理 |
| `relay/rerank_handler.go` | 重排序处理 |
| `relay/responses_handler.go` | 响应处理 |
| `relay/gemini_handler.go` | Gemini处理 |
| `relay/claude_handler.go` | Claude处理 |
| `relay/mjproxy_handler.go` | Midjourney代理 |
| `relay/relay_task.go` | 任务中继 |
| `relay/relay_adaptor.go` | 中继适配器 |
| `relay/websocket.go` | WebSocket处理 |

### 渠道适配器 (relay/channel/)

各种AI服务提供商的适配器实现：

#### 文本生成类
- `relay/channel/openai/` - OpenAI适配器
- `relay/channel/claude/` - Claude适配器
- `relay/channel/gemini/` - Gemini适配器
- `relay/channel/baidu/` - 百度适配器
- `relay/channel/baidu_v2/` - 百度v2适配器
- `relay/channel/ali/` - 阿里云适配器
- `relay/channel/tencent/` - 腾讯适配器
- `relay/channel/zhipu/` - 智谱适配器
- `relay/channel/zhipu_4v/` - 智谱4V适配器
- `relay/channel/deepseek/` - DeepSeek适配器
- `relay/channel/moonshot/` - Moonshot适配器
- `relay/channel/mistral/` - Mistral适配器
- `relay/channel/minimax/` - MiniMax适配器
- `relay/channel/ollama/` - Ollama适配器
- `relay/channel/cohere/` - Cohere适配器
- `relay/channel/perplexity/` - Perplexity适配器
- `relay/channel/xai/` - xAI适配器
- `relay/channel/jimeng/` - 即梦适配器
- `relay/channel/siliconflow/` - SiliconFlow适配器
- `relay/channel/mokaai/` - MokaAI适配器
- `relay/channel/xunfei/` - 讯飞适配器
- `relay/channel/volcengine/` - 火山引擎适配器
- `relay/channel/lingyiwanwu/` - 零一万物适配器
- `relay/channel/ai360/` - 360适配器
- `relay/channel/coze/` - Coze适配器
- `relay/channel/dify/` - Dify适配器

#### 云服务类
- `relay/channel/aws/` - AWS适配器
- `relay/channel/vertex/` - Google Vertex适配器
- `relay/channel/cloudflare/` - Cloudflare适配器

#### 特殊功能类
- `relay/channel/openrouter/` - OpenRouter适配器
- `relay/channel/palm/` - PaLM适配器
- `relay/channel/jina/` - Jina适配器
- `relay/channel/xinference/` - Xinference适配器
- `relay/channel/submodel/` - 子模型适配器

#### 任务类适配器 (relay/channel/task/)
- `relay/channel/task/doubao/` - 豆包视频生成
- `relay/channel/task/jimeng/` - 即梦任务
- `relay/channel/task/kling/` - Kling视频生成
- `relay/channel/task/sora/` - Sora视频生成
- `relay/channel/task/suno/` - Suno音频生成
- `relay/channel/task/vertex/` - Vertex任务
- `relay/channel/task/vidu/` - Vidu视频生成

## 数据传输对象 (dto/)

API请求和响应的数据结构定义。

| 文件 | 主要功能 |
|-----|---------|
| `dto/openai_request.go` | OpenAI请求DTO |
| `dto/openai_response.go` | OpenAI响应DTO |
| `dto/gemini.go` | Gemini DTO |
| `dto/claude.go` | Claude DTO |
| `dto/openai_image.go` | OpenAI图像DTO |
| `dto/video.go` | 视频DTO |
| `dto/audio.go` | 音频DTO |
| `dto/embedding.go` | 嵌入向量DTO |
| `dto/rerank.go` | 重排序DTO |
| `dto/midjourney.go` | Midjourney DTO |
| `dto/suno.go` | Suno DTO |
| `dto/realtime.go` | 实时API DTO |
| `dto/task.go` | 任务DTO |
| `dto/playground.go` | Playground DTO |
| `dto/pricing.go` | 定价DTO |
| `dto/user_settings.go` | 用户设置DTO |
| `dto/channel_settings.go` | 渠道设置DTO |
| `dto/ratio_sync.go` | 费率同步DTO |
| `dto/request_common.go` | 通用请求DTO |
| `dto/error.go` | 错误DTO |
| `dto/sensitive.go` | 敏感词DTO |
| `dto/notify.go` | 通知DTO |

## 配置管理 (setting/)

系统配置管理模块。

### 系统设置 (setting/system_setting/)
- `fetch_setting.go` - 获取设置
- `oidc.go` - OIDC配置
- `passkey.go` - Passkey配置
- `legal.go` - 法律条款配置

### 控制台设置 (setting/console_setting/)
- `config.go` - 控制台配置
- `validation.go` - 配置验证

### 运营设置 (setting/operation_setting/)
- `operation_setting.go` - 运营设置主文件
- `general_setting.go` - 通用设置
- `monitor_setting.go` - 监控设置
- `payment_setting.go` - 支付设置
- `tools.go` - 工具设置

### 模型设置 (setting/model_setting/)
- `global.go` - 全局模型设置
- `claude.go` - Claude模型设置
- `gemini.go` - Gemini模型设置

### 费率设置 (setting/ratio_setting/)
- `model_ratio.go` - 模型费率
- `group_ratio.go` - 组费率
- `expose_ratio.go` - 暴露费率
- `cache_ratio.go` - 缓存费率
- `exposed_cache.go` - 暴露缓存

### 其他配置
- `setting/chat.go` - 聊天配置
- `setting/rate_limit.go` - 限流配置
- `setting/user_usable_group.go` - 用户可用组配置
- `setting/auto_group.go` - 自动分组配置
- `setting/sensitive.go` - 敏感词配置
- `setting/midjourney.go` - Midjourney配置
- `setting/payment_stripe.go` - Stripe支付配置

## 中间件 (middleware/)

HTTP中间件，处理认证、限流、CORS等功能。

| 文件 | 主要功能 |
|-----|---------|
| `middleware/auth.go` | 认证中间件（用户/管理员/Token） |
| `middleware/rate-limit.go` | 限流中间件 |
| `middleware/model-rate-limit.go` | 模型限流 |
| `middleware/distributor.go` | 请求分发器 |
| `middleware/cors.go` | CORS处理 |
| `middleware/cache.go` | 缓存中间件 |
| `middleware/disable-cache.go` | 禁用缓存 |
| `middleware/logger.go` | 日志记录 |
| `middleware/recover.go` | 错误恢复 |
| `middleware/request-id.go` | 请求ID |
| `middleware/stats.go` | 统计中间件 |
| `middleware/turnstile-check.go` | Turnstile验证 |
| `middleware/secure_verification.go` | 安全验证 |
| `middleware/email-verification-rate-limit.go` | 邮件验证限流 |
| `middleware/jimeng_adapter.go` | 即梦适配器中间件 |
| `middleware/doubao_adapter.go` | 豆包适配器中间件 |
| `middleware/kling_adapter.go` | Kling适配器中间件 |
| `middleware/utils.go` | 中间件工具 |

## 类型定义 (types/)

通用类型定义。

| 文件 | 主要功能 |
|-----|---------|
| `types/error.go` | 错误类型定义 |
| `types/request_meta.go` | 请求元数据类型 |
| `types/relay_format.go` | 中继格式类型 |
| `types/channel_error.go` | 渠道错误类型 |
| `types/price_data.go` | 价格数据类型 |
| `types/file_data.go` | 文件数据类型 |
| `types/set.go` | 集合类型 |

## 常量定义 (constant/)

系统常量定义。

| 文件 | 主要功能 |
|-----|---------|
| `constant/channel.go` | 渠道相关常量 |
| `constant/api_type.go` | API类型常量 |
| `constant/endpoint_type.go` | 端点类型常量 |
| `constant/task.go` | 任务相关常量 |
| `constant/midjourney.go` | Midjourney常量 |
| `constant/context_key.go` | 上下文键常量 |
| `constant/finish_reason.go` | 完成原因常量 |
| `constant/multi_key_mode.go` | 多密钥模式常量 |
| `constant/azure.go` | Azure常量 |
| `constant/cache_key.go` | 缓存键常量 |
| `constant/env.go` | 环境变量常量 |
| `constant/setup.go` | 初始化常量 |

## 通用工具 (common/)

通用工具函数和辅助代码。

| 文件 | 主要功能 |
|-----|---------|
| `common/utils.go` | 通用工具函数 |
| `common/str.go` | 字符串处理 |
| `common/json.go` | JSON处理 |
| `common/crypto.go` | 加密解密 |
| `common/hash.go` | 哈希处理 |
| `common/gin.go` | Gin框架辅助 |
| `common/database.go` | 数据库工具 |
| `common/redis.go` | Redis工具 |
| `common/email.go` | 邮件发送 |
| `common/totp.go` | TOTP验证 |
| `common/verification.go` | 验证码处理 |
| `common/ip.go` | IP处理 |
| `common/model.go` | 模型工具 |
| `common/api_type.go` | API类型工具 |
| `common/endpoint_type.go` | 端点类型工具 |
| `common/endpoint_defaults.go` | 端点默认值 |
| `common/constants.go` | 通用常量 |
| `common/page_info.go` | 分页信息 |
| `common/truncation.go` | 截断处理 |
| `common/ssrf_protection.go` | SSRF防护 |
| `common/rate-limit.go` | 限流工具 |
| `common/quota.go` | 配额工具 |
| `common/topup-ratio.go` | 充值比例 |
| `common/limiter/` | 限流器实现 |
| `common/init.go` | 初始化 |
| `common/sys_log.go` | 系统日志 |
| `common/pprof.go` | 性能分析 |
| `common/copy.go` | 复制工具 |
| `common/custom-event.go` | 自定义事件 |
| `common/go-channel.go` | Go通道工具 |
| `common/gopool.go` | Go协程池 |
| `common/validate.go` | 验证工具 |

## 快速定位指南

### 当遇到问题时：

1. **API路由问题** → 查看 `router/api-router.go`
2. **中继转发问题** → 查看 `router/relay-router.go` 和 `relay/` 目录
3. **用户相关功能** → `controller/user.go`, `model/user.go`, `service/user_notify.go`
4. **渠道管理** → `controller/channel.go`, `model/channel.go`, `service/channel.go`
5. **Token管理** → `controller/token.go`, `model/token.go`, `service/token_counter.go`
6. **认证问题** → `controller/passkey.go`, `controller/twofa.go`, `middleware/auth.go`
7. **支付充值** → `controller/topup.go`, `controller/topup_stripe.go`, `model/topup.go`
8. **配额管理** → `service/quota.go`, `service/pre_consume_quota.go`
9. **图像处理** → `relay/image_handler.go`, `service/image.go`, `relay/channel/*/image.go`
10. **视频任务** → `controller/task_video.go`, `relay/channel/task/`
11. **Midjourney** → `controller/midjourney.go`, `relay/mjproxy_handler.go`, `service/midjourney.go`
12. **配置管理** → `controller/option.go`, `setting/` 目录
13. **数据库操作** → `model/` 目录对应模型文件
14. **业务逻辑** → `service/` 目录对应服务文件
15. **中间件** → `middleware/` 目录

### 渠道适配器定位

- **特定AI服务问题** → `relay/channel/{服务名}/`
- **文本生成适配** → `relay/channel/{服务名}/adaptor.go` 或 `relay-{服务名}.go`
- **图像生成适配** → `relay/channel/{服务名}/image.go`
- **视频生成适配** → `relay/channel/task/{服务名}/`

