package controller

import (
	"errors"
	"fmt"
	"one-api/model"
	"one-api/setting/payg_setting"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// prepaidTokenOrderConfig implements PayOrderConfig for prepaid-token orders.
type prepaidTokenOrderConfig struct{}

var prepaidTokenCfg PayOrderConfig = &prepaidTokenOrderConfig{}

func (*prepaidTokenOrderConfig) TradeNoPrefix() string      { return "PAYT" }
func (*prepaidTokenOrderConfig) EpayProductName() string    { return "PayToken" }
func (*prepaidTokenOrderConfig) NotifyPath() string         { return "/api/pay_token/epay/notify" }
func (*prepaidTokenOrderConfig) ReturnPath() string         { return "/api/pay_token/epay/return" }
func (*prepaidTokenOrderConfig) CheckoutPath() string       { return "" }
func (*prepaidTokenOrderConfig) ReturnRedirectPath() string { return "/console/my_subscription" }
func (*prepaidTokenOrderConfig) LogPrefix() string          { return "按token付费" }
func (*prepaidTokenOrderConfig) BalanceRecordType() string {
	return model.BalanceRecordTypePayTokenPayOut
}
func (*prepaidTokenOrderConfig) BalanceRecordRemark() string { return "pay_token" }
func (*prepaidTokenOrderConfig) PayMethodBalance() string    { return model.PayTokenPayMethodBalance }
func (*prepaidTokenOrderConfig) PayMethodEpay() string       { return model.PayTokenPayMethodEpay }
func (*prepaidTokenOrderConfig) UseGatewayQRCode() bool      { return false }
func (*prepaidTokenOrderConfig) StatusPending() string       { return model.PayTokenOrderStatusPending }
func (*prepaidTokenOrderConfig) StatusSuccess() string       { return model.PayTokenOrderStatusSuccess }
func (*prepaidTokenOrderConfig) RequireProductId() bool      { return true }
func (*prepaidTokenOrderConfig) SupportsQuantity() bool      { return false }

func (*prepaidTokenOrderConfig) ComputeCredit(amountFen int64) (int, error) {
	if amountFen <= 0 {
		return 0, errors.New("money 必须大于0")
	}
	rate := payg_setting.GetPaygSettings().CreditTokensPerCny
	if rate <= 0 {
		return 0, errors.New("按token付费兑换比例未配置")
	}
	dFen := decimal.NewFromInt(amountFen)
	dYuan := dFen.Div(decimal.NewFromInt(100))
	dTokens := dYuan.Mul(decimal.NewFromInt(int64(rate)))
	credit := int(dTokens.Floor().IntPart())
	if credit <= 0 {
		return 0, errors.New("兑换tokens过低")
	}
	return credit, nil
}

func (*prepaidTokenOrderConfig) ValidateAndLoadProduct(productId int) (productInfo, error) {
	if productId <= 0 {
		return productInfo{}, errors.New("请先选择按token付费商品")
	}
	var product model.PayTokenProduct
	if err := model.DB.Where("id = ?", productId).First(&product).Error; err != nil {
		return productInfo{}, err
	}
	if !product.Enabled {
		return productInfo{}, errors.New("按token付费商品已下架")
	}
	if product.Stock != nil && *product.Stock <= 0 {
		return productInfo{}, errors.New("按token付费商品库存不足")
	}

	allowedGroupIDs, err := model.GetPayTokenProductAllowedGroupIDsTx(nil, product.Id)
	if err != nil {
		return productInfo{}, err
	}
	if len(allowedGroupIDs) == 0 {
		return productInfo{}, errors.New("请先配置按token付费可用分组")
	}
	if err := model.ValidateGroupIDsExist(nil, allowedGroupIDs); err != nil {
		return productInfo{}, err
	}
	allowedGroupIDsJSON, err := model.MarshalGroupIDsJSON(allowedGroupIDs)
	if err != nil {
		return productInfo{}, err
	}

	return productInfo{
		ProductId:           product.Id,
		ProductName:         product.Name,
		SortOrder:           product.SortOrder,
		AllowedGroupIDs:     allowedGroupIDs,
		AllowedGroupIDsJSON: allowedGroupIDsJSON,
	}, nil
}

func (*prepaidTokenOrderConfig) InsertOrder(tx *gorm.DB, p orderParams) error {
	order := &model.PayTokenOrder{
		UserId:          p.UserId,
		ProductId:       p.ProductId,
		ProductName:     p.ProductName,
		AllowedGroupIds: p.AllowedGroupIDsJSON,
		TradeNo:         p.TradeNo,
		PayMethod:       p.PayMethod,
		EpayMethod:      p.EpayMethod,
		AmountFen:       p.AmountFen,
		CreditTokens:    p.CreditAmount,
		Status:          model.PayTokenOrderStatusPending,
	}
	return order.Insert(tx)
}

func (*prepaidTokenOrderConfig) SaveEpayCheckout(tx *gorm.DB, tradeNo string, checkout epayGatewayCheckout) error {
	return nil
}

func (*prepaidTokenOrderConfig) CompleteOrderTx(tx *gorm.DB, tradeNo string, paidAt int64, paidFen int64, pInfo productInfo) error {
	return completePayTokenOrderTx(tx, tradeNo, paidAt, paidFen, pInfo.ProductName, pInfo.SortOrder, pInfo.AllowedGroupIDs)
}

func (*prepaidTokenOrderConfig) QueryOrderStatus(tradeNo string, userId int) (gin.H, error) {
	var order model.PayTokenOrder
	if err := model.DB.
		Select("trade_no", "status", "paid_at", "finished_at", "amount_fen", "credit_tokens", "pay_method").
		Where("trade_no = ? AND user_id = ?", tradeNo, userId).
		First(&order).Error; err != nil {
		return nil, err
	}
	return gin.H{
		"trade_no":      order.TradeNo,
		"status":        order.Status,
		"paid_at":       order.PaidAt,
		"finished_at":   order.FinishedAt,
		"amount_fen":    order.AmountFen,
		"credit_tokens": order.CreditTokens,
	}, nil
}

func (*prepaidTokenOrderConfig) ReadOrderUserAndInviter(tx *gorm.DB, tradeNo string) (int, int, error) {
	var order model.PayTokenOrder
	if err := tx.Select("user_id", "inviter_id").Where("trade_no = ?", tradeNo).First(&order).Error; err != nil {
		return 0, 0, err
	}
	return order.UserId, order.InviterId, nil
}

// --- Gin handlers (thin wrappers) ---

func CreatePayTokenOrder(c *gin.Context)    { CreatePayOrder(c, prepaidTokenCfg) }
func PayTokenEpayNotify(c *gin.Context)     { PayOrderEpayNotify(c, prepaidTokenCfg) }
func PayTokenEpayReturn(c *gin.Context)     { PayOrderEpayReturn(c, prepaidTokenCfg) }
func GetPayTokenOrderStatus(c *gin.Context) { GetPayOrderStatus(c, prepaidTokenCfg) }

// --- Order completion (type-specific logic) ---

func completePayTokenOrderTx(tx *gorm.DB, tradeNo string, paidAt int64, paidFen int64, productName string, productSortOrder int, productAllowedGroupIDs []int) error {
	if tx == nil {
		return errors.New("tx 为空")
	}
	if tradeNo == "" {
		return errors.New("tradeNo 为空")
	}

	var order model.PayTokenOrder
	if err := lockForUpdate(tx).Where("trade_no = ?", tradeNo).First(&order).Error; err != nil {
		return err
	}

	if order.Status == model.PayTokenOrderStatusSuccess {
		return nil
	}
	if order.Status != model.PayTokenOrderStatusPending {
		return errors.New("订单状态错误")
	}
	if paidFen != order.AmountFen {
		return fmt.Errorf("订单金额不一致：paid=%s expected=%s", formatFenYuan(paidFen), formatFenYuan(order.AmountFen))
	}
	if order.CreditTokens <= 0 {
		return errors.New("订单兑换tokens未配置")
	}
	if order.ProductId > 0 {
		if err := model.ConsumePayTokenProductStockTx(tx, order.ProductId, 1); err != nil {
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

	groupIDs := model.NormalizeUniqueSortedIDs(productAllowedGroupIDs)
	if len(groupIDs) == 0 {
		if ids, err := model.ParseGroupIDsJSON(order.AllowedGroupIds); err == nil && len(ids) > 0 {
			groupIDs = ids
		}
	}
	if len(groupIDs) == 0 && order.ProductId > 0 {
		ids, pErr := model.GetPayTokenProductAllowedGroupIDsTx(tx, order.ProductId)
		if pErr == nil && len(ids) > 0 {
			groupIDs = ids
		}
	}
	if len(groupIDs) == 0 && order.ProductId > 0 {
		if p, ok := payg_setting.FindPayTokenProductByID(order.ProductId); ok {
			groupIDs = model.NormalizeUniqueSortedIDs(p.AllowedGroupIds)
		}
	}
	if len(groupIDs) == 0 {
		return errors.New("按token付费可用分组为空，无法充值")
	}

	balanceProductId := order.ProductId
	if balanceProductId == 0 {
		balanceProductId = -1
	}

	name := order.ProductName
	sortOrder := 0
	if strings.TrimSpace(name) == "" {
		name = strings.TrimSpace(productName)
	}
	if productSortOrder > 0 {
		sortOrder = productSortOrder
	}
	if order.ProductId > 0 && (name == "" || sortOrder <= 0) {
		if p, ok := payg_setting.FindPayTokenProductByID(order.ProductId); ok {
			if name == "" {
				name = p.Name
			}
			if sortOrder <= 0 && p.SortOrder > 0 {
				sortOrder = p.SortOrder
			}
		}
	}

	if err := model.UpsertPayTokenUserBalanceTx(
		tx, order.UserId, balanceProductId, name, sortOrder, groupIDs, order.CreditTokens,
	); err != nil {
		return err
	}
	if _, err := model.SyncUserPayTokenSnapshotFromBalancesTx(tx, order.UserId); err != nil {
		return err
	}

	commissionEligible := order.PayMethod == model.PayTokenPayMethodEpay
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

	order.Status = model.PayTokenOrderStatusSuccess
	order.PaidAt = paidAt
	order.FinishedAt = paidAt
	order.IsFirstPurchase = isFirstPurchase
	order.InviterId = inviterId
	order.CommissionPercent = commissionPercent
	order.CommissionFen = commissionFen
	return tx.Save(&order).Error
}
