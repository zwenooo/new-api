package model

import (
	"fmt"

	"gorm.io/gorm"
)

// BackfillTokenAllowedGroupsSortOrder initializes token_allowed_groups.sort_order for legacy rows.
//
// When sort_order column is introduced, historical rows will typically have the default value 0.
// For tokens with more than one allowed group and all sort_order values equal to 0, this backfill
// assigns a deterministic order:
// - token.default_group_id (when present and included) is placed first
// - remaining groups are ordered by group_id ASC
func BackfillTokenAllowedGroupsSortOrder(tx *gorm.DB) error {
	if tx == nil {
		tx = DB
	}
	if tx == nil {
		return fmt.Errorf("nil db")
	}
	if !tx.Migrator().HasTable(&TokenAllowedGroup{}) {
		return nil
	}
	if !tx.Migrator().HasColumn(&TokenAllowedGroup{}, "sort_order") {
		return nil
	}

	var tokenIDs []int
	if err := tx.Model(&TokenAllowedGroup{}).
		Select("token_id").
		Group("token_id").
		Having("COUNT(*) > 1").
		Having("MAX(sort_order) = 0").
		Pluck("token_id", &tokenIDs).Error; err != nil {
		return err
	}
	if len(tokenIDs) == 0 {
		return nil
	}

	type tokenRow struct {
		Id             int `gorm:"column:id"`
		DefaultGroupId int `gorm:"column:default_group_id"`
	}
	var tokens []tokenRow
	if err := tx.Model(&Token{}).
		Select("id", "default_group_id").
		Where("id IN ?", tokenIDs).
		Find(&tokens).Error; err != nil {
		return err
	}
	defaultByToken := make(map[int]int, len(tokens))
	for _, t := range tokens {
		if t.Id <= 0 {
			continue
		}
		defaultByToken[t.Id] = t.DefaultGroupId
	}

	type row struct {
		TokenId int `gorm:"column:token_id"`
		GroupId int `gorm:"column:group_id"`
	}
	var rows []row
	if err := tx.Model(&TokenAllowedGroup{}).
		Select("token_id", "group_id").
		Where("token_id IN ?", tokenIDs).
		Order("token_id ASC").
		Order("group_id ASC").
		Find(&rows).Error; err != nil {
		return err
	}
	byToken := make(map[int][]int, len(tokenIDs))
	for _, r := range rows {
		if r.TokenId <= 0 || r.GroupId <= 0 {
			continue
		}
		byToken[r.TokenId] = append(byToken[r.TokenId], r.GroupId)
	}

	for _, tokenID := range tokenIDs {
		groupIDs := normalizeUniqueSortedIDs(byToken[tokenID])
		if len(groupIDs) <= 1 {
			continue
		}

		defaultGroupID := defaultByToken[tokenID]
		desired := make([]int, 0, len(groupIDs))
		if defaultGroupID > 0 {
			for _, gid := range groupIDs {
				if gid == defaultGroupID {
					desired = append(desired, gid)
					break
				}
			}
		}
		for _, gid := range groupIDs {
			if gid <= 0 {
				continue
			}
			if len(desired) > 0 && gid == desired[0] {
				continue
			}
			desired = append(desired, gid)
		}
		if len(desired) != len(groupIDs) {
			return fmt.Errorf("token#%d sort_order backfill failed: group count mismatch", tokenID)
		}

		caseSQL := "CASE group_id"
		args := make([]interface{}, 0, len(desired)*2+1)
		for idx, gid := range desired {
			caseSQL += " WHEN ? THEN ?"
			args = append(args, gid, idx)
		}
		caseSQL += " ELSE sort_order END"
		args = append(args, tokenID)

		if err := tx.Exec(
			fmt.Sprintf("UPDATE %s SET sort_order = %s WHERE token_id = ?", (&TokenAllowedGroup{}).TableName(), caseSQL),
			args...,
		).Error; err != nil {
			return err
		}
	}

	return nil
}

