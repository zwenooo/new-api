package model

import (
	"fmt"
	"sort"

	"gorm.io/gorm"
)

type userSubscriptionEffectiveGroupDailyLimitSource struct {
	Kind       string
	SourceId   int
	RevisionId int
}

func (s userSubscriptionEffectiveGroupDailyLimitSource) missingGroupDailyLimitError(groupID int) error {
	label := "未知分组"
	if v, ok := GetGroupLabelByID(groupID); ok {
		label = v
	}
	switch s.Kind {
	case "preset_revision":
		return fmt.Errorf("商品 revision #%d 缺少分组 %s 日限额配置", s.RevisionId, label)
	case "preset":
		return fmt.Errorf("订阅商品 #%d 缺少分组 %s 日限额配置", s.SourceId, label)
	case "redemption":
		return fmt.Errorf("兑换码 #%d 缺少分组 %s 日限额配置", s.SourceId, label)
	default:
		return fmt.Errorf("订阅额度 #%d 缺少分组 %s 日限额配置", s.SourceId, label)
	}
}

func setEffectiveUserSubscriptionGroupDailyLimits(
	effective map[int]map[int]int,
	sourceBySubID map[int]userSubscriptionEffectiveGroupDailyLimitSource,
	sub UserSubscription,
	limitByGroupID map[int]int,
	source userSubscriptionEffectiveGroupDailyLimitSource,
) {
	if sub.BillingUnit == UserSubscriptionBillingUnitTokens {
		effective[sub.Id] = scaleEffectiveGroupLimitMapToStored(limitByGroupID)
	} else {
		effective[sub.Id] = limitByGroupID
	}
	sourceBySubID[sub.Id] = source
}

func loadEffectiveUserSubscriptionGroupDailyLimitsTx(tx *gorm.DB, subs []UserSubscription) (map[int]map[int]int, map[int]userSubscriptionEffectiveGroupDailyLimitSource, error) {
	if tx == nil {
		tx = DB
	}
	effective := make(map[int]map[int]int, len(subs))
	sourceBySubID := make(map[int]userSubscriptionEffectiveGroupDailyLimitSource, len(subs))
	if len(subs) == 0 {
		return effective, sourceBySubID, nil
	}

	productIDs := make([]int, 0, len(subs))
	redemptionIDs := make([]int, 0, len(subs))
	subIDs := make([]int, 0, len(subs))
	for _, sub := range subs {
		if sub.Id <= 0 {
			continue
		}
		subIDs = append(subIDs, sub.Id)
		if sub.SourcePresetId > 0 {
			productIDs = append(productIDs, sub.SourcePresetId)
		}
		if sub.SourceRedemptionId > 0 {
			redemptionIDs = append(redemptionIDs, sub.SourceRedemptionId)
		}
	}

	productGroupDailyLimitsByProductID, err := getSubscriptionProductGroupDailyLimitsByProductIDsTx(tx, productIDs)
	if err != nil {
		return nil, nil, err
	}
	presetExistsSet, err := getExistingRedemptionPresetIDSetTx(tx, productIDs)
	if err != nil {
		return nil, nil, err
	}
	redemptionGroupDailyLimitsByRedemptionID, err := getRedemptionGroupDailyLimitsByRedemptionIDsTx(tx, redemptionIDs)
	if err != nil {
		return nil, nil, err
	}
	userSubGroupDailyLimitsBySubID, err := getUserSubscriptionGroupDailyLimitsBySubscriptionIDsTx(tx, subIDs)
	if err != nil {
		return nil, nil, err
	}

	bindingBySubID, err := getUserSubscriptionPresetRevisionBindingsBySubscriptionIDsTx(tx, subIDs)
	if err != nil {
		return nil, nil, err
	}
	revisionIDs := make([]int, 0, len(bindingBySubID))
	for _, binding := range bindingBySubID {
		revisionIDs = append(revisionIDs, binding.RevisionId)
	}
	revisionGroupDailyLimitsByRevisionID, err := getRedemptionPresetRevisionGroupDailyLimitsByRevisionIDsTx(tx, revisionIDs)
	if err != nil {
		return nil, nil, err
	}

	for _, sub := range subs {
		if sub.Id <= 0 {
			continue
		}
		if sub.SourcePresetId > 0 {
			if sub.SourceRedemptionId > 0 && len(redemptionGroupDailyLimitsByRedemptionID[sub.SourceRedemptionId]) > 0 {
				setEffectiveUserSubscriptionGroupDailyLimits(
					effective,
					sourceBySubID,
					sub,
					redemptionGroupDailyLimitsByRedemptionID[sub.SourceRedemptionId],
					userSubscriptionEffectiveGroupDailyLimitSource{Kind: "redemption", SourceId: sub.SourceRedemptionId},
				)
				continue
			}
			if _, ok := presetExistsSet[sub.SourcePresetId]; ok {
				if len(productGroupDailyLimitsByProductID[sub.SourcePresetId]) > 0 {
					setEffectiveUserSubscriptionGroupDailyLimits(
						effective,
						sourceBySubID,
						sub,
						productGroupDailyLimitsByProductID[sub.SourcePresetId],
						userSubscriptionEffectiveGroupDailyLimitSource{Kind: "preset", SourceId: sub.SourcePresetId},
					)
				}
				continue
			}
		}
		if len(userSubGroupDailyLimitsBySubID[sub.Id]) > 0 {
			setEffectiveUserSubscriptionGroupDailyLimits(
				effective,
				sourceBySubID,
				sub,
				userSubGroupDailyLimitsBySubID[sub.Id],
				userSubscriptionEffectiveGroupDailyLimitSource{Kind: "subscription", SourceId: sub.Id},
			)
			continue
		}
		if sub.SourceRedemptionId > 0 && len(redemptionGroupDailyLimitsByRedemptionID[sub.SourceRedemptionId]) > 0 {
			setEffectiveUserSubscriptionGroupDailyLimits(
				effective,
				sourceBySubID,
				sub,
				redemptionGroupDailyLimitsByRedemptionID[sub.SourceRedemptionId],
				userSubscriptionEffectiveGroupDailyLimitSource{Kind: "redemption", SourceId: sub.SourceRedemptionId},
			)
			continue
		}
		if binding, ok := bindingBySubID[sub.Id]; ok {
			if len(revisionGroupDailyLimitsByRevisionID[binding.RevisionId]) > 0 {
				setEffectiveUserSubscriptionGroupDailyLimits(
					effective,
					sourceBySubID,
					sub,
					revisionGroupDailyLimitsByRevisionID[binding.RevisionId],
					userSubscriptionEffectiveGroupDailyLimitSource{
						Kind:       "preset_revision",
						SourceId:   binding.PresetId,
						RevisionId: binding.RevisionId,
					},
				)
			}
		}
	}
	return effective, sourceBySubID, nil
}

