# AWS Bedrock - Thinking Blocks 验证错误

## 错误信息

```
API Error: 400
ValidationException: 'thinking' or 'redacted_thinking' blocks in the latest assistant message 
cannot be modified. These blocks must remain as they were in the original response.
```

## 快速诊断

| 问题 | 答案 |
|------|------|
| **错误类型** | 用户调用错误（参数问题） |
| **影响范围** | 仅 AWS Bedrock Claude 渠道 |
| **触发场景** | 多轮对话时修改了历史消息中的 thinking 块 |
| **严重程度** | ⚠️ 中等 - 会导致请求失败 |

## 问题原因

### 为什么会出现这个错误？

Claude 的 Extended Thinking 功能会在响应中返回 `thinking` 块，这些块包含：
- `type`: "thinking" 或 "redacted_thinking"
- `thinking`: 思考过程的文本内容
- `signature`: 数字签名，用于验证完整性

**AWS Bedrock 会验证这个签名**，确保 thinking 块没有被篡改。如果检测到修改，就会拒绝请求。

### 常见触发场景

#### ❌ 场景 1：修改了 thinking 内容
```json
// 用户错误地编辑了 thinking 文本
{
  "type": "thinking",
  "thinking": "我修改了这里的内容",  // ❌ 签名会失效
  "signature": "original_signature_here"
}
```

#### ❌ 场景 2：删除了 signature 字段
```json
{
  "type": "thinking",
  "thinking": "Let me think about this..."
  // ❌ 缺少 signature 字段
}
```

#### ❌ 场景 3：手动伪造 thinking 块
```json
// 用户在 assistant 消息中手动添加
{
  "role": "assistant",
  "content": [
    {
      "type": "thinking",
      "thinking": "假的思考内容",  // ❌ 伪造的
      "signature": "fake_sig"
    }
  ]
}
```

## 解决方案

### 方案 1：完整保留 thinking 块（推荐）

**如果需要保留思考过程**，将 Claude 返回的内容完整保存：

```javascript
// ✅ 正确：完整保留响应
const response = await callClaude({
  messages: history
});

// 将完整的 content 保存到历史记录
history.push({
  role: "assistant",
  content: response.content  // 包含所有 thinking 块，保持原样
});
```

### 方案 2：移除所有 thinking 块（简单）

**如果不需要思考过程**，在保存历史前移除：

```javascript
// ✅ 正确：过滤掉 thinking 块
function removeThinkingBlocks(content) {
  if (typeof content === 'string') return content;
  
  if (Array.isArray(content)) {
    return content.filter(block => 
      block.type !== 'thinking' && 
      block.type !== 'redacted_thinking'
    );
  }
  
  return content;
}

// 使用
history.push({
  role: "assistant",
  content: removeThinkingBlocks(response.content)
});
```

### 方案 3：只保留文本内容（最简单）

**如果只需要纯文本回复**：

```javascript
// ✅ 正确：只提取文本
function extractText(content) {
  if (typeof content === 'string') return content;
  
  if (Array.isArray(content)) {
    return content
      .filter(block => block.type === 'text')
      .map(block => block.text)
      .join('');
  }
  
  return content;
}

// 使用
history.push({
  role: "assistant",
  content: extractText(response.content)
});
```

## 代码示例

### Python 示例

```python
def clean_assistant_message(content):
    """清理 assistant 消息，移除可能导致验证失败的 thinking 块"""
    if isinstance(content, str):
        return content
    
    if isinstance(content, list):
        # 方案1：移除所有 thinking 块
        return [
            block for block in content 
            if block.get('type') not in ['thinking', 'redacted_thinking']
        ]
        
        # 或方案2：只保留有 signature 的 thinking 块
        # cleaned = []
        # for block in content:
        #     if block.get('type') in ['thinking', 'redacted_thinking']:
        #         if 'signature' in block:
        #             cleaned.append(block)  # 保留完整的 thinking 块
        #     else:
        #         cleaned.append(block)
        # return cleaned
    
    return content

# 使用示例
history = []

# 第一轮对话
response = client.messages.create(
    model="claude-3-7-sonnet-20250219",
    messages=[{"role": "user", "content": "Hello"}]
)

# 清理后保存
history.append({
    "role": "assistant",
    "content": clean_assistant_message(response.content)
})

# 第二轮对话
response = client.messages.create(
    model="claude-3-7-sonnet-20250219",
    messages=history + [{"role": "user", "content": "Next question"}]
)
```

### JavaScript/TypeScript 示例

