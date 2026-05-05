package model

import (
	"errors"

	"gorm.io/gorm"
)

// BackfillRedemptionPresetAllowedGroupsDefaults fills allowed_group_ids for legacy redemption presets that
// were created before allowed_group_ids became mandatory.
//
// Policy:
// - subscription/tokens/request presets default to the effective legacy fallback group
// - payg presets default to the effective payg fallback group
func BackfillRedemptionPresetAllowedGroupsDefaults(tx *gorm.DB) error {
	if tx == nil {
		tx = DB
	}
	if tx == nil {
		return errors.New("nil db")
	}
	if !tx.Migrator().HasTable(&RedemptionPreset{}) {
		return nil
	}
	if !tx.Migrator().HasColumn(&RedemptionPreset{}, "allowed_group_ids") {
		return nil
	}

	legacyDefaultModelGroupID, err := ResolveLegacyDefaultModelGroupID(tx)
	if err != nil {
		return err
	}
	subscriptionDefault, err := MarshalGroupIDsJSON([]int{legacyDefaultModelGroupID})
	if err != nil {
		return err
	}
	legacyPaygModelGroupID, err := ResolveLegacyPaygModelGroupID(tx)
	if err != nil {
		return err
	}
	paygDefault, err := MarshalGroupIDsJSON([]int{legacyPaygModelGroupID})
	if err != nil {
		return err
	}

	emptyAllowedGroupIDsWhere := func(q *gorm.DB) *gorm.DB {
		return q.Where(jsonColumnIsEmptyCondition("allowed_group_ids"))
	}

	// subscription/tokens/request
	if err := emptyAllowedGroupIDsWhere(tx.Model(&RedemptionPreset{}).Where("mode IN ?", []string{"subscription", "tokens", "request"})).
		Update("allowed_group_ids", subscriptionDefault).Error; err != nil {
		return err
	}
	// payg
	if err := emptyAllowedGroupIDsWhere(tx.Model(&RedemptionPreset{}).Where("mode = ?", "payg")).
		Update("allowed_group_ids", paygDefault).Error; err != nil {
		return err
	}
	return nil
}
