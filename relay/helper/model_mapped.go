package helper

import (
	"github.com/gin-gonic/gin"
	"one-api/dto"
	"one-api/model"
	"one-api/relay/common"
)

func ModelMappedHelper(c *gin.Context, info *common.RelayInfo, request dto.Request) error {
	// map model name
	modelMapping := c.GetString("model_mapping")
	if modelMapping != "" && modelMapping != "{}" {
		mappedModel, isMapped, err := model.ResolveChannelModelMapping(modelMapping, info.OriginModelName)
		if err != nil {
			return err
		}
		info.IsModelMapped = isMapped
		if isMapped {
			info.UpstreamModelName = mappedModel
		}
	}
	if request != nil {
		request.SetModelName(info.UpstreamModelName)
	}
	return nil
}
