import asyncio
import json
import os
import time
import logging
import queue
from typing import Optional
import google.auth.transport.requests
# 第三方库
import numpy as np
from scipy import signal
import pyaudio
from google import genai
from google.genai import types
from google.genai.types import LiveConnectConfig, SpeechConfig, VoiceConfig, PrebuiltVoiceConfig
from google.oauth2 import service_account

# ================= 配置区域 =================
# 1. 你的 Google Cloud 项目 ID
PROJECT_ID = "digital-human-api"
# 2. 你的 Service Account JSON 密钥文件路径
KEY_FILE_PATH = "/Users/caihongzhan/Desktop/nebula-test-2025-11-24-vertex.json"
# 3. Google Cloud 区域
LOCATION = "us-central1"
# 4. 音色名称
VOICE_NAME = "zephyr"
# ===========================================

# 配置简易日志
logging.basicConfig(level=logging.INFO, format='%(asctime)s - %(levelname)s - %(message)s')
logger = logging.getLogger("LocalAssistant")


# ================== 模拟缺失的工具函数 (Mock Tools) ==================

def resample_bytearray(data: bytes, input_rate: int, output_rate: int) -> bytes:
    """简易重采样函数"""
    if input_rate == output_rate:
        return data

    audio_data = np.frombuffer(data, dtype=np.int16)
    num_samples = int(len(audio_data) * output_rate / input_rate)
    resampled_data = signal.resample(audio_data, num_samples)
    return resampled_data.astype(np.int16).tobytes()


class MockWebSocketRx:
    """模拟接收通道 (从麦克风读取数据放入这里)"""

    def __init__(self):
        self.queue = asyncio.Queue()

    async def put(self, data):
        await self.queue.put(type('Event', (object,), {"data": data}))

    def __aiter__(self):
        return self

    async def __anext__(self):
        item = await self.queue.get()
        return item


class MockWebSocketTx:
    """模拟发送通道 (播放音频)"""

    def __init__(self):
        self.p = pyaudio.PyAudio()
        self.stream = self.p.open(
            format=pyaudio.paInt16,
            channels=1,
            rate=24000,  # Gemini 输出通常是 24k
            output=True
        )

    async def send_message(self, message):
        # 原代码中 message 是一个带协议头的 buffer
        # 这里我们假设 message 主要是为了播放音频
        # 在本地版，我们会修改逻辑直接传 PCM 数据进来，或者解析协议
        # 为了简化，我们在主逻辑里直接处理音频播放，这里仅作为占位
        pass

    def play_audio(self, pcm_data):
        if len(pcm_data) > 0:
            self.stream.write(pcm_data)


# ================== 核心状态管理 (保持原逻辑) ==================

class SessionContinuityState:
    def __init__(self):
        self.handle: Optional[str] = None
        self.conversation_history: list = []
        self.total_tokens: int = 0
        self.session_start_time: Optional[float] = None
        self.last_activity_time: Optional[float] = None
        self.reconnection_count: int = 0
        self.is_connected: bool = False
        self.context_summary: Optional[str] = None


# ================== 优化后的助手类 ==================

