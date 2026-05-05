package model

import (
	"errors"

	"gorm.io/gorm"
)

const (
	PaygOrderStatusPending = "pending"
	PaygOrderStatusSuccess = "success"
	PaygOrderStatusFailed  = "failed"
)

const (
	PaygPayMethodEpay    = "epay"
	PaygPayMethodBalance = "balance"
)

// PaygOrder represents a pay-as-you-go top-up order that converts RMB (fen) to non-expiring quota.
type PaygOrder struct {
	Id int `json:"id" gorm:"primaryKey"`

	UserId int `json:"user_id" gorm:"index;not null"`
	// ProductId references the configured payg product id (payg.products). 0 means legacy/default.
	ProductId       int       `json:"product_id" gorm:"type:int;default:0;index;column:product_id"`
	ProductName     string    `json:"product_name" gorm:"type:varchar(64);default:'';column:product_name"`
	PresetId        int       `json:"preset_id" gorm:"type:int;default:0;index"`
	AllowedGroups   JSONValue `json:"allowed_groups" gorm:"type:json;column:allowed_groups"`
	AllowedGroupIds JSONValue `json:"allowed_group_ids" gorm:"type:json;column:allowed_group_ids"`

	TradeNo    string `json:"trade_no" gorm:"uniqueIndex;type:varchar(255);not null"`
	PayMethod  string `json:"pay_method" gorm:"type:varchar(32);index;not null"`
	EpayMethod string `json:"epay_method" gorm:"type:varchar(32);default:'';column:epay_method"`
	EpayGatewayTradeNo string `json:"epay_gateway_trade_no" gorm:"type:varchar(128);default:'';column:epay_gateway_trade_no"`
	EpayPayURL         string `json:"epay_pay_url" gorm:"type:text;column:epay_pay_url"`
	EpayQRCode         string `json:"epay_qrcode" gorm:"type:text;column:epay_qrcode"`
	EpayImageURL       string `json:"epay_image_url" gorm:"type:text;column:epay_image_url"`

	// AmountFen is the paid amount in RMB fen (分).
	AmountFen int64 `json:"amount_fen" gorm:"type:bigint;not null"`
	// CreditQuota is the quota to be credited after payment success (quota units).
	CreditQuota int `json:"credit_quota" gorm:"type:int;default:0;column:credit_quota"`

	Status     string `json:"status" gorm:"type:varchar(32);index;not null"`
	PaidAt     int64  `json:"paid_at" gorm:"bigint;default:0"`
	CreatedAt  int64  `json:"created_at" gorm:"bigint;autoCreateTime"`
	FinishedAt int64  `json:"finished_at" gorm:"bigint;default:0"`

	IsFirstPurchase   bool  `json:"is_first_purchase" gorm:"type:boolean;default:false"`
	InviterId         int   `json:"inviter_id" gorm:"type:int;default:0;index"`
	CommissionPercent int   `json:"commission_percent" gorm:"type:int;default:0"`
	CommissionFen     int64 `json:"commission_fen" gorm:"type:bigint;default:0"`
}

func (o *PaygOrder) Insert(tx *gorm.DB) error {
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
	if o.PayMethod != PaygPayMethodEpay && o.PayMethod != PaygPayMethodBalance {
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
