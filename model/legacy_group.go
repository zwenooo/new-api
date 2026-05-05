package model

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"gorm.io/gorm"
)

var (
	legacySubscriptionDefaultGroupIDCache   int
	legacySubscriptionDefaultGroupIDCacheMu sync.RWMutex
)

func IsInternalDefaultModelGroupCode(code string) bool {
	return strings.EqualFold(strings.TrimSpace(code), "default")
}

func IsInternalDefaultModelGroupID(tx *gorm.DB, groupID int) bool {
	if groupID <= 0 {
		return false
	}
	group, err := GetGroupByID(tx, groupID)
	if err != nil || group == nil {
		return false
	}
	return IsInternalDefaultModelGroupCode(group.Code)
}

func ensureDefaultModelGroupTx(tx *gorm.DB) (*Group, error) {
	if tx == nil {
		tx = DB
	}
	if tx == nil {
		return nil, errors.New("nil db")
	}
	if group, err := GetGroupByCode(tx, "default"); err == nil && group != nil && group.Id > 0 {
		if group.UserSelectable {
			if err := tx.Model(&Group{}).Where("id = ?", group.Id).Update("user_selectable", false).Error; err != nil {
				return nil, err
			}
			group.UserSelectable = false
		}
		return group, nil
	} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	group := &Group{
		Code:           "default",
		Ratio:          1,
		UserSelectable: false,
		Enabled:        true,
		Description:    "system default model group",
	}
	if err := CreateGroup(tx, group); err != nil {
		return nil, err
	}
	group.NormalizeForResponse()
	return group, nil
}

func resolvePreferredFallbackGroup(tx *gorm.DB, preferredCodes ...string) (*Group, error) {
	if tx == nil {
		tx = DB
	}
	if tx == nil {
		return nil, errors.New("nil db")
	}

	for _, code := range preferredCodes {
		code = strings.TrimSpace(code)
		if code == "" {
			continue
		}
		if group, err := GetGroupByCode(tx, code); err == nil && group != nil && group.Id > 0 {
			return group, nil
		}
	}

	for _, code := range preferredCodes {
		if strings.EqualFold(strings.TrimSpace(code), "default") {
			return ensureDefaultModelGroupTx(tx)
		}
	}
	return nil, fmt.Errorf("缺少可用的 legacy 默认分组")
}

func resolveLegacyDefaultModelGroup(tx *gorm.DB) (*Group, error) {
	return resolvePreferredFallbackGroup(tx, legacySubscriptionDefaultGroup, "default")
}

func resolveLegacyPaygModelGroup(tx *gorm.DB) (*Group, error) {
	return resolvePreferredFallbackGroup(tx, "payg", "default", legacySubscriptionDefaultGroup)
}

// ResolveLegacyDefaultModelGroupID returns the effective legacy default model-group id.
// It prefers explicit legacy defaults only and never falls back to an arbitrary active group.
func ResolveLegacyDefaultModelGroupID(tx *gorm.DB) (int, error) {
	group, err := resolveLegacyDefaultModelGroup(tx)
	if err != nil {
		return 0, err
	}
	if group == nil || group.Id <= 0 {
		return 0, errors.New("legacy 默认模型分组 id 无效")
	}
	return group.Id, nil
}

func ResolveLegacyDefaultModelGroupCode(tx *gorm.DB) (string, error) {
	group, err := resolveLegacyDefaultModelGroup(tx)
	if err != nil {
		return "", err
	}
	if group == nil {
		return "", errors.New("legacy 默认模型分组不存在")
	}
	code := strings.TrimSpace(group.Code)
	if code == "" {
		return "", errors.New("legacy 默认模型分组 code 无效")
	}
	return code, nil
}

func ResolveLegacyPaygModelGroupID(tx *gorm.DB) (int, error) {
	group, err := resolveLegacyPaygModelGroup(tx)
	if err != nil {
		return 0, err
	}
	if group == nil || group.Id <= 0 {
		return 0, errors.New("legacy payg 模型分组 id 无效")
	}
	return group.Id, nil
}

func ResolveLegacyPaygModelGroupCode(tx *gorm.DB) (string, error) {
	group, err := resolveLegacyPaygModelGroup(tx)
	if err != nil {
		return "", err
	}
	if group == nil {
		return "", errors.New("legacy payg 模型分组不存在")
	}
	code := strings.TrimSpace(group.Code)
	if code == "" {
		return "", errors.New("legacy payg 模型分组 code 无效")
	}
	return code, nil
}

// ResolveLegacyDefaultModelGroupIDCached caches a successful lookup of ResolveLegacyDefaultModelGroupID.
// It intentionally does not cache failures to avoid "sticky" boot-time errors.
func ResolveLegacyDefaultModelGroupIDCached() (int, error) {
	legacySubscriptionDefaultGroupIDCacheMu.RLock()
	cached := legacySubscriptionDefaultGroupIDCache
	legacySubscriptionDefaultGroupIDCacheMu.RUnlock()
	if cached > 0 {
		return cached, nil
	}

	id, err := ResolveLegacyDefaultModelGroupID(nil)
	if err != nil {
		return 0, err
	}
	legacySubscriptionDefaultGroupIDCacheMu.Lock()
	legacySubscriptionDefaultGroupIDCache = id
	legacySubscriptionDefaultGroupIDCacheMu.Unlock()
	return id, nil
}

func LegacySubscriptionDefaultGroupID(tx *gorm.DB) (int, error) {
	return ResolveLegacyDefaultModelGroupID(tx)
}

func LegacySubscriptionDefaultGroupCode(tx *gorm.DB) (string, error) {
	return ResolveLegacyDefaultModelGroupCode(tx)
}

func PaygFallbackGroupID(tx *gorm.DB) (int, error) {
	return ResolveLegacyPaygModelGroupID(tx)
}

func PaygFallbackGroupCode(tx *gorm.DB) (string, error) {
	return ResolveLegacyPaygModelGroupCode(tx)
}

func LegacySubscriptionDefaultGroupIDCached() (int, error) {
	return ResolveLegacyDefaultModelGroupIDCached()
}
