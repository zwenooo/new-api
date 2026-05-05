package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"one-api/common"
	"one-api/constant"
	"one-api/dto"
	"one-api/model"
	"one-api/service"
	"one-api/setting/payg_setting"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type OpenAIModel struct {
	ID         string `json:"id"`
	Object     string `json:"object"`
	Created    int64  `json:"created"`
	OwnedBy    string `json:"owned_by"`
	Permission []struct {
		ID                 string `json:"id"`
		Object             string `json:"object"`
		Created            int64  `json:"created"`
		AllowCreateEngine  bool   `json:"allow_create_engine"`
		AllowSampling      bool   `json:"allow_sampling"`
		AllowLogprobs      bool   `json:"allow_logprobs"`
		AllowSearchIndices bool   `json:"allow_search_indices"`
		AllowView          bool   `json:"allow_view"`
		AllowFineTuning    bool   `json:"allow_fine_tuning"`
		Organization       string `json:"organization"`
		Group              string `json:"group"`
		IsBlocking         bool   `json:"is_blocking"`
	} `json:"permission"`
	Root   string `json:"root"`
	Parent string `json:"parent"`
}

type OpenAIModelsResponse struct {
	Data    []OpenAIModel `json:"data"`
	Success bool          `json:"success"`
}

type ollamaModelTagsResponse struct {
	Models []struct {
		Name  string `json:"name"`
		Model string `json:"model"`
	} `json:"models"`
}

type geminiModelListResponse struct {
	Models []struct {
		Name interface{} `json:"name"`
	} `json:"models"`
	NextPageToken string `json:"nextPageToken"`
}

func isFetchHeaderPassthroughRuleKey(key string) bool {
	key = strings.TrimSpace(key)
	if key == "" {
		return false
	}
	if key == "*" {
		return true
	}
	lower := strings.ToLower(key)
	return strings.HasPrefix(lower, "re:") || strings.HasPrefix(lower, "regex:")
}

func buildFetchModelsHeaders(channel *model.Channel, key string) (http.Header, error) {
	headers := http.Header{}
	if channel != nil && channel.Type == constant.ChannelTypeAnthropic {
		if key != "" {
			headers.Set("x-api-key", key)
		}
		headers.Set("anthropic-version", "2023-06-01")
	} else if key != "" {
		headers.Set("Authorization", "Bearer "+key)
	}

	if channel == nil {
		return headers, nil
	}
	headerOverride := channel.GetHeaderOverride()
	for k, v := range headerOverride {
		if isFetchHeaderPassthroughRuleKey(k) {
			continue
		}
		str, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("invalid header override for key %s", k)
		}
		if strings.Contains(str, "{api_key}") {
			str = strings.ReplaceAll(str, "{api_key}", key)
		}
		headers.Set(k, str)
	}
	return headers, nil
}

func hasFetchModelsHeaderOverride(channel *model.Channel, headerName string) bool {
	if channel == nil {
		return false
	}
	headerName = strings.TrimSpace(headerName)
	if headerName == "" {
		return false
	}
	for key := range channel.GetHeaderOverride() {
		if isFetchHeaderPassthroughRuleKey(key) {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(key), headerName) {
			return true
		}
	}
	return false
}

func buildFetchModelsURL(channelType int, baseURL string) string {
	switch channelType {
	case constant.ChannelTypeOllama:
		return fmt.Sprintf("%s/api/tags", baseURL)
	case constant.ChannelTypeGemini:
		return fmt.Sprintf("%s/v1beta/openai/models", baseURL)
	case constant.ChannelTypeAli:
		return fmt.Sprintf("%s/compatible-mode/v1/models", baseURL)
	case constant.ChannelTypeZhipu_v4:
		return fmt.Sprintf("%s/api/paas/v4/models", baseURL)
	default:
		return fmt.Sprintf("%s/v1/models", baseURL)
	}
}

func splitFetchModelsKeys(raw string) []string {
	return model.ParseChannelKeyList(raw)
}

func normalizeFetchModelsKeys(keys []string) []string {
	if len(keys) == 0 {
		return []string{""}
	}
	normalized := make([]string, 0, len(keys))
	seen := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, key)
	}
	if len(normalized) == 0 {
		return []string{""}
	}
	return normalized
}

func resolveFetchModelsKeys(channel *model.Channel, keyOverride string) ([]string, error) {
	overrideKeys := splitFetchModelsKeys(keyOverride)
	if len(overrideKeys) > 0 {
		return normalizeFetchModelsKeys(overrideKeys), nil
	}
	if channel == nil {
		return nil, fmt.Errorf("channel is nil")
	}

	keys := channel.GetKeys()
	if !channel.ChannelInfo.IsMultiKey {
		return normalizeFetchModelsKeys(keys), nil
	}

	enabledKeys := make([]string, 0, len(keys))
	for idx, key := range keys {
		if channel.ChannelInfo.MultiKeyStatusList != nil {
			if status, ok := channel.ChannelInfo.MultiKeyStatusList[idx]; ok && status != common.ChannelStatusEnabled {
				continue
			}
		}
		enabledKeys = append(enabledKeys, key)
	}
	if len(enabledKeys) == 0 && len(keys) > 0 {
		enabledKeys = append(enabledKeys, keys[0])
	}
	return normalizeFetchModelsKeys(enabledKeys), nil
}

func fetchGeminiModelIDs(channel *model.Channel, baseURL string, key string) ([]string, error) {
	nextPageToken := ""
	maxPages := 100
	allModels := make([]string, 0)

	for page := 0; page < maxPages; page++ {
		requestURL := fmt.Sprintf("%s/v1beta/models", baseURL)
		if nextPageToken != "" {
			requestURL = fmt.Sprintf("%s?pageToken=%s", requestURL, url.QueryEscape(nextPageToken))
		}
		headers, err := buildFetchModelsHeaders(channel, key)
		if err != nil {
			return nil, err
		}
		if !hasFetchModelsHeaderOverride(channel, "Authorization") {
			headers.Del("Authorization")
		}
		if key != "" && headers.Get("x-goog-api-key") == "" {
			headers.Set("x-goog-api-key", key)
		}
		body, err := GetResponseBody(http.MethodGet, requestURL, channel, headers)
		if err != nil {
			return nil, err
		}

		result := geminiModelListResponse{}
		if err = common.Unmarshal(body, &result); err != nil {
			return nil, err
		}
		for _, item := range result.Models {
			modelName, ok := item.Name.(string)
			if !ok {
				continue
			}
			modelName = strings.TrimPrefix(strings.TrimSpace(modelName), "models/")
			if modelName == "" {
				continue
			}
			allModels = append(allModels, modelName)
		}
		nextPageToken = strings.TrimSpace(result.NextPageToken)
		if nextPageToken == "" {
			break
		}
	}

	return normalizeModelNames(allModels), nil
}

func fetchUpstreamModelIDsForKey(channel *model.Channel, baseURL string, key string) ([]string, error) {
	if channel == nil {
		return nil, fmt.Errorf("channel is nil")
	}
	if channel.Type == constant.ChannelTypeGemini {
		return fetchGeminiModelIDs(channel, baseURL, key)
	}

	requestURL := buildFetchModelsURL(channel.Type, baseURL)
	headers, err := buildFetchModelsHeaders(channel, key)
	if err != nil {
		return nil, err
	}
	body, err := GetResponseBody(http.MethodGet, requestURL, channel, headers)
	if err != nil {
		return nil, err
	}

	if channel.Type == constant.ChannelTypeOllama {
		result := ollamaModelTagsResponse{}
		if err = common.Unmarshal(body, &result); err != nil {
			return nil, err
		}
		ids := make([]string, 0, len(result.Models))
		for _, item := range result.Models {
			name := strings.TrimSpace(item.Name)
			if name == "" {
				name = strings.TrimSpace(item.Model)
			}
			if name == "" {
				continue
			}
			ids = append(ids, name)
		}
		return normalizeModelNames(ids), nil
	}

	result := OpenAIModelsResponse{}
	if err = common.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	ids := make([]string, 0, len(result.Data))
	for _, item := range result.Data {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			continue
		}
		ids = append(ids, id)
	}
	return normalizeModelNames(ids), nil
}

