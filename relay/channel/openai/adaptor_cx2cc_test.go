package openai

import (
	"net/http"
	"net/http/httptest"
	"one-api/common"
	"one-api/constant"
	"one-api/dto"
	relaycommon "one-api/relay/common"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestConvertClaudeRequest_Cx2ccPopulatesResponsesUsageTools(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	common.SetContextKey(c, constant.ContextKeyChannelMessagesToResponsesCompat, true)

	info := &relaycommon.RelayInfo{
		OriginModelName: "claude-3-5-sonnet",
		ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5.2-codex"},
	}
	req := &dto.ClaudeRequest{
		Model: "claude-3-5-sonnet",
		Messages: []dto.ClaudeMessage{
			{Role: "user", Content: "hello"},
		},
		Tools: []map[string]any{
			{
				"type": "web_search_20250305",
				"name": "web_search",
			},
		},
	}

	adaptor := &Adaptor{}
	converted, err := adaptor.ConvertClaudeRequest(c, info, req)
	if err != nil {
		t.Fatalf("ConvertClaudeRequest error: %v", err)
	}

	responsesReq, ok := converted.(dto.OpenAIResponsesRequest)
	if !ok {
		t.Fatalf("expected OpenAIResponsesRequest, got %#v", converted)
	}
	if responsesReq.GetToolsMap()[0]["type"] != dto.BuildInToolWebSearch {
		t.Fatalf("expected normalized responses web_search tool, got %#v", responsesReq.GetToolsMap())
	}

	webSearchTool := relaycommon.EnsureResponsesBuiltInTool(info, dto.BuildInToolWebSearch)
	if webSearchTool == nil {
		t.Fatalf("expected responses usage info to be initialized, got %#v", info.ResponsesUsageInfo)
	}
	if webSearchTool.SearchContextSize != "medium" {
		t.Fatalf("expected default search_context_size to be medium, got %#v", webSearchTool.SearchContextSize)
	}
}
