package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	guuid "github.com/google/uuid"

	instsvc "codex-service-go/internal/services/instances"
)

func (h *APIHandler) handleChatCompat(c *gin.Context, inst *instsvc.InstanceWithPaths) {
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.String(http.StatusBadRequest, "invalid body: %v", err)
		return
	}
	_ = c.Request.Body.Close()

	var raw map[string]interface{}
	if len(bodyBytes) > 0 {
		if err := json.Unmarshal(bodyBytes, &raw); err != nil {
			c.String(http.StatusBadRequest, "invalid json: %v", err)
			return
		}
	}
	if raw == nil {
		raw = make(map[string]interface{})
	}

	var chatReq chatCompletionsRequest
	if shouldTreatAsResponsesFormat(raw) {
		chatReq, err = convertResponsesRequestToChatCompletions(raw)
		if err != nil {
			c.String(http.StatusBadRequest, "invalid responses compat body: %v", err)
			return
		}
	} else if len(bodyBytes) > 0 {
		if err := json.Unmarshal(bodyBytes, &chatReq); err != nil {
			c.String(http.StatusBadRequest, "invalid json: %v", err)
			return
		}
	}

	// 检测并标注丢弃的采样参数（上游 Responses 不支持 temperature/top_p 等），仅做提示，不影响流程
	if len(raw) > 0 {
		dropped := make([]string, 0, 4)
		for _, k := range []string{"temperature", "top_p"} { // 可按需扩展
			if _, ok := raw[k]; ok {
				dropped = append(dropped, k)
			}
		}
		if len(dropped) > 0 {
			c.Writer.Header().Set("X-Compat-Params-Dropped", strings.Join(dropped, ","))
		}
	}

	accept := strings.ToLower(c.GetHeader("Accept"))
	wantStreamByQuery := parseBoolQuery(c.Request.URL, "stream")
	allowStream := chatReq.Stream == nil || (chatReq.Stream != nil && *chatReq.Stream)
	wantStream := (chatReq.Stream != nil && *chatReq.Stream) || wantStreamByQuery || (strings.Contains(accept, "text/event-stream") && allowStream)
	shouldStream := allowStream && (h.compat.ForceStreamCompat || wantStream)

	responsesReq, err := h.buildResponsesRequest(&chatReq, shouldStream)
	if err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}

	payload, err := json.Marshal(responsesReq)
	if err != nil {
		c.String(http.StatusInternalServerError, "encode payload failed")
		return
	}

	// 【关键修复】如果是流式响应，立即设置并发送响应头，防止下游超时
	if shouldStream && h.compat.EnableStreamCompat {
		c.Writer.Header().Set("Content-Type", "text/event-stream")
		c.Writer.Header().Set("Cache-Control", "no-cache")
		c.Writer.Header().Set("Connection", "keep-alive")
		c.Writer.Header().Set("X-Accel-Buffering", "no")
		c.Status(http.StatusOK)
		if flusher, ok := c.Writer.(http.Flusher); ok {
			flusher.Flush()
		}
		headersSent := true
		_ = headersSent // 标记已发送响应头
	}

	var resp *http.Response
	if shouldStream {
		resp, err = h.proxy.CallResponsesStream(c.Request.Context(), *inst, payload)
	} else {
		resp, err = h.proxy.CallResponsesJSON(c.Request.Context(), *inst, payload)
	}
	if err != nil {
		// 如果已经发送了响应头，无法再设置HTTP错误码，只能通过SSE发送错误
		if shouldStream && h.compat.EnableStreamCompat {
			// 发送SSE格式的错误事件
			msg := fmt.Sprintf("Gateway error: %v", err)
			errEvent, _ := json.Marshal(map[string]any{
				"error": map[string]any{
					"message": msg,
					"type":    "internal_error",
				},
			})
			c.Writer.Write([]byte("data: "))
			c.Writer.Write(errEvent)
			c.Writer.Write([]byte("\n\n"))
			c.Writer.Write([]byte("data: [DONE]\n\n"))
			if flusher, ok := c.Writer.(http.Flusher); ok {
				flusher.Flush()
			}
			return
		}
		h.respondGatewayError(c, err)
		return
	}
	defer resp.Body.Close()
	h.clearBlockingStatusOnLocalTestSuccess(c, inst, resp)

	if resp.StatusCode >= 400 {
		data, _ := io.ReadAll(resp.Body)
		ctype := resp.Header.Get("Content-Type")
		if ctype == "" {
			ctype = "application/json; charset=utf-8"
		}
		c.Data(resp.StatusCode, ctype, data)
		return
	}

	model := strings.TrimSpace(responsesReq.Model)
	if model == "" {
		model = strings.TrimSpace(chatReq.Model)
	}
	if model == "" {
		model = "gpt-5-codex"
	}

	reasoningMode := strings.ToLower(strings.TrimSpace(h.compat.ReasoningCompat))

	if shouldStream && h.compat.EnableStreamCompat {
		h.streamResponsesToChatChunks(c, resp, model, reasoningMode, "responses->chat.completion.chunk")
		h.appendLog(inst.LogPath, fmt.Sprintf("[%s] chat stream %s -> %d", time.Now().Format(time.RFC3339), c.Param("instanceID"), http.StatusOK))
		return
	}

	if !shouldStream {
		combined, err := h.aggregateResponsesResultToChat(resp, model, reasoningMode)
		if err != nil {
			c.String(http.StatusBadGateway, "aggregate error: %v", err)
			return
		}
		buf, err := json.Marshal(combined)
		if err != nil {
			c.String(http.StatusInternalServerError, "encode aggregate failed")
			return
		}
		c.Writer.Header().Set("Content-Type", "application/json; charset=utf-8")
		c.Writer.Header().Set("Content-Length", strconv.Itoa(len(buf)))
		c.Writer.Header().Set("X-Compat-Aggregate", "responses->chat.completion")
		c.Writer.Header().Set("X-Compat-Reasoning", reasoningMode)
		c.Status(http.StatusOK)
		if _, err := c.Writer.Write(buf); err != nil {
			c.Error(err)
		}
		h.appendLog(inst.LogPath, fmt.Sprintf("[%s] chat aggregate %s -> 200", time.Now().Format(time.RFC3339), c.Param("instanceID")))
		return
	}

	h.streamResponsesToChatChunks(c, resp, model, reasoningMode, "responses->chat.completion.chunk(fallback)")
	h.appendLog(inst.LogPath, fmt.Sprintf("[%s] chat fallback stream %s -> %d", time.Now().Format(time.RFC3339), c.Param("instanceID"), http.StatusOK))
}

