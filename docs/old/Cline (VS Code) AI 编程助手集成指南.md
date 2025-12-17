# Cline (VS Code) 使用指南


## 一、简介
Cline（原 Claude Dev）是一款强大的 VS Code AI 编程助手插件，支持多种 AI 模型。通过集成 **Nebula lab**，可灵活切换主流 AI 模型辅助编程，覆盖代码生成、解释、重构、测试等全流程开发场景。


## 二、快速安装与配置

### 1. 安装插件
在 VS Code 扩展商店搜索 **“Cline”**，点击“安装”并启用。


### 2. 配置 Nebula API
1. 点击 VS Code 左侧的 **Cline 图标**，打开插件面板。  
2. 点击面板中的设置按钮（齿轮图标），选择 **“OpenAI Compatible”**。  
3. 填写配置参数：  
   - **Base URL**：`https://llm.ai-nebula.com/v1`（固定地址）  
   - **API Key**：输入您的 Nebula lab 密钥（从 Nebula lab控制台获取）  
   - **Model Name**：输入目标模型名称（参考下方“推荐模型”）  


## 三、推荐模型

### 🎯 编程开发首选
| 模型 ID                     | 特点                                  |  
|-----------------------------|---------------------------------------|  
| `claude-sonnet-4-20250514`  | Claude 4 最新版本，编程能力极强        |  
| `o4-mini`                   | OpenAI 推理模型，编程任务首选          |  
| `gemini-2.5-pro`            | 支持 2M 上下文，多模态能力强           |  
| `deepseek-coder`            | 代码专精模型，128K 上下文              |  


### 💡 复杂推理任务
| 模型 ID                                   | 特点                                  |  
|-------------------------------------------|---------------------------------------|  
| `o3`                                      | 最新推理模型，已大幅降价              |  
| `claude-sonnet-4-20250514-thinking`       | Claude 4 思维链模式，推理过程透明     |  
| `deepseek-r1`                             | 最新推理模型，64K 上下文              |  
| `grok-3-mini`                             | 带推理能力的轻量模型                  |  


### 🚀 快速响应
| 模型 ID                     | 特点                                  |  
|-----------------------------|---------------------------------------|  
| `gpt-3.5-turbo`             | 经典模型，性价比高                    |  
| `gemini-2.5-flash`          | 速度快，成本低，1M 上下文             |  
| `claude-3-haiku`            | 轻量快速版本，200K 上下文             |  
| `gpt-4o-mini`               | 轻量快速版本，128K 上下文             |  


### 💰 超高性价比
| 模型 ID                     | 特点                                  |  
|-----------------------------|---------------------------------------|  
| `deepseek-chat`             | 128K 上下文，综合能力强               |  
| `qwen-turbo`                | 阿里通义千问快速版                    |  
| `gpt-4.1-nano`              | 超低成本版本，128K 上下文             |  


## 四、核心功能

### 1. 智能代码补全
输入注释或代码片段，AI 自动补全实现：  
```python
# 输入注释，AI 自动生成代码
# 计算斐波那契数列的第 n 项
def fibonacci(n):
    # Cline 会自动补全递归或迭代实现
    if n <= 0:
        return 0
    elif n == 1:
        return 1
    else:
        return fibonacci(n-1) + fibonacci(n-2)
```


### 2. 代码解释
选中代码片段后，使用以下命令：  
- `Cline: Explain Code`：解释代码逻辑、用途及关键步骤。  
- `Cline: Explain Error`：分析报错信息，定位错误原因并提供修复建议。  


### 3. 代码重构
支持多种重构场景：  
- **性能优化**：识别冗余逻辑、低效算法，提供优化方案。  
- **可读性改进**：重命名变量、拆分复杂函数、添加注释。  
- **问题修复**：自动修复语法错误、逻辑漏洞（如空指针、数组越界）。  


### 4. 生成测试
根据函数自动生成单元测试：  
```javascript
// 原始函数
function add(a, b) {
    return a + b;
}

// Cline 生成的测试（Jest 框架）
describe('add function', () => {
    test('should add two numbers correctly', () => {
        expect(add(2, 3)).toBe(5);
        expect(add(-1, 1)).toBe(0);
        expect(add(0, 0)).toBe(0);
    });
});
```


## 五、常用命令与快捷键

| 命令                  | 快捷键          | 功能描述                  |  
|-----------------------|-----------------|---------------------------|  
| `Cline: Ask`          | `Ctrl+Shift+L`  | 向 AI 提问（如代码问题）  |  
| `Cline: Explain`      | `Ctrl+Shift+E`  | 解释选中的代码            |  
| `Cline: Refactor`     | `Ctrl+Shift+R`  | 重构选中的代码            |  
| `Cline: Generate`     | `Ctrl+Shift+G`  | 根据需求生成代码          |  


