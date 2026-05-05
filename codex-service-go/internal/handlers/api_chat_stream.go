package handlers

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func (h *APIHandler) streamResponsesToChatChunks(c *gin.Context, resp *http.Response, model, reasoningMode, transform string) {
	writer := c.Writer
	flusher, ok := writer.(http.Flusher)
	if !ok {
		c.String(http.StatusInternalServerError, "streaming not supported")
		return
	}
	headers := writer.Header()
	headers.Set("Content-Type", "text/event-stream")
	headers.Set("Cache-Control", "no-cache")
	headers.Set("Connection", "keep-alive")
	// Disable nginx proxy buffering for SSE.
	headers.Set("X-Accel-Buffering", "no")
	headers.Set("X-Compat-Transform", transform)
	headers.Set("X-Compat-Reasoning", reasoningMode)
	c.Status(http.StatusOK)

	wantThinkTags := reasoningMode == "think-tags" || reasoningMode == "both"
	wantReasonField := reasoningMode == "reasoning" || reasoningMode == "both"

	responseID := "chatcmpl-stream"
	roleSent := false
	thinkOpen := false
	toolCallIndex := 0
	hadOutputText := false
	// /responses 的 SSE 可能同时包含 response.output_text.delta（增量）和 response.output_text.done（完整文本）。
	// 若将 done.text 继续作为增量输出，会导致下游把完整文本再追加一次，出现“回答两遍”。
	seenOutputTextDelta := make(map[string]bool)

	scanner := bufio.NewScanner(resp.Body)
	scanner.Split(splitSSE)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	outputTextKey := func(event map[string]interface{}) string {
		itemID := strings.TrimSpace(getMapString(event, "item_id"))
		if itemID == "" {
			itemID = strings.TrimSpace(getMapString(getMap(event, "response"), "id"))
		}
		if itemID == "" {
			itemID = "output_text"
		}
		return itemID + ":" + strconv.Itoa(getMapInt(event, "output_index")) + ":" + strconv.Itoa(getMapInt(event, "content_index"))
	}

	writeChunk := func(delta chatChunkDelta, finish *string) {
		chunk := chatCompletionChunk{
			ID:      responseID,
			Object:  "chat.completion.chunk",
			Created: time.Now().Unix(),
			Model:   model,
			Choices: []chatChunkChoice{{
				Index:        0,
				Delta:        delta,
				FinishReason: finish,
			}},
		}
		payload, err := json.Marshal(chunk)
		if err != nil {
			return
		}
		_, _ = writer.Write(append(append([]byte("data: "), payload...), []byte("\n\n")...))
		flusher.Flush()
	}

	sendRole := func() {
		if roleSent {
			return
		}
		writeChunk(chatChunkDelta{Role: "assistant"}, nil)
		roleSent = true
	}

	sendContent := func(text string) {
		if text == "" {
			return
		}
		sendRole()
		writeChunk(chatChunkDelta{Content: text}, nil)
	}

	sendReason := func(text string) {
		if text == "" {
			return
		}
		if wantThinkTags {
			if !thinkOpen {
				sendContent("<think>")
				thinkOpen = true
			}
			sendContent(text)
		}
		if wantReasonField {
			sendRole()
			writeChunk(chatChunkDelta{Reasoning: text}, nil)
		}
	}

	sendTool := func(call chatToolCallDelta) {
		sendRole()
		writeChunk(chatChunkDelta{ToolCalls: []chatToolCallDelta{call}}, nil)
	}

	for scanner.Scan() {
		raw := strings.TrimSpace(scanner.Text())
		if raw == "" {
			continue
		}
		event := parseSSEPayload(raw)
		if event == nil {
			continue
		}
		if id := getMapString(getMap(event, "response"), "id"); id != "" {
			responseID = id
		}
		typeStr := strings.ToLower(getMapString(event, "type"))
		// Emit the initial "assistant" role chunk as soon as we see response metadata.
		// This helps downstream clients/proxies treat the connection as a streaming SSE response.
		if typeStr == "response.created" {
			sendRole()
			continue
		}
		if deltaStr, ok := event["delta"].(string); ok && strings.HasSuffix(typeStr, ".delta") {
			if deltaStr != "" {
				if typeStr == "response.output_text.delta" {
					hadOutputText = true
					seenOutputTextDelta[outputTextKey(event)] = true
				}
				sendContent(deltaStr)
			}
			continue
		}
		if typeStr == "response.output_text.delta" {
			text := getStringFromRaw(event["delta"])
			if text != "" {
				hadOutputText = true
				seenOutputTextDelta[outputTextKey(event)] = true
				sendContent(text)
			}
			continue
		}
		if typeStr == "response.output_text.done" {
			if seenOutputTextDelta[outputTextKey(event)] {
				continue
			}
			text := getMapString(event, "text")
			if text != "" {
				hadOutputText = true
				sendContent(text)
			}
			continue
		}
		if strings.Contains(typeStr, "reasoning") {
			reason := getStringFromRaw(event["delta"])
			if reason == "" {
				reason = getStringFromRaw(event["summary"])
			}
			sendReason(reason)
			continue
		}
		if typeStr == "response.output_item.done" {
			item := getMap(event, "item")
			if strings.ToLower(getMapString(item, "type")) == "message" && !hadOutputText {
				// Some upstreams only emit a final message item (no output_text deltas/done).
				segments, ok := item["content"].([]interface{})
				if ok {
					var builder strings.Builder
					for _, seg := range segments {
						segMap, ok := seg.(map[string]interface{})
						if !ok {
							continue
						}
						if strings.ToLower(getMapString(segMap, "type")) != "output_text" {
							continue
						}
						builder.WriteString(getMapString(segMap, "text"))
					}
					if builder.Len() > 0 {
						hadOutputText = true
						sendContent(builder.String())
					}
				}
				continue
			}
			if strings.ToLower(getMapString(item, "type")) == "function_call" {
				call := chatToolCallDelta{
					Index: toolCallIndex,
					ID:    getMapString(item, "call_id"),
					Type:  "function",
					Function: &chatToolFunction{
						Name:      getMapString(item, "name"),
						Arguments: getStringFromRaw(item["arguments"]),
					},
				}
				toolCallIndex++
				sendTool(call)
			}
			continue
		}
		if typeStr == "response.completed" {
			if wantThinkTags && thinkOpen {
				sendContent("</think>")
				thinkOpen = false
			}
			finish := "stop"
			writeChunk(chatChunkDelta{}, &finish)
			_, _ = writer.Write([]byte("data: [DONE]\n\n"))
			flusher.Flush()
			return
		}
	}
	if err := scanner.Err(); err != nil {
		c.Error(err)
	}
	if wantThinkTags && thinkOpen {
		sendContent("</think>")
	}
	finish := "stop"
	writeChunk(chatChunkDelta{}, &finish)
	_, _ = writer.Write([]byte("data: [DONE]\n\n"))
	flusher.Flush()
}

