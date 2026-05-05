package controller

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"one-api/common"
	"one-api/model"
	"one-api/setting"
	"one-api/setting/operation_setting"
	"one-api/setting/system_setting"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stripe/stripe-go/v81"
	"github.com/stripe/stripe-go/v81/checkout/session"
	stripeprice "github.com/stripe/stripe-go/v81/price"
	"github.com/stripe/stripe-go/v81/webhook"
	"github.com/thanhpk/randstr"
)

const (
	PaymentMethodStripe = "stripe"
)

var stripeAdaptor = &StripeAdaptor{}

type StripePayRequest struct {
	Amount        int64  `json:"amount"`
	PaymentMethod string `json:"payment_method"`
}

type StripeAdaptor struct {
}

func (*StripeAdaptor) RequestAmount(c *gin.Context, req *StripePayRequest) {
	if req.Amount < getStripeMinTopup() {
		c.JSON(200, gin.H{"message": "error", "data": fmt.Sprintf("充值数量不能小于 %d", getStripeMinTopup())})
		return
	}
	id := c.GetInt("id")
	audienceGroupID, err := model.GetUserAudienceGroupID(id, true)
	if err != nil {
		c.JSON(200, gin.H{"message": "error", "data": "获取用户分组失败"})
		return
	}
	payMoney := getStripePayMoney(float64(req.Amount), audienceGroupID)
	if payMoney <= 0.01 {
		c.JSON(200, gin.H{"message": "error", "data": "充值金额过低"})
		return
	}
	c.JSON(200, gin.H{"message": "success", "data": strconv.FormatFloat(payMoney, 'f', 2, 64)})
}

