## 概述

本文档介绍如何通过 Nebula API 的 OpenAI 兼容接口调用 Google Gemini 通用对话模型（含思考模式与搜索）。

## 基础信息

| 项目 | 内容 |
|------|------|
| Base URL | `https://llm.ai-nebula.com/v1/chat/completions` |
| 认证方式 | API Key (Token) |
| 请求头 | `Authorization: Bearer sk-xxxx`、`Content-Type: application/json` |

## 支持的模型（示例）

- `gemini-2.5-flash` / `gemini-2.5-flash-preview-09-2025`
- `gemini-2.5-flash-lite-preview-09-2025`
- `gemini-2.5-pro`
- `gemini-3-pro-preview`（默认开启思考，使用 thinking_level）
- `gemini-3-pro-preview-thinking-low`
- `gemini-3-pro-preview-thinking-high`

> 以路由配置为准，如有疑问请咨询管理员。

---

## API 接口

### 1. 最小示例（非流式）

```bash
curl -X POST "https://llm.ai-nebula.com/v1/chat/completions" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-xxxx" \
  -d '{
    "model": "gemini-2.5-flash",
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
    "model": "gemini-2.5-flash",
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
- 工具调用：`tools/tool_choice`（OpenAI 兼容格式）
- 思考模式：`thinking_budget`（2.5 系列）或 `thinking_level`（3 Pro Preview）
- 搜索：`googleSearch` 工具

### 4. 工具调用（Functions / Tools）

OpenAI 兼容写法：

```bash
curl -X POST "https://llm.ai-nebula.com/v1/chat/completions" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-xxxx" \
  -d '{
    "model": "gemini-2.5-flash",
    "messages": [
      {"role":"user","content":"广州的天气怎么样？"}
    ],
    "tools": [
      {
        "type": "function",
        "function": {
          "name": "get_weather",
          "description": "根据城市获取天气信息",
          "parameters": {
            "type": "object",
            "properties": {"city": {"type": "string"}},
            "required": ["city"]
          }
        }
      }
    ],
    "tool_choice": "auto"
  }'
```

原生搜索透传（可选，二选一或并存）：

```json
"extra_body": {
  "google": {
    "tools": [
      { "googleSearch": {} }
    ]
  }
}
```

### 5. 思考能力（Thinking）

- 2.5 系列：使用 `thinking_budget`（数字）。`-1` 自动，`0` 关闭，`>0` 指定预算。
- 3 Pro Preview：使用 `thinking_level`（`LOW`/`HIGH`），默认 `HIGH`，无需数字预算。

示例：2.5 系列指定预算

```bash
curl -X POST "https://llm.ai-nebula.com/v1/chat/completions" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-xxxx" \
  -d '{
    "model": "gemini-2.5-flash",
    "messages": [
      {"role":"user","content":"给出一道中等难度的几何题并分步解析。"}
    ],
    "extra_body": {
      "google": {
        "thinking_config": { "thinking_budget": 6000, "include_thoughts": true }
      }
    },
    "stream": true
  }'
```

示例：3 Pro Preview 指定思考级别并开启搜索（推荐写法）

```bash
curl -X POST "https://llm.ai-nebula.com/v1/chat/completions" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-xxxx" \
  -d '{
    "model": "gemini-3-pro-preview",
    "messages": [
      {"role":"user","content":"今天广州的天气怎么样"}
    ],
    "generationConfig": {
      "temperature": 1,
      "maxOutputTokens": 65535,
      "topP": 0.95,
      "thinkingConfig": { "thinkingLevel": "LOW" }
    },
    "tools": [
      { "type": "function", "function": { "name": "googleSearch" } }
    ],
    "stream": true
  }'
```

---

## 响应与用量

- 非流式：一次性返回 `choices`、`usage`
- 流式：SSE 分片返回；若渠道支持 `stream_options.include_usage=true`，可在分片内返回实时用量

---

## 常见问题（FAQ）

1) 与 OpenAI 兼容程度？  
- 使用 OpenAI Chat Completions 格式；少数字段可能因渠道差异被忽略。

2) 思考模式如何开启？  
- 2.5：`thinking_budget` 或模型后缀 `-thinking`/`-thinking-<数字>`，`-nothinking` 关闭。  
- 3 Pro Preview：`thinking_level`（LOW/HIGH），或后缀 `-thinking-low/-thinking-high`（如 `gemini-3-pro-preview-thinking-low`），默认 HIGH。

3) 搜索如何开启？  
- 推荐 tools 函数 `googleSearch`（兼容格式）；或 `extra_body.google.tools` 透传原生。

4) 会计费吗？  
- 思考消耗计入输出 token；搜索按渠道策略计费。

---

## 最佳实践

- 使用流式提升首字时间与交互体验。
- 结构化/确定性输出时降低 `temperature`，控制 `max_tokens`。
- 工具调用做好超时与重试；搜索结果可做兜底校验。


