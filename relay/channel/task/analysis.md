# 四个视频大模型适配器请求格式对比

## 概述

本文档对比分析了四个视频大模型适配器（Doubao、Jimeng、Kling、Suno）的请求格式和参数差异。虽然它们都接受统一的输入接口，但在内部请求格式、关键参数、返回结果处理和认证方式上存在显著差异。

## 统一输入接口

所有适配器都接受相同的 `VideoRequest` 结构体作为输入：

```json
{
  "model": "kling-v1",
  "prompt": "宇航员站起身走了",
  "image": "https://example.com/image.jpg",
  "duration": 5.0,
  "width": 512,
  "height": 512,
  "fps": 30,
  "seed": 20231234,
  "n": 1,
  "response_format": "url",
  "user": "user-1234",
  "metadata": {}
}
```

## 各适配器内部请求格式

### Doubao 适配器

**内部请求结构：**

```json
{
  "model": "string",
  "content": [
    {
      "type": "text",
      "text": "string"
    },
    {
      "type": "image_url",
      "image_url": {
        "url": "string"
      }
    }
  ]
}
```

**关键参数：**
- `model`: 模型名称
- `content`: 内容数组，支持文本和图片URL

### Jimeng 适配器

**内部请求结构：**

```json
{
  "req_key": "string",
  "binary_data_base64": ["string"],
  "image_urls": ["string"],
  "prompt": "string",
  "seed": "number",
  "aspect_ratio": "string"
}
```

**关键参数：**
- `req_key`: 请求密钥
- `binary_data_base64`: Base64编码的二进制数据数组
- `image_urls`: 图片URL数组
- `prompt`: 提示词
- `seed`: 随机种子
- `aspect_ratio`: 宽高比

### Kling 适配器

**内部请求结构：**

```json
{
  "prompt": "string",
  "image": "string",
  "mode": "string",
  "duration": "number",
  "aspect_ratio": "string",
  "model_name": "string"
}
```

**关键参数：**
- `prompt`: 提示词
- `image`: 图片URL
- `mode`: 生成模式
- `duration`: 视频时长
- `aspect_ratio`: 宽高比
- `model_name`: 模型名称

### Suno 适配器

**内部请求结构：**

```json
{
  "gpt_description_prompt": "string",
  "prompt": "string",
  "mv": "string",
  "continue_clip_id": "string",
  "task_id": "string"
}
```

**关键参数：**
- `gpt_description_prompt`: GPT描述提示
- `prompt`: 提示词
- `mv`: 音乐视频相关参数
- `continue_clip_id`: 继续片段ID
- `task_id`: 任务ID

## 返回结果格式对比

| 适配器 | 返回格式 | 说明 |
|--------|----------|------|
| Doubao | `{"task_id": "xxx"}` | 标准化格式，从 `responsePayload.Data.TaskID` 解析 |
| Jimeng | `{"task_id": "xxx"}` | 标准化格式，从 `responsePayload.Data.TaskID` 解析 |
| Kling | `{"task_id": "xxx"}` | 标准化格式，支持多种解析方式 |
| Suno | 原始响应透传 | 特例：直接返回原始API响应，不进行标准化处理 |

## 认证方式对比

- **Doubao**: 使用标准的 Authorization Header
- **Jimeng**: 使用自定义的 req_key 参数
- **Kling**: 使用标准的 Authorization Header
- **Suno**: 使用标准的 Authorization Header

## 主要差异总结

### 请求格式差异

1. **Doubao**: 采用 OpenAI 风格的 content 数组格式
2. **Jimeng**: 使用专有的 req_key 和 binary_data_base64 格式
3. **Kling**: 采用扁平化的参数结构
4. **Suno**: 专注于音频/音乐相关的参数

### 参数映射差异

- **图片处理**: 
  - Doubao: `image_url.url`
  - Jimeng: `image_urls` 数组 + `binary_data_base64`
  - Kling: 直接 `image` 字段
  - Suno: 不直接支持图片

- **提示词处理**:
  - Doubao: 在 `content` 数组中的 `text` 字段
  - Jimeng/Kling: 直接 `prompt` 字段
  - Suno: `prompt` + `gpt_description_prompt`

### 返回结果处理差异

- **标准化程度**: Doubao、Jimeng、Kling 都返回标准的 `{"task_id": "xxx"}` 格式
- **特殊处理**: Suno 直接透传原始响应，不进行格式转换

## 结论

虽然四个适配器都实现了统一的 `VideoRequest` 输入接口，但由于各个视频大模型API的原生格式差异，适配器内部需要进行不同的参数转换和格式适配。除了 Suno 适配器采用透传模式外，其他三个适配器都实现了标准化的 `task_id` 返回格式，确保了上层调用的一致性。
        