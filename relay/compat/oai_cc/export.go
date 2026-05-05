package oai_cc

// CountInputTokensLocal estimates the input tokens for an Anthropic Messages request.
//
// It is a lightweight approximation ported from openai-claude-main (oai-cc) and is
// used by the Anthropic-compatible `/v1/messages/count_tokens` endpoint.
func CountInputTokensLocal(anthropicReq map[string]any) int {
	return countInputTokensLocal(anthropicReq)
}
