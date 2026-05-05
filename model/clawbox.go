package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"one-api/common"
	"one-api/dto"
	"one-api/logger"
	"one-api/setting/operation_setting"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"
)

type ClawBoxRegisterParams struct {
	ActivationCode string
	Username       string
	Password       string
	Remark         string
}

const ClawBoxUserRemark = "ClawBox"

const ClawBoxManagedOpenClawConfigOption = "ClawBoxManagedOpenClawConfig"

const (
	ClawBoxAccessErrorCodeReactivationRequired = "clawbox_reactivation_required"
	ClawBoxAccessErrorCodeUpgradeRequired      = "clawbox_upgrade_required"
	clawBoxManagedAnthropicProviderID          = "clawbox-anthropic"
	defaultClawBoxManagedPrimaryModel          = clawBoxManagedAnthropicProviderID + "/claude-sonnet-4-6"
)

var clawBoxManagedOpenClawConfigAllowedTopLevelKeys = []string{
	"agents",
	"discovery",
	"models",
	"plugins",
	"tools",
}

var defaultClawBoxManagedFallbackModels = []string{
	clawBoxManagedAnthropicProviderID + "/claude-opus-4-6",
}

var defaultClawBoxManagedAnthropicModelMappings = []struct {
	Actual  string
	Display string
}{
	{Actual: "claude-haiku-4-5-20251001", Display: "gpt-5.4"},
	{Actual: "claude-opus-4-5-20251101", Display: "gpt-5.4"},
	{Actual: "claude-opus-4-6", Display: "gpt-5.4"},
	{Actual: "claude-sonnet-4-5-20250929", Display: "gpt-5.4"},
	{Actual: "claude-sonnet-4-6", Display: "gpt-5.4"},
	{Actual: "claude-sonnet-4-7", Display: "gpt-5.4"},
	{Actual: "claude-opus-4-7", Display: "gpt-5.4"},
}

func defaultClawBoxManagedAnthropicProviderModels() []map[string]interface{} {
	models := make([]map[string]interface{}, 0, len(defaultClawBoxManagedAnthropicModelMappings))
	for _, mapping := range defaultClawBoxManagedAnthropicModelMappings {
		models = append(models, map[string]interface{}{
			"id":   mapping.Actual,
			"name": mapping.Display,
		})
	}
	return models
}

func defaultClawBoxManagedAgentModels() map[string]interface{} {
	models := make(map[string]interface{}, len(defaultClawBoxManagedAnthropicModelMappings))
	for _, mapping := range defaultClawBoxManagedAnthropicModelMappings {
		models[fmt.Sprintf("%s/%s", clawBoxManagedAnthropicProviderID, mapping.Actual)] = map[string]interface{}{}
	}
	return models
}

func defaultClawBoxManagedOpenClawConfigPatch() map[string]interface{} {
	return map[string]interface{}{
		"agents": map[string]interface{}{
			"defaults": map[string]interface{}{
				"model": map[string]interface{}{
					"primary":   defaultClawBoxManagedPrimaryModel,
					"fallbacks": append([]string{}, defaultClawBoxManagedFallbackModels...),
				},
				"models": defaultClawBoxManagedAgentModels(),
			},
		},
		"models": map[string]interface{}{
			"providers": map[string]interface{}{
				clawBoxManagedAnthropicProviderID: map[string]interface{}{
					"baseUrl": "${baseurl}",
					"apiKey":  "${apikey}",
					"api":     "anthropic-messages",
					"models":  defaultClawBoxManagedAnthropicProviderModels(),
				},
			},
		},
		"discovery": map[string]interface{}{
			"mdns": map[string]interface{}{
				"mode": "off",
			},
		},
		"tools": map[string]interface{}{
			"profile": "full",
			"allow": []string{
				"group:fs",
				"group:runtime",
				"group:web",
				"group:memory",
				"group:sessions",
				"image",
				"pdf",
			},
		},
	}
}

func defaultClawBoxManagedOpenClawConfigValue() string {
	bytes, err := json.Marshal(defaultClawBoxManagedOpenClawConfigPatch())
	if err != nil {
		panic(fmt.Sprintf("marshal default ClawBox managed config patch failed: %v", err))
	}
	return string(bytes)
}