func splitSSE(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	// Support both LF and CRLF event separators.
	lf := bytes.Index(data, []byte("\n\n"))
	crlf := bytes.Index(data, []byte("\r\n\r\n"))
	if lf >= 0 && (crlf < 0 || lf < crlf) {
		return lf + 2, data[:lf], nil
	}
	if crlf >= 0 {
		return crlf + 4, data[:crlf], nil
	}
	if atEOF {
		return len(data), data, nil
	}
	return 0, nil, nil
}

func (h *APIHandler) aggregateResponsesToChat(resp *http.Response, model, reasoningMode string) (*chatCompletionsResponse, error) {
	scanner := bufio.NewScanner(resp.Body)
	scanner.Split(splitSSE)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	var (
		textBuilder         strings.Builder
		thinkBuilder        strings.Builder
		thinkOpen           bool
		callList            []chatToolCall
		usageSummary        *responsesUsage
		responseID          = "chatcmpl"
		lastResponse        map[string]interface{}
		wantThink           = reasoningMode == "think-tags" || reasoningMode == "both"
		hadOutputText       bool
		seenOutputTextDelta = make(map[string]bool)
	)

	outputTextKey := func(event map[string]interface{}) string {
		itemID := strings.TrimSpace(getMapString(event, "item_id"))
		if itemID == "" {
			itemID = strings.TrimSpace(getMapString(getMap(event, "response"), "id"))
		}
		if itemID == "" {
			itemID = "output_text"
		}
		return itemID + ":" + strconv.Itoa(getMapInt(event, "output_index")) + ":" + strconv.Itoa(getMapInt(event, "content_index"))
	}

	for scanner.Scan() {
		raw := strings.TrimSpace(scanner.Text())
		if raw == "" {
			continue
		}
		event := parseSSEPayload(raw)
		if event == nil {
			continue
		}
		if id := getMapString(getMap(event, "response"), "id"); id != "" {
			responseID = id
		}
		typeStr := strings.ToLower(getMapString(event, "type"))
		if deltaStr, ok := event["delta"].(string); ok && strings.HasSuffix(typeStr, ".delta") {
			if deltaStr != "" {
				if typeStr == "response.output_text.delta" {
					hadOutputText = true
					seenOutputTextDelta[outputTextKey(event)] = true
				}
				textBuilder.WriteString(deltaStr)
			}
			continue
		}
		if typeStr == "response.output_text.delta" {
			text := getStringFromRaw(event["delta"])
			if text != "" {
				hadOutputText = true
				seenOutputTextDelta[outputTextKey(event)] = true
				textBuilder.WriteString(text)
			}
			continue
		}
		if typeStr == "response.output_text.done" {
			if seenOutputTextDelta[outputTextKey(event)] {
				continue
			}
			text := getMapString(event, "text")
			if text != "" {
				hadOutputText = true
				textBuilder.WriteString(text)
			}
			continue
		}
		if strings.Contains(typeStr, "reasoning") && wantThink {
			reason := getStringFromRaw(event["delta"])
			if reason == "" {
				reason = getStringFromRaw(event["summary"])
			}
			if reason != "" {
				if !thinkOpen {
					thinkBuilder.WriteString("<think>")
					thinkOpen = true
				}
				thinkBuilder.WriteString(reason)
			}
			continue
		}
		if typeStr == "response.output_item.done" {
			item := getMap(event, "item")
			if strings.EqualFold(getMapString(item, "type"), "message") && !hadOutputText {
				segments, ok := item["content"].([]interface{})
				if ok {
					for _, seg := range segments {
						segMap, ok := seg.(map[string]interface{})
						if !ok {
							continue
						}
						if strings.ToLower(getMapString(segMap, "type")) != "output_text" {
							continue
						}
						text := getMapString(segMap, "text")
						if text == "" {
							continue
						}
						hadOutputText = true
						textBuilder.WriteString(text)
					}
				}
				continue
			}
			if strings.EqualFold(getMapString(item, "type"), "function_call") {
				call := chatToolCall{
					ID:   getMapString(item, "call_id"),
					Type: "function",
					Function: &chatToolFunction{
						Name:      getMapString(item, "name"),
						Arguments: getStringFromRaw(item["arguments"]),
					},
				}
				callList = append(callList, call)
			}
			continue
		}
		if typeStr == "response.completed" {
			usageSummary = mergeUsage(usageSummary, extractUsageFromSSE(event))
			if respMap := getMap(event, "response"); respMap != nil {
				lastResponse = respMap
			}
			if wantThink && thinkOpen {
				thinkBuilder.WriteString("</think>")
				thinkOpen = false
			}
			continue
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if !hadOutputText && lastResponse != nil {
		if fallback := extractTextFromResponse(lastResponse); fallback != "" {
			hadOutputText = true
			textBuilder.WriteString(fallback)
		}
	}
	if wantThink && thinkOpen {
		thinkBuilder.WriteString("</think>")
		thinkOpen = false
	}
	return buildAggregatedChatResponse(
		responseID,
		model,
		reasoningMode,
		textBuilder.String(),
		thinkBuilder.String(),
		callList,
		usageSummary,
	), nil
}

func (h *APIHandler) aggregateResponsesResultToChat(resp *http.Response, model, reasoningMode string) (*chatCompletionsResponse, error) {
	contentType := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Type")))
	if strings.Contains(contentType, "application/json") {
		return h.aggregateResponsesJSONToChat(resp, model, reasoningMode)
	}
	return h.aggregateResponsesToChat(resp, model, reasoningMode)
}

func (h *APIHandler) aggregateResponsesJSONToChat(resp *http.Response, model, reasoningMode string) (*chatCompletionsResponse, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var response map[string]interface{}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}

	responseID := strings.TrimSpace(getMapString(response, "id"))
	if responseID == "" {
		responseID = "chatcmpl"
	}

	var (
		textBuilder  strings.Builder
		thinkBuilder strings.Builder
		callList     []chatToolCall
		wantThink    = reasoningMode == "think-tags" || reasoningMode == "both"
	)

	outputs, _ := response["output"].([]interface{})
	for _, itemAny := range outputs {
		item, ok := itemAny.(map[string]interface{})
		if !ok {
			continue
		}

		switch strings.ToLower(getMapString(item, "type")) {
		case "reasoning":
			if !wantThink {
				continue
			}
			reason := extractReasoningSummaryText(item["summary"])
			if reason == "" {
				reason = extractReasoningSummaryText(item["content"])
			}
			if reason == "" {
				continue
			}
			if thinkBuilder.Len() == 0 {
				thinkBuilder.WriteString("<think>")
			}
			thinkBuilder.WriteString(reason)
		case "message":
			segments, _ := item["content"].([]interface{})
			for _, segAny := range segments {
				segMap, ok := segAny.(map[string]interface{})
				if !ok {
					continue
				}
				switch strings.ToLower(getMapString(segMap, "type")) {
				case "output_text", "text":
					if text := getMapString(segMap, "text"); text != "" {
						textBuilder.WriteString(text)
					}
				case "output_refusal", "refusal":
					if refusal := getMapString(segMap, "refusal"); refusal != "" {
						textBuilder.WriteString(refusal)
					}
				}
			}
		case "function_call":
			call := chatToolCall{
				ID:   getMapString(item, "call_id"),
				Type: "function",
				Function: &chatToolFunction{
					Name:      getMapString(item, "name"),
					Arguments: getStringFromRaw(item["arguments"]),
				},
			}
			callList = append(callList, call)
		}
	}

	if wantThink && thinkBuilder.Len() > 0 {
		thinkBuilder.WriteString("</think>")
	}

	return buildAggregatedChatResponse(
		responseID,
		model,
		reasoningMode,
		textBuilder.String(),
		thinkBuilder.String(),
		callList,
		extractUsageFromResponse(response),
	), nil
}

func buildAggregatedChatResponse(responseID, model, reasoningMode, responseText, thinkText string, callList []chatToolCall, usageSummary *responsesUsage) *chatCompletionsResponse {
	wantThink := reasoningMode == "think-tags" || reasoningMode == "both"
	wantReason := reasoningMode == "reasoning" || reasoningMode == "both"

	contentText := responseText
	if wantThink {
		contentText = thinkText + contentText
	}

	var contentPtr *string
	if contentText != "" {
		contentPtr = &contentText
	}

	message := chatMessage{Role: "assistant", Content: contentPtr}
	if wantReason {
		reason := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(thinkText, "<think>"), "</think>"))
		if reason != "" {
			message.Reasoning = reason
		}
	}
	if len(callList) > 0 {
		message.ToolCalls = callList
	}

	var usage *chatUsage
	if usageSummary != nil {
		usage = &chatUsage{
			PromptTokens:     usageSummary.InputTokens,
			CompletionTokens: usageSummary.OutputTokens,
			TotalTokens:      usageSummary.TotalTokens,
		}
	}

	return &chatCompletionsResponse{
		ID:      responseID,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []chatChoice{{
			Index:        0,
			Message:      message,
			FinishReason: "stop",
		}},
		Usage: usage,
	}
}

