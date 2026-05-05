package controller

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"one-api/common"
	"one-api/model"
	"one-api/service"
	"one-api/setting/operation_setting"
	"one-api/setting/system_setting"
	"strconv"
	"strings"
	"time"

	"github.com/Calcium-Ion/go-epay/epay"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type createSubscriptionOrderRequest struct {
	PlanId     int    `json:"plan_id" binding:"required"`
	PayMethod  string `json:"pay_method" binding:"required"`   // balance / epay
	EpayMethod string `json:"epay_method" binding:"omitempty"` // alipay / wxpay / ...
	ApplyMode  string `json:"apply_mode" binding:"required"`   // stack / defer
	Quantity   *int   `json:"quantity" binding:"omitempty"`    // default 1
}

func ListSubscriptionPlans(c *gin.Context) {
	// 订阅购买页的“订阅套餐”实际以兑换码“预置商品（订阅额度）”为准
	plans, err := model.ListSubscriptionRedemptionPresets()
	if err != nil {
		common.ApiError(c, err)
		return
	}

	type subscriptionPresetPlan struct {
		*model.RedemptionPreset
		PurchasedCount int64 `json:"purchased_count"`
	}

	userId := c.GetInt("id")
	if userId <= 0 || len(plans) == 0 {
		resp := make([]subscriptionPresetPlan, 0, len(plans))
		for _, p := range plans {
			if p == nil {
				continue
			}
			resp = append(resp, subscriptionPresetPlan{RedemptionPreset: p, PurchasedCount: 0})
		}
		common.ApiSuccess(c, resp)
		return
	}

	presetIds := make([]int, 0, len(plans))
	for _, p := range plans {
		if p == nil || p.Id <= 0 {
			continue
		}
		presetIds = append(presetIds, p.Id)
	}

	purchasedByPreset := make(map[int]int64, len(presetIds))
	if len(presetIds) > 0 {
		type row struct {
			PresetId int
			Count    int64
		}
		var rows []row
		if err := model.DB.Model(&model.SubscriptionOrder{}).
			Select("preset_id, COALESCE(SUM(quantity),0) AS count").
			Where("user_id = ? AND preset_id IN ? AND status = ?", userId, presetIds, model.SubscriptionOrderStatusSuccess).
			Group("preset_id").
			Scan(&rows).Error; err != nil {
			common.ApiError(c, err)
			return
		}
		for _, r := range rows {
			if r.PresetId <= 0 || r.Count <= 0 {
				continue
			}
			purchasedByPreset[r.PresetId] = r.Count
		}
	}

	resp := make([]subscriptionPresetPlan, 0, len(plans))
	for _, p := range plans {
		if p == nil {
			continue
		}
		resp = append(resp, subscriptionPresetPlan{
			RedemptionPreset: p,
			PurchasedCount:   purchasedByPreset[p.Id],
		})
	}
	common.ApiSuccess(c, resp)
}

func AdminListSubscriptionPlans(c *gin.Context) {
	plans, err := model.GetAllSubscriptionPlans()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, plans)
}

type upsertSubscriptionPlanRequest struct {
	Name         string `json:"name" binding:"required"`
	Description  string `json:"description"`
	PriceFen     int64  `json:"price_fen" binding:"required"`
	DurationDays int    `json:"duration_days" binding:"required"`
	Meta         string `json:"meta"`
	Enabled      bool   `json:"enabled"`
	SortOrder    int    `json:"sort_order"`
}

func AdminCreateSubscriptionPlan(c *gin.Context) {
	var req upsertSubscriptionPlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	plan := &model.SubscriptionPlan{
		Name:         strings.TrimSpace(req.Name),
		Description:  strings.TrimSpace(req.Description),
		PriceFen:     req.PriceFen,
		DurationDays: req.DurationDays,
		Meta:         strings.TrimSpace(req.Meta),
		Enabled:      req.Enabled,
		SortOrder:    req.SortOrder,
	}
	if err := plan.Insert(); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, plan)
}

func AdminUpdateSubscriptionPlan(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "id 无效")
		return
	}
	var req upsertSubscriptionPlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	plan, err := model.GetSubscriptionPlanById(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	plan.Name = strings.TrimSpace(req.Name)
	plan.Description = strings.TrimSpace(req.Description)
	plan.PriceFen = req.PriceFen
	plan.DurationDays = req.DurationDays
	plan.Meta = strings.TrimSpace(req.Meta)
	plan.Enabled = req.Enabled
	plan.SortOrder = req.SortOrder
	if err := plan.Update(); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, plan)
}