func buildUserSubscriptionEffectiveDailyLimitBySubID(effectiveGroupDailyLimitsBySubID map[int]map[int]int) map[int]int {
	out := make(map[int]int, len(effectiveGroupDailyLimitsBySubID))
	for subID, groupLimits := range effectiveGroupDailyLimitsBySubID {
		sum := 0
		hasUnlimited := false
		for _, limit := range groupLimits {
			if limit <= 0 {
				hasUnlimited = true
				break
			}
			sum += limit
		}
		if hasUnlimited {
			out[subID] = 0
			continue
		}
		out[subID] = sum
	}
	return out
}

func buildUserSubscriptionGroupDailyLimitItems(limitByGroupID map[int]int) []GroupDailyQuotaLimit {
	if len(limitByGroupID) == 0 {
		return nil
	}
	out := make([]GroupDailyQuotaLimit, 0, len(limitByGroupID))
	for groupID, dailyLimit := range limitByGroupID {
		out = append(out, GroupDailyQuotaLimit{
			GroupId:         groupID,
			DailyQuotaLimit: dailyLimit,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].GroupId < out[j].GroupId
	})
	return out
}

func addUserSubscriptionGroupDailyLimitConfigErr(addConfigErr func(string), sourceBySubID map[int]userSubscriptionEffectiveGroupDailyLimitSource, sub UserSubscription, groupID int) {
	source := sourceBySubID[sub.Id]
	if source.Kind != "" {
		addConfigErr(source.missingGroupDailyLimitError(groupID).Error())
		return
	}
	addConfigErr(fmt.Sprintf("订阅额度 #%d 缺少分组 #%d 日限额配置", sub.Id, groupID))
}

func resolveUserSubscriptionSummaryDailyLimit(sub UserSubscription, today int, effectiveGroupDailyLimitsBySubID map[int]map[int]int, effectiveDailyLimitBySubID map[int]int, groupModeUsedBySubID map[int]int) (int, int, []GroupDailyQuotaLimit) {
	used := sub.DailyQuotaUsed
	dailyLimit := sub.DailyQuotaLimit
	effectiveGroupDailyLimits := effectiveGroupDailyLimitsBySubID[sub.Id]
	if len(effectiveGroupDailyLimits) > 0 {
		used = groupModeUsedBySubID[sub.Id]
		dailyLimit = effectiveDailyLimitBySubID[sub.Id]
		return used, dailyLimit, buildUserSubscriptionGroupDailyLimitItems(effectiveGroupDailyLimits)
	}
	if sub.DailyQuotaLimit > 0 && sub.DailyQuotaResetDate != today {
		used = 0
	}
	return used, dailyLimit, nil
}
