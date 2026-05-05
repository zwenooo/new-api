package cx2cc

import (
	"fmt"
	"strings"

	appcommon "one-api/common"
	"one-api/dto"
	relaycommon "one-api/relay/common"
	oai_cc "one-api/relay/compat/oai_cc"
	relayconstant "one-api/relay/constant"
	"one-api/service"

	"github.com/gin-gonic/gin"
)

type PrepareMessagesToResponsesOptions struct {
	IncludeUpstreamSessionHeaderAliases bool
}

func PrepareChannelMessagesToResponsesCompatRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ClaudeRequest) (*dto.OpenAIResponsesRequest, error) {
	return prepareClaudeMessagesToResponsesRequest(c, info, request, buildPrepareMessagesToResponsesOptions(c))
}

func buildPrepareMessagesToResponsesOptions(c *gin.Context) PrepareMessagesToResponsesOptions {
	return PrepareMessagesToResponsesOptions{
		IncludeUpstreamSessionHeaderAliases: false,
	}
}

func prepareClaudeMessagesToResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ClaudeRequest, opts PrepareMessagesToResponsesOptions) (*dto.OpenAIResponsesRequest, error) {
	if c == nil {
		return nil, fmt.Errorf("gin context is required")
	}
	if info == nil {
		return nil, fmt.Errorf("relay info is required")
	}
	if request == nil {
		return nil, fmt.Errorf("claude request is required")
	}
	if info.ChannelMeta == nil {
		info.ChannelMeta = &relaycommon.ChannelMeta{}
	}

	// Keep the external API as /v1/messages but force the upstream leg to Responses.
	info.DisablePing = true
	info.RelayMode = relayconstant.RelayModeResponses
	info.RequestURLPath = "/v1/responses"

	ids := resolveSessionIDs(c, request)

	stripAnthropicBillingHeader(request)

	var anthropicReq map[string]any
	if b, err := appcommon.Marshal(request); err != nil {
		return nil, err
	} else if err := appcommon.Unmarshal(b, &anthropicReq); err != nil {
		return nil, err
	} else {
		info.OaiCcUsage = oai_cc.BuildUsageContext(ids.SessionKey, anthropicReq)
	}

	responsesReq, originalToNormalized, normalizedToOriginal, err := service.ClaudeToOpenAIResponsesRequestWithToolNameMapping(*request)
	if err != nil {
		return nil, err
	}
	if info.ClaudeConvertInfo == nil {
		info.ClaudeConvertInfo = &relaycommon.ClaudeConvertInfo{
			LastMessagesType: relaycommon.LastMessageTypeNone,
		}
	}
	info.ClaudeConvertInfo.ResponsesToolNameByOriginal = originalToNormalized
	info.ClaudeConvertInfo.ResponsesToolNameByNormalized = normalizedToOriginal
	relaycommon.PopulateResponsesUsageTools(info, responsesReq.GetToolsMap())

	applyMappedModel(info, responsesReq)

	// Some upstreams reject `metadata` in body; keep session/conversation ids in headers instead.
	responsesReq.Metadata = nil
	applySessionContext(c, info, responsesReq, ids, opts.IncludeUpstreamSessionHeaderAliases)

	return responsesReq, nil
}

func stripAnthropicBillingHeader(request *dto.ClaudeRequest) {
	if request == nil || request.System == nil {
		return
	}

	normalizedSystem := request.System
	// Normalize into JSON-native shapes before stripping because earlier middlewares may
	// turn `system` into typed DTO structs.
	if b, err := appcommon.Marshal(request.System); err == nil {
		var sys any
		if err := appcommon.Unmarshal(b, &sys); err == nil && sys != nil {
			normalizedSystem = sys
		}
	}
	if newSystem, removed := oai_cc.StripAnthropicBillingHeaderFromSystemValue(normalizedSystem); removed > 0 {
		request.System = newSystem
	}
}

func applyMappedModel(info *relaycommon.RelayInfo, responsesReq *dto.OpenAIResponsesRequest) {
	if info == nil || responsesReq == nil {
		return
	}

	currentUpstreamModel := strings.TrimSpace(info.UpstreamModelName)
	if currentUpstreamModel != "" {
		responsesReq.Model = currentUpstreamModel
	}
}

func applySessionContext(c *gin.Context, info *relaycommon.RelayInfo, responsesReq *dto.OpenAIResponsesRequest, ids sessionIDs, includeAliases bool) {
	if c == nil || info == nil || responsesReq == nil {
		return
	}
	if strings.TrimSpace(ids.ThreadID) == "" {
		return
	}

	if b, err := appcommon.Marshal(ids.ThreadID); err == nil {
		responsesReq.PromptCacheKey = b
		info.PromptCacheKey = ids.ThreadID
	}

	threadIDHeader := ids.ThreadID
	if len(threadIDHeader) > 256 {
		threadIDHeader = threadIDHeader[:256]
	}

	if c.Request != nil {
		c.Request.Header.Set("session_id", threadIDHeader)
		if includeAliases {
			c.Request.Header.Set("session-id", threadIDHeader)
			c.Request.Header.Set("x-session-id", threadIDHeader)
		}
	}

	info.ConversationId = ids.ConversationID
	info.SessionId = ids.SessionID
}