func aggregateFetchedUpstreamModelIDs(perKeyModels [][]string) []string {
	if len(perKeyModels) == 0 {
		return []string{}
	}
	merged := normalizeModelNames(perKeyModels[0])
	for _, models := range perKeyModels[1:] {
		// A multi-key channel may route any request to any enabled key, so only
		// the models shared by all candidate keys are safe to publish back into
		// channel.Models / abilities_v2.
		merged = intersectModelNames(merged, models)
	}
	return normalizeModelNames(merged)
}

func fetchUpstreamModelIDsForChannel(
	channel *model.Channel,
	baseURLOverride string,
	keyOverride string,
	proxyOverride string,
) ([]string, error) {
	if channel == nil {
		return nil, fmt.Errorf("channel is nil")
	}

	fetchChannel := *channel
	if strings.TrimSpace(proxyOverride) != "" {
		setting := fetchChannel.GetSetting()
		setting.Proxy = strings.TrimSpace(proxyOverride)
		fetchChannel.SetSetting(setting)
	}

	baseURL := strings.TrimSpace(baseURLOverride)
	if baseURL == "" {
		baseURL = strings.TrimSpace(fetchChannel.GetBaseURL())
	}
	if baseURL == "" && fetchChannel.Type >= 0 && fetchChannel.Type < len(constant.ChannelBaseURLs) {
		baseURL = constant.ChannelBaseURLs[fetchChannel.Type]
	}
	if baseURL == "" {
		return nil, fmt.Errorf("missing base url for channel type %d", fetchChannel.Type)
	}

	keys, err := resolveFetchModelsKeys(&fetchChannel, keyOverride)
	if err != nil {
		return nil, err
	}

	perKeyModels := make([][]string, 0, len(keys))
	for _, key := range keys {
		modelIDs, fetchErr := fetchUpstreamModelIDsForKey(&fetchChannel, baseURL, key)
		if fetchErr != nil {
			return nil, fetchErr
		}
		perKeyModels = append(perKeyModels, modelIDs)
	}
	return aggregateFetchedUpstreamModelIDs(perKeyModels), nil
}

func parseStatusFilter(statusParam string) int {
	switch strings.ToLower(statusParam) {
	case "enabled", "1":
		return common.ChannelStatusEnabled
	case "disabled", "0":
		return 0
	default:
		return -1
	}
}

func normalizeChannelBackupGroupIDs(primaryIDs []int, backupIDs []int) ([]int, error) {
	normalized := model.NormalizeUniqueSortedIDs(backupIDs)
	if len(normalized) == 0 {
		return nil, nil
	}
	if err := model.ValidateGroupIDsExist(nil, normalized); err != nil {
		return nil, err
	}
	if len(primaryIDs) == 0 {
		return normalized, nil
	}
	primarySet := make(map[int]struct{}, len(primaryIDs))
	for _, id := range primaryIDs {
		if id <= 0 {
			continue
		}
		primarySet[id] = struct{}{}
	}
	filtered := make([]int, 0, len(normalized))
	for _, id := range normalized {
		if _, exists := primarySet[id]; exists {
			continue
		}
		filtered = append(filtered, id)
	}
	if len(filtered) == 0 {
		return nil, nil
	}
	return filtered, nil
}

func clearChannelInfo(channel *model.Channel) {
	if channel.ChannelInfo.IsMultiKey {
		channel.ChannelInfo.MultiKeyDisabledReason = nil
		channel.ChannelInfo.MultiKeyDisabledTime = nil
	}
}

type channelBoundUserSnapshot struct {
	Id           int    `json:"id"`
	Username     string `json:"username,omitempty"`
	DisplayName  string `json:"display_name,omitempty"`
	PlanType     string `json:"plan_type,omitempty"`
	PlanExpireAt int64  `json:"plan_expire_at,omitempty"`
}

type channelWithBindings struct {
	*model.Channel
	BoundUserCount int                        `json:"bound_user_count"`
	BoundUsers     []channelBoundUserSnapshot `json:"bound_users,omitempty"`
}

type channelBoundUserRow struct {
	ChannelId    int    `gorm:"column:channel_id"`
	UserId       int    `gorm:"column:user_id"`
	Username     string `gorm:"column:username"`
	DisplayName  string `gorm:"column:display_name"`
	PlanType     string `gorm:"column:plan_type"`
	PlanExpireAt int64  `gorm:"column:plan_expire_at"`
}

func wrapChannelsWithBindings(channels []*model.Channel, includeUsers bool) ([]channelWithBindings, error) {
	if len(channels) == 0 {
		return []channelWithBindings{}, nil
	}

	channelIDs := make([]int, 0, len(channels))
	seen := make(map[int]struct{}, len(channels))
	for _, ch := range channels {
		if ch == nil || ch.Id <= 0 {
			continue
		}
		if _, ok := seen[ch.Id]; ok {
			continue
		}
		seen[ch.Id] = struct{}{}
		channelIDs = append(channelIDs, ch.Id)
	}
	if len(channelIDs) == 0 {
		return []channelWithBindings{}, nil
	}

	items := make([]channelWithBindings, 0, len(channels))

	if includeUsers {
		var rows []channelBoundUserRow
		if err := model.DB.
			Table("channel_user_bindings b").
			Select("b.channel_id, b.user_id, u.username, u.display_name, u.plan_type, u.plan_expire_at").
			Joins("LEFT JOIN users u ON u.id = b.user_id").
			Where("b.channel_id IN ?", channelIDs).
			Order("b.channel_id ASC, b.user_id ASC").
			Find(&rows).Error; err != nil {
			return nil, err
		}

		boundUsersByChannelID := make(map[int][]channelBoundUserSnapshot, len(channelIDs))
		for _, row := range rows {
			if row.ChannelId <= 0 || row.UserId <= 0 {
				continue
			}
			boundUsersByChannelID[row.ChannelId] = append(boundUsersByChannelID[row.ChannelId], channelBoundUserSnapshot{
				Id:           row.UserId,
				Username:     strings.TrimSpace(row.Username),
				DisplayName:  strings.TrimSpace(row.DisplayName),
				PlanType:     strings.TrimSpace(row.PlanType),
				PlanExpireAt: row.PlanExpireAt,
			})
		}

		for _, ch := range channels {
			if ch == nil {
				continue
			}
			boundUsers := boundUsersByChannelID[ch.Id]
			items = append(items, channelWithBindings{
				Channel:        ch,
				BoundUserCount: len(boundUsers),
				BoundUsers:     boundUsers,
			})
		}
		return items, nil
	}

	counts, err := model.GetChannelBoundUserCounts(channelIDs)
	if err != nil {
		return nil, err
	}
	for _, ch := range channels {
		if ch == nil {
			continue
		}
		items = append(items, channelWithBindings{
			Channel:        ch,
			BoundUserCount: counts[ch.Id],
		})
	}
	return items, nil
}

