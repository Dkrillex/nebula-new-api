# Gemini 思考模式使用指南

本文档介绍如何在 Nebula API 中开启和使用 Gemini 模型的思考模式（Thinking Mode）。

## 目录

- [概述](#概述)
- [开启思考模式的方式](#开启思考模式的方式)
  - [方式一：模型名称后缀](#方式一模型名称后缀)
  - [方式二：extra_body 配置](#方式二extra_body-配置)
  - [方式三：reasoning_effort 参数](#方式三reasoning_effort-参数)
- [thinking_budget 参数详解](#thinking_budget-参数详解)
- [reasoning_effort 参数详解](#reasoning_effort-参数详解)
- [模型支持情况](#模型支持情况)
- [使用示例](#使用示例)

## 概述

Gemini 思考模式允许模型在生成响应之前进行内部推理，这些推理过程可以通过 `reasoning_content` 字段返回。思考模式适用于需要复杂推理的任务。

**重要提示：**
- 思考模式需要启用思考适配器（`gemini.thinking_adapter_enabled=true`）
- 思考模式需要流式输出（`stream: true`）才能看到思考内容
- 思考预算会占用输出 token 配额

## 开启思考模式的方式

### 方式一：模型名称后缀

#### 1. `-thinking` 后缀（自动预算）

在模型名称后添加 `-thinking` 后缀，系统会自动计算思考预算。

```json
{
    "model": "gemini-2.5-flash-thinking",
    "messages": [
        {
            "role": "user",
            "content": "你好"
        }
    ],
    "stream": true
}
```

**预算计算规则：**
- 如果设置了 `max_tokens`：预算 = `max_tokens * 思考预算占比`（默认 60%）
- 如果未设置 `max_tokens`：可配合 `reasoning_effort` 参数使用

#### 2. `-thinking-<数字>` 后缀（指定预算）

在模型名称后添加 `-thinking-<预算数字>` 后缀，精确指定思考预算。

```json
{
    "model": "gemini-2.5-flash-thinking-128",
    "messages": [
        {
            "role": "user",
            "content": "你好"
        }
    ],
    "stream": true
}
```

**示例：**
- `gemini-2.5-flash-thinking-128` - 思考预算 128 tokens
- `gemini-2.5-flash-thinking-8192` - 思考预算 8192 tokens
- `gemini-2.5-pro-thinking-16384` - 思考预算 16384 tokens

#### 3. `-nothinking` 后缀（禁用思考）

在模型名称后添加 `-nothinking` 后缀，显式禁用思考模式。

```json
{
    "model": "gemini-2.5-flash-nothinking",
    "messages": [
        {
            "role": "user",
            "content": "你好"
        }
    ]
}
```

### 方式二：extra_body 配置

通过 `extra_body` 中的 `google.thinking_config` 配置思考模式。

#### 基本格式

```json
{
    "model": "gemini-2.5-flash",
    "extra_body": {
        "google": {
            "thinking_config": {
                "thinking_budget": 2222,
                "include_thoughts": true
            }
        }
    },
    "messages": [
        {
            "role": "user",
            "content": "你好"
        }
    ],
    "stream": true
}
```

#### thinking_budget 参数值说明

| 值 | 说明 | 行为 |
|---|---|---|
| `-1` | 自动开启思考模式 | 不设置具体预算，让模型自动决定是否思考。设置 `IncludeThoughts: true`，不设置 `ThinkingBudget` |
| `0` | 显式禁用思考模式 | 设置 `ThinkingBudget: 0`，`IncludeThoughts: false` |
| `> 0` | 设置具体思考预算 | 设置具体的思考预算值，系统会自动限制在模型允许的范围内 |

**示例：**

```json
// 自动开启思考模式
{
    "model": "gemini-2.5-flash",
    "extra_body": {
        "google": {
            "thinking_config": {
                "thinking_budget": -1
            }
        }
    }
}

// 禁用思考模式
{
    "model": "gemini-2.5-flash",
    "extra_body": {
        "google": {
            "thinking_config": {
                "thinking_budget": 0
            }
        }
    }
}

// 设置具体预算
{
    "model": "gemini-2.5-flash",
    "extra_body": {
        "google": {
            "thinking_config": {
                "thinking_budget": 2222,
                "include_thoughts": true
            }
        }
    }
}
```

### 方式三：reasoning_effort 参数

当使用 `-thinking` 后缀且未设置 `max_tokens` 时，可以使用 `reasoning_effort` 参数自动计算思考预算。

```json
{
    "model": "gemini-2.5-flash-thinking",
    "reasoning_effort": "medium",
    "messages": [
        {
            "role": "user",
            "content": "你好"
        }
    ],
    "stream": true
}
```

#### reasoning_effort 参数值

| 值 | 说明 | 预算占比 |
|---|---|---|
| `"low"` | 低推理强度 | 约 20% 的 max_budget |
| `"medium"` | 中等推理强度 | 约 50% 的 max_budget |
| `"high"` | 高推理强度 | 约 80% 的 max_budget |

#### 各模型的 max_budget 和计算示例

**gemini-2.5-flash / gemini-2.5-flash-preview-09-2025:**
- max_budget: 24576 tokens
- `low`: 24576 × 20% = 4915 tokens
- `medium`: 24576 × 50% = 12288 tokens
- `high`: 24576 × 80% = 19661 tokens

**gemini-2.5-flash-lite-preview-09-2025:**
- max_budget: 24576 tokens
- `low`: 24576 × 20% = 4915 tokens
- `medium`: 24576 × 50% = 12288 tokens
- `high`: 24576 × 80% = 19661 tokens

**gemini-2.5-pro:**
- max_budget: 32768 tokens（估算）
- `low`: 32768 × 20% = 6554 tokens
- `medium`: 32768 × 50% = 16384 tokens
- `high`: 32768 × 80% = 26214 tokens

## Gemini 3 Pro Preview 思考模式与搜索

Gemini 3 Pro Preview 默认开启思考模式，使用 `thinking_level`（`LOW`/`HIGH`）而非数字预算，并可直接开启 Google Search。

### 思考级别（thinking_level）

- 可选值：`LOW`、`HIGH`
- 默认：`HIGH`
- 适用模型：`gemini-3-pro-preview`
- 配置优先级（高→低）：`extra_body.google.thinking_config.thinking_level` > 模型后缀 `-thinking-low/-thinking-high` > `reasoning_effort`（low/medium→LOW，high→HIGH） > 默认 HIGH

模型后缀示例：
- `gemini-3-pro-preview-thinking-low`
- `gemini-3-pro-preview-thinking-high`

### Google Search

开启方式（可并存，系统会合并去重）：
- OpenAI 兼容：`"tools": [{"type":"function","function":{"name":"googleSearch"}}]`（推荐）
- 原生透传：`extra_body.google.tools: [{"googleSearch": {}}]`

### 示例：仅思考，无搜索

```json
{
  "model": "gemini-3-pro-preview",
  "messages": [{"role": "user", "content": "今天广州的天气怎么样"}],
  "generationConfig": {
    "temperature": 1,
    "maxOutputTokens": 65535,
    "topP": 0.95,
    "thinkingConfig": {
      "thinkingLevel": "HIGH"
    }
  },
  "stream": true
}
```

### 示例：开启搜索（推荐 OpenAI 兼容写法）

```json
{
  "model": "gemini-3-pro-preview",
  "messages": [{"role": "user", "content": "Google 搜索 一下 今天广州的天气怎么样"}],
  "generationConfig": {
    "temperature": 1,
    "maxOutputTokens": 65535,
    "topP": 0.95,
    "thinkingConfig": {
      "thinkingLevel": "LOW"
    }
  },
  "tools": [
    {
      "type": "function",
      "function": { "name": "googleSearch" }
    }
  ],
  "stream": true
}
```

### 示例：开启搜索（原生透传写法）

```json
{
  "model": "gemini-3-pro-preview",
  "messages": [{"role": "user", "content": "Google 搜索 一下 今天广州的天气怎么样"}],
  "extra_body": {
    "google": {
      "thinking_config": { "thinking_level": "LOW" },
      "tools": [
        { "googleSearch": {} }
      ]
    }
  },
  "stream": true
}
```

### 与 Gemini 2.5 的差异

| 项目 | Gemini 2.5 系列 | Gemini 3 Pro Preview |
|---|---|---|
| 思考配置 | `thinking_budget`（数字） | `thinking_level`（LOW/HIGH） |
| 默认行为 | 需显式开启/后缀 | 默认开启思考 |
| 预算范围 | 见下表（1~24576/32768） | 按级别，无数字预算 |
| 搜索 | 支持 | 支持 |

## thinking_budget 参数详解

### 参数说明

`thinking_budget` 指定模型可以用于内部推理的最大 token 数量。思考预算会占用输出 token 配额。

### 特殊值处理

#### thinking_budget: -1（自动模式）

让模型自动决定是否进行思考，不限制思考预算。

**适用场景：**
- 希望模型根据任务复杂度自动决定是否思考
- 不确定应该设置多少预算时

**配置示例：**
```json
{
    "model": "gemini-2.5-flash",
    "extra_body": {
        "google": {
            "thinking_config": {
                "thinking_budget": -1,
                "include_thoughts": true
            }
        }
    }
}
```

#### thinking_budget: 0（禁用思考）

显式禁用思考模式。

**配置示例：**
```json
{
    "model": "gemini-2.5-flash",
    "extra_body": {
        "google": {
            "thinking_config": {
                "thinking_budget": 0
            }
        }
    }
}
```

#### thinking_budget: > 0（指定预算）

设置具体的思考预算值，系统会自动限制在模型允许的范围内。

**配置示例：**
```json
{
    "model": "gemini-2.5-flash",
    "extra_body": {
        "google": {
            "thinking_config": {
                "thinking_budget": 2222,
                "include_thoughts": true
            }
        }
    }
}
```

## reasoning_effort 参数详解

### 参数说明

`reasoning_effort` 用于指定推理强度，系统会根据模型的最大预算自动计算思考预算。

### 使用条件

- 模型名称必须包含 `-thinking` 后缀
- 未设置 `max_tokens` 参数
- 如果设置了 `max_tokens`，系统会优先使用 `max_tokens * 思考预算占比` 计算预算

### 计算逻辑

系统会根据 `reasoning_effort` 的值和模型的最大预算计算思考预算：

```go
switch effort {
case "high":
    budget = max_budget * 80 / 100
case "medium":
    budget = max_budget * 50 / 100
case "low":
    budget = max_budget * 20 / 100
}
```

## 模型支持情况

### 支持的模型

| 模型 | 温度范围 | 默认温度 | 思考预算范围 | 默认模式 | 手动默认值 |
|---|---|---|---|---|---|
| `gemini-2.5-flash` | 0 - 2 | 1 | 1 ~ 24576 | 自动 | 8192 |
| `gemini-2.5-flash-preview-09-2025` | 0 - 2 | 1 | 1 ~ 24576 | 自动 | 8192 |
| `gemini-2.5-flash-lite-preview-09-2025` | 0 - 2 | 1 | 1 ~ 24576 | 自动 | 8192 |
| `gemini-2.5-pro` | 0 - 2 | 1 | 1 ~ 32768 | 自动 | 16384 |

### 注意事项

1. **思考适配器必须启用**：需要在系统配置中设置 `gemini.thinking_adapter_enabled=true`
2. **流式输出**：思考内容需要通过流式输出（`stream: true`）才能看到
3. **预算限制**：系统会自动将思考预算限制在模型允许的范围内
4. **Token 计费**：思考预算会占用输出 token 配额

## 使用示例

### 示例 1：使用模型后缀开启思考模式

```json
{
    "model": "gemini-2.5-flash-thinking",
    "temperature": 1,
    "top_p": 1,
    "messages": [
        {
            "role": "user",
            "content": "请解释量子计算的原理"
        }
    ],
    "stream": true,
    "stream_options": {
        "include_usage": true
    }
}
```

### 示例 2：使用 extra_body 设置具体预算

```json
{
    "model": "gemini-2.5-flash",
    "temperature": 1,
    "top_p": 1,
    "extra_body": {
        "google": {
            "thinking_config": {
                "thinking_budget": 2222,
                "include_thoughts": true
            }
        }
    },
    "messages": [
        {
            "role": "user",
            "content": "请解释量子计算的原理"
        }
    ],
    "stream": true,
    "stream_options": {
        "include_usage": true
    }
}
```

### 示例 3：使用自动模式（-1）

```json
{
    "model": "gemini-2.5-flash",
    "temperature": 1,
    "top_p": 1,
    "extra_body": {
        "google": {
            "thinking_config": {
                "thinking_budget": -1
            }
        }
    },
    "messages": [
        {
            "role": "user",
            "content": "请解释量子计算的原理"
        }
    ],
    "stream": true
}
```

### 示例 4：使用 reasoning_effort

```json
{
    "model": "gemini-2.5-flash-thinking",
    "temperature": 1,
    "top_p": 1,
    "reasoning_effort": "medium",
    "messages": [
        {
            "role": "user",
            "content": "请解释量子计算的原理"
        }
    ],
    "stream": true,
    "stream_options": {
        "include_usage": true
    }
}
```

### 示例 5：禁用思考模式

```json
{
    "model": "gemini-2.5-flash",
    "temperature": 1,
    "top_p": 1,
    "extra_body": {
        "google": {
            "thinking_config": {
                "thinking_budget": 0
            }
        }
    },
    "messages": [
        {
            "role": "user",
            "content": "你好"
        }
    ],
    "stream": true
}
```

### 示例 6：使用精确预算后缀

```json
{
    "model": "gemini-2.5-flash-thinking-8192",
    "temperature": 1,
    "top_p": 1,
    "messages": [
        {
            "role": "user",
            "content": "请解释量子计算的原理"
        }
    ],
    "stream": true,
    "stream_options": {
        "include_usage": true
    }
}
```

## 响应格式

当开启思考模式并设置 `stream: true` 时，响应中会包含 `reasoning_content` 字段：

```json
{
    "id": "chatcmpl-xxx",
    "object": "chat.completion.chunk",
    "created": 1234567890,
    "model": "gemini-2.5-flash",
    "choices": [
        {
            "index": 0,
            "delta": {
                "role": "assistant",
                "reasoning_content": "让我思考一下这个问题...",
                "content": ""
            },
            "finish_reason": null
        }
    ]
}
```

## 常见问题

### Q1: 为什么看不到思考内容？

**A:** 确保：
1. 启用了思考适配器（`gemini.thinking_adapter_enabled=true`）
2. 设置了 `stream: true`
3. 思考预算 > 0 或设置为 -1

### Q2: thinking_budget 设置为 -1 和 0 有什么区别？

**A:**
- `-1`: 自动开启思考模式，让模型自己决定是否思考
- `0`: 显式禁用思考模式

### Q3: 如何选择合适的思考预算？

**A:**
- 简单任务：使用 `reasoning_effort: "low"` 或预算 1000-5000
- 中等任务：使用 `reasoning_effort: "medium"` 或预算 5000-15000
- 复杂任务：使用 `reasoning_effort: "high"` 或预算 15000-24576
- 不确定：使用 `thinking_budget: -1` 让模型自动决定

### Q4: 思考预算会影响计费吗？

**A:** 是的，思考预算会占用输出 token 配额，会计入总 token 使用量。

### Q5: 可以同时使用模型后缀和 extra_body 吗？

**A:** 如果同时使用，`extra_body` 中的配置会优先。建议只使用一种方式。

## 相关配置

### 系统配置

在系统设置中需要启用思考适配器：

```json
{
    "gemini.thinking_adapter_enabled": true,
    "gemini.thinking_adapter_budget_tokens_percentage": 0.6
}
```

- `thinking_adapter_enabled`: 是否启用思考适配器
- `thinking_adapter_budget_tokens_percentage`: 当使用 `-thinking` 后缀且设置了 `max_tokens` 时，思考预算占 `max_tokens` 的百分比（默认 60%）

## 更新日志

- 2025-12-15: 添加对 `thinking_budget: -1` 和 `thinking_budget: 0` 的支持
- 2025-12-15: 修复模型名称后缀匹配问题
- 2025-12-15: 添加 `reasoning_effort` 参数支持

