## 概述

Gemini Live API 是 Google 提供的实时多模态对话接口，支持低延迟的语音和文本交互。本平台完全集成了 Gemini Live API，提供两种接入方式：

- **OpenAI 兼容模式**：通过 `/v1/realtime` 接口，使用 OpenAI Realtime API 的协议格式
- **Gemini 原生模式**：通过 `/v1beta/models/{model}/liveStream` 接口，使用 Gemini 原生协议格式

相比 OpenAI Realtime API，Gemini Live API 提供了更多高级功能：
- 30 种不同风格的音色可选
- 24 种语言支持
- Google 搜索能力（接地功能）
- 主动音频和共情对话模式
- 更灵活的语音识别配置
- 更大的上下文窗口（最大 128K tokens）

## 基础信息

| 项目 | 内容 |
|------|------|
| **Base URL** | `wss://llm.ai-nebula.com` |
| **OpenAI 兼容接口** | `/v1/realtime?model={model}&...参数` |
| **Gemini 原生接口** | `/v1beta/models/{model}/liveStream` |
| **认证方式** | `Authorization: Bearer sk-xxxx` |
| **协议** | WebSocket（JSON 事件流） |
| **支持模型** | `gemini-live-2.5-flash-native-audio`<br/>`gemini-2.5-flash-native-audio-preview-12-2025` |
| **音频格式** | 输入：PCM16 单声道 16kHz<br/>输出：PCM16 单声道 24kHz |

## 支持的功能

### 1. 音色配置

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

### 2. 语言配置

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

**默认语言**：根据系统指令中的语言自动推断

### 3. 输出模式

- `audio_only`（默认）：仅返回音频
- `audio_with_text`：返回音频 + 文本转录（同时收到 audio 和 text 事件）

### 4. Google 搜索（接地功能）

启用后，模型可以在回答问题时搜索最新的网络信息，提供更准确和时效性的答案。

**使用场景**：
- 询问最新新闻、事件
- 查询实时数据（股票、天气等）
- 获取当前事实信息

**默认状态**：关闭

### 5. 高级配置

#### 5.1 主动音频（Proactive Audio）

启用此功能后，模型可以选择不回应与当前对话无关的音频（如背景噪音、无关对话等）。

**默认状态**：关闭

#### 5.2 共情对话（Empathetic Mode）

让模型根据输入内容的情绪表达和语气调整回答风格，提供更有同理心的回复。

**默认状态**：关闭

#### 5.3 语音识别灵敏度

**开始语音识别灵敏度**（start_sensitivity）：
- `low`（默认）：避免因背景噪音或停顿而意外开始响应
  - 对应 API 值：`START_SENSITIVITY_LOW`
- `high`：让模型立即响应，适合安静环境
  - 对应 API 值：`START_SENSITIVITY_HIGH`

**结束语音识别灵敏度**（end_sensitivity）：
- `low`：避免因停顿而提前结束识别
  - 对应 API 值：`END_SENSITIVITY_LOW`
- `high`（默认）：让模型快速响应，适合流畅对话
  - 对应 API 值：`END_SENSITIVITY_HIGH`

> **注意**：在 URL 参数中可以使用简化的 `low`/`high` 值，系统会自动转换为 Gemini API 要求的格式。在 Gemini 原生模式的 setup 消息中，需要使用完整的常量格式。

#### 5.4 音频配置

**前缀内边距**（prefix_padding_ms）：
- 范围：0-1000 毫秒
- 默认：0
- 说明：在音频开头添加短暂的静默片段，确保语音的开头不会被截断

**静默时长**（silence_duration_ms）：
- 范围：0-2000 毫秒
- 默认：0
- 说明：设置系统应等待多长时间的停顿，才会将其视为有意义的静默（比如一句话或一个想法结束）

#### 5.5 上下文管理

> **注意**：这些参数目前在 Gemini Live API 中尚未启用，保留供将来使用。

**上下文大小上限**（context_window_threshold）：
- 范围：5000-128000 tokens
- 默认：128000
- 说明：触发滑动窗口的阈值，达到此值后会自动清理旧的上下文
- **状态**：🚧 暂未支持

**目标上下文大小**（target_context_size）：
- 范围：0-128000 tokens
- 默认：102400
- 说明：滑动窗口后保留在上下文中的 token 数
- **状态**：🚧 暂未支持

## 使用方式

### OpenAI 兼容模式

适合已有 OpenAI Realtime API 客户端代码的用户，只需修改 URL 即可使用。

#### 连接示例

