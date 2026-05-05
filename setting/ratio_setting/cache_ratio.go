package ratio_setting

import (
	"encoding/json"
	"one-api/common"
	"sync"
)

var defaultCacheRatio = map[string]float64{
	"gpt-4":                               0.5,
	"o1":                                  0.5,
	"o1-2024-12-17":                       0.5,
	"o1-preview-2024-09-12":               0.5,
	"o1-preview":                          0.5,
	"o1-mini-2024-09-12":                  0.5,
	"o1-mini":                             0.5,
	"o3-mini":                             0.5,
	"o3-mini-2025-01-31":                  0.5,
	"gpt-4o-2024-11-20":                   0.5,
	"gpt-4o-2024-08-06":                   0.5,
	"gpt-4o":                              0.5,
	"gpt-4o-mini-2024-07-18":              0.5,
	"gpt-4o-mini":                         0.5,
	"gpt-4o-realtime-preview":             0.5,
	"gpt-4o-mini-realtime-preview":        0.5,
	"gpt-4.5-preview":                     0.5,
	"gpt-4.5-preview-2025-02-27":          0.5,
	"gpt-4.1":                             0.25,
	"gpt-4.1-mini":                        0.25,
	"gpt-4.1-nano":                        0.25,
	"gpt-5":                               0.1,
	"gpt-5-2025-08-07":                    0.1,
	"gpt-5-chat-latest":                   0.1,
	"gpt-5.1":                             0.1,
	"gpt-5.2":                             0.1,
	"gpt-5.4":                             0.1,
	"gpt-5.1-codex":                       0.1,
	"gpt-5.1-codex-max":                   0.1,
	"gpt-5.1-codex-mini":                  0.1,
	"gpt-5.2-codex":                       0.1,
	"gpt-5.3-codex":                       0.1,
	"codex-mini-latest":                   0.1,
	"gpt-5-mini":                          0.1,
	"gpt-5-mini-2025-08-07":               0.1,
	"gpt-5-nano":                          0.1,
	"gpt-5-nano-2025-08-07":               0.1,
	"deepseek-chat":                       0.25,
	"deepseek-reasoner":                   0.25,
	"deepseek-coder":                      0.25,
	"claude-3-sonnet-20240229":            0.1,
	"claude-3-opus-20240229":              0.1,
	"claude-3-haiku-20240307":             0.1,
	"claude-3-5-haiku-20241022":           0.1,
	"claude-3-5-sonnet-20240620":          0.1,
	"claude-3-5-sonnet-20241022":          0.1,
	"claude-3-7-sonnet-20250219":          0.1,
	"claude-3-7-sonnet-20250219-thinking": 0.1,
	"claude-sonnet-4-20250514":            0.1,
	"claude-sonnet-4-20250514-thinking":   0.1,
	"claude-opus-4-20250514":              0.1,
	"claude-opus-4-20250514-thinking":     0.1,
	"claude-opus-4-1-20250805":            0.1,
	"claude-opus-4-1-20250805-thinking":   0.1,
}

var defaultCreateCacheRatio = map[string]float64{
	"gpt-5":                               1,
	"gpt-5-2025-08-07":                    1,
	"gpt-5-chat-latest":                   1,
	"gpt-5.1":                             1,
	"gpt-5.2":                             1,
	"gpt-5.4":                             1,
	"gpt-5.1-codex":                       1,
	"gpt-5.1-codex-max":                   1,
	"gpt-5.1-codex-mini":                  1,
	"gpt-5.2-codex":                       1,
	"gpt-5.3-codex":                       1,
	"codex-mini-latest":                   1,
	"gpt-5-mini":                          1,
	"gpt-5-mini-2025-08-07":               1,
	"gpt-5-nano":                          1,
	"gpt-5-nano-2025-08-07":               1,
	"claude-3-sonnet-20240229":            1.25,
	"claude-3-opus-20240229":              1.25,
	"claude-3-haiku-20240307":             1.25,
	"claude-3-5-haiku-20241022":           1.25,
	"claude-3-5-sonnet-20240620":          1.25,
	"claude-3-5-sonnet-20241022":          1.25,
	"claude-3-7-sonnet-20250219":          1.25,
	"claude-3-7-sonnet-20250219-thinking": 1.25,
	"claude-sonnet-4-20250514":            1.25,
	"claude-sonnet-4-20250514-thinking":   1.25,
	"claude-opus-4-20250514":              1.25,
	"claude-opus-4-20250514-thinking":     1.25,
	"claude-opus-4-1-20250805":            1.25,
	"claude-opus-4-1-20250805-thinking":   1.25,
}

