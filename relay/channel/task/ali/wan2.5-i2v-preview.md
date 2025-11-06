## 概述

wan2.5-i2v-preview 是一款图生视频模型，支持从静态图片生成高质量动态视频。本文档描述了如何通过 Nebula API 调用该模型进行视频生成。

**核心特性：**
- 🎬 图片生成视频（Image-to-Video）
- 🎵 支持自定义音频上传
- ⏱️ 支持 5秒/10秒 视频生成
- 📺 支持 480P/720P/1080P 多种分辨率
- 🤖 智能提示词扩写
- 🎼 自动生成画面同步音频

**官方文档：** [阿里云通义万相 API 文档](https://bailian.console.aliyun.com/?tab=api#/api/?type=model&url=2867393)

## 基础信息

| 项目 | 内容 |
|------|------|
| **Base URL** | `https://llm.ai-nebula.com` |
| **认证方式** | API Key (Token) |
| **请求头** | `Authorization: Bearer sk-xxxx` |
| **Content-Type** | `application/json` |
| **任务模式** | 异步任务（提交任务 → 轮询状态） |

## 支持的模型

- `wan2.5-i2v-preview` - 图生视频预览版

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
| `model` | string | 是 | 模型名称 | `"wan2.5-i2v-preview"` |
| `prompt` | string | 是 | 视频生成提示词（描述画面动作和场景） | `"小猫缓缓睁开眼睛，耳朵轻轻抖动"` |
| `image` | string | 是 | 输入图片（支持 HTTPS URL 或 base64 格式） | `"https://example.com/cat.jpg"` |
| `duration` | int | 否 | 视频时长（秒），支持：5、10，默认：5 | `5` |
| `resolution` | string | 否 | 视频分辨率，支持：`"480p"`、`"720p"`、`"1080p"`，默认：`"720p"` | `"720p"` |
| `smart_rewrite` | bool | 否 | 是否启用智能提示词扩写，默认：false | `true` |
| `generate_audio` | bool | 否 | 是否生成与画面同步的音频，默认：false | `true` |
| `audio_url` | string | 否 | 自定义音频文件URL（HTTPS格式） | `"https://example.com/audio.mp3"` |
| `seed` | int | 否 | 随机种子（用于复现结果），范围：0-2147483647 | `12345` |

**请求示例 1：基础图生视频（5秒，720p）**

```bash
curl -X POST "https://llm.ai-nebula.com/v1/video/generations" \
  -H "Authorization: Bearer sk-xxxx" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "wan2.5-i2v-preview",
    "prompt": "小猫缓缓睁开眼睛，耳朵轻轻抖动，镜头缓推拉近",
    "image": "https://example.com/cat.jpg",
    "duration": 5,
    "resolution": "720p"
  }'
```

**请求示例 2：生成带音频的长视频（10秒，1080p）**

```bash
curl -X POST "https://llm.ai-nebula.com/v1/video/generations" \
  -H "Authorization: Bearer sk-xxxx" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "wan2.5-i2v-preview",
    "prompt": "落日余晖洒在湖面，波光粼粼，微风拂过芦苇",
    "image": "https://example.com/sunset.jpg",
    "duration": 10,
    "resolution": "1080p",
    "generate_audio": true,
    "smart_rewrite": true
  }'
```

**请求示例 3：使用 base64 图片和自定义音频**

```bash
curl -X POST "https://llm.ai-nebula.com/v1/video/generations" \
  -H "Authorization: Bearer sk-xxxx" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "wan2.5-i2v-preview",
    "prompt": "音乐节奏带动画面节奏，人群欢呼跳跃",
    "image": "data:image/jpeg;base64,/9j/4AAQSkZJRg...",
    "audio_url": "https://example.com/background-music.mp3",
    "duration": 5,
    "resolution": "720p"
  }'
```

**响应示例：**

```json
{
  "task_id": "ae8eb420-8aa6-440a-8eb8-1d1afe8d5e97",
  "status": "submitted",
  "format": "mp4",
  "metadata": {
    "model_price": 0.0738,
    "requested_seconds": 5,
    "billing_pending": true,
    "group_ratio": 1,
    "token_name": "nebula-video-generations-default",
    "output": {
      "task_id": "ae8eb420-8aa6-440a-8eb8-1d1afe8d5e97",
      "task_status": "PENDING"
    },
    "request_id": "b356ad62-d35f-4f4b-9a6d-0787f6966a68"
  }
}
```

**响应字段说明：**

| 字段名 | 类型 | 说明 |
|--------|------|------|
| `task_id` | string | 任务ID，用于后续查询任务状态 |
| `status` | string | 任务状态，初始值为 `"submitted"` |
| `format` | string | 视频格式，固定为 `"mp4"` |
| `metadata` | object | 元数据信息（计费、原始响应等） |

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
curl -X GET "https://llm.ai-nebula.com/v1/video/generations/ae8eb420-8aa6-440a-8eb8-1d1afe8d5e97" \
  -H "Authorization: Bearer sk-xxxx"
```

**响应示例（排队中）：**

```json
{
  "task_id": "ae8eb420-8aa6-440a-8eb8-1d1afe8d5e97",
  "status": "in_progress",
  "format": "mp4",
  "metadata": {
    "model_price": 0.0738,
    "billing_pending": true,
    "output": {
      "task_status": "PENDING"
    }
  }
}
```

**响应示例（处理中）：**

```json
{
  "task_id": "ae8eb420-8aa6-440a-8eb8-1d1afe8d5e97",
  "status": "in_progress",
  "format": "mp4",
  "metadata": {
    "output": {
      "task_status": "RUNNING"
    }
  }
}
```

**响应示例（已完成）：**

```json
{
  "task_id": "ae8eb420-8aa6-440a-8eb8-1d1afe8d5e97",
  "status": "succeeded",
  "format": "mp4",
  "url": "https://dashscope-result-sh.oss-cn-shanghai.aliyuncs.com/.../video.mp4?Expires=...",
  "metadata": {
    "model_price": 0.0738,
    "billing_pending": false,
    "group_ratio": 1,
    "usage": {
      "video_count": 1,
      "duration": 5,
      "resolution": "720p"
    },
    "output": {
      "task_status": "SUCCEEDED",
      "video_url": "https://dashscope-result-sh.oss-cn-shanghai.aliyuncs.com/.../video.mp4",
      "submit_time": "2025-11-04 19:15:11.819",
      "end_time": "2025-11-04 19:16:40.291"
    },
    "request_id": "35549d8f-58e7-4c5c-a827-39a2f2d58a2d"
  }
}
```

**响应示例（失败）：**

```json
{
  "task_id": "ae8eb420-8aa6-440a-8eb8-1d1afe8d5e97",
  "status": "failed",
  "format": "mp4",
  "metadata": {
    "output": {
      "task_status": "FAILED",
      "code": "InvalidParameter.ImageFormat",
      "message": "图片格式不支持或格式错误"
    }
  }
}
```

**任务状态说明：**

| 状态值 | 说明 | 进度 |
|--------|------|------|
| `submitted` | 任务已提交 | 初始状态 |
| `in_progress` | 任务处理中（包括排队和执行） | 进行中 |
| `succeeded` | 任务成功完成 | 完成 |
| `failed` | 任务失败 | 完成 |

**重要提示：**
- 任务状态为 `in_progress` 时，需要定期轮询（建议每 3-5 秒查询一次）
- 当状态变为 `succeeded` 时，`url` 字段包含视频下载地址（带签名，有效期24小时）
- 当状态变为 `failed` 时，可查看 `metadata.output.message` 了解失败原因
- **不需要额外的下载接口**，视频URL直接在查询结果中返回

---

## 完整调用流程

### 流程示例（Python）

```python
import requests
import time

BASE_URL = "https://llm.ai-nebula.com"
API_KEY = "sk-xxxx"

def generate_video(prompt, image_url, duration=5, resolution="720p"):
    """步骤1：提交视频生成任务"""
    url = f"{BASE_URL}/v1/video/generations"
    headers = {
        "Authorization": f"Bearer {API_KEY}",
        "Content-Type": "application/json"
    }
    payload = {
        "model": "wan2.5-i2v-preview",
        "prompt": prompt,
        "image": image_url,
        "duration": duration,
        "resolution": resolution,
        "smart_rewrite": True,
        "generate_audio": True
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

def main():
    # 1. 提交任务
    task_id = generate_video(
        prompt="小猫缓缓睁开眼睛，耳朵轻轻抖动",
        image_url="https://example.com/cat.jpg",
        duration=5,
        resolution="720p"
    )
    
    # 2. 轮询任务状态
    max_wait_time = 600  # 最大等待时间（10分钟）
    start_time = time.time()
    
    while True:
        status_data = check_task_status(task_id)
        status = status_data.get("status")
        
        print(f"当前状态: {status}")
        
        if status == "succeeded":
            # 任务成功，获取视频URL
            video_url = status_data.get("url")
            print(f"✅ 任务完成！视频URL: {video_url}")
            
            # 可以直接使用URL在网页中播放，或下载视频
            # download_video(video_url, f"{task_id}.mp4")
            break
            
        elif status == "failed":
            error_msg = status_data.get("metadata", {}).get("output", {}).get("message", "未知错误")
            print(f"❌ 任务失败: {error_msg}")
            return
        
        # 检查超时
        if time.time() - start_time > max_wait_time:
            print("⏰ 等待超时")
            return
        
        # 等待5秒后再次查询
        time.sleep(5)

def download_video(video_url, save_path):
    """下载视频到本地（可选）"""
    response = requests.get(video_url, stream=True)
    with open(save_path, 'wb') as f:
        for chunk in response.iter_content(chunk_size=8192):
            f.write(chunk)
    print(f"视频已保存到: {save_path}")

if __name__ == "__main__":
    main()
```

### 流程示例（JavaScript）

```javascript
const BASE_URL = 'https://llm.ai-nebula.com';
const API_KEY = 'sk-xxxx';

async function generateVideo(prompt, imageUrl, duration = 5, resolution = '720p') {
  // 步骤1：提交任务
  const response = await fetch(`${BASE_URL}/v1/video/generations`, {
    method: 'POST',
    headers: {
      'Authorization': `Bearer ${API_KEY}`,
      'Content-Type': 'application/json'
    },
    body: JSON.stringify({
      model: 'wan2.5-i2v-preview',
      prompt,
      image: imageUrl,
      duration,
      resolution,
      smart_rewrite: true,
      generate_audio: true
    })
  });
  
  const result = await response.json();
  console.log(`任务已提交，Task ID: ${result.task_id}`);
  return result.task_id;
}

async function checkTaskStatus(taskId) {
  // 步骤2：查询任务状态
  const response = await fetch(`${BASE_URL}/v1/video/generations/${taskId}`, {
    headers: {
      'Authorization': `Bearer ${API_KEY}`
    }
  });
  
  return await response.json();
}

async function pollTaskUntilComplete(taskId, maxWaitTime = 600000) {
  const startTime = Date.now();
  
  while (true) {
    const statusData = await checkTaskStatus(taskId);
    const status = statusData.status;
    
    console.log(`当前状态: ${status}`);
    
    if (status === 'succeeded') {
      const videoUrl = statusData.url;
      console.log(`✅ 任务完成！视频URL: ${videoUrl}`);
      return videoUrl;
    } else if (status === 'failed') {
      const errorMsg = statusData.metadata?.output?.message || '未知错误';
      console.error(`❌ 任务失败: ${errorMsg}`);
      throw new Error(errorMsg);
    }
    
    // 检查超时
    if (Date.now() - startTime > maxWaitTime) {
      throw new Error('等待超时');
    }
    
    // 等待5秒后再次查询
    await new Promise(resolve => setTimeout(resolve, 5000));
  }
}

// 使用示例
async function main() {
  try {
    const taskId = await generateVideo(
      '小猫缓缓睁开眼睛，耳朵轻轻抖动',
      'https://example.com/cat.jpg',
      5,
      '720p'
    );
    
    const videoUrl = await pollTaskUntilComplete(taskId);
    
    // 在网页中显示视频
    const videoElement = document.createElement('video');
    videoElement.src = videoUrl;
    videoElement.controls = true;
    document.body.appendChild(videoElement);
  } catch (error) {
    console.error('错误:', error.message);
  }
}

main();
```

---

## 参数详细说明

### duration（视频时长）

| 值 | 说明 | 计费 |
|---|------|------|
| `5` | 5秒视频（推荐，默认值） | 按5秒计费 |
| `10` | 10秒视频 | 按10秒计费 |

**注意：** 仅支持 5 和 10 秒，其他值会被自动修正为默认值 `5`。

### resolution（视频分辨率）

| 值 | 分辨率 | 价格（美元/秒） | 5秒费用 | 10秒费用 |
|---|--------|----------------|---------|----------|
| `"480p"` | 480P | $0.0369 | $0.1845 | $0.369 |
| `"720p"` | 720P（默认） | $0.0738 | $0.369 | $0.738 |
| `"1080p"` | 1080P | $0.1233 | $0.6165 | $1.233 |

**注意：** 分辨率越高，视频质量越好，但费用也越高。

### image（输入图片）

支持三种格式：
1. **HTTPS URL 格式（推荐）：** `"https://example.com/image.jpg"`
2. **HTTP URL 格式：** `"http://example.com/image.jpg"`
3. **Base64 格式（不推荐，可能触发大小限制）：** `"data:image/jpeg;base64,/9j/4AAQSkZJRg..."`

**支持的图片格式：** JPEG, PNG  
**建议图片尺寸：** 与目标分辨率匹配或相近  
**最佳实践：** 使用 HTTPS URL 可避免内容审核长度限制

### audio_url（自定义音频）

- 必须是可公开访问的 HTTPS URL
- 支持格式：MP3, WAV
- 建议时长：与视频时长一致
- 文件大小：不超过 15MB

### smart_rewrite（智能提示词扩写）

| 值 | 说明 |
|---|------|
| `true` | 启用智能扩写，系统会自动优化和丰富提示词，提升视频质量 |
| `false` | 不启用（默认），严格按照原始提示词生成 |

### generate_audio（生成音频）

| 值 | 说明 |
|---|------|
| `true` | 自动生成与画面同步的音频（如环境音、动作音效） |
| `false` | 不生成音频（默认） |

**注意：** 如果同时提供了 `audio_url`，`generate_audio` 会被忽略。

### seed（随机种子）

- 范围：0 - 2147483647
- 用途：使用相同的 seed、prompt 和 image 可以生成相似的视频（但不完全相同）
- 不指定时，系统会随机生成

---

## 计费说明

### 计费规则

视频生成按 **分辨率 × 时长** 计费：

```
费用 = 分辨率价格（美元/秒） × 视频时长（秒） × 组倍率
```

### 费用示例

| 分辨率 | 时长 | 单价 | 总费用 | 约合人民币 |
|-------|------|------|--------|-----------|
| 480P | 5秒 | $0.0369/秒 | $0.1845 | ¥1.33 |
| 720P | 5秒 | $0.0738/秒 | $0.369 | ¥2.66 |
| 1080P | 5秒 | $0.1233/秒 | $0.6165 | ¥4.44 |
| 480P | 10秒 | $0.0369/秒 | $0.369 | ¥2.66 |
| 720P | 10秒 | $0.0738/秒 | $0.738 | ¥5.32 |
| 1080P | 10秒 | $0.1233/秒 | $1.233 | ¥8.88 |

*汇率按 1 USD = 7.2 CNY 估算*

### 扣费时机

- **提交时**：进行余额预检查（不实际扣费）
- **完成时**：任务成功完成后，根据实际生成的分辨率和时长扣费
- **失败时**：任务失败不扣费

---

## 错误处理

### 常见错误码

| HTTP状态码 | 错误代码 | 说明 | 解决方法 |
|-----------|---------|------|---------|
| 400 | `InvalidParameter.ImageFormat` | 图片格式不支持 | 使用 JPEG 或 PNG 格式 |
| 400 | `InvalidParameter.ImageSize` | 图片尺寸超限 | 压缩图片至 5MB 以下 |
| 400 | `InvalidParameter.DataInspection` | 内容审核失败 | 检查图片和提示词内容 |
| 401 | `InvalidApiKey` | API Key 无效 | 检查 Authorization 头 |
| 403 | `InsufficientQuota` | 余额不足 | 充值后重试 |
| 429 | `Throttling.RateLimit` | 请求频率过高 | 降低请求频率 |
| 500 | `InternalError` | 服务器内部错误 | 稍后重试 |

### 错误响应示例

```json
{
  "code": "fail_to_fetch_task",
  "message": "{\"request_id\":\"167c0805-d44d-4f5c-8b8f-d4c628a18b81\",\"code\":\"InvalidParameter.ImageFormat\",\"message\":\"图片格式不支持或格式错误\"}",
  "data": null
}
```

---

## 最佳实践

### 1. 轮询策略

```python
import time

def smart_poll(task_id, initial_interval=3, max_interval=10):
    """智能轮询：根据任务状态动态调整间隔"""
    interval = initial_interval
    
    while True:
        status_data = check_task_status(task_id)
        status = status_data.get("status")
        
        if status in ["succeeded", "failed"]:
            return status_data
        
        # 动态调整轮询间隔（避免频繁请求）
        time.sleep(interval)
        interval = min(interval + 1, max_interval)
```

### 2. 错误重试

```python
from functools import wraps
import time

def retry_on_error(max_retries=3, backoff=2):
    """指数退避重试装饰器"""
    def decorator(func):
        @wraps(func)
        def wrapper(*args, **kwargs):
            for attempt in range(max_retries):
                try:
                    return func(*args, **kwargs)
                except requests.exceptions.RequestException as e:
                    if attempt == max_retries - 1:
                        raise
                    wait_time = backoff ** attempt
                    print(f"请求失败，{wait_time}秒后重试...")
                    time.sleep(wait_time)
        return wrapper
    return decorator

@retry_on_error(max_retries=3)
def generate_video_with_retry(prompt, image_url):
    return generate_video(prompt, image_url)
```

### 3. 批量处理

```python
import asyncio
import aiohttp

async def batch_generate_videos(tasks):
    """批量提交任务（异步）"""
    async with aiohttp.ClientSession() as session:
        async def submit_task(task):
            async with session.post(
                f"{BASE_URL}/v1/video/generations",
                headers={"Authorization": f"Bearer {API_KEY}"},
                json={
                    "model": "wan2.5-i2v-preview",
                    "prompt": task["prompt"],
                    "image": task["image"],
                    "duration": task.get("duration", 5),
                    "resolution": task.get("resolution", "720p")
                }
            ) as response:
                return await response.json()
        
        results = await asyncio.gather(*[submit_task(task) for task in tasks])
        return results

# 使用示例
tasks = [
    {"prompt": "小猫玩耍", "image": "https://example.com/cat1.jpg"},
    {"prompt": "小狗奔跑", "image": "https://example.com/dog1.jpg"},
]
results = asyncio.run(batch_generate_videos(tasks))
```

---

## 注意事项

1. ⚠️ **配额消耗：** 视频生成会消耗较多配额，请确保账户有足够的余额
2. ⚠️ **任务保留：** 生成的视频URL有效期为 24 小时，请及时下载
3. ⚠️ **内容审核：** 图片和提示词必须符合内容安全规范，否则任务会失败
4. ⚠️ **并发限制：** 单个账户可能存在并发任务数量限制
5. ⚠️ **网络要求：** 图片URL必须可公开访问，建议使用 HTTPS 协议
6. ⚠️ **处理时间：** 5秒视频通常需要 1-2 分钟，10秒视频需要 2-3 分钟
7. ⚠️ **质量权衡：** 1080P 质量最好但费用最高，建议根据实际需求选择

---

## 常见问题 FAQ

### Q1: 为什么任务一直处于 PENDING 状态？

**A:** 可能原因：
1. 服务器负载较高，任务排队中
2. 图片无法访问（检查 URL 是否有效）
3. 内容审核中（敏感内容可能需要更长时间）

**建议：** 等待 5-10 分钟，如仍未处理请联系技术支持。

### Q2: 为什么生成的视频没有音频？

**A:** 需要满足以下任一条件：
1. 设置 `generate_audio: true`（自动生成音频）
2. 提供 `audio_url`（自定义音频）

如果都没有设置，生成的视频将是静音的。

### Q3: 如何获得更好的视频效果？

**A:** 建议：
1. 使用高质量的输入图片（分辨率高、清晰度好）
2. 提供详细的提示词（描述动作、镜头、场景细节）
3. 启用 `smart_rewrite` 功能
4. 选择更高的分辨率（720P 或 1080P）
5. 使用 seed 参数多次尝试，选择最佳结果

### Q4: 可以生成多长的视频？

**A:** 目前仅支持 5 秒和 10 秒，不支持更长的视频。

### Q5: 视频URL过期后如何重新获取？

**A:** 视频URL有效期为24小时，过期后无法重新获取。建议：
1. 任务成功后立即下载视频
2. 保存到自己的存储服务
3. 如需要，可以使用相同参数重新生成

---

## 参考资料

- **官方文档：** [阿里云通义万相 API 文档](https://bailian.console.aliyun.com/?tab=api#/api/?type=model&url=2867393)
- **技术支持：** support@ai-nebula.com
- **API 文档：** https://docs.ai-nebula.com

---

**文档版本：** v1.0  
**最后更新：** 2025-11-04  
**兼容模型版本：** wan2.5-i2v-preview
