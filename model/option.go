package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"one-api/common"
	"one-api/setting"
	"one-api/setting/config"
	"one-api/setting/operation_setting"
	"one-api/setting/performance_setting"
	"one-api/setting/personal_setting"
	"one-api/setting/ratio_setting"
	"one-api/setting/system_setting"
	"os"
	"strconv"
	"strings"
	"time"

	promptdef "codex-service-go/prompt"
)

type Option struct {
	Key   string `json:"key" gorm:"primaryKey"`
	Value string `json:"value" gorm:"type:longtext"`
}

const legacyStartupOptionCleanupKey = "startup.cleanup_legacy_options.enabled"
const legacyGroupOptionMigrationKey = "startup.migrate_legacy_group_options.enabled"
const legacyUserQuotaSnapshotSyncKey = "startup.sync_legacy_user_quota_snapshots.enabled"
const legacyTokenGroupReconcileKey = "startup.reconcile_legacy_token_group.enabled"
const legacyDiscreteQuotaStorageMigrationKey = "startup.migrate_discrete_quota_storage.enabled"

func AllOption() ([]*Option, error) {
	var options []*Option
	var err error
	err = DB.Find(&options).Error
	return options, err
}

func InitOptionMap() {
	common.OptionMapRWMutex.Lock()
	common.OptionMap = make(map[string]string)

	// 添加原有的系统配置
	common.OptionMap["FileUploadPermission"] = strconv.Itoa(common.FileUploadPermission)
	common.OptionMap["FileDownloadPermission"] = strconv.Itoa(common.FileDownloadPermission)
	common.OptionMap["ImageUploadPermission"] = strconv.Itoa(common.ImageUploadPermission)
	common.OptionMap["ImageDownloadPermission"] = strconv.Itoa(common.ImageDownloadPermission)
	common.OptionMap["PasswordLoginEnabled"] = strconv.FormatBool(common.PasswordLoginEnabled)
	common.OptionMap["PasswordRegisterEnabled"] = strconv.FormatBool(common.PasswordRegisterEnabled)
	common.OptionMap["EmailVerificationEnabled"] = strconv.FormatBool(common.EmailVerificationEnabled)
	common.OptionMap["GitHubOAuthEnabled"] = strconv.FormatBool(common.GitHubOAuthEnabled)
	common.OptionMap["LinuxDOOAuthEnabled"] = strconv.FormatBool(common.LinuxDOOAuthEnabled)
	common.OptionMap["TelegramOAuthEnabled"] = strconv.FormatBool(common.TelegramOAuthEnabled)
	common.OptionMap["WeChatAuthEnabled"] = strconv.FormatBool(common.WeChatAuthEnabled)
	common.OptionMap["TurnstileCheckEnabled"] = strconv.FormatBool(common.TurnstileCheckEnabled)
	common.OptionMap["RegisterEnabled"] = strconv.FormatBool(common.RegisterEnabled)
	common.OptionMap["ClawBoxActivationEnabled"] = strconv.FormatBool(common.ClawBoxActivationEnabled)
	common.OptionMap["ClawBoxRegisterEnabled"] = strconv.FormatBool(common.ClawBoxRegisterEnabled)
	common.OptionMap["ClawBoxMaxDevices"] = strconv.Itoa(common.ClawBoxMaxDevices)
	common.OptionMap["ClawBoxProductModeEnabled"] = "false"
	common.OptionMap["ClawBoxProductId"] = ""
	common.OptionMap["ClawBoxInitialShrimp"] = strconv.Itoa(common.ClawBoxInitialShrimp)
	common.OptionMap[ClawBoxManagedOpenClawConfigOption] = defaultClawBoxManagedOpenClawConfigValue()
	common.OptionMap[ClawBoxPortableRepoOptionKey] = "zwenooo/ClawBox"
	common.OptionMap[ClawBoxPortableChannelOptionKey] = "stable"
	common.OptionMap[ClawBoxPortableUpdateEnabledKey] = "true"
	common.OptionMap[ClawBoxPortableGitHubTokenKey] = ""
	common.OptionMap["AutomaticDisableChannelEnabled"] = strconv.FormatBool(common.AutomaticDisableChannelEnabled)
	common.OptionMap["AutomaticEnableChannelEnabled"] = strconv.FormatBool(common.AutomaticEnableChannelEnabled)
	common.OptionMap["LogConsumeEnabled"] = strconv.FormatBool(common.LogConsumeEnabled)
	common.OptionMap["LogConsumeInProgressEnabled"] = strconv.FormatBool(common.LogConsumeInProgressEnabled)
	// Persistent full request trace (headers/bodies stored outside SQL; index stored in DB).
	// Enabled is admin-configurable via /console/setting?tab=log, with env REQUEST_TRACE_ENABLED as default.
	common.OptionMap["request_trace.enabled"] = strconv.FormatBool(common.RequestTraceEnabled)
	// Request trace retention is admin-configurable via /console/setting?tab=log.
	// Default is taken from env (REQUEST_TRACE_RETENTION_MINUTES or legacy REQUEST_TRACE_RETENTION_DAYS),
	// but the option values themselves are loaded from DB below.
	// Keep these keys present (even empty) so the settings UI can save them.
	common.OptionMap["request_trace.retention_minutes"] = ""
	// Legacy days-based retention option (deprecated; kept for backward compatibility).
	common.OptionMap["request_trace.retention_days"] = ""
	common.OptionMap["DisplayInCurrencyEnabled"] = strconv.FormatBool(common.DisplayInCurrencyEnabled)
	common.OptionMap["DisplayTokenStatEnabled"] = strconv.FormatBool(common.DisplayTokenStatEnabled)
	common.OptionMap["StompKingRankMode"] = common.StompKingRankMode
	common.OptionMap["DrawingEnabled"] = strconv.FormatBool(common.DrawingEnabled)
	common.OptionMap["TaskEnabled"] = strconv.FormatBool(common.TaskEnabled)
	common.OptionMap["DataExportEnabled"] = strconv.FormatBool(common.DataExportEnabled)
	// Revision marker for distributed channel cache refresh.
	common.OptionMap["channel_cache.revision"] = ""
	common.OptionMap["group_settings.revision"] = ""
	common.OptionMap["ChannelDisableThreshold"] = strconv.FormatFloat(common.ChannelDisableThreshold, 'f', -1, 64)
	common.OptionMap["EmailDomainRestrictionEnabled"] = strconv.FormatBool(common.EmailDomainRestrictionEnabled)
	common.OptionMap["EmailAliasRestrictionEnabled"] = strconv.FormatBool(common.EmailAliasRestrictionEnabled)
	common.OptionMap["EmailDomainWhitelist"] = strings.Join(common.EmailDomainWhitelist, ",")
	common.OptionMap["SMTPServer"] = ""
	common.OptionMap["SMTPFrom"] = ""
	common.OptionMap["SMTPPort"] = strconv.Itoa(common.SMTPPort)
	common.OptionMap["SMTPAccount"] = ""
	common.OptionMap["SMTPToken"] = ""
	common.OptionMap["SMTPSSLEnabled"] = strconv.FormatBool(common.SMTPSSLEnabled)
	common.OptionMap["Notice"] = ""
	common.OptionMap["About"] = ""
	common.OptionMap["HomePageContent"] = ""
	common.OptionMap["Footer"] = common.Footer
	common.OptionMap["SystemName"] = common.SystemName
	common.OptionMap["Logo"] = common.Logo
	common.OptionMap["ServerAddress"] = ""
	common.OptionMap["BaseUrls"] = "[]"
	common.OptionMap["WorkerUrl"] = system_setting.WorkerUrl
	common.OptionMap["WorkerValidKey"] = system_setting.WorkerValidKey
	common.OptionMap["WorkerAllowHttpImageRequestEnabled"] = strconv.FormatBool(system_setting.WorkerAllowHttpImageRequestEnabled)
	common.OptionMap["PayAddress"] = ""
	common.OptionMap["CustomCallbackAddress"] = ""
	common.OptionMap["EpayId"] = ""
	common.OptionMap["EpayKey"] = ""
	common.OptionMap["Price"] = strconv.FormatFloat(operation_setting.Price, 'f', -1, 64)
	common.OptionMap["USDExchangeRate"] = strconv.FormatFloat(operation_setting.USDExchangeRate, 'f', -1, 64)
	common.OptionMap["MinTopUp"] = strconv.Itoa(operation_setting.MinTopUp)
	common.OptionMap["StripeMinTopUp"] = strconv.Itoa(setting.StripeMinTopUp)
	common.OptionMap["StripeApiSecret"] = setting.StripeApiSecret
	common.OptionMap["StripeWebhookSecret"] = setting.StripeWebhookSecret
	common.OptionMap["StripePriceId"] = setting.StripePriceId
	common.OptionMap["StripeUnitPrice"] = strconv.FormatFloat(setting.StripeUnitPrice, 'f', -1, 64)
	common.OptionMap["TopupGroupRatio"] = common.TopupGroupRatio2JSONString()
	common.OptionMap["Chats"] = setting.Chats2JsonString()
	common.OptionMap["AutoGroups"] = setting.AutoGroups2JsonString()
	common.OptionMap["DefaultUseAutoGroup"] = strconv.FormatBool(setting.DefaultUseAutoGroup)
	common.OptionMap["GroupManagementHideDisabledEnabled"] = strconv.FormatBool(true)
	common.OptionMap["ProductManagementHideArchivedEnabled"] = strconv.FormatBool(true)
	common.OptionMap[legacyStartupOptionCleanupKey] = "false"
	common.OptionMap["PayMethods"] = operation_setting.PayMethods2JsonString()
	common.OptionMap[personal_setting.OptionKeyAccountBindingVisibility] = personal_setting.DefaultAccountBindingVisibilityJSON
	common.OptionMap[personal_setting.OptionKeyWalletInvitationVisibility] = "true"
	common.OptionMap[personal_setting.OptionKeyOtherSettingsVisibility] = "true"
	common.OptionMap["GitHubClientId"] = ""
	common.OptionMap["GitHubClientSecret"] = ""
	common.OptionMap["TelegramBotToken"] = ""
	common.OptionMap["TelegramBotName"] = ""
	common.OptionMap["WeChatServerAddress"] = ""
	common.OptionMap["WeChatServerToken"] = ""
	common.OptionMap["WeChatAccountQRCodeImageURL"] = ""
	common.OptionMap["OpenRouterPriceSyncToken"] = ""
	common.OptionMap["TurnstileSiteKey"] = ""
	common.OptionMap["TurnstileSecretKey"] = ""
	common.OptionMap["QuotaForNewUser"] = strconv.Itoa(common.QuotaForNewUser)
	common.OptionMap["ClawBoxSignupShrimpQuota"] = strconv.Itoa(common.ClawBoxSignupShrimpQuota)
	common.OptionMap["QuotaForInviter"] = strconv.Itoa(common.QuotaForInviter)
	common.OptionMap["QuotaForInvitee"] = strconv.Itoa(common.QuotaForInvitee)
	common.OptionMap["SubscriptionInviteCommissionFirstPercent"] = strconv.Itoa(operation_setting.SubscriptionInviteCommissionFirstPercent)
	common.OptionMap["SubscriptionInviteCommissionRepeatPercent"] = strconv.Itoa(operation_setting.SubscriptionInviteCommissionRepeatPercent)
	common.OptionMap["QuotaRemindThreshold"] = strconv.Itoa(common.QuotaRemindThreshold)
	common.OptionMap["PreConsumedQuota"] = strconv.Itoa(common.PreConsumedQuota)
	common.OptionMap["ModelRequestConcurrencyLimit"] = strconv.Itoa(setting.ModelRequestConcurrencyLimit)
	common.OptionMap["ModelRequestConcurrencyLimitWaitSeconds"] = strconv.Itoa(setting.ModelRequestConcurrencyLimitWaitSeconds)
	common.OptionMap["ModelRequestConcurrencyLimitGroup"] = setting.ModelRequestConcurrencyLimitGroup2JSONString()
	common.OptionMap["ModelRequestRateLimitCount"] = strconv.Itoa(setting.ModelRequestRateLimitCount)
	common.OptionMap["ModelRequestRateLimitDurationMinutes"] = strconv.Itoa(setting.ModelRequestRateLimitDurationMinutes)
	common.OptionMap["ModelRequestRateLimitSuccessCount"] = strconv.Itoa(setting.ModelRequestRateLimitSuccessCount)
	common.OptionMap["ModelRequestRateLimitGroup"] = setting.ModelRequestRateLimitGroup2JSONString()
	common.OptionMap["ModelRatio"] = ratio_setting.ModelRatio2JSONString()
	common.OptionMap["ModelPrice"] = ratio_setting.ModelPrice2JSONString()
	common.OptionMap["CacheRatio"] = ratio_setting.CacheRatio2JSONString()
	common.OptionMap["CreateCacheRatio"] = ratio_setting.CreateCacheRatio2JSONString()
	common.OptionMap["GroupRatio"] = ratio_setting.GroupRatio2JSONString()
	common.OptionMap["GroupGroupRatio"] = ratio_setting.GroupGroupRatio2JSONString()
	common.OptionMap["UserUsableGroups"] = setting.UserUsableGroups2JSONString()
	common.OptionMap["CompletionRatio"] = ratio_setting.CompletionRatio2JSONString()
	common.OptionMap["ImageRatio"] = ratio_setting.ImageRatio2JSONString()
	common.OptionMap["AudioRatio"] = ratio_setting.AudioRatio2JSONString()
	common.OptionMap["AudioCompletionRatio"] = ratio_setting.AudioCompletionRatio2JSONString()
	common.OptionMap["TopUpLink"] = common.TopUpLink
	//common.OptionMap["ChatLink"] = common.ChatLink
	//common.OptionMap["ChatLink2"] = common.ChatLink2
	common.OptionMap["QuotaPerUnit"] = strconv.FormatFloat(common.QuotaPerUnit, 'f', -1, 64)
	common.OptionMap["RetryTimes"] = strconv.Itoa(common.RetryTimes)
	common.OptionMap["DataExportInterval"] = strconv.Itoa(common.DataExportInterval)
	common.OptionMap["DataExportDefaultTime"] = common.DataExportDefaultTime
	common.OptionMap["DefaultCollapseSidebar"] = strconv.FormatBool(common.DefaultCollapseSidebar)
	common.OptionMap["MjNotifyEnabled"] = strconv.FormatBool(setting.MjNotifyEnabled)
	common.OptionMap["MjAccountFilterEnabled"] = strconv.FormatBool(setting.MjAccountFilterEnabled)
	common.OptionMap["MjModeClearEnabled"] = strconv.FormatBool(setting.MjModeClearEnabled)
	common.OptionMap["MjForwardUrlEnabled"] = strconv.FormatBool(setting.MjForwardUrlEnabled)
	common.OptionMap["MjActionCheckSuccessEnabled"] = strconv.FormatBool(setting.MjActionCheckSuccessEnabled)
	common.OptionMap["CheckSensitiveEnabled"] = strconv.FormatBool(setting.CheckSensitiveEnabled)
	common.OptionMap["DemoSiteEnabled"] = strconv.FormatBool(operation_setting.DemoSiteEnabled)
	common.OptionMap["SelfUseModeEnabled"] = strconv.FormatBool(operation_setting.SelfUseModeEnabled)
	common.OptionMap["ChatCompletionsEnabled"] = strconv.FormatBool(operation_setting.ChatCompletionsEnabled)
	common.OptionMap["ModelRequestConcurrencyLimitEnabled"] = strconv.FormatBool(setting.ModelRequestConcurrencyLimitEnabled)
	common.OptionMap["ModelRequestRateLimitEnabled"] = strconv.FormatBool(setting.ModelRequestRateLimitEnabled)
	common.OptionMap["CheckSensitiveOnPromptEnabled"] = strconv.FormatBool(setting.CheckSensitiveOnPromptEnabled)
	common.OptionMap["StopOnSensitiveEnabled"] = strconv.FormatBool(setting.StopOnSensitiveEnabled)
	common.OptionMap["SensitiveWords"] = setting.SensitiveWordsToString()
	common.OptionMap["StreamCacheQueueLength"] = strconv.Itoa(setting.StreamCacheQueueLength)
	common.OptionMap["AutomaticDisableKeywords"] = operation_setting.AutomaticDisableKeywordsToString()
	common.OptionMap["AutomaticSwitchKeywords"] = operation_setting.AutomaticSwitchKeywordsToString()
	common.OptionMap["AutomaticSwitchStatusCodeWhitelist"] = operation_setting.AutomaticSwitchStatusCodeWhitelistToString()
	common.OptionMap["AutomaticSwitchMaxRetries"] = strconv.Itoa(operation_setting.AutomaticSwitchMaxRetries)
	common.OptionMap["ResponsesCapacityRetryEnabled"] = strconv.FormatBool(operation_setting.ResponsesCapacityRetryEnabled)
	common.OptionMap["ResponsesCapacityRetryKeywords"] = operation_setting.ResponsesCapacityRetryKeywordsToString()
	common.OptionMap["ExposeRatioEnabled"] = strconv.FormatBool(ratio_setting.IsExposeRatioEnabled())

	// 自动添加所有注册的模型配置
	modelConfigs := config.GlobalConfig.ExportAllConfigs()
	for k, v := range modelConfigs {
		common.OptionMap[k] = v
	}

	// Codex/CLIProxyAPI prompt settings (admin-managed via /console/setting?tab=cx_compat).
	common.OptionMap["codex.prompt.chat_completions.instructions"] = promptdef.GPT5Codex
	// /v1/responses instructions 注入行为（普通渠道 /responses、渠道级 cx2cc 共用）。
	common.OptionMap["cx_compat.responses.codex_cli_rs_ua_contains"] = "codex_vscode,codex_exec,Codex Desktop,codex_cli_rs"
	common.OptionMap["cx_compat.responses.override_instructions"] = strconv.FormatBool(false)
	// /v1/responses request body patch（JSON object merge; applied after responses normalization when enabled）。
	common.OptionMap["cx_compat.responses.body_patch_json"] = ""
	// cx 模型兼容性配置（OpenCode 等客户端）
	common.OptionMap["cx_compat.opencode.instructions"] = strings.TrimSpace(promptdef.OpenCodeCodexHeader)
	common.OptionMap["cx_compat.opencode.instructions_meta"] = `{"source":"builtin"}`
	common.OptionMap["cx_compat.opencode.pinned_instructions"] = ""
	common.OptionMap["cx_compat.opencode.pinned_meta"] = ""
	// 渠道级 cx2cc 运行时兼容参数。
	common.OptionMap["cx2cc.reasoning_effort"] = ""
	common.OptionMap["cx2cc.reasoning_summary"] = ""
	common.OptionMap["cx2cc.prompt_cache.enabled"] = "true"
	common.OptionMap["cx2cc.prompt_cache.entries"] = "5000"
	common.OptionMap["cx2cc.prompt_cache.sessions"] = "2000"
	common.OptionMap["cx2cc.prompt_cache.session_ttl_ms"] = "7200000"
	common.OptionMap["cx2cc.cache_smoothing.enabled"] = "true"
	common.OptionMap["cx2cc.cache_smoothing.drop_threshold"] = "0.3"
	common.OptionMap["cx2cc.cache_smoothing.dampening"] = "0.8"
	common.OptionMap["cx2cc.cache_smoothing.min_prev_read"] = "1000"
	common.OptionMapRWMutex.Unlock()
	loadOptionsFromDatabase()
}

