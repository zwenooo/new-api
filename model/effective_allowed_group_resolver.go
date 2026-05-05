package model

import (
	"errors"
	"fmt"

	"gorm.io/gorm"
)

type effectiveAllowedGroupTarget struct {
	OwnerID          int
	ProductID        int
	RevisionID       int
	SnapshotGroupIDs []int
}

type effectiveAllowedGroupResolverOptions struct {
	OwnerLabel    string
	SnapshotLabel string
}

type effectivePrepaidBalanceAllowedGroupLookup struct {
	LoadCurrentProductGroupIDs func(tx *gorm.DB, productID int) ([]int, error)
	LoadConfiguredGroupIDs     func(productID int) ([]int, bool)
}

type effectivePrepaidBalanceAllowedGroupOptions struct {
	ProductID              int
	FollowCurrentProduct   bool
	StoredGroupIDs         JSONValue
	StoredGroups           JSONValue
	EmptyProductMessage    string
	EmptySnapshotMessage   string
	MissingSnapshotMessage string
}

// getExistingRedemptionPresetIDSetTx returns the preset ids that still exist.
func getExistingRedemptionPresetIDSetTx(tx *gorm.DB, presetIDs []int) (map[int]struct{}, error) {
	if tx == nil {
		tx = DB
	}
	out := make(map[int]struct{}, len(presetIDs))
	ids := normalizeUniqueSortedIDs(presetIDs)
	if len(ids) == 0 {
		return out, nil
	}
	if !tx.Migrator().HasTable(&RedemptionPreset{}) {
		return out, nil
	}
	var existing []int
	if err := tx.Model(&RedemptionPreset{}).
		Where("id IN ?", ids).
		Pluck("id", &existing).Error; err != nil {
		return nil, err
	}
	for _, id := range existing {
		if id > 0 {
			out[id] = struct{}{}
		}
	}
	return out, nil
}

func loadSubscriptionProductGroupIDsByProductIDsTx(tx *gorm.DB, productIDs []int) (map[int][]int, error) {
	if tx == nil {
		tx = DB
	}
	out := make(map[int][]int, len(productIDs))
	ids := normalizeUniqueSortedIDs(productIDs)
	if len(ids) == 0 || !tx.Migrator().HasTable(&SubscriptionProductGroup{}) {
		return out, nil
	}
	type row struct {
		ProductId int `gorm:"column:product_id"`
		GroupId   int `gorm:"column:group_id"`
	}
	var rows []row
	if err := tx.Model(&SubscriptionProductGroup{}).
		Select("product_id", "group_id").
		Where("product_id IN ?", ids).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, r := range rows {
		if r.ProductId <= 0 || r.GroupId <= 0 {
			continue
		}
		out[r.ProductId] = append(out[r.ProductId], r.GroupId)
	}
	for productID, groupIDs := range out {
		filtered, err := filterExistingSortedIDsTx(tx, groupIDs)
		if err != nil {
			return nil, err
		}
		out[productID] = filtered
	}
	return out, nil
}