func GetAllChannels(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	channelData := make([]*model.Channel, 0)
	idSort, _ := strconv.ParseBool(c.Query("id_sort"))
	enableTagMode, _ := strconv.ParseBool(c.Query("tag_mode"))
	statusParam := c.Query("status")
	// statusFilter: -1 all, 1 enabled, 0 disabled (include auto & manual)
	statusFilter := parseStatusFilter(statusParam)
	// type filter
	typeStr := c.Query("type")
	typeFilter := -1
	if typeStr != "" {
		if t, err := strconv.Atoi(typeStr); err == nil {
			typeFilter = t
		}
	}

	var total int64

	if enableTagMode {
		tags, err := model.GetPaginatedTagsWithFilters(pageInfo.GetStartIdx(), pageInfo.GetPageSize(), statusFilter, typeFilter)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
			return
		}
		tagList := make([]string, 0, len(tags))
		for _, tag := range tags {
			if tag == nil || *tag == "" {
				continue
			}
			tagList = append(tagList, *tag)
		}
		if len(tagList) > 0 {
			channelData, err = model.GetChannelsByTags(tagList, idSort, statusFilter, typeFilter)
			if err != nil {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
				return
			}
		}
		total, err = model.CountTagsWithFilters(statusFilter, typeFilter)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
			return
		}
	} else {
		baseQuery := model.DB.Model(&model.Channel{})
		if typeFilter >= 0 {
			baseQuery = baseQuery.Where("type = ?", typeFilter)
		}
		if statusFilter == common.ChannelStatusEnabled {
			baseQuery = baseQuery.Where("status = ?", common.ChannelStatusEnabled)
		} else if statusFilter == 0 {
			baseQuery = baseQuery.Where("status != ?", common.ChannelStatusEnabled)
		}

		baseQuery.Count(&total)

		order := "priority desc"
		if idSort {
			order = "id desc"
		}

		err := baseQuery.Order(order).Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Omit("key").Find(&channelData).Error
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
			return
		}
	}

	if err := model.FillChannelsGroupIDs(channelData); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.FillChannelsBackupGroupIDs(channelData); err != nil {
		common.ApiError(c, err)
		return
	}

	for _, datum := range channelData {
		clearChannelInfo(datum)
	}
	service.FillChannelsFinancialFields(channelData)
	includeBoundUsers, _ := strconv.ParseBool(c.Query("include_bound_users"))
	items, err := wrapChannelsWithBindings(channelData, includeBoundUsers)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	countQuery := model.DB.Model(&model.Channel{})
	if statusFilter == common.ChannelStatusEnabled {
		countQuery = countQuery.Where("status = ?", common.ChannelStatusEnabled)
	} else if statusFilter == 0 {
		countQuery = countQuery.Where("status != ?", common.ChannelStatusEnabled)
	}
	var results []struct {
		Type  int64
		Count int64
	}
	_ = countQuery.Select("type, count(*) as count").Group("type").Find(&results).Error
	typeCounts := make(map[int64]int64)
	for _, r := range results {
		typeCounts[r.Type] = r.Count
	}
	common.ApiSuccess(c, gin.H{
		"items":       items,
		"total":       total,
		"page":        pageInfo.GetPage(),
		"page_size":   pageInfo.GetPageSize(),
		"type_counts": typeCounts,
	})
	return
}

func FetchUpstreamModels(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}

	channel, err := model.GetChannelById(id, true)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	ids, err := fetchUpstreamModelIDsForChannel(channel, "", "", "")
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": fmt.Sprintf("获取模型列表失败: %s", err.Error()),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    ids,
	})
}

func FixChannelsAbilities(c *gin.Context) {
	success, fails, err := model.FixAbility()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"success": success,
			"fails":   fails,
		},
	})
}

func SearchChannels(c *gin.Context) {
	keyword := c.Query("keyword")
	groupID := 0
	if raw := strings.TrimSpace(c.Query("group_id")); raw != "" {
		id, err := strconv.Atoi(raw)
		if err != nil || id <= 0 {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "group_id 无效",
			})
			return
		}
		groupID = id
	}
	modelKeyword := c.Query("model")
	statusParam := c.Query("status")
	statusFilter := parseStatusFilter(statusParam)
	idSort, _ := strconv.ParseBool(c.Query("id_sort"))
	enableTagMode, _ := strconv.ParseBool(c.Query("tag_mode"))
	channelData := make([]*model.Channel, 0)
	if enableTagMode {
		tags, err := model.SearchTags(keyword, groupID, modelKeyword, idSort)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}

		tagList := make([]string, 0, len(tags))
		for _, tag := range tags {
			if tag == nil || *tag == "" {
				continue
			}
			tagList = append(tagList, *tag)
		}
		if len(tagList) > 0 {
			channelData, err = model.GetChannelsByTags(tagList, idSort, statusFilter, -1)
			if err != nil {
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": err.Error(),
				})
				return
			}
		}
	} else {
		channels, err := model.SearchChannels(keyword, groupID, modelKeyword, idSort)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
		channelData = channels
	}

	if statusFilter == common.ChannelStatusEnabled || statusFilter == 0 {
		filtered := make([]*model.Channel, 0, len(channelData))
		for _, ch := range channelData {
			if statusFilter == common.ChannelStatusEnabled && ch.Status != common.ChannelStatusEnabled {
				continue
			}
			if statusFilter == 0 && ch.Status == common.ChannelStatusEnabled {
				continue
			}
			filtered = append(filtered, ch)
		}
		channelData = filtered
	}

	if err := model.FillChannelsGroupIDs(channelData); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.FillChannelsBackupGroupIDs(channelData); err != nil {
		common.ApiError(c, err)
		return
	}

	// calculate type counts for search results
	typeCounts := make(map[int64]int64)
	for _, channel := range channelData {
		typeCounts[int64(channel.Type)]++
	}

	typeParam := c.Query("type")
	typeFilter := -1
	if typeParam != "" {
		if tp, err := strconv.Atoi(typeParam); err == nil {
			typeFilter = tp
		}
	}

	if typeFilter >= 0 {
		filtered := make([]*model.Channel, 0, len(channelData))
		for _, ch := range channelData {
			if ch.Type == typeFilter {
				filtered = append(filtered, ch)
			}
		}
		channelData = filtered
	}

	page, _ := strconv.Atoi(c.DefaultQuery("p", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}

	total := len(channelData)
	startIdx := (page - 1) * pageSize
	if startIdx > total {
		startIdx = total
	}
	endIdx := startIdx + pageSize
	if endIdx > total {
		endIdx = total
	}

	pagedData := channelData[startIdx:endIdx]

	for _, datum := range pagedData {
		clearChannelInfo(datum)
	}
	service.FillChannelsFinancialFields(pagedData)

	includeBoundUsers, _ := strconv.ParseBool(c.Query("include_bound_users"))
	items, err := wrapChannelsWithBindings(pagedData, includeBoundUsers)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"items":       items,
			"total":       total,
			"type_counts": typeCounts,
		},
	})
	return
}

func GetChannel(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	channel, err := model.GetChannelById(id, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if channel != nil {
		groupIDs, err := model.GetChannelGroupIDs(channel.Id)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		if len(groupIDs) == 0 && channel.Status == common.ChannelStatusEnabled {
			common.ApiErrorMsg(c, "渠道缺少分组绑定")
			return
		}
		channel.GroupIds = groupIDs
		channel.BackupGroupIds, err = model.GetChannelBackupGroupIDs(channel.Id)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		clearChannelInfo(channel)
		channel.BillingMode = model.NormalizeChannelBillingMode(channel.BillingMode)
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    channel,
	})
	return
}

