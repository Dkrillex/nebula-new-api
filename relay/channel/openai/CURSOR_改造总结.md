# Cursor GPT 系列调用改造总结

## 一、改造目标

支持 Cursor 调用 `openai/` 前缀的模型时：
- **接口说明**：Cursor 一直调用的是 `/v1/chat/completions` 接口，但请求格式是 `v1/responses` 格式的
- **请求转换**：将 Cursor 发送的 `v1/responses` 格式请求转换为 `v1/chat/completions` 格式，然后发送到上游的 `/v1/responses` 接口
- **响应转换**：将上游返回的 `v1/responses` 格式响应转换为 `v1/chat/completions` 格式返回给客户端
- **其他模型**：对于非 `openai/` 前缀的模型，保持原有的 `v1/chat/completions` 请求和响应流程（直接透传）

## 二、修改的文件清单

### 2.1 核心修改文件

1. **`relay/helper/valid_request.go`**
   - 添加 `detectCursorRequest` 函数：检测是否为 Cursor 请求
   - 修改 `GetAndValidateRequest`：对 `openai/` 前缀的模型进行特殊处理
   - **关键逻辑**：只检测 `openai/` 前缀的模型，设置 `is_cursor` 和 `convert_responses_to_chat` 标志

2. **`controller/relay.go`**
   - 修改路由分发逻辑：根据 `is_cursor` 标志决定使用 `RelayFormatOpenAIResponses` 还是 `RelayFormatOpenAI`
   - **关键逻辑**：对于 `openai/` 前缀的模型，使用 `RelayFormatOpenAIResponses` 格式

3. **`relay/channel/openai/relay_responses.go`**
   - 添加 `convertResponsesToChatCompletions`：非流式响应转换
   - 添加 `convertResponsesStreamToChatCompletions`：流式响应转换
   - 修改 `OaiResponsesHandler`：检查 `convert_responses_to_chat` 标志，进行响应转换
   - 修改 `OaiResponsesStreamHandler`：检查 `convert_responses_to_chat` 标志，进行流式响应转换
   - **关键优化**：工具调用参数增量事件只累积不发送，避免发送不完整的 JSON

4. **`relay/channel/openai/cursor_convert.go`** (新增)
   - `ConvertCursorResponsesToOpenAI`：将 Cursor 发送的 `v1/responses` 格式请求转换为 OpenAI 的 `v1/chat/completions` 格式
   - `ConvertCursorToolsToOpenAI`：转换工具定义格式
   - **关键功能**：处理 `input`、`instructions`、`tools`、`max_output_tokens` 等字段的转换

5. **`relay/common/relay_info.go`**
   - 修改 `GenRelayInfoResponses`：设置 `RequestURLPath = "/v1/responses"`
   - **关键修改**：确保转换后的请求被发送到上游的 `/v1/responses` 接口（而不是 `/v1/chat/completions`）

### 2.2 DTO 修改

6. **`dto/openai_request.go`**
   - 添加 `OpenAIResponsesRequest` 相关字段支持

7. **`dto/openai_response.go`**
   - 在 `ResponsesOutput` 中添加 `Name`, `CallID`, `Arguments`, `Input` 字段
   - 在 `ResponsesStreamResponse` 中添加 `ItemID` 和 `OutputIndex` 字段
   - **关键修改**：支持工具调用信息的提取

### 2.3 其他文件

8. **`relay/channel/openai/helper.go`**
   - 保持原有逻辑，未做修改（流式响应转换在 `relay_responses.go` 中处理）

9. **`relay/channel/openai/relay-openai.go`**
   - 保持原有逻辑，未做修改

## 三、关键判断条件

### 3.1 识别 Cursor 请求的条件

```go
// 在 valid_request.go 中
// Cursor 调用 /v1/chat/completions 接口，但请求体是 v1/responses 格式
// 只检测 openai/ 前缀的模型
if strings.HasPrefix(modelName, "openai/") {
    isCursorRequest = true
}
```

### 3.2 转换标志

- **`is_cursor`**：标识是否为 Cursor 请求（`openai/` 前缀的模型）
- **`convert_responses_to_chat`**：标识是否需要将 Responses 响应转换为 Chat Completions 格式
- **`convert_cursor_to_chat`**：标识是否需要将 Cursor 请求转换为 Chat Completions 格式

### 3.3 转换触发条件

