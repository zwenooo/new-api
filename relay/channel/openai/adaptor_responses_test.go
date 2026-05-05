package openai

import (
	"net/http"
	"net/http/httptest"
	"one-api/constant"
	relaycommon "one-api/relay/common"
	relayconstant "one-api/relay/constant"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSetupRequestHeader_DirectCodexResponsesAddsRequiredHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("Content-Type", "application/json; charset=utf-8")

	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeResponses,
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiKey:         "test-key",
			ChannelType:    constant.ChannelTypeCustom,
			ChannelBaseUrl: "https://chatgpt.com/backend-api/codex/responses",
		},
	}

	header := http.Header{}
	adaptor := &Adaptor{}
	if err := adaptor.SetupRequestHeader(c, &header, info); err != nil {
		t.Fatalf("SetupRequestHeader() error = %v", err)
	}

	if got := header.Get("OpenAI-Beta"); got != "responses=experimental" {
		t.Fatalf("OpenAI-Beta = %q, want responses=experimental", got)
	}
	if got := header.Get("originator"); got != "codex_cli_rs" {
		t.Fatalf("originator = %q, want codex_cli_rs", got)
	}
	if got := header.Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
	if got := header.Get("Accept"); got != "application/json" {
		t.Fatalf("Accept = %q, want application/json", got)
	}
	if got := header.Get("Authorization"); got != "Bearer test-key" {
		t.Fatalf("Authorization = %q, want Bearer test-key", got)
	}
}
