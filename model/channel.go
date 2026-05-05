package model

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"one-api/common"
	"one-api/constant"
	"one-api/dto"
	"one-api/types"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/samber/lo"
	"gorm.io/gorm"
)

type Channel struct {
	Id                 int     `json:"id"`
	Type               int     `json:"type" gorm:"default:0"`
	Key                string  `json:"key" gorm:"not null"`
	OpenAIOrganization *string `json:"openai_organization"`
	TestModel          *string `json:"test_model"`
	Status             int     `json:"status" gorm:"default:1"`
	Name               string  `json:"name" gorm:"index"`
	Weight             *uint   `json:"weight" gorm:"default:0"`
	MaxConcurrency     *int    `json:"max_concurrency" gorm:"column:max_concurrency;default:-1"`
	CreatedTime        int64   `json:"created_time" gorm:"bigint"`
	TestTime           int64   `json:"test_time" gorm:"bigint"`
	ResponseTime       int     `json:"response_time"` // in milliseconds
	BaseURL            *string `json:"base_url" gorm:"column:base_url;default:''"`
	Other              string  `json:"other"`
	Balance            float64 `json:"balance"` // in USD
	BalanceUpdatedTime int64   `json:"balance_updated_time" gorm:"bigint"`
	Models             string  `json:"models"`
	Group              string  `json:"-" gorm:"type:varchar(64);default:'default'"`
	GroupIds           []int   `json:"group_ids,omitempty" gorm:"-"`
	BackupGroupIds     []int   `json:"backup_group_ids,omitempty" gorm:"-"`
	UsedQuota          int64   `json:"used_quota" gorm:"bigint;default:0"`
	VisibleUsedQuota   int64   `json:"visible_used_quota" gorm:"column:visible_used_quota;bigint;default:0"`
	CostUsedQuota      int64   `json:"cost_used_quota" gorm:"column:cost_used_quota;bigint;default:0"`
	BuyCnyPerUsd       float64 `json:"buy_cny_per_usd" gorm:"column:buy_cny_per_usd;default:0"`
	// billing_mode controls how channel financial metrics are derived:
	// - "quota": by quota (USD) conversion
	// - "request": by successful consume log count
	BillingMode         string `json:"billing_mode" gorm:"column:billing_mode;type:varchar(16);default:'quota'"`
	BuyRequestsPerCny   int    `json:"buy_requests_per_cny" gorm:"column:buy_requests_per_cny;type:int;default:0"`   // cost: ¥1 = N requests
	SellRequestsPerCny  int    `json:"sell_requests_per_cny" gorm:"column:sell_requests_per_cny;type:int;default:0"` // revenue: ¥1 = N requests
	RequestSuccessCount int64  `json:"request_success_count" gorm:"column:request_success_count;bigint;default:0"`
	// derived fields (not persisted)
	RevenueCny   *float64 `json:"revenue_cny,omitempty" gorm:"-"`
	CostCny      *float64 `json:"cost_cny,omitempty" gorm:"-"`
	ProfitCny    *float64 `json:"profit_cny,omitempty" gorm:"-"`
	ModelMapping *string  `json:"model_mapping" gorm:"type:text"`
	//MaxInputTokens     *int    `json:"max_input_tokens" gorm:"default:0"`
	StatusCodeMapping *string `json:"status_code_mapping" gorm:"type:varchar(1024);default:''"`
	Priority          *int64  `json:"priority" gorm:"bigint;default:0"`
	AutoBan           *int    `json:"auto_ban" gorm:"default:1"`
	OtherInfo         string  `json:"other_info"`
	Tag               *string `json:"tag" gorm:"index"`
	Setting           *string `json:"setting" gorm:"type:text"` // 渠道额外设置
	ParamOverride     *string `json:"param_override" gorm:"type:text"`
	HeaderOverride    *string `json:"header_override" gorm:"type:text"`
	Remark            string  `json:"remark,omitempty" gorm:"type:varchar(255)" validate:"max=255"`
	// add after v0.8.5
	ChannelInfo ChannelInfo `json:"channel_info" gorm:"type:json"`

	OtherSettings string `json:"settings" gorm:"column:settings"` // 其他设置，存储azure版本等不需要检索的信息，详见dto.ChannelOtherSettings

	// cache info
	Keys []string `json:"-" gorm:"-"`

	// Parsed from `Setting` on demand (hot-path for channel allocation).
	serviceTimePrioritiesOnce sync.Once
	serviceTimePriorities     []dto.ServiceTimePriority
	serviceTimePrioritiesErr  error
}

type ChannelInfo struct {
	IsMultiKey             bool                  `json:"is_multi_key"`                        // 是否多Key模式
	MultiKeySize           int                   `json:"multi_key_size"`                      // 多Key模式下的Key数量
	MultiKeyStatusList     map[int]int           `json:"multi_key_status_list"`               // key状态列表，key index -> status
	MultiKeyDisabledReason map[int]string        `json:"multi_key_disabled_reason,omitempty"` // key禁用原因列表，key index -> reason
	MultiKeyDisabledTime   map[int]int64         `json:"multi_key_disabled_time,omitempty"`   // key禁用时间列表，key index -> time
	MultiKeyPollingIndex   int                   `json:"multi_key_polling_index"`             // 多Key模式下轮询的key索引
	MultiKeyMode           constant.MultiKeyMode `json:"multi_key_mode"`
}

// Value implements driver.Valuer interface
func (c ChannelInfo) Value() (driver.Value, error) {
	return common.Marshal(&c)
}

// Scan implements sql.Scanner interface
func (c *ChannelInfo) Scan(value interface{}) error {
	bytesValue, _ := value.([]byte)
	return common.Unmarshal(bytesValue, c)
}

func parseChannelKeyEntry(raw json.RawMessage) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(trimmed, "\"") {
		var key string
		if err := common.Unmarshal(raw, &key); err == nil {
			return key
		}
	}
	return trimmed
}

// ParseChannelKeyList normalizes multi-key payloads while preserving raw JSON
// objects (for providers like Vertex) and unquoting plain string array entries.
func ParseChannelKeyList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []string{}
	}
	if strings.HasPrefix(raw, "[") {
		var arr []json.RawMessage
		if err := common.Unmarshal([]byte(raw), &arr); err == nil {
			keys := make([]string, len(arr))
			for i, v := range arr {
				keys[i] = parseChannelKeyEntry(v)
			}
			return keys
		}
	}
	return strings.Split(strings.Trim(raw, "\n"), "\n")
}

func (channel *Channel) GetKeys() []string {
	if channel.Key == "" {
		return []string{}
	}
	if len(channel.Keys) > 0 {
		return channel.Keys
	}
	channel.Keys = ParseChannelKeyList(channel.Key)
	return channel.Keys
}

