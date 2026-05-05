package model

import (
	"errors"
	"fmt"
	"one-api/common"
	"one-api/setting/ratio_setting"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Ability struct {
	GroupId   int     `json:"group_id" gorm:"primaryKey;autoIncrement:false;column:group_id;index:idx_abilities_v2_group_model,priority:1"`
	Model     string  `json:"model" gorm:"type:varchar(255);primaryKey;autoIncrement:false"`
	ChannelId int     `json:"channel_id" gorm:"primaryKey;autoIncrement:false;index"`
	Enabled   bool    `json:"enabled"`
	Priority  *int64  `json:"priority" gorm:"bigint;default:0;index"`
	Weight    uint    `json:"weight" gorm:"default:0;index"`
	Tag       *string `json:"tag" gorm:"index"`
}

func (Ability) TableName() string {
	return "abilities_v2"
}

type AbilityWithChannel struct {
	Ability
	ChannelType int `json:"channel_type"`
}

func GetAllEnableAbilityWithChannels() ([]AbilityWithChannel, error) {
	var abilities []AbilityWithChannel
	err := DB.Table("abilities_v2 a").
		Select("a.*, channels.type as channel_type").
		Joins("left join channels on a.channel_id = channels.id").
		Where("a.enabled = ?", true).
		Scan(&abilities).Error
	return abilities, err
}

func GetGroupEnabledModels(groupID int) []string {
	var models []string
	// Find distinct models
	DB.Table("abilities_v2").Where("group_id = ? and enabled = ?", groupID, true).Distinct("model").Pluck("model", &models)
	return models
}

func GetEnabledModels() []string {
	var models []string
	// Find distinct models
	DB.Table("abilities_v2").Where("enabled = ?", true).Distinct("model").Pluck("model", &models)
	return models
}

// GetModelEnabledGroupIDs returns distinct enabled group_ids for a given model.
// It is used to explain "no available channel" cases: whether it's truly missing channels,
// or the user/token is simply not eligible to consume the model's groups.
func GetModelEnabledGroupIDs(modelName string) ([]int, error) {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return nil, errors.New("model 不能为空")
	}

	var groupIDs []int
	if err := DB.Model(&Ability{}).
		Where("model = ? AND enabled = ?", modelName, true).
		Distinct("group_id").
		Pluck("group_id", &groupIDs).Error; err != nil {
		return nil, err
	}

	if len(groupIDs) == 0 {
		matchName := ratio_setting.FormatMatchingModelName(modelName)
		if matchName != "" && matchName != modelName {
			if err := DB.Model(&Ability{}).
				Where("model = ? AND enabled = ?", matchName, true).
				Distinct("group_id").
				Pluck("group_id", &groupIDs).Error; err != nil {
				return nil, err
			}
		}
	}

	groupIDs = normalizeUniqueSortedIDs(groupIDs)
	if len(groupIDs) == 0 {
		return groupIDs, nil
	}

	// Apply group-level model allowlist (if configured) so that callers can distinguish
	// "truly no enabled groups for this model" from "groups exist but user/token is not eligible".
	filtered := make([]int, 0, len(groupIDs))
	for _, gid := range groupIDs {
		if gid <= 0 {
			continue
		}
		if !groupAllowsModel(gid, modelName) {
			continue
		}
		filtered = append(filtered, gid)
	}
	return filtered, nil
}

func GetAllEnableAbilities() []Ability {
	var abilities []Ability
	DB.Find(&abilities, "enabled = ?", true)
	return abilities
}

func getPriority(groupID int, model string, retry int) (int, error) {

	var priorities []int
	err := DB.Model(&Ability{}).
		Select("DISTINCT(priority)").
		Where("group_id = ? and model = ? and enabled = ?", groupID, model, true).
		Order("priority DESC").              // 按优先级降序排序
		Pluck("priority", &priorities).Error // Pluck用于将查询的结果直接扫描到一个切片中

	if err != nil {
		// 处理错误
		return 0, err
	}

	if len(priorities) == 0 {
		// 如果没有查询到优先级，则返回错误
		return 0, errors.New("数据库一致性被破坏")
	}

	// 确定要使用的优先级
	var priorityToUse int
	if retry >= len(priorities) {
		// 如果重试次数大于优先级数，则使用最小的优先级
		priorityToUse = priorities[len(priorities)-1]
	} else {
		priorityToUse = priorities[retry]
	}
	return priorityToUse, nil
}

func getChannelQuery(groupID int, model string, retry int) (*gorm.DB, error) {
	maxPrioritySubQuery := DB.Model(&Ability{}).Select("MAX(priority)").Where("group_id = ? and model = ? and enabled = ?", groupID, model, true)
	channelQuery := DB.Where("group_id = ? and model = ? and enabled = ? and priority = (?)", groupID, model, true, maxPrioritySubQuery)
	if retry != 0 {
		priority, err := getPriority(groupID, model, retry)
		if err != nil {
			return nil, err
		} else {
			channelQuery = DB.Where("group_id = ? and model = ? and enabled = ? and priority = ?", groupID, model, true, priority)
		}
	}

	return channelQuery, nil
}

