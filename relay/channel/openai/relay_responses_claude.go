package openai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"one-api/common"
	"one-api/constant"
	"one-api/dto"
	"one-api/logger"
	relaycommon "one-api/relay/common"
	oai_cc "one-api/relay/compat/oai_cc"
	"one-api/relay/helper"
	"one-api/service"
	"one-api/types"
	"strings"

	"github.com/google/uuid"

	"github.com/gin-gonic/gin"
)

type openAIResponsesRaw struct {
	ID                string            `json:"id"`
	Status            string            `json:"status"`
	Model             string            `json:"model"`
	Output            []json.RawMessage `json:"output"`
	Tools             []map[string]any  `json:"tools"`
	Usage             *dto.Usage        `json:"usage"`
	Error             any               `json:"error,omitempty"`
	IncompleteDetails *struct {
		Reason string `json:"reason"`
	} `json:"incomplete_details,omitempty"`
}

type oaiCcClaudeUsage struct {
	InputTokens   int                      `json:"input_tokens"`
	OutputTokens  int                      `json:"output_tokens"`
	ServerToolUse *dto.ClaudeServerToolUse `json:"server_tool_use,omitempty"`
	oai_cc.UsageCache
	InferenceGeo string `json:"inference_geo,omitempty"`
}

type oaiCcClaudeResponseTextBlock struct {
	Type      string `json:"type"`
	Text      string `json:"text"`
	Citations []any  `json:"citations"`
}

type oaiCcClaudeResponseThinkingBlock struct {
	Type      string `json:"type"`
	Thinking  string `json:"thinking"`
	Signature string `json:"signature"`
}

type oaiCcClaudeResponseToolUseBlock struct {
	Type  string `json:"type"`
	Id    string `json:"id"`
	Name  string `json:"name"`
	Input any    `json:"input"`
}

type oaiCcClaudeResponse struct {
	Id           string  `json:"id"`
	Type         string  `json:"type"`
	Role         string  `json:"role"`
	Content      []any   `json:"content"`
	Model        string  `json:"model"`
	StopReason   string  `json:"stop_reason"`
	StopSequence *string `json:"stop_sequence"`
	Usage        any     `json:"usage"`
}

type oaiCcClaudeStreamMessage struct {
	Id           string  `json:"id"`
	Type         string  `json:"type"`
	Role         string  `json:"role"`
	Content      []any   `json:"content"`
	Model        string  `json:"model"`
	StopReason   *string `json:"stop_reason"`
	StopSequence *string `json:"stop_sequence"`
	Usage        any     `json:"usage"`
}

type oaiCcClaudeStreamMessageStart struct {
	Type    string                   `json:"type"`
	Message oaiCcClaudeStreamMessage `json:"message"`
}

type oaiCcClaudeStreamContentBlockStart struct {
	Type         string `json:"type"`
	Index        int    `json:"index"`
	ContentBlock any    `json:"content_block"`
}

type oaiCcClaudeStreamContentBlockDelta struct {
	Type  string `json:"type"`
	Index int    `json:"index"`
	Delta any    `json:"delta"`
}

type oaiCcClaudeStreamContentBlockStop struct {
	Type  string `json:"type"`
	Index int    `json:"index"`
}

type oaiCcClaudeStreamMessageDeltaDelta struct {
	StopReason   string  `json:"stop_reason"`
	StopSequence *string `json:"stop_sequence"`
}

type oaiCcClaudeStreamMessageDelta struct {
	Type  string                             `json:"type"`
	Delta oaiCcClaudeStreamMessageDeltaDelta `json:"delta"`
	Usage any                                `json:"usage"`
}

type oaiCcClaudeStreamMessageStop struct {
	Type string `json:"type"`
}

type oaiCcClaudeStreamErrorEvent struct {
	Type  string            `json:"type"`
	Error types.ClaudeError `json:"error"`
}

type claudeSseOut struct {
	EventType string
	Payload   any
}

func OaiResponsesToClaudeHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}

	if isEventStreamResponse(resp.Header.Get("Content-Type"), responseBody) {
		final, err := extractFinalResponsesJSONFromEventStream(responseBody)
		if err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}
		responseBody = final
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
		resp = &http.Response{StatusCode: resp.StatusCode, Header: hdr}
	}

	var raw openAIResponsesRaw
	if err := common.Unmarshal(responseBody, &raw); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if oaiError := dto.GetOpenAIError(raw.Error); oaiError != nil && strings.TrimSpace(oaiError.Type) != "" {
		return nil, types.WithOpenAIError(*oaiError, resp.StatusCode)
	}

	claudeModel := info.OriginModelName
	claudeResp := translateResponsesToClaudeMessageWithToolNameMap(raw, claudeModel, responsesToolNameMapFromRelayInfo(info))
	webSearchUsage := buildClaudeWebSearchUsage(recordClaudeWebSearchRequestsFromContent(info, claudeResp.Content))
	recordResponsesNonWebSearchToolCalls(info, raw.Tools)
	upstreamInputTokens, upstreamOutputTokens, upstreamCachedTokens := usageIntsFromResponses(raw.Usage)

	if info != nil && info.OaiCcUsage != nil {
		totalInputTokens := upstreamInputTokens
		if totalInputTokens <= 0 {
			totalInputTokens = info.OaiCcUsage.LocalTotalInputTokens
		}
		uncached, cache := info.OaiCcUsage.FinalUsage(totalInputTokens, upstreamOutputTokens)
		claudeResp.Usage = oaiCcClaudeUsage{
			InputTokens:   uncached,
			OutputTokens:  upstreamOutputTokens,
			UsageCache:    cache,
			ServerToolUse: webSearchUsage,
			InferenceGeo:  "not_available",
		}
	}
	claudeRespBytes, err := common.Marshal(claudeResp)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}

	service.IOCopyBytesGracefully(c, resp, claudeRespBytes)

	uncachedBillingInput := upstreamInputTokens - upstreamCachedTokens
	if uncachedBillingInput < 0 {
		uncachedBillingInput = 0
	}

	billingPromptTokens := uncachedBillingInput
	billingCacheReadTokens := upstreamCachedTokens
	billingCacheCreationTokens := 0
	billingCacheCreation5mTokens := 0
	billingCacheCreation1hTokens := 0
	if info != nil && info.OaiCcUsage != nil {
		totalInputTokens := upstreamInputTokens
		if totalInputTokens <= 0 {
			totalInputTokens = info.OaiCcUsage.LocalTotalInputTokens
		}
		uncached, cache := info.OaiCcUsage.FinalUsage(totalInputTokens, upstreamOutputTokens)
		billingPromptTokens = uncached
		billingCacheReadTokens = cache.CacheReadInputTokens
		billingCacheCreationTokens = cache.CacheCreationInputTokens
		billingCacheCreation5mTokens = cache.ClaudeCacheCreation5m
		billingCacheCreation1hTokens = cache.ClaudeCacheCreation1h
	}
	usage := &dto.Usage{
		PromptTokens:     billingPromptTokens,
		CompletionTokens: upstreamOutputTokens,
		TotalTokens:      billingPromptTokens + upstreamOutputTokens,
	}
	usage.PromptTokensDetails.CachedTokens = billingCacheReadTokens
	usage.PromptTokensDetails.CachedCreationTokens = billingCacheCreationTokens
	usage.ClaudeCacheCreation5mTokens = billingCacheCreation5mTokens
	usage.ClaudeCacheCreation1hTokens = billingCacheCreation1hTokens
	return usage, nil
}

func OaiResponsesToClaudeStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		logger.LogError(c, "invalid response or response body")
		return nil, types.NewOpenAIError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}

	defer service.CloseResponseBodyGracefully(resp)

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		newAPIError := service.RelayErrorHandler(c.Request.Context(), resp, false)
		if newAPIError == nil {
			newAPIError = types.InitOpenAIError(types.ErrorCodeBadResponseStatusCode, resp.StatusCode)
		}
		return nil, newAPIError
	}

	usage := &dto.Usage{}
	translator := newResponsesToClaudeStreamTranslatorWithToolNameMap(info.OriginModelName, info.OaiCcUsage, responsesToolNameMapFromRelayInfo(info))

	helper.StreamScannerHandler(c, resp, info, func(data string) bool {
		data = strings.TrimSpace(data)
		if data == "" {
			return true
		}

		var payload map[string]any
		if err := common.UnmarshalJsonStr(data, &payload); err != nil {
			logger.LogError(c, "failed to unmarshal responses stream payload: "+err.Error())
			if common.GetContextKeyString(c, constant.ContextKeyStreamExitReason) == "" {
				common.SetContextKey(c, constant.ContextKeyStreamExitReason, "bad_stream_data")
				common.SetContextKey(c, constant.ContextKeyStreamExitError, err.Error())
			}
			return false
		}

		// Align with openai-claude-main: prefer SSE `event:` name, and fallback to JSON `type`.
		// Some upstreams omit the `type` field in `data:` because it duplicates the SSE event name.
		eventType := strings.TrimSpace(common.GetContextKeyString(c, constant.ContextKeyUpstreamSSEEvent))
		if eventType == "" {
			eventType = strings.TrimSpace(common.Interface2String(payload["type"]))
		}
		if eventType == "" {
			// Some upstreams emit nested errors without a `type`.
			if _, ok := payload["error"]; ok {
				translator.markError()
				for _, out := range translator.processErrorEvent(payload) {
					if err := writeOaiCcClaudeSseEvent(c, out.EventType, out.Payload); err != nil {
						logger.LogError(c, "failed to write claude sse event: "+err.Error())
						return false
					}
				}
			}
			return true
		}

		outs, finalUsage, shouldContinue := translator.processEvent(eventType, payload)
		if eventType == "response.completed" || eventType == "response.incomplete" || eventType == "response.done" {
			if response, ok := payload["response"].(map[string]any); ok && response != nil {
				recordResponsesNonWebSearchToolCallsAny(info, response["tools"])
			}
		}
		for _, out := range outs {
			if err := writeOaiCcClaudeSseEvent(c, out.EventType, out.Payload); err != nil {
				logger.LogError(c, "failed to write claude sse event: "+err.Error())
				return false
			}
		}
		if finalUsage != nil {
			*usage = *finalUsage
		}
		if !shouldContinue && common.GetContextKeyString(c, constant.ContextKeyStreamExitReason) == "" {
			switch eventType {
			case "response.completed", "response.done":
				common.SetContextKey(c, constant.ContextKeyStreamExitReason, "done")
			case "response.incomplete":
				common.SetContextKey(c, constant.ContextKeyStreamExitReason, "incomplete")
			case "response.failed":
				common.SetContextKey(c, constant.ContextKeyStreamExitReason, "failed")
			}
		}
		return shouldContinue
	})

	recordClaudeWebSearchRequests(info, translator.webSearchRequests)
	streamExitReason := common.GetContextKeyString(c, constant.ContextKeyStreamExitReason)
	if finalOut, finalUsage := translator.finalizeIncomplete(streamExitReason); len(finalOut) > 0 {
		for _, out := range finalOut {
			if err := writeOaiCcClaudeSseEvent(c, out.EventType, out.Payload); err != nil {
				logger.LogError(c, "failed to write synthetic claude sse event: "+err.Error())
				break
			}
		}
		if finalUsage != nil {
			*usage = *finalUsage
		}
		if streamExitReason == "" {
			common.SetContextKey(c, constant.ContextKeyStreamExitReason, "done")
		}
	}

	if streamErr := helper.BuildStreamExitError(c, info); streamErr != nil {
		return nil, streamErr
	}

	if usage.CompletionTokens == 0 && usage.PromptTokens == 0 {
		// Best-effort fallback: keep the accounting consistent even when upstream doesn't emit usage.
		if info != nil && info.OaiCcUsage != nil {
			totalInputTokens := info.OaiCcUsage.LocalTotalInputTokens
			if totalInputTokens <= 0 {
				totalInputTokens = info.PromptTokens
			}
			uncached, cache := info.OaiCcUsage.FinalUsage(totalInputTokens, 0)
			usage.PromptTokens = uncached
			usage.PromptTokensDetails.CachedTokens = cache.CacheReadInputTokens
			usage.PromptTokensDetails.CachedCreationTokens = cache.CacheCreationInputTokens
			usage.ClaudeCacheCreation5mTokens = cache.ClaudeCacheCreation5m
			usage.ClaudeCacheCreation1hTokens = cache.ClaudeCacheCreation1h
			usage.TotalTokens = usage.PromptTokens
		} else {
			usage.PromptTokens = info.PromptTokens
			usage.TotalTokens = usage.PromptTokens
		}
	} else if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}

	return usage, nil
}