type ClawBoxAccessDecision struct {
	Allowed   bool   `json:"allowed"`
	Reason    string `json:"reason,omitempty"`
	ErrorCode string `json:"error_code,omitempty"`
}

func ParseClawBoxManagedOpenClawConfigValue(raw string) (map[string]interface{}, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return map[string]interface{}{}, nil
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
		return nil, errors.New("ClawBox 配置模板必须是合法的 JSON 对象")
	}
	if parsed == nil {
		return nil, errors.New("ClawBox 配置模板必须是 JSON 对象，不能为 null")
	}

	allowedSet := make(map[string]struct{}, len(clawBoxManagedOpenClawConfigAllowedTopLevelKeys))
	for _, key := range clawBoxManagedOpenClawConfigAllowedTopLevelKeys {
		allowedSet[key] = struct{}{}
	}

	for key, value := range parsed {
		if _, ok := allowedSet[key]; !ok {
			return nil, fmt.Errorf(
				"ClawBox 配置模板仅支持以下顶层字段：%s",
				strings.Join(clawBoxManagedOpenClawConfigAllowedTopLevelKeys, " / "),
			)
		}
		if value == nil {
			continue
		}
		if _, ok := value.(map[string]interface{}); !ok {
			return nil, fmt.Errorf("ClawBox 配置模板中的 %s 必须为 JSON 对象或 null", key)
		}
	}

	return parsed, nil
}

func NormalizeClawBoxManagedOpenClawConfigValue(raw string) (string, error) {
	parsed, err := ParseClawBoxManagedOpenClawConfigValue(raw)
	if err != nil {
		return "", err
	}
	if parsed == nil {
		parsed = map[string]interface{}{}
	}
	bytes, err := json.Marshal(parsed)
	if err != nil {
		return "", fmt.Errorf("ClawBox 配置模板序列化失败: %w", err)
	}
	return string(bytes), nil
}

func ClawBoxManagedOpenClawConfigPatch() (map[string]interface{}, error) {
	common.OptionMapRWMutex.RLock()
	raw, exists := common.OptionMap[ClawBoxManagedOpenClawConfigOption]
	common.OptionMapRWMutex.RUnlock()
	if !exists {
		raw = defaultClawBoxManagedOpenClawConfigValue()
	}
	return ParseClawBoxManagedOpenClawConfigValue(raw)
}

func ClawBoxBootstrap() map[string]interface{} {
	loginEnabled := common.PasswordLoginEnabled && !common.TurnstileCheckEnabled
	registerEnabled := common.ClawBoxActivationEnabled &&
		common.ClawBoxRegisterEnabled &&
		common.RegisterEnabled &&
		common.PasswordRegisterEnabled &&
		common.PasswordLoginEnabled &&
		!common.EmailVerificationEnabled
	if registerEnabled {
		if err := ValidateClawBoxProductModeConfigTx(nil); err != nil {
			registerEnabled = false
		}
	}
	return map[string]interface{}{
		"activation_required":     common.ClawBoxActivationEnabled,
		"register_enabled":        registerEnabled,
		"login_enabled":           loginEnabled,
		"initial_shrimp":          maxClawBoxInitialShrimp(),
		"customer_service_qrcode": strings.TrimSpace(operation_setting.GetGeneralSetting().ClawBoxCustomerServiceQRCode),
	}
}

func CheckClawBoxActivationCode(code string) error {
	if !common.ClawBoxActivationEnabled {
		return errors.New("管理员关闭了 ClawBox 激活")
	}

	_, err := fetchActivationCode(DB, strings.TrimSpace(code), false)
	if err != nil {
		return normalizeClawBoxActivationError(err)
	}
	return nil
}

