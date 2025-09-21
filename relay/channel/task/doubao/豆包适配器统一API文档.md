# 豆包视频生成适配器统一API文档

本文档描述了通过统一视频生成接口调用豆包视频大模型的请求格式和响应格式，适用于前端开发者直接使用。

## 一、接口概览

### 基础信息
| 项目 | 内容 |
|------|------|
| **提交任务接口** | `POST /api/sync/system/videos/generations` |
| **查询任务接口** | `GET /api/sync/system/videos/generations/{task_id}?user_id={user_id}` |
| **认证方式** | `access_token` 系统访问令牌 |
| **请求头** | `Content-Type: application/json` |
| **任务模式** | 异步任务：提交获得task_id，轮询查询获取结果 |

### 支持的豆包模型
```json
[
  "doubao-seedance-1-0-pro-250528",    // 专业版文生视频/图生视频
  "doubao-seedance-1-0-lite-t2v-250428", // 轻量版文生视频
  "doubao-seedance-1-0-lite-i2v-250428", // 轻量版图生视频
  "doubao-seaweed-1-0-t2v-250428",     // 海草版文生视频
  "wan2-1-14b-i2v-250417",             // 万象版图生视频
  "wan2-1-14b-flf2v-250417"            // 万象版首尾帧生成
]
```

---

## 二、提交视频生成任务

### 2.1 通用请求格式
```http
POST /api/sync/system/videos/generations?access_token={your_access_token}
Content-Type: application/json

{
  "user_id": 123,
  "model": "doubao-seedance-1-0-pro-250528",
  "metadata": {
    "content": [...], // 豆包原生content数组
    "callback_url": "https://your-domain.com/callback" // 可选
  }
}
```

### 2.2 场景1：文生视频（T2V）
**适用模型**：`doubao-seedance-1-0-pro-250528`、`doubao-seedance-1-0-lite-t2v-250428`

```json
{
  "user_id": 123,
  "model": "doubao-seedance-1-0-pro-250528",
  "metadata": {
    "content": [
      {
        "type": "text",
        "text": "多个镜头。一名侦探进入一间光线昏暗的房间。他检查桌上的线索，手里拿起桌上的某个物品。镜头转向他正在思索。 --ratio 16:9 --dur 5"
      }
    ]
  }
}
```

### 2.3 场景2：图生视频-首帧（I2V）
**适用模型**：`doubao-seedance-1-0-pro-250528`、`doubao-seedance-1-0-lite-i2v-250428`

```json
{
  "user_id": 123,
  "model": "doubao-seedance-1-0-pro-250528", 
  "metadata": {
    "content": [
      {
        "type": "text",
        "text": "女孩抱着狐狸，女孩睁开眼，温柔地看向镜头，狐狸友善地抱着，镜头缓缓拉出，女孩的头发被风吹动 --ratio adaptive --dur 5"
      },
      {
        "type": "image_url",
        "image_url": {
          "url": "https://example.com/first_frame.jpg"
        }
      }
    ]
  }
}
```

### 2.4 场景3：图生视频-首尾帧（seedance-lite）
**适用模型**：`doubao-seedance-1-0-lite-i2v-250428`

```json
{
  "user_id": 123,
  "model": "doubao-seedance-1-0-lite-i2v-250428",
  "metadata": {
    "content": [
      {
        "type": "text", 
        "text": "一只蓝绿精卫鸟变成人形 --rs 720p --dur 5 --cf false"
      },
      {
        "type": "image_url",
        "image_url": {
          "url": "https://example.com/first_frame.png"
        },
        "role": "first_frame"
      },
      {
        "type": "image_url", 
        "image_url": {
          "url": "https://example.com/last_frame.png"
        },
        "role": "last_frame"
      }
    ]
  }
}
```

### 2.5 场景4：图生视频-首尾帧（wan2.1）
**适用模型**：`wan2-1-14b-flf2v-250417`

```json
{
  "user_id": 123,
  "model": "wan2-1-14b-flf2v-250417",
  "metadata": {
    "content": [
      {
        "type": "text",
        "text": "CG动画风格，一只蓝色的小鸟从地面起飞，煽动翅膀。小鸟羽毛细腻，胸前有独特的花纹 --rs 720p --dur 5"
      },
      {
        "type": "image_url",
        "image_url": {
          "url": "https://example.com/first_frame.png" 
        },
        "role": "first_frame"
      },
      {
        "type": "image_url",
        "image_url": {
          "url": "https://example.com/last_frame.png"
        },
        "role": "last_frame"
      }
    ]
  }
}
```

