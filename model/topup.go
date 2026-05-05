package model

import (
	"errors"
	"fmt"
	"one-api/common"
	"one-api/logger"
	"strings"

	"gorm.io/gorm"
)

type TopUp struct {
	Id               int     `json:"id"`
	UserId           int     `json:"user_id" gorm:"index"`
	Amount           int64   `json:"amount"`
	CreditQuota      int64   `json:"credit_quota"`
	Money            float64 `json:"money"`
	PaymentAmountFen int64   `json:"payment_amount_fen"`
	PaymentCurrency  string  `json:"payment_currency" gorm:"type:varchar(16);default:''"`
	TradeNo          string  `json:"trade_no" gorm:"unique;type:varchar(255);index"`
	CreateTime       int64   `json:"create_time"`
	CompleteTime     int64   `json:"complete_time"`
	Status           string  `json:"status"`
}

func (topUp *TopUp) Insert() error {
	var err error
	err = DB.Create(topUp).Error
	return err
}

func (topUp *TopUp) Update() error {
	var err error
	err = DB.Save(topUp).Error
	return err
}

func GetTopUpById(id int) *TopUp {
	var topUp *TopUp
	var err error
	err = DB.Where("id = ?", id).First(&topUp).Error
	if err != nil {
		return nil
	}
	return topUp
}

func GetTopUpByTradeNo(tradeNo string) *TopUp {
	var topUp *TopUp
	var err error
	err = DB.Where("trade_no = ?", tradeNo).First(&topUp).Error
	if err != nil {
		return nil
	}
	return topUp
}

func Recharge(referenceId string, customerId string, paidFen int64, paidCurrency string) (err error) {
	if referenceId == "" {
		return errors.New("未提供支付单号")
	}
	if paidFen <= 0 {
		return errors.New("未提供有效支付金额")
	}

	var quota float64
	topUp := &TopUp{}
	paidCurrency = strings.ToLower(strings.TrimSpace(paidCurrency))

	err = DB.Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).Where("trade_no = ?", referenceId).First(topUp).Error; err != nil {
			return errors.New("充值订单不存在")
		}

		if topUp.Status != common.TopUpStatusPending {
			return errors.New("充值订单状态错误")
		}

		if topUp.CreditQuota > 0 || topUp.PaymentAmountFen > 0 {
			expectedCurrency := strings.ToLower(strings.TrimSpace(topUp.PaymentCurrency))
			if expectedCurrency == "" {
				expectedCurrency = "cny"
			}
			if paidCurrency != "" && paidCurrency != expectedCurrency {
				return fmt.Errorf("支付货币不一致：paid=%s expected=%s", strings.ToUpper(paidCurrency), strings.ToUpper(expectedCurrency))
			}
			if topUp.PaymentAmountFen <= 0 {
				return errors.New("充值订单缺少支付金额元数据")
			}
			if topUp.PaymentAmountFen != paidFen {
				return fmt.Errorf("支付金额不一致：paid=%d expected=%d", paidFen, topUp.PaymentAmountFen)
			}
			if topUp.CreditQuota <= 0 {
				return errors.New("充值订单缺少到账额度元数据")
			}
			quota = float64(topUp.CreditQuota)
		} else {
			if paidCurrency != "" && paidCurrency != "cny" {
				return fmt.Errorf("暂不支持按 %s 计入充值统计", strings.ToUpper(paidCurrency))
			}
			if topUp.Money <= 0 {
				return errors.New("充值订单金额无效")
			}
			quota = topUp.Money * common.QuotaPerUnit
		}

		topUp.Money = float64(paidFen) / 100
		topUp.PaymentAmountFen = paidFen
		if paidCurrency != "" {
			topUp.PaymentCurrency = paidCurrency
		}
		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		if err := tx.Save(topUp).Error; err != nil {
			return err
		}

		if quota <= 0 {
			return errors.New("充值额度无效")
		}
		if err := tx.Model(&User{}).Where("id = ?", topUp.UserId).Updates(map[string]interface{}{"stripe_customer": customerId, "quota": gorm.Expr("quota + ?", quota)}).Error; err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return errors.New("充值失败，" + err.Error())
	}

	RecordLog(topUp.UserId, LogTypeTopup, fmt.Sprintf("使用在线充值成功，充值金额: %v，支付金额：%.2f", logger.FormatQuota(int(quota)), topUp.Money))

	return nil
}
