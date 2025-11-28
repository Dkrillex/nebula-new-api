## 概述

Sora 2 是 OpenAI 推出的视频生成模型，支持文本生成视频（T2V）和图生视频（I2V）和视频生视频(V2V,见文档Remix 模式)功能。本文档描述了如何通过 Nebula API 调用 Sora 2 进行视频生成。

## 基础信息

| 项目 | 内容 |
|------|------|
| **Base URL** | `https://llm.ai-nebula.com` |
| **认证方式** | API Key (Token) |
| **请求头** | `Authorization: Bearer sk-xxxx` |
| **Content-Type** | `application/json` |
| **任务模式** | 异步任务（提交任务 → 轮询状态 → 下载结果） |

## 支持的模型

- `sora-2` - Sora 2 标准版

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
| `model` | string | 是 | 模型名称 | `"sora-2"` |
| `prompt` | string | 是 | 视频生成提示词 | `"一只可爱的小猫在花园里玩耍"` |
| `seconds` | string/int | 否 | 视频时长（秒），支持：4、8、12，默认：4 | `"4"` 或 `8` |
| `size` | string | 否 | 视频分辨率，支持：`"720x1280"`（竖屏）或 `"1280x720"`（横屏），注意:只能传尺寸刚好的图片 | `"720x1280"` |
| `width` | int | 否 | 视频宽度（与 `height` 一起使用，会自动转换为 `size`） | `720` |
| `height` | int | 否 | 视频高度（与 `width` 一起使用，会自动转换为 `size`） | `1280` |
| `input_reference` | string | 否 | 参考图片（支持 URL 或 base64 格式） | `"https://example.com/image.jpg"` 或 `"data:image/jpeg;base64,..."` |
| `remix_from_video_id` | string | 否 | Remix 模式：基于已有视频ID进行重新生成（必须以 `video_` 开头） | `"video_12345"` |
| `user` | string | 否 | 用户标识 | `"user-1234"` |

**请求示例 1：基础文本生成视频**

```bash
curl -X POST "https://llm.ai-nebula.com/v1/video/generations" \
  -H "Authorization: Bearer sk-xxxx" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "sora-2",
    "prompt": "一只可爱的小猫在花园里玩耍，阳光明媚，画面温馨",
    "seconds": "4",
    "size": "720x1280"
  }'
```

**请求示例 2：使用图片参考（I2V）**

```bash
curl -X POST "https://llm.ai-nebula.com/v1/video/generations" \
  -H "Authorization: Bearer sk-xxxx" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "sora-2",
    "prompt": "画面从静态图片开始动态变化，小猫慢慢开始移动",
    "input_reference": "https://example.com/cat.jpg",
    "seconds": "8",
    "size": "1280x720"
  }'
```

**请求示例 3：使用 base64 图片**

```bash
curl -X POST "https://llm.ai-nebula.com/v1/video/generations" \
  -H "Authorization: Bearer sk-xxxx" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "sora-2",
    "prompt": "基于这张图片生成视频",
    "input_reference": "data:image/jpeg;base64,/9j/4AAQSkZJRg...",
    "seconds": "12"
  }'
```

**请求示例 4：Remix 模式**

```bash
curl -X POST "https://llm.ai-nebula.com/v1/video/generations" \
  -H "Authorization: Bearer sk-xxxx" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "sora-2",
    "prompt": "修改后的提示词，基于原视频重新生成",
    "remix_from_video_id": "video_1234567890"
  }'
```

**响应示例：**

```json
{
    "format": "mp4",
    "metadata": {
        "completed_at": null,
        "created_at": 1762220876,
        "error": null,
        "expires_at": null,
        "id": "video_69095b4ce0048190893a01510c0c98b0",
        "model": "sora-2",
        "object": "video",
        "progress": 0,
        "remixed_from_video_id": null,
        "seconds": "4",
        "size": "1280x720",
        "status": "queued"
    },
    "status": "submitted",
    "task_id": "video_69095b4ce0048190893a01510c0c98b0"
}

```