### 2.6 场景5：Base64图片输入
```json
{
  "user_id": 123,
  "model": "doubao-seedance-1-0-lite-i2v-250428",
  "metadata": {
    "content": [
      {
        "type": "text",
        "text": "女孩抱着狐狸，温柔地看向镜头 --ratio adaptive --dur 5"
      },
      {
        "type": "image_url",
        "image_url": {
          "url": "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAA..."
        }
      }
    ]
  }
}
```

### 2.7 场景6：参考图输入（seedance-lite）
**适用模型**：`doubao-seedance-1-0-lite-i2v-250428`

```json
{
  "user_id": 123,
  "model": "doubao-seedance-1-0-lite-i2v-250428",
  "metadata": {
    "content": [
      {
        "type": "text",
        "text": "[图1]戴着眼镜穿着蓝色T恤的男生和[图2]的柯基小狗，坐在[图3]的草坪上，3D卡通风格"
      },
      {
        "type": "image_url",
        "image_url": {
          "url": "https://example.com/ref1.png"
        },
        "role": "reference_image"
      },
      {
        "type": "image_url", 
        "image_url": {
          "url": "https://example.com/ref2.png"
        },
        "role": "reference_image"
      },
      {
        "type": "image_url",
        "image_url": {
          "url": "https://example.com/ref3.png" 
        },
        "role": "reference_image"
      }
    ]
  }
}
```

### 2.8 场景7：配置回调通知
```json
{
  "user_id": 123,
  "model": "doubao-seedance-1-0-lite-t2v-250428",
  "metadata": {
    "content": [
      {
        "type": "text",
        "text": "写实风格，晴朗的蓝天之下，一大片白色的雏菊花田 --ratio 16:9"
      }
    ],
    "callback_url": "https://your-domain.com/video-callback"
  }
}
```

---

## 三、文本提示词参数说明

### 3.1 支持的参数
| 参数简写 | 参数全称 | 说明 | 示例值 | 支持模型 |
|----------|----------|------|--------|-----------|
| `--rs` | `--resolution` | 视频分辨率 | `480p`, `720p`, `1080p` | 所有模型 |
| `--ratio` | `--ratio` | 宽高比 | `16:9`, `9:16`, `1:1`, `adaptive` | 所有模型 |
| `--dur` | `--duration` | 视频时长(秒) | `5`, `10` | 所有模型 |
| `--fps` | `--framespersecond` | 帧率 | `24`, `30` | 所有模型 |
| `--seed` | `--seed` | 随机种子 | `12345` | 所有模型 |
| `--wm` | `--watermark` | 水印开关 | `true`, `false` | 部分模型 |
| `--cf` | `--camerafixed` | 固定摄像头 | `true`, `false` | lite模型 |

### 3.2 参数使用示例
```json
{
  "type": "text",
  "text": "女孩抱着狐狸 --rs 720p --ratio 16:9 --dur 5 --fps 24 --seed 11 --cf false"
}
```

---

## 四、提交任务响应格式

### 4.1 成功响应
```json
{
  "task_id": "cgt-2025123456-abcd",
  "status": "submitted"
}
```

### 4.2 失败响应
```json
{
  "error": {
    "code": "invalid_request",
    "message": "模型参数不能为空"
  }
}
```

---

## 五、查询视频任务状态

### 5.1 查询请求
```http
GET /api/sync/system/videos/generations/{task_id}?user_id={user_id}&access_token={your_access_token}
```

**路径参数：**
- `task_id`: 提交任务返回的任务ID

**查询参数：**
- `user_id`: 用户ID
- `access_token`: 系统访问令牌

### 5.2 查询响应格式

#### 任务进行中
```json
{
  "task_id": "cgt-2025123456-abcd",
  "status": "queued",  // 或 "in_progress"
  "url": "",
  "format": "mp4",
  "metadata": null,
  "error": null
}
```

#### 任务成功
```json
{
  "task_id": "cgt-2025123456-abcd", 
  "status": "succeeded",
  "url": "https://ark-content-generation-cn-beijing.tos-cn-beijing.volces.com/video.mp4?X-Tos-Algorithm=...",
  "format": "mp4",
  "metadata": {
    "id": "cgt-2025123456-abcd",
    "model": "doubao-seedance-1-0-pro-250528",
    "status": "succeeded",
    "content": {
      "video_url": "https://ark-content-generation-cn-beijing.tos-cn-beijing.volces.com/video.mp4?..."
    },
    "usage": {
      "completion_tokens": 108900,
      "total_tokens": 108900
    },
    "created_at": 1743414619,
    "updated_at": 1743414673,
    "seed": 10,
    "resolution": "720p", 
    "duration": 5,
    "ratio": "16:9",
    "framespersecond": 24
  },
  "error": null
}
```

