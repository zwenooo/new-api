package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ChannelGroup struct {
	ChannelId int `json:"channel_id" gorm:"primaryKey;autoIncrement:false;column:channel_id"`
	GroupId   int `json:"group_id" gorm:"primaryKey;autoIncrement:false;column:group_id;index:idx_channel_groups_group"`
}

func (ChannelGroup) TableName() string {
	return "channel_groups"
}

type ChannelBackupGroup struct {
	ChannelId int `json:"channel_id" gorm:"primaryKey;autoIncrement:false;column:channel_id"`
	GroupId   int `json:"group_id" gorm:"primaryKey;autoIncrement:false;column:group_id;index:idx_channel_backup_groups_group"`
}

func (ChannelBackupGroup) TableName() string {
	return "channel_backup_groups"
}

type TokenAllowedGroup struct {
	TokenId   int `json:"token_id" gorm:"primaryKey;autoIncrement:false;column:token_id;index:idx_token_allowed_groups_order,priority:1"`
	GroupId   int `json:"group_id" gorm:"primaryKey;autoIncrement:false;column:group_id;index:idx_token_allowed_groups_group"`
	SortOrder int `json:"sort_order" gorm:"type:int;not null;default:0;column:sort_order;index:idx_token_allowed_groups_order,priority:2"`
}

func (TokenAllowedGroup) TableName() string {
	return "token_allowed_groups"
}

type SubscriptionProductGroup struct {
	ProductId int `json:"product_id" gorm:"primaryKey;autoIncrement:false;column:product_id"`
	GroupId   int `json:"group_id" gorm:"primaryKey;autoIncrement:false;column:group_id;index:idx_subscription_product_groups_group"`
}

func (SubscriptionProductGroup) TableName() string {
	return "subscription_product_groups"
}

type PaygProduct struct {
	Id          int    `json:"id" gorm:"primaryKey;autoIncrement:false"`
	Name        string `json:"name" gorm:"type:varchar(64);not null"`
	Description string `json:"description" gorm:"type:text;column:description"`
	Enabled     bool   `json:"enabled" gorm:"type:boolean;not null;default:true;column:enabled"`
	Archived    bool   `json:"archived" gorm:"type:boolean;not null;default:false;column:archived"`
	SortOrder   int    `json:"sort_order" gorm:"type:int;not null;default:0;column:sort_order"`
	// Stock is the remaining inventory for this product.
	// nil means unlimited; 0 means sold out.
	Stock *int `json:"stock" gorm:"type:int;column:stock"`
}

func (PaygProduct) TableName() string {
	return "payg_products"
}

type PaygProductGroup struct {
	ProductId int `json:"product_id" gorm:"primaryKey;autoIncrement:false;column:product_id"`
	GroupId   int `json:"group_id" gorm:"primaryKey;autoIncrement:false;column:group_id;index:idx_payg_product_groups_group"`
}

func (PaygProductGroup) TableName() string {
	return "payg_product_groups"
}

type UserSubscriptionGroup struct {
	SubscriptionId int `json:"subscription_id" gorm:"primaryKey;autoIncrement:false;column:subscription_id"`
	GroupId        int `json:"group_id" gorm:"primaryKey;autoIncrement:false;column:group_id;index:idx_user_subscription_groups_group"`
}

func (UserSubscriptionGroup) TableName() string {
	return "user_subscription_groups"
}

type UserRequestSubscriptionGroup struct {
	SubscriptionId int `json:"subscription_id" gorm:"primaryKey;autoIncrement:false;column:subscription_id"`
	GroupId        int `json:"group_id" gorm:"primaryKey;autoIncrement:false;column:group_id;index:idx_user_request_subscription_groups_group"`
}

func (UserRequestSubscriptionGroup) TableName() string {
	return "user_request_subscription_groups"
}