func resolveEffectiveAllowedGroupsTx(tx *gorm.DB, targets []effectiveAllowedGroupTarget, opts effectiveAllowedGroupResolverOptions) (map[int][]int, error) {
	if tx == nil {
		tx = DB
	}
	out := make(map[int][]int, len(targets))
	if len(targets) == 0 {
		return out, nil
	}

	ownerLabel := opts.OwnerLabel
	if ownerLabel == "" {
		ownerLabel = "记录"
	}
	snapshotLabel := opts.SnapshotLabel
	if snapshotLabel == "" {
		snapshotLabel = "快照"
	}

	productIDs := make([]int, 0, len(targets))
	revisionIDs := make([]int, 0, len(targets))
	for _, target := range targets {
		if target.ProductID > 0 {
			productIDs = append(productIDs, target.ProductID)
		}
		if target.RevisionID > 0 {
			revisionIDs = append(revisionIDs, target.RevisionID)
		}
	}

	productGroupIDsByProductID, err := loadSubscriptionProductGroupIDsByProductIDsTx(tx, productIDs)
	if err != nil {
		return nil, err
	}
	presetExistsSet, err := getExistingRedemptionPresetIDSetTx(tx, productIDs)
	if err != nil {
		return nil, err
	}
	revisionGroupIDsByRevisionID, err := getRedemptionPresetRevisionGroupIDsByRevisionIDsTx(tx, revisionIDs)
	if err != nil {
		return nil, err
	}

	validatedGroupIDs := make(map[int]struct{})

	for _, target := range targets {
		if target.OwnerID <= 0 {
			continue
		}

		var allowedIDs []int
		if target.ProductID > 0 {
			allowedIDs = normalizeUniqueSortedIDs(productGroupIDsByProductID[target.ProductID])
			if len(allowedIDs) > 0 {
				out[target.OwnerID] = allowedIDs
				continue
			}
			if _, ok := presetExistsSet[target.ProductID]; ok {
				return nil, fmt.Errorf("%s #%d 绑定商品 #%d 未配置可用分组", ownerLabel, target.OwnerID, target.ProductID)
			}
			if target.RevisionID > 0 {
				allowedIDs = normalizeUniqueSortedIDs(revisionGroupIDsByRevisionID[target.RevisionID])
			}
			if len(allowedIDs) == 0 {
				allowedIDs = normalizeUniqueSortedIDs(target.SnapshotGroupIDs)
			}
			if len(allowedIDs) == 0 {
				if target.RevisionID > 0 {
					return nil, fmt.Errorf("%s #%d 绑定商品已不存在，且商品 revision #%d 缺少可用分组", ownerLabel, target.OwnerID, target.RevisionID)
				}
				return nil, fmt.Errorf("%s #%d 绑定商品 #%d 已不存在，且%s缺少可用分组", ownerLabel, target.OwnerID, target.ProductID, snapshotLabel)
			}
			allowedIDs, err = validateResolvedAllowedGroupsTx(tx, allowedIDs, validatedGroupIDs)
			if err != nil {
				return nil, err
			}
			if len(allowedIDs) == 0 {
				if target.RevisionID > 0 {
					return nil, fmt.Errorf("%s #%d 绑定商品已不存在，且商品 revision #%d 缺少可用分组", ownerLabel, target.OwnerID, target.RevisionID)
				}
				return nil, fmt.Errorf("%s #%d 绑定商品 #%d 已不存在，且%s缺少可用分组", ownerLabel, target.OwnerID, target.ProductID, snapshotLabel)
			}
			out[target.OwnerID] = allowedIDs
			continue
		}

		allowedIDs = normalizeUniqueSortedIDs(target.SnapshotGroupIDs)
		if len(allowedIDs) == 0 && target.RevisionID > 0 {
			allowedIDs = normalizeUniqueSortedIDs(revisionGroupIDsByRevisionID[target.RevisionID])
			if len(allowedIDs) == 0 {
				return nil, fmt.Errorf("%s #%d 绑定商品 revision #%d 缺少可用分组", ownerLabel, target.OwnerID, target.RevisionID)
			}
		}
		if len(allowedIDs) == 0 {
			return nil, fmt.Errorf("%s #%d 缺少可用分组", ownerLabel, target.OwnerID)
		}
		allowedIDs, err = validateResolvedAllowedGroupsTx(tx, allowedIDs, validatedGroupIDs)
		if err != nil {
			return nil, err
		}
		if len(allowedIDs) == 0 {
			if target.RevisionID > 0 {
				return nil, fmt.Errorf("%s #%d 绑定商品 revision #%d 缺少可用分组", ownerLabel, target.OwnerID, target.RevisionID)
			}
			return nil, fmt.Errorf("%s #%d 缺少可用分组", ownerLabel, target.OwnerID)
		}
		out[target.OwnerID] = allowedIDs
	}

	return out, nil
}

