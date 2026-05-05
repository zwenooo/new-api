package model

import (
	"errors"
	"strings"
)

type AdminOrdersListQuery struct {
	Keyword        string
	Status         string
	PayMethod      string
	UserId         int
	StartTimestamp int64
	EndTimestamp   int64
}

type AdminSubscriptionOrderListItem struct {
	Id int `json:"id"`

	UserId   int    `json:"user_id"`
	Username string `json:"username"`
	Email    string `json:"email"`

	TradeNo   string `json:"trade_no"`
	Status    string `json:"status"`
	PayMethod string `json:"pay_method"`
	ApplyMode string `json:"apply_mode"`
	AmountFen int64  `json:"amount_fen"`

	PlanId   int    `json:"plan_id"`
	PresetId int    `json:"preset_id"`
	PlanName string `json:"plan_name"`

	CreatedAt  int64 `json:"created_at"`
	PaidAt     int64 `json:"paid_at"`
	FinishedAt int64 `json:"finished_at"`

	MembershipStartAt  int64 `json:"membership_start_at"`
	MembershipExpireAt int64 `json:"membership_expire_at"`
}

func ListAdminSubscriptionOrders(query AdminOrdersListQuery, startIdx int, pageSize int) ([]*AdminSubscriptionOrderListItem, int64, error) {
	if startIdx < 0 {
		return nil, 0, errors.New("startIdx 无效")
	}
	if pageSize <= 0 {
		return nil, 0, errors.New("pageSize 无效")
	}

	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	q := tx.Table("subscription_orders o").
		Select(`
o.id,
o.user_id,
u.username,
u.email,
o.trade_no,
o.status,
o.pay_method,
o.apply_mode,
o.amount_fen,
o.plan_id,
o.preset_id,
COALESCE(p.name, rp.name) AS plan_name,
o.created_at,
o.paid_at,
o.finished_at,
o.membership_start_at,
o.membership_expire_at
`).
		Joins("JOIN users u ON u.id = o.user_id").
		Joins("LEFT JOIN subscription_plans p ON p.id = o.plan_id").
		Joins("LEFT JOIN redemption_presets rp ON rp.id = o.preset_id")

	if query.UserId > 0 {
		q = q.Where("o.user_id = ?", query.UserId)
	}
	if query.Status != "" {
		q = q.Where("o.status = ?", query.Status)
	}
	if query.PayMethod != "" {
		q = q.Where("o.pay_method = ?", query.PayMethod)
	}
	if query.StartTimestamp > 0 {
		q = q.Where("o.created_at >= ?", query.StartTimestamp)
	}
	if query.EndTimestamp > 0 {
		q = q.Where("o.created_at <= ?", query.EndTimestamp)
	}
	if keyword := strings.TrimSpace(query.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		q = q.Where(
			"(o.trade_no LIKE ? OR u.username LIKE ? OR u.email LIKE ? OR p.name LIKE ? OR rp.name LIKE ?)",
			like, like, like, like, like,
		)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	items := make([]*AdminSubscriptionOrderListItem, 0)
	if err := q.Order("o.created_at DESC, o.id DESC").
		Limit(pageSize).
		Offset(startIdx).
		Scan(&items).Error; err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	if err := tx.Commit().Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

type AdminTopUpOrderListItem struct {
	Id int `json:"id"`

	UserId   int    `json:"user_id"`
	Username string `json:"username"`
	Email    string `json:"email"`

	TradeNo string `json:"trade_no"`
	Status  string `json:"status"`
	Amount  int64  `json:"amount"`
	Money   float64 `json:"money"`

	CreateTime   int64 `json:"create_time"`
	CompleteTime int64 `json:"complete_time"`
}

func ListAdminTopUpOrders(query AdminOrdersListQuery, startIdx int, pageSize int) ([]*AdminTopUpOrderListItem, int64, error) {
	if startIdx < 0 {
		return nil, 0, errors.New("startIdx 无效")
	}
	if pageSize <= 0 {
		return nil, 0, errors.New("pageSize 无效")
	}

	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	q := tx.Table("top_ups t").
		Select(`
t.id,
t.user_id,
u.username,
u.email,
t.trade_no,
t.status,
t.amount,
t.money,
t.create_time,
t.complete_time
`).
		Joins("JOIN users u ON u.id = t.user_id")

	if query.UserId > 0 {
		q = q.Where("t.user_id = ?", query.UserId)
	}
	if query.Status != "" {
		q = q.Where("t.status = ?", query.Status)
	}
	if query.StartTimestamp > 0 {
		q = q.Where("t.create_time >= ?", query.StartTimestamp)
	}
	if query.EndTimestamp > 0 {
		q = q.Where("t.create_time <= ?", query.EndTimestamp)
	}
	if keyword := strings.TrimSpace(query.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		q = q.Where("(t.trade_no LIKE ? OR u.username LIKE ? OR u.email LIKE ?)", like, like, like)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	items := make([]*AdminTopUpOrderListItem, 0)
	if err := q.Order("t.create_time DESC, t.id DESC").
		Limit(pageSize).
		Offset(startIdx).
		Scan(&items).Error; err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	if err := tx.Commit().Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

type AdminPaygOrderListItem struct {
	Id int `json:"id"`

	UserId   int    `json:"user_id"`
	Username string `json:"username"`
	Email    string `json:"email"`

	TradeNo    string `json:"trade_no"`
	Status     string `json:"status"`
	PayMethod  string `json:"pay_method"`
	EpayMethod string `json:"epay_method"`

	AmountFen   int64 `json:"amount_fen"`
	CreditQuota int   `json:"credit_quota"`

	PresetId   int    `json:"preset_id"`
	PresetName string `json:"preset_name"`

	CreatedAt  int64 `json:"created_at"`
	PaidAt     int64 `json:"paid_at"`
	FinishedAt int64 `json:"finished_at"`
}

func ListAdminPaygOrders(query AdminOrdersListQuery, startIdx int, pageSize int) ([]*AdminPaygOrderListItem, int64, error) {
	if startIdx < 0 {
		return nil, 0, errors.New("startIdx 无效")
	}
	if pageSize <= 0 {
		return nil, 0, errors.New("pageSize 无效")
	}

	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	q := tx.Table("payg_orders o").
		Select(`
o.id,
o.user_id,
u.username,
u.email,
o.trade_no,
o.status,
o.pay_method,
o.epay_method,
o.amount_fen,
o.credit_quota,
o.preset_id,
COALESCE(rp.name, '') AS preset_name,
o.created_at,
o.paid_at,
o.finished_at
`).
		Joins("JOIN users u ON u.id = o.user_id").
		Joins("LEFT JOIN redemption_presets rp ON rp.id = o.preset_id")

	if query.UserId > 0 {
		q = q.Where("o.user_id = ?", query.UserId)
	}
	if query.Status != "" {
		q = q.Where("o.status = ?", query.Status)
	}
	if query.PayMethod != "" {
		q = q.Where("o.pay_method = ?", query.PayMethod)
	}
	if query.StartTimestamp > 0 {
		q = q.Where("o.created_at >= ?", query.StartTimestamp)
	}
	if query.EndTimestamp > 0 {
		q = q.Where("o.created_at <= ?", query.EndTimestamp)
	}
	if keyword := strings.TrimSpace(query.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		q = q.Where(
			"(o.trade_no LIKE ? OR u.username LIKE ? OR u.email LIKE ? OR o.epay_method LIKE ? OR rp.name LIKE ?)",
			like, like, like, like, like,
		)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	items := make([]*AdminPaygOrderListItem, 0)
	if err := q.Order("o.created_at DESC, o.id DESC").
		Limit(pageSize).
		Offset(startIdx).
		Scan(&items).Error; err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	if err := tx.Commit().Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

type AdminPayRequestOrderListItem struct {
	Id int `json:"id"`

	UserId   int    `json:"user_id"`
	Username string `json:"username"`
	Email    string `json:"email"`

	TradeNo    string `json:"trade_no"`
	Status     string `json:"status"`
	PayMethod  string `json:"pay_method"`
	EpayMethod string `json:"epay_method"`

	AmountFen       int64 `json:"amount_fen"`
	CreditRequests  int   `json:"credit_requests"`

	CreatedAt  int64 `json:"created_at"`
	PaidAt     int64 `json:"paid_at"`
	FinishedAt int64 `json:"finished_at"`
}

func ListAdminPayRequestOrders(query AdminOrdersListQuery, startIdx int, pageSize int) ([]*AdminPayRequestOrderListItem, int64, error) {
	if startIdx < 0 {
		return nil, 0, errors.New("startIdx 无效")
	}
	if pageSize <= 0 {
		return nil, 0, errors.New("pageSize 无效")
	}

	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	q := tx.Table("pay_request_orders o").
		Select(`
o.id,
o.user_id,
u.username,
u.email,
o.trade_no,
o.status,
o.pay_method,
o.epay_method,
o.amount_fen,
o.credit_requests,
o.created_at,
o.paid_at,
o.finished_at
`).
		Joins("JOIN users u ON u.id = o.user_id")

	if query.UserId > 0 {
		q = q.Where("o.user_id = ?", query.UserId)
	}
	if query.Status != "" {
		q = q.Where("o.status = ?", query.Status)
	}
	if query.PayMethod != "" {
		q = q.Where("o.pay_method = ?", query.PayMethod)
	}
	if query.StartTimestamp > 0 {
		q = q.Where("o.created_at >= ?", query.StartTimestamp)
	}
	if query.EndTimestamp > 0 {
		q = q.Where("o.created_at <= ?", query.EndTimestamp)
	}
	if keyword := strings.TrimSpace(query.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		q = q.Where(
			"(o.trade_no LIKE ? OR u.username LIKE ? OR u.email LIKE ? OR o.epay_method LIKE ?)",
			like, like, like, like,
		)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	items := make([]*AdminPayRequestOrderListItem, 0)
	if err := q.Order("o.created_at DESC, o.id DESC").
		Limit(pageSize).
		Offset(startIdx).
		Scan(&items).Error; err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	if err := tx.Commit().Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

type AdminPayTokenOrderListItem struct {
	Id int `json:"id"`

	UserId   int    `json:"user_id"`
	Username string `json:"username"`
	Email    string `json:"email"`

	TradeNo    string `json:"trade_no"`
	Status     string `json:"status"`
	PayMethod  string `json:"pay_method"`
	EpayMethod string `json:"epay_method"`

	AmountFen    int64 `json:"amount_fen"`
	CreditTokens int   `json:"credit_tokens"`

	CreatedAt  int64 `json:"created_at"`
	PaidAt     int64 `json:"paid_at"`
	FinishedAt int64 `json:"finished_at"`
}

func ListAdminPayTokenOrders(query AdminOrdersListQuery, startIdx int, pageSize int) ([]*AdminPayTokenOrderListItem, int64, error) {
	if startIdx < 0 {
		return nil, 0, errors.New("startIdx 无效")
	}
	if pageSize <= 0 {
		return nil, 0, errors.New("pageSize 无效")
	}

	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	q := tx.Table("pay_token_orders o").
		Select(`
o.id,
o.user_id,
u.username,
u.email,
o.trade_no,
o.status,
o.pay_method,
o.epay_method,
o.amount_fen,
o.credit_tokens,
o.created_at,
o.paid_at,
o.finished_at
`).
		Joins("JOIN users u ON u.id = o.user_id")

	if query.UserId > 0 {
		q = q.Where("o.user_id = ?", query.UserId)
	}
	if query.Status != "" {
		q = q.Where("o.status = ?", query.Status)
	}
	if query.PayMethod != "" {
		q = q.Where("o.pay_method = ?", query.PayMethod)
	}
	if query.StartTimestamp > 0 {
		q = q.Where("o.created_at >= ?", query.StartTimestamp)
	}
	if query.EndTimestamp > 0 {
		q = q.Where("o.created_at <= ?", query.EndTimestamp)
	}
	if keyword := strings.TrimSpace(query.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		q = q.Where(
			"(o.trade_no LIKE ? OR u.username LIKE ? OR u.email LIKE ? OR o.epay_method LIKE ?)",
			like, like, like, like,
		)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	items := make([]*AdminPayTokenOrderListItem, 0)
	if err := q.Order("o.created_at DESC, o.id DESC").
		Limit(pageSize).
		Offset(startIdx).
		Scan(&items).Error; err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	if err := tx.Commit().Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}
