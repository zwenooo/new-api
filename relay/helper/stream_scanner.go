package helper

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"one-api/common"
	"one-api/constant"
	"one-api/logger"
	relaycommon "one-api/relay/common"
	"one-api/setting/operation_setting"
	"one-api/types"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/bytedance/gopkg/util/gopool"

	"github.com/gin-gonic/gin"
)

const (
	InitialScannerBufferSize    = 64 << 10 // 64KB (64*1024)
	DefaultMaxScannerBufferSize = 64 << 20 // 64MB (64*1024*1024) default SSE buffer size
	DefaultPingInterval         = 10 * time.Second
)

func getScannerBufferSize() int {
	if constant.StreamScannerMaxBufferMB > 0 {
		return constant.StreamScannerMaxBufferMB << 20
	}
	return DefaultMaxScannerBufferSize
}

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
		stopChan     = make(chan bool, 3) // 增加缓冲区避免阻塞
		activityCh   = make(chan struct{}, 1)
		scanner      = bufio.NewScanner(resp.Body)
		timeoutTimer = time.NewTimer(streamingTimeout)
		pingTicker   *time.Ticker
		writeMutex   sync.Mutex     // Mutex to protect concurrent writes
		wg           sync.WaitGroup // 用于等待所有 goroutine 退出
		lastEvent    string
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

		if timeoutTimer != nil {
			if !timeoutTimer.Stop() {
				select {
				case <-timeoutTimer.C:
				default:
				}
			}
		}
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

	scanner.Buffer(make([]byte, InitialScannerBufferSize), getScannerBufferSize())
	scanner.Split(bufio.ScanLines)
	copyUpstreamStreamHeaders(c, resp)
	SetEventStreamHeaders(c)

	ctx, cancel := context.WithCancel(c.Request.Context())
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

	dataChan := make(chan string, 10)

	wg.Add(1)
	gopool.Go(func() {
		defer func() {
			wg.Done()
			if r := recover(); r != nil {
				logger.LogError(c, fmt.Sprintf("data handler goroutine panic: %v\n%s", r, string(debug.Stack())))
				if common.GetContextKeyString(c, constant.ContextKeyStreamExitReason) == "" {
					common.SetContextKey(c, constant.ContextKeyStreamExitReason, "data_handler_panic")
				}
				if common.GetContextKeyString(c, constant.ContextKeyStreamExitError) == "" {
					common.SetContextKey(c, constant.ContextKeyStreamExitError, fmt.Sprintf("panic: %v", r))
				}
			}
			cancel()
			common.SafeSendBool(stopChan, true)
			if common.DebugEnabled {
				println("data handler goroutine exited")
			}
		}()

		// Drain queued stream events before honoring shutdown. The scanner can enqueue
		// a terminal payload immediately before `[DONE]`, and dropping that payload
		// causes clients to see an incomplete stream with zero usage.
		for data := range dataChan {
			shouldContinue := true
			func() {
				writeMutex.Lock()
				defer writeMutex.Unlock()
				defer func() {
					if r := recover(); r != nil {
						logger.LogError(c, fmt.Sprintf("data handler panic: %v\n%s", r, string(debug.Stack())))
						if common.GetContextKeyString(c, constant.ContextKeyStreamExitReason) == "" {
							common.SetContextKey(c, constant.ContextKeyStreamExitReason, "data_handler_panic")
						}
						if common.GetContextKeyString(c, constant.ContextKeyStreamExitError) == "" {
							common.SetContextKey(c, constant.ContextKeyStreamExitError, fmt.Sprintf("panic: %v", r))
						}
						shouldContinue = false
					}
				}()
				shouldContinue = dataHandler(data)
			}()
			if !shouldContinue {
				return
			}
		}
	})

	// Scanner goroutine with improved error handling
	wg.Add(1)
	common.RelayCtxGo(ctx, func() {
		defer func() {
			close(dataChan)
			wg.Done()
			if r := recover(); r != nil {
				logger.LogError(c, fmt.Sprintf("scanner goroutine panic: %v", r))
			}
			common.SafeSendBool(stopChan, true)
			if common.DebugEnabled {
				println("scanner goroutine exited")
			}
		}()

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

			select {
			case activityCh <- struct{}{}:
			default:
			}

			line := strings.TrimSuffix(scanner.Text(), "\r")
			trimmed := strings.TrimSpace(line)
			if common.DebugEnabled {
				println(line)
			}

			if strings.HasPrefix(trimmed, "event:") {
				lastEvent = strings.TrimSpace(strings.TrimPrefix(trimmed, "event:"))
				common.SetContextKey(c, constant.ContextKeyUpstreamSSEEvent, lastEvent)
				continue
			}

			if trimmed == "" {
				if lastEvent != "" {
					lastEvent = ""
					common.SetContextKey(c, constant.ContextKeyUpstreamSSEEvent, "")
				}
				continue
			}

			// Ignore SSE comments (e.g. keepalive ":" lines from some upstreams).
			if strings.HasPrefix(trimmed, ":") {
				continue
			}

			var data string
			if strings.HasPrefix(trimmed, "data:") {
				data = strings.TrimSpace(strings.TrimPrefix(trimmed, "data:"))
			} else if strings.HasPrefix(trimmed, "[DONE]") {
				data = "[DONE]"
			} else {
				continue
			}

			if strings.HasPrefix(data, "[DONE]") {
				// done, 处理完成标志，直接退出停止读取剩余数据防止出错
				if common.DebugEnabled {
					println("received [DONE], stopping scanner")
				}
				common.SetContextKey(c, constant.ContextKeyStreamExitReason, "done")
				return
			} else {
				info.SetFirstResponseTime()
				select {
				case dataChan <- data:
				case <-ctx.Done():
					return
				case <-stopChan:
					return
				}
			}
		}

		if err := scanner.Err(); err != nil {
			if err != io.EOF {
				existingReason := common.GetContextKeyString(c, constant.ContextKeyStreamExitReason)
				if existingReason == "" {
					logger.LogError(c, "scanner error: "+err.Error())
					common.SetContextKey(c, constant.ContextKeyStreamExitError, err.Error())
					if errors.Is(err, bufio.ErrTooLong) {
						common.SetContextKey(c, constant.ContextKeyStreamExitReason, "scanner_too_long")
					} else {
						common.SetContextKey(c, constant.ContextKeyStreamExitReason, "scanner_error")
					}
				} else if common.DebugEnabled {
					println("scanner stopped after reason:", existingReason, "err:", err.Error())
				}
			}
		}
	})

	// 主循环等待完成或超时（由本 goroutine 统一 Reset timer，避免 ticker/reset 竞态）
	for {
		select {
		case <-timeoutTimer.C:
			// 超时处理逻辑
			common.SetContextKey(c, constant.ContextKeyStreamExitReason, "timeout")
			if common.GetContextKeyString(c, constant.ContextKeyStreamExitError) == "" {
				common.SetContextKey(c, constant.ContextKeyStreamExitError, fmt.Sprintf("no upstream data for %ds", int64(streamingTimeout.Seconds())))
			}
			logger.LogError(c, fmt.Sprintf("streaming timeout: channel=%d model=%s timeout=%ds", c.GetInt("channel_id"), info.OriginModelName, int64(streamingTimeout.Seconds())))
			return
		case <-activityCh:
			if timeoutTimer != nil {
				if !timeoutTimer.Stop() {
					select {
					case <-timeoutTimer.C:
					default:
					}
				}
				timeoutTimer.Reset(streamingTimeout)
			}
		case <-stopChan:
			// 正常结束
			logger.LogRequestInfo(c, "streaming finished")
			return
		case <-c.Request.Context().Done():
			// 客户端断开连接
			common.SetContextKey(c, constant.ContextKeyStreamExitReason, "client_disconnected")
			if common.GetContextKeyString(c, constant.ContextKeyStreamExitError) == "" {
				common.SetContextKey(c, constant.ContextKeyStreamExitError, fmt.Sprintf("client_ip=%s ua=%q", c.ClientIP(), c.GetHeader("User-Agent")))
			}
			logger.LogRequestInfo(c, "client disconnected")
			return
		}
	}
}

