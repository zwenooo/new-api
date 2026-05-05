package controller

import (
    "net/http"
    "one-api/common"
    "one-api/service"
    "time"

    "github.com/gin-gonic/gin"
)

type testProxyRequest struct {
    Proxy   string `json:"proxy"`
    TestURL string `json:"test_url,omitempty"`
}

// TestProxy tests reachability via a given proxy URL by issuing a lightweight HTTP request.
// Admin only. Returns success flag and basic diagnostics (status, latency).
func TestProxy(c *gin.Context) {
    var req testProxyRequest
    if err := common.UnmarshalBodyReusable(c, &req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{
            "success": false,
            "message": "invalid request body",
        })
        return
    }
    if req.Proxy == "" {
        c.JSON(http.StatusOK, gin.H{
            "success": false,
            "message": "proxy is required",
        })
        return
    }
    if req.TestURL == "" {
        // a tiny endpoint that returns 204 quickly
        req.TestURL = "https://www.gstatic.com/generate_204"
    }

    client, err := service.NewProxyHttpClient(req.Proxy)
    if err != nil {
        c.JSON(http.StatusOK, gin.H{
            "success": false,
            "message": "invalid proxy url: " + err.Error(),
        })
        return
    }

    start := time.Now()
    resp, err := client.Get(req.TestURL)
    latency := time.Since(start).Milliseconds()
    if err != nil {
        c.JSON(http.StatusOK, gin.H{
            "success": false,
            "message": err.Error(),
            "data": gin.H{
                "latency_ms": latency,
            },
        })
        return
    }
    defer service.CloseResponseBodyGracefully(resp)

    // treat 2xx/3xx/401 as reachable (401 is common for protected APIs)
    ok := resp.StatusCode/100 == 2 || resp.StatusCode/100 == 3 || resp.StatusCode == 401
    c.JSON(http.StatusOK, gin.H{
        "success": ok,
        "message": resp.Status,
        "data": gin.H{
            "latency_ms": latency,
            "status_code": resp.StatusCode,
            "url":        req.TestURL,
        },
    })
}

