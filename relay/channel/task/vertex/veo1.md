# Nebula Veo 视频生成快速上手

本文档面向 API 使用方，介绍如何通过 Nebula 统一接口调用 Google Vertex Veo 模型生成视频。内容涵盖请求格式、参数说明以及常见示例。

支持的模型：`veo-3.1-generate-preview`、`veo-3.1-fast-generate-preview`、`veo-3.0-generate-preview`、`veo-3.0-fast-generate-001`。

## 1. 基本信息

| 项目 | 内容 |
|------|------|
| **Base URL** | `https://llm.ai-nebula.com` |
| **接口路径** | `POST /v1/video/generations` |
| **认证方式** | `Authorization: Bearer <API Key>` |
| **Content-Type** | `application/json` |
| **任务模式** | 异步：提交任务 → 轮询状态 → 获取结果 |

## 2. 请求参数

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `model` | string | 是 | 模型名称，常用：`veo-3.1-generate-preview`、`veo-3.1-fast-generate-preview`、`veo-3.0-generate-preview`、`veo-3.0-fast-generate-001`。|
| `prompt` | string | 是 | 文本提示词，描述需要生成的视频内容。|
| `durationSeconds` | int | 否 | 视频时长（秒），支持 `4` / `6` / `8` / `12`，默认 `4`。|
| `aspectRatio` | string | 否 | 宽高比，仅支持 `16:9` 或 `9:16`，默认 `16:9`。|
| `resolution` | string | 否 | 分辨率选项，`720p` 或 `1080p`，默认 `1080p`。|
| `fps` | int | 否 | 帧率，默认 24，可按需覆盖。|
| `sampleCount` | int | 否 | 每次生成的视频数量，范围 `1-4`，默认 `1`。|
| `generateAudio` | bool | 否 | 是否生成同步音频，默认 `false`；快速版模型会忽略该参数并始终输出含音频视频。|
| `personGeneration` | string | 否 | 人像生成策略：`allow_all`（所有年龄）/ `allow_adult`（成年人）/ `dont_allow`（禁止），默认 `allow_all`。|
| `addWatermark` | bool | 否 | 是否添加水印，默认 `false`。|
| `seed` | int | 否 | 随机种子，固定后可获得可重复结果。|
| `image` | string | 否 | 首帧参考图，支持 HTTP(S) URL 或 Base64 Data URI。|
| `lastFrame` | string | 否 | 尾帧参考图，支持 HTTP(S) URL 或 Base64 Data URI。|

> **提示**
>
> - 若传入 `image`，Veo 会将首帧作为起始画面进行过渡；同时提供 `lastFrame` 可实现首尾帧约束。
> - Nebula 会自动把参数写入 `metadata` 并转成 Vertex 所需格式，无需手动处理。

## 3. 提交示例

### 3.1 Curl
```bash
curl -X POST "https://llm.ai-nebula.com/v1/video/generations" \
  -H "Authorization: Bearer sk-xxxx" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "veo-3.1-generate-preview",
    "prompt": "科幻城市的清晨航拍，阳光穿过云层",
    "durationSeconds": 8,
    "aspectRatio": "16:9",
    "resolution": "1080p",
    "generateAudio": true,
    "image": "https://example.com/start-frame.png",
    "lastFrame": "https://example.com/end-frame.png"
  }'
```

### 3.2 Python
```python
import requests

url = "https://llm.ai-nebula.com/v1/video/generations"
headers = {
    "Authorization": "Bearer sk-xxxx",
    "Content-Type": "application/json",
}
payload = {
    "model": "veo-3.0-generate-001",
    "prompt": "一只飞翔的海鸥贴近海面，镜头缓慢跟随",
    "durationSeconds": 6,
    "aspectRatio": "9:16",
    "resolution": "720p",
    "generateAudio": False
}
resp = requests.post(url, json=payload, headers=headers)
print(resp.json())
```

## 4. 查询任务

