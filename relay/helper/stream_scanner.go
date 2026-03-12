package helper

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"one-api/common"
	"one-api/constant"
	"one-api/logger"
	relaycommon "one-api/relay/common"
	"one-api/setting/operation_setting"
	"strings"
	"sync"
	"time"

	"github.com/bytedance/gopkg/util/gopool"

	"github.com/gin-gonic/gin"
)

const (
	InitialScannerBufferSize = 64 << 10 // 64KB (64*1024)
	MaxScannerBufferSize     = 10 << 20 // 10MB (10*1024*1024)
	DefaultPingInterval      = 10 * time.Second
)

func StreamScannerHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo, dataHandler func(data string) bool) {

	if resp == nil || dataHandler == nil {
		return
	}

	// 确保响应体总是被关闭
	defer func() {
		if resp.Body != nil {
			resp.Body.Close()
		}
	}()

	streamingTimeout := time.Duration(constant.StreamingTimeout) * time.Second

	var (
		stopChan   = make(chan bool, 3) // 增加缓冲区避免阻塞
		scanner    = bufio.NewScanner(resp.Body)
		ticker     = time.NewTicker(streamingTimeout)
		pingTicker *time.Ticker
		writeMutex sync.Mutex     // Mutex to protect concurrent writes
		wg         sync.WaitGroup // 用于等待所有 goroutine 退出
	)

	generalSettings := operation_setting.GetGeneralSetting()
	pingEnabled := generalSettings.PingIntervalEnabled && !info.DisablePing
	pingInterval := time.Duration(generalSettings.PingIntervalSeconds) * time.Second
	if pingInterval <= 0 {
		pingInterval = DefaultPingInterval
	}

	if pingEnabled {
		pingTicker = time.NewTicker(pingInterval)
	}

	if common.DebugEnabled {
		// print timeout and ping interval for debugging
		println("relay timeout seconds:", common.RelayTimeout)
		println("streaming timeout seconds:", int64(streamingTimeout.Seconds()))
		println("ping interval seconds:", int64(pingInterval.Seconds()))
	}

	// 改进资源清理，确保所有 goroutine 正确退出
	defer func() {
		// 通知所有 goroutine 停止
		common.SafeSendBool(stopChan, true)

		ticker.Stop()
		if pingTicker != nil {
			pingTicker.Stop()
		}

		// 等待所有 goroutine 退出，最多等待5秒
		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()

		select {
		case <-done:
		case <-time.After(5 * time.Second):
			logger.LogError(c, "timeout waiting for goroutines to exit")
		}

		close(stopChan)
	}()

	scanner.Buffer(make([]byte, InitialScannerBufferSize), MaxScannerBufferSize)
	scanner.Split(bufio.ScanLines)
	SetEventStreamHeaders(c)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctx = context.WithValue(ctx, "stop_chan", stopChan)

	// Handle ping data sending with improved error handling
	if pingEnabled && pingTicker != nil {
		wg.Add(1)
		gopool.Go(func() {
			defer func() {
				wg.Done()
				if r := recover(); r != nil {
					logger.LogError(c, fmt.Sprintf("ping goroutine panic: %v", r))
					common.SafeSendBool(stopChan, true)
				}
				if common.DebugEnabled {
					println("ping goroutine exited")
				}
			}()

			// 添加超时保护，防止 goroutine 无限运行
			maxPingDuration := 30 * time.Minute // 最大 ping 持续时间
			pingTimeout := time.NewTimer(maxPingDuration)
			defer pingTimeout.Stop()

			for {
				select {
				case <-pingTicker.C:
					// 使用超时机制防止写操作阻塞
					done := make(chan error, 1)
					go func() {
						writeMutex.Lock()
						defer writeMutex.Unlock()
						done <- PingData(c)
					}()

					select {
					case err := <-done:
						if err != nil {
							logger.LogError(c, "ping data error: "+err.Error())
							return
						}
						if common.DebugEnabled {
							println("ping data sent")
						}
					case <-time.After(10 * time.Second):
						logger.LogError(c, "ping data send timeout")
						return
					case <-ctx.Done():
						return
					case <-stopChan:
						return
					}
				case <-ctx.Done():
					return
				case <-stopChan:
					return
				case <-c.Request.Context().Done():
					// 监听客户端断开连接
					return
				case <-pingTimeout.C:
					logger.LogError(c, "ping goroutine max duration reached")
					return
				}
			}
		})
	}

	// Scanner goroutine with improved error handling
	wg.Add(1)
	common.RelayCtxGo(ctx, func() {
		defer func() {
			wg.Done()
			if r := recover(); r != nil {
				logger.LogError(c, fmt.Sprintf("scanner goroutine panic: %v", r))
			}
			common.SafeSendBool(stopChan, true)
			if common.DebugEnabled {
				println("scanner goroutine exited")
			}
		}()

		// 按 SSE 规范聚合事件：多行以 data: 开头，空行表示一个事件结束
		var eventBuilder strings.Builder
		hasEvent := false
		eventCount := 0
		const maxDebugEvents = 5 // 打印前5个和后5个事件
		var lastEvents []string  // 保存最后几个事件，用于最后打印
		const maxLastEvents = 5

		for scanner.Scan() {
			// 检查是否需要停止
			select {
			case <-stopChan:
				return
			case <-ctx.Done():
				return
			case <-c.Request.Context().Done():
				return
			default:
			}

			ticker.Reset(streamingTimeout)
			line := scanner.Text()
			line = strings.TrimSuffix(line, "\r")

			// 空行：一个事件结束
			if len(strings.TrimSpace(line)) == 0 {
				if hasEvent && eventBuilder.Len() > 0 {
					payload := eventBuilder.String()
					eventBuilder.Reset()
					hasEvent = false

					if strings.HasPrefix(payload, "[DONE]") {
						if common.DebugEnabled {
							// 打印最后几个事件
							if len(lastEvents) > 0 {
								println(fmt.Sprintf("--- last %d events (total: %d) ---", len(lastEvents), eventCount))
								for i, event := range lastEvents {
									truncatedContent := common.TruncateJsonValues(event)
									println(fmt.Sprintf("event[%d]: data: %s", eventCount-len(lastEvents)+i, truncatedContent))
								}
							}
							println("received [DONE], stopping scanner")
						}
						return
					}

					// 打印前5个事件
					if common.DebugEnabled && eventCount < maxDebugEvents {
						truncatedContent := common.TruncateJsonValues(payload)
						println(fmt.Sprintf("event[%d]: data: %s", eventCount, truncatedContent))
					}

					// 保存最后几个事件（最多保存5个）
					if common.DebugEnabled {
						lastEvents = append(lastEvents, payload)
						if len(lastEvents) > maxLastEvents {
							lastEvents = lastEvents[1:] // 移除最旧的事件
						}
					}

					eventCount++

					info.SetFirstResponseTime()

					done := make(chan bool, 1)
					go func(p string) {
						writeMutex.Lock()
						defer writeMutex.Unlock()
						done <- dataHandler(p)
					}(payload)

					select {
					case success := <-done:
						if !success {
							return
						}
					case <-time.After(10 * time.Second):
						logger.LogError(c, "data handler timeout")
						return
					case <-ctx.Done():
						return
					case <-stopChan:
						return
					}
				}
				continue
			}

			// 只处理 data: 行，忽略 event:、id:、retry: 等
			if strings.HasPrefix(line, "data:") {
				content := strings.TrimLeft(strings.TrimPrefix(line, "data:"), " ")
				if eventBuilder.Len() > 0 {
					eventBuilder.WriteString("\n")
				}
				eventBuilder.WriteString(content)
				hasEvent = true
				continue
			}
			// 直接收到 [DONE]（有些上游会不加 data: 前缀）
			if strings.HasPrefix(line, "[DONE]") {
				common.SysLog(fmt.Sprintf("[StreamScanner] Received [DONE] signal, total events received: %d", eventCount))
				if common.DebugEnabled {
					// 打印最后几个事件
					if len(lastEvents) > 0 {
						println(fmt.Sprintf("--- last %d events (total: %d) ---", len(lastEvents), eventCount))
						for i, event := range lastEvents {
							truncatedContent := common.TruncateJsonValues(event)
							println(fmt.Sprintf("event[%d]: data: %s", eventCount-len(lastEvents)+i, truncatedContent))
						}
					}
					println("received [DONE], stopping scanner")
				}
				return
			}
			// 其他前缀忽略
		}

		// 文件尾还有未发送的事件
		if hasEvent && eventBuilder.Len() > 0 {
			payload := eventBuilder.String()
			if !strings.HasPrefix(payload, "[DONE]") {
				// 打印前5个或后5个事件
				if common.DebugEnabled {
					if eventCount < maxDebugEvents {
						// 前5个事件
						truncatedContent := common.TruncateJsonValues(payload)
						println(fmt.Sprintf("event[%d]: data: %s", eventCount, truncatedContent))
					} else {
						// 保存到最后5个事件中
						lastEvents = append(lastEvents, payload)
						if len(lastEvents) > maxLastEvents {
							lastEvents = lastEvents[1:]
						}
					}
				}
				eventCount++

				info.SetFirstResponseTime()
				done := make(chan bool, 1)
				go func(p string) {
					writeMutex.Lock()
					defer writeMutex.Unlock()
					done <- dataHandler(p)
				}(payload)

				select {
				case success := <-done:
					if !success {
						return
					}
				case <-time.After(10 * time.Second):
					logger.LogError(c, "data handler timeout")
					return
				case <-ctx.Done():
					return
				case <-stopChan:
					return
				}
			}
		}

		// 打印最后几个事件（如果还有未打印的）
		if common.DebugEnabled && len(lastEvents) > 0 && eventCount > maxDebugEvents {
			println(fmt.Sprintf("--- last %d events (total: %d) ---", len(lastEvents), eventCount))
			startIdx := eventCount - len(lastEvents)
			for i, event := range lastEvents {
				truncatedContent := common.TruncateJsonValues(event)
				println(fmt.Sprintf("event[%d]: data: %s", startIdx+i, truncatedContent))
			}
		}

		if err := scanner.Err(); err != nil {
			if err != io.EOF {
				logger.LogError(c, "scanner error: "+err.Error())
			}
		}
	})

	// 主循环等待完成或超时
	select {
	case <-ticker.C:
		// 超时处理逻辑
		logger.LogError(c, "streaming timeout")
	case <-stopChan:
		// 正常结束
		logger.LogInfo(c, "streaming finished")
	case <-c.Request.Context().Done():
		// 客户端断开连接
		logger.LogInfo(c, "client disconnected")
	}
}
