# AWS Bedrock Claude API - Thinking Blocks 错误说明

## 错误信息

```
API Error: 400 {"error":{"message":"InvokeModelWithResponseStream: operation error Bedrock Runtime: 
InvokeModelWithResponseStream, https response error StatusCode: 400, ValidationException: 
***.***,content,1: 'thinking' or 'redacted_thinking' blocks in the latest assistant message 
cannot be modified. These blocks must remain as they were in the original response."}}
```

## 问题性质

**这是用户调用问题，不是代码问题。**

## 根本原因

AWS Bedrock Claude API 对 `thinking` 和 `redacted_thinking` 块有严格的完整性校验：

1. **多轮对话场景**：当进行多轮对话时，历史消息中的 assistant 角色消息如果包含 `thinking` 块，必须保持原样
2. **防篡改机制**：Claude 使用签名(`signature` 字段)来验证 thinking 块的完整性
3. **不允许伪造**：用户不能手动创建或修改 thinking 块的内容

## 触发条件

这个错误通常在以下场景发生：

### 场景 1：修改历史 thinking 块
```json
// 第一轮：Claude 返回
{
  "role": "assistant",
  "content": [
    {
      "type": "thinking",
      "thinking": "Let me analyze this...",
      "signature": "abc123..."
    },
    {
      "type": "text",
      "text": "Here's my answer..."
    }
  ]
}

// 第二轮：用户错误地修改了 thinking 内容
{
  "messages": [
    {
      "role": "assistant",
      "content": [
        {
          "type": "thinking",
          "thinking": "Modified thinking content",  // ❌ 错误：修改了原始内容
          "signature": "abc123..."
        },
        {
          "type": "text", 
          "text": "Here's my answer..."
        }
      ]
    },
    {
      "role": "user",
      "content": "Next question..."
    }
  ]
}
```

### 场景 2：删除 thinking 块的字段
```json
// ❌ 错误：删除了 signature 或其他必需字段
{
  "type": "thinking",
  "thinking": "Let me analyze this..."
  // 缺少 signature 字段
}
```

### 场景 3：手动伪造 thinking 块
```json
// ❌ 错误：用户在 assistant 消息中手动添加 thinking 块
{
  "role": "assistant",
  "content": [
    {
      "type": "thinking",
      "thinking": "Fake thinking content",  // 伪造的 thinking
      "signature": "fake_signature"
    }
  ]
}
```

## 解决方案

### 方案 1：完整保留 thinking 块（推荐）

在多轮对话中，将 Claude 返回的 assistant 消息**完整保留**，不做任何修改：

```javascript
// ✅ 正确做法
const history = [];

// 第一轮对话
const response1 = await claude.messages.create({
  model: "claude-3-7-sonnet-20250219",
  messages: [{
    role: "user",
    content: "Solve this problem..."
  }]
});

// 完整保留响应（包括所有 thinking 块）
history.push({
  role: "user", 
  content: "Solve this problem..."
});
history.push({
  role: "assistant",
  content: response1.content  // ✅ 完整保留，不修改
});

// 第二轮对话
const response2 = await claude.messages.create({
  model: "claude-3-7-sonnet-20250219",
  messages: [
    ...history,
    {
      role: "user",
      content: "Next question..."
    }
  ]
});
```

### 方案 2：移除所有 thinking 块

如果不需要保留思考过程，可以在存储历史消息时**完全移除** thinking 块：

```javascript
// ✅ 正确做法：过滤掉 thinking 块
function filterThinkingBlocks(content) {
  if (typeof content === 'string') {
    return content;
  }
  
  if (Array.isArray(content)) {
    return content.filter(block => 
      block.type !== 'thinking' && 
      block.type !== 'redacted_thinking'
    );
  }
  
  return content;
}

// 存储历史时过滤
history.push({
  role: "assistant",
  content: filterThinkingBlocks(response.content)  // ✅ 移除 thinking 块
});
```

### 方案 3：只保留文本内容

如果只需要文本回复，提取 text 类型的内容：

```javascript
// ✅ 正确做法：只保留 text 块
function extractTextContent(content) {
  if (typeof content === 'string') {
    return content;
  }
  
  if (Array.isArray(content)) {
    const textBlocks = content.filter(block => block.type === 'text');
    if (textBlocks.length === 1) {
      return textBlocks[0].text;
    }
    return textBlocks;
  }
  
  return content;
}

history.push({
  role: "assistant",
  content: extractTextContent(response.content)  // ✅ 只保留文本
});
```

