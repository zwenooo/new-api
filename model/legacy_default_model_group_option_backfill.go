package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

func BackfillGroupRatioEnsureLegacyDefaultModelGroup(tx *gorm.DB) error {
	if tx == nil {
		return fmt.Errorf("nil db")
	}
	legacyDefaultModelGroupCode, err := ResolveLegacyDefaultModelGroupCode(tx)
	if err != nil {
		// If legacy fallback cannot be resolved, keep startup non-fatal.
		return nil
	}

	var opt Option
	err = tx.Select("key", "value").
		Where(commonKeyCol+" = ?", "GroupRatio").
		First(&opt).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}

	raw := strings.TrimSpace(opt.Value)
	if raw == "" || raw == "null" {
		raw = "{}"
	}

	var groupRatio map[string]float64
	if err := json.Unmarshal([]byte(raw), &groupRatio); err != nil {
		return fmt.Errorf("GroupRatio 配置格式错误: %w", err)
	}
	if groupRatio == nil {
		groupRatio = make(map[string]float64)
	}
	if _, ok := groupRatio[legacyDefaultModelGroupCode]; ok {
		return nil
	}

	groupRatio[legacyDefaultModelGroupCode] = 1
	updated, err := json.Marshal(groupRatio)
	if err != nil {
		return err
	}
	return tx.Model(&Option{}).
		Where(commonKeyCol+" = ?", "GroupRatio").
		Update("value", string(updated)).Error
}

func BackfillUserUsableGroupsEnsureLegacyDefaultModelGroup(tx *gorm.DB) error {
	if tx == nil {
		return fmt.Errorf("nil db")
	}
	legacyDefaultModelGroupCode, err := ResolveLegacyDefaultModelGroupCode(tx)
	if err != nil {
		return nil
	}

	var opt Option
	err = tx.Select("key", "value").
		Where(commonKeyCol+" = ?", "UserUsableGroups").
		First(&opt).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}

	raw := strings.TrimSpace(opt.Value)
	if raw == "" || raw == "null" {
		raw = "{}"
	}

	var groups map[string]string
	if err := json.Unmarshal([]byte(raw), &groups); err != nil {
		return fmt.Errorf("UserUsableGroups 配置格式错误: %w", err)
	}
	if groups == nil {
		groups = make(map[string]string)
	}
	if _, ok := groups[legacyDefaultModelGroupCode]; ok {
		return nil
	}

	groups[legacyDefaultModelGroupCode] = legacyDefaultModelGroupCode + "分组"
	updated, err := json.Marshal(groups)
	if err != nil {
		return err
	}
	return tx.Model(&Option{}).
		Where(commonKeyCol+" = ?", "UserUsableGroups").
		Update("value", string(updated)).Error
}