func GetChannelProfitDailyStats(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if id <= 0 {
		common.ApiErrorMsg(c, "渠道ID无效")
		return
	}

	channel, err := model.GetChannelById(id, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if channel == nil {
		common.ApiErrorMsg(c, "渠道不存在")
		return
	}

	// Default range: last 30 days (inclusive).
	now := time.Now().In(time.Local)
	startTime := now.AddDate(0, 0, -29)
	endTime := now

	granularity := strings.ToLower(strings.TrimSpace(c.Query("granularity")))
	if granularity == "" {
		granularity = "day"
	}
	switch granularity {
	case "day", "week", "month":
	default:
		common.ApiErrorMsg(c, "granularity 无效，应为 day/week/month")
		return
	}

	parseDate := func(raw string) (time.Time, error) {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return time.Time{}, fmt.Errorf("empty date")
		}
		return time.ParseInLocation("2006-01-02", raw, time.Local)
	}

	if startStr := strings.TrimSpace(c.Query("start")); startStr != "" {
		tm, err := parseDate(startStr)
		if err != nil {
			common.ApiErrorMsg(c, "start 日期格式错误，应为 YYYY-MM-DD")
			return
		}
		startTime = tm
	}
	if endStr := strings.TrimSpace(c.Query("end")); endStr != "" {
		tm, err := parseDate(endStr)
		if err != nil {
			common.ApiErrorMsg(c, "end 日期格式错误，应为 YYYY-MM-DD")
			return
		}
		endTime = tm
	}

	startDay := common.DateToInt(startTime)
	endDay := common.DateToInt(endTime)
	if startDay > endDay {
		common.ApiErrorMsg(c, "start 不能大于 end")
		return
	}

	rows, err := model.ListChannelRequestDailyStats(id, startDay, endDay)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	byDay := make(map[int]*model.ChannelRequestDailyStat, len(rows))
	for _, r := range rows {
		if r == nil {
			continue
		}
		byDay[r.Day] = r
	}

	mode := model.NormalizeChannelBillingMode(strings.TrimSpace(channel.BillingMode))
	buyRate := channel.BuyRequestsPerCny
	sellRate := channel.SellRequestsPerCny
	buyCnyPerUsd := channel.BuyCnyPerUsd
	creditUsdPerCny := payg_setting.GetPaygSettings().CreditUsdPerCny

	type item struct {
		Day           int      `json:"day"`
		BucketEndDay  int      `json:"bucket_end_day,omitempty"`
		SuccessCount  int64    `json:"success_count"`
		UsedQuota     int64    `json:"used_quota"`
		CostUsedQuota int64    `json:"cost_used_quota"`
		RevenueCny    *float64 `json:"revenue_cny,omitempty"`
		CostCny       *float64 `json:"cost_cny,omitempty"`
		ProfitCny     *float64 `json:"profit_cny,omitempty"`
		ProfitRate    *float64 `json:"profit_rate,omitempty"`
	}

	items := make([]item, 0)
	totalSuccess := int64(0)
	totalUsedQuota := int64(0)
	totalCostUsedQuota := int64(0)
	totalRevenue := 0.0
	totalCost := 0.0

	startDate := time.Date(startTime.In(time.Local).Year(), startTime.In(time.Local).Month(), startTime.In(time.Local).Day(), 0, 0, 0, 0, time.Local)
	endDate := time.Date(endTime.In(time.Local).Year(), endTime.In(time.Local).Month(), endTime.In(time.Local).Day(), 0, 0, 0, 0, time.Local)

	startOfISOWeek := func(t time.Time) time.Time {
		local := t.In(time.Local)
		dayStart := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.Local)
		weekday := int(dayStart.Weekday())
		offset := (weekday + 6) % 7 // Monday=0, Sunday=6
		return dayStart.AddDate(0, 0, -offset)
	}

	bucketEndFor := func(bucketStart time.Time) time.Time {
		switch granularity {
		case "day":
			return bucketStart
		case "week":
			ws := startOfISOWeek(bucketStart)
			we := ws.AddDate(0, 0, 6)
			if we.After(endDate) {
				return endDate
			}
			return we
		case "month":
			local := bucketStart.In(time.Local)
			ms := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, time.Local)
			me := ms.AddDate(0, 1, 0).AddDate(0, 0, -1)
			if me.After(endDate) {
				return endDate
			}
			return me
		default:
			// unreachable because validated above
			return bucketStart
		}
	}

	cur := startDate
	for !cur.After(endDate) {
		bucketStart := cur
		bucketEnd := bucketEndFor(bucketStart)

		cnt := int64(0)
		usedQuota := int64(0)
		costUsedQuota := int64(0)
		dayCursor := bucketStart
		for !dayCursor.After(bucketEnd) {
			day := common.DateToInt(dayCursor)
			stat := byDay[day]
			if stat != nil {
				cnt += stat.SuccessCount
				usedQuota += stat.UsedQuota
				costUsedQuota += stat.CostUsedQuota
			}
			dayCursor = dayCursor.AddDate(0, 0, 1)
		}

		row := item{
			Day:           common.DateToInt(bucketStart),
			SuccessCount:  cnt,
			UsedQuota:     usedQuota,
			CostUsedQuota: costUsedQuota,
		}
		if bucketEnd.After(bucketStart) {
			row.BucketEndDay = common.DateToInt(bucketEnd)
		}

		totalSuccess += cnt
		totalUsedQuota += usedQuota
		totalCostUsedQuota += costUsedQuota

		var revenue *float64
		var cost *float64

		if mode == model.ChannelBillingModeRequest {
			if buyRate > 0 && sellRate > 0 {
				r := float64(cnt) / float64(sellRate)
				c := float64(cnt) / float64(buyRate)
				revenue = &r
				cost = &c
			}
		} else {
			// quota mode
			if creditUsdPerCny > 0 {
				revenueUsd := float64(usedQuota) / common.QuotaPerUnit
				r := revenueUsd / creditUsdPerCny
				revenue = &r
			}
			if buyCnyPerUsd > 0 {
				costUsd := float64(costUsedQuota) / common.QuotaPerUnit
				c := costUsd * buyCnyPerUsd
				cost = &c
			}
		}

		row.RevenueCny = revenue
		row.CostCny = cost
		if revenue != nil && cost != nil {
			p := *revenue - *cost
			row.ProfitCny = &p
			if *revenue > 0 {
				pr := p / *revenue
				row.ProfitRate = &pr
			}
			totalRevenue += *revenue
			totalCost += *cost
		}

		items = append(items, row)
		cur = bucketEnd.AddDate(0, 0, 1)
	}

	totalProfit := totalRevenue - totalCost
	totalProfitRate := 0.0
	if totalRevenue > 0 {
		totalProfitRate = totalProfit / totalRevenue
	}

	common.ApiSuccess(c, gin.H{
		"channel_id":            id,
		"granularity":           granularity,
		"billing_mode":          mode,
		"buy_requests_per_cny":  buyRate,
		"sell_requests_per_cny": sellRate,
		"buy_cny_per_usd":       buyCnyPerUsd,
		"credit_usd_per_cny":    creditUsdPerCny,
		"start_day":             startDay,
		"end_day":               endDay,
		"items":                 items,
		"total_success_count":   totalSuccess,
		"total_used_quota":      totalUsedQuota,
		"total_cost_used_quota": totalCostUsedQuota,
		"total_revenue_cny":     totalRevenue,
		"total_cost_cny":        totalCost,
		"total_profit_cny":      totalProfit,
		"total_profit_rate":     totalProfitRate,
	})
}

func GetChannelRequestDailyStats(c *gin.Context) {
	GetChannelProfitDailyStats(c)
}

// GetChannelKey 验证2FA后获取渠道密钥
func GetChannelKey(c *gin.Context) {
	type GetChannelKeyRequest struct {
		Code string `json:"code" binding:"required"`
	}

	var req GetChannelKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, fmt.Errorf("参数错误: %v", err))
		return
	}

	userId := c.GetInt("id")
	channelId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, fmt.Errorf("渠道ID格式错误: %v", err))
		return
	}

	// 获取2FA记录并验证
	twoFA, err := model.GetTwoFAByUserId(userId)
	if err != nil {
		common.ApiError(c, fmt.Errorf("获取2FA信息失败: %v", err))
		return
	}

	if twoFA == nil || !twoFA.IsEnabled {
		common.ApiError(c, fmt.Errorf("用户未启用2FA，无法查看密钥"))
		return
	}

	// 统一的2FA验证逻辑
	if !validateTwoFactorAuth(twoFA, req.Code) {
		common.ApiError(c, fmt.Errorf("验证码或备用码错误，请重试"))
		return
	}

	// 获取渠道信息（包含密钥）
	channel, err := model.GetChannelById(channelId, true)
	if err != nil {
		common.ApiError(c, fmt.Errorf("获取渠道信息失败: %v", err))
		return
	}

	if channel == nil {
		common.ApiError(c, fmt.Errorf("渠道不存在"))
		return
	}

	// 记录操作日志
	model.RecordLog(userId, model.LogTypeSystem, fmt.Sprintf("查看渠道密钥信息 (渠道ID: %d)", channelId))

	// 统一的成功响应格式
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "验证成功",
		"data": map[string]interface{}{
			"key": channel.Key,
		},
	})
}

// validateTwoFactorAuth 统一的2FA验证函数
func validateTwoFactorAuth(twoFA *model.TwoFA, code string) bool {
	// 尝试验证TOTP
	if cleanCode, err := common.ValidateNumericCode(code); err == nil {
		if isValid, _ := twoFA.ValidateTOTPAndUpdateUsage(cleanCode); isValid {
			return true
		}
	}

	// 尝试验证备用码
	if isValid, err := twoFA.ValidateBackupCodeAndUpdateUsage(code); err == nil && isValid {
		return true
	}

	return false
}