func parseBoolQuery(u *url.URL, key string) bool {
	if u == nil {
		return false
	}
	val := strings.ToLower(strings.TrimSpace(u.Query().Get(key)))
	return val == "true" || val == "1" || val == "yes"
}

func (h *APIHandler) buildResponsesRequest(req *chatCompletionsRequest, stream bool) (responsesRequest, error) {
	if req == nil {
		return responsesRequest{}, errors.New("request is nil")
	}
	inputs, err := gatherMessages(req.Messages)
	if err != nil {
		return responsesRequest{}, err
	}
	respReq := responsesRequest{
		Model:          strings.TrimSpace(req.Model),
		Instructions:   "",
		Input:          inputs,
		Stream:         stream,
		Store:          false,
		PromptCacheKey: strings.TrimSpace(req.PromptCacheKey),
	}
	effort := strings.ToLower(strings.TrimSpace(h.compat.ReasoningEffort))
	if effort == "" {
		effort = "medium"
	}
	if req.Reasoning != nil && strings.TrimSpace(req.Reasoning.Effort) != "" {
		effort = strings.ToLower(strings.TrimSpace(req.Reasoning.Effort))
	} else if strings.TrimSpace(req.ReasoningEffort) != "" {
		effort = strings.ToLower(strings.TrimSpace(req.ReasoningEffort))
	}
	if effort == "low" || effort == "medium" || effort == "high" {
		respReq.Reasoning = &responsesReasoning{Effort: effort, Summary: "auto"}
		// 当存在 reasoning 控制时，对齐 Codex CLI：include 加密推理内容
		respReq.Include = append(respReq.Include, "reasoning.encrypted_content")
	} else {
		return responsesRequest{}, fmt.Errorf("unsupported reasoning effort: %q", effort)
	}
	if len(req.Tools) > 0 {
		respReq.Tools, err = normalizeChatTools(req.Tools)
		if err != nil {
			return responsesRequest{}, err
		}
	}
	if toolChoice, err := normalizeChatToolChoice(req.ToolChoice); err != nil {
		return responsesRequest{}, err
	} else if toolChoice != nil {
		respReq.ToolChoice = toolChoice
	}
	if req.ParallelToolCalls != nil {
		respReq.ParallelToolCalls = *req.ParallelToolCalls
	} else {
		respReq.ParallelToolCalls = true
	}
	if respReq.Model == "" {
		respReq.Model = "gpt-5-codex"
	}
	// 若调用方未提供 prompt_cache_key，则生成一个
	if strings.TrimSpace(respReq.PromptCacheKey) == "" {
		if id, err := newUUID(); err == nil {
			respReq.PromptCacheKey = id
		}
	}
	return respReq, nil
}

