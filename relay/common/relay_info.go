package common

import (
	"encoding/json"
	"errors"
	"fmt"
	"one-api/common"
	"one-api/constant"
	"one-api/dto"
	oai_cc "one-api/relay/compat/oai_cc"
	relayconstant "one-api/relay/constant"
	"one-api/setting/model_setting"
	"one-api/types"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type ThinkingContentInfo struct {
	IsFirstThinkingContent  bool
	SendLastThinkingContent bool
	HasSentThinkingContent  bool
}

const (
	LastMessageTypeNone     = "none"
	LastMessageTypeText     = "text"
	LastMessageTypeTools    = "tools"
	LastMessageTypeThinking = "thinking"
)

type ClaudeConvertInfo struct {
	LastMessagesType              string
	Index                         int
	Usage                         *dto.Usage
	FinishReason                  string
	Done                          bool
	ResponsesToolNameByOriginal   map[string]string
	ResponsesToolNameByNormalized map[string]string
}

type ProductQuotaAllocation struct {
	ProductId int `json:"product_id,omitempty"`
	Quota     int `json:"quota,omitempty"`
}

type SubscriptionUnitAllocation struct {
	SubscriptionId      int  `json:"subscription_id,omitempty"`
	GroupId             int  `json:"group_id,omitempty"`
	StatDate            int  `json:"stat_date,omitempty"`
	Amount              int  `json:"amount,omitempty"`
	UsesGroupDailyLimit bool `json:"uses_group_daily_limit,omitempty"`
}

type RerankerInfo struct {
	Documents       []any
	ReturnDocuments bool
}

type BuildInToolInfo struct {
	ToolName          string
	CallCount         int
	SearchContextSize string
}

type ResponsesUsageInfo struct {
	BuiltInTools map[string]*BuildInToolInfo
}

// NormalizeResponsesToolType canonicalizes tool types for OpenAI Responses API usage tracking.
//
// Background: the ecosystem uses multiple names for the same built-in web search tool
// (e.g. "web_search" vs legacy "web_search_preview"), while response stream items use
// the call type "web_search_call". We normalize to avoid key mismatches that can cause
// incorrect accounting or panics.
func NormalizeResponsesToolType(toolType string) string {
	toolType = strings.TrimSpace(toolType)
	if toolType == "" {
		return toolType
	}
	if toolType == dto.BuildInToolWebSearchPreview {
		return dto.BuildInToolWebSearch
	}
	// Accept "web_search" and potential variants like "web_search_*".
	if strings.HasPrefix(toolType, dto.BuildInToolWebSearch) {
		return dto.BuildInToolWebSearch
	}
	return toolType
}

// EnsureResponsesBuiltInTool returns (and if needed initializes) a built-in tool entry
// inside RelayInfo.ResponsesUsageInfo.BuiltInTools using the canonical tool type.
func EnsureResponsesBuiltInTool(info *RelayInfo, toolType string) *BuildInToolInfo {
	if info == nil {
		return nil
	}
	toolType = NormalizeResponsesToolType(toolType)
	if info.ResponsesUsageInfo == nil {
		info.ResponsesUsageInfo = &ResponsesUsageInfo{BuiltInTools: make(map[string]*BuildInToolInfo)}
	}
	if info.ResponsesUsageInfo.BuiltInTools == nil {
		info.ResponsesUsageInfo.BuiltInTools = make(map[string]*BuildInToolInfo)
	}

	// Migrate legacy keys to the canonical one (so call sites can always use the canonical key).
	if toolType == dto.BuildInToolWebSearch {
		if legacy, ok := info.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolWebSearchPreview]; ok && legacy != nil {
			delete(info.ResponsesUsageInfo.BuiltInTools, dto.BuildInToolWebSearchPreview)
			legacy.ToolName = toolType
			if strings.TrimSpace(legacy.SearchContextSize) == "" {
				legacy.SearchContextSize = "medium"
			}
			info.ResponsesUsageInfo.BuiltInTools[toolType] = legacy
			return legacy
		}
		// Also collapse any "web_search_*" variant key to the canonical key.
		for k, v := range info.ResponsesUsageInfo.BuiltInTools {
			if k == toolType || !strings.HasPrefix(k, dto.BuildInToolWebSearch) || v == nil {
				continue
			}
			delete(info.ResponsesUsageInfo.BuiltInTools, k)
			v.ToolName = toolType
			if strings.TrimSpace(v.SearchContextSize) == "" {
				v.SearchContextSize = "medium"
			}
			info.ResponsesUsageInfo.BuiltInTools[toolType] = v
			return v
		}
	}

	tool := info.ResponsesUsageInfo.BuiltInTools[toolType]
	if tool == nil {
		tool = &BuildInToolInfo{
			ToolName:  toolType,
			CallCount: 0,
		}
		if toolType == dto.BuildInToolWebSearch {
			tool.SearchContextSize = "medium"
		}
		info.ResponsesUsageInfo.BuiltInTools[toolType] = tool
	}
	if toolType == dto.BuildInToolWebSearch && strings.TrimSpace(tool.SearchContextSize) == "" {
		tool.SearchContextSize = "medium"
	}
	return tool
}

