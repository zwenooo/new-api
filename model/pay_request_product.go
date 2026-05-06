package model

import (
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// PayRequestProduct represents a pay-per-request product configuration.
type PayRequestProduct struct {
	Id          int    `json:"id" gorm:"primaryKey;autoIncrement:false"`
	Name        string `json:"name" gorm:"type:varchar(64);not null"`
	Description string `json:"description" gorm:"type:text;column:description"`
	Enabled     bool   `json:"enabled" gorm:"type:boolean;not null;default:true;column:enabled"`
	Archived    bool   `json:"archived" gorm:"type:boolean;not null;default:false;column:archived"`
	SortOrder   int    `json:"sort_order" gorm:"type:int;not null;default:0;column:sort_order"`
	// Stock is the remaining inventory for this product.
	// nil means unlimited; 0 means sold out.
	Stock     *int  `json:"stock" gorm:"type:int;column:stock"`
	CreatedAt int64 `json:"created_at" gorm:"bigint;autoCreateTime"`
	UpdatedAt int64 `json:"updated_at" gorm:"bigint;autoUpdateTime"`
}

func (PayRequestProduct) TableName() string {
	return "pay_request_products"
}

// PayRequestProductGroup represents the many-to-many relationship between products and groups.
type PayRequestProductGroup struct {
	ProductId int `json:"product_id" gorm:"primaryKey;autoIncrement:false;column:product_id"`
	GroupId   int `json:"group_id" gorm:"primaryKey;autoIncrement:false;column:group_id;index:idx_pay_request_product_groups_group"`
}

func (PayRequestProductGroup) TableName() string {
	return "pay_request_product_groups"
}

// upsertPayRequestProductTx creates or updates a pay-request product and its group bindings.
func upsertPayRequestProductTx(tx *gorm.DB, product PayRequestProduct, groupIDs []int) error {
	if tx == nil {
		tx = DB
	}
	if tx == nil {
		return errors.New("nil db")
	}
	if product.Id <= 0 {
		return errors.New("product_id 无效")
	}
	if product.Name == "" {
		return errors.New("product_name 不能为空")
	}
	if product.Archived {
		product.Enabled = false
	}
	if err := tx.Select("id", "name", "description", "enabled", "archived", "sort_order", "stock", "created_at", "updated_at").Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{"name", "description", "enabled", "archived", "sort_order", "stock"}),
	}).Create(&product).Error; err != nil {
		return err
	}
	ids := normalizeUniqueSortedIDs(groupIDs)
	if len(ids) == 0 && product.Enabled && !product.Archived {
		return errors.New("至少需要一个分组")
	}
	if len(ids) > 0 {
		if err := ValidateGroupIDsExist(tx, ids); err != nil {
			return err
		}
	}
	if err := tx.Where("product_id = ?", product.Id).Delete(&PayRequestProductGroup{}).Error; err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	rows := make([]PayRequestProductGroup, 0, len(ids))
	for _, groupID := range ids {
		rows = append(rows, PayRequestProductGroup{ProductId: product.Id, GroupId: groupID})
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error
}

// getPayRequestProductGroupIDsTx retrieves the group IDs associated with a product.
func getPayRequestProductGroupIDsTx(tx *gorm.DB, productID int) ([]int, error) {
	if tx == nil {
		tx = DB
	}
	if productID <= 0 {
		return nil, errors.New("product_id 无效")
	}
	var ids []int
	if err := tx.Model(&PayRequestProductGroup{}).Where("product_id = ?", productID).Order("group_id ASC").Pluck("group_id", &ids).Error; err != nil {
		return nil, err
	}
	return filterExistingSortedIDsTx(tx, ids)
}

// GetPayRequestProductAllowedGroupIDsTx returns the allowed group IDs for a pay-request product.
func GetPayRequestProductAllowedGroupIDsTx(tx *gorm.DB, productID int) ([]int, error) {
	return getPayRequestProductGroupIDsTx(tx, productID)
}
