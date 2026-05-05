package controller

import (
	"net/http"
	"one-api/common"
	"one-api/model"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

func AdminListSubscriptionOrders(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)

	keyword := strings.TrimSpace(c.Query("keyword"))
	status := strings.TrimSpace(c.Query("status"))
	payMethod := strings.TrimSpace(c.Query("pay_method"))

	userId := 0
	if raw := strings.TrimSpace(c.Query("user_id")); raw != "" {
		id, err := strconv.Atoi(raw)
		if err != nil || id <= 0 {
			common.ApiErrorMsg(c, "user_id 无效")
			return
		}
		userId = id
	}

	startTimestamp := int64(0)
	if raw := strings.TrimSpace(c.Query("start_timestamp")); raw != "" {
		ts, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || ts < 0 {
			common.ApiErrorMsg(c, "start_timestamp 无效")
			return
		}
		startTimestamp = ts
	}

	endTimestamp := int64(0)
	if raw := strings.TrimSpace(c.Query("end_timestamp")); raw != "" {
		ts, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || ts < 0 {
			common.ApiErrorMsg(c, "end_timestamp 无效")
			return
		}
		endTimestamp = ts
	}

	if startTimestamp > 0 && endTimestamp > 0 && startTimestamp > endTimestamp {
		common.ApiErrorMsg(c, "start_timestamp 不能大于 end_timestamp")
		return
	}

	if status != "" &&
		status != model.SubscriptionOrderStatusPending &&
		status != model.SubscriptionOrderStatusSuccess &&
		status != model.SubscriptionOrderStatusFailed {
		common.ApiErrorMsg(c, "status 无效")
		return
	}

	if payMethod != "" &&
		payMethod != model.SubscriptionPayMethodEpay &&
		payMethod != model.SubscriptionPayMethodBalance {
		common.ApiErrorMsg(c, "pay_method 无效")
		return
	}

	items, total, err := model.ListAdminSubscriptionOrders(model.AdminOrdersListQuery{
		Keyword:        keyword,
		Status:         status,
		PayMethod:      payMethod,
		UserId:         userId,
		StartTimestamp: startTimestamp,
		EndTimestamp:   endTimestamp,
	}, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}

	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(items)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    pageInfo,
	})
}

func AdminListTopUpOrders(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)

	keyword := strings.TrimSpace(c.Query("keyword"))
	status := strings.TrimSpace(c.Query("status"))

	userId := 0
	if raw := strings.TrimSpace(c.Query("user_id")); raw != "" {
		id, err := strconv.Atoi(raw)
		if err != nil || id <= 0 {
			common.ApiErrorMsg(c, "user_id 无效")
			return
		}
		userId = id
	}

	startTimestamp := int64(0)
	if raw := strings.TrimSpace(c.Query("start_timestamp")); raw != "" {
		ts, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || ts < 0 {
			common.ApiErrorMsg(c, "start_timestamp 无效")
			return
		}
		startTimestamp = ts
	}

	endTimestamp := int64(0)
	if raw := strings.TrimSpace(c.Query("end_timestamp")); raw != "" {
		ts, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || ts < 0 {
			common.ApiErrorMsg(c, "end_timestamp 无效")
			return
		}
		endTimestamp = ts
	}

	if startTimestamp > 0 && endTimestamp > 0 && startTimestamp > endTimestamp {
		common.ApiErrorMsg(c, "start_timestamp 不能大于 end_timestamp")
		return
	}

	if status != "" &&
		status != common.TopUpStatusPending &&
		status != common.TopUpStatusSuccess &&
		status != common.TopUpStatusExpired {
		common.ApiErrorMsg(c, "status 无效")
		return
	}

	items, total, err := model.ListAdminTopUpOrders(model.AdminOrdersListQuery{
		Keyword:        keyword,
		Status:         status,
		UserId:         userId,
		StartTimestamp: startTimestamp,
		EndTimestamp:   endTimestamp,
	}, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}

	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(items)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    pageInfo,
	})
}

func AdminListPaygOrders(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)

	keyword := strings.TrimSpace(c.Query("keyword"))
	status := strings.TrimSpace(c.Query("status"))
	payMethod := strings.TrimSpace(c.Query("pay_method"))

	userId := 0
	if raw := strings.TrimSpace(c.Query("user_id")); raw != "" {
		id, err := strconv.Atoi(raw)
		if err != nil || id <= 0 {
			common.ApiErrorMsg(c, "user_id 无效")
			return
		}
		userId = id
	}

	startTimestamp := int64(0)
	if raw := strings.TrimSpace(c.Query("start_timestamp")); raw != "" {
		ts, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || ts < 0 {
			common.ApiErrorMsg(c, "start_timestamp 无效")
			return
		}
		startTimestamp = ts
	}

	endTimestamp := int64(0)
	if raw := strings.TrimSpace(c.Query("end_timestamp")); raw != "" {
		ts, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || ts < 0 {
			common.ApiErrorMsg(c, "end_timestamp 无效")
			return
		}
		endTimestamp = ts
	}

	if startTimestamp > 0 && endTimestamp > 0 && startTimestamp > endTimestamp {
		common.ApiErrorMsg(c, "start_timestamp 不能大于 end_timestamp")
		return
	}

	if status != "" &&
		status != model.PaygOrderStatusPending &&
		status != model.PaygOrderStatusSuccess &&
		status != model.PaygOrderStatusFailed {
		common.ApiErrorMsg(c, "status 无效")
		return
	}

	if payMethod != "" &&
		payMethod != model.PaygPayMethodEpay &&
		payMethod != model.PaygPayMethodBalance {
		common.ApiErrorMsg(c, "pay_method 无效")
		return
	}

	items, total, err := model.ListAdminPaygOrders(model.AdminOrdersListQuery{
		Keyword:        keyword,
		Status:         status,
		PayMethod:      payMethod,
		UserId:         userId,
		StartTimestamp: startTimestamp,
		EndTimestamp:   endTimestamp,
	}, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}

	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(items)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    pageInfo,
	})
}

