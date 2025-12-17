# LangChain 集成Nebula Lab 使用指南


## 一、简介
LangChain 是一个用于开发基于语言模型的应用程序的强大框架。通过集成 **Nebula Lab**，可在 LangChain 中灵活调用各类主流 AI 模型，快速实现对话、问答、Agent 等复杂功能。


## 二、快速开始

### 1. 安装依赖
```bash
pip install langchain langchain-openai
```


### 2. 基础配置
```python
import os
from langchain_openai import ChatOpenAI

# 设置环境变量（Nebula Lab配置）
os.environ["OPENAI_API_KEY"] = "您的Nebula Lab密钥"  # 替换为实际密钥
os.environ["OPENAI_BASE_URL"] = "https://llm.ai-nebula.com/v1"  # 固定API地址

# 初始化模型（以gpt-3.5-turbo为例）
llm = ChatOpenAI(
    model="gpt-3.5-turbo",  # 模型名称
    temperature=0.7  # 生成随机性（0-1，越高越随机）
)
```


## 三、核心功能

### 1. 基础对话
通过 `SystemMessage`（系统提示）和 `HumanMessage`（用户输入）实现简单对话：
```python
from langchain.schema import HumanMessage, SystemMessage

# 定义对话内容
messages = [
    SystemMessage(content="你是一个有帮助的助手，用简洁的语言回答问题"),
    HumanMessage(content="介绍一下 Python 的主要特点")
]

# 调用模型
response = llm.invoke(messages)
print(response.content)  # 输出模型回复
```


### 2. 对话链（带记忆功能）
通过 `ConversationChain` 实现多轮对话记忆：
```python
from langchain.memory import ConversationBufferMemory
from langchain.chains import ConversationChain

# 创建记忆组件（存储对话历史）
memory = ConversationBufferMemory()

# 初始化带记忆的对话链
conversation = ConversationChain(
    llm=llm,
    memory=memory,
    verbose=True  # 打印链执行过程（调试用）
)

# 多轮对话（模型会记住上下文）
conversation.predict(input="我想学习机器学习")
conversation.predict(input="推荐一些入门资源")  # 模型已知上文"学习机器学习"
```


### 3. 文档问答系统（RAG）
结合文档内容回答问题（检索增强生成）：
```python
from langchain.document_loaders import TextLoader
from langchain.text_splitter import CharacterTextSplitter
from langchain.embeddings import OpenAIEmbeddings
from langchain.vectorstores import FAISS
from langchain.chains import RetrievalQA

# 配置嵌入模型（用于文档向量化）
embeddings = OpenAIEmbeddings(
        api_key="您的Nebula lab密钥",
        base_url="https://llm.ai-nebula.com/v1"
)

# 1. 加载文档（示例：加载本地text文件）
loader = TextLoader("document.txt")  # 替换为实际文档路径
documents = loader.load()

# 2. 分割文本（避免文档过长超出模型上下文）
text_splitter = CharacterTextSplitter(
    chunk_size=1000,  # 每个片段长度
    chunk_overlap=0   # 片段重叠长度（增强上下文关联）
)
texts = text_splitter.split_documents(documents)

# 3. 创建向量存储（用于快速检索相关片段）
vectorstore = FAISS.from_documents(texts, embeddings)

# 4. 构建问答链（结合模型与检索结果）
qa = RetrievalQA.from_chain_type(
    llm=llm,
    chain_type="stuff",  # 将检索到的内容"填充"到prompt中
    retriever=vectorstore.as_retriever()  # 检索器
)

# 5. 提问（模型会基于文档内容回答）
result = qa.run("文档中的关键概念是什么？")
print(result)
```


## 四、模型切换
支持调用多种主流模型，只需修改 `model` 参数：
```python
# GPT-4 模型
gpt4 = ChatOpenAI(
    model="gpt-4",
    api_key="您的Nebula lab密钥",
    base_url="https://llm.ai-nebula.com/v1"
)

# Claude 3 模型
claude = ChatOpenAI(
    model="claude-3-opus-20240229",
    api_key="您的Nebula lab密钥",
    base_url="https://llm.ai-nebula.com/v1"
)

# 比较不同模型的回答
question = "解释量子计算的基本原理"
gpt4_answer = gpt4.invoke([HumanMessage(content=question)])
claude_answer = claude.invoke([HumanMessage(content=question)])
```


## 五、高级应用