func GetRandomSatisfiedChannel(groupID int, model string, retry int) (*Channel, error) {
	var abilities []Ability

	channelQuery := DB.Where("group_id = ? and model = ? and enabled = ?", groupID, model, true)
	if common.UsingSQLite || common.UsingPostgreSQL {
		if err := channelQuery.Order("weight DESC").Find(&abilities).Error; err != nil {
			return nil, err
		}
	} else {
		if err := channelQuery.Order("weight DESC").Find(&abilities).Error; err != nil {
			return nil, err
		}
	}
	if len(abilities) == 0 {
		return nil, nil
	}

	channelIDs := make([]int, 0, len(abilities))
	for _, ability := range abilities {
		channelIDs = append(channelIDs, ability.ChannelId)
	}
	if len(channelIDs) == 0 {
		return nil, nil
	}

	var channels []*Channel
	if err := DB.Where("id IN ?", channelIDs).Find(&channels).Error; err != nil {
		return nil, err
	}
	channelByID := make(map[int]*Channel, len(channels))
	for _, ch := range channels {
		channelByID[ch.Id] = ch
	}

	candidateChannels := make([]*Channel, 0, len(channelIDs))
	for _, id := range channelIDs {
		if ch, ok := channelByID[id]; ok {
			candidateChannels = append(candidateChannels, ch)
		}
	}
	return selectCandidateChannel(nil, groupID, candidateChannels, retry)
}

func GetRandomSatisfiedChannelWithContext(c *gin.Context, groupID int, model string, retry int) (*Channel, error) {
	return getRandomSatisfiedChannelWithContextFiltered(c, groupID, model, retry, nil)
}

func getRandomSatisfiedChannelWithContextFiltered(c *gin.Context, groupID int, model string, retry int, channelFilter func(*Channel) bool) (*Channel, error) {
	var abilities []Ability

	// Load all candidate abilities (do NOT filter by static priority here), then
	// select in Go using channel.GetPriority() which may apply time-based overrides
	// from channel setting and channel-level max concurrency.
	channelQuery := DB.Where("group_id = ? and model = ? and enabled = ?", groupID, model, true)

	channelQuery, err := applyAbilityUserAccessFilter(c, channelQuery)
	if err != nil {
		return nil, err
	}

	err = channelQuery.Order("weight DESC").Find(&abilities).Error
	if err != nil {
		return nil, err
	}
	if len(abilities) == 0 {
		return nil, nil
	}

	channelIDs := make([]int, 0, len(abilities))
	excluded := getExcludedChannelIDSet(c)
	for _, ability := range abilities {
		if _, skip := excluded[ability.ChannelId]; skip {
			continue
		}
		channelIDs = append(channelIDs, ability.ChannelId)
	}
	if len(channelIDs) == 0 {
		return nil, nil
	}

	var channels []*Channel
	if err := DB.Where("id IN ?", channelIDs).Find(&channels).Error; err != nil {
		return nil, err
	}
	channelByID := make(map[int]*Channel, len(channels))
	for _, ch := range channels {
		channelByID[ch.Id] = ch
	}

	candidateChannels := make([]*Channel, 0, len(channelIDs))
	for _, id := range channelIDs {
		if ch, ok := channelByID[id]; ok {
			if channelFilter != nil && !channelFilter(ch) {
				continue
			}
			candidateChannels = append(candidateChannels, ch)
		}
	}
	if len(candidateChannels) == 0 {
		return nil, nil
	}
	return selectCandidateChannel(c, groupID, candidateChannels, retry)
}

func (channel *Channel) AddAbilities(tx *gorm.DB) error {
	models_ := strings.Split(channel.Models, ",")
	groupIDs, err := getChannelGroupIDsTx(tx, channel.Id)
	if err != nil {
		return err
	}
	if len(groupIDs) == 0 {
		return errors.New("channel 分组为空")
	}
	abilitySet := make(map[string]struct{})
	abilities := make([]Ability, 0, len(models_))
	for _, model := range models_ {
		m := strings.TrimSpace(model)
		if m == "" {
			continue
		}
		for _, groupID := range groupIDs {
			key := fmt.Sprintf("%d|%s", groupID, m)
			if _, exists := abilitySet[key]; exists {
				continue
			}
			abilitySet[key] = struct{}{}
			ability := Ability{
				GroupId:   groupID,
				Model:     m,
				ChannelId: channel.Id,
				Enabled:   channel.Status == common.ChannelStatusEnabled,
				Priority:  channel.Priority,
				Weight:    uint(channel.GetWeight()),
				Tag:       channel.Tag,
			}
			abilities = append(abilities, ability)
		}
	}
	if len(abilities) == 0 {
		return nil
	}
	// choose DB or provided tx
	useDB := DB
	if tx != nil {
		useDB = tx
	}
	for _, chunk := range lo.Chunk(abilities, 50) {
		err := useDB.Clauses(clause.OnConflict{DoNothing: true}).Create(&chunk).Error
		if err != nil {
			return err
		}
	}
	return nil
}

