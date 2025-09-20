# 火山引擎视频大模型（豆包关联）API 调用样例文档

本文整理火山引擎方舟平台视频大模型的 **提交视频生成任务** 与 **查询视频生成任务** 全场景 API 样例，包含入参格式、出参结构及关键说明，适用于所有支持的视频生成模型（如 `doubao-seedance` 系列、`wan2.1-14b` 系列等）。


## 一、接口基础信息
### 1.1 通用请求配置
| 配置项         | 内容                                                                 |
|----------------|----------------------------------------------------------------------|
| 请求方式       | 提交任务：`POST`；查询任务：`GET`                                    |
| 基础请求地址   | 提交任务：`https://ark.cn-beijing.volces.com/api/v3/contents/generations/tasks` <br> 查询任务：`https://ark.cn-beijing.volces.com/api/v3/contents/generations/tasks/{task_id}` |
| 必选请求头     | - `Content-Type: application/json` <br> - `Authorization: Bearer $ARK_API_KEY`（`$ARK_API_KEY` 需替换为你的真实 API 密钥） |
| 核心说明       | 1. 视频生成为 **异步任务**：提交任务仅返回任务 ID，需通过查询接口获取最终结果；<br> 2. 视频 URL 为临时链接，需及时保存至本地；<br> 3. 模型 ID 需提前开通服务，确保权限有效。 |


## 二、提交视频生成任务（入参+出参样例）
按生成场景分类，涵盖文生视频、图生视频（首帧/首尾帧/参考图）、Base64 图片输入、回调配置等全场景。


### 2.1 场景 1：文生视频（T2V）
仅通过文本描述生成视频，支持 `doubao-seedance-pro`、`doubao-seedance-lite-t2v`、`doubao-seaweed` 等模型。

#### 入参样例（curl）
```bash
curl -X POST https://ark.cn-beijing.volces.com/api/v3/contents/generations/tasks \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $ARK_API_KEY" \
  -d '{
    "model": "doubao-seedance-1-0-pro-250528", # 替换为目标文生视频模型ID
    "content": [
        {
            "type": "text", # 输入类型：固定为"text"
            "text": "多个镜头。一名侦探进入一间光线昏暗的房间。他检查桌上的线索，手里拿起桌上的某个物品。镜头转向他正在思索。 --ratio 16:9" # 文本提示词+可选参数（宽高比16:9）
        }
    ]
}'
```


### 2.2 场景 2：图生视频-首帧输入（I2V-首帧）
通过「首帧图片+文本」生成视频，支持 `doubao-seedance-pro`、`doubao-seaweed`、`wan2-1-14b-i2v` 等模型。

#### 入参样例（curl）
```bash
curl -X POST https://ark.cn-beijing.volces.com/api/v3/contents/generations/tasks \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $ARK_API_KEY" \
  -d '{
    "model": "doubao-seedance-1-0-pro-250528", # 支持首帧图生视频的模型ID
    "content": [
        {
            "type": "text",
            "text": "女孩抱着狐狸，女孩睁开眼，温柔地看向镜头，狐狸友善地抱着，镜头缓缓拉出，女孩的头发被风吹动  --ratio adaptive  --dur 5" # adaptive：自动适配图片比例
        },
        {
            "type": "image_url", # 输入类型：图片URL
            "image_url": {
                "url": "https://ark-project.tos-cn-beijing.volces.com/doc_image/i2v_foxrgirl.png" # 首帧图片公网URL
            }
        }
    ]
}'
```


### 2.3 场景 3：seedance-lite-首尾帧输入（I2V-首尾帧）
仅 `doubao-seedance-1-0-lite-i2v` 模型支持，通过「首帧+尾帧+文本」生成视频（分辨率限 480p/720p）。

#### 入参样例（curl）
```bash
curl -X POST https://ark.cn-beijing.volces.com/api/v3/contents/generations/tasks \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $ARK_API_KEY" \
  -d '{
    "model": "doubao-seedance-1-0-lite-i2v-250428", # 固定模型ID
    "content": [
         {
            "type": "text",
            "text": "一只蓝绿精卫鸟变成人形 --rs 720p  --dur 5 --cf false" # rs：分辨率；cf：是否固定摄像头
        },
        {
            "type": "image_url",
            "image_url": {
                "url": "https://ark-project.tos-cn-beijing.volces.com/doc_image/seelite_first_frame.png"
            },
            "role": "first_frame" # 标记为「首帧」
        },
        {
            "type": "image_url",
            "image_url": {
                "url": "https://ark-project.tos-cn-beijing.volces.com/doc_image/seelite_last_frame.png"
            },
            "role": "last_frame" # 标记为「尾帧」
        }
    ]
}'
```


