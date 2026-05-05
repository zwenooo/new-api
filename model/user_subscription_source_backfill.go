package model

import (
	"strconv"
	"strings"

	"gorm.io/gorm"
)

// BackfillUserSubscriptionSourceRefs parses legacy UserSubscription.Source and fills denormalized references.
// It is best-effort and only touches rows that haven't been backfilled yet.
func BackfillUserSubscriptionSourceRefs(tx *gorm.DB) error {
	if tx == nil {
		tx = DB
	}

	type subRow struct {
		Id     int
		Source string
	}

	// subscription_order:<order_id> -> (source_order_id, source_preset_id)
	{
		var subs []subRow
		if err := tx.Model(&UserSubscription{}).
			Select("id", "source").
			Where("source_order_id = 0 AND source_preset_id = 0").
			Where("source LIKE ?", "subscription_order:%").
			Find(&subs).Error; err != nil {
			return err
		}
		if len(subs) > 0 {
			orderIDs := make([]int, 0, len(subs))
			sub2order := make(map[int]int, len(subs))
			seenOrder := make(map[int]struct{}, len(subs))
			for _, s := range subs {
				raw := strings.TrimSpace(s.Source)
				raw = strings.TrimPrefix(raw, "subscription_order:")
				raw = strings.TrimSpace(raw)
				oid, err := strconv.Atoi(raw)
				if err != nil || oid <= 0 {
					continue
				}
				sub2order[s.Id] = oid
				if _, ok := seenOrder[oid]; ok {
					continue
				}
				seenOrder[oid] = struct{}{}
				orderIDs = append(orderIDs, oid)
			}

			if len(orderIDs) > 0 {
				type orderRow struct {
					Id       int `gorm:"column:id"`
					PresetId int `gorm:"column:preset_id"`
				}
				var orders []orderRow
				if err := tx.Model(&SubscriptionOrder{}).
					Select("id", "preset_id").
					Where("id IN ?", orderIDs).
					Find(&orders).Error; err != nil {
					return err
				}
				orderExists := make(map[int]struct{}, len(orders))
				order2preset := make(map[int]int, len(orders))
				presetIDSet := make(map[int]struct{}, len(orders))
				for _, o := range orders {
					if o.Id <= 0 {
						continue
					}
					orderExists[o.Id] = struct{}{}
					if o.PresetId <= 0 {
						continue
					}
					order2preset[o.Id] = o.PresetId
					presetIDSet[o.PresetId] = struct{}{}
				}

				// Only link to existing presets. Historical databases might contain deleted presets.
				presetExists := make(map[int]struct{}, len(presetIDSet))
				if len(presetIDSet) > 0 && tx.Migrator().HasTable(&RedemptionPreset{}) {
					ids := make([]int, 0, len(presetIDSet))
					for pid := range presetIDSet {
						ids = append(ids, pid)
					}
					var existing []int
					if err := tx.Model(&RedemptionPreset{}).Where("id IN ?", ids).Pluck("id", &existing).Error; err != nil {
						return err
					}
					for _, pid := range existing {
						if pid > 0 {
							presetExists[pid] = struct{}{}
						}
					}
				}

				for subID, oid := range sub2order {
					if _, ok := orderExists[oid]; !ok {
						continue
					}
					pid := order2preset[oid]
					updates := map[string]any{
						"source_order_id": oid,
					}
					if pid > 0 {
						if _, ok := presetExists[pid]; ok {
							updates["source_preset_id"] = pid
						}
					}
					if err := tx.Model(&UserSubscription{}).
						Where("id = ?", subID).
						Updates(updates).Error; err != nil {
						return err
					}
				}
			}
		}
	}

	// redeem:<redemption_id> -> source_redemption_id
	{
		var subs []subRow
		if err := tx.Model(&UserSubscription{}).
			Select("id", "source").
			Where("source_redemption_id = 0").
			Where("source LIKE ?", "redeem:%").
			Find(&subs).Error; err != nil {
			return err
		}
		for _, s := range subs {
			raw := strings.TrimSpace(s.Source)
			raw = strings.TrimPrefix(raw, "redeem:")
			raw = strings.TrimSpace(raw)
			rid, err := strconv.Atoi(raw)
			if err != nil || rid <= 0 {
				continue
			}
			if err := tx.Model(&UserSubscription{}).
				Where("id = ?", s.Id).
				Update("source_redemption_id", rid).Error; err != nil {
				return err
			}
		}
	}

	return nil
}
