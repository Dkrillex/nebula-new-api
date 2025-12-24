## 概述

Realtime API 提供低时延的文本 / 语音实时对话能力，当前开放 `gpt-realtime` 与 `gpt-realtime-mini` 两个模型。通过 WebSocket 建立长连，按事件流交互（发送会话设置、对话消息、请求生成；接收增量文本/语音及使用量）。

## 基础信息

| 项目 | 内容 |
|------|------|
| **Base URL** | `wss://llm.ai-nebula.com` |
| **接口地址** | `/v1/realtime?model={model}` |
| **认证方式** | `Authorization: Bearer sk-xxxx` |
| **协议** | WebSocket（JSON 事件流） |
| **支持模型** | `gpt-realtime`、`gpt-realtime-mini` |
| **音频格式** | 输入输出均为 PCM16 单声道，采样率 24000Hz（如启用音频） |

## 事件总览

**客户端发送**
- `session.update`：设置/更新会话配置（文本/音频、系统指令、语音等）
- `conversation.item.create`：发送对话消息（`input_text` 或 `input_audio`）
- `input_audio_buffer.append` + `input_audio_buffer.commit`：流式推送音频
- `response.create`：请求生成一次回复

**服务端返回**
- `session.created` / `session.updated`：会话就绪或已更新
- `response.created`：已开始生成
- `response.text.delta` / `response.text.done`：文本增量与完成
- `response.audio_transcript.delta` / `response.audio_transcript.done`：语音转写增量与完成
- `response.audio.delta` / `response.audio.done`：音频增量与完成
- `response.done`：本轮结束，包含 `usage` 统计
- `error`：错误事件

## 调用流程（建议）

1) 建立连接：`wss://llm.ai-nebula.com/v1/realtime?model=gpt-realtime`（或 `gpt-realtime-mini`），携带 `Authorization` 头。  
2) 发送 `session.update` 配置会话（可多次更新）。  
3) 通过 `conversation.item.create` 发送用户消息（文本或音频）。  
4) 发送 `response.create` 触发生成，接收增量事件。  
5) 可在同一连接内继续多轮对话，重复步骤 3-4。  

### 会话配置示例（session.update）

```json
{
  "event_id": "evt_001",
  "type": "session.update",
  "session": {
    "modalities": ["text", "audio"],        // 支持 text / audio 二选一或同时
    "instructions": "你是一个友好的助手",
    "voice": "alloy",                       // 语音合成音色，可不填
    "temperature": 0.8,
    "input_audio_format": "pcm16",
    "output_audio_format": "pcm16",
    "input_audio_transcription": { "model": "whisper-1" }
  }
}
```

### 文本消息示例（conversation.item.create）

```json
{
  "event_id": "evt_002",
  "type": "conversation.item.create",
  "item": {
    "id": "item_01",
    "type": "message",
    "role": "user",
    "content": [
      { "type": "input_text", "text": "你好，请简单介绍一下你自己。" }
    ]
  }
}
```

### 触发生成（response.create）

```json
{ "event_id": "evt_003", "type": "response.create" }
```

### 服务端典型返回

```json
{ "type": "session.created", "session": { "id": "sess_xxx" } }
{ "type": "response.created", "response": { "id": "resp_xxx" } }
{ "type": "response.text.delta", "delta": "你好！我是" }
{ "type": "response.text.delta", "delta": " Nebula 的实时助手。" }
{ "type": "response.done",
  "response": {
    "usage": {
      "total_tokens": 123,
      "input_tokens": 45,
      "output_tokens": 78
    }
  }
}
```

## 音频输入与流式推送

### 一次性发送音频

`content` 中放入 `input_audio`，音频需先 base64 编码（PCM16，24k 单声道）。

```json
{
  "type": "conversation.item.create",
  "item": {
    "type": "message",
    "role": "user",
    "content": [
      { "type": "input_audio", "audio": "<base64-of-pcm16>" }
    ]
  }
}
```

### 流式发送音频

1) 多次发送 `input_audio_buffer.append`，字段 `audio` 为 base64 片段。  
2) 发送 `input_audio_buffer.commit`，由服务端生成对话项。  
3) 再调用 `response.create` 获取回复。  

