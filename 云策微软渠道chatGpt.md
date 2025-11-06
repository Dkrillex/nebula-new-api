根据提供的《Azure OpenAI API服务说明》文档，未明确提及 **`/v1/response` 接口**，但文档中“二、响应API”章节详细描述了功能与 `/v1/response` 逻辑一致的接口（路径为 `https://gpt.yunstorm.com/openai/responses`），推测可能是路径表述差异（如 `/openai/responses` 与 `/v1/response` 为同类功能接口）。以下从接口核心信息、使用示例、关键注意事项三方面解析该类“响应API”：


### 一、接口核心信息（基础配置）
该接口用于接收文本/图片输入，并返回模型生成的响应，核心连接参数如下：

| 配置项          | 具体内容                                                                 |
|-----------------|--------------------------------------------------------------------------|
| **接口秘钥**    | `$API_KEY`（用户需替换为实际分配的有效秘钥，无秘钥将无法通过鉴权）       |
| **Host 地址**   | 基础域名：`https://gpt.yunstorm.com/`；完整请求路径：`https://gpt.yunstorm.com/openai/responses` |
| **支持模型**    | gpt-4o、gpt-4o-mini、gpt-4.1、gpt-4.1-mini、gpt-4-turbo、o1、o3-mini、o4-mini、gpt-5、gpt-5-mini、gpt-5-chat |
| **API 版本**    | `2025-04-01-preview`（必须在请求中指定，否则可能导致接口调用失败）       |
| **支持输入类型** | 1. 纯文本输入；2. 文本+图片混合输入（图片支持 **URL** 或 **Base64 编码**格式） |


### 二、使用示例（代码/REST）
文档提供了 Python 代码示例（文本/图片输入）和 REST API 示例，可直接参考适配使用：

#### 1. Python 代码示例
##### （1）纯文本输入场景
适用于仅需要文本交互的需求（如问答、内容生成）：
```python
import os
from openai import AzureOpenAI

# 1. 初始化客户端（配置连接信息）
client = AzureOpenAI(
    base_url = "https://gpt.yunstorm.com/openai",  # 基础路径（固定）
    api_key='$API_KEY ',  # 替换为实际秘钥
    api_version="2025-04-01-preview"  # 固定版本号
)

# 2. 调用接口发送文本请求
response = client.responses.create(
    input=[  # 输入内容（包含system指令和user问题）
        {
            "role": "system",  # 系统角色：定义助手行为（如身份、规则）
            "content": "Assistant is a large language model trained by OpenAI.",  # 助手身份说明
        },
        {
            "role": "user",  # 用户角色：实际需求输入
            "content": "Who were the founders of Microsoft?"  # 用户问题（可替换为自定义内容）
        },
    ],
    model="o4-mini"  # 选择使用的模型（需在支持列表内）
) 

# 3. 打印响应结果（返回JSON格式数据）
print(response.to_json())
```

##### （2）文本+图片混合输入场景
适用于需要分析图片内容的需求（如图片描述、图像问答）：
```python
import os
from openai import AzureOpenAI

# 1. 初始化客户端（同文本输入）
client = AzureOpenAI(
    base_url = "https://gpt.yunstorm.com/openai",
    api_key='$API_KEY ',
    api_version="2025-04-01-preview"
)

# 2. 配置图片数据（支持URL或Base64，需替换为实际图片地址/编码）
imagedata = "picturecontent"  # 示例："https://example.com/xxx.jpg" 或 Base64字符串

# 3. 调用接口发送混合请求
response = client.responses.create(
    input=[
        {
            "role": "system",
            "content": "Assistant is a large language model trained by OpenAI.",
        },
        {
            "role": "user",
            "content": [  # 混合内容：包含图片和文本问题
                {"type": "input_text", "text": "what is in this image?"},  # 用户文本问题
                {"type": "input_image", "image_url": f"{imagedata}"}  # 图片数据
            ],
        },
    ],
    model="o4-mini",  # 建议选择支持多模态的模型（如gpt-4o、o4-mini）
)

# 4. 打印响应结果
print(response.to_json())
```

#### 2. REST API 示例（curl 命令）
适用于非 Python 环境，通过 HTTP 请求直接调用接口（以纯文本输入为例）：
```bash
curl -X POST "https://gpt.yunstorm.com/openai/responses?api-version=2025-04-01-preview" \
    -H "Content-Type: application/json" \  # 固定请求头：指定JSON格式
    -H "api-key: $API_KEY " \  # 鉴权头：替换为实际秘钥
    -d '{
    "input": [  # 输入内容（结构同Python示例）
        {
            "role": "system",
            "content": "Assistant is a large language model trained by OpenAI."
        },
        {
            "role": "user",
            "content": "Who were the founders of Microsoft?"
        }
    ],
    "model": "o4-mini"  # 选择模型
}'
```


### 三、关键注意事项
1. **秘钥与鉴权**：`$API_KEY` 必须替换为实际有效的企业秘钥，缺失或错误会返回“401 未授权”错误。
2. **模型选择**：
   - 文本输入：所有支持模型均可使用；
   - 图片输入：需选择支持多模态的模型（如 gpt-4o、o4-mini），使用不支持图片的模型会导致请求失败。
3. **图片格式**：图片输入仅支持 **URL** 或 **Base64 编码**，需确保 URL 可公开访问（无权限限制）、Base64 编码完整（无截断）。
4. **API 版本**：必须指定 `api-version=2025-04-01-preview`，文档中未提及其他版本兼容性，不建议随意修改。
5. **响应格式**：接口返回结果为 JSON 格式，可通过 `response.to_json()`（Python）或直接解析 curl 响应体获取关键信息（如生成内容、模型信息、耗时等）。