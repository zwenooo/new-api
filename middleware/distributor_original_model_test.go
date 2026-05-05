package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"one-api/common"
	"one-api/constant"

	"github.com/gin-gonic/gin"
)

func TestDistributeSeedsOriginalModelForDeferredResponsesSelection(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var gotOriginalModel string

	router := gin.New()
	router.Use(func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyUsingGroupId, 101)
		c.Next()
	})
	router.Use(Distribute())
	router.POST("/v1/responses", func(c *gin.Context) {
		gotOriginalModel = common.GetContextKeyString(c, constant.ContextKeyOriginalModel)
		c.Status(http.StatusNoContent)
	})

	body := []byte(`{"model":"gpt-5.2-codex","input":"hi"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
	if gotOriginalModel != "gpt-5.2-codex" {
		t.Fatalf("original_model = %q, want %q", gotOriginalModel, "gpt-5.2-codex")
	}
}

func TestDistributeRejectsWhitespaceOnlyResponsesModel(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyUsingGroupId, 101)
		c.Next()
	})
	router.Use(Distribute())
	router.POST("/v1/responses", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	body := []byte(`{"model":"   ","input":"hi"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "模型名称不能为空") {
		t.Fatalf("response body = %q, want contains %q", rec.Body.String(), "模型名称不能为空")
	}
}

func TestGetModelRequestTreatsWhitespaceModerationsModelAsDefault(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/v1/moderations", bytes.NewReader([]byte(`{"model":"   ","input":"hi"}`)))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	modelRequest, shouldSelectChannel, err := getModelRequest(c)
	if err != nil {
		t.Fatalf("getModelRequest() error = %v", err)
	}
	if !shouldSelectChannel {
		t.Fatalf("shouldSelectChannel = false, want true")
	}
	if modelRequest == nil {
		t.Fatal("modelRequest = nil, want non-nil")
	}
	if modelRequest.Model != "text-moderation-stable" {
		t.Fatalf("modelRequest.Model = %q, want %q", modelRequest.Model, "text-moderation-stable")
	}
}

func TestSeedOriginalModelContextDoesNotOverrideExistingValue(t *testing.T) {
	gin.SetMode(gin.TestMode)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(c, constant.ContextKeyOriginalModel, "existing-model")

	seedOriginalModelContext(c, "new-model")

	if got := common.GetContextKeyString(c, constant.ContextKeyOriginalModel); got != "existing-model" {
		t.Fatalf("original_model = %q, want %q", got, "existing-model")
	}
}
