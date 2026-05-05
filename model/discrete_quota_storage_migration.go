package model

import (
	"errors"
	"fmt"
	"strings"

	"one-api/billing"

	"gorm.io/gorm"
)

const (
	optionKeyDiscreteQuotaStorageScaledV1       = "billing.discrete_quota_storage_scaled_v1"
	optionKeyDiscreteQuotaStorageSchemaBigintV1 = "billing.discrete_quota_storage_schema_bigint_v1"

	optionKeyDiscreteQuotaStorageScaledUsersV1                    = "billing.discrete_quota_storage_scaled_v1.users"
	optionKeyDiscreteQuotaStorageScaledUserSubscriptionsV1        = "billing.discrete_quota_storage_scaled_v1.user_subscriptions"
	optionKeyDiscreteQuotaStorageScaledUserSubGroupDailyLimitsV1  = "billing.discrete_quota_storage_scaled_v1.user_subscription_group_daily_limits"
	optionKeyDiscreteQuotaStorageScaledUserSubGroupDailyUsagesV1  = "billing.discrete_quota_storage_scaled_v1.user_subscription_group_daily_usages"
	optionKeyDiscreteQuotaStorageScaledUserRequestSubscriptionsV1 = "billing.discrete_quota_storage_scaled_v1.user_request_subscriptions"
	optionKeyDiscreteQuotaStorageScaledPayRequestUserBalancesV1   = "billing.discrete_quota_storage_scaled_v1.pay_request_user_balances"
	optionKeyDiscreteQuotaStorageScaledPayTokenUserBalancesV1     = "billing.discrete_quota_storage_scaled_v1.pay_token_user_balances"
)

type discreteQuotaMigrationStep struct {
	markerKey string
	run       func(tx *gorm.DB, scale int) error
}

func ensureDiscreteQuotaColumnsBigint(tx *gorm.DB) error {
	if tx == nil {
		tx = DB
	}
	type alterTarget struct {
		model interface{}
		field string
	}
	targets := []alterTarget{
		{model: &User{}, field: "TokensQuota"},
		{model: &User{}, field: "TokensHistoryQuota"},
		{model: &User{}, field: "PayRequestQuota"},
		{model: &User{}, field: "PayRequestHistoryQuota"},
		{model: &User{}, field: "PayTokenQuota"},
		{model: &User{}, field: "PayTokenHistoryQuota"},
		{model: &UserSubscription{}, field: "TotalQuota"},
		{model: &UserSubscription{}, field: "RemainingQuota"},
		{model: &UserSubscription{}, field: "DailyQuotaLimit"},
		{model: &UserSubscription{}, field: "DailyQuotaUsed"},
		{model: &UserRequestSubscription{}, field: "DailyRequestLimit"},
		{model: &UserRequestSubscription{}, field: "DailyRequestUsed"},
		{model: &UserRequestSubscription{}, field: "TotalRequestLimit"},
		{model: &UserRequestSubscription{}, field: "TotalRequestUsed"},
		{model: &UserSubscriptionGroupDailyLimit{}, field: "DailyLimitQuota"},
		{model: &UserSubscriptionGroupDailyUsage{}, field: "UsedQuota"},
		{model: &PayRequestUserBalance{}, field: "RemainingRequests"},
		{model: &PayRequestUserBalance{}, field: "HistoryRequests"},
		{model: &PayTokenUserBalance{}, field: "RemainingTokens"},
		{model: &PayTokenUserBalance{}, field: "HistoryTokens"},
	}
	for _, target := range targets {
		if !tx.Migrator().HasColumn(target.model, target.field) {
			continue
		}
		if err := tx.Migrator().AlterColumn(target.model, target.field); err != nil {
			return err
		}
	}
	return nil
}