## 六、高级功能

### 1. 多文件操作
支持跨文件理解与修改，例如：  
```
"将 utils.js 中的 formatDate 函数移动到 helpers.js，并更新所有引用该函数的文件"
```


### 2. 架构设计
根据需求生成项目架构方案：  
```
请为电商后台管理系统设计架构，包含：
- 用户管理（登录、权限）
- 商品管理（CRUD、分类）
- 订单处理（创建、支付、物流）
- 数据分析（销量统计、用户画像）
技术栈：React + Node.js + PostgreSQL
```


### 3. 代码审查
自动化 PR 审查，覆盖多维度：  
```
Review PR:
- 检查是否符合 ESLint 规范
- 发现潜在 bug（如未处理的异常）
- 性能优化建议（如重复计算优化）
- 安全漏洞扫描（如 SQL 注入风险）
```


## 七、使用技巧

### 1. 上下文管理
提供详细上下文，让 AI 生成更精准的代码：  
```javascript
// @context: React 组件用于用户认证
// @requirements: 支持 OAuth2 登录流程（GitHub、Google）
// @constraints: 兼容 NextJS 13+ 服务器组件
// @style: 使用 Tailwind CSS 实现按钮样式

// AI 基于以上信息生成适配的组件代码
```


### 2. 自定义提示词
在 VS Code 设置中配置常用提示模板：  
```json
{
    "cline.customPrompts": {
        "codeReview": "审查代码，重点关注性能、安全和团队代码规范",
        "optimize": "优化代码性能（减少冗余计算）和可读性（添加 JSDoc 注释）",
        "document": "生成详细的中文文档，包含功能说明、参数解释和使用示例"
    }
}
```


### 3. 项目级配置
在项目根目录创建 `.cline/config.json`，统一代码风格：  
```json
{
    "model": "claude-3-5-sonnet-20241022",  // 默认模型
    "temperature": 0.7,  // 生成随机性（0-1）
    "language": "zh-CN",  // 优先中文交互
    "codeStyle": {
        "naming": "camelCase",  // 变量命名风格
        "indent": 4,  // 缩进空格数
        "quotes": "single"  // 字符串引号类型
    }
}
```


## 八、故障排除

### 连接失败
- 检查配置：`Base URL` 必须为 `https://llm.ai-nebula.com/v1`，API 密钥需有效。  
- 验证网络：关闭代理或 VPN 重试，确保能访问 API 地址。  


### 响应超时
- 优化方案：使用更快的模型（如 `gemini-2.5-flash`）、拆分大任务（如“先设计函数接口，再实现逻辑”）。  


### 生成质量差
- 改进方法：补充更多上下文（如技术栈、约束条件）、切换更强大的模型（如 `claude-sonnet-4-20250514`）、明确需求细节（避免模糊描述）。  


## 九、最佳实践

### 1. 模型选择策略
| 任务类型       | 推荐模型                     | 成本级别 | 原因                                  |  
|----------------|------------------------------|----------|---------------------------------------|  
| 简单补全       | `gpt-3.5-turbo` / `gemini-2.5-flash` | 极低     | 速度快，成本低，满足基础需求          |  
| 代码生成       | `claude-sonnet-4-20250514` / `o4-mini` | 中等     | 编程能力最强，逻辑严谨                |  
| 复杂推理       | `o3` / `deepseek-r1`         | 中低     | 推理能力强，`o3` 性价比已优化         |  
| 架构设计       | `deepseek-r1` + `claude-4-sonnet` | 低       | 先规划后实现，平衡效率与成本          |  
| 代码审查       | `gemini-2.5-pro` / `claude-4-sonnet` | 中等     | 长上下文支持，能理解多文件关联        |  
| 批量重构       | `deepseek-chat` / `qwen-turbo` | 极低     | 快速处理重复任务，成本可控            |  
| 文档生成       | `gpt-4.1` / `qwen-max`       | 中低     | 表达自然流畅，适配中文场景            |  
| 长文本处理     | `gemini-2.5-pro`（2M 上下文） | 中等     | 支持超长代码库或文档的理解            |  


### 2. 提示词优化
- ❌ 不佳示例：`“修复这个函数”`  
- ✅ 优质示例：`“修复 calculateTotal 函数中的浮点数精度问题，确保金额计算准确到小数点后两位（如 1.995 应四舍五入为 2.00）”`  


