package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"one-api/common"
	"one-api/logger"
	"strconv"
	"strings"
	"sync"
	"time"

	"one-api/dto"
	"one-api/model"
	"one-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
)

const (
	defaultTimeoutSeconds = 10
	defaultEndpoint       = "/api/ratio_config"
	officialPresetPath    = "/llm-metadata/api/newapi/ratio_config-v1-base.json"
	openRouterModelsPath  = "/api/v1/models"
	openRouterTokenOption = "OpenRouterPriceSyncToken"
	maxConcurrentFetches  = 8
	maxRatioConfigBytes   = 10 << 20 // 10MB
	floatEpsilon          = 1e-9
)

const (
	syncSourceTypeGeneric        = ""
	syncSourceTypeOfficialPreset = "official_ratio_preset"
	syncSourceTypeOpenRouter     = "openrouter_models"
)

func nearlyEqual(a, b float64) bool {
	if a > b {
		return a-b < floatEpsilon
	}
	return b-a < floatEpsilon
}

func valuesEqual(a, b interface{}) bool {
	af, aok := a.(float64)
	bf, bok := b.(float64)
	if aok && bok {
		return nearlyEqual(af, bf)
	}
	return a == b
}

var ratioTypes = []string{"model_ratio", "completion_ratio", "cache_ratio", "create_cache_ratio", "model_price"}

type upstreamResult struct {
	Name string         `json:"name"`
	Data map[string]any `json:"data,omitempty"`
	Err  string         `json:"err,omitempty"`
}

type openRouterModelsResponse struct {
	Data []openRouterModel `json:"data"`
}

type openRouterModel struct {
	ID           string                 `json:"id"`
	Architecture openRouterArchitecture `json:"architecture"`
	Pricing      openRouterPricing      `json:"pricing"`
}

type openRouterArchitecture struct {
	OutputModalities []string `json:"output_modalities"`
}

type openRouterPricing struct {
	Prompt          string `json:"prompt"`
	Completion      string `json:"completion"`
	Request         string `json:"request"`
	InputCacheRead  string `json:"input_cache_read"`
	InputCacheWrite string `json:"input_cache_write"`
}

func normalizeSyncSourceType(sourceType string) string {
	return strings.TrimSpace(sourceType)
}

func getOpenRouterPriceSyncToken() string {
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	return strings.TrimSpace(common.Interface2String(common.OptionMap[openRouterTokenOption]))
}

func resolveUpstreamBearerToken(chItem dto.UpstreamDTO) string {
	token := strings.TrimSpace(chItem.BearerToken)
	if token != "" {
		return token
	}
	if normalizeSyncSourceType(chItem.SourceType) == syncSourceTypeOpenRouter {
		return getOpenRouterPriceSyncToken()
	}
	return ""
}

func defaultSyncEndpointForSource(sourceType string) string {
	switch normalizeSyncSourceType(sourceType) {
	case syncSourceTypeOfficialPreset:
		return officialPresetPath
	case syncSourceTypeOpenRouter:
		return openRouterModelsPath
	default:
		return defaultEndpoint
	}
}

func roundRatioValue(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return v
	}
	return math.Round(v*1e12) / 1e12
}

func parseOpenRouterPriceValue(raw string) (float64, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, false
	}
	return value, true
}

func openRouterModelSupportsText(model openRouterModel) bool {
	if len(model.Architecture.OutputModalities) == 0 {
		return true
	}
	for _, modality := range model.Architecture.OutputModalities {
		if strings.EqualFold(strings.TrimSpace(modality), "text") {
			return true
		}
	}
	return false
}

func normalizeOpenRouterModelName(raw string) string {
	name := strings.TrimSpace(raw)
	if idx := strings.Index(name, "/"); idx >= 0 {
		name = name[idx+1:]
	}
	if strings.HasPrefix(name, "claude-") {
		name = strings.ReplaceAll(name, ".", "-")
	}
	return strings.TrimSpace(name)
}

func defaultCompletionRatioForSync(modelName string) float64 {
	return roundRatioValue(ratio_setting.GetCompletionRatio(modelName))
}

