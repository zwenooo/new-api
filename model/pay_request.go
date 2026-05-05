package model

import (
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func GetUserPayRequestQuotaTx(tx *gorm.DB, userId int) (int, error) {
	if tx == nil {
		tx = DB
	}
	if userId <= 0 {
		return 0, errors.New("userId 无效")
	}
	var user User
	if err := tx.Select("pay_request_quota").Where("id = ?", userId).First(&user).Error; err != nil {
		return 0, err
	}
	if user.PayRequestQuota < 0 {
		return 0, errors.New("pay_request_quota 状态错误")
	}
	return user.PayRequestQuota, nil
}

func PreConsumeUserPayRequestQuota(userId int, count int) error {
	if userId <= 0 {
		return errors.New("userId 无效")
	}
	if count <= 0 {
		return errors.New("count 无效")
	}

	return DB.Transaction(func(tx *gorm.DB) error {
		var user User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("id", "pay_request_quota").
			Where("id = ?", userId).
			First(&user).Error; err != nil {
			return err
		}
		if user.PayRequestQuota < count {
			return errors.New("按次付费次数不足")
		}
		return tx.Model(&User{}).Where("id = ?", userId).Update("pay_request_quota", gorm.Expr("pay_request_quota - ?", count)).Error
	})
}

// PreConsumeUserPayRequestQuotaWithProduct consumes strictly from pay_request_user_balances.
func PreConsumeUserPayRequestQuotaWithProduct(userId int, groupID int, count int) (productId int, err error) {
	if userId <= 0 {
		return 0, errors.New("userId 无效")
	}
	if count <= 0 {
		return 0, errors.New("count 无效")
	}

	err = DB.Transaction(func(tx *gorm.DB) error {
		if groupID > 0 {
			foundProductId, ok, findErr := FindUserPayRequestConsumableProductIdTx(tx, userId, groupID, count)
			if findErr != nil {
				return findErr
			}
			if ok && foundProductId != 0 {
				var balance PayRequestUserBalance
				if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
					Where("user_id = ? AND product_id = ?", userId, foundProductId).
					First(&balance).Error; err != nil {
					return err
				}
				if balance.RemainingRequests < count {
					return errors.New("按次付费次数不足")
				}
				if err := tx.Model(&PayRequestUserBalance{}).
					Where("id = ?", balance.Id).
					Update("remaining_requests", gorm.Expr("remaining_requests - ?", count)).Error; err != nil {
					return err
				}
				var user User
				if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
					Select("id", "pay_request_quota", "pay_request_history_quota", "pay_request_allowed_groups").
					Where("id = ?", userId).
					First(&user).Error; err != nil {
					return err
				}
				if _, err := syncLockedUserPayRequestSnapshotFromBalancesTx(tx, &user); err != nil {
					return err
				}
				productId = foundProductId
				return nil
			}
		}

		return errors.New("按次付费次数不足")
	})
	return productId, err
}

func ReturnUserPayRequestQuota(userId int, count int) error {
	if userId <= 0 {
		return errors.New("userId 无效")
	}
	if count <= 0 {
		return errors.New("count 无效")
	}

	return DB.Transaction(func(tx *gorm.DB) error {
		var user User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("id", "pay_request_quota").
			Where("id = ?", userId).
			First(&user).Error; err != nil {
			return err
		}
		// pay_request_quota can be increased even if it is currently negative due to data corruption,
		// but we keep the logic strict: refuse to operate on invalid state.
		if user.PayRequestQuota < 0 {
			return errors.New("pay_request_quota 状态错误")
		}
		return tx.Model(&User{}).Where("id = ?", userId).Update("pay_request_quota", gorm.Expr("pay_request_quota + ?", count)).Error
	})
}

// ReturnUserPayRequestQuotaWithProduct returns quota strictly to pay_request_user_balances.
func ReturnUserPayRequestQuotaWithProduct(userId int, productId int, count int) error {
	if userId <= 0 {
		return errors.New("userId 无效")
	}
	if count <= 0 {
		return errors.New("count 无效")
	}

	return DB.Transaction(func(tx *gorm.DB) error {
		if productId != 0 {
			var balance PayRequestUserBalance
			err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("user_id = ? AND product_id = ?", userId, productId).
				First(&balance).Error
			if err == nil {
				if balance.RemainingRequests < 0 {
					return errors.New("remaining_requests 状态错误")
				}
				if err := tx.Model(&PayRequestUserBalance{}).
					Where("id = ?", balance.Id).
					Update("remaining_requests", gorm.Expr("remaining_requests + ?", count)).Error; err != nil {
					return err
				}
				var user User
				if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
					Select("id", "pay_request_quota", "pay_request_history_quota", "pay_request_allowed_groups").
					Where("id = ?", userId).
					First(&user).Error; err != nil {
					return err
				}
				if _, err := syncLockedUserPayRequestSnapshotFromBalancesTx(tx, &user); err != nil {
					return err
				}
				return nil
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		}

		return errors.New("按次付费商品未指定，无法返还次数")
	})
}