func translateResponsesToClaudeMessage(raw openAIResponsesRaw, claudeModel string) *oaiCcClaudeResponse {
	return translateResponsesToClaudeMessageWithToolNameMap(raw, claudeModel, nil)
}

func translateResponsesToClaudeMessageWithToolNameMap(raw openAIResponsesRaw, claudeModel string, normalizedToolNameMap map[string]string) *oaiCcClaudeResponse {
	contentBlocks := make([]any, 0)
	hasFunctionCall := false
	hasRefusal := false
	webSearchRequests := 0

	pushText := func(text any, annotations any) {
		t := common.Interface2String(text)
		citations := make([]any, 0)
		if arr, ok := annotations.([]any); ok {
			citations = arr
		}
		contentBlocks = append(contentBlocks, oaiCcClaudeResponseTextBlock{
			Type:      "text",
			Text:      t,
			Citations: citations,
		})
	}

	for _, itemRaw := range raw.Output {
		if len(bytes.TrimSpace(itemRaw)) == 0 {
			continue
		}

		var item map[string]any
		if err := common.Unmarshal(itemRaw, &item); err != nil {
			continue
		}

		itemType := strings.TrimSpace(common.Interface2String(item["type"]))
		switch itemType {
		case "reasoning":
			thinkingText := extractReasoningSummaryText(item["summary"])
			if thinkingText != "" {
				contentBlocks = append(contentBlocks, oaiCcClaudeResponseThinkingBlock{
					Type:      "thinking",
					Thinking:  thinkingText,
					Signature: generateThinkingSignature(thinkingText),
				})
			}
		case "message":
			for _, partAny := range asArray(item["content"]) {
				part, ok := partAny.(map[string]any)
				if !ok {
					continue
				}
				partType := strings.TrimSpace(common.Interface2String(part["type"]))
				switch partType {
				case "output_text", "text":
					pushText(part["text"], part["annotations"])
				case "output_refusal", "refusal":
					hasRefusal = true
					pushText(part["refusal"], nil)
				}
			}
		case "function_call":
			hasFunctionCall = true
			callId := service.ResponsesCallIDToClaudeToolUseID(common.Interface2String(item["call_id"]))
			name := restoreResponsesToolName(common.Interface2String(item["name"]), normalizedToolNameMap)
			input := parseJSONFromString(item["arguments"])
			if input == nil {
				input = map[string]any{}
			}
			contentBlocks = append(contentBlocks, oaiCcClaudeResponseToolUseBlock{
				Type:  "tool_use",
				Id:    callId,
				Name:  name,
				Input: input,
			})
		case dto.BuildInCallWebSearchCall:
			webSearchRequests++
			contentBlocks = append(contentBlocks, buildClaudeWebSearchContentBlocks(item)...)
		case "output_text", "text":
			pushText(item["text"], item["annotations"])
		}
	}

	status := strings.TrimSpace(raw.Status)
	if status == "" {
		status = "completed"
	}
	incompleteReason := ""
	if raw.IncompleteDetails != nil {
		incompleteReason = strings.TrimSpace(raw.IncompleteDetails.Reason)
	}

	stopReason := "end_turn"
	switch {
	case hasFunctionCall:
		stopReason = "tool_use"
	case hasRefusal:
		stopReason = "refusal"
	case status == "incomplete":
		if incompleteReason == "max_output_tokens" {
			stopReason = "max_tokens"
		}
	}

	usagePayload := map[string]any{
		"input_tokens":  0,
		"output_tokens": 0,
	}
	if raw.Usage != nil {
		usagePayload["input_tokens"] = raw.Usage.InputTokens
		usagePayload["output_tokens"] = raw.Usage.OutputTokens
		cachedTokens := 0
		if raw.Usage.InputTokensDetails != nil {
			cachedTokens = raw.Usage.InputTokensDetails.CachedTokens
		}
		usagePayload["cache_read_input_tokens"] = cachedTokens
		usagePayload["cache_creation_input_tokens"] = 0
	}
	if webSearchRequests > 0 {
		usagePayload["server_tool_use"] = map[string]any{
			"web_search_requests": webSearchRequests,
		}
	}

	return &oaiCcClaudeResponse{
		Id:           generateClaudeMessageId(),
		Type:         "message",
		Role:         "assistant",
		Content:      contentBlocks,
		Model:        claudeModel,
		StopReason:   stopReason,
		StopSequence: nil,
		Usage:        usagePayload,
	}
}

func responsesToolNameMapFromRelayInfo(info *relaycommon.RelayInfo) map[string]string {
	if info == nil || info.ClaudeConvertInfo == nil || len(info.ClaudeConvertInfo.ResponsesToolNameByNormalized) == 0 {
		return nil
	}
	return info.ClaudeConvertInfo.ResponsesToolNameByNormalized
}

func restoreResponsesToolName(name string, normalizedToolNameMap map[string]string) string {
	name = strings.TrimSpace(name)
	if name == "" || len(normalizedToolNameMap) == 0 {
		return name
	}
	if original, ok := normalizedToolNameMap[name]; ok {
		original = strings.TrimSpace(original)
		if original != "" {
			return original
		}
	}
	return name
}

type responsesToClaudeStreamTranslator struct {
	msgId                   string
	model                   string
	blockIndex              int
	messageStarted          bool
	messageStopped          bool
	sawError                bool
	outputIndexToBlock      []int
	messagePartToBlock      map[string]int
	messageOutputIndexToBlk map[int][]int
	blocksStopped           []bool
	reasoningText           []string
	functionArgsDeltaSeen   map[int]bool
	hasFunctionCall         bool
	hasRefusal              bool
	webSearchRequests       int
	oaiCcUsage              *oai_cc.UsageContext
	messageStartUsage       *oaiCcClaudeUsage
	normalizedToolNameMap   map[string]string
}

func newResponsesToClaudeStreamTranslator(model string, usageCtx *oai_cc.UsageContext) *responsesToClaudeStreamTranslator {
	return newResponsesToClaudeStreamTranslatorWithToolNameMap(model, usageCtx, nil)
}

