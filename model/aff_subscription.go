package model

import (
	"errors"
	"one-api/common"
)

type AffSubscriptionRecord struct {
	OrderId           int    `json:"order_id"`
	UserId            int    `json:"user_id"`
	Email             string `json:"email"`
	PlanId            int    `json:"plan_id"`
	PlanName          string `json:"plan_name"`
	AmountFen         int64  `json:"amount_fen"`
	CommissionPercent int    `json:"commission_percent"`
	CommissionFen     int64  `json:"commission_fen"`
	IsFirstPurchase   int    `json:"is_first_purchase"`
	PaidAt            int64  `json:"paid_at"`
	Type              string `json:"type"`
}

func CountInvitedPaidUsersBySubscription(inviterId int) (int64, error) {
	if inviterId <= 0 {
		return 0, errors.New("inviterId 无效")
	}
	var count int64
	query := `
SELECT COUNT(*) AS count
FROM (
  SELECT o.user_id AS uid
  FROM subscription_orders o
  WHERE o.inviter_id = ? AND o.status = ? AND o.pay_method = ?
  UNION
  SELECT o.user_id AS uid
  FROM payg_orders o
  WHERE o.inviter_id = ? AND o.status = ? AND o.pay_method = ?
  UNION
  SELECT o.user_id AS uid
  FROM pay_request_orders o
  WHERE o.inviter_id = ? AND o.status = ? AND o.pay_method = ?
  UNION
  SELECT o.user_id AS uid
  FROM pay_token_orders o
  WHERE o.inviter_id = ? AND o.status = ? AND o.pay_method = ?
  UNION
  SELECT r.used_user_id AS uid
  FROM redemptions r
  WHERE r.inviter_id = ? AND r.status = ?
) t`
	if err := DB.Raw(query,
		inviterId, SubscriptionOrderStatusSuccess, SubscriptionPayMethodEpay,
		inviterId, PaygOrderStatusSuccess, PaygPayMethodEpay,
		inviterId, PayRequestOrderStatusSuccess, PayRequestPayMethodEpay,
		inviterId, PayTokenOrderStatusSuccess, PayTokenPayMethodEpay,
		inviterId, common.RedemptionCodeStatusUsed,
	).Scan(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func ListInvitationSubscriptionRecords(inviterId int, page int, pageSize int) ([]*AffSubscriptionRecord, int64, error) {
	if inviterId <= 0 {
		return nil, 0, errors.New("inviterId 无效")
	}
	if page <= 0 {
		return nil, 0, errors.New("page 无效")
	}
	if pageSize <= 0 {
		return nil, 0, errors.New("pageSize 无效")
	}

	records := make([]*AffSubscriptionRecord, 0)
	var totalSubscription int64
	if err := DB.Model(&SubscriptionOrder{}).
		Where("inviter_id = ? AND status = ? AND pay_method = ?", inviterId, SubscriptionOrderStatusSuccess, SubscriptionPayMethodEpay).
		Count(&totalSubscription).Error; err != nil {
		return nil, 0, err
	}
	var totalPayg int64
	if err := DB.Model(&PaygOrder{}).
		Where("inviter_id = ? AND status = ? AND pay_method = ?", inviterId, PaygOrderStatusSuccess, PaygPayMethodEpay).
		Count(&totalPayg).Error; err != nil {
		return nil, 0, err
	}
	var totalPayRequest int64
	if err := DB.Model(&PayRequestOrder{}).
		Where("inviter_id = ? AND status = ? AND pay_method = ?", inviterId, PayRequestOrderStatusSuccess, PayRequestPayMethodEpay).
		Count(&totalPayRequest).Error; err != nil {
		return nil, 0, err
	}
	var totalPayToken int64
	if err := DB.Model(&PayTokenOrder{}).
		Where("inviter_id = ? AND status = ? AND pay_method = ?", inviterId, PayTokenOrderStatusSuccess, PayTokenPayMethodEpay).
		Count(&totalPayToken).Error; err != nil {
		return nil, 0, err
	}
	var totalRedemption int64
	if err := DB.Model(&Redemption{}).
		Where("inviter_id = ? AND status = ?", inviterId, common.RedemptionCodeStatusUsed).
		Count(&totalRedemption).Error; err != nil {
		return nil, 0, err
	}
	total := totalSubscription + totalPayg + totalPayRequest + totalPayToken + totalRedemption

	query := `
SELECT *
FROM (
  SELECT
    o.id AS order_id,
    o.user_id AS user_id,
    u.email AS email,
    o.plan_id AS plan_id,
    COALESCE(p.name, rp.name) AS plan_name,
    o.amount_fen AS amount_fen,
    o.commission_percent AS commission_percent,
    o.commission_fen AS commission_fen,
    CASE WHEN o.is_first_purchase THEN 1 ELSE 0 END AS is_first_purchase,
    o.paid_at AS paid_at,
    'subscription' AS type
	  FROM subscription_orders o
	  JOIN users u ON u.id = o.user_id
	  LEFT JOIN subscription_plans p ON p.id = o.plan_id
	  LEFT JOIN redemption_presets rp ON rp.id = o.preset_id
	  WHERE o.inviter_id = ? AND o.status = ? AND o.pay_method = ?

	  UNION ALL

	  SELECT
	    o.id AS order_id,
	    o.user_id AS user_id,
	    u.email AS email,
	    o.product_id AS plan_id,
	    COALESCE(NULLIF(o.product_name, ''), 'PAYG') AS plan_name,
	    o.amount_fen AS amount_fen,
	    o.commission_percent AS commission_percent,
	    o.commission_fen AS commission_fen,
	    CASE WHEN o.is_first_purchase THEN 1 ELSE 0 END AS is_first_purchase,
	    o.paid_at AS paid_at,
	    'payg' AS type
	  FROM payg_orders o
	  JOIN users u ON u.id = o.user_id
	  WHERE o.inviter_id = ? AND o.status = ? AND o.pay_method = ?

	  UNION ALL

	  SELECT
	    o.id AS order_id,
	    o.user_id AS user_id,
	    u.email AS email,
	    o.product_id AS plan_id,
	    COALESCE(NULLIF(o.product_name, ''), 'PAY_REQUEST') AS plan_name,
	    o.amount_fen AS amount_fen,
	    o.commission_percent AS commission_percent,
	    o.commission_fen AS commission_fen,
	    CASE WHEN o.is_first_purchase THEN 1 ELSE 0 END AS is_first_purchase,
	    o.paid_at AS paid_at,
	    'pay_request' AS type
	  FROM pay_request_orders o
	  JOIN users u ON u.id = o.user_id
	  WHERE o.inviter_id = ? AND o.status = ? AND o.pay_method = ?

	  UNION ALL

	  SELECT
	    o.id AS order_id,
	    o.user_id AS user_id,
	    u.email AS email,
	    o.product_id AS plan_id,
	    COALESCE(NULLIF(o.product_name, ''), 'PAY_TOKEN') AS plan_name,
	    o.amount_fen AS amount_fen,
	    o.commission_percent AS commission_percent,
	    o.commission_fen AS commission_fen,
	    CASE WHEN o.is_first_purchase THEN 1 ELSE 0 END AS is_first_purchase,
	    o.paid_at AS paid_at,
	    'pay_token' AS type
	  FROM pay_token_orders o
	  JOIN users u ON u.id = o.user_id
	  WHERE o.inviter_id = ? AND o.status = ? AND o.pay_method = ?

	  UNION ALL

  SELECT
    r.id AS order_id,
    r.used_user_id AS user_id,
    u.email AS email,
    0 AS plan_id,
    r.name AS plan_name,
    r.price_fen AS amount_fen,
    r.commission_percent AS commission_percent,
    r.commission_fen AS commission_fen,
    CASE WHEN r.is_first_purchase THEN 1 ELSE 0 END AS is_first_purchase,
    r.redeemed_time AS paid_at,
    'redemption' AS type
  FROM redemptions r
  JOIN users u ON u.id = r.used_user_id
  WHERE r.inviter_id = ? AND r.status = ?
) t
ORDER BY t.paid_at DESC, t.order_id DESC
	LIMIT ? OFFSET ?`
	if err := DB.Raw(query,
		inviterId, SubscriptionOrderStatusSuccess, SubscriptionPayMethodEpay,
		inviterId, PaygOrderStatusSuccess, PaygPayMethodEpay,
		inviterId, PayRequestOrderStatusSuccess, PayRequestPayMethodEpay,
		inviterId, PayTokenOrderStatusSuccess, PayTokenPayMethodEpay,
		inviterId, common.RedemptionCodeStatusUsed,
		pageSize, (page-1)*pageSize,
	).Scan(&records).Error; err != nil {
		return nil, 0, err
	}

	return records, total, nil
}