func loadOptionsFromDatabase() {
	options, _ := AllOption()
	reloadOptions := false
	cleanupLegacyStartupOptions := shouldCleanupLegacyStartupOptions(options)

	// Migration: legacy request_trace.retention_days -> request_trace.retention_minutes (minutes-based).
	// We keep the legacy key for rollback compatibility, but store the computed minutes so the new
	// UI/logic can display and use it consistently.
	{
		hasMinutes := false
		daysRaw := ""
		for _, option := range options {
			if option == nil {
				continue
			}
			key := strings.TrimSpace(option.Key)
			switch key {
			case "request_trace.retention_minutes":
				hasMinutes = true
			case "request_trace.retention_days":
				daysRaw = strings.TrimSpace(option.Value)
			}
		}
		if !hasMinutes && daysRaw != "" {
			if days, err := strconv.Atoi(daysRaw); err == nil && days > 0 {
				minutes := days * 24 * 60
				if err := UpdateOption("request_trace.retention_minutes", strconv.Itoa(minutes)); err != nil {
					common.SysLog(fmt.Sprintf("failed to migrate request_trace.retention_days to retention_minutes: %v", err))
				}
			}
		}
	}
	{
		hasArchived := false
		legacyHideValue := ""
		for _, option := range options {
			if option == nil {
				continue
			}
			switch strings.TrimSpace(option.Key) {
			case "ProductManagementHideArchivedEnabled":
				hasArchived = true
			case "ProductManagementHideDisabledEnabled":
				legacyHideValue = strings.TrimSpace(option.Value)
			}
		}
		migratedArchived := hasArchived
		if !hasArchived && legacyHideValue != "" {
			if err := UpdateOption("ProductManagementHideArchivedEnabled", legacyHideValue); err != nil {
				common.SysLog(fmt.Sprintf("failed to migrate ProductManagementHideDisabledEnabled: %v", err))
			} else {
				migratedArchived = true
				reloadOptions = true
			}
		}
		if legacyHideValue != "" && migratedArchived && cleanupLegacyStartupOptions {
			if err := DB.Where("key = ?", "ProductManagementHideDisabledEnabled").Delete(&Option{}).Error; err != nil {
				common.SysLog(fmt.Sprintf("failed to cleanup ProductManagementHideDisabledEnabled: %v", err))
			} else {
				reloadOptions = true
			}
		}
	}
	if migrateLegacyCx2ccOptionKeys(options, cleanupLegacyStartupOptions) {
		reloadOptions = true
	}
	if reloadOptions {
		options, _ = AllOption()
	}

	for _, option := range options {
		key := strings.TrimSpace(option.Key)
		if key == "ProductManagementHideDisabledEnabled" || isLegacyCx2ccOptionKey(key) {
			continue
		}
		err := updateOptionMap(option.Key, option.Value)
		if err != nil {
			common.SysLog(fmt.Sprintf("failed to update option map: key=%s err=%v", option.Key, err))
		}
	}
	// Keep group settings (ratio + user-usable groups) synced from DB-backed `groups` table.
	// A revision option makes cross-node refresh explicit while still ensuring startup sync.
	if err := EnsureGroupSettingsSynced(false); err != nil {
		common.SysLog("failed to sync group settings from DB: " + err.Error())
	}
}

