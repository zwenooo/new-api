package refresh

import (
	"context"
	"log"
	"strings"
	"time"

	instsvc "codex-service-go/internal/services/instances"
	proxysvc "codex-service-go/internal/services/proxy"
)

// Service periodically refreshes instance auth tokens (auth.json) to keep them valid,
// following Codex client behavior (refresh at ~8 days cadence).
type Service struct {
	instances *instsvc.Service
	proxy     *proxysvc.Service
	interval  time.Duration
	enabled   bool
	minDays   int
	maxDays   int
}

func NewService(instances *instsvc.Service, proxy *proxysvc.Service) *Service {
	// Default interval: 6 hours
	return &Service{instances: instances, proxy: proxy, interval: 6 * time.Hour, enabled: false, minDays: 8, maxDays: 8}
}

func (s *Service) SetInterval(d time.Duration) {
	if d > 0 {
		s.interval = d
	}
}

func (s *Service) SetEnabled(on bool) { s.enabled = on }
func (s *Service) IsEnabled() bool    { return s.enabled }
func (s *Service) SetRangeDays(min, max int) {
	if min > 0 {
		s.minDays = min
	}
	if max >= min {
		s.maxDays = max
	}
}

func (s *Service) Start(ctxDone <-chan struct{}) {
	// Initial sweep shortly after start
	go s.sweepOnce()
	ticker := time.NewTicker(s.interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctxDone:
				return
			case <-ticker.C:
				s.sweepOnce()
			}
		}
	}()
}

func (s *Service) sweepOnce() {
	if s == nil || s.instances == nil {
		return
	}
	if !s.enabled {
		return
	}
	list, err := s.instances.List(context.Background())
	if err != nil {
		log.Printf("[refresh] list instances: %v", err)
		return
	}
	authMetaList, err := s.instances.ListAuthMeta(context.Background())
	if err != nil {
		log.Printf("[refresh] list auth meta: %v", err)
		return
	}
	type meta struct {
		lastRefresh string
	}
	metaByID := make(map[int64]meta, len(authMetaList))
	for _, m := range authMetaList {
		metaByID[m.InstanceID] = meta{lastRefresh: m.LastRefresh}
	}
	now := time.Now()
	// 策略：每个实例在 [minDays,maxDays] 天的随机点上尝试刷新一次；
	// 兜底：超过 8 天无条件刷新（对齐 Codex CLI 的默认策略）
	const hardMaxAge = 8 * 24 * time.Hour
	for _, it := range list {
		if !it.AuthExists {
			continue
		}
		last := parseLastRefresh(metaByID[it.ID].lastRefresh)
		// 随机计划点：last + jitter( minDays..maxDays )
		jitter := randomWithinDays(s.minDays, s.maxDays)
		due := last.Add(jitter)
		// 首次无 last 的情况：立即刷新
		if last.IsZero() {
			due = now
		}
		// 兜底：超过 8 天无条件刷新
		if !last.IsZero() && now.Sub(last) >= hardMaxAge {
			due = now
		}
		if now.Before(due) {
			continue
		}
		// Use the proxy ChatGPT auth helper to perform refresh and persist last_refresh
		if s.proxy == nil {
			continue
		}
		if err := s.proxy.RefreshInstanceAuth(context.Background(), it.ID); err != nil {
			log.Printf("[refresh] instance %d refresh failed: %v", it.ID, err)
			continue
		}
		log.Printf("[refresh] instance %d auth refreshed", it.ID)
	}
}

func parseLastRefresh(raw string) time.Time {
	ts := strings.TrimSpace(raw)
	if ts == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
		return t
	}
	if t, err := time.Parse(time.RFC3339, ts); err == nil {
		return t
	}
	return time.Time{}
}

func randomWithinDays(min, max int) time.Duration {
	if max < min {
		max = min
	}
	if min <= 0 {
		min = 1
	}
	// 使用时间种子产生简易随机（避免引入额外依赖）。
	// 随机秒数：在 [minDays*86400, maxDays*86400) 区间均匀分布。
	baseMin := int64(min) * 86400
	baseMax := int64(max) * 86400
	span := baseMax - baseMin
	if span <= 0 {
		span = 1
	}
	// 取纳秒时间混洗，非强加密随机即可。
	nsec := time.Now().UnixNano()
	// 简单 LCG
	x := (nsec*1103515245 + 12345) & 0x7fffffff
	sec := baseMin + (int64(x) % span)
	return time.Duration(sec) * time.Second
}