### 2.4 场景 4：wan2.1-14b-首尾帧输入（I2V-首尾帧）
仅 `wan2-1-14b-flf2v` 模型支持，通过「首帧+尾帧+文本」生成视频。

#### 入参样例（curl）
```bash
curl -X POST https://ark.cn-beijing.volces.com/api/v3/contents/generations/tasks \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $ARK_API_KEY" \
  -d '{
    "model": "wan2-1-14b-flf2v-250417", # 固定模型ID
    "content": [
         {
            "type": "text",
            "text": "CG动画风格，一只蓝色的小鸟从地面起飞，煽动翅膀。小鸟羽毛细腻，胸前有独特的花纹，背景是蓝天白云，阳光明媚。镜头跟随小鸟向上移动，展现出小鸟飞翔的姿态和天空的广阔。近景，仰视视角。--rs 720p  --dur 5"
        },
        {
            "type": "image_url",
            "image_url": {
                "url": "https://ark-project.tos-cn-beijing.volces.com/doc_image/wan_input_first_frame.png"
            },
            "role": "first_frame"
        },
        {
            "type": "image_url",
            "image_url": {
                "url": "https://ark-project.tos-cn-beijing.volces.com/doc_image/wan_input_last_frame.png"
            },
            "role": "last_frame"
        }
    ]
}'
```


### 2.5 场景 5：图生视频-Base64 编码输入
图片以 Base64 格式传入（无需公网 URL），支持所有图生视频模型。

#### 入参样例（curl）
```bash
curl -X POST https://ark.cn-beijing.volces.com/api/v3/contents/generations/tasks \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $ARK_API_KEY" \
  -d '{
    "model": "doubao-seedance-1-0-lite-i2v-250428", # 支持图生视频的模型ID
    "content": [
        {
            "type": "text",
            "text": "女孩抱着狐狸，女孩睁开眼，温柔地看向镜头，狐狸友善地抱着，镜头缓缓拉出，女孩的头发被风吹动  --ratio adaptive  --dur 5"
        },
        {
            "type": "image_url",
            "image_url": {
                "url": "data:image/png;base64,aHR0cHM6Ly9hcmstcHJvamVjdC50b3MtY24tYmVpamluZy52b2xjZXMuY29tL2RvY19pbWFnZS9pMnZfZm94cmdpcmwucG5n" # Base64格式：data:image/[格式];base64,[编码内容]
            }
        }
    ]
}'
```


### 2.6 场景 6：配置回调通知（callback_url）
任务状态变更（排队/运行/成功/失败）时，平台自动向 `callback_url` 推送结果，减少轮询开销。

#### 入参样例（curl）
```bash
curl https://ark.cn-beijing.volces.com/api/v3/contents/generations/tasks \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $ARK_API_KEY" \
  -d $'{
    "model": "doubao-seedance-1-0-lite-t2v-250428", # 文生视频模型ID
    "content": [
        {
            "type": "text",
            "text": "写实风格，晴朗的蓝天之下，一大片白色的雏菊花田，镜头逐渐拉近，最终定格在一朵雏菊花的特写上，花瓣上有几颗晶莹的露珠。 --ratio 16:9"
        }
    ],
    "callback_url": "https://your-domain.com/video-callback" # 替换为你的真实回调地址
}'
```


### 2.7 场景 7：文本提示词-全量参数（简写/全称）
指定视频分辨率、帧率、水印、种子等所有参数，支持参数简写或全称。

#### 入参样例（参数简写）
```json
"content": [
    {
        "type": "text",
        "text": "女孩抱着狐狸 --rs 720p --rt 16:9 --dur 5 --fps 24 --wm true --seed 11 --cf false"
        // rs=resolution（分辨率）、rt=ratio（宽高比）、dur=duration（时长）、fps=framespersecond（帧率）、wm=watermark（水印）、seed（随机种子）、cf=camerafixed（固定摄像头）
    },
    {
        "type": "image_url",
        "image_url": {
            "url": "https://ark-project.tos-cn-beijing.volces.com/doc_image/i2v_foxrgirl.png"
        }
    }
]
```