func IncrementResponsesBuiltInToolCall(info *RelayInfo, toolType string) {
	tool := EnsureResponsesBuiltInTool(info, toolType)
	if tool != nil {
		tool.CallCount++
	}
}

func PopulateResponsesUsageTools(info *RelayInfo, tools []map[string]any) {
	if info == nil {
		return
	}
	if info.ResponsesUsageInfo == nil {
		info.ResponsesUsageInfo = &ResponsesUsageInfo{
			BuiltInTools: make(map[string]*BuildInToolInfo),
		}
	} else if info.ResponsesUsageInfo.BuiltInTools == nil {
		info.ResponsesUsageInfo.BuiltInTools = make(map[string]*BuildInToolInfo)
	}

	for _, tool := range tools {
		if tool == nil {
			continue
		}
		rawToolType := common.Interface2String(tool["type"])
		toolType := NormalizeResponsesToolType(rawToolType)
		toolInfo := EnsureResponsesBuiltInTool(info, toolType)
		if toolInfo == nil {
			continue
		}
		switch toolType {
		case dto.BuildInToolWebSearch:
			searchContextSize := common.Interface2String(tool["search_context_size"])
			if strings.TrimSpace(searchContextSize) == "" {
				searchContextSize = "medium"
			}
			toolInfo.SearchContextSize = searchContextSize
		}
	}
}

type ChannelMeta struct {
	ChannelType          int
	ChannelId            int
	ChannelIsMultiKey    bool
	ChannelMultiKeyIndex int
	ChannelBaseUrl       string
	ApiType              int
	ApiVersion           string
	ApiKey               string
	Organization         string
	ChannelCreateTime    int64
	ParamOverride        map[string]interface{}
	HeadersOverride      map[string]interface{}
	ChannelSetting       dto.ChannelSettings
	ChannelOtherSettings dto.ChannelOtherSettings
	UpstreamModelName    string
	IsModelMapped        bool
	SupportStreamOptions bool // 是否支持流式选项
}

