package model

import (
	"errors"

	"gorm.io/gorm"
)

const (
	PayRequestOrderStatusPending = "pending"
	PayRequestOrderStatusSuccess = "success"
	PayRequestOrderStatusFailed  = "failed"
)

const (
	PayRequestPayMethodEpay    = "epay"
	PayRequestPayMethodBalance = "balance"
)

// PayRequestOrder represents a top-up order that converts RMB (fen) to non-expiring request credits.
type PayRequestOrder struct {
	Id int `json:"id" gorm:"primaryKey"`

	UserId int `json:"user_id" gorm:"index;not null"`

	TradeNo    string `json:"trade_no" gorm:"uniqueIndex;type:varchar(255);not null"`
	PayMethod  string `json:"pay_method" gorm:"type:varchar(32);index;not null"`
	EpayMethod string `json:"epay_method" gorm:"type:varchar(32);default:'';column:epay_method"`

	// AmountFen is the paid amount in RMB fen (分).
	AmountFen int64 `json:"amount_fen" gorm:"type:bigint;not null"`
	// CreditRequests is the request credits to be credited after payment success (count).
	CreditRequests int `json:"credit_requests" gorm:"type:int;default:0;column:credit_requests"`

	// Product fields for product-based pay-request system
	ProductId       int       `json:"product_id" gorm:"type:int;default:0;column:product_id;index"`
	ProductName     string    `json:"product_name" gorm:"type:varchar(64);default:'';column:product_name"`
	AllowedGroupIds JSONValue `json:"allowed_group_ids" gorm:"type:json;column:allowed_group_ids"`

	Status     string `json:"status" gorm:"type:varchar(32);index;not null"`
	PaidAt     int64  `json:"paid_at" gorm:"bigint;default:0"`
	CreatedAt  int64  `json:"created_at" gorm:"bigint;autoCreateTime"`
	FinishedAt int64  `json:"finished_at" gorm:"bigint;default:0"`

	IsFirstPurchase   bool  `json:"is_first_purchase" gorm:"type:boolean;default:false"`
	InviterId         int   `json:"inviter_id" gorm:"type:int;default:0;index"`
	CommissionPercent int   `json:"commission_percent" gorm:"type:int;default:0"`
	CommissionFen     int64 `json:"commission_fen" gorm:"type:bigint;default:0"`
}

func (o *PayRequestOrder) Insert(tx *gorm.DB) error {
	if o == nil {
		return errors.New("order 为空")
	}
	if tx == nil {
		tx = DB
	}
	if o.UserId <= 0 {
		return errors.New("user_id 无效")
	}
	if o.TradeNo == "" {
		return errors.New("trade_no 不能为空")
	}
	if o.PayMethod != PayRequestPayMethodEpay && o.PayMethod != PayRequestPayMethodBalance {
		return errors.New("pay_method 无效")
	}
	if o.AmountFen <= 0 {
		return errors.New("amount_fen 必须大于0")
	}
	if o.Status == "" {
		return errors.New("status 不能为空")
	}
	return tx.Create(o).Error
}