func (channel *Channel) DeleteAbilities() error {
	return DB.Where("channel_id = ?", channel.Id).Delete(&Ability{}).Error
}

// UpdateAbilities updates abilities of this channel.
// Make sure the channel is completed before calling this function.
func (channel *Channel) UpdateAbilities(tx *gorm.DB) error {
	isNewTx := false
	// 如果没有传入事务，创建新的事务
	if tx == nil {
		tx = DB.Begin()
		if tx.Error != nil {
			return tx.Error
		}
		isNewTx = true
		defer func() {
			if r := recover(); r != nil {
				tx.Rollback()
			}
		}()
	}

	// First delete all abilities of this channel
	err := tx.Where("channel_id = ?", channel.Id).Delete(&Ability{}).Error
	if err != nil {
		if isNewTx {
			tx.Rollback()
		}
		return err
	}

	// Then add new abilities
	models_ := strings.Split(channel.Models, ",")
	groupIDs, err := getChannelGroupIDsTx(tx, channel.Id)
	if err != nil {
		if isNewTx {
			tx.Rollback()
		}
		return err
	}
	if len(groupIDs) == 0 {
		if isNewTx {
			tx.Rollback()
		}
		return errors.New("channel 分组为空")
	}
	abilitySet := make(map[string]struct{})
	abilities := make([]Ability, 0, len(models_))
	for _, model := range models_ {
		m := strings.TrimSpace(model)
		if m == "" {
			continue
		}
		for _, groupID := range groupIDs {
			key := fmt.Sprintf("%d|%s", groupID, m)
			if _, exists := abilitySet[key]; exists {
				continue
			}
			abilitySet[key] = struct{}{}
			ability := Ability{
				GroupId:   groupID,
				Model:     m,
				ChannelId: channel.Id,
				Enabled:   channel.Status == common.ChannelStatusEnabled,
				Priority:  channel.Priority,
				Weight:    uint(channel.GetWeight()),
				Tag:       channel.Tag,
			}
			abilities = append(abilities, ability)
		}
	}

	if len(abilities) > 0 {
		for _, chunk := range lo.Chunk(abilities, 50) {
			err = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&chunk).Error
			if err != nil {
				if isNewTx {
					tx.Rollback()
				}
				return err
			}
		}
	}

	// 如果是新创建的事务，需要提交
	if isNewTx {
		return tx.Commit().Error
	}

	return nil
}

func UpdateAbilityStatus(channelId int, status bool) error {
	return DB.Model(&Ability{}).Where("channel_id = ?", channelId).Select("enabled").Update("enabled", status).Error
}

func UpdateAbilityStatusByTag(tag string, status bool) error {
	return DB.Model(&Ability{}).Where("tag = ?", tag).Select("enabled").Update("enabled", status).Error
}

func UpdateAbilityByTag(tag string, newTag *string, priority *int64, weight *uint) error {
	ability := Ability{}
	if newTag != nil {
		ability.Tag = newTag
	}
	if priority != nil {
		ability.Priority = priority
	}
	if weight != nil {
		ability.Weight = *weight
	}
	return DB.Model(&Ability{}).Where("tag = ?", tag).Updates(ability).Error
}

var fixLock = sync.Mutex{}

func FixAbility() (int, int, error) {
	lock := fixLock.TryLock()
	if !lock {
		return 0, 0, errors.New("已经有一个修复任务在运行中，请稍后再试")
	}
	defer fixLock.Unlock()

	// truncate abilities table
	if common.UsingSQLite {
		err := DB.Exec("DELETE FROM abilities_v2").Error
		if err != nil {
			common.SysLog(fmt.Sprintf("Delete abilities failed: %s", err.Error()))
			return 0, 0, err
		}
	} else {
		err := DB.Exec("TRUNCATE TABLE abilities_v2").Error
		if err != nil {
			common.SysLog(fmt.Sprintf("Truncate abilities failed: %s", err.Error()))
			return 0, 0, err
		}
	}
	var channels []*Channel
	// Find all channels
	err := DB.Model(&Channel{}).Find(&channels).Error
	if err != nil {
		return 0, 0, err
	}
	if len(channels) == 0 {
		return 0, 0, nil
	}
	successCount := 0
	failCount := 0
	for _, chunk := range lo.Chunk(channels, 50) {
		ids := lo.Map(chunk, func(c *Channel, _ int) int { return c.Id })
		// Delete all abilities of this channel
		err = DB.Where("channel_id IN ?", ids).Delete(&Ability{}).Error
		if err != nil {
			common.SysLog(fmt.Sprintf("Delete abilities failed: %s", err.Error()))
			failCount += len(chunk)
			continue
		}
		// Then add new abilities
		for _, channel := range chunk {
			err = channel.AddAbilities(nil)
			if err != nil {
				common.SysLog(fmt.Sprintf("Add abilities for channel %d failed: %s", channel.Id, err.Error()))
				failCount++
			} else {
				successCount++
			}
		}
	}
	InitChannelCache()
	return successCount, failCount, nil
}