type RelayInfo struct {
	TokenId                    int
	TokenKey                   string
	UserId                     int
	UsingGroupId               int                          // 使用的分组
	QuotaBucket                string                       // 计费桶：subscription / payg / free（由上层选择）
	SubscriptionAllocations    []SubscriptionUnitAllocation // 订阅额度/订阅tokens：本次请求实际扣费的订阅分配（用于精确回滚/结算）
	PaygProductId              int                          // 按量付费：首个实际扣费的商品ID（兼容旧快照/日志）
	PaygProductAllocations     []ProductQuotaAllocation     // 按量付费：本次请求实际扣费的商品分配（用于精确回滚/结算）
	PayTokenProductId          int                          // 按token付费：首个实际扣费的商品ID（兼容旧快照/日志）
	PayTokenProductAllocations []ProductQuotaAllocation     // 按token付费：本次请求实际扣费的商品分配（用于精确回滚/结算）
	UserGroupId                int                          // 用户 audience/user-group
	TokenUnlimited             bool
	StartTime                  time.Time
	FirstResponseTime          time.Time
	isFirstResponse            bool
	//SendLastReasoningResponse bool
	IsStream                     bool
	IsGeminiBatchEmbedding       bool
	IsPlayground                 bool
	UsePrice                     bool
	RelayMode                    int
	OriginModelName              string
	RequestURLPath               string
	PromptTokens                 int
	ShouldIncludeUsage           bool
	DisablePing                  bool // 是否禁止向下游发送自定义 Ping
	ClientWs                     *websocket.Conn
	TargetWs                     *websocket.Conn
	InputAudioFormat             string
	OutputAudioFormat            string
	RealtimeTools                []dto.RealTimeTool
	IsFirstRequest               bool
	AudioUsage                   bool
	ReasoningEffort              string
	ServiceTier                  string
	UserSetting                  dto.UserSetting
	UserEmail                    string
	UserQuota                    int
	BaseMultiplier               float64
	RelayFormat                  types.RelayFormat
	SendResponseCount            int
	FinalPreConsumedQuota        int                      // 最终预消耗的配额
	FinalPreConsumedTokens       int                      // 最终预消耗的 tokens（tokens 计费桶）
	FinalPreConsumedRequests     int                      // 最终预消耗的次数（按请求次数订阅）
	RequestSubscriptionId        int                      // 按次订阅：实际预扣/扣费的订阅ID（用于失败返还）
	FinalPreConsumedPayRequests  int                      // 最终预消耗的次数（按次付费）
	PayRequestProductId          int                      // 按次付费：首个实际扣费的商品ID（兼容旧快照/日志）
	PayRequestProductAllocations []ProductQuotaAllocation // 按次付费：本次请求实际扣费的商品分配（用于精确回滚/结算）
	LogId                        int                      // 初始日志条目ID（用于流式请求实时显示和后续更新）
	LogCreatedAt                 int64                    // 初始日志条目创建时间（Unix seconds，用于服务状态聚合）
	ConversationId               string
	SessionId                    string
	PromptCacheKey               string
	OaiCcUsage                   *oai_cc.UsageContext
	IsClaudeBetaQuery            bool // /v1/messages?beta=true

	PriceData types.PriceData

	Request dto.Request

	ThinkingContentInfo
	*ClaudeConvertInfo
	*RerankerInfo
	*ResponsesUsageInfo
	*ChannelMeta
	*TaskRelayInfo
}

func (info *RelayInfo) InitChannelMeta(c *gin.Context) {
	channelType := common.GetContextKeyInt(c, constant.ContextKeyChannelType)
	paramOverride := common.GetContextKeyStringMap(c, constant.ContextKeyChannelParamOverride)
	headerOverride := common.GetContextKeyStringMap(c, constant.ContextKeyChannelHeaderOverride)
	apiType, _ := common.ChannelType2APIType(channelType)
	channelMeta := &ChannelMeta{
		ChannelType:          channelType,
		ChannelId:            common.GetContextKeyInt(c, constant.ContextKeyChannelId),
		ChannelIsMultiKey:    common.GetContextKeyBool(c, constant.ContextKeyChannelIsMultiKey),
		ChannelMultiKeyIndex: common.GetContextKeyInt(c, constant.ContextKeyChannelMultiKeyIndex),
		ChannelBaseUrl:       common.GetContextKeyString(c, constant.ContextKeyChannelBaseUrl),
		ApiType:              apiType,
		ApiVersion:           c.GetString("api_version"),
		ApiKey:               common.GetContextKeyString(c, constant.ContextKeyChannelKey),
		Organization:         c.GetString("channel_organization"),
		ChannelCreateTime:    c.GetInt64("channel_create_time"),
		ParamOverride:        paramOverride,
		HeadersOverride:      headerOverride,
		UpstreamModelName:    common.GetContextKeyString(c, constant.ContextKeyOriginalModel),
		IsModelMapped:        false,
		SupportStreamOptions: false,
	}

	if channelType == constant.ChannelTypeAzure {
		channelMeta.ApiVersion = GetAPIVersion(c)
	}
	if channelType == constant.ChannelTypeVertexAi {
		channelMeta.ApiVersion = c.GetString("region")
	}

	channelSetting, ok := common.GetContextKeyType[dto.ChannelSettings](c, constant.ContextKeyChannelSetting)
	if ok {
		channelMeta.ChannelSetting = channelSetting
	}

	channelOtherSettings, ok := common.GetContextKeyType[dto.ChannelOtherSettings](c, constant.ContextKeyChannelOtherSetting)
	if ok {
		channelMeta.ChannelOtherSettings = channelOtherSettings
	}

	if streamSupportedChannels[channelMeta.ChannelType] {
		channelMeta.SupportStreamOptions = true
	}

	info.ChannelMeta = channelMeta

	// reset some fields based on channel meta
	// 重置某些字段，例如模型名称等
	if info.Request != nil {
		info.Request.SetModelName(info.OriginModelName)
	}
}

