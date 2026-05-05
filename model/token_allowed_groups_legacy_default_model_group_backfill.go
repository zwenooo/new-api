package model

import "gorm.io/gorm"

// BackfillTokenAllowedGroupsLegacyDefaultModelGroup sets allowed_groups to the effective
// legacy default model-group for legacy tokens that have empty/NULL allowed_groups.
// for legacy tokens that have empty/NULL allowed_groups, to avoid being treated as unrestricted.
func BackfillTokenAllowedGroupsLegacyDefaultModelGroup(tx *gorm.DB) error {
	if tx == nil {
		tx = DB
	}
	legacyDefaultModelGroupCode, err := ResolveLegacyDefaultModelGroupCode(tx)
	if err != nil {
		return err
	}
	allowedGroups, err := MarshalGroupNamesJSON([]string{legacyDefaultModelGroupCode})
	if err != nil {
		return err
	}
	query := tx.Model(&Token{})
	query = query.Where(jsonColumnIsEmptyCondition("allowed_groups"))
	return query.Update("allowed_groups", allowedGroups).Error
}
