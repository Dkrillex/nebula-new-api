# Gemini Live API 集成修复总结

## 问题发现

通过分析官方示例代码（`generative-ai/gemini/multimodal-live-api/native-audio-websocket-demo-apps`），发现了以下关键问题：

### 1. **JSON 字段命名错误** ⚠️ 关键问题

**问题**：我们的 DTO 使用了 **camelCase**，但 Gemini Live API 要求 **snake_case**

**官方格式**：
```javascript
{
  setup: {
    model: "projects/.../models/...",
    generation_config: {          // snake_case ✓
      response_modalities: ["AUDIO"],
      speech_config: {
        voice_config: {
          prebuilt_voice_config: {
            voice_name: "Puck"
          }
        }
      }
    },
    system_instruction: { parts: [{ text: "..." }] },
    input_audio_transcription: {},
    output_audio_transcription: {}
  }
}
```

**我们之前的格式**（错误）：
```json
{
  "setup": {
    "model": "...",
    "generationConfig": {         // camelCase ✗
      "responseModalities": [...],
      "speechConfig": {...}
    }
  }
}
```

**修复**：更新了所有 DTO 的 JSON 标签为 snake_case

### 2. **Service Account 认证问题**

**问题**：WebSocket 连接时，`AccountCredentials` 未初始化，导致 JWT 创建失败

**原因**：
- 普通 HTTP 请求通过 `getRequestUrl` 初始化凭证
- WebSocket 连接直接在 `GetRequestURL` 返回 URL，跳过了初始化步骤

**修复**：
- 在 `SetupRequestHeader` 中添加凭证初始化逻辑
- 改进私钥解析，正确处理 PEM 格式和空白字符

### 3. **缺少必要的 DTO 字段**

**问题**：缺少官方 API 使用的字段

**新增字段**：
- `input_audio_transcription`：输入音频转录配置
- `output_audio_transcription`：输出音频转录配置
- `proactivity`：主动性配置
- `realtime_input_config`：实时输入配置
- `input_transcription`/`output_transcription`：服务器响应中的转录内容

## 修复内容

### 1. DTO 字段命名修复（`dto/gemini_live.go`）

所有 JSON 标签从 camelCase 改为 snake_case：

| 结构体 | 旧标签 | 新标签 |
|--------|--------|--------|
| GeminiLiveSetup | `generationConfig` | `generation_config` |
| GeminiLiveSetup | `systemInstruction` | `system_instruction` |
| GeminiLiveSetup | `speechConfig` | `speech_config` |
| GeminiLiveGenerationConfig | `responseModalities` | `response_modalities` |
| GeminiLiveGenerationConfig | `speechConfig` | `speech_config` |
| GeminiLiveGenerationConfig | `topP` | `top_p` |
| GeminiLiveGenerationConfig | `topK` | `top_k` |
| GeminiLiveGenerationConfig | `maxOutputTokens` | `max_output_tokens` |
| GeminiLiveSpeechConfig | `voiceConfig` | `voice_config` |
| GeminiLiveVoiceConfig | `prebuiltVoiceConfig` | `prebuilt_voice_config` |
| GeminiLivePrebuiltVoiceConfig | `voiceName` | `voice_name` |
| GeminiLiveClientContent | `turnComplete` | `turn_complete` |
| GeminiLiveRealtimeInput | `mediaChunks` | `media_chunks` |
| GeminiLiveMediaChunk | `mimeType` | `mime_type` |
| GeminiLivePartData | `inlineData` | `inline_data` |
| GeminiLivePartData | `functionCall` | `function_call` |
| GeminiLivePartData | `functionResponse` | `function_response` |
| GeminiLiveInlineData | `mimeType` | `mime_type` |
| GeminiLiveServerContent | `modelTurn` | `model_turn` |
| GeminiLiveServerContent | `turnComplete` | `turn_complete` |
| GeminiLiveTool | `functionDeclarations` | `function_declarations` |
| GeminiLiveTool | `googleSearch` | `google_search` |
| GeminiLiveToolCall | `functionCalls` | `function_calls` |
| GeminiLiveToolResponse | `functionResponses` | `function_responses` |