func mergeUsage(current, incoming *responsesUsage) *responsesUsage {
	if incoming == nil {
		return current
	}
	if current == nil {
		return incoming
	}
	if incoming.InputTokens != 0 {
		current.InputTokens = incoming.InputTokens
	}
	if incoming.OutputTokens != 0 {
		current.OutputTokens = incoming.OutputTokens
	}
	if incoming.TotalTokens != 0 {
		current.TotalTokens = incoming.TotalTokens
	}
	return current
}

func extractUsageFromSSE(event map[string]interface{}) *responsesUsage {
	if event == nil {
		return nil
	}
	usage := getMap(getMap(event, "response"), "usage")
	if usage == nil {
		usage = getMap(event, "usage")
	}
	if usage == nil {
		return nil
	}
	return &responsesUsage{
		InputTokens:  getMapInt(usage, "input_tokens"),
		OutputTokens: getMapInt(usage, "output_tokens"),
		TotalTokens:  getMapInt(usage, "total_tokens"),
	}
}

func extractUsageFromResponse(resp map[string]interface{}) *responsesUsage {
	return extractUsageFromSSE(map[string]interface{}{"response": resp})
}

func extractTextFromResponse(resp map[string]interface{}) string {
	outputs, ok := resp["output"].([]interface{})
	if !ok {
		return ""
	}
	var builder strings.Builder
	for _, item := range outputs {
		node, ok := item.(map[string]interface{})
		if !ok || strings.ToLower(getMapString(node, "type")) != "message" {
			continue
		}
		segments, ok := node["content"].([]interface{})
		if !ok {
			continue
		}
		for _, seg := range segments {
			segMap, ok := seg.(map[string]interface{})
			if !ok {
				continue
			}
			if strings.ToLower(getMapString(segMap, "type")) == "output_text" {
				builder.WriteString(getMapString(segMap, "text"))
			}
		}
	}
	return builder.String()
}