//var defaultCreateCacheRatio = map[string]float64{}

var cacheRatioMap map[string]float64
var cacheRatioMapMutex sync.RWMutex
var createCacheRatioMap = defaultCreateCacheRatio
var createCacheRatioMapMutex sync.RWMutex

// GetCacheRatioMap returns the cache ratio map
func GetCacheRatioMap() map[string]float64 {
	cacheRatioMapMutex.RLock()
	defer cacheRatioMapMutex.RUnlock()
	return cacheRatioMap
}

// CacheRatio2JSONString converts the cache ratio map to a JSON string
func CacheRatio2JSONString() string {
	cacheRatioMapMutex.RLock()
	defer cacheRatioMapMutex.RUnlock()
	jsonBytes, err := json.Marshal(cacheRatioMap)
	if err != nil {
		common.SysLog("error marshalling cache ratio: " + err.Error())
	}
	return string(jsonBytes)
}

func CreateCacheRatio2JSONString() string {
	createCacheRatioMapMutex.RLock()
	defer createCacheRatioMapMutex.RUnlock()
	jsonBytes, err := json.Marshal(createCacheRatioMap)
	if err != nil {
		common.SysLog("error marshalling create cache ratio: " + err.Error())
	}
	return string(jsonBytes)
}

// UpdateCacheRatioByJSONString updates the cache ratio map from a JSON string
func UpdateCacheRatioByJSONString(jsonStr string) error {
	cacheRatioMapMutex.Lock()
	defer cacheRatioMapMutex.Unlock()
	cacheRatioMap = make(map[string]float64)
	err := json.Unmarshal([]byte(jsonStr), &cacheRatioMap)
	if err == nil {
		InvalidateExposedDataCache()
	}
	return err
}

func UpdateCreateCacheRatioByJSONString(jsonStr string) error {
	createCacheRatioMapMutex.Lock()
	defer createCacheRatioMapMutex.Unlock()
	createCacheRatioMap = make(map[string]float64)
	err := json.Unmarshal([]byte(jsonStr), &createCacheRatioMap)
	if err == nil {
		InvalidateExposedDataCache()
	}
	return err
}

// GetCacheRatio returns the cache ratio for a model
func GetCacheRatio(name string) (float64, bool) {
	cacheRatioMapMutex.RLock()
	defer cacheRatioMapMutex.RUnlock()
	name = FormatMatchingModelName(name)
	ratio, ok := cacheRatioMap[name]
	if !ok {
		return 1, false // Default to 1 if not found
	}
	return ratio, true
}

func GetCreateCacheRatio(name string) (float64, bool) {
	createCacheRatioMapMutex.RLock()
	defer createCacheRatioMapMutex.RUnlock()
	name = FormatMatchingModelName(name)
	ratio, ok := createCacheRatioMap[name]
	if !ok {
		return 1.25, false // Default to 1.25 if not found
	}
	return ratio, true
}

func GetCacheRatioCopy() map[string]float64 {
	cacheRatioMapMutex.RLock()
	defer cacheRatioMapMutex.RUnlock()
	copyMap := make(map[string]float64, len(cacheRatioMap))
	for k, v := range cacheRatioMap {
		copyMap[k] = v
	}
	return copyMap
}

func GetCreateCacheRatioCopy() map[string]float64 {
	createCacheRatioMapMutex.RLock()
	defer createCacheRatioMapMutex.RUnlock()
	copyMap := make(map[string]float64, len(createCacheRatioMap))
	for k, v := range createCacheRatioMap {
		copyMap[k] = v
	}
	return copyMap
}