func AdminDeleteSubscriptionPlan(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "id 无效")
		return
	}
	if err := model.DeleteSubscriptionPlanById(id); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, true)
}

func formatFenYuan(fen int64) string {
	if fen < 0 {
		fen = -fen
	}
	return fmt.Sprintf("%d.%02d", fen/100, fen%100)
}

func parseYuanToFen(money string) (int64, error) {
	fen, err := common.YuanStringToFen(money)
	if err != nil {
		return 0, err
	}
	if fen <= 0 {
		return 0, errors.New("money 必须大于0")
	}
	return fen, nil
}

func collectEpayCallbackParams(c *gin.Context) map[string]string {
	params := map[string]string{}
	if c == nil || c.Request == nil {
		return params
	}
	_ = c.Request.ParseForm()
	for key, values := range c.Request.Form {
		if len(values) == 0 {
			continue
		}
		params[key] = values[0]
	}
	return params
}

func CreateSubscriptionOrder(c *gin.Context) {
	var req createSubscriptionOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	quantity := 1
	if req.Quantity != nil {
		quantity = *req.Quantity
	}
	if quantity <= 0 || quantity > 100 {
		common.ApiErrorMsg(c, "quantity 无效")
		return
	}
	if req.ApplyMode != model.SubscriptionApplyModeStack && req.ApplyMode != model.SubscriptionApplyModeDefer {
		common.ApiErrorMsg(c, "apply_mode 无效")
		return
	}
	if req.PayMethod != model.SubscriptionPayMethodBalance && req.PayMethod != model.SubscriptionPayMethodEpay {
		common.ApiErrorMsg(c, "pay_method 无效")
		return
	}

	// 订阅购买页的 plan_id 实际为“预置商品（订阅额度）”的 id
	preset, err := model.GetRedemptionPresetById(req.PlanId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	lockedPresetRevision, err := model.EnsureCurrentRedemptionPresetRevision(preset.Id)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	effectiveName := strings.TrimSpace(lockedPresetRevision.Name)
	effectiveMode := strings.TrimSpace(lockedPresetRevision.Mode)
	effectiveEnabled := lockedPresetRevision.Enabled
	effectivePriceFen := lockedPresetRevision.PriceFen
	effectivePurchaseLimit := lockedPresetRevision.PurchaseLimit
	effectiveMultiQuantityEnabled := lockedPresetRevision.MultiQuantityEnabled
	effectiveMultiQuantityDeferOnly := lockedPresetRevision.MultiQuantityDeferOnly
	effectiveQuota := lockedPresetRevision.Quota
	effectiveDailyQuotaLimit := lockedPresetRevision.DailyQuotaLimit
	effectiveDailyRequestLimit := lockedPresetRevision.DailyRequestLimit
	effectiveQuotaValidDays := lockedPresetRevision.QuotaValidDays

	if effectiveMode != "subscription" && effectiveMode != "tokens" && effectiveMode != "request" {
		common.ApiErrorMsg(c, "商品类型错误")
		return
	}
	if !effectiveEnabled {
		common.ApiErrorMsg(c, "商品已下架")
		return
	}
	if effectivePriceFen <= 0 {
		common.ApiErrorMsg(c, "商品价格未配置")
		return
	}
	if preset.Stock != nil && *preset.Stock < quantity {
		common.ApiErrorMsg(c, "商品库存不足")
		return
	}
	switch effectiveMode {
	case "subscription", "tokens":
		if effectiveQuota < 0 || effectiveDailyQuotaLimit < 0 || effectiveQuotaValidDays < 0 {
			common.ApiErrorMsg(c, "商品规格错误")
			return
		}
	case "request":
		if effectiveDailyRequestLimit < 0 || effectiveQuota < 0 || effectiveQuotaValidDays < 0 {
			common.ApiErrorMsg(c, "商品规格错误")
			return
		}
	}
	allowedGroupIDs := []int{}
	if len(lockedPresetRevision.AllowedGroupIds) > 0 {
		var ids []int
		if err := common.Unmarshal([]byte(lockedPresetRevision.AllowedGroupIds), &ids); err != nil {
			common.ApiError(c, err)
			return
		}
		allowedGroupIDs = model.NormalizeUniqueSortedIDs(ids)
	}
	if len(allowedGroupIDs) == 0 {
		common.ApiErrorMsg(c, "商品未配置可用分组")
		return
	}
	if err := model.ValidateGroupIDsExist(nil, allowedGroupIDs); err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	if quantity > 1 && effectiveMultiQuantityDeferOnly && req.ApplyMode != model.SubscriptionApplyModeDefer {
		common.ApiErrorMsg(c, "仅支持顺延")
		return
	}
	if quantity > 1 && !effectiveMultiQuantityEnabled {
		common.ApiErrorMsg(c, "该商品不支持多数量购买")
		return
	}

	totalAmountFen := effectivePriceFen * int64(quantity)
	if totalAmountFen <= 0 {
		common.ApiErrorMsg(c, "订单金额无效")
		return
	}

	userId := c.GetInt("id")
	tradeNo := fmt.Sprintf("SUB%dNO%s%d", userId, common.GetRandomString(6), time.Now().Unix())
	now := common.GetTimestamp()

	// 易支付场景：订单创建成功后可能稍后才支付并回调；先对顺延进行可行性校验，避免支付后因时间溢出等原因无法完成订单。
	if req.ApplyMode == model.SubscriptionApplyModeDefer && req.PayMethod == model.SubscriptionPayMethodEpay {
		startAt := now
		var maxExpire int64
		switch effectiveMode {
		case "subscription":
			maxExpire, err = model.GetUserSubscriptionMaxExpireAt(nil, userId, now)
		case "tokens":
			maxExpire, err = model.GetUserSubscriptionMaxExpireAtWithBillingUnit(nil, userId, now, model.UserSubscriptionBillingUnitTokens)
		case "request":
			maxExpire, err = model.GetUserRequestSubscriptionMaxExpireAt(nil, userId, now)
		default:
			err = errors.New("商品类型错误")
		}
		if err != nil {
			common.ApiError(c, err)
			return
		}
		if maxExpire >= startAt {
			startAt = maxExpire + 1
		}
		if startAt > common.MaxSupportedUnixTimestamp {
			common.ApiErrorMsg(c, "用户订阅到期时间异常")
			return
		}

		if effectiveQuotaValidDays > 0 {
			extendSeconds := int64(effectiveQuotaValidDays) * common.SecondsPerDay * int64(quantity)
			if extendSeconds <= 0 || extendSeconds > common.MaxSupportedUnixTimestamp-startAt {
				common.ApiErrorMsg(c, "订阅有效期过大，最大支持到 "+common.MaxSupportedUnixTimestampLabel)
				return
			}
		} else if effectiveQuotaValidDays == 0 {
			// 0 表示永久
		}
	}

	if req.PayMethod == model.SubscriptionPayMethodBalance {
		var inviterId int
		if err := model.DB.Transaction(func(tx *gorm.DB) error {
			var buyer model.User
			if err := lockForUpdate(tx).
				Select("id", "balance_fen", "inviter_id").
				Where("id = ?", userId).
				First(&buyer).Error; err != nil {
				return err
			}
			inviterId = buyer.InviterId

			if effectivePurchaseLimit > 0 {
				pendingCount, err := model.CountUserSubscriptionOrdersByPresetStatus(tx, userId, preset.Id, model.SubscriptionOrderStatusPending)
				if err != nil {
					return err
				}
				if pendingCount > 0 {
					return errors.New("存在待支付订单，请先完成支付")
				}
				successUnits, err := model.SumUserSubscriptionOrderQuantityByPresetStatus(tx, userId, preset.Id, model.SubscriptionOrderStatusSuccess)
				if err != nil {
					return err
				}
				if successUnits+int64(quantity) > int64(effectivePurchaseLimit) {
					return fmt.Errorf("已达到该商品限购次数（%d 次）", effectivePurchaseLimit)
				}
			}

			if buyer.BalanceFen < totalAmountFen {
				return errors.New("余额不足")
			}
			balanceBeforeFen := buyer.BalanceFen
			balanceAfterFen := balanceBeforeFen - totalAmountFen
			if balanceAfterFen < 0 {
				return errors.New("余额不足")
			}
			if err := tx.Model(&model.User{}).
				Where("id = ?", userId).
				Update("balance_fen", gorm.Expr("balance_fen - ?", totalAmountFen)).Error; err != nil {
				return err
			}
			if err := model.CreateBalanceRecord(
				tx,
				userId,
				model.BalanceRecordTypeSubscriptionPayOut,
				-totalAmountFen,
				balanceBeforeFen,
				balanceAfterFen,
				effectiveName,
			); err != nil {
				return err
			}
			order := &model.SubscriptionOrder{
				UserId:           userId,
				PlanId:           0,
				PresetId:         preset.Id,
				PresetRevisionId: lockedPresetRevision.Id,
				TradeNo:          tradeNo,
				PayMethod:        model.SubscriptionPayMethodBalance,
				ApplyMode:        req.ApplyMode,
				Quantity:         quantity,
				AmountFen:        totalAmountFen,
				Status:           model.SubscriptionOrderStatusPending,
			}
			if err := order.Insert(tx); err != nil {
				return err
			}
			return completeSubscriptionOrderTx(tx, tradeNo, now, totalAmountFen)
		}); err != nil {
			common.ApiError(c, err)
			return
		}
		_ = model.InvalidateUserCache(userId)
		if inviterId > 0 {
			_ = model.InvalidateUserCache(inviterId)
		}
		common.ApiSuccess(c, gin.H{"trade_no": tradeNo, "status": model.SubscriptionOrderStatusSuccess})
		return
	}

	if strings.TrimSpace(req.EpayMethod) == "" {
		common.ApiErrorMsg(c, "epay_method 不能为空")
		return
	}
	if !operation_setting.ContainsPayMethod(req.EpayMethod) {
		common.ApiErrorMsg(c, "epay_method 无效")
		return
	}

	callBackAddress := strings.TrimRight(strings.TrimSpace(service.GetCallbackAddress()), "/")
	if callBackAddress == "" {
		common.ApiErrorMsg(c, "请先配置服务器地址/回调地址")
		return
	}
	if strings.HasSuffix(callBackAddress, "/v1") {
		common.ApiErrorMsg(c, "回调地址配置错误：不要包含 /v1")
		return
	}
	if !strings.HasPrefix(callBackAddress, "http://") && !strings.HasPrefix(callBackAddress, "https://") {
		common.ApiErrorMsg(c, "回调地址配置错误：必须以 http(s):// 开头")
		return
	}

	serverAddress := strings.TrimRight(strings.TrimSpace(system_setting.ServerAddress), "/")
	if serverAddress == "" {
		common.ApiErrorMsg(c, "请先配置服务器地址")
		return
	}
	if strings.HasSuffix(serverAddress, "/v1") {
		common.ApiErrorMsg(c, "服务器地址配置错误：不要包含 /v1")
		return
	}
	if !strings.HasPrefix(serverAddress, "http://") && !strings.HasPrefix(serverAddress, "https://") {
		common.ApiErrorMsg(c, "服务器地址配置错误：必须以 http(s):// 开头")
		return
	}

	returnUrl, _ := url.Parse(serverAddress + "/api/subscription/epay/return")
	notifyUrl, _ := url.Parse(callBackAddress + "/api/subscription/epay/notify")
	client := GetEpayClient()
	if client == nil {
		common.ApiErrorMsg(c, "当前管理员未配置支付信息")
		return
	}

	payTradeNo := tradeNo
	payAmountFen := totalAmountFen
	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		var buyer model.User
		if err := lockForUpdate(tx).
			Select("id").
			Where("id = ?", userId).
			First(&buyer).Error; err != nil {
			return err
		}

		if effectivePurchaseLimit > 0 {
			pendingOrder, err := model.GetLatestUserSubscriptionOrderByPresetStatus(tx, userId, preset.Id, model.SubscriptionOrderStatusPending)
			if err != nil {
				return err
			}
			if pendingOrder != nil {
				if pendingOrder.PayMethod == model.SubscriptionPayMethodEpay &&
					pendingOrder.ApplyMode == req.ApplyMode &&
					pendingOrder.Quantity == quantity {
					payTradeNo = pendingOrder.TradeNo
					payAmountFen = pendingOrder.AmountFen
					return nil
				}
				return errors.New("存在待支付订单，请先完成支付")
			}
			successUnits, err := model.SumUserSubscriptionOrderQuantityByPresetStatus(tx, userId, preset.Id, model.SubscriptionOrderStatusSuccess)
			if err != nil {
				return err
			}
			if successUnits+int64(quantity) > int64(effectivePurchaseLimit) {
				return fmt.Errorf("已达到该商品限购次数（%d 次）", effectivePurchaseLimit)
			}
		}
		order := &model.SubscriptionOrder{
			UserId:           userId,
			PlanId:           0,
			PresetId:         preset.Id,
			PresetRevisionId: lockedPresetRevision.Id,
			TradeNo:          payTradeNo,
			PayMethod:        model.SubscriptionPayMethodEpay,
			ApplyMode:        req.ApplyMode,
			Quantity:         quantity,
			AmountFen:        payAmountFen,
			Status:           model.SubscriptionOrderStatusPending,
		}
		return order.Insert(tx)
	}); err != nil {
		common.ApiError(c, err)
		return
	}

	uri, params, err := client.Purchase(&epay.PurchaseArgs{
		Type:           req.EpayMethod,
		ServiceTradeNo: payTradeNo,
		Name:           effectiveName,
		Money:          formatFenYuan(payAmountFen),
		Device:         epay.PC,
		NotifyUrl:      notifyUrl,
		ReturnUrl:      returnUrl,
	})
	if err != nil {
		common.ApiErrorMsg(c, "拉起支付失败")
		return
	}

	c.JSON(200, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"trade_no": payTradeNo,
			"url":      uri,
			"params":   params,
		},
	})
}

