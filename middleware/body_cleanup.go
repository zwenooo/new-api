package middleware

import (
	"one-api/common"

	"github.com/gin-gonic/gin"
)

func BodyStorageCleanup() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer common.CleanupBodyStorage(c)
		c.Next()
	}
}