// validateChannel 通用的渠道校验函数
func validateChannel(channel *model.Channel, isAdd bool) error {
	// 校验 channel settings
	if err := channel.ValidateSettings(); err != nil {
		return fmt.Errorf("渠道额外设置[channel setting] 格式错误：%s", err.Error())
	}

	// 如果是添加操作，检查 channel 和 key 是否为空
	if isAdd {
		if channel == nil || channel.Key == "" {
			return fmt.Errorf("channel cannot be empty")
		}

		// 检查模型名称长度是否超过 255
		for _, m := range channel.GetModels() {
			if len(m) > 255 {
				return fmt.Errorf("模型名称过长: %s", m)
			}
		}
	}

	// VertexAI 特殊校验
	if channel.Type == constant.ChannelTypeVertexAi {
		if channel.Other == "" {
			return fmt.Errorf("部署地区不能为空")
		}

		regionMap, err := common.StrToMap(channel.Other)
		if err != nil {
			return fmt.Errorf("部署地区必须是标准的Json格式，例如{\"default\": \"us-central1\", \"region2\": \"us-east1\"}")
		}

		if regionMap["default"] == nil {
			return fmt.Errorf("部署地区必须包含default字段")
		}
	}

	// Billing mode validation (quota vs request-count).
	mode := model.NormalizeChannelBillingMode(strings.TrimSpace(channel.BillingMode))
	switch mode {
	case model.ChannelBillingModeQuota:
		// no extra validation
	case model.ChannelBillingModeRequest:
		if channel.BuyRequestsPerCny <= 0 {
			return fmt.Errorf("按次计费模式下，进价兑换比例 buy_requests_per_cny 必须大于 0")
		}
		if channel.SellRequestsPerCny <= 0 {
			return fmt.Errorf("按次计费模式下，售价兑换比例 sell_requests_per_cny 必须大于 0")
		}
	default:
		return fmt.Errorf("billing_mode 无效")
	}

	if channel.GetMaxConcurrency() == 0 || channel.GetMaxConcurrency() < -1 {
		return fmt.Errorf("max_concurrency 仅支持 -1（不限制）或大于 0 的整数")
	}

	return nil
}

type AddChannelRequest struct {
	Mode                      string                `json:"mode"`
	MultiKeyMode              constant.MultiKeyMode `json:"multi_key_mode"`
	BatchAddSetKeyPrefix2Name bool                  `json:"batch_add_set_key_prefix_2_name"`
	Channel                   *model.Channel        `json:"channel"`
}

func getVertexArrayKeys(keys string) ([]string, error) {
	if keys == "" {
		return nil, nil
	}
	var keyArray []interface{}
	err := common.Unmarshal([]byte(keys), &keyArray)
	if err != nil {
		return nil, fmt.Errorf("批量添加 Vertex AI 必须使用标准的JsonArray格式，例如[{key1}, {key2}...]，请检查输入: %w", err)
	}
	cleanKeys := make([]string, 0, len(keyArray))
	for _, key := range keyArray {
		var keyStr string
		switch v := key.(type) {
		case string:
			keyStr = strings.TrimSpace(v)
		default:
			bytes, err := json.Marshal(v)
			if err != nil {
				return nil, fmt.Errorf("Vertex AI key JSON 编码失败: %w", err)
			}
			keyStr = string(bytes)
		}
		if keyStr != "" {
			cleanKeys = append(cleanKeys, keyStr)
		}
	}
	if len(cleanKeys) == 0 {
		return nil, fmt.Errorf("批量添加 Vertex AI 的 keys 不能为空")
	}
	return cleanKeys, nil
}

func AddChannel(c *gin.Context) {
	addChannelRequest := AddChannelRequest{}
	err := c.ShouldBindJSON(&addChannelRequest)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if addChannelRequest.Channel == nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "channel cannot be empty",
		})
		return
	}

	// Validate groups: group_ids must reference existing group ids (managed in `model_groups` table).
	groupIDs := model.NormalizeUniqueSortedIDs(addChannelRequest.Channel.GroupIds)
	if len(groupIDs) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "group_ids 不能为空",
		})
		return
	}
	if err := model.ValidateGroupIDsExist(nil, groupIDs); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	addChannelRequest.Channel.GroupIds = groupIDs
	backupGroupIDs, err := normalizeChannelBackupGroupIDs(groupIDs, addChannelRequest.Channel.BackupGroupIds)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	addChannelRequest.Channel.BackupGroupIds = backupGroupIDs

	// 使用统一的校验函数
	if err := validateChannel(addChannelRequest.Channel, true); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	addChannelRequest.Channel.CreatedTime = common.GetTimestamp()
	keys := make([]string, 0)
	switch addChannelRequest.Mode {
	case "multi_to_single":
		addChannelRequest.Channel.ChannelInfo.IsMultiKey = true
		addChannelRequest.Channel.ChannelInfo.MultiKeyMode = addChannelRequest.MultiKeyMode
		if addChannelRequest.Channel.Type == constant.ChannelTypeVertexAi && addChannelRequest.Channel.GetOtherSettings().VertexKeyType != dto.VertexKeyTypeAPIKey {
			array, err := getVertexArrayKeys(addChannelRequest.Channel.Key)
			if err != nil {
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": err.Error(),
				})
				return
			}
			addChannelRequest.Channel.ChannelInfo.MultiKeySize = len(array)
			addChannelRequest.Channel.Key = strings.Join(array, "\n")
		} else {
			cleanKeys := make([]string, 0)
			for _, key := range strings.Split(addChannelRequest.Channel.Key, "\n") {
				if key == "" {
					continue
				}
				key = strings.TrimSpace(key)
				cleanKeys = append(cleanKeys, key)
			}
			addChannelRequest.Channel.ChannelInfo.MultiKeySize = len(cleanKeys)
			addChannelRequest.Channel.Key = strings.Join(cleanKeys, "\n")
		}
		keys = []string{addChannelRequest.Channel.Key}
	case "batch":
		if addChannelRequest.Channel.Type == constant.ChannelTypeVertexAi && addChannelRequest.Channel.GetOtherSettings().VertexKeyType != dto.VertexKeyTypeAPIKey {
			// multi json
			keys, err = getVertexArrayKeys(addChannelRequest.Channel.Key)
			if err != nil {
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": err.Error(),
				})
				return
			}
		} else {
			keys = strings.Split(addChannelRequest.Channel.Key, "\n")
		}
	case "single":
		keys = []string{addChannelRequest.Channel.Key}
	default:
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "不支持的添加模式",
		})
		return
	}

	channels := make([]model.Channel, 0, len(keys))
	for _, key := range keys {
		if key == "" {
			continue
		}
		localChannel := addChannelRequest.Channel
		localChannel.Key = key
		if addChannelRequest.BatchAddSetKeyPrefix2Name && len(keys) > 1 {
			keyPrefix := localChannel.Key
			if len(localChannel.Key) > 8 {
				keyPrefix = localChannel.Key[:8]
			}
			localChannel.Name = fmt.Sprintf("%s %s", localChannel.Name, keyPrefix)
		}
		channels = append(channels, *localChannel)
	}
	err = model.BatchInsertChannels(channels)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	model.BumpChannelCacheRevision()
	model.InitChannelCache()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
	return
}

func DeleteChannel(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "invalid id"})
		return
	}
	channel, err := model.GetChannelById(id, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := channel.Delete(); err != nil {
		common.ApiError(c, err)
		return
	}
	model.BumpChannelCacheRevision()
	model.InitChannelCache()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
	return
}

func DeleteDisabledChannel(c *gin.Context) {
	rows, err := model.DeleteDisabledChannel()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	model.BumpChannelCacheRevision()
	model.InitChannelCache()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    rows,
	})
	return
}

type ChannelTag struct {
	Tag          string  `json:"tag"`
	NewTag       *string `json:"new_tag"`
	Priority     *int64  `json:"priority"`
	Weight       *uint   `json:"weight"`
	ModelMapping *string `json:"model_mapping"`
	Models       *string `json:"models"`
	GroupIds     *[]int  `json:"group_ids"`
}

func DisableTagChannels(c *gin.Context) {
	channelTag := ChannelTag{}
	err := c.ShouldBindJSON(&channelTag)
	if err != nil || channelTag.Tag == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "参数错误",
		})
		return
	}
	err = model.DisableChannelByTag(channelTag.Tag)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	model.BumpChannelCacheRevision()
	model.InitChannelCache()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
	return
}

func EnableTagChannels(c *gin.Context) {
	channelTag := ChannelTag{}
	err := c.ShouldBindJSON(&channelTag)
	if err != nil || channelTag.Tag == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "参数错误",
		})
		return
	}
	err = model.EnableChannelByTag(channelTag.Tag)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	model.BumpChannelCacheRevision()
	model.InitChannelCache()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
	return
}

