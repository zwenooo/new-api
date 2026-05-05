package model

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

func BackfillTokenGroupLegacyDefaultModelGroup(tx *gorm.DB) error {
	if tx == nil {
		return fmt.Errorf("nil db")
	}
	if strings.TrimSpace(commonGroupCol) == "" {
		return fmt.Errorf("commonGroupCol is empty")
	}
	legacyDefaultModelGroupCode, err := ResolveLegacyDefaultModelGroupCode(tx)
	if err != nil {
		return err
	}
	// Legacy-only backfill:
	// - Only touch tokens without allowed_groups (older schema), and without an explicit default group.
	// - Never override tokens that already have allowed_groups set, otherwise it can break the invariant:
	//   token.group must be empty or included in token.allowed_groups.
	query := tx.Model(&Token{})
	query = query.Where(fmt.Sprintf("(%s IS NULL OR %s = '') AND %s", commonGroupCol, commonGroupCol, jsonColumnIsEmptyCondition("allowed_groups")))
	return query.Update("group", legacyDefaultModelGroupCode).Error
}
