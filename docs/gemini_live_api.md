# Gemini Live API 集成指南

本文档介绍如何在 Nebula API 中使用 Gemini Live API 进行实时语音和视频交互。

## 目录

- [概述](#概述)
- [支持的模型](#支持的模型)
- [接口说明](#接口说明)
  - [统一接口（OpenAI 兼容）](#统一接口openai-兼容)
  - [原生接口（Gemini 原生）](#原生接口gemini-原生)
- [协议模式](#协议模式)
  - [OpenAI 兼容模式](#openai-兼容模式)
  - [Gemini 原生模式](#gemini-原生模式)
- [渠道配置](#渠道配置)
- [使用示例](#使用示例)
  - [OpenAI 兼容模式示例](#openai-兼容模式示例)
  - [Gemini 原生模式示例](#gemini-原生模式示例)
- [功能特性](#功能特性)
- [技术规范](#技术规范)
- [常见问题](#常见问题)

## 概述

Gemini Live API 支持与 Gemini 进行低延迟、实时的语音和视频交互。它能够处理连续的音频、视频或文本流，以提供即时、自然逼真的语音回答。

**主要特性：**
- ✅ 高音质：提供多种语言的自然、逼真的语音
- ✅ 多语言支持：支持用 24 种语言进行对话
- ✅ 打断功能：用户可以随时中断模型，以便进行响应式互动
- ✅ 共情对话：根据用户输入内容的情绪表达调整回答风格和语气
- ✅ 工具使用：集成函数调用和 Google 搜索等工具
- ✅ 音频转写：提供用户输入和模型输出的文本转写内容
- ✅ 主动音频：可控制模型何时响应以及在哪些情境下响应

## 支持的模型

以下模型支持 Gemini Live API：

| 模型 ID | 可用性 | 使用场景 | 主要特性 |
|---------|--------|----------|----------|
| `gemini-live-2.5-flash-native-audio` | 已全面推出 | **推荐**。低延迟语音代理。支持无缝多语言切换和情感基调。 | 原生音频、音频转写、语音活动检测、共情对话、主动音频、工具使用 |
| `gemini-live-2.5-flash-preview-native-audio-09-2025` | 公开预览版 | 实时语音代理的成本效益。 | 原生音频、音频转写、语音活动检测、共情对话、主动音频、工具使用 |
| `gemini-2.5-flash-native-audio-preview-12-2025` | 公开预览版 | 实时语音代理的成本效益。 | 原生音频、音频转写、语音活动检测、共情对话、主动音频、工具使用 |

## 接口说明

Nebula API 提供了两种接口方式来使用 Gemini Live API：

### 统一接口（OpenAI 兼容）

**端点：** `wss://your-api-domain.com/v1/realtime`

**特点：**
- 使用 OpenAI Realtime API 格式
- 自动协议转换，兼容现有 OpenAI 客户端
- 根据模型名称自动路由到 Gemini Live API

**示例：**
```javascript
const ws = new WebSocket('wss://your-api-domain.com/v1/realtime?model=gemini-live-2.5-flash-native-audio');
```

### 原生接口（Gemini 原生）

**端点：** `wss://your-api-domain.com/v1beta/models/{model}/liveStream`

**特点：**
- 使用 Gemini Live API 原生格式
- 直接透传，无协议转换
- 支持 Gemini 所有原生特性

**示例：**
```javascript
const ws = new WebSocket('wss://your-api-domain.com/v1beta/models/gemini-live-2.5-flash-native-audio/liveStream');
```

## 协议模式

系统会自动检测客户端使用的协议格式，并自动进行适配。

### OpenAI 兼容模式

当客户端发送的消息包含 `type: "session.update"` 等 OpenAI Realtime API 格式时，系统会自动：

1. **协议转换**：将 OpenAI 格式转换为 Gemini Live 格式
2. **音频格式转换**：自动处理采样率转换（24kHz ↔ 16kHz）
3. **语音映射**：自动映射 OpenAI 语音到 Gemini 语音
4. **工具转换**：自动转换工具定义格式

**支持的 OpenAI Realtime 事件：**
- `session.update` - 会话配置
- `input_audio_buffer.append` - 音频输入
- `conversation.item.create` - 文本输入
- `response.audio.delta` - 音频输出
- `response.done` - 响应完成

### Gemini 原生模式

当客户端发送的消息包含 `setup` 等 Gemini Live API 格式时，系统会：

1. **透明代理**：直接转发消息，不进行转换
2. **原生支持**：支持所有 Gemini Live API 特性

**支持的 Gemini Live 消息类型：**
- `setup` - 会话配置
- `clientContent` - 客户端内容（文本/音频）
- `realtimeInput` - 实时音频输入
- `toolResponse` - 工具响应
- `serverContent` - 服务器内容（文本/音频）
- `toolCall` - 工具调用

## 渠道配置

### Google AI Studio 配置

```json
{
  "type": 24,
  "name": "Gemini Live (Google AI Studio)",
  "base_url": "https://generativelanguage.googleapis.com",
  "key": "your-google-ai-studio-api-key",
  "models": [
    "gemini-live-2.5-flash-native-audio",
    "gemini-2.5-flash-native-audio-preview-12-2025"
  ]
}
```

### Vertex AI 配置

```json
{
  "type": 24,
  "name": "Gemini Live (Vertex AI)",
  "base_url": "https://us-central1-aiplatform.googleapis.com",
  "key": "your-vertex-ai-credentials",
  "models": [
    "gemini-live-2.5-flash-native-audio",
    "gemini-live-2.5-flash-preview-native-audio-09-2025"
  ]
}
```

**注意：**
- Google AI Studio 使用 API Key 认证（`?key=xxx` 查询参数）
- Vertex AI 使用 OAuth2 或应用默认凭证（ADC）

## 使用示例

### OpenAI 兼容模式示例

#### JavaScript 示例

```javascript
const ws = new WebSocket('wss://your-api-domain.com/v1/realtime?model=gemini-live-2.5-flash-native-audio', {
  headers: {
    'Authorization': 'Bearer your-api-token'
  }
});

ws.onopen = () => {
  console.log('WebSocket connected');
  
  // 发送会话配置
  ws.send(JSON.stringify({
    type: "session.update",
    session: {
      modalities: ["text", "audio"],
      instructions: "You are a helpful assistant. Speak naturally and conversationally.",
      voice: "alloy",
      input_audio_format: "pcm16",
      output_audio_format: "pcm16",
      input_audio_transcription: {
        model: "whisper-1"
      }
    }
  }));
};

ws.onmessage = (event) => {
  const message = JSON.parse(event.data);
  console.log('Received:', message);
  
  if (message.type === "response.audio.delta") {
    // 处理音频数据
    const audioData = message.audio;
    // audioData 是 base64 编码的 PCM 音频
  } else if (message.type === "response.text.delta") {
    // 处理文本增量
    console.log('Text:', message.delta);
  } else if (message.type === "response.done") {
    // 响应完成
    console.log('Usage:', message.response.usage);
  }
};

// 发送音频数据
function sendAudio(audioBuffer) {
  const base64Audio = btoa(
    String.fromCharCode(...new Uint8Array(audioBuffer))
  );
  
  ws.send(JSON.stringify({
    type: "input_audio_buffer.append",
    audio: base64Audio
  }));
}

// 发送文本消息
function sendText(text) {
  ws.send(JSON.stringify({
    type: "conversation.item.create",
    item: {
      type: "message",
      role: "user",
      content: [
        {
          type: "input_text",
          text: text
        }
      ]
    }
  }));
}
```

#### Python 示例

```python
import websocket
import json
import base64
import threading

def on_message(ws, message):
    data = json.loads(message)
    print(f"Received: {data}")
    
    if data.get("type") == "response.audio.delta":
        # 处理音频数据
        audio_data = base64.b64decode(data["audio"])
        # 播放或处理音频
    elif data.get("type") == "response.text.delta":
        print(f"Text: {data.get('delta')}")
    elif data.get("type") == "response.done":
        print(f"Usage: {data.get('response', {}).get('usage')}")

def on_error(ws, error):
    print(f"Error: {error}")

def on_close(ws, close_status_code, close_msg):
    print("Connection closed")

def on_open(ws):
    print("WebSocket connected")
    
    # 发送会话配置
    setup_message = {
        "type": "session.update",
        "session": {
            "modalities": ["text", "audio"],
            "instructions": "You are a helpful assistant.",
            "voice": "alloy",
            "input_audio_format": "pcm16",
            "output_audio_format": "pcm16"
        }
    }
    ws.send(json.dumps(setup_message))

# 连接 WebSocket
ws_url = "wss://your-api-domain.com/v1/realtime?model=gemini-live-2.5-flash-native-audio"
ws = websocket.WebSocketApp(
    ws_url,
    header={"Authorization": "Bearer your-api-token"},
    on_open=on_open,
    on_message=on_message,
    on_error=on_error,
    on_close=on_close
)

ws.run_forever()
```

### Gemini 原生模式示例

#### JavaScript 示例

```javascript
const ws = new WebSocket('wss://your-api-domain.com/v1beta/models/gemini-live-2.5-flash-native-audio/liveStream', {
  headers: {
    'Authorization': 'Bearer your-api-token'
  }
});

ws.onopen = () => {
  console.log('WebSocket connected');
  
  // 发送 setup 消息
  ws.send(JSON.stringify({
    setup: {
      model: "gemini-live-2.5-flash-native-audio",
      generationConfig: {
        temperature: 0.7,
        responseModalities: ["AUDIO", "TEXT"]
      },
      systemInstruction: {
        parts: [
          { text: "You are a helpful assistant. Speak naturally and conversationally." }
        ]
      },
      speechConfig: {
        voiceConfig: {
          prebuiltVoiceConfig: {
            voiceName: "Puck"
          }
        }
      }
    }
  }));
};

ws.onmessage = (event) => {
  const message = JSON.parse(event.data);
  console.log('Received:', message);
  
  if (message.serverContent) {
    if (message.serverContent.modelTurn) {
      // 处理模型输出
      message.serverContent.modelTurn.parts.forEach(part => {
        if (part.text) {
          console.log('Text:', part.text);
        }
        if (part.inlineData && part.inlineData.mimeType === "audio/pcm") {
          // 处理音频数据
          const audioData = part.inlineData.data;
          // audioData 是 base64 编码的 PCM 音频
        }
      });
    }
    if (message.serverContent.turnComplete) {
      console.log('Turn complete');
    }
  }
  
  if (message.setupComplete) {
    console.log('Setup complete');
  }
};

// 发送实时音频输入
function sendRealtimeAudio(audioBuffer) {
  const base64Audio = btoa(
    String.fromCharCode(...new Uint8Array(audioBuffer))
  );
  
  ws.send(JSON.stringify({
    realtimeInput: {
      mediaChunks: [
        {
          mimeType: "audio/pcm;rate=16000",
          data: base64Audio
        }
      ]
    }
  }));
}

// 发送文本消息
function sendText(text) {
  ws.send(JSON.stringify({
    clientContent: {
      turns: [
        {
          role: "user",
          parts: [
            { text: text }
          ]
        }
      ],
      turnComplete: true
    }
  }));
}
```

## 功能特性

### 1. 自动协议检测

系统会根据客户端发送的首条消息自动判断协议格式：
- 包含 `type: "session.update"` → OpenAI 格式
- 包含 `setup` 字段 → Gemini 原生格式

### 2. 音频格式转换

系统自动处理音频格式差异：

| 属性 | OpenAI Realtime | Gemini Live |
|-----|----------------|-------------|
| 输入采样率 | 24kHz | 16kHz |
| 输出采样率 | 24kHz | 24kHz |
| 编码 | PCM16 | PCM16 |
| 字节序 | 小端 | 小端 |

### 3. 语音映射

OpenAI 语音到 Gemini 语音的自动映射：

| OpenAI 语音 | Gemini 语音 |
|------------|------------|
| alloy | Puck |
| echo | Charon |
| fable | Kore |
| onyx | Fenrir |
| nova | Aoede |
| shimmer | Puck |

### 4. Token 统计

系统会分别统计：
- 文本 Token（输入/输出）
- 音频 Token（输入/输出）
- 总 Token 数

### 5. 配额管理

- 支持预消费和后消费机制
- 实时统计使用量
- 自动扣费

## 技术规范

### WebSocket 端点

**Google AI Studio：**
```
wss://generativelanguage.googleapis.com/ws/google.ai.generativelanguage.v1beta.GenerativeService.BidiGenerateContent?key={API_KEY}
```

**Vertex AI：**
```
wss://{region}-aiplatform.googleapis.com/ws/google.cloud.aiplatform.v1beta1.LlmBidiService/BidiGenerateContent
```

### 认证方式

**Google AI Studio：**
- 使用 API Key：`?key={API_KEY}` 查询参数

**Vertex AI：**
- OAuth2 Token：`Authorization: Bearer {token}` header
- 应用默认凭证（ADC）

### 音频格式

**输入音频：**
- 格式：16-bit PCM
- 采样率：16kHz
- 字节序：小端
- 编码：Base64

**输出音频：**
- 格式：16-bit PCM
- 采样率：24kHz
- 字节序：小端
- 编码：Base64

## 常见问题

### Q1: 如何选择使用哪个接口？

**A:** 
- 如果你的客户端已经使用 OpenAI Realtime API，使用统一接口 `/v1/realtime`
- 如果你想使用 Gemini 的所有原生特性，使用原生接口 `/v1beta/models/{model}/liveStream`

### Q2: 音频格式不匹配怎么办？

**A:** 系统会自动处理音频格式转换。在 OpenAI 兼容模式下，系统会自动将 24kHz 转换为 16kHz（输入）或反之（输出）。

### Q3: 支持哪些语音？

**A:** 
- OpenAI 兼容模式：支持 OpenAI 的 6 种语音（alloy, echo, fable, onyx, nova, shimmer），会自动映射到 Gemini 语音
- Gemini 原生模式：支持 Gemini 的所有预设语音（Puck, Charon, Kore, Fenrir, Aoede 等）

### Q4: 如何启用工具调用？

**A:** 在会话配置中添加工具定义：

**OpenAI 格式：**
```json
{
  "type": "session.update",
  "session": {
    "tools": [
      {
        "type": "function",
        "name": "get_weather",
        "description": "Get the weather",
        "parameters": {
          "type": "object",
          "properties": {
            "location": {
              "type": "string"
            }
          }
        }
      }
    ]
  }
}
```

**Gemini 格式：**
```json
{
  "setup": {
    "tools": [
      {
        "functionDeclarations": [
          {
            "name": "get_weather",
            "description": "Get the weather",
            "parameters": {
              "type": "object",
              "properties": {
                "location": {
                  "type": "string"
                }
              }
            }
          }
        ]
      }
    ]
  }
}
```

### Q5: 如何获取使用量统计？

**A:** 在响应完成时会收到使用量信息：

**OpenAI 格式：**
```json
{
  "type": "response.done",
  "response": {
    "usage": {
      "total_tokens": 100,
      "input_tokens": 50,
      "output_tokens": 50,
      "input_token_details": {
        "text_tokens": 30,
        "audio_tokens": 20
      },
      "output_token_details": {
        "text_tokens": 25,
        "audio_tokens": 25
      }
    }
  }
}
```

### Q6: 支持视频输入吗？

**A:** 是的，Gemini Live API 支持视频输入。在 `clientContent` 或 `conversation.item.create` 中可以包含视频数据（JPEG 格式，1 FPS）。

### Q7: 如何中断模型响应？

**A:** 在 OpenAI 兼容模式下，发送 `input_audio_buffer.append` 会自动中断当前响应。在 Gemini 原生模式下，发送新的 `realtimeInput` 或 `clientContent` 会中断当前响应。

## 参考文档

- [Gemini Live API 官方文档](https://docs.cloud.google.com/vertex-ai/generative-ai/docs/live-api?hl=zh-cn)
- [WebSocket 协议文档](https://docs.cloud.google.com/vertex-ai/generative-ai/docs/live-api/get-started-websocket?hl=zh-cn)
- [发送音频视频流](https://docs.cloud.google.com/vertex-ai/generative-ai/docs/live-api/send-audio-video-streams?hl=zh-cn)
- [配置语言和语音](https://docs.cloud.google.com/vertex-ai/generative-ai/docs/live-api/configure-language-voice?hl=zh-cn)

## 更新日志

### 2025-01-XX
- ✅ 初始版本发布
- ✅ 支持 OpenAI 兼容模式和 Gemini 原生模式
- ✅ 支持自动协议检测和转换
- ✅ 支持音频格式自动转换
- ✅ 支持工具调用
- ✅ 支持 Token 统计和配额管理