func newResponsesToClaudeStreamTranslatorWithToolNameMap(model string, usageCtx *oai_cc.UsageContext, normalizedToolNameMap map[string]string) *responsesToClaudeStreamTranslator {
	t := &responsesToClaudeStreamTranslator{
		msgId:                   generateClaudeMessageId(),
		model:                   model,
		outputIndexToBlock:      make([]int, 0),
		messagePartToBlock:      make(map[string]int),
		messageOutputIndexToBlk: make(map[int][]int),
		blocksStopped:           make([]bool, 0),
		reasoningText:           make([]string, 0),
		functionArgsDeltaSeen:   make(map[int]bool),
		oaiCcUsage:              usageCtx,
		normalizedToolNameMap:   normalizedToolNameMap,
	}
	if usageCtx != nil {
		t.messageStartUsage = &oaiCcClaudeUsage{
			InputTokens:  usageCtx.MessageStartInputTokens,
			OutputTokens: 0,
			UsageCache:   usageCtx.MessageStartUsageCache,
			InferenceGeo: "not_available",
		}
	}
	return t
}

func (t *responsesToClaudeStreamTranslator) restoreToolName(name string) string {
	if t == nil {
		return strings.TrimSpace(name)
	}
	return restoreResponsesToolName(name, t.normalizedToolNameMap)
}

func (t *responsesToClaudeStreamTranslator) processEvent(eventType string, payload map[string]any) ([]claudeSseOut, *dto.Usage, bool) {
	out := make([]claudeSseOut, 0, 2)

	switch eventType {
	case "response.created", "response.in_progress":
		if !t.messageStarted {
			t.messageStarted = true
			startUsage := any(map[string]any{"input_tokens": 0, "output_tokens": 0})
			if t.messageStartUsage != nil {
				startUsage = t.messageStartUsage
			}
			out = append(out, claudeSseOut{
				EventType: "message_start",
				Payload: oaiCcClaudeStreamMessageStart{
					Type: "message_start",
					Message: oaiCcClaudeStreamMessage{
						Id:           t.msgId,
						Type:         "message",
						Role:         "assistant",
						Content:      make([]any, 0),
						Model:        t.model,
						StopReason:   nil,
						StopSequence: nil,
						Usage:        startUsage,
					},
				},
			})
		}
		return out, nil, true

	case "response.output_item.added":
		item, _ := payload["item"].(map[string]any)
		itemType := strings.TrimSpace(common.Interface2String(item["type"]))
		oi := asNonNegativeInt(payload["output_index"])

		switch itemType {
		case "reasoning":
			t.startThinkingBlockIfMissing(&out, oi)
		case "function_call":
			t.hasFunctionCall = true
			callId := service.ResponsesCallIDToClaudeToolUseID(common.Interface2String(item["call_id"]))
			name := t.restoreToolName(common.Interface2String(item["name"]))
			t.startToolUseBlockIfMissing(&out, oi, callId, name)
		}
		return out, nil, true

	case "response.content_part.added":
		oi := asNonNegativeInt(payload["output_index"])
		ci := asNonNegativeInt(payload["content_index"])
		part, _ := payload["part"].(map[string]any)
		partType := strings.TrimSpace(common.Interface2String(part["type"]))
		if partType == "refusal" {
			t.hasRefusal = true
		}
		if partType == "" || partType == "output_text" || partType == "text" || partType == "refusal" {
			t.startTextBlockIfMissing(&out, oi, ci)
		}
		return out, nil, true

	case "response.outtext.delta", "response.output_text.delta":
		delta := asDeltaText(payload["delta"])
		oi := asNonNegativeInt(payload["output_index"])
		ci := asNonNegativeInt(payload["content_index"])
		idx := t.startTextBlockIfMissing(&out, oi, ci)
		out = append(out, claudeSseOut{
			EventType: "content_block_delta",
			Payload: oaiCcClaudeStreamContentBlockDelta{
				Type:  "content_block_delta",
				Index: idx,
				Delta: map[string]any{"type": "text_delta", "text": delta},
			},
		})
		return out, nil, true

	case "response.refusal.delta":
		delta := asDeltaText(payload["delta"])
		oi := asNonNegativeInt(payload["output_index"])
		ci := asNonNegativeInt(payload["content_index"])
		idx := t.startTextBlockIfMissing(&out, oi, ci)
		t.hasRefusal = true
		out = append(out, claudeSseOut{
			EventType: "content_block_delta",
			Payload: oaiCcClaudeStreamContentBlockDelta{
				Type:  "content_block_delta",
				Index: idx,
				Delta: map[string]any{"type": "text_delta", "text": delta},
			},
		})
		return out, nil, true

	case "response.reasoning_summary_text.delta":
		delta := asDeltaText(payload["delta"])
		oi := asNonNegativeInt(payload["output_index"])
		idx := t.startThinkingBlockIfMissing(&out, oi)
		t.ensureReasoningSlot(oi)
		t.reasoningText[oi] += delta
		out = append(out, claudeSseOut{
			EventType: "content_block_delta",
			Payload: oaiCcClaudeStreamContentBlockDelta{
				Type:  "content_block_delta",
				Index: idx,
				Delta: map[string]any{"type": "thinking_delta", "thinking": delta},
			},
		})
		return out, nil, true

	case "response.function_call_arguments.delta":
		delta := asDeltaText(payload["delta"])
		oi := asNonNegativeInt(payload["output_index"])
		callId := service.ResponsesCallIDToClaudeToolUseID(common.Interface2String(payload["call_id"]))
		name := t.restoreToolName(common.Interface2String(payload["name"]))
		t.hasFunctionCall = true
		idx := t.startToolUseBlockIfMissing(&out, oi, callId, name)
		if delta == "" {
			return out, nil, true
		}
		t.functionArgsDeltaSeen[oi] = true
		out = append(out, claudeSseOut{
			EventType: "content_block_delta",
			Payload: oaiCcClaudeStreamContentBlockDelta{
				Type:  "content_block_delta",
				Index: idx,
				Delta: map[string]any{"type": "input_json_delta", "partial_json": delta},
			},
		})
		return out, nil, true

	case "response.function_call_arguments.done":
		oi := asNonNegativeInt(payload["output_index"])
		if t.functionArgsDeltaSeen[oi] {
			return out, nil, true
		}

		idx := t.getOutputItemBlockIndex(oi)
		if idx < 0 {
			callID := strings.TrimSpace(common.Interface2String(payload["call_id"]))
			if callID == "" {
				return out, nil, true
			}
			name := t.restoreToolName(common.Interface2String(payload["name"]))
			idx = t.startToolUseBlockIfMissing(&out, oi, service.ResponsesCallIDToClaudeToolUseID(callID), name)
		}

		args := strings.TrimSpace(common.Interface2String(payload["arguments"]))
		if args == "" {
			return out, nil, true
		}

		t.hasFunctionCall = true
		t.functionArgsDeltaSeen[oi] = true
		out = append(out, claudeSseOut{
			EventType: "content_block_delta",
			Payload: oaiCcClaudeStreamContentBlockDelta{
				Type:  "content_block_delta",
				Index: idx,
				Delta: map[string]any{"type": "input_json_delta", "partial_json": args},
			},
		})
		return out, nil, true

	case "response.output_item.done":
		oi := asNonNegativeInt(payload["output_index"])
		item, _ := payload["item"].(map[string]any)
		itemType := strings.TrimSpace(common.Interface2String(item["type"]))
		if itemType == dto.BuildInCallWebSearchCall {
			status := strings.TrimSpace(common.Interface2String(item["status"]))
			if status == "" || status == "completed" {
				out = append(out, t.buildClaudeWebSearchStreamEvents(item)...)
			}
			return out, nil, true
		}

		if itemType == "message" {
			blocks := t.messageOutputIndexToBlk[oi]
			for _, idx := range blocks {
				if !t.isStopped(idx) {
					t.markStopped(idx)
					out = append(out, claudeSseOut{
						EventType: "content_block_stop",
						Payload: oaiCcClaudeStreamContentBlockStop{
							Type:  "content_block_stop",
							Index: idx,
						},
					})
				}
			}
			return out, nil, true
		}

		idx := t.getOutputItemBlockIndex(oi)
		if idx < 0 {
			return out, nil, true
		}

		if itemType == "reasoning" {
			t.ensureReasoningSlot(oi)
			sig := generateThinkingSignature(t.reasoningText[oi])
			out = append(out, claudeSseOut{
				EventType: "content_block_delta",
				Payload: oaiCcClaudeStreamContentBlockDelta{
					Type:  "content_block_delta",
					Index: idx,
					Delta: map[string]any{"type": "signature_delta", "signature": sig},
				},
			})
		}

		if !t.isStopped(idx) {
			t.markStopped(idx)
			out = append(out, claudeSseOut{
				EventType: "content_block_stop",
				Payload: oaiCcClaudeStreamContentBlockStop{
					Type:  "content_block_stop",
					Index: idx,
				},
			})
		}

		return out, nil, true

	case "response.completed", "response.incomplete":
		response, _ := payload["response"].(map[string]any)
		finalOut, finalUsage := t.finalizeMessage(response)
		out = append(out, finalOut...)
		return out, finalUsage, false

	case "response.done":
		// Some upstreams (e.g. codex_cli_rs compatible) emit response.done as a terminal event.
		response, _ := payload["response"].(map[string]any)
		finalOut, finalUsage := t.finalizeMessage(response)
		out = append(out, finalOut...)
		return out, finalUsage, false

	case "response.failed":
		t.markError()
		response, _ := payload["response"].(map[string]any)
		out = append(out, t.errorFromFailedResponse(response)...)
		return out, nil, false

	case "error":
		t.markError()
		out = append(out, t.processErrorEvent(payload)...)
		return out, nil, true
	}

	return out, nil, true
}