func SubscriptionEpayNotify(c *gin.Context) {
	params := collectEpayCallbackParams(c)

	client := GetEpayClient()
	if client == nil {
		log.Println("订阅易支付回调失败 未找到配置信息")
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}
	verifyInfo, err := client.Verify(params)
	if err != nil || !verifyInfo.VerifyStatus {
		log.Println("订阅易支付回调签名验证失败")
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}

	if verifyInfo.TradeStatus != epay.StatusTradeSuccess {
		log.Printf("订阅易支付异常回调: %v", verifyInfo)
		_, _ = c.Writer.Write([]byte("success"))
		return
	}

	tradeNo := verifyInfo.ServiceTradeNo
	if tradeNo == "" {
		log.Printf("订阅易支付回调 trade_no 为空: %v", verifyInfo)
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}

	money := params["money"]
	paidFen, err := parseYuanToFen(money)
	if err != nil {
		log.Printf("订阅易支付回调 money 解析失败 trade_no=%s money=%s err=%v", tradeNo, money, err)
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}
	paidAt := common.GetTimestamp()

	LockOrder(tradeNo)
	defer UnlockOrder(tradeNo)

	var userId int
	var inviterId int
	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		if err := completeSubscriptionOrderTx(tx, tradeNo, paidAt, paidFen); err != nil {
			return err
		}
		var order model.SubscriptionOrder
		if err := tx.Select("user_id", "inviter_id").Where("trade_no = ?", tradeNo).First(&order).Error; err == nil {
			userId = order.UserId
			inviterId = order.InviterId
		}
		return nil
	}); err != nil {
		log.Printf("订阅易支付回调处理失败 trade_no=%s err=%v", tradeNo, err)
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}

	_, _ = c.Writer.Write([]byte("success"))

	if userId > 0 {
		_ = model.InvalidateUserCache(userId)
	}
	if inviterId > 0 {
		_ = model.InvalidateUserCache(inviterId)
	}
}

