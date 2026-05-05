package model

import (
	"errors"
	"fmt"
	"one-api/common"
	"one-api/setting/operation_setting"
	"sort"

	"gorm.io/gorm"
)

const optionKeyBackfillInvitationIsFirstPurchasePaidEvents = "BackfillInvitationIsFirstPurchasePaidEventsV2"

type paidEventKind string

const (
	paidEventKindSubscription paidEventKind = "subscription"
	paidEventKindPayg         paidEventKind = "payg"
	paidEventKindPayRequest   paidEventKind = "pay_request"
	paidEventKindPayToken     paidEventKind = "pay_token"
	paidEventKindRedemption   paidEventKind = "redemption"
)

type paidEvent struct {
	kind paidEventKind
	id   int
	ts   int64
}

func CountUserSuccessfulCommissionablePaidEventsTx(tx *gorm.DB, userId int) (int64, error) {
	if userId <= 0 {
		return 0, errors.New("userId 无效")
	}
	if tx == nil {
		tx = DB
	}
	if tx == nil {
		return 0, errors.New("tx 为空")
	}

	var total int64
	counts := []struct {
		model interface{}
		query string
		args  []interface{}
	}{
		{
			model: &SubscriptionOrder{},
			query: "user_id = ? AND status = ? AND pay_method = ?",
			args:  []interface{}{userId, SubscriptionOrderStatusSuccess, SubscriptionPayMethodEpay},
		},
		{
			model: &PaygOrder{},
			query: "user_id = ? AND status = ? AND pay_method = ?",
			args:  []interface{}{userId, PaygOrderStatusSuccess, PaygPayMethodEpay},
		},
		{
			model: &PayRequestOrder{},
			query: "user_id = ? AND status = ? AND pay_method = ?",
			args:  []interface{}{userId, PayRequestOrderStatusSuccess, PayRequestPayMethodEpay},
		},
		{
			model: &PayTokenOrder{},
			query: "user_id = ? AND status = ? AND pay_method = ?",
			args:  []interface{}{userId, PayTokenOrderStatusSuccess, PayTokenPayMethodEpay},
		},
		{
			model: &Redemption{},
			query: "used_user_id = ? AND status = ? AND price_fen > 0",
			args:  []interface{}{userId, common.RedemptionCodeStatusUsed},
		},
	}

	for _, item := range counts {
		var count int64
		if err := tx.Model(item.model).Where(item.query, item.args...).Count(&count).Error; err != nil {
			return 0, err
		}
		total += count
	}

	return total, nil
}

func GetInvitationCommissionPercent(isFirst bool) (int, error) {
	percent := operation_setting.SubscriptionInviteCommissionRepeatPercent
	if isFirst {
		percent = operation_setting.SubscriptionInviteCommissionFirstPercent
	}
	if percent < 0 || percent > 100 {
		return 0, errors.New("分佣比例配置错误")
	}
	return percent, nil
}

func ApplyInvitationCommissionTx(tx *gorm.DB, userId int, settledFen int64, isFirst bool) (int, int, int64, error) {
	if tx == nil {
		tx = DB
	}
	if tx == nil {
		return 0, 0, 0, errors.New("tx 为空")
	}
	if userId <= 0 {
		return 0, 0, 0, errors.New("userId 无效")
	}
	if settledFen < 0 {
		return 0, 0, 0, errors.New("settledFen 无效")
	}

	commissionPercent, err := GetInvitationCommissionPercent(isFirst)
	if err != nil {
		return 0, 0, 0, err
	}

	var buyer User
	if err := lockForUpdate(tx).
		Select("id", "inviter_id").
		Where("id = ?", userId).
		First(&buyer).Error; err != nil {
		return 0, 0, 0, err
	}

	inviterId := buyer.InviterId
	if inviterId <= 0 || commissionPercent <= 0 || settledFen <= 0 {
		return inviterId, commissionPercent, 0, nil
	}

	commissionFen := (settledFen * int64(commissionPercent)) / 100
	if commissionFen <= 0 {
		return inviterId, commissionPercent, 0, nil
	}

	if err := tx.Model(&User{}).Where("id = ?", inviterId).Updates(map[string]interface{}{
		"aff_quota":   gorm.Expr("aff_quota + ?", commissionFen),
		"aff_history": gorm.Expr("aff_history + ?", commissionFen),
	}).Error; err != nil {
		return 0, 0, 0, err
	}

	return inviterId, commissionPercent, commissionFen, nil
}

