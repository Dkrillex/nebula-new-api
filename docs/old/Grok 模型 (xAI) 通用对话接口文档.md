## 概述

本文档介绍如何通过 Nebula Api 的 OpenAI 兼容接口调用 Grok 模型能力，包含最小示例、流式推送、工具调用与结构化输出等要点。

## 基础信息

| 项目 | 内容 |
|------|------|
| Base URL | `https://llm.ai-nebula.com/v1/chat/completions` |
| 认证方式 | API Key (Token) |
| 请求头 | `Authorization: Bearer sk-xxxx`、`Content-Type: application/json` |

## 支持的模型

- **grok-3**：标准版本
- **grok-4**：最新版本
- **grok-3-fast**：快速版本
- **grok-4-fast-reasoning**：快速推理版本，专门用于需要深度推理的场景

---

## API 接口

### 1. 最小示例（非流式）

```bash
curl -X POST "https://llm.ai-nebula.com/v1/chat/completions" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-xxxx" \
  -d '{
    "model": "grok-4",
    "messages": [
      {"role":"system","content":"You are a helpful assistant."},
      {"role":"user","content":"Explain backprop in 2 sentences."}
    ]
  }'
```

### 2. 流式 SSE 示例

```bash
curl -N -X POST "https://llm.ai-nebula.com/v1/chat/completions" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-xxxx" \
  -d '{
    "model": "grok-4",
    "stream": true,
    "messages": [
      {"role":"system","content":"You are a helpful assistant."},
      {"role":"user","content":"Explain backprop in 2 sentences."}
    ]
  }'
```

### 3. 工具调用（Functions / Tools）

```bash
curl -X POST "https://llm.ai-nebula.com/v1/chat/completions" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-xxxx" \
  -d '{
    "model": "grok-4",
    "messages": [
      {"role":"user","content":"What is the weather in Shanghai?"}
    ],
    "tools": [
      {
        "type": "function",
        "function": {
          "name": "get_weather",
          "description": "Get weather by city",
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

#### 工具调用完整流程（两阶段）

1) 第一阶段：模型返回 `tool_calls`（content 通常为 null，finish_reason=tool_calls）。你需要根据 `tool_calls[*].function.name/arguments` 在你的服务端执行对应函数。

2) 第二阶段：把工具执行结果作为一条 `role:"tool"` 消息回传给模型，并继续补全（可流式）。

非流式续写示例（第二阶段）：

```bash
curl -X POST "https://llm.ai-nebula.com/v1/chat/completions" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-xxxx" \
  -d '{
    "model": "grok-4",
    "messages": [
      {"role":"user","content":"What is the weather in Shanghai?"},
      {"role":"assistant","tool_calls":[
        {"id":"call_8uPhimaucX5cODepJKS5EVRK","type":"function",
         "function":{"name":"get_weather","arguments":"{\"city\":\"Shanghai\"}"}}
      ]},
      {"role":"tool","tool_call_id":"call_8uPhimaucX5cODepJKS5EVRK",
       "content":"{\"temp\":\"22°C\",\"condition\":\"Cloudy\",\"aqi\":53}"}
    ]
  }'
```

流式续写示例（第二阶段也支持流式）：

```bash
curl -N -X POST "https://llm.ai-nebula.com/v1/chat/completions" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-xxxx" \
  -d '{
    "model": "grok-4",
    "stream": true,
    "messages": [
      {"role":"user","content":"What is the weather in Shanghai?"},
      {"role":"assistant","tool_calls":[
        {"id":"call_8uPhimaucX5cODepJKS5EVRK","type":"function",
         "function":{"name":"get_weather","arguments":"{\"city\":\"Shanghai\"}"}}
      ]},
      {"role":"tool","tool_call_id":"call_8uPhimaucX5cODepJKS5EVRK",
       "content":"{\"temp\":\"22°C\",\"condition\":\"Cloudy\",\"aqi\":53}"}
    ]
  }'
```

注意：
- `tool_call_id` 必须与第一阶段返回一致。
- 工具执行失败时应返回可读的错误信息或降级结果，避免阻塞后续补全。

### 4. 结构化输出（response_format/json_schema）

```bash
curl -X POST "https://llm.ai-nebula.com/v1/chat/completions" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-xxxx" \
  -d '{
    "model": "grok-4",
    "response_format": {
      "type": "json_schema",
      "json_schema": {
        "name": "Answer",
        "schema": {
          "type": "object",
          "properties": {"summary": {"type": "string"}},
          "required": ["summary"]
        }
      }
    },
    "messages": [
      {"role":"user","content":"Return JSON with a summary field."}
    ]
  }'
```

---

## 响应与用量

- **非流式**：一次性返回标准 OpenAI 结构，包含 `choices`、`usage`
- **流式**：SSE 分片返回，末尾可能附带 `usage` 聚合；若开启 `stream_options.include_usage=true`，分片可能包含实时用量
- **Reasoning Tokens**：对于支持推理的模型（如 `grok-4-fast-reasoning`），响应中的 `usage` 会区分 `completion_tokens` 和 `reasoning_tokens`
  - 非流式响应：`text_tokens = completion_tokens - reasoning_tokens`
  - 流式响应：usage 统计会在流式响应中实时更新或最终聚合

---

## 常见问题（FAQ）

1) 如何提升结构化输出的稳定性？
- 使用 `response_format: json_schema` 并提供严格的 JSON Schema；必要时配合降低 `temperature`、设置 `max_tokens`

2) 工具调用如何落库执行？
- 读取增量分片中的 `tool_calls`，服务端执行函数并把结果再作为 `tool` 消息回传给模型

3) 是否支持 Reproducible（种子）？
- 支持 `seed` 参数，建议仅在需要可复现的链路开启

4) 如何选择模型？
- `grok-3` 和 `grok-4` 是标准版本，适用于大多数场景
- `grok-3-fast` 是快速版本，适用于需要快速响应的场景
- `grok-4-fast-reasoning` 是推理版本，适用于需要深度思考和复杂推理的场景

---

## 最佳实践

- 流式前端使用事件流解析并及时渲染
- 严格的 JSON 模式下建议关闭/降低 `temperature`
- 工具调用做好超时与重试机制，避免阻塞模型响应
- 根据任务需求选择合适的模型：快速响应使用 `grok-3-fast`，复杂推理使用 `grok-4-fast-reasoning`

---

## 关于"深度思考/推理过程"

- Grok 模型（如 `grok-4`、`grok-3`）支持推理能力，但不会输出可视化的思维链文本
- `grok-4-fast-reasoning` 是快速推理版本，专门用于需要深度推理的场景
- 响应中的 `usage` 字段会包含 `reasoning_tokens` 统计（在 `completion_token_details` 中），用于了解模型的推理消耗
- `text_tokens = completion_tokens - reasoning_tokens`，方便区分实际输出文本和推理过程消耗的 token