func (t *responsesToClaudeStreamTranslator) processErrorEvent(payload map[string]any) []claudeSseOut {
	base := payload
	if base == nil {
		base = map[string]any{}
	}
	// Some upstreams wrap error details inside `{ error: { ... } }`.
	if errObj, ok := base["error"].(map[string]any); ok && errObj != nil {
		base = errObj
	}

	code := strings.ToLower(strings.TrimSpace(common.Interface2String(base["code"])))
	message := strings.TrimSpace(common.Interface2String(base["message"]))
	if message == "" {
		// Fallback to top-level message when we unwrapped into an empty error object.
		message = strings.TrimSpace(common.Interface2String(payload["message"]))
	}
	if message == "" {
		message = "Unknown error"
	}

	errorType := "api_error"
	switch {
	case strings.Contains(code, "rate_limit"):
		errorType = "rate_limit_error"
	case strings.Contains(code, "auth") || strings.Contains(code, "api_key"):
		errorType = "authentication_error"
	case strings.Contains(code, "permission"):
		errorType = "permission_error"
	case strings.Contains(code, "invalid_request"):
		errorType = "invalid_request_error"
	case strings.Contains(code, "overloaded"):
		errorType = "overloaded_error"
	}

	return []claudeSseOut{{
		EventType: "error",
		Payload: oaiCcClaudeStreamErrorEvent{
			Type: "error",
			Error: types.ClaudeError{
				Type:    errorType,
				Message: message,
			},
		},
	}}
}

func (t *responsesToClaudeStreamTranslator) errorFromFailedResponse(response map[string]any) []claudeSseOut {
	respErr, _ := response["error"].(map[string]any)
	errType := strings.TrimSpace(common.Interface2String(respErr["type"]))
	errMsg := strings.TrimSpace(common.Interface2String(respErr["message"]))
	if errMsg == "" {
		errMsg = "Unknown error"
	}
	outType := mapOpenAIErrorTypeToAnthropic(errType)
	return []claudeSseOut{{
		EventType: "error",
		Payload: oaiCcClaudeStreamErrorEvent{
			Type: "error",
			Error: types.ClaudeError{
				Type:    outType,
				Message: errMsg,
			},
		},
	}}
}

func mapOpenAIErrorTypeToAnthropic(t string) string {
	switch strings.TrimSpace(t) {
	case "invalid_request_error", "authentication_error", "permission_error", "rate_limit_error":
		return t
	default:
		return "api_error"
	}
}

