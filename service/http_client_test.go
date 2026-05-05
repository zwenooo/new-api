package service

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	"one-api/common"
	"one-api/setting/system_setting"
)

func withFetchSettingSnapshot(t *testing.T) {
	t.Helper()

	fetchSetting := system_setting.GetFetchSetting()
	previous := *fetchSetting
	t.Cleanup(func() {
		*fetchSetting = previous
	})
}

func withRelayHTTPConfigSnapshot(t *testing.T) {
	t.Helper()

	prevTimeout := common.RelayTimeout
	prevHeaderTimeout := common.RelayResponseHeaderTimeout
	prevMaxIdle := common.RelayMaxIdleConns
	prevMaxIdlePerHost := common.RelayMaxIdleConnsPerHost
	t.Cleanup(func() {
		common.RelayTimeout = prevTimeout
		common.RelayResponseHeaderTimeout = prevHeaderTimeout
		common.RelayMaxIdleConns = prevMaxIdle
		common.RelayMaxIdleConnsPerHost = prevMaxIdlePerHost
		httpClient = nil
		ResetProxyClientCache()
	})
}

func TestCheckRedirectBlocksPrivateIPWhenFetchSettingDisallowsIt(t *testing.T) {
	withFetchSettingSnapshot(t)

	fetchSetting := system_setting.GetFetchSetting()
	fetchSetting.EnableSSRFProtection = true
	fetchSetting.AllowPrivateIp = false

	req := &http.Request{URL: &url.URL{Scheme: "http", Host: "127.0.0.1", Path: "/redirect"}}
	if err := checkRedirect(req, nil); err == nil {
		t.Fatal("checkRedirect() error = nil, want private-ip redirect rejection")
	}
}

func TestCheckRedirectStopsAfterTenHops(t *testing.T) {
	withFetchSettingSnapshot(t)

	fetchSetting := system_setting.GetFetchSetting()
	fetchSetting.EnableSSRFProtection = false

	req := &http.Request{URL: &url.URL{Scheme: "https", Host: "example.com", Path: "/redirect"}}
	via := make([]*http.Request, 10)
	if err := checkRedirect(req, via); err == nil || err.Error() != "stopped after 10 redirects" {
		t.Fatalf("checkRedirect() error = %v, want stopped after 10 redirects", err)
	}
}

func TestInitHttpClientBuildsWrappedClientWithRedirectChecks(t *testing.T) {
	withRelayHTTPConfigSnapshot(t)

	common.RelayTimeout = 7
	common.RelayResponseHeaderTimeout = 13
	common.RelayMaxIdleConns = 321
	common.RelayMaxIdleConnsPerHost = 123

	InitHttpClient()
	client := GetHttpClient()
	if client == nil {
		t.Fatal("GetHttpClient() = nil")
	}
	if client.Timeout != 7*time.Second {
		t.Fatalf("client.Timeout = %v, want %v", client.Timeout, 7*time.Second)
	}
	if client.CheckRedirect == nil {
		t.Fatal("client.CheckRedirect = nil, want redirect validator")
	}
	traceTransport, ok := client.Transport.(*traceRoundTripper)
	if !ok || traceTransport == nil {
		t.Fatalf("client.Transport = %T, want *traceRoundTripper", client.Transport)
	}
	baseTransport, ok := traceTransport.base.(*http.Transport)
	if !ok || baseTransport == nil {
		t.Fatalf("traceTransport.base = %T, want *http.Transport", traceTransport.base)
	}
	if baseTransport.ResponseHeaderTimeout != 13*time.Second {
		t.Fatalf("ResponseHeaderTimeout = %v, want %v", baseTransport.ResponseHeaderTimeout, 13*time.Second)
	}
	if baseTransport.MaxIdleConns != 321 {
		t.Fatalf("MaxIdleConns = %d, want 321", baseTransport.MaxIdleConns)
	}
	if baseTransport.MaxIdleConnsPerHost != 123 {
		t.Fatalf("MaxIdleConnsPerHost = %d, want 123", baseTransport.MaxIdleConnsPerHost)
	}
}

func TestNewProxyHttpClientCachesHTTPProxyClients(t *testing.T) {
	withRelayHTTPConfigSnapshot(t)

	common.RelayTimeout = 9
	common.RelayResponseHeaderTimeout = 11
	common.RelayMaxIdleConns = 55
	common.RelayMaxIdleConnsPerHost = 22

	clientA, err := NewProxyHttpClient("http://proxy.example:8080")
	if err != nil {
		t.Fatalf("NewProxyHttpClient() error = %v", err)
	}
	clientB, err := NewProxyHttpClient("http://proxy.example:8080")
	if err != nil {
		t.Fatalf("NewProxyHttpClient() second error = %v", err)
	}
	if clientA != clientB {
		t.Fatal("NewProxyHttpClient() did not reuse cached proxy client")
	}
	if clientA.CheckRedirect == nil {
		t.Fatal("proxy client CheckRedirect = nil, want redirect validator")
	}
	traceTransport, ok := clientA.Transport.(*traceRoundTripper)
	if !ok || traceTransport == nil {
		t.Fatalf("proxy client Transport = %T, want *traceRoundTripper", clientA.Transport)
	}
	baseTransport, ok := traceTransport.base.(*http.Transport)
	if !ok || baseTransport == nil {
		t.Fatalf("proxy trace base = %T, want *http.Transport", traceTransport.base)
	}
	if baseTransport.ResponseHeaderTimeout != 11*time.Second {
		t.Fatalf("ResponseHeaderTimeout = %v, want %v", baseTransport.ResponseHeaderTimeout, 11*time.Second)
	}
	if baseTransport.MaxIdleConns != 55 {
		t.Fatalf("MaxIdleConns = %d, want 55", baseTransport.MaxIdleConns)
	}
}

func TestResetProxyClientCacheDropsCachedProxyClient(t *testing.T) {
	withRelayHTTPConfigSnapshot(t)

	clientA, err := NewProxyHttpClient("http://proxy.example:8080")
	if err != nil {
		t.Fatalf("NewProxyHttpClient() error = %v", err)
	}
	ResetProxyClientCache()
	clientB, err := NewProxyHttpClient("http://proxy.example:8080")
	if err != nil {
		t.Fatalf("NewProxyHttpClient() after reset error = %v", err)
	}
	if clientA == clientB {
		t.Fatal("ResetProxyClientCache() did not force proxy client recreation")
	}
}
