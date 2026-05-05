package controller

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"log"
	"net/http"
	"net/url"
	"one-api/common"
	"one-api/model"
	"one-api/service"
	"one-api/setting/operation_setting"
	"one-api/setting/system_setting"
	pathpkg "path"
	"strings"
	"time"

	"github.com/Calcium-Ion/go-epay/epay"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// PayOrderConfig defines the type-specific behavior for a pay order flow.
// Each pay type (payg, pay_request, pay_token) provides its own implementation.
type PayOrderConfig interface {
	// Identity
	TradeNoPrefix() string   // "PAYG" / "PAYR" / "PAYT"
	EpayProductName() string // "PayAsYouGo" / "PayPerRequest" / "PayToken"
	NotifyPath() string      // "/api/payg/epay/notify"
	ReturnPath() string      // "/api/payg/epay/return"
	CheckoutPath() string
	ReturnRedirectPath() string
	LogPrefix() string // "按量付费" / "按次付费" / "按token付费"

	// Balance record type for balance payment
	BalanceRecordType() string   // model.BalanceRecordTypePaygPayOut etc.
	BalanceRecordRemark() string // "payg" / "pay_request" / "pay_token"

	// Pay method constants
	PayMethodBalance() string
	PayMethodEpay() string
	UseGatewayQRCode() bool

	// Order status constants
	StatusPending() string
	StatusSuccess() string

	// Whether product_id is required.
	RequireProductId() bool

	// Whether quantity parameter is supported.
	SupportsQuantity() bool

	// ComputeCredit converts amountFen to credit amount
	ComputeCredit(amountFen int64) (int, error)

	// ValidateAndLoadProduct validates product_id and returns product info.
	// Returns productId=0 if no product (legacy mode).
	ValidateAndLoadProduct(productId int) (info productInfo, err error)

	// InsertOrder creates the order record in the database.
	InsertOrder(tx *gorm.DB, params orderParams) error
	SaveEpayCheckout(tx *gorm.DB, tradeNo string, checkout epayGatewayCheckout) error

	// CompleteOrderTx runs the type-specific order completion logic inside a transaction.
	CompleteOrderTx(tx *gorm.DB, tradeNo string, paidAt int64, paidFen int64, pInfo productInfo) error

	// QueryOrderStatus returns the order status response for the given trade_no and user_id.
	QueryOrderStatus(tradeNo string, userId int) (gin.H, error)

	// ReadOrderUserAndInviter reads user_id and inviter_id from a completed order.
	ReadOrderUserAndInviter(tx *gorm.DB, tradeNo string) (userId int, inviterId int, err error)
}

type epayOrderQueryResponse struct {
	Code       interface{} `json:"code"`
	Msg        string      `json:"msg"`
	TradeNo    string      `json:"trade_no"`
	OutTradeNo string      `json:"out_trade_no"`
	Money      string      `json:"money"`
	Status     int         `json:"status"`
}

func isEpaySuccessCode(code interface{}) bool {
	switch value := code.(type) {
	case float64:
		return int(value) == 1
	case float32:
		return int(value) == 1
	case int:
		return value == 1
	case int32:
		return value == 1
	case int64:
		return value == 1
	case string:
		return strings.TrimSpace(value) == "1"
	default:
		return false
	}
}

