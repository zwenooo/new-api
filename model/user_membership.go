package model

import (
	"database/sql"
	"errors"
	"time"

	"gorm.io/gorm"
)

type UserMembership struct {
	Id     int `json:"id" gorm:"primaryKey"`
	UserId int `json:"user_id" gorm:"index;not null"`
	PlanId int `json:"plan_id" gorm:"index;not null"`
	OrderId int `json:"order_id" gorm:"uniqueIndex;not null"`

	StartAt  int64 `json:"start_at" gorm:"bigint;index;not null"`
	ExpireAt int64 `json:"expire_at" gorm:"bigint;index;not null"`

	PlanMeta  string `json:"plan_meta" gorm:"type:text"`
	CreatedAt int64  `json:"created_at" gorm:"bigint;autoCreateTime"`
}

func (m *UserMembership) Insert(tx *gorm.DB) error {
	if m == nil {
		return errors.New("membership 为空")
	}
	if tx == nil {
		tx = DB
	}
	if m.UserId <= 0 {
		return errors.New("user_id 无效")
	}
	if m.PlanId <= 0 {
		return errors.New("plan_id 无效")
	}
	if m.OrderId <= 0 {
		return errors.New("order_id 无效")
	}
	if m.StartAt <= 0 {
		return errors.New("start_at 无效")
	}
	if m.ExpireAt <= m.StartAt {
		return errors.New("expire_at 必须大于 start_at")
	}
	return tx.Create(m).Error
}

func GetUserMemberships(tx *gorm.DB, userId int) ([]*UserMembership, error) {
	if userId <= 0 {
		return nil, errors.New("userId 无效")
	}
	if tx == nil {
		tx = DB
	}
	var items []*UserMembership
	if err := tx.Where("user_id = ?", userId).Order("expire_at desc, id desc").Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func GetUserMembershipMaxExpireAt(tx *gorm.DB, userId int, now int64) (int64, error) {
	if userId <= 0 {
		return 0, errors.New("userId 无效")
	}
	if tx == nil {
		tx = DB
	}
	if now <= 0 {
		now = time.Now().Unix()
	}
	var maxExpire sql.NullInt64
	if err := tx.Model(&UserMembership{}).
		Where("user_id = ? AND expire_at > ?", userId, now).
		Select("MAX(expire_at)").
		Scan(&maxExpire).Error; err != nil {
		return 0, err
	}
	if !maxExpire.Valid {
		return 0, nil
	}
	return maxExpire.Int64, nil
}