```go
// 在 relay_responses.go 中
if c.GetBool("convert_responses_to_chat") {
    // 进行响应转换
}
```

## 四、影响范围分析

### 4.1 ✅ 不会影响非 Cursor 调用

**原因：**

1. **严格的模型前缀判断**
   ```go
   if strings.HasPrefix(modelName, "openai/") {
       // 只有 openai/ 前缀的模型才会进入 Cursor 处理逻辑
   }
   ```

2. **条件判断保护**
   - 所有转换逻辑都有 `c.GetBool("convert_responses_to_chat")` 或 `c.GetBool("is_cursor")` 判断
   - 非 Cursor 调用不会设置这些标志，因此不会进入转换逻辑

3. **独立的处理路径**
   - Cursor 调用（`openai/` 前缀）：`RelayFormatOpenAIResponses` → 请求转换 → 发送到 `/v1/responses` → 响应转换 → 返回 Chat Completions 格式
   - 非 Cursor 调用：`RelayFormatOpenAI` → `OpenaiHandler` → 直接透传到 `/v1/chat/completions`

### 4.2 可能的影响点（已做保护）

1. **`valid_request.go` 中的请求解析**
   - ✅ 保护：只对 `openai/` 前缀的模型进行特殊处理
   - ✅ 保护：如果解析失败，回退到标准 Chat Completions 格式

2. **`relay_responses.go` 中的响应转换**
   - ✅ 保护：只有 `convert_responses_to_chat` 为 `true` 时才转换
   - ✅ 保护：非 Cursor 调用不会设置此标志

3. **`relay_info.go` 中的 URL 路径设置**
   - ✅ 保护：只在 `GenRelayInfoResponses` 中设置 `/v1/responses`
   - ✅ 保护：非 Responses 格式的请求不会调用此函数

## 五、数据流图

### 5.1 Cursor 调用流程（openai/ 前缀）

```
Cursor 客户端调用 /v1/chat/completions 接口
    ↓
请求体是 v1/responses 格式（包含 input、instructions 等字段）
    ↓
valid_request.go: 检测 openai/ 前缀 → 设置 is_cursor=true
    ↓
controller/relay.go: 设置 RelayFormatOpenAIResponses
    ↓
cursor_convert.go: ConvertCursorResponsesToOpenAI (请求转换：v1/responses → v1/chat/completions)
    ↓
发送到上游 /v1/responses 接口（使用转换后的 Chat Completions 格式）
    ↓
上游返回 Responses 格式响应
    ↓
relay_responses.go: convertResponsesStreamToChatCompletions (响应转换：Responses → Chat Completions)
    ↓
返回给 Cursor 客户端 Chat Completions 格式（符合 /v1/chat/completions 接口规范）
```

### 5.2 非 Cursor 调用流程（标准流程）

```
客户端调用 /v1/chat/completions 接口
    ↓
请求体是标准的 v1/chat/completions 格式（包含 messages 字段）
    ↓
valid_request.go: 标准解析，不设置 is_cursor
    ↓
controller/relay.go: 设置 RelayFormatOpenAI
    ↓
发送到上游 /v1/chat/completions 接口（直接透传）
    ↓
上游返回 Chat Completions 格式响应
    ↓
直接透传给客户端（不转换）
```

## 六、关键优化点

### 6.1 工具调用参数流式传输优化

**问题**：在流式传输中，工具调用的 `arguments` 是增量片段（如 `"{\""`, `"target"`），Cursor 可能在接收到不完整的 JSON 时尝试解析，导致 "invalid arguments" 错误。

**解决方案**：
- `response.function_call_arguments.delta` 和 `response.custom_tool_call_input.delta` 事件中，只累积参数，不立即发送
- 在 `response.output_item.done` 事件中发送完整的工具调用（此时参数已完整）

### 6.2 工具调用索引设置

- 在所有工具调用相关事件中正确设置 `index` 字段
- 使用 `OutputIndex` 计算工具调用索引（从 1 开始转换为从 0 开始）

## 七、测试建议

### 7.1 Cursor 调用测试

1. **测试 openai/ 前缀的模型**
   - 使用 `openai/gpt-4`、`openai/gpt-5.1-chat` 等模型
   - 验证请求转换是否正确
   - 验证响应转换是否正确
   - 验证工具调用是否正常工作

2. **测试工具调用**
   - 验证工具调用参数是否正确传输
   - 验证工具调用索引是否正确
   - 验证流式传输是否正常

