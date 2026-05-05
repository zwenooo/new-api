package controller

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestCollectEpayCallbackParamsSupportsGetQuery(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	req := httptest.NewRequest("GET", "/api/payg/epay/notify?trade_no=TRADE123&money=12.34", nil)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req

	params := collectEpayCallbackParams(c)
	if got := params["trade_no"]; got != "TRADE123" {
		t.Fatalf("trade_no = %q, want %q", got, "TRADE123")
	}
	if got := params["money"]; got != "12.34" {
		t.Fatalf("money = %q, want %q", got, "12.34")
	}
}

func TestCollectEpayCallbackParamsSupportsPostForm(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	body := strings.NewReader("trade_no=TRADE456&money=56.78&trade_status=TRADE_SUCCESS")
	req := httptest.NewRequest("POST", "/api/payg/epay/notify?sign=query-sign", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req

	params := collectEpayCallbackParams(c)
	if got := params["trade_no"]; got != "TRADE456" {
		t.Fatalf("trade_no = %q, want %q", got, "TRADE456")
	}
	if got := params["money"]; got != "56.78" {
		t.Fatalf("money = %q, want %q", got, "56.78")
	}
	if got := params["trade_status"]; got != "TRADE_SUCCESS" {
		t.Fatalf("trade_status = %q, want %q", got, "TRADE_SUCCESS")
	}
	if got := params["sign"]; got != "query-sign" {
		t.Fatalf("sign = %q, want %q", got, "query-sign")
	}
}
