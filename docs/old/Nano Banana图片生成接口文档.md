## 1. 接口基础信息

- **模型名称**: gemini-2.5-flash-image (Nano Banana)
- **基础URL**: `https://llm.ai-nebula.com/v1/images/generations`
- **认证方式**: Bearer Token
- **认证令牌**: `Bearer sk-xxxxxxxxxx`
- **核心能力**: 
  - ✅ 文生图（纯文本描述生成图片）
  - ✅ 图生图（单图片+文本生成新图片）
  - ✅ 多图生一图（多张图片融合生成新图片）
  - ✅ 多轮对话式图片生成（上下文连续修改）
- **支持的宽高比**: 1:1、3:2、2:3、3:4、4:3、4:5、5:4、9:16、16:9、21:9
- **支持的图片格式**: PNG、JPEG、JPG、WEBP
- **图片大小限制**: 最大 7MB

### 1.1 核心参数说明

| 参数名 | 类型 | 必填 | 说明 | 示例值 |
|--------|------|------|------|--------|
| model | string | 是 | 模型名称 | gemini-2.5-flash-image |
| prompt | string | 否* | 文本提示词 | "一只可爱的橙色小猫" |
| contents | array | 否* | 多模态内容（支持上下文对话和图生图） | 见示例 |
| response_format | string | 否 | 响应格式：b64_json| b64_json |
| size | string | 否 | 图片宽高比（比例或像素） | "16:9" 或 "1792x1024" |

*注：prompt 和 contents 二选一，至少提供一个

**图片输入要求**：
- 支持格式：PNG、JPEG、JPG、WEBP
- 最大大小：7MB
- 输入方式：支持 **URL** 和 **Base64** 两种格式
  - URL格式：`"image": "https://example.com/image.jpg"`
  - Base64格式：`"image": "data:image/png;base64,iVBORw0..."`

### 1.2 宽高比设置方式

通过 `size` 参数设置图片宽高比，支持两种格式：

#### 格式1：直接使用比例（推荐）
```json
{
  "size": "16:9"
}
```

#### 格式2：使用像素尺寸（自动转换为对应比例）
```json
{
  "size": "1792x1024"  // 自动转换为 16:9
}
```

支持的像素尺寸及其对应比例，请参考下方的「1.3 支持的宽高比及对应像素尺寸」表格。

### 1.3 支持的宽高比及对应像素尺寸

| 宽高比 | 对应像素尺寸 | 适用场景 |
|--------|-------------|---------|
| 1:1 | 1024x1024 | 正方形图片、头像、社交媒体 |
| 3:2 | 1536x1024 | 标准摄影比例 |
| 2:3 | 1024x1536 | 竖版海报、手机壁纸 |
| 3:4 | 1536x2048 | 竖版照片 |
| 4:3 | 2048x1536 | 传统显示器比例 |
| 4:5 | 1024x1280 | Instagram 竖版 |
| 5:4 | 1280x1024 | 传统显示器比例 |
| 9:16 | 1024x1792 | 手机竖屏、短视频封面 |
| 16:9 | 1792x1024 | 宽屏、视频封面、桌面壁纸 |
| 21:9 | 1024x2176 | 超宽屏 |

## 2. 简单文生图功能

### 2.1 基础文生图（默认1:1比例）

生成一张"可爱的橙色小猫坐在花园里"的图片：

```bash
curl -X POST "https://llm.ai-nebula.com/v1/images/generations" \
  -H "Authorization: Bearer sk-Iia3/AM7jEGHAxxxxcWWEQz0J7SirQ=" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-2.5-flash-image",
    "prompt": "一只可爱的橙色小猫坐在花园里，阳光明媚，高质量摄影",
    "response_format": "b64_json"
  }'
```

### 2.2 指定宽高比的文生图（16:9 宽屏）

生成一张16:9宽屏比例的风景图：

```bash
curl -X POST "https://llm.ai-nebula.com/v1/images/generations" \
  -H "Authorization: Bearer sk-Iia3/AM7jEGxxxxWEQz0J7SirQ=" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-2.5-flash-image",
    "prompt": "壮丽的山脉日出景色，金色的阳光洒在雪山上，高质量摄影",
    "size": "16:9",
    "response_format": "b64_json"
  }'
```

### 2.3 竖屏比例文生图（9:16 手机屏幕）

生成一张适合手机竖屏的海报：

```bash
curl -X POST "https://llm.ai-nebula.com/v1/images/generations" \
  -H "Authorization: Bearer sk-Iia3/AM7jEGHAXnE3dcWWEQz0J7SirQ=" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-2.5-flash-image",
    "prompt": "时尚的产品海报，现代简约风格，垂直构图",
    "size": "9:16",
    "response_format": "b64_json"
  }'
```

