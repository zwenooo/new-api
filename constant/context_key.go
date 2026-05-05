package constant

type ContextKey string

const (
	ContextKeyTokenCountMeta ContextKey = "token_count_meta"
	ContextKeyPromptTokens   ContextKey = "prompt_tokens"

	ContextKeyOriginalModel                 ContextKey = "original_model"
	ContextKeyRequestStartTime              ContextKey = "request_start_time"
	ContextKeyDeferredResponsesWSDistribute ContextKey = "deferred_responses_ws_distribute"
	ContextKeyResponsesForceUpstreamStream  ContextKey = "responses_force_upstream_stream"

	/* token related keys */
	ContextKeyTokenUnlimited            ContextKey = "token_unlimited_quota"
	ContextKeyTokenKey                  ContextKey = "token_key"
	ContextKeyTokenId                   ContextKey = "token_id"
	ContextKeyTokenGroup                ContextKey = "token_group"
	ContextKeyTokenAllowedGroups        ContextKey = "token_allowed_groups"
	ContextKeyTokenGroupId              ContextKey = "token_group_id"
	ContextKeyTokenAllowedGroupIds      ContextKey = "token_allowed_group_ids"
	ContextKeyResolvedGroupCandidateIds ContextKey = "resolved_group_candidate_ids"
	ContextKeyRuntimeSelectionAuthority ContextKey = "runtime_selection_authority"
	ContextKeyTokenSpecificChannelId    ContextKey = "specific_channel_id"
	ContextKeyTokenModelLimitEnabled    ContextKey = "token_model_limit_enabled"
	ContextKeyTokenModelLimit           ContextKey = "token_model_limit"
	ContextKeyTokenDailyQuotaLimit      ContextKey = "token_daily_quota_limit"
	ContextKeyTokenDailyQuotaUsed       ContextKey = "token_daily_quota_used"
	ContextKeyTokenDailyQuotaResetDate  ContextKey = "token_daily_quota_reset_date"

	/* channel related keys */
	ContextKeyChannelId                        ContextKey = "channel_id"
	ContextKeyChannelName                      ContextKey = "channel_name"
	ContextKeyChannelCreateTime                ContextKey = "channel_create_time"
	ContextKeyChannelBaseUrl                   ContextKey = "base_url"
	ContextKeyChannelType                      ContextKey = "channel_type"
	ContextKeyChannelSetting                   ContextKey = "channel_setting"
	ContextKeyChannelOtherSetting              ContextKey = "channel_other_setting"
	ContextKeyChannelParamOverride             ContextKey = "param_override"
	ContextKeyChannelHeaderOverride            ContextKey = "header_override"
	ContextKeyChannelOrganization              ContextKey = "channel_organization"
	ContextKeyChannelAutoBan                   ContextKey = "auto_ban"
	ContextKeyChannelModelMapping              ContextKey = "model_mapping"
	ContextKeyChannelStatusCodeMapping         ContextKey = "status_code_mapping"
	ContextKeyChannelIsMultiKey                ContextKey = "channel_is_multi_key"
	ContextKeyChannelMultiKeyIndex             ContextKey = "channel_multi_key_index"
	ContextKeyChannelKey                       ContextKey = "channel_key"
	ContextKeyChannelAttemptStartTime          ContextKey = "channel_attempt_start_time"
	ContextKeyChannelMessagesToResponsesCompat ContextKey = "channel_messages_to_responses_compat"

	/* user related keys */
	ContextKeyUserId      ContextKey = "id"
	ContextKeyUserSetting ContextKey = "user_setting"
	ContextKeyUserQuota   ContextKey = "user_quota"
	ContextKeyUserStatus  ContextKey = "user_status"
	ContextKeyUserEmail   ContextKey = "user_email"
	ContextKeyUserGroup   ContextKey = "user_group"
	ContextKeyUsingGroup  ContextKey = "group"
	// ContextKeyUserGroupId is the audience/user-group context key.
	// Do not confuse it with the legacy default model-group field users.group_id.
	ContextKeyUserGroupId          ContextKey = "user_group_id"
	ContextKeyDefaultModelGroupId  ContextKey = "default_model_group_id"
	ContextKeyUsingGroupId         ContextKey = "group_id"
	ContextKeyUserName             ContextKey = "username"
	ContextKeyUserDailyQuotaLimit  ContextKey = "user_daily_quota_limit"
	ContextKeyUserDailyQuotaUsed   ContextKey = "user_daily_quota_used"
	ContextKeyUserBaseMultiplier   ContextKey = "user_base_multiplier"
	ContextKeyUserPlanType         ContextKey = "user_plan_type"
	ContextKeyUserPlanStartAt      ContextKey = "user_plan_start_at"
	ContextKeyUserPlanExpireAt     ContextKey = "user_plan_expire_at"
	ContextKeyUserAdminPermissions ContextKey = "user_admin_permissions"

	ContextKeySystemPromptOverride ContextKey = "system_prompt_override"

	// streaming diagnostics
	ContextKeyStreamExitReason ContextKey = "stream_exit_reason"
	ContextKeyStreamExitError  ContextKey = "stream_exit_error"
	ContextKeyUpstreamSSEEvent ContextKey = "upstream_sse_event"
)