func (info *RelayInfo) ToString() string {
	if info == nil {
		return "RelayInfo<nil>"
	}

	// Basic info
	b := &strings.Builder{}
	fmt.Fprintf(b, "RelayInfo{ ")
	fmt.Fprintf(b, "RelayFormat: %s, ", info.RelayFormat)
	fmt.Fprintf(b, "RelayMode: %d, ", info.RelayMode)
	fmt.Fprintf(b, "IsStream: %t, ", info.IsStream)
	fmt.Fprintf(b, "IsPlayground: %t, ", info.IsPlayground)
	fmt.Fprintf(b, "RequestURLPath: %q, ", info.RequestURLPath)
	fmt.Fprintf(b, "OriginModelName: %q, ", info.OriginModelName)
	fmt.Fprintf(b, "PromptTokens: %d, ", info.PromptTokens)
	fmt.Fprintf(b, "ShouldIncludeUsage: %t, ", info.ShouldIncludeUsage)
	fmt.Fprintf(b, "DisablePing: %t, ", info.DisablePing)
	fmt.Fprintf(b, "SendResponseCount: %d, ", info.SendResponseCount)
	fmt.Fprintf(b, "FinalPreConsumedQuota: %d, ", info.FinalPreConsumedQuota)

	// User & token info (mask secrets)
	fmt.Fprintf(b, "User{ Id: %d, Email: %q, GroupId: %d, UsingGroupId: %d, Quota: %d }, ",
		info.UserId, common.MaskEmail(info.UserEmail), info.UserGroupId, info.UsingGroupId, info.UserQuota)
	fmt.Fprintf(b, "Token{ Id: %d, Unlimited: %t, Key: ***masked*** }, ", info.TokenId, info.TokenUnlimited)

	// Time info
	latencyMs := info.FirstResponseTime.Sub(info.StartTime).Milliseconds()
	fmt.Fprintf(b, "Timing{ Start: %s, FirstResponse: %s, LatencyMs: %d }, ",
		info.StartTime.Format(time.RFC3339Nano), info.FirstResponseTime.Format(time.RFC3339Nano), latencyMs)

	// Audio / realtime
	if info.InputAudioFormat != "" || info.OutputAudioFormat != "" || len(info.RealtimeTools) > 0 || info.AudioUsage {
		fmt.Fprintf(b, "Realtime{ AudioUsage: %t, InFmt: %q, OutFmt: %q, Tools: %d }, ",
			info.AudioUsage, info.InputAudioFormat, info.OutputAudioFormat, len(info.RealtimeTools))
	}

	// Reasoning
	if info.ReasoningEffort != "" {
		fmt.Fprintf(b, "ReasoningEffort: %q, ", info.ReasoningEffort)
	}

	// Price data (non-sensitive)
	if info.PriceData.UsePrice {
		fmt.Fprintf(b, "PriceData{ %s }, ", info.PriceData.ToSetting())
	}

	// Channel metadata (mask ApiKey)
	if info.ChannelMeta != nil {
		cm := info.ChannelMeta
		fmt.Fprintf(b, "ChannelMeta{ Type: %d, Id: %d, IsMultiKey: %t, MultiKeyIndex: %d, BaseURL: %q, ApiType: %d, ApiVersion: %q, Organization: %q, CreateTime: %d, UpstreamModelName: %q, IsModelMapped: %t, SupportStreamOptions: %t, ApiKey: ***masked*** }, ",
			cm.ChannelType, cm.ChannelId, cm.ChannelIsMultiKey, cm.ChannelMultiKeyIndex, cm.ChannelBaseUrl, cm.ApiType, cm.ApiVersion, cm.Organization, cm.ChannelCreateTime, cm.UpstreamModelName, cm.IsModelMapped, cm.SupportStreamOptions)
	}

	// Responses usage info (non-sensitive)
	if info.ResponsesUsageInfo != nil && len(info.ResponsesUsageInfo.BuiltInTools) > 0 {
		fmt.Fprintf(b, "ResponsesTools{ ")
		first := true
		for name, tool := range info.ResponsesUsageInfo.BuiltInTools {
			if !first {
				fmt.Fprintf(b, ", ")
			}
			first = false
			if tool != nil {
				fmt.Fprintf(b, "%s: calls=%d", name, tool.CallCount)
			} else {
				fmt.Fprintf(b, "%s: calls=0", name)
			}
		}
		fmt.Fprintf(b, " }, ")
	}

	fmt.Fprintf(b, "}")
	return b.String()
}

