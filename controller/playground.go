package controller

import (
	"one-api/common"
	"one-api/constant"
	"one-api/types"
	"time"

	"github.com/gin-gonic/gin"
)

func Playground(c *gin.Context) {
	common.SetContextKey(c, constant.ContextKeyRequestStartTime, time.Now())
	Relay(c, types.RelayFormatOpenAI)
}