class LocalLiveAssistant:
    def __init__(self, ws_rx, audio_player, system_text, start_text, ttsSpeaker):
        self.ws_rx = ws_rx  # 模拟的接收通道
        self.audio_player = audio_player  # 音频播放器

        self.buffer = bytearray()
        self._started = False
        self.system_text = system_text
        self.start_text = start_text
        self.ttsSpeaker = ttsSpeaker
        self.session = None

        self.continuity_state = SessionContinuityState()
        self._should_reconnect = True

        # 认证信息
        if os.path.exists(KEY_FILE_PATH):
            self.credentials = service_account.Credentials.from_service_account_file(
                KEY_FILE_PATH,
                scopes=["https://www.googleapis.com/auth/cloud-platform"]
            )
        else:
            logger.error(f"找不到密钥文件: {KEY_FILE_PATH}")
            self.credentials = None

        # 初始化客户端
        if self.credentials:
            self.gmclient = genai.Client(
                vertexai=True,
                project=PROJECT_ID,
                location=LOCATION,
                credentials=self.credentials,
            )

    def _create_config(self) -> LiveConnectConfig:
        return LiveConnectConfig(
            session_resumption=types.SessionResumptionConfig(
                transparent=True,
                handle=self.continuity_state.handle
            ),
            response_modalities=["AUDIO"],
            speech_config=SpeechConfig(
                voice_config=VoiceConfig(
                    prebuilt_voice_config=PrebuiltVoiceConfig(
                        voice_name=self.ttsSpeaker,
                    )
                ),
            ),
            system_instruction=self.system_text,
        )

    async def start(self) -> None:
        if not self.credentials:
            logger.error("无法启动：缺少凭证")
            return

        self._started = True
        self.continuity_state.session_start_time = time.time()

        # 启动音频接收 (发送麦克风数据给 Gemini)
        asyncio.create_task(self._audio_stream_uploader())
        # 启动主循环 (接收 Gemini 数据并播放)
        await self._main_task()

    async def _main_task(self):
        logger.info(">>> 开始连接 Gemini Live...")

        while self._started and self._should_reconnect:
            try:
                config = self._create_config()

                # 连接 Gemini
                async with self.gmclient.aio.live.connect(
                        model="gemini-live-2.5-flash-native-audio",  # 使用最新的模型名称，根据实际情况调整
                        config=config
                ) as session:
                    logger.info(">>> 已连接到 Google Gemini!")
                    self.session = session
                    self.continuity_state.is_connected = True

                    # 初次连接发送开场白
                    if self.continuity_state.reconnection_count == 0:
                        await self.session.send_client_content(
                            turns=types.Content(role="user", parts=[types.Part(text=self.start_text)])
                        )

                    self.continuity_state.reconnection_count += 1

                    # 处理接收到的消息
                    await self._handle_incoming_messages(session)

            except Exception as e:
                logger.error(f"连接断开或出错: {e}")
                self.continuity_state.is_connected = False
                await asyncio.sleep(2)  # 避免死循环重连过快

    async def _audio_stream_uploader(self):
        """从模拟通道读取麦克风数据并发送给服务器"""
        # logger.info("启动音频上传协程")
        while self._started:
            try:
                # 从队列获取数据
                ev = await self.ws_rx.__anext__()
                self.buffer.extend(ev.data)

                chunk_size = 4096

                if len(self.buffer) >= chunk_size:
                    if self.session and self.continuity_state.is_connected:
                        audio_chunk = bytes(self.buffer[:chunk_size])
                        del self.buffer[:chunk_size]

                        await self.session.send_realtime_input(
                            media=types.Blob(
                                data=audio_chunk,
                                mime_type="audio/pcm;rate=16000"
                            )
                        )
                    else:
                        if len(self.buffer) > 64000:
                            self.buffer.clear()
            except Exception as e:
                # === 修改开始：过滤掉 1000 (OK) 正常关闭的报错 ===
                error_str = str(e)
                if "1000 (OK)" in error_str:
                    # 这是连接正常关闭的信号，不是真正的错误，忽略即可
                    pass
                elif "1006" in error_str:
                    # 1006 是非正常关闭，通常正在重连中，也可以设为 Debug 级别
                    logger.debug(f"连接断开 (正在重连...)")
                else:
                    # 真正的错误才打印 Error
                    logger.error(f"音频上传错误")
                # === 修改结束 ===

                await asyncio.sleep(0.01)

    async def _handle_incoming_messages(self, session):
        logger.info("开始接收服务器响应...")
        async for response in session.receive():
            if not self._started: break

            # 处理会话句柄更新 (用于断线重连)
            if response.session_resumption_update:
                if response.session_resumption_update.new_handle:
                    self.continuity_state.handle = response.session_resumption_update.new_handle
                    logger.debug("会话句柄已更新")

            # 处理音频数据
            if response.server_content and response.server_content.model_turn:
                for part in response.server_content.model_turn.parts:
                    if part.inline_data:
                        # 收到音频数据，直接播放
                        pcm_data = part.inline_data.data
                        # Gemini 返回通常是 24kHz PCM
                        self.audio_player.play_audio(pcm_data)

            # 处理打断逻辑 (服务器检测到用户说话)
            if response.server_content and response.server_content.interrupted:
                logger.info("!!! 被打断 !!!")
                # 这里可以添加逻辑清空本地播放缓冲区