func (channel *Channel) GetNextEnabledKey() (string, int, *types.NewAPIError) {
	// If not in multi-key mode, return the original key string directly.
	if !channel.ChannelInfo.IsMultiKey {
		return channel.Key, 0, nil
	}

	// Obtain all keys (split by \n)
	keys := channel.GetKeys()
	if len(keys) == 0 {
		// No keys available, return error, should disable the channel
		return "", 0, types.NewError(errors.New("no keys available"), types.ErrorCodeChannelNoAvailableKey)
	}

	lock := GetChannelPollingLock(channel.Id)
	lock.Lock()
	defer lock.Unlock()

	statusList := channel.ChannelInfo.MultiKeyStatusList
	// helper to get key status, default to enabled when missing
	getStatus := func(idx int) int {
		if statusList == nil {
			return common.ChannelStatusEnabled
		}
		if status, ok := statusList[idx]; ok {
			return status
		}
		return common.ChannelStatusEnabled
	}

	// Collect indexes of enabled keys
	enabledIdx := make([]int, 0, len(keys))
	for i := range keys {
		if getStatus(i) == common.ChannelStatusEnabled {
			enabledIdx = append(enabledIdx, i)
		}
	}
	// If no specific status list or none enabled, fall back to first key
	if len(enabledIdx) == 0 {
		return keys[0], 0, nil
	}

	switch channel.ChannelInfo.MultiKeyMode {
	case constant.MultiKeyModeRandom:
		// Randomly pick one enabled key
		selectedIdx := enabledIdx[rand.Intn(len(enabledIdx))]
		return keys[selectedIdx], selectedIdx, nil
	case constant.MultiKeyModePolling:
		// Use channel-specific lock to ensure thread-safe polling

		channelInfo, err := CacheGetChannelInfo(channel.Id)
		if err != nil {
			return "", 0, types.NewError(err, types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
		}
		//println("before polling index:", channel.ChannelInfo.MultiKeyPollingIndex)
		defer func() {
			if common.DebugEnabled {
				println(fmt.Sprintf("channel %d polling index: %d", channel.Id, channel.ChannelInfo.MultiKeyPollingIndex))
			}
			if !common.MemoryCacheEnabled {
				_ = channel.SaveChannelInfo()
			} else {
				// CacheUpdateChannel(channel)
			}
		}()
		// Start from the saved polling index and look for the next enabled key
		start := channelInfo.MultiKeyPollingIndex
		if start < 0 || start >= len(keys) {
			start = 0
		}
		for i := 0; i < len(keys); i++ {
			idx := (start + i) % len(keys)
			if getStatus(idx) == common.ChannelStatusEnabled {
				// update polling index for next call (point to the next position)
				channel.ChannelInfo.MultiKeyPollingIndex = (idx + 1) % len(keys)
				return keys[idx], idx, nil
			}
		}
		// Fallback – should not happen, but return first enabled key
		return keys[enabledIdx[0]], enabledIdx[0], nil
	default:
		// Unknown mode, default to first enabled key (or original key string)
		return keys[enabledIdx[0]], enabledIdx[0], nil
	}
}

func (channel *Channel) SaveChannelInfo() error {
	return DB.Model(channel).Update("channel_info", channel.ChannelInfo).Error
}

func (channel *Channel) GetModels() []string {
	if channel.Models == "" {
		return []string{}
	}
	return strings.Split(strings.Trim(channel.Models, ","), ",")
}

func filterChannelBackupGroupIDs(primaryIDs []int, backupIDs []int) []int {
	normalizedBackup := normalizeUniqueSortedIDs(backupIDs)
	if len(normalizedBackup) == 0 {
		return nil
	}
	primarySet := make(map[int]struct{}, len(primaryIDs))
	for _, id := range normalizeUniqueSortedIDs(primaryIDs) {
		if id <= 0 {
			continue
		}
		primarySet[id] = struct{}{}
	}
	filtered := make([]int, 0, len(normalizedBackup))
	for _, id := range normalizedBackup {
		if _, exists := primarySet[id]; exists {
			continue
		}
		filtered = append(filtered, id)
	}
	if len(filtered) == 0 {
		return nil
	}
	return filtered
}

func (channel *Channel) GetGroups() []string {
	if channel == nil {
		return []string{}
	}
	if channel.Id > 0 && DB != nil {
		groupIDs, err := getChannelGroupIDsTx(DB, channel.Id)
		if err == nil && len(groupIDs) > 0 {
			codes, err := GroupCodesFromIDs(DB, groupIDs)
			if err == nil && len(codes) > 0 {
				return codes
			}
		}
	}
	return []string{}
}

func (channel *Channel) GetOtherInfo() map[string]interface{} {
	otherInfo := make(map[string]interface{})
	if channel.OtherInfo != "" {
		err := common.Unmarshal([]byte(channel.OtherInfo), &otherInfo)
		if err != nil {
			common.SysLog(fmt.Sprintf("failed to unmarshal other info: channel_id=%d, tag=%s, name=%s, error=%v", channel.Id, channel.GetTag(), channel.Name, err))
		}
	}
	return otherInfo
}

func (channel *Channel) SetOtherInfo(otherInfo map[string]interface{}) {
	otherInfoBytes, err := json.Marshal(otherInfo)
	if err != nil {
		common.SysLog(fmt.Sprintf("failed to marshal other info: channel_id=%d, tag=%s, name=%s, error=%v", channel.Id, channel.GetTag(), channel.Name, err))
		return
	}
	channel.OtherInfo = string(otherInfoBytes)
}

func (channel *Channel) GetTag() string {
	if channel.Tag == nil {
		return ""
	}
	return *channel.Tag
}

func (channel *Channel) SetTag(tag string) {
	channel.Tag = &tag
}

func (channel *Channel) GetAutoBan() bool {
	if channel.AutoBan == nil {
		return false
	}
	return *channel.AutoBan == 1
}

func (channel *Channel) Save() error {
	return DB.Save(channel).Error
}

func (channel *Channel) SaveWithoutKey() error {
	return DB.Omit("key").Save(channel).Error
}

func GetAllChannels(startIdx int, num int, selectAll bool, idSort bool) ([]*Channel, error) {
	var channels []*Channel
	var err error
	order := "priority desc"
	if idSort {
		order = "id desc"
	}
	if selectAll {
		err = DB.Order(order).Find(&channels).Error
	} else {
		err = DB.Order(order).Limit(num).Offset(startIdx).Omit("key").Find(&channels).Error
	}
	return channels, err
}

func GetChannelsByTag(tag string, idSort bool) ([]*Channel, error) {
	var channels []*Channel
	order := "priority desc"
	if idSort {
		order = "id desc"
	}
	err := DB.Where("tag = ?", tag).Order(order).Find(&channels).Error
	return channels, err
}

// GetChannelsByTags returns channels for the given tag list in the same tag order as input.
// This is mainly used by tag aggregation mode listing/search to avoid N+1 queries.
//
// - It omits the sensitive `key` field by default.
// - statusFilter: -1(all), 1(enabled), 0(disabled: status != 1)
// - typeFilter: -1(all), otherwise exact match.
func GetChannelsByTags(tags []string, idSort bool, statusFilter int, typeFilter int) ([]*Channel, error) {
	if len(tags) == 0 {
		return []*Channel{}, nil
	}

	tagList := make([]string, 0, len(tags))
	seen := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		tagList = append(tagList, tag)
	}
	if len(tagList) == 0 {
		return []*Channel{}, nil
	}

	order := "priority desc"
	if idSort {
		order = "id desc"
	}

	const chunkSize = 900 // SQLite default variable limit is 999; keep some headroom.
	byTag := make(map[string][]*Channel, len(tagList))

	for start := 0; start < len(tagList); start += chunkSize {
		end := start + chunkSize
		if end > len(tagList) {
			end = len(tagList)
		}
		chunk := tagList[start:end]

		var chunkChannels []*Channel
		q := DB.Model(&Channel{}).Where("tag IN ?", chunk).Omit("key")
		if typeFilter >= 0 {
			q = q.Where("type = ?", typeFilter)
		}
		if statusFilter == common.ChannelStatusEnabled {
			q = q.Where("status = ?", common.ChannelStatusEnabled)
		} else if statusFilter == 0 {
			q = q.Where("status != ?", common.ChannelStatusEnabled)
		}
		if err := q.Order("tag asc").Order(order).Find(&chunkChannels).Error; err != nil {
			return nil, err
		}
		for _, ch := range chunkChannels {
			if ch == nil || ch.Tag == nil {
				continue
			}
			byTag[*ch.Tag] = append(byTag[*ch.Tag], ch)
		}
	}

	ordered := make([]*Channel, 0)
	for _, tag := range tagList {
		ordered = append(ordered, byTag[tag]...)
	}
	return ordered, nil
}