func SyncOptions(frequency int) {
	for {
		time.Sleep(time.Duration(frequency) * time.Second)
		common.SysLog("syncing options from database")
		loadOptionsFromDatabase()
	}
}

func shouldCleanupLegacyStartupOptions(options []*Option) bool {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("STARTUP_CLEANUP_LEGACY_OPTIONS_ENABLED")), "true") {
		return true
	}
	for _, option := range options {
		if option == nil {
			continue
		}
		if strings.TrimSpace(option.Key) != legacyStartupOptionCleanupKey {
			continue
		}
		return strings.EqualFold(strings.TrimSpace(option.Value), "true")
	}
	return false
}

func shouldApplyLegacyGroupOptionMigrations() bool {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("STARTUP_MIGRATE_LEGACY_GROUP_OPTIONS_ENABLED")), "true") {
		return true
	}
	if DB == nil || !DB.Migrator().HasTable(&Option{}) {
		return false
	}
	var option Option
	if err := DB.Where("key = ?", legacyGroupOptionMigrationKey).First(&option).Error; err != nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(option.Value), "true")
}

func shouldSyncLegacyUserQuotaSnapshots() bool {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("STARTUP_SYNC_LEGACY_USER_QUOTA_SNAPSHOTS_ENABLED")), "true") {
		return true
	}
	if DB == nil || !DB.Migrator().HasTable(&Option{}) {
		return false
	}
	var option Option
	if err := DB.Where("key = ?", legacyUserQuotaSnapshotSyncKey).First(&option).Error; err != nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(option.Value), "true")
}

