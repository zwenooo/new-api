package model

import (
	"errors"

	"gorm.io/gorm"
)

var ErrProductOutOfStock = errors.New("商品库存不足")

func ConsumeRedemptionPresetStockTx(tx *gorm.DB, presetID int, quantity int) error {
	if presetID <= 0 {
		return errors.New("preset_id 无效")
	}
	if quantity <= 0 {
		return errors.New("quantity 无效")
	}
	if tx == nil {
		tx = DB
	}
	if tx == nil {
		return errors.New("nil db")
	}

	var preset RedemptionPreset
	if err := lockForUpdate(tx).
		Select("id", "stock").
		Where("id = ?", presetID).
		First(&preset).Error; err != nil {
		return err
	}
	if preset.Stock == nil {
		return nil
	}
	if *preset.Stock < quantity {
		return ErrProductOutOfStock
	}
	next := *preset.Stock - quantity
	return tx.Model(&RedemptionPreset{}).Where("id = ?", presetID).Update("stock", next).Error
}

// consumeProductStockByTableTx is the generic stock consumption logic.
// It locks the row, checks stock (nil = unlimited), and decrements.
func consumeProductStockByTableTx(tx *gorm.DB, tableName string, productID int, quantity int) error {
	if productID <= 0 {
		return errors.New("product_id 无效")
	}
	if quantity <= 0 {
		return errors.New("quantity 无效")
	}
	if tx == nil {
		tx = DB
	}
	if tx == nil {
		return errors.New("nil db")
	}

	type stockRow struct {
		Stock *int `gorm:"column:stock"`
	}
	var row stockRow
	query := tx.Table(tableName).Select("stock").Where("id = ?", productID)
	if supportsForUpdate(tx) {
		query = lockForUpdate(tx).Table(tableName).Select("stock").Where("id = ?", productID)
	}
	if err := query.Take(&row).Error; err != nil {
		return err
	}
	stock := row.Stock
	if stock == nil {
		return nil
	}
	if *stock < quantity {
		return ErrProductOutOfStock
	}
	next := *stock - quantity
	return tx.Exec("UPDATE "+tableName+" SET stock = ? WHERE id = ?", next, productID).Error
}

func ConsumePaygProductStockTx(tx *gorm.DB, productID int, quantity int) error {
	return consumeProductStockByTableTx(tx, "payg_products", productID, quantity)
}

func ConsumePayRequestProductStockTx(tx *gorm.DB, productID int, quantity int) error {
	return consumeProductStockByTableTx(tx, "pay_request_products", productID, quantity)
}

func ConsumePayTokenProductStockTx(tx *gorm.DB, productID int, quantity int) error {
	return consumeProductStockByTableTx(tx, "pay_token_products", productID, quantity)
}
