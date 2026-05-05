package oai_cc

import (
	"one-api/common"
	"strconv"
	"strings"
)

func readGlobalOption(key string) string {
	return readGlobalOptionAny(key)
}

func readGlobalOptionAny(keys ...string) string {
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	if common.OptionMap == nil {
		return ""
	}
	for _, rawKey := range keys {
		key := strings.TrimSpace(rawKey)
		if key == "" {
			continue
		}
		if value, ok := common.OptionMap[key]; ok {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func parseGlobalOptionBool(key string, defaultValue bool) bool {
	return parseGlobalOptionBoolAny(defaultValue, key)
}

func parseGlobalOptionBoolAny(defaultValue bool, keys ...string) bool {
	raw := strings.ToLower(readGlobalOptionAny(keys...))
	if raw == "" {
		return defaultValue
	}
	switch raw {
	case "1", "true", "on", "yes", "enabled":
		return true
	case "0", "false", "off", "no", "disabled":
		return false
	default:
		return defaultValue
	}
}

func parseGlobalOptionInt(key string, defaultValue int) int {
	return parseGlobalOptionIntAny(defaultValue, key)
}

func parseGlobalOptionIntAny(defaultValue int, keys ...string) int {
	raw := readGlobalOptionAny(keys...)
	if raw == "" {
		return defaultValue
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return defaultValue
	}
	return n
}

func parseGlobalOptionFloat(key string, defaultValue float64) float64 {
	return parseGlobalOptionFloatAny(defaultValue, key)
}

func parseGlobalOptionFloatAny(defaultValue float64, keys ...string) float64 {
	raw := readGlobalOptionAny(keys...)
	if raw == "" {
		return defaultValue
	}
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil || !isFinite(f) {
		return defaultValue
	}
	return f
}