func queryEpayOrder(tradeNo string) (*epayOrderQueryResponse, error) {
	tradeNo = strings.TrimSpace(tradeNo)
	if tradeNo == "" {
		return nil, errors.New("trade_no 不能为空")
	}

	client := GetEpayClient()
	if client == nil {
		return nil, errors.New("当前管理员未配置支付信息")
	}
	if client.BaseUrl == nil {
		return nil, errors.New("支付地址无效")
	}

	queryURL := *client.BaseUrl
	queryURL.RawQuery = ""
	queryURL.Fragment = ""
	basePath := strings.TrimSpace(queryURL.Path)
	switch {
	case basePath == "" || basePath == "/":
		queryURL.Path = "/api.php"
	case strings.HasSuffix(strings.ToLower(basePath), ".php"):
		dirPath := pathpkg.Dir(basePath)
		if dirPath == "." {
			dirPath = ""
		}
		queryURL.Path = pathpkg.Join(dirPath, "api.php")
	default:
		queryURL.Path = pathpkg.Join(basePath, "api.php")
	}
	params := queryURL.Query()
	params.Set("act", "order")
	params.Set("pid", strings.TrimSpace(client.Config.PartnerID))
	params.Set("key", strings.TrimSpace(client.Config.Key))
	params.Set("out_trade_no", tradeNo)
	queryURL.RawQuery = params.Encode()

	req, err := http.NewRequest(http.MethodGet, queryURL.String(), nil)
	if err != nil {
		return nil, err
	}

	httpClient := &http.Client{Timeout: 5 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("支付网关查单失败: HTTP %d", resp.StatusCode)
	}

	var result epayOrderQueryResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if !isEpaySuccessCode(result.Code) {
		if strings.TrimSpace(result.Msg) != "" {
			return nil, errors.New(strings.TrimSpace(result.Msg))
		}
		return nil, errors.New("支付网关查单失败")
	}
	if result.OutTradeNo != "" && result.OutTradeNo != tradeNo {
		return nil, fmt.Errorf("支付网关返回的订单号不匹配: expected=%s actual=%s", tradeNo, result.OutTradeNo)
	}
	return &result, nil
}

func tryCompletePendingEpayOrder(cfg PayOrderConfig, tradeNo string) (bool, error) {
	result, err := queryEpayOrder(tradeNo)
	if err != nil {
		return false, err
	}
	if result.Status != 1 {
		return false, nil
	}

	paidFen, err := parseYuanToFen(result.Money)
	if err != nil {
		return false, err
	}
	paidAt := common.GetTimestamp()

	LockOrder(tradeNo)
	defer UnlockOrder(tradeNo)

	var userId int
	var inviterId int
	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		if err := cfg.CompleteOrderTx(tx, tradeNo, paidAt, paidFen, productInfo{}); err != nil {
			return err
		}
		var readErr error
		userId, inviterId, readErr = cfg.ReadOrderUserAndInviter(tx, tradeNo)
		if readErr != nil {
			log.Printf("%s主动查单后读取订单信息失败 trade_no=%s err=%v", cfg.LogPrefix(), tradeNo, readErr)
		}
		return nil
	}); err != nil {
		return false, err
	}

	if userId > 0 {
		_ = model.InvalidateUserCache(userId)
	}
	if inviterId > 0 {
		_ = model.InvalidateUserCache(inviterId)
	}
	return true, nil
}

func buildPayOrderCheckoutURL(serverAddress string, cfg PayOrderConfig, tradeNo string) string {
	checkoutPath := strings.TrimSpace(cfg.CheckoutPath())
	tradeNo = strings.TrimSpace(tradeNo)
	serverAddress = strings.TrimRight(strings.TrimSpace(serverAddress), "/")
	if checkoutPath == "" || tradeNo == "" || serverAddress == "" {
		return ""
	}
	return fmt.Sprintf("%s%s?trade_no=%s", serverAddress, checkoutPath, url.QueryEscape(tradeNo))
}