// newUUID 生成随机 UUID 字符串
func newUUID() (string, error) {
	// 使用 google/uuid 生成 v4
	v4, err := guuid.NewRandom()
	if err != nil {
		return "", err
	}
	return v4.String(), nil
}

func gatherMessages(messages []chatMessageInput) ([]responsesInput, error) {
	inputs := make([]responsesInput, 0, len(messages))
	toolCalls := make(map[string]struct{})
	for _, msg := range messages {
		role := strings.ToLower(strings.TrimSpace(msg.Role))
		switch role {
		case "system", "developer":
			content := flattenDeveloperContent(msg.Content)
			if len(content) > 0 {
				inputs = append(inputs, responsesInput{Type: "message", Role: "developer", Content: content})
			}
		case "user":
			content := flattenUserContent(msg.Content)
			if len(content) > 0 {
				inputs = append(inputs, responsesInput{Type: "message", Role: "user", Content: content})
			}
		case "assistant":
			content := flattenAssistantContent(msg.Content)
			if len(content) > 0 {
				inputs = append(inputs, responsesInput{Type: "message", Role: "assistant", Content: content})
			}
			for _, toolCall := range msg.ToolCalls {
				toolType := strings.ToLower(strings.TrimSpace(toolCall.Type))
				if toolType != "" && toolType != "function" {
					return nil, fmt.Errorf("unsupported tool call type: %q", toolCall.Type)
				}
				callID := strings.TrimSpace(toolCall.ID)
				if callID == "" {
					return nil, errors.New("assistant tool call id is empty")
				}
				if toolCall.Function == nil || strings.TrimSpace(toolCall.Function.Name) == "" {
					return nil, errors.New("assistant tool call function name is empty")
				}
				toolCalls[callID] = struct{}{}
				inputs = append(inputs, responsesInput{
					Type:      "function_call",
					CallID:    callID,
					Name:      strings.TrimSpace(toolCall.Function.Name),
					Arguments: toolCall.Function.Arguments,
				})
			}
		case "tool":
			callID := strings.TrimSpace(msg.ToolCallID)
			if callID == "" {
				return nil, errors.New("tool_call_id is required for tool messages")
			}
			if _, ok := toolCalls[callID]; !ok {
				return nil, fmt.Errorf("tool message references unknown tool_call_id: %s", callID)
			}
			inputs = append(inputs, responsesInput{
				Type:   "function_call_output",
				CallID: callID,
				Output: flattenToolContent(msg.Content),
			})
		case "":
			// ignore empty role
		default:
			return nil, fmt.Errorf("unsupported message role: %q", msg.Role)
		}
	}
	if len(inputs) == 0 {
		return nil, errors.New("messages is empty")
	}
	return inputs, nil
}

func flattenDeveloperContent(content interface{}) []responsesContent {
	switch v := content.(type) {
	case string:
		text := strings.TrimSpace(v)
		if text != "" {
			return []responsesContent{{Type: "input_text", Text: text}}
		}
	case []interface{}:
		parts := make([]responsesContent, 0, len(v))
		for _, item := range v {
			switch vv := item.(type) {
			case string:
				text := strings.TrimSpace(vv)
				if text != "" {
					parts = append(parts, responsesContent{Type: "input_text", Text: text})
				}
			case map[string]interface{}:
				if t := strings.ToLower(getMapString(vv, "type")); t == "text" || t == "input_text" {
					text := strings.TrimSpace(getMapString(vv, "text"))
					if text != "" {
						parts = append(parts, responsesContent{Type: "input_text", Text: text})
					}
				}
			}
		}
		return parts
	}
	return nil
}

func flattenUserContent(content interface{}) []responsesContent {
	switch v := content.(type) {
	case string:
		text := strings.TrimSpace(v)
		if text == "" {
			return nil
		}
		return []responsesContent{{Type: "input_text", Text: text}}
	case []interface{}:
		var out []responsesContent
		for _, item := range v {
			switch vv := item.(type) {
			case string:
				if strings.TrimSpace(vv) != "" {
					out = append(out, responsesContent{Type: "input_text", Text: vv})
				}
			case map[string]interface{}:
				typeStr := strings.ToLower(getMapString(vv, "type"))
				if typeStr == "text" {
					text := getMapString(vv, "text")
					if strings.TrimSpace(text) != "" {
						out = append(out, responsesContent{Type: "input_text", Text: text})
					}
				} else if typeStr == "image_url" {
					img := parseImageURL(vv["image_url"])
					if img != "" {
						out = append(out, responsesContent{Type: "input_image", ImageURL: img})
					}
				}
			}
		}
		return out
	default:
		return nil
	}
}

