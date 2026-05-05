package controller

import (
	"errors"
	"fmt"
	"log"
	"net/url"
	"one-api/common"
	"one-api/logger"
	"one-api/model"
	"one-api/service"
	"one-api/setting"
	"one-api/setting/operation_setting"
	"one-api/setting/payg_setting"
	"one-api/setting/subscription_setting"
	"one-api/setting/system_setting"
	"strconv"
	"sync"
	"time"

	"github.com/Calcium-Ion/go-epay/epay"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

func sumUserPaygCurrentQuotaByProductIDs(userID int, productIDs []int) (int, error) {
	if userID <= 0 {
		return 0, errors.New("userId 无效")
	}
	if len(productIDs) == 0 {
		return 0, nil
	}
	productSet := make(map[int]struct{}, len(productIDs))
	for _, productID := range productIDs {
		if productID <= 0 {
			continue
		}
		productSet[productID] = struct{}{}
	}
	if len(productSet) == 0 {
		return 0, nil
	}
	balances, err := model.GetUserPaygBalances(userID, true)
	if err != nil {
		return 0, err
	}
	total := 0
	for _, balance := range balances {
		if _, ok := productSet[balance.ProductId]; !ok {
			continue
		}
		if balance.RemainingQuota <= 0 {
			continue
		}
		total += balance.RemainingQuota
	}
	return total, nil
}

func GetTopUpInfo(c *gin.Context) {
	userID := c.GetInt("id")

	// 获取支付方式
	payMethods := operation_setting.PayMethods

	// 如果启用了 Stripe 支付，添加到支付方法列表
	if setting.StripeApiSecret != "" && setting.StripeWebhookSecret != "" && setting.StripePriceId != "" {
		// 检查是否已经包含 Stripe
		hasStripe := false
		for _, method := range payMethods {
			if method["type"] == "stripe" {
				hasStripe = true
				break
			}
		}

		if !hasStripe {
			stripeMethod := map[string]string{
				"name":      "Stripe",
				"type":      "stripe",
				"color":     "rgba(var(--semi-purple-5), 1)",
				"min_topup": strconv.Itoa(setting.StripeMinTopUp),
			}
			payMethods = append(payMethods, stripeMethod)
		}
	}

	paygEnabled := false
	paygAllowedGroupIDs := make([]int, 0)
	paygProducts := make([]gin.H, 0)
	if model.DB != nil && model.DB.Migrator().HasTable(&model.PaygProduct{}) && model.DB.Migrator().HasTable(&model.PaygProductGroup{}) {
		var products []model.PaygProduct
		if err := model.DB.Order("sort_order DESC, id DESC").Find(&products).Error; err != nil {
			common.ApiError(c, err)
			return
		}
		if len(products) > 0 {
			ids := make([]int, 0, len(products))
			for _, p := range products {
				if p.Id <= 0 {
					continue
				}
				ids = append(ids, p.Id)
			}
			type row struct {
				ProductId int `gorm:"column:product_id"`
				GroupId   int `gorm:"column:group_id"`
			}
			var rows []row
			if len(ids) > 0 {
				if err := model.DB.Model(&model.PaygProductGroup{}).
					Select("product_id", "group_id").
					Where("product_id IN ?", ids).
					Find(&rows).Error; err != nil {
					common.ApiError(c, err)
					return
				}
			}
			groupIDsByProduct := make(map[int][]int, len(products))
			union := make([]int, 0)
			unionSeen := make(map[int]struct{}, 16)
			for _, r := range rows {
				if r.ProductId <= 0 || r.GroupId <= 0 {
					continue
				}
				groupIDsByProduct[r.ProductId] = append(groupIDsByProduct[r.ProductId], r.GroupId)
				if _, ok := unionSeen[r.GroupId]; ok {
					continue
				}
				unionSeen[r.GroupId] = struct{}{}
				union = append(union, r.GroupId)
			}
			paygAllowedGroupIDs = model.NormalizeUniqueSortedIDs(union)
			for _, p := range products {
				groupIDs := model.NormalizeUniqueSortedIDs(groupIDsByProduct[p.Id])
				if p.Enabled {
					paygEnabled = true
				}
				paygProducts = append(paygProducts, gin.H{
					"id":                p.Id,
					"name":              p.Name,
					"description":       p.Description,
					"enabled":           p.Enabled,
					"sort_order":        p.SortOrder,
					"stock":             p.Stock,
					"allowed_group_ids": groupIDs,
				})
			}
		}
	} else {
		// Fallback to legacy in-memory config (pre-migration window).
		paygProductsRaw := payg_setting.GetPaygSettings().Products
		paygProducts = make([]gin.H, 0, len(paygProductsRaw))
		union := make([]int, 0, 16)
		unionSeen := make(map[int]struct{}, 16)
		for _, p := range paygProductsRaw {
			if p.Enabled {
				paygEnabled = true
			}
			groupIDs := model.NormalizeUniqueSortedIDs(p.AllowedGroupIds)
			if len(groupIDs) == 0 && len(p.AllowedGroups) > 0 {
				ids, err := model.LegacyGroupIDsFromCodes(nil, p.AllowedGroups)
				if err != nil {
					common.ApiError(c, err)
					return
				}
				groupIDs = ids
			}
			if len(groupIDs) == 0 {
				common.ApiErrorMsg(c, "按量付费商品可用分组不能为空")
				return
			}
			for _, gid := range groupIDs {
				if gid <= 0 {
					continue
				}
				if _, ok := unionSeen[gid]; ok {
					continue
				}
				unionSeen[gid] = struct{}{}
				union = append(union, gid)
			}
			paygProducts = append(paygProducts, gin.H{
				"id":                p.Id,
				"name":              p.Name,
				"description":       p.Description,
				"enabled":           p.Enabled,
				"sort_order":        p.SortOrder,
				"stock":             p.Stock,
				"allowed_group_ids": groupIDs,
			})
		}
		paygAllowedGroupIDs = model.NormalizeUniqueSortedIDs(union)
	}
	if model.ClawBoxProductModeEnabled() {
		productID, err := model.ResolveClawBoxProductIDTx(nil)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		filteredProducts := make([]gin.H, 0, 1)
		filteredGroups := make([]int, 0)
		filteredEnabled := false
		for _, product := range paygProducts {
			id, _ := product["id"].(int)
			if id != productID {
				continue
			}
			filteredProducts = append(filteredProducts, product)
			if groupIDs, ok := product["allowed_group_ids"].([]int); ok {
				filteredGroups = model.NormalizeUniqueSortedIDs(groupIDs)
			}
			if enabled, ok := product["enabled"].(bool); ok {
				filteredEnabled = enabled
			}
			break
		}
		if len(filteredProducts) == 0 {
			common.ApiErrorMsg(c, "ClawBox 商品不存在或未同步到商品列表")
			return
		}
		paygProducts = filteredProducts
		paygAllowedGroupIDs = filteredGroups
		paygEnabled = filteredEnabled
	}
	paygCurrentProductIDs := make([]int, 0, len(paygProducts))
	for _, product := range paygProducts {
		id, ok := product["id"].(int)
		if !ok || id <= 0 {
			continue
		}
		paygCurrentProductIDs = append(paygCurrentProductIDs, id)
	}
	paygCurrentQuota, err := sumUserPaygCurrentQuotaByProductIDs(userID, paygCurrentProductIDs)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// Pay Request Products
	payRequestEnabled := false
	payRequestAllowedGroupIDs := make([]int, 0)
	payRequestProducts := make([]gin.H, 0)
	if model.DB != nil && model.DB.Migrator().HasTable(&model.PayRequestProduct{}) && model.DB.Migrator().HasTable(&model.PayRequestProductGroup{}) {
		var products []model.PayRequestProduct
		if err := model.DB.Order("sort_order DESC, id DESC").Find(&products).Error; err != nil {
			common.ApiError(c, err)
			return
		}
		if len(products) > 0 {
			ids := make([]int, 0, len(products))
			for _, p := range products {
				if p.Id <= 0 {
					continue
				}
				ids = append(ids, p.Id)
			}
			type row struct {
				ProductId int `gorm:"column:product_id"`
				GroupId   int `gorm:"column:group_id"`
			}
			var rows []row
			if len(ids) > 0 {
				if err := model.DB.Model(&model.PayRequestProductGroup{}).
					Select("product_id", "group_id").
					Where("product_id IN ?", ids).
					Find(&rows).Error; err != nil {
					common.ApiError(c, err)
					return
				}
			}
			groupIDsByProduct := make(map[int][]int, len(products))
			union := make([]int, 0)
			unionSeen := make(map[int]struct{}, 16)
			for _, r := range rows {
				if r.ProductId <= 0 || r.GroupId <= 0 {
					continue
				}
				groupIDsByProduct[r.ProductId] = append(groupIDsByProduct[r.ProductId], r.GroupId)
				if _, ok := unionSeen[r.GroupId]; ok {
					continue
				}
				unionSeen[r.GroupId] = struct{}{}
				union = append(union, r.GroupId)
			}
			payRequestAllowedGroupIDs = model.NormalizeUniqueSortedIDs(union)
			for _, p := range products {
				groupIDs := model.NormalizeUniqueSortedIDs(groupIDsByProduct[p.Id])
				if p.Enabled {
					payRequestEnabled = true
				}
				payRequestProducts = append(payRequestProducts, gin.H{
					"id":                p.Id,
					"name":              p.Name,
					"description":       p.Description,
					"enabled":           p.Enabled,
					"sort_order":        p.SortOrder,
					"stock":             p.Stock,
					"allowed_group_ids": groupIDs,
				})
			}
		}
	} else {
		// Fallback to legacy in-memory config (pre-migration window).
		payRequestProductsRaw := payg_setting.GetPaygSettings().PayRequestProducts
		payRequestProducts = make([]gin.H, 0, len(payRequestProductsRaw))
		union := make([]int, 0, 16)
		unionSeen := make(map[int]struct{}, 16)
		for _, p := range payRequestProductsRaw {
			if p.Enabled {
				payRequestEnabled = true
			}
			groupIDs := model.NormalizeUniqueSortedIDs(p.AllowedGroupIds)
			if len(groupIDs) == 0 && len(p.AllowedGroups) > 0 {
				ids, err := model.LegacyGroupIDsFromCodes(nil, p.AllowedGroups)
				if err != nil {
					common.ApiError(c, err)
					return
				}
				groupIDs = ids
			}
			if len(groupIDs) == 0 {
				common.ApiErrorMsg(c, "按次付费商品可用分组不能为空")
				return
			}
			for _, gid := range groupIDs {
				if gid <= 0 {
					continue
				}
				if _, ok := unionSeen[gid]; ok {
					continue
				}
				unionSeen[gid] = struct{}{}
				union = append(union, gid)
			}
			payRequestProducts = append(payRequestProducts, gin.H{
				"id":                p.Id,
				"name":              p.Name,
				"description":       p.Description,
				"enabled":           p.Enabled,
				"sort_order":        p.SortOrder,
				"stock":             p.Stock,
				"allowed_group_ids": groupIDs,
			})
		}
		payRequestAllowedGroupIDs = model.NormalizeUniqueSortedIDs(union)
	}

	// Pay Token Products
	payTokenEnabled := false
	payTokenAllowedGroupIDs := make([]int, 0)
	payTokenProducts := make([]gin.H, 0)
	if model.DB != nil && model.DB.Migrator().HasTable(&model.PayTokenProduct{}) && model.DB.Migrator().HasTable(&model.PayTokenProductGroup{}) {
		var products []model.PayTokenProduct
		if err := model.DB.Order("sort_order DESC, id DESC").Find(&products).Error; err != nil {
			common.ApiError(c, err)
			return
		}
		if len(products) > 0 {
			ids := make([]int, 0, len(products))
			for _, p := range products {
				if p.Id <= 0 {
					continue
				}
				ids = append(ids, p.Id)
			}
			type row struct {
				ProductId int `gorm:"column:product_id"`
				GroupId   int `gorm:"column:group_id"`
			}
			var rows []row
			if len(ids) > 0 {
				if err := model.DB.Model(&model.PayTokenProductGroup{}).
					Select("product_id", "group_id").
					Where("product_id IN ?", ids).
					Find(&rows).Error; err != nil {
					common.ApiError(c, err)
					return
				}
			}
			groupIDsByProduct := make(map[int][]int, len(products))
			union := make([]int, 0)
			unionSeen := make(map[int]struct{}, 16)
			for _, r := range rows {
				if r.ProductId <= 0 || r.GroupId <= 0 {
					continue
				}
				groupIDsByProduct[r.ProductId] = append(groupIDsByProduct[r.ProductId], r.GroupId)
				if _, ok := unionSeen[r.GroupId]; ok {
					continue
				}
				unionSeen[r.GroupId] = struct{}{}
				union = append(union, r.GroupId)
			}
			payTokenAllowedGroupIDs = model.NormalizeUniqueSortedIDs(union)
			for _, p := range products {
				groupIDs := model.NormalizeUniqueSortedIDs(groupIDsByProduct[p.Id])
				if p.Enabled {
					payTokenEnabled = true
				}
				payTokenProducts = append(payTokenProducts, gin.H{
					"id":                p.Id,
					"name":              p.Name,
					"description":       p.Description,
					"enabled":           p.Enabled,
					"sort_order":        p.SortOrder,
					"stock":             p.Stock,
					"allowed_group_ids": groupIDs,
				})
			}
		}
	} else {
		// Fallback to legacy in-memory config (pre-migration window).
		payTokenProductsRaw := payg_setting.GetPaygSettings().PayTokenProducts
		payTokenProducts = make([]gin.H, 0, len(payTokenProductsRaw))
		union := make([]int, 0, 16)
		unionSeen := make(map[int]struct{}, 16)
		for _, p := range payTokenProductsRaw {
			if p.Enabled {
				payTokenEnabled = true
			}
			groupIDs := model.NormalizeUniqueSortedIDs(p.AllowedGroupIds)
			if len(groupIDs) == 0 && len(p.AllowedGroups) > 0 {
				ids, err := model.LegacyGroupIDsFromCodes(nil, p.AllowedGroups)
				if err != nil {
					common.ApiError(c, err)
					return
				}
				groupIDs = ids
			}
			if len(groupIDs) == 0 {
				common.ApiErrorMsg(c, "按token付费商品可用分组不能为空")
				return
			}
			for _, gid := range groupIDs {
				if gid <= 0 {
					continue
				}
				if _, ok := unionSeen[gid]; ok {
					continue
				}
				unionSeen[gid] = struct{}{}
				union = append(union, gid)
			}
			payTokenProducts = append(payTokenProducts, gin.H{
				"id":                p.Id,
				"name":              p.Name,
				"description":       p.Description,
				"enabled":           p.Enabled,
				"sort_order":        p.SortOrder,
				"stock":             p.Stock,
				"allowed_group_ids": groupIDs,
			})
		}
		payTokenAllowedGroupIDs = model.NormalizeUniqueSortedIDs(union)
	}

	data := gin.H{
		"enable_online_topup":                 operation_setting.PayAddress != "" && operation_setting.EpayId != "" && operation_setting.EpayKey != "",
		"enable_stripe_topup":                 setting.StripeApiSecret != "" && setting.StripeWebhookSecret != "" && setting.StripePriceId != "",
		"pay_methods":                         payMethods,
		"subscription_checkout_mode":          subscription_setting.GetSubscriptionCheckoutSettings().CheckoutMode,
		"subscription_traffic_message":        subscription_setting.GetSubscriptionCheckoutSettings().TrafficMessage,
		"subscription_traffic_qrcode":         subscription_setting.GetSubscriptionCheckoutSettings().TrafficQRCode,
		"subscription_store_notice":           subscription_setting.GetSubscriptionCheckoutSettings().StoreNotice,
		"payg_enabled":                        paygEnabled,
		"payg_description":                    "",
		"payg_products":                       paygProducts,
		"payg_allowed_group_ids":              paygAllowedGroupIDs,
		"payg_current_quota":                  paygCurrentQuota,
		"payg_credit_usd_per_cny":             payg_setting.GetPaygSettings().CreditUsdPerCny,
		"pay_request_enabled":                 payRequestEnabled,
		"pay_request_products":                payRequestProducts,
		"pay_request_allowed_group_ids":       payRequestAllowedGroupIDs,
		"pay_request_credit_requests_per_cny": payg_setting.GetPaygSettings().CreditRequestsPerCny,
		"pay_token_enabled":                   payTokenEnabled,
		"pay_token_products":                  payTokenProducts,
		"pay_token_allowed_group_ids":         payTokenAllowedGroupIDs,
		"pay_token_credit_tokens_per_cny":     payg_setting.GetPaygSettings().CreditTokensPerCny,
		"min_topup":                           operation_setting.MinTopUp,
		"stripe_min_topup":                    setting.StripeMinTopUp,
		"amount_options":                      operation_setting.GetPaymentSetting().AmountOptions,
		"discount":                            operation_setting.GetPaymentSetting().AmountDiscount,
	}
	common.ApiSuccess(c, data)
}

type EpayRequest struct {
	Amount        int64  `json:"amount"`
	PaymentMethod string `json:"payment_method"`
	TopUpCode     string `json:"top_up_code"`
}

type AmountRequest struct {
	Amount    int64  `json:"amount"`
	TopUpCode string `json:"top_up_code"`
}

func GetEpayClient() *epay.Client {
	if operation_setting.PayAddress == "" || operation_setting.EpayId == "" || operation_setting.EpayKey == "" {
		return nil
	}
	withUrl, err := epay.NewClient(&epay.Config{
		PartnerID: operation_setting.EpayId,
		Key:       operation_setting.EpayKey,
	}, operation_setting.PayAddress)
	if err != nil {
		return nil
	}
	return withUrl
}

func getPayMoney(amount int64, groupID int) float64 {
	dAmount := decimal.NewFromInt(amount)

	if !common.DisplayInCurrencyEnabled {
		dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
		dAmount = dAmount.Div(dQuotaPerUnit)
	}

	topupGroupRatio := common.GetTopupGroupRatio(groupID)
	if topupGroupRatio == 0 {
		topupGroupRatio = 1
	}

	dTopupGroupRatio := decimal.NewFromFloat(topupGroupRatio)
	dPrice := decimal.NewFromFloat(operation_setting.Price)
	// apply optional preset discount by the original request amount (if configured), default 1.0
	discount := 1.0
	if ds, ok := operation_setting.GetPaymentSetting().AmountDiscount[int(amount)]; ok {
		if ds > 0 {
			discount = ds
		}
	}
	dDiscount := decimal.NewFromFloat(discount)

	payMoney := dAmount.Mul(dPrice).Mul(dTopupGroupRatio).Mul(dDiscount)

	return payMoney.InexactFloat64()
}

func getMinTopup() int64 {
	minTopup := operation_setting.MinTopUp
	if !common.DisplayInCurrencyEnabled {
		dMinTopup := decimal.NewFromInt(int64(minTopup))
		dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
		minTopup = int(dMinTopup.Mul(dQuotaPerUnit).IntPart())
	}
	return int64(minTopup)
}

func calculateTopUpCreditQuota(amount int64) (int64, error) {
	if amount <= 0 {
		return 0, errors.New("充值数量无效")
	}
	if !common.DisplayInCurrencyEnabled {
		return amount, nil
	}

	dAmount := decimal.NewFromInt(amount)
	dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
	creditQuota := dAmount.Mul(dQuotaPerUnit).IntPart()
	if creditQuota <= 0 {
		return 0, errors.New("充值额度无效")
	}
	return creditQuota, nil
}

func normalizeTopUpStoredPayment(payMoney float64) (float64, int64, error) {
	fen, err := common.YuanFloatToFen(payMoney)
	if err != nil {
		return 0, 0, err
	}
	return float64(fen) / 100, fen, nil
}

func RequestEpay(c *gin.Context) {
	var req EpayRequest
	err := c.ShouldBindJSON(&req)
	if err != nil {
		c.JSON(200, gin.H{"message": "error", "data": "参数错误"})
		return
	}
	if req.Amount < getMinTopup() {
		c.JSON(200, gin.H{"message": "error", "data": fmt.Sprintf("充值数量不能小于 %d", getMinTopup())})
		return
	}

	id := c.GetInt("id")
	audienceGroupID, err := model.GetUserAudienceGroupID(id, true)
	if err != nil {
		c.JSON(200, gin.H{"message": "error", "data": "获取用户分组失败"})
		return
	}
	creditQuota, err := calculateTopUpCreditQuota(req.Amount)
	if err != nil {
		c.JSON(200, gin.H{"message": "error", "data": err.Error()})
		return
	}
	payMoney, payFen, err := normalizeTopUpStoredPayment(getPayMoney(req.Amount, audienceGroupID))
	if err != nil {
		c.JSON(200, gin.H{"message": "error", "data": "充值金额格式错误"})
		return
	}
	if payMoney < 0.01 {
		c.JSON(200, gin.H{"message": "error", "data": "充值金额过低"})
		return
	}

	if !operation_setting.ContainsPayMethod(req.PaymentMethod) {
		c.JSON(200, gin.H{"message": "error", "data": "支付方式不存在"})
		return
	}

	callBackAddress := service.GetCallbackAddress()
	returnUrl, _ := url.Parse(system_setting.ServerAddress + "/console/log")
	notifyUrl, _ := url.Parse(callBackAddress + "/api/user/epay/notify")
	tradeNo := fmt.Sprintf("%s%d", common.GetRandomString(6), time.Now().Unix())
	tradeNo = fmt.Sprintf("USR%dNO%s", id, tradeNo)
	client := GetEpayClient()
	if client == nil {
		c.JSON(200, gin.H{"message": "error", "data": "当前管理员未配置支付信息"})
		return
	}
	uri, params, err := client.Purchase(&epay.PurchaseArgs{
		Type:           req.PaymentMethod,
		ServiceTradeNo: tradeNo,
		Name:           fmt.Sprintf("TopUp %d", req.Amount),
		Money:          strconv.FormatFloat(payMoney, 'f', 2, 64),
		Device:         epay.PC,
		NotifyUrl:      notifyUrl,
		ReturnUrl:      returnUrl,
	})
	if err != nil {
		c.JSON(200, gin.H{"message": "error", "data": "拉起支付失败"})
		return
	}
	topUp := &model.TopUp{
		UserId:           id,
		Amount:           req.Amount,
		CreditQuota:      creditQuota,
		Money:            payMoney,
		PaymentAmountFen: payFen,
		PaymentCurrency:  "cny",
		TradeNo:          tradeNo,
		CreateTime:       time.Now().Unix(),
		Status:           common.TopUpStatusPending,
	}
	err = topUp.Insert()
	if err != nil {
		c.JSON(200, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}
	c.JSON(200, gin.H{"message": "success", "data": params, "url": uri})
}

// tradeNo lock
var orderLocks sync.Map
var createLock sync.Mutex

// LockOrder 尝试对给定订单号加锁
func LockOrder(tradeNo string) {
	lock, ok := orderLocks.Load(tradeNo)
	if !ok {
		createLock.Lock()
		defer createLock.Unlock()
		lock, ok = orderLocks.Load(tradeNo)
		if !ok {
			lock = new(sync.Mutex)
			orderLocks.Store(tradeNo, lock)
		}
	}
	lock.(*sync.Mutex).Lock()
}

// UnlockOrder 释放给定订单号的锁
func UnlockOrder(tradeNo string) {
	lock, ok := orderLocks.Load(tradeNo)
	if ok {
		lock.(*sync.Mutex).Unlock()
	}
}

func EpayNotify(c *gin.Context) {
	params := collectEpayCallbackParams(c)
	client := GetEpayClient()
	if client == nil {
		log.Println("易支付回调失败 未找到配置信息")
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}
	verifyInfo, err := client.Verify(params)
	if err != nil || !verifyInfo.VerifyStatus {
		log.Println("易支付回调签名验证失败")
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}

	if verifyInfo.TradeStatus != epay.StatusTradeSuccess {
		log.Printf("易支付异常回调: %v", verifyInfo)
		_, _ = c.Writer.Write([]byte("success"))
		return
	}

	tradeNo := verifyInfo.ServiceTradeNo
	if tradeNo == "" {
		log.Printf("易支付回调 trade_no 为空: %v", verifyInfo)
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}

	paidFen, err := parseYuanToFen(params["money"])
	if err != nil {
		log.Printf("易支付回调 money 解析失败 trade_no=%s money=%s err=%v", tradeNo, params["money"], err)
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}

	LockOrder(tradeNo)
	defer UnlockOrder(tradeNo)

	processed := false
	quotaToAdd := 0
	topUpMoney := 0.0
	userId := 0

	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		var topUp model.TopUp
		if err := lockForUpdate(tx).
			Where("trade_no = ?", tradeNo).
			First(&topUp).Error; err != nil {
			return err
		}
		userId = topUp.UserId
		topUpMoney = topUp.Money

		if topUp.Status == common.TopUpStatusSuccess {
			return nil
		}
		if topUp.Status != common.TopUpStatusPending {
			return errors.New("充值订单状态错误")
		}

		expectedFen := topUp.PaymentAmountFen
		if expectedFen <= 0 {
			expectedFen, err = common.YuanFloatToFen(topUp.Money)
			if err != nil {
				return err
			}
		}
		if expectedFen != paidFen {
			return fmt.Errorf("订单金额不一致：paid=%s expected=%s", formatFenYuan(paidFen), formatFenYuan(expectedFen))
		}

		topUp.Money = float64(paidFen) / 100
		topUp.PaymentAmountFen = paidFen
		topUp.PaymentCurrency = "cny"
		topUp.Status = common.TopUpStatusSuccess
		topUp.CompleteTime = common.GetTimestamp()
		if err := tx.Save(&topUp).Error; err != nil {
			return err
		}

		if topUp.CreditQuota > 0 {
			quotaToAdd = int(topUp.CreditQuota)
		} else {
			dAmount := decimal.NewFromInt(int64(topUp.Amount))
			dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
			quotaToAdd = int(dAmount.Mul(dQuotaPerUnit).IntPart())
		}
		if quotaToAdd <= 0 {
			return errors.New("充值额度无效")
		}
		topUpMoney = topUp.Money

		if err := tx.Model(&model.User{}).
			Where("id = ?", topUp.UserId).
			Update("quota", gorm.Expr("quota + ?", quotaToAdd)).Error; err != nil {
			return err
		}

		processed = true
		return nil
	}); err != nil {
		log.Printf("易支付回调处理失败 trade_no=%s err=%v", tradeNo, err)
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}

	_, _ = c.Writer.Write([]byte("success"))

	if processed && userId > 0 {
		_ = model.InvalidateUserCache(userId)
		model.RecordLog(userId, model.LogTypeTopup, fmt.Sprintf("使用在线充值成功，充值金额: %v，支付金额：%f", logger.LogQuota(quotaToAdd), topUpMoney))
	}
}

func RequestAmount(c *gin.Context) {
	var req AmountRequest
	err := c.ShouldBindJSON(&req)
	if err != nil {
		c.JSON(200, gin.H{"message": "error", "data": "参数错误"})
		return
	}

	if req.Amount < getMinTopup() {
		c.JSON(200, gin.H{"message": "error", "data": fmt.Sprintf("充值数量不能小于 %d", getMinTopup())})
		return
	}
	id := c.GetInt("id")
	audienceGroupID, err := model.GetUserAudienceGroupID(id, true)
	if err != nil {
		c.JSON(200, gin.H{"message": "error", "data": "获取用户分组失败"})
		return
	}
	payMoney := getPayMoney(req.Amount, audienceGroupID)
	if payMoney <= 0.01 {
		c.JSON(200, gin.H{"message": "error", "data": "充值金额过低"})
		return
	}
	c.JSON(200, gin.H{"message": "success", "data": strconv.FormatFloat(payMoney, 'f', 2, 64)})
}
