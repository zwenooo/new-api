package handlers

import (
	"io"
	"net/http"
)

// AggregateSSEToChatFromReader 暴露聚合能力用于回放/验证工具。
// reasoningMode: "think-tags" | "reasoning" | "both"
func AggregateSSEToChatFromReader(r io.Reader, model, reasoningMode string) (*chatCompletionsResponse, error) {
	resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(r)}
	h := &APIHandler{}
	return h.aggregateResponsesToChat(resp, model, reasoningMode)
}