func SearchChannels(keyword string, groupID int, model string, idSort bool) ([]*Channel, error) {
	var channels []*Channel
	modelsCol := "`models`"

	// 如果是 PostgreSQL，使用双引号
	if common.UsingPostgreSQL {
		modelsCol = `"models"`
	}

	baseURLCol := "`base_url`"
	// 如果是 PostgreSQL，使用双引号
	if common.UsingPostgreSQL {
		baseURLCol = `"base_url"`
	}

	order := "priority desc"
	if idSort {
		order = "id desc"
	}

	// 构造基础查询
	baseQuery := DB.Model(&Channel{}).Omit("key")
	if groupID > 0 {
		baseQuery = baseQuery.Joins("JOIN channel_groups cg ON cg.channel_id = channels.id").Where("cg.group_id = ?", groupID)
	}

	// 构造WHERE子句
	var whereClause string
	var args []interface{}
	whereClause = "(id = ? OR name LIKE ? OR " + commonKeyCol + " = ? OR " + baseURLCol + " LIKE ?) AND " + modelsCol + " LIKE ?"
	args = append(args, common.String2Int(keyword), "%"+keyword+"%", keyword, "%"+keyword+"%", "%"+model+"%")

	// 执行查询
	err := baseQuery.Where(whereClause, args...).Order(order).Find(&channels).Error
	if err != nil {
		return nil, err
	}
	return channels, nil
}

func GetChannelById(id int, selectAll bool) (*Channel, error) {
	channel := &Channel{Id: id}
	var err error = nil
	if selectAll {
		err = DB.First(channel, "id = ?", id).Error
	} else {
		err = DB.Omit("key").First(channel, "id = ?", id).Error
	}
	if err != nil {
		return nil, err
	}
	if channel == nil {
		return nil, errors.New("channel not found")
	}
	return channel, nil
}

func BatchInsertChannels(channels []Channel) error {
	if len(channels) == 0 {
		return nil
	}
	tx := DB.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	for _, chunk := range lo.Chunk(channels, 50) {
		if err := tx.Omit("Group").Create(&chunk).Error; err != nil {
			tx.Rollback()
			return err
		}
		for _, channel_ := range chunk {
			if channel_.Id <= 0 {
				tx.Rollback()
				return errors.New("channel_id 无效")
			}
			if err := upsertChannelGroupsTx(tx, channel_.Id, channel_.GroupIds); err != nil {
				tx.Rollback()
				return err
			}
			if err := upsertChannelBackupGroupsTx(tx, channel_.Id, channel_.BackupGroupIds); err != nil {
				tx.Rollback()
				return err
			}
			if err := channel_.AddAbilities(tx); err != nil {
				tx.Rollback()
				return err
			}
		}
	}
	return tx.Commit().Error
}

