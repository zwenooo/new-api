package controller

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"html"
	"image/png"
	"one-api/common"
	"one-api/model"
	"one-api/setting/payg_setting"
	"strings"

	"github.com/boombuler/barcode"
	"github.com/boombuler/barcode/qr"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// prepaidCreditOrderConfig implements PayOrderConfig for prepaid-credit orders.
// It still serves the historical /payg route and order model for compatibility.
type prepaidCreditOrderConfig struct{}

var prepaidCreditCfg PayOrderConfig = &prepaidCreditOrderConfig{}

func (*prepaidCreditOrderConfig) TradeNoPrefix() string       { return "PAYG" }
func (*prepaidCreditOrderConfig) EpayProductName() string     { return "PayAsYouGo" }
func (*prepaidCreditOrderConfig) NotifyPath() string          { return "/api/payg/epay/notify" }
func (*prepaidCreditOrderConfig) ReturnPath() string          { return "/api/payg/epay/return" }
func (*prepaidCreditOrderConfig) CheckoutPath() string        { return "/api/payg/epay/checkout" }
func (*prepaidCreditOrderConfig) ReturnRedirectPath() string  { return "/console/my_subscription" }
func (*prepaidCreditOrderConfig) LogPrefix() string           { return "按量付费" }
func (*prepaidCreditOrderConfig) BalanceRecordType() string   { return model.BalanceRecordTypePaygPayOut }
func (*prepaidCreditOrderConfig) BalanceRecordRemark() string { return "payg" }
func (*prepaidCreditOrderConfig) PayMethodBalance() string    { return model.PaygPayMethodBalance }
func (*prepaidCreditOrderConfig) PayMethodEpay() string       { return model.PaygPayMethodEpay }
func (*prepaidCreditOrderConfig) UseGatewayQRCode() bool      { return true }
func (*prepaidCreditOrderConfig) StatusPending() string       { return model.PaygOrderStatusPending }
func (*prepaidCreditOrderConfig) StatusSuccess() string       { return model.PaygOrderStatusSuccess }
func (*prepaidCreditOrderConfig) RequireProductId() bool      { return true }
func (*prepaidCreditOrderConfig) SupportsQuantity() bool      { return false }

func (*prepaidCreditOrderConfig) ComputeCredit(amountFen int64) (int, error) {
	if amountFen <= 0 {
		return 0, errors.New("money 必须大于0")
	}
	rate := payg_setting.GetPaygSettings().CreditUsdPerCny
	if rate <= 0 {
		return 0, errors.New("按量付费兑换比例未配置")
	}
	dFen := decimal.NewFromInt(amountFen)
	dYuan := dFen.Div(decimal.NewFromInt(100))
	dUsd := dYuan.Mul(decimal.NewFromFloat(rate))
	dQuota := dUsd.Mul(decimal.NewFromFloat(common.QuotaPerUnit))
	creditQuota := int(dQuota.Round(0).IntPart())
	if creditQuota <= 0 {
		return 0, errors.New("兑换额度过低")
	}
	return creditQuota, nil
}

