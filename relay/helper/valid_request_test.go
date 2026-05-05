package helper

import (
	"net/http"
	"net/http/httptest"
	relayconstant "one-api/relay/constant"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func newJSONValidationContext(method string, target string, body string) *gin.Context {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req
	return c
}

func TestGetAndValidateResponsesRequestRejectsWhitespaceOnlyModel(t *testing.T) {
	gin.SetMode(gin.TestMode)

	c := newJSONValidationContext(http.MethodPost, "/v1/responses", `{"model":"   ","input":"hi"}`)
	_, err := GetAndValidateResponsesRequest(c)
	if err == nil || err.Error() != "model is required" {
		t.Fatalf("GetAndValidateResponsesRequest() error = %v, want model is required", err)
	}
}

func TestGetAndValidateResponsesWebSocketRequestRejectsWhitespaceOnlyModel(t *testing.T) {
	body := []byte(`{"type":"response.create","model":"   ","input":"hi"}`)

	_, _, err := GetAndValidateResponsesWebSocketRequestBytes(body)
	if err == nil || err.Error() != "model is required" {
		t.Fatalf("GetAndValidateResponsesWebSocketRequestBytes() error = %v, want model is required", err)
	}
}

func TestGetAndValidateTextRequestRejectsWhitespaceOnlyModel(t *testing.T) {
	gin.SetMode(gin.TestMode)

	c := newJSONValidationContext(http.MethodPost, "/v1/chat/completions", `{"model":"   ","messages":[{"role":"user","content":"hi"}]}`)
	_, err := GetAndValidateTextRequest(c, relayconstant.RelayModeChatCompletions)
	if err == nil || err.Error() != "model is required" {
		t.Fatalf("GetAndValidateTextRequest() error = %v, want model is required", err)
	}
}

func TestGetAndValidateEmbeddingRequestUsesTrimmedPathModelFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)

	c := newJSONValidationContext(http.MethodPost, "/v1/engines/text-embedding-3-large/embeddings", `{"model":"   ","input":"hello"}`)
	c.Params = gin.Params{{Key: "model", Value: " text-embedding-3-large "}}

	req, err := GetAndValidateEmbeddingRequest(c, relayconstant.RelayModeEmbeddings)
	if err != nil {
		t.Fatalf("GetAndValidateEmbeddingRequest() error = %v", err)
	}
	if req.Model != "text-embedding-3-large" {
		t.Fatalf("EmbeddingRequest.Model = %q, want %q", req.Model, "text-embedding-3-large")
	}
}

func TestGetAndValidOpenAIImageRequestRejectsWhitespaceOnlyModel(t *testing.T) {
	gin.SetMode(gin.TestMode)

	c := newJSONValidationContext(http.MethodPost, "/v1/images/generations", `{"model":"   ","prompt":"draw"}`)
	_, err := GetAndValidOpenAIImageRequest(c, relayconstant.RelayModeImagesGenerations)
	if err == nil || err.Error() != "model is required" {
		t.Fatalf("GetAndValidOpenAIImageRequest() error = %v, want model is required", err)
	}
}

func TestGetAndValidateClaudeRequestRejectsWhitespaceOnlyModel(t *testing.T) {
	gin.SetMode(gin.TestMode)

	c := newJSONValidationContext(http.MethodPost, "/v1/messages", `{"model":"   ","messages":[{"role":"user","content":"hi"}]}`)
	_, err := GetAndValidateClaudeRequest(c)
	if err == nil || err.Error() != "field model is required" {
		t.Fatalf("GetAndValidateClaudeRequest() error = %v, want field model is required", err)
	}
}

func TestGetAndValidAudioRequestRejectsWhitespaceOnlySpeechModel(t *testing.T) {
	gin.SetMode(gin.TestMode)

	c := newJSONValidationContext(http.MethodPost, "/v1/audio/speech", `{"model":"   ","input":"hi","voice":"alloy"}`)
	_, err := GetAndValidAudioRequest(c, relayconstant.RelayModeAudioSpeech)
	if err == nil || err.Error() != "model is required" {
		t.Fatalf("GetAndValidAudioRequest() error = %v, want model is required", err)
	}
}