func (*StripeAdaptor) RequestPay(c *gin.Context, req *StripePayRequest) {
	if req.PaymentMethod != PaymentMethodStripe {
		c.JSON(200, gin.H{"message": "error", "data": "不支持的支付渠道"})
		return
	}
	if req.Amount < getStripeMinTopup() {
		c.JSON(200, gin.H{"message": fmt.Sprintf("充值数量不能小于 %d", getStripeMinTopup()), "data": 10})
		return
	}
	if req.Amount > 10000 {
		c.JSON(200, gin.H{"message": "充值数量不能大于 10000", "data": 10})
		return
	}

	id := c.GetInt("id")
	user, _ := model.GetUserById(id, false)
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
	payMoney, payFen, err := normalizeTopUpStoredPayment(getStripePayMoney(float64(req.Amount), audienceGroupID))
	if err != nil {
		c.JSON(200, gin.H{"message": "error", "data": "充值金额格式错误"})
		return
	}
	if payMoney < 0.01 {
		c.JSON(200, gin.H{"message": "error", "data": "充值金额过低"})
		return
	}

	reference := fmt.Sprintf("transfer-api-ref-%d-%d-%s", user.Id, time.Now().UnixMilli(), randstr.String(4))
	referenceId := "ref_" + common.Sha1([]byte(reference))

	payLink, err := genStripeLink(referenceId, user.StripeCustomer, user.Email, req.Amount, payFen)
	if err != nil {
		log.Println("获取Stripe Checkout支付链接失败", err)
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
		TradeNo:          referenceId,
		CreateTime:       time.Now().Unix(),
		Status:           common.TopUpStatusPending,
	}
	err = topUp.Insert()
	if err != nil {
		c.JSON(200, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}
	c.JSON(200, gin.H{
		"message": "success",
		"data": gin.H{
			"pay_link": payLink,
		},
	})
}

func RequestStripeAmount(c *gin.Context) {
	var req StripePayRequest
	err := c.ShouldBindJSON(&req)
	if err != nil {
		c.JSON(200, gin.H{"message": "error", "data": "参数错误"})
		return
	}
	stripeAdaptor.RequestAmount(c, &req)
}

func RequestStripePay(c *gin.Context) {
	var req StripePayRequest
	err := c.ShouldBindJSON(&req)
	if err != nil {
		c.JSON(200, gin.H{"message": "error", "data": "参数错误"})
		return
	}
	stripeAdaptor.RequestPay(c, &req)
}

func StripeWebhook(c *gin.Context) {
	payload, err := io.ReadAll(c.Request.Body)
	if err != nil {
		log.Printf("解析Stripe Webhook参数失败: %v\n", err)
		c.AbortWithStatus(http.StatusServiceUnavailable)
		return
	}

	signature := c.GetHeader("Stripe-Signature")
	endpointSecret := setting.StripeWebhookSecret
	event, err := webhook.ConstructEventWithOptions(payload, signature, endpointSecret, webhook.ConstructEventOptions{
		IgnoreAPIVersionMismatch: true,
	})

	if err != nil {
		log.Printf("Stripe Webhook验签失败: %v\n", err)
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	switch event.Type {
	case stripe.EventTypeCheckoutSessionCompleted:
		sessionCompleted(event)
	case stripe.EventTypeCheckoutSessionExpired:
		sessionExpired(event)
	default:
		log.Printf("不支持的Stripe Webhook事件类型: %s\n", event.Type)
	}

	c.Status(http.StatusOK)
}

func sessionCompleted(event stripe.Event) {
	customerId := event.GetObjectValue("customer")
	referenceId := event.GetObjectValue("client_reference_id")
	status := event.GetObjectValue("status")
	amountTotal := strings.TrimSpace(event.GetObjectValue("amount_total"))
	if "complete" != status {
		log.Println("错误的Stripe Checkout完成状态:", status, ",", referenceId)
		return
	}

	paidFen, err := strconv.ParseInt(amountTotal, 10, 64)
	if err != nil || paidFen <= 0 {
		log.Println("Stripe Checkout amount_total 无效:", amountTotal, ",", referenceId)
		return
	}

	currency := strings.ToLower(strings.TrimSpace(event.GetObjectValue("currency")))
	err = model.Recharge(referenceId, customerId, paidFen, currency)
	if err != nil {
		log.Println(err.Error(), referenceId)
		return
	}

	log.Printf("收到款项：%s, %.2f(%s)", referenceId, float64(paidFen)/100, strings.ToUpper(currency))
}

func sessionExpired(event stripe.Event) {
	referenceId := event.GetObjectValue("client_reference_id")
	status := event.GetObjectValue("status")
	if "expired" != status {
		log.Println("错误的Stripe Checkout过期状态:", status, ",", referenceId)
		return
	}

	if len(referenceId) == 0 {
		log.Println("未提供支付单号")
		return
	}

	topUp := model.GetTopUpByTradeNo(referenceId)
	if topUp == nil {
		log.Println("充值订单不存在", referenceId)
		return
	}

	if topUp.Status != common.TopUpStatusPending {
		log.Println("充值订单状态错误", referenceId)
	}

	topUp.Status = common.TopUpStatusExpired
	err := topUp.Update()
	if err != nil {
		log.Println("过期充值订单失败", referenceId, ", err:", err.Error())
		return
	}

	log.Println("充值订单已过期", referenceId)
}

func genStripeLink(referenceId string, customerId string, email string, amount int64, payFen int64) (string, error) {
	if !strings.HasPrefix(setting.StripeApiSecret, "sk_") && !strings.HasPrefix(setting.StripeApiSecret, "rk_") {
		return "", fmt.Errorf("无效的Stripe API密钥")
	}
	if payFen <= 0 {
		return "", errors.New("支付金额无效")
	}

	stripe.Key = setting.StripeApiSecret
	lineItem, err := buildStripeCheckoutLineItem(amount, payFen)
	if err != nil {
		return "", err
	}

	params := &stripe.CheckoutSessionParams{
		ClientReferenceID: stripe.String(referenceId),
		SuccessURL:        stripe.String(system_setting.ServerAddress + "/console/log"),
		CancelURL:         stripe.String(system_setting.ServerAddress + "/topup"),
		LineItems:         []*stripe.CheckoutSessionLineItemParams{lineItem},
		Mode:              stripe.String(string(stripe.CheckoutSessionModePayment)),
	}

	if "" == customerId {
		if "" != email {
			params.CustomerEmail = stripe.String(email)
		}

		params.CustomerCreation = stripe.String(string(stripe.CheckoutSessionCustomerCreationAlways))
	} else {
		params.Customer = stripe.String(customerId)
	}

	result, err := session.New(params)
	if err != nil {
		return "", err
	}

	return result.URL, nil
}

func buildStripeCheckoutLineItem(amount int64, payFen int64) (*stripe.CheckoutSessionLineItemParams, error) {
	priceInfo, err := loadStripeCheckoutPrice()
	if err != nil {
		return nil, err
	}

	priceData := &stripe.CheckoutSessionLineItemPriceDataParams{
		Currency:   stripe.String("cny"),
		UnitAmount: stripe.Int64(payFen),
	}
	if priceInfo.Product != nil && priceInfo.Product.ID != "" {
		priceData.Product = stripe.String(priceInfo.Product.ID)
	} else {
		priceData.ProductData = &stripe.CheckoutSessionLineItemPriceDataProductDataParams{
			Name: stripe.String(fmt.Sprintf("TopUp %d", amount)),
		}
	}

	return &stripe.CheckoutSessionLineItemParams{
		PriceData: priceData,
		Quantity:  stripe.Int64(1),
	}, nil
}

func loadStripeCheckoutPrice() (*stripe.Price, error) {
	params := &stripe.PriceParams{}
	params.AddExpand("product")
	priceInfo, err := stripeprice.Get(setting.StripePriceId, params)
	if err != nil {
		return nil, err
	}
	if !priceInfo.Active {
		return nil, errors.New("Stripe 商品价格已停用")
	}
	if strings.ToLower(string(priceInfo.Currency)) != "cny" {
		return nil, fmt.Errorf("Stripe 商品价格货币必须为 CNY，当前为 %s", strings.ToUpper(string(priceInfo.Currency)))
	}
	return priceInfo, nil
}

func getStripePayMoney(amount float64, groupID int) float64 {
	originalAmount := amount
	if !common.DisplayInCurrencyEnabled {
		amount = amount / common.QuotaPerUnit
	}
	// Using float64 for monetary calculations is acceptable here due to the small amounts involved
	topupGroupRatio := common.GetTopupGroupRatio(groupID)
	if topupGroupRatio == 0 {
		topupGroupRatio = 1
	}
	// apply optional preset discount by the original request amount (if configured), default 1.0
	discount := 1.0
	if ds, ok := operation_setting.GetPaymentSetting().AmountDiscount[int(originalAmount)]; ok {
		if ds > 0 {
			discount = ds
		}
	}
	payMoney := amount * setting.StripeUnitPrice * topupGroupRatio * discount
	return payMoney
}

func getStripeMinTopup() int64 {
	minTopup := setting.StripeMinTopUp
	if !common.DisplayInCurrencyEnabled {
		minTopup = minTopup * int(common.QuotaPerUnit)
	}
	return int64(minTopup)
}
