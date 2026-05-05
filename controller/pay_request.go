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

// prepaidRequestOrderConfig implements PayOrderConfig for prepaid-request orders.
type prepaidRequestOrderConfig struct{}

var prepaidRequestCfg PayOrderConfig = &prepaidRequestOrderConfig{}

func (*prepaidRequestOrderConfig) TradeNoPrefix() string   { return "PAYR" }
func (*prepaidRequestOrderConfig) EpayProductName() string { return "PayPerRequest" }
func (*prepaidRequestOrderConfig) NotifyPath() string      { return "/api/pay_request/epay/notify" }
func (*prepaidRequestOrderConfig) ReturnPath() string      { return "/api/pay_request/epay/return" }
func (*prepaidRequestOrderConfig) CheckoutPath() string    { return "" }
func (*prepaidRequestOrderConfig) ReturnRedirectPath() string {
	return "/console/my_subscription"
}
func (*prepaidRequestOrderConfig) LogPrefix() string { return "按次付费" }
func (*prepaidRequestOrderConfig) BalanceRecordType() string {
	return model.BalanceRecordTypePayRequestPayOut
}
func (*prepaidRequestOrderConfig) BalanceRecordRemark() string { return "pay_request" }
func (*prepaidRequestOrderConfig) PayMethodBalance() string    { return model.PayRequestPayMethodBalance }
func (*prepaidRequestOrderConfig) PayMethodEpay() string       { return model.PayRequestPayMethodEpay }
func (*prepaidRequestOrderConfig) UseGatewayQRCode() bool      { return false }
func (*prepaidRequestOrderConfig) StatusPending() string       { return model.PayRequestOrderStatusPending }
func (*prepaidRequestOrderConfig) StatusSuccess() string       { return model.PayRequestOrderStatusSuccess }
func (*prepaidRequestOrderConfig) RequireProductId() bool      { return true }
func (*prepaidRequestOrderConfig) SupportsQuantity() bool      { return false }

func (*prepaidRequestOrderConfig) ComputeCredit(amountFen int64) (int, error) {
	if amountFen <= 0 {
		return 0, errors.New("money 必须大于0")
	}
	rate := payg_setting.GetPaygSettings().CreditRequestsPerCny
	if rate <= 0 {
		return 0, errors.New("按次付费兑换比例未配置")
	}
	dFen := decimal.NewFromInt(amountFen)
	dYuan := dFen.Div(decimal.NewFromInt(100))
	dCount := dYuan.Mul(decimal.NewFromInt(int64(rate)))
	credit := int(dCount.Floor().IntPart())
	if credit <= 0 {
		return 0, errors.New("兑换次数过低")
	}
	return credit, nil
}

func (*prepaidRequestOrderConfig) ValidateAndLoadProduct(productId int) (productInfo, error) {
	if productId <= 0 {
		return productInfo{}, errors.New("请先选择按次付费商品")
	}

	// Try DB first, then fallback to in-memory config
	if model.DB != nil && model.DB.Migrator().HasTable(&model.PayRequestProduct{}) && model.DB.Migrator().HasTable(&model.PayRequestProductGroup{}) {
		var product model.PayRequestProduct
		if err := model.DB.Where("id = ?", productId).First(&product).Error; err != nil {
			return productInfo{}, err
		}
		if !product.Enabled {
			return productInfo{}, errors.New("商品不存在或已禁用")
		}
		if product.Stock != nil && *product.Stock <= 0 {
			return productInfo{}, errors.New("按次付费商品库存不足")
		}

		ids, err := model.GetPayRequestProductAllowedGroupIDsTx(nil, product.Id)
		if err != nil {
			return productInfo{}, err
		}
		if len(ids) == 0 {
			return productInfo{}, errors.New("商品未配置可用分组")
		}
		if err := model.ValidateGroupIDsExist(nil, ids); err != nil {
			return productInfo{}, err
		}
		marshaledJSON, err := model.MarshalGroupIDsJSON(ids)
		if err != nil {
			return productInfo{}, err
		}

		return productInfo{
			ProductId:           product.Id,
			ProductName:         product.Name,
			SortOrder:           product.SortOrder,
			AllowedGroupIDs:     ids,
			AllowedGroupIDsJSON: marshaledJSON,
		}, nil
	}

	// Fallback to in-memory config
	product, ok := payg_setting.FindPayRequestProductByID(productId)
	if !ok || !product.Enabled {
		return productInfo{}, errors.New("商品不存在或已禁用")
	}
	if product.Stock != nil && *product.Stock <= 0 {
		return productInfo{}, errors.New("按次付费商品库存不足")
	}
	if len(product.AllowedGroupIds) == 0 {
		return productInfo{}, errors.New("商品未配置可用分组")
	}
	if err := model.ValidateGroupIDsExist(nil, product.AllowedGroupIds); err != nil {
		return productInfo{}, err
	}
	marshaledJSON, err := model.MarshalGroupIDsJSON(product.AllowedGroupIds)
	if err != nil {
		return productInfo{}, err
	}

	return productInfo{
		ProductId:           product.Id,
		ProductName:         product.Name,
		SortOrder:           product.SortOrder,
		AllowedGroupIDs:     product.AllowedGroupIds,
		AllowedGroupIDsJSON: marshaledJSON,
	}, nil
}