func shouldReconcileLegacyTokenGroup() bool {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("STARTUP_RECONCILE_LEGACY_TOKEN_GROUP_ENABLED")), "true") {
		return true
	}
	if DB == nil || !DB.Migrator().HasTable(&Option{}) {
		return false
	}
	var option Option
	if err := DB.Where("key = ?", legacyTokenGroupReconcileKey).First(&option).Error; err != nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(option.Value), "true")
}

func shouldMigrateDiscreteQuotaStorage() bool {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("STARTUP_MIGRATE_DISCRETE_QUOTA_STORAGE_ENABLED")), "true") {
		return true
	}
	if DB == nil || !DB.Migrator().HasTable(&Option{}) {
		return false
	}
	var option Option
	if err := DB.Where("key = ?", legacyDiscreteQuotaStorageMigrationKey).First(&option).Error; err != nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(option.Value), "true")
}

type optionAlias struct {
	current string
	legacy  string
}

var legacyCx2ccOptionAliases = []optionAlias{
	{current: "cx2cc.reasoning_effort", legacy: "cx_pool.cx2cc.reasoning_effort"},
	{current: "cx2cc.reasoning_summary", legacy: "cx_pool.cx2cc.reasoning_summary"},
	{current: "cx2cc.prompt_cache.enabled", legacy: "cx_pool.cx2cc.prompt_cache.enabled"},
	{current: "cx2cc.prompt_cache.entries", legacy: "cx_pool.cx2cc.prompt_cache.entries"},
	{current: "cx2cc.prompt_cache.sessions", legacy: "cx_pool.cx2cc.prompt_cache.sessions"},
	{current: "cx2cc.prompt_cache.session_ttl_ms", legacy: "cx_pool.cx2cc.prompt_cache.session_ttl_ms"},
	{current: "cx2cc.cache_smoothing.enabled", legacy: "cx_pool.cx2cc.cache_smoothing.enabled"},
	{current: "cx2cc.cache_smoothing.drop_threshold", legacy: "cx_pool.cx2cc.cache_smoothing.drop_threshold"},
	{current: "cx2cc.cache_smoothing.dampening", legacy: "cx_pool.cx2cc.cache_smoothing.dampening"},
	{current: "cx2cc.cache_smoothing.min_prev_read", legacy: "cx_pool.cx2cc.cache_smoothing.min_prev_read"},
}

