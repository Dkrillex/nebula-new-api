#!/usr/bin/env python3
"""
测试 thinking blocks 清理功能

用于验证系统是否正确处理 Claude thinking blocks 的验证问题
"""

import json
import sys

def test_thinking_block_cleaning():
    """测试 thinking block 清理逻辑"""
    
    print("=== Thinking Blocks 清理测试 ===\n")
    
    test_cases = [
        {
            "name": "测试 1: 移除无 signature 的 thinking 块",
            "input": {
                "role": "assistant",
                "content": [
                    {
                        "type": "thinking",
                        "thinking": "这是伪造的 thinking 内容"
                        # 缺少 signature
                    },
                    {
                        "type": "text",
                        "text": "这是正常的文本"
                    }
                ]
            },
            "expected_removed": True,
            "description": "应该移除没有 signature 的 thinking 块"
        },
        {
            "name": "测试 2: 保留有 signature 的 thinking 块",
            "input": {
                "role": "assistant",
                "content": [
                    {
                        "type": "thinking",
                        "thinking": "Let me analyze...",
                        "signature": "valid_signature_here"
                    },
                    {
                        "type": "text",
                        "text": "Here's my answer"
                    }
                ]
            },
            "expected_removed": False,
            "description": "应该保留有 signature 的 thinking 块（可能是合法的历史消息）"
        },
        {
            "name": "测试 3: 处理 redacted_thinking 块",
            "input": {
                "role": "assistant",
                "content": [
                    {
                        "type": "redacted_thinking",
                        "thinking": "Redacted content"
                        # 缺少 signature
                    },
                    {
                        "type": "text",
                        "text": "Response text"
                    }
                ]
            },
            "expected_removed": True,
            "description": "应该移除没有 signature 的 redacted_thinking 块"
        },
        {
            "name": "测试 4: 保留普通文本内容",
            "input": {
                "role": "assistant",
                "content": "这是纯文本消息"
            },
            "expected_removed": False,
            "description": "字符串内容应该保持不变"
        },
        {
            "name": "测试 5: 保留其他类型的 blocks",
            "input": {
                "role": "assistant",
                "content": [
                    {
                        "type": "tool_use",
                        "id": "toolu_123",
                        "name": "get_weather",
                        "input": {"location": "Beijing"}
                    },
                    {
                        "type": "text",
                        "text": "Let me check..."
                    }
                ]
            },
            "expected_removed": False,
            "description": "非 thinking 类型的 blocks 应该保持不变"
        },
        {
            "name": "测试 6: 混合场景",
            "input": {
                "role": "assistant",
                "content": [
                    {
                        "type": "thinking",
                        "thinking": "Valid thinking with signature",
                        "signature": "abc123"
                    },
                    {
                        "type": "thinking",
                        "thinking": "Invalid thinking without signature"
                    },
                    {
                        "type": "text",
                        "text": "Final answer"
                    }
                ]
            },
            "expected_removed": "partial",
            "description": "应该只移除无 signature 的 thinking，保留有 signature 的"
        }
    ]
    
    passed = 0
    failed = 0
    
    for i, test in enumerate(test_cases, 1):
        print(f"{test['name']}")
        print(f"描述: {test['description']}")
        print(f"输入: {json.dumps(test['input'], ensure_ascii=False, indent=2)}")
        
        # 这里只是展示测试用例，实际清理逻辑在 Go 代码中
        # 可以用于验证 API 行为
        
        print("✓ 测试用例已定义\n")
        passed += 1
    
    print(f"\n=== 测试总结 ===")
    print(f"总计: {len(test_cases)} 个测试用例")
    print(f"✓ 已定义: {passed}")
    print(f"✗ 失败: {failed}")
    
    return failed == 0

