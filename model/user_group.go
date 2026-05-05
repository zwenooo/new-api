package model

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"
)

type UserGroup struct {
	Id                 int       `json:"id" gorm:"primaryKey;autoIncrement"`
	Code               string    `json:"code" gorm:"type:varchar(64);not null;uniqueIndex:idx_user_groups_code"`
	Name               string    `json:"name" gorm:"type:varchar(64);not null"`
	Description        string    `json:"description" gorm:"type:text"`
	SortOrder          int       `json:"sort_order" gorm:"type:int;default:0;column:sort_order"`
	Enabled            bool      `json:"enabled" gorm:"type:boolean;not null;default:true;index"`
	SourceModelGroupId int       `json:"source_model_group_id" gorm:"type:int;default:0;index;column:source_model_group_id"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

func (UserGroup) TableName() string {
	return "user_groups"
}

const defaultUserGroupCode = "default"
const defaultUserGroupName = "默认用户分组"

func normalizeUserGroupCode(code string) (string, error) {
	value := strings.TrimSpace(code)
	if value == "" {
		return "", errors.New("用户分组编码不能为空")
	}
	if utf8.RuneCountInString(value) > 64 {
		return "", errors.New("用户分组编码过长")
	}
	return value, nil
}

func normalizeUserGroupName(name string) (string, error) {
	value := strings.TrimSpace(name)
	if value == "" {
		return "", errors.New("用户分组名称不能为空")
	}
	if utf8.RuneCountInString(value) > 64 {
		return "", errors.New("用户分组名称过长")
	}
	return value, nil
}

func normalizeUserGroupDescription(description string) (string, error) {
	value := strings.TrimSpace(description)
	if utf8.RuneCountInString(value) > 2048 {
		return "", errors.New("用户分组说明过长")
	}
	return value, nil
}

func ListUserGroups(tx *gorm.DB) ([]UserGroup, error) {
	if tx == nil {
		tx = DB
	}
	if tx == nil || !tx.Migrator().HasTable(&UserGroup{}) {
		return []UserGroup{}, nil
	}
	var groups []UserGroup
	if err := tx.Order("sort_order DESC, id ASC").Find(&groups).Error; err != nil {
		return nil, err
	}
	return groups, nil
}

func GetUserGroupByID(tx *gorm.DB, id int) (*UserGroup, error) {
	if tx == nil {
		tx = DB
	}
	if id <= 0 {
		return nil, errors.New("用户分组 id 无效")
	}
	var group UserGroup
	if err := tx.Where("id = ?", id).First(&group).Error; err != nil {
		return nil, err
	}
	return &group, nil
}

func UserGroupIDNameMap(tx *gorm.DB, ids []int) (map[int]string, error) {
	if tx == nil {
		tx = DB
	}
	if tx == nil || !tx.Migrator().HasTable(&UserGroup{}) {
		return map[int]string{}, nil
	}
	normalized := normalizeUniqueSortedIDs(ids)
	if len(normalized) == 0 {
		return map[int]string{}, nil
	}
	type row struct {
		Id   int    `gorm:"column:id"`
		Name string `gorm:"column:name"`
		Code string `gorm:"column:code"`
	}
	var rows []row
	if err := tx.Model(&UserGroup{}).Select("id", "name", "code").Where("id IN ?", normalized).Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make(map[int]string, len(rows))
	for _, row := range rows {
		if row.Id <= 0 {
			continue
		}
		label := strings.TrimSpace(row.Name)
		if label == "" {
			label = strings.TrimSpace(row.Code)
		}
		if label == "" {
			continue
		}
		result[row.Id] = label
	}
	return result, nil
}

func CreateUserGroup(tx *gorm.DB, group *UserGroup) error {
	if group == nil {
		return errors.New("用户分组为空")
	}
	if tx == nil {
		tx = DB
	}
	code, err := normalizeUserGroupCode(group.Code)
	if err != nil {
		return err
	}
	name, err := normalizeUserGroupName(group.Name)
	if err != nil {
		return err
	}
	description, err := normalizeUserGroupDescription(group.Description)
	if err != nil {
		return err
	}
	group.Code = code
	group.Name = name
	group.Description = description
	if group.SortOrder < 0 {
		group.SortOrder = 0
	}
	return tx.Create(group).Error
}

type UpdateUserGroupParams struct {
	Code        *string
	Name        *string
	Description *string
	SortOrder   *int
	Enabled     *bool
}

func UpdateUserGroupByID(tx *gorm.DB, id int, params UpdateUserGroupParams) (*UserGroup, error) {
	if tx == nil {
		tx = DB
	}
	if id <= 0 {
		return nil, errors.New("用户分组 id 无效")
	}
	updates := map[string]interface{}{}
	if params.Code != nil {
		value, err := normalizeUserGroupCode(*params.Code)
		if err != nil {
			return nil, err
		}
		updates["code"] = value
	}
	if params.Name != nil {
		value, err := normalizeUserGroupName(*params.Name)
		if err != nil {
			return nil, err
		}
		updates["name"] = value
	}
	if params.Description != nil {
		value, err := normalizeUserGroupDescription(*params.Description)
		if err != nil {
			return nil, err
		}
		updates["description"] = value
	}
	if params.SortOrder != nil {
		if *params.SortOrder < 0 {
			return nil, errors.New("排序值不能小于 0")
		}
		updates["sort_order"] = *params.SortOrder
	}
	if params.Enabled != nil {
		updates["enabled"] = *params.Enabled
	}
	if len(updates) == 0 {
		return GetUserGroupByID(tx, id)
	}
	if err := tx.Model(&UserGroup{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return nil, err
	}
	return GetUserGroupByID(tx, id)
}

func DeleteUserGroupByID(tx *gorm.DB, id int) error {
	if tx == nil {
		tx = DB
	}
	if id <= 0 {
		return errors.New("用户分组 id 无效")
	}
	return tx.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&User{}).Where("user_group_id = ?", id).Update("user_group_id", 0).Error; err != nil {
			return err
		}
		return tx.Delete(&UserGroup{}, "id = ?", id).Error
	})
}

func nextAvailableUserGroupCodeTx(tx *gorm.DB, preferred string) (string, error) {
	code, err := normalizeUserGroupCode(preferred)
	if err != nil {
		return "", err
	}
	candidate := code
	suffix := 2
	for {
		var count int64
		if err := tx.Model(&UserGroup{}).Where("code = ?", candidate).Count(&count).Error; err != nil {
			return "", err
		}
		if count == 0 {
			return candidate, nil
		}
		candidate = fmt.Sprintf("%s-%d", code, suffix)
		suffix++
	}
}

func ensureDefaultUserGroupTx(tx *gorm.DB) (int, error) {
	if tx == nil {
		tx = DB
	}
	if tx == nil {
		return 0, errors.New("nil db")
	}
	if !tx.Migrator().HasTable(&UserGroup{}) {
		return 0, nil
	}

	var group UserGroup
	if err := tx.Where("source_model_group_id = ?", 0).
		Where("code = ?", defaultUserGroupCode).
		First(&group).Error; err == nil {
		return group.Id, nil
	} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, err
	}

	if err := tx.Where("code = ?", defaultUserGroupCode).First(&group).Error; err == nil {
		if group.SourceModelGroupId == 0 {
			return group.Id, nil
		}
	} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, err
	}

	code, err := nextAvailableUserGroupCodeTx(tx, defaultUserGroupCode)
	if err != nil {
		return 0, err
	}
	if code == defaultUserGroupCode {
		group = UserGroup{
			Code:               defaultUserGroupCode,
			Name:               defaultUserGroupName,
			Description:        "",
			Enabled:            true,
			SourceModelGroupId: 0,
		}
		if err := CreateUserGroup(tx, &group); err != nil {
			return 0, err
		}
		return group.Id, nil
	}

	group = UserGroup{
		Code:               code,
		Name:               defaultUserGroupName,
		Description:        "",
		Enabled:            true,
		SourceModelGroupId: 0,
	}
	if err := CreateUserGroup(tx, &group); err != nil {
		return 0, err
	}
	return group.Id, nil
}

func ensureUserGroupForModelGroupTx(tx *gorm.DB, modelGroupID int) (int, error) {
	if tx == nil {
		tx = DB
	}
	if modelGroupID <= 0 {
		return 0, nil
	}
	if !tx.Migrator().HasTable(&UserGroup{}) {
		return 0, nil
	}

	var existing UserGroup
	if err := tx.Where("source_model_group_id = ?", modelGroupID).First(&existing).Error; err == nil {
		return existing.Id, nil
	} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, err
	}

	modelGroup, err := GetGroupByID(tx, modelGroupID)
	if err != nil {
		return 0, err
	}
	preferredCode := strings.TrimSpace(modelGroup.Code)
	if preferredCode == "" || strings.HasPrefix(preferredCode, "__archived_group_") {
		preferredCode = fmt.Sprintf("legacy-model-group-%d", modelGroupID)
	}
	code, err := nextAvailableUserGroupCodeTx(tx, preferredCode)
	if err != nil {
		return 0, err
	}
	name := strings.TrimSpace(modelGroup.DisplayName)
	if name == "" || strings.HasPrefix(name, "__archived_group_") {
		name = strings.TrimSpace(modelGroup.Code)
	}
	if name == "" || strings.HasPrefix(name, "__archived_group_") {
		name = fmt.Sprintf("历史用户分组 %d", modelGroupID)
	}
	group := &UserGroup{
		Code:               code,
		Name:               name,
		Description:        "",
		Enabled:            true,
		SourceModelGroupId: modelGroupID,
	}
	if err := CreateUserGroup(tx, group); err != nil {
		return 0, err
	}
	return group.Id, nil
}

func ResolveUserGroupIDForUserTx(tx *gorm.DB, requestedUserGroupID int, modelGroupID int) (int, error) {
	if tx == nil {
		tx = DB
	}
	if requestedUserGroupID > 0 {
		if _, err := GetUserGroupByID(tx, requestedUserGroupID); err != nil {
			return 0, err
		}
		return requestedUserGroupID, nil
	}
	if modelGroupID <= 0 {
		return ensureDefaultUserGroupTx(tx)
	}
	return ensureUserGroupForModelGroupTx(tx, modelGroupID)
}

func BackfillUserGroupsFromModelGroupsTx(tx *gorm.DB) error {
	if tx == nil {
		tx = DB
	}
	if tx == nil {
		return errors.New("nil db")
	}
	if !tx.Migrator().HasTable(&UserGroup{}) || !tx.Migrator().HasColumn(&User{}, "user_group_id") {
		return nil
	}

	var existingUserGroupCount int64
	if err := tx.Model(&UserGroup{}).Count(&existingUserGroupCount).Error; err != nil {
		return err
	}
	var assignedUserCount int64
	if err := tx.Model(&User{}).Where("user_group_id > 0").Count(&assignedUserCount).Error; err != nil {
		return err
	}
	if existingUserGroupCount > 0 && assignedUserCount > 0 {
		return nil
	}

	type row struct {
		Id          int `gorm:"column:id"`
		GroupId     int `gorm:"column:group_id"`
		UserGroupId int `gorm:"column:user_group_id"`
	}
	var rows []row
	if err := tx.Model(&User{}).Select("id", "group_id", "user_group_id").Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		if row.Id <= 0 || row.UserGroupId > 0 {
			continue
		}
		userGroupID, err := ResolveUserGroupIDForUserTx(tx, 0, row.GroupId)
		if err != nil {
			return err
		}
		if userGroupID <= 0 {
			continue
		}
		if err := tx.Model(&User{}).Where("id = ?", row.Id).Update("user_group_id", userGroupID).Error; err != nil {
			return err
		}
	}
	return nil
}
