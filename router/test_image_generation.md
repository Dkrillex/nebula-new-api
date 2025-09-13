# 外部系统图片生成接口测试

## 接口信息
- **路径**: `/api/sync/system/images/generations`
- **方法**: POST
- **认证**: 系统访问令牌 (access_token)

## 请求示例

```bash
curl -X POST "http://localhost:3000/api/sync/system/images/generations" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_SYSTEM_ACCESS_TOKEN" \
  -d '{
    "user_id": 1,
    "model": "dall-e-3",
    "group": "default",
    "prompt": "一只可爱的小猫在花园里玩耍",
    "n": 1,
    "size": "1024x1024",
    "quality": "standard",
    "response_format": "url",
    "style": "vivid"
  }'
```

### 使用私有参数的请求示例（豆包模型）

```bash
curl -X POST "http://localhost:3000/api/sync/system/images/generations" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_SYSTEM_ACCESS_TOKEN" \
  -d '{
    "user_id": 1,
    "model": "doubao-image",
    "group": "default",
    "prompt": "一只可爱的小猫",
    "n": 1,
    "size": "1024x1024",
    "watermark": false,
    "seed": 12345,
    "guidance_scale": 7.5
  }'
```

### 使用私有参数的请求示例（Gemini模型）

```bash
curl -X POST "http://localhost:3000/api/sync/system/images/generations" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_SYSTEM_ACCESS_TOKEN" \
  -d '{
    "user_id": 1,
    "model": "gemini-2.5-flash-image",
    "group": "default",
    "prompt": "Generate a futuristic cityscape",
    "n": 1,
    "aspect_ratio": "16:9",
    "person_generation": "allow_adult",
    "sample_count": 2
  }'
```

## 请求参数说明

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| user_id | int | 是 | 实际扣费的用户ID |
| model | string | 是 | 图片生成模型名称 |
| group | string | 否 | 用户分组，为空时使用用户默认分组 |
| prompt | string | 是 | 图片生成提示词 |
| n | uint | 否 | 生成图片数量，默认为1 |
| size | string | 否 | 图片尺寸，如 "1024x1024" |
| quality | string | 否 | 图片质量，如 "standard" 或 "hd" |
| response_format | string | 否 | 响应格式，"url" 或 "b64_json" |
| style | string | 否 | 图片风格，如 "vivid" 或 "natural" |
| 其他字段 | any | 否 | 大模型私有参数，如 watermark, seed, guidance_scale, aspect_ratio 等 |

## 实现特点

1. **用户鉴权**: 通过 `user_id` 指定实际扣费用户
2. **临时令牌**: 生成格式为 `nebula-image-generations-{group}` 的临时令牌
3. **直接扣费**: 不走API Key，直接从用户余额扣除quota
4. **渠道选择**: 自动选择合适的图片生成渠道
5. **错误处理**: 统一的错误响应格式
6. **私有参数支持**: 通过 `Extra` 字段支持各大模型的私有参数，如gemini和豆包适配器

## 与内部接口的区别

| 特性 | 内部接口 (`/v1/images/generations`) | 外部接口 (`/api/sync/system/images/generations`) |
|------|-----------------------------------|--------------------------------------------------|
| 认证方式 | API Key (TokenAuth) | 系统访问令牌 (SystemAccessTokenAuth) |
| 用户识别 | 通过令牌获取用户信息 | 通过请求体中的 user_id 指定 |
| 临时令牌名称 | - | `nebula-image-generations-{group}` |
| 使用场景 | 系统内部调用 | 外部系统集成 |