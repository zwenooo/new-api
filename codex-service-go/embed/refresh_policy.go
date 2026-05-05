package embed

import proxysvc "codex-service-go/internal/services/proxy"

type RefreshError = proxysvc.RefreshError

func TokenRefreshEnabled() bool {
	return proxysvc.TokenRefreshEnabled()
}

func SetTokenRefreshEnabled(enabled bool) {
	proxysvc.SetTokenRefreshEnabled(enabled)
}