func (t *responsesToClaudeStreamTranslator) finalizeMessage(response map[string]any) ([]claudeSseOut, *dto.Usage) {
	if t.messageStopped {
		return nil, nil
	}
	out := make([]claudeSseOut, 0, 4)

	for i := 0; i < t.blockIndex; i++ {
		if !t.isStopped(i) {
			t.markStopped(i)
			idx := i
			out = append(out, claudeSseOut{
				EventType: "content_block_stop",
				Payload: oaiCcClaudeStreamContentBlockStop{
					Type:  "content_block_stop",
					Index: idx,
				},
			})
		}
	}

	usageMap, _ := response["usage"].(map[string]any)
	outputTokens := asNonNegativeInt(usageMap["output_tokens"])
	inputTokens := asNonNegativeInt(usageMap["input_tokens"])
	cachedTokens := 0
	if details, ok := usageMap["input_tokens_details"].(map[string]any); ok {
		cachedTokens = asNonNegativeInt(details["cached_tokens"])
	}

	status := strings.TrimSpace(common.Interface2String(response["status"]))
	if status == "" {
		status = "completed"
	}
	incompleteReason := ""
	if details, ok := response["incomplete_details"].(map[string]any); ok {
		incompleteReason = strings.TrimSpace(common.Interface2String(details["reason"]))
	}

	stopReason := "end_turn"
	switch {
	case t.hasFunctionCall:
		stopReason = "tool_use"
	case t.hasRefusal:
		stopReason = "refusal"
	case status == "incomplete":
		if incompleteReason == "max_output_tokens" {
			stopReason = "max_tokens"
		}
	}

	finalUsageForClient := any(map[string]any{
		"output_tokens":               outputTokens,
		"input_tokens":                inputTokens,
		"cache_read_input_tokens":     cachedTokens,
		"cache_creation_input_tokens": 0,
	})
	if t.oaiCcUsage != nil {
		totalInputTokens := inputTokens
		if totalInputTokens <= 0 {
			totalInputTokens = t.oaiCcUsage.LocalTotalInputTokens
		}
		uncached, cache := t.oaiCcUsage.FinalUsage(totalInputTokens, outputTokens)
		finalUsageForClient = oaiCcClaudeUsage{
			InputTokens:   uncached,
			OutputTokens:  outputTokens,
			UsageCache:    cache,
			ServerToolUse: buildClaudeWebSearchUsage(t.webSearchRequests),
		}
	} else if t.webSearchRequests > 0 {
		if usageOut, ok := finalUsageForClient.(map[string]any); ok {
			usageOut["server_tool_use"] = map[string]any{
				"web_search_requests": t.webSearchRequests,
			}
		}
	}

	out = append(out, claudeSseOut{
		EventType: "message_delta",
		Payload: oaiCcClaudeStreamMessageDelta{
			Type: "message_delta",
			Delta: oaiCcClaudeStreamMessageDeltaDelta{
				StopReason:   stopReason,
				StopSequence: nil,
			},
			Usage: finalUsageForClient,
		},
	})
	out = append(out, claudeSseOut{
		EventType: "message_stop",
		Payload: oaiCcClaudeStreamMessageStop{
			Type: "message_stop",
		},
	})
	t.messageStopped = true

	uncachedBillingInput := inputTokens - cachedTokens
	if uncachedBillingInput < 0 {
		uncachedBillingInput = 0
	}

	billingPromptTokens := uncachedBillingInput
	billingCacheReadTokens := cachedTokens
	billingCacheCreationTokens := 0
	billingCacheCreation5mTokens := 0
	billingCacheCreation1hTokens := 0
	if t.oaiCcUsage != nil {
		totalInputTokens := inputTokens
		if totalInputTokens <= 0 {
			totalInputTokens = t.oaiCcUsage.LocalTotalInputTokens
		}
		uncached, cache := t.oaiCcUsage.FinalUsage(totalInputTokens, outputTokens)
		billingPromptTokens = uncached
		billingCacheReadTokens = cache.CacheReadInputTokens
		billingCacheCreationTokens = cache.CacheCreationInputTokens
		billingCacheCreation5mTokens = cache.ClaudeCacheCreation5m
		billingCacheCreation1hTokens = cache.ClaudeCacheCreation1h
	}
	return out, &dto.Usage{
		PromptTokens:     billingPromptTokens,
		CompletionTokens: outputTokens,
		TotalTokens:      billingPromptTokens + outputTokens,
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens:         billingCacheReadTokens,
			CachedCreationTokens: billingCacheCreationTokens,
		},
		ClaudeCacheCreation5mTokens: billingCacheCreation5mTokens,
		ClaudeCacheCreation1hTokens: billingCacheCreation1hTokens,
	}
}

func (t *responsesToClaudeStreamTranslator) finalizeIncomplete(streamExitReason string) ([]claudeSseOut, *dto.Usage) {
	if !t.messageStarted || t.messageStopped || t.sawError {
		return nil, nil
	}
	streamExitReason = strings.TrimSpace(streamExitReason)
	if streamExitReason != "" && streamExitReason != "done" {
		return nil, nil
	}
	return t.finalizeMessage(nil)
}

func (t *responsesToClaudeStreamTranslator) markError() {
	t.sawError = true
}

func (t *responsesToClaudeStreamTranslator) messagePartKey(outputIndex int, contentIndex int) string {
	return fmt.Sprintf("%d:%d", outputIndex, contentIndex)
}

func (t *responsesToClaudeStreamTranslator) startTextBlockIfMissing(out *[]claudeSseOut, outputIndex int, contentIndex int) int {
	key := t.messagePartKey(outputIndex, contentIndex)
	if existing, ok := t.messagePartToBlock[key]; ok {
		return existing
	}

	idx := t.blockIndex
	t.blockIndex++
	t.messagePartToBlock[key] = idx
	t.messageOutputIndexToBlk[outputIndex] = append(t.messageOutputIndexToBlk[outputIndex], idx)

	*out = append(*out, claudeSseOut{
		EventType: "content_block_start",
		Payload: oaiCcClaudeStreamContentBlockStart{
			Type:  "content_block_start",
			Index: idx,
			ContentBlock: map[string]any{
				"type": "text",
				"text": "",
			},
		},
	})
	return idx
}