func RegisterClawBoxUserWithActivation(params ClawBoxRegisterParams) (*User, *RedeemResult, error) {
	if !common.ClawBoxActivationEnabled || !common.ClawBoxRegisterEnabled {
		return nil, nil, errors.New("管理员关闭了 ClawBox 开通")
	}
	if !common.PasswordLoginEnabled {
		return nil, nil, errors.New("管理员关闭了密码登录")
	}
	if !common.RegisterEnabled {
		return nil, nil, errors.New("管理员关闭了注册入口")
	}
	if !common.PasswordRegisterEnabled {
		return nil, nil, errors.New("管理员关闭了密码注册入口")
	}
	if common.EmailVerificationEnabled {
		return nil, nil, errors.New("管理员开启了邮箱验证，ClawBox 注册不可用")
	}
	if err := ValidateClawBoxProductModeConfigTx(nil); err != nil {
		return nil, nil, fmt.Errorf("ClawBox 商品配置无效: %w", err)
	}

	username := strings.TrimSpace(params.Username)
	password := strings.TrimSpace(params.Password)
	remark := ClawBoxUserRemark
	activationCode := strings.TrimSpace(params.ActivationCode)

	if err := validateClawBoxRegistration(username, password, remark, activationCode); err != nil {
		return nil, nil, err
	}

	exist, err := CheckUserExistOrDeleted(username, "")
	if err != nil {
		return nil, nil, fmt.Errorf("数据库错误，请稍后重试: %w", err)
	}
	if exist {
		return nil, nil, errors.New("用户名已存在，或已注销")
	}

	var (
		createdUser  *User
		createdToken *Token
		addedQuota   int
	)
	err = DB.Transaction(func(tx *gorm.DB) error {
		user, createErr := createClawBoxUserTx(tx, username, password, remark)
		if createErr != nil {
			return createErr
		}

		if consumeErr := consumeActivationCodeTx(tx, activationCode, user.Id); consumeErr != nil {
			return normalizeClawBoxActivationError(consumeErr)
		}

		initialQuota, grantErr := grantClawBoxInitialShrimpTx(tx, user)
		if grantErr != nil {
			return grantErr
		}

		token, tokenErr := ensureClawBoxRelayTokenTx(tx, user.Id)
		if tokenErr != nil {
			return tokenErr
		}

		createdUser = user
		createdToken = token
		addedQuota = initialQuota
		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	if createdUser == nil {
		return nil, nil, errors.New("ClawBox 开通失败")
	}

	if err := updateUserCache(*createdUser); err != nil {
		return nil, nil, err
	}
	if addedQuota > 0 {
		RecordLog(createdUser.Id, LogTypeSystem, fmt.Sprintf("ClawBox 初始虾粮到账 %s", logger.LogQuota(addedQuota)))
	}
	if createdToken != nil {
		_ = InvalidateTokenCache(createdToken.Key)
	}
	return createdUser, &RedeemResult{Mode: "activation", AddedQuota: addedQuota}, nil
}

func createClawBoxUserTx(tx *gorm.DB, username string, password string, remark string) (*User, error) {
	hashedPassword, err := common.Password2Hash(password)
	if err != nil {
		return nil, err
	}

	groupIDs, err := LegacyGroupIDsFromCodes(tx, []string{"default"})
	if err != nil {
		return nil, err
	}
	if len(groupIDs) != 1 || groupIDs[0] <= 0 {
		return nil, errors.New("默认模型分组无效")
	}

	setting := dto.UserSetting{
		RecordIpLog:    true,
		SidebarModules: generateDefaultSidebarConfigForRole(common.RoleCommonUser),
	}
	settingJSON, err := common.Marshal(setting)
	if err != nil {
		return nil, err
	}

	user := &User{
		Username:        username,
		Password:        hashedPassword,
		DisplayName:     username,
		AvatarSeed:      common.GetRandomString(16),
		Role:            common.RoleCommonUser,
		Status:          common.UserStatusEnabled,
		Quota:           0,
		BaseMultiplier:  1,
		GroupId:         groupIDs[0],
		Group:           "default",
		AffCode:         common.GetRandomString(4),
		Remark:          remark,
		DailyQuotaLimit: 0,
		Setting:         string(settingJSON),
	}

	if err := tx.Create(user).Error; err != nil {
		return nil, err
	}
	return user, nil
}

func maxClawBoxInitialShrimp() int {
	if common.ClawBoxInitialShrimp <= 0 {
		return 0
	}
	return common.ClawBoxInitialShrimp
}

func clawBoxInitialQuotaUnits() int {
	initialShrimp := maxClawBoxInitialShrimp()
	if initialShrimp <= 0 {
		return 0
	}
	return int(math.Round(float64(initialShrimp) * common.QuotaPerUnit))
}

func grantClawBoxInitialShrimpTx(tx *gorm.DB, user *User) (int, error) {
	if tx == nil {
		tx = DB
	}
	if user == nil || user.Id <= 0 {
		return 0, errors.New("ClawBox 用户无效")
	}

	initialQuota := clawBoxInitialQuotaUnits()
	if initialQuota <= 0 {
		return 0, nil
	}

	productID, productName, sortOrder, allowedGroupIDs, err := resolveClawBoxInitialGrantTargetTx(tx, user.Id)
	if err != nil {
		return 0, err
	}
	if len(allowedGroupIDs) == 0 {
		return 0, errors.New("ClawBox 初始虾粮缺少可用分组")
	}
	if err := UpsertPaygUserBalanceTx(tx, user.Id, productID, productName, sortOrder, allowedGroupIDs, initialQuota); err != nil {
		return 0, err
	}

	balances, err := GetUserPaygBalancesTx(tx, user.Id, true)
	if err != nil {
		return 0, err
	}
	unionGroupsJSON, err := UnionPaygAllowedGroupsFromBalances(balances)
	if err != nil {
		return 0, err
	}
	if err := tx.Model(&User{}).Where("id = ?", user.Id).Updates(map[string]interface{}{
		"payg_quota":          gorm.Expr("payg_quota + ?", initialQuota),
		"payg_history_quota":  gorm.Expr("payg_history_quota + ?", initialQuota),
		"payg_allowed_groups": unionGroupsJSON,
		"quota":               gorm.Expr("quota + ?", initialQuota),
	}).Error; err != nil {
		return 0, err
	}

	user.Quota += initialQuota
	user.PayAsYouGoQuota += initialQuota
	user.PayAsYouGoHistoryQuota += initialQuota
	user.PayAsYouGoAllowedGroups = unionGroupsJSON
	return initialQuota, nil
}

func resolveClawBoxInitialGrantTargetTx(tx *gorm.DB, userID int) (productID int, productName string, sortOrder int, allowedGroupIDs []int, err error) {
	if tx == nil {
		tx = DB
	}
	if isClawBoxProductModeEnabled() {
		productID, err = resolveClawBoxProductIDTx(tx)
		if err != nil {
			return 0, "", 0, nil, err
		}
		var product PaygProduct
		if err := tx.Where("id = ?", productID).First(&product).Error; err != nil {
			return 0, "", 0, nil, err
		}
		allowedGroupIDs, err = GetPaygProductAllowedGroupIDsTx(tx, productID)
		if err != nil {
			return 0, "", 0, nil, err
		}
		return productID, product.Name, product.SortOrder, normalizeUniquePositiveIDsKeepOrder(allowedGroupIDs), nil
	}

	allowedGroupIDs, err = resolveLegacyClawBoxAllowedGroupIDsTx(tx, userID)
	if err != nil {
		return 0, "", 0, nil, err
	}
	return -1, "ClawBox 初始虾粮", 0, normalizeUniquePositiveIDsKeepOrder(allowedGroupIDs), nil
}

func validateClawBoxRegistration(username string, password string, remark string, activationCode string) error {
	if strings.TrimSpace(activationCode) == "" {
		return errors.New("激活码不能为空")
	}
	nameLen := utf8.RuneCountInString(username)
	if nameLen == 0 || nameLen > 12 {
		return errors.New("用户名长度必须在 1-12 之间")
	}
	passwordLen := utf8.RuneCountInString(password)
	if passwordLen < 8 || passwordLen > 20 {
		return errors.New("密码长度必须在 8-20 之间")
	}
	if utf8.RuneCountInString(remark) > 255 {
		return errors.New("备注长度不能超过 255")
	}
	return nil
}

func normalizeClawBoxActivationError(err error) error {
	if err == nil {
		return nil
	}
	message := strings.TrimSpace(err.Error())
	if message == "" {
		return errors.New("激活失败")
	}
	message = strings.ReplaceAll(message, "兑换码", "激活码")
	return errors.New(message)
}

// ========== 设备 Session ==========

type ClawBoxDeviceSession struct {
	Id        int64  `json:"id" gorm:"primaryKey;autoIncrement"`
	UserId    int    `json:"user_id" gorm:"index:idx_user_device,unique;not null"`
	DeviceId  string `json:"device_id" gorm:"index:idx_user_device,unique;type:varchar(64);not null"`
	LastSeen  int64  `json:"last_seen" gorm:"not null"`
	CreatedAt int64  `json:"created_at" gorm:"autoCreateTime"`
}

type ClawBoxPortableMediumInput struct {
	PortableId    string `json:"portable_id"`
	DriveRoot     string `json:"drive_root"`
	VolumeSerial  string `json:"volume_serial"`
	DeviceSerial  string `json:"device_serial"`
	PnpDeviceId   string `json:"pnp_device_id"`
	Transport     string `json:"transport"`
	DriveType     int    `json:"drive_type"`
	Model         string `json:"model"`
	MediaType     string `json:"media_type"`
	InterfaceType string `json:"interface_type"`
	IsUsb         bool   `json:"is_usb"`
}

type ClawBoxPortableBinding struct {
	Id            int64  `json:"id" gorm:"primaryKey;autoIncrement"`
	UserId        int    `json:"user_id" gorm:"uniqueIndex:idx_clawbox_portable_user;not null"`
	PortableId    string `json:"portable_id" gorm:"index:idx_clawbox_portable_id;type:varchar(64);not null"`
	DriveRoot     string `json:"drive_root" gorm:"type:varchar(16)"`
	VolumeSerial  string `json:"volume_serial" gorm:"type:varchar(64)"`
	DeviceSerial  string `json:"device_serial" gorm:"type:varchar(255)"`
	PnpDeviceId   string `json:"pnp_device_id" gorm:"type:text"`
	Transport     string `json:"transport" gorm:"type:varchar(32);not null"`
	DriveType     int    `json:"drive_type" gorm:"not null;default:0"`
	Model         string `json:"model" gorm:"type:varchar(255)"`
	MediaType     string `json:"media_type" gorm:"type:varchar(255)"`
	InterfaceType string `json:"interface_type" gorm:"type:varchar(64)"`
	LastSeen      int64  `json:"last_seen" gorm:"not null"`
	CreatedAt     int64  `json:"created_at" gorm:"autoCreateTime"`
}

func allowClawBoxAccess() ClawBoxAccessDecision {
	return ClawBoxAccessDecision{Allowed: true}
}

func denyClawBoxAccess(reason string, errorCode string) ClawBoxAccessDecision {
	return ClawBoxAccessDecision{
		Allowed:   false,
		Reason:    strings.TrimSpace(reason),
		ErrorCode: strings.TrimSpace(errorCode),
	}
}

func buildClawBoxPortableBinding(userID int, portableMedium *ClawBoxPortableMediumInput) (*ClawBoxPortableBinding, error) {
	if portableMedium == nil {
		return nil, errors.New("未检测到当前存储介质信息")
	}

	portableID := strings.TrimSpace(portableMedium.PortableId)
	if portableID == "" {
		return nil, errors.New("无法识别当前存储介质，请更换目录后重试")
	}

	return &ClawBoxPortableBinding{
		UserId:        userID,
		PortableId:    portableID,
		DriveRoot:     strings.TrimSpace(portableMedium.DriveRoot),
		VolumeSerial:  strings.TrimSpace(portableMedium.VolumeSerial),
		DeviceSerial:  strings.TrimSpace(portableMedium.DeviceSerial),
		PnpDeviceId:   strings.TrimSpace(portableMedium.PnpDeviceId),
		Transport:     strings.ToLower(strings.TrimSpace(portableMedium.Transport)),
		DriveType:     portableMedium.DriveType,
		Model:         strings.TrimSpace(portableMedium.Model),
		MediaType:     strings.TrimSpace(portableMedium.MediaType),
		InterfaceType: strings.TrimSpace(portableMedium.InterfaceType),
		LastSeen:      time.Now().Unix(),
	}, nil
}

func upsertClawBoxPortableBindingTx(tx *gorm.DB, binding *ClawBoxPortableBinding) error {
	if tx == nil {
		return errors.New("db transaction is required")
	}
	if binding == nil {
		return errors.New("binding is required")
	}

	var existing ClawBoxPortableBinding
	err := tx.Where("user_id = ?", binding.UserId).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return tx.Create(binding).Error
	}
	if err != nil {
		return err
	}

	return tx.Model(&existing).Updates(map[string]interface{}{
		"portable_id":    binding.PortableId,
		"drive_root":     binding.DriveRoot,
		"volume_serial":  binding.VolumeSerial,
		"device_serial":  binding.DeviceSerial,
		"pnp_device_id":  binding.PnpDeviceId,
		"transport":      binding.Transport,
		"drive_type":     binding.DriveType,
		"model":          binding.Model,
		"media_type":     binding.MediaType,
		"interface_type": binding.InterfaceType,
		"last_seen":      binding.LastSeen,
	}).Error
}

func VerifyClawBoxAccess(userID int, deviceID string, platform string, portableMedium *ClawBoxPortableMediumInput) (ClawBoxAccessDecision, error) {
	_ = platform
	_ = portableMedium

	allowed, reason, err := VerifyClawBoxDevice(userID, deviceID)
	if err != nil {
		return ClawBoxAccessDecision{}, err
	}
	if !allowed {
		return denyClawBoxAccess(reason, ""), nil
	}
	return allowClawBoxAccess(), nil
}

func VerifyClawBoxPortableMedium(userID int, portableMedium *ClawBoxPortableMediumInput) (ClawBoxAccessDecision, error) {
	binding, err := buildClawBoxPortableBinding(userID, portableMedium)
	if err != nil {
		return denyClawBoxAccess(err.Error(), ""), nil
	}

	decision := allowClawBoxAccess()
	err = DB.Transaction(func(tx *gorm.DB) error {
		var existing ClawBoxPortableBinding
		err := tx.Where("user_id = ?", userID).First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return upsertClawBoxPortableBindingTx(tx, binding)
		}
		if err != nil {
			return err
		}
		if strings.TrimSpace(existing.PortableId) != binding.PortableId {
			decision = denyClawBoxAccess(
				"当前内容已迁移到新的磁盘或设备。如需继续使用，请输入新的激活码完成重新激活。",
				ClawBoxAccessErrorCodeReactivationRequired,
			)
			return nil
		}
		return upsertClawBoxPortableBindingTx(tx, binding)
	})
	if err != nil {
		return ClawBoxAccessDecision{}, err
	}
	return decision, nil
}