func BatchDeleteChannels(ids []int) error {
	if len(ids) == 0 {
		return nil
	}
	// 使用事务 分批删除channel表和abilities表
	tx := DB.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	for _, chunk := range lo.Chunk(ids, 200) {
		if err := tx.Where("channel_id in (?)", chunk).Delete(&ChannelUserBinding{}).Error; err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Where("channel_id in (?)", chunk).Delete(&ChannelGroup{}).Error; err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Where("channel_id in (?)", chunk).Delete(&ChannelBackupGroup{}).Error; err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Where("channel_id in (?)", chunk).Delete(&Ability{}).Error; err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Where("id in (?)", chunk).Delete(&Channel{}).Error; err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit().Error
}

func (channel *Channel) GetPriority() int64 {
	return channel.GetEffectivePriorityAt(time.Now())
}

func (channel *Channel) GetEffectivePriorityAt(t time.Time) int64 {
	if channel == nil || channel.Priority == nil {
		if channel == nil {
			return 0
		}
		// Even when base priority is nil, allow schedules to raise/lower priority.
		return channel.getPriorityByHour(t.Hour(), 0)
	}
	return channel.getPriorityByHour(t.Hour(), *channel.Priority)
}

func (channel *Channel) getPriorityByHour(hour int, base int64) int64 {
	if channel == nil {
		return base
	}
	items, err := channel.getServiceTimePriorities()
	if err != nil || len(items) == 0 {
		return base
	}

	matched := false
	var max int64
	for _, it := range items {
		if !hourInServiceRange(hour, it.StartHour, it.EndHour) {
			continue
		}
		if !matched || it.Priority > max {
			max = it.Priority
			matched = true
		}
	}
	if matched {
		return max
	}
	return base
}

func hourInServiceRange(hour, startHour, endHour int) bool {
	if hour < 0 || hour > 23 {
		return false
	}
	if startHour < 0 || startHour > 23 || endHour < 0 || endHour > 23 {
		return false
	}
	if startHour <= endHour {
		return hour >= startHour && hour <= endHour
	}
	// Wrap across midnight, e.g. 22-6.
	return hour >= startHour || hour <= endHour
}

func (channel *Channel) getServiceTimePriorities() ([]dto.ServiceTimePriority, error) {
	if channel == nil {
		return nil, errors.New("nil channel")
	}
	channel.serviceTimePrioritiesOnce.Do(func() {
		if channel.Setting == nil || strings.TrimSpace(*channel.Setting) == "" {
			channel.serviceTimePriorities = nil
			channel.serviceTimePrioritiesErr = nil
			return
		}
		var partial struct {
			ServiceTimePriorities []dto.ServiceTimePriority `json:"service_time_priorities"`
		}
		channel.serviceTimePrioritiesErr = common.Unmarshal([]byte(*channel.Setting), &partial)
		if channel.serviceTimePrioritiesErr != nil {
			return
		}
		channel.serviceTimePriorities = partial.ServiceTimePriorities
	})
	return channel.serviceTimePriorities, channel.serviceTimePrioritiesErr
}

func (channel *Channel) GetWeight() int {
	if channel.Weight == nil {
		return 0
	}
	return int(*channel.Weight)
}

func (channel *Channel) GetMaxConcurrency() int {
	if channel.MaxConcurrency == nil {
		return -1
	}
	return *channel.MaxConcurrency
}

func (channel *Channel) GetBaseURL() string {
	if channel.BaseURL == nil {
		return ""
	}
	url := *channel.BaseURL
	if url == "" {
		url = constant.ChannelBaseURLs[channel.Type]
	}
	return url
}

func (channel *Channel) GetModelMapping() string {
	if channel.ModelMapping == nil {
		return ""
	}
	return *channel.ModelMapping
}

func (channel *Channel) GetStatusCodeMapping() string {
	if channel.StatusCodeMapping == nil {
		return ""
	}
	return *channel.StatusCodeMapping
}

func (channel *Channel) Insert() error {
	if channel == nil {
		return errors.New("nil channel")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Omit("Group").Create(channel).Error; err != nil {
			return err
		}
		if err := upsertChannelGroupsTx(tx, channel.Id, channel.GroupIds); err != nil {
			return err
		}
		if err := upsertChannelBackupGroupsTx(tx, channel.Id, channel.BackupGroupIds); err != nil {
			return err
		}
		return channel.AddAbilities(tx)
	})
}

func (channel *Channel) Update() error {
	return channel.updateWithForceColumns(nil)
}

// UpdateWithForceColumns updates the channel and forces specific columns to be updated
// even when their values are zero-values (e.g. 0 / "" / nil).
//
// This is mainly used by patch-style update endpoints where the client explicitly
// sends some fields that may legitimately be set to zero (like buy_cny_per_usd).
func (channel *Channel) UpdateWithForceColumns(forceColumns []string) error {
	return channel.updateWithForceColumns(forceColumns)
}

func (channel *Channel) updateWithForceColumns(forceColumns []string) error {
	// If this is a multi-key channel, recalculate MultiKeySize based on the current key list to avoid inconsistency after editing keys
	if channel.ChannelInfo.IsMultiKey {
		var keyStr string
		if channel.Key != "" {
			keyStr = channel.Key
		} else {
			// If key is not provided, read the existing key from the database
			if existing, err := GetChannelById(channel.Id, true); err == nil {
				keyStr = existing.Key
			}
		}
		// Parse the key list (supports newline separation or JSON array)
		keys := ParseChannelKeyList(keyStr)
		channel.ChannelInfo.MultiKeySize = len(keys)
		// Clean up status data that exceeds the new key count to prevent index out of range
		if channel.ChannelInfo.MultiKeyStatusList != nil {
			for idx := range channel.ChannelInfo.MultiKeyStatusList {
				if idx >= channel.ChannelInfo.MultiKeySize {
					delete(channel.ChannelInfo.MultiKeyStatusList, idx)
				}
			}
		}
	}
	if channel == nil {
		return errors.New("nil channel")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(channel).Omit("Group").Updates(channel).Error; err != nil {
			return err
		}
		if len(forceColumns) > 0 {
			cols := make([]string, 0, len(forceColumns))
			seen := make(map[string]struct{}, len(forceColumns))
			for _, col := range forceColumns {
				col = strings.TrimSpace(col)
				if col == "" {
					continue
				}
				if _, ok := seen[col]; ok {
					continue
				}
				seen[col] = struct{}{}
				cols = append(cols, col)
			}
			if len(cols) > 0 {
				if err := tx.Model(channel).Omit("Group").Select(cols).Updates(channel).Error; err != nil {
					return err
				}
			}
		}
		if err := upsertChannelGroupsTx(tx, channel.Id, channel.GroupIds); err != nil {
			return err
		}
		if err := upsertChannelBackupGroupsTx(tx, channel.Id, channel.BackupGroupIds); err != nil {
			return err
		}
		if err := tx.Model(channel).First(channel, "id = ?", channel.Id).Error; err != nil {
			return err
		}
		return channel.UpdateAbilities(tx)
	})
}

func (channel *Channel) UpdateResponseTime(responseTime int64) {
	err := DB.Model(channel).Select("response_time", "test_time").Updates(Channel{
		TestTime:     common.GetTimestamp(),
		ResponseTime: int(responseTime),
	}).Error
	if err != nil {
		common.SysLog(fmt.Sprintf("failed to update response time: channel_id=%d, error=%v", channel.Id, err))
	}
}

func (channel *Channel) UpdateBalance(balance float64) {
	err := DB.Model(channel).Select("balance_updated_time", "balance").Updates(Channel{
		BalanceUpdatedTime: common.GetTimestamp(),
		Balance:            balance,
	}).Error
	if err != nil {
		common.SysLog(fmt.Sprintf("failed to update balance: channel_id=%d, error=%v", channel.Id, err))
	}
}

func (channel *Channel) Delete() error {
	if channel == nil || channel.Id <= 0 {
		return errors.New("invalid channel id")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("channel_id = ?", channel.Id).Delete(&ChannelUserBinding{}).Error; err != nil {
			return err
		}
		if err := tx.Where("channel_id = ?", channel.Id).Delete(&ChannelGroup{}).Error; err != nil {
			return err
		}
		if err := tx.Where("channel_id = ?", channel.Id).Delete(&ChannelBackupGroup{}).Error; err != nil {
			return err
		}
		if err := tx.Where("channel_id = ?", channel.Id).Delete(&Ability{}).Error; err != nil {
			return err
		}
		return tx.Delete(channel).Error
	})
}

var channelStatusLock sync.Mutex

// channelPollingLocks stores locks for each channel.id to ensure thread-safe polling
var channelPollingLocks sync.Map