## 代码层面的预防措施

### 在中间件层添加验证（可选）

可以在请求处理前验证和清理 thinking 块：

```go
// 示例：在 relay/claude_handler.go 或中间件中添加
func validateAndCleanThinkingBlocks(messages []dto.ClaudeMessage) []dto.ClaudeMessage {
    for i := range messages {
        if messages[i].Role == "assistant" {
            // 检查 content 是否包含 thinking 块
            if content, ok := messages[i].Content.([]interface{}); ok {
                cleaned := make([]interface{}, 0)
                for _, item := range content {
                    if block, ok := item.(map[string]interface{}); ok {
                        blockType := block["type"]
                        // 跳过 thinking 和 redacted_thinking 块
                        if blockType == "thinking" || blockType == "redacted_thinking" {
                            // 选项1：完全跳过（如果没有正确的签名）
                            if _, hasSignature := block["signature"]; !hasSignature {
                                continue
                            }
                            // 选项2：如果有签名但内容被修改，也跳过
                            // 这里需要更复杂的验证逻辑
                        }
                        cleaned = append(cleaned, item)
                    }
                }
                messages[i].Content = cleaned
            }
        }
    }
    return messages
}
```

## 最佳实践建议

### 1. 客户端开发者

- ✅ **完整保留**：将 Claude 返回的 content 完整保存，不做任何修改
- ✅ **过滤移除**：如果不需要 thinking，在存储前完全移除这些块
- ❌ **禁止修改**：绝不修改 thinking 块的任何字段（thinking、signature 等）
- ❌ **禁止伪造**：不要在 assistant 消息中手动添加 thinking 块

### 2. 前端应用

```typescript
// ✅ 推荐的消息处理方式
interface Message {
  role: 'user' | 'assistant';
  content: string | Array<ContentBlock>;
}

// 存储到历史记录时
function storeMessage(response: ClaudeResponse): Message {
  return {
    role: 'assistant',
    // 选项1：完整保留（如果需要展示思考过程）
    content: response.content,
    
    // 选项2：只保留非 thinking 的内容
    // content: response.content.filter(b => 
    //   b.type !== 'thinking' && b.type !== 'redacted_thinking'
    // ),
    
    // 选项3：只保留文本
    // content: response.content
    //   .filter(b => b.type === 'text')
    //   .map(b => b.text)
    //   .join('')
  };
}
```

### 3. 后端 API 开发者

- ✅ 在文档中明确说明 thinking 块的处理规则
- ✅ 提供辅助函数帮助用户正确处理历史消息
- ⚠️ 考虑在代理层自动过滤无效的 thinking 块（可选）

## 相关字段说明

AWS Bedrock Claude API 中 thinking 块的完整结构：

```json
{
  "type": "thinking",           // 块类型，必须是 "thinking" 或 "redacted_thinking"
  "thinking": "...",            // 思考内容（可能很长）
  "signature": "..."            // 完整性签名，用于验证内容未被篡改
}
```

**重要字段：**
- `type`: 标识块类型
- `thinking`: 实际的思考内容
- `signature`: Claude 生成的签名，用于验证完整性，**不能伪造或修改**

## 总结

| 问题 | 回答 |
|------|------|
| **是代码问题还是用户问题？** | **用户调用问题** - 用户在多轮对话中错误地修改了 thinking 块 |
| **如何避免？** | 方案1：完整保留原始 content<br>方案2：完全移除 thinking 块<br>方案3：只提取 text 类型内容 |
| **能否修改 thinking？** | ❌ **绝对不能** - 会导致签名验证失败 |
| **能否手动创建 thinking？** | ❌ **不能** - 缺少有效签名会被拒绝 |
| **能否删除 thinking？** | ✅ **可以** - 完全移除不会有问题 |

## 参考资料

- [Claude Extended Thinking 官方文档](https://docs.anthropic.com/en/docs/build-with-claude/extended-thinking)
- [AWS Bedrock Claude API 文档](https://docs.aws.amazon.com/bedrock/latest/userguide/model-parameters-anthropic-claude-messages.html)
- 相关代码文件：
  - `relay/channel/aws/dto.go` - AWS Bedrock 请求处理
  - `relay/claude_handler.go` - Claude 请求处理
  - `dto/claude.go` - Claude 数据结构定义