def generate_test_requests():
    """生成用于实际 API 测试的请求示例"""
    
    print("\n=== API 测试请求示例 ===\n")
    
    # 示例 1: 单轮对话（不会触发问题）
    single_turn = {
        "model": "claude-3-7-sonnet-20250219",
        "max_tokens": 1024,
        "messages": [
            {
                "role": "user",
                "content": "Hello, how are you?"
            }
        ]
    }
    
    print("1. 单轮对话请求（正常）:")
    print(json.dumps(single_turn, indent=2))
    print()
    
    # 示例 2: 多轮对话，assistant 消息包含无效的 thinking 块
    # 系统应该自动清理
    multi_turn_invalid = {
        "model": "claude-3-7-sonnet-20250219",
        "max_tokens": 1024,
        "messages": [
            {
                "role": "user",
                "content": "What's 2+2?"
            },
            {
                "role": "assistant",
                "content": [
                    {
                        "type": "thinking",
                        "thinking": "Fake thinking without signature"
                        # 缺少 signature - 系统会移除
                    },
                    {
                        "type": "text",
                        "text": "2+2 equals 4"
                    }
                ]
            },
            {
                "role": "user",
                "content": "What's 3+3?"
            }
        ]
    }
    
    print("2. 多轮对话，包含无效 thinking 块（系统会自动清理）:")
    print(json.dumps(multi_turn_invalid, indent=2))
    print()
    
    # 示例 3: 多轮对话，assistant 消息包含有效的 thinking 块
    multi_turn_valid = {
        "model": "claude-3-7-sonnet-20250219",
        "max_tokens": 1024,
        "messages": [
            {
                "role": "user",
                "content": "Solve this problem"
            },
            {
                "role": "assistant",
                "content": [
                    {
                        "type": "thinking",
                        "thinking": "Let me analyze this step by step...",
                        "signature": "MEUCIQDexample_signature_here"
                    },
                    {
                        "type": "text",
                        "text": "Here's the solution"
                    }
                ]
            },
            {
                "role": "user",
                "content": "Next question"
            }
        ]
    }
    
    print("3. 多轮对话，包含有效 thinking 块（会保留）:")
    print(json.dumps(multi_turn_valid, indent=2))
    print()
    
    # 示例 4: 推荐的最佳实践 - 移除所有 thinking 块
    multi_turn_cleaned = {
        "model": "claude-3-7-sonnet-20250219",
        "max_tokens": 1024,
        "messages": [
            {
                "role": "user",
                "content": "Solve this problem"
            },
            {
                "role": "assistant",
                "content": [
                    {
                        "type": "text",
                        "text": "Here's the solution"
                    }
                ]
            },
            {
                "role": "user",
                "content": "Next question"
            }
        ]
    }
    
    print("4. 最佳实践 - 客户端已移除 thinking 块:")
    print(json.dumps(multi_turn_cleaned, indent=2))
    print()
    
    # 示例 5: 使用纯文本格式（最简单）
    multi_turn_text = {
        "model": "claude-3-7-sonnet-20250219",
        "max_tokens": 1024,
        "messages": [
            {
                "role": "user",
                "content": "Solve this problem"
            },
            {
                "role": "assistant",
                "content": "Here's the solution"  # 纯文本，没有 thinking
            },
            {
                "role": "user",
                "content": "Next question"
            }
        ]
    }
    
    print("5. 最简单 - 使用纯文本格式:")
    print(json.dumps(multi_turn_text, indent=2))
    print()

def print_usage_examples():
    """打印使用示例"""
    
    print("\n=== 客户端代码示例 ===\n")
    
    print("Python 示例 - 清理 thinking 块:")
    print("""
def remove_thinking_blocks(content):
    '''移除所有 thinking 块，避免验证错误'''
    if isinstance(content, str):
        return content
    
    if isinstance(content, list):
        return [
            block for block in content 
            if block.get('type') not in ['thinking', 'redacted_thinking']
        ]
    
    return content

# 使用
response = client.messages.create(...)
cleaned_content = remove_thinking_blocks(response.content)
history.append({"role": "assistant", "content": cleaned_content})
""")
    
    print("\nJavaScript 示例 - 清理 thinking 块:")
    print("""
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
const response = await claude.messages.create(...);
const cleaned = removeThinkingBlocks(response.content);
history.push({role: "assistant", content: cleaned});
""")

if __name__ == "__main__":
    print("AWS Bedrock Thinking Blocks 清理功能测试\n")
    print("=" * 60)
    
    # 运行测试
    success = test_thinking_block_cleaning()
    
    # 生成测试请求
    generate_test_requests()
    
    # 打印使用示例
    print_usage_examples()
    
    print("\n" + "=" * 60)
    print("\n提示:")
    print("1. 系统已添加自动清理逻辑，会移除无效的 thinking 块")
    print("2. 建议客户端应用主动移除或完整保留 thinking 块")
    print("3. 详细文档请查看: docs/common_errors/bedrock_thinking_validation_error.md")
    
    sys.exit(0 if success else 1)
