package model

import (
	"fmt"
	"one-api/common"
	"strings"

	"gorm.io/gorm"
)

// BackfillTokenGroupEnsureInAllowedGroups clears token.group when it is not included in token.allowed_groups.
//
// Invariant:
// - token.group must be empty OR included in token.allowed_groups (when allowed_groups is non-empty).
//
// This fixes historical data corruption caused by legacy backfills / old clients.
func BackfillTokenGroupEnsureInAllowedGroups(tx *gorm.DB) error {
	if tx == nil {
		tx = DB
	}
	if tx == nil {
		return fmt.Errorf("nil db")
	}
	if strings.TrimSpace(commonGroupCol) == "" {
		return fmt.Errorf("commonGroupCol is empty")
	}

	type tokenRow struct {
		Id            int       `gorm:"column:id"`
		Group         string    `gorm:"column:group"`
		AllowedGroups JSONValue `gorm:"column:allowed_groups"`
	}

	query := tx.Model(&Token{}).
		Select("id", commonGroupCol, "allowed_groups").
		Where(fmt.Sprintf("%s IS NOT NULL AND %s <> ''", commonGroupCol, commonGroupCol))
	query = query.Where(jsonColumnIsNotEmptyCondition("allowed_groups"))

	var rows []tokenRow
	if err := query.Find(&rows).Error; err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}

	invalidIDs := make([]int, 0)
	for _, row := range rows {
		group := strings.TrimSpace(row.Group)
		if group == "" {
			continue
		}
		allowed, err := ParseGroupNamesJSON(row.AllowedGroups)
		if err != nil {
			return fmt.Errorf("token#%d allowed_groups 解析失败: %w", row.Id, err)
		}
		if len(allowed) == 0 {
			continue
		}
		if !common.StringsContains(allowed, group) {
			invalidIDs = append(invalidIDs, row.Id)
		}
	}
	if len(invalidIDs) == 0 {
		return nil
	}
	return tx.Model(&Token{}).Where("id IN ?", invalidIDs).Update("group", "").Error
}
