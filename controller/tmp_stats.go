package controller

import (
	"net/http"
	"one-api/common"
	"one-api/constant"
	"one-api/model"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	tmpRelayBillingStatsRetentionMinutes = 180
	tmpRelayBillingStatsDefaultMinutes   = 60
)

type tmpRelayBillingMinuteBucket struct {
	TimestampMinute  int64
	Total            int64
	LogicModeCount   map[string]int64
	QuotaBucketCount map[string]int64
	DecisionCount    map[string]int64
	UsingGroupCount  map[int]int64
}

type tmpRelayBillingStatsStore struct {
	mu      sync.RWMutex
	buckets map[int64]*tmpRelayBillingMinuteBucket
}

var tmpRelayBillingStats = &tmpRelayBillingStatsStore{
	buckets: make(map[int64]*tmpRelayBillingMinuteBucket, 256),
}

type tmpStatsSeries struct {
	Key    string  `json:"key"`
	Label  string  `json:"label"`
	Values []int64 `json:"values"`
}

type tmpStatsGroupItem struct {
	GroupId int    `json:"group_id"`
	Label   string `json:"label"`
	Count   int64  `json:"count"`
}

type tmpRelayBillingStatsResponse struct {
	GeneratedAt           int64               `json:"generated_at"`
	RangeMinutes          int                 `json:"range_minutes"`
	Points                []int64             `json:"points"`
	Total                 int64               `json:"total"`
	LegacyCount           int64               `json:"legacy_count"`
	NewCount              int64               `json:"new_count"`
	LogicModeSeries       []tmpStatsSeries    `json:"logic_mode_series"`
	QuotaBucketSeries     []tmpStatsSeries    `json:"quota_bucket_series"`
	DecisionSourceSeries  []tmpStatsSeries    `json:"decision_source_series"`
	TopUsingGroups        []tmpStatsGroupItem `json:"top_using_groups"`
	AvailableLogicModes   []string            `json:"available_logic_modes"`
	AvailableQuotaBuckets []string            `json:"available_quota_buckets"`
	AvailableDecisions    []string            `json:"available_decisions"`
}

func normalizeTmpRelayBillingDecisionSource(c *gin.Context, usingGroupID int) string {
	if c == nil {
		return "unknown"
	}
	tokenPrimaryGroupID := common.GetContextKeyInt(c, constant.ContextKeyTokenGroupId)
	defaultModelGroupID := common.GetContextKeyInt(c, constant.ContextKeyDefaultModelGroupId)

	switch {
	case usingGroupID > 0 && tokenPrimaryGroupID > 0 && usingGroupID == tokenPrimaryGroupID:
		return "token_primary_model_group"
	case usingGroupID > 0 && defaultModelGroupID > 0 && usingGroupID == defaultModelGroupID:
		return "default_model_group_fallback"
	default:
		return "runtime_selected_model_group"
	}
}

func normalizeTmpRelayBillingLogicMode(c *gin.Context, usingGroupID int) string {
	if c == nil {
		return "unknown_logic"
	}
	tokenPrimaryGroupID := common.GetContextKeyInt(c, constant.ContextKeyTokenGroupId)
	defaultModelGroupID := common.GetContextKeyInt(c, constant.ContextKeyDefaultModelGroupId)
	if usingGroupID > 0 && tokenPrimaryGroupID <= 0 && defaultModelGroupID > 0 && usingGroupID == defaultModelGroupID {
		return "legacy_default_model_group_logic"
	}
	if usingGroupID > 0 {
		return "new_runtime_logic"
	}
	return "unknown_logic"
}

func trimTmpRelayBillingStatsLocked(nowMinute int64) {
	if len(tmpRelayBillingStats.buckets) == 0 {
		return
	}
	cutoff := nowMinute - int64(tmpRelayBillingStatsRetentionMinutes-1)
	for ts := range tmpRelayBillingStats.buckets {
		if ts >= cutoff {
			continue
		}
		delete(tmpRelayBillingStats.buckets, ts)
	}
}

func recordTmpRelayBillingDecision(c *gin.Context, quotaBucket string, usingGroupID int) {
	quotaBucket = strings.TrimSpace(quotaBucket)
	if quotaBucket == "" {
		quotaBucket = "unknown"
	}
	decisionSource := normalizeTmpRelayBillingDecisionSource(c, usingGroupID)
	nowMinute := time.Now().Unix() / 60 * 60

	tmpRelayBillingStats.mu.Lock()
	defer tmpRelayBillingStats.mu.Unlock()

	trimTmpRelayBillingStatsLocked(nowMinute)

	bucket, ok := tmpRelayBillingStats.buckets[nowMinute]
	if !ok {
		bucket = &tmpRelayBillingMinuteBucket{
			TimestampMinute:  nowMinute,
			LogicModeCount:   make(map[string]int64, 4),
			QuotaBucketCount: make(map[string]int64, 8),
			DecisionCount:    make(map[string]int64, 8),
			UsingGroupCount:  make(map[int]int64, 16),
		}
		tmpRelayBillingStats.buckets[nowMinute] = bucket
	}

	bucket.Total++
	bucket.LogicModeCount[normalizeTmpRelayBillingLogicMode(c, usingGroupID)]++
	bucket.QuotaBucketCount[quotaBucket]++
	bucket.DecisionCount[decisionSource]++
	if usingGroupID > 0 {
		bucket.UsingGroupCount[usingGroupID]++
	}
}

