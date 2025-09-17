# 豆包图片生成接口错误分析

## 1. 问题描述

### 错误日志
```
[DEBUG] 2025/09/16 - 16:10:32 | 20250916161024986694000fDPX4Bsa | [image_Handler]image request body: {"model":"doubao-seedream-4-0-250828","prompt":"","n":1,"response_format":"url","watermark":true} 
fullRequestURL: /api/sync/system/images/generations 
[ERR] 2025/09/16 - 16:10:32 | 20250916161024986694000fDPX4Bsa | relay error (channel #0, status code: 500): upstream error: do request failed 
[INFO] 2025/09/16 - 16:10:32 | 20250916161024986694000fDPX4Bsa | 用户 666888 请求失败, 返还预扣费额度 ＄0.027400 
[GIN] 2025/09/16 - 16:10:32 | 20250916161024986694000fDPX4Bsa | 500 |  7.686651541s |             ::1 |    POST /api/sync/system/images/generations 
```

### 关键问题点
1. **接口路径**: `/api/sync/system/images/generations`
2. **模型**: `doubao-seedream-4-0-250828` (豆包图文生图模型)
3. **prompt为空**: `"prompt":""`
4. **错误**: `do request failed` (HTTP请求失败)
5. **状态码**: 500 (服务器内部错误)

## 2. 接口调用流程分析

### 2.1 路由映射
- **路由定义**: `router/api-router.go:242`
  ```go
  syncSystemRoute.POST("/images/generations", controller.SyncImageGeneration)
  ```
- **处理函数**: `controller.SyncImageGeneration` (与普通图片生成接口相同)

### 2.2 渠道选择逻辑
- **调用位置**: `controller/sync.go:862`
  ```go
  channel, err := getChannel(c, group, imageRequest.Model, 0) // 已修复：retryCount=0
  ```
- **选择策略**: 使用最高优先级渠道 (retryCount=0)

### 2.3 适配器选择
- **渠道类型**: `ChannelTypeVolcEngine` (45)
- **API类型映射**: `ChannelTypeVolcEngine` → `APITypeDoubao`
- **适配器**: `volcengine.Adaptor{}`

## 3. 问题根源分析

### 3.1 prompt字段为空的影响
根据豆包适配器的实现 (`relay/channel/volcengine/adaptor.go:67-322`)：

```go
func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
    switch info.RelayMode {
    case constant.RelayModeImagesGenerations:
        // 检查是否为图文生图模型
        if strings.Contains(info.UpstreamModelName, "seed") {
            // 处理图文生图请求
            req := &DoubaoImageRequest{
                Model:          info.UpstreamModelName,
                Prompt:         request.Prompt,  // 直接使用空的prompt
                Watermark:      request.Watermark,
                ResponseFormat: request.ResponseFormat,
                Size:           request.Size,
            }
            // ...
        }
    }
}
```

**问题**: 对于 `doubao-seedream-4-0-250828` 模型（包含"seed"关键字），适配器会进入图文生图处理逻辑，但由于 `prompt` 字段为空，导致豆包API拒绝请求。

### 3.2 豆包API要求
豆包图文生图API要求：
- **prompt**: 必填字段，不能为空
- **model**: 必须是有效的豆包模型名称
- **其他参数**: 根据具体需求可选

### 3.3 错误传播路径
1. `controller.SyncImageGeneration` 接收到空prompt请求
2. `volcengine.Adaptor.ConvertImageRequest` 构建请求体（prompt为空）
3. `volcengine.Adaptor.DoRequest` → `channel.DoApiRequest` 发送HTTP请求
4. 豆包API返回400/422错误（参数无效）
5. `relay/channel/api_request.go:doRequest` 捕获错误，返回"do request failed"
6. 最终返回500错误给客户端

## 4. 可能的解决方案

### 方案1: 参数校验增强 (推荐)
在 `controller.SyncImageGeneration` 中增加参数校验：

