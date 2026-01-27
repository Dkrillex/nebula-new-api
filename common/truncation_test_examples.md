# TruncateJsonValues 测试场景示例

## 场景1: 普通长字符串截断
**输入:**
```json
{
    "id": "cgt-20260107001604-kjfxq",
    "data": ["wowowowowoowowowowowowoowowowowowowowowwowowowowowowowowowoowowowowowowoowowowowowowowowowowo", "wowowowowoowowowowowowoowowowowowowowowwowowowowowowowowowoowowowowowowoowowowowowowowowowowo"]
}
```

**输出:**
```json
{
    "id": "cgt-20260107001604-kjfxq",
    "data": ["wowowowowoowowowowowowoowowowowowowowowwowowowowowowowowowoowowowowowowoowowowowowowo...", "wowowowowoowowowowowowoowowowowowowowowwowowowowowowowowowoowowowowowowoowowowowowowowowowowow..."]
}
```

## 场景2: URL不被截断
**输入:**
```json
{
    "video_url": "https://ark-content-generation-cn-beijing.tos-cn-beijing.volces.com/doubao-seedance-1-0-lite-t2v/02176771616479800000000000000000000ffffac19188da0c05f.mp4?X-Tos-Algorithm=TOS4-HMAC-SHA256&X-Tos-Credential=AKLTYWJkZTExNjA1ZDUyNDc3YzhjNTM5OGIyNjBhNDcyOTQ%2F20260106%2Fcn-beijing%2Ftos%2Frequest&X-Tos-Date=20260106T161621Z&X-Tos-Expires=86400&X-Tos-Signature=2578581ea0f340df6a3168ca0b7a1708d33afc6cb2fbe80f86a22b9bec99fa2f&X-Tos-SignedHeaders=host",
    "short_text": "normal text"
}
```

**输出:**
```json
{
    "video_url": "https://ark-content-generation-cn-beijing.tos-cn-beijing.volces.com/doubao-seedance-1-0-lite-t2v/02176771616479800000000000000000000ffffac19188da0c05f.mp4?X-Tos-Algorithm=TOS4-HMAC-SHA256&X-Tos-Credential=AKLTYWJkZTExNjA1ZDUyNDc3YzhjNTM5OGIyNjBhNDcyOTQ%2F20260106%2Fcn-beijing%2Ftos%2Frequest&X-Tos-Date=20260106T161621Z&X-Tos-Expires=86400&X-Tos-Signature=2578581ea0f340df6a3168ca0b7a1708d33afc6cb2fbe80f86a22b9bec99fa2f&X-Tos-SignedHeaders=host",
    "short_text": "normal text"
}
```

## 场景3: 嵌套对象中的长字符串
**输入:**
```json
{
    "content": {
        "video_url": "https://example.com/video.mp4",
        "description": "这是一个非常长的描述文本，超过了100个字符的限制，所以应该被截断。这个描述包含了很多详细信息，用于测试截断功能是否正常工作。"
    },
    "status": "succeeded"
}
```

**输出:**
```json
{
    "content": {
        "video_url": "https://example.com/video.mp4",
        "description": "这是一个非常长的描述文本，超过了100个字符的限制，所以应该被截断。这个描述包含了很多详细信息，用于测试截断功能是否正..."
    },
    "status": "succeeded"
}
```

## 场景4: 混合场景（URL + 长字符串）
**输入:**
```json
{
    "id": "cgt-20260107001604-kjfxq",
    "seed": 48239,
    "model": "doubao-seedance-1-0-lite-t2v-250428",
    "ratio": "16:9",
    "usage": {
        "total_tokens": 103818,
        "completion_tokens": 103818
    },
    "data": ["wowowowowoowowowowowowoowowowowowowowowwowowowowowowowowowoowowowowowowoowowowowowowowowowowo", "wowowowowoowowowowowowoowowowowowowowowwowowowowowowowowowoowowowowowowoowowowowowowowowowowo"],
    "status": "succeeded",
    "content": {
        "video_url": "https://ark-content-generation-cn-beijing.tos-cn-beijing.volces.com/doubao-seedance-1-0-lite-t2v/02176771616479800000000000000000000ffffac19188da0c05f.mp4?X-Tos-Algorithm=TOS4-HMAC-SHA256&X-Tos-Credential=AKLTYWJkZTExNjA1ZDUyNDc3YzhjNTM5OGIyNjBhNDcyOTQ%2F20260106%2Fcn-beijing%2Ftos%2Frequest&X-Tos-Date=20260106T161621Z&X-Tos-Expires=86400&X-Tos-Signature=2578581ea0f340df6a3168ca0b7a1708d33afc6cb2fbe80f86a22b9bec99fa2f&X-Tos-SignedHeaders=host"
    },
    "duration": 5
}
```

