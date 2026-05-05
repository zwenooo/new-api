package types

import (
	"errors"
	"net/http"
	"testing"
)

func TestToOpenAIErrorMapsBadRequestToInvalidRequestType(t *testing.T) {
	err := NewErrorWithStatusCode(errors.New("model missing"), ErrorCodeModelNotFound, http.StatusBadRequest, ErrOptionWithSkipRetry())

	openaiErr := err.ToOpenAIError()
	if openaiErr.Type != "invalid_request_error" {
		t.Fatalf("ToOpenAIError type = %q, want %q", openaiErr.Type, "invalid_request_error")
	}
	if openaiErr.Code != ErrorCodeModelNotFound {
		t.Fatalf("ToOpenAIError code = %#v, want %#v", openaiErr.Code, ErrorCodeModelNotFound)
	}
}

func TestToOpenAIErrorKeepsServerErrorsAsTransferAPIError(t *testing.T) {
	err := NewErrorWithStatusCode(errors.New("db broken"), ErrorCodeModelNotFound, http.StatusServiceUnavailable, ErrOptionWithSkipRetry())

	openaiErr := err.ToOpenAIError()
	if openaiErr.Type != string(ErrorTypeNewAPIError) {
		t.Fatalf("ToOpenAIError type = %q, want %q", openaiErr.Type, string(ErrorTypeNewAPIError))
	}
}
