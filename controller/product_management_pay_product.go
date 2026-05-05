package controller

import (
	"net/http"
	"one-api/common"
	"one-api/model"
	"strconv"

	"github.com/gin-gonic/gin"
)

type restorePayProductRevisionRequest struct {
	RevisionId int `json:"revision_id"`
}

func ListProductManagementPayProductRevisions(c *gin.Context) {
	productType := c.Param("type")
	productID, err := strconv.Atoi(c.Param("id"))
	if err != nil || productID <= 0 {
		common.ApiErrorMsg(c, "id 无效")
		return
	}

	var data any
	switch productType {
	case "payg":
		data, err = model.ListPaygProductRevisions(productID)
	case "pay_request":
		data, err = model.ListPayRequestProductRevisions(productID)
	case "pay_token":
		data, err = model.ListPayTokenProductRevisions(productID)
	default:
		common.ApiErrorMsg(c, "商品类型无效")
		return
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    data,
	})
}

func RestoreProductManagementPayProductRevision(c *gin.Context) {
	productType := c.Param("type")
	productID, err := strconv.Atoi(c.Param("id"))
	if err != nil || productID <= 0 {
		common.ApiErrorMsg(c, "id 无效")
		return
	}
	var req restorePayProductRevisionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	if req.RevisionId <= 0 {
		common.ApiErrorMsg(c, "revision_id 无效")
		return
	}

	var data any
	switch productType {
	case "payg":
		data, err = model.RestorePaygProductFromRevisionTx(nil, productID, req.RevisionId)
		if err == nil {
			err = model.SyncPaygProductsOptionFromDB()
		}
		if err == nil {
			err = model.SyncAllClawBoxRelayTokens()
		}
	case "pay_request":
		data, err = model.RestorePayRequestProductFromRevisionTx(nil, productID, req.RevisionId)
		if err == nil {
			err = model.SyncPayRequestProductsOptionFromDB()
		}
	case "pay_token":
		data, err = model.RestorePayTokenProductFromRevisionTx(nil, productID, req.RevisionId)
		if err == nil {
			err = model.SyncPayTokenProductsOptionFromDB()
		}
	default:
		common.ApiErrorMsg(c, "商品类型无效")
		return
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    data,
	})
}
