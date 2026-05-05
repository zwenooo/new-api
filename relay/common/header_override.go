package common

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	appcommon "one-api/common"
	"one-api/constant"
)

var headerOverrideVarPattern = regexp.MustCompile(`\$\{([a-zA-Z0-9_]+)\}`)

func ResolveHeaderOverride(c *gin.Context, info *RelayInfo, override map[string]interface{}) (map[string]string, error) {
	if len(override) == 0 {
		return nil, nil
	}

	vars := buildHeaderOverrideVars(c, info)
	resolved := make(map[string]string, len(override))
	for key, rawValue := range override {
		value, ok := rawValue.(string)
		if !ok {
			return nil, fmt.Errorf("header override value for %s must be string", key)
		}
		rendered, err := replaceHeaderOverrideVars(value, vars)
		if err != nil {
			return nil, err
		}
		resolved[key] = rendered
	}
	return resolved, nil
}

func buildHeaderOverrideVars(c *gin.Context, info *RelayInfo) map[string]string {
	itoaNonZero := func(v int) string {
		if v <= 0 {
			return ""
		}
		return strconv.Itoa(v)
	}

	userGroupID := appcommon.GetContextKeyInt(c, constant.ContextKeyUserGroupId)
	usingGroupID := appcommon.GetContextKeyInt(c, constant.ContextKeyUsingGroupId)
	tokenGroupID := appcommon.GetContextKeyInt(c, constant.ContextKeyTokenGroupId)

	vars := map[string]string{
		"user_id":                     strconv.Itoa(appcommon.GetContextKeyInt(c, constant.ContextKeyUserId)),
		"username":                    appcommon.GetContextKeyString(c, constant.ContextKeyUserName),
		"user_email":                  appcommon.GetContextKeyString(c, constant.ContextKeyUserEmail),
		"user_group":                  itoaNonZero(userGroupID),
		"user_group_id":               itoaNonZero(userGroupID),
		"user_default_model_group_id": itoaNonZero(appcommon.GetContextKeyInt(c, constant.ContextKeyDefaultModelGroupId)),
		"using_group_id":              itoaNonZero(usingGroupID),
		"token_id":                    strconv.Itoa(appcommon.GetContextKeyInt(c, constant.ContextKeyTokenId)),
		"token_group":                 itoaNonZero(tokenGroupID), // legacy token primary model-group id
		"token_group_id":              itoaNonZero(tokenGroupID),
		"channel_id":                  strconv.Itoa(appcommon.GetContextKeyInt(c, constant.ContextKeyChannelId)),
		"channel_name":                appcommon.GetContextKeyString(c, constant.ContextKeyChannelName),
		"channel_type":                strconv.Itoa(appcommon.GetContextKeyInt(c, constant.ContextKeyChannelType)),
		"model":                       safeRelayInfoValue(info, func(i *RelayInfo) string { return i.UpstreamModelName }),
		"origin_model":                safeRelayInfoValue(info, func(i *RelayInfo) string { return i.OriginModelName }),
		"request_id":                  strings.TrimSpace(c.GetString(appcommon.RequestIdKey)),
		"client_ip":                   c.ClientIP(),
	}
	return vars
}

func replaceHeaderOverrideVars(value string, vars map[string]string) (string, error) {
	if !strings.Contains(value, "${") {
		return value, nil
	}

	matches := headerOverrideVarPattern.FindAllStringSubmatchIndex(value, -1)
	if len(matches) == 0 {
		return value, nil
	}

	var builder strings.Builder
	last := 0
	for _, match := range matches {
		builder.WriteString(value[last:match[0]])
		key := value[match[2]:match[3]]
		replacement, ok := vars[key]
		if !ok {
			return "", fmt.Errorf("unknown header override variable: %s", key)
		}
		builder.WriteString(replacement)
		last = match[1]
	}
	builder.WriteString(value[last:])
	return builder.String(), nil
}

func safeRelayInfoValue(info *RelayInfo, getter func(*RelayInfo) string) string {
	if info == nil {
		return ""
	}
	return getter(info)
}
