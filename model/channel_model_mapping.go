package model

import (
	"encoding/json"
	"errors"
	"one-api/constant"
	"strings"
)

var (
	ErrChannelModelMappingUnmarshal = errors.New("unmarshal_model_mapping_failed")
	ErrChannelModelMappingCycle     = errors.New("model_mapping_contains_cycle")
)

func ResolveChannelModelMapping(modelMapping string, requestedModel string) (string, bool, error) {
	modelMapping = strings.TrimSpace(modelMapping)
	requestedModel = strings.TrimSpace(requestedModel)
	if modelMapping == "" || modelMapping == "{}" || requestedModel == "" {
		return requestedModel, false, nil
	}

	modelMap := make(map[string]string)
	if err := json.Unmarshal([]byte(modelMapping), &modelMap); err != nil {
		return "", false, ErrChannelModelMappingUnmarshal
	}

	currentModel := requestedModel
	visitedModels := map[string]bool{
		currentModel: true,
	}
	for {
		mappedModel, exists := modelMap[currentModel]
		mappedModel = strings.TrimSpace(mappedModel)
		if !exists || mappedModel == "" {
			break
		}
		if visitedModels[mappedModel] {
			if mappedModel == currentModel {
				if currentModel == requestedModel {
					return requestedModel, false, nil
				}
				return currentModel, true, nil
			}
			return "", false, ErrChannelModelMappingCycle
		}
		visitedModels[mappedModel] = true
		currentModel = mappedModel
	}

	if currentModel == requestedModel {
		return requestedModel, false, nil
	}
	return currentModel, true, nil
}

func ChannelSupportsMessagesToResponsesCompat(channelType int) bool {
	switch channelType {
	case constant.ChannelTypeOpenAI,
		constant.ChannelTypeAzure,
		constant.ChannelTypeCustom,
		constant.ChannelTypeOpenRouter,
		constant.ChannelTypeXinference:
		return true
	default:
		return false
	}
}