func isTruthyQueryValue(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func writeEpayCheckoutHTML(c *gin.Context, actionURL string, params map[string]string) {
	if c == nil {
		return
	}

	fields := make([]string, 0, len(params))
	for key, value := range params {
		fields = append(fields, fmt.Sprintf(
			`<input type="hidden" name="%s" value="%s">`,
			html.EscapeString(key),
			html.EscapeString(value),
		))
	}

	page := fmt.Sprintf(`<!DOCTYPE html>
<html lang="zh-CN">
  <head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width,initial-scale=1">
    <title>ClawBox 支付跳转</title>
  </head>
  <body>
    <form id="clawbox-epay-form" action="%s" method="post">
      %s
    </form>
    <p>正在打开支付页...</p>
    <noscript>
      <button type="submit" form="clawbox-epay-form">继续前往支付页</button>
    </noscript>
    <script>
      document.getElementById('clawbox-epay-form').submit();
    </script>
  </body>
</html>`,
		html.EscapeString(actionURL),
		strings.Join(fields, "\n      "),
	)

	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(http.StatusOK, page)
}

// productInfo holds product metadata resolved during order creation.
type productInfo struct {
	ProductId           int
	ProductName         string
	SortOrder           int
	AllowedGroupIDs     []int
	AllowedGroupIDsJSON model.JSONValue
}

// orderParams holds the common parameters for inserting an order.
type orderParams struct {
	UserId              int
	TradeNo             string
	PayMethod           string
	EpayMethod          string
	AmountFen           int64
	CreditAmount        int
	ProductId           int
	ProductName         string
	AllowedGroupIDsJSON model.JSONValue
	EpayGatewayTradeNo  string
	EpayPayURL          string
	EpayQRCode          string
	EpayImageURL        string
}

// CreatePayOrder is the generic handler for creating a pay order.
func CreatePayOrder(c *gin.Context, cfg PayOrderConfig) {
	var req struct {
		Money      string `json:"money" binding:"required"`
		PayMethod  string `json:"pay_method" binding:"required"`
		EpayMethod string `json:"epay_method" binding:"omitempty"`
		ProductId  int    `json:"product_id" binding:"omitempty"`
		Quantity   int    `json:"quantity" binding:"omitempty"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	if req.PayMethod != cfg.PayMethodBalance() && req.PayMethod != cfg.PayMethodEpay() {
		common.ApiErrorMsg(c, "pay_method 无效")
		return
	}
	if req.Quantity < 0 {
		common.ApiErrorMsg(c, "quantity 无效")
		return
	}
	if !cfg.SupportsQuantity() && req.Quantity > 1 {
		common.ApiErrorMsg(c, "当前订单类型不支持 quantity")
		return
	}

	amountFen, err := parseYuanToFen(req.Money)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	creditAmount, err := cfg.ComputeCredit(amountFen)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// Apply quantity multiplier if supported
	if cfg.SupportsQuantity() {
		quantity := 1
		if req.Quantity > 0 {
			quantity = req.Quantity
		}
		if quantity > 100 {
			common.ApiErrorMsg(c, "quantity 过大")
			return
		}
		creditAmount = creditAmount * quantity
	}

	// Validate product
	if cfg.RequireProductId() && req.ProductId <= 0 {
		common.ApiErrorMsg(c, "请先选择商品")
		return
	}
	pInfo, err := cfg.ValidateAndLoadProduct(req.ProductId)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	userId := c.GetInt("id")
	tradeNo := fmt.Sprintf("%s%dNO%s%d", cfg.TradeNoPrefix(), userId, common.GetRandomString(6), time.Now().Unix())
	now := common.GetTimestamp()

	oParams := orderParams{
		UserId:              userId,
		TradeNo:             tradeNo,
		PayMethod:           req.PayMethod,
		EpayMethod:          strings.TrimSpace(req.EpayMethod),
		AmountFen:           amountFen,
		CreditAmount:        creditAmount,
		ProductId:           pInfo.ProductId,
		ProductName:         pInfo.ProductName,
		AllowedGroupIDsJSON: pInfo.AllowedGroupIDsJSON,
	}

	// Balance payment path
	if req.PayMethod == cfg.PayMethodBalance() {
		if err := model.DB.Transaction(func(tx *gorm.DB) error {
			var buyer model.User
			if err := lockForUpdate(tx).
				Select("id", "balance_fen").
				Where("id = ?", userId).
				First(&buyer).Error; err != nil {
				return err
			}
			if buyer.BalanceFen < amountFen {
				return errors.New("余额不足")
			}
			balanceBeforeFen := buyer.BalanceFen
			balanceAfterFen := balanceBeforeFen - amountFen
			if balanceAfterFen < 0 {
				return errors.New("余额不足")
			}
			if err := tx.Model(&model.User{}).
				Where("id = ?", userId).
				Update("balance_fen", gorm.Expr("balance_fen - ?", amountFen)).Error; err != nil {
				return err
			}
			if err := model.CreateBalanceRecord(
				tx, userId, cfg.BalanceRecordType(),
				-amountFen, balanceBeforeFen, balanceAfterFen,
				cfg.BalanceRecordRemark(),
			); err != nil {
				return err
			}

			oParams.PayMethod = cfg.PayMethodBalance()
			oParams.EpayMethod = ""
			if err := cfg.InsertOrder(tx, oParams); err != nil {
				return err
			}
			return cfg.CompleteOrderTx(tx, tradeNo, now, amountFen, pInfo)
		}); err != nil {
			common.ApiError(c, err)
			return
		}
		_ = model.InvalidateUserCache(userId)
		common.ApiSuccess(c, gin.H{"trade_no": tradeNo, "status": cfg.StatusSuccess()})
		return
	}

	// Epay payment path
	if strings.TrimSpace(req.EpayMethod) == "" {
		common.ApiErrorMsg(c, "epay_method 不能为空")
		return
	}
	if !operation_setting.ContainsPayMethod(req.EpayMethod) {
		common.ApiErrorMsg(c, "epay_method 无效")
		return
	}

	callBackAddress, serverAddress, err := validatePayAddresses()
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}

	returnUrl, _ := url.Parse(serverAddress + cfg.ReturnPath())
	notifyUrl, _ := url.Parse(callBackAddress + cfg.NotifyPath())
	client := GetEpayClient()
	if client == nil {
		common.ApiErrorMsg(c, "当前管理员未配置支付信息")
		return
	}

	oParams.PayMethod = cfg.PayMethodEpay()
	if cfg.UseGatewayQRCode() {
		if err := model.DB.Transaction(func(tx *gorm.DB) error {
			return cfg.InsertOrder(tx, oParams)
		}); err != nil {
			common.ApiError(c, err)
			return
		}

		checkout, err := createEpayGatewayCheckout(
			c,
			client,
			cfg,
			tradeNo,
			amountFen,
			req.EpayMethod,
			notifyUrl,
			returnUrl,
		)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		if err := model.DB.Transaction(func(tx *gorm.DB) error {
			return cfg.SaveEpayCheckout(tx, tradeNo, *checkout)
		}); err != nil {
			common.ApiError(c, err)
			return
		}

		response := gin.H{
			"trade_no":         tradeNo,
			"gateway_trade_no": checkout.GatewayTradeNo,
			"pay_page_url":     checkout.PayPageURL,
			"qr_code":          checkout.QRCode,
			"qr_image_url":     checkout.QRImageURL,
		}
		if checkoutURL := buildPayOrderCheckoutURL(serverAddress, cfg, tradeNo); checkoutURL != "" {
			response["checkout_url"] = checkoutURL
		}
		common.ApiSuccess(c, response)
		return
	}

	uri, params, err := client.Purchase(&epay.PurchaseArgs{
		Type:           req.EpayMethod,
		ServiceTradeNo: tradeNo,
		Name:           cfg.EpayProductName(),
		Money:          formatFenYuan(amountFen),
		Device:         epay.PC,
		NotifyUrl:      notifyUrl,
		ReturnUrl:      returnUrl,
	})
	if err != nil {
		common.ApiErrorMsg(c, "拉起支付失败")
		return
	}
	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		return cfg.InsertOrder(tx, oParams)
	}); err != nil {
		common.ApiError(c, err)
		return
	}

	response := gin.H{
		"trade_no": tradeNo,
		"url":      uri,
		"params":   params,
	}
	if checkoutURL := buildPayOrderCheckoutURL(serverAddress, cfg, tradeNo); checkoutURL != "" {
		response["checkout_url"] = checkoutURL
	}
	common.ApiSuccess(c, response)
}

// PayOrderEpayNotify is the generic handler for epay notify callbacks.
func PayOrderEpayNotify(c *gin.Context, cfg PayOrderConfig) {
	params := collectEpayCallbackParams(c)

	client := GetEpayClient()
	if client == nil {
		log.Printf("%s易支付回调失败 未找到配置信息", cfg.LogPrefix())
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}
	verifyInfo, err := client.Verify(params)
	if err != nil || !verifyInfo.VerifyStatus {
		log.Printf("%s易支付回调签名验证失败", cfg.LogPrefix())
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}

	if verifyInfo.TradeStatus != epay.StatusTradeSuccess {
		log.Printf("%s易支付异常回调: %v", cfg.LogPrefix(), verifyInfo)
		_, _ = c.Writer.Write([]byte("success"))
		return
	}

	tradeNo := verifyInfo.ServiceTradeNo
	if tradeNo == "" {
		log.Printf("%s易支付回调 trade_no 为空: %v", cfg.LogPrefix(), verifyInfo)
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}

	money := params["money"]
	paidFen, err := parseYuanToFen(money)
	if err != nil {
		log.Printf("%s易支付回调 money 解析失败 trade_no=%s money=%s err=%v", cfg.LogPrefix(), tradeNo, money, err)
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}
	paidAt := common.GetTimestamp()

	LockOrder(tradeNo)
	defer UnlockOrder(tradeNo)

	var userId int
	var inviterId int
	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		if err := cfg.CompleteOrderTx(tx, tradeNo, paidAt, paidFen, productInfo{}); err != nil {
			return err
		}
		var readErr error
		userId, inviterId, readErr = cfg.ReadOrderUserAndInviter(tx, tradeNo)
		if readErr != nil {
			// Non-fatal: order is already completed, just log
			log.Printf("%s易支付回调读取订单信息失败 trade_no=%s err=%v", cfg.LogPrefix(), tradeNo, readErr)
		}
		return nil
	}); err != nil {
		log.Printf("%s易支付回调处理失败 trade_no=%s err=%v", cfg.LogPrefix(), tradeNo, err)
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

// PayOrderEpayReturn is the generic handler for epay return redirects.
func PayOrderEpayReturn(c *gin.Context, cfg PayOrderConfig) {
	params := collectEpayCallbackParams(c)
	redirectTo := strings.TrimSpace(cfg.ReturnRedirectPath())
	if redirectTo == "" {
		redirectTo = "/console/my_subscription"
	}

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
		return cfg.CompleteOrderTx(tx, tradeNo, paidAt, paidFen, productInfo{})
	}); err != nil {
		log.Printf("%s易支付回跳处理失败 trade_no=%s err=%v", cfg.LogPrefix(), tradeNo, err)
	}

	c.Redirect(http.StatusFound, redirectTo)
}

// GetPayOrderStatus is the generic handler for querying order status.
func GetPayOrderStatus(c *gin.Context, cfg PayOrderConfig) {
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

	result, err := cfg.QueryOrderStatus(tradeNo, userId)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			common.ApiErrorMsg(c, "订单不存在")
			return
		}
		common.ApiError(c, err)
		return
	}

	status := strings.TrimSpace(fmt.Sprint(result["status"]))
	rawSync := strings.TrimSpace(c.Query("sync"))
	shouldSync := status == cfg.StatusPending() && (rawSync == "" || isTruthyQueryValue(rawSync))
	if shouldSync {
		if _, err := tryCompletePendingEpayOrder(cfg, tradeNo); err != nil {
			common.ApiError(c, err)
			return
		}

		result, err = cfg.QueryOrderStatus(tradeNo, userId)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				common.ApiErrorMsg(c, "订单不存在")
				return
			}
			common.ApiError(c, err)
			return
		}
	}

	common.ApiSuccess(c, result)
}

// validatePayAddresses validates callback and server addresses for epay.
func validatePayAddresses() (callBackAddress string, serverAddress string, err error) {
	callBackAddress = strings.TrimRight(strings.TrimSpace(service.GetCallbackAddress()), "/")
	if callBackAddress == "" {
		return "", "", errors.New("请先配置服务器地址/回调地址")
	}
	if strings.HasSuffix(callBackAddress, "/v1") {
		return "", "", errors.New("回调地址配置错误：不要包含 /v1")
	}
	if !strings.HasPrefix(callBackAddress, "http://") && !strings.HasPrefix(callBackAddress, "https://") {
		return "", "", errors.New("回调地址配置错误：必须以 http(s):// 开头")
	}

	serverAddress = strings.TrimRight(strings.TrimSpace(system_setting.ServerAddress), "/")
	if serverAddress == "" {
		return "", "", errors.New("请先配置服务器地址")
	}
	if strings.HasSuffix(serverAddress, "/v1") {
		return "", "", errors.New("服务器地址配置错误：不要包含 /v1")
	}
	if !strings.HasPrefix(serverAddress, "http://") && !strings.HasPrefix(serverAddress, "https://") {
		return "", "", errors.New("服务器地址配置错误：必须以 http(s):// 开头")
	}
	return callBackAddress, serverAddress, nil
}