### 2. Vertex Adaptor 修复

#### `relay/channel/vertex/adaptor.go`

**`DoRequest` 方法**：
```go
func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	// Check if this is a Gemini Live API request (WebSocket)
	if gemini.IsGeminiLiveModel(info.UpstreamModelName) {
		return channel.DoWssRequest(a, c, info, requestBody)
	}
	return channel.DoApiRequest(a, c, info, requestBody)
}
```

**`DoResponse` 方法**：
```go
func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	// Handle Gemini Live API (WebSocket)
	if gemini.IsGeminiLiveModel(info.UpstreamModelName) && info.TargetWs != nil {
		newErr, realtimeUsage := gemini.GeminiLiveHandler(c, info)
		return realtimeUsage, newErr
	}
	// ... 其他处理
}
```

**`GetRequestURL` 方法**：
```go
// Check if this is a Gemini Live API request (WebSocket)
if gemini.IsGeminiLiveModel(info.UpstreamModelName) {
	baseURL := info.ChannelBaseUrl
	// Convert https:// to wss:// or http:// to ws://
	if strings.HasPrefix(baseURL, "https://") {
		baseURL = "wss://" + strings.TrimPrefix(baseURL, "https://")
	} else if strings.HasPrefix(baseURL, "http://") {
		baseURL = "ws://" + strings.TrimPrefix(baseURL, "http://")
	}
	
	// For Vertex AI, use the BidiGenerateContent endpoint
	return fmt.Sprintf("%s/ws/google.cloud.aiplatform.v1beta1.LlmBidiService/BidiGenerateContent", baseURL), nil
}
```

**`SetupRequestHeader` 方法**：
```go
func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)

	// Initialize AccountCredentials if using Service Account mode
	if info.ChannelOtherSettings.VertexKeyType != dto.VertexKeyTypeAPIKey && a.AccountCredentials.ClientEmail == "" {
		adc := &Credentials{}
		if err := common.Unmarshal([]byte(info.ApiKey), adc); err != nil {
			return fmt.Errorf("failed to decode credentials file: %w", err)
		}
		a.AccountCredentials = *adc
	}

	// Handle Gemini Live API (WebSocket) authentication
	if gemini.IsGeminiLiveModel(info.UpstreamModelName) {
		if info.ChannelOtherSettings.VertexKeyType == dto.VertexKeyTypeAPIKey {
			// For API Key mode with Gemini Live, use API Key directly
			req.Set("Authorization", "Bearer "+info.ApiKey)
		} else {
			// For Service Account mode, get OAuth2 token
			accessToken, err := getAccessToken(a, info)
			if err != nil {
				return err
			}
			req.Set("Authorization", "Bearer "+accessToken)
		}
	} else {
		// For non-Live API requests, use existing logic
		if info.ChannelOtherSettings.VertexKeyType != dto.VertexKeyTypeAPIKey {
			accessToken, err := getAccessToken(a, info)
			if err != nil {
				return err
			}
			req.Set("Authorization", "Bearer "+accessToken)
		}
	}

	if a.AccountCredentials.ProjectID != "" {
		req.Set("x-goog-user-project", a.AccountCredentials.ProjectID)
	}
	return nil
}
```

### 3. 私钥解析改进（`relay/channel/vertex/service_account.go`）

改进了 `createSignedJWT` 函数，更好地处理私钥格式：