// 定义支持流式选项的通道类型
var streamSupportedChannels = map[int]bool{
	constant.ChannelTypeOpenAI:     true,
	constant.ChannelTypeAnthropic:  true,
	constant.ChannelTypeAws:        true,
	constant.ChannelTypeGemini:     true,
	constant.ChannelCloudflare:     true,
	constant.ChannelTypeAzure:      true,
	constant.ChannelTypeVolcEngine: true,
	constant.ChannelTypeOllama:     true,
	constant.ChannelTypeXai:        true,
	constant.ChannelTypeDeepSeek:   true,
	constant.ChannelTypeBaiduV2:    true,
}

func GenRelayInfoWs(c *gin.Context, ws *websocket.Conn) *RelayInfo {
	info := genBaseRelayInfo(c, nil)
	info.RelayFormat = types.RelayFormatOpenAIRealtime
	info.ClientWs = ws
	info.InputAudioFormat = "pcm16"
	info.OutputAudioFormat = "pcm16"
	info.IsFirstRequest = true
	return info
}

func GenRelayInfoClaude(c *gin.Context, request dto.Request) *RelayInfo {
	info := genBaseRelayInfo(c, request)
	info.RelayFormat = types.RelayFormatClaude
	info.ShouldIncludeUsage = false
	info.ClaudeConvertInfo = &ClaudeConvertInfo{
		LastMessagesType: LastMessageTypeNone,
	}
	info.IsClaudeBetaQuery = c.Query("beta") == "true" || isClaudeBetaForced(c)
	return info
}

func isClaudeBetaForced(c *gin.Context) bool {
	channelOtherSettings, ok := common.GetContextKeyType[dto.ChannelOtherSettings](c, constant.ContextKeyChannelOtherSetting)
	return ok && channelOtherSettings.ClaudeBetaQuery
}

func GenRelayInfoRerank(c *gin.Context, request *dto.RerankRequest) *RelayInfo {
	info := genBaseRelayInfo(c, request)
	info.RelayMode = relayconstant.RelayModeRerank
	info.RelayFormat = types.RelayFormatRerank
	info.RerankerInfo = &RerankerInfo{
		Documents:       request.Documents,
		ReturnDocuments: request.GetReturnDocuments(),
	}
	return info
}

