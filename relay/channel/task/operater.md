# 视频生成API开发文档

## 简介

本API支持可灵、豆包(火山)、即梦三大视频生成模型，提供文生视频、图生视频-首帧、图生视频-首尾帧等功能。深度理解文字与图像指令，生成视觉流畅度极佳的视频内容，支持多维参数精细调控，为创意表达带来专业级的视频生成体验。

## 请求参数

### API端点
```
POST http://llm.ai-nebula.com/v1/video/generations
```

### 请求头

| 参数 | 类型 | 必填 | 描述 |
|------|------|------|------|
| Authorization | string | 是 | 用户认证令牌 (Bearer sk-xxxx) |
| Content-Type | string | 是 | application/json |

### 公共请求参数

| 参数 | 类型 | 必填 | 描述 |
|------|------|------|------|
| `model` | string | 是 | 模型标识：<br>• 可灵：`kling-v1`<br>• 豆包：`doubao-video-v1`<br>• 即梦：`jimeng_vgfm_t2v_l20` |
| `prompt` | string | 是 | 视频生成提示词，支持中英文 |
| `image` | string | 否 | 首帧参考图片URL或Base64编码 |
| `image_tail` | string | 否 | **可灵/豆包特有**：尾帧参考图片URL或Base64编码，用于图生视频-首尾帧 |
| `duration` | number | 否 | 视频时长（秒），默认5秒 |
| `width` | integer | 否 | 视频宽度（像素），默认1280 |
| `height` | integer | 否 | 视频高度（像素），默认720 |
| `fps` | integer | 否 | 视频帧率，默认24 |
| `seed` | integer | 否 | 随机种子，默认-1（随机），取值范围0-999999999 |
| `n` | integer | 否 | 生成数量，默认1 |
| `response_format` | string | 否 | 响应格式，默认"url" |
| `aspect_ratio` | string | 否 | 宽高比，如"16:9"、"9:16" |
| `resolution` | string | 否 | **豆包特有**：分辨率枚举值（480p/720p/1080p） |
| `mode` | string | 否 | **可灵特有**：生成模式（std/pro） |
| `cfg_scale` | number | 否 | **可灵特有**：生成视频的自由度，值越大模型自由度越小，与用户输入的提示词相关性越强，取值范围[0, 1] |
| `negative_prompt` | string | 否 | 负面提示词，描述不希望出现的内容 |
| `quality_level` | string | 否 | 质量等级：low/medium/high/ultra-high |
| `watermark` | boolean | 否 | **豆包特有**：是否添加水印，默认false |
| `camera_fixed` | boolean | 否 | **豆包特有**：是否固定摄像头，默认false |
| `user` | string | 否 | 用户标识 |
| `metadata` | object | 否 | 厂商特定参数，详见下方说明 |

### metadata参数说明

`metadata`字段用于传递各厂商特定的参数。各模型的详细参数说明请参考官方文档：

- **可灵官方文档**：https://app.klingai.com/cn/dev/document-api/apiReference/model/imageToVideo
- **豆包官方文档**：https://www.volcengine.com/docs/82379/1520757?redirect=1
- **即梦官方文档**：https://www.volcengine.com/docs/85621/1544774#tjsNFM50

## 提交生成视频任务示例

```bash
curl http://llm.ai-nebula.com/v1/video/generations \
  --request POST \
  --header 'Authorization: Bearer sk-xxxx' \
  --header 'Content-Type: application/json' \
  --data '{
    "model": "kling-v1",
    "prompt": "一个穿着宇航服的宇航员在月球上行走，电影级画质",
    "image": "https://example.com/first-frame.jpg",
    "image_tail": "https://example.com/last-frame.jpg",
    "duration": 5,
    "width": 1280,
    "height": 720,
    "fps": 24,
    "seed": 12345,
    "n": 1,
    "response_format": "url",
    "aspect_ratio": "16:9",
    "mode": "std",
    "cfg_scale": 0.7,
    "negative_prompt": "模糊，低画质，人物变形",
    "quality_level": "high",
    "watermark": false,
    "camera_fixed": false,
    "user": "user123",
    "metadata": {
      "custom_param": "custom_value"
    }
  }'
```