func discreteQuotaMigrationMarkerIsTrue(tx *gorm.DB, key string) (bool, error) {
	if tx == nil {
		tx = DB
	}
	if tx == nil || !tx.Migrator().HasTable(&Option{}) {
		return false, nil
	}
	var marker Option
	err := tx.Where(commonKeyCol+" = ?", key).First(&marker).Error
	if err == nil {
		return strings.TrimSpace(marker.Value) == "true", nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	return false, err
}

func setDiscreteQuotaMigrationMarkerTrue(tx *gorm.DB, key string) error {
	if tx == nil {
		tx = DB
	}
	if tx == nil {
		return errors.New("nil db")
	}
	marker := Option{Key: key}
	if err := tx.FirstOrCreate(&marker, Option{Key: key}).Error; err != nil {
		return err
	}
	marker.Value = "true"
	return tx.Save(&marker).Error
}

func runDiscreteQuotaMigrationStep(tx *gorm.DB, step discreteQuotaMigrationStep, scale int) error {
	done, err := discreteQuotaMigrationMarkerIsTrue(tx, step.markerKey)
	if err != nil {
		return err
	}
	if done {
		return nil
	}
	if err := step.run(tx, scale); err != nil {
		return fmt.Errorf("%s: %w", step.markerKey, err)
	}
	return setDiscreteQuotaMigrationMarkerTrue(tx, step.markerKey)
}

func MigrateDiscreteQuotaStorageToScaledBigint(tx *gorm.DB) error {
	if tx == nil {
		tx = DB
	}
	if !tx.Migrator().HasTable(&Option{}) {
		return nil
	}

	migrated, err := discreteQuotaMigrationMarkerIsTrue(tx, optionKeyDiscreteQuotaStorageScaledV1)
	if err != nil {
		return err
	}
	if migrated {
		return nil
	}

	schemaReady, err := discreteQuotaMigrationMarkerIsTrue(tx, optionKeyDiscreteQuotaStorageSchemaBigintV1)
	if err != nil {
		return err
	}
	if !schemaReady {
		if err := ensureDiscreteQuotaColumnsBigint(tx); err != nil {
			return err
		}
		if err := setDiscreteQuotaMigrationMarkerTrue(tx, optionKeyDiscreteQuotaStorageSchemaBigintV1); err != nil {
			return err
		}
	}

	scale := billing.DiscreteQuotaScale

	steps := []discreteQuotaMigrationStep{
		{
			markerKey: optionKeyDiscreteQuotaStorageScaledUsersV1,
			run: func(tx *gorm.DB, scale int) error {
				if !tx.Migrator().HasTable(&User{}) {
					return nil
				}
				return tx.Model(&User{}).
					Where("tokens_quota <> 0 OR tokens_history_quota <> 0 OR pay_request_quota <> 0 OR pay_request_history_quota <> 0 OR pay_token_quota <> 0 OR pay_token_history_quota <> 0").
					Updates(map[string]interface{}{
						"tokens_quota":              gorm.Expr("tokens_quota * ?", scale),
						"tokens_history_quota":      gorm.Expr("tokens_history_quota * ?", scale),
						"pay_request_quota":         gorm.Expr("pay_request_quota * ?", scale),
						"pay_request_history_quota": gorm.Expr("pay_request_history_quota * ?", scale),
						"pay_token_quota":           gorm.Expr("pay_token_quota * ?", scale),
						"pay_token_history_quota":   gorm.Expr("pay_token_history_quota * ?", scale),
					}).Error
			},
		},
		{
			markerKey: optionKeyDiscreteQuotaStorageScaledUserSubscriptionsV1,
			run: func(tx *gorm.DB, scale int) error {
				if !tx.Migrator().HasTable(&UserSubscription{}) {
					return nil
				}
				return tx.Model(&UserSubscription{}).
					Where("billing_unit = ?", UserSubscriptionBillingUnitTokens).
					Updates(map[string]interface{}{
						"total_quota":       gorm.Expr("total_quota * ?", scale),
						"remaining_quota":   gorm.Expr("remaining_quota * ?", scale),
						"daily_quota_limit": gorm.Expr("daily_quota_limit * ?", scale),
						"daily_quota_used":  gorm.Expr("daily_quota_used * ?", scale),
					}).Error
			},
		},
		{
			markerKey: optionKeyDiscreteQuotaStorageScaledUserSubGroupDailyLimitsV1,
			run: func(tx *gorm.DB, scale int) error {
				if !tx.Migrator().HasTable(&UserSubscriptionGroupDailyLimit{}) || !tx.Migrator().HasTable(&UserSubscription{}) {
					return nil
				}
				tokenSubscriptionScope := tx.Model(&UserSubscription{}).
					Select("id").
					Where("billing_unit = ?", UserSubscriptionBillingUnitTokens)
				return tx.Model(&UserSubscriptionGroupDailyLimit{}).
					Where("subscription_id IN (?)", tokenSubscriptionScope).
					Update("daily_limit_quota", gorm.Expr("daily_limit_quota * ?", scale)).Error
			},
		},
		{
			markerKey: optionKeyDiscreteQuotaStorageScaledUserSubGroupDailyUsagesV1,
			run: func(tx *gorm.DB, scale int) error {
				if !tx.Migrator().HasTable(&UserSubscriptionGroupDailyUsage{}) || !tx.Migrator().HasTable(&UserSubscription{}) {
					return nil
				}
				tokenSubscriptionScope := tx.Model(&UserSubscription{}).
					Select("id").
					Where("billing_unit = ?", UserSubscriptionBillingUnitTokens)
				return tx.Model(&UserSubscriptionGroupDailyUsage{}).
					Where("subscription_id IN (?)", tokenSubscriptionScope).
					Update("used_quota", gorm.Expr("used_quota * ?", scale)).Error
			},
		},
		{
			markerKey: optionKeyDiscreteQuotaStorageScaledUserRequestSubscriptionsV1,
			run: func(tx *gorm.DB, scale int) error {
				if !tx.Migrator().HasTable(&UserRequestSubscription{}) {
					return nil
				}
				return tx.Model(&UserRequestSubscription{}).
					Where("daily_request_limit <> 0 OR daily_request_used <> 0 OR total_request_limit <> 0 OR total_request_used <> 0").
					Updates(map[string]interface{}{
						"daily_request_limit": gorm.Expr("daily_request_limit * ?", scale),
						"daily_request_used":  gorm.Expr("daily_request_used * ?", scale),
						"total_request_limit": gorm.Expr("total_request_limit * ?", scale),
						"total_request_used":  gorm.Expr("total_request_used * ?", scale),
					}).Error
			},
		},
		{
			markerKey: optionKeyDiscreteQuotaStorageScaledPayRequestUserBalancesV1,
			run: func(tx *gorm.DB, scale int) error {
				if !tx.Migrator().HasTable(&PayRequestUserBalance{}) {
					return nil
				}
				return tx.Model(&PayRequestUserBalance{}).
					Where("remaining_requests <> 0 OR history_requests <> 0").
					Updates(map[string]interface{}{
						"remaining_requests": gorm.Expr("remaining_requests * ?", scale),
						"history_requests":   gorm.Expr("history_requests * ?", scale),
					}).Error
			},
		},
		{
			markerKey: optionKeyDiscreteQuotaStorageScaledPayTokenUserBalancesV1,
			run: func(tx *gorm.DB, scale int) error {
				if !tx.Migrator().HasTable(&PayTokenUserBalance{}) {
					return nil
				}
				return tx.Model(&PayTokenUserBalance{}).
					Where("remaining_tokens <> 0 OR history_tokens <> 0").
					Updates(map[string]interface{}{
						"remaining_tokens": gorm.Expr("remaining_tokens * ?", scale),
						"history_tokens":   gorm.Expr("history_tokens * ?", scale),
					}).Error
			},
		},
	}
	for _, step := range steps {
		if err := runDiscreteQuotaMigrationStep(tx, step, scale); err != nil {
			return err
		}
	}
	return setDiscreteQuotaMigrationMarkerTrue(tx, optionKeyDiscreteQuotaStorageScaledV1)
}