func defaultCacheRatioForSync(modelName string) float64 {
	ratio, _ := ratio_setting.GetCacheRatio(modelName)
	return roundRatioValue(ratio)
}

func defaultCreateCacheRatioForSync(modelName string) float64 {
	ratio, _ := ratio_setting.GetCreateCacheRatio(modelName)
	return roundRatioValue(ratio)
}

func convertOpenRouterModelsResponse(body []byte) (map[string]any, error) {
	var payload openRouterModelsResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}

	modelRatioMap := make(map[string]any)
	completionRatioMap := make(map[string]any)
	cacheRatioMap := make(map[string]any)
	createCacheRatioMap := make(map[string]any)
	modelPriceMap := make(map[string]any)

	for _, item := range payload.Data {
		modelName := normalizeOpenRouterModelName(item.ID)
		if modelName == "" || !openRouterModelSupportsText(item) {
			continue
		}

		promptPrice, hasPromptPrice := parseOpenRouterPriceValue(item.Pricing.Prompt)
		requestPrice, hasRequestPrice := parseOpenRouterPriceValue(item.Pricing.Request)
		if hasPromptPrice && promptPrice > 0 {
			modelRatioMap[modelName] = roundRatioValue(promptPrice * common.QuotaPerUnit)

			if completionPrice, ok := parseOpenRouterPriceValue(item.Pricing.Completion); ok && completionPrice >= 0 {
				if promptPrice > 0 {
					completionRatioMap[modelName] = roundRatioValue(completionPrice / promptPrice)
				} else if completionPrice == 0 {
					completionRatioMap[modelName] = roundRatioValue(completionPrice)
				}
			} else {
				completionRatioMap[modelName] = defaultCompletionRatioForSync(modelName)
			}

			if cacheReadPrice, ok := parseOpenRouterPriceValue(item.Pricing.InputCacheRead); ok && cacheReadPrice >= 0 {
				if promptPrice > 0 {
					cacheRatioMap[modelName] = roundRatioValue(cacheReadPrice / promptPrice)
				} else if cacheReadPrice == 0 {
					cacheRatioMap[modelName] = roundRatioValue(cacheReadPrice)
				}
			} else {
				cacheRatioMap[modelName] = defaultCacheRatioForSync(modelName)
			}

			if cacheWritePrice, ok := parseOpenRouterPriceValue(item.Pricing.InputCacheWrite); ok && cacheWritePrice >= 0 {
				if promptPrice > 0 {
					createCacheRatioMap[modelName] = roundRatioValue(cacheWritePrice / promptPrice)
				} else if cacheWritePrice == 0 {
					createCacheRatioMap[modelName] = roundRatioValue(cacheWritePrice)
				}
			} else {
				createCacheRatioMap[modelName] = defaultCreateCacheRatioForSync(modelName)
			}
			continue
		}

		if hasRequestPrice && requestPrice >= 0 {
			modelPriceMap[modelName] = roundRatioValue(requestPrice)
			continue
		}

		if hasPromptPrice && promptPrice == 0 {
			modelRatioMap[modelName] = 0.0
			completionRatioMap[modelName] = 0.0
			cacheRatioMap[modelName] = 0.0
			createCacheRatioMap[modelName] = 0.0
		}
	}

	converted := make(map[string]any)
	if len(modelRatioMap) > 0 {
		converted["model_ratio"] = modelRatioMap
	}
	if len(completionRatioMap) > 0 {
		converted["completion_ratio"] = completionRatioMap
	}
	if len(cacheRatioMap) > 0 {
		converted["cache_ratio"] = cacheRatioMap
	}
	if len(createCacheRatioMap) > 0 {
		converted["create_cache_ratio"] = createCacheRatioMap
	}
	if len(modelPriceMap) > 0 {
		converted["model_price"] = modelPriceMap
	}
	return converted, nil
}