func GenRelayInfoOpenAIAudio(c *gin.Context, request dto.Request) *RelayInfo {
	info := genBaseRelayInfo(c, request)
	info.RelayFormat = types.RelayFormatOpenAIAudio
	return info
}

func GenRelayInfoEmbedding(c *gin.Context, request dto.Request) *RelayInfo {
	info := genBaseRelayInfo(c, request)
	info.RelayFormat = types.RelayFormatEmbedding
	return info
}

func GenRelayInfoResponses(c *gin.Context, request *dto.OpenAIResponsesRequest, ws *websocket.Conn) *RelayInfo {
	info := genBaseRelayInfo(c, request)
	info.RelayMode = relayconstant.RelayModeResponses
	info.RelayFormat = types.RelayFormatOpenAIResponses
	info.ClientWs = ws
	info.ServiceTier = NormalizeServiceTier(request.ServiceTier)
	PopulateResponsesUsageTools(info, request.GetToolsMap())
	return info
}

func GenRelayInfoGemini(c *gin.Context, request dto.Request) *RelayInfo {
	info := genBaseRelayInfo(c, request)
	info.RelayFormat = types.RelayFormatGemini
	info.ShouldIncludeUsage = false

	return info
}

func GenRelayInfoImage(c *gin.Context, request dto.Request) *RelayInfo {
	info := genBaseRelayInfo(c, request)
	info.RelayFormat = types.RelayFormatOpenAIImage
	return info
}

func GenRelayInfoOpenAI(c *gin.Context, request dto.Request) *RelayInfo {
	info := genBaseRelayInfo(c, request)
	info.RelayFormat = types.RelayFormatOpenAI
	return info
}

func genBaseRelayInfo(c *gin.Context, request dto.Request) *RelayInfo {

	//channelType := common.GetContextKeyInt(c, constant.ContextKeyChannelType)
	//channelId := common.GetContextKeyInt(c, constant.ContextKeyChannelId)
	//paramOverride := common.GetContextKeyStringMap(c, constant.ContextKeyChannelParamOverride)

	startTime := common.GetContextKeyTime(c, constant.ContextKeyRequestStartTime)
	if startTime.IsZero() {
		startTime = time.Now()
	}

	isStream := false

	if request != nil {
		isStream = request.IsStream(c)
	}

	// firstResponseTime = time.Now() - 1 second

	info := &RelayInfo{
		Request: request,

		UserId:       common.GetContextKeyInt(c, constant.ContextKeyUserId),
		UsingGroupId: common.GetContextKeyInt(c, constant.ContextKeyUsingGroupId),
		UserGroupId:  common.GetContextKeyInt(c, constant.ContextKeyUserGroupId),
		UserQuota:    common.GetContextKeyInt(c, constant.ContextKeyUserQuota),
		UserEmail:    common.GetContextKeyString(c, constant.ContextKeyUserEmail),

		OriginModelName: common.GetContextKeyString(c, constant.ContextKeyOriginalModel),
		PromptTokens:    common.GetContextKeyInt(c, constant.ContextKeyPromptTokens),

		TokenId:        common.GetContextKeyInt(c, constant.ContextKeyTokenId),
		TokenKey:       common.GetContextKeyString(c, constant.ContextKeyTokenKey),
		TokenUnlimited: common.GetContextKeyBool(c, constant.ContextKeyTokenUnlimited),

		isFirstResponse: true,
		RelayMode:       relayconstant.Path2RelayMode(c.Request.URL.Path),
		RequestURLPath:  c.Request.URL.String(),
		IsStream:        isStream,

		StartTime:         startTime,
		FirstResponseTime: startTime.Add(-time.Second),
		ThinkingContentInfo: ThinkingContentInfo{
			IsFirstThinkingContent:  true,
			SendLastThinkingContent: false,
		},
	}

	if info.RelayMode == relayconstant.RelayModeUnknown {
		info.RelayMode = c.GetInt("relay_mode")
	}

	if strings.HasPrefix(c.Request.URL.Path, "/pg") {
		info.IsPlayground = true
		info.RequestURLPath = strings.TrimPrefix(info.RequestURLPath, "/pg")
		info.RequestURLPath = "/v1" + info.RequestURLPath
	}

	userSetting, ok := common.GetContextKeyType[dto.UserSetting](c, constant.ContextKeyUserSetting)
	if ok {
		info.UserSetting = userSetting
	}

	baseMultiplier := common.GetContextKeyFloat64(c, constant.ContextKeyUserBaseMultiplier)
	if baseMultiplier <= 0 {
		baseMultiplier = 1
	}
	info.BaseMultiplier = baseMultiplier

	return info
}

