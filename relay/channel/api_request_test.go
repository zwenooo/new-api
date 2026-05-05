package channel

import (
	"net/http"
	"net/http/httptest"
	"testing"

	common2 "one-api/common"
	"one-api/constant"
	relaycommon "one-api/relay/common"
	relayconstant "one-api/relay/constant"
	"one-api/types"

	"github.com/gin-gonic/gin"
)

func TestSetupApiRequestHeader_Cx2ccAcceptMatchesStreamMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		isStream   bool
		relayMode  int
		relayFmt   types.RelayFormat
		wantAccept string
	}{
		{
			name:       "cx2cc stream uses sse accept",
			isStream:   true,
			relayMode:  relayconstant.RelayModeResponses,
			relayFmt:   types.RelayFormatClaude,
			wantAccept: "text/event-stream",
		},
		{
			name:       "cx2cc non-stream uses json accept",
			isStream:   false,
			relayMode:  relayconstant.RelayModeResponses,
			relayFmt:   types.RelayFormatClaude,
			wantAccept: "application/json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
			common2.SetContextKey(c, constant.ContextKeyChannelMessagesToResponsesCompat, true)

			info := &relaycommon.RelayInfo{
				RelayMode:   tt.relayMode,
				RelayFormat: tt.relayFmt,
				IsStream:    tt.isStream,
			}

			headers := http.Header{}
			SetupApiRequestHeader(info, c, &headers)

			if got := headers.Get("Accept"); got != tt.wantAccept {
				t.Fatalf("Accept = %q, want %q", got, tt.wantAccept)
			}
		})
	}
}

func TestSetupApiRequestHeader_ForwardsCodexCompatHeadersForCx2cc(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	c.Request.Header.Set("x-codex-beta-features", "planner")
	c.Request.Header.Set("x-oai-web-search-eligible", "true")
	c.Request.Header.Set("x-openai-subagent", "worker")

	common2.SetContextKey(c, constant.ContextKeyChannelMessagesToResponsesCompat, true)

	info := &relaycommon.RelayInfo{
		RelayMode:   relayconstant.RelayModeResponses,
		RelayFormat: types.RelayFormatClaude,
		IsStream:    true,
	}

	headers := http.Header{}
	SetupApiRequestHeader(info, c, &headers)

	if got := headers.Get("x-codex-beta-features"); got != "planner" {
		t.Fatalf("x-codex-beta-features = %q, want planner", got)
	}
	if got := headers.Get("x-oai-web-search-eligible"); got != "true" {
		t.Fatalf("x-oai-web-search-eligible = %q, want true", got)
	}
	if got := headers.Get("x-openai-subagent"); got != "worker" {
		t.Fatalf("x-openai-subagent = %q, want worker", got)
	}
}

func TestSetupApiRequestHeader_ForwardsRequiredHeadersForOrdinaryCx2cc(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	c.Request.Header.Set("User-Agent", "third-party-client/1.0")
	c.Request.Header.Set("conversation_id", "conv_123")
	c.Request.Header.Set("accept-language", "zh-CN")
	c.Request.Header.Set("x-codex-turn-state", "turn-1")
	c.Request.Header.Set("x-codex-turn-metadata", "meta-1")
	c.Request.Header.Set("x-oai-web-search-eligible", "true")
	c.Request.Header.Set("x-openai-subagent", "worker")

	common2.SetContextKey(c, constant.ContextKeyChannelMessagesToResponsesCompat, true)
	common2.SetContextKey(c, constant.ContextKeyResponsesForceUpstreamStream, true)

	info := &relaycommon.RelayInfo{
		RelayMode:   relayconstant.RelayModeResponses,
		RelayFormat: types.RelayFormatClaude,
		IsStream:    false,
	}

	headers := http.Header{}
	SetupApiRequestHeader(info, c, &headers)

	if got := headers.Get("Accept"); got != "text/event-stream" {
		t.Fatalf("Accept = %q, want text/event-stream", got)
	}
	if got := headers.Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
	if got := headers.Get("OpenAI-Beta"); got != "responses=experimental" {
		t.Fatalf("OpenAI-Beta = %q, want responses=experimental", got)
	}
	if got := headers.Get("originator"); got != "codex_cli_rs" {
		t.Fatalf("originator = %q, want codex_cli_rs", got)
	}
	if got := headers.Get("User-Agent"); got != "third-party-client/1.0" {
		t.Fatalf("User-Agent = %q, want third-party-client/1.0", got)
	}
	if got := headers.Get("conversation_id"); got != "conv_123" {
		t.Fatalf("conversation_id = %q, want conv_123", got)
	}
	if got := headers.Get("accept-language"); got != "zh-CN" {
		t.Fatalf("accept-language = %q, want zh-CN", got)
	}
	if got := headers.Get("x-codex-turn-state"); got != "turn-1" {
		t.Fatalf("x-codex-turn-state = %q, want turn-1", got)
	}
	if got := headers.Get("x-codex-turn-metadata"); got != "meta-1" {
		t.Fatalf("x-codex-turn-metadata = %q, want meta-1", got)
	}
	if got := headers.Get("x-oai-web-search-eligible"); got != "true" {
		t.Fatalf("x-oai-web-search-eligible = %q, want true", got)
	}
	if got := headers.Get("x-openai-subagent"); got != "worker" {
		t.Fatalf("x-openai-subagent = %q, want worker", got)
	}
}

func TestSetupApiRequestHeader_OrdinaryCx2ccPreservesExplicitOriginatorAndBeta(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	c.Request.Header.Set("originator", "custom-originator")
	c.Request.Header.Set("OpenAI-Beta", "responses=experimental,responses_websockets=2026-02-06")

	common2.SetContextKey(c, constant.ContextKeyChannelMessagesToResponsesCompat, true)

	info := &relaycommon.RelayInfo{
		RelayMode:   relayconstant.RelayModeResponses,
		RelayFormat: types.RelayFormatClaude,
		IsStream:    false,
	}

	headers := http.Header{}
	SetupApiRequestHeader(info, c, &headers)

	if got := headers.Get("originator"); got != "custom-originator" {
		t.Fatalf("originator = %q, want custom-originator", got)
	}
	if got := headers.Get("OpenAI-Beta"); got != "responses=experimental,responses_websockets=2026-02-06" {
		t.Fatalf("OpenAI-Beta = %q, want explicit client value", got)
	}
}

func TestSetupApiRequestHeader_ForceResponsesStreamUsesSSEAccept(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("Accept", "application/json")

	common2.SetContextKey(c, constant.ContextKeyResponsesForceUpstreamStream, true)

	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeResponses,
		IsStream:  false,
	}

	headers := http.Header{}
	SetupApiRequestHeader(info, c, &headers)

	if got := headers.Get("Accept"); got != "text/event-stream" {
		t.Fatalf("Accept = %q, want text/event-stream", got)
	}
}
