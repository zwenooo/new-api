package helper

import (
	"one-api/common"
	relaycommon "one-api/relay/common"

	"github.com/gin-gonic/gin"
)

func IsAdminUser(c *gin.Context) bool {
	return c.GetInt("role") >= common.RoleAdminUser
}

func DisplayModelName(c *gin.Context, info *relaycommon.RelayInfo) string {
	if IsAdminUser(c) {
		return info.UpstreamModelName
	}
	return info.OriginModelName
}