func EditTagChannels(c *gin.Context) {
	channelTag := ChannelTag{}
	err := c.ShouldBindJSON(&channelTag)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "参数错误",
		})
		return
	}
	if channelTag.Tag == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "tag不能为空",
		})
		return
	}
	if channelTag.GroupIds != nil {
		ids := model.NormalizeUniqueSortedIDs(*channelTag.GroupIds)
		if len(ids) == 0 {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "group_ids 不能为空",
			})
			return
		}
		if err := model.ValidateGroupIDsExist(nil, ids); err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
		*channelTag.GroupIds = ids
	}
	err = model.EditChannelByTag(channelTag.Tag, channelTag.NewTag, channelTag.ModelMapping, channelTag.Models, channelTag.GroupIds, channelTag.Priority, channelTag.Weight)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	model.BumpChannelCacheRevision()
	model.InitChannelCache()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
	return
}

type ChannelBatch struct {
	Ids []int   `json:"ids"`
	Tag *string `json:"tag"`
}

type ChannelGroupBatch struct {
	Ids      []int `json:"ids"`
	GroupIds []int `json:"group_ids"`
}

type ChannelModelsBatch struct {
	Ids          []int    `json:"ids"`
	AddModels    []string `json:"add_models"`
	RemoveModels []string `json:"remove_models"`
}

func DeleteChannelBatch(c *gin.Context) {
	channelBatch := ChannelBatch{}
	err := c.ShouldBindJSON(&channelBatch)
	if err != nil || len(channelBatch.Ids) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "参数错误",
		})
		return
	}
	err = model.BatchDeleteChannels(channelBatch.Ids)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	model.BumpChannelCacheRevision()
	model.InitChannelCache()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    len(channelBatch.Ids),
	})
	return
}

// BatchResetChannelUsedQuota 批量重置渠道已用额度为 0。
func BatchResetChannelUsedQuota(c *gin.Context) {
	channelBatch := ChannelBatch{}
	err := c.ShouldBindJSON(&channelBatch)
	if err != nil || len(channelBatch.Ids) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "参数错误",
		})
		return
	}

	affected, err := model.BatchResetChannelUsedQuota(channelBatch.Ids)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	model.BumpChannelCacheRevision()
	model.InitChannelCache()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    int(affected),
	})
}

type PatchChannel struct {
	model.Channel
	MultiKeyMode *string `json:"multi_key_mode"`
	KeyMode      *string `json:"key_mode"` // 多key模式下密钥覆盖或者追加
}

func UpdateChannel(c *gin.Context) {
	channel := PatchChannel{}
	body, err := c.GetRawData()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	requestFields := map[string]json.RawMessage{}
	_ = json.Unmarshal(body, &requestFields)
	if err := json.Unmarshal(body, &channel); err != nil {
		common.ApiError(c, err)
		return
	}
	_, hasGroupIDs := requestFields["group_ids"]
	_, hasBackupGroupIDs := requestFields["backup_group_ids"]

	// Validate groups only when `group_ids` is explicitly provided (patch semantics).
	if hasGroupIDs {
		groupIDs := model.NormalizeUniqueSortedIDs(channel.GroupIds)
		if len(groupIDs) == 0 {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "group_ids 不能为空",
			})
			return
		}
		if err := model.ValidateGroupIDsExist(nil, groupIDs); err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
		channel.GroupIds = groupIDs
	}
	if hasBackupGroupIDs {
		backupGroupIDs, err := normalizeChannelBackupGroupIDs(channel.GroupIds, channel.BackupGroupIds)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
		channel.BackupGroupIds = backupGroupIDs
	}

	// 使用统一的校验函数
	if err := validateChannel(&channel.Channel, false); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	// Preserve existing ChannelInfo to ensure multi-key channels keep correct state even if the client does not send ChannelInfo in the request.
	originChannel, err := model.GetChannelById(channel.Id, true)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	// Always copy the original ChannelInfo so that fields like IsMultiKey and MultiKeySize are retained.
	channel.ChannelInfo = originChannel.ChannelInfo

	// Patch semantics: if group_ids is not explicitly provided, preserve existing bindings.
	if !hasGroupIDs {
		ids, err := model.GetChannelGroupIDsTx(nil, channel.Id)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		if len(ids) == 0 {
			common.ApiErrorMsg(c, "渠道缺少分组绑定")
			return
		}
		channel.GroupIds = ids
	}
	if !hasBackupGroupIDs {
		ids, err := model.GetChannelBackupGroupIDsTx(nil, channel.Id)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		channel.BackupGroupIds = ids
	}
	if hasGroupIDs || hasBackupGroupIDs {
		backupGroupIDs, err := normalizeChannelBackupGroupIDs(channel.GroupIds, channel.BackupGroupIds)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
		channel.BackupGroupIds = backupGroupIDs
	}

	// If the request explicitly specifies a new MultiKeyMode, apply it on top of the original info.
	if channel.MultiKeyMode != nil && *channel.MultiKeyMode != "" {
		channel.ChannelInfo.MultiKeyMode = constant.MultiKeyMode(*channel.MultiKeyMode)
	}

	// 处理多key模式下的密钥追加/覆盖逻辑
	if channel.KeyMode != nil && channel.ChannelInfo.IsMultiKey {
		switch *channel.KeyMode {
		case "append":
			// 追加模式：将新密钥添加到现有密钥列表
			if originChannel.Key != "" {
				var newKeys []string
				existingKeys := model.ParseChannelKeyList(originChannel.Key)

				// 处理 Vertex AI 的特殊情况
				if channel.Type == constant.ChannelTypeVertexAi && channel.GetOtherSettings().VertexKeyType != dto.VertexKeyTypeAPIKey {
					// 尝试解析新密钥为JSON数组
					if strings.HasPrefix(strings.TrimSpace(channel.Key), "[") {
						array, err := getVertexArrayKeys(channel.Key)
						if err != nil {
							c.JSON(http.StatusOK, gin.H{
								"success": false,
								"message": "追加密钥解析失败: " + err.Error(),
							})
							return
						}
						newKeys = array
					} else {
						// 单个JSON密钥
						newKeys = []string{channel.Key}
					}
					// 合并密钥
					allKeys := append(existingKeys, newKeys...)
					channel.Key = strings.Join(allKeys, "\n")
				} else {
					// 普通渠道的处理
					inputKeys := strings.Split(channel.Key, "\n")
					for _, key := range inputKeys {
						key = strings.TrimSpace(key)
						if key != "" {
							newKeys = append(newKeys, key)
						}
					}
					// 合并密钥
					allKeys := append(existingKeys, newKeys...)
					channel.Key = strings.Join(allKeys, "\n")
				}
			}
		case "replace":
			// 覆盖模式：直接使用新密钥（默认行为，不需要特殊处理）
		}
	}

	// NOTE: Gorm Updates(struct) ignores zero-values. Some channel fields (like buy_cny_per_usd, remark)
	// are valid when set to zero/empty, and should be persisted when explicitly provided by the client.
	forceColumns := make([]string, 0, 2)
	if _, ok := requestFields["buy_cny_per_usd"]; ok {
		forceColumns = append(forceColumns, "buy_cny_per_usd")
	}
	if _, ok := requestFields["remark"]; ok {
		forceColumns = append(forceColumns, "remark")
	}

	err = channel.UpdateWithForceColumns(forceColumns)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	model.BumpChannelCacheRevision()
	model.InitChannelCache()
	channel.Key = ""
	clearChannelInfo(&channel.Channel)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    channel,
	})
	return
}