func (*prepaidRequestOrderConfig) InsertOrder(tx *gorm.DB, p orderParams) error {
	order := &model.PayRequestOrder{
		UserId:          p.UserId,
		TradeNo:         p.TradeNo,
		PayMethod:       p.PayMethod,
		EpayMethod:      p.EpayMethod,
		AmountFen:       p.AmountFen,
		CreditRequests:  p.CreditAmount,
		ProductId:       p.ProductId,
		ProductName:     p.ProductName,
		AllowedGroupIds: p.AllowedGroupIDsJSON,
		Status:          model.PayRequestOrderStatusPending,
	}
	return order.Insert(tx)
}

func (*prepaidRequestOrderConfig) SaveEpayCheckout(tx *gorm.DB, tradeNo string, checkout epayGatewayCheckout) error {
	return nil
}

func (*prepaidRequestOrderConfig) CompleteOrderTx(tx *gorm.DB, tradeNo string, paidAt int64, paidFen int64, pInfo productInfo) error {
	return completePayRequestOrderTx(tx, tradeNo, paidAt, paidFen, pInfo.ProductName, pInfo.SortOrder, pInfo.AllowedGroupIDs)
}

func (*prepaidRequestOrderConfig) QueryOrderStatus(tradeNo string, userId int) (gin.H, error) {
	var order model.PayRequestOrder
	if err := model.DB.
		Select("trade_no", "status", "paid_at", "finished_at", "amount_fen", "credit_requests", "pay_method").
		Where("trade_no = ? AND user_id = ?", tradeNo, userId).
		First(&order).Error; err != nil {
		return nil, err
	}
	return gin.H{
		"trade_no":        order.TradeNo,
		"status":          order.Status,
		"paid_at":         order.PaidAt,
		"finished_at":     order.FinishedAt,
		"amount_fen":      order.AmountFen,
		"credit_requests": order.CreditRequests,
	}, nil
}

func (*prepaidRequestOrderConfig) ReadOrderUserAndInviter(tx *gorm.DB, tradeNo string) (int, int, error) {
	var order model.PayRequestOrder
	if err := tx.Select("user_id", "inviter_id").Where("trade_no = ?", tradeNo).First(&order).Error; err != nil {
		return 0, 0, err
	}
	return order.UserId, order.InviterId, nil
}

// --- Gin handlers (thin wrappers) ---

func CreatePayRequestOrder(c *gin.Context)    { CreatePayOrder(c, prepaidRequestCfg) }
func PayRequestEpayNotify(c *gin.Context)     { PayOrderEpayNotify(c, prepaidRequestCfg) }
func PayRequestEpayReturn(c *gin.Context)     { PayOrderEpayReturn(c, prepaidRequestCfg) }
func GetPayRequestOrderStatus(c *gin.Context) { GetPayOrderStatus(c, prepaidRequestCfg) }

// --- Order completion (type-specific logic, keeps strict compatibility for historical legacy orders) ---

func resolveLegacyPayRequestOrderGroupIDsTx(tx *gorm.DB, userId int) ([]int, error) {
	if tx == nil {
		return nil, errors.New("tx 为空")
	}
	if userId <= 0 {
		return nil, errors.New("user_id 无效")
	}

	_, groupIDs, err := model.GetUserPayRequestBalanceInfoTx(tx, userId)
	if err != nil {
		return nil, err
	}
	groupIDs = model.NormalizeUniqueSortedIDs(groupIDs)
	if len(groupIDs) > 0 {
		return groupIDs, nil
	}

	var user model.User
	if err := tx.Select("pay_request_allowed_groups").Where("id = ?", userId).First(&user).Error; err != nil {
		return nil, err
	}

	groupIDs, idErr := model.ParseGroupIDsJSON(user.PayRequestAllowedGroups)
	if idErr == nil && len(groupIDs) > 0 {
		return model.NormalizeUniqueSortedIDs(groupIDs), nil
	}

	codes, codeErr := model.ParseGroupNamesJSON(user.PayRequestAllowedGroups)
	if codeErr != nil {
		if idErr != nil {
			return nil, idErr
		}
		return nil, codeErr
	}
	if len(codes) == 0 {
		return nil, nil
	}

	groupIDs, err = model.GroupIDsFromCodes(tx, codes)
	if err != nil {
		return nil, err
	}
	return model.NormalizeUniqueSortedIDs(groupIDs), nil
}

