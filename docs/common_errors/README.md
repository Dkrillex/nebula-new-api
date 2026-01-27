# 常见错误处理指南

本目录包含常见 API 错误的诊断和解决方案。

## 错误列表

### AWS Bedrock 相关

#### [Thinking Blocks 验证错误](./bedrock_thinking_validation_error.md)
- **错误码**: 400 ValidationException
- **关键词**: `thinking blocks cannot be modified`
- **类型**: 用户调用问题
- **影响**: AWS Bedrock Claude 渠道
- **解决**: 避免修改历史消息中的 thinking 块

## 使用指南

### 如何查找错误解决方案

1. **通过错误码查找**
   - 400: 请求参数错误
   - 401: 认证失败
   - 403: 权限不足
   - 429: 请求过多
   - 500: 服务器错误

2. **通过关键词搜索**
   ```bash
   # 在本目录搜索错误信息中的关键词
   grep -r "thinking blocks" .
   grep -r "ValidationException" .
   ```

3. **通过渠道类型查找**
   - AWS Bedrock
   - Claude (Anthropic)
   - OpenAI
   - Gemini
   - 等等

## 错误诊断流程

```
收到错误 → 查看错误码 → 查看错误消息 → 搜索本目录 → 按照解决方案操作
```

## 需要帮助？

如果本目录中没有您遇到的错误：

1. 查看主文档目录: `../`
2. 检查相关渠道的文档
3. 提交 Issue 或咨询支持团队

## 贡献

发现新的常见错误？欢迎提交文档！

文档格式参考：
- 错误信息示例
- 问题原因分析
- 解决方案（多个备选）
- 代码示例
- 预防措施