#### 任务失败
```json
{
  "task_id": "cgt-2025123456-abcd",
  "status": "failed",
  "url": "",
  "format": "mp4", 
  "metadata": {
    "id": "cgt-2025123456-abcd",
    "status": "failed",
    "error": {
      "code": 40001,
      "message": "模型资源不足",
      "type": "resource_exhausted"
    },
    "reason": "模型资源不足"
  },
  "error": {
    "code": 400,
    "message": "模型资源不足"
  }
}
```

---

## 六、状态映射表

| 豆包状态 | 统一状态 | 说明 |
|----------|----------|------|
| `queued` | `queued` | 任务排队中 |
| `running` | `in_progress` | 任务执行中 |
| `succeeded` | `succeeded` | 任务成功完成 |
| `failed` | `failed` | 任务执行失败 |
| `cancelled` | `failed` | 任务被取消（视为失败） |

---

## 七、前端使用示例

### 7.1 提交任务（JavaScript）
```javascript
async function submitVideoTask() {
  const response = await fetch('/api/sync/system/videos/generations?access_token=your_token', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json'
    },
    body: JSON.stringify({
      user_id: 123,
      model: "doubao-seedance-1-0-pro-250528",
      metadata: {
        content: [
          {
            type: "text", 
            text: "一名侦探进入光线昏暗的房间 --ratio 16:9 --dur 5"
          }
        ]
      }
    })
  });
  
  const result = await response.json();
  if (result.task_id) {
    console.log('任务提交成功，ID:', result.task_id);
    return result.task_id;
  } else {
    console.error('提交失败:', result.error);
  }
}
```

### 7.2 查询任务状态（JavaScript）
```javascript
async function queryTaskStatus(taskId, userId) {
  const response = await fetch(
    `/api/sync/system/videos/generations/${taskId}?user_id=${userId}&access_token=your_token`
  );
  
  const result = await response.json();
  
  switch (result.status) {
    case 'queued':
    case 'in_progress':
      console.log('任务进行中，状态:', result.status);
      // 继续轮询
      setTimeout(() => queryTaskStatus(taskId, userId), 5000);
      break;
      
    case 'succeeded':
      console.log('任务成功完成！');
      console.log('视频URL:', result.url);
      console.log('元数据:', result.metadata);
      // 下载或展示视频
      break;
      
    case 'failed':
      console.error('任务失败:', result.error?.message);
      break;
  }
  
  return result;
}
```

### 7.3 完整工作流程
```javascript
async function generateVideo() {
  try {
    // 1. 提交任务
    const taskId = await submitVideoTask();
    if (!taskId) return;
    
    // 2. 轮询查询状态  
    const finalResult = await queryTaskStatus(taskId, 123);
    
    // 3. 处理结果
    if (finalResult.status === 'succeeded') {
      // 视频生成成功，处理视频URL
      handleVideoSuccess(finalResult.url, finalResult.metadata);
    }
    
  } catch (error) {
    console.error('视频生成过程出错:', error);
  }
}

function handleVideoSuccess(videoUrl, metadata) {
  // 创建视频元素
  const video = document.createElement('video');
  video.src = videoUrl;
  video.controls = true;
  document.body.appendChild(video);
  
  // 显示元数据
  console.log('视频分辨率:', metadata.resolution);
  console.log('视频时长:', metadata.duration, '秒');
  console.log('帧率:', metadata.framespersecond);
}
```

---

## 八、注意事项

1. **视频URL有效期**：返回的视频URL为临时链接，通常24小时有效，请及时下载保存
2. **轮询间隔**：建议每5-10秒查询一次任务状态，避免过于频繁的请求
3. **参数替换**：所有示例中的access_token、user_id、回调URL需要替换为真实值
4. **模型权限**：确保使用的模型已开通服务权限
5. **图片要求**：图片URL必须可公网访问，Base64编码需包含完整的data URL前缀
6. **回调通知**：配置callback_url后，任务状态变更时会自动推送到指定地址

---

## 九、错误码说明

| 错误码 | 说明 | 解决方案 |
|--------|------|----------|
| `invalid_request` | 请求格式错误 | 检查JSON格式和必需参数 |
| `invalid_model` | 模型不存在或未授权 | 确认模型ID正确且已开通服务 |
| `quota_exhausted` | 配额不足 | 检查账户余额或配额限制 |
| `rate_limit_exceeded` | 请求频率超限 | 减少请求频率，增加间隔时间 |
| `task_not_found` | 任务不存在 | 检查task_id和user_id是否正确 |

通过以上文档，前端开发者可以完整地集成豆包视频生成功能，覆盖从任务提交到结果获取的全流程。