**响应字段说明：**

| 字段名 | 类型 | 说明 |
|--------|------|------|
| `task_id` | string | 任务ID，用于后续查询任务状态和下载视频 |
| `status` | string | 任务状态，初始值为 `"submitted"` |

---

### 2. 查询任务状态

**接口地址：** `GET /v1/video/generations/{task_id}`

**请求头：**
```
Authorization: Bearer sk-xxxx
```

**路径参数：**

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| `task_id` | string | 是 | 任务ID（从提交任务的响应中获取） |

**请求示例：**

```bash
curl -X GET "https://llm.ai-nebula.com/v1/video/generations/video_1234567890abcdef" \
  -H "Authorization: Bearer sk-xxxx"
```

**响应示例（排队中）：**

```json
{
  "task_id": "video_1234567890abcdef",
  "id": "video_1234567890abcdef",
  "object": "video_generation",
  "model": "sora-2",
  "status": "queued",
  "progress": 0,
  "created_at": 1704067200,
  "prompt": "一只可爱的小猫在花园里玩耍，阳光明媚，画面温馨",
  "seconds": "4",
  "size": "720x1280"
}
```

**响应示例（处理中）：**

```json
{
  "task_id": "video_1234567890abcdef",
  "id": "video_1234567890abcdef",
  "object": "video_generation",
  "model": "sora-2",
  "status": "in_progress",
  "progress": 50,
  "created_at": 1704067200,
  "prompt": "一只可爱的小猫在花园里玩耍，阳光明媚，画面温馨",
  "seconds": "4",
  "size": "720x1280"
}
```

**响应示例（已完成）：**

```json
{
  "task_id": "video_1234567890abcdef",
  "id": "video_1234567890abcdef",
  "object": "video_generation",
  "model": "sora-2",
  "status": "succeeded",
  "progress": 100,
  "created_at": 1704067200,
  "completed_at": 1704067800,
  "prompt": "一只可爱的小猫在花园里玩耍，阳光明媚，画面温馨",
  "seconds": "4",
  "size": "720x1280",
  "width": 720,
  "height": 1280,
  "n_seconds": 4
}
```

**响应示例（失败）：**

```json
{
  "task_id": "video_1234567890abcdef",
  "id": "video_1234567890abcdef",
  "object": "video_generation",
  "model": "sora-2",
  "status": "failed",
  "progress": 100,
  "created_at": 1704067200,
  "failure_reason": "提示词包含不当内容",
  "error": {
    "message": "提示词包含不当内容",
    "code": "content_policy_violation"
  }
}
```

**任务状态说明：**

| 状态值 | 说明 | 进度 |
|--------|------|------|
| `queued` | 任务已排队，等待处理 | 0-20% |
| `in_progress` | 任务正在处理中 | 20-99% |
| `succeeded` | 任务成功完成 | 100% |
| `failed` | 任务失败 | 100% |

**重要提示：**
- 任务状态为 `queued` 或 `in_progress` 时，需要定期轮询（建议每 3-5 秒查询一次）
- 当状态变为 `succeeded` 时，可以使用任务ID下载视频
- 当状态变为 `failed` 时，可以查看 `failure_reason` 或 `error` 字段了解失败原因

---

### 3. 下载视频

**接口地址：** `GET /v1/video/generations/download?id={task_id}`

**请求头：**
```
Authorization: Bearer sk-xxxx
```

**查询参数：**

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| `id` | string | 是 | 任务ID（必须是已完成的任务） |

**请求示例：**

```bash
curl -X GET "https://llm.ai-nebula.com/v1/video/generations/download?id=video_1234567890abcdef" \
  -H "Authorization: Bearer sk-xxxx"
```

**响应示例：**

```json
{
  "success": true,
  "generation_id": "video_1234567890abcdef",
  "task_id": "video_1234567890abcdef",
  "format": "mp4",
  "size": 15728640,
  "base64": "AAAAIGZ0eXBpc29tAAACAGlzb21pc28yYXZjMW1wNDEAAAAIZnJlZQAAB...",
}
```

