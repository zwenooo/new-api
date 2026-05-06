package model

import (
	"context"
	"fmt"
	"one-api/common"
	"one-api/constant"
	"one-api/dto"
	"one-api/logger"
	"one-api/types"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/bytedance/gopkg/util/gopool"
	"gorm.io/gorm"
)

type Log struct {
	Id                 int    `json:"id" gorm:"index:idx_created_at_id,priority:1"`
	UserId             int    `json:"user_id" gorm:"index"`
	CreatedAt          int64  `json:"created_at" gorm:"bigint;index:idx_created_at_id,priority:2;index:idx_created_at_type"`
	Type               int    `json:"type" gorm:"index:idx_created_at_type"`
	Content            string `json:"content"`
	Username           string `json:"username" gorm:"index;index:index_username_model_name,priority:2;default:''"`
	TokenName          string `json:"token_name" gorm:"index;default:''"`
	ModelName          string `json:"model_name" gorm:"index;index:index_username_model_name,priority:1;default:''"`
	Quota              int    `json:"quota" gorm:"default:0"`
	VisibleQuota       int    `json:"visible_quota" gorm:"default:0"`
	CostQuota          int    `json:"cost_quota" gorm:"default:0"`
	PromptTokens       int    `json:"prompt_tokens" gorm:"default:0"`
	CompletionTokens   int    `json:"completion_tokens" gorm:"default:0"`
	UseTime            int    `json:"use_time" gorm:"default:0"`
	IsStream           bool   `json:"is_stream"`
	ChannelId          int    `json:"channel" gorm:"index"`
	ChannelName        string `json:"channel_name" gorm:"->"`
	TokenId            int    `json:"token_id" gorm:"default:0;index"`
	Group              string `json:"group" gorm:"index"`
	Ip                 string `json:"ip" gorm:"index;default:''"`
	RequestId          string `json:"request_id" gorm:"type:varchar(64);index;default:'';column:request_id"`
	Other              string `json:"other"`
	QuotaLegacy        bool   `json:"quota_legacy,omitempty" gorm:"-"`
	QuotaDetailsHidden bool   `json:"quota_details_hidden,omitempty" gorm:"-"`
}

const serviceStatusUpdateTimeout = 3 * time.Second

func queueServiceStatusBucketStatsOnLogInsert(requestID string, logType int, createdAt int64, groupCode string, content string, other string) {
	gopool.Go(func() {
		ctx, cancel := context.WithTimeout(context.Background(), serviceStatusUpdateTimeout)
		defer cancel()
		if err := UpdateServiceStatusBucketStatsOnLogInsert(ctx, requestID, logType, createdAt, groupCode, content, other); err != nil {
			logger.LogError(context.Background(), "failed to update service status bucket stats: "+err.Error())
		}
	})
}

func queueServiceStatusBucketStatsOnLogTypeChange(requestID string, createdAt int64, oldGroupCode string, newGroupCode string, oldType int, oldContent string, oldOther string, newType int, newContent string, newOther string) {
	gopool.Go(func() {
		ctx, cancel := context.WithTimeout(context.Background(), serviceStatusUpdateTimeout)
		defer cancel()
		if err := UpdateServiceStatusBucketStatsOnLogTypeChange(ctx, requestID, createdAt, oldGroupCode, newGroupCode, oldType, oldContent, oldOther, newType, newContent, newOther); err != nil {
			logger.LogError(context.Background(), "failed to update service status bucket stats: "+err.Error())
		}
	})
}