func (t *responsesToClaudeStreamTranslator) startThinkingBlockIfMissing(out *[]claudeSseOut, outputIndex int) int {
	if existing := t.getOutputItemBlockIndex(outputIndex); existing >= 0 {
		return existing
	}

	idx := t.blockIndex
	t.blockIndex++
	t.setOutputItemBlockIndex(outputIndex, idx)
	t.ensureReasoningSlot(outputIndex)
	t.reasoningText[outputIndex] = ""

	*out = append(*out, claudeSseOut{
		EventType: "content_block_start",
		Payload: oaiCcClaudeStreamContentBlockStart{
			Type:  "content_block_start",
			Index: idx,
			ContentBlock: map[string]any{
				"type":     "thinking",
				"thinking": "",
			},
		},
	})
	return idx
}

func (t *responsesToClaudeStreamTranslator) startToolUseBlockIfMissing(out *[]claudeSseOut, outputIndex int, callId string, name string) int {
	if existing := t.getOutputItemBlockIndex(outputIndex); existing >= 0 {
		return existing
	}

	idx := t.blockIndex
	t.blockIndex++
	t.setOutputItemBlockIndex(outputIndex, idx)

	*out = append(*out, claudeSseOut{
		EventType: "content_block_start",
		Payload: oaiCcClaudeStreamContentBlockStart{
			Type:  "content_block_start",
			Index: idx,
			ContentBlock: map[string]any{
				"type":  "tool_use",
				"id":    callId,
				"name":  name,
				"input": map[string]any{},
			},
		},
	})
	return idx
}

func (t *responsesToClaudeStreamTranslator) buildClaudeWebSearchStreamEvents(item map[string]any) []claudeSseOut {
	toolUseID, input := buildClaudeWebSearchToolPayload(item)
	t.webSearchRequests++

	serverToolUseIndex := t.blockIndex
	t.blockIndex++
	webSearchResultIndex := t.blockIndex
	t.blockIndex++
	t.markStopped(serverToolUseIndex)
	t.markStopped(webSearchResultIndex)

	return []claudeSseOut{
		{
			EventType: "content_block_start",
			Payload: oaiCcClaudeStreamContentBlockStart{
				Type:  "content_block_start",
				Index: serverToolUseIndex,
				ContentBlock: map[string]any{
					"type":  "server_tool_use",
					"id":    toolUseID,
					"name":  dto.BuildInToolWebSearch,
					"input": input,
				},
			},
		},
		{
			EventType: "content_block_stop",
			Payload: oaiCcClaudeStreamContentBlockStop{
				Type:  "content_block_stop",
				Index: serverToolUseIndex,
			},
		},
		{
			EventType: "content_block_start",
			Payload: oaiCcClaudeStreamContentBlockStart{
				Type:  "content_block_start",
				Index: webSearchResultIndex,
				ContentBlock: map[string]any{
					"type":        "web_search_tool_result",
					"tool_use_id": toolUseID,
					"content":     []any{},
				},
			},
		},
		{
			EventType: "content_block_stop",
			Payload: oaiCcClaudeStreamContentBlockStop{
				Type:  "content_block_stop",
				Index: webSearchResultIndex,
			},
		},
	}
}

func (t *responsesToClaudeStreamTranslator) setOutputItemBlockIndex(outputIndex int, blockIndex int) {
	for len(t.outputIndexToBlock) <= outputIndex {
		t.outputIndexToBlock = append(t.outputIndexToBlock, -1)
	}
	t.outputIndexToBlock[outputIndex] = blockIndex
}

func (t *responsesToClaudeStreamTranslator) getOutputItemBlockIndex(outputIndex int) int {
	if outputIndex < 0 || outputIndex >= len(t.outputIndexToBlock) {
		return -1
	}
	return t.outputIndexToBlock[outputIndex]
}

func (t *responsesToClaudeStreamTranslator) ensureReasoningSlot(outputIndex int) {
	for len(t.reasoningText) <= outputIndex {
		t.reasoningText = append(t.reasoningText, "")
	}
}

func (t *responsesToClaudeStreamTranslator) isStopped(blockIndex int) bool {
	if blockIndex < 0 || blockIndex >= len(t.blocksStopped) {
		return false
	}
	return t.blocksStopped[blockIndex]
}

func (t *responsesToClaudeStreamTranslator) markStopped(blockIndex int) {
	for len(t.blocksStopped) <= blockIndex {
		t.blocksStopped = append(t.blocksStopped, false)
	}
	t.blocksStopped[blockIndex] = true
}

func asNonNegativeInt(value any) int {
	switch v := value.(type) {
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return 0
		}
		if v < 0 {
			return 0
		}
		return int(math.Floor(v))
	case int:
		if v < 0 {
			return 0
		}
		return v
	case int64:
		if v < 0 {
			return 0
		}
		return int(v)
	}
	return 0
}

func asDeltaText(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case map[string]any:
		if t, ok := v["text"].(string); ok {
			return t
		}
	}
	return ""
}

func asArray(value any) []any {
	if value == nil {
		return nil
	}
	if arr, ok := value.([]any); ok {
		return arr
	}
	return nil
}

