## 概述

本文档介绍如何通过 Nebula Api 的 OpenAI 兼容接口调用 Grok 模型（xAI）能力，包含最小示例、流式推送、工具调用、结构化输出、搜索模式与推理参数等要点。

## 基础信息

| 项目 | 内容 |
|------|------|
| Base URL | `https://llm.ai-nebula.com/v1/chat/completions` |
| 认证方式 | API Key (Token) |
| 请求头 | `Authorization: Bearer sk-xxxx`、`Content-Type: application/json` |

## 支持的模型

### 主要模型系列

- **grok-4**：最新版本，支持搜索模式（`grok-4-search`）
  - 实际模型：`grok-4`、`grok-4-0709`
  - 搜索模式：`grok-4-0709-search`
- **grok-3**：标准版本（实际映射为 `grok-3-beta`）
- **grok-3-mini**：轻量版本（实际映射为 `grok-3-mini-beta`）
- **grok-3-fast**：快速版本（实际映射为 `grok-3-fast-beta`）
- **grok-3-mini-fast**：轻量快速版本（实际映射为 `grok-3-mini-fast-beta`）
- **grok-4-fast-reasoning**：grok-4-fast 的思考推理版本（如上游支持，可通过路由配置使用）

### 模型名称映射说明

为简化使用，文档中使用简化模型名称，实际系统会自动映射到上游模型：

- `grok-3` → `grok-3-beta`
- `grok-3-fast` → `grok-3-fast-beta`
- `grok-3-mini` → `grok-3-mini-beta`
- `grok-3-mini-fast` → `grok-3-mini-fast-beta`

### Reasoning Effort 变体

对于 grok-3-mini 系列，支持通过模型后缀指定推理强度：

- `grok-3-mini-high`：高推理强度（实际映射为 `grok-3-mini-beta-high`）
- `grok-3-mini-medium`：中等推理强度（实际映射为 `grok-3-mini-beta-medium`）
- `grok-3-mini-low`：低推理强度（实际映射为 `grok-3-mini-beta-low`）
- `grok-3-mini-fast-high/medium/low`：快速版本的推理强度变体

### 搜索模式

部分模型支持搜索模式，在模型名称后添加 `-search` 后缀即可启用。系统会自动检测 `-search` 后缀并添加 `search_parameters` 参数：

- `grok-4-search`：启用搜索功能的 grok-4（实际映射为 `grok-4-0709-search`）
  - 系统会自动移除 `-search` 后缀，并将模型名设置为 `grok-4-0709`，同时在请求中添加 `search_parameters: {"mode": "on"}`

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

### 5. 搜索模式（Search Mode）

部分模型支持搜索模式，可以在模型名称后添加 `-search` 后缀启用。启用后，系统会自动添加 `search_parameters` 参数。

```bash
curl -X POST "https://llm.ai-nebula.com/v1/chat/completions" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-xxxx" \
  -d '{
    "model": "grok-4-search",
    "messages": [
      {"role":"user","content":"What are the latest developments in AI?"}
    ]
  }'
```

注意：
- 搜索模式会自动在请求中添加 `search_parameters: {"mode": "on"}`
- 系统会自动移除模型名中的 `-search` 后缀，并将实际模型名设置为对应的上游模型
- 仅支持搜索的模型可以使用此功能（如 `grok-4-search` 映射到 `grok-4-0709-search`）

### 6. Reasoning Effort 参数

对于 grok-3-mini 系列模型（包括 `grok-3-mini` 和 `grok-3-mini-fast`），支持通过 `reasoning_effort` 参数或模型后缀控制推理强度。

#### 方式一：使用模型后缀

```bash
curl -X POST "https://llm.ai-nebula.com/v1/chat/completions" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-xxxx" \
  -d '{
    "model": "grok-3-mini-high",
    "messages": [
      {"role":"user","content":"Solve this complex math problem: ..."}
    ],
    "max_completion_tokens": 4096
  }'
```

#### 方式二：使用 reasoning_effort 参数

