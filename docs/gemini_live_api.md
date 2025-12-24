# Gemini Live API 集成指南

本文档介绍如何在 Nebula API 中使用 Gemini Live API 进行实时语音和视频交互。

## 目录

- [概述](#概述)
- [支持的模型](#支持的模型)
- [接口说明](#接口说明)
- [音色和语言配置](#音色和语言配置)
- [使用示例](#使用示例)
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

## 接口说明

**端点：** `wss://llm.ai-nebula.com/v1beta/models/{model}/liveStream`

**特点：**
- 使用 Gemini Live API 原生格式
- 直接透传，无协议转换
- 支持 Gemini 所有原生特性

**示例：**
```javascript
const ws = new WebSocket('wss://llm.ai-nebula.com/v1beta/models/gemini-live-2.5-flash-native-audio/liveStream', {
  headers: {
    'Authorization': 'Bearer sk-xxxx'
  }
});
```

## 音色和语言配置

### 音色配置

Gemini Live API 支持 30 种不同风格的预设音色，每种音色都有独特的表达特点：

| 音色名称 | 风格特点 | 音色名称 | 风格特点 | 音色名称 | 风格特点 |
|---------|---------|---------|---------|---------|---------|
| Zephyr | 明快 | Puck | 欢快 | Charon | 信息丰富 |
| Kore | 坚定 | Fenrir | 兴奋 | Leda | 青春活力 |
| Orus | 坚定 | Aoede | 轻快 | Callirrhoe | 轻松愉快 |
| Autonoe | 明快 | Enceladus | 气声 | Iapetus | 清晰明了 |
| Umbriel | 轻松 | Algieba | 流畅 | Despina | 流畅自然 |
| Erinome | 清晰 | Algenib | 沙哑 | Rasalgethi | 信息丰富 |
| Laomedeia | 欢快 | Achernar | 柔和 | Alnilam | 坚定有力 |
| Schedar | 平稳 | Gacrux | 成熟 | Pulcherrima | 积极向上 |
| Achird | 友好 | Zubenelgenubi | 随意 | Vindemiatrix | 温柔舒缓 |
| Sadachbia | 活泼 | Sadaltager | 博学 | Sulafat | 温暖舒适 |

**默认音色**：Zephyr（明快）

### 语言配置

支持 24 种语言，通过 BCP-47 语言代码指定：

| 语言 | 代码 | 语言 | 代码 |
|------|------|------|------|
| 阿拉伯语（埃及） | ar-EG | 德语（德国） | de-DE |
| 英语（美国） | en-US | 西班牙语（美国） | es-US |
| 法语（法国） | fr-FR | 印地语（印度） | hi-IN |
| 印度尼西亚语 | id-ID | 意大利语（意大利） | it-IT |
| 日语（日本） | ja-JP | 韩语（韩国） | ko-KR |
| 葡萄牙语（巴西） | pt-BR | 俄语（俄罗斯） | ru-RU |
| 荷兰语（荷兰） | nl-NL | 波兰语（波兰） | pl-PL |
| 泰语（泰国） | th-TH | 土耳其语（土耳其） | tr-TR |
| 越南语（越南） | vi-VN | 罗马尼亚语 | ro-RO |
| 乌克兰语 | uk-UA | 孟加拉语 | bn-BD |
| 英语（印度） | en-IN | 马拉地语（印度） | mr-IN |
| 泰米尔语（印度） | ta-IN | 泰卢固语（印度） | te-IN |
| 中文（简体） | zh-CN | | |

**默认语言**：根据系统指令中的语言自动推断

## 使用示例

### JavaScript 示例

```javascript
const ws = new WebSocket('wss://llm.ai-nebula.com/v1beta/models/gemini-live-2.5-flash-native-audio/liveStream', {
  headers: {
    'Authorization': 'Bearer sk-xxxx'
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
        responseModalities: ["AUDIO"]
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
    // 处理输出转录（音频转文本）
    if (message.serverContent.outputTranscription) {
      const text = message.serverContent.outputTranscription.text;
      if (text) {
        console.log('[转录]', text);
      }
    }
    
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

### Python 示例

```python
import websocket
import json
import base64
import threading

def on_message(ws, message):
    data = json.loads(message)
    print(f"Received: {data}")
    
    # 处理输出转录
    if "serverContent" in data:
        server_content = data["serverContent"]
        
        if "outputTranscription" in server_content:
            transcription = server_content["outputTranscription"]
            text = transcription.get("text", "")
            if text:
                print(f"[转录] {text}")
        
        if "modelTurn" in server_content:
            model_turn = server_content["modelTurn"]
            if "parts" in model_turn:
                for part in model_turn["parts"]:
                    if "text" in part:
                        print(f"Text: {part['text']}")
                    elif "inlineData" in part:
                        inline_data = part["inlineData"]
                        if inline_data.get("mimeType") == "audio/pcm":
                            audio_b64 = inline_data.get("data", "")
                            if audio_b64:
                                audio_data = base64.b64decode(audio_b64)
                                # 处理音频数据

def on_error(ws, error):
    print(f"Error: {error}")

def on_close(ws, close_status_code, close_msg):
    print("Connection closed")

def on_open(ws):
    print("WebSocket connected")
    
    # 发送 setup 消息
    setup_message = {
        "setup": {
            "model": "gemini-live-2.5-flash-native-audio",
            "generationConfig": {
                "temperature": 0.7,
                "responseModalities": ["AUDIO"]
            },
            "systemInstruction": {
                "parts": [
                    {"text": "You are a helpful assistant."}
                ]
            },
            "speechConfig": {
                "voiceConfig": {
                    "prebuiltVoiceConfig": {
                        "voiceName": "Puck"
                    }
                }
            }
        }
    }
    ws.send(json.dumps(setup_message))

# 连接 WebSocket
ws_url = "wss://llm.ai-nebula.com/v1beta/models/gemini-live-2.5-flash-native-audio/liveStream"
ws = websocket.WebSocketApp(
    ws_url,
    header={"Authorization": "Bearer sk-xxxx"},
    on_open=on_open,
    on_message=on_message,
    on_error=on_error,
    on_close=on_close
)

ws.run_forever()
```

### 完整配置示例

#### 示例 1：仅音频模式

```json
{
  "setup": {
    "model": "gemini-live-2.5-flash-native-audio",
    "generationConfig": {
      "temperature": 0.7,
      "responseModalities": ["AUDIO"],
      "speechConfig": {
        "voiceConfig": {
          "prebuiltVoiceConfig": {
            "voiceName": "Zephyr"
          }
        },
        "languageCode": "zh-CN"
      }
    },
    "systemInstruction": {
      "parts": [
        {"text": "你是一个友好的助手，请用自然、对话式的方式回答问题。"}
      ]
    }
  }
}
```

#### 示例 2：音频 + 文本转录模式（推荐）

```json
{
  "setup": {
    "model": "gemini-live-2.5-flash-native-audio",
    "generationConfig": {
      "temperature": 0.7,
      "responseModalities": ["AUDIO", "TEXT"],
      "speechConfig": {
        "voiceConfig": {
          "prebuiltVoiceConfig": {
            "voiceName": "Zephyr"
          }
        },
        "languageCode": "zh-CN"
      }
    },
    "systemInstruction": {
      "parts": [
        {"text": "你是一个友好的助手，请用自然、对话式的方式回答问题。"}
      ]
    },
    "tools": {
      "googleSearch": {}
    },
    "proactivity": {
      "proactiveAudio": false,
      "empatheticMode": true
    },
    "outputAudioTranscription": {},
    "realtimeInputConfig": {
      "automaticActivityDetection": {
        "disabled": false,
        "startOfSpeechSensitivity": "START_SENSITIVITY_LOW",
        "endOfSpeechSensitivity": "END_SENSITIVITY_HIGH",
        "prefixPaddingMs": 0,
        "silenceDurationMs": 0
      }
    }
  }
}
```

**配置说明：**
- `responseModalities`: 响应模态，只能选择以下两种之一：
  - `["AUDIO"]` - 仅音频输出
  - `["AUDIO", "TEXT"]` - 音频 + 文本转录（推荐，可同时获得音频和文本）
- `voiceName`: 音色名称，支持 30 种预设音色（见上方音色配置表）
- `languageCode`: 语言代码，支持 24 种语言（见上方语言配置表）
- `googleSearch`: 启用 Google 搜索功能
- `proactiveAudio`: 主动音频，模型可以选择不回应无关音频
- `empatheticMode`: 共情对话，根据情绪调整回答风格
- `outputAudioTranscription`: 启用输出音频转文本（需要在 `responseModalities` 中包含 `"TEXT"` 才能看到转录文本）
- `automaticActivityDetection`: 语音活动检测配置

## 功能特性

### 1. 支持的 Gemini Live 消息类型

**客户端消息：**
- `setup` - 会话配置
- `clientContent` - 客户端内容（文本/音频）
- `realtimeInput` - 实时音频输入
- `toolResponse` - 工具响应

**服务器消息：**
- `setupComplete` - 设置完成确认
- `serverContent` - 服务器内容（文本/音频/转录）
- `toolCall` - 工具调用
- `toolCallCancellation` - 工具调用取消
- `usageMetadata` - 使用量统计

### 2. Token 统计

系统会分别统计：
- 文本 Token（输入/输出）
- 音频 Token（输入/输出）
- 总 Token 数

使用量信息会在 `usageMetadata` 消息中返回：

```json
{
  "usageMetadata": {
    "totalTokenCount": 100,
    "inputTokenCount": 50,
    "outputTokenCount": 50,
    "inputTokenDetails": {
      "textTokens": 30,
      "audioTokens": 20
    },
    "outputTokenDetails": {
      "textTokens": 25,
      "audioTokens": 25
    }
  }
}
```

### 3. 配额管理

- 支持预消费和后消费机制
- 实时统计使用量
- 自动扣费

### 4. 定价说明

**重要提示：** 模型价格可能会变动，具体定价请以模型广场显示的最新价格为准。

Gemini Live API 按 token 计费，分别统计文本和音频 tokens：
- **文本 Token**：用于输入的文本内容和输出的文本转录
- **音频 Token**：用于输入的音频和输出的音频内容

系统会在 `usageMetadata` 消息中返回详细的使用量统计，包括文本和音频的输入/输出 token 数量。

## 技术规范

### WebSocket 端点

**统一入口：**
```
wss://llm.ai-nebula.com/v1beta/models/{model}/liveStream
```

**上游端点（系统内部）：**

**Google AI Studio：**
```
wss://generativelanguage.googleapis.com/ws/google.ai.generativelanguage.v1beta.GenerativeService.BidiGenerateContent?key={API_KEY}
```

**Vertex AI：**
```
wss://{region}-aiplatform.googleapis.com/ws/google.cloud.aiplatform.v1beta1.LlmBidiService/BidiGenerateContent
```

### 认证方式

客户端需要提供认证头：
```
Authorization: Bearer sk-xxxx
```

**上游认证：**

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

### Q1: 如何选择音色？

**A:** 在 setup 消息的 `speechConfig.voiceConfig.prebuiltVoiceConfig.voiceName` 中指定音色名称。支持 30 种预设音色，完整列表请查看上方的[音色配置](#音色配置)章节。默认音色为 Zephyr。

### Q2: 如何启用音频转文本？

**A:** 需要同时满足两个条件：
1. 在 `generationConfig.responseModalities` 中包含 `"TEXT"`（例如：`["AUDIO", "TEXT"]`）
2. 在 setup 消息中添加 `outputAudioTranscription: {}` 字段

启用后，服务器会在 `serverContent.outputTranscription` 中返回音频的文本转录。

### Q3: 如何启用 Google 搜索功能？

**A:** 在 setup 消息中添加 `tools: { googleSearch: {} }` 字段。启用后，模型可以在回答问题时搜索最新的网络信息。

### Q4: 如何启用工具调用？

**A:** 在 setup 消息中添加工具定义：

```json
{
  "setup": {
    "tools": {
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
  }
}
```

### Q5: 如何中断模型响应？

**A:** 发送新的 `realtimeInput` 或 `clientContent` 消息会中断当前响应。

### Q6: 支持视频输入吗？

**A:** 是的，Gemini Live API 支持视频输入。在 `clientContent` 中可以包含视频数据（JPEG 格式，1 FPS）。

### Q7: 如何获取使用量统计？

**A:** 系统会在响应过程中或响应完成时发送 `usageMetadata` 消息，包含详细的使用量统计信息。

### Q8: 语音识别灵敏度如何配置？

**A:** 在 setup 消息的 `realtimeInputConfig.automaticActivityDetection` 中配置：

```json
{
  "realtimeInputConfig": {
    "automaticActivityDetection": {
      "startOfSpeechSensitivity": "START_SENSITIVITY_LOW",  // 或 "START_SENSITIVITY_HIGH"
      "endOfSpeechSensitivity": "END_SENSITIVITY_HIGH",      // 或 "END_SENSITIVITY_LOW"
      "prefixPaddingMs": 0,                                   // 0-1000ms
      "silenceDurationMs": 0                                  // 0-2000ms
    }
  }
}
```

## 参考文档

- [Gemini Live API 官方文档](https://docs.cloud.google.com/vertex-ai/generative-ai/docs/live-api?hl=zh-cn)
- [WebSocket 协议文档](https://docs.cloud.google.com/vertex-ai/generative-ai/docs/live-api/get-started-websocket?hl=zh-cn)
- [发送音频视频流](https://docs.cloud.google.com/vertex-ai/generative-ai/docs/live-api/send-audio-video-streams?hl=zh-cn)
- [配置语言和语音](https://docs.cloud.google.com/vertex-ai/generative-ai/docs/live-api/configure-language-voice?hl=zh-cn)

## 更新日志

### 2025-12-22
- ✅ 简化文档，统一使用 Gemini 原生模式
- ✅ 统一使用 `wss://llm.ai-nebula.com` 作为服务地址
- ✅ 更新所有示例代码使用统一地址
- ✅ 添加完整的配置示例和说明