```json
{ "type": "input_audio_buffer.append", "audio": "<chunk-1-base64>" }
{ "type": "input_audio_buffer.append", "audio": "<chunk-2-base64>" }
{ "type": "input_audio_buffer.commit", "item": { "type": "message", "role": "user" } }
{ "type": "response.create" }
```

## Python 快速示例

```python
import json, time, websocket

API_BASE = "wss://llm.ai-nebula.com"
API_KEY = "sk-xxxx"
MODEL = "gpt-realtime"

ws = websocket.WebSocketApp(
    f"{API_BASE}/v1/realtime?model={MODEL}",
    header={"Authorization": f"Bearer {API_KEY}"},
    on_message=lambda ws, msg: print("[recv]", msg)
)

ws.on_open = lambda ws: (
    ws.send(json.dumps({"type": "session.update", "session": {
        "modalities": ["text"],
        "instructions": "你是一个简洁的助手"
    }}, ensure_ascii=False)),
    ws.send(json.dumps({"type": "conversation.item.create", "item": {
        "type": "message", "role": "user",
        "content": [{"type": "input_text", "text": "用一句话介绍 Nebula。"}]
    }}, ensure_ascii=False)),
    ws.send(json.dumps({"type": "response.create"}))
)

ws.run_forever()
time.sleep(5)
ws.close()
```

## 常见错误

- 认证失败：确认 `Authorization` 头中 API Key 有效且前缀为 `sk-`。  
- 模型不存在：查询参数 `model` 仅支持 `gpt-realtime` / `gpt-realtime-mini`。  
- 音频解码错误：确保音频为 PCM16 单声道、采样率 24000Hz，并正确做 base64 编码。  
- 未收到增量事件：检查是否已发送 `response.create`，或连接是否仍然存活。  

---

## 外部系统代扣实时对话 `/api/sync/system/realtime`

供外部系统使用系统访问令牌发起实时对话，为指定 `user_id` 进行扣费和渠道选择，内部会转发到 `/v1/realtime`。

**地址与认证**
- URL：`wss://llm.ai-nebula.com/api/sync/system/realtime`
- 认证：`Authorization: <system_access_token>`（无需 `Bearer ` 前缀）
- 查询参数：
  - `user_id`（必填，int）：要扣费的实际用户 ID
  - `model`（必填，string）：`gpt-realtime` 或 `gpt-realtime-mini`
  - `group`（选填，string）：指定分组，缺省则使用该用户的默认分组

**行为与差异点**
- 授权方式不同：使用系统访问令牌；扣费主体为查询参数中的 `user_id`。
- 内部会为该用户创建临时 Token，按分组选择可用渠道后转发到 `/v1/realtime`，事件协议与返回流相同。
- 在上下文标记为 playground 场景，便于日志与路由策略区分。

**典型握手示例（Python websocket-client）**

```python
import json, websocket

WS_URL = "wss://llm.ai-nebula.com/api/sync/system/realtime"
ACCESS_TOKEN = "sys-access-token-xxx"  # 系统访问令牌
USER_ID = 123
MODEL = "gpt-realtime"

def on_open(ws):
    # 会话配置
    ws.send(json.dumps({
        "type": "session.update",
        "session": {"modalities": ["text"], "instructions": "你是一个简洁助手"}
    }, ensure_ascii=False))
    # 发送用户消息
    ws.send(json.dumps({
        "type": "conversation.item.create",
        "item": {
            "type": "message", "role": "user",
            "content": [{"type": "input_text", "text": "你好，帮我做个自我介绍"}]
        }
    }, ensure_ascii=False))
    # 请求生成
    ws.send(json.dumps({"type": "response.create"}, ensure_ascii=False))

ws = websocket.WebSocketApp(
    f"{WS_URL}?user_id={USER_ID}&model={MODEL}",
    header={"Authorization": ACCESS_TOKEN},
    on_open=on_open,
    on_message=lambda ws, msg: print("[recv]", msg)
)
ws.run_forever()
```

**使用提示**
- `user_id` 必须存在且未被禁用；`model` 需为已配置的 realtime 模型。
- 如需切分组路由，请传 `group`；否则使用用户默认分组。
- 后续事件发送与 `/v1/realtime` 完全一致（`session.update`、`conversation.item.create`、`response.create` 等）。  

