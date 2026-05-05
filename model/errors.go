package model

import "errors"

var (
	// ErrUserDailyQuotaExceeded 表示用户当日额度已耗尽。
	ErrUserDailyQuotaExceeded = errors.New("user daily quota exceeded")
	// ErrTokenDailyQuotaExceeded 表示令牌当日额度已耗尽。
	ErrTokenDailyQuotaExceeded = errors.New("token daily quota exceeded")
)