func flattenAssistantContent(content interface{}) []responsesContent {
	switch v := content.(type) {
	case string:
		text := strings.TrimSpace(v)
		if text == "" {
			return nil
		}
		return []responsesContent{{Type: "output_text", Text: text}}
	case []interface{}:
		var out []responsesContent
		for _, item := range v {
			switch vv := item.(type) {
			case string:
				if strings.TrimSpace(vv) != "" {
					out = append(out, responsesContent{Type: "output_text", Text: vv})
				}
			case map[string]interface{}:
				typeStr := strings.ToLower(getMapString(vv, "type"))
				if typeStr == "text" || typeStr == "output_text" {
					text := getMapString(vv, "text")
					if strings.TrimSpace(text) != "" {
						out = append(out, responsesContent{Type: "output_text", Text: text})
					}
				}
			}
		}
		return out
	default:
		return nil
	}
}

func flattenToolContent(content interface{}) string {
	switch v := content.(type) {
	case string:
		return strings.TrimSpace(v)
	case []interface{}:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			switch value := item.(type) {
			case string:
				text := strings.TrimSpace(value)
				if text != "" {
					parts = append(parts, text)
				}
			case map[string]interface{}:
				itemType := strings.ToLower(strings.TrimSpace(getMapString(value, "type")))
				switch itemType {
				case "text", "input_text", "output_text":
					text := strings.TrimSpace(getMapString(value, "text"))
					if text != "" {
						parts = append(parts, text)
					}
				default:
					raw := strings.TrimSpace(getStringFromRaw(value))
					if raw != "" {
						parts = append(parts, raw)
					}
				}
			default:
				raw := strings.TrimSpace(getStringFromRaw(value))
				if raw != "" {
					parts = append(parts, raw)
				}
			}
		}
		return strings.TrimSpace(strings.Join(parts, "\n"))
	case map[string]interface{}:
		if text := strings.TrimSpace(getMapString(v, "text")); text != "" {
			return text
		}
		return strings.TrimSpace(getStringFromRaw(v))
	default:
		return strings.TrimSpace(getStringFromRaw(content))
	}
}

func normalizeChatTools(tools []interface{}) ([]interface{}, error) {
	if len(tools) == 0 {
		return nil, nil
	}
	normalized := make([]interface{}, 0, len(tools))
	for _, tool := range tools {
		toolMap, ok := tool.(map[string]interface{})
		if !ok {
			normalized = append(normalized, tool)
			continue
		}
		toolType := strings.ToLower(strings.TrimSpace(getMapString(toolMap, "type")))
		if toolType != "function" {
			normalized = append(normalized, tool)
			continue
		}
		function := getMap(toolMap, "function")
		next := map[string]interface{}{
			"type": "function",
		}
		name := strings.TrimSpace(getMapString(toolMap, "name"))
		if name == "" && function != nil {
			name = strings.TrimSpace(getMapString(function, "name"))
		}
		if name == "" {
			return nil, errors.New("function tool name is empty")
		}
		next["name"] = name
		description := strings.TrimSpace(getMapString(toolMap, "description"))
		if description == "" && function != nil {
			description = strings.TrimSpace(getMapString(function, "description"))
		}
		if description != "" {
			next["description"] = description
		}
		if parameters, ok := toolMap["parameters"]; ok {
			next["parameters"] = parameters
		} else if function != nil {
			if parameters, ok := function["parameters"]; ok {
				next["parameters"] = parameters
			}
		}
		if strict, ok := toolMap["strict"]; ok {
			next["strict"] = strict
		} else if function != nil {
			if strict, ok := function["strict"]; ok {
				next["strict"] = strict
			}
		}
		normalized = append(normalized, next)
	}
	return normalized, nil
}

func normalizeChatToolChoice(choice interface{}) (interface{}, error) {
	switch v := choice.(type) {
	case nil:
		return nil, nil
	case string:
		v = strings.TrimSpace(v)
		if v == "" {
			return nil, nil
		}
		return v, nil
	case map[string]interface{}:
		if len(v) == 0 {
			return nil, nil
		}
		toolType := strings.ToLower(strings.TrimSpace(getMapString(v, "type")))
		if toolType != "function" {
			return v, nil
		}
		name := strings.TrimSpace(getMapString(v, "name"))
		if name == "" {
			function := getMap(v, "function")
			if function != nil {
				name = strings.TrimSpace(getMapString(function, "name"))
			}
		}
		if name == "" {
			return nil, errors.New("function tool_choice name is empty")
		}
		return map[string]interface{}{
			"type": "function",
			"name": name,
		}, nil
	default:
		return v, nil
	}
}

func parseImageURL(raw interface{}) string {
	switch v := raw.(type) {
	case string:
		return strings.TrimSpace(v)
	case map[string]interface{}:
		return strings.TrimSpace(getMapString(v, "url"))
	default:
		return ""
	}
}