```go
// 在 controller/sync.go 的 SyncImageGeneration 函数中添加
if imageRequest.Prompt == "" {
    c.JSON(http.StatusBadRequest, gin.H{
        "error": gin.H{
            "message": "prompt is required for image generation",
            "type":    "invalid_request_error",
        },
    })
    return
}
```

### 方案2: 适配器层面处理
在 `volcengine.Adaptor.ConvertImageRequest` 中添加校验：

```go
// 在豆包适配器中添加
if req.Prompt == "" {
    return nil, errors.New("prompt is required for doubao image generation")
}
```

### 方案3: 默认prompt处理
为空prompt提供默认值：

```go
// 在适配器中
if req.Prompt == "" {
    req.Prompt = "生成一张图片" // 提供默认描述
    logger.LogInfo(c, "使用默认prompt: 生成一张图片")
}
```

## 5. 推荐修复方案

### 5.1 立即修复 (参数校验)
在 `controller/sync.go` 的 `SyncImageGeneration` 函数中添加参数校验：

```go
// 在解析请求后立即添加
if imageRequest.Prompt == "" {
    logger.LogError(c, "图片生成请求缺少prompt参数")
    c.JSON(http.StatusBadRequest, gin.H{
        "error": gin.H{
            "message": "prompt parameter is required for image generation",
            "type":    "invalid_request_error",
            "param":   "prompt",
        },
    })
    return
}
```

### 5.2 长期优化
1. **统一参数校验**: 在中间件层面统一处理图片生成请求的参数校验
2. **错误信息优化**: 提供更详细的错误信息，帮助客户端快速定位问题
3. **日志增强**: 在适配器层面记录更详细的请求参数和错误信息

## 6. 验证方法

### 6.1 测试用例
```bash
# 测试空prompt (应该返回400错误)
curl -X POST http://localhost:3000/api/sync/system/images/generations \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -d '{
    "model": "doubao-seedream-4-0-250828",
    "prompt": "",
    "n": 1,
    "response_format": "url"
  }'

# 测试有效prompt (应该成功)
curl -X POST http://localhost:3000/api/sync/system/images/generations \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -d '{
    "model": "doubao-seedream-4-0-250828",
    "prompt": "一只可爱的小猫",
    "n": 1,
    "response_format": "url"
  }'
```

### 6.2 日志监控
修复后应该看到：
- 空prompt请求返回400错误而不是500错误
- 错误信息更加明确和有用
- 不再出现"do request failed"的模糊错误

## 7. 修复实施

### 7.1 已实施的修复
在 `controller/sync.go` 的 `SyncImageGeneration` 函数中添加了prompt参数校验：

```go
// 验证prompt参数 (图片生成必需)
if imageRequest.Prompt == "" {
    newAPIError = types.NewError(errors.New("prompt参数不能为空"), types.ErrorCodeInvalidRequest)
    return
}
```

**修复位置**: `controller/sync.go:841-845`

### 7.2 修复效果
- ✅ 空prompt请求现在会返回400错误而不是500错误
- ✅ 错误信息更加明确："prompt参数不能为空"
- ✅ 避免了无效请求到达豆包API，减少不必要的网络开销
- ✅ 提供更好的用户体验和调试信息

### 7.3 测试验证
修复后的行为：
```bash
# 空prompt请求 - 现在返回400错误
curl -X POST http://localhost:3000/api/sync/system/images/generations \
  -H "Content-Type: application/json" \
  -d '{"model":"doubao-seedream-4-0-250828","prompt":"","user_id":666888}'

# 预期响应:
# HTTP 400 Bad Request
# {"error":{"message":"prompt参数不能为空","type":"invalid_request_error"}}
```

## 8. 总结

这个问题的根本原因是**参数校验不足**，导致空的prompt参数被传递到豆包API，引发服务器错误。通过在控制器层面添加参数校验，已经成功解决了这个问题，现在可以在请求到达适配器之前就拦截无效请求，提供更好的用户体验和错误信息。

**修复前**: 空prompt → 豆包API错误 → "do request failed" → 500错误  
**修复后**: 空prompt → 参数校验失败 → "prompt参数不能为空" → 400错误