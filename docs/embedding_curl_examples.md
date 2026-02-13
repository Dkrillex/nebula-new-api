# Embedding 接口 cURL 示例

假设：
- 中继 base URL：`https://your-relay-host/v1`（请替换为实际地址）
- 鉴权：`Authorization: Bearer YOUR_API_KEY`（或你配置的 key）
- 模型名：`gemini-embedding-001`（可改为你渠道里配置的 embedding 模型）

**说明**：Gemini 原生接口的模型由 **URL 路径** 指定（如 `/v1/models/gemini-embedding-001:embedContent`），请求体里**不要带** `model` 字段，否则可能报错或与路径不一致。

---

## 1. Gemini 原生单条：embedContent

**路径**：`POST /v1/models/{model}:embedContent`

```bash
curl -X POST "https://your-relay-host/v1/models/gemini-embedding-001:embedContent" \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "content": {
      "parts": [
        { "text": "要嵌入的文本内容" }
      ]
    }
  }'
```

可选：`outputDimensionality`（仅部分模型支持，如 text-embedding-004、gemini-embedding-001）：

```bash
curl -X POST "https://your-relay-host/v1/models/gemini-embedding-001:embedContent" \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "content": {
      "parts": [
        { "text": "要嵌入的文本内容" }
      ]
    },
    "outputDimensionality": 768
  }'
```

---

## 2. Gemini 原生批量：batchEmbedContents

**路径**：`POST /v1/models/{model}:batchEmbedContents`

模型由 URL 指定，每个 `requests` 项里**不要带** `model`，只保留 `content`：

```bash
curl -X POST "https://your-relay-host/v1/models/gemini-embedding-001:batchEmbedContents" \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "requests": [
      {
        "content": {
          "parts": [
            { "text": "第一段文本" }
          ]
        }
      },
      {
        "content": {
          "parts": [
            { "text": "第二段文本" }
          ]
        }
      }
    ]
  }'
```

---

## 3. OpenAI 兼容单条：/v1/embeddings（单 input）

**路径**：`POST /v1/embeddings`

```bash
curl -X POST "https://your-relay-host/v1/embeddings" \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-embedding-001",
    "input": "要嵌入的文本内容"
  }'
```

---

## 4. OpenAI 兼容批量：/v1/embeddings（input 数组）

**路径**：`POST /v1/embeddings`

```bash
curl -X POST "https://your-relay-host/v1/embeddings" \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-embedding-001",
    "input": [
      "第一段文本",
      "第二段文本"
    ]
  }'
```

可选：指定维度（部分模型支持）：

```bash
curl -X POST "https://your-relay-host/v1/embeddings" \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-embedding-001",
    "input": ["文本1", "文本2"],
    "dimensions": 768
  }'
```

---

## 路径与格式对照

| 类型           | 路径                                   | 单条/批量 |
|----------------|----------------------------------------|-----------|
| Gemini 原生    | `/v1/models/{model}:embedContent`      | 单条      |
| Gemini 原生    | `/v1/models/{model}:batchEmbedContents`| 批量      |
| OpenAI 兼容    | `/v1/embeddings`                       | 单条/批量（由 `input` 为 string 或 array 决定） |

鉴权方式以你当前配置为准（如 `Authorization: Bearer <key>` 或 `x-api-key` 等）。

---

## 常见报错与排查

- **不要在请求体里带 model**：Gemini 原生路径（`:embedContent` / `:batchEmbedContents`）的模型由 URL 里的 `{model}` 指定，请求体里不要再写 `model`，否则可能报“模型与路径不一致”或上游校验错误。
- **content 必填**：单条/批量里的 `content.parts` 必填，且至少有一个 `text` 非空；否则会报参数错误。
- **JSON 与编码**：`-d` 用单引号包 JSON、中文无需转义；若在 Windows 或脚本里用双引号，注意内部双引号要转义 `\"`。
- **鉴权**：401 检查 API Key 或 Bearer token 是否正确、是否带在正确的 Header 上。