### 7.2 非 Cursor 调用测试

1. **测试标准模型**
   - 使用 `gpt-4`、`gpt-3.5-turbo` 等标准模型名
   - 验证请求和响应是否正常（不应该进入转换逻辑）

2. **测试其他渠道**
   - 验证 Claude、Gemini 等其他渠道是否正常
   - 验证 Responses 格式的其他使用场景是否正常

## 八、注意事项

1. **模型名前缀必须严格匹配**
   - 只有 `openai/` 前缀的模型才会进入 Cursor 处理逻辑
   - 其他模型名（如 `gpt-4`、`claude-3`）不会受到影响

2. **转换标志的作用域**
   - `is_cursor` 和 `convert_responses_to_chat` 标志只在当前请求的 `gin.Context` 中有效
   - 不会影响其他请求

3. **工具调用参数累积**
   - 工具调用参数在流式传输中会累积，直到工具调用完成
   - 这避免了发送不完整的 JSON 导致解析错误

## 十、不同 openai/ 前缀模型的调用差异

### 10.1 核心处理逻辑相同

**所有 `openai/` 前缀的模型都走相同的处理流程：**

```go
// 在 valid_request.go 中
if strings.HasPrefix(modelName, "openai/") {
    // 无论是 openai/gpt-5.1-codex 还是 openai/gpt-5
    // 都会进入相同的 Cursor 处理逻辑
    isCursorRequest = true
}
```

**处理流程：**
1. ✅ 检测 `openai/` 前缀 → 设置 `is_cursor=true`
2. ✅ 去掉 `openai/` 前缀（如 `openai/gpt-5.1-codex` → `gpt-5.1-codex`）
3. ✅ 请求转换：`ConvertCursorResponsesToOpenAI`
4. ✅ 发送到上游 `/v1/responses` 接口
5. ✅ 响应转换：`convertResponsesStreamToChatCompletions`

### 10.2 可能的差异点

**在 `adaptor.go` 中，去掉前缀后的模型名可能会有特殊适配：**

```go
// 在 adaptor.go 中（去掉 openai/ 前缀后的模型名）
if strings.HasPrefix(info.UpstreamModelName, "gpt-5") {
    // 针对 gpt-5 系列的特殊处理
    if info.UpstreamModelName != "gpt-5-chat-latest" {
        request.Temperature = nil  // 某些 gpt-5 模型不支持 temperature
    }
}
```

**这意味着：**
- `openai/gpt-5.1-codex` → 去掉前缀后是 `gpt-5.1-codex` → 可能触发 `gpt-5` 的特殊处理
- `openai/gpt-5` → 去掉前缀后是 `gpt-5` → 会触发 `gpt-5` 的特殊处理
- `openai/gpt-4` → 去掉前缀后是 `gpt-4` → 不会触发 `gpt-5` 的特殊处理

### 10.3 总结

| 模型名 | 去掉前缀后 | Cursor 处理 | 特殊适配 |
|--------|-----------|------------|---------|
| `openai/gpt-5.1-codex` | `gpt-5.1-codex` | ✅ 相同 | 可能触发 `gpt-5` 适配 |
| `openai/gpt-5` | `gpt-5` | ✅ 相同 | 会触发 `gpt-5` 适配 |
| `openai/gpt-4` | `gpt-4` | ✅ 相同 | 不触发 `gpt-5` 适配 |

**结论：**
- **核心处理逻辑完全相同**：所有 `openai/` 前缀的模型都走相同的请求/响应转换流程
- **可能的差异**：去掉前缀后的模型名可能会触发不同的适配逻辑（如 `gpt-5` 系列的特殊处理）
- **这些差异不影响 Cursor 调用流程**：只是针对不同模型的上游 API 参数适配

## 九、回滚方案

如果出现问题，可以通过以下方式回滚：

1. **移除模型前缀判断**
   ```go
   // 在 valid_request.go 中注释掉
   // if strings.HasPrefix(modelName, "openai/") {
   //     isCursorRequest = true
   // }
   ```

2. **移除转换逻辑**
   ```go
   // 在 relay_responses.go 中注释掉
   // if c.GetBool("convert_responses_to_chat") {
   //     // 转换逻辑
   // }
   ```

3. **恢复原始 URL 路径**
   ```go
   // 在 relay_info.go 中恢复
   // info.RequestURLPath = "/v1/responses"  // 注释掉或删除
   ```

