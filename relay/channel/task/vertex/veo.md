## 概述

Veo 系列是 Google Vertex AI 推出的多模态视频生成模型，支持文本生成视频（T2V）以及在首帧、尾帧的约束下生成连贯视频。本文档说明如何通过 Nebula API 调用 Veo 进行视频生成。

## 基础信息

| 项目 | 内容 |
|------|------|
| **Base URL** | `https://llm.ai-nebula.com` |
| **认证方式** | API Key (Token) |
| **请求头** | `Authorization: Bearer sk-xxxx` / `Content-Type: application/json` |
| **任务模式** | 异步任务（提交任务 → 轮询状态 → 获取结果） |

## 支持的模型

- `veo-3.1-generate-preview`
- `veo-3.1-fast-generate-preview`
- `veo-3.0-generate-preview`
- `veo-3.0-fast-generate-001`

> 其他 Veo 模型如需开放，请联系平台同步渠道与映射。

## API 接口

### 1. 提交视频生成任务

**接口地址：** `POST /v1/video/generations`

**请求头：**
```
Authorization: Bearer sk-xxxx
Content-Type: application/json
```

**请求参数：**

| 参数名 | 类型 | 必填 | 说明 | 示例值 |
|--------|------|------|------|--------|
| `model` | string | 是 | 模型名称 | `"veo-3.0-generate-001"` |
| `prompt` | string | 是 | 视频生成提示词 | `"清晨的科幻城市航拍镜头"` |
| `durationSeconds` / `duration_seconds` | int | 否 | 视频时长，仅支持 `4` / `6` / `8` 秒，默认 `4`。 | `8` |
| `aspectRatio` / `aspect_ratio` | string | 否 | 宽高比，仅支持 `16:9` 或 `9:16`，默认 `16:9`。 | `"16:9"` |
| `resolution` | string | 否 | 分辨率选项，`720p` 或 `1080p`，默认 `1080p`。 | `"1080p"` |
| `fps` | int | 否 | 帧率，默认 24，可按需覆盖。 | `24` |
| `image` | string | 否 | 首帧图片，用于 I2V/StoryBoard，支持 HTTP(S) URL 或 `data:image/...;base64,...`。 | `"https://example.com/frame0.png"` |
| `lastFrame` / `last_frame` | string | 否 | 尾帧图片约束，同上。 | `"data:image/png;base64,iVBORw0KGgo..."` |
| `generateAudio` / `generate_audio` | bool | 否 | 是否生成同步音频。Nebula 会依据该参数切换 $0.2（无音频）或 $0.4（含音频）的按秒计费；快速版模型会忽略该参数并始终包含音频。 | `true` |
| `personGeneration` / `person_generation` | string | 否 | 人像生成策略：`allow_all`（所有年龄，默认）/ `allow_adult`（成年人）/ `dont_allow`（禁止人像）。 | `"allow_adult"` |
| `addWatermark` / `add_watermark` | bool | 否 | 是否在成品中加入水印，默认 `false`。 | `true` |
| `seed` | int | 否 | 随机种子；未传则由上游自动分配。 | `12345` |
| `metadata` | object | 否 | 自定义参数，若已提供上述字段，系统会自动合并进 metadata。 | `{}` |

> 默认帧率为 24，如需其他值可传入 `fps` 参数。
> 快速版模型（`veo-3.0-fast-generate-001`、`veo-3.1-fast-generate-preview`）默认附带音频并忽略 `generateAudio` 参数，Nebula 会自动按含音频档位计费。

| `user` | string | 否 | 自定义用户标识（原样回传）。 | `"user-1234"` |

> Nebula 会自动提取 `durationSeconds`、`aspectRatio`、`resolution`、`image`、`lastFrame`、`generateAudio`、`personGeneration`、`addWatermark`、`seed` 并转写为 Vertex 所需的 Metadata；URL 图片会被下载后按需转为 Base64。

**请求示例（文本生成视频 + 首帧约束）：**

```bash
curl -X POST "https://llm.ai-nebula.com/v1/video/generations" \
  -H "Authorization: Bearer sk-xxxx" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "veo-3.0-generate-001",
    "prompt": "清晨阳光洒在赛博城市的高楼群，镜头慢慢推进",
    "durationSeconds": 8,
    "aspectRatio": "16:9",
    "resolution": "1080p",
    "image": "https://example.com/storyboard.png",
    "generateAudio": true,
    "personGeneration": "allow_all",
    "addWatermark": false
  }'
```

**提交响应示例：**

```json
{
  "task_id": "cHJvamVjdHMvZXhhbXBsZS9sb2NhdGlvbnMvdXMtY2VudHJhbDEvcHVibGlzaGVycy9nb29nbGUvbW9kZWxzL3Zlby0zLjAtZ2VuZXJhdGUtMDAxL29wZXJhdGlvbnMvMTIzNDU2Nzg5MA"
}
```

- `task_id` 是经过编码的 Vertex Operation 名称，后续查询与下载均需使用该值。

### 2. 查询任务状态

**接口地址：** `GET /v1/video/generations/{task_id}`

**请求头：**
```
Authorization: Bearer sk-xxxx
```

**响应字段：**

| 字段名 | 类型 | 说明 |
|--------|------|------|
| `task_id` | string | 任务ID（与提交返回一致） |
| `status` | string | 任务状态：`submitted` → `queued` → `in_progress` → `succeeded` / `failed` |
| `url` | string | 成功时返回 `data:video/mp4;base64,...` 格式的视频数据 URI（仅 Veo 使用 `fail_reason` 持久化） |
| `format` | string | 视频格式，默认 `mp4` |
| `metadata` | object | Vertex 原始响应，包含 `response.videos[0]...` 等字段，内部 Base64 会被截断显示 |
| `error` | object | 失败时的错误信息（含 `code`、`message`） |

