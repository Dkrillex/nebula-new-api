# 通义千问图像生成接口文档

## 1. 接口基础信息

- **模型名称**: qwen-image-plus (通义千问图像生成增强版)
- **基础URL**: `https://llm.ai-nebula.com/v1/images/generations`
- **认证方式**: Bearer Token
- **认证令牌**: `Bearer sk-xxxxxxxxxx`
- **核心能力**: 
  - ✅ 文生图（纯文本描述生成图片）
  - ✅ 中英文文本渲染（擅长在图片中生成复杂文字）
  - ✅ 多种艺术风格
  - ✅ 提示词智能扩展
  - ❌ 不支持图生图
- **支持的尺寸**: 1328×1328、1664×928、928×1664、1472×1140、1140×1472
- **计费方式**: 按张计费（¥0.18/张,官方¥0.2/张）
 - **官方文档**: [阿里百炼模型接口文档（qwen-image-plus）](https://bailian.console.aliyun.com/?spm=5176.29597918.J_SEsSjsNv72yRuRFS2VknO.2.80d27b08ALuu1L&tab=api#/api/?type=model&url=2975126)

### 1.1 核心参数说明

| 参数名 | 类型 | 必填 | 说明 | 示例值 |
|--------|------|------|------|--------|
| model | string | 是 | 模型名称 | qwen-image-plus |
| prompt | string | 是 | 文本提示词 | "一只可爱的橙色小猫" |
| parameters | object | 是 | 生成参数 | 见下方说明 |

### 1.2 请求参数结构

qwen-image-plus 支持两种参数格式：

#### 格式1：简化格式（推荐）

直接平铺参数，更简洁：

```json
{
  "model": "qwen-image-plus",
  "prompt": "您的提示词内容",
  "parameters": {
    "size": "1328*1328",           // 图片尺寸（必填）
    "negative_prompt": "",          // 负面提示词（可选）
    "prompt_extend": true,          // 提示词扩展（可选，默认true）
    "watermark": true               // 水印（可选，默认true）
  }
}
```

#### 格式2：完整格式

使用 `extra` 和 `input.messages` 的完整结构：

```json
{
  "model": "qwen-image-plus",
  "prompt": "您的提示词内容",
  "extra": {
    "input": {
      "messages": [
        {
          "role": "user",
          "content": [
            {
              "text": "您的提示词内容"
            }
          ]
        }
      ]
    },
    "parameters": {
      "size": "1328*1328",
      "negative_prompt": "",
      "prompt_extend": true,
      "watermark": true
    }
  }
}
```

**建议**：使用格式1（简化格式），更简洁易读。

### 1.3 支持的图片尺寸

| 尺寸 | 宽高比 | 适用场景 |
|------|--------|---------|
| 1328×1328 | 1:1 | 正方形图片、头像、社交媒体 |
| 1664×928 | 16:9 | 宽屏、视频封面、桌面壁纸 |
| 928×1664 | 9:16 | 手机竖屏、短视频封面 |
| 1472×1140 | 4:3 | 横屏照片 |
| 1140×1472 | 3:4 | 竖版照片、海报 |

## 2. 文生图功能

### 2.1 基础文生图（正方形 1:1）

生成一张正方形的图片：

```bash
curl -X POST "https://llm.ai-nebula.com/v1/images/generations" \
  -H "Authorization: Bearer sk-xxxxxxxxxx" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen-image-plus",
    "prompt": "一只可爱的橙色小猫坐在花园里，阳光明媚，高质量摄影",
    "parameters": {
      "size": "1328*1328",
      "negative_prompt": "",
      "prompt_extend": true,
      "watermark": true
    }
  }'
```

### 2.2 宽屏图片生成（16:9）

生成一张16:9宽屏比例的风景图：

```bash
curl -X POST "https://llm.ai-nebula.com/v1/images/generations" \
  -H "Authorization: Bearer sk-xxxxxxxxxx" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen-image-plus",
    "prompt": "壮丽的山脉日出景色，金色的阳光洒在雪山上，高质量摄影",
    "parameters": {
      "size": "1664*928",
      "negative_prompt": "",
      "prompt_extend": true,
      "watermark": true
    }
  }'
```

### 2.3 竖屏图片生成（9:16）

生成一张适合手机竖屏的海报：

```bash
curl -X POST "https://llm.ai-nebula.com/v1/images/generations" \
  -H "Authorization: Bearer sk-xxxxxxxxxx" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen-image-plus",
    "prompt": "时尚的产品海报，现代简约风格，垂直构图",
    "extra": {
      "parameters": {
        "size": "928*1664",
        "negative_prompt": "",
        "prompt_extend": true,
        "watermark": true
      }
    }
  }'
```

## 3. 高级功能

### 3.1 文本渲染（核心优势）

qwen-image-plus 特别擅长在图片中生成复杂文字，支持中英文文本、多行布局、段落级文本生成：

```bash
curl -X POST "https://llm.ai-nebula.com/v1/images/generations" \
  -H "Authorization: Bearer sk-xxxxxxxxxx" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen-image-plus",
    "prompt": "一副典雅庄重的对联悬挂于厅堂之中，房间是个安静古典的中式布置，桌子上放着一些青花瓷，对联上左书\"义本生知人机同道善思新\"，右书\"通云赋智乾坤启数高志远\"，横批\"智启通义\"，字体飘逸，中间挂着一幅中国风的画作，内容是岳阳楼。",
    "parameters": {
      "size": "1328*1328",
      "negative_prompt": "",
      "prompt_extend": false,
      "watermark": true
    }
  }'
```

**文本渲染应用场景**：
- 海报设计（带标题和说明文字）
- 对联、书法作品
- 产品包装设计（带品牌文字）
- 广告图（带广告语）
- 教育图表（带标注文字）

### 3.2 负面提示词（Negative Prompt）

使用负面提示词来排除不想要的元素：

```bash
curl -X POST "https://llm.ai-nebula.com/v1/images/generations" \
  -H "Authorization: Bearer sk-xxxxxxxxxx" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen-image-plus",
    "prompt": "一位美丽的女性肖像照，专业摄影，高清画质",
    "parameters": {
      "size": "1140*1472",
      "negative_prompt": "模糊、低质量、扭曲、变形、水印、文字标记",
      "prompt_extend": true,
      "watermark": true
    }
  }'
```

**负面提示词常用场景**：
- 排除低质量效果：`"模糊、噪点、低分辨率、像素化"`
- 排除特定元素：`"水印、文字、logo、版权标识"`
- 排除风格：`"卡通、动漫、抽象、写实"`
- 排除缺陷：`"扭曲、变形、多余的肢体、错误的比例"`

### 3.3 提示词扩展

通过 `prompt_extend` 参数控制是否自动扩展和优化提示词：

#### 启用提示词扩展（推荐）

```bash
curl -X POST "https://llm.ai-nebula.com/v1/images/generations" \
  -H "Authorization: Bearer sk-xxxxxxxxxx" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen-image-plus",
    "prompt": "夕阳下的城市",
    "parameters": {
      "size": "1664*928",
      "negative_prompt": "",
      "prompt_extend": true,
      "watermark": true
    }
  }'
```

系统会自动将"夕阳下的城市"扩展为更详细的描述，提升生成效果。

#### 禁用提示词扩展

如果您的提示词已经非常详细，可以关闭扩展：

```bash
curl -X POST "https://llm.ai-nebula.com/v1/images/generations" \
  -H "Authorization: Bearer sk-xxxxxxxxxx" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen-image-plus",
    "prompt": "一座现代化的城市天际线，在温暖的金色夕阳照耀下，高楼大厦的玻璃幕墙反射着橙红色的光芒，远处有山脉的轮廓，天空中有几朵云彩，色调温暖，氛围宁静，专业摄影，高质量，8k分辨率",
    "parameters": {
      "size": "1664*928",
      "negative_prompt": "",
      "prompt_extend": false,
      "watermark": true
    }
  }'
```

**提示词扩展建议**：
- ✅ 简短提示词：启用扩展（prompt_extend: true）
- ❌ 详细提示词：关闭扩展（prompt_extend: false）
- ✅ 需要精确控制：关闭扩展
- ✅ 快速创作：启用扩展

### 3.4 水印控制

通过 `watermark` 参数控制是否在生成的图片上添加水印：

```bash
curl -X POST "https://llm.ai-nebula.com/v1/images/generations" \
  -H "Authorization: Bearer sk-xxxxxxxxxx" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen-image-plus",
    "prompt": "科技感十足的未来城市，夜景",
    "parameters": {
      "size": "1664*928",
      "negative_prompt": "",
      "prompt_extend": true,
      "watermark": false
    }
  }'
```

**注意**：关闭水印可能违反某些使用条款，请谨慎选择。

## 4. 响应处理说明

### 4.1 响应格式

成功响应（状态码 200）会返回包含图像数据的 JSON：

```json
{
  "code": 200,
  "msg": "操作成功",
  "data": {
    "data": [
      {
        "url": "https://dashscope-result-wlcb-acdr-1.oss-cn-wulanchabu-acdr-1.aliyuncs.com/...",
        "b64_json": "",
        "revised_prompt": ""
      }
    ],
    "created": 1762411334,
    "metadata": {
      "output": {
        "choices": [
          {
            "finish_reason": "stop",
            "message": {
              "content": [
                {
                  "image": "https://..."
                }
              ],
              "role": "assistant"
            }
          }
        ],
        "task_metric": {
          "FAILED": 0,
          "SUCCEEDED": 1,
          "TOTAL": 1
        }
      },
      "request_id": "c9c7658e-a78e-4f3d-b59c-c25ab83c833b",
      "usage": {
        "height": 1328,
        "image_count": 1,
        "width": 1328
      }
    }
  }
}
```

### 4.2 响应字段说明

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

### 4.3 错误处理

如果请求失败，会返回错误信息：

```json
{
  "code": 400,
  "msg": "参数错误：不支持的图片尺寸",
  "data": null
}
```

常见错误码：
- `400`：参数错误（如模型名称错误、尺寸格式错误等）
- `401`：认证失败（API密钥无效或过期）
- `403`：权限不足或余额不足
- `429`：请求过于频繁，超出速率限制
- `500`：服务器内部错误

## 5. 最佳实践

### 5.1 尺寸选择建议

- **社交媒体**：
  - 微信朋友圈：1328×1328 (1:1)
  - 微博头图：1664×928 (16:9)
  - 抖音封面：928×1664 (9:16)
  - 小红书：1140×1472 (3:4)

- **设计用途**：
  - 网站横幅：1664×928 (16:9)
  - 海报：1140×1472 (3:4) 或 928×1664 (9:16)
  - 产品图片：1328×1328 (1:1) 或 1472×1140 (4:3)
  - 手机壁纸：928×1664 (9:16)

- **视频相关**：
  - 视频封面：1664×928 (16:9)
  - 短视频封面：928×1664 (9:16)

### 5.2 提示词优化建议

#### 文本渲染提示词技巧

qwen-image-plus 的核心优势是**复杂文本渲染**，在提示词中明确说明文字内容：

✅ **好的提示词**（明确文字内容）：
```
"一张海报，标题是'夏日特惠'，副标题'全场5折起'，背景是清新的海滩场景"
```

❌ **不好的提示词**（模糊描述）：
```
"一张促销海报"
```

✅ **好的提示词**（详细文字布局）：
```
"一副中式对联，左联'春回大地千山秀'，右联'日照神州百业兴'，横批'万象更新'，背景是红色宣纸，字体是楷体金色"
```

❌ **不好的提示词**（缺少文字）：
```
"一副中式对联"
```

#### 通用提示词技巧

1. **明确构图方向**：
   - 横屏：使用"横向构图"、"宽屏视角"
   - 竖屏：使用"竖向构图"、"垂直视角"

2. **添加高质量关键词**：
   - "高质量"、"高清"、"专业摄影"
   - "8k分辨率"、"细节丰富"

3. **指定艺术风格**：
   - "写实风格"、"油画风格"、"水彩画"
   - "中国风"、"赛博朋克"、"现代简约"

4. **描述光线和氛围**：
   - "柔和的光线"、"戏剧性的光影"
   - "温暖的色调"、"冷色调"

### 5.3 负面提示词使用技巧

合理使用负面提示词可以显著提升生成质量：

#### 场景1：人物肖像
```json
{
  "negative_prompt": "模糊、低质量、扭曲、变形、多余的肢体、错误的比例、丑陋"
}
```

#### 场景2：风景照片
```json
{
  "negative_prompt": "模糊、噪点、过度曝光、欠曝、低分辨率、水印"
}
```

#### 场景3：产品图
```json
{
  "negative_prompt": "模糊、阴影过重、反光过度、颜色失真、背景杂乱"
}
```

#### 场景4：文字渲染
```json
{
  "negative_prompt": "模糊的文字、错别字、扭曲的字体、不清晰的笔画"
}
```

### 5.4 参数组合建议

| 使用场景 | size | prompt_extend | watermark | negative_prompt |
|---------|------|--------------|-----------|----------------|
| 快速创作 | 1328×1328 | true | true | "" |
| 精确控制 | 按需 | false | false | 详细描述 |
| 文本渲染 | 1328×1328 | false | true | "模糊的文字" |
| 高质量输出 | 1664×928 | false | false | "模糊、低质量、噪点" |
| 海报设计 | 928×1664 | true | false | "杂乱、低质量" |

## 6. 计费说明

### 6.1 计费方式

qwen-image-plus 采用**按张计费**模式：

- **官方价格**：¥0.20/张
- **系统价格**：¥0.18/张（9折优惠）
- **计费单位**：按生成成功的图片数量计费
- **价格统一**：所有尺寸价格相同

### 6.2 计费示例

```
生成1张图片：1张 × ¥0.18 = ¥0.18
生成5张图片：5张 × ¥0.18 = ¥0.90
```

### 6.3 计费日志格式

成功生成后，系统会记录计费信息：

```json
{
  "content": "图片生成：1张 × ¥0.18 = ¥0.18",
  "modelName": "qwen-image-plus",
  "quota": 12329,
  "quotaDollar": "0.024658"
}
```

## 7. 常见问题

### Q1: qwen-image-plus 支持图生图吗？

A: **不支持**。qwen-image-plus 是纯文生图模型，只支持通过文本描述生成图片，不支持上传参考图片。如需图生图功能，请使用 `gemini-2.5-flash-image` 或 `doubao-seedream-4-0-250828`。

### Q2: 如何生成带有中文文字的图片？

A: qwen-image-plus 特别擅长中英文文本渲染，只需在提示词中明确说明文字内容：

```json
{
  "prompt": "一张海报，标题是'双十一大促'，副标题'全场5折起'，背景是红色渐变"
}
```

模型会自动在生成的图片中渲染这些文字。

### Q3: 生成的图片有效期是多久？

A: 图片URL的有效期约为**24小时**。建议在收到响应后立即下载保存，或上传到您自己的存储服务。

### Q4: 可以一次生成多张图片吗？

A: 当前版本 qwen-image-plus **每次请求生成1张图片**。如需生成多张图片，请发起多个并发请求。

### Q5: 提示词扩展和负面提示词可以同时使用吗？

A: 可以！两者互不冲突：
- `prompt_extend: true` - 系统会扩展您的正面提示词
- `negative_prompt: "..."` - 同时应用负面约束

建议：简短提示词 + 启用扩展 + 添加负面提示词，效果最佳。

### Q6: 如何确保生成的图片没有水印？

A: 设置 `watermark: false`：

```json
{
  "parameters": {
    "watermark": false
  }
}
```

**注意**：根据阿里云服务条款，去除水印可能有使用限制，请遵守相关规定。

### Q7: 支持哪些图片格式输出？

A: qwen-image-plus 返回的图片URL指向 **PNG 格式**的图片，保证高质量输出。

### Q8: 图片生成需要多长时间？

A: 通常在 **5-15秒** 内完成，具体时间取决于：
- 提示词复杂度
- 图片尺寸
- 服务器负载

## 8. 完整请求示例

### 示例1：带文字的产品海报

```bash
curl -X POST "https://llm.ai-nebula.com/v1/images/generations" \
  -H "Authorization: Bearer sk-xxxxxxxxxx" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen-image-plus",
    "prompt": "一张咖啡产品海报，顶部大标题\"匠心咖啡\"，副标题\"醇香每一刻\"，中间是一杯冒着热气的拿铁咖啡，背景是咖啡豆，暖色调，高级质感",
    "parameters": {
      "size": "928*1664",
      "negative_prompt": "模糊、低质量、杂乱",
      "prompt_extend": false,
      "watermark": false
    }
  }'
```

### 示例2：中国风书法对联

```bash
curl -X POST "https://llm.ai-nebula.com/v1/images/generations" \
  -H "Authorization: Bearer sk-xxxxxxxxxx" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen-image-plus",
    "prompt": "一幅精美的中式对联，左联\"春风化雨润桃李\"，右联\"丹心一片育栋梁\"，横批\"师恩难忘\"，字体是行楷，墨色浓郁，背景是淡雅的宣纸纹理，边框有梅花图案",
    "parameters": {
      "size": "1140*1472",
      "negative_prompt": "模糊的文字、扭曲的字体、错别字",
      "prompt_extend": false,
      "watermark": true
    }
  }'
```

### 示例3：简短提示词 + 自动扩展

```bash
curl -X POST "https://llm.ai-nebula.com/v1/images/generations" \
  -H "Authorization: Bearer sk-xxxxxxxxxx" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen-image-plus",
    "prompt": "赛博朋克城市",
    "parameters": {
      "size": "1664*928",
      "negative_prompt": "模糊、低质量",
      "prompt_extend": true,
      "watermark": true
    }
  }'
```

系统会自动将"赛博朋克城市"扩展为详细描述，提升生成效果。

## 9. 性能优化建议

### 9.1 批量生成

如需生成多张图片，建议并发请求以提高效率：

```bash
# 同时发起3个请求
for i in {1..3}; do
  curl -X POST "https://llm.ai-nebula.com/v1/images/generations" \
    -H "Authorization: Bearer sk-xxxxxxxxxx" \
    -H "Content-Type: application/json" \
    -d '{...}' &
done
wait
```

### 9.2 提示词优化策略

1. **简短提示词**：启用 `prompt_extend: true`，让系统自动优化
2. **详细提示词**：关闭 `prompt_extend: false`，保持原始描述
3. **文本渲染**：在提示词中用引号明确标注文字内容
4. **添加负面词**：排除常见缺陷（模糊、低质量等）

### 9.3 缓存策略

对于相同参数的请求，建议在客户端进行缓存：
- 缓存键：`${prompt}_${size}_${negative_prompt}_${prompt_extend}`
- 缓存时间：根据业务需求，建议1-24小时

## 10. 快速参考

### 模型参数速查

| 参数 | 值 |
|------|-----|
| 模型名称 | qwen-image-plus |
| 支持功能 | 文生图（纯文本生成） |
| 核心优势 | 复杂文本渲染（中英文） |
| 支持尺寸 | 1328×1328, 1664×928, 928×1664, 1472×1140, 1140×1472 |
| 图片输入 | ❌ 不支持 |
| 响应格式 | URL（有效期约24小时） |
| 计费方式 | 按张计费（¥0.18/张） |

### 核心功能速查

| 功能 | 支持 | 说明 |
|------|------|------|
| 文生图 | ✅ | 纯文本描述生成图片 |
| 文本渲染 | ✅ | 擅长中英文文字渲染 |
| 提示词扩展 | ✅ | 可选的智能扩展 |
| 负面提示词 | ✅ | 排除不想要的元素 |
| 水印控制 | ✅ | 可选择是否添加水印 |
| 图生图 | ❌ | 不支持 |
| 多图融合 | ❌ | 不支持 |
| 对话上下文 | ❌ | 不支持 |

### 常用尺寸速查

| 尺寸 | 比例 | 场景 |
|------|------|------|
| 1328×1328 | 1:1 | 社交媒体、头像 |
| 1664×928 | 16:9 | 视频封面、横屏 |
| 928×1664 | 9:16 | 短视频、手机壁纸 |
| 1472×1140 | 4:3 | 横屏照片 |
| 1140×1472 | 3:4 | 竖版海报 |

### 参数组合速查

| 场景 | size | prompt_extend | watermark | negative_prompt |
|------|------|--------------|-----------|----------------|
| 快速创作 | 1328×1328 | true | true | "" |
| 文本海报 | 928×1664 | false | false | "模糊的文字" |
| 高质量照片 | 1664×928 | false | false | "模糊、低质量、噪点" |
| 对联书法 | 1140×1472 | false | true | "模糊的文字、错别字" |

---

**文档版本**: v1.0  
**更新时间**: 2025-11-06  
**模型**: 通义千问图像生成增强版 (qwen-image-plus)  
**技术支持**: https://llm.ai-nebula.com

---

## 附录：完整请求示例（Python）

```python
import requests
import json

url = "https://llm.ai-nebula.com/v1/images/generations"
headers = {
    "Authorization": "Bearer sk-xxxxxxxxxx",
    "Content-Type": "application/json"
}

payload = {
    "model": "qwen-image-plus",
    "prompt": "一只可爱的橙色小猫坐在花园里，阳光明媚，高质量摄影",
    "parameters": {
        "size": "1328*1328",
        "negative_prompt": "模糊、低质量",
        "prompt_extend": True,
        "watermark": True
    }
}

response = requests.post(url, headers=headers, json=payload)
result = response.json()

if result["code"] == 200:
    image_url = result["data"]["data"][0]["url"]
    print(f"图片生成成功：{image_url}")
    
    # 下载图片
    import urllib.request
    urllib.request.urlretrieve(image_url, "output.png")
    print("图片已保存为 output.png")
else:
    print(f"生成失败：{result['msg']}")
```

## 附录：完整请求示例（Node.js）

```javascript
const axios = require('axios');
const fs = require('fs');
const https = require('https');

async function generateImage() {
  const response = await axios.post(
    'https://llm.ai-nebula.com/v1/images/generations',
    {
      model: 'qwen-image-plus',
      prompt: '一只可爱的橙色小猫坐在花园里，阳光明媚，高质量摄影',
      parameters: {
        size: '1328*1328',
        negative_prompt: '模糊、低质量',
        prompt_extend: true,
        watermark: true
      }
    },
    {
      headers: {
        'Authorization': 'Bearer sk-xxxxxxxxxx',
        'Content-Type': 'application/json'
      }
    }
  );

  if (response.data.code === 200) {
    const imageUrl = response.data.data.data[0].url;
    console.log(`图片生成成功：${imageUrl}`);
    
    // 下载图片
    const file = fs.createWriteStream('output.png');
    https.get(imageUrl, (response) => {
      response.pipe(file);
      file.on('finish', () => {
        file.close();
        console.log('图片已保存为 output.png');
      });
    });
  } else {
    console.log(`生成失败：${response.data.msg}`);
  }
}

generateImage();
```

---

## 技术支持

如有任何问题，请联系：
- 文档地址：https://llm.ai-nebula.com/docs
- 技术支持：support@ai-nebula.com