// RemoveDisabledFields 从请求 JSON 数据中移除渠道设置中禁用的字段
// service_tier: 服务层级字段，可能导致额外计费（OpenAI、Claude、Responses API 支持）
// inference_geo: Claude 数据驻留推理区域字段（仅 Claude 支持，默认过滤）
// store: 数据存储授权字段，涉及用户隐私（仅 OpenAI、Responses API 支持，默认允许透传，禁用后可能导致 Codex 无法使用）
// safety_identifier: 安全标识符，用于向 OpenAI 报告违规用户（仅 OpenAI 支持，涉及用户隐私）
// stream_options.include_obfuscation: 响应流混淆控制字段（仅 OpenAI Responses API 支持）
func RemoveDisabledFields(jsonData []byte, channelOtherSettings dto.ChannelOtherSettings, channelPassThroughEnabled bool) ([]byte, error) {
	if model_setting.GetGlobalSettings().PassThroughRequestEnabled || channelPassThroughEnabled {
		return jsonData, nil
	}

	var data map[string]interface{}
	if err := common.Unmarshal(jsonData, &data); err != nil {
		common.SysError("RemoveDisabledFields Unmarshal error :" + err.Error())
		return jsonData, nil
	}

	if !channelOtherSettings.AllowServiceTier {
		delete(data, "service_tier")
	}

	if !channelOtherSettings.AllowInferenceGeo {
		delete(data, "inference_geo")
	}

	if channelOtherSettings.DisableStore {
		delete(data, "store")
	}

	if !channelOtherSettings.AllowSafetyIdentifier {
		delete(data, "safety_identifier")
	}

	if !channelOtherSettings.AllowIncludeObfuscation {
		if streamOptionsAny, exists := data["stream_options"]; exists {
			if streamOptions, ok := streamOptionsAny.(map[string]interface{}); ok {
				delete(streamOptions, "include_obfuscation")
				if len(streamOptions) == 0 {
					delete(data, "stream_options")
				} else {
					data["stream_options"] = streamOptions
				}
			}
		}
	}

	jsonDataAfter, err := common.Marshal(data)
	if err != nil {
		common.SysError("RemoveDisabledFields Marshal error :" + err.Error())
		return jsonData, nil
	}
	return jsonDataAfter, nil
}

func GenRelayInfo(c *gin.Context, relayFormat types.RelayFormat, request dto.Request, ws *websocket.Conn) (*RelayInfo, error) {
	switch relayFormat {
	case types.RelayFormatOpenAI:
		return GenRelayInfoOpenAI(c, request), nil
	case types.RelayFormatOpenAIAudio:
		return GenRelayInfoOpenAIAudio(c, request), nil
	case types.RelayFormatOpenAIImage:
		return GenRelayInfoImage(c, request), nil
	case types.RelayFormatOpenAIRealtime:
		return GenRelayInfoWs(c, ws), nil
	case types.RelayFormatClaude:
		return GenRelayInfoClaude(c, request), nil
	case types.RelayFormatRerank:
		if request, ok := request.(*dto.RerankRequest); ok {
			return GenRelayInfoRerank(c, request), nil
		}
		return nil, errors.New("request is not a RerankRequest")
	case types.RelayFormatGemini:
		return GenRelayInfoGemini(c, request), nil
	case types.RelayFormatEmbedding:
		return GenRelayInfoEmbedding(c, request), nil
	case types.RelayFormatOpenAIResponses:
		if request, ok := request.(*dto.OpenAIResponsesRequest); ok {
			return GenRelayInfoResponses(c, request, ws), nil
		}
		return nil, errors.New("request is not a OpenAIResponsesRequest")
	case types.RelayFormatTask:
		return genBaseRelayInfo(c, nil), nil
	case types.RelayFormatMjProxy:
		return genBaseRelayInfo(c, nil), nil
	default:
		return nil, errors.New("invalid relay format")
	}
}

