# Gemini Embedding 001 模型接入实施总结

## 实施日期
2026-02-13

## 实施内容

本次实施完成了 `gemini-embedding-001` 模型在 Vertex AI 和 Gemini 渠道的接入，支持 OpenAI 格式和 Gemini 原生格式的 API 调用。

## 代码变更

### 1. 模型列表更新

**文件**: `relay/channel/gemini/constant.go`

在 embedding models 部分添加了 `gemini-embedding-001`:

```go
// embedding models
"gemini-embedding-exp-03-07",
"gemini-embedding-001",  // 新增
"text-embedding-004",
"embedding-001",
```

### 2. Vertex AI 适配器增强

**文件**: `relay/channel/vertex/adaptor.go`

#### 2.1 实现 ConvertEmbeddingRequest 方法

将 OpenAI 格式的 embedding 请求转换为 Gemini 格式:

```go
func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
    // Use Gemini adaptor to convert embedding request
    geminiAdaptor := gemini.Adaptor{}
    return geminiAdaptor.ConvertEmbeddingRequest(c, info, request)
}
```

#### 2.2 GetRequestURL 方法增强

在构建请求 URL 时添加了对 embedding 模型的支持:

```go
// Check if this is an embedding model
if strings.HasPrefix(info.UpstreamModelName, "text-embedding") ||
    strings.HasPrefix(info.UpstreamModelName, "embedding") ||
    strings.HasPrefix(info.UpstreamModelName, "gemini-embedding") {
    // For embedding models, use batchEmbedContents endpoint
    action := "embedContent"
    if info.IsGeminiBatchEmbedding {
        action = "batchEmbedContents"
    }
    suffix = action
}
```

对于 embedding 模型,会构建如下 URL:
- 单个文本: `https://{region}-aiplatform.googleapis.com/v1/projects/{project_id}/locations/{region}/publishers/google/models/gemini-embedding-001:embedContent`
- 批量文本: `https://{region}-aiplatform.googleapis.com/v1/projects/{project_id}/locations/{region}/publishers/google/models/gemini-embedding-001:batchEmbedContents`

#### 2.3 DoResponse 方法增强

添加了对 embedding 响应的处理:

```go
} else if info.RelayMode == constant.RelayModeEmbeddings {
    // Handle embedding requests
    geminiAdaptor := gemini.Adaptor{}
    return geminiAdaptor.DoResponse(c, resp, info)
}
```

## 功能特性

### 支持的 API 格式

#### 1. OpenAI 格式 (`/v1/embeddings`)

```json
{
  "model": "gemini-embedding-001",
  "input": ["文本1", "文本2"],
  "dimensions": 768
}
```

#### 2. Gemini 原生格式

**单个文本** (`/v1beta/models/gemini-embedding-001:embedContent`):
```json
{
  "model": "models/gemini-embedding-001",
  "content": {
    "parts": [{"text": "文本内容"}]
  },
  "taskType": "RETRIEVAL_DOCUMENT"
}
```

**批量文本** (`/v1beta/models/gemini-embedding-001:batchEmbedContents`):
```json
{
  "requests": [
    {
      "model": "models/gemini-embedding-001",
      "content": {
        "parts": [{"text": "文本1"}]
      },
      "taskType": "RETRIEVAL_DOCUMENT"
    }
  ]
}
```

### 支持的 task_type 参数

- `RETRIEVAL_DOCUMENT` - 文档检索
- `RETRIEVAL_QUERY` - 查询检索
- `SEMANTIC_SIMILARITY` - 语义相似度
- `CLASSIFICATION` - 分类
- `CLUSTERING` - 聚类

### 支持的渠道

1. **Vertex AI 渠道**
   - 使用 Service Account JSON 配置
   - 包含 project_id 信息
   - OAuth2 认证,Token 自动缓存 35 分钟
   - 支持多区域部署

2. **Gemini 渠道**
   - 使用 API Key 认证
   - 直接访问 Google AI Studio API

## 计费机制

### Token 计算

- **输入 token**: 根据实际文本内容计算
- **输出 token**: 0 (embedding 模型无输出)
- **总 token**: 等于输入 token

### 扣费流程

1. 请求处理完成后,从响应或预估中获取 token 使用量
2. 根据模型倍率计算费用: `费用 = token数量 × 模型倍率 × 基础价格`
3. 从用户账户扣除相应金额
4. 记录使用日志到数据库

## 技术架构

### 数据流程

```
用户请求 
  → API路由层 (/v1/embeddings 或 /v1beta/models/{model}:embedContent)
  → 中间件 (鉴权、限流等)
  → EmbeddingHelper 或 GeminiEmbeddingHandler
  → 渠道选择
  → Vertex/Gemini 适配器
  → ConvertEmbeddingRequest (格式转换)
  → 构建请求 (URL + Body + Headers)
  → 认证 (Service Account OAuth2 或 API Key)
  → 发送到上游 API (Vertex AI 或 Gemini API)
  → GeminiEmbeddingHandler (响应处理)
  → 转换为 OpenAI 格式
  → 计算 token 使用量
  → postConsumeQuota (扣费)
  → 返回响应给用户
```

### 认证机制

#### Vertex AI (Service Account)
1. 从渠道配置读取 Service Account JSON
2. 解析 project_id, client_email, private_key
3. 使用 RSA 私钥签名 JWT
4. 交换 JWT 获取 OAuth2 Access Token
5. Token 缓存 35 分钟
6. 请求 Header: `Authorization: Bearer {token}`

