# Gemini Embedding 001 模型使用指南

## 简介

`gemini-embedding-001` 是 Google 最新的文本向量化模型,支持英语、多语言和代码任务,输出维度最高可达 3072,最大序列长度为 2048 tokens。

## 快速开始

### 1. 配置渠道

#### Vertex AI 渠道

在系统中添加 Vertex AI 渠道:

1. 渠道类型: `VertexAI`
2. 认证方式: Service Account JSON
3. 上传 Service Account JSON 文件 (包含 project_id)
4. 启用 `gemini-embedding-001` 模型

#### Gemini 渠道

在系统中添加 Gemini 渠道:

1. 渠道类型: `Google Gemini`
2. 认证方式: API Key
3. 输入您的 Google AI Studio API Key
4. 启用 `gemini-embedding-001` 模型

### 2. 获取 API 令牌

在系统中创建或使用现有的 API 令牌。

### 3. 调用 API

系统支持三种 API 调用格式，您可以根据需求选择：

#### 方式一：OpenAI 格式 (推荐，兼容性最好)

**基本调用:**

```bash
curl -X POST http://your-api-endpoint/v1/embeddings \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_API_TOKEN" \
  -d '{
    "model": "gemini-embedding-001",
    "input": "你的文本内容"
  }'
```

**批量处理:**

```bash
curl -X POST http://your-api-endpoint/v1/embeddings \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_API_TOKEN" \
  -d '{
    "model": "gemini-embedding-001",
    "input": ["文本1", "文本2", "文本3"]
  }'
```

**指定维度:**

```bash
curl -X POST http://your-api-endpoint/v1/embeddings \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_API_TOKEN" \
  -d '{
    "model": "gemini-embedding-001",
    "input": "你的文本内容",
    "dimensions": 768
  }'
```

#### 方式二：Gemini 原生格式

**单个文本:**

```bash
curl -X POST http://your-api-endpoint/v1beta/models/gemini-embedding-001:embedContent \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_API_TOKEN" \
  -d '{
    "content": {
      "parts": [{"text": "你的文本内容"}]
    },
    "taskType": "RETRIEVAL_DOCUMENT"
  }'
```

**批量文本:**

```bash
curl -X POST http://your-api-endpoint/v1beta/models/gemini-embedding-001:batchEmbedContents \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_API_TOKEN" \
  -d '{
    "requests": [
      {
        "content": {
          "parts": [{"text": "文本1"}]
        },
        "taskType": "RETRIEVAL_DOCUMENT"
      },
      {
        "content": {
          "parts": [{"text": "文本2"}]
        },
        "taskType": "RETRIEVAL_QUERY"
      }
    ]
  }'
```

> **注意**: Gemini 原生格式请求体中**不要包含 `model` 字段**（模型已在 URL 中指定）

#### 方式三：Vertex AI 原生格式 (instances)

**单个文本:**

```bash
curl -X POST http://your-api-endpoint/v1beta/models/gemini-embedding-001:predict \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_API_TOKEN" \
  -d '{
    "instances": [
      {
        "task_type": "RETRIEVAL_DOCUMENT",
        "title": "文档标题（可选）",
        "content": "你的文本内容"
      }
    ]
  }'
```

**批量文本:**

```bash
curl -X POST http://your-api-endpoint/v1beta/models/gemini-embedding-001:predict \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_API_TOKEN" \
  -d '{
    "instances": [
      {
        "task_type": "RETRIEVAL_DOCUMENT",
        "title": "文档1标题",
        "content": "文本内容1"
      },
      {
        "task_type": "RETRIEVAL_QUERY",
        "content": "查询文本2"
      }
    ]
  }'
```

## Task Type 参数

使用 Gemini 原生格式时,可以指定 `taskType` 来优化特定用途的 embedding:

- `RETRIEVAL_DOCUMENT`: 为文档生成 embedding (用于检索时的语料库)
- `RETRIEVAL_QUERY`: 为查询生成 embedding (用于检索时的问题)
- `SEMANTIC_SIMILARITY`: 语义相似度计算
- `CLASSIFICATION`: 文本分类
- `CLUSTERING`: 文本聚类

**示例:**

```python
import requests

# 为文档库生成 embedding
documents = ["文档1内容", "文档2内容", "文档3内容"]
doc_embeddings = []

for doc in documents:
    response = requests.post(
        "http://your-api-endpoint/v1beta/models/gemini-embedding-001:embedContent",
        headers={
            "Content-Type": "application/json",
            "Authorization": "Bearer YOUR_API_TOKEN"
        },
        json={
            "model": "models/gemini-embedding-001",
            "content": {"parts": [{"text": doc}]},
            "taskType": "RETRIEVAL_DOCUMENT"
        }
    )
    doc_embeddings.append(response.json()["embedding"]["values"])

# 为查询生成 embedding
query = "用户的查询问题"
response = requests.post(
    "http://your-api-endpoint/v1beta/models/gemini-embedding-001:embedContent",
    headers={
        "Content-Type": "application/json",
        "Authorization": "Bearer YOUR_API_TOKEN"
    },
    json={
        "model": "models/gemini-embedding-001",
        "content": {"parts": [{"text": query}]},
        "taskType": "RETRIEVAL_QUERY"
    }
)
query_embedding = response.json()["embedding"]["values"]

# 计算相似度并检索最相关的文档
# ...
```

## Python SDK 示例

```python
import openai

openai.api_key = "YOUR_API_TOKEN"
openai.api_base = "http://your-api-endpoint/v1"

# 单个文本
response = openai.Embedding.create(
    model="gemini-embedding-001",
    input="你的文本内容"
)
embedding = response['data'][0]['embedding']

# 批量文本
response = openai.Embedding.create(
    model="gemini-embedding-001",
    input=["文本1", "文本2", "文本3"]
)
embeddings = [item['embedding'] for item in response['data']]
```

## 计费说明

- 仅按输入 token 计费
- 输出 token 为 0
- 计费公式: `费用 = 输入token数 × 模型倍率 × 基础价格`
- 具体价格请咨询管理员或查看系统配置

## 技术规格

- **最大输入长度**: 2048 tokens
- **输出维度**: 1-3072 (默认为模型原生维度)
- **支持语言**: 英语、中文、日语等多种语言
- **支持内容**: 文本、代码

## 常见问题

### Q: 如何选择合适的维度?

A: 
- 较低维度 (128-768): 适合内存受限场景,检索速度快
- 中等维度 (768-1536): 平衡性能和质量
- 高维度 (1536-3072): 最佳质量,适合精确匹配

### Q: RETRIEVAL_DOCUMENT 和 RETRIEVAL_QUERY 有什么区别?

A: 
- `RETRIEVAL_DOCUMENT`: 用于索引文档库,优化文档的表示
- `RETRIEVAL_QUERY`: 用于查询时,优化问题的表示
- 使用正确的 task_type 可以提高检索准确率

### Q: 为什么我的请求返回 401?

A: 
- 检查 API Token 是否正确
- 检查 Token 是否过期
- 确认 Authorization Header 格式: `Bearer YOUR_TOKEN`

### Q: 批量请求有数量限制吗?

A: 
- OpenAI 格式: 建议不超过 100 个文本
- Gemini 原生格式: 最多 100 个请求

### Q: 支持哪些编程语言?

A: 
- 支持所有能发起 HTTP 请求的语言
- Python, JavaScript, Java, Go, PHP, Ruby 等
- 可以使用 OpenAI SDK (设置自定义 api_base)

## 更多资源

- [详细测试文档](./gemini_embedding_test.md)
- [实施总结](./gemini_embedding_implementation.md)
- [测试脚本](../scripts/test_gemini_embedding.py)

## 支持

如有问题,请:
1. 查看系统日志
2. 运行测试脚本诊断
3. 联系系统管理员