### 2.4 使用像素尺寸指定比例

也可以直接使用像素尺寸，系统会自动转换为对应比例：

```bash
curl -X POST "https://llm.ai-nebula.com/v1/images/generations" \
  -H "Authorization: Bearer sk-Iia3/AM7jEGHAXnE3dcWWEQz0J7SirQ=" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-2.5-flash-image",
    "prompt": "科技感十足的未来城市，夜景",
    "size": "1792x1024",
    "response_format": "b64_json"
  }'
```

## 3. 图生图功能

### 3.1 基础图生图（默认比例）

基于基础图像生成新图片。**支持两种图片输入方式**：
- **方式1**：URL 地址（系统会自动下载图片）
- **方式2**：Base64 编码（需要带 data URI 前缀）

#### 示例1：使用 Base64 输入

```bash
curl -X POST "https://llm.ai-nebula.com/v1/images/generations" \
  -H "Authorization: Bearer sk-Iia3/AM7jEGxxxE3dcWWEQz0J7SirQ=" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-2.5-flash-image",
    "response_format": "b64_json",
    "contents": [
      {
        "role": "user",
        "parts": [
          {
            "text": "根据这张图帮我生成一张俯瞰广州塔的图片"
          },
          {
            "image": "data:image/png;base64,iVBORw0KGgoAAxxxx[已截断]"
          }
        ]
      }
    ]
  }'
```

#### 示例2：使用 URL 输入

```bash
curl -X POST "https://llm.ai-nebula.com/v1/images/generations" \
  -H "Authorization: Bearer sk-Iia3/AMxxxxxEQz0J7SirQ=" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-2.5-flash-image",
    "response_format": "b64_json",
    "contents": [
      {
        "role": "user",
        "parts": [
          {
            "text": "将这张图片转换为水彩画风格"
          },
          {
            "image": "https://example.com/original-image.jpg"
          }
        ]
      }
    ]
  }'
```

### 3.2 指定宽高比的图生图（21:9 超宽屏）

生成21:9超宽屏比例的图片（支持 Base64 和 URL 两种输入方式）：

#### 使用 URL 输入（推荐）

```bash
curl -X POST "https://llm.ai-nebula.com/v1/images/generations" \
  -H "Authorization: Bearer sk-Iia3/AM7xxxXnE3dcWWEQz0J7SirQ=" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-2.5-flash-image",
    "response_format": "b64_json",
    "size": "21:9",
    "contents": [
      {
        "role": "user",
        "parts": [
          {
            "text": "根据这张图生成超宽屏全景图，保留主体建筑，扩展周围景色"
          },
          {
            "image": "https://example.com/building.jpg"
          }
        ]
      }
    ]
  }'
```

#### 使用 Base64 输入

```bash
curl -X POST "https://llm.ai-nebula.com/v1/images/generations" \
  -H "Authorization: Bearer sk-Iia3/AM7jEGHAXnE3dcWWEQz0J7SirQ=" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-2.5-flash-image",
    "response_format": "b64_json",
    "size": "21:9",
    "contents": [
      {
        "role": "user",
        "parts": [
          {
            "text": "根据这张图生成超宽屏全景图，保留主体建筑，扩展周围景色"
          },
          {
            "image": "data:image/png;base64,iVBORw0KGgoAAxxxx[已截断]"
          }
        ]
      }
    ]
  }'
```

### 3.3 多图生一图（多图片融合）

Nano Banana 支持同时输入多张图片，模型会综合分析所有图片并生成新图片。适用于：
- **风格融合**：将一张图的风格应用到另一张图
- **元素组合**：从多张图中提取元素进行组合
- **对比参考**：提供多个参考图，让模型理解你的需求
- **场景混合**：融合多个场景的特点

#### 示例1：风格迁移（2图融合）

将一张图片的风格应用到另一张图片的内容：

```bash
curl -X POST "https://llm.ai-nebula.com/v1/images/generations" \
  -H "Authorization: Bearer sk-Iia3/AM7jxxxdcWWEQz0J7SirQ=" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-2.5-flash-image",
    "response_format": "b64_json",
    "size": "16:9",
    "contents": [
      {
        "role": "user",
        "parts": [
          {
            "text": "请将第一张图片的油画风格应用到第二张图片的内容上，生成一幅新的艺术作品"
          },
          {
            "image": "https://example.com/oil-painting-style.jpg"
          },
          {
            "image": "https://example.com/landscape-photo.jpg"
          }
        ]
      }
    ]
  }'
```