**查询示例：**

```bash
curl -X GET "https://llm.ai-nebula.com/v1/video/generations/cHJvamVjdHMv..." \
  -H "Authorization: Bearer sk-xxxx"
```

**响应示例（处理中）：**

```json
{
  "task_id": "cHJvamVjdHMv...",
  "status": "in_progress",
  "format": "mp4",
  "metadata": {
    "response": {
      "@type": "type.googleapis.com/google.cloud.aiplatform.v1.types.VideoGenerationPredictionResult",
      "videos": []
    }
  }
}
```

**响应示例（已完成）：**

```json
{
  "task_id": "cHJvamVjdHMv...",
  "status": "succeeded",
  "format": "mp4",
  "url": "data:video/mp4;base64,AAAAIGZ0eXBpc29tAAACAGlzb20...",
  "metadata": {
    "response": {
      "videos": [
        {
          "mimeType": "video/mp4",
          "encoding": "mp4",
          "bytesBase64Encoded": "AAAAIGZ0eXBpc29tAAACAGlz..." // 已截断
        }
      ],
      "raiMediaFilteredCount": 0
    }
  }
}
```

> Veo 任务完成后，`url` 字段即为完整的数据 URI，可直接用于前端 `<video>` 播放或转存文件。

**响应示例（失败）：**

```json
{
  "task_id": "cHJvamVjdHMv...",
  "status": "failed",
  "error": {
    "code": 400,
    "message": "Prompt violates policy"
  }
}
```

### 3. 下载视频

Veo 的视频结果已经以内嵌 Base64（Data URL）形式写入 `url`。当前的 `/v1/video/generations/download` 接口主要面向 Sora 等需要二次拉取上游文件的模型，对 Veo 暂无直接作用。

- **推荐做法：** 使用状态查询接口返回的 `url` 字段，前端可直接设置为 `<video src="data:video/mp4;base64,...">` 或在后端解析 Base64 保存为 `*.mp4`。
- **注意：** 任务清理线程会在有效期后将 `url` 从 `fail_reason` 中清理为 `"expired"`，届时需重新生成视频。

如果希望统一实现下载逻辑，可在业务侧调用状态接口并手动持久化 Base64 数据，再对外提供下载服务。

## 元数据说明

- `metadata.response`：Vertex 原始返回，包含 `videos` 数组（通常只返回一个元素）、`encoding`、`mimeType` 等。
- `metadata.response.videos[0].bytesBase64Encoded`：Nebula 会截断展示，避免响应过大；完整内容请使用 `url`。
- `metadata.response.raiMediaFilteredCount`：可能的内容安全过滤计数，>0 表示部分帧被过滤。
- 请求期间传入的 `durationSeconds`、`aspectRatio`、`resolution`、`fps`、`image`、`lastFrame`、`generateAudio`、`personGeneration`、`addWatermark`、`seed` 会被写入任务 `metadata`，可在数据库或后端日志中查看。

## 计费与配额

- Nebula 根据请求中记录的 `requested_seconds` （默认 4 秒，如显式传入会覆盖）进行按秒扣费，并结合 `generateAudio` 字段选择对应的秒价。
- 价格来源于后台 `VideoModelPricePerSecond` / `OriginVideoModelPricePerSecond` 配置，示例：
  ```json
  {
    "veo-3.1-generate-preview": {
      "noAudio": 0.2,
      "audio": 0.4
    }
  }
  ```
  若仅配置单一数值，则无论是否生成音频都会使用该价格；缺省时 Nebula 会回退到内置默认值。
- 当前参考价：
  - `veo-3.0-generate-001`：$0.15 / 秒
  - `veo-3.0/3.1-generate-preview`：不含音频 `$0.2 / 秒`，含音频 `$0.4 / 秒`
  - 其他 Veo 3.x 预览模型与上游价格保持一致，实际收费以平台配置为准。
- 任务提交阶段仅做余额校验，不会预扣费；扣费在任务完成后由轮询线程执行，日志中会记录 `generate_audio` 与最终单价。

## 视频有效期与清理策略

- 环境变量 `VIDEO_RETENTION_HOURS`（默认 12）控制 Veo 成功任务中数据 URI 的保留时长。
- 环境变量 `VIDEO_CLEAN_INTERVAL_MINUTES`（默认 30）控制清理线程扫描间隔。
- 超过保留期的任务，其 `fail_reason` 会被置为 `"expired"`，状态接口的 `url` 也将不可用；如需长期保存，请在有效期内下载。

## 最佳实践

- **轮询频率：** 建议每 3–5 秒查询一次状态，避免触发 Google Vertex 的频控。
- **超时控制：** 单个任务建议设置 5–10 分钟超时，避免无限等待。
- **错误重试：** 对 `429`、网络超时等临时错误使用指数退避重试；业务错误需更换提示词或参数。
- **资源管理：** Base64 视频体积较大，建议后端拿到 `url` 后立即转存为文件并释放内存。
- **帧约束：** 若同时提供 `image` 与 `lastFrame`，请确保与 `aspectRatio`/`resolution` 匹配，避免上游裁剪失真。
- **版本升级：** 如需接入更新的 Veo 3.1+ 模型，需同步更新渠道凭证、价格与文档。

