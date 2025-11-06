# 通义千问图像编辑接口文档

## 1. 接口基础信息

- **模型名称**: qwen-image-edit-plus / qwen-image-edit-plus-2025-10-30 (通义千问图像编辑模型)
- **基础URL**: `https://llm.ai-nebula.com/v1/images/generations`
- **认证方式**: Bearer Token
- **认证令牌**: `Bearer sk-xxxxxxxxxx`
- **核心能力**: 
  - ✅ 单图编辑（1张输入图片）
  - ✅ 多图融合（2-3张输入图片）
  - ✅ 修改图内文字
  - ✅ 增删/移动物体
  - ✅ 改变主体动作
  - ✅ 迁移图片风格
  - ✅ 增强画面细节
  - ❌ 不支持纯文生图（必须有图片输入）
- **图片输入**: 1-3张（支持URL或Base64）
- **图片输出**: 1-6张
- **计费方式**: 按张计费（¥0.18/张，官方¥0.2/张）
- **官方文档**: [阿里百炼模型接口文档（qwen-image-edit-plus）](https://bailian.console.aliyun.com/?spm=5176.29597918.J_SEsSjsNv72yRuRFS2VknO.2.80d27b08ALuu1L&tab=api#/api/?type=model&url=2975126)

### 1.1 核心参数说明

| 参数名 | 类型 | 必填 | 说明 | 示例值 |
|--------|------|------|------|--------|
| model | string | 是 | 模型名称 | qwen-image-edit-plus |
| prompt | string | 是 | 图像编辑指令（文本提示词） | "图1中的女生穿着图2中的黑色裙子按图3的姿势坐下" |
| parameters | object | 是 | 生成参数 | 见下方说明 |

### 1.2 请求参数结构

qwen-image-edit-plus 使用简化格式：

```json
{
  "model": "qwen-image-edit-plus",
  "prompt": "图像编辑指令",
  "parameters": {
    "n": 2,                          // 输出图像数量（1-6）
    "negative_prompt": "",           // 负面提示词（可选）
    "watermark": false,              // 水印（可选，默认false）
    "seed": 12345                    // 随机种子（可选）
  },
  "contents": [
    {
      "role": "user",
      "parts": [
        { "image": "https://..." },  // 第1张图片（必选）
        { "image": "https://..." },  // 第2张图片（可选）
        { "image": "https://..." },  // 第3张图片（可选）
        { "text": "编辑指令" }        // 文本指令（必选，放在最后）
      ]
    }
  ]
}
```

### 1.3 图片输入要求

#### 图片格式
- **支持格式**: JPG、JPEG、PNG、BMP、TIFF、WEBP
- **图片数量**: 1-3张
- **输入方式**: URL 或 Base64 编码

#### 图片规格
- **分辨率**: 宽度和高度均需在 **384-3072 像素**范围内
- **文件大小**: 不超过 **10MB**
- **宽高比**: 无限制

#### Base64 编码格式

```
data:{mime_type};base64,{base64_data}
```

**MIME类型对应关系**：
- JPEG/JPG: `image/jpeg`
- PNG: `image/png`
- BMP: `image/bmp`
- TIFF: `image/tiff`
- WEBP: `image/webp`

**示例**：
```
data:image/jpeg;base64,/9j/4AAQSkZJRgABAQEAYABgAAD...
```

## 2. 单图编辑

### 2.1 修改图内文字

使用单张图片，精确修改图片中的文字内容：

```bash
curl -X POST "https://llm.ai-nebula.com/v1/images/generations" \
  -H "Authorization: Bearer sk-xxxxxxxxxx" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen-image-edit-plus",
    "prompt": "将图片中的\"Hello\"改成\"你好\"，保持其他内容不变",
    "parameters": {
      "n": 2,
      "negative_prompt": "模糊的文字、错别字",
      "watermark": false
    },
    "contents": [
      {
        "role": "user",
        "parts": [
          {
            "image": "https://example.com/input.jpg"
          },
          {
            "text": "将图片中的\"Hello\"改成\"你好\"，保持其他内容不变"
          }
        ]
      }
    ]
  }'
```

### 2.2 增删物体

在图片中添加或删除物体：

```bash
curl -X POST "https://llm.ai-nebula.com/v1/images/generations" \
  -H "Authorization: Bearer sk-xxxxxxxxxx" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen-image-edit-plus",
    "prompt": "在图片的右侧添加一棵樱花树，树下有几片飘落的花瓣",
    "parameters": {
      "n": 2,
      "negative_prompt": "不自然、违和感、低质量",
      "watermark": false
    },
    "contents": [
      {
        "role": "user",
        "parts": [
          {
            "image": "https://example.com/garden.jpg"
          },
          {
            "text": "在图片的右侧添加一棵樱花树，树下有几片飘落的花瓣"
          }
        ]
      }
    ]
  }'
```

### 2.3 风格迁移

改变图片的艺术风格：

```bash
curl -X POST "https://llm.ai-nebula.com/v1/images/generations" \
  -H "Authorization: Bearer sk-xxxxxxxxxx" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen-image-edit-plus",
    "prompt": "将这张照片转换为水彩画风格，保持原有的构图和主体",
    "parameters": {
      "n": 3,
      "negative_prompt": "过度渲染、失真",
      "watermark": false
    },
    "contents": [
      {
        "role": "user",
        "parts": [
          {
            "image": "https://example.com/photo.jpg"
          },
          {
            "text": "将这张照片转换为水彩画风格，保持原有的构图和主体"
          }
        ]
      }
    ]
  }'
```

### 2.4 改变主体动作

修改人物或动物的动作姿态：

```bash
curl -X POST "https://llm.ai-nebula.com/v1/images/generations" \
  -H "Authorization: Bearer sk-xxxxxxxxxx" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen-image-edit-plus",
    "prompt": "让图片中的猫咪从坐着改为跳跃的姿势，保持猫咪的外观和背景不变",
    "parameters": {
      "n": 2,
      "negative_prompt": "不自然的姿势、扭曲",
      "watermark": false,
      "seed": 42
    },
    "contents": [
      {
        "role": "user",
        "parts": [
          {
            "image": "https://example.com/cat.jpg"
          },
          {
            "text": "让图片中的猫咪从坐着改为跳跃的姿势，保持猫咪的外观和背景不变"
          }
        ]
      }
    ]
  }'
```

## 3. 多图融合

### 3.1 服装迁移（2图融合）

将一张图片中的服装应用到另一张图片的人物上：

```bash
curl -X POST "https://llm.ai-nebula.com/v1/images/generations" \
  -H "Authorization: Bearer sk-xxxxxxxxxx" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen-image-edit-plus",
    "prompt": "图1中的女生穿上图2中的白色连衣裙，保持图1女生的发型、表情和姿势不变",
    "parameters": {
      "n": 2,
      "negative_prompt": "不自然、比例失调",
      "watermark": false
    },
    "contents": [
      {
        "role": "user",
        "parts": [
          {
            "image": "https://example.com/person.jpg"
          },
          {
            "image": "https://example.com/dress.jpg"
          },
          {
            "text": "图1中的女生穿上图2中的白色连衣裙，保持图1女生的发型、表情和姿势不变"
          }
        ]
      }
    ]
  }'
```

### 3.2 姿势迁移（3图融合）

综合多张图片的元素：

```bash
curl -X POST "https://llm.ai-nebula.com/v1/images/generations" \
  -H "Authorization: Bearer sk-xxxxxxxxxx" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen-image-edit-plus",
    "prompt": "图1中的女生穿着图2中的黑色裙子按图3的姿势坐下，保持其服装、发型和表情不变，动作自然流畅",
    "parameters": {
      "n": 3,
      "negative_prompt": "扭曲、不协调、低质量",
      "watermark": false
    },
    "contents": [
      {
        "role": "user",
        "parts": [
          {
            "image": "https://example.com/person.jpg"
          },
          {
            "image": "https://example.com/skirt.jpg"
          },
          {
            "image": "https://example.com/pose.jpg"
          },
          {
            "text": "图1中的女生穿着图2中的黑色裙子按图3的姿势坐下，保持其服装、发型和表情不变，动作自然流畅"
          }
        ]
      }
    ]
  }'
```

### 3.3 场景融合

将多张图片的元素融合到一个场景中：

```bash
curl -X POST "https://llm.ai-nebula.com/v1/images/generations" \
  -H "Authorization: Bearer sk-xxxxxxxxxx" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen-image-edit-plus",
    "prompt": "将图1的建筑、图2的天空和图3的前景花园融合成一张完整的风景照，确保光线和色调协调一致",
    "parameters": {
      "n": 4,
      "negative_prompt": "接缝明显、色彩不匹配、不自然",
      "watermark": false
    },
    "contents": [
      {
        "role": "user",
        "parts": [
          {
            "image": "https://example.com/building.jpg"
          },
          {
            "image": "https://example.com/sky.jpg"
          },
          {
            "image": "https://example.com/garden.jpg"
          },
          {
            "text": "将图1的建筑、图2的天空和图3的前景花园融合成一张完整的风景照，确保光线和色调协调一致"
          }
        ]
      }
    ]
  }'
```

## 4. 高级功能

### 4.1 使用负面提示词

通过负面提示词排除不想要的效果：

```bash
curl -X POST "https://llm.ai-nebula.com/v1/images/generations" \
  -H "Authorization: Bearer sk-xxxxxxxxxx" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen-image-edit-plus",
    "prompt": "将这张肖像照转换为油画风格",
    "parameters": {
      "n": 2,
      "negative_prompt": "低分辨率、错误、最差质量、低质量、残缺、多余的手指、比例不良、过度渲染、色彩失真",
      "watermark": false
    },
    "contents": [
      {
        "role": "user",
        "parts": [
          {
            "image": "https://example.com/portrait.jpg"
          },
          {
            "text": "将这张肖像照转换为油画风格"
          }
        ]
      }
    ]
  }'
```

**负面提示词常用场景**：
- 人物编辑：`"扭曲、变形、多余的肢体、错误的比例、不协调"`
- 风格迁移：`"过度渲染、失真、色彩不匹配、不自然"`
- 文字编辑：`"模糊的文字、错别字、扭曲的字体、不清晰的笔画"`
- 物体编辑：`"不自然、违和感、接缝明显、阴影错误"`

### 4.2 随机种子（Seed）

使用相同的 seed 参数可使生成内容保持相对稳定：

```bash
curl -X POST "https://llm.ai-nebula.com/v1/images/generations" \
  -H "Authorization: Bearer sk-xxxxxxxxxx" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen-image-edit-plus",
    "prompt": "在图片中添加一只蝴蝶",
    "parameters": {
      "n": 1,
      "negative_prompt": "",
      "watermark": false,
      "seed": 123456
    },
    "contents": [
      {
        "role": "user",
        "parts": [
          {
            "image": "https://example.com/flower.jpg"
          },
          {
            "text": "在图片中添加一只蝴蝶"
          }
        ]
      }
    ]
  }'
```

**注意**：
- Seed 取值范围：`[0, 2147483647]`
- 使用相同的 seed 可以获得相似（但不保证完全相同）的结果
- 如果不提供 seed，算法将自动使用随机数种子

### 4.3 批量生成（n 参数）

一次请求生成多张变体：

```bash
curl -X POST "https://llm.ai-nebula.com/v1/images/generations" \
  -H "Authorization: Bearer sk-xxxxxxxxxx" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen-image-edit-plus",
    "prompt": "将这张白天的照片改成夜晚场景",
    "parameters": {
      "n": 6,
      "negative_prompt": "不自然的光线、过暗",
      "watermark": false
    },
    "contents": [
      {
        "role": "user",
        "parts": [
          {
            "image": "https://example.com/daytime.jpg"
          },
          {
            "text": "将这张白天的照片改成夜晚场景"
          }
        ]
      }
    ]
  }'
```

- **n 参数范围**: 1-6张
- **计费说明**: 按生成的图片数量计费，生成6张图片 = 6张 × ¥0.18 = ¥1.08

## 5. 响应处理说明

### 5.1 响应格式

成功响应（状态码 200）会返回包含图像数据的 JSON：

```json
{
  "code": 200,
  "msg": "操作成功",
  "data": {
    "data": [
      {
        "url": "https://dashscope-result-sz.oss-cn-shenzhen.aliyuncs.com/xxx.png?Expires=xxx",
        "b64_json": "",
        "revised_prompt": ""
      },
      {
        "url": "https://dashscope-result-sz.oss-cn-shenzhen.aliyuncs.com/yyy.png?Expires=xxx",
        "b64_json": "",
        "revised_prompt": ""
      }
    ],
    "created": 1762415000,
    "metadata": {
      "output": {
        "choices": [
          {
            "finish_reason": "stop",
            "message": {
              "content": [
                { "image": "https://..." },
                { "image": "https://..." }
              ],
              "role": "assistant"
            }
          }
        ]
      },
      "usage": {
        "width": 1248,
        "image_count": 2,
        "height": 832
      },
      "request_id": "bf37ca26-0abe-98e4-8065-xxxxxx"
    }
  }
}
```

### 5.2 响应字段说明

| 字段路径 | 类型 | 说明 |
|---------|------|------|
| code | int | 响应状态码，200表示成功 |
| msg | string | 响应消息 |
| data.data[] | array | 生成的图片数组 |
| data.data[].url | string | 图片URL地址（有效期约24小时） |
| data.created | int | 生成时间戳 |
| data.metadata | object | 厂家原始响应数据 |
| data.metadata.usage.image_count | int | 生成的图片数量 |
| data.metadata.usage.width | int | 图片宽度 |
| data.metadata.usage.height | int | 图片高度 |

### 5.3 错误处理

如果请求失败，会返回错误信息：

```json
{
  "code": 400,
  "msg": "参数错误：图片数量超过限制",
  "data": null
}
```

常见错误码：
- `400`：参数错误（如图片格式错误、尺寸超限等）
- `401`：认证失败（API密钥无效或过期）
- `403`：权限不足或余额不足
- `429`：请求过于频繁，超出速率限制
- `500`：服务器内部错误

## 6. 最佳实践

### 6.1 多图编辑提示词技巧

#### 明确图片标记

在处理多张图片时，**必须**使用"图1"、"图2"、"图3"明确指代：

✅ **好的提示词**（明确标记）：
```
"图1中的女生穿着图2中的黑色裙子按图3的姿势坐下"
```

❌ **不好的提示词**（缺少标记）：
```
"女生穿着黑色裙子坐下"
```

#### 详细描述期望效果

提供详细的描述可以获得更好的编辑效果：

✅ **好的提示词**：
```
"图1中的女生穿着图2中的黑色裙子按图3的姿势坐下，保持其服装、发型和表情不变，动作自然流畅，光线和阴影保持一致"
```

❌ **简单的提示词**：
```
"图1的女生换衣服"
```

### 6.2 图片输入建议

#### 图片质量
- ✅ 使用高分辨率、清晰的图片
- ✅ 确保光线充足、对比度适中
- ❌ 避免模糊、过暗或过亮的图片

#### 图片内容
- ✅ 主体明确、背景简洁
- ✅ 构图合理、比例正确
- ❌ 避免过于复杂的场景

#### 图片尺寸
- **最小**: 384×384 像素
- **最大**: 3072×3072 像素
- **推荐**: 1024×1024 至 2048×2048 像素

### 6.3 负面提示词策略

根据不同的编辑类型使用合适的负面提示词：

| 编辑类型 | 推荐负面提示词 |
|---------|--------------|
| 人物编辑 | "扭曲、变形、多余的肢体、错误的比例、不协调、面部扭曲" |
| 风格迁移 | "过度渲染、失真、色彩不匹配、不自然、画风不统一" |
| 文字编辑 | "模糊的文字、错别字、扭曲的字体、不清晰的笔画、重叠文字" |
| 物体编辑 | "不自然、违和感、接缝明显、阴影错误、透视不对" |
| 多图融合 | "不协调、接缝明显、光线不匹配、色调不一致、拼接痕迹" |

### 6.4 参数组合建议

| 使用场景 | n | negative_prompt | watermark | seed |
|---------|---|----------------|-----------|------|
| 快速测试 | 1-2 | "" | false | 不设置 |
| 高质量输出 | 3-4 | 详细描述 | false | 不设置 |
| 批量生成 | 6 | 简要描述 | false | 不设置 |
| 稳定复现 | 1 | 详细描述 | false | 固定值 |

## 7. 计费说明

### 7.1 计费方式

qwen-image-edit-plus 采用**按张计费**模式：

- **官方价格**：¥0.20/张
- **系统价格**：¥0.18/张（9折优惠）
- **计费单位**：按生成成功的图片数量计费
- **价格统一**：所有输出尺寸价格相同

### 7.2 计费示例

```
生成2张图片：2张 × ¥0.18 = ¥0.36
生成6张图片：6张 × ¥0.18 = ¥1.08
```

### 7.3 计费日志格式

成功生成后，系统会记录计费信息：

```json
{
  "content": "图片生成：2张 × ¥0.18 = ¥0.36",
  "modelName": "qwen-image-edit-plus",
  "quota": 12329,
  "quotaDollar": "0.049316"
}
```

## 8. 常见问题

### Q1: qwen-image-edit-plus 支持纯文生图吗？

A: **不支持**。qwen-image-edit-plus 是图像编辑模型，必须提供至少 1 张输入图片。如需纯文生图功能，请使用 `qwen-image-plus` 或 `doubao-seedream-4-0-250828`。

### Q2: 最多可以上传几张图片？

A: 最多支持 **3 张图片**同时输入。单图编辑使用 1 张，多图融合使用 2-3 张。

### Q3: 一次可以生成几张图片？

A: 通过 `n` 参数可以控制输出数量，范围是 **1-6 张**。

### Q4: 生成的图片有效期是多久？

A: 图片URL的有效期约为**24小时**。建议在收到响应后立即下载保存，或上传到您自己的存储服务。

### Q5: 多图编辑时如何指代不同的图片？

A: **必须**在提示词中使用"图1"、"图2"、"图3"明确指代对应的图片，例如：

```
"图1中的女生穿着图2中的黑色裙子按图3的姿势坐下"
```

如果不明确标记，可能会出现不符合预期的编辑结果。

### Q6: 图片输入支持哪些格式？

A: 支持 **URL** 和 **Base64 编码**两种方式：
- URL：直接提供图片的 HTTP/HTTPS 链接
- Base64：格式为 `data:image/{type};base64,{data}`

### Q7: 如何提高编辑质量？

A: 建议：
1. 使用高分辨率、清晰的输入图片
2. 提供详细、具体的编辑指令
3. 多图编辑时明确使用"图1"、"图2"标记
4. 合理使用负面提示词排除不想要的效果
5. 适当调整 `n` 参数生成多个变体供选择

### Q8: seed 参数有什么用？

A: seed（随机种子）可以使生成结果保持相对稳定。使用相同的 seed、相同的输入和参数，可以获得相似（但不保证完全相同）的输出。适合用于：
- 复现特定效果
- A/B 测试
- 批量生产相似风格的图片

### Q9: 可以去除水印吗？

A: 可以，设置 `watermark: false` 即可。**注意**：根据阿里云服务条款，去除水印可能有使用限制，请遵守相关规定。

### Q10: 支持哪些图片格式输出？

A: qwen-image-edit-plus 返回的图片URL指向 **PNG 格式**的图片，保证高质量输出。输出图片的长宽比与输入图片保持一致。

## 9. 完整请求示例

### 示例1：单图编辑 - 修改文字（Python）

```python
import requests
import json

url = "https://llm.ai-nebula.com/v1/images/generations"
headers = {
    "Authorization": "Bearer sk-xxxxxxxxxx",
    "Content-Type": "application/json"
}

payload = {
    "model": "qwen-image-edit-plus",
    "prompt": "将图片中的\"Open\"改成\"开门\"，保持其他内容不变",
    "parameters": {
        "n": 2,
        "negative_prompt": "模糊的文字、错别字、扭曲的字体",
        "watermark": False
    },
    "contents": [
        {
            "role": "user",
            "parts": [
                {
                    "image": "https://example.com/sign.jpg"
                },
                {
                    "text": "将图片中的\"Open\"改成\"开门\"，保持其他内容不变"
                }
            ]
        }
    ]
}

response = requests.post(url, headers=headers, json=payload)
result = response.json()

if result["code"] == 200:
    images = result["data"]["data"]
    print(f"成功生成 {len(images)} 张图片：")
    for i, img in enumerate(images):
        print(f"图片 {i+1}: {img['url']}")
        
        # 下载图片
        import urllib.request
        urllib.request.urlretrieve(img['url'], f"output_{i+1}.png")
    
    print("所有图片已保存")
else:
    print(f"生成失败：{result['msg']}")
```

### 示例2：多图融合 - 服装迁移（Node.js）

```javascript
const axios = require('axios');
const fs = require('fs');
const https = require('https');

async function editImage() {
  const response = await axios.post(
    'https://llm.ai-nebula.com/v1/images/generations',
    {
      model: 'qwen-image-edit-plus',
      prompt: '图1中的女生穿上图2中的红色连衣裙，保持图1女生的发型、表情和姿势不变',
      parameters: {
        n: 3,
        negative_prompt: '不自然、比例失调、色彩不匹配',
        watermark: false
      },
      contents: [
        {
          role: 'user',
          parts: [
            { image: 'https://example.com/person.jpg' },
            { image: 'https://example.com/dress.jpg' },
            { text: '图1中的女生穿上图2中的红色连衣裙，保持图1女生的发型、表情和姿势不变' }
          ]
        }
      ]
    },
    {
      headers: {
        'Authorization': 'Bearer sk-xxxxxxxxxx',
        'Content-Type': 'application/json'
      }
    }
  );

  if (response.data.code === 200) {
    const images = response.data.data.data;
    console.log(`成功生成 ${images.length} 张图片：`);
    
    // 下载所有图片
    images.forEach((img, index) => {
      const file = fs.createWriteStream(`output_${index + 1}.png`);
      https.get(img.url, (response) => {
        response.pipe(file);
        file.on('finish', () => {
          file.close();
          console.log(`图片 ${index + 1} 已保存`);
        });
      });
    });
  } else {
    console.log(`生成失败：${response.data.msg}`);
  }
}

editImage();
```

### 示例3：Base64 图片输入（Python）

```python
import requests
import base64
import json

# 读取本地图片并转换为Base64
def image_to_base64(image_path):
    with open(image_path, "rb") as image_file:
        encoded_string = base64.b64encode(image_file.read()).decode('utf-8')
    
    # 获取MIME类型
    if image_path.lower().endswith('.jpg') or image_path.lower().endswith('.jpeg'):
        mime_type = 'image/jpeg'
    elif image_path.lower().endswith('.png'):
        mime_type = 'image/png'
    elif image_path.lower().endswith('.webp'):
        mime_type = 'image/webp'
    else:
        mime_type = 'image/jpeg'  # 默认
    
    return f"data:{mime_type};base64,{encoded_string}"

url = "https://llm.ai-nebula.com/v1/images/generations"
headers = {
    "Authorization": "Bearer sk-xxxxxxxxxx",
    "Content-Type": "application/json"
}

# 转换本地图片为Base64
image_base64 = image_to_base64("local_image.jpg")

payload = {
    "model": "qwen-image-edit-plus",
    "prompt": "将这张照片的背景模糊化，突出前景人物",
    "parameters": {
        "n": 2,
        "negative_prompt": "过度模糊、失焦",
        "watermark": False
    },
    "contents": [
        {
            "role": "user",
            "parts": [
                {
                    "image": image_base64
                },
                {
                    "text": "将这张照片的背景模糊化，突出前景人物"
                }
            ]
        }
    ]
}

response = requests.post(url, headers=headers, json=payload)
result = response.json()

if result["code"] == 200:
    print(f"成功生成 {len(result['data']['data'])} 张图片")
else:
    print(f"生成失败：{result['msg']}")
```

## 10. 性能优化建议

### 10.1 图片预处理

在上传图片前进行适当的预处理可以提高编辑效果：

1. **压缩图片**: 将大图压缩到合理尺寸（推荐1024-2048像素）
2. **调整亮度**: 确保图片光线充足，对比度适中
3. **裁剪无关内容**: 移除不必要的背景或边缘

### 10.2 提示词优化

1. **明确具体**: 避免模糊的描述，给出具体的编辑要求
2. **分步描述**: 复杂编辑可以分解为多个简单的步骤
3. **保留原有特征**: 明确说明需要保留哪些原始特征

### 10.3 批量生成策略

如需生成大量图片：

1. **并发请求**: 同时发起多个独立的请求
2. **使用队列**: 实现请求队列管理，避免超出速率限制
3. **错误重试**: 实现自动重试机制处理临时失败

```bash
# 并发发起3个请求
for i in {1..3}; do
  curl -X POST "https://llm.ai-nebula.com/v1/images/generations" \
    -H "Authorization: Bearer sk-xxxxxxxxxx" \
    -H "Content-Type: application/json" \
    -d '{...}' &
done
wait
```

## 11. 快速参考

### 模型参数速查

| 参数 | 值 |
|------|-----|
| 模型名称 | qwen-image-edit-plus, qwen-image-edit-plus-2025-10-30 |
| 支持功能 | 图像编辑（图生图） |
| 图片输入 | 1-3张 (URL或Base64) |
| 图片输出 | 1-6张 |
| 输入格式 | JPG, JPEG, PNG, BMP, TIFF, WEBP |
| 输出格式 | PNG |
| 分辨率限制 | 384-3072 像素 |
| 文件大小限制 | ≤10MB |
| 响应格式 | URL（有效期约24小时） |
| 计费方式 | 按张计费（¥0.18/张） |

### 核心功能速查

| 功能 | 支持 | 说明 |
|------|------|------|
| 单图编辑 | ✅ | 1张输入图片 |
| 多图融合 | ✅ | 2-3张输入图片 |
| 修改文字 | ✅ | 精确修改图内文字 |
| 增删物体 | ✅ | 添加或删除物体 |
| 改变动作 | ✅ | 修改人物/动物姿态 |
| 风格迁移 | ✅ | 改变艺术风格 |
| 负面提示词 | ✅ | 排除不想要的元素 |
| 随机种子 | ✅ | 控制生成稳定性 |
| 纯文生图 | ❌ | 必须有图片输入 |

### 参数组合速查

| 场景 | 图片数 | n | negative_prompt | watermark | seed |
|------|-------|---|----------------|-----------|------|
| 单图编辑 | 1 | 2-3 | 简要描述 | false | 不设置 |
| 多图融合 | 2-3 | 2-4 | 详细描述 | false | 不设置 |
| 高质量输出 | 1-3 | 3-4 | 详细描述 | false | 不设置 |
| 稳定复现 | 1-3 | 1 | 详细描述 | false | 固定值 |
| 批量测试 | 1-3 | 6 | 简要描述 | false | 不设置 |

---

**文档版本**: v1.0  
**更新时间**: 2025-11-06  
**模型**: 通义千问图像编辑模型 (qwen-image-edit-plus)  
**技术支持**: https://llm.ai-nebula.com

---

## 技术支持

如有任何问题，请联系：
- 文档地址：https://llm.ai-nebula.com/docs
- 技术支持：support@ai-nebula.com