func extractReasoningSummaryText(value any) string {
	if item, ok := value.(map[string]interface{}); ok {
		if text := strings.TrimSpace(getMapString(item, "text")); text != "" {
			return text
		}
	}
	if items, ok := value.([]interface{}); ok {
		parts := make([]string, 0, len(items))
		for _, itemAny := range items {
			item, ok := itemAny.(map[string]interface{})
			if !ok {
				continue
			}
			if text := strings.TrimSpace(getMapString(item, "text")); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "\n")
	}
	return strings.TrimSpace(getStringFromRaw(value))
}

func parseSSEPayload(raw string) map[string]interface{} {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	lines := strings.Split(raw, "\n")
	var dataLines []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(line[5:]))
		}
	}
	if len(dataLines) == 0 {
		return nil
	}
	joined := strings.Join(dataLines, "\n")
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(joined), &obj); err != nil {
		return nil
	}
	return obj
}

func getMap(m map[string]interface{}, key string) map[string]interface{} {
	if m == nil {
		return nil
	}
	if val, ok := m[key]; ok {
		if mm, ok := val.(map[string]interface{}); ok {
			return mm
		}
	}
	return nil
}

func getMapString(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	if val, ok := m[key]; ok {
		return getStringFromRaw(val)
	}
	return ""
}

func getMapInt(m map[string]interface{}, key string) int {
	if m == nil {
		return 0
	}
	val, ok := m[key]
	if !ok {
		return 0
	}
	switch v := val.(type) {
	case float64:
		return int(math.Round(v))
	case float32:
		return int(math.Round(float64(v)))
	case int:
		return v
	case int64:
		return int(v)
	case json.Number:
		i, _ := v.Int64()
		return int(i)
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err == nil {
			return n
		}
	}
	return 0
}

func getStringFromRaw(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case []byte:
		return string(val)
	case json.Number:
		return val.String()
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(val), 'f', -1, 64)
	case int:
		return strconv.Itoa(val)
	case int64:
		return strconv.FormatInt(val, 10)
	case map[string]interface{}:
		if data, err := json.Marshal(val); err == nil {
			return string(data)
		}
	case []interface{}:
		if data, err := json.Marshal(val); err == nil {
			return string(data)
		}
	}
	return ""
}
