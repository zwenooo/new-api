package types

import "fmt"

type GroupRatioInfo struct {
	// EffectiveGroupRatio is the real settled ratio used for enforcement/deduction.
	EffectiveGroupRatio float64
	// PublicGroupRatio is the ratio exposed to end users.
	PublicGroupRatio float64
	// PrivateGroupRatio is internal-only ratio (may include user-group overrides).
	PrivateGroupRatio float64
	// GroupRatio is kept for backward compatibility, equals EffectiveGroupRatio.
	GroupRatio float64
	// GroupSpecialRatio is kept for backward compatibility, equals PrivateGroupRatio when HasSpecialRatio=true.
	GroupSpecialRatio float64
	HasSpecialRatio   bool
	// BaseMultiplierApplied indicates whether the stored base_multiplier contributed to the effective ratio.
	BaseMultiplierApplied bool
	// Source describes which rule produced the effective ratio: public, base_multiplier, legacy, profile, override.
	Source string
}

type PriceData struct {
	ModelPrice              float64
	ModelRatio              float64
	CompletionRatio         float64
	CacheRatio              float64
	CacheCreationRatio      float64
	CacheCreation5mRatio    float64
	CacheCreation1hRatio    float64
	ImageRatio              float64
	AudioRatio              float64
	AudioCompletionRatio    float64
	UsePrice                bool
	ShouldPreConsumedQuota  int
	VisiblePreConsumedQuota int
	GroupRatioInfo          GroupRatioInfo
	ServiceTier             string
	ServiceTierMultiplier   float64
}

type PerCallPriceData struct {
	ModelPrice     float64
	Quota          int
	VisibleQuota   int
	CostQuota      int
	GroupRatioInfo GroupRatioInfo
}

func (p PriceData) ToSetting() string {
	return fmt.Sprintf("ModelPrice: %f, ModelRatio: %f, CompletionRatio: %f, CacheRatio: %f, EffectiveGroupRatio: %f, PublicGroupRatio: %f, PrivateGroupRatio: %f, BaseMultiplierApplied: %t, UsePrice: %t, CacheCreationRatio: %f, CacheCreation5mRatio: %f, CacheCreation1hRatio: %f, ShouldPreConsumedQuota: %d, VisiblePreConsumedQuota: %d, ImageRatio: %f, AudioRatio: %f, AudioCompletionRatio: %f, ServiceTier: %q, ServiceTierMultiplier: %f", p.ModelPrice, p.ModelRatio, p.CompletionRatio, p.CacheRatio, p.GroupRatioInfo.EffectiveGroupRatio, p.GroupRatioInfo.PublicGroupRatio, p.GroupRatioInfo.PrivateGroupRatio, p.GroupRatioInfo.BaseMultiplierApplied, p.UsePrice, p.CacheCreationRatio, p.CacheCreation5mRatio, p.CacheCreation1hRatio, p.ShouldPreConsumedQuota, p.VisiblePreConsumedQuota, p.ImageRatio, p.AudioRatio, p.AudioCompletionRatio, p.ServiceTier, p.ServiceTierMultiplier)
}
