# Nebula V1 Models API 接口文档

## 接口说明

`GET /api/nebula/v1/models` - 获取当前用户可用的模型列表,包含详细的价格信息和厂商信息。

## 认证要求

- **必需**: 用户登录认证 (UserAuth中间件)
- 使用session或token进行身份验证

## 权限控制

- 仅返回当前用户所在分组可用的模型
- 基于abilities表中的分组权限进行过滤

## 响应格式

### 成功响应 (200 OK)

```json
{
  "success": true,
  "data": [
    {
      "id": "claude-opus-4-1-20250805",
      "model_name": "claude-opus-4-1-20250805",
      "vendor": {
        "id": 2,
        "name": "Anthropic",
        "description": "AI safety and research company",
        "icon": "SimpleIconsAnthropic"
      },
      "description": "Claude Opus 4.1 - 最强大的模型",
      "description_en": "Claude Opus 4.1 - Most capable model",
      "icon": "model-icon",
      "icon_url": "https://example.com/icon.png",
      "tags": "推理,代码,长文本",
      "tags_en": "reasoning,code,long-context",
      "pricing": {
        "billing_type": "token",
        "input_text_price": 0.015,
        "output_text_price": 0.075,
        "input_image_price": 0.048,
        "output_image_price": 0.24
      },
      "supported_endpoints": ["chat_completions", "messages"]
    },
    {
      "id": "dall-e-3",
      "model_name": "dall-e-3",
      "vendor": {
        "id": 1,
        "name": "OpenAI",
        "icon": "SimpleIconsOpenai"
      },
      "pricing": {
        "billing_type": "per_call",
        "price_per_call": 0.04,
        "price_per_image": 0.04
      },
      "supported_endpoints": ["images_generations"]
    },
    {
      "id": "sora-2",
      "model_name": "sora-2",
      "vendor": {
        "id": 1,
        "name": "OpenAI"
      },
      "pricing": {
        "billing_type": "per_second",
        "price_per_second": 0.1
      },
      "supported_endpoints": ["videos"]
    }
  ]
}
```

### 错误响应

```json
{
  "success": false,
  "message": "获取用户分组失败"
}
```

## 计费类型说明

### 1. token - 按量计费

按token数量计费,价格单位: **$/M tokens** (美元/百万tokens)

可能包含的价格字段:
- `input_text_price`: 文本输入价格
- `output_text_price`: 文本输出价格
- `input_image_price`: 图片输入价格
- `output_image_price`: 图片输出价格
- `input_audio_price`: 音频输入价格
- `output_audio_price`: 音频输出价格

### 2. per_call - 按次计费

固定价格,每次API调用收费,价格单位: **$/次**

包含的价格字段:
- `price_per_call`: 每次调用价格
- `price_per_image`: 每张图片价格 (与price_per_call相同,兼容字段)

### 3. per_second - 按秒计费

按生成秒数计费,主要用于视频生成,价格单位: **$/秒**

包含的价格字段:
- `price_per_second`: 每秒价格

## 价格计算规则

### Quota与美元转换

系统内部使用quota作为计费单位:
- **500,000 quota = $1 USD**
- **1 quota = $0.000002 USD**

### 按量计费价格计算

对于按量计费模型,价格计算公式:

```
输入价格 ($/M tokens) = (ModelRatio × 2) / 1000
输出价格 ($/M tokens) = 输入价格 × CompletionRatio
```

## 测试步骤

### 1. 准备测试环境

确保系统已启动并且有测试用户账号。

### 2. 获取认证Token/Session

登录系统获取有效的认证凭证。

### 3. 发送请求

```bash
# 使用curl测试
curl -X GET 'http://localhost:3000/api/nebula/v1/models' \
  -H 'Authorization: Bearer YOUR_TOKEN' \
  -H 'Cookie: session=YOUR_SESSION'
```

### 4. 验证响应

检查返回的数据:

#### 基础验证
- [ ] 响应状态码为200
- [ ] success字段为true
- [ ] data字段是数组类型
- [ ] 每个模型都有id和model_name字段

#### 权限验证
- [ ] 仅返回当前用户分组可用的模型
- [ ] 不同分组用户看到的模型列表不同

#### 价格验证
- [ ] 每个模型都有pricing对象
- [ ] billing_type值正确 (token/per_call/per_second)
- [ ] 价格字段与billing_type匹配:
  - token类型有input/output价格
  - per_call类型有price_per_call
  - per_second类型有price_per_second

#### 厂商验证
- [ ] 有vendor_id的模型包含vendor对象
- [ ] vendor对象包含id, name等字段

#### 端点验证
- [ ] supported_endpoints数组不为空
- [ ] 端点类型正确 (chat_completions, images_generations等)

### 5. 价格准确性验证

选择几个已知价格的模型进行验证:

**GPT-4 示例** (假设ModelRatio=15):
```
输入价格 = (15 × 2) / 1000 = 0.03 $/M tokens
```

**DALL-E-3 示例** (假设ModelPrice=0.04):
```
价格 = 0.04 $/次
```

**Sora-2 示例** (假设VideoPrice=0.1):
```
价格 = 0.1 $/秒
```

## 常见问题

### Q: 为什么某些模型没有出现在列表中?

A: 可能的原因:
1. 模型在models表中被禁用 (status != 1)
2. 模型在abilities表中没有为您的用户分组启用
3. 模型在abilities表中被禁用 (enabled = false)

### Q: 价格显示为0或null是什么原因?

A: 可能的原因:
1. 模型的价格配置缺失
2. 对应的ratio配置为0
3. 计费类型与价格配置不匹配

### Q: 如何测试不同分组的权限?

A: 创建不同分组的测试用户,分别登录后调用接口查看返回的模型列表差异。

## 数据来源

- **模型列表**: abilities表 (启用状态 + 分组关系)
- **模型元数据**: models表 (描述、标签、状态、vendor_id)
- **厂商信息**: vendors表
- **价格配置**: options表 (ModelRatio, ModelPrice, VideoModelPricePerSecond等)
- **端点信息**: models表endpoints字段 + 默认端点映射

## 相关接口对比

| 接口 | 路径 | 认证 | 价格格式 | 分组过滤 | 厂商信息 |
|------|------|------|----------|----------|----------|
| OpenAI兼容 | /v1/models | Token | ✗ | ✓ | ✗ |
| Pricing | /api/pricing | 可选 | Quota | ✓ | ✗ |
| **Nebula Models** | **/api/nebula/v1/models** | **必需** | **USD分类** | **✓** | **✓** |