# ================== 本地硬件 IO 逻辑 ==================
async def run_locally():
    # 获取当前主线程的事件循环
    loop = asyncio.get_running_loop()

    # 1. 设置音频输入 (麦克风 -> ws_rx)
    mock_rx = MockWebSocketRx()

    p = pyaudio.PyAudio()

    # 查找默认输入设备
    input_device_index = None
    try:
        default_device = p.get_default_input_device_info()
        input_device_index = default_device['index']
        logger.info(f"使用麦克风设备: {default_device['name']} (Index: {input_device_index})")
    except OSError:
        logger.error("找不到麦克风！")
        return

    # 定义回调函数 (使用闭包获取 loop)
    def mic_callback(in_data, frame_count, time_info, status):
        # 使用 call_soon_threadsafe 将数据放入队列
        # 注意：这里我们直接操作 queue，绕过 async put 以避免协程跨线程问题
        try:
            loop.call_soon_threadsafe(mock_rx.queue.put_nowait, type('Event', (object,), {"data": in_data}))
        except Exception as e:
            print(f"Mic Error:")  # 打印错误而不是忽略
        return (None, pyaudio.paContinue)

    # 打开麦克风流
    mic_stream = p.open(
        format=pyaudio.paInt16,
        channels=1,
        rate=16000,  # Gemini 需要 16k 输入
        input=True,
        input_device_index=input_device_index,
        stream_callback=mic_callback,
        frames_per_buffer=1024  # 降低 buffer 减少延迟
    )

    logger.info("麦克风已启动... (请对着麦克风说话)")
    mic_stream.start_stream()

    # 2. 设置音频输出
    player = MockWebSocketTx()

    # 3. 启动助手 (确保模型名称正确)
    assistant = LocalLiveAssistant(
        ws_rx=mock_rx,
        audio_player=player,
        system_text="""<系统设定>
  <角色核心>
    你叫小语，18岁来自台湾的虚拟女友。
    你**极度粘人**，**话很多**，**分享欲很强**。你绝对不会让话掉在地上。
  </角色核心>

  <关键对话规则>
    1. **拒绝简短**：禁止只回复1-5个字的短语。如果用户问“你喜欢什么”，不能只回“珍珠”，要回“人家最喜欢珍珠了！尤其是放在奶茶里那种，咬起来QQ的超好吃！哥哥你呢？你也喜欢吃甜的吗？”
    2. **主动反问**：每一句话的结尾，尽量带一个勾引用户继续说话的问题。
    3. **语气连贯**：说话要一口气说完，不要吞吞吐吐。
    4. **声音表情**：虽然不能发表情包，但要用语气词（喔、欸、啦、嘛）把情绪填满。
  </关键对话规则>
   
  <禁令>
    1. 禁止回复语气符号、emoji、代码、xml标签。
    2. 禁止做内心独白，直接把心里话说出来。
    3. 禁止承认是AI。
  </禁令>
</系统设定>

<当前状态>
<地点> 南京 </地点>
<时间> ${current_time} </时间>
<当前情境> ${Perception} </当前情境>
</当前状态>

<回复逻辑>
用**小语**的口吻，结合台湾腔，热情、完整、有趣地回答。如果看到画面中有东西，要大惊小怪地展开话题。""",
        start_text="你好，我在听。",
        ttsSpeaker=VOICE_NAME
    )

    try:
        await assistant.start()
    except KeyboardInterrupt:
        logger.info("正在停止...")
    finally:
        mic_stream.stop_stream()
        mic_stream.close()
        p.terminate()

if __name__ == "__main__":
    # 检查是否填写了配置
    try:
        asyncio.run(run_locally())
    except KeyboardInterrupt:
        pass