**响应字段说明：**

| 字段名 | 类型 | 说明 |
|--------|------|------|
| `success` | boolean | 是否成功 |
| `generation_id` | string | 生成ID（与 task_id 相同） |
| `task_id` | string | 任务ID |
| `format` | string | 视频格式（固定为 `"mp4"`） |
| `size` | number | 视频文件大小（字节） |
| `base64` | string | Base64 编码的视频数据 |

**使用示例：**

**JavaScript / HTML：**
```html
<script>
async function downloadVideo(taskId) {
  const response = await fetch(
    `https://llm.ai-nebula.com/v1/video/generations/download?id=${taskId}`,
    {
      headers: {
        'Authorization': 'Bearer sk-xxxx'
      }
    }
  );
  
  const data = await response.json();
  
  // 方式1：使用 data_url 直接显示
  const videoElement = document.createElement('video');
  videoElement.src = data.data_url;
  videoElement.controls = true;
  document.body.appendChild(videoElement);
  
  // 方式2：下载文件
  const binaryString = atob(data.base64);
  const bytes = new Uint8Array(binaryString.length);
  for (let i = 0; i < binaryString.length; i++) {
    bytes[i] = binaryString.charCodeAt(i);
  }
  const blob = new Blob([bytes], { type: 'video/mp4' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = `${taskId}.mp4`;
  a.click();
}
</script>
```

**Python：**
```python
import requests
import base64

def download_video(task_id, api_key):
    url = f"https://llm.ai-nebula.com/v1/video/generations/download?id={task_id}"
    headers = {
        "Authorization": f"Bearer {api_key}"
    }
    
    response = requests.get(url, headers=headers)
    data = response.json()
    
    if data.get("success"):
        # 保存视频文件
        video_bytes = base64.b64decode(data["base64"])
        with open(f"{task_id}.mp4", "wb") as f:
            f.write(video_bytes)
        print(f"视频已保存: {task_id}.mp4")
    else:
        print("下载失败")

# 使用示例
download_video("video_1234567890abcdef", "sk-xxxx")
```

---

## 完整调用流程

### 流程示例（Python）

```python
import requests
import time

BASE_URL = "https://llm.ai-nebula.com"
API_KEY = "sk-xxxx"

def generate_video(prompt, model="sora-2"):
    """步骤1：提交视频生成任务"""
    url = f"{BASE_URL}/v1/video/generations"
    headers = {
        "Authorization": f"Bearer {API_KEY}",
        "Content-Type": "application/json"
    }
    payload = {
        "model": model,
        "prompt": prompt,
        "seconds": "4",
        "size": "720x1280"
    }
    
    response = requests.post(url, json=payload, headers=headers)
    result = response.json()
    task_id = result["task_id"]
    print(f"任务已提交，Task ID: {task_id}")
    return task_id

def check_task_status(task_id):
    """步骤2：查询任务状态"""
    url = f"{BASE_URL}/v1/video/generations/{task_id}"
    headers = {
        "Authorization": f"Bearer {API_KEY}"
    }
    
    response = requests.get(url, headers=headers)
    return response.json()

def download_video(task_id):
    """步骤3：下载视频"""
    url = f"{BASE_URL}/v1/video/generations/download"
    headers = {
        "Authorization": f"Bearer {API_KEY}"
    }
    params = {"id": task_id}
    
    response = requests.get(url, params=params, headers=headers)
    return response.json()

def main():
    # 1. 提交任务
    task_id = generate_video("一只可爱的小猫在花园里玩耍")
    
    # 2. 轮询任务状态
    max_wait_time = 300  # 最大等待时间（秒）
    start_time = time.time()
    
    while True:
        status_data = check_task_status(task_id)
        status = status_data.get("status")
        
        print(f"当前状态: {status}, 进度: {status_data.get('progress', 0)}%")
        
        if status == "succeeded":
            print("✅ 任务完成！")
            break
        elif status == "failed":
            print(f"❌ 任务失败: {status_data.get('failure_reason', '未知错误')}")
            return
        
        # 检查超时
        if time.time() - start_time > max_wait_time:
            print("⏰ 等待超时")
            return
        
        # 等待3秒后再次查询
        time.sleep(3)
    
    # 3. 下载视频
    print("开始下载视频...")
    video_data = download_video(task_id)
    
    if video_data.get("success"):
        # 保存视频文件
        import base64
        video_bytes = base64.b64decode(video_data["base64"])
        with open(f"{task_id}.mp4", "wb") as f:
            f.write(video_bytes)
        print(f"✅ 视频已保存: {task_id}.mp4")
    else:
        print("❌ 下载失败")

if __name__ == "__main__":
    main()
```

---

## 参数说明

### seconds（视频时长）

| 值 | 说明 |
|---|------|
| `"4"` | 4秒视频（默认值） |
| `"8"` | 8秒视频 |
| `"12"` | 12秒视频 |

**注意：** 仅支持上述三个值，其他值会被自动修正为默认值 `"4"`。

### size（视频分辨率）

| 值 | 说明 | 宽高比 |
|---|------|--------|
| `"720x1280"` | 竖屏（默认值） | 9:16 |
| `"1280x720"` | 横屏 | 16:9 |

**注意：**
- 仅支持上述两个分辨率
- 可以使用 `width` 和 `height` 参数，系统会自动转换为 `size` 参数
- 如果提供的 `width` 和 `height` 不匹配上述分辨率，会自动修正为默认值 `"720x1280"`

### input_reference（参考图片）

支持两种格式：
1. **URL 格式：** `"https://example.com/image.jpg"`
2. **Base64 格式：** `"data:image/jpeg;base64,/9j/4AAQSkZJRg..."`

**支持的图片格式：** JPEG, PNG

### remix_from_video_id（Remix 模式）

- 必须是已完成任务的 `task_id`
- 必须以 `video_` 开头
- 使用 Remix 模式时，只需提供 `prompt` 参数，其他参数会被忽略

---

## 错误处理

### 常见错误码

| HTTP状态码 | 错误信息 | 说明 |
|-----------|---------|------|
| 400 | `invalid_request_error` | 请求参数错误 |
| 401 | `authentication_error` | API Key 无效或未提供 |
| 403 | `permission_denied` | 无权限访问 |
| 404 | `not_found` | 任务不存在 |
| 429 | `rate_limit_error` | 请求频率过高 |
| 500 | `server_error` | 服务器内部错误 |

### 错误响应示例

```json
{
  "error": {
    "message": "Invalid API key",
    "type": "authentication_error",
    "code": "invalid_api_key"
  }
}
```

---

## 最佳实践

1. **轮询间隔：** 建议每 3-5 秒查询一次任务状态，避免过于频繁的请求
2. **超时处理：** 设置合理的超时时间（建议 5-10 分钟），避免无限等待
3. **错误重试：** 对于网络错误或临时错误，建议实现指数退避重试机制
4. **资源清理：** 下载完成后及时清理 base64 数据，避免内存占用过大
5. **异步处理：** 在服务端应用中，建议使用异步任务队列处理视频生成请求

---

## 注意事项

1. ⚠️ **配额消耗：** 视频生成会消耗大量配额,目前为1s/0.1$，请确保账户有足够的余额
2. ⚠️ **任务保存：** 生成的任务id会保留24小时，请及时下载结果
3. ⚠️ **提示词限制：** 提示词不能包含不当内容，否则任务会失败
4. ⚠️ **并发限制：** 单个账户可能存在并发任务数量限制
5. ⚠️ **文件大小：** 下载的视频文件可能较大，请注意网络带宽和存储空间

---


**文档版本：** v1.0  
**最后更新：** 2025-11-03

