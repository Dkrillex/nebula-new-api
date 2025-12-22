#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
Gemini Live API 测试脚本
支持 Gemini Live API 的实时语音和文本交互
支持两种模式：OpenAI 兼容模式（通过 /v1/realtime）和 Gemini 原生模式（通过 /v1beta/models/.../liveStream）
"""

import json
import time
import uuid
import websocket
import threading
import base64
from io import BytesIO

try:
    import sounddevice as sd
    HAS_SD = True
except Exception:
    HAS_SD = False

# ========== 配置区域 ==========
API_BASE = "ws://localhost:3003"  # 本地测试地址
API_KEY = "sk-uQQ76mqGcyfwEqfYp1LL4fRyCvHA7oSRJCgZQq697tbYsmpD"

# 模型选择（Gemini Live API 模型）
MODEL = "gemini-live-2.5-flash-native-audio"
# MODEL = "gemini-live-2.5-flash-preview-native-audio-09-2025"
# MODEL = "gemini-2.5-flash-native-audio-preview-12-2025"

# 使用模式：'openai' 使用 OpenAI 兼容接口，'gemini' 使用 Gemini 原生接口
MODE = "gemini"  # 或 "openai"

# ========== 高级配置 ==========
# 音色配置（30种可选音色之一）
VOICE = "Zephyr"  # 默认音色：明快
# 其他音色: Kore, Orus, Autonoe, Umbriel, Erinome, Laomedeia, Schedar, Achird, Sadachbia,
#          Puck, Fenrir, Aoede, Enceladus, Algieba, Algenib, Achernar, Gacrux, 
#          Zubenelgenubi, Sadaltager, Charon, Leda, Callirrhoe, Iapetus, Despina,
#          Rasalgethi, Alnilam, Pulcherrima, Vindemiatrix, Sulafat

# 语言配置（24种可选语言之一）
LANGUAGE = "zh-CN"  # 默认中文
# 其他语言: en-US, ja-JP, ko-KR, fr-FR, de-DE, es-US, pt-BR, it-IT, ru-RU, 
#          ar-EG, hi-IN, id-ID, nl-NL, pl-PL, th-TH, tr-TR, vi-VN, ro-RO,
#          uk-UA, bn-BD, en-IN, mr-IN, ta-IN, te-IN

# 输出模式
OUTPUT_MODE = "audio_only"  # "audio_only" 或 "audio_with_text"（音频+文本转录）

# 功能开关
GOOGLE_SEARCH = False      # 是否启用 Google 搜索（接地）
PROACTIVE_AUDIO = False    # 主动音频：模型可以选择不回应与当前对话无关的音频
EMPATHETIC_MODE = False    # 共情对话：根据输入内容的情绪表达和语气调整回答风格

# 语音识别灵敏度（使用用户友好的值，服务器会自动转换）
START_SENSITIVITY = "low"   # 开始识别灵敏度："low" 或 "high"
END_SENSITIVITY = "high"    # 结束识别灵敏度："low" 或 "high"

# 音频配置
PREFIX_PADDING_MS = 0       # 前缀内边距（0-1000ms）
SILENCE_DURATION_MS = 0     # 静默时长（0-2000ms）

# 上下文管理
CONTEXT_WINDOW = 128000     # 上下文大小上限（5000-128000）
TARGET_CONTEXT = 102400     # 目标上下文大小（0-128000）

# ==============================

class GeminiLiveClient:
    def __init__(self, api_base, api_key, model, mode="openai", **config):
        self.api_base = api_base.rstrip('/')
        self.api_key = api_key
        self.model = model
        self.mode = mode
        self.config = config  # 存储高级配置
        self.ws = None
        self.connected = False
        self.setup_complete = False
        self.output_audio_buffer = BytesIO()
        self.output_text_buffer = []
        self.output_stream = None
        # Gemini 输入音频：16kHz，输出音频：24kHz
        self.input_sample_rate = 16000
        self.output_sample_rate = 24000
        self.missing_audio_warned = False
        self.audio_saved = False

    def _on_message(self, ws, message):
        try:
            event = json.loads(message)
            
            if self.mode == "openai":
                self._handle_openai_message(event)
            else:
                self._handle_gemini_message(event)
                
        except Exception as e:
            print(f"\n解析消息错误: {e}")
            print(f"原始消息: {message[:200]}")

    def _handle_openai_message(self, event):
        """处理 OpenAI 兼容格式的消息"""
        event_type = event.get("type")

        if event_type == "session.created":
            print("✓ 会话已创建")
            self.setup_complete = True
        elif event_type == "session.updated":
            print("✓ 会话已更新")
            self.setup_complete = True
        elif event_type == "response.created":
            self.output_audio_buffer = BytesIO()
            self.output_text_buffer = []
            self.audio_saved = False
            print("✓ 响应已创建")
        elif event_type == "response.audio_transcript.delta":
            delta = event.get("delta", "")
            print(delta, end="", flush=True)
        elif event_type == "response.text.delta":
            delta = event.get("delta", "")
            if delta:
                self.output_text_buffer.append(delta)
                print(delta, end="", flush=True)
        elif event_type == "response.text.done":
            if self.output_text_buffer:
                print(f"\n✓ 文本完成: {''.join(self.output_text_buffer)}")
        elif event_type == "response.audio.delta":
            audio_b64 = event.get("audio", "")
            if audio_b64:
                audio_bytes = base64.b64decode(audio_b64)
                self.output_audio_buffer.write(audio_bytes)
                self._start_audio_playback()
                if self.output_stream:
                    try:
                        self.output_stream.write(audio_bytes)
                    except Exception as e:
                        print(f"\n⚠️ 音频播放失败: {e}")
                        self._stop_audio_playback()
        elif event_type == "response.audio.done":
            self._stop_audio_playback()
            print(f"\n✓ 音频完成，累计 {self.output_audio_buffer.tell()} bytes")
            self._save_audio_buffer(tag="response.audio.done")
        elif event_type == "response.done":
            self._stop_audio_playback()
            print("\n✓ 响应完成")
            self._save_audio_buffer(tag="response.done")
            if self.output_text_buffer:
                print(f"  完整文本: {''.join(self.output_text_buffer)}")
                self.output_text_buffer = []
            usage = event.get("response", {}).get("usage", {})
            if usage:
                print(f"  使用量: {usage.get('total_tokens', 0)} tokens")
        elif event_type == "error":
            error = event.get("error", {})
            print(f"\n✗ 错误: {error.get('message', 'Unknown')}")

    def _handle_gemini_message(self, event):
        """处理 Gemini 原生格式的消息"""
        if "setupComplete" in event:
            print("✓ 设置完成")
            self.setup_complete = True
        elif "serverContent" in event:
            server_content = event["serverContent"]
            
            # 处理输出转录（音频转文本）
            if "outputTranscription" in server_content:
                transcription = server_content["outputTranscription"]
                text = transcription.get("text", "")
                if text:
                    self.output_text_buffer.append(text)
                    print(f"[转录] {text}", end="", flush=True)
            
            # 处理输入转录（用户音频转文本）
            if "inputTranscription" in server_content:
                transcription = server_content["inputTranscription"]
                text = transcription.get("text", "")
                if text:
                    print(f"\n[输入转录] {text}")
            
            if server_content.get("turnComplete"):
                self._stop_audio_playback()
                print("\n✓ 回合完成")
                self._save_audio_buffer(tag="turnComplete")
                if self.output_text_buffer:
                    print(f"  完整文本: {''.join(self.output_text_buffer)}")
                    self.output_text_buffer = []
            
            if server_content.get("interrupted"):
                print("\n⚠️ 响应被中断")
            
            if "modelTurn" in server_content:
                model_turn = server_content["modelTurn"]
                if "parts" in model_turn:
                    for part in model_turn["parts"]:
                        if "text" in part:
                            text = part["text"]
                            self.output_text_buffer.append(text)
                            print(text, end="", flush=True)
                        elif "inlineData" in part:
                            inline_data = part["inlineData"]
                            if inline_data.get("mimeType") == "audio/pcm":
                                audio_b64 = inline_data.get("data", "")
                                if audio_b64:
                                    audio_bytes = base64.b64decode(audio_b64)
                                    self.output_audio_buffer.write(audio_bytes)
                                    self._start_audio_playback()
                                    if self.output_stream:
                                        try:
                                            self.output_stream.write(audio_bytes)
                                        except Exception as e:
                                            print(f"\n⚠️ 音频播放失败: {e}")
                                            self._stop_audio_playback()
        elif "toolCall" in event:
            tool_call = event["toolCall"]
            print(f"\n[工具调用] {tool_call}")
        elif "error" in event:
            error = event["error"]
            print(f"\n✗ 错误: {error}")

    def _on_error(self, ws, error):
        print(f"错误: {error}")

    def _on_close(self, ws, *args):
        self.connected = False
        self.setup_complete = False
        self._stop_audio_playback()
        print("\n连接已关闭")

    def _on_open(self, ws):
        self.connected = True
        print(f"✓ 已连接到 {self.model} (模式: {self.mode})")

        if self.mode == "openai":
            # OpenAI 兼容模式：发送 session.update
            self.send_openai_session_update()
        else:
            # Gemini 原生模式：发送 setup
            self.send_gemini_setup()
        
        time.sleep(0.5)

    def send_openai_session_update(self):
        """发送 OpenAI 兼容格式的会话配置"""
        message = {
            "type": "session.update",
            "session": {
                "modalities": ["text", "audio"],
                "instructions": "你是一个友好的助手，请用自然、对话式的方式回答问题。",
                "voice": "alloy",  # OpenAI 语音，会自动映射到 Gemini
                "input_audio_format": "pcm16",
                "output_audio_format": "pcm16",
                "input_audio_transcription": {
                    "model": "whisper-1"
                }
            }
        }
        self.ws.send(json.dumps(message))

    def send_gemini_setup(self):
        """发送 Gemini 原生格式的 setup 消息"""
        # 构建语音配置
        speech_config = {
            "voice_config": {
                "prebuilt_voice_config": {
                    "voice_name": self.config.get("voice", "Zephyr")
                }
            }
        }
        
        # 添加语言配置
        if self.config.get("language"):
            speech_config["language_code"] = self.config["language"]
        
        # 构建生成配置
        generation_config = {
            "temperature": 0.7,
            "response_modalities": ["AUDIO"],  # Gemini 只能有一个，优先音频
            "speech_config": speech_config
        }
        
        # 注意：Gemini Live API 目前不支持上下文配置字段
        # 这些字段被保留在 URL 参数中，但不会在 setup 消息中发送
        
        message = {
            "setup": {
                "model": self.model,
                "generation_config": generation_config,
                "system_instruction": {
                    "parts": [
                        {
                            "text": "你是一个友好的助手，请用自然、对话式的方式回答问题。"
                        }
                    ]
                }
            }
        }
        
        # 添加 Google 搜索
        if self.config.get("google_search"):
            message["setup"]["tools"] = {
                "google_search": {}
            }
        
        # 添加主动音频和共情模式
        if self.config.get("proactive_audio") or self.config.get("empathetic_mode"):
            message["setup"]["proactivity"] = {
                "proactive_audio": self.config.get("proactive_audio", False),
                "empathetic_mode": self.config.get("empathetic_mode", False)
            }
        
        # 添加输出转录
        if self.config.get("output_mode") == "audio_with_text":
            message["setup"]["output_audio_transcription"] = {}
        
        # 添加语音识别配置
        # 将用户友好的值转换为 Gemini API 格式
        start_sens = self.config.get("start_sensitivity", "low")
        end_sens = self.config.get("end_sensitivity", "high")
        
        # 转换为 Gemini API 要求的大写常量格式
        start_sens_map = {
            "low": "START_SENSITIVITY_LOW",
            "high": "START_SENSITIVITY_HIGH"
        }
        end_sens_map = {
            "low": "END_SENSITIVITY_LOW",
            "high": "END_SENSITIVITY_HIGH"
        }
        
        message["setup"]["realtime_input_config"] = {
            "automatic_activity_detection": {
                "disabled": False,
                "start_of_speech_sensitivity": start_sens_map.get(start_sens, "START_SENSITIVITY_LOW"),
                "end_of_speech_sensitivity": end_sens_map.get(end_sens, "END_SENSITIVITY_HIGH"),
                "prefix_padding_ms": self.config.get("prefix_padding_ms", 0),
                "silence_duration_ms": self.config.get("silence_duration_ms", 0)
            }
        }
        
        self.ws.send(json.dumps(message))

    def send_openai_text(self, text):
        """发送 OpenAI 格式的文本消息"""
        message = {
            "type": "conversation.item.create",
            "item": {
                "id": f"item_{uuid.uuid4().hex[:8]}",
                "type": "message",
                "role": "user",
                "content": [{"type": "input_text", "text": text}]
            }
        }
        self.ws.send(json.dumps(message))
        # 请求响应
        response_msg = {
            "type": "response.create",
            "response": {"modalities": ["text", "audio"]}
        }
        self.ws.send(json.dumps(response_msg))

    def send_gemini_text(self, text):
        """发送 Gemini 格式的文本消息"""
        message = {
            "client_content": {  # 使用 snake_case
                "turns": [
                    {
                        "role": "user",
                        "parts": [
                            {"text": text}
                        ]
                    }
                ],
                "turn_complete": True  # 使用 snake_case
            }
        }
        self.ws.send(json.dumps(message))

    def send_openai_audio(self, audio_b64):
        """发送 OpenAI 格式的音频消息"""
        message = {
            "type": "input_audio_buffer.append",
            "audio": audio_b64
        }
        self.ws.send(json.dumps(message))

    def send_gemini_audio(self, audio_b64):
        """发送 Gemini 格式的实时音频输入"""
        message = {
            "realtime_input": {  # 使用 snake_case
                "media_chunks": [  # 使用 snake_case
                    {
                        "mime_type": "audio/pcm;rate=16000",  # 使用 snake_case
                        "data": audio_b64
                    }
                ]
            }
        }
        self.ws.send(json.dumps(message))

    def send_text(self, text):
        """发送文本消息（自动选择格式）"""
        if self.mode == "openai":
            self.send_openai_text(text)
        else:
            self.send_gemini_text(text)

    def send_audio(self, audio_b64):
        """发送音频消息（自动选择格式）"""
        if self.mode == "openai":
            self.send_openai_audio(audio_b64)
        else:
            self.send_gemini_audio(audio_b64)

    def connect(self):
        """建立 WebSocket 连接"""
        if self.mode == "openai":
            # OpenAI 兼容接口，支持 URL 参数配置
            params = [f"model={self.model}"]
            
            # 添加高级配置参数
            if self.config.get("voice"):
                params.append(f"voice={self.config['voice']}")
            if self.config.get("language"):
                params.append(f"language={self.config['language']}")
            if self.config.get("output_mode"):
                params.append(f"output_mode={self.config['output_mode']}")
            if self.config.get("google_search"):
                params.append(f"google_search=true")
            if self.config.get("proactive_audio"):
                params.append(f"proactive_audio=true")
            if self.config.get("empathetic_mode"):
                params.append(f"empathetic_mode=true")
            if self.config.get("start_sensitivity"):
                params.append(f"start_sensitivity={self.config['start_sensitivity']}")
            if self.config.get("end_sensitivity"):
                params.append(f"end_sensitivity={self.config['end_sensitivity']}")
            if self.config.get("prefix_padding_ms"):
                params.append(f"prefix_padding_ms={self.config['prefix_padding_ms']}")
            if self.config.get("silence_duration_ms"):
                params.append(f"silence_duration_ms={self.config['silence_duration_ms']}")
            if self.config.get("context_window"):
                params.append(f"context_window_threshold={self.config['context_window']}")
            if self.config.get("target_context"):
                params.append(f"target_context_size={self.config['target_context']}")
            
            url = f"{self.api_base}/v1/realtime?{'&'.join(params)}"
        else:
            # Gemini 原生接口
            url = f"{self.api_base}/v1beta/models/{self.model}/liveStream"
        
        headers = {"Authorization": f"Bearer {self.api_key}"}

        self.ws = websocket.WebSocketApp(
            url,
            header=headers,
            on_message=self._on_message,
            on_error=self._on_error,
            on_close=self._on_close,
            on_open=self._on_open
        )

        thread = threading.Thread(target=self.ws.run_forever)
        thread.daemon = True
        thread.start()

        # 等待连接
        for _ in range(50):
            if self.connected:
                break
            time.sleep(0.1)

        if not self.connected:
            raise Exception("连接超时")

        # 等待设置完成
        for _ in range(30):
            if self.setup_complete:
                break
            time.sleep(0.1)

    def close(self):
        if self.ws:
            self.ws.close()
        self._stop_audio_playback()

    def _start_audio_playback(self):
        """在本机实时播放 AI 返回的 PCM16 音频"""
        if not HAS_SD:
            if not self.missing_audio_warned:
                print("⚠️ 未安装 sounddevice，无法实时播放音频")
                self.missing_audio_warned = True
            return
        if self.output_stream:
            return
        try:
            self.output_stream = sd.RawOutputStream(
                samplerate=self.output_sample_rate,  # Gemini 输出是 24kHz
                channels=1,
                dtype="int16",
                blocksize=0,
            )
            self.output_stream.start()
        except Exception as e:
            if not self.missing_audio_warned:
                print(f"⚠️ 启动音频播放失败: {e}")
                self.missing_audio_warned = True
            self.output_stream = None

    def _stop_audio_playback(self):
        """停止并清理音频播放流"""
        if self.output_stream:
            try:
                self.output_stream.stop()
                self.output_stream.close()
            except Exception:
                pass
            self.output_stream = None

    def _save_audio_buffer(self, tag: str = "response.done"):
        """将当前音频缓冲写入 PCM 文件（若有数据且未保存）"""
        if self.audio_saved:
            return
        if self.output_audio_buffer.tell() <= 0:
            return
        filename = f"gemini_output_audio_{int(time.time())}.pcm"
        try:
            self.output_audio_buffer.seek(0)
            with open(filename, "wb") as f:
                f.write(self.output_audio_buffer.read())
            print(f"  [{tag}] 音频已保存: {filename} (播放: ffplay -f s16le -ar {self.output_sample_rate} -ac 1 {filename})")
            self.audio_saved = True
        except Exception as e:
            print(f"  [{tag}] 保存音频失败: {e}")
        finally:
            self.output_audio_buffer = BytesIO()


def record_audio_base64(seconds: int = 5, sample_rate: int = 16000) -> str:
    """
    录制指定时长音频并返回 base64 字符串
    Gemini Live API 输入音频需要 16kHz
    """
    if not HAS_SD:
        print("缺少 sounddevice，无法录音，请先 pip install sounddevice")
        return ""
    print(f"开始录音 {seconds}s (采样率: {sample_rate}Hz)...")
    audio = sd.rec(int(seconds * sample_rate), samplerate=sample_rate, channels=1, dtype="int16")
    sd.wait()
    print("录音完成")
    return base64.b64encode(audio.tobytes()).decode()


def main():
    print("=" * 60)
    print("Gemini Live API 测试")
    print("=" * 60)
    print(f"模型: {MODEL}")
    print(f"模式: {MODE} ({'OpenAI 兼容' if MODE == 'openai' else 'Gemini 原生'})")
    print(f"地址: {API_BASE}")
    print(f"音色: {VOICE}")
    print(f"语言: {LANGUAGE}")
    print(f"输出模式: {OUTPUT_MODE}")
    if GOOGLE_SEARCH:
        print("✓ Google 搜索已启用")
    if PROACTIVE_AUDIO:
        print("✓ 主动音频已启用")
    if EMPATHETIC_MODE:
        print("✓ 共情对话已启用")
    print("=" * 60)

    # 构建配置字典
    config = {
        "voice": VOICE,
        "language": LANGUAGE,
        "output_mode": OUTPUT_MODE,
        "google_search": GOOGLE_SEARCH,
        "proactive_audio": PROACTIVE_AUDIO,
        "empathetic_mode": EMPATHETIC_MODE,
        "start_sensitivity": START_SENSITIVITY,
        "end_sensitivity": END_SENSITIVITY,
        "prefix_padding_ms": PREFIX_PADDING_MS,
        "silence_duration_ms": SILENCE_DURATION_MS,
        "context_window": CONTEXT_WINDOW,
        "target_context": TARGET_CONTEXT
    }

    client = GeminiLiveClient(API_BASE, API_KEY, MODEL, MODE, **config)

    try:
        # 连接
        print("\n[正在连接...]")
        client.connect()
        time.sleep(1)

        print("\n指令说明：")
        print("  - 直接输入文本发送消息")
        print("  - 输入 /a 录音5秒并发送语音（16kHz）")
        print("  - 输入 /mode 切换模式（openai/gemini）")
        print("  - 输入 /q 退出")
        
        while True:
            user_input = input("\n输入内容(/a 录音, /mode 切换, /q 退出，回车默认问好): ").strip()
            
            if user_input in ("/q", "q", "quit", "exit"):
                print("退出会话")
                break

            if user_input == "/mode":
                new_mode = "gemini" if client.mode == "openai" else "openai"
                print(f"切换模式需要重新连接，当前模式: {client.mode} -> {new_mode}")
                client.close()
                client.mode = new_mode
                client.connect()
                time.sleep(1)
                continue

            if user_input in ("/a", "a"):
                audio_b64 = record_audio_base64(sample_rate=client.input_sample_rate)
                if not audio_b64:
                    continue
                print(f"\n[发送音频] {len(audio_b64)} 字符 (base64)")
                client.send_audio(audio_b64)
            else:
                if not user_input:
                    user_input = "你好，请简单介绍一下你自己。"
                print(f"\n[发送消息] {user_input}")
                client.send_text(user_input)

            # 等待响应
            time.sleep(0.2)
            print("[等待响应...]\n")
            
            # 简单等待一轮完成
            time.sleep(20)

    except KeyboardInterrupt:
        print("\n\n用户中断")
    except Exception as e:
        print(f"\n错误: {e}")
        import traceback
        traceback.print_exc()
    finally:
        client.close()
        print("\n测试完成")


if __name__ == "__main__":
    main()