**输出:**
```json
{
    "id": "cgt-20260107001604-kjfxq",
    "seed": 48239,
    "model": "doubao-seedance-1-0-lite-t2v-250428",
    "ratio": "16:9",
    "usage": {
        "total_tokens": 103818,
        "completion_tokens": 103818
    },
    "data": ["wowowowowoowowowowowowoowowowowowowowowwowowowowowowowowowoowowowowowowoowowowowowowo...", "wowowowowoowowowowowowoowowowowowowowowwowowowowowowowowowoowowowowowowoowowowowowowowowowowow..."],
    "status": "succeeded",
    "content": {
        "video_url": "https://ark-content-generation-cn-beijing.tos-cn-beijing.volces.com/doubao-seedance-1-0-lite-t2v/02176771616479800000000000000000000ffffac19188da0c05f.mp4?X-Tos-Algorithm=TOS4-HMAC-SHA256&X-Tos-Credential=AKLTYWJkZTExNjA1ZDUyNDc3YzhjNTM5OGIyNjBhNDcyOTQ%2F20260106%2Fcn-beijing%2Ftos%2Frequest&X-Tos-Date=20260106T161621Z&X-Tos-Expires=86400&X-Tos-Signature=2578581ea0f340df6a3168ca0b7a1708d33afc6cb2fbe80f86a22b9bec99fa2f&X-Tos-SignedHeaders=host"
    },
    "duration": 5
}
```

## 场景5: http:// 开头的URL（非https）
**输入:**
```json
{
    "image_url": "http://example.com/very/long/path/to/image/file.jpg?param1=value1&param2=value2&param3=value3&param4=value4&param5=value5&param6=value6&param7=value7&param8=value8",
    "long_description": "这是一个超过100个字符的长描述文本，应该被截断。这个文本包含了很多内容，用于测试JSON截断功能是否能够正确处理各种情况。"
}
```

**输出:**
```json
{
    "image_url": "http://example.com/very/long/path/to/image/file.jpg?param1=value1&param2=value2&param3=value3&param4=value4&param5=value5&param6=value6&param7=value7&param8=value8",
    "long_description": "这是一个超过100个字符的长描述文本，应该被截断。这个文本包含了很多内容，用于测试JSON截断功能是否能够正确处理各种情况..."
}
```

## 场景6: 数组中的混合类型
**输入:**
```json
{
    "messages": [
        "short message",
        "这是一个非常长的消息内容，超过了100个字符的限制，所以应该被截断。这个消息包含了很多详细信息，用于测试截断功能。",
        "https://example.com/api/v1/endpoint?token=abc123&user=test&action=create&type=video&format=mp4&quality=high&duration=60&size=1920x1080"
    ]
}
```

**输出:**
```json
{
    "messages": [
        "short message",
        "这是一个非常长的消息内容，超过了100个字符的限制，所以应该被截断。这个消息包含了很多详细信息，用于测试截断功能...",
        "https://example.com/api/v1/endpoint?token=abc123&user=test&action=create&type=video&format=mp4&quality=high&duration=60&size=1920x1080"
    ]
}
```

## 场景7: 边界情况 - 正好100个字符
**输入:**
```json
{
    "exactly_100": "1234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890",
    "exactly_101": "12345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901"
}
```

**输出:**
```json
{
    "exactly_100": "1234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890",
    "exactly_101": "1234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890..."
}
```

## 场景8: 非字符串类型不受影响
**输入:**
```json
{
    "number": 12345678901234567890,
    "boolean": true,
    "null_value": null,
    "nested": {
        "array": [1, 2, 3, 4, 5],
        "object": {
            "key": "value"
        }
    }
}
```

**输出:**
```json
{
    "number": 12345678901234567890,
    "boolean": true,
    "null_value": null,
    "nested": {
        "array": [1, 2, 3, 4, 5],
        "object": {
            "key": "value"
        }
    }
}
```