```javascript
// 基础连接
const ws = new WebSocket(
  "wss://llm.ai-nebula.com/v1/realtime?model=gemini-live-2.5-flash-native-audio",
  { headers: { "Authorization": "Bearer sk-xxxx" } }
);

// 带高级配置的连接
const ws = new WebSocket(
  "wss://llm.ai-nebula.com/v1/realtime?" +
  "model=gemini-live-2.5-flash-native-audio&" +
  "voice=Zephyr&" +
  "language=zh-CN&" +
  "output_mode=audio_with_text&" +
  "google_search=true&" +
  "empathetic_mode=true&" +
  "start_sensitivity=low&" +
  "end_sensitivity=high",
  { headers: { "Authorization": "Bearer sk-xxxx" } }
);
```

#### 会话配置（session.update）

```json
{
  "type": "session.update",
  "session": {
    "modalities": ["audio", "text"],
    "instructions": "你是一个友好的助手，请用自然、对话式的方式回答问题。",
    "voice": "Zephyr",
    "temperature": 0.8
  }
}
```

**注意**：session.update 中的配置会覆盖 URL 参数中的配置。

### Gemini 原生模式

使用 Gemini 原生协议，可以访问所有 Gemini Live API 的原生功能。

#### 连接示例

```javascript
const ws = new WebSocket(
  "wss://llm.ai-nebula.com/v1beta/models/gemini-live-2.5-flash-native-audio/liveStream",
  { headers: { "Authorization": "Bearer sk-xxxx" } }
);
```

#### Setup 消息示例

```json
{
  "setup": {
    "model": "gemini-live-2.5-flash-native-audio",
    "generation_config": {
      "temperature": 0.7,
      "response_modalities": ["AUDIO"],
      "speech_config": {
        "voice_config": {
          "prebuilt_voice_config": {
            "voice_name": "Zephyr"
          }
        },
        "language_code": "zh-CN"
      }
    },
    "system_instruction": {
      "parts": [
        {"text": "你是一个友好的助手，请用自然、对话式的方式回答问题。"}
      ]
    },
    "tools": {
      "google_search": {}
    },
    "proactivity": {
      "proactive_audio": false,
      "empathetic_mode": true
    },
    "output_audio_transcription": {},
    "realtime_input_config": {
      "automatic_activity_detection": {
        "disabled": false,
        "start_of_speech_sensitivity": "START_SENSITIVITY_LOW",
        "end_of_speech_sensitivity": "END_SENSITIVITY_HIGH",
        "prefix_padding_ms": 0,
        "silence_duration_ms": 0
      }
    }
  }
}
```

## 完整参数列表

| 参数名 | 类型 | 默认值 | 取值范围 | 说明 |
|-------|------|--------|---------|------|
| model | string | - | 必填 | 模型名称 |
| voice | string | Zephyr | 30种音色之一 | 语音音色 |
| language | string | - | BCP-47 代码 | 语言设置 |
| output_mode | string | audio_only | audio_only / audio_with_text | 输出模式 |
| google_search | boolean | false | true / false | 启用 Google 搜索 |
| proactive_audio | boolean | false | true / false | 主动音频 |
| empathetic_mode | boolean | false | true / false | 共情对话 |
| start_sensitivity | string | low | low / high | 开始识别灵敏度 |
| end_sensitivity | string | high | low / high | 结束识别灵敏度 |
| prefix_padding_ms | integer | 0 | 0-1000 | 前缀内边距（毫秒） |
| silence_duration_ms | integer | 0 | 0-2000 | 静默时长（毫秒） |
| context_window_threshold | integer | 128000 | 5000-128000 | 上下文大小上限 🚧 暂未支持 |
| target_context_size | integer | 102400 | 0-128000 | 目标上下文大小 🚧 暂未支持 |

## Python 示例代码

### 基础示例

```python
import websocket
import json

API_BASE = "wss://llm.ai-nebula.com"
API_KEY = "sk-xxxx"
MODEL = "gemini-live-2.5-flash-native-audio"

def on_message(ws, message):
    event = json.loads(message)
    print(f"收到: {event}")

def on_open(ws):
    # OpenAI 兼容模式：发送 session.update
    ws.send(json.dumps({
        "type": "session.update",
        "session": {
            "modalities": ["text"],
            "instructions": "你是一个友好的助手"
        }
    }))
    
    # 发送文本消息
    ws.send(json.dumps({
        "type": "conversation.item.create",
        "item": {
            "type": "message",
            "role": "user",
            "content": [{"type": "input_text", "text": "你好"}]
        }
    }))
    
    # 请求生成
    ws.send(json.dumps({"type": "response.create"}))

ws = websocket.WebSocketApp(
    f"{API_BASE}/v1/realtime?model={MODEL}",
    header={"Authorization": f"Bearer {API_KEY}"},
    on_message=on_message,
    on_open=on_open
)

ws.run_forever()
```