func (info *RelayInfo) SetPromptTokens(promptTokens int) {
	info.PromptTokens = promptTokens
}

func (info *RelayInfo) SetFirstResponseTime() {
	if info.isFirstResponse {
		info.FirstResponseTime = time.Now()
		info.isFirstResponse = false
	}
}

func (info *RelayInfo) HasSendResponse() bool {
	return info.FirstResponseTime.After(info.StartTime)
}

type TaskRelayInfo struct {
	Action               string
	OriginTaskID         string
	PublicTaskID         string
	OriginUpstreamTaskID string

	ConsumeQuota  bool
	LockedChannel any
}

type TaskSubmitReq struct {
	Prompt         string                 `json:"prompt"`
	Model          string                 `json:"model,omitempty"`
	Mode           string                 `json:"mode,omitempty"`
	Image          string                 `json:"image,omitempty"`
	Images         []string               `json:"images,omitempty"`
	Size           string                 `json:"size,omitempty"`
	Duration       int                    `json:"duration,omitempty"`
	Seconds        string                 `json:"seconds,omitempty"`
	InputReference string                 `json:"input_reference,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
}

func (t *TaskSubmitReq) GetPrompt() string {
	return t.Prompt
}

func (t *TaskSubmitReq) HasImage() bool {
	return len(t.Images) > 0
}

func (t *TaskSubmitReq) UnmarshalJSON(data []byte) error {
	type Alias TaskSubmitReq
	aux := &struct {
		Metadata json.RawMessage `json:"metadata,omitempty"`
		*Alias
	}{
		Alias: (*Alias)(t),
	}

	if err := common.Unmarshal(data, &aux); err != nil {
		return err
	}

	if len(aux.Metadata) > 0 {
		var metadataStr string
		if err := common.Unmarshal(aux.Metadata, &metadataStr); err == nil && metadataStr != "" {
			var metadataObj map[string]interface{}
			if err := common.Unmarshal([]byte(metadataStr), &metadataObj); err == nil {
				t.Metadata = metadataObj
				return nil
			}
		}

		var metadataObj map[string]interface{}
		if err := common.Unmarshal(aux.Metadata, &metadataObj); err == nil {
			t.Metadata = metadataObj
		}
	}

	return nil
}

func (t *TaskSubmitReq) UnmarshalMetadata(v any) error {
	if t.Metadata == nil {
		return nil
	}
	metadataBytes, err := common.Marshal(t.Metadata)
	if err != nil {
		return fmt.Errorf("marshal metadata failed: %w", err)
	}
	if err := common.Unmarshal(metadataBytes, v); err != nil {
		return fmt.Errorf("unmarshal metadata to target failed: %w", err)
	}
	return nil
}

type TaskInfo struct {
	Code             int    `json:"code"`
	TaskID           string `json:"task_id"`
	Status           string `json:"status"`
	Reason           string `json:"reason,omitempty"`
	Url              string `json:"url,omitempty"`
	RemoteUrl        string `json:"remote_url,omitempty"`
	Progress         string `json:"progress,omitempty"`
	CompletionTokens int    `json:"completion_tokens,omitempty"`
	TotalTokens      int    `json:"total_tokens,omitempty"`
}

func FailTaskInfo(reason string) *TaskInfo {
	return &TaskInfo{
		Status: "FAILURE",
		Reason: reason,
	}
}