#### 示例2：元素组合（3图融合）

从多张图片中提取不同元素进行组合：

```bash
curl -X POST "https://llm.ai-nebula.com/v1/images/generations" \
  -H "Authorization: Bearer sk-Iia3/AM7jEGHAXnE3dcWWEQz0J7SirQ=" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-2.5-flash-image",
    "response_format": "b64_json",
    "size": "1:1",
    "contents": [
      {
        "role": "user",
        "parts": [
          {
            "text": "创作一张新图片：使用第一张图的天空，第二张图的建筑，第三张图的前景植物"
          },
          {
            "image": "data:image/jpeg;base64,/9j/4AAQSkZJRg[图1 base64已截断]"
          },
          {
            "image": "https://example.com/building.jpg"
          },
          {
            "image": "data:image/png;base64,iVBORw0KGgo[图3 base64已截断]"
          }
        ]
      }
    ]
  }'
```

#### 示例3：产品设计参考（多图+详细描述）

提供多张参考图，生成符合要求的产品设计图：

```bash
curl -X POST "https://llm.ai-nebula.com/v1/images/generations" \
  -H "Authorization: Bearer sk-Iia3/xxxnE3dcWWEQz0J7SirQ=" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-2.5-flash-image",
    "response_format": "b64_json",
    "size": "4:3",
    "contents": [
      {
        "role": "user",
        "parts": [
          {
            "text": "参考这三张图片，设计一款现代简约风格的咖啡杯：采用第一张图的配色方案，第二张图的极简线条，第三张图的手柄设计"
          },
          {
            "image": "https://example.com/color-reference.jpg"
          },
          {
            "image": "https://example.com/minimalist-design.jpg"
          },
          {
            "image": "https://example.com/handle-design.jpg"
          }
        ]
      }
    ]
  }'
```

#### 示例4：混合 URL 和 Base64 输入

可以灵活组合 URL 和 Base64 两种输入方式：

```bash
curl -X POST "https://llm.ai-nebula.com/v1/images/generations" \
  -H "Authorization: Bearer sk-Iia3/AM7xxxcWWEQz0J7SirQ=" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-2.5-flash-image",
    "response_format": "b64_json",
    "size": "9:16",
    "contents": [
      {
        "role": "user",
        "parts": [
          {
            "text": "将这两张图片的精华元素融合，创作一张时尚海报"
          },
          {
            "image": "https://example.com/fashion-photo1.jpg"
          },
          {
            "image": "data:image/webp;base64,UklGRiQAAABXRUJQVlA[base64已截断]"
          }
        ]
      }
    ]
  }'
```

**多图生成提示**：
- 支持 2-5 张图片同时输入
- 可以混合使用 URL 和 Base64 格式
- 在提示词中明确说明每张图片的作用和如何融合
- 图片顺序会影响生成结果，重要的图片放在前面
- 每张图片都需要符合格式和大小限制（PNG/JPEG/JPG/WEBP，最大7MB）

### 3.4 上下文对话式图片生成

Nano Banana 支持多轮对话式图片生成，可以在已生成图片的基础上继续修改：

```bash
curl -X POST "https://llm.ai-nebula.com/v1/images/generations" \
  -H "Authorization: Bearer sk-Iia3/AMxxxxcWWEQz0J7SirQ=" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-2.5-flash-image",
    "response_format": "b64_json",
    "size": "16:9",
    "contents": [
      {
        "role": "user",
        "parts": [
          {
            "text": "根据这张图帮我生成一张俯瞰广州塔的图片，16:9宽屏"
          },
          {
            "image": "data:image/png;base64,iVBORw0KGgoAAxxxx[已截断]"
          }
        ]
      },
      {
        "role": "model",
        "parts": [
          {
            "text": "已为您生成1张图片，这是俯瞰广州塔的16:9宽屏图片"
          },
          {
            "image": "data:image/png;base64,iVBORw0KGgoAAxxxx[已截断]"
          }
        ]
      },
      {
        "role": "user",
        "parts": [
          {
            "text": "把景色换成夜色下雪的场景，保持16:9比例"
          }
        ]
      }
    ]
  }'
```

## 4. 响应处理说明

### 4.1 响应格式

1. 成功响应（状态码 200）会返回包含图像数据的 JSON
2. 当 `response_format` 设为 `b64_json` 时，图像数据在 `data[].b64_json` 字段中
3. **注意**：Nano Banana 仅支持 `b64_json` 格式，不支持 `url` 格式

### 4.2 成功响应示例