func SubscriptionEpayReturn(c *gin.Context) {
	params := collectEpayCallbackParams(c)
	redirectTo := "/console/my_subscription"

	client := GetEpayClient()
	if client == nil {
		c.Redirect(http.StatusFound, redirectTo)
		return
	}
	verifyInfo, err := client.Verify(params)
	if err != nil || !verifyInfo.VerifyStatus {
		c.Redirect(http.StatusFound, redirectTo)
		return
	}

	if verifyInfo.TradeStatus != epay.StatusTradeSuccess {
		c.Redirect(http.StatusFound, redirectTo)
		return
	}

	tradeNo := verifyInfo.ServiceTradeNo
	if tradeNo == "" {
		c.Redirect(http.StatusFound, redirectTo)
		return
	}

	money := params["money"]
	paidFen, err := parseYuanToFen(money)
	if err != nil {
		c.Redirect(http.StatusFound, redirectTo)
		return
	}
	paidAt := common.GetTimestamp()

	LockOrder(tradeNo)
	defer UnlockOrder(tradeNo)

	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		return completeSubscriptionOrderTx(tx, tradeNo, paidAt, paidFen)
	}); err != nil {
		log.Printf("订阅易支付回跳处理失败 trade_no=%s err=%v", tradeNo, err)
	}

	c.Redirect(http.StatusFound, redirectTo)
}