func ReactivateClawBoxPortableMedium(userID int, activationCode string, deviceID string, portableMedium *ClawBoxPortableMediumInput) (ClawBoxAccessDecision, error) {
	activationCode = strings.TrimSpace(activationCode)
	if activationCode == "" {
		return denyClawBoxAccess("激活码不能为空", ""), nil
	}
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return denyClawBoxAccess("设备标识无效", ""), nil
	}

	binding, err := buildClawBoxPortableBinding(userID, portableMedium)
	if err != nil {
		return denyClawBoxAccess(err.Error(), ""), nil
	}

	decision := allowClawBoxAccess()
	var invalidatedRelayKeys []string
	var relayToken *Token
	err = DB.Transaction(func(tx *gorm.DB) error {
		var existing ClawBoxPortableBinding
		err := tx.Where("user_id = ?", userID).First(&existing).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err == nil && strings.TrimSpace(existing.PortableId) == binding.PortableId {
			decision = denyClawBoxAccess("当前存储介质已激活，无需重新激活。", "")
			return nil
		}

		if consumeErr := consumeActivationCodeTx(tx, activationCode, userID); consumeErr != nil {
			return normalizeClawBoxActivationError(consumeErr)
		}
		if err := upsertClawBoxPortableBindingTx(tx, binding); err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", userID).Delete(&ClawBoxDeviceSession{}).Error; err != nil {
			return err
		}
		var relayTokens []Token
		if err := tx.Where("user_id = ? AND name = ?", userID, ClawBoxRelayTokenName).Find(&relayTokens).Error; err != nil {
			return err
		}
		for _, item := range relayTokens {
			if strings.TrimSpace(item.Key) != "" {
				invalidatedRelayKeys = append(invalidatedRelayKeys, item.Key)
			}
		}
		relayToken, err = rotateClawBoxRelayTokenTx(tx, userID)
		if err != nil {
			return err
		}
		return upsertClawBoxDeviceSessionTx(tx, userID, deviceID)
	})
	if err != nil {
		return ClawBoxAccessDecision{}, err
	}
	for _, key := range invalidatedRelayKeys {
		_ = InvalidateTokenCache(key)
	}
	if relayToken != nil {
		_ = InvalidateTokenCache(relayToken.Key)
	}
	return decision, nil
}