func migrateLegacyCx2ccOptionKeys(options []*Option, cleanupEnabled bool) bool {
	seen := make(map[string]string, len(options))
	for _, option := range options {
		if option == nil {
			continue
		}
		seen[strings.TrimSpace(option.Key)] = strings.TrimSpace(option.Value)
	}

	changed := false
	for _, alias := range legacyCx2ccOptionAliases {
		legacyValue, hasLegacy := seen[alias.legacy]
		_, hasCurrent := seen[alias.current]
		if hasLegacy && !hasCurrent {
			if err := UpdateOption(alias.current, legacyValue); err != nil {
				common.SysLog(fmt.Sprintf("failed to migrate %s to %s: %v", alias.legacy, alias.current, err))
				continue
			}
			hasCurrent = true
			changed = true
		}
		if cleanupEnabled && hasLegacy && hasCurrent {
			if err := DB.Where("key = ?", alias.legacy).Delete(&Option{}).Error; err != nil {
				common.SysLog(fmt.Sprintf("failed to cleanup %s: %v", alias.legacy, err))
			} else {
				changed = true
			}
		}
	}
	return changed
}

func isLegacyCx2ccOptionKey(key string) bool {
	for _, alias := range legacyCx2ccOptionAliases {
		if key == alias.legacy {
			return true
		}
	}
	return false
}

func UpdateOption(key string, value string) error {
	if key == "BaseUrls" {
		normalized, _, err := normalizeBaseUrlsOptionValue(value)
		if err != nil {
			return err
		}
		value = normalized
	}
	if err := validateOptionValue(key, value); err != nil {
		return err
	}
	// Save to database first
	option := Option{
		Key: key,
	}
	// https://gorm.io/docs/update.html#Save-All-Fields
	if err := DB.FirstOrCreate(&option, Option{Key: key}).Error; err != nil {
		return err
	}
	option.Value = value
	// Save is a combination function.
	// If save value does not contain primary key, it will execute Create,
	// otherwise it will execute Update (with all fields).
	if err := DB.Save(&option).Error; err != nil {
		return err
	}
	// Update OptionMap
	return updateOptionMap(key, value)
}