### 音色切换示例

```python
# 使用 URL 参数设置音色
url = f"{API_BASE}/v1/realtime?model={MODEL}&voice=Puck&language=ja-JP"

ws = websocket.WebSocketApp(
    url,
    header={"Authorization": f"Bearer {API_KEY}"},
    # ... 其他回调
)
```

### 启用 Google 搜索示例

```python
# OpenAI 兼容模式
url = f"{API_BASE}/v1/realtime?model={MODEL}&google_search=true"

# Gemini 原生模式
def on_open(ws):
    ws.send(json.dumps({
        "setup": {
            "model": MODEL,
            "generation_config": {
                "response_modalities": ["TEXT"]
            },
            "tools": {
                "google_search": {}
            }
        }
    }))
```

### 高级配置示例

```python
# 完整配置
url = (
    f"{API_BASE}/v1/realtime?"
    f"model={MODEL}&"
    f"voice=Zephyr&"
    f"language=zh-CN&"
    f"output_mode=audio_with_text&"
    f"google_search=true&"
    f"proactive_audio=true&"
    f"empathetic_mode=true&"
    f"start_sensitivity=low&"
    f"end_sensitivity=high&"
    f"prefix_padding_ms=20&"
    f"silence_duration_ms=100&"
    f"context_window_threshold=128000&"
    f"target_context_size=102400"
)

ws = websocket.WebSocketApp(
    url,
    header={"Authorization": f"Bearer {API_KEY}"},
    # ... 回调函数
)
```

## 常见问题

### 音色不生效

**问题**：设置了音色参数，但听到的语音没有变化。

**解决方案**：
1. 确认音色名称拼写正确（区分大小写）
2. OpenAI 模式：检查 URL 参数或 session.update 中的 voice 字段
3. Gemini 原生模式：检查 setup 消息中的 voice_name 字段
4. 注意：某些模型可能不支持所有音色，建议使用默认支持的音色

### Google 搜索无响应

**问题**：启用了 Google 搜索，但模型没有使用搜索结果。

**解决方案**：
1. 确认参数 `google_search=true` 已正确设置
2. Gemini 原生模式需在 setup 的 tools 中添加 `google_search` 对象
3. 提问时明确提示模型需要搜索（如："搜索一下..."、"最新的..."）
4. Google 搜索功能需要模型自主判断是否使用，不是所有问题都会触发搜索

### 语音识别过于敏感

**问题**：模型经常被背景噪音或停顿触发，导致频繁中断。

**解决方案**：
1. 将 `start_sensitivity` 设置为 `low`（降低开始识别的灵敏度）
2. 增加 `silence_duration_ms`（如设置为 500-1000ms）
3. 启用 `proactive_audio=true`（让模型过滤无关音频）
4. 确保录音环境尽量安静

### 上下文溢出

**问题**：长对话后模型无法记住早期内容。

**解决方案**：
1. 增大 `context_window_threshold`（最大 128000）
2. 调整 `target_context_size`，控制滑动窗口后保留的内容量
3. 对于超长对话，考虑定期总结之前的内容
4. 使用系统指令引导模型关注重点信息

### 音频质量问题

**问题**：输出音频有杂音或不流畅。

**解决方案**：
1. 确认输入音频格式正确（PCM16 单声道 16kHz）
2. 增加 `prefix_padding_ms=20`（避免开头被截断）
3. 检查网络连接质量
4. 尝试不同的音色

## 定价说明

Gemini Live API 按 token 计费，音频和文本分别计价：

### 价格表

| 类型 | 价格（每百万 tokens） |
|------|---------------------|
| 文本输入 | $0.375 |
| 文本输出 | $1.50 |
| 音频输入 | $2.25 |
| 音频输出 | $9.00 |

### 计费说明

1. **音频 Token 计算**：
   - 音频按时长转换为 tokens
   - 16kHz 音频：约 100 tokens/秒
   - 24kHz 音频：约 150 tokens/秒

2. **文本 Token 计算**：
   - 中文：约 1.5-2 字符/token
   - 英文：约 4 字符/token

3. **计费时机**：
   - 实时对话采用**后计费**模式
   - 会话结束时统一结算
   - 每轮对话完成后会返回 usage 统计

### 使用建议

1. 使用 `output_mode=audio_only` 可节省文本输出成本
2. 合理设置 `context_window_threshold` 避免不必要的上下文保留
3. 使用 `proactive_audio=true` 过滤无关音频输入
4. 对于仅需文本的场景，设置 `modalities=["text"]` 可大幅降低成本


---

**更新时间**：2025-12-22