func snapshotTmpRelayBillingStats(rangeMinutes int) *tmpRelayBillingStatsResponse {
	if rangeMinutes <= 0 {
		rangeMinutes = tmpRelayBillingStatsDefaultMinutes
	}
	if rangeMinutes > tmpRelayBillingStatsRetentionMinutes {
		rangeMinutes = tmpRelayBillingStatsRetentionMinutes
	}

	nowMinute := time.Now().Unix() / 60 * 60
	points := make([]int64, 0, rangeMinutes)
	for i := 0; i < rangeMinutes; i++ {
		offset := rangeMinutes - 1 - i
		points = append(points, nowMinute-int64(offset*60))
	}

	tmpRelayBillingStats.mu.RLock()
	defer tmpRelayBillingStats.mu.RUnlock()

	logicModeKeysSet := make(map[string]struct{}, 4)
	quotaKeysSet := make(map[string]struct{}, 8)
	decisionKeysSet := make(map[string]struct{}, 8)
	groupTotals := make(map[int]int64, 16)
	total := int64(0)
	legacyCount := int64(0)
	newCount := int64(0)

	for _, ts := range points {
		bucket := tmpRelayBillingStats.buckets[ts]
		if bucket == nil {
			continue
		}
		total += bucket.Total
		for key, count := range bucket.LogicModeCount {
			logicModeKeysSet[key] = struct{}{}
			switch key {
			case "legacy_default_model_group_logic":
				legacyCount += count
			case "new_runtime_logic":
				newCount += count
			}
		}
		for key := range bucket.QuotaBucketCount {
			quotaKeysSet[key] = struct{}{}
		}
		for key := range bucket.DecisionCount {
			decisionKeysSet[key] = struct{}{}
		}
		for groupID, count := range bucket.UsingGroupCount {
			groupTotals[groupID] += count
		}
	}

	logicModeKeys := make([]string, 0, len(logicModeKeysSet))
	for key := range logicModeKeysSet {
		logicModeKeys = append(logicModeKeys, key)
	}
	sort.Strings(logicModeKeys)

	quotaKeys := make([]string, 0, len(quotaKeysSet))
	for key := range quotaKeysSet {
		quotaKeys = append(quotaKeys, key)
	}
	sort.Strings(quotaKeys)

	decisionKeys := make([]string, 0, len(decisionKeysSet))
	for key := range decisionKeysSet {
		decisionKeys = append(decisionKeys, key)
	}
	sort.Strings(decisionKeys)

	buildSeries := func(keys []string, valueOf func(bucket *tmpRelayBillingMinuteBucket, key string) int64) []tmpStatsSeries {
		series := make([]tmpStatsSeries, 0, len(keys))
		for _, key := range keys {
			item := tmpStatsSeries{
				Key:    key,
				Label:  key,
				Values: make([]int64, 0, len(points)),
			}
			for _, ts := range points {
				bucket := tmpRelayBillingStats.buckets[ts]
				item.Values = append(item.Values, valueOf(bucket, key))
			}
			series = append(series, item)
		}
		return series
	}

	logicModeSeries := buildSeries(logicModeKeys, func(bucket *tmpRelayBillingMinuteBucket, key string) int64 {
		if bucket == nil {
			return 0
		}
		return bucket.LogicModeCount[key]
	})
	quotaSeries := buildSeries(quotaKeys, func(bucket *tmpRelayBillingMinuteBucket, key string) int64 {
		if bucket == nil {
			return 0
		}
		return bucket.QuotaBucketCount[key]
	})
	decisionSeries := buildSeries(decisionKeys, func(bucket *tmpRelayBillingMinuteBucket, key string) int64 {
		if bucket == nil {
			return 0
		}
		return bucket.DecisionCount[key]
	})

	groupItems := make([]tmpStatsGroupItem, 0, len(groupTotals))
	for groupID, count := range groupTotals {
		if groupID <= 0 || count <= 0 {
			continue
		}
		label, ok := model.GetGroupLabelByID(groupID)
		if !ok || strings.TrimSpace(label) == "" {
			label = "#" + strconv.Itoa(groupID)
		}
		groupItems = append(groupItems, tmpStatsGroupItem{
			GroupId: groupID,
			Label:   label,
			Count:   count,
		})
	}
	sort.Slice(groupItems, func(i, j int) bool {
		if groupItems[i].Count == groupItems[j].Count {
			return groupItems[i].GroupId < groupItems[j].GroupId
		}
		return groupItems[i].Count > groupItems[j].Count
	})
	if len(groupItems) > 12 {
		groupItems = groupItems[:12]
	}

	return &tmpRelayBillingStatsResponse{
		GeneratedAt:           time.Now().Unix(),
		RangeMinutes:          rangeMinutes,
		Points:                points,
		Total:                 total,
		LegacyCount:           legacyCount,
		NewCount:              newCount,
		LogicModeSeries:       logicModeSeries,
		QuotaBucketSeries:     quotaSeries,
		DecisionSourceSeries:  decisionSeries,
		TopUsingGroups:        groupItems,
		AvailableLogicModes:   logicModeKeys,
		AvailableQuotaBuckets: quotaKeys,
		AvailableDecisions:    decisionKeys,
	}
}

func resetTmpRelayBillingStats() {
	tmpRelayBillingStats.mu.Lock()
	defer tmpRelayBillingStats.mu.Unlock()
	tmpRelayBillingStats.buckets = make(map[int64]*tmpRelayBillingMinuteBucket, 256)
}

func GetTmpRelayBillingStats(c *gin.Context) {
	rangeMinutes := tmpRelayBillingStatsDefaultMinutes
	if raw := strings.TrimSpace(c.Query("minutes")); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 {
			rangeMinutes = v
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    snapshotTmpRelayBillingStats(rangeMinutes),
	})
}

func ResetTmpRelayBillingStats(c *gin.Context) {
	resetTmpRelayBillingStats()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}
