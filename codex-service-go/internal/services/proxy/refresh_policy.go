package proxy

import "sync/atomic"

var tokenRefreshEnabled atomic.Bool

func TokenRefreshEnabled() bool {
	return tokenRefreshEnabled.Load()
}

func SetTokenRefreshEnabled(enabled bool) {
	tokenRefreshEnabled.Store(enabled)
}
