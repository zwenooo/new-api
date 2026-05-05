package model

import "errors"

type SubscriptionPlan struct {
	Id           int    `json:"id" gorm:"primaryKey"`
	Name         string `json:"name" gorm:"type:varchar(100);not null"`
	Description  string `json:"description" gorm:"type:varchar(255);default:''"`
	PriceFen     int64  `json:"price_fen" gorm:"type:bigint;not null"`
	DurationDays int    `json:"duration_days" gorm:"type:int;not null"`
	Meta         string `json:"meta" gorm:"type:text"`
	Enabled      bool   `json:"enabled" gorm:"type:boolean;default:true;index"`
	SortOrder    int    `json:"sort_order" gorm:"type:int;default:0;index"`
	CreatedAt    int64  `json:"created_at" gorm:"bigint;autoCreateTime"`
	UpdatedAt    int64  `json:"updated_at" gorm:"bigint;autoUpdateTime"`
}

func (plan *SubscriptionPlan) Validate() error {
	if plan == nil {
		return errors.New("plan 为空")
	}
	if plan.Name == "" {
		return errors.New("plan name 不能为空")
	}
	if plan.PriceFen <= 0 {
		return errors.New("plan price_fen 必须大于0")
	}
	if plan.DurationDays <= 0 {
		return errors.New("plan duration_days 必须大于0")
	}
	return nil
}

func (plan *SubscriptionPlan) Insert() error {
	if err := plan.Validate(); err != nil {
		return err
	}
	return DB.Create(plan).Error
}

func (plan *SubscriptionPlan) Update() error {
	if plan.Id <= 0 {
		return errors.New("plan id 无效")
	}
	if err := plan.Validate(); err != nil {
		return err
	}
	return DB.Save(plan).Error
}

func GetSubscriptionPlanById(id int) (*SubscriptionPlan, error) {
	if id <= 0 {
		return nil, errors.New("id 无效")
	}
	var plan SubscriptionPlan
	if err := DB.Where("id = ?", id).First(&plan).Error; err != nil {
		return nil, err
	}
	return &plan, nil
}

func GetAllSubscriptionPlans() ([]*SubscriptionPlan, error) {
	var plans []*SubscriptionPlan
	if err := DB.Order("sort_order desc, id desc").Find(&plans).Error; err != nil {
		return nil, err
	}
	return plans, nil
}

func GetEnabledSubscriptionPlans() ([]*SubscriptionPlan, error) {
	var plans []*SubscriptionPlan
	if err := DB.Where("enabled = ?", true).Order("sort_order desc, id desc").Find(&plans).Error; err != nil {
		return nil, err
	}
	return plans, nil
}

func DeleteSubscriptionPlanById(id int) error {
	if id <= 0 {
		return errors.New("id 无效")
	}
	return DB.Delete(&SubscriptionPlan{}, "id = ?", id).Error
}

