package controller

import (
	"net/http"
	"one-api/common"
	oai_cc "one-api/relay/compat/oai_cc"

	"github.com/gin-gonic/gin"
)

// CountClaudeMessageTokens implements Anthropic-compatible:
//
//	POST /v1/messages/count_tokens
//
// This endpoint is required by some Claude clients (e.g. `/compact` flow).
// It mirrors openai-claude-main (oai-cc) behavior by using the same lightweight
// local token estimator.
func CountClaudeMessageTokens(c *gin.Context) {
	body, err := common.GetRequestBody(c)
	if err != nil {
		c.JSON(common.RequestBodyErrorStatusCode(err), gin.H{
			"type": "error",
			"error": gin.H{
				"type":    "invalid_request_error",
				"message": err.Error(),
			},
		})
		return
	}

	var req map[string]any
	if err := common.Unmarshal(body, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"type": "error",
			"error": gin.H{
				"type":    "invalid_request_error",
				"message": "Failed to parse request body",
			},
		})
		return
	}

	tokens := oai_cc.CountInputTokensLocal(req)
	c.JSON(http.StatusOK, gin.H{
		"input_tokens": tokens,
	})
}
