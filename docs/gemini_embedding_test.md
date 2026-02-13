# Gemini Embedding 001 测试指南

本文档说明如何测试 gemini-embedding-001 模型的接入功能。

## 前提条件

1. 系统已启动并运行
2. 已配置好 Vertex AI 或 Gemini 渠道
3. 渠道中已启用 gemini-embedding-001 模型
4. 拥有有效的 API 令牌

## 测试场景

### 1. OpenAI 格式 - 单个文本 (/v1/embeddings)

**请求示例:**

```bash
curl -X POST http://localhost:3000/v1/embeddings \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_API_TOKEN" \
  -d '{
    "model": "gemini-embedding-001",
    "input": "Hello, this is a test text for embedding."
  }'
```

**预期响应:**

```json
{
  "object": "list",
  "data": [
    {
      "object": "embedding",
      "embedding": [0.123, 0.456, ...],
      "index": 0
    }
  ],
  "model": "gemini-embedding-001",
  "usage": {
    "prompt_tokens": 8,
    "total_tokens": 8
  }
}
```

### 2. OpenAI 格式 - 批量文本

**请求示例:**

```bash
curl -X POST http://localhost:3000/v1/embeddings \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_API_TOKEN" \
  -d '{
    "model": "gemini-embedding-001",
    "input": [
      "First text for embedding",
      "Second text for embedding",
      "Third text for embedding"
    ]
  }'
```

**预期响应:**

```json
{
  "object": "list",
  "data": [
    {
      "object": "embedding",
      "embedding": [0.123, 0.456, ...],
      "index": 0
    },
    {
      "object": "embedding",
      "embedding": [0.789, 0.012, ...],
      "index": 1
    },
    {
      "object": "embedding",
      "embedding": [0.345, 0.678, ...],
      "index": 2
    }
  ],
  "model": "gemini-embedding-001",
  "usage": {
    "prompt_tokens": 15,
    "total_tokens": 15
  }
}
```

### 3. OpenAI 格式 - 指定维度

**请求示例:**

```bash
curl -X POST http://localhost:3000/v1/embeddings \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_API_TOKEN" \
  -d '{
    "model": "gemini-embedding-001",
    "input": "Test text with custom dimensions",
    "dimensions": 768
  }'
```

**验证点:**
- 返回的 embedding 向量长度应该是 768

### 4. Gemini 原生格式 - 单个文本 (:embedContent)

**请求示例:**

```bash
curl -X POST http://localhost:3000/v1beta/models/gemini-embedding-001:embedContent \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_API_TOKEN" \
  -d '{
    "model": "models/gemini-embedding-001",
    "content": {
      "parts": [
        {"text": "Hello, this is a test text for embedding."}
      ]
    },
    "taskType": "RETRIEVAL_DOCUMENT"
  }'
```

**预期响应:**

```json
{
  "embedding": {
    "values": [0.123, 0.456, ...]
  }
}
```

### 5. Gemini 原生格式 - 批量文本 (:batchEmbedContents)

**请求示例:**

```bash
curl -X POST http://localhost:3000/v1beta/models/gemini-embedding-001:batchEmbedContents \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_API_TOKEN" \
  -d '{
    "requests": [
      {
        "model": "models/gemini-embedding-001",
        "content": {
          "parts": [{"text": "First text for embedding"}]
        },
        "taskType": "RETRIEVAL_DOCUMENT"
      },
      {
        "model": "models/gemini-embedding-001",
        "content": {
          "parts": [{"text": "Second text for embedding"}]
        },
        "taskType": "RETRIEVAL_QUERY"
      }
    ]
  }'
```

**预期响应:**

```json
{
  "embeddings": [
    {
      "values": [0.123, 0.456, ...]
    },
    {
      "values": [0.789, 0.012, ...]
    }
  ]
}
```

### 6. 测试不同的 task_type

支持的 task_type 值:
- `RETRIEVAL_DOCUMENT` - 文档检索 (为文档生成embedding)
- `RETRIEVAL_QUERY` - 查询检索 (为查询生成embedding)
- `SEMANTIC_SIMILARITY` - 语义相似度
- `CLASSIFICATION` - 分类
- `CLUSTERING` - 聚类

**请求示例:**

```bash
curl -X POST http://localhost:3000/v1beta/models/gemini-embedding-001:embedContent \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_API_TOKEN" \
  -d '{
    "model": "models/gemini-embedding-001",
    "content": {
      "parts": [{"text": "What is the capital of France?"}]
    },
    "taskType": "RETRIEVAL_QUERY"
  }'
```

## 渠道特定测试

### Vertex AI 渠道测试

1. 确保渠道类型为 VertexAI (类型 41)
2. 确保已配置 Service Account JSON (包含 project_id)
3. 验证 OAuth2 Token 自动获取和缓存
4. 测试不同区域 (us-central1, asia-southeast1 等)

### Gemini 渠道测试

1. 确保渠道类型为 Google Gemini
2. 确保已配置 API Key
3. 测试基本的 API Key 认证

## 计费验证

### 验证点

