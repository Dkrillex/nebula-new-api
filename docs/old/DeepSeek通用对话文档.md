## 概述

本文档介绍如何通过 Nebula Api的 OpenAI 兼容接口调用 DeepSeek 对话模型。

## 基础信息

| 项目 | 内容 |
|------|------|
| Base URL | `https://llm.ai-nebula.com/v1/chat/completions` |
| 认证方式 | API Key (Token) |
| 请求头 | `Authorization: Bearer sk-xxxx`、`Content-Type: application/json` |

## 支持的模型（示例）

- `deepseek-v3-1-250821`
- 其他 DeepSeek 系列（以路由配置为准）

---

## API 接口

### 1. 最小示例（非流式）

```bash
curl -X POST "https://llm.ai-nebula.com/v1/chat/completions" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-xxxx" \
  -d '{
    "model": "deepseek-v3-1-250821",
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
    "model": "deepseek-v3-1-250821",
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

### 4. 工具调用（Functions / Tools）

```bash
curl -X POST "https://llm.ai-nebula.com/v1/chat/completions" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-xxxx" \
  -d '{
    "model": "deepseek-v3-1-250821",
    "messages": [
      {"role":"user","content":"广州的天气怎么样？"}
    ],
    "thinking": {"type": "enabled"},
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

#### 工具调用完整流程（两阶段）

1) 第一阶段：模型返回 `tool_calls`（content 通常为 null，finish_reason=tool_calls）。你需要根据 `tool_calls[*].function.name/arguments` 在你的服务端执行对应函数。

2) 第二阶段：把工具执行结果作为一条 `role:"tool"` 消息回传给模型，并继续补全（可流式）。

非流式续写示例（第二阶段）：

```bash
curl -X POST "https://llm.ai-nebula.com/v1/chat/completions" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-xxxx" \
  -d '{
    "model": "deepseek-v3-1-250821",
    "thinking": {"type": "enabled"},
    "messages": [
      {"role":"user","content":"广州的天气怎么样？"},
      {"role":"assistant","tool_calls":[
        {"id":"call_8uPhimaucX5cODepJKS5EVRK","type":"function",
         "function":{"name":"get_weather","arguments":"{\"city\":\"广州\"}"}}
      ]},
      {"role":"tool","tool_call_id":"call_8uPhimaucX5cODepJKS5EVRK",
       "content":"{\"temp\":\"22°C\",\"condition\":\"多云\",\"aqi\":53}"}
    ]
  }'
```

流式续写示例（第二阶段也支持流式）：

```bash
curl -N -X POST "https://llm.ai-nebula.com/v1/chat/completions" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-xxxx" \
  -d '{
    "model": "deepseek-v3-1-250821",
    "stream": true,
    "messages": [
      {"role":"user","content":"广州的天气怎么样？"},
      {"role":"assistant","tool_calls":[
        {"id":"call_8uPhimaucX5cODepJKS5EVRK","type":"function",
         "function":{"name":"get_weather","arguments":"{\"city\":\"广州\"}"}}
      ]},
      {"role":"tool","tool_call_id":"call_8uPhimaucX5cODepJKS5EVRK",
       "content":"{\"temp\":\"22°C\",\"condition\":\"多云\",\"aqi\":53}"}
    ]
  }'
```

注意：
- `tool_call_id` 必须与第一阶段返回一致。
- 工具执行失败时应返回可读的错误信息或降级结果，避免阻塞后续补全。
- DeepSeek 对工具调用的支持程度可能因模型版本而异，建议在使用前确认渠道支持情况。

### 5. 思考能力（Thinking）

DeepSeek支持以 `thinking` 字段开启/关闭思考能力。默认关闭：

```json
thinking={
  "type": "disabled"  // 默认行为：关闭思考能力
  // "type": "enabled" // 开启思考能力
}
```

在 OpenAI 兼容请求中，可直接在顶层传入 `thinking` 字段：

```bash
curl -X POST "https://llm.ai-nebula.com/v1/chat/completions" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-xxxx" \
  -d '{
    "model": "deepseek-v3-1-250821",
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
- 使用 OpenAI Chat Completions 格式；少数扩展字段可能不生效，以渠道支持为准.

2) 是否支持结构化输出？
- 支持 `response_format: json_schema`；复杂 Schema 时建议降低 `temperature` 提升一致性

3) 是否支持思维链/搜索类开关？
- 视渠道与模型版本而定；如需专属能力可联系管理员开通或在 `parameters` 透传（若通道支持）

4) 是否支持工具调用？
- 支持 OpenAI 兼容的工具调用格式，具体支持程度可能因模型版本和渠道而异

---

## 最佳实践

- 使用流式提升首字时间与交互体验
- 严格结构化输出时降低 `temperature`，并控制 `max_tokens`
- 对工具调用结果做好容错、重试与超时控制