func FetchUpstreamRatios(c *gin.Context) {
	var req dto.UpstreamRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}

	if req.Timeout <= 0 {
		req.Timeout = defaultTimeoutSeconds
	}

	var upstreams []dto.UpstreamDTO

	if len(req.Upstreams) > 0 {
		for _, u := range req.Upstreams {
			if strings.HasPrefix(u.BaseURL, "http") {
				if u.Endpoint == "" {
					u.Endpoint = defaultSyncEndpointForSource(u.SourceType)
				}
				u.BaseURL = strings.TrimRight(u.BaseURL, "/")
				upstreams = append(upstreams, u)
			}
		}
	} else if len(req.ChannelIDs) > 0 {
		intIds := make([]int, 0, len(req.ChannelIDs))
		for _, id64 := range req.ChannelIDs {
			intIds = append(intIds, int(id64))
		}
		dbChannels, err := model.GetChannelsByIds(intIds)
		if err != nil {
			logger.LogError(c.Request.Context(), "failed to query channels: "+err.Error())
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "查询渠道失败"})
			return
		}
		for _, ch := range dbChannels {
			if base := ch.GetBaseURL(); strings.HasPrefix(base, "http") {
				upstreams = append(upstreams, dto.UpstreamDTO{
					ID:         ch.Id,
					Name:       ch.Name,
					BaseURL:    strings.TrimRight(base, "/"),
					Endpoint:   "",
					SourceType: syncSourceTypeGeneric,
				})
			}
		}
	}

	if len(upstreams) == 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "无有效上游渠道"})
		return
	}

	var wg sync.WaitGroup
	ch := make(chan upstreamResult, len(upstreams))

	sem := make(chan struct{}, maxConcurrentFetches)

	dialer := &net.Dialer{Timeout: 10 * time.Second}
	transport := &http.Transport{MaxIdleConns: 100, IdleConnTimeout: 90 * time.Second, TLSHandshakeTimeout: 10 * time.Second, ExpectContinueTimeout: 1 * time.Second, ResponseHeaderTimeout: 10 * time.Second}
	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			host = addr
		}
		// 对 github.io 优先尝试 IPv4，失败则回退 IPv6
		if strings.HasSuffix(host, "github.io") {
			if conn, err := dialer.DialContext(ctx, "tcp4", addr); err == nil {
				return conn, nil
			}
			return dialer.DialContext(ctx, "tcp6", addr)
		}
		return dialer.DialContext(ctx, network, addr)
	}
	client := &http.Client{Transport: transport}

	for _, chn := range upstreams {
		wg.Add(1)
		go func(chItem dto.UpstreamDTO) {
			defer wg.Done()

			sem <- struct{}{}
			defer func() { <-sem }()

			endpoint := chItem.Endpoint
			var fullURL string
			if strings.HasPrefix(endpoint, "http://") || strings.HasPrefix(endpoint, "https://") {
				fullURL = endpoint
			} else {
				if endpoint == "" {
					endpoint = defaultSyncEndpointForSource(chItem.SourceType)
				} else if !strings.HasPrefix(endpoint, "/") {
					endpoint = "/" + endpoint
				}
				fullURL = chItem.BaseURL + endpoint
			}

			uniqueName := chItem.Name
			if chItem.ID != 0 {
				uniqueName = fmt.Sprintf("%s(%d)", chItem.Name, chItem.ID)
			}

			bearerToken := resolveUpstreamBearerToken(chItem)

			if normalizeSyncSourceType(chItem.SourceType) == syncSourceTypeOpenRouter && bearerToken == "" {
				ch <- upstreamResult{Name: uniqueName, Err: "缺少 OpenRouter API Token"}
				return
			}

			ctx, cancel := context.WithTimeout(c.Request.Context(), time.Duration(req.Timeout)*time.Second)
			defer cancel()

			httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
			if err != nil {
				logger.LogWarn(c.Request.Context(), "build request failed: "+err.Error())
				ch <- upstreamResult{Name: uniqueName, Err: err.Error()}
				return
			}
			httpReq.Header.Set("Accept", "application/json")
			if bearerToken != "" {
				httpReq.Header.Set("Authorization", "Bearer "+bearerToken)
			}

			// 简单重试：最多 3 次，指数退避
			var resp *http.Response
			var lastErr error
			for attempt := 0; attempt < 3; attempt++ {
				resp, lastErr = client.Do(httpReq)
				if lastErr == nil {
					break
				}
				time.Sleep(time.Duration(200*(1<<attempt)) * time.Millisecond)
			}
			if lastErr != nil {
				logger.LogWarn(c.Request.Context(), "http error on "+chItem.Name+": "+lastErr.Error())
				ch <- upstreamResult{Name: uniqueName, Err: lastErr.Error()}
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				logger.LogWarn(c.Request.Context(), "non-200 from "+chItem.Name+": "+resp.Status)
				ch <- upstreamResult{Name: uniqueName, Err: resp.Status}
				return
			}

			// Content-Type 和响应体大小校验
			if ct := resp.Header.Get("Content-Type"); ct != "" && !strings.Contains(strings.ToLower(ct), "application/json") {
				logger.LogWarn(c.Request.Context(), "unexpected content-type from "+chItem.Name+": "+ct)
			}
			limited := io.LimitReader(resp.Body, maxRatioConfigBytes)
			bodyBytes, err := io.ReadAll(limited)
			if err != nil {
				logger.LogWarn(c.Request.Context(), "read response failed from "+chItem.Name+": "+err.Error())
				ch <- upstreamResult{Name: uniqueName, Err: err.Error()}
				return
			}

			if normalizeSyncSourceType(chItem.SourceType) == syncSourceTypeOpenRouter {
				converted, err := convertOpenRouterModelsResponse(bodyBytes)
				if err != nil {
					logger.LogWarn(c.Request.Context(), "openrouter data parse failed from "+chItem.Name+": "+err.Error())
					ch <- upstreamResult{Name: uniqueName, Err: "无法解析 OpenRouter 模型价格"}
					return
				}
				ch <- upstreamResult{Name: uniqueName, Data: converted}
				return
			}

			// 兼容两种上游接口格式：
			//  type1: /api/ratio_config -> data 为 map[string]any，包含 model_ratio/completion_ratio/cache_ratio/model_price
			//  type2: /api/pricing      -> data 为 []Pricing 列表，需要转换为与 type1 相同的 map 格式
			var body struct {
				Success bool            `json:"success"`
				Data    json.RawMessage `json:"data"`
				Message string          `json:"message"`
			}

			if err := json.Unmarshal(bodyBytes, &body); err != nil {
				logger.LogWarn(c.Request.Context(), "json decode failed from "+chItem.Name+": "+err.Error())
				ch <- upstreamResult{Name: uniqueName, Err: err.Error()}
				return
			}

			if !body.Success {
				ch <- upstreamResult{Name: uniqueName, Err: body.Message}
				return
			}

			// 若 Data 为空，将继续按 type1 尝试解析（与多数静态 ratio_config 兼容）

			// 尝试按 type1 解析
			var type1Data map[string]any
			if err := json.Unmarshal(body.Data, &type1Data); err == nil {
				// 如果包含至少一个 ratioTypes 字段，则认为是 type1
				isType1 := false
				for _, rt := range ratioTypes {
					if _, ok := type1Data[rt]; ok {
						isType1 = true
						break
					}
				}
				if isType1 {
					ch <- upstreamResult{Name: uniqueName, Data: type1Data}
					return
				}
			}

			// 如果不是 type1，则尝试按 type2 (/api/pricing) 解析
			var pricingItems []struct {
				ModelName       string  `json:"model_name"`
				QuotaType       int     `json:"quota_type"`
				ModelRatio      float64 `json:"model_ratio"`
				ModelPrice      float64 `json:"model_price"`
				CompletionRatio float64 `json:"completion_ratio"`
			}
			if err := json.Unmarshal(body.Data, &pricingItems); err != nil {
				logger.LogWarn(c.Request.Context(), "unrecognized data format from "+chItem.Name+": "+err.Error())
				ch <- upstreamResult{Name: uniqueName, Err: "无法解析上游返回数据"}
				return
			}

			modelRatioMap := make(map[string]float64)
			completionRatioMap := make(map[string]float64)
			modelPriceMap := make(map[string]float64)

			for _, item := range pricingItems {
				if item.QuotaType == 1 {
					modelPriceMap[item.ModelName] = item.ModelPrice
				} else {
					modelRatioMap[item.ModelName] = item.ModelRatio
					// completionRatio 可能为 0，此时也直接赋值，保持与上游一致
					completionRatioMap[item.ModelName] = item.CompletionRatio
				}
			}

			converted := make(map[string]any)

			if len(modelRatioMap) > 0 {
				ratioAny := make(map[string]any, len(modelRatioMap))
				for k, v := range modelRatioMap {
					ratioAny[k] = v
				}
				converted["model_ratio"] = ratioAny
			}

			if len(completionRatioMap) > 0 {
				compAny := make(map[string]any, len(completionRatioMap))
				for k, v := range completionRatioMap {
					compAny[k] = v
				}
				converted["completion_ratio"] = compAny
			}

			if len(modelPriceMap) > 0 {
				priceAny := make(map[string]any, len(modelPriceMap))
				for k, v := range modelPriceMap {
					priceAny[k] = v
				}
				converted["model_price"] = priceAny
			}

			ch <- upstreamResult{Name: uniqueName, Data: converted}
		}(chn)
	}

	wg.Wait()
	close(ch)

	localData := ratio_setting.GetExposedData()

	var testResults []dto.TestResult
	var successfulChannels []struct {
		name string
		data map[string]any
	}

	for r := range ch {
		if r.Err != "" {
			testResults = append(testResults, dto.TestResult{
				Name:   r.Name,
				Status: "error",
				Error:  r.Err,
			})
		} else {
			testResults = append(testResults, dto.TestResult{
				Name:   r.Name,
				Status: "success",
			})
			successfulChannels = append(successfulChannels, struct {
				name string
				data map[string]any
			}{name: r.Name, data: r.Data})
		}
	}

	differences := buildDifferences(localData, successfulChannels)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"differences":  differences,
			"test_results": testResults,
		},
	})
}

