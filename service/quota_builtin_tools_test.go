package service

import (
	"net/http"
	"net/http/httptest"
	"one-api/dto"
	relaycommon "one-api/relay/common"
	"one-api/setting/operation_setting"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestCalculateBuiltInToolChargeSummary_UsesResponsesAndClaudeToolUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	ctx.Set("claude_web_search_requests", 2)

	info := &relaycommon.RelayInfo{}
	relaycommon.PopulateResponsesUsageTools(info, []map[string]any{
		{
			"type":                dto.BuildInToolWebSearch,
			"search_context_size": "high",
		},
		{
			"type": dto.BuildInToolFileSearch,
		},
	})
	relaycommon.EnsureResponsesBuiltInTool(info, dto.BuildInToolWebSearch).CallCount = 3
	relaycommon.EnsureResponsesBuiltInTool(info, dto.BuildInToolFileSearch).CallCount = 4

	summary := calculateBuiltInToolChargeSummary(ctx, info, "claude-3-5-sonnet", "gpt-5.2-codex", 2, 1.5)

	if summary.ResponsesWebSearchCallCount != 3 {
		t.Fatalf("expected responses web search count 3, got %d", summary.ResponsesWebSearchCallCount)
	}
	if summary.ResponsesWebSearchContextSize != "high" {
		t.Fatalf("expected responses web search context size high, got %q", summary.ResponsesWebSearchContextSize)
	}
	if summary.ResponsesWebSearchPrice <= 0 || summary.ResponsesWebSearchQuota <= 0 || summary.ResponsesWebSearchCostQuota <= 0 {
		t.Fatalf("expected responses web search charges to be populated, got %#v", summary)
	}
	if want := operation_setting.GetWebSearchPricePerThousand("gpt-5.2-codex", "high"); summary.ResponsesWebSearchPrice != want {
		t.Fatalf("expected responses web search price to follow upstream model pricing %v, got %v", want, summary.ResponsesWebSearchPrice)
	}
	if summary.ClaudeWebSearchCallCount != 2 {
		t.Fatalf("expected claude web search count 2, got %d", summary.ClaudeWebSearchCallCount)
	}
	if summary.ClaudeWebSearchPrice <= 0 || summary.ClaudeWebSearchQuota <= 0 || summary.ClaudeWebSearchCostQuota <= 0 {
		t.Fatalf("expected claude web search charges to be populated, got %#v", summary)
	}
	if summary.FileSearchCallCount != 4 {
		t.Fatalf("expected file search count 4, got %d", summary.FileSearchCallCount)
	}
	if summary.FileSearchPrice <= 0 || summary.FileSearchQuota <= 0 || summary.FileSearchCostQuota <= 0 {
		t.Fatalf("expected file search charges to be populated, got %#v", summary)
	}
}

func TestBuiltInToolChargeSummary_ExtraContentIncludesToolBreakdown(t *testing.T) {
	summary := builtInToolChargeSummary{
		ResponsesWebSearchCallCount:   1,
		ResponsesWebSearchContextSize: "medium",
		ResponsesWebSearchQuota:       10,
		ClaudeWebSearchCallCount:      2,
		ClaudeWebSearchQuota:          5,
		FileSearchCallCount:           3,
		FileSearchQuota:               1.5,
	}

	extra := summary.ExtraContent()
	if extra == "" {
		t.Fatal("expected extra content to be generated")
	}
	if want := "Web Search 调用 1 次"; !contains(extra, want) {
		t.Fatalf("expected extra content to include %q, got %q", want, extra)
	}
	if want := "Claude Web Search 调用 2 次"; !contains(extra, want) {
		t.Fatalf("expected extra content to include %q, got %q", want, extra)
	}
	if want := "File Search 调用 3 次"; !contains(extra, want) {
		t.Fatalf("expected extra content to include %q, got %q", want, extra)
	}
}

func contains(s string, sub string) bool {
	return strings.Contains(s, sub)
}