func AdminListPayRequestOrders(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)

	keyword := strings.TrimSpace(c.Query("keyword"))
	status := strings.TrimSpace(c.Query("status"))
	payMethod := strings.TrimSpace(c.Query("pay_method"))

	userId := 0
	if raw := strings.TrimSpace(c.Query("user_id")); raw != "" {
		id, err := strconv.Atoi(raw)
		if err != nil || id <= 0 {
			common.ApiErrorMsg(c, "user_id 无效")
			return
		}
		userId = id
	}

	startTimestamp := int64(0)
	if raw := strings.TrimSpace(c.Query("start_timestamp")); raw != "" {
		ts, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || ts < 0 {
			common.ApiErrorMsg(c, "start_timestamp 无效")
			return
		}
		startTimestamp = ts
	}

	endTimestamp := int64(0)
	if raw := strings.TrimSpace(c.Query("end_timestamp")); raw != "" {
		ts, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || ts < 0 {
			common.ApiErrorMsg(c, "end_timestamp 无效")
			return
		}
		endTimestamp = ts
	}

	if startTimestamp > 0 && endTimestamp > 0 && startTimestamp > endTimestamp {
		common.ApiErrorMsg(c, "start_timestamp 不能大于 end_timestamp")
		return
	}

	if status != "" &&
		status != model.PayRequestOrderStatusPending &&
		status != model.PayRequestOrderStatusSuccess &&
		status != model.PayRequestOrderStatusFailed {
		common.ApiErrorMsg(c, "status 无效")
		return
	}

	if payMethod != "" &&
		payMethod != model.PayRequestPayMethodEpay &&
		payMethod != model.PayRequestPayMethodBalance {
		common.ApiErrorMsg(c, "pay_method 无效")
		return
	}

	items, total, err := model.ListAdminPayRequestOrders(model.AdminOrdersListQuery{
		Keyword:        keyword,
		Status:         status,
		PayMethod:      payMethod,
		UserId:         userId,
		StartTimestamp: startTimestamp,
		EndTimestamp:   endTimestamp,
	}, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}

	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(items)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    pageInfo,
	})
}

func AdminListPayTokenOrders(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)

	keyword := strings.TrimSpace(c.Query("keyword"))
	status := strings.TrimSpace(c.Query("status"))
	payMethod := strings.TrimSpace(c.Query("pay_method"))

	userId := 0
	if raw := strings.TrimSpace(c.Query("user_id")); raw != "" {
		id, err := strconv.Atoi(raw)
		if err != nil || id <= 0 {
			common.ApiErrorMsg(c, "user_id 无效")
			return
		}
		userId = id
	}

	startTimestamp := int64(0)
	if raw := strings.TrimSpace(c.Query("start_timestamp")); raw != "" {
		ts, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || ts < 0 {
			common.ApiErrorMsg(c, "start_timestamp 无效")
			return
		}
		startTimestamp = ts
	}

	endTimestamp := int64(0)
	if raw := strings.TrimSpace(c.Query("end_timestamp")); raw != "" {
		ts, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || ts < 0 {
			common.ApiErrorMsg(c, "end_timestamp 无效")
			return
		}
		endTimestamp = ts
	}

	if startTimestamp > 0 && endTimestamp > 0 && startTimestamp > endTimestamp {
		common.ApiErrorMsg(c, "start_timestamp 不能大于 end_timestamp")
		return
	}

	if status != "" &&
		status != model.PayTokenOrderStatusPending &&
		status != model.PayTokenOrderStatusSuccess &&
		status != model.PayTokenOrderStatusFailed {
		common.ApiErrorMsg(c, "status 无效")
		return
	}

	if payMethod != "" &&
		payMethod != model.PayTokenPayMethodEpay &&
		payMethod != model.PayTokenPayMethodBalance {
		common.ApiErrorMsg(c, "pay_method 无效")
		return
	}

	items, total, err := model.ListAdminPayTokenOrders(model.AdminOrdersListQuery{
		Keyword:        keyword,
		Status:         status,
		PayMethod:      payMethod,
		UserId:         userId,
		StartTimestamp: startTimestamp,
		EndTimestamp:   endTimestamp,
	}, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}

	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(items)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    pageInfo,
	})
}

func AdminGetOrderDailyRevenueStats(c *gin.Context) {
	orderType := strings.TrimSpace(c.Query("order_type"))
	validTypes := map[string]bool{"": true, "subscription": true, "topup": true, "payg": true, "pay_request": true, "pay_token": true}
	if !validTypes[orderType] {
		common.ApiErrorMsg(c, "order_type 无效")
		return
	}

	period := strings.TrimSpace(c.Query("period"))
	if period == "" {
		period = "day"
	}
	validPeriods := map[string]bool{"day": true, "week": true, "month": true, "year": true}
	if !validPeriods[period] {
		common.ApiErrorMsg(c, "period 无效")
		return
	}

	anchorDate := strings.TrimSpace(c.Query("date"))
	timeZone := strings.TrimSpace(c.Query("time_zone"))
	stats, err := model.GetOrderRevenueStats(orderType, period, anchorDate, timeZone)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    stats,
	})
}
