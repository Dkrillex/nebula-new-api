## 概述

本文档介绍如何通过 Nebula Api的 OpenAI 兼容接口调用标准 ChatGPT（OpenAI Chat Completions）能力，包含最小示例、流式推送、工具调用与结构化输出要点。

## 基础信息

| 项目 | 内容 |
|------|------|
| Base URL | `https://llm.ai-nebula.com/v1/chat/completions` |
| 认证方式 | API Key (Token) |
| 请求头 | `Authorization: Bearer sk-xxxx`、`Content-Type: application/json` |

## 支持的模型（示例）

- `gpt-4o`、`gpt-4.1`、`gpt-4o-mini`、`gpt-3.5-turbo` 等（以路由配置为准）

---

## API 接口

### 1. 最小示例（非流式）

```bash
curl -X POST "https://llm.ai-nebula.com/v1/chat/completions" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-xxxx" \
  -d '{
    "model": "gpt-4o",
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
    "model": "gpt-4o",
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
    "model": "gpt-4o",
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
    "model": "gpt-4o",
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
    "model": "gpt-4o",
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
    "model": "gpt-4o",
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

- 非流式：一次性返回标准 OpenAI 结构，包含 `choices`、`usage`
- 流式：SSE 分片返回，末尾可能附带 `usage` 聚合；若开启 `stream_options.include_usage=true` 的通道，分片可能包含实时用量

---

## 常见问题（FAQ）

1) 如何提升结构化输出的稳定性？
- 使用 `response_format: json_schema` 并提供严格的 JSON Schema；必要时配合降低 `temperature`、设置 `max_tokens`

2) 工具调用如何落库执行？
- 读取增量分片中的 `tool_calls`，服务端执行函数并把结果再作为 `tool` 消息回传给模型

3) 是否支持 Reproducible（种子）？
- 若通道支持可使用 `seed`；不同厂商实现可能差异，建议仅在需要可复现的链路开启

---

## 最佳实践

- 流式前端使用事件流解析并及时渲染
- 严格的 JSON 模式下建议关闭/降低 `temperature`
- 工具调用做好超时与重试机制，避免阻塞模型响应



---

## 关于“深度思考/推理过程”

- 常规 ChatGPT 系列（如 `gpt-4o`、`gpt-4.1`、`gpt-3.5-turbo`）不提供可视化的思维链输出；请求体中传 `enable_thinking` 不会生效。
- 如需带有“推理相关能力/用量统计”的模型（如 `o1`、`o3`、`o4-mini` 等），请使用 Responses API（/v1/responses）。
- 云策微软（Azure 风格）在 Responses API 下，`input` 应使用 `role/content` 的消息项（type 省略或等效于 message）；不要使用 `input_text`。

### Responses API 快速示例

```bash
curl -N -X POST "https://llm.ai-nebula.com/v1/responses" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-xxxx" \
  -d '{
    "model": "o4-mini",
    "stream": true,
    "reasoning": {"summary": "auto"},
    "max_output_tokens": 2048,
    "input": [
      {"role": "system", "content": "你是一名优秀的数学家,能解决世界上的所有数学难题。"},
      {"role": "user",   "content": "汉诺塔的公式是啥？"}
    ]
  }'
```


<!-- UNSUPPORTED: 以下内容因当前通道不支持，已隐藏。如需启用，请联系管理员。
### 推理模型（o1 / o3）正确用法（Responses API）

推理模型应通过 `/v1/responses` 接口调用，`enable_thinking` 不需要、也不生效。

最小示例（非流式）：

```bash
curl -X POST "https://llm.ai-nebula.com/v1/responses" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-xxxx" \
  -d '{
    "model": "o1",
    "input": [
      {"type":"input_text","text":"你是一名优秀的数学家,能解决世界上的所有数学难题。请回答：汉诺塔的公式是啥？"}
    ],
    "max_output_tokens": 8192
  }'
```

流式示例：

```bash
curl -N -X POST "https://llm.ai-nebula.com/v1/responses" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-xxxx" \
  -d '{
    "model": "o1",
    "stream": true,
    "input": [
      {"type":"input_text","text":"你是一名优秀的数学家,能解决世界上的所有数学难题。请回答：汉诺塔的公式是啥？"}
    ],
    "max_output_tokens": 8192
  }'
```

提示：
- Responses API 的字段与 Chat Completions 不同（如 `input` 是多段媒体/文本数组）。
- 推理模型可能返回 `reasoning`/用量明细，但不会输出完整的思维链文本。
-->## 概述

本文档介绍如何通过 Nebula 的 OpenAI 兼容接口调用标准 ChatGPT（OpenAI Chat Completions）能力，包含最小示例、流式推送、工具调用与结构化输出要点。

## 基础信息

| 项目 | 内容 |
|------|------|
| Base URL | `https://llm.ai-nebula.com/v1/chat/completions` |
| 认证方式 | API Key (Token) |
| 请求头 | `Authorization: Bearer sk-xxxx`、`Content-Type: application/json` |

## 支持的模型（示例）

- `gpt-4o`、`gpt-4.1`、`gpt-4o-mini`、`gpt-3.5-turbo` 等（以路由配置为准）

---

## API 接口

### 1. 最小示例（非流式）

```bash
curl -X POST "https://llm.ai-nebula.com/v1/chat/completions" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-xxxx" \
  -d '{
    "model": "gpt-4o",
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
    "model": "gpt-4o",
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
    "model": "gpt-4o",
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
    "model": "gpt-4o",
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
    "model": "gpt-4o",
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
    "model": "gpt-4o",
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

- 非流式：一次性返回标准 OpenAI 结构，包含 `choices`、`usage`
- 流式：SSE 分片返回，末尾可能附带 `usage` 聚合；若开启 `stream_options.include_usage=true` 的通道，分片可能包含实时用量

---

## 常见问题（FAQ）

1) 如何提升结构化输出的稳定性？
- 使用 `response_format: json_schema` 并提供严格的 JSON Schema；必要时配合降低 `temperature`、设置 `max_tokens`

2) 工具调用如何落库执行？
- 读取增量分片中的 `tool_calls`，服务端执行函数并把结果再作为 `tool` 消息回传给模型

3) 是否支持 Reproducible（种子）？
- 若通道支持可使用 `seed`；不同厂商实现可能差异，建议仅在需要可复现的链路开启

---

## 最佳实践

- 流式前端使用事件流解析并及时渲染
- 严格的 JSON 模式下建议关闭/降低 `temperature`
- 工具调用做好超时与重试机制，避免阻塞模型响应



---

## 关于“深度思考/推理过程”

- 常规 ChatGPT 系列（如 `gpt-4o`、`gpt-4.1`、`gpt-3.5-turbo`）不提供可视化的思维链输出；请求体中传 `enable_thinking` 不会生效。
- 如需带有“推理相关能力/用量统计”的模型（如 `o1`、`o3`、`o4-mini` 等），请使用 Responses API（/v1/responses）。



```bash
curl -N -X POST "https://llm.ai-nebula.com/v1/responses" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-xxxx" \
  -d '{
    "model": "o4-mini",
    "stream": true,
    "max_output_tokens": 2048,
    "input": [
      {"role": "system", "content": "你是一名优秀的数学家,能解决世界上的所有数学难题。"},
      {"role": "user",   "content": "汉诺塔的公式是啥？"}
    ]
  }'
```