```typescript
interface ThinkingBlock {
  type: 'thinking' | 'redacted_thinking';
  thinking: string;
  signature: string;
}

interface TextBlock {
  type: 'text';
  text: string;
}

type ContentBlock = ThinkingBlock | TextBlock | any;

function cleanAssistantContent(
  content: string | ContentBlock[]
): string | ContentBlock[] {
  if (typeof content === 'string') {
    return content;
  }

  // 过滤掉 thinking 块
  return content.filter(block => 
    block.type !== 'thinking' && 
    block.type !== 'redacted_thinking'
  );
}

// 使用示例
const history: Array<{role: string; content: any}> = [];

const response1 = await claude.messages.create({
  model: "claude-3-7-sonnet-20250219",
  messages: [{role: "user", content: "Hello"}]
});

// 清理后保存到历史
history.push({
  role: "assistant",
  content: cleanAssistantContent(response1.content)
});

// 继续对话
const response2 = await claude.messages.create({
  model: "claude-3-7-sonnet-20250219",
  messages: [
    ...history,
    {role: "user", content: "Next question"}
  ]
});
```

## 系统层面的保护

### 已实施的自动保护

本系统已在 `relay/helper/valid_request.go` 中添加了自动保护逻辑：

1. **自动检测**：检查 assistant 消息中的 thinking 块
2. **验证 signature**：移除没有 signature 字段的 thinking 块（可能是伪造的）
3. **保留合法块**：有 signature 的 thinking 块会被保留

这意味着：
- ✅ 用户伪造的 thinking 块会被自动移除
- ✅ 合法的历史 thinking 块会被保留
- ⚠️ 但如果用户修改了 thinking 内容但保留了旧签名，仍会验证失败

### 建议

即使有系统保护，**仍建议客户端应用主动处理**：
1. 要么完整保留 thinking 块（包括 signature）
2. 要么完全移除 thinking 块

## 相关配置

### Extended Thinking 模型

使用 `-thinking` 后缀的模型会自动启用 thinking 功能：

```bash
# 自动启用 thinking
claude-3-7-sonnet-20250219-thinking

# 不启用 thinking
claude-3-7-sonnet-20250219
```

### Thinking 配置参数

```json
{
  "thinking": {
    "type": "enabled",
    "budget_tokens": 2000  // 思考预算
  }
}
```

**注意**：`budget_tokens` 必须小于 `max_tokens`

## 调试技巧

### 如何判断是否是这个问题？

1. 查看错误信息：包含 "thinking" 或 "redacted_thinking blocks cannot be modified"
2. 检查是否是多轮对话
3. 检查历史消息中是否有 assistant 角色的消息包含 thinking 块

### 如何验证修复？

```bash
# 测试单轮对话（不应该有问题）
curl -X POST /v1/messages \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-3-7-sonnet-20250219",
    "messages": [{"role": "user", "content": "Hello"}]
  }'

# 测试多轮对话（带 thinking 块）
# 应该能正常工作（系统会自动清理无效的 thinking 块）
```

## 常见问题

### Q: 为什么 Claude 要添加 signature 验证？

**A:** 防止用户伪造或篡改 AI 的思考过程，确保完整性和可信度。

### Q: 可以手动创建 thinking 块吗？

**A:** ❌ 不可以。thinking 块必须由 Claude 生成，包含有效的数字签名。

### Q: 如果我想保留思考过程怎么办？

**A:** ✅ 完整保留 Claude 返回的 content，不要修改任何字段。

### Q: 系统会自动处理这个问题吗？

**A:** ⚠️ 部分处理。系统会移除明显伪造的 thinking 块（无 signature），但无法修复被篡改的块（有 signature 但内容被修改）。

### Q: 这个问题只在 AWS Bedrock 上出现吗？

**A:** ✅ 是的。Anthropic 官方 API 也有 thinking 功能，但验证机制可能不同。

## 相关资源

- [文档：Thinking Blocks 详细说明](../bedrock_thinking_blocks_error.md)
- [Claude Extended Thinking 官方文档](https://docs.anthropic.com/en/docs/build-with-claude/extended-thinking)
- [AWS Bedrock Claude 文档](https://docs.aws.amazon.com/bedrock/latest/userguide/model-parameters-anthropic-claude-messages.html)
- 代码文件：
  - `relay/helper/valid_request.go` - 请求验证和清理逻辑
  - `relay/channel/aws/dto.go` - AWS Bedrock 请求处理
  - `relay/claude_handler.go` - Claude 请求处理

## 更新日志

- **2026-01-27**: 添加自动 thinking 块清理逻辑
- **2026-01-27**: 创建文档和用户指南