func buildDifferences(localData map[string]any, successfulChannels []struct {
	name string
	data map[string]any
}) map[string]map[string]dto.DifferenceItem {
	differences := make(map[string]map[string]dto.DifferenceItem)

	allModels := make(map[string]struct{})

	for _, ratioType := range ratioTypes {
		if localRatioAny, ok := localData[ratioType]; ok {
			if localRatio, ok := localRatioAny.(map[string]float64); ok {
				for modelName := range localRatio {
					allModels[modelName] = struct{}{}
				}
			}
		}
	}

	for _, channel := range successfulChannels {
		for _, ratioType := range ratioTypes {
			if upstreamRatio, ok := channel.data[ratioType].(map[string]any); ok {
				for modelName := range upstreamRatio {
					allModels[modelName] = struct{}{}
				}
			}
		}
	}

	confidenceMap := make(map[string]map[string]bool)

	// 预处理阶段：检查pricing接口的可信度
	for _, channel := range successfulChannels {
		confidenceMap[channel.name] = make(map[string]bool)

		modelRatios, hasModelRatio := channel.data["model_ratio"].(map[string]any)
		completionRatios, hasCompletionRatio := channel.data["completion_ratio"].(map[string]any)

		if hasModelRatio && hasCompletionRatio {
			// 遍历所有模型，检查是否满足不可信条件
			for modelName := range allModels {
				// 默认为可信
				confidenceMap[channel.name][modelName] = true

				// 检查是否满足不可信条件：model_ratio为37.5且completion_ratio为1
				if modelRatioVal, ok := modelRatios[modelName]; ok {
					if completionRatioVal, ok := completionRatios[modelName]; ok {
						// 转换为float64进行比较
						if modelRatioFloat, ok := modelRatioVal.(float64); ok {
							if completionRatioFloat, ok := completionRatioVal.(float64); ok {
								if modelRatioFloat == 37.5 && completionRatioFloat == 1.0 {
									confidenceMap[channel.name][modelName] = false
								}
							}
						}
					}
				}
			}
		} else {
			// 如果不是从pricing接口获取的数据，则全部标记为可信
			for modelName := range allModels {
				confidenceMap[channel.name][modelName] = true
			}
		}
	}

	for modelName := range allModels {
		for _, ratioType := range ratioTypes {
			var localValue interface{} = nil
			if localRatioAny, ok := localData[ratioType]; ok {
				if localRatio, ok := localRatioAny.(map[string]float64); ok {
					if val, exists := localRatio[modelName]; exists {
						localValue = val
					}
				}
			}

			upstreamValues := make(map[string]interface{})
			confidenceValues := make(map[string]bool)
			hasUpstreamValue := false
			hasDifference := false

			for _, channel := range successfulChannels {
				var upstreamValue interface{} = nil

				if upstreamRatio, ok := channel.data[ratioType].(map[string]any); ok {
					if val, exists := upstreamRatio[modelName]; exists {
						upstreamValue = val
						hasUpstreamValue = true

						if localValue != nil && !valuesEqual(localValue, val) {
							hasDifference = true
						} else if valuesEqual(localValue, val) {
							upstreamValue = "same"
						}
					}
				}
				if upstreamValue == nil && localValue == nil {
					upstreamValue = "same"
				}

				if localValue == nil && upstreamValue != nil && upstreamValue != "same" {
					hasDifference = true
				}

				upstreamValues[channel.name] = upstreamValue

				confidenceValues[channel.name] = confidenceMap[channel.name][modelName]
			}

			shouldInclude := false

			if localValue != nil {
				if hasDifference {
					shouldInclude = true
				}
			} else {
				if hasUpstreamValue {
					shouldInclude = true
				}
			}

			if shouldInclude {
				if differences[modelName] == nil {
					differences[modelName] = make(map[string]dto.DifferenceItem)
				}
				differences[modelName][ratioType] = dto.DifferenceItem{
					Current:    localValue,
					Upstreams:  upstreamValues,
					Confidence: confidenceValues,
				}
			}
		}
	}

	channelHasDiff := make(map[string]bool)
	for _, ratioMap := range differences {
		for _, item := range ratioMap {
			for chName, val := range item.Upstreams {
				if val != nil && val != "same" {
					channelHasDiff[chName] = true
				}
			}
		}
	}

	for modelName, ratioMap := range differences {
		for ratioType, item := range ratioMap {
			for chName := range item.Upstreams {
				if !channelHasDiff[chName] {
					delete(item.Upstreams, chName)
					delete(item.Confidence, chName)
				}
			}

			allSame := true
			for _, v := range item.Upstreams {
				if v != "same" {
					allSame = false
					break
				}
			}
			if len(item.Upstreams) == 0 || allSame {
				delete(ratioMap, ratioType)
			} else {
				differences[modelName][ratioType] = item
			}
		}

		if len(ratioMap) == 0 {
			delete(differences, modelName)
		}
	}

	return differences
}