```bash
curl -X POST "https://llm.ai-nebula.com/v1/chat/completions" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-xxxx" \
  -d '{
    "model": "grok-3-mini",
    "reasoning_effort": "high",
    "messages": [
      {"role":"user","content":"Solve this complex math problem: ..."}
    ],
    "max_completion_tokens": 4096
  }'
```

支持的 `reasoning_effort` 值：
- `high`：高推理强度，适用于复杂问题
- `medium`：中等推理强度，平衡性能和质量
- `low`：低推理强度，快速响应

注意：
- 对于 grok-3-mini 系列（包括 `grok-3-mini` 和 `grok-3-mini-fast`），如果设置了 `max_tokens`，系统会自动转换为 `max_completion_tokens`
- `reasoning_effort` 参数仅对以 `grok-3-mini` 开头的模型有效
- 使用模型后缀方式时，系统会自动移除后缀（如 `-high`、`-medium`、`-low`）并设置对应的 `reasoning_effort` 值

### 7. 图片生成

Grok 支持图片生成功能，使用 `/v1/images/generations` 接口。

```bash
curl -X POST "https://llm.ai-nebula.com/v1/images/generations" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-xxxx" \
  -d '{
    "model": "grok-2-image",
    "prompt": "A beautiful sunset over the ocean",
    "n": 1,
    "response_format": "url"
  }'
```

注意：
- 当前支持的图片生成模型：`grok-2-image`
- 支持的参数：`model`、`prompt`（必需）、`n`、`response_format`
- 暂不支持 `quality`、`size`、`style` 等参数

---

## 响应与用量

- **非流式**：一次性返回标准 OpenAI 结构，包含 `choices`、`usage`
- **流式**：SSE 分片返回，末尾可能附带 `usage` 聚合；若开启 `stream_options.include_usage=true` 的通道，分片可能包含实时用量
- **Reasoning Tokens**：对于支持推理的模型，响应中的 `usage` 会区分 `completion_tokens` 和 `reasoning_tokens`
  - 非流式响应：`text_tokens = completion_tokens - reasoning_tokens`（系统会自动计算）
  - 流式响应：usage 统计会在流式响应中实时更新或最终聚合

---

## 常见问题（FAQ）

1) 如何提升结构化输出的稳定性？
- 使用 `response_format: json_schema` 并提供严格的 JSON Schema；必要时配合降低 `temperature`、设置 `max_tokens`

2) 工具调用如何落库执行？
- 读取增量分片中的 `tool_calls`，服务端执行函数并把结果再作为 `tool` 消息回传给模型

3) 是否支持 Reproducible（种子）？
- 若通道支持可使用 `seed`；不同厂商实现可能差异，建议仅在需要可复现的链路开启

4) grok-3-mini 系列如何使用 max_tokens？
- 对于 grok-3-mini 系列，系统会自动将 `max_tokens` 转换为 `max_completion_tokens`，建议直接使用 `max_completion_tokens` 参数

5) 如何选择推理强度？
- 对于复杂问题或需要深度思考的场景，使用 `high`；对于快速响应场景，使用 `low`；一般场景使用 `medium` 或默认值

---

## 最佳实践

- 流式前端使用事件流解析并及时渲染
- 严格的 JSON 模式下建议关闭/降低 `temperature`
- 工具调用做好超时与重试机制，避免阻塞模型响应
- 对于 grok-3-mini 系列，根据任务复杂度选择合适的 `reasoning_effort` 值
- 搜索模式适用于需要实时信息的场景，但会增加响应时间

---

## 关于"深度思考/推理过程"

- Grok 模型（如 `grok-4`、`grok-3`）支持推理能力，但不会输出可视化的思维链文本
- `grok-4-fast-reasoning` 是 grok-4-fast 的思考推理版本（如上游支持，可通过路由配置使用），专门用于需要深度推理的场景
- 对于 grok-3-mini 系列（包括 `grok-3-mini` 和 `grok-3-mini-fast`），可以通过 `reasoning_effort` 参数或模型后缀控制推理强度
- 响应中的 `usage` 字段会包含 `reasoning_tokens` 统计（在 `completion_token_details` 中），用于了解模型的推理消耗
- 系统会自动计算 `text_tokens = completion_tokens - reasoning_tokens`，方便区分实际输出文本和推理过程消耗的 token

