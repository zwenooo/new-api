package model

import (
	"errors"

	"gorm.io/gorm"
)

const (
	SubscriptionOrderStatusPending = "pending"
	SubscriptionOrderStatusSuccess = "success"
	SubscriptionOrderStatusFailed  = "failed"
)

const (
	SubscriptionPayMethodEpay    = "epay"
	SubscriptionPayMethodBalance = "balance"
)

const (
	SubscriptionApplyModeStack = "stack"
	SubscriptionApplyModeDefer = "defer"
)

type SubscriptionOrder struct {
	Id int `json:"id" gorm:"primaryKey"`

	UserId           int `json:"user_id" gorm:"index;not null"`
	PlanId           int `json:"plan_id" gorm:"index;not null"`
	PresetId         int `json:"preset_id" gorm:"type:int;default:0;index"`
	PresetRevisionId int `json:"preset_revision_id" gorm:"type:int;default:0;index;column:preset_revision_id"`

	TradeNo    string `json:"trade_no" gorm:"uniqueIndex;type:varchar(255);not null"`
	PayMethod  string `json:"pay_method" gorm:"type:varchar(32);index;not null"`
	ApplyMode  string `json:"apply_mode" gorm:"type:varchar(16);not null"`
	Quantity   int    `json:"quantity" gorm:"type:int;not null;default:1"`
	AmountFen  int64  `json:"amount_fen" gorm:"type:bigint;not null"`
	Status     string `json:"status" gorm:"type:varchar(32);index;not null"`
	PaidAt     int64  `json:"paid_at" gorm:"bigint;default:0"`
	CreatedAt  int64  `json:"created_at" gorm:"bigint;autoCreateTime"`
	FinishedAt int64  `json:"finished_at" gorm:"bigint;default:0"`

	MembershipStartAt  int64 `json:"membership_start_at" gorm:"bigint;default:0"`
	MembershipExpireAt int64 `json:"membership_expire_at" gorm:"bigint;default:0"`

	IsFirstPurchase   bool  `json:"is_first_purchase" gorm:"type:boolean;default:false"`
	InviterId         int   `json:"inviter_id" gorm:"type:int;default:0;index"`
	CommissionPercent int   `json:"commission_percent" gorm:"type:int;default:0"`
	CommissionFen     int64 `json:"commission_fen" gorm:"type:bigint;default:0"`
}

func (o *SubscriptionOrder) Insert(tx *gorm.DB) error {
	if o == nil {
		return errors.New("order 为空")
	}
	if tx == nil {
		tx = DB
	}
	if o.UserId <= 0 {
		return errors.New("user_id 无效")
	}
	// plan_id 与 preset_id 必须且只能有一个
	if (o.PlanId <= 0) == (o.PresetId <= 0) {
		return errors.New("plan_id/preset_id 无效")
	}
	if o.TradeNo == "" {
		return errors.New("trade_no 不能为空")
	}
	if o.PayMethod != SubscriptionPayMethodEpay && o.PayMethod != SubscriptionPayMethodBalance {
		return errors.New("pay_method 无效")
	}
	if o.ApplyMode != SubscriptionApplyModeStack && o.ApplyMode != SubscriptionApplyModeDefer {
		return errors.New("apply_mode 无效")
	}
	if o.Quantity <= 0 {
		return errors.New("quantity 无效")
	}
	if o.AmountFen < 0 {
		return errors.New("amount_fen 不能小于0")
	}
	if o.Status == "" {
		return errors.New("status 不能为空")
	}
	return tx.Create(o).Error
}

func (o *SubscriptionOrder) Update(tx *gorm.DB) error {
	if o == nil {
		return errors.New("order 为空")
	}
	if o.Id <= 0 {
		return errors.New("order id 无效")
	}
	if tx == nil {
		tx = DB
	}
	return tx.Save(o).Error
}

func GetSubscriptionOrderByTradeNo(tradeNo string) (*SubscriptionOrder, error) {
	if tradeNo == "" {
		return nil, errors.New("tradeNo 为空")
	}
	var order SubscriptionOrder
	if err := DB.Where("trade_no = ?", tradeNo).First(&order).Error; err != nil {
		return nil, err
	}
	return &order, nil
}

func CountUserSuccessfulSubscriptionOrders(tx *gorm.DB, userId int) (int64, error) {
	if userId <= 0 {
		return 0, errors.New("userId 无效")
	}
	if tx == nil {
		tx = DB
	}
	var count int64
	if err := tx.Model(&SubscriptionOrder{}).Where("user_id = ? AND status = ?", userId, SubscriptionOrderStatusSuccess).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func CountUserSuccessfulCommissionableSubscriptionOrders(tx *gorm.DB, userId int) (int64, error) {
	if userId <= 0 {
		return 0, errors.New("userId 无效")
	}
	if tx == nil {
		tx = DB
	}
	var count int64
	if err := tx.Model(&SubscriptionOrder{}).
		Where("user_id = ? AND status = ? AND pay_method = ?", userId, SubscriptionOrderStatusSuccess, SubscriptionPayMethodEpay).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func CountUserSubscriptionOrdersByPresetStatus(tx *gorm.DB, userId int, presetId int, status string) (int64, error) {
	if userId <= 0 {
		return 0, errors.New("userId 无效")
	}
	if presetId <= 0 {
		return 0, errors.New("presetId 无效")
	}
	if status == "" {
		return 0, errors.New("status 不能为空")
	}
	if tx == nil {
		tx = DB
	}
	var count int64
	if err := tx.Model(&SubscriptionOrder{}).
		Where("user_id = ? AND preset_id = ? AND status = ?", userId, presetId, status).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func GetLatestUserSubscriptionOrderByPresetStatus(tx *gorm.DB, userId int, presetId int, status string) (*SubscriptionOrder, error) {
	if userId <= 0 {
		return nil, errors.New("userId 无效")
	}
	if presetId <= 0 {
		return nil, errors.New("presetId 无效")
	}
	if status == "" {
		return nil, errors.New("status 不能为空")
	}
	if tx == nil {
		tx = DB
	}
	var order SubscriptionOrder
	if err := tx.Where("user_id = ? AND preset_id = ? AND status = ?", userId, presetId, status).
		Order("id DESC").
		First(&order).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &order, nil
}

func SumUserSubscriptionOrderQuantityByPresetStatus(tx *gorm.DB, userId int, presetId int, status string) (int64, error) {
	if userId <= 0 {
		return 0, errors.New("userId 无效")
	}
	if presetId <= 0 {
		return 0, errors.New("presetId 无效")
	}
	if status == "" {
		return 0, errors.New("status 不能为空")
	}
	if tx == nil {
		tx = DB
	}
	var total int64
	if err := tx.Model(&SubscriptionOrder{}).
		Select("COALESCE(SUM(quantity),0)").
		Where("user_id = ? AND preset_id = ? AND status = ?", userId, presetId, status).
		Scan(&total).Error; err != nil {
		return 0, err
	}
	return total, nil
}