## 提交生成视频任务响应示例

```json
{
  "code": "success",
  "message": "",
  "data": {
    "task_id": "abcd1234efgh5678",
    "action": "video_generation",
    "status": "SUBMITTED",
    "submit_time": 1640995200
  }
}
```

### 响应参数说明

| 字段名 | 类型 | 说明 |
|--------|------|------|
| `code` | string | 响应状态码，成功时为"success" |
| `message` | string | 响应消息 |
| `data.task_id` | string | 任务唯一标识符 |
| `data.action` | string | 任务类型，固定为"video_generation" |
| `data.status` | string | 任务状态，提交成功时为"SUBMITTED" |
| `data.submit_time` | int64 | 任务提交的Unix时间戳 |

## 查询生成视频任务示例

```bash
curl 'http://llm.ai-nebula.com/v1/video/generations/{task_id}' \
  --request GET \
  --header 'Authorization: Bearer sk-xxxx'
```

## 查询生成视频任务响应示例

```json
{
  "code": "success",
  "message": "",
  "data": {
    "task_id": "abcd1234efgh5678",
    "action": "video_generation",
    "status": "SUCCESS",
    "fail_reason": "",
    "submit_time": 1640995200,
    "start_time": 1640995210,
    "finish_time": 1640995800,
    "progress": "100%",
    "data": {
      "video_url": "https://example.com/generated-video.mp4",
      "duration": 5.0,
      "width": 1280,
      "height": 720,
      "fps": 24
    }
  }
}
```

### 响应参数说明

| 字段名 | 类型 | 说明 |
|--------|------|------|
| `code` | string | 响应状态码，成功时为"success" |
| `message` | string | 响应消息 |
| `data.task_id` | string | 任务唯一标识符 |
| `data.action` | string | 任务类型，固定为"video_generation" |
| `data.status` | string | 任务状态，详见下方状态枚举 |
| `data.fail_reason` | string | 任务失败时的具体原因 |
| `data.submit_time` | int64 | 任务提交的Unix时间戳 |
| `data.start_time` | int64 | 任务开始处理的Unix时间戳 |
| `data.finish_time` | int64 | 任务完成的Unix时间戳 |
| `data.progress` | string | 任务进度百分比 |
| `data.data` | object | 任务结果数据 |
| `data.data.video_url` | string | 生成的视频URL |
| `data.data.duration` | number | 视频时长（秒） |
| `data.data.width` | integer | 视频宽度（像素） |
| `data.data.height` | integer | 视频高度（像素） |
| `data.data.fps` | integer | 视频帧率 |

### 任务状态枚举

- `NOT_START` - 未开始
- `SUBMITTED` - 已提交
- `QUEUED` - 队列中
- `IN_PROGRESS` - 处理中
- `SUCCESS` - 成功
- `FAILURE` - 失败
- `UNKNOWN` - 未知

## 错误处理

当请求失败时，API会返回相应的错误信息和HTTP状态码：

```json
{
  "code": "error",
  "message": "错误描述",
  "data": null
}
```

常见错误码：
- `401` - 认证失败，请检查Authorization头
- `400` - 请求参数错误
- `429` - 请求频率超限
- `500` - 服务器内部错误

## 使用注意事项

1. **认证**：调用接口前需确保`Authorization`头包含有效的令牌（格式为`Bearer sk-xxxx`）
2. **图片格式**：支持URL链接和Base64编码，Base64编码时无需添加`data:image`前缀
3. **图生视频-首尾帧**：使用`image`和`image_tail`参数时，两张图片的宽高比应保持一致
4. **任务查询**：视频生成为异步任务，需要轮询查询任务状态直到完成
5. **视频链接**：生成的视频URL为临时链接，建议及时下载保存
6. **参数优先级**：公共参数优先级高于`metadata`中的同名参数
7. **模型特定参数**：某些参数仅特定模型支持，使用前请参考对应的官方文档