#### 入参样例（参数全称）
```json
"content": [
    {
        "type": "text",
        "text": "女孩抱着狐狸 --resolution 720p --ratio 16:9 --duration 5 --framespersecond 24 --watermark true --seed 11 --camerafixed false"
    },
    {
        "type": "image_url",
        "image_url": {
            "url": "https://ark-project.tos-cn-beijing.volces.com/doc_image/i2v_foxrgirl.png"
        }
    }
]
```


### 2.8 场景 8：seedance-lite-参考图输入
仅 `doubao-seedance-1-0-lite-i2v` 模型支持，通过「多张参考图+文本」生成视频（文本中用 `[图N]` 关联参考图）。

#### 入参样例（curl）
```bash
curl -X POST https://ark.cn-beijing.volces.com/api/v3/contents/generations/tasks \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $ARK_API_KEY" \
  -d '{
    "model": "doubao-seedance-1-0-lite-i2v-250428", # 固定模型ID
    "content": [
         {
            "type": "text",
            "text": "[图1]戴着眼镜穿着蓝色T恤的男生和[图2]的柯基小狗，坐在[图3]的草坪上，3D卡通风格" # [图N] 对应下方 reference_image 的顺序
        },
        {
            "type": "image_url",
            "image_url": {
                "url": "https://ark-project.tos-cn-beijing.volces.com/doc_image/seelite_ref_1.png"
            },
            "role": "reference_image" # 标记为「参考图」
        },
        {
            "type": "image_url",
            "image_url": {
                "url": "https://ark-project.tos-cn-beijing.volces.com/doc_image/seelite_ref_2.png"
            },
            "role": "reference_image"
        },
        {
            "type": "image_url",
            "image_url": {
                "url": "https://ark-project.tos-cn-beijing.volces.com/doc_image/seelite_ref_3.png"
            },
            "role": "reference_image"
        }
    ]
}'
```


### 2.9 提交任务-出参样例
提交成功后仅返回 **任务 ID**，用于后续查询结果：
```json
{
  "id": "cgt-2025******-****" // 任务唯一标识（示例格式）
}
```


## 三、查询视频生成任务（入参+出参样例）
通过「提交任务返回的 ID」查询任务状态（如 `queued` 排队中、`running` 运行中、`succeeded` 成功、`failed` 失败）及最终视频结果。


### 3.1 查询任务-入参样例（curl）
```bash
curl -X GET "https://ark.cn-beijing.volces.com/api/v3/contents/generations/tasks/cgt-2025****" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $ARK_API_KEY"
# 注：将 "cgt-2025****" 替换为提交任务返回的真实任务ID
```


### 3.2 查询任务-出参样例（任务成功状态）
任务成功（`status: "succeeded"`）时，返回视频 URL 及生成参数详情：
```json
{
  "id": "cgt-2025******-****", // 任务ID
  "model": "doubao-seedance-1-0-pro-250528", // 调用的模型ID
  "status": "succeeded", // 任务状态（succeeded/failed/running/queued）
  "content": {
    "video_url": "https://ark-content-generation-cn-beijing.tos-cn-beijing.volces.com/doubao-seedance-1-0-pro/****.mp4?X-Tos-Algorithm=TOS4-HMAC-SHA256&X-Tos-Credential=AKLTY****%2Fcn-beijing%2Ftos%2Frequest&X-Tos-Date=20250331T095113Z&X-Tos-Expires=86400&X-Tos-Signature=***&X-Tos-SignedHeaders=host"
    // 视频临时访问URL（有效期通常1天，需及时下载）
  },
  "seed": 10, // 随机种子（用于复现相似结果）
  "resolution": "720p", // 视频分辨率
  "duration": 5, // 视频时长（秒）
  "ratio": "16:9", // 视频宽高比
  "framespersecond": 24, // 视频帧率（帧/秒）
  "usage": {
    "completion_tokens": 108900, // 生成消耗的tokens
    "total_tokens": 108900 // 总消耗tokens（文生视频无prompt tokens）
  },
  "created_at": 1743414619, // 任务创建时间（时间戳）
  "updated_at": 1743414673 // 任务更新时间（时间戳）
}
```


## 四、注意事项
1. **参数替换**：所有样例中的 `$ARK_API_KEY`、`callback_url`、`task_id`、图片 URL/Base64 需替换为你的真实信息；
2. **模型权限**：确保调用的模型已在火山引擎控制台开通服务，否则会返回权限错误；
3. **视频有效期**：`video_url` 为临时带签名 URL，有效期通常为 24 小时，建议生成后立即下载保存；
4. **状态处理**：若查询时 `status` 为 `failed`，可在返回结果中查看 `error` 字段获取失败原因（如参数错误、模型资源不足）。