// GetChannelPollingLock returns or creates a mutex for the given channel ID
func GetChannelPollingLock(channelId int) *sync.Mutex {
	if lock, exists := channelPollingLocks.Load(channelId); exists {
		return lock.(*sync.Mutex)
	}
	// Create new lock for this channel
	newLock := &sync.Mutex{}
	actual, _ := channelPollingLocks.LoadOrStore(channelId, newLock)
	return actual.(*sync.Mutex)
}

// CleanupChannelPollingLocks removes locks for channels that no longer exist
// This is optional and can be called periodically to prevent memory leaks
func CleanupChannelPollingLocks() {
	var activeChannelIds []int
	DB.Model(&Channel{}).Pluck("id", &activeChannelIds)

	activeChannelSet := make(map[int]bool)
	for _, id := range activeChannelIds {
		activeChannelSet[id] = true
	}

	channelPollingLocks.Range(func(key, value interface{}) bool {
		channelId := key.(int)
		if !activeChannelSet[channelId] {
			channelPollingLocks.Delete(channelId)
		}
		return true
	})
}

func handlerMultiKeyUpdate(channel *Channel, usingKey string, status int, reason string) {
	keys := channel.GetKeys()
	if len(keys) == 0 {
		channel.Status = status
	} else {
		var keyIndex int
		for i, key := range keys {
			if key == usingKey {
				keyIndex = i
				break
			}
		}
		if channel.ChannelInfo.MultiKeyStatusList == nil {
			channel.ChannelInfo.MultiKeyStatusList = make(map[int]int)
		}
		if status == common.ChannelStatusEnabled {
			delete(channel.ChannelInfo.MultiKeyStatusList, keyIndex)
		} else {
			channel.ChannelInfo.MultiKeyStatusList[keyIndex] = status
			if channel.ChannelInfo.MultiKeyDisabledReason == nil {
				channel.ChannelInfo.MultiKeyDisabledReason = make(map[int]string)
			}
			if channel.ChannelInfo.MultiKeyDisabledTime == nil {
				channel.ChannelInfo.MultiKeyDisabledTime = make(map[int]int64)
			}
			channel.ChannelInfo.MultiKeyDisabledReason[keyIndex] = reason
			channel.ChannelInfo.MultiKeyDisabledTime[keyIndex] = common.GetTimestamp()
		}
		if len(channel.ChannelInfo.MultiKeyStatusList) >= channel.ChannelInfo.MultiKeySize {
			channel.Status = common.ChannelStatusAutoDisabled
			info := channel.GetOtherInfo()
			info["status_reason"] = "All keys are disabled"
			info["status_time"] = common.GetTimestamp()
			channel.SetOtherInfo(info)
		}
	}
}

func UpdateChannelStatus(channelId int, usingKey string, status int, reason string) bool {
	if common.MemoryCacheEnabled {
		channelStatusLock.Lock()
		defer channelStatusLock.Unlock()

		channelCache, _ := CacheGetChannel(channelId)
		if channelCache == nil {
			return false
		}
		if channelCache.ChannelInfo.IsMultiKey {
			// Use per-channel lock to prevent concurrent map read/write with GetNextEnabledKey
			pollingLock := GetChannelPollingLock(channelId)
			pollingLock.Lock()
			// 如果是多Key模式，更新缓存中的状态
			handlerMultiKeyUpdate(channelCache, usingKey, status, reason)
			newStatus := channelCache.Status
			pollingLock.Unlock()
			if newStatus != common.ChannelStatusEnabled {
				CacheUpdateChannelStatus(channelId, newStatus)
			} else {
				CacheUpdateChannel(channelCache)
			}
		} else {
			// 如果缓存渠道存在，且状态已是目标状态，直接返回
			if channelCache.Status == status {
				return false
			}
			CacheUpdateChannelStatus(channelId, status)
		}
	}

	shouldUpdateAbilities := false
	defer func() {
		if shouldUpdateAbilities {
			err := UpdateAbilityStatus(channelId, status == common.ChannelStatusEnabled)
			if err != nil {
				common.SysLog(fmt.Sprintf("failed to update ability status: channel_id=%d, error=%v", channelId, err))
			}
		}
	}()
	channel, err := GetChannelById(channelId, true)
	if err != nil {
		return false
	} else {
		if channel.Status == status {
			return false
		}

		if channel.ChannelInfo.IsMultiKey {
			beforeStatus := channel.Status
			// Protect map writes with the same per-channel lock used by readers
			pollingLock := GetChannelPollingLock(channelId)
			pollingLock.Lock()
			handlerMultiKeyUpdate(channel, usingKey, status, reason)
			pollingLock.Unlock()
			if beforeStatus != channel.Status {
				shouldUpdateAbilities = true
			}
		} else {
			info := channel.GetOtherInfo()
			info["status_reason"] = reason
			info["status_time"] = common.GetTimestamp()
			channel.SetOtherInfo(info)
			channel.Status = status
			shouldUpdateAbilities = true
		}
		err = channel.SaveWithoutKey()
		if err != nil {
			common.SysLog(fmt.Sprintf("failed to update channel status: channel_id=%d, status=%d, error=%v", channel.Id, status, err))
			return false
		}
	}
	// Broadcast cache refresh to other nodes (SyncChannelCache will pick it up).
	BumpChannelCacheRevision()
	return true
}

func EnableChannelByTag(tag string) error {
	err := DB.Model(&Channel{}).Where("tag = ?", tag).Update("status", common.ChannelStatusEnabled).Error
	if err != nil {
		return err
	}
	err = UpdateAbilityStatusByTag(tag, true)
	return err
}

func DisableChannelByTag(tag string) error {
	err := DB.Model(&Channel{}).Where("tag = ?", tag).Update("status", common.ChannelStatusManuallyDisabled).Error
	if err != nil {
		return err
	}
	err = UpdateAbilityStatusByTag(tag, false)
	return err
}

func EditChannelByTag(tag string, newTag *string, modelMapping *string, models *string, groupIDs *[]int, priority *int64, weight *uint) error {
	updateData := Channel{}
	shouldReCreateAbilities := false
	shouldUpdateGroups := false
	updatedTag := tag
	// 如果 newTag 不为空且不等于 tag，则更新 tag
	if newTag != nil && *newTag != tag {
		updateData.Tag = newTag
		updatedTag = *newTag
	}
	if modelMapping != nil && *modelMapping != "" {
		updateData.ModelMapping = modelMapping
	}
	if models != nil && *models != "" {
		shouldReCreateAbilities = true
		updateData.Models = *models
	}
	if groupIDs != nil {
		shouldReCreateAbilities = true
		shouldUpdateGroups = true
	}
	if priority != nil {
		updateData.Priority = priority
	}
	if weight != nil {
		updateData.Weight = weight
	}

	err := DB.Model(&Channel{}).Where("tag = ?", tag).Omit("Group").Updates(updateData).Error
	if err != nil {
		return err
	}
	if !shouldReCreateAbilities {
		return UpdateAbilityByTag(tag, newTag, priority, weight)
	}

	channels, err := GetChannelsByTag(updatedTag, false)
	if err != nil {
		return err
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		for _, channel := range channels {
			if channel == nil || channel.Id <= 0 {
				continue
			}
			if shouldUpdateGroups {
				if err := upsertChannelGroupsTx(tx, channel.Id, *groupIDs); err != nil {
					return err
				}
				backupGroupIDs, err := getChannelBackupGroupIDsTx(tx, channel.Id)
				if err != nil {
					return err
				}
				if err := upsertChannelBackupGroupsTx(tx, channel.Id, filterChannelBackupGroupIDs(*groupIDs, backupGroupIDs)); err != nil {
					return err
				}
			}
			if err := channel.UpdateAbilities(tx); err != nil {
				return err
			}
		}
		return nil
	})
}

