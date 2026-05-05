package openai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"one-api/common"
	"one-api/constant"
	"one-api/dto"
	"one-api/logger"
	relaycommon "one-api/relay/common"
	"one-api/relay/helper"
	"one-api/service"
	"one-api/types"
	"strings"

	"github.com/gin-gonic/gin"
)

type responsesEventStreamEnvelope struct {
	Type     string          `json:"type"`
	Response json.RawMessage `json:"response,omitempty"`
	Error    json.RawMessage `json:"error,omitempty"`
}

type responsesFunctionCallFallback struct {
	Name      string
	Arguments string
}

func isEventStreamResponse(contentType string, body []byte) bool {
	if strings.Contains(strings.ToLower(strings.TrimSpace(contentType)), "text/event-stream") {
		return true
	}
	b := bytes.TrimSpace(body)
	if len(b) == 0 {
		return false
	}
	// SSE payloads typically start with "event:" / "data:" or ":" comments.
	if bytes.HasPrefix(b, []byte("event:")) || bytes.HasPrefix(b, []byte("data:")) || bytes.HasPrefix(b, []byte(":")) {
		return true
	}
	return false
}

func extractFinalResponsesJSONFromEventStream(body []byte) (json.RawMessage, error) {
	if len(body) == 0 {
		return nil, fmt.Errorf("empty event-stream body")
	}

	// Prefer terminal events that include a full "response" object.
	priority := map[string]int{
		"response.completed":  4,
		"response.done":       3,
		"response.incomplete": 2,
		"response.failed":     1,
	}

	bestPriority := -1
	var best json.RawMessage
	var lastUnmarshalErr error

	var cur bytes.Buffer
	flush := func() bool {
		if cur.Len() == 0 {
			return true
		}
		payload := bytes.TrimSpace(cur.Bytes())
		cur.Reset()

		if len(payload) == 0 {
			return true
		}
		if bytes.Equal(payload, []byte("[DONE]")) {
			return false
		}

		var env responsesEventStreamEnvelope
		if err := common.Unmarshal(payload, &env); err != nil {
			lastUnmarshalErr = err
			return true
		}

		if len(env.Response) == 0 {
			return true
		}

		p := 0
		if v, ok := priority[strings.TrimSpace(env.Type)]; ok {
			p = v
		}

		// Keep the latest/best terminal response. Fallback: keep the last seen response object.
		if p > bestPriority || (p == bestPriority && len(env.Response) > 0) {
			bestPriority = p
			best = env.Response
		}
		return true
	}

	lines := bytes.Split(body, []byte{'\n'})
	for _, rawLine := range lines {
		line := bytes.TrimSpace(bytes.TrimSuffix(rawLine, []byte{'\r'}))
		if len(line) == 0 {
			if !flush() {
				break
			}
			continue
		}
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		part := bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
		if cur.Len() > 0 {
			_ = cur.WriteByte('\n')
		}
		_, _ = cur.Write(part)
	}
	_ = flush()

	if len(best) > 0 {
		return best, nil
	}
	if lastUnmarshalErr != nil {
		return nil, fmt.Errorf("parse event-stream payload failed: %w", lastUnmarshalErr)
	}
	return nil, fmt.Errorf("no responses object found in event-stream")
}

func responsesFunctionCallFallbackKey(payload map[string]interface{}) string {
	if payload == nil {
		return ""
	}
	if callID := strings.TrimSpace(common.Interface2String(payload["call_id"])); callID != "" {
		return callID
	}
	if id := strings.TrimSpace(common.Interface2String(payload["id"])); id != "" {
		return id
	}
	if outputIndex, ok := payload["output_index"]; ok {
		key := strings.TrimSpace(fmt.Sprintf("%v", outputIndex))
		if key != "" && key != "<nil>" {
			return "output_index:" + key
		}
	}
	name := strings.TrimSpace(common.Interface2String(payload["name"]))
	if name != "" {
		return "name:" + name
	}
	return ""
}

func ensureResponsesFunctionCallFallback(state map[string]responsesFunctionCallFallback, order *[]string, payload map[string]interface{}) string {
	key := responsesFunctionCallFallbackKey(payload)
	if key == "" {
		return ""
	}
	if _, exists := state[key]; !exists {
		*order = append(*order, key)
	}
	entry := state[key]
	if name := strings.TrimSpace(common.Interface2String(payload["name"])); name != "" {
		entry.Name = name
	}
	state[key] = entry
	return key
}

