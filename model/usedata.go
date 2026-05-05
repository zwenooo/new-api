package model

import (
	"fmt"
	"gorm.io/gorm"
	"one-api/common"
	"sync"
	"time"
)

// QuotaData 柱状图数据
type QuotaData struct {
	Id           int    `json:"id"`
	UserID       int    `json:"user_id" gorm:"index"`
	Username     string `json:"username" gorm:"index:idx_qdt_model_user_name,priority:2;size:64;default:''"`
	ModelName    string `json:"model_name" gorm:"index:idx_qdt_model_user_name,priority:1;size:64;default:''"`
	CreatedAt    int64  `json:"created_at" gorm:"bigint;index:idx_qdt_created_at,priority:2"`
	TokenUsed    int    `json:"token_used" gorm:"default:0"`
	Count        int    `json:"count" gorm:"default:0"`
	Quota        int    `json:"quota" gorm:"default:0"`
	VisibleQuota int    `json:"visible_quota" gorm:"default:0"`
	CostQuota    int    `json:"cost_quota" gorm:"default:0"`
	QuotaLegacy  bool   `json:"quota_legacy,omitempty" gorm:"-"`
}

func UpdateQuotaData() {
	for {
		if common.DataExportEnabled {
			common.SysLog("正在更新数据看板数据...")
			SaveQuotaDataCache()
		}
		time.Sleep(time.Duration(common.DataExportInterval) * time.Minute)
	}
}

var CacheQuotaData = make(map[string]*QuotaData)
var CacheQuotaDataLock = sync.Mutex{}

func logQuotaDataCache(userId int, username string, modelName string, quota int, visibleQuota int, costQuota int, createdAt int64, tokenUsed int) {
	key := fmt.Sprintf("%d-%s-%s-%d", userId, username, modelName, createdAt)
	quotaData, ok := CacheQuotaData[key]
	if ok {
		quotaData.Count += 1
		quotaData.Quota += quota
		quotaData.VisibleQuota += visibleQuota
		quotaData.CostQuota += costQuota
		quotaData.TokenUsed += tokenUsed
	} else {
		quotaData = &QuotaData{
			UserID:       userId,
			Username:     username,
			ModelName:    modelName,
			CreatedAt:    createdAt,
			Count:        1,
			Quota:        quota,
			VisibleQuota: visibleQuota,
			CostQuota:    costQuota,
			TokenUsed:    tokenUsed,
		}
	}
	CacheQuotaData[key] = quotaData
}

func LogQuotaData(userId int, username string, modelName string, quota int, visibleQuota int, costQuota int, createdAt int64, tokenUsed int) {
	// 只精确到小时
	createdAt = createdAt - (createdAt % 3600)

	CacheQuotaDataLock.Lock()
	defer CacheQuotaDataLock.Unlock()
	logQuotaDataCache(userId, username, modelName, quota, visibleQuota, costQuota, createdAt, tokenUsed)
}

func SaveQuotaDataCache() {
	CacheQuotaDataLock.Lock()
	defer CacheQuotaDataLock.Unlock()
	size := len(CacheQuotaData)
	// If the row already exists for the (user_id, username, model_name, created_at hour) key,
	// UPDATE it first to avoid an extra SELECT per flush entry.
	for _, quotaData := range CacheQuotaData {
		if quotaData == nil {
			continue
		}
		tx := DB.Table("quota_data").Where(
			"user_id = ? and username = ? and model_name = ? and created_at = ?",
			quotaData.UserID, quotaData.Username, quotaData.ModelName, quotaData.CreatedAt,
		).Updates(map[string]interface{}{
			"count":         gorm.Expr("count + ?", quotaData.Count),
			"quota":         gorm.Expr("quota + ?", quotaData.Quota),
			"visible_quota": gorm.Expr("visible_quota + ?", quotaData.VisibleQuota),
			"cost_quota":    gorm.Expr("cost_quota + ?", quotaData.CostQuota),
			"token_used":    gorm.Expr("token_used + ?", quotaData.TokenUsed),
		})
		if tx.Error != nil {
			common.SysLog(fmt.Sprintf("SaveQuotaDataCache update error: %v", tx.Error))
			continue
		}
		if tx.RowsAffected == 0 {
			if err := DB.Table("quota_data").Create(quotaData).Error; err != nil {
				common.SysLog(fmt.Sprintf("SaveQuotaDataCache insert error: %v", err))
			}
		}
	}
	CacheQuotaData = make(map[string]*QuotaData)
	common.SysLog(fmt.Sprintf("保存数据看板数据成功，共保存%d条数据", size))
}

func GetQuotaDataByUsername(username string, startTime int64, endTime int64) (quotaData []*QuotaData, err error) {
	var quotaDatas []*QuotaData
	// 从quota_data表中查询数据
	err = DB.Table("quota_data").Where("username = ? and created_at >= ? and created_at <= ?", username, startTime, endTime).Find(&quotaDatas).Error
	return quotaDatas, err
}

func GetQuotaDataByUserId(userId int, startTime int64, endTime int64) (quotaData []*QuotaData, err error) {
	var quotaDatas []*QuotaData
	// 从quota_data表中查询数据
	err = DB.Table("quota_data").Where("user_id = ? and created_at >= ? and created_at <= ?", userId, startTime, endTime).Find(&quotaDatas).Error
	return quotaDatas, err
}

func GetAllQuotaDates(startTime int64, endTime int64, username string) (quotaData []*QuotaData, err error) {
	if username != "" {
		return GetQuotaDataByUsername(username, startTime, endTime)
	}
	var quotaDatas []*QuotaData
	// 从quota_data表中查询数据
	// only select model_name, sum(count) as count, sum(quota) as quota, model_name, created_at from quota_data group by model_name, created_at;
	//err = DB.Table("quota_data").Where("created_at >= ? and created_at <= ?", startTime, endTime).Find(&quotaDatas).Error
	err = DB.Table("quota_data").Select("model_name, sum(count) as count, sum(quota) as quota, sum(visible_quota) as visible_quota, sum(cost_quota) as cost_quota, sum(token_used) as token_used, created_at").Where("created_at >= ? and created_at <= ?", startTime, endTime).Group("model_name, created_at").Find(&quotaDatas).Error
	return quotaDatas, err
}