1. **Token 计数准确性**
   - 查看日志确认 `prompt_tokens` 与实际输入文本长度相匹配
   - 确认 `completion_tokens` 为 0 (embedding 模型无输出)
   - 确认 `total_tokens` = `prompt_tokens`

2. **扣费金额正确性**
   - 在系统中查看用户额度变化
   - 计算: 实际扣费 = token数量 × 模型倍率 × 基础价格
   - 与预期扣费金额对比

3. **使用日志记录**
   - 检查数据库中的使用日志表
   - 确认记录包含:
     - 用户ID
     - 模型名称 (gemini-embedding-001)
     - Token使用量
     - 扣费金额
     - 请求时间
     - 渠道信息

## 错误处理测试

### 1. 无效的 API Token

**请求:**
```bash
curl -X POST http://localhost:3000/v1/embeddings \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer INVALID_TOKEN" \
  -d '{
    "model": "gemini-embedding-001",
    "input": "Test text"
  }'
```

**预期:** 返回 401 Unauthorized

### 2. 空输入文本

**请求:**
```bash
curl -X POST http://localhost:3000/v1/embeddings \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_API_TOKEN" \
  -d '{
    "model": "gemini-embedding-001",
    "input": ""
  }'
```

**预期:** 返回 400 Bad Request 或从上游API返回的错误

### 3. 超长文本 (超过2048 tokens)

**请求:**
```bash
curl -X POST http://localhost:3000/v1/embeddings \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_API_TOKEN" \
  -d '{
    "model": "gemini-embedding-001",
    "input": "Very long text... (repeat many times to exceed 2048 tokens)"
  }'
```

**预期:** 上游API返回错误,系统正确转发错误信息

### 4. 不支持的维度值

**请求:**
```bash
curl -X POST http://localhost:3000/v1/embeddings \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_API_TOKEN" \
  -d '{
    "model": "gemini-embedding-001",
    "input": "Test text",
    "dimensions": 10000
  }'
```

**预期:** 上游API返回错误 (超过最大维度3072)

## 性能测试

### 批量请求测试

测试批量处理能力 (最多100个文本):

```bash
# 生成包含100个文本的JSON
cat > batch_test.json << 'EOF'
{
  "model": "gemini-embedding-001",
  "input": [
    "Text 1",
    "Text 2",
    ...
    "Text 100"
  ]
}
EOF

curl -X POST http://localhost:3000/v1/embeddings \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_API_TOKEN" \
  -d @batch_test.json
```

**验证点:**
- 响应时间合理
- 所有文本都返回了embedding
- Token计数准确

## 集成测试

### 与现有embedding模型兼容性

确认 gemini-embedding-001 与以下模型使用相同的代码路径:
- text-embedding-004
- embedding-001
- gemini-embedding-exp-03-07

**测试方法:**
1. 对相同文本使用不同模型生成embedding
2. 比较API响应格式是否一致
3. 验证计费逻辑是否相同

## 测试清单

- [ ] OpenAI格式单个文本请求
- [ ] OpenAI格式批量文本请求
- [ ] OpenAI格式指定维度请求
- [ ] Gemini原生格式单个文本请求
- [ ] Gemini原生格式批量文本请求
- [ ] 测试所有task_type参数
- [ ] Vertex AI渠道测试
- [ ] Gemini渠道测试
- [ ] Token计数验证
- [ ] 扣费金额验证
- [ ] 使用日志记录验证
- [ ] 无效Token错误处理
- [ ] 空输入错误处理
- [ ] 超长文本错误处理
- [ ] 不支持维度值错误处理
- [ ] 批量请求性能测试
- [ ] 与其他embedding模型兼容性测试

## 日志查看

在测试过程中,查看以下日志以验证功能:

```bash
# 查看系统日志
tail -f /path/to/logs/system.log | grep -i "embedding\|gemini"

# 查看请求日志
tail -f /path/to/logs/request.log | grep "gemini-embedding-001"

# 查看错误日志
tail -f /path/to/logs/error.log
```

## 数据库查询

验证计费记录:

```sql
-- 查询最近的embedding请求
SELECT * FROM logs 
WHERE model_name = 'gemini-embedding-001' 
ORDER BY created_at DESC 
LIMIT 10;

-- 查询用户余额变化
SELECT * FROM users 
WHERE id = YOUR_USER_ID;

-- 查询渠道使用统计
SELECT channel_id, COUNT(*) as request_count, SUM(quota) as total_quota
FROM logs 
WHERE model_name = 'gemini-embedding-001' 
GROUP BY channel_id;
```

## 问题排查

如果遇到问题,按以下步骤排查:

1. **请求未到达后端**
   - 检查防火墙/代理设置
   - 验证URL是否正确
   - 检查API Token格式

2. **返回500错误**
   - 查看系统日志中的错误堆栈
   - 检查渠道配置是否正确
   - 验证Service Account JSON是否有效

3. **返回的embedding格式不正确**
   - 检查响应转换逻辑
   - 验证上游API返回的原始响应

4. **Token计数不准确**
   - 检查token计数逻辑
   - 对比上游API返回的token数量

5. **扣费异常**
   - 检查模型倍率配置
   - 验证计费逻辑代码
   - 查看数据库中的扣费记录