func GetSyncableChannels(c *gin.Context) {
	channels, err := model.GetAllChannels(0, 0, true, false)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	var syncableChannels []dto.SyncableChannel
	for _, channel := range channels {
		if channel.GetBaseURL() != "" {
			syncableChannels = append(syncableChannels, dto.SyncableChannel{
				ID:              channel.Id,
				Name:            channel.Name,
				BaseURL:         channel.GetBaseURL(),
				Status:          channel.Status,
				SourceType:      syncSourceTypeGeneric,
				DefaultEndpoint: defaultEndpoint,
			})
		}
	}

	syncableChannels = append(syncableChannels, dto.SyncableChannel{
		ID:              -100,
		Name:            "官方倍率预设",
		BaseURL:         "https://basellm.github.io",
		Status:          1,
		SourceType:      syncSourceTypeOfficialPreset,
		DefaultEndpoint: officialPresetPath,
	})
	syncableChannels = append(syncableChannels, dto.SyncableChannel{
		ID:                  -101,
		Name:                "OpenRouter 官方模型价格",
		BaseURL:             "https://openrouter.ai",
		Status:              1,
		SourceType:          syncSourceTypeOpenRouter,
		DefaultEndpoint:     openRouterModelsPath,
		RequiresBearerToken: true,
	})

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    syncableChannels,
	})
}
