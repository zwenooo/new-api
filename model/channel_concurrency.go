package model

import (
	"errors"
	"fmt"
	"one-api/common"
	"sync"
)

var ErrChannelConcurrencyLimitReached = errors.New("all candidate channels reached max_concurrency")

func IsChannelConcurrencyLimitReachedErr(err error) bool {
	return errors.Is(err, ErrChannelConcurrencyLimitReached)
}

type channelConcurrencyLimiter struct {
	channelID int
	mu        sync.Mutex
	inFlight  int
}

func (l *channelConcurrencyLimiter) TryAcquire(limit int) bool {
	if limit <= 0 {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.inFlight >= limit {
		return false
	}
	l.inFlight++
	return true
}

func (l *channelConcurrencyLimiter) HasAvailable(limit int) bool {
	if limit <= 0 {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.inFlight < limit
}

func (l *channelConcurrencyLimiter) Release() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.inFlight <= 0 {
		common.SysError(fmt.Sprintf("channel concurrency release underflow: channel=%d", l.channelID))
		return
	}
	l.inFlight--
}

var channelConcurrencyLimiters sync.Map

func getChannelConcurrencyLimiter(channelID int) *channelConcurrencyLimiter {
	if channelID <= 0 {
		return nil
	}
	if v, ok := channelConcurrencyLimiters.Load(channelID); ok {
		return v.(*channelConcurrencyLimiter)
	}
	limiter := &channelConcurrencyLimiter{channelID: channelID}
	actual, _ := channelConcurrencyLimiters.LoadOrStore(channelID, limiter)
	return actual.(*channelConcurrencyLimiter)
}

func ChannelHasAvailableConcurrency(channelID int, limit int) bool {
	if channelID <= 0 || limit <= 0 {
		return true
	}
	limiter := getChannelConcurrencyLimiter(channelID)
	if limiter == nil {
		return true
	}
	return limiter.HasAvailable(limit)
}

func TryAcquireChannelConcurrency(channelID int, limit int) bool {
	if channelID <= 0 || limit <= 0 {
		return true
	}
	limiter := getChannelConcurrencyLimiter(channelID)
	if limiter == nil {
		return true
	}
	return limiter.TryAcquire(limit)
}

func ReleaseChannelConcurrency(channelID int) {
	if channelID <= 0 {
		return
	}
	limiter := getChannelConcurrencyLimiter(channelID)
	if limiter == nil {
		return
	}
	limiter.Release()
}

func ChannelHasAvailableRequestSlot(channel *Channel) bool {
	if channel == nil {
		return false
	}
	return ChannelHasAvailableConcurrency(channel.Id, channel.GetMaxConcurrency())
}

func TryAcquireChannelRequestSlot(channel *Channel) bool {
	if channel == nil {
		return false
	}
	return TryAcquireChannelConcurrency(channel.Id, channel.GetMaxConcurrency())
}