func validateResolvedAllowedGroupsTx(tx *gorm.DB, allowedIDs []int, validatedGroupIDs map[int]struct{}) ([]int, error) {
	if len(allowedIDs) == 0 {
		return nil, nil
	}
	missingValidation := make([]int, 0, len(allowedIDs))
	for _, groupID := range allowedIDs {
		if groupID <= 0 {
			continue
		}
		if _, ok := validatedGroupIDs[groupID]; ok {
			continue
		}
		missingValidation = append(missingValidation, groupID)
	}
	if len(missingValidation) == 0 {
		return normalizeUniqueSortedIDs(allowedIDs), nil
	}
	validMissing, err := filterExistingSortedIDsTx(tx, missingValidation)
	if err != nil {
		return nil, err
	}
	validSet := make(map[int]struct{}, len(validMissing))
	for _, groupID := range validMissing {
		validSet[groupID] = struct{}{}
	}
	filtered := make([]int, 0, len(allowedIDs))
	for _, groupID := range allowedIDs {
		if groupID <= 0 {
			continue
		}
		if _, ok := validatedGroupIDs[groupID]; ok {
			filtered = append(filtered, groupID)
			continue
		}
		if _, ok := validSet[groupID]; !ok {
			continue
		}
		filtered = append(filtered, groupID)
	}
	for _, groupID := range validMissing {
		validatedGroupIDs[groupID] = struct{}{}
	}
	return normalizeUniqueSortedIDs(filtered), nil
}

func resolveEffectivePrepaidBalanceAllowedGroupIDsTx(tx *gorm.DB, lookup effectivePrepaidBalanceAllowedGroupLookup, opts effectivePrepaidBalanceAllowedGroupOptions) ([]int, error) {
	if tx == nil {
		tx = DB
	}
	if opts.FollowCurrentProduct && opts.ProductID > 0 {
		if lookup.LoadCurrentProductGroupIDs != nil {
			ids, err := lookup.LoadCurrentProductGroupIDs(tx, opts.ProductID)
			if err == nil && len(ids) > 0 {
				ids = normalizeUniqueSortedIDs(ids)
				if len(ids) == 0 {
					return nil, errors.New(opts.EmptyProductMessage)
				}
				return ids, nil
			}
		}
		if lookup.LoadConfiguredGroupIDs != nil {
			if ids, ok := lookup.LoadConfiguredGroupIDs(opts.ProductID); ok {
				ids = normalizeUniqueSortedIDs(ids)
				if len(ids) == 0 {
					return nil, errors.New(opts.EmptyProductMessage)
				}
				return ids, nil
			}
		}
	}

	if len(opts.StoredGroupIDs) > 0 {
		ids, err := ParseGroupIDsJSON(opts.StoredGroupIDs)
		if err != nil {
			return nil, err
		}
		if len(ids) == 0 {
			return nil, errors.New(opts.EmptySnapshotMessage)
		}
		return ids, nil
	}

	if len(opts.StoredGroups) > 0 {
		ids, idErr := ParseGroupIDsJSON(opts.StoredGroups)
		if idErr == nil && len(ids) > 0 {
			return ids, nil
		}
		codes, codeErr := ParseGroupNamesJSON(opts.StoredGroups)
		if codeErr != nil {
			if idErr != nil {
				return nil, idErr
			}
			return nil, codeErr
		}
		if len(codes) == 0 {
			return nil, errors.New(opts.EmptySnapshotMessage)
		}
		ids, _, err := existingLegacyGroupIDsFromCodes(tx, codes)
		if err != nil {
			return nil, err
		}
		ids = normalizeUniqueSortedIDs(ids)
		if len(ids) == 0 {
			return nil, errors.New(opts.EmptySnapshotMessage)
		}
		return ids, nil
	}

	return nil, errors.New(opts.MissingSnapshotMessage)
}