func UpdateChannelUsedQuota(id int, quota int) {
	UpdateChannelUsageQuotas(id, quota, quota, 0)
}

func UpdateChannelUsageQuotas(id int, quota int, visibleQuota int, costQuota int) {
	if quota != 0 || visibleQuota != 0 || costQuota != 0 {
		AddChannelDailyUsageQuotas(id, common.GetTimestamp(), int64(quota), int64(visibleQuota), int64(costQuota))
	}
	if common.BatchUpdateEnabled {
		if quota != 0 {
			addNewRecord(BatchUpdateTypeChannelUsedQuota, id, quota)
		}
		if visibleQuota != 0 {
			addNewRecord(BatchUpdateTypeChannelVisibleUsedQuota, id, visibleQuota)
		}
		if costQuota != 0 {
			addNewRecord(BatchUpdateTypeChannelCostUsedQuota, id, costQuota)
		}
		return
	}
	updateChannelUsageQuotas(id, quota, visibleQuota, costQuota)
}

func updateChannelUsedQuota(id int, quota int) {
	err := DB.Model(&Channel{}).Where("id = ?", id).Update("used_quota", gorm.Expr("used_quota + ?", quota)).Error
	if err != nil {
		common.SysLog(fmt.Sprintf("failed to update channel used quota: channel_id=%d, delta_quota=%d, error=%v", id, quota, err))
	}
}

func UpdateChannelCostUsedQuota(id int, quota int) {
	UpdateChannelUsageQuotas(id, 0, 0, quota)
}

func updateChannelCostUsedQuota(id int, quota int) {
	err := DB.Model(&Channel{}).Where("id = ?", id).Update("cost_used_quota", gorm.Expr("cost_used_quota + ?", quota)).Error
	if err != nil {
		common.SysLog(fmt.Sprintf("failed to update channel cost used quota: channel_id=%d, delta_quota=%d, error=%v", id, quota, err))
	}
}

func updateChannelVisibleUsedQuota(id int, quota int) {
	err := DB.Model(&Channel{}).Where("id = ?", id).Update("visible_used_quota", gorm.Expr("visible_used_quota + ?", quota)).Error
	if err != nil {
		common.SysLog(fmt.Sprintf("failed to update channel visible used quota: channel_id=%d, delta_quota=%d, error=%v", id, quota, err))
	}
}

func updateChannelUsageQuotas(id int, quota int, visibleQuota int, costQuota int) {
	updates := make(map[string]interface{}, 3)
	if quota != 0 {
		updates["used_quota"] = gorm.Expr("used_quota + ?", quota)
	}
	if visibleQuota != 0 {
		updates["visible_used_quota"] = gorm.Expr("visible_used_quota + ?", visibleQuota)
	}
	if costQuota != 0 {
		updates["cost_used_quota"] = gorm.Expr("cost_used_quota + ?", costQuota)
	}
	if len(updates) == 0 {
		return
	}
	err := DB.Model(&Channel{}).Where("id = ?", id).Updates(updates).Error
	if err != nil {
		common.SysLog(fmt.Sprintf("failed to update channel usage quotas: channel_id=%d, delta_quota=%d, delta_visible_quota=%d, delta_cost_quota=%d, error=%v", id, quota, visibleQuota, costQuota, err))
	}
}

func ResetChannelUsedQuota(id int) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&Channel{}).Where("id = ?", id).Updates(map[string]interface{}{
			"used_quota":            0,
			"visible_used_quota":    0,
			"cost_used_quota":       0,
			"request_success_count": 0,
		}).Error; err != nil {
			return err
		}
		if err := tx.Where("channel_id = ?", id).Delete(&ChannelRequestDailyStat{}).Error; err != nil {
			return err
		}
		return nil
	})
}

func DeleteChannelByStatus(status int64) (int64, error) {
	var ids []int
	if err := DB.Model(&Channel{}).Where("status = ?", status).Pluck("id", &ids).Error; err != nil {
		return 0, err
	}
	if err := BatchDeleteChannels(ids); err != nil {
		return 0, err
	}
	return int64(len(ids)), nil
}

func DeleteDisabledChannel() (int64, error) {
	var ids []int
	if err := DB.Model(&Channel{}).
		Where("status = ? or status = ?", common.ChannelStatusAutoDisabled, common.ChannelStatusManuallyDisabled).
		Pluck("id", &ids).Error; err != nil {
		return 0, err
	}
	if err := BatchDeleteChannels(ids); err != nil {
		return 0, err
	}
	return int64(len(ids)), nil
}

func GetPaginatedTags(offset int, limit int) ([]*string, error) {
	var tags []*string
	err := DB.Model(&Channel{}).Select("DISTINCT tag").Where("tag != ''").Offset(offset).Limit(limit).Find(&tags).Error
	return tags, err
}

func GetPaginatedTagsWithFilters(offset int, limit int, statusFilter int, typeFilter int) ([]*string, error) {
	var tags []*string
	q := DB.Model(&Channel{}).
		Select("DISTINCT tag").
		Where("tag is not null AND tag != ''")
	if typeFilter >= 0 {
		q = q.Where("type = ?", typeFilter)
	}
	if statusFilter == common.ChannelStatusEnabled {
		q = q.Where("status = ?", common.ChannelStatusEnabled)
	} else if statusFilter == 0 {
		q = q.Where("status != ?", common.ChannelStatusEnabled)
	}
	if err := q.Offset(offset).Limit(limit).Find(&tags).Error; err != nil {
		return nil, err
	}
	return tags, nil
}

func CountTagsWithFilters(statusFilter int, typeFilter int) (int64, error) {
	var total int64
	q := DB.Model(&Channel{}).
		Where("tag is not null AND tag != ''")
	if typeFilter >= 0 {
		q = q.Where("type = ?", typeFilter)
	}
	if statusFilter == common.ChannelStatusEnabled {
		q = q.Where("status = ?", common.ChannelStatusEnabled)
	} else if statusFilter == 0 {
		q = q.Where("status != ?", common.ChannelStatusEnabled)
	}
	if err := q.Distinct("tag").Count(&total).Error; err != nil {
		return 0, err
	}
	return total, nil
}