func parseJSONFromString(value any) any {
	s, ok := value.(string)
	if !ok {
		if m, ok := value.(map[string]any); ok {
			return m
		}
		return nil
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	var out any
	if err := common.Unmarshal([]byte(s), &out); err != nil {
		return nil
	}
	return out
}

func buildClaudeWebSearchContentBlocks(item map[string]any) []any {
	toolUseID, input := buildClaudeWebSearchToolPayload(item)
	return []any{
		map[string]any{
			"type":  "server_tool_use",
			"id":    toolUseID,
			"name":  dto.BuildInToolWebSearch,
			"input": input,
		},
		map[string]any{
			"type":        "web_search_tool_result",
			"tool_use_id": toolUseID,
			"content":     []any{},
		},
	}
}

func buildClaudeWebSearchToolPayload(item map[string]any) (string, map[string]any) {
	itemID := strings.TrimSpace(common.Interface2String(item["id"]))
	if itemID == "" {
		itemID = strings.ReplaceAll(uuid.NewString(), "-", "")
	}

	input := map[string]any{}
	if action, ok := item["action"].(map[string]any); ok && action != nil {
		if query := strings.TrimSpace(common.Interface2String(action["query"])); query != "" {
			input["query"] = query
		}
	}

	return "srvtoolu_" + itemID, input
}

func buildClaudeWebSearchUsage(callCount int) *dto.ClaudeServerToolUse {
	if callCount <= 0 {
		return nil
	}
	return &dto.ClaudeServerToolUse{WebSearchRequests: callCount}
}

func recordClaudeWebSearchRequests(info *relaycommon.RelayInfo, callCount int) int {
	if callCount <= 0 || info == nil {
		return callCount
	}
	tool := relaycommon.EnsureResponsesBuiltInTool(info, dto.BuildInToolWebSearch)
	if tool == nil {
		return callCount
	}
	tool.CallCount += callCount
	return callCount
}

func recordClaudeWebSearchRequestsFromContent(info *relaycommon.RelayInfo, content []any) int {
	return recordClaudeWebSearchRequests(info, countClaudeWebSearchRequestsFromContent(content))
}

func recordResponsesNonWebSearchToolCallsAny(info *relaycommon.RelayInfo, toolsAny any) {
	if info == nil || toolsAny == nil {
		return
	}
	tools := make([]map[string]any, 0)
	switch typed := toolsAny.(type) {
	case []map[string]any:
		tools = typed
	case []any:
		for _, toolAny := range typed {
			if tool, ok := toolAny.(map[string]any); ok && tool != nil {
				tools = append(tools, tool)
			}
		}
	}
	recordResponsesNonWebSearchToolCalls(info, tools)
}

func recordResponsesNonWebSearchToolCalls(info *relaycommon.RelayInfo, tools []map[string]any) {
	if info == nil || len(tools) == 0 {
		return
	}
	relaycommon.PopulateResponsesUsageTools(info, tools)
	for _, tool := range tools {
		toolType := relaycommon.NormalizeResponsesToolType(common.Interface2String(tool["type"]))
		if strings.TrimSpace(toolType) == "" || toolType == dto.BuildInToolWebSearch {
			continue
		}
		relaycommon.IncrementResponsesBuiltInToolCall(info, toolType)
	}
}

func countClaudeWebSearchRequestsFromContent(content []any) int {
	count := 0
	for _, blockAny := range content {
		block, ok := blockAny.(map[string]any)
		if !ok || block == nil {
			continue
		}
		if strings.TrimSpace(common.Interface2String(block["type"])) != "server_tool_use" {
			continue
		}
		if strings.TrimSpace(common.Interface2String(block["name"])) != dto.BuildInToolWebSearch {
			continue
		}
		count++
	}
	return count
}

func extractReasoningSummaryText(value any) string {
	arr := asArray(value)
	if len(arr) == 0 {
		if s, ok := value.(string); ok {
			return s
		}
		return ""
	}
	parts := make([]string, 0, len(arr))
	for _, itemAny := range arr {
		item, ok := itemAny.(map[string]any)
		if !ok {
			continue
		}
		if t, ok := item["text"].(string); ok && t != "" {
			parts = append(parts, t)
		}
	}
	return strings.Join(parts, "\n")
}

func generateClaudeMessageId() string {
	id := strings.ReplaceAll(uuid.NewString(), "-", "")
	if len(id) > 24 {
		id = id[:24]
	}
	return "msg_" + id
}

func generateThinkingSignature(content string) string {
	var hash uint64 = 5381
	b := []byte(content)
	for _, by := range b {
		hash = (hash * 33) + uint64(by)
	}
	return fmt.Sprintf("sig_%016x", hash)
}

func usageIntsFromResponses(u *dto.Usage) (inputTokens int, outputTokens int, cachedTokens int) {
	if u == nil {
		return 0, 0, 0
	}

	inputTokens = u.InputTokens
	outputTokens = u.OutputTokens

	// Best-effort compatibility with providers that only fill prompt/completion tokens.
	if inputTokens <= 0 && u.PromptTokens > 0 {
		inputTokens = u.PromptTokens
	}
	if outputTokens <= 0 && u.CompletionTokens > 0 {
		outputTokens = u.CompletionTokens
	}

	if u.InputTokensDetails != nil {
		cachedTokens = u.InputTokensDetails.CachedTokens
	} else {
		cachedTokens = u.PromptTokensDetails.CachedTokens
	}

	if inputTokens < 0 {
		inputTokens = 0
	}
	if outputTokens < 0 {
		outputTokens = 0
	}
	if cachedTokens < 0 {
		cachedTokens = 0
	}
	return inputTokens, outputTokens, cachedTokens
}

func buildClaudeUsageFromOaiCc(inputTokens int, outputTokens int, cache oai_cc.UsageCache, inferenceGeo string) *dto.ClaudeUsage {
	if inputTokens < 0 {
		inputTokens = 0
	}
	if outputTokens < 0 {
		outputTokens = 0
	}

	u := &dto.ClaudeUsage{
		InputTokens:              inputTokens,
		OutputTokens:             outputTokens,
		CacheReadInputTokens:     asNonNegativeInt(cache.CacheReadInputTokens),
		CacheCreationInputTokens: asNonNegativeInt(cache.CacheCreationInputTokens),
		CacheCreation: &dto.ClaudeCacheCreationUsage{
			Ephemeral5mInputTokens: asNonNegativeInt(cache.CacheCreation.Ephemeral5mInputTokens),
			Ephemeral1hInputTokens: asNonNegativeInt(cache.CacheCreation.Ephemeral1hInputTokens),
		},
		ClaudeCacheCreation5mTokens: asNonNegativeInt(cache.ClaudeCacheCreation5m),
		ClaudeCacheCreation1hTokens: asNonNegativeInt(cache.ClaudeCacheCreation1h),
	}
	if strings.TrimSpace(inferenceGeo) != "" {
		u.InferenceGeo = inferenceGeo
	}
	return u
}

func writeOaiCcClaudeSseEvent(c *gin.Context, eventType string, payload any) error {
	if c == nil {
		return fmt.Errorf("nil gin context")
	}
	b, err := common.Marshal(payload)
	if err != nil {
		return err
	}
	c.Render(-1, common.CustomEvent{Data: fmt.Sprintf("event: %s\n", eventType)})
	c.Render(-1, common.CustomEvent{Data: "data: " + string(b)})
	_ = helper.FlushWriter(c)
	return nil
}