func BackfillInvitationIsFirstPurchasePaidEvents(db *gorm.DB) error {
	if db == nil {
		db = DB
	}
	if db == nil {
		return errors.New("db 为空")
	}

	var marker Option
	if err := db.First(&marker, commonKeyCol+" = ?", optionKeyBackfillInvitationIsFirstPurchasePaidEvents).Error; err == nil {
		if marker.Value == "true" {
			return nil
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	var subscriptionUserIds []int
	if err := db.Model(&SubscriptionOrder{}).
		Distinct("user_id").
		Where("status = ? AND pay_method = ?", SubscriptionOrderStatusSuccess, SubscriptionPayMethodEpay).
		Pluck("user_id", &subscriptionUserIds).Error; err != nil {
		return err
	}
	var paygUserIds []int
	if err := db.Model(&PaygOrder{}).
		Distinct("user_id").
		Where("status = ? AND pay_method = ?", PaygOrderStatusSuccess, PaygPayMethodEpay).
		Pluck("user_id", &paygUserIds).Error; err != nil {
		return err
	}
	var payRequestUserIds []int
	if err := db.Model(&PayRequestOrder{}).
		Distinct("user_id").
		Where("status = ? AND pay_method = ?", PayRequestOrderStatusSuccess, PayRequestPayMethodEpay).
		Pluck("user_id", &payRequestUserIds).Error; err != nil {
		return err
	}
	var payTokenUserIds []int
	if err := db.Model(&PayTokenOrder{}).
		Distinct("user_id").
		Where("status = ? AND pay_method = ?", PayTokenOrderStatusSuccess, PayTokenPayMethodEpay).
		Pluck("user_id", &payTokenUserIds).Error; err != nil {
		return err
	}

	var redemptionUserIds []int
	if err := db.Model(&Redemption{}).
		Distinct("used_user_id").
		Where("status = ? AND price_fen > 0", common.RedemptionCodeStatusUsed).
		Pluck("used_user_id", &redemptionUserIds).Error; err != nil {
		return err
	}

	userIdSet := make(map[int]struct{}, len(subscriptionUserIds)+len(paygUserIds)+len(payRequestUserIds)+len(payTokenUserIds)+len(redemptionUserIds))
	for _, ids := range [][]int{subscriptionUserIds, paygUserIds, payRequestUserIds, payTokenUserIds, redemptionUserIds} {
		for _, userId := range ids {
			if userId <= 0 {
				continue
			}
			userIdSet[userId] = struct{}{}
		}
	}

	userIds := make([]int, 0, len(userIdSet))
	for userId := range userIdSet {
		userIds = append(userIds, userId)
	}
	sort.Ints(userIds)

	return db.Transaction(func(tx *gorm.DB) error {
		for _, userId := range userIds {
			first, err := findUserFirstPaidEvent(tx, userId)
			if err != nil {
				return err
			}
			if first.kind == "" || first.id <= 0 {
				continue
			}

			if err := tx.Model(&SubscriptionOrder{}).
				Where("user_id = ? AND status = ? AND pay_method = ?", userId, SubscriptionOrderStatusSuccess, SubscriptionPayMethodEpay).
				Update("is_first_purchase", false).Error; err != nil {
				return err
			}
			if err := tx.Model(&PaygOrder{}).
				Where("user_id = ? AND status = ? AND pay_method = ?", userId, PaygOrderStatusSuccess, PaygPayMethodEpay).
				Update("is_first_purchase", false).Error; err != nil {
				return err
			}
			if err := tx.Model(&PayRequestOrder{}).
				Where("user_id = ? AND status = ? AND pay_method = ?", userId, PayRequestOrderStatusSuccess, PayRequestPayMethodEpay).
				Update("is_first_purchase", false).Error; err != nil {
				return err
			}
			if err := tx.Model(&PayTokenOrder{}).
				Where("user_id = ? AND status = ? AND pay_method = ?", userId, PayTokenOrderStatusSuccess, PayTokenPayMethodEpay).
				Update("is_first_purchase", false).Error; err != nil {
				return err
			}
			if err := tx.Model(&Redemption{}).
				Where("used_user_id = ? AND status = ? AND price_fen > 0", userId, common.RedemptionCodeStatusUsed).
				Update("is_first_purchase", false).Error; err != nil {
				return err
			}

			switch first.kind {
			case paidEventKindSubscription:
				if err := tx.Model(&SubscriptionOrder{}).
					Where("id = ?", first.id).
					Update("is_first_purchase", true).Error; err != nil {
					return err
				}
			case paidEventKindPayg:
				if err := tx.Model(&PaygOrder{}).
					Where("id = ?", first.id).
					Update("is_first_purchase", true).Error; err != nil {
					return err
				}
			case paidEventKindPayRequest:
				if err := tx.Model(&PayRequestOrder{}).
					Where("id = ?", first.id).
					Update("is_first_purchase", true).Error; err != nil {
					return err
				}
			case paidEventKindPayToken:
				if err := tx.Model(&PayTokenOrder{}).
					Where("id = ?", first.id).
					Update("is_first_purchase", true).Error; err != nil {
					return err
				}
			case paidEventKindRedemption:
				if err := tx.Model(&Redemption{}).
					Where("id = ?", first.id).
					Update("is_first_purchase", true).Error; err != nil {
					return err
				}
			default:
				return fmt.Errorf("unknown paid event kind: %s", first.kind)
			}
		}

		saveMarker := Option{Key: optionKeyBackfillInvitationIsFirstPurchasePaidEvents}
		if err := tx.FirstOrCreate(&saveMarker, Option{Key: optionKeyBackfillInvitationIsFirstPurchasePaidEvents}).Error; err != nil {
			return err
		}
		saveMarker.Value = "true"
		return tx.Save(&saveMarker).Error
	})
}

func findUserFirstPaidEvent(tx *gorm.DB, userId int) (paidEvent, error) {
	if tx == nil {
		return paidEvent{}, errors.New("tx 为空")
	}
	if userId <= 0 {
		return paidEvent{}, errors.New("userId 无效")
	}

	var firstOrder SubscriptionOrder
	orderErr := tx.Model(&SubscriptionOrder{}).
		Select("id", "paid_at").
		Where("user_id = ? AND status = ? AND pay_method = ?", userId, SubscriptionOrderStatusSuccess, SubscriptionPayMethodEpay).
		Order("paid_at ASC, id ASC").
		First(&firstOrder).Error
	if orderErr != nil && !errors.Is(orderErr, gorm.ErrRecordNotFound) {
		return paidEvent{}, orderErr
	}

	var firstPaygOrder PaygOrder
	paygErr := tx.Model(&PaygOrder{}).
		Select("id", "paid_at").
		Where("user_id = ? AND status = ? AND pay_method = ?", userId, PaygOrderStatusSuccess, PaygPayMethodEpay).
		Order("paid_at ASC, id ASC").
		First(&firstPaygOrder).Error
	if paygErr != nil && !errors.Is(paygErr, gorm.ErrRecordNotFound) {
		return paidEvent{}, paygErr
	}

	var firstPayRequestOrder PayRequestOrder
	payRequestErr := tx.Model(&PayRequestOrder{}).
		Select("id", "paid_at").
		Where("user_id = ? AND status = ? AND pay_method = ?", userId, PayRequestOrderStatusSuccess, PayRequestPayMethodEpay).
		Order("paid_at ASC, id ASC").
		First(&firstPayRequestOrder).Error
	if payRequestErr != nil && !errors.Is(payRequestErr, gorm.ErrRecordNotFound) {
		return paidEvent{}, payRequestErr
	}

	var firstPayTokenOrder PayTokenOrder
	payTokenErr := tx.Model(&PayTokenOrder{}).
		Select("id", "paid_at").
		Where("user_id = ? AND status = ? AND pay_method = ?", userId, PayTokenOrderStatusSuccess, PayTokenPayMethodEpay).
		Order("paid_at ASC, id ASC").
		First(&firstPayTokenOrder).Error
	if payTokenErr != nil && !errors.Is(payTokenErr, gorm.ErrRecordNotFound) {
		return paidEvent{}, payTokenErr
	}

	var firstRedemption Redemption
	redemptionErr := tx.Model(&Redemption{}).
		Select("id", "redeemed_time").
		Where("used_user_id = ? AND status = ? AND price_fen > 0", userId, common.RedemptionCodeStatusUsed).
		Order("redeemed_time ASC, id ASC").
		First(&firstRedemption).Error
	if redemptionErr != nil && !errors.Is(redemptionErr, gorm.ErrRecordNotFound) {
		return paidEvent{}, redemptionErr
	}

	var orderEvent *paidEvent
	if orderErr == nil {
		if firstOrder.PaidAt <= 0 {
			return paidEvent{}, fmt.Errorf("subscription order paid_at 异常: user_id=%d order_id=%d paid_at=%d", userId, firstOrder.Id, firstOrder.PaidAt)
		}
		e := paidEvent{kind: paidEventKindSubscription, id: firstOrder.Id, ts: firstOrder.PaidAt}
		orderEvent = &e
	}
	if paygErr == nil {
		if firstPaygOrder.PaidAt <= 0 {
			return paidEvent{}, fmt.Errorf("payg order paid_at 异常: user_id=%d order_id=%d paid_at=%d", userId, firstPaygOrder.Id, firstPaygOrder.PaidAt)
		}
		e := paidEvent{kind: paidEventKindPayg, id: firstPaygOrder.Id, ts: firstPaygOrder.PaidAt}
		orderEvent = pickEarlierPaidEvent(orderEvent, &e)
	}
	if payRequestErr == nil {
		if firstPayRequestOrder.PaidAt <= 0 {
			return paidEvent{}, fmt.Errorf("pay_request order paid_at 异常: user_id=%d order_id=%d paid_at=%d", userId, firstPayRequestOrder.Id, firstPayRequestOrder.PaidAt)
		}
		e := paidEvent{kind: paidEventKindPayRequest, id: firstPayRequestOrder.Id, ts: firstPayRequestOrder.PaidAt}
		orderEvent = pickEarlierPaidEvent(orderEvent, &e)
	}
	if payTokenErr == nil {
		if firstPayTokenOrder.PaidAt <= 0 {
			return paidEvent{}, fmt.Errorf("pay_token order paid_at 异常: user_id=%d order_id=%d paid_at=%d", userId, firstPayTokenOrder.Id, firstPayTokenOrder.PaidAt)
		}
		e := paidEvent{kind: paidEventKindPayToken, id: firstPayTokenOrder.Id, ts: firstPayTokenOrder.PaidAt}
		orderEvent = pickEarlierPaidEvent(orderEvent, &e)
	}

	var redemptionEvent *paidEvent
	if redemptionErr == nil {
		if firstRedemption.RedeemedTime <= 0 {
			return paidEvent{}, fmt.Errorf("redemption redeemed_time 异常: user_id=%d redemption_id=%d redeemed_time=%d", userId, firstRedemption.Id, firstRedemption.RedeemedTime)
		}
		e := paidEvent{kind: paidEventKindRedemption, id: firstRedemption.Id, ts: firstRedemption.RedeemedTime}
		redemptionEvent = &e
	}

	if orderEvent == nil && redemptionEvent == nil {
		return paidEvent{}, nil
	}
	if orderEvent != nil && redemptionEvent == nil {
		return *orderEvent, nil
	}
	if orderEvent == nil && redemptionEvent != nil {
		return *redemptionEvent, nil
	}

	if orderEvent.ts < redemptionEvent.ts {
		return *orderEvent, nil
	}
	if redemptionEvent.ts < orderEvent.ts {
		return *redemptionEvent, nil
	}
	return *orderEvent, nil
}

func pickEarlierPaidEvent(current *paidEvent, candidate *paidEvent) *paidEvent {
	if candidate == nil {
		return current
	}
	if current == nil {
		return candidate
	}
	if candidate.ts < current.ts {
		return candidate
	}
	if candidate.ts == current.ts && candidate.id < current.id {
		return candidate
	}
	return current
}
