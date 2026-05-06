package controller

import (
	"errors"
	"net/http"
	"one-api/common"
	"one-api/model"
	"one-api/setting/operation_setting"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

func GetAllLogs(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	logType, _ := strconv.Atoi(c.Query("type"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	username := c.Query("username")
	tokenName := c.Query("token_name")
	modelName := c.Query("model_name")
	requestID := strings.TrimSpace(c.Query("request_id"))
	channel, _ := strconv.Atoi(c.Query("channel"))
	group, err := parseLogGroupIDQuery(c)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	logs, total, err := model.GetAllLogs(logType, startTimestamp, endTimestamp, modelName, username, tokenName, requestID, pageInfo.GetStartIdx(), pageInfo.GetPageSize(), channel, group)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(logs)
	common.ApiSuccess(c, pageInfo)
	return
}

func GetUserLogs(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	userId := c.GetInt("id")
	logType, _ := strconv.Atoi(c.Query("type"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	tokenName := c.Query("token_name")
	modelName := c.Query("model_name")
	requestID := strings.TrimSpace(c.Query("request_id"))
	group, err := parseLogGroupIDQuery(c)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	logs, total, err := model.GetUserLogs(userId, logType, startTimestamp, endTimestamp, modelName, tokenName, requestID, pageInfo.GetStartIdx(), pageInfo.GetPageSize(), group)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(logs)
	common.ApiSuccess(c, pageInfo)
	return
}

func SearchAllLogs(c *gin.Context) {
	keyword := c.Query("keyword")
	logs, err := model.SearchAllLogs(keyword)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    logs,
	})
	return
}

func SearchUserLogs(c *gin.Context) {
	keyword := c.Query("keyword")
	userId := c.GetInt("id")
	logs, err := model.SearchUserLogs(userId, keyword)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    logs,
	})
	return
}

func GetLogByKey(c *gin.Context) {
	key := c.Query("key")
	logs, err := model.GetLogByKey(key)
	if err != nil {
		c.JSON(200, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(200, gin.H{
		"success": true,
		"message": "",
		"data":    logs,
	})
}

func extractLogSnippetByRequestID(logContent string, requestID string, beforeBytes int, afterBytes int) string {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" || logContent == "" {
		return ""
	}
	if beforeBytes < 0 {
		beforeBytes = 0
	}
	if afterBytes < 0 {
		afterBytes = 0
	}

	needles := []string{
		"REQUEST_ID: " + requestID,
		"(request_id=" + requestID + ")",
	}

	idx := -1
	for _, needle := range needles {
		if needle == "" {
			continue
		}
		idx = strings.Index(logContent, needle)
		if idx >= 0 {
			break
		}
	}
	if idx < 0 {
		return ""
	}

	start := idx - beforeBytes
	if start < 0 {
		start = 0
	}
	end := idx + afterBytes
	if end > len(logContent) {
		end = len(logContent)
	}

	if lineStart := strings.LastIndex(logContent[:start], "\n"); lineStart >= 0 {
		start = lineStart + 1
	}
	if lineEnd := strings.Index(logContent[end:], "\n"); lineEnd >= 0 {
		end += lineEnd + 1
	}
	if start >= end {
		return ""
	}
	return logContent[start:end]
}

func GetLogsStat(c *gin.Context) {
	logType, _ := strconv.Atoi(c.Query("type"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	tokenName := c.Query("token_name")
	username := c.Query("username")
	modelName := c.Query("model_name")
	channel, _ := strconv.Atoi(c.Query("channel"))
	group, err := parseLogGroupIDQuery(c)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	stat := model.SumUsedQuota(logType, startTimestamp, endTimestamp, modelName, username, tokenName, channel, group)
	//tokenNum := model.SumUsedToken(logType, startTimestamp, endTimestamp, modelName, username, "")
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"quota":         stat.Quota,
			"visible_quota": stat.VisibleQuota,
			"cost_quota":    stat.CostQuota,
			"count":         stat.Count,
			"rpm":           stat.Rpm,
			"tpm":           stat.Tpm,
		},
	})
	return
}

func GetLogsSelfStat(c *gin.Context) {
	userId := c.GetInt("id")
	logType, _ := strconv.Atoi(c.Query("type"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	tokenName := c.Query("token_name")
	modelName := c.Query("model_name")
	channel, _ := strconv.Atoi(c.Query("channel"))
	group, err := parseLogGroupIDQuery(c)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	quotaNum := model.SumUsedQuotaByUserId(userId, logType, startTimestamp, endTimestamp, modelName, tokenName, channel, group)
	quotaLegacy, legacyErr := model.HasLegacyHiddenUserConsumeLogs(userId, startTimestamp, endTimestamp, modelName, tokenName, channel, group)
	if legacyErr != nil {
		common.ApiError(c, legacyErr)
		return
	}
	//tokenNum := model.SumUsedToken(logType, startTimestamp, endTimestamp, modelName, username, tokenName)
	c.JSON(200, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"quota":         quotaNum.Quota,
			"visible_quota": quotaNum.VisibleQuota,
			"cost_quota":    quotaNum.CostQuota,
			"count":         quotaNum.Count,
			"rpm":           quotaNum.Rpm,
			"tpm":           quotaNum.Tpm,
			"quota_legacy":  quotaLegacy,
			//"token": tokenNum,
		},
	})
	return
}

func GetLogsTokenQuotaStat(c *gin.Context) {
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	username := c.Query("username")
	tokenName := c.Query("token_name")
	modelName := c.Query("model_name")
	channel, _ := strconv.Atoi(c.Query("channel"))
	group, err := parseLogGroupIDQuery(c)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}

	stat, err := model.SumTokenQuotaStat(startTimestamp, endTimestamp, modelName, username, tokenName, channel, group)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, stat)
}