#### Gemini (API Key)
1. 从渠道配置读取 API Key
2. 请求 Header: `x-goog-api-key: {api_key}`

## 测试资源

### 测试文档
`docs/gemini_embedding_test.md` - 包含详细的测试用例和验证步骤

### 测试脚本
`scripts/test_gemini_embedding.py` - Python 自动化测试脚本

运行测试:
```bash
python scripts/test_gemini_embedding.py YOUR_API_TOKEN
```

测试覆盖:
- OpenAI 格式单个/批量文本
- OpenAI 格式指定维度
- Gemini 原生格式单个/批量文本
- 不同 task_type 参数
- 错误处理

## 兼容性

与现有 embedding 模型共享相同代码路径:
- `text-embedding-004`
- `embedding-001`
- `gemini-embedding-exp-03-07`

确保了代码复用和一致性。

## 配置要求

### Vertex AI 渠道配置

1. 渠道类型: VertexAI (类型 41)
2. 认证方式: Service Account JSON
3. 必需字段:
   - `project_id`: GCP 项目 ID
   - `client_email`: Service Account 邮箱
   - `private_key`: RSA 私钥
   - `client_id`: 客户端 ID
4. 可选字段:
   - `api_version`: 区域设置 (默认 us-central1)
   - `proxy`: 代理服务器地址

### Gemini 渠道配置

1. 渠道类型: Google Gemini
2. 认证方式: API Key
3. 必需字段:
   - API Key

### 模型倍率配置

管理员需要在系统中配置 `gemini-embedding-001` 的计费倍率,例如:
- 输入: 0.00001 (每 1K tokens 的价格倍率)
- 输出: 0 (embedding 无输出)

## 注意事项

1. **Project 配置**: 用户无需配置 project,所有 project 信息都包含在渠道的 Service Account JSON 中
2. **Token 计数**: embedding 模型只计算输入 token,输出 token 始终为 0
3. **维度限制**: gemini-embedding-001 最大支持 3072 维,超过会被上游 API 拒绝
4. **文本长度**: 最大支持 2048 tokens,超过会被上游 API 拒绝
5. **批量限制**: batchEmbedContents 最多支持 100 个文本
6. **缓存**: Vertex AI 的 OAuth2 Token 会自动缓存,无需每次请求都获取
7. **区域选择**: 可通过渠道的 `api_version` 字段指定区域,默认使用 us-central1

## 错误处理

系统会正确处理以下错误:
- 认证失败 (401)
- 无效的 API Token
- 空输入文本
- 超长文本 (>2048 tokens)
- 不支持的维度值 (>3072)
- 网络超时
- 上游 API 错误

所有错误都会转换为标准格式返回给用户。

## 日志和监控

### 系统日志
- OAuth2 Token 获取和缓存
- 请求 URL 构建
- 模型名称映射
- 认证方式选择

### 请求日志
- 请求时间
- 用户 ID
- 模型名称
- Token 使用量
- 扣费金额
- 渠道 ID
- 响应状态码

### 错误日志
- 错误类型
- 错误消息
- 堆栈跟踪
- 请求上下文

## 性能考虑

1. **Token 缓存**: OAuth2 Token 缓存 35 分钟,减少认证请求
2. **连接复用**: HTTP 客户端复用连接
3. **批量处理**: 支持批量请求,一次性处理多个文本
4. **超时控制**: 请求超时时间为 30 秒

## 后续优化建议

1. **缓存优化**: 考虑对相同文本的 embedding 结果进行缓存
2. **速率限制**: 添加针对 embedding 请求的速率限制
3. **监控告警**: 添加 Token 使用量和错误率的监控告警
4. **成本优化**: 统计不同 task_type 的使用情况,优化成本
5. **性能测试**: 进行压力测试,确定系统承载能力

## 验收标准

- [x] 代码已实现并提交
- [x] 支持 OpenAI 格式和 Gemini 原生格式
- [x] Vertex AI 和 Gemini 渠道都能正常工作
- [x] Token 计数准确
- [x] 扣费逻辑正确
- [x] 测试文档完整
- [x] 测试脚本可用
- [x] 错误处理完善
- [x] 日志记录完整

## 文件清单

### 修改的文件
1. `relay/channel/gemini/constant.go` - 添加模型到列表
2. `relay/channel/vertex/adaptor.go` - 实现 embedding 支持

### 新增的文件
1. `docs/gemini_embedding_test.md` - 测试文档
2. `scripts/test_gemini_embedding.py` - 测试脚本
3. `docs/gemini_embedding_implementation.md` - 本实施总结

## 总结

本次实施成功地将 `gemini-embedding-001` 模型接入到系统中,完全复用了现有的代码架构,最小化了代码变更,同时保证了功能的完整性和兼容性。实现了:

1. ✅ 双格式支持 (OpenAI + Gemini 原生)
2. ✅ 双渠道支持 (Vertex AI + Gemini)
3. ✅ 完整的 task_type 参数支持
4. ✅ 准确的 Token 计算和扣费
5. ✅ 完善的错误处理
6. ✅ 详细的测试文档和脚本

用户现在可以通过标准的 API 令牌鉴权,使用 `gemini-embedding-001` 模型进行文本向量化,系统会自动根据使用量进行扣费。