func normalizeUniqueSortedIDs(ids []int) []int {
	if len(ids) == 0 {
		return nil
	}
	seen := make(map[int]struct{}, len(ids))
	out := make([]int, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	if len(out) == 0 {
		return nil
	}
	sort.Ints(out)
	return out
}

func NormalizeUniqueSortedIDs(ids []int) []int {
	return normalizeUniqueSortedIDs(ids)
}

func upsertChannelGroupsTx(tx *gorm.DB, channelID int, groupIDs []int) error {
	if tx == nil {
		tx = DB
	}
	if tx == nil {
		return errors.New("nil db")
	}
	if channelID <= 0 {
		return errors.New("channel_id 无效")
	}
	ids := normalizeUniqueSortedIDs(groupIDs)
	if len(ids) == 0 {
		return errors.New("至少需要一个分组")
	}
	if err := ValidateGroupIDsExist(tx, ids); err != nil {
		return err
	}
	if err := tx.Where("channel_id = ?", channelID).Delete(&ChannelGroup{}).Error; err != nil {
		return err
	}
	rows := make([]ChannelGroup, 0, len(ids))
	for _, groupID := range ids {
		rows = append(rows, ChannelGroup{ChannelId: channelID, GroupId: groupID})
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error
}

func getChannelGroupIDsTx(tx *gorm.DB, channelID int) ([]int, error) {
	if tx == nil {
		tx = DB
	}
	if channelID <= 0 {
		return nil, errors.New("channel_id 无效")
	}
	var ids []int
	if err := tx.Model(&ChannelGroup{}).Where("channel_id = ?", channelID).Order("group_id ASC").Pluck("group_id", &ids).Error; err != nil {
		return nil, err
	}
	return filterExistingSortedIDsTx(tx, ids)
}

func GetChannelGroupIDsTx(tx *gorm.DB, channelID int) ([]int, error) {
	return getChannelGroupIDsTx(tx, channelID)
}

func GetChannelGroupIDs(channelID int) ([]int, error) {
	return getChannelGroupIDsTx(DB, channelID)
}

func ListChannelIDsByGroupTx(tx *gorm.DB, groupID int) ([]int, error) {
	if tx == nil {
		tx = DB
	}
	if groupID <= 0 {
		return nil, errors.New("分组 id 无效")
	}
	var channelIDs []int
	if err := tx.Model(&ChannelGroup{}).
		Where("group_id = ?", groupID).
		Order("channel_id ASC").
		Pluck("channel_id", &channelIDs).Error; err != nil {
		return nil, err
	}
	return normalizeUniqueSortedIDs(channelIDs), nil
}

type GroupChannelBindingItem struct {
	Id       int    `json:"id"`
	Name     string `json:"name"`
	Status   int    `json:"status"`
	Type     int    `json:"type"`
	Tag      string `json:"tag,omitempty"`
	GroupIds []int  `json:"group_ids"`
	Selected bool   `json:"selected"`
}

func validateChannelIDsExist(tx *gorm.DB, ids []int) error {
	if tx == nil {
		tx = DB
	}
	normalized := normalizeUniqueSortedIDs(ids)
	if len(normalized) == 0 {
		return nil
	}
	var existing []int
	if err := tx.Model(&Channel{}).Where("id IN ?", normalized).Pluck("id", &existing).Error; err != nil {
		return err
	}
	existingSet := make(map[int]struct{}, len(existing))
	for _, id := range existing {
		existingSet[id] = struct{}{}
	}
	missing := make([]string, 0)
	for _, id := range normalized {
		if _, ok := existingSet[id]; ok {
			continue
		}
		missing = append(missing, fmt.Sprintf("%d", id))
	}
	if len(missing) > 0 {
		return fmt.Errorf("以下渠道不存在: %s", strings.Join(missing, ", "))
	}
	return nil
}

func ListGroupChannelBindings(tx *gorm.DB, groupID int) ([]GroupChannelBindingItem, error) {
	if tx == nil {
		tx = DB
	}
	if groupID <= 0 {
		return nil, errors.New("分组 id 无效")
	}
	if _, err := GetGroupByID(tx, groupID); err != nil {
		return nil, err
	}

	type channelRow struct {
		Id     int     `gorm:"column:id"`
		Name   string  `gorm:"column:name"`
		Status int     `gorm:"column:status"`
		Type   int     `gorm:"column:type"`
		Tag    *string `gorm:"column:tag"`
	}
	var channels []channelRow
	if err := tx.Model(&Channel{}).
		Select("id", "name", "status", "type", "tag").
		Order("priority DESC").
		Order("id DESC").
		Find(&channels).Error; err != nil {
		return nil, err
	}
	if len(channels) == 0 {
		return []GroupChannelBindingItem{}, nil
	}

	channelIDs := make([]int, 0, len(channels))
	for _, channel := range channels {
		if channel.Id <= 0 {
			continue
		}
		channelIDs = append(channelIDs, channel.Id)
	}
	channelIDs = normalizeUniqueSortedIDs(channelIDs)
	if len(channelIDs) == 0 {
		return []GroupChannelBindingItem{}, nil
	}

	type bindingRow struct {
		ChannelId int `gorm:"column:channel_id"`
		GroupId   int `gorm:"column:group_id"`
	}
	var rows []bindingRow
	if err := tx.Model(&ChannelGroup{}).
		Select("channel_id", "group_id").
		Where("channel_id IN ?", channelIDs).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	groupIDsByChannel := make(map[int][]int, len(channelIDs))
	for _, row := range rows {
		if row.ChannelId <= 0 || row.GroupId <= 0 {
			continue
		}
		groupIDsByChannel[row.ChannelId] = append(groupIDsByChannel[row.ChannelId], row.GroupId)
	}

	items := make([]GroupChannelBindingItem, 0, len(channels))
	for _, channel := range channels {
		groupIDs := normalizeUniqueSortedIDs(groupIDsByChannel[channel.Id])
		tag := ""
		if channel.Tag != nil {
			tag = strings.TrimSpace(*channel.Tag)
		}
		items = append(items, GroupChannelBindingItem{
			Id:       channel.Id,
			Name:     strings.TrimSpace(channel.Name),
			Status:   channel.Status,
			Type:     channel.Type,
			Tag:      tag,
			GroupIds: groupIDs,
			Selected: slicesContainsInt(groupIDs, groupID),
		})
	}
	return items, nil
}

func SyncGroupChannelBindings(tx *gorm.DB, groupID int, channelIDs []int) (int, error) {
	if tx == nil {
		tx = DB
	}
	if groupID <= 0 {
		return 0, errors.New("分组 id 无效")
	}

	desiredChannelIDs := normalizeUniqueSortedIDs(channelIDs)
	if err := validateChannelIDsExist(tx, desiredChannelIDs); err != nil {
		return 0, err
	}

	affected := 0
	err := tx.Transaction(func(tx *gorm.DB) error {
		if _, err := GetGroupByID(tx, groupID); err != nil {
			return err
		}

		currentChannelIDs, err := ListChannelIDsByGroupTx(tx, groupID)
		if err != nil {
			return err
		}

		touchedSet := make(map[int]struct{}, len(currentChannelIDs)+len(desiredChannelIDs))
		for _, id := range currentChannelIDs {
			if id > 0 {
				touchedSet[id] = struct{}{}
			}
		}
		for _, id := range desiredChannelIDs {
			if id > 0 {
				touchedSet[id] = struct{}{}
			}
		}

		touchedIDs := make([]int, 0, len(touchedSet))
		for id := range touchedSet {
			touchedIDs = append(touchedIDs, id)
		}
		touchedIDs = normalizeUniqueSortedIDs(touchedIDs)
		if len(touchedIDs) == 0 {
			return nil
		}

		var channels []*Channel
		if err := tx.Where("id IN ?", touchedIDs).Find(&channels).Error; err != nil {
			return err
		}
		channelByID := make(map[int]*Channel, len(channels))
		for _, channel := range channels {
			if channel == nil || channel.Id <= 0 {
				continue
			}
			channelByID[channel.Id] = channel
		}

		desiredSet := make(map[int]struct{}, len(desiredChannelIDs))
		for _, id := range desiredChannelIDs {
			desiredSet[id] = struct{}{}
		}

		type syncPlan struct {
			channel        *Channel
			nextGroupIDs   []int
			backupGroupIDs []int
		}
		plans := make([]syncPlan, 0, len(touchedIDs))
		blocked := make([]string, 0)

		for _, channelID := range touchedIDs {
			channel, ok := channelByID[channelID]
			if !ok || channel == nil {
				return fmt.Errorf("渠道 %d 不存在", channelID)
			}

			currentGroupIDs, err := getChannelGroupIDsTx(tx, channel.Id)
			if err != nil {
				return err
			}
			nextGroupIDs := append([]int(nil), currentGroupIDs...)
			_, wantsBinding := desiredSet[channel.Id]
			hasBinding := slicesContainsInt(currentGroupIDs, groupID)

			if wantsBinding && !hasBinding {
				nextGroupIDs = normalizeUniqueSortedIDs(append(nextGroupIDs, groupID))
			}
			if !wantsBinding && hasBinding {
				nextGroupIDs = removeIntID(nextGroupIDs, groupID)
				if len(nextGroupIDs) == 0 {
					label := strings.TrimSpace(channel.Name)
					if label == "" {
						label = fmt.Sprintf("#%d", channel.Id)
					} else {
						label = fmt.Sprintf("%s (#%d)", label, channel.Id)
					}
					blocked = append(blocked, label)
					continue
				}
			}

			if equalSortedIDs(currentGroupIDs, nextGroupIDs) {
				continue
			}

			backupGroupIDs, err := getChannelBackupGroupIDsTx(tx, channel.Id)
			if err != nil {
				return err
			}
			plans = append(plans, syncPlan{
				channel:        channel,
				nextGroupIDs:   nextGroupIDs,
				backupGroupIDs: filterChannelBackupGroupIDs(nextGroupIDs, backupGroupIDs),
			})
		}

		if len(blocked) > 0 {
			return fmt.Errorf("以下渠道移除当前分组后将失去所有主分组，请先为它们绑定其他分组：%s", strings.Join(blocked, ", "))
		}

		for _, plan := range plans {
			if err := upsertChannelGroupsTx(tx, plan.channel.Id, plan.nextGroupIDs); err != nil {
				return err
			}
			if err := upsertChannelBackupGroupsTx(tx, plan.channel.Id, plan.backupGroupIDs); err != nil {
				return err
			}
			if err := plan.channel.UpdateAbilities(tx); err != nil {
				return err
			}
		}
		affected = len(plans)
		return nil
	})
	if err != nil {
		return 0, err
	}
	return affected, nil
}

func slicesContainsInt(ids []int, target int) bool {
	for _, id := range ids {
		if id == target {
			return true
		}
	}
	return false
}

func removeIntID(ids []int, target int) []int {
	if len(ids) == 0 {
		return nil
	}
	filtered := make([]int, 0, len(ids))
	for _, id := range ids {
		if id == target {
			continue
		}
		filtered = append(filtered, id)
	}
	return normalizeUniqueSortedIDs(filtered)
}

func equalSortedIDs(left []int, right []int) bool {
	left = normalizeUniqueSortedIDs(left)
	right = normalizeUniqueSortedIDs(right)
	if len(left) != len(right) {
		return false
	}
	for idx := range left {
		if left[idx] != right[idx] {
			return false
		}
	}
	return true
}

func upsertChannelBackupGroupsTx(tx *gorm.DB, channelID int, groupIDs []int) error {
	if tx == nil {
		tx = DB
	}
	if tx == nil {
		return errors.New("nil db")
	}
	if channelID <= 0 {
		return errors.New("channel_id 无效")
	}
	ids := normalizeUniqueSortedIDs(groupIDs)
	if err := tx.Where("channel_id = ?", channelID).Delete(&ChannelBackupGroup{}).Error; err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	if err := ValidateGroupIDsExist(tx, ids); err != nil {
		return err
	}
	rows := make([]ChannelBackupGroup, 0, len(ids))
	for _, groupID := range ids {
		rows = append(rows, ChannelBackupGroup{ChannelId: channelID, GroupId: groupID})
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error
}

func getChannelBackupGroupIDsTx(tx *gorm.DB, channelID int) ([]int, error) {
	if tx == nil {
		tx = DB
	}
	if channelID <= 0 {
		return nil, errors.New("channel_id 无效")
	}
	var ids []int
	if err := tx.Model(&ChannelBackupGroup{}).Where("channel_id = ?", channelID).Order("group_id ASC").Pluck("group_id", &ids).Error; err != nil {
		return nil, err
	}
	return filterExistingSortedIDsTx(tx, ids)
}

func GetChannelBackupGroupIDsTx(tx *gorm.DB, channelID int) ([]int, error) {
	return getChannelBackupGroupIDsTx(tx, channelID)
}

func GetChannelBackupGroupIDs(channelID int) ([]int, error) {
	return getChannelBackupGroupIDsTx(DB, channelID)
}

func FillChannelsGroupIDsTx(tx *gorm.DB, channels []*Channel) error {
	if len(channels) == 0 {
		return nil
	}
	if tx == nil {
		tx = DB
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
	channelIDs = normalizeUniqueSortedIDs(channelIDs)
	if len(channelIDs) == 0 {
		return nil
	}

	type row struct {
		ChannelId int `gorm:"column:channel_id"`
		GroupId   int `gorm:"column:group_id"`
	}
	var rows []row
	if err := tx.Model(&ChannelGroup{}).
		Select("channel_id", "group_id").
		Where("channel_id IN ?", channelIDs).
		Find(&rows).Error; err != nil {
		return err
	}
	byChannel := make(map[int][]int, len(channelIDs))
	for _, r := range rows {
		if r.ChannelId <= 0 || r.GroupId <= 0 {
			continue
		}
		byChannel[r.ChannelId] = append(byChannel[r.ChannelId], r.GroupId)
	}

	for _, ch := range channels {
		if ch == nil || ch.Id <= 0 {
			continue
		}
		ids := normalizeUniqueSortedIDs(byChannel[ch.Id])
		if len(ids) == 0 {
			ch.GroupIds = []int{}
			continue
		}
		ch.GroupIds = ids
	}
	return nil
}

func FillChannelsGroupIDs(channels []*Channel) error {
	return FillChannelsGroupIDsTx(DB, channels)
}

func FillChannelsBackupGroupIDsTx(tx *gorm.DB, channels []*Channel) error {
	if len(channels) == 0 {
		return nil
	}
	if tx == nil {
		tx = DB
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
	channelIDs = normalizeUniqueSortedIDs(channelIDs)
	if len(channelIDs) == 0 {
		return nil
	}

	type row struct {
		ChannelId int `gorm:"column:channel_id"`
		GroupId   int `gorm:"column:group_id"`
	}
	var rows []row
	if err := tx.Model(&ChannelBackupGroup{}).
		Select("channel_id", "group_id").
		Where("channel_id IN ?", channelIDs).
		Find(&rows).Error; err != nil {
		return err
	}
	byChannel := make(map[int][]int, len(channelIDs))
	for _, r := range rows {
		if r.ChannelId <= 0 || r.GroupId <= 0 {
			continue
		}
		byChannel[r.ChannelId] = append(byChannel[r.ChannelId], r.GroupId)
	}

	for _, ch := range channels {
		if ch == nil || ch.Id <= 0 {
			continue
		}
		ch.BackupGroupIds = normalizeUniqueSortedIDs(byChannel[ch.Id])
	}
	return nil
}

func FillChannelsBackupGroupIDs(channels []*Channel) error {
	return FillChannelsBackupGroupIDsTx(DB, channels)
}

func upsertTokenAllowedGroupsTx(tx *gorm.DB, tokenID int, groupIDs []int) error {
	if tx == nil {
		tx = DB
	}
	if tx == nil {
		return errors.New("nil db")
	}
	if tokenID <= 0 {
		return errors.New("token_id 无效")
	}
	ids := normalizeUniquePositiveIDsKeepOrder(groupIDs)
	if len(ids) == 0 {
		return errors.New("至少需要一个分组")
	}
	if err := ValidateGroupIDsExist(tx, ids); err != nil {
		return err
	}
	if err := tx.Where("token_id = ?", tokenID).Delete(&TokenAllowedGroup{}).Error; err != nil {
		return err
	}
	rows := make([]TokenAllowedGroup, 0, len(ids))
	for idx, groupID := range ids {
		rows = append(rows, TokenAllowedGroup{TokenId: tokenID, GroupId: groupID, SortOrder: idx})
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error
}

func getTokenAllowedGroupIDsTx(tx *gorm.DB, tokenID int) ([]int, error) {
	if tx == nil {
		tx = DB
	}
	if tokenID <= 0 {
		return nil, errors.New("token_id 无效")
	}
	type row struct {
		GroupId   int `gorm:"column:group_id"`
		SortOrder int `gorm:"column:sort_order"`
	}
	var rows []row
	if err := tx.Model(&TokenAllowedGroup{}).
		Select("group_id", "sort_order").
		Where("token_id = ?", tokenID).
		Order("sort_order ASC").
		Order("group_id ASC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	ids := make([]int, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.GroupId)
	}
	return filterExistingActiveGroupIDsKeepOrderTx(tx, ids)
}

func upsertSubscriptionProductGroupsTx(tx *gorm.DB, productID int, groupIDs []int) error {
	if tx == nil {
		tx = DB
	}
	if tx == nil {
		return errors.New("nil db")
	}
	if productID <= 0 {
		return errors.New("product_id 无效")
	}
	ids := normalizeUniqueSortedIDs(groupIDs)
	if len(ids) == 0 {
		return errors.New("至少需要一个分组")
	}
	if err := ValidateGroupIDsExist(tx, ids); err != nil {
		return err
	}
	if err := tx.Where("product_id = ?", productID).Delete(&SubscriptionProductGroup{}).Error; err != nil {
		return err
	}
	rows := make([]SubscriptionProductGroup, 0, len(ids))
	for _, groupID := range ids {
		rows = append(rows, SubscriptionProductGroup{ProductId: productID, GroupId: groupID})
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error
}

func getSubscriptionProductGroupIDsTx(tx *gorm.DB, productID int) ([]int, error) {
	if tx == nil {
		tx = DB
	}
	if productID <= 0 {
		return nil, errors.New("product_id 无效")
	}
	var ids []int
	if err := tx.Model(&SubscriptionProductGroup{}).Where("product_id = ?", productID).Order("group_id ASC").Pluck("group_id", &ids).Error; err != nil {
		return nil, err
	}
	return filterExistingSortedIDsTx(tx, ids)
}

func upsertPaygProductTx(tx *gorm.DB, product PaygProduct, groupIDs []int) error {
	if tx == nil {
		tx = DB
	}
	if tx == nil {
		return errors.New("nil db")
	}
	if product.Id <= 0 {
		return errors.New("product_id 无效")
	}
	if product.Name == "" {
		return errors.New("product_name 不能为空")
	}
	if product.Archived {
		product.Enabled = false
	}
	if err := tx.Select("id", "name", "description", "enabled", "archived", "sort_order", "stock").Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{"name", "description", "enabled", "archived", "sort_order", "stock"}),
	}).Create(&product).Error; err != nil {
		return err
	}
	ids := normalizeUniqueSortedIDs(groupIDs)
	if len(ids) == 0 {
		return errors.New("至少需要一个分组")
	}
	if err := ValidateGroupIDsExist(tx, ids); err != nil {
		return err
	}
	if err := tx.Where("product_id = ?", product.Id).Delete(&PaygProductGroup{}).Error; err != nil {
		return err
	}
	rows := make([]PaygProductGroup, 0, len(ids))
	for _, groupID := range ids {
		rows = append(rows, PaygProductGroup{ProductId: product.Id, GroupId: groupID})
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error
}

func getPaygProductGroupIDsTx(tx *gorm.DB, productID int) ([]int, error) {
	if tx == nil {
		tx = DB
	}
	if productID <= 0 {
		return nil, errors.New("product_id 无效")
	}
	var ids []int
	if err := tx.Model(&PaygProductGroup{}).Where("product_id = ?", productID).Order("group_id ASC").Pluck("group_id", &ids).Error; err != nil {
		return nil, err
	}
	return filterExistingSortedIDsTx(tx, ids)
}

func GetPaygProductAllowedGroupIDsTx(tx *gorm.DB, productID int) ([]int, error) {
	return getPaygProductGroupIDsTx(tx, productID)
}

func upsertUserSubscriptionGroupsTx(tx *gorm.DB, subscriptionID int, groupIDs []int) error {
	if tx == nil {
		tx = DB
	}
	if tx == nil {
		return errors.New("nil db")
	}
	if subscriptionID <= 0 {
		return errors.New("subscription_id 无效")
	}
	ids := normalizeUniqueSortedIDs(groupIDs)
	if len(ids) == 0 {
		return errors.New("至少需要一个分组")
	}
	if err := ValidateGroupIDsExist(tx, ids); err != nil {
		return err
	}
	if err := tx.Where("subscription_id = ?", subscriptionID).Delete(&UserSubscriptionGroup{}).Error; err != nil {
		return err
	}
	rows := make([]UserSubscriptionGroup, 0, len(ids))
	for _, groupID := range ids {
		rows = append(rows, UserSubscriptionGroup{SubscriptionId: subscriptionID, GroupId: groupID})
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error
}

func getUserSubscriptionGroupIDsTx(tx *gorm.DB, subscriptionID int) ([]int, error) {
	if tx == nil {
		tx = DB
	}
	if subscriptionID <= 0 {
		return nil, errors.New("subscription_id 无效")
	}
	var ids []int
	if err := tx.Model(&UserSubscriptionGroup{}).Where("subscription_id = ?", subscriptionID).Order("group_id ASC").Pluck("group_id", &ids).Error; err != nil {
		return nil, err
	}
	return filterExistingSortedIDsTx(tx, ids)
}

func upsertUserRequestSubscriptionGroupsTx(tx *gorm.DB, subscriptionID int, groupIDs []int) error {
	if tx == nil {
		tx = DB
	}
	if tx == nil {
		return errors.New("nil db")
	}
	if subscriptionID <= 0 {
		return errors.New("subscription_id 无效")
	}
	ids := normalizeUniqueSortedIDs(groupIDs)
	if len(ids) == 0 {
		return errors.New("至少需要一个分组")
	}
	if err := ValidateGroupIDsExist(tx, ids); err != nil {
		return err
	}
	if err := tx.Where("subscription_id = ?", subscriptionID).Delete(&UserRequestSubscriptionGroup{}).Error; err != nil {
		return err
	}
	rows := make([]UserRequestSubscriptionGroup, 0, len(ids))
	for _, groupID := range ids {
		rows = append(rows, UserRequestSubscriptionGroup{SubscriptionId: subscriptionID, GroupId: groupID})
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error
}

func getUserRequestSubscriptionGroupIDsTx(tx *gorm.DB, subscriptionID int) ([]int, error) {
	if tx == nil {
		tx = DB
	}
	if subscriptionID <= 0 {
		return nil, errors.New("subscription_id 无效")
	}
	var ids []int
	if err := tx.Model(&UserRequestSubscriptionGroup{}).Where("subscription_id = ?", subscriptionID).Order("group_id ASC").Pluck("group_id", &ids).Error; err != nil {
		return nil, err
	}
	return filterExistingSortedIDsTx(tx, ids)
}

func ValidateGroupCanDelete(tx *gorm.DB, groupID int) error {
	if tx == nil {
		tx = DB
	}
	if groupID <= 0 {
		return errors.New("分组 id 无效")
	}
	group, err := GetGroupByID(tx, groupID)
	if err != nil {
		return err
	}
	groupCode := strings.TrimSpace(group.Code)
	if groupCode == "" {
		return errors.New("分组 code 无效")
	}
	if groupCode == "default" {
		return fmt.Errorf("该分组当前被系统逻辑依赖（code=%s，动态 fallback 默认分组），不可删除；如不想展示可关闭「启用」或「用户可选」", groupCode)
	}

	refs := make([]string, 0, 16)
	checkRef := func(label string, table any, where string, args ...any) error {
		if !tx.Migrator().HasTable(table) {
			return nil
		}
		var cnt int64
		if err := tx.Model(table).Where(where, args...).Count(&cnt).Error; err != nil {
			return err
		}
		if cnt > 0 {
			refs = append(refs, fmt.Sprintf("%s=%d", label, cnt))
		}
		return nil
	}
	if err := checkRef("渠道分组绑定(channel_groups)", &ChannelGroup{}, "group_id = ?", groupID); err != nil {
		return err
	}
	if err := checkRef("渠道备用分组绑定(channel_backup_groups)", &ChannelBackupGroup{}, "group_id = ?", groupID); err != nil {
		return err
	}
	if err := checkRef("令牌可用分组(token_allowed_groups)", &TokenAllowedGroup{}, "group_id = ?", groupID); err != nil {
		return err
	}
	if tx.Migrator().HasColumn(&User{}, "group_id") {
		if err := checkRef("旧默认模型分组(users.group_id)", &User{}, "group_id = ?", groupID); err != nil {
			return err
		}
	}
	if err := checkRef("订阅商品分组(subscription_product_groups)", &SubscriptionProductGroup{}, "group_id = ?", groupID); err != nil {
		return err
	}
	if err := checkRef("商品版本分组(redemption_preset_revision_groups)", &RedemptionPresetRevisionGroup{}, "group_id = ?", groupID); err != nil {
		return err
	}
	if err := checkRef("订阅商品分组日限额(subscription_product_group_daily_limits)", &SubscriptionProductGroupDailyLimit{}, "group_id = ?", groupID); err != nil {
		return err
	}
	if err := checkRef("商品版本分组日限额(redemption_preset_revision_group_daily_limits)", &RedemptionPresetRevisionGroupDailyLimit{}, "group_id = ?", groupID); err != nil {
		return err
	}
	if err := checkRef("兑换码分组日限额(redemption_group_daily_limits)", &RedemptionGroupDailyLimit{}, "group_id = ?", groupID); err != nil {
		return err
	}
	if err := checkRef("按量商品分组(payg_product_groups)", &PaygProductGroup{}, "group_id = ?", groupID); err != nil {
		return err
	}
	if err := checkRef("按次商品分组(pay_request_product_groups)", &PayRequestProductGroup{}, "group_id = ?", groupID); err != nil {
		return err
	}
	if err := checkRef("用户订阅可用分组(user_subscription_groups)", &UserSubscriptionGroup{}, "group_id = ?", groupID); err != nil {
		return err
	}
	if err := checkRef("用户按次订阅可用分组(user_request_subscription_groups)", &UserRequestSubscriptionGroup{}, "group_id = ?", groupID); err != nil {
		return err
	}
	if err := checkRef("用户订阅分组日限额(user_subscription_group_daily_limits)", &UserSubscriptionGroupDailyLimit{}, "group_id = ?", groupID); err != nil {
		return err
	}
	if err := checkRef("用户订阅分组日用量(user_subscription_group_daily_usages)", &UserSubscriptionGroupDailyUsage{}, "group_id = ?", groupID); err != nil {
		return err
	}
	if tx.Migrator().HasTable(&Ability{}) {
		if err := checkRef("模型能力分组(abilities.group_id)", &Ability{}, "group_id = ?", groupID); err != nil {
			return err
		}
	}

	// config/options references: prevent deleting a group still referenced by runtime config.
	checkOptionRef := func(label string, key string, contains func(raw string) (bool, error)) error {
		raw, ok, err := readLegacyOptionValue(tx, key)
		if err != nil || !ok {
			return err
		}
		referenced, err := contains(raw)
		if err != nil {
			return err
		}
		if referenced {
			refs = append(refs, label)
		}
		return nil
	}
	if err := checkOptionRef("系统设置/自动分组(AutoGroups)", "AutoGroups", func(raw string) (bool, error) {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || trimmed == "null" {
			return false, nil
		}
		var ids []int
		if err := json.Unmarshal([]byte(trimmed), &ids); err == nil {
			for _, id := range ids {
				if id == groupID {
					return true, nil
				}
			}
			return false, nil
		}
		// legacy-only: ["codex","vip"]
		var groups []string
		if err := json.Unmarshal([]byte(trimmed), &groups); err != nil {
			return false, err
		}
		for _, g := range groups {
			if strings.TrimSpace(g) == groupCode {
				return true, nil
			}
		}
		return false, nil
	}); err != nil {
		return err
	}
	if err := checkOptionRef("倍率设置/分组倍率(GroupGroupRatio)", "GroupGroupRatio", func(raw string) (bool, error) {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || trimmed == "null" {
			return false, nil
		}
		parsed := map[int]map[int]float64{}
		if err := json.Unmarshal([]byte(trimmed), &parsed); err == nil {
			if _, ok := parsed[groupID]; ok {
				return true, nil
			}
			for _, inner := range parsed {
				if _, ok := inner[groupID]; ok {
					return true, nil
				}
			}
			return false, nil
		}
		// legacy-only: {"codex":{"vip":0.5}}
		legacy := map[string]map[string]float64{}
		if err := json.Unmarshal([]byte(trimmed), &legacy); err != nil {
			return false, err
		}
		for outer, inner := range legacy {
			if strings.TrimSpace(outer) == groupCode {
				return true, nil
			}
			for k := range inner {
				if strings.TrimSpace(k) == groupCode {
					return true, nil
				}
			}
		}
		return false, nil
	}); err != nil {
		return err
	}
	if len(refs) > 0 {
		return fmt.Errorf("分组仍被引用，无法删除：%s", strings.Join(refs, "，"))
	}

	return nil
}
