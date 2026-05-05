package model

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"one-api/common"
	"sort"
	"strings"

	"gorm.io/gorm"
)

// PrefillGroup 用于存储可复用的“组”信息，例如模型组、标签组、端点组等。
// Name 字段保持唯一，用于在前端下拉框中展示。
// Type 字段用于区分组的类别，可选值如：model、tag、endpoint。
// Items 字段使用 JSON 数组保存对应类型的字符串集合，示例：
// ["gpt-4o", "gpt-3.5-turbo"]
// 设计遵循 3NF，避免冗余，提供灵活扩展能力。

// JSONValue 基于 json.RawMessage 实现，支持从数据库的 []byte 和 string 两种类型读取
type JSONValue json.RawMessage

// Value 实现 driver.Valuer 接口，用于数据库写入
func (j JSONValue) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return []byte(j), nil
}

// Scan 实现 sql.Scanner 接口，兼容不同驱动返回的类型
func (j *JSONValue) Scan(value interface{}) error {
	switch v := value.(type) {
	case nil:
		*j = nil
		return nil
	case []byte:
		// 拷贝底层字节，避免保留底层缓冲区
		b := make([]byte, len(v))
		copy(b, v)
		*j = JSONValue(b)
		return nil
	case string:
		*j = JSONValue([]byte(v))
		return nil
	default:
		// 其他类型尝试序列化为 JSON
		b, err := json.Marshal(v)
		if err != nil {
			return err
		}
		*j = JSONValue(b)
		return nil
	}
}

// MarshalJSON 确保在对外编码时与 json.RawMessage 行为一致
func (j JSONValue) MarshalJSON() ([]byte, error) {
	if j == nil {
		return []byte("null"), nil
	}
	return j, nil
}

// UnmarshalJSON 确保在对外解码时与 json.RawMessage 行为一致
func (j *JSONValue) UnmarshalJSON(data []byte) error {
	if data == nil {
		*j = nil
		return nil
	}
	b := make([]byte, len(data))
	copy(b, data)
	*j = JSONValue(b)
	return nil
}

type PrefillGroup struct {
	Id          int            `json:"id"`
	Name        string         `json:"name" gorm:"size:64;not null;uniqueIndex:uk_prefill_name,where:deleted_at IS NULL"`
	Type        string         `json:"type" gorm:"size:32;index;not null"`
	Items       JSONValue      `json:"items" gorm:"type:json"`
	Description string         `json:"description,omitempty" gorm:"type:varchar(255)"`
	CreatedTime int64          `json:"created_time" gorm:"bigint"`
	UpdatedTime int64          `json:"updated_time" gorm:"bigint"`
	DeletedAt   gorm.DeletedAt `json:"-" gorm:"index"`
}

// Insert 新建组
func (g *PrefillGroup) Insert() error {
	now := common.GetTimestamp()
	g.CreatedTime = now
	g.UpdatedTime = now
	return DB.Create(g).Error
}

// IsPrefillGroupNameDuplicated 检查组名称是否重复（排除自身 ID）
func IsPrefillGroupNameDuplicated(id int, name string) (bool, error) {
	if name == "" {
		return false, nil
	}
	var cnt int64
	err := DB.Model(&PrefillGroup{}).Where("name = ? AND id <> ?", name, id).Count(&cnt).Error
	return cnt > 0, err
}

// Update 更新组
func (g *PrefillGroup) Update() error {
	g.UpdatedTime = common.GetTimestamp()
	return DB.Save(g).Error
}

// DeleteByID 根据 ID 删除组
func DeletePrefillGroupByID(id int) error {
	return DB.Delete(&PrefillGroup{}, id).Error
}

// GetAllPrefillGroups 获取全部组，可按类型过滤（为空则返回全部）
func GetAllPrefillGroups(groupType string) ([]*PrefillGroup, error) {
	var groups []*PrefillGroup
	query := DB.Model(&PrefillGroup{})
	if groupType != "" {
		query = query.Where("type = ?", groupType)
	}
	if err := query.Order("updated_time DESC").Find(&groups).Error; err != nil {
		return nil, err
	}
	return groups, nil
}

func ValidatePrefillGroupIDsExist(tx *gorm.DB, groupType string, ids []int) error {
	if tx == nil {
		tx = DB
	}
	if tx == nil {
		return errors.New("nil db")
	}
	if len(ids) == 0 {
		return nil
	}
	groupType = strings.TrimSpace(groupType)
	if groupType == "" {
		return errors.New("prefill_group type 不能为空")
	}

	// Rely on gorm soft-delete filter to exclude deleted rows.
	var existing []int
	if err := tx.Model(&PrefillGroup{}).
		Where("id IN ? AND type = ?", ids, groupType).
		Pluck("id", &existing).Error; err != nil {
		return err
	}
	existSet := make(map[int]struct{}, len(existing))
	for _, id := range existing {
		existSet[id] = struct{}{}
	}
	missing := make([]int, 0)
	for _, id := range ids {
		if _, ok := existSet[id]; ok {
			continue
		}
		missing = append(missing, id)
	}
	if len(missing) > 0 {
		sort.Ints(missing)
		if groupType == "model" {
			return fmt.Errorf("模型预填组不存在: %v", missing)
		}
		return fmt.Errorf("预填组不存在: %v", missing)
	}
	return nil
}

func ValidateModelPrefillGroupNotReferenced(tx *gorm.DB, prefillGroupID int) error {
	if tx == nil {
		tx = DB
	}
	if tx == nil {
		return errors.New("nil db")
	}
	if prefillGroupID <= 0 {
		return errors.New("模型预填组 id 无效")
	}
	if !tx.Migrator().HasTable(&Group{}) {
		return nil
	}
	if !tx.Migrator().HasColumn(&Group{}, "allowed_model_prefill_group_ids") {
		return nil
	}

	type row struct {
		Id                        int      `gorm:"column:id"`
		Code                      string   `gorm:"column:code"`
		AllowedModelPrefillGroupIds JSONValue `gorm:"column:allowed_model_prefill_group_ids"`
	}
	var rows []row
	if err := tx.Model(&Group{}).
		Select("id", "code", "allowed_model_prefill_group_ids").
		Find(&rows).Error; err != nil {
		return err
	}

	refs := make([]string, 0)
	for _, r := range rows {
		if r.Id <= 0 {
			continue
		}
		if _, ids, err := normalizeAllowedModelPrefillGroupIDsJSON(r.AllowedModelPrefillGroupIds); err != nil {
			return fmt.Errorf("分组 %s 可选模型预填组配置无效: %w", strings.TrimSpace(r.Code), err)
		} else if len(ids) > 0 {
			for _, id := range ids {
				if id == prefillGroupID {
					code := strings.TrimSpace(r.Code)
					if code == "" {
						code = fmt.Sprintf("%d", r.Id)
					}
					refs = append(refs, fmt.Sprintf("%s(#%d)", code, r.Id))
					break
				}
			}
		}
	}

	if len(refs) == 0 {
		return nil
	}
	sort.Strings(refs)
	return fmt.Errorf("该模型预填组正在被分组引用: %s", strings.Join(refs, ", "))
}