- 接口：`GET /v1/video/generations/{task_id}`
- 返回示例（成功，`sampleCount = 4`）：
```json
{
  "task_id": "cHJvamVjdHMv...",
  "status": "succeeded",
  "format": "mp4",
  "url": [
    "https://nebula-ads.oss-cn-guangzhou.aliyuncs.com/2025/11/18/abc123/veo-demo-1.mp4",
    "https://nebula-ads.oss-cn-guangzhou.aliyuncs.com/2025/11/18/abc123/veo-demo-2.mp4",
    "https://nebula-ads.oss-cn-guangzhou.aliyuncs.com/2025/11/18/abc123/veo-demo-3.mp4",
    "https://nebula-ads.oss-cn-guangzhou.aliyuncs.com/2025/11/18/abc123/veo-demo-4.mp4"
  ],
  "metadata": {
    "durationSeconds": 12,
    "aspectRatio": "9:16",
    "resolution": "1080p",
    "fps": 24,
    "generateAudio": true,
    "response": {
      "videos": [
        {
          "mimeType": "video/mp4",
          "encoding": "mp4",
          "url": "https://nebula-ads.oss-cn-guangzhou.aliyuncs.com/2025/11/18/abc123/veo-demo-1.mp4"
        },
        {
          "mimeType": "video/mp4",
          "encoding": "mp4",
          "url": "https://nebula-ads.oss-cn-guangzhou.aliyuncs.com/2025/11/18/abc123/veo-demo-2.mp4"
        },
        {
          "mimeType": "video/mp4",
          "encoding": "mp4",
          "url": "https://nebula-ads.oss-cn-guangzhou.aliyuncs.com/2025/11/18/abc123/veo-demo-3.mp4"
        },
        {
          "mimeType": "video/mp4",
          "encoding": "mp4",
          "url": "https://nebula-ads.oss-cn-guangzhou.aliyuncs.com/2025/11/18/abc123/veo-demo-4.mp4"
        }
      ]
    }
  }
}
```
- `url` 字段：当只生成 1 个视频时为字符串，多视频时返回字符串数组。
 - `sampleCount = 1`：`"url": "https://.../veo-demo-1.mp4"`
 - `sampleCount > 1`：`"url": ["https://.../veo-demo-1.mp4", "https://.../veo-demo-2.mp4", ...]`
- `metadata.response.videos` 数组中追加了 `url` 字段，可直接获取每段视频的最终地址。
- 其他字段（`durationSeconds`、`aspectRatio`、`generateAudio` 等）会回显调用时的参数，便于追踪。

## 5. 常见输入组合

| 场景 | 必需字段 | 可选字段 |
|------|----------|----------|
| 纯文本生成 | `model`, `prompt` | `durationSeconds`, `aspectRatio`, `resolution`, `generateAudio` |
| 首帧参考 | `model`, `prompt`, `image` | 其他字段同上 |
| 首尾帧约束 | `model`, `prompt`, `image`, `lastFrame` | 同上 |
| 高分辨率生成 | `model`, `prompt`, `resolution`, `aspectRatio` | `fps`, `generateAudio` |

## 6. 响应状态说明

| 状态 | 说明 |
|------|------|
| `submitted` / `queued` | 任务已排队，等待开始。|
| `in_progress` | 任务正在处理，可重复查询。|
| `succeeded` | 生成成功，`url` 为最终视频地址（字符串或字符串数组）。|
| `failed` | 任务失败，`error` 字段提供错误码与信息。|

## 7. 小贴士

- 建议每 3~5 秒轮询一次任务状态，避免触发频控。
- 如果 `url` 为字符串数组，请根据需求逐个下载；单个字符串可直接用于 `<video>` 播放或落盘保存。
- 费用按「视频时长 × sampleCount」计算；开启音频会自动使用含音频价格。
- 若需生成音频，请将 `generateAudio` 置为 `true`，系统会自动选择含音频的计费档位。
- 默认帧率为 24，必要时可传入 `fps` 参数覆盖。
- 若返回 `failed` 且 `message` 提示策略问题，可尝试修改提示词或降低时长/分辨率后重试。
- 快速版模型（`veo-3.0-fast-generate-001`、`veo-3.1-fast-generate-preview`）默认自带音频并忽略 `generateAudio` 参数，可直接按含音频场景调用。
