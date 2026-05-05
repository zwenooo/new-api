package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"one-api/billing"
	"one-api/common"
	"one-api/setting/ratio_setting"
	"one-api/types"

	"gorm.io/gorm"
)

const (
	CustomerTypeRetail   = "retail"
	CustomerTypeReseller = "reseller"

	defaultRetailPricingProfileCode = "retail"
	pricingRuleCacheTTL             = time.Minute
)

type PricingProfile struct {
	Id            int       `json:"id" gorm:"primaryKey;autoIncrement"`
	Code          string    `json:"code" gorm:"type:varchar(64);not null;uniqueIndex:idx_pricing_profiles_code"`
	Name          string    `json:"name" gorm:"type:varchar(64);not null"`
	Audience      string    `json:"audience" gorm:"type:varchar(32);not null;default:'retail';index"`
	DefaultFactor float64   `json:"default_factor" gorm:"type:double precision;not null;default:1"`
	Enabled       bool      `json:"enabled" gorm:"type:boolean;not null;default:true;index"`
	Description   string    `json:"description" gorm:"type:text"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func (PricingProfile) TableName() string {
	return "pricing_profiles"
}

type PricingProfileGroupFactor struct {
	Id        int       `json:"id" gorm:"primaryKey;autoIncrement"`
	ProfileId int       `json:"profile_id" gorm:"not null;uniqueIndex:idx_pricing_profile_group_factors_profile_group"`
	GroupId   int       `json:"group_id" gorm:"not null;uniqueIndex:idx_pricing_profile_group_factors_profile_group"`
	Factor    float64   `json:"factor" gorm:"type:double precision;not null;default:1"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (PricingProfileGroupFactor) TableName() string {
	return "pricing_profile_group_factors"
}

type UserGroupPriceOverride struct {
	Id        int       `json:"id" gorm:"primaryKey;autoIncrement"`
	UserId    int       `json:"user_id" gorm:"not null;uniqueIndex:idx_user_group_price_overrides_user_group;index"`
	GroupId   int       `json:"group_id" gorm:"not null;uniqueIndex:idx_user_group_price_overrides_user_group"`
	Factor    float64   `json:"factor" gorm:"type:double precision;not null;default:1"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (UserGroupPriceOverride) TableName() string {
	return "user_group_price_overrides"
}

type PriceGroupFactor struct {
	GroupId int     `json:"group_id"`
	Factor  float64 `json:"factor"`
}

type GroupUserPriceOverrideEntry struct {
	UserId      int     `json:"user_id"`
	Username    string  `json:"username"`
	DisplayName string  `json:"display_name"`
	Email       string  `json:"email"`
	Factor      float64 `json:"factor"`
}

type SaveGroupUserPriceOverrideEntry struct {
	UserId int     `json:"user_id"`
	Factor float64 `json:"factor"`
}

type PricingProfileDetail struct {
	PricingProfile
	GroupFactors []PriceGroupFactor `json:"group_factors"`
	UserCount    int64              `json:"user_count"`
}

type LegacyPricingUser struct {
	Id                    int     `json:"id"`
	Username              string  `json:"username"`
	DisplayName           string  `json:"display_name"`
	GroupId               int     `json:"group_id"`
	GroupLabel            string  `json:"group_label"`
	BaseMultiplier        float64 `json:"base_multiplier"`
	BaseMultiplierApplied bool    `json:"base_multiplier_applied"`
	LegacyTargetCount     int     `json:"legacy_target_count"`
	EffectiveSource       string  `json:"effective_source"`
	CustomerType          string  `json:"customer_type"`
	PricingProfileId      int     `json:"pricing_profile_id"`
	PricingProfileLabel   string  `json:"pricing_profile_label"`
}

type UserGroupPricingPreview struct {
	GroupId          int     `json:"group_id"`
	PublicRatio      float64 `json:"public_ratio"`
	EffectiveRatio   float64 `json:"effective_ratio"`
	AppliedFactor    float64 `json:"applied_factor"`
	Source           string  `json:"source"`
	PricingProfileId int     `json:"pricing_profile_id,omitempty"`
}

func resolveUserAudienceGroupID(user *UserBase) int {
	if user == nil {
		return 0
	}
	if user.UserGroupId > 0 {
		return user.UserGroupId
	}
	return user.GroupId
}

func resolveStoredUserAudienceGroupID(user *User) int {
	if user == nil {
		return 0
	}
	if user.UserGroupId > 0 {
		return user.UserGroupId
	}
	return user.GroupId
}

var (
	pricingRuleCacheMu                  sync.RWMutex
	pricingRuleCacheSyncedAt            time.Time
	pricingRuleProfilesByID             = map[int]PricingProfile{}
	pricingRuleProfileFactorsByProfile  = map[int]map[int]float64{}
	pricingRuleUserOverridesByUser      = map[int]map[int]float64{}
	defaultRetailPricingProfileCachedID int
)

func normalizeCustomerType(value string) (string, error) {
	v := strings.TrimSpace(strings.ToLower(value))
	if v == "" {
		return CustomerTypeRetail, nil
	}
	switch v {
	case CustomerTypeRetail, CustomerTypeReseller:
		return v, nil
	default:
		return "", fmt.Errorf("客户类型无效: %s", value)
	}
}

func validatePricingFactor(field string, factor float64) error {
	if math.IsNaN(factor) || math.IsInf(factor, 0) {
		return fmt.Errorf("%s 必须为有限数字", field)
	}
	if factor <= 0 {
		return fmt.Errorf("%s 必须大于 0", field)
	}
	return nil
}

func normalizePriceGroupFactors(factors []PriceGroupFactor) ([]PriceGroupFactor, error) {
	if len(factors) == 0 {
		return []PriceGroupFactor{}, nil
	}
	out := make([]PriceGroupFactor, 0, len(factors))
	seen := make(map[int]struct{}, len(factors))
	for _, item := range factors {
		groupID := item.GroupId
		if groupID <= 0 {
			return nil, errors.New("分组 id 无效")
		}
		if _, ok := seen[groupID]; ok {
			return nil, fmt.Errorf("分组 %d 重复", groupID)
		}
		if err := validatePricingFactor("倍率", item.Factor); err != nil {
			return nil, err
		}
		seen[groupID] = struct{}{}
		out = append(out, PriceGroupFactor{
			GroupId: groupID,
			Factor:  item.Factor,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].GroupId < out[j].GroupId
	})
	return out, nil
}

func ParsePriceGroupFactorsJSON(raw JSONValue) ([]PriceGroupFactor, error) {
	if len(raw) == 0 {
		return []PriceGroupFactor{}, nil
	}
	var factors []PriceGroupFactor
	if err := json.Unmarshal(raw, &factors); err != nil {
		return nil, err
	}
	return normalizePriceGroupFactors(factors)
}

func MarshalPriceGroupFactorsJSON(factors []PriceGroupFactor) (JSONValue, error) {
	normalized, err := normalizePriceGroupFactors(factors)
	if err != nil {
		return nil, err
	}
	b, err := json.Marshal(normalized)
	if err != nil {
		return nil, err
	}
	return JSONValue(b), nil
}

func pricingProfileExistsTx(tx *gorm.DB, profileID int) (*PricingProfile, error) {
	if tx == nil {
		tx = DB
	}
	if tx == nil {
		return nil, errors.New("db 未初始化")
	}
	if profileID <= 0 {
		return nil, errors.New("价格模板 id 无效")
	}
	var profile PricingProfile
	if err := tx.Where("id = ?", profileID).First(&profile).Error; err != nil {
		return nil, err
	}
	return &profile, nil
}

func validatePricingProfileForUserTx(tx *gorm.DB, customerType string, profileID int) error {
	if profileID <= 0 {
		return nil
	}
	profile, err := pricingProfileExistsTx(tx, profileID)
	if err != nil {
		return err
	}
	if !profile.Enabled {
		return errors.New("价格模板已停用")
	}
	if profile.Audience != "" && customerType != "" && profile.Audience != customerType {
		return fmt.Errorf("价格模板 %s 不适用于客户类型 %s", profile.Name, customerType)
	}
	return nil
}

func ensureDefaultRetailPricingProfileTx(tx *gorm.DB) (int, error) {
	if tx == nil {
		tx = DB
	}
	if tx == nil {
		return 0, errors.New("db 未初始化")
	}
	if !tx.Migrator().HasTable(&PricingProfile{}) {
		return 0, nil
	}
	var profile PricingProfile
	if err := tx.Where("code = ?", defaultRetailPricingProfileCode).First(&profile).Error; err == nil {
		return profile.Id, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, err
	}
	profile = PricingProfile{
		Code:          defaultRetailPricingProfileCode,
		Name:          "零售默认",
		Audience:      CustomerTypeRetail,
		DefaultFactor: 1,
		Enabled:       true,
		Description:   "默认零售基准价",
	}
	if err := tx.Create(&profile).Error; err != nil {
		return 0, err
	}
	return profile.Id, nil
}

func BackfillUserPricingProfiles(tx *gorm.DB) error {
	if tx == nil {
		tx = DB
	}
	if tx == nil {
		return nil
	}
	if !tx.Migrator().HasTable(&PricingProfile{}) || !tx.Migrator().HasTable(&User{}) {
		return nil
	}
	if _, err := ensureDefaultRetailPricingProfileTx(tx); err != nil {
		return err
	}
	if tx.Migrator().HasColumn(&User{}, "customer_type") {
		if err := tx.Model(&User{}).
			Where("customer_type IS NULL OR customer_type = ''").
			Update("customer_type", CustomerTypeRetail).Error; err != nil {
			return err
		}
	}
	return refreshPricingRuleCacheTx(tx)
}

func RefreshPricingRuleCache() error {
	if DB == nil {
		return nil
	}
	return refreshPricingRuleCacheTx(DB)
}

func refreshPricingRuleCacheTx(tx *gorm.DB) error {
	if tx == nil {
		tx = DB
	}
	if tx == nil {
		return nil
	}

	profilesByID := make(map[int]PricingProfile)
	profileFactors := make(map[int]map[int]float64)
	userOverrides := make(map[int]map[int]float64)
	retailID := 0

	if tx.Migrator().HasTable(&PricingProfile{}) {
		var profiles []PricingProfile
		if err := tx.Order("id ASC").Find(&profiles).Error; err != nil {
			return err
		}
		for _, profile := range profiles {
			profilesByID[profile.Id] = profile
			if strings.EqualFold(strings.TrimSpace(profile.Code), defaultRetailPricingProfileCode) && retailID == 0 {
				retailID = profile.Id
			}
		}
	}

	if tx.Migrator().HasTable(&PricingProfileGroupFactor{}) {
		var factors []PricingProfileGroupFactor
		if err := tx.Order("profile_id ASC, group_id ASC").Find(&factors).Error; err != nil {
			return err
		}
		for _, factor := range factors {
			if factor.ProfileId <= 0 || factor.GroupId <= 0 {
				continue
			}
			if _, ok := profileFactors[factor.ProfileId]; !ok {
				profileFactors[factor.ProfileId] = make(map[int]float64)
			}
			profileFactors[factor.ProfileId][factor.GroupId] = factor.Factor
		}
	}

	if tx.Migrator().HasTable(&UserGroupPriceOverride{}) {
		var overrides []UserGroupPriceOverride
		if err := tx.Order("user_id ASC, group_id ASC").Find(&overrides).Error; err != nil {
			return err
		}
		for _, override := range overrides {
			if override.UserId <= 0 || override.GroupId <= 0 {
				continue
			}
			if _, ok := userOverrides[override.UserId]; !ok {
				userOverrides[override.UserId] = make(map[int]float64)
			}
			userOverrides[override.UserId][override.GroupId] = override.Factor
		}
	}

	pricingRuleCacheMu.Lock()
	pricingRuleProfilesByID = profilesByID
	pricingRuleProfileFactorsByProfile = profileFactors
	pricingRuleUserOverridesByUser = userOverrides
	defaultRetailPricingProfileCachedID = retailID
	pricingRuleCacheSyncedAt = time.Now()
	pricingRuleCacheMu.Unlock()
	return nil
}

func ensurePricingRuleCacheFresh() {
	pricingRuleCacheMu.RLock()
	needsRefresh := len(pricingRuleProfilesByID) == 0 || time.Since(pricingRuleCacheSyncedAt) > pricingRuleCacheTTL
	pricingRuleCacheMu.RUnlock()
	if !needsRefresh {
		return
	}
	if err := RefreshPricingRuleCache(); err != nil {
		common.SysLog("failed to refresh pricing rule cache: " + err.Error())
	}
}

func GetDefaultRetailPricingProfileID() int {
	ensurePricingRuleCacheFresh()
	pricingRuleCacheMu.RLock()
	defer pricingRuleCacheMu.RUnlock()
	return defaultRetailPricingProfileCachedID
}

func GetPricingProfileLabel(profileID int) (string, bool) {
	if profileID <= 0 {
		return "", false
	}
	ensurePricingRuleCacheFresh()
	pricingRuleCacheMu.RLock()
	profile, ok := pricingRuleProfilesByID[profileID]
	pricingRuleCacheMu.RUnlock()
	if !ok {
		return "", false
	}
	label := strings.TrimSpace(profile.Name)
	if label == "" {
		label = strings.TrimSpace(profile.Code)
	}
	if label == "" {
		return "", false
	}
	return label, true
}

func listPricingProfileGroupFactorsTx(tx *gorm.DB, profileIDs []int) (map[int][]PriceGroupFactor, error) {
	out := make(map[int][]PriceGroupFactor)
	if tx == nil {
		tx = DB
	}
	if tx == nil || len(profileIDs) == 0 || !tx.Migrator().HasTable(&PricingProfileGroupFactor{}) {
		return out, nil
	}
	ids := NormalizeUniqueSortedIDs(profileIDs)
	var rows []PricingProfileGroupFactor
	if err := tx.Where("profile_id IN ?", ids).Order("profile_id ASC, group_id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		if row.ProfileId <= 0 || row.GroupId <= 0 {
			continue
		}
		out[row.ProfileId] = append(out[row.ProfileId], PriceGroupFactor{
			GroupId: row.GroupId,
			Factor:  row.Factor,
		})
	}
	return out, nil
}

func ListPricingProfiles(tx *gorm.DB) ([]PricingProfileDetail, error) {
	if tx == nil {
		tx = DB
	}
	if tx == nil {
		return []PricingProfileDetail{}, nil
	}
	if !tx.Migrator().HasTable(&PricingProfile{}) {
		return []PricingProfileDetail{}, nil
	}

	var profiles []PricingProfile
	if err := tx.Order("id ASC").Find(&profiles).Error; err != nil {
		return nil, err
	}
	profileIDs := make([]int, 0, len(profiles))
	for _, profile := range profiles {
		if profile.Id > 0 {
			profileIDs = append(profileIDs, profile.Id)
		}
	}
	factorMap, err := listPricingProfileGroupFactorsTx(tx, profileIDs)
	if err != nil {
		return nil, err
	}

	userCountByProfile := make(map[int]int64)
	if len(profileIDs) > 0 && tx.Migrator().HasTable(&User{}) {
		type row struct {
			PricingProfileId int   `gorm:"column:pricing_profile_id"`
			Count            int64 `gorm:"column:count"`
		}
		var rows []row
		if err := tx.Model(&User{}).
			Select("pricing_profile_id, COUNT(*) AS count").
			Where("pricing_profile_id IN ?", profileIDs).
			Group("pricing_profile_id").
			Scan(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			if row.PricingProfileId > 0 {
				userCountByProfile[row.PricingProfileId] = row.Count
			}
		}
	}

	items := make([]PricingProfileDetail, 0, len(profiles))
	for _, profile := range profiles {
		items = append(items, PricingProfileDetail{
			PricingProfile: profile,
			GroupFactors:   factorMap[profile.Id],
			UserCount:      userCountByProfile[profile.Id],
		})
	}
	return items, nil
}

type SavePricingProfileParams struct {
	Code          string
	Name          string
	Audience      string
	DefaultFactor float64
	Enabled       bool
	Description   string
	GroupFactors  []PriceGroupFactor
}

func normalizeSavePricingProfileParams(params SavePricingProfileParams, existing *PricingProfile) (SavePricingProfileParams, error) {
	params.Code = strings.TrimSpace(params.Code)
	if params.Code == "" {
		return params, errors.New("模板编码不能为空")
	}
	params.Name = strings.TrimSpace(params.Name)
	if params.Name == "" {
		params.Name = params.Code
	}
	audience, err := normalizeCustomerType(params.Audience)
	if err != nil {
		return params, err
	}
	params.Audience = audience
	if err := validatePricingFactor("默认倍率", params.DefaultFactor); err != nil {
		return params, err
	}
	params.Description = strings.TrimSpace(params.Description)
	params.GroupFactors, err = normalizePriceGroupFactors(params.GroupFactors)
	if err != nil {
		return params, err
	}
	_ = existing
	return params, nil
}

func CreatePricingProfile(tx *gorm.DB, params SavePricingProfileParams) (*PricingProfileDetail, error) {
	if tx == nil {
		tx = DB
	}
	if tx == nil {
		return nil, errors.New("db 未初始化")
	}
	params, err := normalizeSavePricingProfileParams(params, nil)
	if err != nil {
		return nil, err
	}
	if len(params.GroupFactors) > 0 {
		if err := ValidateGroupIDsExist(tx, extractPriceGroupFactorIDs(params.GroupFactors)); err != nil {
			return nil, err
		}
	}

	var createdID int
	if err := tx.Transaction(func(tx *gorm.DB) error {
		profile := PricingProfile{
			Code:          params.Code,
			Name:          params.Name,
			Audience:      params.Audience,
			DefaultFactor: params.DefaultFactor,
			Enabled:       params.Enabled,
			Description:   params.Description,
		}
		if err := tx.Create(&profile).Error; err != nil {
			return err
		}
		if err := replacePricingProfileGroupFactorsTx(tx, profile.Id, params.GroupFactors); err != nil {
			return err
		}
		createdID = profile.Id
		return nil
	}); err != nil {
		return nil, err
	}
	if err := RefreshPricingRuleCache(); err != nil {
		return nil, err
	}
	return GetPricingProfileByID(nil, createdID)
}

func UpdatePricingProfile(tx *gorm.DB, id int, params SavePricingProfileParams) (*PricingProfileDetail, error) {
	if tx == nil {
		tx = DB
	}
	if tx == nil {
		return nil, errors.New("db 未初始化")
	}
	existing, err := pricingProfileExistsTx(tx, id)
	if err != nil {
		return nil, err
	}
	params, err = normalizeSavePricingProfileParams(params, existing)
	if err != nil {
		return nil, err
	}
	if len(params.GroupFactors) > 0 {
		if err := ValidateGroupIDsExist(tx, extractPriceGroupFactorIDs(params.GroupFactors)); err != nil {
			return nil, err
		}
	}

	if err := tx.Transaction(func(tx *gorm.DB) error {
		updates := map[string]interface{}{
			"code":           params.Code,
			"name":           params.Name,
			"audience":       params.Audience,
			"default_factor": params.DefaultFactor,
			"enabled":        params.Enabled,
			"description":    params.Description,
		}
		if err := tx.Model(&PricingProfile{}).Where("id = ?", id).Updates(updates).Error; err != nil {
			return err
		}
		return replacePricingProfileGroupFactorsTx(tx, id, params.GroupFactors)
	}); err != nil {
		return nil, err
	}
	if err := RefreshPricingRuleCache(); err != nil {
		return nil, err
	}
	return GetPricingProfileByID(nil, id)
}

func GetPricingProfileByID(tx *gorm.DB, id int) (*PricingProfileDetail, error) {
	if tx == nil {
		tx = DB
	}
	if tx == nil {
		return nil, errors.New("db 未初始化")
	}
	profile, err := pricingProfileExistsTx(tx, id)
	if err != nil {
		return nil, err
	}
	factorMap, err := listPricingProfileGroupFactorsTx(tx, []int{id})
	if err != nil {
		return nil, err
	}
	var userCount int64
	if tx.Migrator().HasTable(&User{}) {
		if err := tx.Model(&User{}).Where("pricing_profile_id = ?", id).Count(&userCount).Error; err != nil {
			return nil, err
		}
	}
	return &PricingProfileDetail{
		PricingProfile: *profile,
		GroupFactors:   factorMap[id],
		UserCount:      userCount,
	}, nil
}

func DeletePricingProfile(tx *gorm.DB, id int) error {
	if tx == nil {
		tx = DB
	}
	if tx == nil {
		return errors.New("db 未初始化")
	}
	profile, err := pricingProfileExistsTx(tx, id)
	if err != nil {
		return err
	}
	if strings.EqualFold(strings.TrimSpace(profile.Code), defaultRetailPricingProfileCode) {
		return errors.New("默认零售模板不能删除")
	}
	var userCount int64
	if tx.Migrator().HasTable(&User{}) {
		if err := tx.Model(&User{}).Where("pricing_profile_id = ?", id).Count(&userCount).Error; err != nil {
			return err
		}
	}
	if userCount > 0 {
		return errors.New("价格模板仍在使用中")
	}
	if err := tx.Transaction(func(tx *gorm.DB) error {
		if tx.Migrator().HasTable(&PricingProfileGroupFactor{}) {
			if err := tx.Where("profile_id = ?", id).Delete(&PricingProfileGroupFactor{}).Error; err != nil {
				return err
			}
		}
		return tx.Where("id = ?", id).Delete(&PricingProfile{}).Error
	}); err != nil {
		return err
	}
	return RefreshPricingRuleCache()
}

func extractPriceGroupFactorIDs(factors []PriceGroupFactor) []int {
	ids := make([]int, 0, len(factors))
	for _, factor := range factors {
		if factor.GroupId > 0 {
			ids = append(ids, factor.GroupId)
		}
	}
	return NormalizeUniqueSortedIDs(ids)
}

func replacePricingProfileGroupFactorsTx(tx *gorm.DB, profileID int, factors []PriceGroupFactor) error {
	if tx == nil {
		return errors.New("tx 为空")
	}
	if !tx.Migrator().HasTable(&PricingProfileGroupFactor{}) {
		return nil
	}
	if err := tx.Where("profile_id = ?", profileID).Delete(&PricingProfileGroupFactor{}).Error; err != nil {
		return err
	}
	if len(factors) == 0 {
		return nil
	}
	rows := make([]PricingProfileGroupFactor, 0, len(factors))
	for _, factor := range factors {
		rows = append(rows, PricingProfileGroupFactor{
			ProfileId: profileID,
			GroupId:   factor.GroupId,
			Factor:    factor.Factor,
		})
	}
	return tx.Create(&rows).Error
}

func ListUserGroupPriceOverrides(tx *gorm.DB, userID int) ([]PriceGroupFactor, error) {
	if tx == nil {
		tx = DB
	}
	if tx == nil || userID <= 0 || !tx.Migrator().HasTable(&UserGroupPriceOverride{}) {
		return []PriceGroupFactor{}, nil
	}
	var rows []UserGroupPriceOverride
	if err := tx.Where("user_id = ?", userID).Order("group_id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]PriceGroupFactor, 0, len(rows))
	for _, row := range rows {
		if row.GroupId <= 0 {
			continue
		}
		out = append(out, PriceGroupFactor{
			GroupId: row.GroupId,
			Factor:  row.Factor,
		})
	}
	return out, nil
}

func ListUserGroupPriceOverridesByGroup(tx *gorm.DB, groupID int) ([]GroupUserPriceOverrideEntry, error) {
	if tx == nil {
		tx = DB
	}
	if tx == nil || groupID <= 0 || !tx.Migrator().HasTable(&UserGroupPriceOverride{}) {
		return []GroupUserPriceOverrideEntry{}, nil
	}
	if err := ValidateGroupIDsExist(tx, []int{groupID}); err != nil {
		return nil, err
	}

	type row struct {
		UserId      int     `gorm:"column:user_id"`
		Username    string  `gorm:"column:username"`
		DisplayName string  `gorm:"column:display_name"`
		Email       string  `gorm:"column:email"`
		Factor      float64 `gorm:"column:factor"`
	}
	var rows []row
	if err := tx.Table("user_group_price_overrides AS ugo").
		Select("ugo.user_id, users.username, users.display_name, users.email, ugo.factor").
		Joins("JOIN users ON users.id = ugo.user_id AND users.deleted_at IS NULL").
		Where("ugo.group_id = ?", groupID).
		Order("ugo.user_id ASC").
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	items := make([]GroupUserPriceOverrideEntry, 0, len(rows))
	for _, row := range rows {
		if row.UserId <= 0 {
			continue
		}
		items = append(items, GroupUserPriceOverrideEntry{
			UserId:      row.UserId,
			Username:    strings.TrimSpace(row.Username),
			DisplayName: strings.TrimSpace(row.DisplayName),
			Email:       strings.TrimSpace(row.Email),
			Factor:      row.Factor,
		})
	}
	return items, nil
}

func SyncUserGroupPriceOverridesByGroup(tx *gorm.DB, groupID int, entries []SaveGroupUserPriceOverrideEntry) (int, []int, error) {
	if tx == nil {
		tx = DB
	}
	if tx == nil {
		return 0, nil, errors.New("db 未初始化")
	}
	if groupID <= 0 {
		return 0, nil, errors.New("分组 id 无效")
	}
	if err := ValidateGroupIDsExist(tx, []int{groupID}); err != nil {
		return 0, nil, err
	}
	if !tx.Migrator().HasTable(&UserGroupPriceOverride{}) {
		return 0, nil, nil
	}

	normalized := make([]SaveGroupUserPriceOverrideEntry, 0, len(entries))
	seen := make(map[int]struct{}, len(entries))
	for _, entry := range entries {
		if entry.UserId <= 0 {
			return 0, nil, errors.New("用户 id 无效")
		}
		if _, ok := seen[entry.UserId]; ok {
			return 0, nil, fmt.Errorf("用户 %d 重复", entry.UserId)
		}
		if err := validatePricingFactor("专属倍率", entry.Factor); err != nil {
			return 0, nil, err
		}
		seen[entry.UserId] = struct{}{}
		normalized = append(normalized, SaveGroupUserPriceOverrideEntry{
			UserId: entry.UserId,
			Factor: entry.Factor,
		})
	}

	oldUserIDs := make([]int, 0)
	if err := tx.Model(&UserGroupPriceOverride{}).
		Where("group_id = ?", groupID).
		Distinct("user_id").
		Pluck("user_id", &oldUserIDs).Error; err != nil {
		return 0, nil, err
	}

	if len(normalized) > 0 {
		userIDs := make([]int, 0, len(normalized))
		for _, entry := range normalized {
			userIDs = append(userIDs, entry.UserId)
		}
		userIDs = normalizeUniqueSortedIDs(userIDs)
		var existingUserIDs []int
		if err := tx.Model(&User{}).
			Where("id IN ? AND deleted_at IS NULL", userIDs).
			Pluck("id", &existingUserIDs).Error; err != nil {
			return 0, nil, err
		}
		existingSet := make(map[int]struct{}, len(existingUserIDs))
		for _, userID := range existingUserIDs {
			existingSet[userID] = struct{}{}
		}
		for _, userID := range userIDs {
			if _, ok := existingSet[userID]; ok {
				continue
			}
			return 0, nil, fmt.Errorf("用户不存在: %d", userID)
		}
	}

	affectedUserIDs := make(map[int]struct{}, len(oldUserIDs)+len(normalized))
	for _, userID := range oldUserIDs {
		if userID > 0 {
			affectedUserIDs[userID] = struct{}{}
		}
	}
	for _, entry := range normalized {
		if entry.UserId > 0 {
			affectedUserIDs[entry.UserId] = struct{}{}
		}
	}

	if err := tx.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("group_id = ?", groupID).Delete(&UserGroupPriceOverride{}).Error; err != nil {
			return err
		}
		if len(normalized) == 0 {
			return nil
		}
		rows := make([]UserGroupPriceOverride, 0, len(normalized))
		for _, entry := range normalized {
			rows = append(rows, UserGroupPriceOverride{
				UserId:  entry.UserId,
				GroupId: groupID,
				Factor:  entry.Factor,
			})
		}
		return tx.Create(&rows).Error
	}); err != nil {
		return 0, nil, err
	}

	ids := make([]int, 0, len(affectedUserIDs))
	for userID := range affectedUserIDs {
		ids = append(ids, userID)
	}
	sort.Ints(ids)
	return len(normalized), ids, nil
}

func replaceUserGroupPriceOverridesTx(tx *gorm.DB, userID int, factors []PriceGroupFactor) error {
	if tx == nil {
		return errors.New("tx 为空")
	}
	if userID <= 0 {
		return errors.New("userId 无效")
	}
	if !tx.Migrator().HasTable(&UserGroupPriceOverride{}) {
		return nil
	}
	if err := tx.Where("user_id = ?", userID).Delete(&UserGroupPriceOverride{}).Error; err != nil {
		return err
	}
	if len(factors) == 0 {
		return nil
	}
	rows := make([]UserGroupPriceOverride, 0, len(factors))
	for _, factor := range factors {
		rows = append(rows, UserGroupPriceOverride{
			UserId:  userID,
			GroupId: factor.GroupId,
			Factor:  factor.Factor,
		})
	}
	return tx.Create(&rows).Error
}

func ListLegacyPricingUsers(tx *gorm.DB) ([]LegacyPricingUser, error) {
	if tx == nil {
		tx = DB
	}
	if tx == nil || !tx.Migrator().HasTable(&User{}) {
		return []LegacyPricingUser{}, nil
	}

	legacyRatios := ratio_setting.GetGroupGroupRatioCopy()

	overrideUserIDs := make([]int, 0)
	if tx.Migrator().HasTable(&UserGroupPriceOverride{}) {
		if err := tx.Model(&UserGroupPriceOverride{}).Distinct("user_id").Pluck("user_id", &overrideUserIDs).Error; err != nil {
			return nil, err
		}
	}

	var users []User
	query := tx.Model(&User{}).
		Where("pricing_profile_id = ?", 0)
	if len(overrideUserIDs) > 0 {
		query = query.Where("id NOT IN ?", overrideUserIDs)
	}
	if err := query.Order("id ASC").Find(&users).Error; err != nil {
		return nil, err
	}

	items := make([]LegacyPricingUser, 0, len(users))
	for _, user := range users {
		audienceGroupID := resolveStoredUserAudienceGroupID(&user)
		groupLabel := ""
		if audienceGroupID > 0 {
			if labelMap, err := UserGroupIDNameMap(tx, []int{audienceGroupID}); err == nil {
				groupLabel = labelMap[audienceGroupID]
			}
		}
		profileLabel := ""
		if user.PricingProfileId > 0 {
			profileLabel, _ = GetPricingProfileLabel(user.PricingProfileId)
		}
		targetCount := 0
		if legacyTargets, ok := legacyRatios[audienceGroupID]; ok {
			targetCount = len(legacyTargets)
		}
		effectiveSource := "public"
		baseMultiplierApplied := false
		baseMultiplier := resolveUserBaseMultiplier(user.ToBaseUser())
		if targetCount > 0 {
			effectiveSource = "legacy"
		} else if baseMultiplier != 1 {
			effectiveSource = "base_multiplier"
			baseMultiplierApplied = true
		} else {
			continue
		}
		items = append(items, LegacyPricingUser{
			Id:                    user.Id,
			Username:              user.Username,
			DisplayName:           user.DisplayName,
			GroupId:               audienceGroupID,
			GroupLabel:            groupLabel,
			BaseMultiplier:        user.BaseMultiplier,
			BaseMultiplierApplied: baseMultiplierApplied,
			LegacyTargetCount:     targetCount,
			EffectiveSource:       effectiveSource,
			CustomerType:          user.CustomerType,
			PricingProfileId:      user.PricingProfileId,
			PricingProfileLabel:   profileLabel,
		})
	}
	return items, nil
}

type resolvedUserPricingRule struct {
	HasNewRule         bool
	Factor             float64
	UsedOverride       bool
	UsedProfile        bool
	UsedBaseMultiplier bool
	UsedProfileID      int
	ResolvedGroupRatio float64
}

func resolveUserPricingRule(user *UserBase, usingGroupID int, publicGroupRatio float64) resolvedUserPricingRule {
	if user == nil || user.Id <= 0 || usingGroupID <= 0 {
		return resolvedUserPricingRule{}
	}
	ensurePricingRuleCacheFresh()
	result := resolvedUserPricingRule{}
	baseMultiplier := resolveUserBaseMultiplier(user)

	pricingRuleCacheMu.RLock()
	if overrides, ok := pricingRuleUserOverridesByUser[user.Id]; ok {
		if factor, ok := overrides[usingGroupID]; ok && factor > 0 {
			result.HasNewRule = true
			result.Factor = factor
			result.UsedOverride = true
			result.ResolvedGroupRatio = factor
			pricingRuleCacheMu.RUnlock()
			return result
		}
	}
	if user.PricingProfileId > 0 {
		if profile, ok := pricingRuleProfilesByID[user.PricingProfileId]; ok {
			if perGroup, ok := pricingRuleProfileFactorsByProfile[user.PricingProfileId]; ok {
				if override, ok := perGroup[usingGroupID]; ok && override > 0 {
					result.HasNewRule = true
					result.UsedProfile = true
					result.UsedProfileID = profile.Id
					result.Factor = override
					result.ResolvedGroupRatio = override
					pricingRuleCacheMu.RUnlock()
					return result
				}
			}
			factor := profile.DefaultFactor
			if factor <= 0 {
				factor = 1
			}
			result.HasNewRule = true
			result.UsedProfile = true
			result.UsedProfileID = profile.Id
			if baseMultiplier != 1 {
				factor *= baseMultiplier
				result.UsedBaseMultiplier = true
			}
			result.Factor = factor
			result.ResolvedGroupRatio = publicGroupRatio * factor
			pricingRuleCacheMu.RUnlock()
			return result
		}
		result.HasNewRule = true
		result.Factor = baseMultiplier
		result.UsedProfile = true
		result.UsedProfileID = user.PricingProfileId
		result.UsedBaseMultiplier = baseMultiplier != 1
		result.ResolvedGroupRatio = publicGroupRatio * baseMultiplier
		pricingRuleCacheMu.RUnlock()
		return result
	}
	pricingRuleCacheMu.RUnlock()
	return result
}

func safeBaseMultiplier(multiplier float64) float64 {
	if math.IsNaN(multiplier) || math.IsInf(multiplier, 0) || multiplier <= 0 {
		return 1
	}
	return multiplier
}

func resolveUserBaseMultiplier(user *UserBase) float64 {
	if user == nil {
		return 1
	}
	return safeBaseMultiplier(user.BaseMultiplier)
}

func resolveUserPricingRuleSource(resolved resolvedUserPricingRule) string {
	if resolved.UsedOverride {
		return "override"
	}
	if resolved.UsedProfile {
		return "profile"
	}
	if resolved.UsedBaseMultiplier {
		return "base_multiplier"
	}
	return "public"
}

func resolveAppliedPricingFactor(publicRatio float64, effectiveRatio float64) float64 {
	if effectiveRatio <= 0 {
		return 0
	}
	if publicRatio <= 0 {
		return 1
	}
	return effectiveRatio / publicRatio
}

func ResolveUserGroupRatioInfo(user *UserBase, usingGroupID int) types.GroupRatioInfo {
	info := types.GroupRatioInfo{
		EffectiveGroupRatio: 1,
		PublicGroupRatio:    1,
		PrivateGroupRatio:   1,
		GroupRatio:          1,
		GroupSpecialRatio:   -1,
	}
	if usingGroupID <= 0 {
		return info
	}

	publicGroupRatio := ratio_setting.GetGroupRatio(usingGroupID)
	info.EffectiveGroupRatio = publicGroupRatio
	info.PublicGroupRatio = publicGroupRatio
	info.PrivateGroupRatio = publicGroupRatio
	info.GroupRatio = publicGroupRatio
	info.Source = "public"

	if user == nil || user.Id <= 0 {
		return info
	}

	resolved := resolveUserPricingRule(user, usingGroupID, publicGroupRatio)
	if resolved.HasNewRule {
		info.EffectiveGroupRatio = resolved.ResolvedGroupRatio
		info.PublicGroupRatio = resolved.ResolvedGroupRatio
		info.PrivateGroupRatio = resolved.ResolvedGroupRatio
		info.GroupRatio = resolved.ResolvedGroupRatio
		info.GroupSpecialRatio = resolved.ResolvedGroupRatio
		info.HasSpecialRatio = true
		info.BaseMultiplierApplied = resolved.UsedBaseMultiplier
		info.Source = resolveUserPricingRuleSource(resolved)
		return info
	}

	baseMultiplier := resolveUserBaseMultiplier(user)
	if legacyRatio, ok := ratio_setting.GetGroupGroupRatio(resolveUserAudienceGroupID(user), usingGroupID); ok {
		info.EffectiveGroupRatio = legacyRatio
		info.PublicGroupRatio = legacyRatio
		info.PrivateGroupRatio = legacyRatio
		info.GroupRatio = legacyRatio
		info.GroupSpecialRatio = legacyRatio
		info.HasSpecialRatio = true
		info.BaseMultiplierApplied = false
		info.Source = "legacy"
		return info
	}
	if baseMultiplier != 1 {
		effectiveRatio := publicGroupRatio * baseMultiplier
		info.EffectiveGroupRatio = effectiveRatio
		info.PublicGroupRatio = effectiveRatio
		info.PrivateGroupRatio = effectiveRatio
		info.GroupRatio = effectiveRatio
		info.GroupSpecialRatio = effectiveRatio
		info.HasSpecialRatio = true
		info.BaseMultiplierApplied = true
		info.Source = "base_multiplier"
	}
	return info
}

func ResolveUserGroupRatioInfoByID(userID int, usingGroupID int) (types.GroupRatioInfo, *UserBase, error) {
	if userID <= 0 {
		return ResolveUserGroupRatioInfo(nil, usingGroupID), nil, nil
	}
	user, err := GetUserCache(userID)
	if err != nil {
		return types.GroupRatioInfo{}, nil, err
	}
	return ResolveUserGroupRatioInfo(user, usingGroupID), user, nil
}

func ResolveDisplayedGroupRatio(user *UserBase, usingGroupID int) float64 {
	info := ResolveUserGroupRatioInfo(user, usingGroupID)
	if info.PublicGroupRatio <= 0 {
		return 0
	}
	return info.PublicGroupRatio
}

func ResolveDisplayedGroupRatioByUserID(userID int, usingGroupID int) (float64, error) {
	if userID <= 0 {
		return ratio_setting.GetGroupRatio(usingGroupID), nil
	}
	user, err := GetUserCache(userID)
	if err != nil {
		return 0, err
	}
	return ResolveDisplayedGroupRatio(user, usingGroupID), nil
}

func BuildUserGroupPricingPreview(user *UserBase) []UserGroupPricingPreview {
	groupRatios := ratio_setting.GetGroupRatioCopy()
	if len(groupRatios) == 0 {
		return []UserGroupPricingPreview{}
	}

	groupIDs := make([]int, 0, len(groupRatios))
	for groupID := range groupRatios {
		if groupID > 0 {
			groupIDs = append(groupIDs, groupID)
		}
	}
	sort.Ints(groupIDs)

	items := make([]UserGroupPricingPreview, 0, len(groupIDs))
	for _, groupID := range groupIDs {
		publicRatio := groupRatios[groupID]
		effectiveRatio := publicRatio
		appliedFactor := resolveAppliedPricingFactor(publicRatio, effectiveRatio)
		source := "public"
		profileID := 0

		if user != nil && user.Id > 0 {
			resolved := resolveUserPricingRule(user, groupID, publicRatio)
			if resolved.HasNewRule {
				effectiveRatio = resolved.ResolvedGroupRatio
				appliedFactor = resolved.Factor
				source = resolveUserPricingRuleSource(resolved)
				profileID = resolved.UsedProfileID
			} else if legacyRatio, ok := ratio_setting.GetGroupGroupRatio(resolveUserAudienceGroupID(user), groupID); ok {
				effectiveRatio = legacyRatio
				appliedFactor = resolveAppliedPricingFactor(publicRatio, effectiveRatio)
				source = "legacy"
			} else {
				baseMultiplier := resolveUserBaseMultiplier(user)
				if baseMultiplier != 1 {
					effectiveRatio = publicRatio * baseMultiplier
					appliedFactor = baseMultiplier
					source = "base_multiplier"
				}
			}
		}

		items = append(items, UserGroupPricingPreview{
			GroupId:          groupID,
			PublicRatio:      publicRatio,
			EffectiveRatio:   effectiveRatio,
			AppliedFactor:    appliedFactor,
			Source:           source,
			PricingProfileId: profileID,
		})
	}
	return items
}

func ComputeUserTokenUsage(userID int, usingGroupID int, tokens int) (int, error) {
	if tokens <= 0 {
		return 0, nil
	}
	info, _, err := ResolveUserGroupRatioInfoByID(userID, usingGroupID)
	if err != nil {
		return 0, err
	}
	return billing.ScaleTokensByGroupRatio(tokens, info.EffectiveGroupRatio), nil
}

func ComputeUserRequestUsage(userID int, usingGroupID int, requests int) (int, error) {
	if requests <= 0 {
		return 0, nil
	}
	info, _, err := ResolveUserGroupRatioInfoByID(userID, usingGroupID)
	if err != nil {
		return 0, err
	}
	return billing.ScaleRequestsByGroupRatio(requests, info.EffectiveGroupRatio), nil
}
