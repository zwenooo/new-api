package common

import (
	"net/http"
	"net/http/httptest"
	"one-api/dto"
	"one-api/types"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

func TestGenRelayInfoResponsesKeepsClientWebSocket(t *testing.T) {
	gin.SetMode(gin.TestMode)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)

	request := &dto.OpenAIResponsesRequest{
		Model: "gpt-5.2-codex",
	}
	clientWS := &websocket.Conn{}

	info, err := GenRelayInfo(c, types.RelayFormatOpenAIResponses, request, clientWS)
	if err != nil {
		t.Fatalf("GenRelayInfo returned error: %v", err)
	}
	if info == nil {
		t.Fatal("expected relay info")
	}
	if info.ClientWs != clientWS {
		t.Fatal("expected relay info to preserve client websocket for responses relay")
	}
	if info.RelayFormat != types.RelayFormatOpenAIResponses {
		t.Fatalf("unexpected relay format: %v", info.RelayFormat)
	}
}

func TestPopulateResponsesUsageTools_CanonicalizesWebSearchAndPreservesContextSize(t *testing.T) {
	info := &RelayInfo{}

	PopulateResponsesUsageTools(info, []map[string]any{
		{
			"type":                "web_search_preview",
			"search_context_size": "high",
		},
	})

	if info.ResponsesUsageInfo == nil || info.ResponsesUsageInfo.BuiltInTools == nil {
		t.Fatal("expected responses usage info to be initialized")
	}
	if _, exists := info.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolWebSearchPreview]; exists {
		t.Fatalf("expected legacy web_search_preview key to be canonicalized away: %#v", info.ResponsesUsageInfo.BuiltInTools)
	}
	webSearchTool := info.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolWebSearch]
	if webSearchTool == nil {
		t.Fatalf("expected canonical web_search tool entry, got %#v", info.ResponsesUsageInfo.BuiltInTools)
	}
	if webSearchTool.SearchContextSize != "high" {
		t.Fatalf("expected search_context_size to be preserved, got %#v", webSearchTool.SearchContextSize)
	}
}