func UpdateOptionsAtomic(options map[string]string) error {
	if len(options) == 0 {
		return nil
	}
	for key, value := range options {
		if err := validateOptionValue(key, value); err != nil {
			return err
		}
	}
	tx := DB.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	for key, value := range options {
		option := Option{Key: key}
		if err := tx.FirstOrCreate(&option, Option{Key: key}).Error; err != nil {
			_ = tx.Rollback()
			return err
		}
		option.Value = value
		if err := tx.Save(&option).Error; err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	if err := tx.Commit().Error; err != nil {
		return err
	}
	for key, value := range options {
		if err := updateOptionMap(key, value); err != nil {
			return err
		}
	}
	return nil
}

func updateOptionMap(key string, value string) (err error) {
	common.OptionMapRWMutex.Lock()
	defer common.OptionMapRWMutex.Unlock()
	common.OptionMap[key] = value
	switch key {
	case "codex.prompt.chat_completions.instructions":
		_ = os.Setenv("ONEAPI_CODEX_PROMPT_CHAT_COMPLETIONS_INSTRUCTIONS", value)
	}

	// 检查是否是模型配置 - 使用更规范的方式处理
	if handleConfigUpdate(key, value) {
		return nil // 已由配置系统处理
	}

	// 处理传统配置项...
	if strings.HasSuffix(key, "Permission") {
		intValue, _ := strconv.Atoi(value)
		switch key {
		case "FileUploadPermission":
			common.FileUploadPermission = intValue
		case "FileDownloadPermission":
			common.FileDownloadPermission = intValue
		case "ImageUploadPermission":
			common.ImageUploadPermission = intValue
		case "ImageDownloadPermission":
			common.ImageDownloadPermission = intValue
		}
	}
	if strings.HasSuffix(key, "Enabled") || key == "DefaultCollapseSidebar" || key == "DefaultUseAutoGroup" {
		boolValue := value == "true"
		switch key {
		case "PasswordRegisterEnabled":
			common.PasswordRegisterEnabled = boolValue
		case "PasswordLoginEnabled":
			common.PasswordLoginEnabled = boolValue
		case "EmailVerificationEnabled":
			common.EmailVerificationEnabled = boolValue
		case "GitHubOAuthEnabled":
			common.GitHubOAuthEnabled = boolValue
		case "LinuxDOOAuthEnabled":
			common.LinuxDOOAuthEnabled = boolValue
		case "WeChatAuthEnabled":
			common.WeChatAuthEnabled = boolValue
		case "TelegramOAuthEnabled":
			common.TelegramOAuthEnabled = boolValue
		case "TurnstileCheckEnabled":
			common.TurnstileCheckEnabled = boolValue
		case "RegisterEnabled":
			common.RegisterEnabled = boolValue
		case "ClawBoxActivationEnabled":
			common.ClawBoxActivationEnabled = boolValue
		case "ClawBoxRegisterEnabled":
			common.ClawBoxRegisterEnabled = boolValue
		case "EmailDomainRestrictionEnabled":
			common.EmailDomainRestrictionEnabled = boolValue
		case "EmailAliasRestrictionEnabled":
			common.EmailAliasRestrictionEnabled = boolValue
		case "AutomaticDisableChannelEnabled":
			common.AutomaticDisableChannelEnabled = boolValue
		case "AutomaticEnableChannelEnabled":
			common.AutomaticEnableChannelEnabled = boolValue
		case "LogConsumeEnabled":
			common.LogConsumeEnabled = boolValue
		case "LogConsumeInProgressEnabled":
			common.LogConsumeInProgressEnabled = boolValue
		case "DisplayInCurrencyEnabled":
			common.DisplayInCurrencyEnabled = boolValue
		case "DisplayTokenStatEnabled":
			common.DisplayTokenStatEnabled = boolValue
		case "DrawingEnabled":
			common.DrawingEnabled = boolValue
		case "TaskEnabled":
			common.TaskEnabled = boolValue
		case "DataExportEnabled":
			common.DataExportEnabled = boolValue
		case "DefaultCollapseSidebar":
			common.DefaultCollapseSidebar = boolValue
		case "MjNotifyEnabled":
			setting.MjNotifyEnabled = boolValue
		case "MjAccountFilterEnabled":
			setting.MjAccountFilterEnabled = boolValue
		case "MjModeClearEnabled":
			setting.MjModeClearEnabled = boolValue
		case "MjForwardUrlEnabled":
			setting.MjForwardUrlEnabled = boolValue
		case "MjActionCheckSuccessEnabled":
			setting.MjActionCheckSuccessEnabled = boolValue
		case "CheckSensitiveEnabled":
			setting.CheckSensitiveEnabled = boolValue
		case "DemoSiteEnabled":
			operation_setting.DemoSiteEnabled = boolValue
		case "SelfUseModeEnabled":
			operation_setting.SelfUseModeEnabled = boolValue
		case "ChatCompletionsEnabled":
			operation_setting.ChatCompletionsEnabled = boolValue
		case "ResponsesCapacityRetryEnabled":
			operation_setting.ResponsesCapacityRetryEnabled = boolValue
		case "CheckSensitiveOnPromptEnabled":
			setting.CheckSensitiveOnPromptEnabled = boolValue
		case "ModelRequestConcurrencyLimitEnabled":
			setting.ModelRequestConcurrencyLimitEnabled = boolValue
		case "ModelRequestRateLimitEnabled":
			setting.ModelRequestRateLimitEnabled = boolValue
		case "StopOnSensitiveEnabled":
			setting.StopOnSensitiveEnabled = boolValue
		case "SMTPSSLEnabled":
			common.SMTPSSLEnabled = boolValue
		case "WorkerAllowHttpImageRequestEnabled":
			system_setting.WorkerAllowHttpImageRequestEnabled = boolValue
		case "DefaultUseAutoGroup":
			setting.DefaultUseAutoGroup = boolValue
		case "ExposeRatioEnabled":
			ratio_setting.SetExposeRatioEnabled(boolValue)
		}
	}
	switch key {
	case "StompKingRankMode":
		normalized := strings.TrimSpace(value)
		if normalized == common.StompKingRankModeVisibleQuota {
			normalized = common.StompKingRankModeCostQuota
		}
		if normalized != common.StompKingRankModeQuota &&
			normalized != common.StompKingRankModeCostQuota &&
			normalized != common.StompKingRankModeSuccessCount {
			return fmt.Errorf("invalid StompKingRankMode: %s", value)
		}
		common.StompKingRankMode = normalized
	case "EmailDomainWhitelist":
		common.EmailDomainWhitelist = strings.Split(value, ",")
	case "SMTPServer":
		common.SMTPServer = value
	case "SMTPPort":
		intValue, _ := strconv.Atoi(value)
		common.SMTPPort = intValue
	case "SMTPAccount":
		common.SMTPAccount = value
	case "SMTPFrom":
		common.SMTPFrom = value
	case "SMTPToken":
		common.SMTPToken = value
	case "ServerAddress":
		system_setting.ServerAddress = value
	case "BaseUrls":
		normalized, urls, err := normalizeBaseUrlsOptionValue(value)
		if err != nil {
			return err
		}
		common.OptionMap[key] = normalized
		system_setting.BaseUrls = urls
	case "WorkerUrl":
		system_setting.WorkerUrl = value
	case "WorkerValidKey":
		system_setting.WorkerValidKey = value
	case "PayAddress":
		operation_setting.PayAddress = value
	case "Chats":
		err = setting.UpdateChatsByJsonString(value)
	case "AutoGroups":
		err = setting.UpdateAutoGroupsByJsonString(value)
	case "CustomCallbackAddress":
		operation_setting.CustomCallbackAddress = value
	case "EpayId":
		operation_setting.EpayId = value
	case "EpayKey":
		operation_setting.EpayKey = value
	case "Price":
		operation_setting.Price, _ = strconv.ParseFloat(value, 64)
	case "USDExchangeRate":
		operation_setting.USDExchangeRate, _ = strconv.ParseFloat(value, 64)
	case "MinTopUp":
		operation_setting.MinTopUp, _ = strconv.Atoi(value)
	case "StripeApiSecret":
		setting.StripeApiSecret = value
	case "StripeWebhookSecret":
		setting.StripeWebhookSecret = value
	case "StripePriceId":
		setting.StripePriceId = value
	case "StripeUnitPrice":
		setting.StripeUnitPrice, _ = strconv.ParseFloat(value, 64)
	case "StripeMinTopUp":
		setting.StripeMinTopUp, _ = strconv.Atoi(value)
	case "TopupGroupRatio":
		err = common.UpdateTopupGroupRatioByJSONString(value)
	case "GitHubClientId":
		common.GitHubClientId = value
	case "GitHubClientSecret":
		common.GitHubClientSecret = value
	case "LinuxDOClientId":
		common.LinuxDOClientId = value
	case "LinuxDOClientSecret":
		common.LinuxDOClientSecret = value
	case "LinuxDOMinimumTrustLevel":
		common.LinuxDOMinimumTrustLevel, _ = strconv.Atoi(value)
	case "Footer":
		common.Footer = value
	case "SystemName":
		common.SystemName = value
	case "Logo":
		common.Logo = value
	case "WeChatServerAddress":
		common.WeChatServerAddress = value
	case "WeChatServerToken":
		common.WeChatServerToken = value
	case "WeChatAccountQRCodeImageURL":
		common.WeChatAccountQRCodeImageURL = value
	case "TelegramBotToken":
		common.TelegramBotToken = value
	case "TelegramBotName":
		common.TelegramBotName = value
	case "TurnstileSiteKey":
		common.TurnstileSiteKey = value
	case "TurnstileSecretKey":
		common.TurnstileSecretKey = value
	case "QuotaForNewUser":
		common.QuotaForNewUser, _ = strconv.Atoi(value)
	case "ClawBoxSignupShrimpQuota":
		common.ClawBoxSignupShrimpQuota, _ = strconv.Atoi(value)
	case "ClawBoxInitialShrimp":
		common.ClawBoxInitialShrimp, _ = strconv.Atoi(value)
	case "ClawBoxMaxDevices":
		common.ClawBoxMaxDevices, _ = strconv.Atoi(value)
	case "QuotaForInviter":
		common.QuotaForInviter, _ = strconv.Atoi(value)
	case "QuotaForInvitee":
		common.QuotaForInvitee, _ = strconv.Atoi(value)
	case "SubscriptionInviteCommissionFirstPercent":
		percent, convErr := strconv.Atoi(value)
		if convErr != nil {
			err = errors.New("SubscriptionInviteCommissionFirstPercent 必须为整数")
			break
		}
		if percent < 0 || percent > 100 {
			err = errors.New("SubscriptionInviteCommissionFirstPercent 必须在 0-100 之间")
			break
		}
		operation_setting.SubscriptionInviteCommissionFirstPercent = percent
	case "SubscriptionInviteCommissionRepeatPercent":
		percent, convErr := strconv.Atoi(value)
		if convErr != nil {
			err = errors.New("SubscriptionInviteCommissionRepeatPercent 必须为整数")
			break
		}
		if percent < 0 || percent > 100 {
			err = errors.New("SubscriptionInviteCommissionRepeatPercent 必须在 0-100 之间")
			break
		}
		operation_setting.SubscriptionInviteCommissionRepeatPercent = percent
	case "QuotaRemindThreshold":
		common.QuotaRemindThreshold, _ = strconv.Atoi(value)
	case "PreConsumedQuota":
		common.PreConsumedQuota, _ = strconv.Atoi(value)
	case "ModelRequestConcurrencyLimit":
		limit, convErr := strconv.Atoi(value)
		if convErr != nil {
			err = errors.New("ModelRequestConcurrencyLimit 必须为整数")
			break
		}
		if limit < 0 {
			err = errors.New("ModelRequestConcurrencyLimit 必须大于等于0")
			break
		}
		setting.ModelRequestConcurrencyLimit = limit
	case "ModelRequestConcurrencyLimitWaitSeconds":
		waitSeconds, convErr := strconv.Atoi(value)
		if convErr != nil {
			err = errors.New("ModelRequestConcurrencyLimitWaitSeconds 必须为整数")
			break
		}
		if waitSeconds < 0 {
			err = errors.New("ModelRequestConcurrencyLimitWaitSeconds 必须大于等于0")
			break
		}
		setting.ModelRequestConcurrencyLimitWaitSeconds = waitSeconds
	case "ModelRequestConcurrencyLimitGroup":
		err = setting.UpdateModelRequestConcurrencyLimitGroupByJSONString(value)
	case "ModelRequestRateLimitCount":
		setting.ModelRequestRateLimitCount, _ = strconv.Atoi(value)
	case "ModelRequestRateLimitDurationMinutes":
		setting.ModelRequestRateLimitDurationMinutes, _ = strconv.Atoi(value)
	case "ModelRequestRateLimitSuccessCount":
		setting.ModelRequestRateLimitSuccessCount, _ = strconv.Atoi(value)
	case "ModelRequestRateLimitGroup":
		err = setting.UpdateModelRequestRateLimitGroupByJSONString(value)
	case "RetryTimes":
		retryTimes, convErr := strconv.Atoi(value)
		if convErr != nil {
			err = errors.New("RetryTimes 必须为整数")
			break
		}
		if retryTimes < 0 {
			err = errors.New("RetryTimes 必须大于等于 0")
			break
		}
		common.RetryTimes = retryTimes
	case "DataExportInterval":
		common.DataExportInterval, _ = strconv.Atoi(value)
	case "DataExportDefaultTime":
		common.DataExportDefaultTime = value
	case "ModelRatio":
		err = ratio_setting.UpdateModelRatioByJSONString(value)
	case "GroupRatio":
		// GroupRatio has been migrated to DB-backed `groups` table.
		// Keep legacy option readable for compatibility, but do not let it override DB source of truth.
		if DB == nil || !DB.Migrator().HasTable(&Group{}) {
			err = ratio_setting.UpdateGroupRatioByJSONString(value)
		}
	case "GroupGroupRatio":
		err = ratio_setting.UpdateGroupGroupRatioByJSONString(value)
	case "UserUsableGroups":
		// UserUsableGroups has been migrated to DB-backed `groups` table.
		// Keep legacy option readable for compatibility, but do not let it override DB source of truth.
		if DB == nil || !DB.Migrator().HasTable(&Group{}) {
			err = setting.UpdateUserUsableGroupsByJSONString(value)
		}
	case "CompletionRatio":
		err = ratio_setting.UpdateCompletionRatioByJSONString(value)
	case "ModelPrice":
		err = ratio_setting.UpdateModelPriceByJSONString(value)
	case "CacheRatio":
		err = ratio_setting.UpdateCacheRatioByJSONString(value)
	case "CreateCacheRatio":
		err = ratio_setting.UpdateCreateCacheRatioByJSONString(value)
	case "ImageRatio":
		err = ratio_setting.UpdateImageRatioByJSONString(value)
	case "AudioRatio":
		err = ratio_setting.UpdateAudioRatioByJSONString(value)
	case "AudioCompletionRatio":
		err = ratio_setting.UpdateAudioCompletionRatioByJSONString(value)
	case "TopUpLink":
		common.TopUpLink = value
	//case "ChatLink":
	//	common.ChatLink = value
	//case "ChatLink2":
	//	common.ChatLink2 = value
	case "ChannelDisableThreshold":
		common.ChannelDisableThreshold, _ = strconv.ParseFloat(value, 64)
	case "QuotaPerUnit":
		common.QuotaPerUnit, _ = strconv.ParseFloat(value, 64)
	case "SensitiveWords":
		setting.SensitiveWordsFromString(value)
	case "AutomaticDisableKeywords":
		operation_setting.AutomaticDisableKeywordsFromString(value)
	case "AutomaticSwitchKeywords":
		operation_setting.AutomaticSwitchKeywordsFromString(value)
	case "AutomaticSwitchStatusCodeWhitelist":
		err = operation_setting.AutomaticSwitchStatusCodeWhitelistFromString(value)
	case "AutomaticSwitchMaxRetries":
		retryTimes, convErr := strconv.Atoi(value)
		if convErr != nil {
			err = errors.New("AutomaticSwitchMaxRetries 必须为整数")
			break
		}
		if validateErr := operation_setting.ValidateAutomaticSwitchMaxRetries(retryTimes); validateErr != nil {
			err = validateErr
			break
		}
		operation_setting.AutomaticSwitchMaxRetries = retryTimes
	case "ResponsesCapacityRetryKeywords":
		operation_setting.ResponsesCapacityRetryKeywordsFromString(value)
	case "StreamCacheQueueLength":
		setting.StreamCacheQueueLength, _ = strconv.Atoi(value)
	case "PayMethods":
		err = operation_setting.UpdatePayMethodsByJsonString(value)
	}
	return err
}

func validateOptionValue(key string, value string) error {
	switch key {
	case "RetryTimes":
		retryTimes, convErr := strconv.Atoi(value)
		if convErr != nil {
			return errors.New("RetryTimes 必须为整数")
		}
		if retryTimes < 0 {
			return errors.New("RetryTimes 必须大于等于 0")
		}
		return nil
	case "AutomaticSwitchStatusCodeWhitelist":
		return operation_setting.ValidateAutomaticSwitchStatusCodeWhitelist(value)
	case "AutomaticSwitchMaxRetries":
		retryTimes, convErr := strconv.Atoi(value)
		if convErr != nil {
			return errors.New("AutomaticSwitchMaxRetries 必须为整数")
		}
		return operation_setting.ValidateAutomaticSwitchMaxRetries(retryTimes)
	case "ResponsesCapacityRetryEnabled":
		if _, err := strconv.ParseBool(value); err != nil {
			return errors.New("ResponsesCapacityRetryEnabled 必须为布尔值")
		}
		return nil
	default:
		return nil
	}
}

func normalizeBaseUrlsOptionValue(value string) (normalized string, urls []string, err error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "[]", []string{}, nil
	}

	var parsed []string
	if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
		return "", nil, err
	}

	out := make([]string, 0, len(parsed))
	seen := make(map[string]struct{}, len(parsed))
	for _, raw := range parsed {
		v := strings.TrimSpace(raw)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}

	bytes, err := json.Marshal(out)
	if err != nil {
		return "", nil, err
	}
	return string(bytes), out, nil
}

// handleConfigUpdate 处理分层配置更新，返回是否已处理
func handleConfigUpdate(key, value string) bool {
	parts := strings.SplitN(key, ".", 2)
	if len(parts) != 2 {
		return false // 不是分层配置
	}

	configName := parts[0]
	configKey := parts[1]

	// 获取配置对象
	cfg := config.GlobalConfig.Get(configName)
	if cfg == nil {
		return false // 未注册的配置
	}

	// 更新配置
	configMap := map[string]string{
		configKey: value,
	}
	config.UpdateConfigFromMap(cfg, configMap)
	if configName == "performance_setting" {
		performance_setting.UpdateAndSync()
	}

	return true // 已处理
}
