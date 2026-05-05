package model

import (
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
)

func GetUserRelaySubscriptionCandidateGroups(userId int) (quotaGroups []int, hasQuota bool, tokenGroups []int, hasToken bool, err error) {
	if userId <= 0 {
		return nil, false, nil, false, errors.New("userId 无效")
	}

	quotaGroups = make([]int, 0)
	tokenGroups = make([]int, 0)

	err = DB.Transaction(func(tx *gorm.DB) error {
		if err := ensureUserSubscriptionsFresh(tx, userId); err != nil {
			return err
		}

		now := time.Now().Unix()
		var subs []UserSubscription
		if err := tx.Select("id", "source_preset_id", "billing_unit").
			Where(
				"user_id = ? AND credited = ? AND invalid_at = 0 AND (total_quota = 0 OR remaining_quota > 0) AND (start_at = 0 OR start_at <= ?) AND (expire_at = 0 OR expire_at >= ?) AND (billing_unit = ? OR billing_unit = ? OR billing_unit IS NULL OR billing_unit = '')",
				userId,
				true,
				now,
				now,
				UserSubscriptionBillingUnitQuota,
				UserSubscriptionBillingUnitTokens,
			).
			Find(&subs).Error; err != nil {
			return err
		}
		if len(subs) == 0 {
			return nil
		}

		allowedBySubID, err := resolveSubscriptionAllowedGroupsTx(tx, subs)
		if err != nil {
			return err
		}

		quotaSeen := make(map[int]struct{})
		tokenSeen := make(map[int]struct{})
		for _, sub := range subs {
			unit := strings.TrimSpace(sub.BillingUnit)
			if unit == "" {
				unit = UserSubscriptionBillingUnitQuota
			}

			target := &quotaGroups
			seen := quotaSeen
			switch unit {
			case UserSubscriptionBillingUnitTokens:
				target = &tokenGroups
				seen = tokenSeen
				hasToken = true
			default:
				hasQuota = true
			}

			for _, gid := range allowedBySubID[sub.Id] {
				if gid <= 0 {
					continue
				}
				if _, ok := seen[gid]; ok {
					continue
				}
				seen[gid] = struct{}{}
				*target = append(*target, gid)
			}
		}
		return nil
	})
	if err != nil {
		return nil, false, nil, false, err
	}

	return normalizeUniqueSortedIDs(quotaGroups), hasQuota, normalizeUniqueSortedIDs(tokenGroups), hasToken, nil
}