func completePayRequestOrderTx(tx *gorm.DB, tradeNo string, paidAt int64, paidFen int64, productName string, productSortOrder int, productAllowedGroupIDs []int) error {
	if tx == nil {
		return errors.New("tx 为空")
	}
	if tradeNo == "" {
		return errors.New("tradeNo 为空")
	}

	var order model.PayRequestOrder
	if err := lockForUpdate(tx).Where("trade_no = ?", tradeNo).First(&order).Error; err != nil {
		return err
	}

	if order.Status == model.PayRequestOrderStatusSuccess {
		return nil
	}
	if order.Status != model.PayRequestOrderStatusPending {
		return errors.New("订单状态错误")
	}
	if paidFen != order.AmountFen {
		return fmt.Errorf("订单金额不一致：paid=%s expected=%s", formatFenYuan(paidFen), formatFenYuan(order.AmountFen))
	}
	if order.CreditRequests <= 0 {
		return errors.New("订单兑换次数未配置")
	}
	if order.ProductId > 0 {
		if err := model.ConsumePayRequestProductStockTx(tx, order.ProductId, 1); err != nil {
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
	if ids, err := model.ParseGroupIDsJSON(order.AllowedGroupIds); err == nil && len(ids) > 0 {
		groupIDs = ids
	}

	balanceProductId := order.ProductId
	name := order.ProductName
	sortOrder := 0
	if order.ProductId > 0 {
		if len(groupIDs) == 0 {
			ids, pErr := model.GetPayRequestProductAllowedGroupIDsTx(tx, order.ProductId)
			if pErr == nil && len(ids) > 0 {
				groupIDs = ids
			}
		}
		if len(groupIDs) == 0 {
			if p, ok := payg_setting.FindPayRequestProductByID(order.ProductId); ok {
				groupIDs = model.NormalizeUniqueSortedIDs(p.AllowedGroupIds)
			}
		}

		if strings.TrimSpace(name) == "" {
			name = strings.TrimSpace(productName)
		}
		if productSortOrder > 0 {
			sortOrder = productSortOrder
		}
		if name == "" || sortOrder <= 0 {
			if p, ok := payg_setting.FindPayRequestProductByID(order.ProductId); ok {
				if name == "" {
					name = p.Name
				}
				if sortOrder <= 0 && p.SortOrder > 0 {
					sortOrder = p.SortOrder
				}
			}
		}
	} else if len(groupIDs) == 0 {
		ids, err := resolveLegacyPayRequestOrderGroupIDsTx(tx, order.UserId)
		if err != nil {
			return err
		}
		groupIDs = ids
	}

	groupIDs = model.NormalizeUniqueSortedIDs(groupIDs)
	if len(groupIDs) == 0 {
		return errors.New("按次付费可用分组为空，无法充值")
	}
	if err := model.ValidateGroupIDsExist(tx, groupIDs); err != nil {
		return err
	}
	groupIDsJSON, err := model.MarshalGroupIDsJSON(groupIDs)
	if err != nil {
		return err
	}
	if balanceProductId == 0 {
		balanceProductId = -1
	}

	if err := model.UpsertPayRequestUserBalanceTx(
		tx, order.UserId, balanceProductId, name, sortOrder, groupIDs, order.CreditRequests,
	); err != nil {
		return err
	}
	if _, err := model.SyncUserPayRequestSnapshotFromBalancesTx(tx, order.UserId); err != nil {
		return err
	}

	commissionEligible := order.PayMethod == model.PayRequestPayMethodEpay
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

	order.Status = model.PayRequestOrderStatusSuccess
	order.PaidAt = paidAt
	order.FinishedAt = paidAt
	if strings.TrimSpace(order.ProductName) == "" {
		order.ProductName = name
	}
	order.AllowedGroupIds = groupIDsJSON
	order.IsFirstPurchase = isFirstPurchase
	order.InviterId = inviterId
	order.CommissionPercent = commissionPercent
	order.CommissionFen = commissionFen
	return tx.Save(&order).Error
}
