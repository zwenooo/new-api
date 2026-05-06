package router

import (
	"one-api/constant"
	"one-api/controller"
	"one-api/middleware"

	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
)

func SetApiRouter(router *gin.Engine) {
	apiRouter := router.Group("/api")
	apiRouter.Use(gzip.Gzip(
		gzip.DefaultCompression,
		gzip.WithExcludedPaths([]string{"/api/user/epay", "/api/subscription/epay", "/api/payg/epay", "/api/pay_request/epay", "/api/pay_token/epay"}),
	))
	apiRouter.Use(middleware.GlobalAPIRateLimit())
	apiRouter.Use(middleware.BodyStorageCleanup())
	{
		apiRouter.GET("/setup", controller.GetSetup)
		apiRouter.POST("/setup", controller.PostSetup)
		// 禁止缓存：避免 /api/status 被浏览器或中间层缓存，导致系统公告（announcements）无法即时刷新
		apiRouter.GET("/status", middleware.DisableCache(), controller.GetStatus)
		apiRouter.GET("/uptime/status", controller.GetUptimeKumaStatus)
		apiRouter.GET("/service_status/timeline.png", middleware.DisableCache(), controller.GetServiceStatusTimelinePNG)
		apiRouter.GET("/service_status/timeline.svg", middleware.DisableCache(), controller.GetServiceStatusTimelineSVG)
		apiRouter.GET("/service_status/timeline", middleware.UserAuth(), controller.GetServiceStatusTimeline)
		apiRouter.GET("/models", middleware.UserAuth(), controller.DashboardListModels)
		apiRouter.GET("/status/test", middleware.AdminAuth(), controller.TestStatus)
		apiRouter.GET("/tmp_stats/relay_billing", middleware.AdminAuth(), controller.GetTmpRelayBillingStats)
		apiRouter.POST("/tmp_stats/relay_billing/reset", middleware.AdminAuth(), controller.ResetTmpRelayBillingStats)
		// 通知也不应被缓存，保持行为一致
		apiRouter.GET("/notice", middleware.DisableCache(), controller.GetNotice)
		apiRouter.GET("/about", controller.GetAbout)
		//apiRouter.GET("/midjourney", controller.GetMidjourney)
		apiRouter.GET("/home_page_content", controller.GetHomePageContent)
		apiRouter.GET("/pricing", middleware.TryUserAuth(), controller.GetPricing)
		apiRouter.GET("/verification", middleware.EmailVerificationRateLimit(), middleware.TurnstileCheck(), controller.SendEmailVerification)
		apiRouter.GET("/reset_password", middleware.CriticalRateLimit(), middleware.TurnstileCheck(), controller.SendPasswordResetEmail)
		apiRouter.POST("/user/reset", middleware.CriticalRateLimit(), controller.ResetPassword)
		apiRouter.GET("/oauth/github", middleware.CriticalRateLimit(), controller.GitHubOAuth)
		apiRouter.GET("/oauth/oidc", middleware.CriticalRateLimit(), controller.OidcAuth)
		apiRouter.GET("/oauth/linuxdo", middleware.CriticalRateLimit(), controller.LinuxdoOAuth)
		apiRouter.GET("/oauth/state", middleware.CriticalRateLimit(), controller.GenerateOAuthCode)
		apiRouter.GET("/oauth/wechat", middleware.CriticalRateLimit(), controller.WeChatAuth)
		apiRouter.GET("/oauth/wechat/bind", middleware.CriticalRateLimit(), controller.WeChatBind)
		apiRouter.GET("/oauth/email/bind", middleware.CriticalRateLimit(), controller.EmailBind)
		apiRouter.GET("/oauth/telegram/login", middleware.CriticalRateLimit(), controller.TelegramLogin)
		apiRouter.GET("/oauth/telegram/bind", middleware.CriticalRateLimit(), controller.TelegramBind)
		apiRouter.GET("/ratio_config", middleware.CriticalRateLimit(), controller.GetRatioConfig)

		apiRouter.POST("/stripe/webhook", controller.StripeWebhook)

		clawboxRoute := apiRouter.Group("/clawbox")
		{
			clawboxRoute.GET("/bootstrap", controller.GetClawBoxBootstrap)
			clawboxRoute.GET("/relay-token", middleware.UserAuth(), controller.GetClawBoxRelayToken)
			clawboxRoute.POST("/relay-token", middleware.UserAuth(), controller.GetClawBoxRelayToken)
			clawboxRoute.POST("/activation/check", middleware.CriticalRateLimit(), controller.CheckClawBoxActivationCode)
			clawboxRoute.POST("/register", middleware.CriticalRateLimit(), controller.RegisterClawBox)
			clawboxRoute.GET("/update/bundled-latest.json", controller.GetClawBoxBundledUpdate)
			clawboxRoute.PUT("/update/bundled-latest.json", middleware.AdminAuth(), controller.SetClawBoxBundledUpdate)
			clawboxRoute.GET("/update/desktop-latest.json", middleware.DisableCache(), controller.GetClawBoxInstalledUpdate)
			clawboxRoute.GET("/update/desktop/releases/:version/download", controller.DownloadClawBoxInstalledRelease)
			clawboxRoute.GET("/update/portable/status", middleware.DisableCache(), controller.GetClawBoxPortableUpdateStatus)
			clawboxRoute.GET("/update/portable-status.json", middleware.DisableCache(), controller.GetClawBoxPortableUpdateStatus)
			clawboxRoute.GET("/update/portable-latest.json", middleware.DisableCache(), controller.GetClawBoxPortableUpdate)
			clawboxRoute.GET("/update/portable-releases.json", middleware.DisableCache(), controller.GetClawBoxPortableReleaseCatalog)
			clawboxRoute.GET("/update/portable/releases/:id/manifest", middleware.DisableCache(), controller.GetClawBoxPortableReleaseManifest)
			clawboxRoute.GET("/update/portable/releases/:id/download", controller.DownloadClawBoxPortableRelease)
			clawboxRoute.POST("/auth/verify", middleware.UserAuth(), controller.VerifyClawBoxAuth)
			clawboxRoute.POST("/auth/reactivate", middleware.UserAuth(), controller.ReactivateClawBoxPortableMedium)
			clawboxRoute.POST("/auth/unregister-device", middleware.UserAuth(), controller.UnregisterClawBoxDevice)

			clawboxAdminRoute := clawboxRoute.Group("/update/portable")
			clawboxAdminRoute.Use(middleware.AdminAuth())
			{
				clawboxAdminRoute.GET("/github-token", controller.GetClawBoxPortableGitHubToken)
				clawboxAdminRoute.PUT("/github-token", controller.SetClawBoxPortableGitHubToken)
				clawboxAdminRoute.DELETE("/github-token", controller.ClearClawBoxPortableGitHubToken)
				clawboxAdminRoute.GET("/releases", controller.ListClawBoxPortableReleases)
				clawboxAdminRoute.POST("/releases", controller.CreateClawBoxPortableRelease)
				clawboxAdminRoute.POST("/releases/sync/github", controller.SyncClawBoxPortableReleaseFromGitHub)
				clawboxAdminRoute.POST("/releases/:id/activate", controller.ActivateClawBoxPortableRelease)
				clawboxAdminRoute.DELETE("/releases/:id", controller.DeleteClawBoxPortableRelease)
			}
		}

		userRoute := apiRouter.Group("/user")
		{
			userRoute.POST("/register", middleware.CriticalRateLimit(), middleware.TurnstileCheck(), controller.Register)
			userRoute.POST("/login", middleware.CriticalRateLimit(), middleware.TurnstileCheck(), controller.Login)
			userRoute.POST("/login/2fa", middleware.CriticalRateLimit(), controller.Verify2FALogin)
			//userRoute.POST("/tokenlog", middleware.CriticalRateLimit(), controller.TokenLog)
			userRoute.GET("/logout", controller.Logout)
			userRoute.GET("/epay/notify", controller.EpayNotify)
			userRoute.POST("/epay/notify", controller.EpayNotify)
			userRoute.GET("/groups", controller.GetUserGroups)

			selfRoute := userRoute.Group("/")
			selfRoute.Use(middleware.UserAuth())
			{
				selfRoute.GET("/self/groups", controller.GetUserGroups)
				selfRoute.GET("/self", controller.GetSelf)
				selfRoute.POST("/self/subscriptions/:subId/activate", controller.ActivateSelfSubscription)
				selfRoute.POST("/self/request_subscriptions/:subId/activate", controller.ActivateSelfRequestSubscription)
				selfRoute.PUT("/avatar", controller.UpdateAvatar)
				selfRoute.GET("/models", controller.GetUserModels)
				selfRoute.PUT("/self", controller.UpdateSelf)
				selfRoute.DELETE("/self", controller.DeleteSelf)
				selfRoute.GET("/token", controller.GenerateAccessToken)
				selfRoute.GET("/aff", controller.GetAffCode)
				selfRoute.GET("/aff/records", controller.ListInvitationSubscriptionRecords)
				selfRoute.GET("/balance/records", controller.ListSelfBalanceRecords)
				selfRoute.GET("/topup/info", controller.GetTopUpInfo)
				selfRoute.POST("/topup", middleware.CriticalRateLimit(), controller.TopUp)
				selfRoute.POST("/pay", middleware.CriticalRateLimit(), controller.RequestEpay)
				selfRoute.POST("/amount", controller.RequestAmount)
				selfRoute.POST("/stripe/pay", middleware.CriticalRateLimit(), controller.RequestStripePay)
				selfRoute.POST("/stripe/amount", controller.RequestStripeAmount)
				selfRoute.POST("/aff_transfer", controller.TransferAffQuota)
				selfRoute.PUT("/setting", controller.UpdateUserSetting)
				// 2FA routes
				selfRoute.GET("/2fa/status", controller.Get2FAStatus)
				selfRoute.POST("/2fa/setup", controller.Setup2FA)
				selfRoute.POST("/2fa/enable", controller.Enable2FA)
				selfRoute.POST("/2fa/disable", controller.Disable2FA)
				selfRoute.POST("/2fa/backup_codes", controller.RegenerateBackupCodes)
			}

			adminRoute := userRoute.Group("/")
			adminRoute.Use(middleware.AdminAuth())
			{
				adminRoute.GET("/", controller.GetAllUsers)
				adminRoute.GET("/search", controller.SearchUsers)
				adminRoute.GET("/:id", controller.GetUser)
				adminRoute.POST("/", controller.CreateUser)
				adminRoute.POST("/manage", controller.ManageUser)
				adminRoute.PUT("/", controller.UpdateUser)
				adminRoute.DELETE("/:id", controller.DeleteUser)
				adminRoute.GET("/:id/subscriptions", controller.ListUserSubscriptions)
				adminRoute.POST("/:id/subscriptions", controller.CreateUserSubscription)
				adminRoute.POST("/:id/subscriptions/preset", controller.CreateUserSubscriptionByPreset)
				adminRoute.PATCH("/:id/subscriptions/reorder", controller.ReorderUserSubscriptions)
				adminRoute.PATCH("/:id/subscriptions/:subId", controller.UpdateUserSubscription)
				adminRoute.DELETE("/:id/subscriptions/:subId", controller.DeleteUserSubscription)
				adminRoute.GET("/:id/request_subscriptions", controller.ListUserRequestSubscriptions)
				adminRoute.POST("/:id/request_subscriptions", controller.CreateUserRequestSubscription)
				adminRoute.POST("/:id/request_subscriptions/preset", controller.CreateUserRequestSubscriptionByPreset)
				adminRoute.PATCH("/:id/request_subscriptions/reorder", controller.ReorderUserRequestSubscriptions)
				adminRoute.PATCH("/:id/request_subscriptions/:subId", controller.UpdateUserRequestSubscription)
				adminRoute.DELETE("/:id/request_subscriptions/:subId", controller.DeleteUserRequestSubscription)
				adminRoute.POST("/:id/payg/topup", controller.AdminTopupUserPayg)
				adminRoute.POST("/:id/payg/topup/group", controller.AdminTopupUserPaygByGroup)
				adminRoute.PATCH("/:id/payg/balances/reorder", controller.AdminReorderUserPaygBalances)
				adminRoute.PATCH("/:id/payg/balances/:productId", controller.AdminUpdateUserPaygBalanceAllowedGroups)
				adminRoute.DELETE("/:id/payg/balances/:productId", controller.AdminDeleteUserPaygBalance)
				adminRoute.POST("/subscriptions/bulk/duration", controller.BulkUpdateUserSubscriptionDuration)
				adminRoute.POST("/subscriptions/bulk/compensation", controller.BulkCompensateSubscriptionsByPreset)
				adminRoute.POST("/subscriptions/bulk/original-compensation", controller.BulkExtendOriginalSubscriptions)
				// Admin 2FA routes
				adminRoute.GET("/2fa/stats", controller.Admin2FAStats)
				adminRoute.DELETE("/:id/2fa", controller.AdminDisable2FA)
			}
		}

		subscriptionRoute := apiRouter.Group("/subscription")
		{
			subscriptionRoute.GET("/plans", middleware.UserAuth(), controller.ListSubscriptionPlans)
			subscriptionRoute.POST("/order", middleware.CriticalRateLimit(), middleware.UserAuth(), controller.CreateSubscriptionOrder)
			subscriptionRoute.GET("/order/status", middleware.UserAuth(), controller.GetSubscriptionOrderStatus)
			subscriptionRoute.GET("/epay/notify", controller.SubscriptionEpayNotify)
			subscriptionRoute.POST("/epay/notify", controller.SubscriptionEpayNotify)
			subscriptionRoute.GET("/epay/return", controller.SubscriptionEpayReturn)

			subscriptionAdmin := subscriptionRoute.Group("/")
			subscriptionAdmin.Use(middleware.RootAuth())
			{
				subscriptionAdmin.GET("/plans/all", controller.AdminListSubscriptionPlans)
				subscriptionAdmin.POST("/plans", controller.AdminCreateSubscriptionPlan)
				subscriptionAdmin.PUT("/plans/:id", controller.AdminUpdateSubscriptionPlan)
				subscriptionAdmin.DELETE("/plans/:id", controller.AdminDeleteSubscriptionPlan)
			}
		}

		paygRoute := apiRouter.Group("/payg")
		{
			paygRoute.POST("/order", middleware.CriticalRateLimit(), middleware.UserAuth(), controller.CreatePaygOrder)
			paygRoute.GET("/order/status", middleware.UserAuth(), controller.GetPaygOrderStatus)
			paygRoute.GET("/epay/checkout", controller.PaygEpayCheckout)
			paygRoute.GET("/epay/notify", controller.PaygEpayNotify)
			paygRoute.POST("/epay/notify", controller.PaygEpayNotify)
			paygRoute.GET("/epay/return", controller.PaygEpayReturn)
		}

		payRequestRoute := apiRouter.Group("/pay_request")
		{
			payRequestRoute.POST("/order", middleware.CriticalRateLimit(), middleware.UserAuth(), controller.CreatePayRequestOrder)
			payRequestRoute.GET("/order/status", middleware.UserAuth(), controller.GetPayRequestOrderStatus)
			payRequestRoute.GET("/epay/notify", controller.PayRequestEpayNotify)
			payRequestRoute.POST("/epay/notify", controller.PayRequestEpayNotify)
			payRequestRoute.GET("/epay/return", controller.PayRequestEpayReturn)
		}

		payTokenRoute := apiRouter.Group("/pay_token")
		{
			payTokenRoute.POST("/order", middleware.CriticalRateLimit(), middleware.UserAuth(), controller.CreatePayTokenOrder)
			payTokenRoute.GET("/order/status", middleware.UserAuth(), controller.GetPayTokenOrderStatus)
			payTokenRoute.GET("/epay/notify", controller.PayTokenEpayNotify)
			payTokenRoute.POST("/epay/notify", controller.PayTokenEpayNotify)
			payTokenRoute.GET("/epay/return", controller.PayTokenEpayReturn)
		}

		pricingProfileReadRoute := apiRouter.Group("/pricing_profiles")
		pricingProfileReadRoute.Use(middleware.DisableCache(), middleware.AdminAuth())
		{
			pricingProfileReadRoute.GET("/", controller.ListPricingProfiles)
		}

		pricingProfileAdminRoute := apiRouter.Group("/pricing_profiles")
		pricingProfileAdminRoute.Use(middleware.DisableCache(), middleware.RootAuth())
		{
			pricingProfileAdminRoute.GET("/legacy_users", controller.ListLegacyPricingUsers)
			pricingProfileAdminRoute.POST("/", controller.CreatePricingProfile)
			pricingProfileAdminRoute.PUT("/:id", controller.UpdatePricingProfile)
			pricingProfileAdminRoute.DELETE("/:id", controller.DeletePricingProfile)
		}

		orderRoute := apiRouter.Group("/order")
		orderRoute.Use(middleware.AdminAuth(), middleware.RequireAdminModulePermission(constant.AdminModuleOrder))
		{
			orderRoute.GET("/subscriptions", controller.AdminListSubscriptionOrders)
			orderRoute.GET("/topups", controller.AdminListTopUpOrders)
			orderRoute.GET("/paygs", controller.AdminListPaygOrders)
			orderRoute.GET("/pay_requests", controller.AdminListPayRequestOrders)
			orderRoute.GET("/pay_tokens", controller.AdminListPayTokenOrders)
			orderRoute.GET("/stats/daily", controller.AdminGetOrderDailyRevenueStats)
		}

		productManagementRoute := apiRouter.Group("/product_management")
		productManagementRoute.Use(middleware.AdminAuth(), middleware.RequireAdminModulePermission(constant.AdminModuleProductManagement))
		{
			productManagementRoute.GET("/option", controller.GetProductManagementOptions)
			productManagementRoute.PUT("/option", controller.UpdateProductManagementOption)
			productManagementRoute.POST("/reorder", controller.ReorderProductManagementProducts)
			productManagementRoute.GET("/presets", controller.ListProductManagementPresets)
			productManagementRoute.POST("/presets", controller.UpsertProductManagementPreset)
			productManagementRoute.GET("/presets/:id/revisions", controller.ListProductManagementPresetRevisions)
			productManagementRoute.POST("/presets/:id/restore", controller.RestoreProductManagementPresetRevision)
			productManagementRoute.GET("/pay_products/:type/:id/revisions", controller.ListProductManagementPayProductRevisions)
			productManagementRoute.POST("/pay_products/:type/:id/restore", controller.RestoreProductManagementPayProductRevision)
			productManagementRoute.DELETE("/presets/:id", controller.DeleteProductManagementPreset)
			productManagementRoute.POST("/presets/generate", controller.GenerateProductManagementPresetRedemptions)
		}

		optionRoute := apiRouter.Group("/option")
		optionRoute.Use(middleware.RootAuth())
		{
			optionRoute.GET("/", controller.GetOptions)
			optionRoute.PUT("/", controller.UpdateOption)
			optionRoute.POST("/rest_model_ratio", controller.ResetModelRatio)
			optionRoute.POST("/migrate_console_setting", controller.MigrateConsoleSetting) // 用于迁移检测的旧键，下个版本会删除
		}

		cxCompatRoute := apiRouter.Group("/cx_compat")
		cxCompatRoute.Use(middleware.RootAuth())
		{
			cxCompatRoute.GET("/opencode/instructions", controller.GetCxCompatOpenCodeInstructions)
			cxCompatRoute.POST("/opencode/instructions/sync", controller.SyncCxCompatOpenCodeInstructions)
			cxCompatRoute.POST("/opencode/instructions/sync/github", controller.SyncCxCompatOpenCodeInstructionsFromGitHub)
			cxCompatRoute.GET("/opencode/instructions/github/branches", controller.GetCxCompatOpenCodeGitHubBranches)
			cxCompatRoute.GET("/opencode/instructions/github/commits", controller.GetCxCompatOpenCodeGitHubCommits)
			cxCompatRoute.POST("/opencode/instructions/pin_default", controller.PinCxCompatOpenCodeInstructionsAsDefault)
			cxCompatRoute.POST("/opencode/instructions/restore_default", controller.RestoreCxCompatOpenCodeInstructionsDefault)
		}
		ratioSyncRoute := apiRouter.Group("/ratio_sync")
		ratioSyncRoute.Use(middleware.RootAuth())
		{
			ratioSyncRoute.GET("/channels", controller.GetSyncableChannels)
			ratioSyncRoute.POST("/fetch", controller.FetchUpstreamRatios)
		}
		performanceRoute := apiRouter.Group("/performance")
		performanceRoute.Use(middleware.RootAuth())
		{
			performanceRoute.GET("/stats", controller.GetPerformanceStats)
			performanceRoute.DELETE("/disk_cache", controller.ClearDiskCache)
			performanceRoute.POST("/reset_stats", controller.ResetPerformanceStats)
			performanceRoute.POST("/gc", controller.ForceGC)
		}
		channelRoute := apiRouter.Group("/channel")
		channelRoute.Use(middleware.AdminAuth())
		{
			channelRoute.GET("/", controller.GetAllChannels)
			channelRoute.GET("/search", controller.SearchChannels)
			channelRoute.GET("/models", controller.ChannelListModels)
			channelRoute.GET("/models_enabled", controller.EnabledListModels)
			channelRoute.GET("/abnormal_consume/config", controller.GetChannelAbnormalConsumeConfig)
			channelRoute.PUT("/abnormal_consume/config", controller.UpdateChannelAbnormalConsumeConfig)
			channelRoute.GET("/abnormal_consume", controller.ListChannelAbnormalConsumeRecords)
			channelRoute.DELETE("/abnormal_consume", controller.ClearChannelAbnormalConsumeRecords)
			channelRoute.GET("/:id", controller.GetChannel)
			channelRoute.GET("/:id/profit_stats/daily", controller.GetChannelProfitDailyStats)
			channelRoute.GET("/:id/request_stats/daily", controller.GetChannelRequestDailyStats)
			channelRoute.POST("/:id/key", middleware.CriticalRateLimit(), middleware.DisableCache(), controller.GetChannelKey)
			channelRoute.GET("/test", controller.TestAllChannels)
			channelRoute.GET("/test/:id", controller.TestChannel)
			channelRoute.POST("/test_proxy", controller.TestProxy)
			channelRoute.GET("/update_balance", controller.UpdateAllChannelsBalance)
			channelRoute.GET("/update_balance/:id", controller.UpdateChannelBalance)
			channelRoute.POST("/", controller.AddChannel)
			channelRoute.PUT("/", controller.UpdateChannel)
			channelRoute.DELETE("/disabled", controller.DeleteDisabledChannel)
			channelRoute.POST("/tag/disabled", controller.DisableTagChannels)
			channelRoute.POST("/tag/enabled", controller.EnableTagChannels)
			channelRoute.PUT("/tag", controller.EditTagChannels)
			channelRoute.DELETE("/:id", controller.DeleteChannel)
			channelRoute.POST("/reset_used_quota/:id", controller.ResetChannelUsedQuota)
			channelRoute.POST("/batch", controller.DeleteChannelBatch)
			channelRoute.POST("/batch/reset_used_quota", controller.BatchResetChannelUsedQuota)
			channelRoute.POST("/batch/group", controller.BatchSetChannelGroup)
			channelRoute.POST("/batch/models", controller.BatchUpdateChannelModels)
			channelRoute.POST("/batch/bind_users", controller.BatchBindChannelUsers)
			channelRoute.POST("/fix", controller.FixChannelsAbilities)
			channelRoute.GET("/fetch_models/:id", controller.FetchUpstreamModels)
			channelRoute.POST("/fetch_models", controller.FetchModels)
			channelRoute.POST("/upstream_updates/detect", controller.DetectChannelUpstreamModelUpdates)
			channelRoute.POST("/upstream_updates/detect_all", controller.DetectAllChannelUpstreamModelUpdates)
			channelRoute.POST("/upstream_updates/apply", controller.ApplyChannelUpstreamModelUpdates)
			channelRoute.POST("/upstream_updates/apply_all", controller.ApplyAllChannelUpstreamModelUpdates)
			channelRoute.POST("/batch/tag", controller.BatchSetChannelTag)
			channelRoute.GET("/tag/models", controller.GetTagModels)
			channelRoute.POST("/copy/:id", controller.CopyChannel)
			channelRoute.POST("/multi_key/manage", controller.ManageMultiKeys)
		}
		tokenRoute := apiRouter.Group("/token")
		tokenRoute.Use(middleware.UserAuth())
		{
			tokenRoute.GET("/", controller.GetAllTokens)
			tokenRoute.GET("/search", controller.SearchTokens)
			tokenRoute.GET("/:id", controller.GetToken)
			tokenRoute.POST("/", controller.AddToken)
			tokenRoute.PUT("/", controller.UpdateToken)
			tokenRoute.DELETE("/:id", controller.DeleteToken)
			tokenRoute.POST("/batch", controller.DeleteTokenBatch)
		}

		usageRoute := apiRouter.Group("/usage")
		usageRoute.Use(middleware.CriticalRateLimit())
		{
			tokenUsageRoute := usageRoute.Group("/token")
			tokenUsageRoute.Use(middleware.TokenAuth())
			{
				tokenUsageRoute.GET("/", controller.GetTokenUsage)
			}
		}

		redemptionRoute := apiRouter.Group("/redemption")
		redemptionRoute.Use(middleware.AdminAuth())
		{
			redemptionRoute.GET("/", controller.GetAllRedemptions)
			redemptionRoute.GET("/search", controller.SearchRedemptions)
			redemptionRoute.GET("/presets", controller.ListRedemptionPresets)
			redemptionRoute.POST("/presets", controller.UpsertRedemptionPreset)
			redemptionRoute.GET("/presets/:id/revisions", controller.ListRedemptionPresetRevisions)
			redemptionRoute.POST("/presets/:id/restore", controller.RestoreRedemptionPresetRevision)
			redemptionRoute.DELETE("/presets/:id", controller.DeleteRedemptionPreset)
			redemptionRoute.POST("/presets/generate", controller.GenerateRedemptionByPreset)
			redemptionRoute.POST("/payg/generate", controller.GeneratePaygRedemptions)
			redemptionRoute.GET("/:id", controller.GetRedemption)
			redemptionRoute.POST("/", controller.AddRedemption)
			redemptionRoute.PUT("/", controller.UpdateRedemption)
			redemptionRoute.POST("/batch/status", controller.BatchUpdateRedemptionStatus)
			redemptionRoute.DELETE("/invalid", controller.DeleteInvalidRedemption)
			redemptionRoute.DELETE("/:id", controller.DeleteRedemption)
		}
		logRoute := apiRouter.Group("/log")
		logRoute.GET("/", middleware.AdminAuth(), controller.GetAllLogs)
		logRoute.DELETE("/", middleware.AdminAuth(), controller.DeleteHistoryLogs)
		logRoute.GET("/stat", middleware.AdminAuth(), controller.GetLogsStat)
		logRoute.GET("/king_rank", controller.GetDailyTokenKingRank)
		logRoute.GET("/token_quota_stat", middleware.AdminAuth(), controller.GetLogsTokenQuotaStat)
		logRoute.GET("/cache_stat", middleware.AdminAuth(), controller.GetLogsCacheStat)
		logRoute.GET("/cache_stat/by_ua", middleware.AdminAuth(), controller.GetLogsCacheStatByUA)
		logRoute.GET("/self/stat", middleware.UserAuth(), controller.GetLogsSelfStat)
		logRoute.GET("/self/token_quota_stat", middleware.UserAuth(), controller.GetLogsSelfTokenQuotaStat)
		logRoute.GET("/self/cache_stat", middleware.UserAuth(), controller.GetLogsSelfCacheStat)
		logRoute.GET("/self/cache_stat/by_ua", middleware.UserAuth(), controller.GetLogsSelfCacheStatByUA)
		logRoute.GET("/global/cache_stat", middleware.UserAuth(), controller.GetLogsGlobalCacheStat)
		logRoute.GET("/global/cache_stat/by_ua", middleware.UserAuth(), controller.GetLogsGlobalCacheStatByUA)
		logRoute.GET("/search", middleware.AdminAuth(), controller.SearchAllLogs)
		logRoute.GET("/self", middleware.UserAuth(), controller.GetUserLogs)
		logRoute.GET("/self/search", middleware.UserAuth(), controller.SearchUserLogs)

		traceRoute := apiRouter.Group("/request_trace")
		{
			// download_url is opened in a browser tab; allow session-based auth when custom headers are absent.
			traceRoute.GET("/object", middleware.AdminPageOrHeaderAuth(), controller.GetRequestTraceObject)
			traceRoute.GET("/:request_id", middleware.AdminAuth(), controller.GetRequestTrace)
		}

		dataRoute := apiRouter.Group("/data")
		dataRoute.GET("/", middleware.AdminAuth(), controller.GetAllQuotaDates)
		dataRoute.GET("/self", middleware.UserAuth(), controller.GetUserQuotaDates)

		logRoute.Use(middleware.CORS())
		{
			logRoute.GET("/token", controller.GetLogByKey)
		}

		apiRouter.GET("/group/resolve", middleware.UserAuth(), controller.ResolveGroups)

		groupRoute := apiRouter.Group("/group")
		groupRoute.Use(middleware.AdminAuth())
		{
			groupRoute.GET("/no_billing/product_options", controller.GetGroupNoBillingProductOptions)
			groupRoute.GET("/:id/channels", controller.GetGroupChannels)
			groupRoute.GET("/:id/user_price_overrides", controller.GetGroupUserPriceOverrides)
			groupRoute.POST("/:id/token_remap", controller.RemapGroupTokens)
			groupRoute.GET("/", controller.GetGroups)
			groupRoute.POST("/", controller.CreateGroup)
			groupRoute.PUT("/:id/channels", controller.SyncGroupChannels)
			groupRoute.PUT("/:id/user_price_overrides", controller.SyncGroupUserPriceOverrides)
			groupRoute.PUT("/", controller.UpdateGroup)
			groupRoute.DELETE("/:id", controller.DeleteGroup)
		}

		userGroupRoute := apiRouter.Group("/user_group")
		userGroupRoute.Use(middleware.AdminAuth())
		{
			userGroupRoute.GET("/", controller.GetUserGroupsAdmin)
			userGroupRoute.POST("/", controller.CreateUserGroup)
			userGroupRoute.PUT("/", controller.UpdateUserGroup)
			userGroupRoute.DELETE("/:id", controller.DeleteUserGroup)
		}

		prefillGroupRoute := apiRouter.Group("/prefill_group")
		prefillGroupRoute.Use(middleware.AdminAuth())
		{
			prefillGroupRoute.GET("/", controller.GetPrefillGroups)
			prefillGroupRoute.POST("/", controller.CreatePrefillGroup)
			prefillGroupRoute.PUT("/", controller.UpdatePrefillGroup)
			prefillGroupRoute.DELETE("/:id", controller.DeletePrefillGroup)
		}

		mjRoute := apiRouter.Group("/mj")
		mjRoute.GET("/self", middleware.UserAuth(), controller.GetUserMidjourney)
		mjRoute.GET("/", middleware.AdminAuth(), controller.GetAllMidjourney)

		taskRoute := apiRouter.Group("/task")
		{
			taskRoute.GET("/self", middleware.UserAuth(), controller.GetUserTask)
			taskRoute.GET("/", middleware.AdminAuth(), controller.GetAllTask)
		}

		vendorRoute := apiRouter.Group("/vendors")
		vendorRoute.Use(middleware.AdminAuth())
		{
			vendorRoute.GET("/", controller.GetAllVendors)
			vendorRoute.GET("/search", controller.SearchVendors)
			vendorRoute.GET("/:id", controller.GetVendorMeta)
			vendorRoute.POST("/", controller.CreateVendorMeta)
			vendorRoute.PUT("/", controller.UpdateVendorMeta)
			vendorRoute.DELETE("/:id", controller.DeleteVendorMeta)
		}

		modelsRoute := apiRouter.Group("/models")
		modelsRoute.Use(middleware.AdminAuth())
		{
			modelsRoute.GET("/sync_upstream/preview", controller.SyncUpstreamPreview)
			modelsRoute.POST("/sync_upstream", controller.SyncUpstreamModels)
			modelsRoute.GET("/missing", controller.GetMissingModels)
			modelsRoute.GET("/", controller.GetAllModelsMeta)
			modelsRoute.GET("/search", controller.SearchModelsMeta)
			modelsRoute.GET("/:id", controller.GetModelMeta)
			modelsRoute.POST("/", controller.CreateModelMeta)
			modelsRoute.PUT("/", controller.UpdateModelMeta)
			modelsRoute.DELETE("/:id", controller.DeleteModelMeta)
		}
	}
}