func (*prepaidCreditOrderConfig) ValidateAndLoadProduct(productId int) (productInfo, error) {
	if model.ClawBoxProductModeEnabled() {
		clawBoxProductID, err := model.ResolveClawBoxProductIDTx(nil)
		if err != nil {
			return productInfo{}, err
		}
		if productId != clawBoxProductID {
			return productInfo{}, errors.New("ClawBox 当前仅允许充值指定商品")
		}
	}
	if productId <= 0 {
		return productInfo{}, errors.New("请先选择按量付费商品")
	}
	var product model.PaygProduct
	if err := model.DB.Where("id = ?", productId).First(&product).Error; err != nil {
		return productInfo{}, err
	}
	if !product.Enabled {
		return productInfo{}, errors.New("按量付费商品已下架")
	}
	if product.Stock != nil && *product.Stock <= 0 {
		return productInfo{}, errors.New("按量付费商品库存不足")
	}

	allowedGroupIDs, err := model.GetPaygProductAllowedGroupIDsTx(nil, product.Id)
	if err != nil {
		return productInfo{}, err
	}
	if len(allowedGroupIDs) == 0 {
		return productInfo{}, errors.New("请先配置按量付费可用分组")
	}
	if err := model.ValidateGroupIDsExist(nil, allowedGroupIDs); err != nil {
		return productInfo{}, err
	}
	var allowedGroupIDsJSON model.JSONValue
	if b, mErr := common.Marshal(allowedGroupIDs); mErr == nil {
		allowedGroupIDsJSON = model.JSONValue(b)
	} else {
		return productInfo{}, mErr
	}

	return productInfo{
		ProductId:           product.Id,
		ProductName:         product.Name,
		SortOrder:           0, // payg doesn't pass sortOrder at creation time
		AllowedGroupIDs:     allowedGroupIDs,
		AllowedGroupIDsJSON: allowedGroupIDsJSON,
	}, nil
}

func (*prepaidCreditOrderConfig) InsertOrder(tx *gorm.DB, p orderParams) error {
	order := &model.PaygOrder{
		UserId:             p.UserId,
		ProductId:          p.ProductId,
		ProductName:        p.ProductName,
		PresetId:           0,
		AllowedGroups:      nil,
		AllowedGroupIds:    p.AllowedGroupIDsJSON,
		TradeNo:            p.TradeNo,
		PayMethod:          p.PayMethod,
		EpayMethod:         p.EpayMethod,
		EpayGatewayTradeNo: p.EpayGatewayTradeNo,
		EpayPayURL:         p.EpayPayURL,
		EpayQRCode:         p.EpayQRCode,
		EpayImageURL:       p.EpayImageURL,
		AmountFen:          p.AmountFen,
		CreditQuota:        p.CreditAmount,
		Status:             model.PaygOrderStatusPending,
	}
	return order.Insert(tx)
}

func (*prepaidCreditOrderConfig) SaveEpayCheckout(tx *gorm.DB, tradeNo string, checkout epayGatewayCheckout) error {
	return tx.Model(&model.PaygOrder{}).
		Where("trade_no = ?", tradeNo).
		Updates(map[string]interface{}{
			"epay_gateway_trade_no": strings.TrimSpace(checkout.GatewayTradeNo),
			"epay_pay_url":          strings.TrimSpace(checkout.PayPageURL),
			"epay_qrcode":           strings.TrimSpace(checkout.QRCode),
			"epay_image_url":        strings.TrimSpace(checkout.QRImageURL),
		}).Error
}

func (*prepaidCreditOrderConfig) CompleteOrderTx(tx *gorm.DB, tradeNo string, paidAt int64, paidFen int64, pInfo productInfo) error {
	return completePaygOrderTx(tx, tradeNo, paidAt, paidFen)
}

func (*prepaidCreditOrderConfig) QueryOrderStatus(tradeNo string, userId int) (gin.H, error) {
	var order model.PaygOrder
	if err := model.DB.
		Select("trade_no", "status", "paid_at", "finished_at", "amount_fen", "credit_quota", "pay_method").
		Where("trade_no = ? AND user_id = ?", tradeNo, userId).
		First(&order).Error; err != nil {
		return nil, err
	}
	return gin.H{
		"trade_no":     order.TradeNo,
		"status":       order.Status,
		"paid_at":      order.PaidAt,
		"finished_at":  order.FinishedAt,
		"amount_fen":   order.AmountFen,
		"credit_quota": order.CreditQuota,
	}, nil
}

func (*prepaidCreditOrderConfig) ReadOrderUserAndInviter(tx *gorm.DB, tradeNo string) (int, int, error) {
	var order model.PaygOrder
	if err := tx.Select("user_id", "inviter_id").Where("trade_no = ?", tradeNo).First(&order).Error; err != nil {
		return 0, 0, err
	}
	return order.UserId, order.InviterId, nil
}