func captureSafeRequestHeadersForLog(c *gin.Context) map[string][]string {
	if c == nil || c.Request == nil || c.Request.Header == nil {
		return nil
	}

	// Only capture a small allowlist to avoid storing sensitive headers like Authorization/Cookie.
	allowed := []string{
		"Accept",
		"Accept-Encoding",
		"Accept-Language",
		"Cache-Control",
		"Content-Type",
		"Origin",
		"Pragma",
		"Referer",
		"Sec-Fetch-Dest",
		"Sec-Fetch-Mode",
		"Sec-Fetch-Site",
		"X-Forwarded-Proto",
	}

	out := make(map[string][]string)
	for _, key := range allowed {
		values := c.Request.Header.Values(key)
		if len(values) == 0 {
			continue
		}
		cleaned := make([]string, 0, len(values))
		for _, v := range values {
			v = strings.TrimSpace(v)
			if v == "" {
				continue
			}
			cleaned = append(cleaned, v)
		}
		if len(cleaned) > 0 {
			out[key] = cleaned
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func attachRequestDebugInfoForErrorLog(c *gin.Context, other map[string]interface{}) map[string]interface{} {
	if c == nil || c.Request == nil || other == nil {
		return other
	}
	// Only attach for error logs (other contains error_type/error_code/status_code).
	if _, ok := other["error_type"]; !ok {
		return other
	}

	adminInfo, _ := other["admin_info"].(map[string]interface{})
	if adminInfo == nil {
		adminInfo = make(map[string]interface{})
		other["admin_info"] = adminInfo
	}

	if _, exists := adminInfo["request_headers"]; !exists {
		if headers := captureSafeRequestHeadersForLog(c); headers != nil {
			adminInfo["request_headers"] = headers
		}
	}
	if _, exists := adminInfo["request_content_length"]; !exists {
		if c.Request.ContentLength > 0 {
			adminInfo["request_content_length"] = c.Request.ContentLength
		}
	}
	return other
}

func ensureRequestMetaForLog(c *gin.Context, other map[string]interface{}) map[string]interface{} {
	if other == nil {
		other = make(map[string]interface{})
	}
	if c == nil {
		return other
	}

	if _, ok := other["request_id"]; !ok {
		if requestID := strings.TrimSpace(c.GetString(common.RequestIdKey)); requestID != "" {
			other["request_id"] = requestID
		}
	}

	if c.Request == nil {
		return other
	}
	if _, ok := other["request_method"]; !ok {
		if method := strings.TrimSpace(c.Request.Method); method != "" {
			other["request_method"] = method
		}
	}
	if _, ok := other["request_path"]; !ok {
		path := ""
		if c.Request.URL != nil {
			path = c.Request.URL.Path
		}
		if path = strings.TrimSpace(path); path != "" {
			other["request_path"] = path
		}
	}
	if _, ok := other["request_ua"]; !ok {
		if ua := strings.TrimSpace(c.Request.UserAgent()); ua != "" {
			other["request_ua"] = ua
		}
	}
	return other
}

func shouldRecordIPInLog(c *gin.Context, userId int) bool {
	if c != nil {
		if settingMap, ok := common.GetContextKeyType[dto.UserSetting](c, constant.ContextKeyUserSetting); ok {
			return settingMap.RecordIpLog
		}
	}
	if userId <= 0 {
		return false
	}
	if settingMap, err := GetUserSetting(userId, false); err == nil {
		return settingMap.RecordIpLog
	}
	return false
}

const (
	LogTypeUnknown = iota
	LogTypeTopup
	LogTypeConsume
	LogTypeManage
	LogTypeSystem
	LogTypeError
	LogTypeConsumeInProgress
)

func formatUserLogs(logs []*Log) {
	for i := range logs {
		logs[i].ChannelName = ""
		var otherMap map[string]interface{}
		otherMap, _ = common.StrToMap(logs[i].Other)
		if logs[i].Quota > 0 && logs[i].CostQuota <= 0 {
			logs[i].QuotaLegacy = true
		}
		if otherMap != nil {
			// delete admin
			delete(otherMap, "admin_info")
			delete(otherMap, "is_model_mapped")
			delete(otherMap, "upstream_model_name")
		}
		logs[i].Other = common.MapToJsonStr(otherMap)
		logs[i].Id = logs[i].Id % 1024
	}
}

func buildUserConsumeLogFilterQuery(userId int, startTimestamp int64, endTimestamp int64, modelName string, tokenName string, channel int, group string) *gorm.DB {
	tx := LOG_DB.Table("logs").Where("user_id = ? AND type = ?", userId, LogTypeConsume)
	if startTimestamp != 0 {
		tx = tx.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("created_at <= ?", endTimestamp)
	}
	if tokenName != "" {
		tx = tx.Where("token_name = ?", tokenName)
	}
	if modelName != "" {
		tx = tx.Where("model_name like ?", modelName)
	}
	if channel != 0 {
		tx = tx.Where("channel_id = ?", channel)
	}
	if group != "" {
		tx = tx.Where(logGroupCol+" = ?", group)
	}
	return tx
}

func buildLegacyHiddenUserConsumeLogMarkerQuery(tx *gorm.DB) *gorm.DB {
	return tx.Where(
		"(other LIKE ? OR other LIKE ? OR (other LIKE ? AND other NOT LIKE ?))",
		"%\"group_ratio_source\":\"legacy\"%",
		"%\"group_ratio_source\":\"base_multiplier\"%",
		"%user_group_ratio%",
		"%\"group_ratio_source\":%",
	)
}

func HasLegacyHiddenUserConsumeLogs(userId int, startTimestamp int64, endTimestamp int64, modelName string, tokenName string, channel int, group string) (bool, error) {
	if userId <= 0 {
		return false, nil
	}
	var count int64
	err := buildLegacyHiddenUserConsumeLogMarkerQuery(
		buildUserConsumeLogFilterQuery(userId, startTimestamp, endTimestamp, modelName, tokenName, channel, group),
	).
		Where("quota > 0 AND visible_quota = 0 AND cost_quota = 0").
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func GetLegacyHiddenUserConsumeQuotaBuckets(userId int, startTimestamp int64, endTimestamp int64) (map[int64]struct{}, error) {
	buckets := make(map[int64]struct{})
	if userId <= 0 {
		return buckets, nil
	}
	type bucketRow struct {
		CreatedAt int64 `gorm:"column:created_at"`
	}
	rows := make([]bucketRow, 0)
	err := buildLegacyHiddenUserConsumeLogMarkerQuery(
		buildUserConsumeLogFilterQuery(userId, startTimestamp, endTimestamp, "", "", 0, ""),
	).
		Select("DISTINCT created_at - (created_at % 3600) AS created_at").
		Where("quota > 0 AND visible_quota = 0 AND cost_quota = 0").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		if row.CreatedAt <= 0 {
			continue
		}
		buckets[row.CreatedAt] = struct{}{}
	}
	return buckets, nil
}

func GetLogByKey(key string) (logs []*Log, err error) {
	if os.Getenv("LOG_SQL_DSN") != "" {
		var tk Token
		if err = DB.Model(&Token{}).Where(logKeyCol+"=?", strings.TrimPrefix(key, "sk-")).First(&tk).Error; err != nil {
			return nil, err
		}
		err = LOG_DB.Model(&Log{}).Where("token_id=?", tk.Id).Find(&logs).Error
	} else {
		err = LOG_DB.Joins("left join tokens on tokens.id = logs.token_id").Where("tokens."+commonKeyCol+" = ?", strings.TrimPrefix(key, "sk-")).Find(&logs).Error
	}
	formatUserLogs(logs)
	return logs, err
}

func RecordLog(userId int, logType int, content string) {
	if logType == LogTypeConsume && !common.LogConsumeEnabled {
		return
	}
	username, _ := GetUsernameById(userId, false)
	log := &Log{
		UserId:    userId,
		Username:  username,
		CreatedAt: common.GetTimestamp(),
		Type:      logType,
		Content:   content,
	}
	err := LOG_DB.Create(log).Error
	if err != nil {
		common.SysLog("failed to record log: " + err.Error())
	}
}

type RecordTaskBillingLogParams struct {
	UserId    int
	LogType   int
	Content   string
	ChannelId int
	ModelName string
	Quota     int
	TokenId   int
	Group     string
	Other     map[string]interface{}
}

func RecordTaskBillingLog(params RecordTaskBillingLogParams) {
	if params.LogType == LogTypeConsume && !common.LogConsumeEnabled {
		return
	}

	username, _ := GetUsernameById(params.UserId, false)
	tokenName := ""
	if params.TokenId > 0 {
		if token, err := GetTokenById(params.TokenId); err == nil && token != nil {
			tokenName = token.Name
		}
	}

	log := &Log{
		UserId:    params.UserId,
		Username:  username,
		CreatedAt: common.GetTimestamp(),
		Type:      params.LogType,
		Content:   params.Content,
		TokenName: tokenName,
		ModelName: params.ModelName,
		Quota:     params.Quota,
		ChannelId: params.ChannelId,
		TokenId:   params.TokenId,
		Group:     params.Group,
		Other:     common.MapToJsonStr(params.Other),
	}
	if err := LOG_DB.Create(log).Error; err != nil {
		common.SysLog("failed to record task billing log: " + err.Error())
	}
}

func RecordErrorLog(c *gin.Context, userId int, channelId int, modelName string, tokenName string, content string, tokenId int, useTimeSeconds int,
	isStream bool, group string, other map[string]interface{}) {
	logger.LogRequestInfo(c, fmt.Sprintf("record error log: userId=%d, channelId=%d, modelName=%s, tokenName=%s, content=%s", userId, channelId, modelName, tokenName, content))
	username := c.GetString("username")
	requestID := strings.TrimSpace(c.GetString(common.RequestIdKey))
	other = ensureRequestMetaForLog(c, other)
	other = attachRequestDebugInfoForErrorLog(c, other)
	otherStr := common.MapToJsonStr(other)
	needRecordIp := shouldRecordIPInLog(c, userId)
	log := &Log{
		UserId:           userId,
		Username:         username,
		CreatedAt:        common.GetTimestamp(),
		Type:             LogTypeError,
		Content:          content,
		PromptTokens:     0,
		CompletionTokens: 0,
		TokenName:        tokenName,
		ModelName:        modelName,
		Quota:            0,
		ChannelId:        channelId,
		TokenId:          tokenId,
		UseTime:          useTimeSeconds,
		IsStream:         isStream,
		Group:            group,
		Ip: func() string {
			if needRecordIp {
				return c.ClientIP()
			}
			return ""
		}(),
		RequestId: requestID,
		Other:     otherStr,
	}
	err := LOG_DB.Create(log).Error
	if err != nil {
		logger.LogError(c, "failed to record log: "+err.Error())
		return
	}
	queueServiceStatusBucketStatsOnLogInsert(log.RequestId, log.Type, log.CreatedAt, log.Group, log.Content, log.Other)
}

type RecordConsumeLogParams struct {
	CreatedAt        int64                  `json:"created_at"`
	ChannelId        int                    `json:"channel_id"`
	PromptTokens     int                    `json:"prompt_tokens"`
	CompletionTokens int                    `json:"completion_tokens"`
	ModelName        string                 `json:"model_name"`
	TokenName        string                 `json:"token_name"`
	Quota            int                    `json:"quota"`
	VisibleQuota     int                    `json:"visible_quota"`
	CostQuota        int                    `json:"cost_quota"`
	Content          string                 `json:"content"`
	TokenId          int                    `json:"token_id"`
	UseTimeSeconds   int                    `json:"use_time_seconds"`
	IsStream         bool                   `json:"is_stream"`
	Group            string                 `json:"group"`
	Other            map[string]interface{} `json:"other"`
}

func RecordConsumeLog(c *gin.Context, userId int, params RecordConsumeLogParams) {
	if !common.LogConsumeEnabled {
		return
	}
	logger.LogRequestInfo(c, fmt.Sprintf("record consume log: userId=%d, params=%s", userId, common.GetJsonString(params)))
	username := c.GetString("username")
	requestID := strings.TrimSpace(c.GetString(common.RequestIdKey))
	createdAt := params.CreatedAt
	if createdAt <= 0 {
		createdAt = common.GetTimestamp()
	}
	params.Other = ensureRequestMetaForLog(c, params.Other)
	otherStr := common.MapToJsonStr(params.Other)
	needRecordIp := shouldRecordIPInLog(c, userId)
	log := &Log{
		UserId:           userId,
		Username:         username,
		CreatedAt:        createdAt,
		Type:             LogTypeConsume,
		Content:          params.Content,
		PromptTokens:     params.PromptTokens,
		CompletionTokens: params.CompletionTokens,
		TokenName:        params.TokenName,
		ModelName:        params.ModelName,
		Quota:            params.Quota,
		VisibleQuota:     params.VisibleQuota,
		CostQuota:        params.CostQuota,
		ChannelId:        params.ChannelId,
		TokenId:          params.TokenId,
		UseTime:          params.UseTimeSeconds,
		IsStream:         params.IsStream,
		Group:            params.Group,
		Ip: func() string {
			if needRecordIp {
				return c.ClientIP()
			}
			return ""
		}(),
		RequestId: requestID,
		Other:     otherStr,
	}
	err := LOG_DB.Create(log).Error
	if err != nil {
		logger.LogError(c, "failed to record log: "+err.Error())
		return
	}
	IncrementChannelRequestSuccessStats(params.ChannelId, createdAt)
	IncrementUserRequestDailyStats(
		userId,
		createdAt,
		int64(params.Quota),
		int64(params.VisibleQuota),
		int64(params.CostQuota),
		int64(params.PromptTokens+params.CompletionTokens),
	)
	queueServiceStatusBucketStatsOnLogInsert(log.RequestId, log.Type, log.CreatedAt, log.Group, log.Content, log.Other)
	if common.DataExportEnabled {
		gopool.Go(func() {
			LogQuotaData(
				userId,
				username,
				params.ModelName,
				params.Quota,
				params.VisibleQuota,
				params.CostQuota,
				common.GetTimestamp(),
				params.PromptTokens+params.CompletionTokens,
			)
		})
	}
}

// RecordInitialConsumeLog 在请求开始时记录初始日志条目（用于流式响应实时显示）
// 返回日志ID供后续更新使用
func RecordInitialConsumeLog(c *gin.Context, userId int, params RecordConsumeLogParams) int {
	if !common.LogConsumeEnabled {
		return 0
	}
	username := c.GetString("username")
	requestID := strings.TrimSpace(c.GetString(common.RequestIdKey))
	createdAt := params.CreatedAt
	if createdAt <= 0 {
		createdAt = common.GetTimestamp()
	}

	// 初始日志内容标记为"进行中"
	initialContent := "(进行中...)"
	if params.Content != "" {
		initialContent = params.Content + " (进行中...)"
	}

	needRecordIp := shouldRecordIPInLog(c, userId)

	// 初始化Other为空对象，避免前端读取时出错
	initialOther := params.Other
	if initialOther == nil {
		initialOther = make(map[string]interface{})
	}
	// 存储 conversation_id, session_id, prompt_cache_key
	if conversationID, ok := params.Other["conversation_id"]; ok {
		initialOther["conversation_id"] = conversationID
	}
	if sessionID, ok := params.Other["session_id"]; ok {
		initialOther["session_id"] = sessionID
	}
	if promptCacheKey, ok := params.Other["prompt_cache_key"]; ok {
		initialOther["prompt_cache_key"] = promptCacheKey
	}
	initialOther = ensureRequestMetaForLog(c, initialOther)

	log := &Log{
		UserId:           userId,
		Username:         username,
		CreatedAt:        createdAt,
		Type:             LogTypeConsumeInProgress, // 使用新的状态码 6
		Content:          initialContent,
		PromptTokens:     params.PromptTokens, // 预估的prompt tokens
		CompletionTokens: 0,                   // 初始为0，后续更新
		TokenName:        params.TokenName,
		ModelName:        params.ModelName,
		Quota:            params.Quota, // 预扣费配额
		VisibleQuota:     params.VisibleQuota,
		CostQuota:        params.CostQuota,
		ChannelId:        params.ChannelId,
		TokenId:          params.TokenId,
		UseTime:          0, // 初始为0，后续更新
		IsStream:         params.IsStream,
		Group:            params.Group,
		Ip: func() string {
			if needRecordIp {
				return c.ClientIP()
			}
			return ""
		}(),
		RequestId: requestID,
		Other:     common.MapToJsonStr(initialOther),
	}
	err := LOG_DB.Create(log).Error
	if err != nil {
		logger.LogError(c, "failed to record initial log: "+err.Error())
		return 0
	}
	logger.LogRequestInfo(c, fmt.Sprintf("created initial log entry: logId=%d, userId=%d", log.Id, userId))
	return log.Id
}

// UpdateConsumeLog 更新已存在的日志条目（流式响应完成后更新）
func UpdateConsumeLog(c *gin.Context, logId int, params RecordConsumeLogParams) {
	if !common.LogConsumeEnabled || logId == 0 {
		return
	}

	params.Other = ensureRequestMetaForLog(c, params.Other)
	otherStr := common.MapToJsonStr(params.Other)
	updates := map[string]interface{}{
		"type":              LogTypeConsume, // 更新回正常消费状态
		"completion_tokens": params.CompletionTokens,
		"quota":             params.Quota,
		"visible_quota":     params.VisibleQuota,
		"cost_quota":        params.CostQuota,
		"content":           params.Content,
		"use_time":          params.UseTimeSeconds,
		"other":             otherStr,
		"is_stream":         params.IsStream,
	}

	// 如果prompt tokens有变化也更新（某些场景下可能重新计算）
	if params.PromptTokens > 0 {
		updates["prompt_tokens"] = params.PromptTokens
	}
	if params.ChannelId > 0 {
		updates["channel_id"] = params.ChannelId
	}
	if strings.TrimSpace(params.TokenName) != "" {
		updates["token_name"] = params.TokenName
	}
	if strings.TrimSpace(params.ModelName) != "" {
		updates["model_name"] = params.ModelName
	}
	if params.TokenId > 0 {
		updates["token_id"] = params.TokenId
	}
	if strings.TrimSpace(params.Group) != "" {
		updates["group"] = params.Group
	}

	tx := LOG_DB.Model(&Log{}).Where("id = ? AND type = ?", logId, LogTypeConsumeInProgress).Updates(updates)
	if tx.Error != nil {
		logger.LogError(c, "failed to update consume log: "+tx.Error.Error())
		return
	}
	updatedFromInProgress := tx.RowsAffected > 0
	if !updatedFromInProgress {
		// Keep legacy behavior: allow updating the entry even if it's already updated.
		if err := LOG_DB.Model(&Log{}).Where("id = ?", logId).Updates(updates).Error; err != nil {
			logger.LogError(c, "failed to update consume log: "+err.Error())
			return
		}
	}
	logger.LogRequestInfo(c, fmt.Sprintf("updated log entry: logId=%d", logId))
	if updatedFromInProgress {
		createdAt := params.CreatedAt
		if createdAt <= 0 {
			createdAt = common.GetTimestamp()
		}
		IncrementChannelRequestSuccessStats(params.ChannelId, createdAt)
		userId := c.GetInt("id")
		IncrementUserRequestDailyStats(
			userId,
			createdAt,
			int64(params.Quota),
			int64(params.VisibleQuota),
			int64(params.CostQuota),
			int64(params.PromptTokens+params.CompletionTokens),
		)
	}
	if updatedFromInProgress {
		queueServiceStatusBucketStatsOnLogTypeChange(
			strings.TrimSpace(c.GetString(common.RequestIdKey)),
			params.CreatedAt,
			params.Group,
			params.Group,
			LogTypeConsumeInProgress,
			"",
			"",
			LogTypeConsume,
			params.Content,
			otherStr,
		)
	}

	if common.DataExportEnabled {
		userId := c.GetInt("id")
		username := c.GetString("username")
		gopool.Go(func() {
			LogQuotaData(
				userId,
				username,
				params.ModelName,
				params.Quota,
				params.VisibleQuota,
				params.CostQuota,
				common.GetTimestamp(),
				params.PromptTokens+params.CompletionTokens,
			)
		})
	}
}

// UpdateConsumeLogAsError updates an existing consume log entry to error status with detailed fields.
// This is mainly used when a streaming request succeeded but the final quota settlement failed,
// to avoid leaving the log stuck in "in progress".
func UpdateConsumeLogAsError(c *gin.Context, logId int, params RecordConsumeLogParams) {
	if !common.LogConsumeEnabled || logId == 0 {
		return
	}

	params.Other = ensureRequestMetaForLog(c, params.Other)
	otherStr := common.MapToJsonStr(params.Other)
	updates := map[string]interface{}{
		"type":              LogTypeError,
		"completion_tokens": params.CompletionTokens,
		"quota":             params.Quota,
		"visible_quota":     params.VisibleQuota,
		"cost_quota":        params.CostQuota,
		"content":           params.Content,
		"use_time":          params.UseTimeSeconds,
		"other":             otherStr,
		"is_stream":         params.IsStream,
	}

	if params.PromptTokens > 0 {
		updates["prompt_tokens"] = params.PromptTokens
	}
	if params.ChannelId > 0 {
		updates["channel_id"] = params.ChannelId
	}
	if strings.TrimSpace(params.TokenName) != "" {
		updates["token_name"] = params.TokenName
	}
	if strings.TrimSpace(params.ModelName) != "" {
		updates["model_name"] = params.ModelName
	}
	if params.TokenId > 0 {
		updates["token_id"] = params.TokenId
	}
	if strings.TrimSpace(params.Group) != "" {
		updates["group"] = params.Group
	}

	tx := LOG_DB.Model(&Log{}).Where("id = ? AND type = ?", logId, LogTypeConsumeInProgress).Updates(updates)
	if tx.Error != nil {
		logger.LogError(c, "failed to update consume log as error: "+tx.Error.Error())
		return
	}
	if tx.RowsAffected == 0 {
		// Already updated (success/error). Avoid overriding the final status.
		return
	}
	queueServiceStatusBucketStatsOnLogTypeChange(
		strings.TrimSpace(c.GetString(common.RequestIdKey)),
		params.CreatedAt,
		params.Group,
		params.Group,
		LogTypeConsumeInProgress,
		"",
		"",
		LogTypeError,
		params.Content,
		otherStr,
	)
}

// UpdateConsumeLogOnError 在请求失败时更新日志状态为错误
func UpdateConsumeLogOnError(c *gin.Context, logId int, createdAt int64, group string, content string) {
	if !common.LogConsumeEnabled || logId == 0 {
		return
	}
	updates := map[string]interface{}{
		"type":    LogTypeError,
		"content": content,
	}
	err := LOG_DB.Model(&Log{}).Where("id = ?", logId).Updates(updates).Error
	if err != nil {
		logger.LogError(c, "failed to update consume log on error: "+err.Error())
		return
	}
	queueServiceStatusBucketStatsOnLogTypeChange(
		strings.TrimSpace(c.GetString(common.RequestIdKey)),
		createdAt,
		group,
		group,
		LogTypeConsumeInProgress,
		"",
		"",
		LogTypeError,
		content,
		"",
	)
}

func GetAllLogs(logType int, startTimestamp int64, endTimestamp int64, modelName string, username string, tokenName string, requestID string, startIdx int, num int, channel int, group string) (logs []*Log, total int64, err error) {
	var tx *gorm.DB
	if logType == LogTypeUnknown {
		tx = LOG_DB
	} else {
		tx = LOG_DB.Where("logs.type = ?", logType)
	}

	if modelName != "" {
		tx = tx.Where("logs.model_name like ?", modelName)
	}
	if username != "" {
		tx = tx.Where("logs.username = ?", username)
	}
	if tokenName != "" {
		tx = tx.Where("logs.token_name = ?", tokenName)
	}
	if requestID != "" {
		tx = tx.Where("logs.request_id = ?", requestID)
	}
	if startTimestamp != 0 {
		tx = tx.Where("logs.created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("logs.created_at <= ?", endTimestamp)
	}
	if channel != 0 {
		tx = tx.Where("logs.channel_id = ?", channel)
	}
	if group != "" {
		tx = tx.Where("logs."+logGroupCol+" = ?", group)
	}
	err = tx.Model(&Log{}).Count(&total).Error
	if err != nil {
		return nil, 0, err
	}
	err = tx.Order("logs.id desc").Limit(num).Offset(startIdx).Find(&logs).Error
	if err != nil {
		return nil, 0, err
	}

	channelIds := types.NewSet[int]()
	for _, log := range logs {
		if log.ChannelId != 0 {
			channelIds.Add(log.ChannelId)
		}
	}

	if channelIds.Len() > 0 {
		var channels []struct {
			Id   int    `gorm:"column:id"`
			Name string `gorm:"column:name"`
		}
		if err = DB.Table("channels").Select("id, name").Where("id IN ?", channelIds.Items()).Find(&channels).Error; err != nil {
			return logs, total, err
		}
		channelMap := make(map[int]string, len(channels))
		for _, channel := range channels {
			channelMap[channel.Id] = channel.Name
		}
		for i := range logs {
			logs[i].ChannelName = channelMap[logs[i].ChannelId]
		}
	}

	return logs, total, err
}

func GetUserLogs(userId int, logType int, startTimestamp int64, endTimestamp int64, modelName string, tokenName string, requestID string, startIdx int, num int, group string) (logs []*Log, total int64, err error) {
	tx := LOG_DB.Where("logs.user_id = ?", userId)
	switch {
	case logType == LogTypeManage:
		// Block management logs from self usage view
		tx = tx.Where("1 = 0")
	case logType != LogTypeUnknown:
		tx = tx.Where("logs.type = ?", logType)
	default:
		tx = tx.Where("logs.type <> ?", LogTypeManage)
	}

	if modelName != "" {
		tx = tx.Where("logs.model_name like ?", modelName)
	}
	if tokenName != "" {
		tx = tx.Where("logs.token_name = ?", tokenName)
	}
	if requestID != "" {
		tx = tx.Where("logs.request_id = ?", requestID)
	}
	if startTimestamp != 0 {
		tx = tx.Where("logs.created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("logs.created_at <= ?", endTimestamp)
	}
	if group != "" {
		tx = tx.Where("logs."+logGroupCol+" = ?", group)
	}
	err = tx.Model(&Log{}).Count(&total).Error
	if err != nil {
		return nil, 0, err
	}
	err = tx.Order("logs.id desc").Limit(num).Offset(startIdx).Find(&logs).Error
	if err != nil {
		return nil, 0, err
	}

	formatUserLogs(logs)
	return logs, total, err
}

func SearchAllLogs(keyword string) (logs []*Log, err error) {
	err = LOG_DB.Where("type = ? or content LIKE ?", keyword, keyword+"%").Order("id desc").Limit(common.MaxRecentItems).Find(&logs).Error
	return logs, err
}

func SearchUserLogs(userId int, keyword string) (logs []*Log, err error) {
	err = LOG_DB.Where("user_id = ? and type = ?", userId, keyword).Order("id desc").Limit(common.MaxRecentItems).Find(&logs).Error
	formatUserLogs(logs)
	return logs, err
}

type Stat struct {
	Quota        int `json:"quota"`
	VisibleQuota int `json:"visible_quota"`
	CostQuota    int `json:"cost_quota"`
	Rpm          int `json:"rpm"`
	Tpm          int `json:"tpm"`
	Count        int `json:"count"`
}

type CacheStat struct {
	CacheHitTokens    int `json:"cache_hit_tokens"`
	PromptTokensTotal int `json:"prompt_tokens_total"`
}

type CacheStatByUA struct {
	Group             string `json:"group"`
	UA                string `json:"ua"`
	CacheHitTokens    int    `json:"cache_hit_tokens"`
	PromptTokensTotal int    `json:"prompt_tokens_total"`
}

type TokenQuotaStat struct {
	TokenName    string `json:"token_name" gorm:"column:token_name"`
	Quota        int    `json:"quota" gorm:"column:quota"`
	VisibleQuota int    `json:"visible_quota" gorm:"column:visible_quota"`
	CostQuota    int    `json:"cost_quota" gorm:"column:cost_quota"`
	Count        int    `json:"count" gorm:"column:count"`
}

func normalizeCacheStatUAKeywords(uaKeywords []string) []string {
	if len(uaKeywords) == 0 {
		return nil
	}
	keywords := make([]string, 0, len(uaKeywords))
	seen := make(map[string]struct{}, len(uaKeywords))
	for _, keyword := range uaKeywords {
		keyword = strings.TrimSpace(keyword)
		if keyword == "" {
			continue
		}
		normalized := strings.ToLower(keyword)
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		keywords = append(keywords, keyword)
	}
	return keywords
}

func getCacheStatNumber(otherMap map[string]interface{}, key string) int {
	if otherMap == nil {
		return 0
	}
	switch v := otherMap[key].(type) {
	case float64:
		return int(v)
	case float32:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	case int32:
		return int(v)
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(v))
		return n
	default:
		return 0
	}
}

func parseCacheStatOtherForUA(other string) (requestUA string, cacheHitTokens int, cacheCreationTokens int, isClaude bool, err error) {
	otherStr := strings.TrimSpace(other)
	if otherStr == "" || !strings.Contains(otherStr, "request_ua") {
		return "", 0, 0, false, nil
	}
	otherMap, err := common.StrToMap(otherStr)
	if err != nil {
		return "", 0, 0, false, err
	}
	if otherMap == nil {
		return "", 0, 0, false, nil
	}

	if v, ok := otherMap["request_ua"]; ok {
		requestUA = strings.TrimSpace(fmt.Sprint(v))
	}
	cacheHitTokens = getCacheStatNumber(otherMap, "cache_tokens")
	cacheCreationTokens = getCacheStatNumber(otherMap, "cache_creation_tokens")
	switch v := otherMap["claude"].(type) {
	case bool:
		isClaude = v
	case string:
		isClaude = strings.EqualFold(strings.TrimSpace(v), "true")
	}
	return requestUA, cacheHitTokens, cacheCreationTokens, isClaude, nil
}

func parseCacheStatOther(other string) (cacheHitTokens int, cacheCreationTokens int, isClaude bool, err error) {
	otherStr := strings.TrimSpace(other)
	if otherStr == "" {
		return 0, 0, false, nil
	}
	if !strings.Contains(otherStr, "cache_tokens") &&
		!strings.Contains(otherStr, "cache_creation_tokens") &&
		!strings.Contains(otherStr, "claude") {
		return 0, 0, false, nil
	}

	otherMap, err := common.StrToMap(otherStr)
	if err != nil {
		return 0, 0, false, err
	}
	if otherMap == nil {
		return 0, 0, false, nil
	}
	if v, ok := otherMap["cache_tokens"]; ok {
		switch vv := v.(type) {
		case float64:
			cacheHitTokens = int(vv)
		case int:
			cacheHitTokens = vv
		}
	}
	if v, ok := otherMap["cache_creation_tokens"]; ok {
		switch vv := v.(type) {
		case float64:
			cacheCreationTokens = int(vv)
		case int:
			cacheCreationTokens = vv
		}
	}
	if v, ok := otherMap["claude"]; ok {
		if vv, ok := v.(bool); ok && vv {
			isClaude = true
		}
	}
	return cacheHitTokens, cacheCreationTokens, isClaude, nil
}

func makeCacheStatByUAKey(group string, ua string) string {
	return group + "\x00" + ua
}

func sortedCacheStatByUAResult(stats map[string]CacheStatByUA, keywords []string) []CacheStatByUA {
	groupsSet := make(map[string]struct{})
	for _, stat := range stats {
		if stat.CacheHitTokens == 0 && stat.PromptTokensTotal == 0 {
			continue
		}
		groupsSet[stat.Group] = struct{}{}
	}

	groups := make([]string, 0, len(groupsSet))
	for group := range groupsSet {
		groups = append(groups, group)
	}
	sort.Slice(groups, func(i, j int) bool {
		left := strings.TrimSpace(groups[i])
		right := strings.TrimSpace(groups[j])
		if left == "" {
			return false
		}
		if right == "" {
			return true
		}
		leftNum, leftErr := strconv.Atoi(left)
		rightNum, rightErr := strconv.Atoi(right)
		if leftErr == nil && rightErr == nil {
			return leftNum < rightNum
		}
		return left < right
	})

	result := make([]CacheStatByUA, 0, len(groups)*len(keywords))
	for _, group := range groups {
		for _, keyword := range keywords {
			key := makeCacheStatByUAKey(group, keyword)
			stat, ok := stats[key]
			if !ok {
				stat = CacheStatByUA{Group: group, UA: keyword}
			}
			result = append(result, stat)
		}
	}
	return result
}

func SumCacheStatByUA(startTimestamp int64, endTimestamp int64, modelName string, username string, tokenName string, channel int, group string, uaKeywords []string) ([]CacheStatByUA, error) {
	keywords := normalizeCacheStatUAKeywords(uaKeywords)
	if len(keywords) == 0 {
		return []CacheStatByUA{}, nil
	}

	stats := make(map[string]CacheStatByUA)

	if common.LogSqlType == common.DatabaseTypeMySQL {
		type cacheStatByUAAggRow struct {
			Group             string `gorm:"column:group"`
			UA                string `gorm:"column:ua"`
			CacheHitTokens    int    `gorm:"column:cache_hit_tokens"`
			PromptTokensTotal int    `gorm:"column:prompt_tokens_total"`
		}

		cacheTokensExpr := `CASE WHEN JSON_VALID(other) THEN COALESCE(CAST(JSON_UNQUOTE(JSON_EXTRACT(other, '$.cache_tokens')) AS SIGNED), 0) ELSE 0 END`
		cacheCreationTokensExpr := `CASE WHEN JSON_VALID(other) THEN COALESCE(CAST(JSON_UNQUOTE(JSON_EXTRACT(other, '$.cache_creation_tokens')) AS SIGNED), 0) ELSE 0 END`
		isClaudeCond := `JSON_VALID(other) AND JSON_UNQUOTE(JSON_EXTRACT(other, '$.claude')) = 'true'`
		requestUAExpr := `CASE WHEN JSON_VALID(other) THEN COALESCE(JSON_UNQUOTE(JSON_EXTRACT(other, '$.request_ua')), '') ELSE '' END`
		requestUAMatchExpr := fmt.Sprintf("LOWER(%s)", requestUAExpr)

		caseParts := make([]string, 0, len(keywords))
		selectArgs := make([]interface{}, 0, len(keywords)*2)
		whereParts := make([]string, 0, len(keywords))
		whereArgs := make([]interface{}, 0, len(keywords))
		for _, keyword := range keywords {
			pattern := "%" + escapeLikePattern(strings.ToLower(keyword)) + "%"
			caseParts = append(caseParts, fmt.Sprintf("WHEN %s LIKE ? ESCAPE '!' THEN ?", requestUAMatchExpr))
			selectArgs = append(selectArgs, pattern, keyword)
			whereParts = append(whereParts, fmt.Sprintf("%s LIKE ? ESCAPE '!'", requestUAMatchExpr))
			whereArgs = append(whereArgs, pattern)
		}

		selectExpr := fmt.Sprintf(
			"%s AS `group`, "+
				"CASE %s ELSE '' END AS ua, "+
				"COALESCE(SUM(%s), 0) AS cache_hit_tokens, "+
				"(COALESCE(SUM(prompt_tokens), 0) + COALESCE(SUM(CASE WHEN %s THEN (%s + %s) ELSE 0 END), 0)) AS prompt_tokens_total",
			logGroupCol,
			strings.Join(caseParts, " "),
			cacheTokensExpr,
			isClaudeCond,
			cacheTokensExpr,
			cacheCreationTokensExpr,
		)

		tx := LOG_DB.Table("logs").
			Select(selectExpr, selectArgs...).
			Where("type = ?", LogTypeConsume)

		if username != "" {
			tx = tx.Where("username = ?", username)
		}
		if tokenName != "" {
			tx = tx.Where("token_name = ?", tokenName)
		}
		if startTimestamp != 0 {
			tx = tx.Where("created_at >= ?", startTimestamp)
		}
		if endTimestamp != 0 {
			tx = tx.Where("created_at <= ?", endTimestamp)
		}
		if modelName != "" {
			tx = tx.Where("model_name like ?", modelName)
		}
		if channel != 0 {
			tx = tx.Where("channel_id = ?", channel)
		}
		if group != "" {
			tx = tx.Where(logGroupCol+" = ?", group)
		}
		tx = tx.Where("("+strings.Join(whereParts, " OR ")+")", whereArgs...)

		rows := make([]cacheStatByUAAggRow, 0, len(keywords))
		if err := tx.Group(logGroupCol + ", ua").Scan(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			if row.UA == "" {
				continue
			}
			row.Group = strings.TrimSpace(row.Group)
			stats[makeCacheStatByUAKey(row.Group, row.UA)] = CacheStatByUA{
				Group:             row.Group,
				UA:                row.UA,
				CacheHitTokens:    row.CacheHitTokens,
				PromptTokensTotal: row.PromptTokensTotal,
			}
		}
		return sortedCacheStatByUAResult(stats, keywords), nil
	}

	tx := LOG_DB.Model(&Log{}).
		Select(logGroupCol+", prompt_tokens, other").
		Where("type = ?", LogTypeConsume)

	if username != "" {
		tx = tx.Where("username = ?", username)
	}
	if tokenName != "" {
		tx = tx.Where("token_name = ?", tokenName)
	}
	if startTimestamp != 0 {
		tx = tx.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("created_at <= ?", endTimestamp)
	}
	if modelName != "" {
		tx = tx.Where("model_name like ?", modelName)
	}
	if channel != 0 {
		tx = tx.Where("channel_id = ?", channel)
	}
	if group != "" {
		tx = tx.Where(logGroupCol+" = ?", group)
	}

	rows, err := tx.Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	lowerKeywords := make([]string, 0, len(keywords))
	for _, keyword := range keywords {
		lowerKeywords = append(lowerKeywords, strings.ToLower(keyword))
	}

	for rows.Next() {
		var rowGroup string
		var promptTokens int
		var other string
		if err := rows.Scan(&rowGroup, &promptTokens, &other); err != nil {
			return nil, err
		}

		requestUA, cacheHitTokens, cacheCreationTokens, isClaude, err := parseCacheStatOtherForUA(other)
		if err != nil {
			return nil, err
		}
		if requestUA == "" {
			continue
		}

		lowerUA := strings.ToLower(requestUA)
		matchedKeyword := ""
		for idx, lowerKeyword := range lowerKeywords {
			if lowerKeyword == "" {
				continue
			}
			if strings.Contains(lowerUA, lowerKeyword) {
				matchedKeyword = keywords[idx]
				break
			}
		}
		if matchedKeyword == "" {
			continue
		}

		rowGroup = strings.TrimSpace(rowGroup)
		key := makeCacheStatByUAKey(rowGroup, matchedKeyword)
		stat := stats[key]
		stat.Group = rowGroup
		stat.UA = matchedKeyword
		stat.CacheHitTokens += cacheHitTokens
		promptTokensTotal := promptTokens
		if isClaude {
			promptTokensTotal = promptTokens + cacheHitTokens + cacheCreationTokens
		}
		stat.PromptTokensTotal += promptTokensTotal
		stats[key] = stat
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return sortedCacheStatByUAResult(stats, keywords), nil
}

func SumCacheStat(startTimestamp int64, endTimestamp int64, modelName string, username string, tokenName string, channel int, group string) (CacheStat, error) {
	// MySQL: prefer SQL aggregation to avoid streaming huge rows into Go
	// which can easily take minutes and overload both MySQL and API.
	if common.LogSqlType == common.DatabaseTypeMySQL {
		type cacheStatAggRow struct {
			CacheHitTokens    int `gorm:"column:cache_hit_tokens"`
			PromptTokensTotal int `gorm:"column:prompt_tokens_total"`
		}

		cacheTokensExpr := `CASE WHEN JSON_VALID(other) THEN COALESCE(CAST(JSON_UNQUOTE(JSON_EXTRACT(other, '$.cache_tokens')) AS SIGNED), 0) ELSE 0 END`
		cacheCreationTokensExpr := `CASE WHEN JSON_VALID(other) THEN COALESCE(CAST(JSON_UNQUOTE(JSON_EXTRACT(other, '$.cache_creation_tokens')) AS SIGNED), 0) ELSE 0 END`
		isClaudeCond := `JSON_VALID(other) AND JSON_UNQUOTE(JSON_EXTRACT(other, '$.claude')) = 'true'`

		selectExpr := fmt.Sprintf(
			"COALESCE(SUM(%s), 0) AS cache_hit_tokens, "+
				"(COALESCE(SUM(prompt_tokens), 0) + COALESCE(SUM(CASE WHEN %s THEN (%s + %s) ELSE 0 END), 0)) AS prompt_tokens_total",
			cacheTokensExpr,
			isClaudeCond,
			cacheTokensExpr,
			cacheCreationTokensExpr,
		)

		tx := LOG_DB.Table("logs").
			Select(selectExpr).
			Where("type = ?", LogTypeConsume)

		if username != "" {
			tx = tx.Where("username = ?", username)
		}
		if tokenName != "" {
			tx = tx.Where("token_name = ?", tokenName)
		}
		if startTimestamp != 0 {
			tx = tx.Where("created_at >= ?", startTimestamp)
		}
		if endTimestamp != 0 {
			tx = tx.Where("created_at <= ?", endTimestamp)
		}
		if modelName != "" {
			tx = tx.Where("model_name like ?", modelName)
		}
		if channel != 0 {
			tx = tx.Where("channel_id = ?", channel)
		}
		if group != "" {
			tx = tx.Where(logGroupCol+" = ?", group)
		}

		var row cacheStatAggRow
		if err := tx.Scan(&row).Error; err != nil {
			return CacheStat{}, err
		}
		return CacheStat{
			CacheHitTokens:    row.CacheHitTokens,
			PromptTokensTotal: row.PromptTokensTotal,
		}, nil
	}

	tx := LOG_DB.Model(&Log{}).
		Select("prompt_tokens, other").
		Where("type = ?", LogTypeConsume)

	if username != "" {
		tx = tx.Where("username = ?", username)
	}
	if tokenName != "" {
		tx = tx.Where("token_name = ?", tokenName)
	}
	if startTimestamp != 0 {
		tx = tx.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("created_at <= ?", endTimestamp)
	}
	if modelName != "" {
		tx = tx.Where("model_name like ?", modelName)
	}
	if channel != 0 {
		tx = tx.Where("channel_id = ?", channel)
	}
	if group != "" {
		tx = tx.Where(logGroupCol+" = ?", group)
	}

	rows, err := tx.Rows()
	if err != nil {
		return CacheStat{}, err
	}
	defer rows.Close()

	stat := CacheStat{}
	for rows.Next() {
		var promptTokens int
		var other string
		if err := rows.Scan(&promptTokens, &other); err != nil {
			return CacheStat{}, err
		}

		cacheHitTokens, cacheCreationTokens, isClaude, err := parseCacheStatOther(other)
		if err != nil {
			return CacheStat{}, err
		}

		stat.CacheHitTokens += cacheHitTokens

		promptTokensTotal := promptTokens
		if isClaude {
			promptTokensTotal = promptTokens + cacheHitTokens + cacheCreationTokens
		}
		stat.PromptTokensTotal += promptTokensTotal
	}

	if err := rows.Err(); err != nil {
		return CacheStat{}, err
	}
	return stat, nil
}

func buildTokenQuotaStatQuery(startTimestamp int64, endTimestamp int64, modelName string, username string, tokenName string, channel int, group string) *gorm.DB {
	tx := LOG_DB.Table("logs").
		Select("token_name, COALESCE(sum(quota), 0) quota, COALESCE(sum(visible_quota), 0) visible_quota, COALESCE(sum(cost_quota), 0) cost_quota, count(*) count").
		Where("type = ?", LogTypeConsume)

	if username != "" {
		tx = tx.Where("username = ?", username)
	}
	if tokenName != "" {
		tx = tx.Where("token_name = ?", tokenName)
	}
	if startTimestamp != 0 {
		tx = tx.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("created_at <= ?", endTimestamp)
	}
	if modelName != "" {
		tx = tx.Where("model_name like ?", modelName)
	}
	if channel != 0 {
		tx = tx.Where("channel_id = ?", channel)
	}
	if group != "" {
		tx = tx.Where(logGroupCol+" = ?", group)
	}

	return tx.Group("token_name").Order("quota desc")
}

func SumTokenQuotaStat(startTimestamp int64, endTimestamp int64, modelName string, username string, tokenName string, channel int, group string) ([]TokenQuotaStat, error) {
	stats := make([]TokenQuotaStat, 0)
	err := buildTokenQuotaStatQuery(startTimestamp, endTimestamp, modelName, username, tokenName, channel, group).Scan(&stats).Error
	if err != nil {
		return nil, err
	}
	return stats, nil
}

func SumTokenQuotaStatByUserId(userId int, startTimestamp int64, endTimestamp int64, modelName string, tokenName string, channel int, group string) ([]TokenQuotaStat, error) {
	if userId <= 0 {
		return []TokenQuotaStat{}, nil
	}
	stats := make([]TokenQuotaStat, 0)
	tx := buildTokenQuotaStatQuery(startTimestamp, endTimestamp, modelName, "", tokenName, channel, group)
	tx = tx.Where("user_id = ?", userId)
	if err := tx.Scan(&stats).Error; err != nil {
		return nil, err
	}
	return stats, nil
}

func SumUsedQuota(logType int, startTimestamp int64, endTimestamp int64, modelName string, username string, tokenName string, channel int, group string) (stat Stat) {
	type quotaCountResult struct {
		Quota        int `json:"quota"`
		VisibleQuota int `json:"visible_quota"`
		CostQuota    int `json:"cost_quota"`
		Count        int `json:"count"`
	}
	type rpmTpmResult struct {
		Rpm int `json:"rpm"`
		Tpm int `json:"tpm"`
	}

	tx := LOG_DB.Table("logs").Select("COALESCE(sum(quota), 0) quota, COALESCE(sum(visible_quota), 0) visible_quota, COALESCE(sum(cost_quota), 0) cost_quota, count(*) count")

	// 为rpm和tpm创建单独的查询
	rpmTpmQuery := LOG_DB.Table("logs").Select("count(*) rpm, COALESCE(sum(prompt_tokens), 0) + COALESCE(sum(completion_tokens), 0) tpm")

	if username != "" {
		tx = tx.Where("username = ?", username)
		rpmTpmQuery = rpmTpmQuery.Where("username = ?", username)
	}
	if tokenName != "" {
		tx = tx.Where("token_name = ?", tokenName)
		rpmTpmQuery = rpmTpmQuery.Where("token_name = ?", tokenName)
	}
	if startTimestamp != 0 {
		tx = tx.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("created_at <= ?", endTimestamp)
	}
	if modelName != "" {
		tx = tx.Where("model_name like ?", modelName)
		rpmTpmQuery = rpmTpmQuery.Where("model_name like ?", modelName)
	}
	if channel != 0 {
		tx = tx.Where("channel_id = ?", channel)
		rpmTpmQuery = rpmTpmQuery.Where("channel_id = ?", channel)
	}
	if group != "" {
		tx = tx.Where(logGroupCol+" = ?", group)
		rpmTpmQuery = rpmTpmQuery.Where(logGroupCol+" = ?", group)
	}

	tx = tx.Where("type = ?", LogTypeConsume)
	rpmTpmQuery = rpmTpmQuery.Where("type = ?", LogTypeConsume)

	// 只统计最近60秒的rpm和tpm
	rpmTpmQuery = rpmTpmQuery.Where("created_at >= ?", time.Now().Add(-60*time.Second).Unix())

	// 执行查询
	var quotaCount quotaCountResult
	tx.Scan(&quotaCount)
	var rpmTpm rpmTpmResult
	rpmTpmQuery.Scan(&rpmTpm)

	stat.Quota = quotaCount.Quota
	stat.VisibleQuota = quotaCount.VisibleQuota
	stat.CostQuota = quotaCount.CostQuota
	stat.Count = quotaCount.Count
	stat.Rpm = rpmTpm.Rpm
	stat.Tpm = rpmTpm.Tpm

	return stat
}

func SumUsedQuotaByUserId(userId int, logType int, startTimestamp int64, endTimestamp int64, modelName string, tokenName string, channel int, group string) (stat Stat) {
	type quotaCountResult struct {
		Quota        int `json:"quota"`
		VisibleQuota int `json:"visible_quota"`
		CostQuota    int `json:"cost_quota"`
		Count        int `json:"count"`
	}
	type rpmTpmResult struct {
		Rpm int `json:"rpm"`
		Tpm int `json:"tpm"`
	}

	nowUnix := time.Now().Unix()
	effectiveEnd := endTimestamp
	if effectiveEnd == 0 || effectiveEnd > nowUnix {
		effectiveEnd = nowUnix
	}

	// Optimization: when the query has no per-model/token/channel/group filters, use the daily stats table
	// for quota/count and only query logs for rpm/tpm (last 60s).
	canUseDaily := userId > 0 &&
		strings.TrimSpace(modelName) == "" &&
		strings.TrimSpace(tokenName) == "" &&
		channel == 0 &&
		strings.TrimSpace(group) == "" &&
		startTimestamp > 0 &&
		effectiveEnd > 0 &&
		effectiveEnd >= startTimestamp

	if canUseDaily {
		startLocal := time.Unix(startTimestamp, 0).In(time.Local)
		endLocal := time.Unix(effectiveEnd, 0).In(time.Local)

		startDayInt := common.DateToInt(startLocal)
		endDayInt := common.DateToInt(endLocal)
		startDayStartUnix := common.GetStartOfDayUnix(startLocal)
		endDayStartUnix := common.GetStartOfDayUnix(endLocal)
		todayDayInt := common.GetTodayDateInt()

		// If the query range stays within a single day, we can use the daily row only when it covers
		// the whole available day segment (i.e. full past day, or today with end >= now).
		if startDayInt == endDayInt {
			endOfDayUnix := startDayStartUnix + 86400 - 1
			canUseSingleDayDaily :=
				startTimestamp == startDayStartUnix &&
					((effectiveEnd == nowUnix && endDayInt == todayDayInt) || effectiveEnd >= endOfDayUnix)
			if canUseSingleDayDaily {
				var row struct {
					Quota        int64 `gorm:"column:quota"`
					VisibleQuota int64 `gorm:"column:visible_quota"`
					CostQuota    int64 `gorm:"column:cost_quota"`
					Count        int64 `gorm:"column:count"`
				}
				if err := DB.Table("user_request_daily_stats").
					Select("COALESCE(SUM(used_quota), 0) AS quota, COALESCE(SUM(visible_used_quota), 0) AS visible_quota, COALESCE(SUM(cost_used_quota), 0) AS cost_quota, COALESCE(SUM(success_count), 0) AS count").
					Where("user_id = ? AND day = ?", userId, startDayInt).
					Scan(&row).Error; err == nil {
					stat.Quota = int(row.Quota)
					stat.VisibleQuota = int(row.VisibleQuota)
					stat.CostQuota = int(row.CostQuota)
					stat.Count = int(row.Count)
				}
			} else {
				// Fall back to exact log scan for the single-day arbitrary range.
				var row quotaCountResult
				if err := LOG_DB.Table("logs").
					Select("COALESCE(sum(quota), 0) quota, COALESCE(sum(visible_quota), 0) visible_quota, COALESCE(sum(cost_quota), 0) cost_quota, count(*) count").
					Where("user_id = ? AND type = ? AND created_at >= ? AND created_at <= ?", userId, LogTypeConsume, startTimestamp, effectiveEnd).
					Scan(&row).Error; err == nil {
					stat.Quota = row.Quota
					stat.VisibleQuota = row.VisibleQuota
					stat.CostQuota = row.CostQuota
					stat.Count = row.Count
				}
			}
		} else {
			// Multi-day range: use daily stats for full days, logs only for partial boundary days.
			fullStartDay := startDayInt
			if startTimestamp != startDayStartUnix {
				fullStartDay = startDayInt + 1
			}

			fullEndDay := endDayInt
			endOfEndDayUnix := endDayStartUnix + 86400 - 1
			if effectiveEnd >= endOfEndDayUnix || (effectiveEnd == nowUnix && endDayInt == todayDayInt) {
				// include end day (full past day or today's available segment)
			} else {
				fullEndDay = endDayInt - 1
			}

			// 1) partial start day
			if startTimestamp != startDayStartUnix {
				endOfStartDay := startDayStartUnix + 86400 - 1
				segEnd := effectiveEnd
				if segEnd > endOfStartDay {
					segEnd = endOfStartDay
				}
				if segEnd >= startTimestamp {
					var row quotaCountResult
					if err := LOG_DB.Table("logs").
						Select("COALESCE(sum(quota), 0) quota, COALESCE(sum(visible_quota), 0) visible_quota, COALESCE(sum(cost_quota), 0) cost_quota, count(*) count").
						Where("user_id = ? AND type = ? AND created_at >= ? AND created_at <= ?", userId, LogTypeConsume, startTimestamp, segEnd).
						Scan(&row).Error; err == nil {
						stat.Quota += row.Quota
						stat.VisibleQuota += row.VisibleQuota
						stat.CostQuota += row.CostQuota
						stat.Count += row.Count
					}
				}
			}

			// 2) full days via daily stats
			if fullStartDay <= fullEndDay {
				var row struct {
					Quota        int64 `gorm:"column:quota"`
					VisibleQuota int64 `gorm:"column:visible_quota"`
					CostQuota    int64 `gorm:"column:cost_quota"`
					Count        int64 `gorm:"column:count"`
				}
				if err := DB.Table("user_request_daily_stats").
					Select("COALESCE(SUM(used_quota), 0) AS quota, COALESCE(SUM(visible_used_quota), 0) AS visible_quota, COALESCE(SUM(cost_used_quota), 0) AS cost_quota, COALESCE(SUM(success_count), 0) AS count").
					Where("user_id = ? AND day >= ? AND day <= ?", userId, fullStartDay, fullEndDay).
					Scan(&row).Error; err == nil {
					stat.Quota += int(row.Quota)
					stat.VisibleQuota += int(row.VisibleQuota)
					stat.CostQuota += int(row.CostQuota)
					stat.Count += int(row.Count)
				}
			}

			// 3) partial end day (skip if included via daily stats)
			endIncluded := fullStartDay <= fullEndDay && endDayInt >= fullStartDay && endDayInt <= fullEndDay
			if !endIncluded {
				segStart := endDayStartUnix
				if segStart < startTimestamp {
					segStart = startTimestamp
				}
				if effectiveEnd >= segStart {
					var row quotaCountResult
					if err := LOG_DB.Table("logs").
						Select("COALESCE(sum(quota), 0) quota, COALESCE(sum(visible_quota), 0) visible_quota, COALESCE(sum(cost_quota), 0) cost_quota, count(*) count").
						Where("user_id = ? AND type = ? AND created_at >= ? AND created_at <= ?", userId, LogTypeConsume, segStart, effectiveEnd).
						Scan(&row).Error; err == nil {
						stat.Quota += row.Quota
						stat.VisibleQuota += row.VisibleQuota
						stat.CostQuota += row.CostQuota
						stat.Count += row.Count
					}
				}
			}
		}

		// rpm/tpm: last 60 seconds
		var rpmTpm rpmTpmResult
		if err := LOG_DB.Table("logs").
			Select("count(*) rpm, COALESCE(sum(prompt_tokens), 0) + COALESCE(sum(completion_tokens), 0) tpm").
			Where("user_id = ? AND type = ? AND created_at >= ?", userId, LogTypeConsume, time.Now().Add(-60*time.Second).Unix()).
			Scan(&rpmTpm).Error; err == nil {
			stat.Rpm = rpmTpm.Rpm
			stat.Tpm = rpmTpm.Tpm
		}
		return stat
	}

	// Fallback: exact log aggregation by user_id (supports additional filters).
	tx := LOG_DB.Table("logs").Select("COALESCE(sum(quota), 0) quota, COALESCE(sum(visible_quota), 0) visible_quota, COALESCE(sum(cost_quota), 0) cost_quota, count(*) count")
	rpmTpmQuery := LOG_DB.Table("logs").Select("count(*) rpm, COALESCE(sum(prompt_tokens), 0) + COALESCE(sum(completion_tokens), 0) tpm")

	if userId > 0 {
		tx = tx.Where("user_id = ?", userId)
		rpmTpmQuery = rpmTpmQuery.Where("user_id = ?", userId)
	}
	if tokenName != "" {
		tx = tx.Where("token_name = ?", tokenName)
		rpmTpmQuery = rpmTpmQuery.Where("token_name = ?", tokenName)
	}
	if startTimestamp != 0 {
		tx = tx.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("created_at <= ?", endTimestamp)
	}
	if modelName != "" {
		tx = tx.Where("model_name like ?", modelName)
		rpmTpmQuery = rpmTpmQuery.Where("model_name like ?", modelName)
	}
	if channel != 0 {
		tx = tx.Where("channel_id = ?", channel)
		rpmTpmQuery = rpmTpmQuery.Where("channel_id = ?", channel)
	}
	if group != "" {
		tx = tx.Where(logGroupCol+" = ?", group)
		rpmTpmQuery = rpmTpmQuery.Where(logGroupCol+" = ?", group)
	}

	tx = tx.Where("type = ?", LogTypeConsume)
	rpmTpmQuery = rpmTpmQuery.Where("type = ?", LogTypeConsume)
	rpmTpmQuery = rpmTpmQuery.Where("created_at >= ?", time.Now().Add(-60*time.Second).Unix())

	var quotaCount quotaCountResult
	_ = tx.Scan(&quotaCount).Error
	var rpmTpm rpmTpmResult
	_ = rpmTpmQuery.Scan(&rpmTpm).Error

	stat.Quota = quotaCount.Quota
	stat.VisibleQuota = quotaCount.VisibleQuota
	stat.CostQuota = quotaCount.CostQuota
	stat.Count = quotaCount.Count
	stat.Rpm = rpmTpm.Rpm
	stat.Tpm = rpmTpm.Tpm
	return stat
}

func SumUserCacheStatByUA(userId int, startTimestamp int64, endTimestamp int64, uaKeywords []string) ([]CacheStatByUA, error) {
	return SumCacheStatByUAForUser(userId, startTimestamp, endTimestamp, uaKeywords)
}

func SumCacheStatByUAForUser(userId int, startTimestamp int64, endTimestamp int64, uaKeywords []string) ([]CacheStatByUA, error) {
	keywords := normalizeCacheStatUAKeywords(uaKeywords)
	if len(keywords) == 0 {
		return []CacheStatByUA{}, nil
	}

	stats := make(map[string]CacheStatByUA)
	if common.LogSqlType == common.DatabaseTypeMySQL {
		type cacheStatByUAAggRow struct {
			Group             string `gorm:"column:group"`
			UA                string `gorm:"column:ua"`
			CacheHitTokens    int    `gorm:"column:cache_hit_tokens"`
			PromptTokensTotal int    `gorm:"column:prompt_tokens_total"`
		}

		cacheTokensExpr := `CASE WHEN JSON_VALID(other) THEN COALESCE(CAST(JSON_UNQUOTE(JSON_EXTRACT(other, '$.cache_tokens')) AS SIGNED), 0) ELSE 0 END`
		cacheCreationTokensExpr := `CASE WHEN JSON_VALID(other) THEN COALESCE(CAST(JSON_UNQUOTE(JSON_EXTRACT(other, '$.cache_creation_tokens')) AS SIGNED), 0) ELSE 0 END`
		isClaudeCond := `JSON_VALID(other) AND JSON_UNQUOTE(JSON_EXTRACT(other, '$.claude')) = 'true'`
		requestUAExpr := `CASE WHEN JSON_VALID(other) THEN COALESCE(JSON_UNQUOTE(JSON_EXTRACT(other, '$.request_ua')), '') ELSE '' END`
		requestUAMatchExpr := fmt.Sprintf("LOWER(%s)", requestUAExpr)

		caseParts := make([]string, 0, len(keywords))
		selectArgs := make([]interface{}, 0, len(keywords)*2)
		whereParts := make([]string, 0, len(keywords))
		whereArgs := make([]interface{}, 0, len(keywords))
		for _, keyword := range keywords {
			pattern := "%" + escapeLikePattern(strings.ToLower(keyword)) + "%"
			caseParts = append(caseParts, fmt.Sprintf("WHEN %s LIKE ? ESCAPE '!' THEN ?", requestUAMatchExpr))
			selectArgs = append(selectArgs, pattern, keyword)
			whereParts = append(whereParts, fmt.Sprintf("%s LIKE ? ESCAPE '!'", requestUAMatchExpr))
			whereArgs = append(whereArgs, pattern)
		}

		selectExpr := fmt.Sprintf(
			"%s AS `group`, "+
				"CASE %s ELSE '' END AS ua, "+
				"COALESCE(SUM(%s), 0) AS cache_hit_tokens, "+
				"(COALESCE(SUM(prompt_tokens), 0) + COALESCE(SUM(CASE WHEN %s THEN (%s + %s) ELSE 0 END), 0)) AS prompt_tokens_total",
			logGroupCol,
			strings.Join(caseParts, " "),
			cacheTokensExpr,
			isClaudeCond,
			cacheTokensExpr,
			cacheCreationTokensExpr,
		)

		tx := LOG_DB.Table("logs").
			Select(selectExpr, selectArgs...).
			Where("user_id = ? AND type = ?", userId, LogTypeConsume)
		if startTimestamp != 0 {
			tx = tx.Where("created_at >= ?", startTimestamp)
		}
		if endTimestamp != 0 {
			tx = tx.Where("created_at <= ?", endTimestamp)
		}
		tx = tx.Where("("+strings.Join(whereParts, " OR ")+")", whereArgs...)

		rows := make([]cacheStatByUAAggRow, 0, len(keywords))
		if err := tx.Group(logGroupCol + ", ua").Scan(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			if row.UA == "" {
				continue
			}
			row.Group = strings.TrimSpace(row.Group)
			stats[makeCacheStatByUAKey(row.Group, row.UA)] = CacheStatByUA{
				Group:             row.Group,
				UA:                row.UA,
				CacheHitTokens:    row.CacheHitTokens,
				PromptTokensTotal: row.PromptTokensTotal,
			}
		}
		return sortedCacheStatByUAResult(stats, keywords), nil
	}

	tx := LOG_DB.Model(&Log{}).
		Select(logGroupCol+", prompt_tokens, other").
		Where("user_id = ? AND type = ?", userId, LogTypeConsume)
	if startTimestamp != 0 {
		tx = tx.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("created_at <= ?", endTimestamp)
	}

	rows, err := tx.Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	lowerKeywords := make([]string, 0, len(keywords))
	for _, keyword := range keywords {
		lowerKeywords = append(lowerKeywords, strings.ToLower(keyword))
	}

	for rows.Next() {
		var rowGroup string
		var promptTokens int
		var other string
		if err := rows.Scan(&rowGroup, &promptTokens, &other); err != nil {
			return nil, err
		}

		requestUA, cacheHitTokens, cacheCreationTokens, isClaude, err := parseCacheStatOtherForUA(other)
		if err != nil {
			return nil, err
		}
		if requestUA == "" {
			continue
		}

		lowerUA := strings.ToLower(requestUA)
		matchedKeyword := ""
		for idx, lowerKeyword := range lowerKeywords {
			if lowerKeyword == "" {
				continue
			}
			if strings.Contains(lowerUA, lowerKeyword) {
				matchedKeyword = keywords[idx]
				break
			}
		}
		if matchedKeyword == "" {
			continue
		}

		rowGroup = strings.TrimSpace(rowGroup)
		key := makeCacheStatByUAKey(rowGroup, matchedKeyword)
		stat := stats[key]
		stat.Group = rowGroup
		stat.UA = matchedKeyword
		stat.CacheHitTokens += cacheHitTokens
		promptTokensTotal := promptTokens
		if isClaude {
			promptTokensTotal = promptTokens + cacheHitTokens + cacheCreationTokens
		}
		stat.PromptTokensTotal += promptTokensTotal
		stats[key] = stat
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return sortedCacheStatByUAResult(stats, keywords), nil
}

func SumUserCacheStat(userId int, startTimestamp int64, endTimestamp int64) (CacheStat, error) {
	// MySQL: prefer SQL aggregation to avoid streaming huge rows into Go.
	if common.LogSqlType == common.DatabaseTypeMySQL {
		type cacheStatAggRow struct {
			CacheHitTokens    int `gorm:"column:cache_hit_tokens"`
			PromptTokensTotal int `gorm:"column:prompt_tokens_total"`
		}

		cacheTokensExpr := `CASE WHEN JSON_VALID(other) THEN COALESCE(CAST(JSON_UNQUOTE(JSON_EXTRACT(other, '$.cache_tokens')) AS SIGNED), 0) ELSE 0 END`
		cacheCreationTokensExpr := `CASE WHEN JSON_VALID(other) THEN COALESCE(CAST(JSON_UNQUOTE(JSON_EXTRACT(other, '$.cache_creation_tokens')) AS SIGNED), 0) ELSE 0 END`
		isClaudeCond := `JSON_VALID(other) AND JSON_UNQUOTE(JSON_EXTRACT(other, '$.claude')) = 'true'`

		selectExpr := fmt.Sprintf(
			"COALESCE(SUM(%s), 0) AS cache_hit_tokens, "+
				"(COALESCE(SUM(prompt_tokens), 0) + COALESCE(SUM(CASE WHEN %s THEN (%s + %s) ELSE 0 END), 0)) AS prompt_tokens_total",
			cacheTokensExpr,
			isClaudeCond,
			cacheTokensExpr,
			cacheCreationTokensExpr,
		)

		tx := LOG_DB.Table("logs").
			Select(selectExpr).
			Where("user_id = ? AND type = ?", userId, LogTypeConsume)
		if startTimestamp != 0 {
			tx = tx.Where("created_at >= ?", startTimestamp)
		}
		if endTimestamp != 0 {
			tx = tx.Where("created_at <= ?", endTimestamp)
		}

		var row cacheStatAggRow
		if err := tx.Scan(&row).Error; err != nil {
			return CacheStat{}, err
		}
		return CacheStat{
			CacheHitTokens:    row.CacheHitTokens,
			PromptTokensTotal: row.PromptTokensTotal,
		}, nil
	}

	tx := LOG_DB.Model(&Log{}).
		Select("prompt_tokens, other").
		Where("user_id = ? AND type = ?", userId, LogTypeConsume)
	if startTimestamp != 0 {
		tx = tx.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("created_at <= ?", endTimestamp)
	}
	rows, err := tx.Rows()
	if err != nil {
		return CacheStat{}, err
	}
	defer rows.Close()

	stat := CacheStat{}
	for rows.Next() {
		var promptTokens int
		var other string
		if err := rows.Scan(&promptTokens, &other); err != nil {
			return CacheStat{}, err
		}

		cacheHitTokens, cacheCreationTokens, isClaude, err := parseCacheStatOther(other)
		if err != nil {
			return CacheStat{}, err
		}

		stat.CacheHitTokens += cacheHitTokens

		promptTokensTotal := promptTokens
		if isClaude {
			promptTokensTotal = promptTokens + cacheHitTokens + cacheCreationTokens
		}
		stat.PromptTokensTotal += promptTokensTotal
	}
	if err := rows.Err(); err != nil {
		return CacheStat{}, err
	}

	return stat, nil
}

func SumUsedToken(logType int, startTimestamp int64, endTimestamp int64, modelName string, username string, tokenName string) (token int) {
	tx := LOG_DB.Table("logs").Select("ifnull(sum(prompt_tokens),0) + ifnull(sum(completion_tokens),0)")
	if username != "" {
		tx = tx.Where("username = ?", username)
	}
	if tokenName != "" {
		tx = tx.Where("token_name = ?", tokenName)
	}
	if startTimestamp != 0 {
		tx = tx.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("created_at <= ?", endTimestamp)
	}
	if modelName != "" {
		tx = tx.Where("model_name = ?", modelName)
	}
	tx.Where("type = ?", LogTypeConsume).Scan(&token)
	return token
}

type TopTokenUser struct {
	UserId       int          `json:"user_id"`
	Username     string       `json:"username"`
	AvatarSeed   string       `json:"avatar_seed"`
	Quota        int          `json:"quota"`
	Tokens       int          `json:"tokens"`
	SuccessCount int          `json:"success_count"`
	ModelQuota   []ModelQuota `json:"model_quota"`
}

type ModelQuota struct {
	ModelName    string `json:"model_name"`
	Quota        int    `json:"quota"`
	SuccessCount int64  `json:"success_count"`
}

type topTokenRow struct {
	UserId       int    `json:"user_id"`
	Username     string `json:"username"`
	AvatarSeed   string `json:"avatar_seed"`
	Quota        int    `json:"quota"`
	Tokens       int    `json:"tokens"`
	SuccessCount int    `json:"success_count"`
}

type modelQuotaRow struct {
	UserId       int    `json:"user_id"`
	ModelName    string `json:"model_name"`
	Quota        int    `json:"quota"`
	SuccessCount int64  `json:"success_count"`
}

func GetDailyTopTokenUsers(startTimestamp int64, endTimestamp int64, limit int) ([]TopTokenUser, error) {
	return GetDailyTopKingUsers(startTimestamp, endTimestamp, limit, common.StompKingRankModeQuota)
}

func GetDailyTopKingUsers(startTimestamp int64, endTimestamp int64, limit int, rankMode string) ([]TopTokenUser, error) {
	if limit <= 0 {
		limit = 10
	}
	var orderBy string
	var dailyQuotaColumn string
	var logQuotaColumn string
	switch rankMode {
	case common.StompKingRankModeQuota:
		orderBy = "s.used_quota desc, s.success_count desc"
		dailyQuotaColumn = "s.used_quota"
		logQuotaColumn = "quota"
	case common.StompKingRankModeVisibleQuota:
		orderBy = "s.visible_used_quota desc, s.success_count desc"
		dailyQuotaColumn = "s.visible_used_quota"
		logQuotaColumn = "visible_quota"
	case common.StompKingRankModeCostQuota:
		orderBy = "s.cost_used_quota desc, s.success_count desc"
		dailyQuotaColumn = "s.cost_used_quota"
		logQuotaColumn = "cost_quota"
	case common.StompKingRankModeSuccessCount:
		orderBy = "s.success_count desc, s.used_quota desc"
		dailyQuotaColumn = "s.used_quota"
		logQuotaColumn = "quota"
	default:
		return nil, fmt.Errorf("invalid StompKingRankMode: %s", rankMode)
	}

	day := common.DateToInt(time.Unix(startTimestamp, 0))
	rows := make([]topTokenRow, 0)
	if err := DB.Table("user_request_daily_stats AS s").
		Select(fmt.Sprintf("s.user_id, u.username, u.avatar_seed, %s AS quota, s.tokens, s.success_count", dailyQuotaColumn)).
		Joins("JOIN users u ON u.id = s.user_id").
		Where("s.day = ?", day).
		Order(orderBy).
		Limit(limit).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return []TopTokenUser{}, nil
	}
	userIds := make([]int, 0, len(rows))
	for _, row := range rows {
		userIds = append(userIds, row.UserId)
	}

	modelQuotaRows := make([]modelQuotaRow, 0)
	modelQuotaTx := LOG_DB.Table("logs").
		Select(fmt.Sprintf("user_id, model_name, COALESCE(sum(%s),0) AS quota, COUNT(1) AS success_count", logQuotaColumn)).
		Where("type = ?", LogTypeConsume).
		Where("user_id IN ?", userIds)
	if startTimestamp != 0 {
		modelQuotaTx = modelQuotaTx.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		modelQuotaTx = modelQuotaTx.Where("created_at <= ?", endTimestamp)
	}

	modelQuotaOrderBy := "user_id asc, quota desc"
	if rankMode == common.StompKingRankModeSuccessCount {
		modelQuotaOrderBy = "user_id asc, success_count desc"
	}
	if err := modelQuotaTx.
		Group("user_id, model_name").
		Order(modelQuotaOrderBy).
		Scan(&modelQuotaRows).Error; err != nil {
		return nil, err
	}
	modelQuotaMap := make(map[int][]ModelQuota, len(userIds))
	for _, row := range modelQuotaRows {
		modelQuotaMap[row.UserId] = append(modelQuotaMap[row.UserId], ModelQuota{
			ModelName:    row.ModelName,
			Quota:        row.Quota,
			SuccessCount: row.SuccessCount,
		})
	}
	items := make([]TopTokenUser, 0, len(rows))
	for _, row := range rows {
		modelQuota := modelQuotaMap[row.UserId]
		if modelQuota == nil {
			modelQuota = []ModelQuota{}
		}
		items = append(items, TopTokenUser{
			UserId:       row.UserId,
			Username:     row.Username,
			AvatarSeed:   row.AvatarSeed,
			Quota:        row.Quota,
			Tokens:       row.Tokens,
			SuccessCount: row.SuccessCount,
			ModelQuota:   modelQuota,
		})
	}
	return items, nil
}

func DeleteOldLog(ctx context.Context, targetTimestamp int64, limit int) (int64, error) {
	var total int64 = 0

	for {
		if nil != ctx.Err() {
			return total, ctx.Err()
		}

		result := LOG_DB.Where("created_at < ?", targetTimestamp).Limit(limit).Delete(&Log{})
		if nil != result.Error {
			return total, result.Error
		}

		total += result.RowsAffected

		if result.RowsAffected < int64(limit) {
			break
		}
	}

	return total, nil
}
