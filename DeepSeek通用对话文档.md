## 概述

本文档介绍如何通过 Nebula 的 OpenAI 兼容接口调用 DeepSeek 对话模型。

## 基础信息

| 项目 | 内容 |
|------|------|
| Base URL | `https://llm.ai-nebula.com/v1/chat/completions` |
| 认证方式 | API Key (Token) |
| 请求头 | `Authorization: Bearer sk-xxxx`、`Content-Type: application/json` |

## 支持的模型（示例）

- `deepseek-v3-1`
- 其他 DeepSeek 系列（以路由配置为准）

---

## API 接口

### 1. 最小示例（非流式）

```bash
curl -X POST "https://llm.ai-nebula.com/v1/chat/completions" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-xxxx" \
  -d '{
    "model": "deepseek-v3-1",
    "messages": [
      {"role":"system","content":"You are a helpful assistant."},
      {"role":"user","content":"Explain gradient descent in one paragraph."}
    ]
  }'
```

### 2. 流式 SSE 示例

```bash
curl -N -X POST "https://llm.ai-nebula.com/v1/chat/completions" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-xxxx" \
  -d '{
    "model": "deepseek-v3-1",
    "stream": true,
    "messages": [
      {"role":"system","content":"You are a helpful assistant."},
      {"role":"user","content":"Explain gradient descent in one paragraph."}
    ]
  }'
```

### 3. 常用参数

- 采样与控制：`temperature`、`top_p`、`max_tokens`、`stop`
- 结构化输出：`response_format/json_schema`
- 工具调用：`tools/tool_choice`（遵循 OpenAI 兼容格式）

> 注：DeepSeek 在不同渠道可能支持更多特性或差异化字段，Nebula 会尽量在兼容层透传与规整，建议仅使用通用字段，或咨询渠道支持列表。

### 4. 思考能力（Thinking）

DeepSeek（经火山引擎 Ark 渠道）支持以 `thinking` 字段开启/关闭思考能力。默认关闭：

```json
thinking={
  "type": "disabled"  // 默认行为：关闭思考能力
  // "type": "enabled" // 开启思考能力
}
```

在 OpenAI 兼容请求中，可直接在顶层传入 `thinking` 字段（Nebula 将按渠道要求透传）：

```bash
curl -X POST "https://llm.ai-nebula.com/v1/chat/completions" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-xxxx" \
  -d '{
    "model": "deepseek-v3-1",
    "messages": [
      {"role":"system","content":"You are a helpful assistant."},
      {"role":"user","content":"给出一道中等难度的几何题并分步解析。"}
    ],
    "thinking": {"type": "enabled"}
  }'
```

说明：
- 不同模型/版本对思考能力的输出形态可能不同（例如是否返回显式 reasoning 字段或仅体现在内容结构中）。
- 若需在终端直观看到思考过程且渠道以流式返回，你可以搭配 `stream: true` 以更好的交互体验。

---

## 响应与用量

- 非流式：一次性返回 `choices`、`usage`
- 流式：SSE 分片返回，末尾可能包含 `usage`；若渠道支持 `stream_options.include_usage=true`，可能在分片内返回实时用量

---

## 常见问题（FAQ）

1) 与 OpenAI 兼容程度？
- 使用 OpenAI Chat Completions 格式；少数扩展字段可能不生效，以渠道支持为准

2) 是否支持结构化输出？
- 支持 `response_format: json_schema`；复杂 Schema 时建议降低 `temperature` 提升一致性

3) 是否支持思维链/搜索类开关？
- 视渠道与模型版本而定；如需专属能力可联系管理员开通或在 `parameters` 透传（若通道支持）

---

## 最佳实践

- 使用流式提升首字时间与交互体验
- 严格结构化输出时降低 `temperature`，并控制 `max_tokens`
- 对工具调用结果做好容错、重试与超时控制