func SearchTags(keyword string, groupID int, model string, idSort bool) ([]*string, error) {
	var tags []*string
	modelsCol := "`models`"

	// 如果是 PostgreSQL，使用双引号
	if common.UsingPostgreSQL {
		modelsCol = `"models"`
	}

	baseURLCol := "`base_url`"
	// 如果是 PostgreSQL，使用双引号
	if common.UsingPostgreSQL {
		baseURLCol = `"base_url"`
	}

	order := "priority desc"
	if idSort {
		order = "id desc"
	}

	// 构造基础查询
	baseQuery := DB.Model(&Channel{}).Omit("key")
	if groupID > 0 {
		baseQuery = baseQuery.Joins("JOIN channel_groups cg ON cg.channel_id = channels.id").Where("cg.group_id = ?", groupID)
	}

	// 构造WHERE子句
	var whereClause string
	var args []interface{}
	whereClause = "(id = ? OR name LIKE ? OR " + commonKeyCol + " = ? OR " + baseURLCol + " LIKE ?) AND " + modelsCol + " LIKE ?"
	args = append(args, common.String2Int(keyword), "%"+keyword+"%", keyword, "%"+keyword+"%", "%"+model+"%")

	subQuery := baseQuery.Where(whereClause, args...).
		Select("tag").
		Where("tag != ''").
		Order(order)

	err := DB.Table("(?) as sub", subQuery).
		Select("DISTINCT tag").
		Find(&tags).Error

	if err != nil {
		return nil, err
	}

	return tags, nil
}

func (channel *Channel) ValidateSettings() error {
	channelParams := &dto.ChannelSettings{}
	if channel.Setting != nil && *channel.Setting != "" {
		err := common.Unmarshal([]byte(*channel.Setting), channelParams)
		if err != nil {
			return err
		}
	}
	if channelParams.MessagesToResponsesCompat && !ChannelSupportsMessagesToResponsesCompat(channel.Type) {
		return fmt.Errorf("messages_to_responses_compat 仅支持 OpenAI/Azure/Custom/OpenRouter/Xinference 渠道")
	}
	if err := validateChannelServiceTimePriorities(channelParams.ServiceTimePriorities); err != nil {
		return err
	}
	return nil
}

func validateChannelServiceTimePriorities(items []dto.ServiceTimePriority) error {
	if len(items) == 0 {
		return nil
	}
	// By-hour coverage map, each hour represents [h:00, h+1:00).
	coveredBy := make([]int, 24)
	for i := range coveredBy {
		coveredBy[i] = -1
	}
	for idx, it := range items {
		if it.StartHour < 0 || it.StartHour > 23 {
			return fmt.Errorf("service_time_priorities[%d].start_hour 必须在 0-23 之间", idx)
		}
		if it.EndHour < 0 || it.EndHour > 23 {
			return fmt.Errorf("service_time_priorities[%d].end_hour 必须在 0-23 之间", idx)
		}
		if it.Priority < 0 {
			return fmt.Errorf("service_time_priorities[%d].priority 必须大于等于 0", idx)
		}

		mark := func(h int) error {
			if h < 0 || h > 23 {
				return fmt.Errorf("service_time_priorities[%d] 包含无效小时: %d", idx, h)
			}
			if prev := coveredBy[h]; prev >= 0 {
				return fmt.Errorf("service_time_priorities 时间段重叠：第 %d 项与第 %d 项在 %02d:00-%02d:00 重叠", prev+1, idx+1, h, (h+1)%24)
			}
			coveredBy[h] = idx
			return nil
		}

		if it.StartHour <= it.EndHour {
			for h := it.StartHour; h <= it.EndHour; h++ {
				if err := mark(h); err != nil {
					return err
				}
			}
			continue
		}

		// Wrap across midnight, e.g. 22-6.
		for h := it.StartHour; h <= 23; h++ {
			if err := mark(h); err != nil {
				return err
			}
		}
		for h := 0; h <= it.EndHour; h++ {
			if err := mark(h); err != nil {
				return err
			}
		}
	}
	return nil
}

func (channel *Channel) GetSetting() dto.ChannelSettings {
	setting := dto.ChannelSettings{}
	if channel.Setting != nil && *channel.Setting != "" {
		err := common.Unmarshal([]byte(*channel.Setting), &setting)
		if err != nil {
			common.SysLog(fmt.Sprintf("failed to unmarshal setting: channel_id=%d, error=%v", channel.Id, err))
			channel.Setting = nil // 清空设置以避免后续错误
			_ = channel.Save()    // 保存修改
		}
	}
	return setting
}

func (channel *Channel) SetSetting(setting dto.ChannelSettings) {
	settingBytes, err := common.Marshal(setting)
	if err != nil {
		common.SysLog(fmt.Sprintf("failed to marshal setting: channel_id=%d, error=%v", channel.Id, err))
		return
	}
	channel.Setting = common.GetPointer[string](string(settingBytes))
}

func (channel *Channel) GetOtherSettings() dto.ChannelOtherSettings {
	setting := dto.ChannelOtherSettings{}
	if channel.OtherSettings != "" {
		err := common.UnmarshalJsonStr(channel.OtherSettings, &setting)
		if err != nil {
			common.SysLog(fmt.Sprintf("failed to unmarshal setting: channel_id=%d, error=%v", channel.Id, err))
			channel.OtherSettings = "{}" // 清空设置以避免后续错误
			_ = channel.Save()           // 保存修改
		}
	}
	return setting
}

func (channel *Channel) SetOtherSettings(setting dto.ChannelOtherSettings) {
	settingBytes, err := common.Marshal(setting)
	if err != nil {
		common.SysLog(fmt.Sprintf("failed to marshal setting: channel_id=%d, error=%v", channel.Id, err))
		return
	}
	channel.OtherSettings = string(settingBytes)
}

func (channel *Channel) GetParamOverride() map[string]interface{} {
	paramOverride := make(map[string]interface{})
	if channel.ParamOverride != nil && *channel.ParamOverride != "" {
		err := common.Unmarshal([]byte(*channel.ParamOverride), &paramOverride)
		if err != nil {
			common.SysLog(fmt.Sprintf("failed to unmarshal param override: channel_id=%d, error=%v", channel.Id, err))
		}
	}
	return paramOverride
}

func (channel *Channel) GetHeaderOverride() map[string]interface{} {
	headerOverride := make(map[string]interface{})
	if channel.HeaderOverride != nil && *channel.HeaderOverride != "" {
		err := common.Unmarshal([]byte(*channel.HeaderOverride), &headerOverride)
		if err != nil {
			common.SysLog(fmt.Sprintf("failed to unmarshal header override: channel_id=%d, error=%v", channel.Id, err))
		}
	}
	return headerOverride
}

