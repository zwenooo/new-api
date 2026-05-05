package model

import (
	"encoding/json"
	"fmt"
	"hash/crc32"
	"math/rand"
	"one-api/common"
	"one-api/constant"
	"one-api/setting/operation_setting"
	"one-api/setting/ratio_setting"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

var groupID2model2channels map[int]map[string][]int // enabled channel
var channelsIDM map[int]*Channel                    // all channels include disabled
var channelID2groupIDs map[int][]int
var channelID2backupGroupIDs map[int][]int
var channelSyncLock sync.RWMutex

type channelStatusCacheEntry struct {
	status    int
	checkedAt time.Time
}

var channelStatusCache sync.Map

const channelStatusCacheTTL = 2 * time.Second

const channelCacheRevisionOptionKey = "channel_cache.revision"

var (
	channelCacheRevisionMu          sync.Mutex
	channelCacheLastSyncedRevision  string
	channelCacheLastSyncedRevisionT time.Time
)

func readChannelCacheRevisionFromOptionMap() string {
	common.OptionMapRWMutex.RLock()
	rev := strings.TrimSpace(common.OptionMap[channelCacheRevisionOptionKey])
	common.OptionMapRWMutex.RUnlock()
	return rev
}

func markChannelCacheSynced(rev string) {
	channelCacheRevisionMu.Lock()
	channelCacheLastSyncedRevision = strings.TrimSpace(rev)
	channelCacheLastSyncedRevisionT = time.Now()
	channelCacheRevisionMu.Unlock()
}

func getChannelCacheLastSyncedRevision() string {
	channelCacheRevisionMu.Lock()
	rev := channelCacheLastSyncedRevision
	channelCacheRevisionMu.Unlock()
	return rev
}

func BumpChannelCacheRevision() {
	rev := strconv.FormatInt(time.Now().UnixNano(), 10)
	if err := UpdateOption(channelCacheRevisionOptionKey, rev); err != nil {
		common.SysLog("failed to bump channel cache revision: " + err.Error())
	}
}

func InitChannelCache() {
	if !common.MemoryCacheEnabled {
		return
	}
	newChannelId2channel := make(map[int]*Channel)
	var channels []*Channel
	DB.Find(&channels)
	for _, channel := range channels {
		newChannelId2channel[channel.Id] = channel
	}

	var bindings []ChannelGroup
	_ = DB.Model(&ChannelGroup{}).Select("channel_id", "group_id").Find(&bindings).Error

	channelGroups := make(map[int][]int, len(bindings))
	groupSet := make(map[int]struct{}, len(bindings))
	for _, b := range bindings {
		if b.ChannelId <= 0 || b.GroupId <= 0 {
			continue
		}
		channelGroups[b.ChannelId] = append(channelGroups[b.ChannelId], b.GroupId)
		groupSet[b.GroupId] = struct{}{}
	}
	for channelID, groupIDs := range channelGroups {
		channelGroups[channelID] = normalizeUniqueSortedIDs(groupIDs)
	}

	var backupBindings []ChannelBackupGroup
	_ = DB.Model(&ChannelBackupGroup{}).Select("channel_id", "group_id").Find(&backupBindings).Error
	channelBackupGroups := make(map[int][]int, len(backupBindings))
	for _, b := range backupBindings {
		if b.ChannelId <= 0 || b.GroupId <= 0 {
			continue
		}
		channelBackupGroups[b.ChannelId] = append(channelBackupGroups[b.ChannelId], b.GroupId)
	}
	for channelID, groupIDs := range channelBackupGroups {
		channelBackupGroups[channelID] = normalizeUniqueSortedIDs(groupIDs)
	}

	newGroup2model2channels := make(map[int]map[string][]int, len(groupSet))
	for groupID := range groupSet {
		newGroup2model2channels[groupID] = make(map[string][]int)
	}
	for _, channel := range channels {
		if channel.Status != common.ChannelStatusEnabled {
			continue // skip disabled channels
		}
		groupIDs := normalizeUniqueSortedIDs(channelGroups[channel.Id])
		if len(groupIDs) == 0 {
			continue
		}
		models := strings.Split(channel.Models, ",")
		for _, groupID := range groupIDs {
			if groupID <= 0 {
				continue
			}
			model2channels := newGroup2model2channels[groupID]
			if model2channels == nil {
				model2channels = make(map[string][]int)
				newGroup2model2channels[groupID] = model2channels
			}
			for _, model := range models {
				m := strings.TrimSpace(model)
				if m == "" {
					continue
				}
				model2channels[m] = append(model2channels[m], channel.Id)
			}
		}
	}

	// sort by priority
	for groupID, model2channels := range newGroup2model2channels {
		for model, channels := range model2channels {
			sort.Slice(channels, func(i, j int) bool {
				return newChannelId2channel[channels[i]].GetPriority() > newChannelId2channel[channels[j]].GetPriority()
			})
			newGroup2model2channels[groupID][model] = channels
		}
	}

	channelSyncLock.Lock()
	groupID2model2channels = newGroup2model2channels
	channelID2groupIDs = channelGroups
	channelID2backupGroupIDs = channelBackupGroups
	//channelsIDM = newChannelId2channel
	for i, channel := range newChannelId2channel {
		if channel.ChannelInfo.IsMultiKey {
			channel.Keys = channel.GetKeys()
			if channel.ChannelInfo.MultiKeyMode == constant.MultiKeyModePolling {
				if oldChannel, ok := channelsIDM[i]; ok {
					// 存在旧的渠道，如果是多key且轮询，保留轮询索引信息
					if oldChannel.ChannelInfo.IsMultiKey && oldChannel.ChannelInfo.MultiKeyMode == constant.MultiKeyModePolling {
						channel.ChannelInfo.MultiKeyPollingIndex = oldChannel.ChannelInfo.MultiKeyPollingIndex
					}
				}
			}
		}
	}
	channelsIDM = newChannelId2channel
	channelSyncLock.Unlock()
	common.SysLog("channels synced from database")

	if err := InitChannelBindingCache(); err != nil {
		common.SysLog("failed to sync channel bindings from database: " + err.Error())
	}

	markChannelCacheSynced(readChannelCacheRevisionFromOptionMap())
}

func SyncChannelCache(frequency int) {
	for {
		time.Sleep(time.Duration(frequency) * time.Second)
		if !common.MemoryCacheEnabled {
			continue
		}
		rev := readChannelCacheRevisionFromOptionMap()
		if rev == "" || rev == getChannelCacheLastSyncedRevision() {
			continue
		}
		common.SysLog("syncing channels from database")
		InitChannelCache()
	}
}

func CacheGetRandomSatisfiedChannel(c *gin.Context, groupID int, model string, retry int) (*Channel, error) {
	return getRandomSatisfiedChannel(c, groupID, model, retry, true, nil)
}

func GroupHasEnabledModel(groupID int, model string) bool {
	if groupID <= 0 {
		return false
	}
	model = strings.TrimSpace(model)
	if model == "" {
		return false
	}

	if common.MemoryCacheEnabled {
		channelSyncLock.RLock()
		model2channels := groupID2model2channels[groupID]
		lookupModel := model
		channels := model2channels[lookupModel]
		if len(channels) == 0 {
			lookupModel = ratio_setting.FormatMatchingModelName(model)
			channels = model2channels[lookupModel]
		}
		channelSyncLock.RUnlock()
		return len(channels) > 0
	}

	var count int64
	lookupModel := model
	if err := DB.Model(&Ability{}).
		Where("group_id = ? AND model = ? AND enabled = ?", groupID, lookupModel, true).
		Count(&count).Error; err == nil && count > 0 {
		return true
	}

	lookupModel = ratio_setting.FormatMatchingModelName(model)
	if lookupModel == "" || lookupModel == model {
		return false
	}
	count = 0
	return DB.Model(&Ability{}).
		Where("group_id = ? AND model = ? AND enabled = ?", groupID, lookupModel, true).
		Count(&count).Error == nil && count > 0
}

func GroupHasMessagesToResponsesCompatChannel(groupID int, requestedModel string) (bool, error) {
	requestedModel = strings.TrimSpace(requestedModel)
	if groupID <= 0 || requestedModel == "" {
		return false, nil
	}

	channelIDs, err := getEnabledChannelIDsByGroup(groupID)
	if err != nil {
		return false, err
	}
	for _, channelID := range channelIDs {
		ch, err := getChannelForSelection(channelID)
		if err != nil {
			return false, err
		}
		if ch == nil || ch.Status != common.ChannelStatusEnabled {
			continue
		}
		supportsCompatModel, err := channelSupportsMessagesToResponsesCompatModel(ch, requestedModel)
		if err != nil {
			return false, err
		}
		if !supportsCompatModel {
			continue
		}
		return true, nil
	}
	return false, nil
}

func CacheGetRandomSatisfiedMessagesToResponsesCompatChannel(c *gin.Context, groupID int, requestedModel string, retry int) (*Channel, error) {
	if groupID <= 0 || strings.TrimSpace(requestedModel) == "" {
		return nil, nil
	}

	channelIDs, err := getEnabledChannelIDsByGroup(groupID)
	if err != nil {
		return nil, err
	}
	if len(channelIDs) == 0 {
		return nil, nil
	}

	channelIDs, err = filterChannelIDsByUserAccess(c, channelIDs)
	if err != nil {
		return nil, err
	}
	if excluded := getExcludedChannelIDSet(c); len(excluded) > 0 {
		filtered := make([]int, 0, len(channelIDs))
		for _, channelID := range channelIDs {
			if _, skip := excluded[channelID]; skip {
				continue
			}
			filtered = append(filtered, channelID)
		}
		channelIDs = filtered
	}
	if len(channelIDs) == 0 {
		return nil, nil
	}

	candidates := make([]*Channel, 0, len(channelIDs))
	for _, channelID := range channelIDs {
		ch, err := getChannelForSelection(channelID)
		if err != nil {
			return nil, err
		}
		if ch == nil || ch.Status != common.ChannelStatusEnabled {
			continue
		}
		supportsCompatModel, err := channelSupportsMessagesToResponsesCompatModel(ch, requestedModel)
		if err != nil {
			return nil, err
		}
		if !supportsCompatModel {
			continue
		}
		if !ensureChannelActive(ch) {
			continue
		}
		candidates = append(candidates, ch)
	}

	return selectCandidateChannel(c, groupID, candidates, retry)
}

func channelSupportsMessagesToResponsesCompatModel(ch *Channel, requestedModel string) (bool, error) {
	if ch == nil || !ch.GetSetting().MessagesToResponsesCompat {
		return false, nil
	}

	explicitMapping, err := channelHasExplicitModelMappingEntry(ch.GetModelMapping(), requestedModel)
	if err != nil {
		return false, err
	}
	if explicitMapping {
		return true, nil
	}

	mappedModel, isMapped, err := ResolveChannelModelMapping(ch.GetModelMapping(), requestedModel)
	if err != nil {
		return false, err
	}
	if isMapped && strings.TrimSpace(mappedModel) != "" {
		return true, nil
	}

	return channelDeclaresModel(ch, requestedModel), nil
}

func channelHasExplicitModelMappingEntry(modelMapping string, requestedModel string) (bool, error) {
	modelMapping = strings.TrimSpace(modelMapping)
	requestedModel = strings.TrimSpace(requestedModel)
	if modelMapping == "" || modelMapping == "{}" || requestedModel == "" {
		return false, nil
	}

	modelMap := make(map[string]string)
	if err := json.Unmarshal([]byte(modelMapping), &modelMap); err != nil {
		return false, ErrChannelModelMappingUnmarshal
	}

	mappedModel, exists := modelMap[requestedModel]
	return exists && strings.TrimSpace(mappedModel) != "", nil
}

func channelDeclaresModel(ch *Channel, requestedModel string) bool {
	if ch == nil {
		return false
	}
	lookupSet := buildRequestedModelLookupSet(requestedModel)
	if len(lookupSet) == 0 {
		return false
	}
	for _, channelModel := range strings.Split(ch.Models, ",") {
		channelModel = strings.TrimSpace(channelModel)
		if _, ok := lookupSet[channelModel]; ok {
			return true
		}
	}
	return false
}

func buildRequestedModelLookupSet(requestedModel string) map[string]struct{} {
	lookupModels := []string{requestedModel}
	if normalized := strings.TrimSpace(ratio_setting.FormatMatchingModelName(requestedModel)); normalized != "" && normalized != requestedModel {
		lookupModels = append(lookupModels, normalized)
	}
	lookupSet := make(map[string]struct{}, len(lookupModels))
	for _, item := range lookupModels {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		lookupSet[item] = struct{}{}
	}
	return lookupSet
}

func getEnabledChannelIDsByGroup(groupID int) ([]int, error) {
	if groupID <= 0 {
		return nil, nil
	}
	if !common.MemoryCacheEnabled {
		var ids []int
		if err := DB.Model(&Channel{}).
			Joins("JOIN channel_groups cg ON cg.channel_id = channels.id").
			Where("cg.group_id = ? AND channels.status = ?", groupID, common.ChannelStatusEnabled).
			Pluck("channels.id", &ids).Error; err != nil {
			return nil, err
		}
		return normalizeUniqueSortedIDs(ids), nil
	}

	channelSyncLock.RLock()
	model2channels := groupID2model2channels[groupID]
	channelSyncLock.RUnlock()
	if len(model2channels) == 0 {
		return nil, nil
	}

	seen := make(map[int]struct{}, 64)
	ids := make([]int, 0, 64)
	for _, channels := range model2channels {
		for _, channelID := range channels {
			if channelID <= 0 {
				continue
			}
			if _, ok := seen[channelID]; ok {
				continue
			}
			seen[channelID] = struct{}{}
			ids = append(ids, channelID)
		}
	}
	sort.Ints(ids)
	return ids, nil
}

func getChannelForSelection(channelID int) (*Channel, error) {
	if channelID <= 0 {
		return nil, nil
	}
	if common.MemoryCacheEnabled {
		channelSyncLock.RLock()
		ch := channelsIDM[channelID]
		channelSyncLock.RUnlock()
		if ch != nil {
			return ch, nil
		}
	}
	return GetChannelById(channelID, true)
}

const (
	channelSelectionWeightSmoothingFactor = 10
	channelSelectionMaxInt64              = int64(^uint64(0) >> 1)
)

func availableChannelsForSelection(channels []*Channel) ([]*Channel, error) {
	if len(channels) == 0 {
		return nil, nil
	}
	available := make([]*Channel, 0, len(channels))
	hadCandidate := false
	for _, ch := range channels {
		if ch == nil {
			continue
		}
		hadCandidate = true
		if !ChannelHasAvailableRequestSlot(ch) {
			continue
		}
		available = append(available, ch)
	}
	if len(available) == 0 && hadCandidate {
		return nil, ErrChannelConcurrencyLimitReached
	}
	return available, nil
}

func selectCandidateChannel(c *gin.Context, groupID int, channels []*Channel, retry int) (*Channel, error) {
	if len(channels) == 0 {
		return nil, nil
	}
	available, err := availableChannelsForSelection(channels)
	if err != nil {
		return nil, err
	}
	if len(available) == 0 {
		return nil, nil
	}
	if len(available) == 1 {
		return available[0], nil
	}

	stickyRetry := retry
	if stickyRetry < 0 {
		stickyRetry = 0
	}

	uniquePriorities := make(map[int64]struct{}, len(available))
	for _, ch := range available {
		if ch == nil {
			continue
		}
		uniquePriorities[ch.GetPriority()] = struct{}{}
	}
	if len(uniquePriorities) == 0 {
		return nil, nil
	}

	sortedUniquePriorities := make([]int64, 0, len(uniquePriorities))
	for priority := range uniquePriorities {
		sortedUniquePriorities = append(sortedUniquePriorities, priority)
	}
	sort.Slice(sortedUniquePriorities, func(i, j int) bool {
		return sortedUniquePriorities[i] > sortedUniquePriorities[j]
	})

	priorityRetry := stickyRetry
	if priorityRetry >= len(sortedUniquePriorities) {
		priorityRetry = len(sortedUniquePriorities) - 1
	}
	targetPriority := sortedUniquePriorities[priorityRetry]

	targetChannels := make([]*Channel, 0, len(available))
	for _, ch := range available {
		if ch != nil && ch.GetPriority() == targetPriority {
			targetChannels = append(targetChannels, ch)
		}
	}
	if len(targetChannels) == 0 {
		return nil, nil
	}

	if sticky := selectStickyChannel(c, targetChannels, groupID, stickyRetry); sticky != nil {
		return sticky, nil
	}

	totalWeight := int64(0)
	for _, ch := range targetChannels {
		baseWeight := int64(ch.GetWeight() + channelSelectionWeightSmoothingFactor)
		if baseWeight < 1 {
			baseWeight = 1
		}
		if totalWeight > channelSelectionMaxInt64-baseWeight {
			totalWeight = channelSelectionMaxInt64
		} else {
			totalWeight += baseWeight
		}
	}
	if totalWeight <= 0 {
		return targetChannels[0], nil
	}

	randomWeight := rand.Int63n(totalWeight)
	for _, ch := range targetChannels {
		randomWeight -= int64(ch.GetWeight() + channelSelectionWeightSmoothingFactor)
		if randomWeight < 0 {
			return ch, nil
		}
	}
	return targetChannels[0], nil
}

func GetChannelRetryGroupIDs(_ int, primaryGroupID int) ([]int, error) {
	if primaryGroupID <= 0 {
		return nil, nil
	}
	return []int{primaryGroupID}, nil
}

func getExcludedChannelIDSet(c *gin.Context) map[int]struct{} {
	if c == nil {
		return nil
	}
	used := c.GetStringSlice("use_channel")
	if len(used) == 0 {
		return nil
	}
	excluded := make(map[int]struct{}, len(used))
	for _, raw := range used {
		id, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil || id <= 0 {
			continue
		}
		excluded[id] = struct{}{}
	}
	if len(excluded) == 0 {
		return nil
	}
	return excluded
}

func getRandomSatisfiedChannel(c *gin.Context, groupID int, model string, retry int, checkAllowlist bool, channelFilter func(*Channel) bool) (*Channel, error) {
	if checkAllowlist {
		if !groupAllowsModel(groupID, model) {
			return nil, nil
		}
	}

	// if memory cache is disabled, get channel directly from database
	if !common.MemoryCacheEnabled {
		return getRandomSatisfiedChannelWithContextFiltered(c, groupID, model, retry, channelFilter)
	}

	// `retry` is used in two different ways:
	// 1) choose a lower-priority tier when multiple priorities exist
	// 2) rotate among channels within the same priority tier (sticky selection)
	//
	// When all candidate channels share the same priority, we still want retries to
	// dispatch to a different channel, so keep the original retry value for the
	// intra-tier rotation while clamping only the priority-tier selection.
	stickyRetry := retry
	if stickyRetry < 0 {
		stickyRetry = 0
	}

	for {
		channelSyncLock.RLock()

		model2channels := groupID2model2channels[groupID]
		lookupModel := strings.TrimSpace(model)
		channels := model2channels[lookupModel]

		// If no channels found, try to find channels with the normalized model name.
		if len(channels) == 0 {
			lookupModel = ratio_setting.FormatMatchingModelName(model)
			channels = model2channels[lookupModel]
		}
		if len(channels) == 0 {
			channelSyncLock.RUnlock()
			return nil, nil
		}

		filtered, err := filterChannelIDsByUserAccess(c, channels)
		if err != nil {
			channelSyncLock.RUnlock()
			return nil, err
		}
		channels = filtered
		if excluded := getExcludedChannelIDSet(c); len(excluded) > 0 {
			candidates := make([]int, 0, len(channels))
			for _, channelID := range channels {
				if _, skip := excluded[channelID]; skip {
					continue
				}
				candidates = append(candidates, channelID)
			}
			channels = candidates
		}
		if len(channels) == 0 {
			channelSyncLock.RUnlock()
			return nil, nil
		}

		candidateChannels := make([]*Channel, 0, len(channels))
		for _, channelId := range channels {
			ch, ok := channelsIDM[channelId]
			if !ok || ch == nil {
				channelSyncLock.RUnlock()
				return nil, fmt.Errorf("数据库一致性错误，渠道# %d 不存在，请联系管理员修复", channelId)
			}
			if channelFilter != nil && !channelFilter(ch) {
				continue
			}
			candidateChannels = append(candidateChannels, ch)
		}

		channelSyncLock.RUnlock()

		selectedChannel, err := selectCandidateChannel(c, groupID, candidateChannels, stickyRetry)
		if err != nil {
			return nil, err
		}
		if selectedChannel == nil {
			return nil, nil
		}
		if !ensureChannelActive(selectedChannel) {
			continue
		}
		return selectedChannel, nil
	}
}

func ensureChannelActive(channel *Channel) bool {
	if channel == nil {
		return false
	}
	if channel.Status != common.ChannelStatusEnabled {
		CacheUpdateChannelStatus(channel.Id, channel.Status)
		return false
	}
	if entryValue, ok := channelStatusCache.Load(channel.Id); ok {
		entry := entryValue.(channelStatusCacheEntry)
		if time.Since(entry.checkedAt) < channelStatusCacheTTL {
			if entry.status == common.ChannelStatusEnabled {
				return true
			}
			CacheUpdateChannelStatus(channel.Id, entry.status)
			return false
		}
	}
	latest, err := GetChannelById(channel.Id, true)
	if err != nil {
		return false
	}
	channelStatusCache.Store(channel.Id, channelStatusCacheEntry{
		status:    latest.Status,
		checkedAt: time.Now(),
	})
	if latest.Status != common.ChannelStatusEnabled {
		CacheUpdateChannelStatus(latest.Id, latest.Status)
		return false
	}
	CacheUpdateChannel(latest)
	return true
}

func selectStickyChannel(c *gin.Context, channels []*Channel, groupID int, retry int) *Channel {
	if c == nil || len(channels) == 0 {
		return nil
	}

	if operation_setting.GetChannelAllocationSetting().UserStickyExclusiveEnabled {
		userID := common.GetContextKeyInt(c, constant.ContextKeyUserId)
		if userID == 0 || groupID <= 0 {
			return selectStickyChannelLegacy(c, channels, retry)
		}

		now := time.Now()
		ttlSeconds := operation_setting.GetChannelAllocationSetting().UserStickyExclusiveTTLSeconds
		var ttl time.Duration
		if ttlSeconds > 0 {
			ttl = time.Duration(ttlSeconds) * time.Second
		}

		candidateIDs := make([]int, 0, len(channels))
		for _, ch := range channels {
			if ch == nil {
				continue
			}
			candidateIDs = append(candidateIDs, ch.Id)
		}
		userKey := fmt.Sprintf("%d|%d", groupID, userID)
		baseID, ok := globalUserChannelAllocator.getBaseChannelID(userKey, candidateIDs, retry == 0, now, ttl)
		if !ok || baseID == 0 {
			return nil
		}

		sortedChannels := make([]*Channel, len(channels))
		copy(sortedChannels, channels)
		sort.Slice(sortedChannels, func(i, j int) bool {
			return sortedChannels[i].Id < sortedChannels[j].Id
		})

		baseIdx := -1
		for i, ch := range sortedChannels {
			if ch.Id == baseID {
				baseIdx = i
				break
			}
		}
		if baseIdx == -1 {
			return nil
		}

		if retry > 0 && len(sortedChannels) > 1 {
			return sortedChannels[(baseIdx+retry)%len(sortedChannels)]
		}
		return sortedChannels[baseIdx]
	}

	return selectStickyChannelLegacy(c, channels, retry)
}

func selectStickyChannelLegacy(c *gin.Context, channels []*Channel, retry int) *Channel {
	if c == nil || len(channels) == 0 {
		return nil
	}
	key := common.GetContextKeyString(c, constant.ContextKeyTokenKey)
	if key == "" {
		if tokenID := common.GetContextKeyInt(c, constant.ContextKeyTokenId); tokenID != 0 {
			key = strconv.Itoa(tokenID)
		}
	}
	if key == "" {
		return nil
	}
	sortedChannels := make([]*Channel, len(channels))
	copy(sortedChannels, channels)
	sort.Slice(sortedChannels, func(i, j int) bool {
		return sortedChannels[i].Id < sortedChannels[j].Id
	})
	idx := int(crc32.ChecksumIEEE([]byte(key)) % uint32(len(sortedChannels)))
	if retry > 0 && len(sortedChannels) > 1 {
		idx = (idx + retry) % len(sortedChannels)
	}
	return sortedChannels[idx]
}

func CacheGetChannel(id int) (*Channel, error) {
	if !common.MemoryCacheEnabled {
		return GetChannelById(id, true)
	}
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	c, ok := channelsIDM[id]
	if !ok {
		return nil, fmt.Errorf("渠道# %d，已不存在", id)
	}
	return c, nil
}

func CacheGetChannelInfo(id int) (*ChannelInfo, error) {
	if !common.MemoryCacheEnabled {
		channel, err := GetChannelById(id, true)
		if err != nil {
			return nil, err
		}
		return &channel.ChannelInfo, nil
	}
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	c, ok := channelsIDM[id]
	if !ok {
		return nil, fmt.Errorf("渠道# %d，已不存在", id)
	}
	return &c.ChannelInfo, nil
}

func CacheUpdateChannelStatus(id int, status int) {
	if !common.MemoryCacheEnabled {
		return
	}
	channelSyncLock.Lock()
	defer channelSyncLock.Unlock()
	if channel, ok := channelsIDM[id]; ok {
		channel.Status = status
	}
	if status != common.ChannelStatusEnabled {
		// delete the channel from groupID2model2channels
		for groupID, model2channels := range groupID2model2channels {
			for model, channels := range model2channels {
				for i, channelId := range channels {
					if channelId == id {
						// remove the channel from the slice
						groupID2model2channels[groupID][model] = append(channels[:i], channels[i+1:]...)
						break
					}
				}
			}
		}
	}
}

func CacheUpdateChannel(channel *Channel) {
	if !common.MemoryCacheEnabled {
		return
	}
	channelSyncLock.Lock()
	defer channelSyncLock.Unlock()
	if channel == nil {
		return
	}
	channelsIDM[channel.Id] = channel
}