### 1. Agent 系统（工具调用）
让模型自主调用工具完成任务：
```python
from langchain.agents import create_openai_functions_agent, AgentExecutor
from langchain.tools import Tool
from langchain import hub

# 定义工具（示例：获取天气信息）
def get_weather(location: str) -> str:
    """获取指定地点的天气信息"""
    return f"{location}的天气是晴天，温度25°C"  # 实际场景可对接天气API

# 包装工具
weather_tool = Tool(
    name="Weather",  # 工具名称
    func=get_weather,  # 工具函数
    description="获取指定地点的天气信息（当用户问天气时使用）"  # 工具描述（帮助模型判断何时使用）
)

# 创建Agent
prompt = hub.pull("hwchase17/openai-functions-agent")  # 拉取预设prompt
agent = create_openai_functions_agent(llm, [weather_tool], prompt)
agent_executor = AgentExecutor(agent=agent, tools=[weather_tool])  # 执行器

# 使用Agent（模型会自动调用天气工具）
result = agent_executor.invoke({"input": "北京今天天气怎么样？"})
print(result["output"])
```


### 2. 批量处理
同时处理多个请求，提高效率：
```python
# 批量生成响应
prompts = [
    "解释人工智能",
    "什么是机器学习", 
    "深度学习的应用"
]

# 批量调用模型
responses = llm.batch([
    HumanMessage(content=p) for p in prompts
])

# 输出结果
for response in responses:
    print(response.content)
    print("-" * 50)
```


### 3. 流式输出
实时返回生成结果（适合交互场景）：
```python
from langchain.callbacks.streaming_stdout import StreamingStdOutCallbackHandler

# 配置流式输出
streaming_llm = ChatOpenAI(
    model="gpt-3.5-turbo",
    streaming=True,  # 开启流式
    callbacks=[StreamingStdOutCallbackHandler()]  # 回调函数（实时打印）
)

# 流式生成（结果会逐字输出）
streaming_llm.invoke("写一首关于春天的诗")
```


### 4. 错误处理与成本监控
监控 Token 消耗和调用成本，捕获异常：
```python
from langchain.callbacks import get_openai_callback

try:
    # 监控Token和成本
    with get_openai_callback() as cb:
        response = llm.invoke("你好，介绍一下LangChain")
        print(f"使用的Token数: {cb.total_tokens}")
        print(f"API调用成本: ${cb.total_cost:.6f}")
except Exception as e:
    print(f"发生错误: {e}")  # 捕获调用异常（如网络错误、密钥无效）
```


## 六、最佳实践

### 1. 模型选择策略
根据任务类型选择合适模型，平衡效果与成本：

| 任务类型       | 推荐模型               | 原因                     |
|----------------|------------------------|--------------------------|
| 简单对话       | gpt-3.5-turbo          | 响应快、成本低           |
| 复杂推理       | gpt-4                  | 准确度高、逻辑能力强     |
| 长文本处理     | claude-3-opus          | 支持更长上下文（可达200k Token） |
| 创意写作       | claude-3-sonnet        | 生成内容流畅、创造力强   |


### 2. 成本优化
根据任务复杂度动态切换模型：
```python
class CostOptimizedLLM:
    def __init__(self):
        # 定义低成本和高性能模型
        self.cheap_model = ChatOpenAI(model="gpt-3.5-turbo")
        self.premium_model = ChatOpenAI(model="gpt-4")
    
    def smart_invoke(self, message, complexity="low"):
        """根据任务复杂度选择模型"""
        model = self.premium_model if complexity == "high" else self.cheap_model
        return model.invoke(message)

# 使用示例
optimizer = CostOptimizedLLM()
# 简单任务用低成本模型
optimizer.smart_invoke("总结这段文字", complexity="low")
# 复杂任务用高性能模型
optimizer.smart_invoke("分析财务报表数据", complexity="high")
```


### 3. 缓存策略
缓存重复请求，减少冗余调用：
```python
from langchain.cache import InMemoryCache
from langchain.globals import set_llm_cache

# 启用内存缓存（生产环境可改用Redis等持久化缓存）
set_llm_cache(InMemoryCache())

# 第一次调用：实际请求API
response1 = llm.invoke("什么是人工智能？")
# 第二次调用：相同问题，直接使用缓存
response2 = llm.invoke("什么是人工智能？")  # 无API请求，速度更快
```


### 4. 异步处理
通过异步调用提高并发能力：
```python
import asyncio
from langchain_openai import AsyncChatOpenAI

async def async_chat():
    # 初始化异步模型
    async_llm = AsyncChatOpenAI(
        model="gpt-3.5-turbo",
        api_key="您的Nebula lab密钥",
        base_url="https://llm.ai-nebula.com/v1"
    )
    
    # 异步调用
    response = await async_llm.ainvoke("异步生成的内容")
    return response.content

# 运行异步函数
result = asyncio.run(async_chat())
print(result)
```


## 七、复杂应用示例

