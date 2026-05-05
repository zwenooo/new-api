package model

import (
	"errors"
	"one-api/common"

	"gorm.io/gorm"
)

const (
	BalanceRecordTypeAffTransferIn     = "aff_transfer_in"
	BalanceRecordTypeSubscriptionPayOut = "subscription_pay_out"
	BalanceRecordTypePaygPayOut        = "payg_pay_out"
	BalanceRecordTypePayRequestPayOut  = "pay_request_pay_out"
	BalanceRecordTypePayTokenPayOut    = "pay_token_pay_out"
)

type BalanceRecord struct {
	Id               int    `json:"id" gorm:"primaryKey"`
	UserId           int    `json:"user_id" gorm:"index;not null"`
	Type             string `json:"type" gorm:"type:varchar(32);index;not null"`
	DeltaFen         int64  `json:"delta_fen" gorm:"type:bigint;not null"`
	BalanceBeforeFen int64  `json:"balance_before_fen" gorm:"type:bigint;not null"`
	BalanceAfterFen  int64  `json:"balance_after_fen" gorm:"type:bigint;not null"`
	Remark           string `json:"remark" gorm:"type:varchar(255);default:''"`
	CreatedAt        int64  `json:"created_at" gorm:"bigint;autoCreateTime;index"`
}

func CreateBalanceRecord(
	tx *gorm.DB,
	userId int,
	recordType string,
	deltaFen int64,
	balanceBeforeFen int64,
	balanceAfterFen int64,
	remark string,
) error {
	if userId <= 0 {
		return errors.New("user_id 无效")
	}
	if recordType == "" {
		return errors.New("type 不能为空")
	}
	if deltaFen == 0 {
		return errors.New("delta_fen 不能为 0")
	}
	if balanceBeforeFen < 0 {
		return errors.New("balance_before_fen 不能为负数")
	}
	if balanceAfterFen < 0 {
		return errors.New("balance_after_fen 不能为负数")
	}
	if balanceAfterFen != balanceBeforeFen+deltaFen {
		return errors.New("balance 变更记录不一致")
	}

	if tx == nil {
		tx = DB
	}
	if tx == nil {
		return errors.New("DB 未初始化")
	}

	record := &BalanceRecord{
		UserId:           userId,
		Type:             recordType,
		DeltaFen:         deltaFen,
		BalanceBeforeFen: balanceBeforeFen,
		BalanceAfterFen:  balanceAfterFen,
		Remark:           remark,
		CreatedAt:        common.GetTimestamp(),
	}
	return tx.Create(record).Error
}

func ListUserBalanceRecords(userId int, page int, pageSize int) ([]*BalanceRecord, int64, error) {
	if userId <= 0 {
		return nil, 0, errors.New("userId 无效")
	}
	if page <= 0 {
		return nil, 0, errors.New("page 无效")
	}
	if pageSize <= 0 {
		return nil, 0, errors.New("pageSize 无效")
	}

	var total int64
	if err := DB.Model(&BalanceRecord{}).
		Where("user_id = ?", userId).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}

	records := make([]*BalanceRecord, 0)
	if err := DB.
		Where("user_id = ?", userId).
		Order("created_at DESC, id DESC").
		Limit(pageSize).
		Offset((page - 1) * pageSize).
		Find(&records).Error; err != nil {
		return nil, 0, err
	}

	return records, total, nil
}