```json
{
    "code": 200,
    "msg": "操作成功",
    "data": {
        "data": [
            {
                "url": "",
                "b64_json": "iVBORw0KGgoAAAANSUhEUgAABAAAAAQA[base64数据已截断]",
                "revised_prompt": ""
            }
        ],
        "created": 1757320007
    }
}
```

### 4.3 保存Base64图像数据（命令行示例）

```bash
# 将返回的base64数据保存为图片
echo "iVBORw0KGgoAAAANSUhEUgAABAAAAAQA..." | base64 -d > output.png
```

### 4.4 错误处理

如果请求失败，会返回错误信息：

```json
{
    "code": 400,
    "msg": "参数错误：不支持的宽高比",
    "data": null
}
```

常见错误码：
- `400`：参数错误（如模型名称错误、宽高比格式错误等）
- `401`：认证失败（API密钥无效或过期）
- `429`：请求过于频繁，超出速率限制
- `500`：服务器内部错误

## 5. 最佳实践

### 5.1 宽高比选择建议

- **社交媒体**：
  - Instagram 帖子：1:1 或 4:5
  - Instagram 故事：9:16
  - Twitter/X：16:9
  - Facebook 封面：21:9

- **设计用途**：
  - 网站横幅：16:9 或 21:9
  - 海报：2:3 或 9:16
  - 产品图片：1:1 或 4:3
  - 手机壁纸：9:16

- **视频相关**：
  - YouTube 缩略图：16:9
  - 短视频封面：9:16
  - 宽屏视频：21:9

### 5.2 提示词优化建议

#### 文生图提示词技巧

1. **明确比例需求**：在提示词中说明构图方向
   - 横屏：使用"横向构图"、"宽屏视角"
   - 竖屏：使用"竖向构图"、"垂直视角"

2. **考虑画面布局**：
   - 16:9/21:9：适合包含更多水平元素（如风景、全景）
   - 9:16：适合包含垂直元素（如人物、建筑）
   - 1:1：适合居中对称构图

3. **高质量关键词**：
   - 添加"高质量"、"高清"、"专业摄影"等关键词
   - 指定风格："写实风格"、"油画风格"、"动漫风格"等

#### 多图生一图提示词技巧

1. **明确图片角色**：
   - ✅ 好："将第一张图的油画风格应用到第二张图的风景内容"
   - ❌ 差："把这些图片混合一下"

2. **指定融合方式**：
   - 风格迁移："用第一张图的风格改造第二张图"
   - 元素提取："使用第一张图的天空、第二张图的建筑、第三张图的前景"
   - 参考设计："参考这些图片的配色、线条和布局，设计一个新产品"

3. **详细描述需求**：
   - 说明想要保留什么、改变什么
   - 指定最终效果的期望风格
   - 必要时给出构图建议

**示例对比**：

| 效果 | 提示词示例 |
|------|-----------|
| ❌ 模糊 | "混合这两张图" |
| ✅ 清晰 | "将第一张图的水彩画风格应用到第二张图的城市景观上，保持建筑的细节，但用柔和的色彩和笔触重新呈现" |
| ❌ 模糊 | "把这些图片合成一张" |
| ✅ 清晰 | "创作一张产品海报：采用第一张图的简约配色方案、第二张图的极简主义线条、第三张图的留白布局风格" |

### 5.3 图片输入最佳实践

1. **选择合适的输入方式**：
   - 图片在线上：优先使用 URL，减少传输数据量
   - 图片在本地：使用 Base64，避免上传到临时服务器
   - 需要保护隐私：使用 Base64，不经过第三方URL

2. **图片质量建议**：
   - 推荐分辨率：1024x1024 或更高
   - 文件大小：最大 7MB
   - 格式选择：PNG（高质量）、JPG（较小体积）、WEBP（最佳平衡）

3. **多图片输入技巧（多图生一图）**：
   - **数量控制**：支持 2-5 张图片，建议 2-3 张效果最佳
   - **排列顺序**：按照重要性顺序排列，最重要的图片放在前面
   - **清晰描述**：在提示词中明确说明每张图片的作用和如何融合
   - **应用场景**：
     - 风格迁移：用一张图的风格改造另一张图
     - 元素组合：从多张图中提取不同元素合成新图
     - 产品设计：参考多张图片的特点设计新产品
     - 场景融合：混合多个场景的特色创作新场景
   - **实用技巧**：
     - 明确指定"第一张图"、"第二张图"，避免混淆
     - 说明想要保留或提取的具体元素
     - 可以混合使用 URL 和 Base64 两种输入方式

