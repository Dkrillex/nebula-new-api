#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
GPT-realtime API 简单测试脚本
快速测试 gpt-realtime 和 gpt-realtime-mini
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
MODEL = "gpt-realtime"  # 或 "gpt-realtime-mini"
# MODEL = "gpt-realtime-mini"  # 或 "gpt-realtime-mini"


# ==============================

class SimpleRealtimeClient:
    def __init__(self, api_base, api_key, model):
        self.api_base = api_base.rstrip('/')
        self.api_key = api_key
        self.model = model
        self.ws = None
        self.connected = False
        self.event_id = 0
        self.output_audio_buffer = BytesIO()
        self.output_text_buffer = []
        self.output_stream = None
        self.sample_rate = 24000
        self.missing_audio_warned = False
        self.audio_saved = False

    def _next_event_id(self):
        self.event_id += 1
        return f"evt_{self.event_id:03d}"

    def _on_message(self, ws, message):
        try:
            event = json.loads(message)
            event_type = event.get("type")

            if event_type == "session.created":
                print("✓ 会话已创建")
            elif event_type == "response.created":
                self.output_audio_buffer = BytesIO()
                self.output_text_buffer = []
                self.audio_saved = False
                print("✓ 响应已创建")
            elif event_type == "response.audio_transcript.delta":
                # 流式输出文本
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
                    print(f"\n[音频流] +{len(audio_bytes)} bytes (累计 {self.output_audio_buffer.tell()} bytes)")
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
        except Exception as e:
            print(f"\n解析消息错误: {e}")

    def _on_error(self, ws, error):
        print(f"错误: {error}")

    def _on_close(self, ws, *args):
        self.connected = False
        self._stop_audio_playback()
        print("\n连接已关闭")

    def _on_open(self, ws):
        self.connected = True
        print(f"✓ 已连接到 {self.model}")

        # 自动发送会话更新
        self.send_session_update()
        time.sleep(0.5)

    def send(self, event_type, **data):
        if not self.connected:
            return

        event = {
            "event_id": self._next_event_id(),
            "type": event_type,
            **data
        }
        self.ws.send(json.dumps(event))

    def send_session_update(self):
        self.send("session.update", session={
            "modalities": ["text", "audio"],
            "instructions": "你是一个友好的助手",
            "voice": "alloy",
            "input_audio_format": "pcm16",
            "output_audio_format": "pcm16",
            "input_audio_transcription": {"model": "whisper-1"}
        })

    def send_message(self, text):
        item_id = f"item_{uuid.uuid4().hex[:8]}"
        self.send("conversation.item.create", item={
            "id": item_id,
            "type": "message",
            "role": "user",
            "content": [{"type": "input_text", "text": text}]
        })

    def request_response(self):
        self.send("response.create", response={"modalities": ["text", "audio"]})

    def connect(self):
        url = f"{self.api_base}/v1/realtime?model={self.model}"
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
                samplerate=self.sample_rate,
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
        filename = f"output_audio_{int(time.time())}.pcm"
        try:
            self.output_audio_buffer.seek(0)
            with open(filename, "wb") as f:
                f.write(self.output_audio_buffer.read())
            print(f"  [{tag}] 音频已保存: {filename} (播放: ffplay -f s16le -ar {self.sample_rate} -ac 1 {filename})")
            self.audio_saved = True
        except Exception as e:
            print(f"  [{tag}] 保存音频失败: {e}")
        finally:
            self.output_audio_buffer = BytesIO()


def record_audio_base64(seconds: int = 5, sample_rate: int = 24000) -> str:
    """录制指定时长音频并返回 base64 字符串"""
    if not HAS_SD:
        print("缺少 sounddevice，无法录音，请先 pip install sounddevice")
        return ""
    print(f"开始录音 {seconds}s ...")
    audio = sd.rec(int(seconds * sample_rate), samplerate=sample_rate, channels=1, dtype="int16")
    sd.wait()
    print("录音完成")
    return base64.b64encode(audio.tobytes()).decode()


def main():
    print("=" * 60)
    print("GPT-realtime API 简单测试")
    print("=" * 60)
    print(f"模型: {MODEL}")
    print(f"地址: {API_BASE}")
    print("=" * 60)

    # 配置已写死，无需检查

    client = SimpleRealtimeClient(API_BASE, API_KEY, MODEL)

    try:
        # 连接
        print("\n[正在连接...]")
        client.connect()
        time.sleep(1)

        print("\n指令说明：直接输入文本发送；输入 /a 录音5秒并发送语音；输入 /q 退出。")
        while True:
            user_input = input("\n输入内容(/a 录音, /q 退出，回车默认问好): ").strip()
            if user_input in ("/q", "q", "quit", "exit"):
                print("退出会话")
                break

            if user_input in ("/a", "a"):
                audio_b64 = record_audio_base64()
                if not audio_b64:
                    continue
                client.send("conversation.item.create", item={
                    "id": f"item_{uuid.uuid4().hex[:8]}",
                    "type": "message",
                    "role": "user",
                    "content": [{"type": "input_audio", "audio": audio_b64}]
                })
            else:
                if not user_input:
                    user_input = "你好，请简单介绍一下你自己。"
                print(f"\n[发送消息] {user_input}")
                client.send_message(user_input)

            time.sleep(0.2)
            print("[等待响应...]\n")
            client.request_response()

            # 简单等待一轮完成；如需更精确可改为事件通知
            time.sleep(20)

    except KeyboardInterrupt:
        print("\n\n用户中断")
    except Exception as e:
        print(f"\n错误: {e}")
    finally:
        client.close()
        print("\n测试完成")


if __name__ == "__main__":
    main()

