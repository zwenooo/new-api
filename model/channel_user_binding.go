package model

import (
	"errors"
	"fmt"
	"one-api/common"
	"sync"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ChannelUserBinding struct {
	ChannelId    int   `json:"channel_id" gorm:"primaryKey;autoIncrement:false;index;column:channel_id"`
	UserId       int   `json:"user_id" gorm:"primaryKey;autoIncrement:false;index;column:user_id"`
	CreatedTime  int64 `json:"created_time" gorm:"bigint;autoCreateTime;column:created_time"`
}

var (
	channelBindingLock         sync.RWMutex
	channelBoundUsersCache     map[int]map[int]struct{}
	userBoundChannelsCache     map[int]map[int]struct{}
	channelBindingCacheReady   bool
	channelBindingCacheInitErr error
)

func InitChannelBindingCache() error {
	if !common.MemoryCacheEnabled {
		return nil
	}

	var bindings []ChannelUserBinding
	if err := DB.Find(&bindings).Error; err != nil {
		channelBindingLock.Lock()
		channelBoundUsersCache = map[int]map[int]struct{}{}
		userBoundChannelsCache = map[int]map[int]struct{}{}
		channelBindingCacheReady = false
		channelBindingCacheInitErr = err
		channelBindingLock.Unlock()
		common.SysLog("failed to init channel binding cache: " + err.Error())
		return err
	}

	newChannelBoundUsers := make(map[int]map[int]struct{})
	newUserBoundChannels := make(map[int]map[int]struct{})
	for _, b := range bindings {
		if b.ChannelId <= 0 || b.UserId <= 0 {
			continue
		}
		uSet, ok := newChannelBoundUsers[b.ChannelId]
		if !ok {
			uSet = make(map[int]struct{})
			newChannelBoundUsers[b.ChannelId] = uSet
		}
		uSet[b.UserId] = struct{}{}

		chSet, ok := newUserBoundChannels[b.UserId]
		if !ok {
			chSet = make(map[int]struct{})
			newUserBoundChannels[b.UserId] = chSet
		}
		chSet[b.ChannelId] = struct{}{}
	}

	channelBindingLock.Lock()
	channelBoundUsersCache = newChannelBoundUsers
	userBoundChannelsCache = newUserBoundChannels
	channelBindingCacheReady = true
	channelBindingCacheInitErr = nil
	channelBindingLock.Unlock()

	common.SysLog(fmt.Sprintf("channel bindings synced from database, bindings=%d", len(bindings)))
	return nil
}

func ensureChannelBindingCacheReady() error {
	if !common.MemoryCacheEnabled {
		return nil
	}
	channelBindingLock.RLock()
	ready := channelBindingCacheReady
	err := channelBindingCacheInitErr
	channelBindingLock.RUnlock()
	if ready {
		return nil
	}
	if err != nil {
		return err
	}
	return errors.New("channel binding cache not initialized")
}

func ChannelHasAnyBinding(channelId int) (bool, error) {
	if channelId <= 0 {
		return false, errors.New("invalid channel id")
	}

	if !common.MemoryCacheEnabled {
		var count int64
		if err := DB.Model(&ChannelUserBinding{}).Where("channel_id = ?", channelId).Count(&count).Error; err != nil {
			return false, err
		}
		return count > 0, nil
	}

	if err := ensureChannelBindingCacheReady(); err != nil {
		return false, err
	}
	channelBindingLock.RLock()
	uSet := channelBoundUsersCache[channelId]
	channelBindingLock.RUnlock()
	return len(uSet) > 0, nil
}

func IsChannelBoundToUser(channelId int, userId int) (bool, error) {
	if channelId <= 0 || userId <= 0 {
		return false, errors.New("invalid channel_id or user_id")
	}

	if !common.MemoryCacheEnabled {
		var count int64
		if err := DB.Model(&ChannelUserBinding{}).
			Where("channel_id = ? AND user_id = ?", channelId, userId).
			Count(&count).Error; err != nil {
			return false, err
		}
		return count > 0, nil
	}

	if err := ensureChannelBindingCacheReady(); err != nil {
		return false, err
	}
	channelBindingLock.RLock()
	uSet := channelBoundUsersCache[channelId]
	_, ok := uSet[userId]
	channelBindingLock.RUnlock()
	return ok, nil
}

func BatchReplaceChannelBoundUsers(channelIds []int, userIds []int) error {
	if len(channelIds) == 0 {
		return errors.New("channel ids is empty")
	}

	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("channel_id IN ?", channelIds).Delete(&ChannelUserBinding{}).Error; err != nil {
			return err
		}
		if len(userIds) == 0 {
			return nil
		}

		bindings := make([]ChannelUserBinding, 0, len(channelIds)*len(userIds))
		for _, channelId := range channelIds {
			if channelId <= 0 {
				continue
			}
			for _, userId := range userIds {
				if userId <= 0 {
					continue
				}
				bindings = append(bindings, ChannelUserBinding{
					ChannelId: channelId,
					UserId:    userId,
				})
			}
		}
		if len(bindings) == 0 {
			return nil
		}

		const chunkSize = 200
		for start := 0; start < len(bindings); start += chunkSize {
			end := start + chunkSize
			if end > len(bindings) {
				end = len(bindings)
			}
			chunk := bindings[start:end]
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&chunk).Error; err != nil {
				return err
			}
		}

		return nil
	})
}

func GetChannelBoundUserCounts(channelIds []int) (map[int]int, error) {
	ids := normalizeUniqueSortedIDs(channelIds)
	if len(ids) == 0 {
		return map[int]int{}, nil
	}

	if common.MemoryCacheEnabled {
		if err := ensureChannelBindingCacheReady(); err == nil {
			counts := make(map[int]int, len(ids))
			channelBindingLock.RLock()
			for _, channelId := range ids {
				counts[channelId] = len(channelBoundUsersCache[channelId])
			}
			channelBindingLock.RUnlock()
			return counts, nil
		}
	}

	type row struct {
		ChannelId int   `gorm:"column:channel_id"`
		Count     int64 `gorm:"column:cnt"`
	}
	var rows []row
	if err := DB.Model(&ChannelUserBinding{}).
		Select("channel_id, COUNT(*) as cnt").
		Where("channel_id IN ?", ids).
		Group("channel_id").
		Find(&rows).Error; err != nil {
		return nil, err
	}

	counts := make(map[int]int, len(rows))
	for _, r := range rows {
		if r.ChannelId <= 0 || r.Count <= 0 {
			continue
		}
		counts[r.ChannelId] = int(r.Count)
	}
	return counts, nil
}