### 5.4 性能优化建议

1. **批量生成**：如需生成多张图片，可以并发请求以提高效率
2. **缓存策略**：对于相同参数的请求，建议在客户端进行缓存
3. **异步处理**：对于非实时需求，建议使用异步处理机制
4. **图片预处理**：对于大尺寸图片，建议先压缩到合理大小再传输

## 6. 常见问题

### Q1: 如何在对话中保持相同的宽高比？

A: 在 `contents` 对话数组中，每次请求都要带上 `size` 参数，系统会为当前请求应用指定的宽高比。

### Q2: 使用 URL 图片有什么要求？

A: 
- URL 必须是可公开访问的 HTTP/HTTPS 地址
- 支持格式：PNG、JPEG、JPG、WEBP
- 文件大小：最大 7MB
- 系统会自动下载并转换为 Base64 格式传递给模型

### Q3: Base64 图片格式有什么要求？

A: 
- 必须包含完整的 data URI 前缀，如：`data:image/png;base64,iVBORw0...`
- 支持的格式：`image/png`、`image/jpeg`、`image/webp`
- 文件大小：最大 7MB（编码前）
- 确保 Base64 数据编码正确

### Q4: Nano Banana 支持哪些宽高比？

A: Nano Banana (gemini-2.5-flash-image) 支持文档中列出的所有10种宽高比：1:1、3:2、2:3、3:4、4:3、4:5、5:4、9:16、16:9、21:9。

### Q5: 生成的图片实际像素是多少？

A: 图片的实际像素由模型决定，通常会根据指定的宽高比生成高质量图片。不同宽高比可能有不同的像素尺寸，但都会保持指定的比例关系。

### Q6: 可以同时上传多张图片吗（多图生一图）？

A: 可以！Nano Banana 支持同时输入 2-5 张图片进行融合生成：
- **风格迁移**：将一张图的风格应用到另一张图
- **元素组合**：从多张图中提取不同元素
- **产品设计**：参考多张图片生成新设计
- **场景融合**：混合多个场景特点

**使用方法**：在 `contents[].parts` 数组中添加多个 `image` 对象，并在 `text` 中明确说明如何处理这些图片。详见文档 3.3 节的多图生一图示例。

**最佳实践**：
- 提供清晰的文字说明，告诉模型如何使用每张图片
- 图片顺序很重要，将最重要的图片放在前面
- 每张图片都必须符合格式和大小要求（PNG/JPEG/JPG/WEBP，最大7MB）

---

**文档版本**: v2.1  
**更新时间**: 2025-11-05  
**模型**: Nano Banana (gemini-2.5-flash-image)  
**技术支持**: https://llm.ai-nebula.com

---

## 快速参考

### 模型参数速查

| 参数 | 值 |
|------|-----|
| 模型名称 | gemini-2.5-flash-image |
| 支持格式 | PNG、JPEG、JPG、WEBP |
| 最大尺寸 | 7MB（单张图片） |
| 支持宽高比 | 1:1, 3:2, 2:3, 3:4, 4:3, 4:5, 5:4, 9:16, 16:9, 21:9 |
| 图片输入 | URL 或 Base64 |
| 多图输入 | 支持 2-5 张图片同时输入 |
| 响应格式 | b64_json |

### 核心功能速查

| 功能 | 说明 | 示例章节 |
|------|------|---------|
| 文生图 | 纯文本描述生成图片 | 2.1-2.4 |
| 图生图 | 单图+文本生成新图 | 3.1-3.2 |
| 多图生一图 | 2-5张图片融合生成 | 3.3 |
| 对话生成 | 多轮连续修改图片 | 3.4 |

### 常用宽高比速查

| 比例 | 像素 | 场景 |
|------|------|------|
| 1:1 | 1024x1024 | 社交媒体、头像 |
| 16:9 | 1792x1024 | 视频封面、横屏壁纸 |
| 9:16 | 1024x1792 | 短视频、手机壁纸 |
| 21:9 | 1024x2176 | 超宽屏全景 |

### 多图生一图应用场景

| 场景 | 图片数量 | 提示词示例 |
|------|---------|-----------|
| 风格迁移 | 2张 | "将第一张图的油画风格应用到第二张图的内容" |
| 元素组合 | 2-3张 | "使用第一张图的天空+第二张图的建筑+第三张图的植物" |
| 产品设计 | 3-4张 | "参考这些图片，设计一款咖啡杯：第一张的配色+第二张的线条+第三张的手柄" |
| 场景融合 | 2张 | "融合这两个场景的特点，创作一个新环境" |