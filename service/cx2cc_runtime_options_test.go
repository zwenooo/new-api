package service

import (
	"testing"

	"one-api/common"
	"one-api/dto"
)

func TestClaudeThinkingToResponsesReasoning_PrefersNewCx2ccOptionKeys(t *testing.T) {
	restoreCx2ccReasoningOptions(t, map[string]string{
		Cx2ccReasoningEffortOpt:        "high",
		Cx2ccReasoningSummaryOpt:       "detailed",
		legacyCx2ccReasoningEffortOpt:  "low",
		legacyCx2ccReasoningSummaryOpt: "concise",
	})

	got := claudeThinkingToResponsesReasoning(&dto.Thinking{Type: "adaptive"})
	if got == nil {
		t.Fatal("reasoning = nil, want non-nil")
	}
	if got.Effort != "high" {
		t.Fatalf("effort = %q, want %q", got.Effort, "high")
	}
	if got.Summary != "detailed" {
		t.Fatalf("summary = %q, want %q", got.Summary, "detailed")
	}
}

func TestClaudeThinkingToResponsesReasoning_FallsBackToLegacyCx2ccOptionKeys(t *testing.T) {
	restoreCx2ccReasoningOptions(t, map[string]string{
		legacyCx2ccReasoningEffortOpt:  "low",
		legacyCx2ccReasoningSummaryOpt: "concise",
	})

	got := claudeThinkingToResponsesReasoning(&dto.Thinking{Type: "adaptive"})
	if got == nil {
		t.Fatal("reasoning = nil, want non-nil")
	}
	if got.Effort != "low" {
		t.Fatalf("effort = %q, want %q", got.Effort, "low")
	}
	if got.Summary != "concise" {
		t.Fatalf("summary = %q, want %q", got.Summary, "concise")
	}
}

func restoreCx2ccReasoningOptions(t *testing.T, options map[string]string) {
	t.Helper()

	common.OptionMapRWMutex.Lock()
	backup := cloneCx2ccReasoningOptionMap(common.OptionMap)
	if common.OptionMap == nil {
		common.OptionMap = make(map[string]string)
	}
	for _, key := range []string{
		Cx2ccReasoningEffortOpt,
		Cx2ccReasoningSummaryOpt,
		legacyCx2ccReasoningEffortOpt,
		legacyCx2ccReasoningSummaryOpt,
	} {
		delete(common.OptionMap, key)
	}
	for key, value := range options {
		common.OptionMap[key] = value
	}
	common.OptionMapRWMutex.Unlock()

	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = backup
		common.OptionMapRWMutex.Unlock()
	})
}

func cloneCx2ccReasoningOptionMap(src map[string]string) map[string]string {
	if src == nil {
		return nil
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