func GetLogsSelfTokenQuotaStat(c *gin.Context) {
	userId := c.GetInt("id")
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	tokenName := c.Query("token_name")
	modelName := c.Query("model_name")
	channel, _ := strconv.Atoi(c.Query("channel"))
	group, err := parseLogGroupIDQuery(c)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}

	stat, err := model.SumTokenQuotaStatByUserId(userId, startTimestamp, endTimestamp, modelName, tokenName, channel, group)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, stat)
}

func GetLogsSelfCacheStat(c *gin.Context) {
	userId := c.GetInt("id")
	startTimestamp, err := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	if err != nil || startTimestamp <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "start_timestamp is required",
		})
		return
	}
	endTimestamp, err := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	if err != nil || endTimestamp <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "end_timestamp is required",
		})
		return
	}
	if endTimestamp < startTimestamp {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "end_timestamp must be greater than or equal to start_timestamp",
		})
		return
	}

	stat, err := model.SumUserCacheStat(userId, startTimestamp, endTimestamp)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, stat)
}

func GetLogsSelfCacheStatByUA(c *gin.Context) {
	userId := c.GetInt("id")
	startTimestamp, err := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	if err != nil || startTimestamp <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "start_timestamp is required",
		})
		return
	}
	endTimestamp, err := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	if err != nil || endTimestamp <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "end_timestamp is required",
		})
		return
	}
	if endTimestamp < startTimestamp {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "end_timestamp must be greater than or equal to start_timestamp",
		})
		return
	}

	stat, err := model.SumUserCacheStatByUA(userId, startTimestamp, endTimestamp, parseMonitorUAContains())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, stat)
}

func GetLogsCacheStat(c *gin.Context) {
	startTimestamp, err := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	if err != nil || startTimestamp <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "start_timestamp is required",
		})
		return
	}
	endTimestamp, err := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	if err != nil || endTimestamp <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "end_timestamp is required",
		})
		return
	}
	if endTimestamp < startTimestamp {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "end_timestamp must be greater than or equal to start_timestamp",
		})
		return
	}

	username := c.Query("username")
	tokenName := c.Query("token_name")
	modelName := c.Query("model_name")
	channel, _ := strconv.Atoi(c.Query("channel"))
	group, err := parseLogGroupIDQuery(c)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}

	stat, err := model.SumCacheStat(startTimestamp, endTimestamp, modelName, username, tokenName, channel, group)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, stat)
}

func parseMonitorUAContains() []string {
	raw := strings.TrimSpace(operation_setting.GetMonitorSetting().ServiceStatusUAContains)
	if raw == "" {
		return nil
	}
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	raw = strings.ReplaceAll(raw, "\r", "\n")
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		switch r {
		case '\n', ',', ';':
			return true
		default:
			return false
		}
	})
	if len(parts) == 0 {
		return nil
	}

	keywords := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		keyword := strings.TrimSpace(part)
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

func GetLogsCacheStatByUA(c *gin.Context) {
	startTimestamp, err := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	if err != nil || startTimestamp <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "start_timestamp is required",
		})
		return
	}
	endTimestamp, err := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	if err != nil || endTimestamp <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "end_timestamp is required",
		})
		return
	}
	if endTimestamp < startTimestamp {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "end_timestamp must be greater than or equal to start_timestamp",
		})
		return
	}

	username := c.Query("username")
	tokenName := c.Query("token_name")
	modelName := c.Query("model_name")
	channel, _ := strconv.Atoi(c.Query("channel"))
	group, err := parseLogGroupIDQuery(c)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}

	stat, err := model.SumCacheStatByUA(startTimestamp, endTimestamp, modelName, username, tokenName, channel, group, parseMonitorUAContains())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, stat)
}

func GetLogsGlobalCacheStat(c *gin.Context) {
	startTimestamp, err := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	if err != nil || startTimestamp <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "start_timestamp is required",
		})
		return
	}
	endTimestamp, err := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	if err != nil || endTimestamp <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "end_timestamp is required",
		})
		return
	}
	if endTimestamp < startTimestamp {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "end_timestamp must be greater than or equal to start_timestamp",
		})
		return
	}

	stat, err := model.SumCacheStat(startTimestamp, endTimestamp, "", "", "", 0, "")
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, stat)
}

func GetLogsGlobalCacheStatByUA(c *gin.Context) {
	startTimestamp, err := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	if err != nil || startTimestamp <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "start_timestamp is required",
		})
		return
	}
	endTimestamp, err := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	if err != nil || endTimestamp <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "end_timestamp is required",
		})
		return
	}
	if endTimestamp < startTimestamp {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "end_timestamp must be greater than or equal to start_timestamp",
		})
		return
	}

	stat, err := model.SumCacheStatByUA(startTimestamp, endTimestamp, "", "", "", 0, "", parseMonitorUAContains())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, stat)
}

func DeleteHistoryLogs(c *gin.Context) {
	targetTimestamp, _ := strconv.ParseInt(c.Query("target_timestamp"), 10, 64)
	if targetTimestamp == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "target timestamp is required",
		})
		return
	}
	count, err := model.DeleteOldLog(c.Request.Context(), targetTimestamp, 100)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    count,
	})
	return
}

func parseLogGroupIDQuery(c *gin.Context) (string, error) {
	raw := strings.TrimSpace(c.Query("group_id"))
	if raw == "" {
		raw = strings.TrimSpace(c.Query("group"))
	}
	if raw == "" {
		return "", nil
	}
	id, err := strconv.Atoi(raw)
	if err != nil || id <= 0 {
		return "", errors.New("group_id 无效")
	}
	return strconv.Itoa(id), nil
}