```go
func createSignedJWT(email, privateKeyPEM string) (string, error) {
	// Normalize the private key: remove header/footer and normalize whitespace
	privateKeyPEM = strings.TrimSpace(privateKeyPEM)
	
	// Remove BEGIN/END markers if present
	privateKeyPEM = strings.ReplaceAll(privateKeyPEM, "-----BEGIN PRIVATE KEY-----", "")
	privateKeyPEM = strings.ReplaceAll(privateKeyPEM, "-----END PRIVATE KEY-----", "")
	privateKeyPEM = strings.ReplaceAll(privateKeyPEM, "-----BEGIN RSA PRIVATE KEY-----", "")
	privateKeyPEM = strings.ReplaceAll(privateKeyPEM, "-----END RSA PRIVATE KEY-----", "")
	
	// Remove all whitespace (including \r, \n, spaces, tabs)
	privateKeyPEM = strings.ReplaceAll(privateKeyPEM, "\r", "")
	privateKeyPEM = strings.ReplaceAll(privateKeyPEM, "\n", "")
	privateKeyPEM = strings.ReplaceAll(privateKeyPEM, "\t", "")
	privateKeyPEM = strings.ReplaceAll(privateKeyPEM, " ", "")
	
	// Handle escaped newlines in JSON strings
	privateKeyPEM = strings.ReplaceAll(privateKeyPEM, "\\n", "")
	
	privateKeyPEM = strings.TrimSpace(privateKeyPEM)
	if privateKeyPEM == "" {
		return "", fmt.Errorf("private key is empty after normalization")
	}

	// Reconstruct PEM format with proper line breaks (64 chars per line)
	pemContent := "-----BEGIN PRIVATE KEY-----\n"
	for i := 0; i < len(privateKeyPEM); i += 64 {
		end := i + 64
		if end > len(privateKeyPEM) {
			end = len(privateKeyPEM)
		}
		pemContent += privateKeyPEM[i:end] + "\n"
	}
	pemContent += "-----END PRIVATE KEY-----\n"

	block, _ := pem.Decode([]byte(pemContent))
	if block == nil {
		return "", fmt.Errorf("failed to parse PEM block containing the private key")
	}
	
	// ... 继续解析和签名
}
```

## 官方实现参考

### 连接流程

1. **客户端 → 本地代理**：
   ```javascript
   const ws = new WebSocket("ws://localhost:8080");
   ws.send(JSON.stringify({ service_url: "wss://..." }));
   ```

2. **本地代理 → Gemini API**：
   - 生成 OAuth2 token（使用 Service Account）
   - 建立 WebSocket 连接到 Gemini API
   - 双向转发所有消息

3. **Setup 消息**：
   ```javascript
   {
     setup: {
       model: "projects/{project}/locations/us-central1/publishers/google/models/{model}",
       generation_config: {
         response_modalities: ["AUDIO"],
         speech_config: {
           voice_config: {
             prebuilt_voice_config: { voice_name: "Puck" }
           }
         }
       },
       system_instruction: { parts: [{ text: "..." }] },
       input_audio_transcription: {},
       output_audio_transcription: {}
     }
   }
   ```

### 消息类型

**客户端到服务器**：
- `setup`：初始配置
- `client_content`：文本消息
- `realtime_input`：实时音频/视频输入
- `tool_response`：工具调用响应

**服务器到客户端**：
- `setupComplete`：设置完成
- `serverContent`：服务器内容（包含 model_turn, turn_complete, interrupted 等）
- `toolCall`：工具调用请求
- `toolCallCancellation`：工具调用取消

## 测试建议

1. **重启服务**：
   ```bash
   cd /Users/caihongzhan/nebulaGitSpace/nebula-new-api
   ./nebula-api
   ```

2. **运行测试脚本**：
   ```bash
   python3 scripts/gemini_live.py
   ```

3. **验证要点**：
   - ✅ WebSocket 连接成功
   - ✅ Setup 消息被接受（收到 `setupComplete`）
   - ✅ 可以发送文本消息并收到响应
   - ✅ 可以发送音频并收到音频响应
   - ✅ Token 计数正确

## 已知限制

1. **音频重采样**：目前使用简单的插值/抽取，可能影响音质。建议使用专业的重采样库（如 libsamplerate）。

2. **协议转换**：OpenAI 兼容模式需要在 OpenAI 格式和 Gemini 格式之间转换，可能存在字段映射不完整的情况。

3. **工具调用**：工具调用的转换逻辑需要进一步测试和完善。

## 下一步

1. 测试所有功能（文本、音频、工具调用）
2. 优化音频重采样算法
3. 完善错误处理和日志
4. 添加更多单元测试
5. 性能优化和并发测试