func GetChannelsByIds(ids []int) ([]*Channel, error) {
	var channels []*Channel
	err := DB.Where("id in (?)", ids).Find(&channels).Error
	return channels, err
}

func BatchSetChannelTag(ids []int, tag *string) error {
	// 开启事务
	tx := DB.Begin()
	if tx.Error != nil {
		return tx.Error
	}

	// 更新标签
	err := tx.Model(&Channel{}).Where("id in (?)", ids).Update("tag", tag).Error
	if err != nil {
		tx.Rollback()
		return err
	}

	// update ability status
	channels, err := GetChannelsByIds(ids)
	if err != nil {
		tx.Rollback()
		return err
	}

	for _, channel := range channels {
		err = channel.UpdateAbilities(tx)
		if err != nil {
			tx.Rollback()
			return err
		}
	}

	// 提交事务
	return tx.Commit().Error
}

// BatchResetChannelUsedQuota 将指定渠道的已用额度批量重置为 0。
func BatchResetChannelUsedQuota(ids []int) (int64, error) {
	var affected int64
	err := DB.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&Channel{}).Where("id in (?)", ids).Updates(map[string]interface{}{
			"used_quota":            0,
			"visible_used_quota":    0,
			"cost_used_quota":       0,
			"request_success_count": 0,
		})
		affected = result.RowsAffected
		if result.Error != nil {
			return result.Error
		}
		if err := tx.Where("channel_id IN ?", ids).Delete(&ChannelRequestDailyStat{}).Error; err != nil {
			return err
		}
		return nil
	})
	return affected, err
}

// BatchSetChannelGroupIDs 为指定渠道批量设置分组，并根据新的分组与模型重建能力表。
func BatchSetChannelGroupIDs(ids []int, groupIDs []int) error {
	tx := DB.Begin()
	if tx.Error != nil {
		return tx.Error
	}

	var channels []*Channel
	if err := tx.Where("id in (?)", ids).Find(&channels).Error; err != nil {
		tx.Rollback()
		return err
	}

	for _, channel := range channels {
		if err := upsertChannelGroupsTx(tx, channel.Id, groupIDs); err != nil {
			tx.Rollback()
			return err
		}
		backupGroupIDs, err := getChannelBackupGroupIDsTx(tx, channel.Id)
		if err != nil {
			tx.Rollback()
			return err
		}
		if err := upsertChannelBackupGroupsTx(tx, channel.Id, filterChannelBackupGroupIDs(groupIDs, backupGroupIDs)); err != nil {
			tx.Rollback()
			return err
		}
		if err := channel.UpdateAbilities(tx); err != nil {
			tx.Rollback()
			return err
		}
	}

	return tx.Commit().Error
}

// BatchUpdateChannelModels 在不破坏已有配置的前提下，为指定渠道批量添加或移除模型。
// addModels / removeModels 中的模型名会被去重、去空白；不存在的模型在移除时不会报错。
func BatchUpdateChannelModels(ids []int, addModels, removeModels []string) (int, error) {
	cleanList := func(list []string) []string {
		seen := make(map[string]struct{})
		var result []string
		for _, raw := range list {
			name := strings.TrimSpace(raw)
			if name == "" {
				continue
			}
			if _, exists := seen[name]; exists {
				continue
			}
			seen[name] = struct{}{}
			result = append(result, name)
		}
		return result
	}

	add := cleanList(addModels)
	remove := cleanList(removeModels)

	if len(add) == 0 && len(remove) == 0 {
		return 0, nil
	}

	tx := DB.Begin()
	if tx.Error != nil {
		return 0, tx.Error
	}

	var channels []*Channel
	if err := tx.Where("id in (?)", ids).Find(&channels).Error; err != nil {
		tx.Rollback()
		return 0, err
	}

	updatedCount := 0

	for _, channel := range channels {
		// 构建当前模型集合
		modelSet := make(map[string]struct{})
		if channel.Models != "" {
			for _, m := range strings.Split(strings.Trim(channel.Models, ","), ",") {
				name := strings.TrimSpace(m)
				if name == "" {
					continue
				}
				modelSet[name] = struct{}{}
			}
		}

		// 先移除
		for _, m := range remove {
			delete(modelSet, m)
		}
		// 再添加
		for _, m := range add {
			modelSet[m] = struct{}{}
		}

		// 生成新的 models 字符串（按字典序排序，保证稳定性）
		newModels := make([]string, 0, len(modelSet))
		for m := range modelSet {
			newModels = append(newModels, m)
		}
		sort.Strings(newModels)
		modelsStr := strings.Join(newModels, ",")

		if modelsStr == channel.Models {
			continue
		}

		if err := tx.Model(&Channel{}).
			Where("id = ?", channel.Id).
			Update("models", modelsStr).Error; err != nil {
			tx.Rollback()
			return updatedCount, err
		}

		channel.Models = modelsStr
		if err := channel.UpdateAbilities(tx); err != nil {
			tx.Rollback()
			return updatedCount, err
		}
		updatedCount++
	}

	if err := tx.Commit().Error; err != nil {
		return updatedCount, err
	}
	return updatedCount, nil
}

// CountAllChannels returns total channels in DB
func CountAllChannels() (int64, error) {
	var total int64
	err := DB.Model(&Channel{}).Count(&total).Error
	return total, err
}

// CountAllTags returns number of non-empty distinct tags
func CountAllTags() (int64, error) {
	var total int64
	err := DB.Model(&Channel{}).Where("tag is not null AND tag != ''").Distinct("tag").Count(&total).Error
	return total, err
}

// Get channels of specified type with pagination
func GetChannelsByType(startIdx int, num int, idSort bool, channelType int) ([]*Channel, error) {
	var channels []*Channel
	order := "priority desc"
	if idSort {
		order = "id desc"
	}
	err := DB.Where("type = ?", channelType).Order(order).Limit(num).Offset(startIdx).Omit("key").Find(&channels).Error
	return channels, err
}

// Count channels of specific type
func CountChannelsByType(channelType int) (int64, error) {
	var count int64
	err := DB.Model(&Channel{}).Where("type = ?", channelType).Count(&count).Error
	return count, err
}

// Return map[type]count for all channels
func CountChannelsGroupByType() (map[int64]int64, error) {
	type result struct {
		Type  int64 `gorm:"column:type"`
		Count int64 `gorm:"column:count"`
	}
	var results []result
	err := DB.Model(&Channel{}).Select("type, count(*) as count").Group("type").Find(&results).Error
	if err != nil {
		return nil, err
	}
	counts := make(map[int64]int64)
	for _, r := range results {
		counts[r.Type] = r.Count
	}
	return counts, nil
}