func appendResponsesFunctionCallFallbackDelta(state map[string]responsesFunctionCallFallback, order *[]string, payload map[string]interface{}) {
	key := ensureResponsesFunctionCallFallback(state, order, payload)
	if key == "" {
		return
	}
	delta := common.Interface2String(payload["delta"])
	if strings.TrimSpace(delta) == "" {
		return
	}
	entry := state[key]
	entry.Arguments += delta
	state[key] = entry
}

func setResponsesFunctionCallFallbackArguments(state map[string]responsesFunctionCallFallback, order *[]string, payload map[string]interface{}) {
	key := ensureResponsesFunctionCallFallback(state, order, payload)
	if key == "" {
		return
	}
	arguments := common.Interface2String(payload["arguments"])
	if strings.TrimSpace(arguments) == "" {
		return
	}
	entry := state[key]
	if len(arguments) >= len(entry.Arguments) {
		entry.Arguments = arguments
	}
	state[key] = entry
}

func appendResponsesMessageOutputText(builder *strings.Builder, item map[string]interface{}) bool {
	if builder == nil || item == nil {
		return false
	}
	contentItems, ok := item["content"].([]interface{})
	if !ok {
		return false
	}
	appended := false
	for _, segmentAny := range contentItems {
		segment, ok := segmentAny.(map[string]interface{})
		if !ok {
			continue
		}
		if strings.TrimSpace(common.Interface2String(segment["type"])) != "output_text" {
			continue
		}
		text := common.Interface2String(segment["text"])
		if text == "" {
			continue
		}
		builder.WriteString(text)
		appended = true
	}
	return appended
}

func buildResponsesUsageFallbackText(text string, functionCalls map[string]responsesFunctionCallFallback, order []string) string {
	var builder strings.Builder
	builder.WriteString(text)
	for _, key := range order {
		entry, ok := functionCalls[key]
		if !ok {
			continue
		}
		builder.WriteString(entry.Name)
		builder.WriteString(entry.Arguments)
	}
	return builder.String()
}

func OaiResponsesHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)

	// read response body
	var responsesResponse dto.OpenAIResponsesResponse
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}

	// Some upstreams (notably codex-service-go/ChatGPT backend) may return SSE even when stream=false.
	// For non-stream /v1/responses callers, we aggregate the terminal SSE event into a single JSON response.
	aggregatedFromEventStream := false
	if isEventStreamResponse(resp.Header.Get("Content-Type"), responseBody) {
		final, err := extractFinalResponsesJSONFromEventStream(responseBody)
		if err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}
		responseBody = final
		aggregatedFromEventStream = true
	}

	err = common.Unmarshal(responseBody, &responsesResponse)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if oaiError := responsesResponse.GetOpenAIError(); oaiError != nil && oaiError.Type != "" {
		return nil, types.WithOpenAIError(*oaiError, resp.StatusCode)
	}

	if responsesResponse.HasImageGenerationCall() {
		c.Set("image_generation_call", true)
		c.Set("image_generation_call_quality", responsesResponse.GetQuality())
		c.Set("image_generation_call_size", responsesResponse.GetSize())
	}

	if !helper.IsAdminUser(c) {
		var bodyMap map[string]interface{}
		if err := common.Unmarshal(responseBody, &bodyMap); err == nil {
			bodyMap["model"] = info.OriginModelName
			responseBody, _ = common.Marshal(bodyMap)
		}
	}

	// 写入新的 response body
	if aggregatedFromEventStream {
		// Upstream headers are for SSE; sanitize and return JSON instead.
		hdr := http.Header{}
		for k, v := range resp.Header {
			if len(v) == 0 {
				continue
			}
			if strings.EqualFold(k, "Content-Length") ||
				strings.EqualFold(k, "Content-Type") ||
				strings.EqualFold(k, "Transfer-Encoding") ||
				strings.EqualFold(k, "Connection") {
				continue
			}
			hdr.Set(k, v[0])
		}
		hdr.Set("Content-Type", "application/json; charset=utf-8")
		service.IOCopyBytesGracefully(c, &http.Response{StatusCode: resp.StatusCode, Header: hdr}, responseBody)
	} else {
		service.IOCopyBytesGracefully(c, resp, responseBody)
	}

	// compute usage
	usage := dto.Usage{}
	if responsesResponse.Usage != nil {
		usage.PromptTokens = responsesResponse.Usage.InputTokens
		usage.CompletionTokens = responsesResponse.Usage.OutputTokens
		usage.TotalTokens = responsesResponse.Usage.TotalTokens
		if responsesResponse.Usage.InputTokensDetails != nil {
			usage.PromptTokensDetails.CachedTokens = responsesResponse.Usage.InputTokensDetails.CachedTokens
		}
	}
	if info == nil || info.ResponsesUsageInfo == nil || info.ResponsesUsageInfo.BuiltInTools == nil {
		return &usage, nil
	}
	// 解析 Tools 用量
	for _, tool := range responsesResponse.Tools {
		toolType := relaycommon.NormalizeResponsesToolType(common.Interface2String(tool["type"]))
		if strings.TrimSpace(toolType) == "" {
			logger.LogError(c, fmt.Sprintf("invalid tool type in response: %v", tool["type"]))
			continue
		}
		buildToolinfo := relaycommon.EnsureResponsesBuiltInTool(info, toolType)
		if buildToolinfo == nil {
			logger.LogError(c, fmt.Sprintf("failed to init BuiltInTools for tool type: %v", tool["type"]))
			continue
		}
		buildToolinfo.CallCount++
	}
	return &usage, nil
}

func OaiResponsesStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		logger.LogError(c, "invalid response or response body")
		return nil, types.NewError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse)
	}

	defer service.CloseResponseBodyGracefully(resp)

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		newAPIError := service.RelayErrorHandler(c.Request.Context(), resp, false)
		if newAPIError == nil {
			newAPIError = types.InitOpenAIError(types.ErrorCodeBadResponseStatusCode, resp.StatusCode)
		}
		return nil, newAPIError
	}

	var usage = &dto.Usage{}
	var responseTextBuilder strings.Builder
	functionCallFallbacks := make(map[string]responsesFunctionCallFallback)
	functionCallFallbackOrder := make([]string, 0, 4)
	sawOutputTextDelta := false
	uaLower := strings.ToLower(strings.TrimSpace(c.GetHeader("User-Agent")))
	compatOpenCode := strings.Contains(uaLower, "opencode/") ||
		strings.Contains(uaLower, "ai-sdk/openai") ||
		strings.Contains(uaLower, "openai/js")

	helper.StreamScannerHandler(c, resp, info, func(data string) bool {
		data = strings.TrimSpace(data)
		if data == "" {
			return true
		}

		// 检查当前数据是否包含 completed 状态和 usage 信息
		var streamResponse dto.ResponsesStreamResponse
		if err := common.UnmarshalJsonStr(data, &streamResponse); err == nil {
			dataForClient := data
			var bodyMap map[string]interface{}
			bodyMapOk := common.UnmarshalJsonStr(data, &bodyMap) == nil
			changedForClient := false
			if bodyMapOk {
				if !helper.IsAdminUser(c) {
					if responseObj, ok := bodyMap["response"].(map[string]interface{}); ok {
						responseObj["model"] = info.OriginModelName
						changedForClient = true
					}
				}
				// ChatGPT Codex backend errors use {type:"error", error:{code,message,param}}.
				// Some OpenAI-compatible clients (e.g. OpenCode) expect code/message/param at top-level.
				if compatOpenCode && strings.TrimSpace(streamResponse.Type) == "error" {
					if nested, ok := bodyMap["error"].(map[string]interface{}); ok && nested != nil {
						if _, ok := bodyMap["code"]; !ok {
							if v, ok := nested["code"]; ok {
								if s := strings.TrimSpace(common.Interface2String(v)); s != "" {
									bodyMap["code"] = s
									changedForClient = true
								}
							}
						}
						if _, ok := bodyMap["message"]; !ok {
							if v, ok := nested["message"]; ok {
								if s := strings.TrimSpace(common.Interface2String(v)); s != "" {
									bodyMap["message"] = s
									changedForClient = true
								}
							}
						}
						if _, ok := bodyMap["param"]; !ok {
							if v, ok := nested["param"]; ok {
								if s := strings.TrimSpace(common.Interface2String(v)); s != "" {
									bodyMap["param"] = s
									changedForClient = true
								}
							}
						}
					}
				}
				if changedForClient {
					if dataBytes, err := common.Marshal(bodyMap); err == nil {
						dataForClient = string(dataBytes)
					}
				}
			}

			if streamResponse.Type == "" {
				if bodyMapOk {
					if _, ok := bodyMap["error"]; ok {
						helper.ResponseChunkData(c, dto.ResponsesStreamResponse{Type: "error"}, dataForClient)
						if common.GetContextKeyString(c, constant.ContextKeyStreamExitReason) == "" {
							common.SetContextKey(c, constant.ContextKeyStreamExitReason, "error")
						}
						// Some upstreams emit an "error" event followed by a terminal response.* event.
						// Keep scanning so clients can reliably receive the final status event.
						return true
					}
				}
				return true
			}

			sendResponsesStreamData(c, streamResponse, dataForClient)
			shouldContinue := true
			switch streamResponse.Type {
			case "response.completed":
				if common.GetContextKeyString(c, constant.ContextKeyStreamExitReason) == "" {
					common.SetContextKey(c, constant.ContextKeyStreamExitReason, "done")
				}
				if streamResponse.Response != nil {
					if streamResponse.Response.Usage != nil {
						if streamResponse.Response.Usage.InputTokens != 0 {
							usage.PromptTokens = streamResponse.Response.Usage.InputTokens
						}
						if streamResponse.Response.Usage.OutputTokens != 0 {
							usage.CompletionTokens = streamResponse.Response.Usage.OutputTokens
						}
						if streamResponse.Response.Usage.TotalTokens != 0 {
							usage.TotalTokens = streamResponse.Response.Usage.TotalTokens
						}
						if streamResponse.Response.Usage.InputTokensDetails != nil {
							usage.PromptTokensDetails.CachedTokens = streamResponse.Response.Usage.InputTokensDetails.CachedTokens
						}
					}
					if streamResponse.Response.HasImageGenerationCall() {
						c.Set("image_generation_call", true)
						c.Set("image_generation_call_quality", streamResponse.Response.GetQuality())
						c.Set("image_generation_call_size", streamResponse.Response.GetSize())
					}
				}
				shouldContinue = false
			case "response.incomplete":
				if common.GetContextKeyString(c, constant.ContextKeyStreamExitReason) == "" {
					common.SetContextKey(c, constant.ContextKeyStreamExitReason, "incomplete")
				}
				if streamResponse.Response != nil && streamResponse.Response.Usage != nil {
					if streamResponse.Response.Usage.InputTokens != 0 {
						usage.PromptTokens = streamResponse.Response.Usage.InputTokens
					}
					if streamResponse.Response.Usage.OutputTokens != 0 {
						usage.CompletionTokens = streamResponse.Response.Usage.OutputTokens
					}
					if streamResponse.Response.Usage.TotalTokens != 0 {
						usage.TotalTokens = streamResponse.Response.Usage.TotalTokens
					}
					if streamResponse.Response.Usage.InputTokensDetails != nil {
						usage.PromptTokensDetails.CachedTokens = streamResponse.Response.Usage.InputTokensDetails.CachedTokens
					}
				}
				shouldContinue = false
			case "response.done":
				if common.GetContextKeyString(c, constant.ContextKeyStreamExitReason) == "" {
					common.SetContextKey(c, constant.ContextKeyStreamExitReason, "done")
				}
				if streamResponse.Response != nil && streamResponse.Response.Usage != nil {
					if streamResponse.Response.Usage.InputTokens != 0 {
						usage.PromptTokens = streamResponse.Response.Usage.InputTokens
					}
					if streamResponse.Response.Usage.OutputTokens != 0 {
						usage.CompletionTokens = streamResponse.Response.Usage.OutputTokens
					}
					if streamResponse.Response.Usage.TotalTokens != 0 {
						usage.TotalTokens = streamResponse.Response.Usage.TotalTokens
					}
					if streamResponse.Response.Usage.InputTokensDetails != nil {
						usage.PromptTokensDetails.CachedTokens = streamResponse.Response.Usage.InputTokensDetails.CachedTokens
					}
				}
				shouldContinue = false
			case "response.failed":
				if common.GetContextKeyString(c, constant.ContextKeyStreamExitReason) == "" {
					common.SetContextKey(c, constant.ContextKeyStreamExitReason, "failed")
				}
				shouldContinue = false
			case "error":
				if common.GetContextKeyString(c, constant.ContextKeyStreamExitReason) == "" {
					common.SetContextKey(c, constant.ContextKeyStreamExitReason, "error")
				}
				if common.GetContextKeyString(c, constant.ContextKeyStreamExitError) == "" && bodyMapOk {
					var exitMsg string
					code, _ := bodyMap["code"].(string)
					message, _ := bodyMap["message"].(string)
					if strings.TrimSpace(code) == "" || strings.TrimSpace(message) == "" {
						if nested, ok := bodyMap["error"].(map[string]interface{}); ok && nested != nil {
							if strings.TrimSpace(code) == "" {
								code = strings.TrimSpace(common.Interface2String(nested["code"]))
							}
							if strings.TrimSpace(message) == "" {
								message = strings.TrimSpace(common.Interface2String(nested["message"]))
							}
						}
					}
					switch {
					case strings.TrimSpace(code) != "" && strings.TrimSpace(message) != "":
						exitMsg = code + ": " + message
					case strings.TrimSpace(message) != "":
						exitMsg = message
					case strings.TrimSpace(code) != "":
						exitMsg = code
					}
					if exitMsg != "" {
						common.SetContextKey(c, constant.ContextKeyStreamExitError, exitMsg)
					}
				}
				// Keep scanning: some upstreams emit "error" before a terminal response.failed event.
				shouldContinue = true
			case "response.output_text.delta":
				// 处理输出文本
				sawOutputTextDelta = true
				responseTextBuilder.WriteString(streamResponse.Delta)
			case "response.function_call_arguments.delta":
				if bodyMapOk {
					appendResponsesFunctionCallFallbackDelta(functionCallFallbacks, &functionCallFallbackOrder, bodyMap)
				}
			case "response.function_call_arguments.done":
				if bodyMapOk {
					setResponsesFunctionCallFallbackArguments(functionCallFallbacks, &functionCallFallbackOrder, bodyMap)
				}
			case dto.ResponsesOutputTypeItemDone:
				if bodyMapOk {
					if item, ok := bodyMap["item"].(map[string]interface{}); ok && item != nil {
						switch strings.TrimSpace(common.Interface2String(item["type"])) {
						case "message":
							if !sawOutputTextDelta {
								appendResponsesMessageOutputText(&responseTextBuilder, item)
							}
						case "function_call":
							setResponsesFunctionCallFallbackArguments(functionCallFallbacks, &functionCallFallbackOrder, item)
						}
					}
				}
				// 函数调用处理
				if streamResponse.Item != nil {
					switch streamResponse.Item.Type {
					case dto.BuildInCallWebSearchCall:
						relaycommon.IncrementResponsesBuiltInToolCall(info, dto.BuildInToolWebSearch)
					}
				}
			}
			return shouldContinue
		} else {
			logger.LogError(c, "failed to unmarshal stream response: "+err.Error())
			if common.GetContextKeyString(c, constant.ContextKeyStreamExitReason) == "" {
				common.SetContextKey(c, constant.ContextKeyStreamExitReason, "bad_stream_data")
				common.SetContextKey(c, constant.ContextKeyStreamExitError, err.Error())
			}
			return false
		}
	})

	if streamErr := helper.BuildStreamExitError(c, info); streamErr != nil {
		return nil, streamErr
	}

	if usage.CompletionTokens == 0 {
		// 计算输出文本和函数调用参数的 token 数量
		tempStr := buildResponsesUsageFallbackText(responseTextBuilder.String(), functionCallFallbacks, functionCallFallbackOrder)
		if len(tempStr) > 0 {
			// 非正常结束，使用输出文本的 token 数量
			completionTokens := service.CountTextToken(tempStr, info.UpstreamModelName)
			usage.CompletionTokens = completionTokens
		}
	}

	if usage.PromptTokens == 0 && usage.CompletionTokens != 0 {
		usage.PromptTokens = info.PromptTokens
	}

	usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens

	return usage, nil
}