### 1. 多模态 RAG 系统（支持图像）
结合文本与图像进行问答（需使用支持多模态的模型如 gpt-4o）：
```python
class MultiModalRAG:
    def __init__(self):
        self.llm = ChatOpenAI(model="gpt-4o")  # gpt-4o支持图像理解
        self.embeddings = OpenAIEmbeddings()
        self.vectorstore = None
        self.text_splitter = CharacterTextSplitter(chunk_size=1000, chunk_overlap=0)
    
    def add_documents(self, documents):
        """添加文档到知识库"""
        texts = self.text_splitter.split_documents(documents)
        self.vectorstore = FAISS.from_documents(texts, self.embeddings)
    
    def query_with_image(self, question, image_url):
        """结合图像和文档回答问题"""
        # 检索相关文档
        context = self.vectorstore.similarity_search(question, k=3) if self.vectorstore else []
        
        # 构建包含图像的消息
        messages = [
            {"role": "system", "content": "基于提供的文档和图像回答问题"},
            {"role": "user", "content": [
                {"type": "text", "text": f"问题: {question}\n\n上下文: {context}"},
                {"type": "image_url", "image_url": {"url": image_url}}  # 图像URL
            ]}
        ]
        
        return self.llm.invoke(messages)
```


### 2. 智能工作流（意图分类 + 路由）
根据用户意图自动路由到对应模型处理：
```python
from langchain.schema.runnable import RunnableLambda, RunnableSequence

def classify_intent(query):
    """将用户查询分类（问答/创作/分析/其他）"""
    classifier = ChatOpenAI(model="gpt-3.5-turbo")
    result = classifier.invoke(f"将以下查询分类为：问答、创作、分析、其他\n\n{query}")
    return result.content.strip()

def route_to_specialist(intent_and_query):
    """根据意图路由到对应模型"""
    intent, query = intent_and_query
    
    # 不同意图使用不同模型
    if "问答" in intent:
        model = ChatOpenAI(model="gpt-3.5-turbo")  # 高效处理问答
    elif "创作" in intent:
        model = ChatOpenAI(model="claude-3-sonnet")  # 擅长创作
    elif "分析" in intent:
        model = ChatOpenAI(model="gpt-4")  # 适合复杂分析
    else:
        model = ChatOpenAI(model="gpt-3.5-turbo")
    
    return model.invoke(query)

# 创建工作流链
workflow = RunnableSequence(
    RunnableLambda(lambda x: (classify_intent(x), x)),  # 先分类，再传递意图和查询
    RunnableLambda(route_to_specialist)  # 路由到对应模型
)

# 使用工作流
result = workflow.invoke("帮我分析这个季度的销售数据")  # 会自动用gpt-4处理
print(result.content)
```


### 3. 性能监控
监控模型调用耗时和成功率：
```python
import time
from functools import wraps

def monitor_llm_calls(func):
    """装饰器：监控LLM调用性能"""
    @wraps(func)
    def wrapper(*args, **kwargs):
        start_time = time.time()
        try:
            result = func(*args, **kwargs)
            success = True
        except Exception as e:
            result = None
            success = False
            print(f"LLM调用失败: {e}")
        
        end_time = time.time()
        duration = end_time - start_time
        print(f"LLM调用 - 成功: {success}, 耗时: {duration:.2f}s")
        return result
    return wrapper

# 使用装饰器监控调用
@monitor_llm_calls
def safe_llm_call(llm, message):
    return llm.invoke(message)

# 测试
safe_llm_call(llm, "测试性能监控")
```


## 八、部署建议

### 1. 生产环境配置
通过环境变量管理配置，提高灵活性：
```python
import os
from langchain_openai import ChatOpenAI

class ProductionLLM:
    def __init__(self):
        self.llm = ChatOpenAI(
            model=os.getenv("LLM_MODEL", "gpt-3.5-turbo"),  # 模型名（默认gpt-3.5-turbo）
            temperature=float(os.getenv("LLM_TEMPERATURE", "0.7")),  # 随机性
            max_tokens=int(os.getenv("LLM_MAX_TOKENS", "1000")),  # 最大生成Token
            request_timeout=int(os.getenv("LLM_REQUEST_TIMEOUT", "60"))  # 超时时间（秒）
        )
    
    def chat(self, message):
        try:
            return self.llm.invoke(message)
        except Exception as e:
            # 生产环境建议用日志系统记录错误（如logging）
            print(f"LLM错误: {e}")
            return "抱歉，服务暂时不可用"
```


### 2. 容错机制（重试逻辑）
通过重试处理临时错误（如网络波动）：
```python
import time
from functools import wraps

def retry_llm_call(max_retries=3, delay=1):
    """装饰器：失败自动重试（指数退避）"""
    def decorator(func):
        @wraps(func)
        def wrapper(*args, **kwargs):
            for attempt in range(max_retries):
                try:
                    return func(*args, **kwargs)
                except Exception as e:
                    if attempt == max_retries - 1:
                        raise e  # 最后一次失败则抛出异常
                    # 指数退避（重试间隔：1s, 2s, 4s...）
                    time.sleep(delay * (2 **attempt))
            return None
        return wrapper
    return decorator

# 使用重试装饰器
@retry_llm_call(max_retries=3)
def robust_llm_call(llm, message):
    return llm.invoke(message)

# 测试
robust_llm_call(llm, "测试容错机制")
```