func BuildStreamExitError(c *gin.Context, info *relaycommon.RelayInfo) *types.NewAPIError {
	if c == nil || c.Writer == nil {
		return nil
	}
	bodySent := c.Writer.Size() > 0
	if bodySent && info != nil {
		bodySent = info.HasSendResponse() || info.SendResponseCount > 0
	}
	if bodySent {
		return nil
	}

	reason := strings.TrimSpace(common.GetContextKeyString(c, constant.ContextKeyStreamExitReason))
	errText := strings.TrimSpace(common.GetContextKeyString(c, constant.ContextKeyStreamExitError))

	switch reason {
	case "done", "client_disconnected":
		return nil
	case "":
		if errText == "" {
			return types.NewOpenAIError(errors.New("no response received from upstream stream"), types.ErrorCodeEmptyResponse, http.StatusInternalServerError)
		}
		return types.NewOpenAIError(fmt.Errorf("upstream stream ended before sending any response: %s", errText), types.ErrorCodeBadResponse, http.StatusBadGateway)
	}

	statusCode := http.StatusBadGateway
	if reason == "timeout" {
		statusCode = http.StatusGatewayTimeout
	}
	if errText == "" {
		errText = fmt.Sprintf("reason=%s", reason)
	}
	return types.NewOpenAIError(fmt.Errorf("upstream stream exited unexpectedly (%s): %s", reason, errText), types.ErrorCodeBadResponse, statusCode)
}

func copyUpstreamStreamHeaders(c *gin.Context, resp *http.Response) {
	if c == nil || resp == nil {
		return
	}
	for key, vals := range resp.Header {
		if len(vals) == 0 {
			continue
		}
		lower := strings.ToLower(key)
		if lower == "x-codex-turn-state" ||
			lower == "x-models-etag" ||
			lower == "cf-ray" ||
			lower == "x-request-id" ||
			lower == "x-oai-request-id" ||
			strings.HasPrefix(lower, "x-codex-") {
			c.Writer.Header().Set(key, vals[0])
		}
	}
}