func FetchModels(c *gin.Context) {
	var req struct {
		BaseURL string `json:"base_url"`
		Type    int    `json:"type"`
		Key     string `json:"key"`
		Proxy   string `json:"proxy"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid request",
		})
		return
	}

	tempChannel := &model.Channel{
		Type: req.Type,
		Key:  req.Key,
	}
	if baseURL := strings.TrimSpace(req.BaseURL); baseURL != "" {
		tempChannel.BaseURL = common.GetPointer(baseURL)
	}
	if proxy := strings.TrimSpace(req.Proxy); proxy != "" {
		tempChannel.SetSetting(dto.ChannelSettings{
			Proxy: proxy,
		})
	}

	models, err := fetchUpstreamModelIDsForChannel(tempChannel, req.BaseURL, req.Key, req.Proxy)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": fmt.Sprintf("获取模型列表失败: %s", err.Error()),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    models,
	})
}

func BatchSetChannelTag(c *gin.Context) {
	channelBatch := ChannelBatch{}
	err := c.ShouldBindJSON(&channelBatch)
	if err != nil || len(channelBatch.Ids) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "参数错误",
		})
		return
	}
	err = model.BatchSetChannelTag(channelBatch.Ids, channelBatch.Tag)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	model.BumpChannelCacheRevision()
	model.InitChannelCache()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    len(channelBatch.Ids),
	})
	return
}

func GetTagModels(c *gin.Context) {
	tag := c.Query("tag")
	if tag == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "tag不能为空",
		})
		return
	}

	channels, err := model.GetChannelsByTag(tag, false) // Assuming false for idSort is fine here
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	var longestModels string
	maxLength := 0

	// Find the longest models string among all channels with the given tag
	for _, channel := range channels {
		if channel.Models != "" {
			currentModels := strings.Split(channel.Models, ",")
			if len(currentModels) > maxLength {
				maxLength = len(currentModels)
				longestModels = channel.Models
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    longestModels,
	})
	return
}

// BatchSetChannelGroup 为多个渠道统一设置分组。
func BatchSetChannelGroup(c *gin.Context) {
	var req ChannelGroupBatch
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Ids) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "参数错误",
		})
		return
	}
	groupIDs := model.NormalizeUniqueSortedIDs(req.GroupIds)
	if len(groupIDs) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "group_ids 不能为空",
		})
		return
	}
	if err := model.ValidateGroupIDsExist(nil, groupIDs); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.BatchSetChannelGroupIDs(req.Ids, groupIDs); err != nil {
		common.ApiError(c, err)
		return
	}
	model.BumpChannelCacheRevision()
	model.InitChannelCache()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    len(req.Ids),
	})
}

// BatchUpdateChannelModels 为多个渠道统一添加/移除模型，遵循“有则操作，无则不变”的原则。
func BatchUpdateChannelModels(c *gin.Context) {
	var req ChannelModelsBatch
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Ids) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "参数错误",
		})
		return
	}
	if len(req.AddModels) == 0 && len(req.RemoveModels) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "请至少提供要添加或移除的模型",
		})
		return
	}
	updated, err := model.BatchUpdateChannelModels(req.Ids, req.AddModels, req.RemoveModels)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	model.BumpChannelCacheRevision()
	model.InitChannelCache()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    updated,
	})
}

type ChannelBindUsersBatch struct {
	Ids     []int `json:"ids"`
	UserIds []int `json:"user_ids"`
}

// BatchBindChannelUsers 为多个渠道统一设置绑定用户集合（全量替换）。
// user_ids 为空表示清空绑定（渠道恢复为普通渠道）。
func BatchBindChannelUsers(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": false,
		"message": "渠道绑定用户功能已下线",
	})
}

// CopyChannel handles cloning an existing channel with its key.
// POST /api/channel/copy/:id
// Optional query params:
//
//	suffix         - string appended to the original name (default "_复制")
//	reset_balance  - bool, when true will reset balance & used_quota to 0 (default true)
func CopyChannel(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "invalid id"})
		return
	}

	suffix := c.DefaultQuery("suffix", "_复制")
	resetBalance := true
	if rbStr := c.DefaultQuery("reset_balance", "true"); rbStr != "" {
		if v, err := strconv.ParseBool(rbStr); err == nil {
			resetBalance = v
		}
	}

	// fetch original channel with key
	origin, err := model.GetChannelById(id, true)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}

	groupIDs, err := model.GetChannelGroupIDs(origin.Id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if len(groupIDs) == 0 {
		common.ApiErrorMsg(c, "渠道缺少分组绑定")
		return
	}

	// clone channel
	clone := *origin // shallow copy is sufficient as we will overwrite primitives
	clone.Id = 0     // let DB auto-generate
	clone.CreatedTime = common.GetTimestamp()
	clone.Name = origin.Name + suffix
	clone.TestTime = 0
	clone.ResponseTime = 0
	clone.GroupIds = groupIDs
	clone.BackupGroupIds, err = model.GetChannelBackupGroupIDs(origin.Id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if resetBalance {
		clone.Balance = 0
		clone.UsedQuota = 0
		clone.CostUsedQuota = 0
		clone.RequestSuccessCount = 0
	}

	// insert
	if err := clone.Insert(); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	model.BumpChannelCacheRevision()
	model.InitChannelCache()
	// success
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": gin.H{"id": clone.Id}})
}

func ResetChannelUsedQuota(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}

	if _, err = model.GetChannelById(id, false); err != nil {
		common.ApiError(c, err)
		return
	}

	if err = model.ResetChannelUsedQuota(id); err != nil {
		common.ApiError(c, err)
		return
	}

	model.BumpChannelCacheRevision()
	model.InitChannelCache()

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"id":                    id,
			"used_quota":            0,
			"cost_used_quota":       0,
			"request_success_count": 0,
		},
	})
}

// MultiKeyManageRequest represents the request for multi-key management operations
type MultiKeyManageRequest struct {
	ChannelId int    `json:"channel_id"`
	Action    string `json:"action"`              // "disable_key", "enable_key", "delete_disabled_keys", "get_key_status"
	KeyIndex  *int   `json:"key_index,omitempty"` // for disable_key and enable_key actions
	Page      int    `json:"page,omitempty"`      // for get_key_status pagination
	PageSize  int    `json:"page_size,omitempty"` // for get_key_status pagination
	Status    *int   `json:"status,omitempty"`    // for get_key_status filtering: 1=enabled, 2=manual_disabled, 3=auto_disabled, nil=all
}

// MultiKeyStatusResponse represents the response for key status query
type MultiKeyStatusResponse struct {
	Keys       []KeyStatus `json:"keys"`
	Total      int         `json:"total"`
	Page       int         `json:"page"`
	PageSize   int         `json:"page_size"`
	TotalPages int         `json:"total_pages"`
	// Statistics
	EnabledCount        int `json:"enabled_count"`
	ManualDisabledCount int `json:"manual_disabled_count"`
	AutoDisabledCount   int `json:"auto_disabled_count"`
}

type KeyStatus struct {
	Index        int    `json:"index"`
	Status       int    `json:"status"` // 1: enabled, 2: disabled
	DisabledTime int64  `json:"disabled_time,omitempty"`
	Reason       string `json:"reason,omitempty"`
	KeyPreview   string `json:"key_preview"` // first 10 chars of key for identification
}

// ManageMultiKeys handles multi-key management operations
func ManageMultiKeys(c *gin.Context) {
	request := MultiKeyManageRequest{}
	err := c.ShouldBindJSON(&request)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	channel, err := model.GetChannelById(request.ChannelId, true)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "渠道不存在",
		})
		return
	}

	if !channel.ChannelInfo.IsMultiKey {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "该渠道不是多密钥模式",
		})
		return
	}

	lock := model.GetChannelPollingLock(channel.Id)
	lock.Lock()
	defer lock.Unlock()

	switch request.Action {
	case "get_key_status":
		keys := channel.GetKeys()

		// Default pagination parameters
		page := request.Page
		pageSize := request.PageSize
		if page <= 0 {
			page = 1
		}
		if pageSize <= 0 {
			pageSize = 50 // Default page size
		}

		// Statistics for all keys (unchanged by filtering)
		var enabledCount, manualDisabledCount, autoDisabledCount int

		// Build all key status data first
		var allKeyStatusList []KeyStatus
		for i, key := range keys {
			status := 1 // default enabled
			var disabledTime int64
			var reason string

			if channel.ChannelInfo.MultiKeyStatusList != nil {
				if s, exists := channel.ChannelInfo.MultiKeyStatusList[i]; exists {
					status = s
				}
			}

			// Count for statistics (all keys)
			switch status {
			case 1:
				enabledCount++
			case 2:
				manualDisabledCount++
			case 3:
				autoDisabledCount++
			}

			if status != 1 {
				if channel.ChannelInfo.MultiKeyDisabledTime != nil {
					disabledTime = channel.ChannelInfo.MultiKeyDisabledTime[i]
				}
				if channel.ChannelInfo.MultiKeyDisabledReason != nil {
					reason = channel.ChannelInfo.MultiKeyDisabledReason[i]
				}
			}

			// Create key preview (first 10 chars)
			keyPreview := key
			if len(key) > 10 {
				keyPreview = key[:10] + "..."
			}

			allKeyStatusList = append(allKeyStatusList, KeyStatus{
				Index:        i,
				Status:       status,
				DisabledTime: disabledTime,
				Reason:       reason,
				KeyPreview:   keyPreview,
			})
		}

		// Apply status filter if specified
		var filteredKeyStatusList []KeyStatus
		if request.Status != nil {
			for _, keyStatus := range allKeyStatusList {
				if keyStatus.Status == *request.Status {
					filteredKeyStatusList = append(filteredKeyStatusList, keyStatus)
				}
			}
		} else {
			filteredKeyStatusList = allKeyStatusList
		}

		// Calculate pagination based on filtered results
		filteredTotal := len(filteredKeyStatusList)
		totalPages := (filteredTotal + pageSize - 1) / pageSize
		if totalPages == 0 {
			totalPages = 1
		}
		if page > totalPages {
			page = totalPages
		}

		// Calculate range for current page
		start := (page - 1) * pageSize
		end := start + pageSize
		if end > filteredTotal {
			end = filteredTotal
		}

		// Get the page data
		var pageKeyStatusList []KeyStatus
		if start < filteredTotal {
			pageKeyStatusList = filteredKeyStatusList[start:end]
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
			"data": MultiKeyStatusResponse{
				Keys:                pageKeyStatusList,
				Total:               filteredTotal, // Total of filtered results
				Page:                page,
				PageSize:            pageSize,
				TotalPages:          totalPages,
				EnabledCount:        enabledCount,        // Overall statistics
				ManualDisabledCount: manualDisabledCount, // Overall statistics
				AutoDisabledCount:   autoDisabledCount,   // Overall statistics
			},
		})
		return

	case "disable_key":
		if request.KeyIndex == nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "未指定要禁用的密钥索引",
			})
			return
		}

		keyIndex := *request.KeyIndex
		if keyIndex < 0 || keyIndex >= channel.ChannelInfo.MultiKeySize {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "密钥索引超出范围",
			})
			return
		}

		if channel.ChannelInfo.MultiKeyStatusList == nil {
			channel.ChannelInfo.MultiKeyStatusList = make(map[int]int)
		}
		if channel.ChannelInfo.MultiKeyDisabledTime == nil {
			channel.ChannelInfo.MultiKeyDisabledTime = make(map[int]int64)
		}
		if channel.ChannelInfo.MultiKeyDisabledReason == nil {
			channel.ChannelInfo.MultiKeyDisabledReason = make(map[int]string)
		}

		channel.ChannelInfo.MultiKeyStatusList[keyIndex] = 2 // disabled

		err = channel.Update()
		if err != nil {
			common.ApiError(c, err)
			return
		}

		model.BumpChannelCacheRevision()
		model.InitChannelCache()
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "密钥已禁用",
		})
		return

	case "enable_key":
		if request.KeyIndex == nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "未指定要启用的密钥索引",
			})
			return
		}

		keyIndex := *request.KeyIndex
		if keyIndex < 0 || keyIndex >= channel.ChannelInfo.MultiKeySize {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "密钥索引超出范围",
			})
			return
		}

		// 从状态列表中删除该密钥的记录，使其回到默认启用状态
		if channel.ChannelInfo.MultiKeyStatusList != nil {
			delete(channel.ChannelInfo.MultiKeyStatusList, keyIndex)
		}
		if channel.ChannelInfo.MultiKeyDisabledTime != nil {
			delete(channel.ChannelInfo.MultiKeyDisabledTime, keyIndex)
		}
		if channel.ChannelInfo.MultiKeyDisabledReason != nil {
			delete(channel.ChannelInfo.MultiKeyDisabledReason, keyIndex)
		}

		err = channel.Update()
		if err != nil {
			common.ApiError(c, err)
			return
		}

		model.BumpChannelCacheRevision()
		model.InitChannelCache()
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "密钥已启用",
		})
		return

	case "enable_all_keys":
		// 清空所有禁用状态，使所有密钥回到默认启用状态
		var enabledCount int
		if channel.ChannelInfo.MultiKeyStatusList != nil {
			enabledCount = len(channel.ChannelInfo.MultiKeyStatusList)
		}

		channel.ChannelInfo.MultiKeyStatusList = make(map[int]int)
		channel.ChannelInfo.MultiKeyDisabledTime = make(map[int]int64)
		channel.ChannelInfo.MultiKeyDisabledReason = make(map[int]string)

		err = channel.Update()
		if err != nil {
			common.ApiError(c, err)
			return
		}

		model.BumpChannelCacheRevision()
		model.InitChannelCache()
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": fmt.Sprintf("已启用 %d 个密钥", enabledCount),
		})
		return

	case "disable_all_keys":
		// 禁用所有启用的密钥
		if channel.ChannelInfo.MultiKeyStatusList == nil {
			channel.ChannelInfo.MultiKeyStatusList = make(map[int]int)
		}
		if channel.ChannelInfo.MultiKeyDisabledTime == nil {
			channel.ChannelInfo.MultiKeyDisabledTime = make(map[int]int64)
		}
		if channel.ChannelInfo.MultiKeyDisabledReason == nil {
			channel.ChannelInfo.MultiKeyDisabledReason = make(map[int]string)
		}

		var disabledCount int
		for i := 0; i < channel.ChannelInfo.MultiKeySize; i++ {
			status := 1 // default enabled
			if s, exists := channel.ChannelInfo.MultiKeyStatusList[i]; exists {
				status = s
			}

			// 只禁用当前启用的密钥
			if status == 1 {
				channel.ChannelInfo.MultiKeyStatusList[i] = 2 // disabled
				disabledCount++
			}
		}

		if disabledCount == 0 {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "没有可禁用的密钥",
			})
			return
		}

		err = channel.Update()
		if err != nil {
			common.ApiError(c, err)
			return
		}

		model.BumpChannelCacheRevision()
		model.InitChannelCache()
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": fmt.Sprintf("已禁用 %d 个密钥", disabledCount),
		})
		return

	case "delete_disabled_keys":
		keys := channel.GetKeys()
		var remainingKeys []string
		var deletedCount int
		var newStatusList = make(map[int]int)
		var newDisabledTime = make(map[int]int64)
		var newDisabledReason = make(map[int]string)

		newIndex := 0
		for i, key := range keys {
			status := 1 // default enabled
			if channel.ChannelInfo.MultiKeyStatusList != nil {
				if s, exists := channel.ChannelInfo.MultiKeyStatusList[i]; exists {
					status = s
				}
			}

			// 只删除自动禁用（status == 3）的密钥，保留启用（status == 1）和手动禁用（status == 2）的密钥
			if status == 3 {
				deletedCount++
			} else {
				remainingKeys = append(remainingKeys, key)
				// 保留非自动禁用密钥的状态信息，重新索引
				if status != 1 {
					newStatusList[newIndex] = status
					if channel.ChannelInfo.MultiKeyDisabledTime != nil {
						if t, exists := channel.ChannelInfo.MultiKeyDisabledTime[i]; exists {
							newDisabledTime[newIndex] = t
						}
					}
					if channel.ChannelInfo.MultiKeyDisabledReason != nil {
						if r, exists := channel.ChannelInfo.MultiKeyDisabledReason[i]; exists {
							newDisabledReason[newIndex] = r
						}
					}
				}
				newIndex++
			}
		}

		if deletedCount == 0 {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "没有需要删除的自动禁用密钥",
			})
			return
		}

		// Update channel with remaining keys
		channel.Key = strings.Join(remainingKeys, "\n")
		channel.ChannelInfo.MultiKeySize = len(remainingKeys)
		channel.ChannelInfo.MultiKeyStatusList = newStatusList
		channel.ChannelInfo.MultiKeyDisabledTime = newDisabledTime
		channel.ChannelInfo.MultiKeyDisabledReason = newDisabledReason

		err = channel.Update()
		if err != nil {
			common.ApiError(c, err)
			return
		}

		model.BumpChannelCacheRevision()
		model.InitChannelCache()
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": fmt.Sprintf("已删除 %d 个自动禁用的密钥", deletedCount),
			"data":    deletedCount,
		})
		return

	default:
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "不支持的操作",
		})
		return
	}
}