// --- Gin handlers (thin wrappers) ---

func CreatePaygOrder(c *gin.Context) { CreatePayOrder(c, prepaidCreditCfg) }

func renderPaygCheckoutQRCodeDataURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("二维码内容为空")
	}

	code, err := qr.Encode(raw, qr.M, qr.Auto)
	if err != nil {
		return "", err
	}
	scaled, err := barcode.Scale(code, 280, 280)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, scaled); err != nil {
		return "", err
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

func PaygEpayCheckout(c *gin.Context) {
	tradeNo := strings.TrimSpace(c.Query("trade_no"))
	if tradeNo == "" {
		c.String(400, "trade_no 不能为空")
		return
	}

	var order model.PaygOrder
	if err := model.DB.
		Select("trade_no", "status", "pay_method", "epay_pay_url", "epay_qrcode", "epay_image_url").
		Where("trade_no = ?", tradeNo).
		First(&order).Error; err != nil {
		c.String(404, "订单不存在")
		return
	}
	if order.PayMethod != model.PaygPayMethodEpay {
		c.String(400, "订单支付方式错误")
		return
	}
	if order.Status != model.PaygOrderStatusPending {
		c.String(400, "订单当前不可继续支付")
		return
	}
	if payURL := strings.TrimSpace(order.EpayPayURL); payURL != "" {
		c.Redirect(302, payURL)
		return
	}
	imageURL := strings.TrimSpace(order.EpayImageURL)
	if imageURL == "" {
		if qrDataURL, err := renderPaygCheckoutQRCodeDataURL(order.EpayQRCode); err == nil {
			imageURL = qrDataURL
		}
	}
	if imageURL != "" {
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.String(200, fmt.Sprintf(`<!DOCTYPE html>
<html lang="zh-CN">
  <head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width,initial-scale=1">
    <title>ClawBox 支付二维码</title>
    <style>
      body { margin: 0; font-family: -apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif; background: #f8fafc; color: #0f172a; }
      main { min-height: 100vh; display: flex; align-items: center; justify-content: center; padding: 24px; }
      .card { width: min(92vw, 420px); background: #fff; border-radius: 20px; padding: 24px; box-shadow: 0 20px 40px rgba(15, 23, 42, 0.12); text-align: center; }
      img { width: min(100%%, 280px); height: auto; border-radius: 16px; background: #fff; }
      p { margin: 0 0 16px; color: #475569; line-height: 1.6; }
      code { font-size: 12px; color: #64748b; word-break: break-all; }
    </style>
  </head>
  <body>
    <main>
      <section class="card">
        <p>请使用支付宝或微信扫码完成付款。</p>
        <img src="%s" alt="支付二维码">
        <p><code>%s</code></p>
      </section>
    </main>
  </body>
</html>`, html.EscapeString(imageURL), html.EscapeString(order.TradeNo)))
		return
	}
	c.String(400, "订单缺少支付二维码，请重新创建订单")
}
func PaygEpayNotify(c *gin.Context)     { PayOrderEpayNotify(c, prepaidCreditCfg) }
func PaygEpayReturn(c *gin.Context)     { PayOrderEpayReturn(c, prepaidCreditCfg) }
func GetPaygOrderStatus(c *gin.Context) { GetPayOrderStatus(c, prepaidCreditCfg) }

// --- Order completion (type-specific logic) ---

func completePaygOrderTx(tx *gorm.DB, tradeNo string, paidAt int64, paidFen int64) error {
	if tx == nil {
		return errors.New("tx 为空")
	}
	if tradeNo == "" {
		return errors.New("tradeNo 为空")
	}

	var order model.PaygOrder
	if err := lockForUpdate(tx).Where("trade_no = ?", tradeNo).First(&order).Error; err != nil {
		return err
	}

	if order.Status == model.PaygOrderStatusSuccess {
		return nil
	}
	if order.Status != model.PaygOrderStatusPending {
		return errors.New("订单状态错误")
	}
	if paidFen != order.AmountFen {
		return fmt.Errorf("订单金额不一致：paid=%s expected=%s", formatFenYuan(paidFen), formatFenYuan(order.AmountFen))
	}
	if order.CreditQuota <= 0 {
		return errors.New("订单兑换额度未配置")
	}
	if order.ProductId > 0 {
		if err := model.ConsumePaygProductStockTx(tx, order.ProductId, 1); err != nil {
			return err
		}
	}

	var user model.User
	if err := lockForUpdate(tx).
		Select("id").
		Where("id = ?", order.UserId).
		First(&user).Error; err != nil {
		return err
	}

	orderGroupIDs, err := model.ParseGroupIDsJSON(order.AllowedGroupIds)
	if err != nil {
		return err
	}
	if len(orderGroupIDs) == 0 {
		if order.ProductId <= 0 {
			return errors.New("订单缺少按量付费商品分组信息")
		}
		productIDs, pErr := model.GetPaygProductAllowedGroupIDsTx(tx, order.ProductId)
		if pErr == nil && len(productIDs) > 0 {
			orderGroupIDs = productIDs
		} else {
			p, ok := payg_setting.FindPaygProductByID(order.ProductId)
			if !ok {
				return errors.New("订单商品已不存在")
			}
			ids := model.NormalizeUniqueSortedIDs(p.AllowedGroupIds)
			if len(ids) == 0 {
				return errors.New("按量付费商品可用分组为空")
			}
			orderGroupIDs = ids
		}
	}
	if len(orderGroupIDs) == 0 {
		return errors.New("按量付费可用分组为空，无法充值")
	}

	balanceProductId := order.ProductId
	if balanceProductId == 0 {
		balanceProductId = -1
	}

	productName := order.ProductName
	sortOrder := 0
	if order.ProductId > 0 {
		if p, ok := payg_setting.FindPaygProductByID(order.ProductId); ok {
			if productName == "" {
				productName = p.Name
			}
			if p.SortOrder > 0 {
				sortOrder = p.SortOrder
			}
		}
	}
	if err := model.UpsertPaygUserBalanceTx(
		tx, order.UserId, balanceProductId, productName, sortOrder, orderGroupIDs, order.CreditQuota,
	); err != nil {
		return err
	}

	balances, err := model.GetUserPaygBalancesTx(tx, order.UserId, true)
	if err != nil {
		return err
	}
	unionGroupsJSON, err := model.UnionPaygAllowedGroupsFromBalances(balances)
	if err != nil {
		return err
	}

	if err := tx.Model(&model.User{}).Where("id = ?", order.UserId).Updates(map[string]interface{}{
		"payg_quota":          gorm.Expr("payg_quota + ?", order.CreditQuota),
		"payg_history_quota":  gorm.Expr("payg_history_quota + ?", order.CreditQuota),
		"payg_allowed_groups": unionGroupsJSON,
		"quota":               gorm.Expr("quota + ?", order.CreditQuota),
	}).Error; err != nil {
		return err
	}

	commissionEligible := order.PayMethod == model.PaygPayMethodEpay
	isFirstPurchase := false
	inviterId := 0
	commissionPercent := 0
	var commissionFen int64
	if commissionEligible {
		paidCount, err := model.CountUserSuccessfulCommissionablePaidEventsTx(tx, order.UserId)
		if err != nil {
			return err
		}
		isFirstPurchase = paidCount == 0
		inviterId, commissionPercent, commissionFen, err = model.ApplyInvitationCommissionTx(tx, order.UserId, paidFen, isFirstPurchase)
		if err != nil {
			return err
		}
	}

	order.Status = model.PaygOrderStatusSuccess
	order.PaidAt = paidAt
	order.FinishedAt = paidAt
	order.IsFirstPurchase = isFirstPurchase
	order.InviterId = inviterId
	order.CommissionPercent = commissionPercent
	order.CommissionFen = commissionFen
	return tx.Save(&order).Error
}