### 3. 渐进式开发
1. 先生成基础框架（如函数接口、组件结构）。  
2. 逐步添加核心功能（分步骤实现，避免一次性生成复杂代码）。  
3. 最后优化性能（如缓存、异步处理）和错误处理（如边界条件判断）。  


### 4. 安全意识
- 避免在提示中包含敏感信息（如 API 密钥、数据库密码）。  
- 审查 AI 生成的代码（尤其是涉及权限、加密的部分）。  
- 验证第三方依赖安全性（如通过 `npm audit` 检查漏洞）。  


## 十、集成工作流

### 1. Git 集成
```bash
# 自动生成 commit message
git add .
# 执行 Cline 命令：根据代码改动生成规范的提交信息（如 "feat: add user login component"）
```


### 2. 测试驱动开发（TDD）
1. 手动编写测试用例（如 Jest、PyTest）。  
2. 用 Cline 生成符合测试要求的实现代码。  
3. 运行测试验证，通过 Cline 修复不通过的用例。  


### 3. CI/CD 集成
生成自动化配置文件（如 GitHub Actions）：  
```yaml
# Cline 生成的 GitHub Actions 配置
name: CI
on: [push, pull_request]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v2
      - name: Set up Node.js
        uses: actions/setup-node@v3
        with:
          node-version: '18'
      - run: npm install
      - run: npm test
```


## 十一、Token 优化与成本控制

### 1. 监控与管理
- 实时监控：在 Cline 侧边栏查看当前会话的 Token 估算成本。  
- 会话管理：保持会话简短（单个任务一个会话），避免上下文无意义累积。  


### 2. 智能模型切换策略
```
规划阶段 → DeepSeek-R1 或 o3（推理能力强，成本低）  
实现阶段 → Claude-4-Sonnet 或 o4-mini（编程能力最强）  
简单任务 → GPT-3.5-Turbo 或 Gemini-2.5-Flash（速度快）  
长文本 → Gemini-2.5-Pro（2M 上下文）  
```


### 3. 配置限制规则
在项目根目录创建 `.clinerules` 文件，约束操作范围：  
```
# 限制 Cline 操作范围
- 不允许修改 node_modules 目录
- 不允许删除 .gitignore 中标记的重要文件
- 限制单次修改文件数量 ≤ 5 个
- 执行破坏性操作（如删除代码块）前必须提示确认
```


### 4. 低成本替代方案
- **GitHub Copilot Pro**：月费 $10，无限使用，适合高频场景。  
- **本地模型（Ollama）**：  
  ```bash
  # 安装 Ollama 并运行本地代码模型
  ollama run deepseek-coder:6.7b  # 代码专精
  ollama run qwen2.5-coder:7b    # 轻量高效
  ```  
- **国产模型**：`qwen-turbo`（阿里）、`glm-4`（智谱）、`ernie-4.0`（百度），成本低且适配中文场景。  


## 十二、最佳实践进阶

### 1. Plan/Act 模式
- **Plan 模式**：用于设计和审查（只读不改），适合用推理模型（如 `o3`）。  
- **Act 模式**：直接实现代码（可修改文件），适合用编程模型（如 `claude-sonnet-4`）。  
- 模型记忆：Cline 会记住两种模式的偏好，自动适配场景。  


### 2. 上下文管理技巧
```javascript
// 明确的上下文注释示例
// @context: React 18 + TypeScript 5，使用函数组件
// @requirements: 支持 SSR（Next.js 14），实现用户信息展示
// @constraints: 不依赖外部状态管理库（如 Redux），使用 React Context
// @performance: 首屏加载时间 < 3s，避免不必要的重渲染
```


### 3. 避免 Token 爆炸
- ❌ 错误做法：同一会话处理多个无关任务、让 AI 读取整个大型代码库、频繁修改需求导致上下文混乱。  
- ✅ 正确做法：每个功能开新会话、只包含相关文件、明确需求后再开始、完成后立即提交代码。  


### 4. 工作流优化
```
# 1. 规划阶段（推理模型）
"用 o3 或 DeepSeek-R1 分析需求，输出架构方案和接口设计"

# 2. 实现阶段（编程模型）
"切换到 Claude-4-Sonnet 或 o4-mini，按模块生成核心代码"

# 3. 测试优化（快速模型）
"用 Gemini-2.5-Flash 生成单元测试，修复报错"

# 4. 文档阶段（综合模型）
"用 GPT-4.1 生成 API 文档和使用说明"

# 5. 代码审查（长上下文模型）
"用 Gemini-2.5-Pro 审查全项目代码，优化细节"
```