func upsertClawBoxDeviceSessionTx(tx *gorm.DB, userID int, deviceID string) error {
	if tx == nil {
		return errors.New("db transaction is required")
	}
	deviceID = strings.TrimSpace(deviceID)
	if userID <= 0 || deviceID == "" {
		return errors.New("设备标识无效")
	}

	now := time.Now().Unix()
	var existing ClawBoxDeviceSession
	err := tx.Where("user_id = ? AND device_id = ?", userID, deviceID).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return tx.Create(&ClawBoxDeviceSession{
			UserId:   userID,
			DeviceId: deviceID,
			LastSeen: now,
		}).Error
	}
	if err != nil {
		return err
	}

	return tx.Model(&existing).Update("last_seen", now).Error
}

// VerifyClawBoxDevice 校验设备是否允许使用，并更新 last_seen。
// 返回 (allowed bool, reason string, err error)
func VerifyClawBoxDevice(userID int, deviceID string) (bool, string, error) {
	maxDevices := common.ClawBoxMaxDevices
	if maxDevices <= 0 {
		return true, "", nil
	}

	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return false, "设备标识无效", nil
	}

	// 尝试更新已有记录的 last_seen
	result := DB.Model(&ClawBoxDeviceSession{}).
		Where("user_id = ? AND device_id = ?", userID, deviceID).
		Update("last_seen", time.Now().Unix())
	if result.Error != nil {
		return false, "", result.Error
	}

	// 记录已存在，直接放行
	if result.RowsAffected > 0 {
		return true, "", nil
	}

	// 新设备，在事务内做 count + insert，避免竞态导致超出席位
	var allowed bool
	var reason string
	err := DB.Transaction(func(tx *gorm.DB) error {
		if maxDevices > 0 {
			var count int64
			if err := tx.Model(&ClawBoxDeviceSession{}).
				Where("user_id = ?", userID).
				Count(&count).Error; err != nil {
				return err
			}
			if int(count) >= maxDevices {
				reason = fmt.Sprintf("该账号已在 %d 台设备上使用，已达上限。请联系管理员或在其他设备上退出登录。", maxDevices)
				return nil
			}
		}

		session := &ClawBoxDeviceSession{
			UserId:   userID,
			DeviceId: deviceID,
			LastSeen: time.Now().Unix(),
		}
		if err := tx.Create(session).Error; err != nil {
			// 并发下已被其他请求插入，视为成功
			var count int64
			tx.Model(&ClawBoxDeviceSession{}).
				Where("user_id = ? AND device_id = ?", userID, deviceID).
				Count(&count)
			if count > 0 {
				allowed = true
				return nil
			}
			return err
		}
		allowed = true
		return nil
	})
	if err != nil {
		return false, "", err
	}
	return allowed, reason, nil
}

// RemoveClawBoxDevice 解绑指定设备（用户主动退出登录时调用）
func RemoveClawBoxDevice(userID int, deviceID string) error {
	if common.ClawBoxMaxDevices <= 0 {
		return nil
	}
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return nil
	}
	return DB.Where("user_id = ? AND device_id = ?", userID, deviceID).
		Delete(&ClawBoxDeviceSession{}).Error
}

// ListClawBoxDevices 列出用户所有设备（管理员用）
func ListClawBoxDevices(userID int) ([]ClawBoxDeviceSession, error) {
	if common.ClawBoxMaxDevices <= 0 {
		return []ClawBoxDeviceSession{}, nil
	}
	var sessions []ClawBoxDeviceSession
	err := DB.Where("user_id = ?", userID).
		Order("last_seen DESC").
		Find(&sessions).Error
	return sessions, err
}