func GetSubscriptionOrderStatus(c *gin.Context) {
	tradeNo := strings.TrimSpace(c.Query("trade_no"))
	if tradeNo == "" {
		common.ApiErrorMsg(c, "trade_no 不能为空")
		return
	}
	userId := c.GetInt("id")
	if userId <= 0 {
		common.ApiErrorMsg(c, "用户未登录")
		return
	}

	var order model.SubscriptionOrder
	if err := model.DB.
		Select("trade_no", "status", "paid_at", "finished_at", "membership_start_at", "membership_expire_at").
		Where("trade_no = ? AND user_id = ?", tradeNo, userId).
		First(&order).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			common.ApiErrorMsg(c, "订单不存在")
			return
		}
		common.ApiError(c, err)
		return
	}

	common.ApiSuccess(c, gin.H{
		"trade_no":             order.TradeNo,
		"status":               order.Status,
		"paid_at":              order.PaidAt,
		"finished_at":          order.FinishedAt,
		"membership_start_at":  order.MembershipStartAt,
		"membership_expire_at": order.MembershipExpireAt,
	})
}

func completeSubscriptionOrderTx(tx *gorm.DB, tradeNo string, paidAt int64, paidFen int64) error {
	if tx == nil {
		return errors.New("tx 为空")
	}
	if tradeNo == "" {
		return errors.New("tradeNo 为空")
	}

	var order model.SubscriptionOrder
	if err := lockForUpdate(tx).Where("trade_no = ?", tradeNo).First(&order).Error; err != nil {
		return err
	}

	if order.Status == model.SubscriptionOrderStatusSuccess {
		return nil
	}
	if order.Status != model.SubscriptionOrderStatusPending {
		return errors.New("订单状态错误")
	}
	if paidFen != order.AmountFen {
		return fmt.Errorf("订单金额不一致：paid=%s expected=%s", formatFenYuan(paidFen), formatFenYuan(order.AmountFen))
	}

	now := paidAt
	if order.Quantity <= 0 {
		return errors.New("订单数量无效")
	}

	successCount, err := model.CountUserSuccessfulCommissionablePaidEventsTx(tx, order.UserId)
	if err != nil {
		return err
	}
	isFirst := successCount == 0

	startAt := now
	expireAt := int64(0)
	if order.PresetId > 0 {
		var preset model.RedemptionPreset
		if err := tx.Where("id = ?", order.PresetId).First(&preset).Error; err != nil {
			return err
		}
		effectiveMode := preset.Mode
		effectiveQuota := preset.Quota
		effectiveDailyQuotaLimit := preset.DailyQuotaLimit
		effectiveDailyRequestLimit := preset.DailyRequestLimit
		effectiveQuotaValidDays := preset.QuotaValidDays
		effectiveMultiQuantityEnabled := preset.MultiQuantityEnabled
		effectiveMultiQuantityDeferOnly := preset.MultiQuantityDeferOnly
		effectiveAllowedGroupIDs := []int{}
		effectiveRevisionID := order.PresetRevisionId
		if effectiveRevisionID > 0 {
			revision, err := model.GetRedemptionPresetRevisionTx(tx, order.PresetId, effectiveRevisionID)
			if err != nil {
				return err
			}
			effectiveMode = revision.Mode
			effectiveQuota = revision.Quota
			effectiveDailyQuotaLimit = revision.DailyQuotaLimit
			effectiveDailyRequestLimit = revision.DailyRequestLimit
			effectiveQuotaValidDays = revision.QuotaValidDays
			effectiveMultiQuantityEnabled = revision.MultiQuantityEnabled
			effectiveMultiQuantityDeferOnly = revision.MultiQuantityDeferOnly
			if len(revision.AllowedGroupIds) > 0 {
				var ids []int
				if err := common.Unmarshal([]byte(revision.AllowedGroupIds), &ids); err != nil {
					return err
				}
				effectiveAllowedGroupIDs = model.NormalizeUniqueSortedIDs(ids)
			}
		} else {
			revision, err := model.EnsureCurrentRedemptionPresetRevisionTx(tx, order.PresetId)
			if err != nil {
				return err
			}
			effectiveRevisionID = revision.Id
			if err := tx.Model(&model.SubscriptionOrder{}).Where("id = ?", order.Id).
				Update("preset_revision_id", effectiveRevisionID).Error; err != nil {
				return err
			}
			effectiveMode = revision.Mode
			effectiveQuota = revision.Quota
			effectiveDailyQuotaLimit = revision.DailyQuotaLimit
			effectiveDailyRequestLimit = revision.DailyRequestLimit
			effectiveQuotaValidDays = revision.QuotaValidDays
			effectiveMultiQuantityEnabled = revision.MultiQuantityEnabled
			effectiveMultiQuantityDeferOnly = revision.MultiQuantityDeferOnly
			if len(revision.AllowedGroupIds) > 0 {
				var ids []int
				if err := common.Unmarshal([]byte(revision.AllowedGroupIds), &ids); err != nil {
					return err
				}
				effectiveAllowedGroupIDs = model.NormalizeUniqueSortedIDs(ids)
			}
		}

		if effectiveMode != "subscription" && effectiveMode != "tokens" && effectiveMode != "request" {
			return errors.New("订单商品类型错误")
		}
		if err := model.ConsumeRedemptionPresetStockTx(tx, preset.Id, order.Quantity); err != nil {
			return err
		}
		if order.Quantity > 1 && effectiveMultiQuantityDeferOnly && order.ApplyMode != model.SubscriptionApplyModeDefer {
			return errors.New("订单生效方式错误")
		}
		if order.Quantity > 1 && !effectiveMultiQuantityEnabled {
			return errors.New("订单多数量购买参数错误")
		}

		if order.ApplyMode == model.SubscriptionApplyModeDefer {
			var maxExpire int64
			switch effectiveMode {
			case "subscription":
				maxExpire, err = model.GetUserSubscriptionMaxExpireAt(tx, order.UserId, now)
			case "tokens":
				maxExpire, err = model.GetUserSubscriptionMaxExpireAtWithBillingUnit(tx, order.UserId, now, model.UserSubscriptionBillingUnitTokens)
			case "request":
				maxExpire, err = model.GetUserRequestSubscriptionMaxExpireAt(tx, order.UserId, now)
			default:
				err = errors.New("订单商品类型错误")
			}
			if err != nil {
				return err
			}
			if maxExpire >= startAt {
				startAt = maxExpire + 1
			}
		}

		if startAt > common.MaxSupportedUnixTimestamp {
			return errors.New("订阅开始时间过大")
		}
		if effectiveQuotaValidDays < 0 {
			return errors.New("商品有效期无效")
		}
		if effectiveQuota < 0 {
			return errors.New("商品额度无效")
		}
		if effectiveMode != "request" && effectiveDailyQuotaLimit < 0 {
			return errors.New("商品每日额度无效")
		}
		if effectiveMode == "request" && effectiveDailyRequestLimit < 0 {
			return errors.New("商品每日次数无效")
		}

		allowedGroupIDs := effectiveAllowedGroupIDs
		if len(allowedGroupIDs) == 0 {
			return errors.New("商品可用分组为空")
		}
		if err := model.ValidateGroupIDsExist(tx, allowedGroupIDs); err != nil {
			return err
		}

		perUnitExtendSeconds := int64(0)
		if effectiveQuotaValidDays > 0 {
			days := int64(effectiveQuotaValidDays)
			if days > common.MaxSupportedUnixTimestamp/common.SecondsPerDay {
				return errors.New("订阅有效期过大")
			}
			perUnitExtendSeconds = days * common.SecondsPerDay
		}

		nextDeferStartAt := startAt
		lastExpireAt := int64(0)
		for i := 0; i < order.Quantity; i++ {
			unitStartAt := startAt
			if order.ApplyMode == model.SubscriptionApplyModeDefer {
				unitStartAt = nextDeferStartAt
			}

			unitExpireAt := int64(0)
			if perUnitExtendSeconds > 0 {
				if unitStartAt > common.MaxSupportedUnixTimestamp {
					return errors.New("订阅开始时间过大")
				}
				if perUnitExtendSeconds > common.MaxSupportedUnixTimestamp-unitStartAt {
					return errors.New("订阅有效期过大")
				}
				unitExpireAt = unitStartAt + perUnitExtendSeconds
			}

			switch effectiveMode {
			case "subscription":
				if _, err := model.CreateUserSubscriptionTx(
					tx,
					order.UserId,
					unitStartAt,
					effectiveQuota,
					effectiveQuota,
					effectiveDailyQuotaLimit,
					unitExpireAt,
					allowedGroupIDs,
					fmt.Sprintf("subscription_order:%d", order.Id),
					model.UserSubscriptionSourceRef{OrderId: order.Id, PresetId: order.PresetId, PresetRevisionId: effectiveRevisionID},
				); err != nil {
					return err
				}
			case "tokens":
				if _, err := model.CreateUserSubscriptionTxWithBillingUnit(
					tx,
					order.UserId,
					unitStartAt,
					effectiveQuota,
					effectiveQuota,
					effectiveDailyQuotaLimit,
					unitExpireAt,
					allowedGroupIDs,
					model.UserSubscriptionBillingUnitTokens,
					fmt.Sprintf("subscription_order:%d", order.Id),
					model.UserSubscriptionSourceRef{OrderId: order.Id, PresetId: order.PresetId, PresetRevisionId: effectiveRevisionID},
				); err != nil {
					return err
				}
			case "request":
				if _, err := model.CreateUserRequestSubscriptionTx(
					tx,
					order.UserId,
					unitStartAt,
					float64(effectiveDailyRequestLimit),
					float64(effectiveQuota),
					unitExpireAt,
					allowedGroupIDs,
					fmt.Sprintf("subscription_order:%d", order.Id),
					model.UserRequestSubscriptionSourceRef{OrderId: order.Id, PresetId: order.PresetId, PresetRevisionId: effectiveRevisionID},
				); err != nil {
					return err
				}
			default:
				return errors.New("订单商品类型错误")
			}

			if unitExpireAt > lastExpireAt {
				lastExpireAt = unitExpireAt
			}
			if order.ApplyMode == model.SubscriptionApplyModeDefer && perUnitExtendSeconds > 0 {
				nextDeferStartAt = unitExpireAt + 1
			}
		}
		expireAt = lastExpireAt
	} else {
		var plan model.SubscriptionPlan
		if err := tx.Where("id = ?", order.PlanId).First(&plan).Error; err != nil {
			return err
		}

		if order.ApplyMode == model.SubscriptionApplyModeDefer {
			maxExpire, err := model.GetUserMembershipMaxExpireAt(tx, order.UserId, now)
			if err != nil {
				return err
			}
			if maxExpire > startAt {
				startAt = maxExpire
			}
		}

		extendSeconds := int64(plan.DurationDays) * common.SecondsPerDay
		if extendSeconds <= 0 {
			return errors.New("套餐有效期无效")
		}
		if startAt > common.MaxSupportedUnixTimestamp {
			return errors.New("订阅开始时间过大")
		}
		if extendSeconds > common.MaxSupportedUnixTimestamp-startAt {
			return errors.New("订阅有效期过大")
		}
		expireAt = startAt + extendSeconds

		membership := &model.UserMembership{
			UserId:   order.UserId,
			PlanId:   order.PlanId,
			OrderId:  order.Id,
			StartAt:  startAt,
			ExpireAt: expireAt,
			PlanMeta: plan.Meta,
		}
		if err := membership.Insert(tx); err != nil {
			return err
		}
	}

	commissionEligible := order.PayMethod == model.SubscriptionPayMethodEpay
	inviterId := 0
	commissionPercent := 0
	var commissionFen int64
	if commissionEligible {
		var commissionErr error
		inviterId, commissionPercent, commissionFen, commissionErr = model.ApplyInvitationCommissionTx(tx, order.UserId, paidFen, isFirst)
		if commissionErr != nil {
			return commissionErr
		}
	}

	order.Status = model.SubscriptionOrderStatusSuccess
	order.PaidAt = now
	order.FinishedAt = now
	order.MembershipStartAt = startAt
	order.MembershipExpireAt = expireAt
	order.IsFirstPurchase = isFirst
	order.InviterId = inviterId
	order.CommissionPercent = commissionPercent
	order.CommissionFen = commissionFen

	return tx.Save(&order).Error
}
