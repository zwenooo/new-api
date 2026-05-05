package model

import (
	"encoding/json"
	"errors"
	"time"

	"one-api/setting/payg_setting"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type PaygProductRevision struct {
	Id              int         `json:"id" gorm:"primaryKey"`
	ProductId       int         `json:"product_id" gorm:"not null;index:idx_payg_product_revisions_product_revision,priority:1;column:product_id"`
	RevisionNo      int         `json:"revision_no" gorm:"not null;index:idx_payg_product_revisions_product_revision,priority:2;column:revision_no"`
	IsCurrent       bool        `json:"is_current" gorm:"type:boolean;default:false;index:idx_payg_product_revisions_product_current,priority:2;column:is_current"`
	SnapshotTime    int64       `json:"snapshot_time" gorm:"bigint;not null;index;column:snapshot_time"`
	Name            string      `json:"name" gorm:"type:varchar(64);not null"`
	Description     string      `json:"description" gorm:"type:text;column:description"`
	Enabled         bool        `json:"enabled" gorm:"type:boolean;not null;default:true;column:enabled"`
	Archived        bool        `json:"archived" gorm:"type:boolean;not null;default:false;column:archived"`
	SortOrder       int         `json:"sort_order" gorm:"type:int;not null;default:0;column:sort_order"`
	Stock           *int        `json:"stock" gorm:"type:int;column:stock"`
	AllowedGroupIds JSONValue   `json:"allowed_group_ids" gorm:"-"`
	CreatedAt       time.Time   `json:"created_at"`
	UpdatedAt       time.Time   `json:"updated_at"`
	Product         PaygProduct `json:"-" gorm:"foreignKey:ProductId;references:Id;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT;"`
}

func (PaygProductRevision) TableName() string {
	return "payg_product_revisions"
}

type PaygProductRevisionGroup struct {
	RevisionId int                 `json:"revision_id" gorm:"primaryKey;autoIncrement:false;column:revision_id"`
	GroupId    int                 `json:"group_id" gorm:"primaryKey;autoIncrement:false;column:group_id;index:idx_payg_product_revision_groups_group"`
	Revision   PaygProductRevision `json:"-" gorm:"foreignKey:RevisionId;references:Id;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	Group      Group               `json:"-" gorm:"foreignKey:GroupId;references:Id;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT;"`
}

func (PaygProductRevisionGroup) TableName() string {
	return "payg_product_revision_groups"
}

type PayRequestProductRevision struct {
	Id              int               `json:"id" gorm:"primaryKey"`
	ProductId       int               `json:"product_id" gorm:"not null;index:idx_pay_request_product_revisions_product_revision,priority:1;column:product_id"`
	RevisionNo      int               `json:"revision_no" gorm:"not null;index:idx_pay_request_product_revisions_product_revision,priority:2;column:revision_no"`
	IsCurrent       bool              `json:"is_current" gorm:"type:boolean;default:false;index:idx_pay_request_product_revisions_product_current,priority:2;column:is_current"`
	SnapshotTime    int64             `json:"snapshot_time" gorm:"bigint;not null;index;column:snapshot_time"`
	Name            string            `json:"name" gorm:"type:varchar(64);not null"`
	Description     string            `json:"description" gorm:"type:text;column:description"`
	Enabled         bool              `json:"enabled" gorm:"type:boolean;not null;default:true;column:enabled"`
	Archived        bool              `json:"archived" gorm:"type:boolean;not null;default:false;column:archived"`
	SortOrder       int               `json:"sort_order" gorm:"type:int;not null;default:0;column:sort_order"`
	Stock           *int              `json:"stock" gorm:"type:int;column:stock"`
	AllowedGroupIds JSONValue         `json:"allowed_group_ids" gorm:"-"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
	Product         PayRequestProduct `json:"-" gorm:"foreignKey:ProductId;references:Id;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT;"`
}

func (PayRequestProductRevision) TableName() string {
	return "pay_request_product_revisions"
}

type PayRequestProductRevisionGroup struct {
	RevisionId int                       `json:"revision_id" gorm:"primaryKey;autoIncrement:false;column:revision_id"`
	GroupId    int                       `json:"group_id" gorm:"primaryKey;autoIncrement:false;column:group_id;index:idx_pay_request_product_revision_groups_group"`
	Revision   PayRequestProductRevision `json:"-" gorm:"foreignKey:RevisionId;references:Id;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	Group      Group                     `json:"-" gorm:"foreignKey:GroupId;references:Id;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT;"`
}

func (PayRequestProductRevisionGroup) TableName() string {
	return "pay_request_product_revision_groups"
}

type PayTokenProductRevision struct {
	Id              int             `json:"id" gorm:"primaryKey"`
	ProductId       int             `json:"product_id" gorm:"not null;index:idx_pay_token_product_revisions_product_revision,priority:1;column:product_id"`
	RevisionNo      int             `json:"revision_no" gorm:"not null;index:idx_pay_token_product_revisions_product_revision,priority:2;column:revision_no"`
	IsCurrent       bool            `json:"is_current" gorm:"type:boolean;default:false;index:idx_pay_token_product_revisions_product_current,priority:2;column:is_current"`
	SnapshotTime    int64           `json:"snapshot_time" gorm:"bigint;not null;index;column:snapshot_time"`
	Name            string          `json:"name" gorm:"type:varchar(64);not null"`
	Description     string          `json:"description" gorm:"type:text;column:description"`
	Enabled         bool            `json:"enabled" gorm:"type:boolean;not null;default:true;column:enabled"`
	Archived        bool            `json:"archived" gorm:"type:boolean;not null;default:false;column:archived"`
	SortOrder       int             `json:"sort_order" gorm:"type:int;not null;default:0;column:sort_order"`
	Stock           *int            `json:"stock" gorm:"type:int;column:stock"`
	AllowedGroupIds JSONValue       `json:"allowed_group_ids" gorm:"-"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
	Product         PayTokenProduct `json:"-" gorm:"foreignKey:ProductId;references:Id;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT;"`
}

func (PayTokenProductRevision) TableName() string {
	return "pay_token_product_revisions"
}

type PayTokenProductRevisionGroup struct {
	RevisionId int                     `json:"revision_id" gorm:"primaryKey;autoIncrement:false;column:revision_id"`
	GroupId    int                     `json:"group_id" gorm:"primaryKey;autoIncrement:false;column:group_id;index:idx_pay_token_product_revision_groups_group"`
	Revision   PayTokenProductRevision `json:"-" gorm:"foreignKey:RevisionId;references:Id;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	Group      Group                   `json:"-" gorm:"foreignKey:GroupId;references:Id;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT;"`
}

func (PayTokenProductRevisionGroup) TableName() string {
	return "pay_token_product_revision_groups"
}

func sameOptionalInt(a *int, b *int) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func getPaygProductSnapshotTx(tx *gorm.DB, productID int) (*payg_setting.PaygProduct, error) {
	if tx == nil {
		tx = DB
	}
	if productID <= 0 {
		return nil, errors.New("product_id 无效")
	}
	var product PaygProduct
	if err := tx.Where("id = ?", productID).First(&product).Error; err != nil {
		return nil, err
	}
	groupIDs, err := getPaygProductGroupIDsTx(tx, productID)
	if err != nil {
		return nil, err
	}
	return &payg_setting.PaygProduct{
		Id:              product.Id,
		Name:            product.Name,
		Description:     product.Description,
		Enabled:         product.Enabled,
		Archived:        product.Archived,
		SortOrder:       product.SortOrder,
		Stock:           product.Stock,
		AllowedGroupIds: groupIDs,
	}, nil
}

func getPayRequestProductSnapshotTx(tx *gorm.DB, productID int) (*payg_setting.PayRequestProduct, error) {
	if tx == nil {
		tx = DB
	}
	if productID <= 0 {
		return nil, errors.New("product_id 无效")
	}
	var product PayRequestProduct
	if err := tx.Where("id = ?", productID).First(&product).Error; err != nil {
		return nil, err
	}
	groupIDs, err := getPayRequestProductGroupIDsTx(tx, productID)
	if err != nil {
		return nil, err
	}
	return &payg_setting.PayRequestProduct{
		Id:              product.Id,
		Name:            product.Name,
		Description:     product.Description,
		Enabled:         product.Enabled,
		Archived:        product.Archived,
		SortOrder:       product.SortOrder,
		Stock:           product.Stock,
		AllowedGroupIds: groupIDs,
	}, nil
}

func getPayTokenProductSnapshotTx(tx *gorm.DB, productID int) (*payg_setting.PayTokenProduct, error) {
	if tx == nil {
		tx = DB
	}
	if productID <= 0 {
		return nil, errors.New("product_id 无效")
	}
	var product PayTokenProduct
	if err := tx.Where("id = ?", productID).First(&product).Error; err != nil {
		return nil, err
	}
	groupIDs, err := getPayTokenProductGroupIDsTx(tx, productID)
	if err != nil {
		return nil, err
	}
	return &payg_setting.PayTokenProduct{
		Id:              product.Id,
		Name:            product.Name,
		Description:     product.Description,
		Enabled:         product.Enabled,
		Archived:        product.Archived,
		SortOrder:       product.SortOrder,
		Stock:           product.Stock,
		AllowedGroupIds: groupIDs,
	}, nil
}

func paygProductRevisionMatches(product *payg_setting.PaygProduct, revision *PaygProductRevision) bool {
	if product == nil || revision == nil {
		return false
	}
	if product.Id != revision.ProductId ||
		product.Name != revision.Name ||
		product.Description != revision.Description ||
		product.Enabled != revision.Enabled ||
		product.Archived != revision.Archived ||
		product.SortOrder != revision.SortOrder ||
		!sameOptionalInt(product.Stock, revision.Stock) {
		return false
	}
	var revisionGroupIDs []int
	if len(revision.AllowedGroupIds) > 0 {
		_ = json.Unmarshal([]byte(revision.AllowedGroupIds), &revisionGroupIDs)
	}
	revisionGroupIDs = normalizeUniqueSortedIDs(revisionGroupIDs)
	productGroupIDs := normalizeUniqueSortedIDs(product.AllowedGroupIds)
	if len(productGroupIDs) != len(revisionGroupIDs) {
		return false
	}
	for i := range productGroupIDs {
		if productGroupIDs[i] != revisionGroupIDs[i] {
			return false
		}
	}
	return true
}

func payRequestProductRevisionMatches(product *payg_setting.PayRequestProduct, revision *PayRequestProductRevision) bool {
	if product == nil || revision == nil {
		return false
	}
	if product.Id != revision.ProductId ||
		product.Name != revision.Name ||
		product.Description != revision.Description ||
		product.Enabled != revision.Enabled ||
		product.Archived != revision.Archived ||
		product.SortOrder != revision.SortOrder ||
		!sameOptionalInt(product.Stock, revision.Stock) {
		return false
	}
	var revisionGroupIDs []int
	if len(revision.AllowedGroupIds) > 0 {
		_ = json.Unmarshal([]byte(revision.AllowedGroupIds), &revisionGroupIDs)
	}
	revisionGroupIDs = normalizeUniqueSortedIDs(revisionGroupIDs)
	productGroupIDs := normalizeUniqueSortedIDs(product.AllowedGroupIds)
	if len(productGroupIDs) != len(revisionGroupIDs) {
		return false
	}
	for i := range productGroupIDs {
		if productGroupIDs[i] != revisionGroupIDs[i] {
			return false
		}
	}
	return true
}

func payTokenProductRevisionMatches(product *payg_setting.PayTokenProduct, revision *PayTokenProductRevision) bool {
	if product == nil || revision == nil {
		return false
	}
	if product.Id != revision.ProductId ||
		product.Name != revision.Name ||
		product.Description != revision.Description ||
		product.Enabled != revision.Enabled ||
		product.Archived != revision.Archived ||
		product.SortOrder != revision.SortOrder ||
		!sameOptionalInt(product.Stock, revision.Stock) {
		return false
	}
	var revisionGroupIDs []int
	if len(revision.AllowedGroupIds) > 0 {
		_ = json.Unmarshal([]byte(revision.AllowedGroupIds), &revisionGroupIDs)
	}
	revisionGroupIDs = normalizeUniqueSortedIDs(revisionGroupIDs)
	productGroupIDs := normalizeUniqueSortedIDs(product.AllowedGroupIds)
	if len(productGroupIDs) != len(revisionGroupIDs) {
		return false
	}
	for i := range productGroupIDs {
		if productGroupIDs[i] != revisionGroupIDs[i] {
			return false
		}
	}
	return true
}

func upsertPaygProductRevisionGroupsTx(tx *gorm.DB, revisionID int, groupIDs []int) error {
	if tx == nil {
		tx = DB
	}
	if revisionID <= 0 {
		return errors.New("revision_id 无效")
	}
	ids := normalizeUniqueSortedIDs(groupIDs)
	if err := tx.Where("revision_id = ?", revisionID).Delete(&PaygProductRevisionGroup{}).Error; err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	if err := ValidateGroupIDsExist(tx, ids); err != nil {
		return err
	}
	rows := make([]PaygProductRevisionGroup, 0, len(ids))
	for _, groupID := range ids {
		rows = append(rows, PaygProductRevisionGroup{RevisionId: revisionID, GroupId: groupID})
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error
}

func upsertPayRequestProductRevisionGroupsTx(tx *gorm.DB, revisionID int, groupIDs []int) error {
	if tx == nil {
		tx = DB
	}
	if revisionID <= 0 {
		return errors.New("revision_id 无效")
	}
	ids := normalizeUniqueSortedIDs(groupIDs)
	if err := tx.Where("revision_id = ?", revisionID).Delete(&PayRequestProductRevisionGroup{}).Error; err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	if err := ValidateGroupIDsExist(tx, ids); err != nil {
		return err
	}
	rows := make([]PayRequestProductRevisionGroup, 0, len(ids))
	for _, groupID := range ids {
		rows = append(rows, PayRequestProductRevisionGroup{RevisionId: revisionID, GroupId: groupID})
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error
}

func upsertPayTokenProductRevisionGroupsTx(tx *gorm.DB, revisionID int, groupIDs []int) error {
	if tx == nil {
		tx = DB
	}
	if revisionID <= 0 {
		return errors.New("revision_id 无效")
	}
	ids := normalizeUniqueSortedIDs(groupIDs)
	if err := tx.Where("revision_id = ?", revisionID).Delete(&PayTokenProductRevisionGroup{}).Error; err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	if err := ValidateGroupIDsExist(tx, ids); err != nil {
		return err
	}
	rows := make([]PayTokenProductRevisionGroup, 0, len(ids))
	for _, groupID := range ids {
		rows = append(rows, PayTokenProductRevisionGroup{RevisionId: revisionID, GroupId: groupID})
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error
}

func createPaygProductRevisionTx(tx *gorm.DB, product *payg_setting.PaygProduct) (*PaygProductRevision, error) {
	if tx == nil {
		tx = DB
	}
	if product == nil || product.Id <= 0 {
		return nil, errors.New("product 无效")
	}
	var maxRevisionNo int
	if err := tx.Model(&PaygProductRevision{}).
		Where("product_id = ?", product.Id).
		Select("COALESCE(MAX(revision_no), 0)").
		Scan(&maxRevisionNo).Error; err != nil {
		return nil, err
	}
	if err := tx.Model(&PaygProductRevision{}).
		Where("product_id = ? AND is_current = ?", product.Id, true).
		Update("is_current", false).Error; err != nil {
		return nil, err
	}
	revision := &PaygProductRevision{
		ProductId:    product.Id,
		RevisionNo:   maxRevisionNo + 1,
		IsCurrent:    true,
		SnapshotTime: time.Now().Unix(),
		Name:         product.Name,
		Description:  product.Description,
		Enabled:      product.Enabled,
		Archived:     product.Archived,
		SortOrder:    product.SortOrder,
		Stock:        product.Stock,
	}
	if err := tx.Create(revision).Error; err != nil {
		return nil, err
	}
	if err := upsertPaygProductRevisionGroupsTx(tx, revision.Id, product.AllowedGroupIds); err != nil {
		return nil, err
	}
	if len(product.AllowedGroupIds) > 0 {
		if b, err := MarshalGroupIDsJSON(product.AllowedGroupIds); err == nil {
			revision.AllowedGroupIds = b
		} else {
			return nil, err
		}
	}
	return revision, nil
}

func createPayRequestProductRevisionTx(tx *gorm.DB, product *payg_setting.PayRequestProduct) (*PayRequestProductRevision, error) {
	if tx == nil {
		tx = DB
	}
	if product == nil || product.Id <= 0 {
		return nil, errors.New("product 无效")
	}
	var maxRevisionNo int
	if err := tx.Model(&PayRequestProductRevision{}).
		Where("product_id = ?", product.Id).
		Select("COALESCE(MAX(revision_no), 0)").
		Scan(&maxRevisionNo).Error; err != nil {
		return nil, err
	}
	if err := tx.Model(&PayRequestProductRevision{}).
		Where("product_id = ? AND is_current = ?", product.Id, true).
		Update("is_current", false).Error; err != nil {
		return nil, err
	}
	revision := &PayRequestProductRevision{
		ProductId:    product.Id,
		RevisionNo:   maxRevisionNo + 1,
		IsCurrent:    true,
		SnapshotTime: time.Now().Unix(),
		Name:         product.Name,
		Description:  product.Description,
		Enabled:      product.Enabled,
		Archived:     product.Archived,
		SortOrder:    product.SortOrder,
		Stock:        product.Stock,
	}
	if err := tx.Create(revision).Error; err != nil {
		return nil, err
	}
	if err := upsertPayRequestProductRevisionGroupsTx(tx, revision.Id, product.AllowedGroupIds); err != nil {
		return nil, err
	}
	if len(product.AllowedGroupIds) > 0 {
		if b, err := MarshalGroupIDsJSON(product.AllowedGroupIds); err == nil {
			revision.AllowedGroupIds = b
		} else {
			return nil, err
		}
	}
	return revision, nil
}

func createPayTokenProductRevisionTx(tx *gorm.DB, product *payg_setting.PayTokenProduct) (*PayTokenProductRevision, error) {
	if tx == nil {
		tx = DB
	}
	if product == nil || product.Id <= 0 {
		return nil, errors.New("product 无效")
	}
	var maxRevisionNo int
	if err := tx.Model(&PayTokenProductRevision{}).
		Where("product_id = ?", product.Id).
		Select("COALESCE(MAX(revision_no), 0)").
		Scan(&maxRevisionNo).Error; err != nil {
		return nil, err
	}
	if err := tx.Model(&PayTokenProductRevision{}).
		Where("product_id = ? AND is_current = ?", product.Id, true).
		Update("is_current", false).Error; err != nil {
		return nil, err
	}
	revision := &PayTokenProductRevision{
		ProductId:    product.Id,
		RevisionNo:   maxRevisionNo + 1,
		IsCurrent:    true,
		SnapshotTime: time.Now().Unix(),
		Name:         product.Name,
		Description:  product.Description,
		Enabled:      product.Enabled,
		Archived:     product.Archived,
		SortOrder:    product.SortOrder,
		Stock:        product.Stock,
	}
	if err := tx.Create(revision).Error; err != nil {
		return nil, err
	}
	if err := upsertPayTokenProductRevisionGroupsTx(tx, revision.Id, product.AllowedGroupIds); err != nil {
		return nil, err
	}
	if len(product.AllowedGroupIds) > 0 {
		if b, err := MarshalGroupIDsJSON(product.AllowedGroupIds); err == nil {
			revision.AllowedGroupIds = b
		} else {
			return nil, err
		}
	}
	return revision, nil
}

func getPaygProductRevisionGroupIDsByRevisionIDsTx(tx *gorm.DB, revisionIDs []int) (map[int][]int, error) {
	if tx == nil {
		tx = DB
	}
	out := make(map[int][]int, len(revisionIDs))
	ids := normalizeUniqueSortedIDs(revisionIDs)
	if len(ids) == 0 {
		return out, nil
	}
	type row struct {
		RevisionId int `gorm:"column:revision_id"`
		GroupId    int `gorm:"column:group_id"`
	}
	var rows []row
	if err := tx.Model(&PaygProductRevisionGroup{}).
		Select("revision_id", "group_id").
		Where("revision_id IN ?", ids).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		if row.RevisionId <= 0 || row.GroupId <= 0 {
			continue
		}
		out[row.RevisionId] = append(out[row.RevisionId], row.GroupId)
	}
	for revisionID, groupIDs := range out {
		out[revisionID] = normalizeUniqueSortedIDs(groupIDs)
	}
	return out, nil
}

func getPayRequestProductRevisionGroupIDsByRevisionIDsTx(tx *gorm.DB, revisionIDs []int) (map[int][]int, error) {
	if tx == nil {
		tx = DB
	}
	out := make(map[int][]int, len(revisionIDs))
	ids := normalizeUniqueSortedIDs(revisionIDs)
	if len(ids) == 0 {
		return out, nil
	}
	type row struct {
		RevisionId int `gorm:"column:revision_id"`
		GroupId    int `gorm:"column:group_id"`
	}
	var rows []row
	if err := tx.Model(&PayRequestProductRevisionGroup{}).
		Select("revision_id", "group_id").
		Where("revision_id IN ?", ids).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		if row.RevisionId <= 0 || row.GroupId <= 0 {
			continue
		}
		out[row.RevisionId] = append(out[row.RevisionId], row.GroupId)
	}
	for revisionID, groupIDs := range out {
		out[revisionID] = normalizeUniqueSortedIDs(groupIDs)
	}
	return out, nil
}

func getPayTokenProductRevisionGroupIDsByRevisionIDsTx(tx *gorm.DB, revisionIDs []int) (map[int][]int, error) {
	if tx == nil {
		tx = DB
	}
	out := make(map[int][]int, len(revisionIDs))
	ids := normalizeUniqueSortedIDs(revisionIDs)
	if len(ids) == 0 {
		return out, nil
	}
	type row struct {
		RevisionId int `gorm:"column:revision_id"`
		GroupId    int `gorm:"column:group_id"`
	}
	var rows []row
	if err := tx.Model(&PayTokenProductRevisionGroup{}).
		Select("revision_id", "group_id").
		Where("revision_id IN ?", ids).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		if row.RevisionId <= 0 || row.GroupId <= 0 {
			continue
		}
		out[row.RevisionId] = append(out[row.RevisionId], row.GroupId)
	}
	for revisionID, groupIDs := range out {
		out[revisionID] = normalizeUniqueSortedIDs(groupIDs)
	}
	return out, nil
}

func hydratePaygProductRevisionsTx(tx *gorm.DB, revisions []*PaygProductRevision) error {
	if tx == nil {
		tx = DB
	}
	if len(revisions) == 0 {
		return nil
	}
	revisionIDs := make([]int, 0, len(revisions))
	for _, revision := range revisions {
		if revision == nil || revision.Id <= 0 {
			continue
		}
		revisionIDs = append(revisionIDs, revision.Id)
	}
	groupIDsByRevisionID, err := getPaygProductRevisionGroupIDsByRevisionIDsTx(tx, revisionIDs)
	if err != nil {
		return err
	}
	for _, revision := range revisions {
		if revision == nil || revision.Id <= 0 {
			continue
		}
		groupIDs := normalizeUniqueSortedIDs(groupIDsByRevisionID[revision.Id])
		if len(groupIDs) == 0 {
			revision.AllowedGroupIds = nil
			continue
		}
		b, err := MarshalGroupIDsJSON(groupIDs)
		if err != nil {
			return err
		}
		revision.AllowedGroupIds = b
	}
	return nil
}

func hydratePayRequestProductRevisionsTx(tx *gorm.DB, revisions []*PayRequestProductRevision) error {
	if tx == nil {
		tx = DB
	}
	if len(revisions) == 0 {
		return nil
	}
	revisionIDs := make([]int, 0, len(revisions))
	for _, revision := range revisions {
		if revision == nil || revision.Id <= 0 {
			continue
		}
		revisionIDs = append(revisionIDs, revision.Id)
	}
	groupIDsByRevisionID, err := getPayRequestProductRevisionGroupIDsByRevisionIDsTx(tx, revisionIDs)
	if err != nil {
		return err
	}
	for _, revision := range revisions {
		if revision == nil || revision.Id <= 0 {
			continue
		}
		groupIDs := normalizeUniqueSortedIDs(groupIDsByRevisionID[revision.Id])
		if len(groupIDs) == 0 {
			revision.AllowedGroupIds = nil
			continue
		}
		b, err := MarshalGroupIDsJSON(groupIDs)
		if err != nil {
			return err
		}
		revision.AllowedGroupIds = b
	}
	return nil
}

func hydratePayTokenProductRevisionsTx(tx *gorm.DB, revisions []*PayTokenProductRevision) error {
	if tx == nil {
		tx = DB
	}
	if len(revisions) == 0 {
		return nil
	}
	revisionIDs := make([]int, 0, len(revisions))
	for _, revision := range revisions {
		if revision == nil || revision.Id <= 0 {
			continue
		}
		revisionIDs = append(revisionIDs, revision.Id)
	}
	groupIDsByRevisionID, err := getPayTokenProductRevisionGroupIDsByRevisionIDsTx(tx, revisionIDs)
	if err != nil {
		return err
	}
	for _, revision := range revisions {
		if revision == nil || revision.Id <= 0 {
			continue
		}
		groupIDs := normalizeUniqueSortedIDs(groupIDsByRevisionID[revision.Id])
		if len(groupIDs) == 0 {
			revision.AllowedGroupIds = nil
			continue
		}
		b, err := MarshalGroupIDsJSON(groupIDs)
		if err != nil {
			return err
		}
		revision.AllowedGroupIds = b
	}
	return nil
}

func ensureCurrentPaygProductRevisionTx(tx *gorm.DB, productID int) (*PaygProductRevision, error) {
	if tx == nil {
		tx = DB
	}
	product, err := getPaygProductSnapshotTx(tx, productID)
	if err != nil {
		return nil, err
	}
	var current PaygProductRevision
	err = tx.Where("product_id = ? AND is_current = ?", productID, true).Order("revision_no DESC, id DESC").First(&current).Error
	if err == nil {
		if err := hydratePaygProductRevisionsTx(tx, []*PaygProductRevision{&current}); err != nil {
			return nil, err
		}
		if paygProductRevisionMatches(product, &current) {
			return &current, nil
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	return createPaygProductRevisionTx(tx, product)
}

func EnsureCurrentPaygProductRevisionTx(tx *gorm.DB, productID int) (*PaygProductRevision, error) {
	return ensureCurrentPaygProductRevisionTx(tx, productID)
}

func ensureCurrentPayRequestProductRevisionTx(tx *gorm.DB, productID int) (*PayRequestProductRevision, error) {
	if tx == nil {
		tx = DB
	}
	product, err := getPayRequestProductSnapshotTx(tx, productID)
	if err != nil {
		return nil, err
	}
	var current PayRequestProductRevision
	err = tx.Where("product_id = ? AND is_current = ?", productID, true).Order("revision_no DESC, id DESC").First(&current).Error
	if err == nil {
		if err := hydratePayRequestProductRevisionsTx(tx, []*PayRequestProductRevision{&current}); err != nil {
			return nil, err
		}
		if payRequestProductRevisionMatches(product, &current) {
			return &current, nil
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	return createPayRequestProductRevisionTx(tx, product)
}

func EnsureCurrentPayRequestProductRevisionTx(tx *gorm.DB, productID int) (*PayRequestProductRevision, error) {
	return ensureCurrentPayRequestProductRevisionTx(tx, productID)
}

func ensureCurrentPayTokenProductRevisionTx(tx *gorm.DB, productID int) (*PayTokenProductRevision, error) {
	if tx == nil {
		tx = DB
	}
	product, err := getPayTokenProductSnapshotTx(tx, productID)
	if err != nil {
		return nil, err
	}
	var current PayTokenProductRevision
	err = tx.Where("product_id = ? AND is_current = ?", productID, true).Order("revision_no DESC, id DESC").First(&current).Error
	if err == nil {
		if err := hydratePayTokenProductRevisionsTx(tx, []*PayTokenProductRevision{&current}); err != nil {
			return nil, err
		}
		if payTokenProductRevisionMatches(product, &current) {
			return &current, nil
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	return createPayTokenProductRevisionTx(tx, product)
}

func EnsureCurrentPayTokenProductRevisionTx(tx *gorm.DB, productID int) (*PayTokenProductRevision, error) {
	return ensureCurrentPayTokenProductRevisionTx(tx, productID)
}

func ListPaygProductRevisionsTx(tx *gorm.DB, productID int) ([]*PaygProductRevision, error) {
	if tx == nil {
		tx = DB
	}
	if productID <= 0 {
		return nil, errors.New("product_id 无效")
	}
	var revisions []*PaygProductRevision
	if err := tx.Where("product_id = ?", productID).
		Order("revision_no DESC, id DESC").
		Find(&revisions).Error; err != nil {
		return nil, err
	}
	if err := hydratePaygProductRevisionsTx(tx, revisions); err != nil {
		return nil, err
	}
	return revisions, nil
}

func ListPayRequestProductRevisionsTx(tx *gorm.DB, productID int) ([]*PayRequestProductRevision, error) {
	if tx == nil {
		tx = DB
	}
	if productID <= 0 {
		return nil, errors.New("product_id 无效")
	}
	var revisions []*PayRequestProductRevision
	if err := tx.Where("product_id = ?", productID).
		Order("revision_no DESC, id DESC").
		Find(&revisions).Error; err != nil {
		return nil, err
	}
	if err := hydratePayRequestProductRevisionsTx(tx, revisions); err != nil {
		return nil, err
	}
	return revisions, nil
}

func ListPayTokenProductRevisionsTx(tx *gorm.DB, productID int) ([]*PayTokenProductRevision, error) {
	if tx == nil {
		tx = DB
	}
	if productID <= 0 {
		return nil, errors.New("product_id 无效")
	}
	var revisions []*PayTokenProductRevision
	if err := tx.Where("product_id = ?", productID).
		Order("revision_no DESC, id DESC").
		Find(&revisions).Error; err != nil {
		return nil, err
	}
	if err := hydratePayTokenProductRevisionsTx(tx, revisions); err != nil {
		return nil, err
	}
	return revisions, nil
}

func ListPaygProductRevisions(productID int) ([]*PaygProductRevision, error) {
	return ListPaygProductRevisionsTx(DB, productID)
}

func ListPayRequestProductRevisions(productID int) ([]*PayRequestProductRevision, error) {
	return ListPayRequestProductRevisionsTx(DB, productID)
}

func ListPayTokenProductRevisions(productID int) ([]*PayTokenProductRevision, error) {
	return ListPayTokenProductRevisionsTx(DB, productID)
}

func GetPaygProductRevisionTx(tx *gorm.DB, productID int, revisionID int) (*PaygProductRevision, error) {
	if tx == nil {
		tx = DB
	}
	if productID <= 0 || revisionID <= 0 {
		return nil, errors.New("参数无效")
	}
	var revision PaygProductRevision
	if err := tx.Where("id = ? AND product_id = ?", revisionID, productID).First(&revision).Error; err != nil {
		return nil, err
	}
	if err := hydratePaygProductRevisionsTx(tx, []*PaygProductRevision{&revision}); err != nil {
		return nil, err
	}
	return &revision, nil
}

func GetPayRequestProductRevisionTx(tx *gorm.DB, productID int, revisionID int) (*PayRequestProductRevision, error) {
	if tx == nil {
		tx = DB
	}
	if productID <= 0 || revisionID <= 0 {
		return nil, errors.New("参数无效")
	}
	var revision PayRequestProductRevision
	if err := tx.Where("id = ? AND product_id = ?", revisionID, productID).First(&revision).Error; err != nil {
		return nil, err
	}
	if err := hydratePayRequestProductRevisionsTx(tx, []*PayRequestProductRevision{&revision}); err != nil {
		return nil, err
	}
	return &revision, nil
}

func GetPayTokenProductRevisionTx(tx *gorm.DB, productID int, revisionID int) (*PayTokenProductRevision, error) {
	if tx == nil {
		tx = DB
	}
	if productID <= 0 || revisionID <= 0 {
		return nil, errors.New("参数无效")
	}
	var revision PayTokenProductRevision
	if err := tx.Where("id = ? AND product_id = ?", revisionID, productID).First(&revision).Error; err != nil {
		return nil, err
	}
	if err := hydratePayTokenProductRevisionsTx(tx, []*PayTokenProductRevision{&revision}); err != nil {
		return nil, err
	}
	return &revision, nil
}

func RestorePaygProductFromRevisionTx(tx *gorm.DB, productID int, revisionID int) (*payg_setting.PaygProduct, error) {
	if tx == nil {
		var restored *payg_setting.PaygProduct
		err := DB.Transaction(func(tx *gorm.DB) error {
			next, err := RestorePaygProductFromRevisionTx(tx, productID, revisionID)
			if err != nil {
				return err
			}
			restored = next
			return nil
		})
		if err != nil {
			return nil, err
		}
		return restored, nil
	}
	revision, err := GetPaygProductRevisionTx(tx, productID, revisionID)
	if err != nil {
		return nil, err
	}
	groupIDs, err := ParseGroupIDsJSON(revision.AllowedGroupIds)
	if err != nil {
		return nil, err
	}
	if err := upsertPaygProductTx(tx, PaygProduct{
		Id:          productID,
		Name:        revision.Name,
		Description: revision.Description,
		Enabled:     revision.Enabled,
		Archived:    revision.Archived,
		SortOrder:   revision.SortOrder,
		Stock:       revision.Stock,
	}, groupIDs); err != nil {
		return nil, err
	}
	if _, err := ensureCurrentPaygProductRevisionTx(tx, productID); err != nil {
		return nil, err
	}
	return getPaygProductSnapshotTx(tx, productID)
}

func RestorePayRequestProductFromRevisionTx(tx *gorm.DB, productID int, revisionID int) (*payg_setting.PayRequestProduct, error) {
	if tx == nil {
		var restored *payg_setting.PayRequestProduct
		err := DB.Transaction(func(tx *gorm.DB) error {
			next, err := RestorePayRequestProductFromRevisionTx(tx, productID, revisionID)
			if err != nil {
				return err
			}
			restored = next
			return nil
		})
		if err != nil {
			return nil, err
		}
		return restored, nil
	}
	revision, err := GetPayRequestProductRevisionTx(tx, productID, revisionID)
	if err != nil {
		return nil, err
	}
	groupIDs, err := ParseGroupIDsJSON(revision.AllowedGroupIds)
	if err != nil {
		return nil, err
	}
	if err := upsertPayRequestProductTx(tx, PayRequestProduct{
		Id:          productID,
		Name:        revision.Name,
		Description: revision.Description,
		Enabled:     revision.Enabled,
		Archived:    revision.Archived,
		SortOrder:   revision.SortOrder,
		Stock:       revision.Stock,
	}, groupIDs); err != nil {
		return nil, err
	}
	if _, err := ensureCurrentPayRequestProductRevisionTx(tx, productID); err != nil {
		return nil, err
	}
	return getPayRequestProductSnapshotTx(tx, productID)
}

func RestorePayTokenProductFromRevisionTx(tx *gorm.DB, productID int, revisionID int) (*payg_setting.PayTokenProduct, error) {
	if tx == nil {
		var restored *payg_setting.PayTokenProduct
		err := DB.Transaction(func(tx *gorm.DB) error {
			next, err := RestorePayTokenProductFromRevisionTx(tx, productID, revisionID)
			if err != nil {
				return err
			}
			restored = next
			return nil
		})
		if err != nil {
			return nil, err
		}
		return restored, nil
	}
	revision, err := GetPayTokenProductRevisionTx(tx, productID, revisionID)
	if err != nil {
		return nil, err
	}
	groupIDs, err := ParseGroupIDsJSON(revision.AllowedGroupIds)
	if err != nil {
		return nil, err
	}
	if err := upsertPayTokenProductTx(tx, PayTokenProduct{
		Id:          productID,
		Name:        revision.Name,
		Description: revision.Description,
		Enabled:     revision.Enabled,
		Archived:    revision.Archived,
		SortOrder:   revision.SortOrder,
		Stock:       revision.Stock,
	}, groupIDs); err != nil {
		return nil, err
	}
	if _, err := ensureCurrentPayTokenProductRevisionTx(tx, productID); err != nil {
		return nil, err
	}
	return getPayTokenProductSnapshotTx(tx, productID)
}

func BackfillPayProductRevisions(tx *gorm.DB) error {
	if tx == nil {
		tx = DB
	}
	if tx == nil {
		return nil
	}
	if tx.Migrator().HasTable(&PaygProduct{}) && tx.Migrator().HasTable(&PaygProductRevision{}) {
		var products []PaygProduct
		if err := tx.Select("id").Find(&products).Error; err != nil {
			return err
		}
		for _, product := range products {
			if product.Id <= 0 {
				continue
			}
			if _, err := ensureCurrentPaygProductRevisionTx(tx, product.Id); err != nil {
				return err
			}
		}
	}
	if tx.Migrator().HasTable(&PayRequestProduct{}) && tx.Migrator().HasTable(&PayRequestProductRevision{}) {
		var products []PayRequestProduct
		if err := tx.Select("id").Find(&products).Error; err != nil {
			return err
		}
		for _, product := range products {
			if product.Id <= 0 {
				continue
			}
			if _, err := ensureCurrentPayRequestProductRevisionTx(tx, product.Id); err != nil {
				return err
			}
		}
	}
	if tx.Migrator().HasTable(&PayTokenProduct{}) && tx.Migrator().HasTable(&PayTokenProductRevision{}) {
		var products []PayTokenProduct
		if err := tx.Select("id").Find(&products).Error; err != nil {
			return err
		}
		for _, product := range products {
			if product.Id <= 0 {
				continue
			}
			if _, err := ensureCurrentPayTokenProductRevisionTx(tx, product.Id); err != nil {
				return err
			}
		}
	}
	return nil
}

func ListPaygProductsForOptionTx(tx *gorm.DB) ([]payg_setting.PaygProduct, error) {
	if tx == nil {
		tx = DB
	}
	var products []PaygProduct
	if err := tx.Order("sort_order DESC, id DESC").Find(&products).Error; err != nil {
		return nil, err
	}
	result := make([]payg_setting.PaygProduct, 0, len(products))
	for _, product := range products {
		groupIDs, err := getPaygProductGroupIDsTx(tx, product.Id)
		if err != nil {
			return nil, err
		}
		result = append(result, payg_setting.PaygProduct{
			Id:              product.Id,
			Name:            product.Name,
			Description:     product.Description,
			Enabled:         product.Enabled,
			Archived:        product.Archived,
			SortOrder:       product.SortOrder,
			Stock:           product.Stock,
			AllowedGroupIds: groupIDs,
		})
	}
	return result, nil
}

func ListPayRequestProductsForOptionTx(tx *gorm.DB) ([]payg_setting.PayRequestProduct, error) {
	if tx == nil {
		tx = DB
	}
	var products []PayRequestProduct
	if err := tx.Order("sort_order DESC, id DESC").Find(&products).Error; err != nil {
		return nil, err
	}
	result := make([]payg_setting.PayRequestProduct, 0, len(products))
	for _, product := range products {
		groupIDs, err := getPayRequestProductGroupIDsTx(tx, product.Id)
		if err != nil {
			return nil, err
		}
		result = append(result, payg_setting.PayRequestProduct{
			Id:              product.Id,
			Name:            product.Name,
			Description:     product.Description,
			Enabled:         product.Enabled,
			Archived:        product.Archived,
			SortOrder:       product.SortOrder,
			Stock:           product.Stock,
			AllowedGroupIds: groupIDs,
		})
	}
	return result, nil
}

func ListPayTokenProductsForOptionTx(tx *gorm.DB) ([]payg_setting.PayTokenProduct, error) {
	if tx == nil {
		tx = DB
	}
	var products []PayTokenProduct
	if err := tx.Order("sort_order DESC, id DESC").Find(&products).Error; err != nil {
		return nil, err
	}
	result := make([]payg_setting.PayTokenProduct, 0, len(products))
	for _, product := range products {
		groupIDs, err := getPayTokenProductGroupIDsTx(tx, product.Id)
		if err != nil {
			return nil, err
		}
		result = append(result, payg_setting.PayTokenProduct{
			Id:              product.Id,
			Name:            product.Name,
			Description:     product.Description,
			Enabled:         product.Enabled,
			Archived:        product.Archived,
			SortOrder:       product.SortOrder,
			Stock:           product.Stock,
			AllowedGroupIds: groupIDs,
		})
	}
	return result, nil
}

func SyncPaygProductsOptionFromDB() error {
	products, err := ListPaygProductsForOptionTx(DB)
	if err != nil {
		return err
	}
	b, err := json.Marshal(products)
	if err != nil {
		return err
	}
	return UpdateOption("payg.products", string(b))
}

func SyncPayRequestProductsOptionFromDB() error {
	products, err := ListPayRequestProductsForOptionTx(DB)
	if err != nil {
		return err
	}
	b, err := json.Marshal(products)
	if err != nil {
		return err
	}
	return UpdateOption("payg.pay_request_products", string(b))
}

func SyncPayTokenProductsOptionFromDB() error {
	products, err := ListPayTokenProductsForOptionTx(DB)
	if err != nil {
		return err
	}
	b, err := json.Marshal(products)
	if err != nil {
		return err
	}
	return UpdateOption("payg.pay_token_products", string(b))
}
