package model

import (
	"errors"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func filterChannelIDsByUserAccess(c *gin.Context, channelIDs []int) ([]int, error) {
	if c == nil {
		return nil, errors.New("missing request context")
	}
	return channelIDs, nil
}

func applyAbilityUserAccessFilter(c *gin.Context, query *gorm.DB) (*gorm.DB, error) {
	if c == nil {
		return nil, errors.New("missing request context")
	}
	return query, nil
}
