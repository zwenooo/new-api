package model

import (
	"fmt"
	"one-api/common"
	"testing"
)

func TestDefaultClawBoxManagedOpenClawConfigValueIsValid(t *testing.T) {
	patch, err := ParseClawBoxManagedOpenClawConfigValue(defaultClawBoxManagedOpenClawConfigValue())
	if err != nil {
		t.Fatalf("default managed config should be valid json object: %v", err)
	}

	agents, ok := patch["agents"].(map[string]interface{})
	if !ok {
		t.Fatalf("default managed config should include agents object")
	}
	defaults, ok := agents["defaults"].(map[string]interface{})
	if !ok {
		t.Fatalf("default managed config should include agents.defaults object")
	}
	model, ok := defaults["model"].(map[string]interface{})
	if !ok {
		t.Fatalf("default managed config should include agents.defaults.model object")
	}
	if got := model["primary"]; got != defaultClawBoxManagedPrimaryModel {
		t.Fatalf("unexpected default primary model: %v", got)
	}
	fallbacks, ok := model["fallbacks"].([]interface{})
	if !ok || len(fallbacks) != len(defaultClawBoxManagedFallbackModels) {
		t.Fatalf("unexpected default fallback models: %#v", model["fallbacks"])
	}
	for index, expected := range defaultClawBoxManagedFallbackModels {
		if fallbacks[index] != expected {
			t.Fatalf("unexpected default fallback model at index %d: %v", index, fallbacks[index])
		}
	}

	models, ok := defaults["models"].(map[string]interface{})
	if !ok {
		t.Fatalf("default managed config should include agents.defaults.models object")
	}
	if len(models) != len(defaultClawBoxManagedAnthropicModelMappings) {
		t.Fatalf("unexpected default managed model count: %d", len(models))
	}
	for _, mapping := range defaultClawBoxManagedAnthropicModelMappings {
		fullKey := fmt.Sprintf("%s/%s", clawBoxManagedAnthropicProviderID, mapping.Actual)
		entry, ok := models[fullKey].(map[string]interface{})
		if !ok {
			t.Fatalf("default managed config should include agents.defaults.models.%s object", fullKey)
		}
		if len(entry) != 0 {
			t.Fatalf("default managed config should keep %s model config empty to avoid duplicate aliases: %#v", fullKey, entry)
		}
	}

	discovery, ok := patch["discovery"].(map[string]interface{})
	if !ok {
		t.Fatalf("default managed config should include discovery object")
	}
	mdns, ok := discovery["mdns"].(map[string]interface{})
	if !ok || mdns["mode"] != "off" {
		t.Fatalf("unexpected default discovery config: %#v", discovery)
	}

	tools, ok := patch["tools"].(map[string]interface{})
	if !ok {
		t.Fatalf("default managed config should include tools object")
	}
	if tools["profile"] != "full" {
		t.Fatalf("unexpected default tools profile: %#v", tools["profile"])
	}
	allow, ok := tools["allow"].([]interface{})
	if !ok || len(allow) != 7 {
		t.Fatalf("unexpected default tools allowlist: %#v", tools["allow"])
	}

	providers, ok := patch["models"].(map[string]interface{})
	if !ok {
		t.Fatalf("default managed config should include models object")
	}
	providerEntries, ok := providers["providers"].(map[string]interface{})
	if !ok {
		t.Fatalf("default managed config should include models.providers object")
	}
	if _, exists := providerEntries["clawbox-openai"]; exists {
		t.Fatalf("default managed config should not include clawbox-openai provider shell anymore")
	}
	anthropicProvider, ok := providerEntries[clawBoxManagedAnthropicProviderID].(map[string]interface{})
	if !ok {
		t.Fatalf("default managed config should include clawbox-anthropic provider shell")
	}
	if anthropicProvider["baseUrl"] != "${baseurl}" || anthropicProvider["apiKey"] != "${apikey}" || anthropicProvider["api"] != "anthropic-messages" {
		t.Fatalf("unexpected anthropic provider placeholder config: %#v", anthropicProvider)
	}
	providerModels, ok := anthropicProvider["models"].([]interface{})
	if !ok || len(providerModels) != len(defaultClawBoxManagedAnthropicModelMappings) {
		t.Fatalf("unexpected anthropic provider models: %#v", anthropicProvider["models"])
	}
	for index, mapping := range defaultClawBoxManagedAnthropicModelMappings {
		entry, ok := providerModels[index].(map[string]interface{})
		if !ok {
			t.Fatalf("unexpected anthropic provider model entry at index %d: %#v", index, providerModels[index])
		}
		if entry["id"] != mapping.Actual || entry["name"] != mapping.Display {
			t.Fatalf("unexpected anthropic provider model mapping at index %d: %#v", index, entry)
		}
	}
}

func TestClawBoxManagedOpenClawConfigPatchUsesBuiltinDefaultWhenOptionMissing(t *testing.T) {
	common.OptionMapRWMutex.Lock()
	previous := common.OptionMap
	common.OptionMap = map[string]string{}
	common.OptionMapRWMutex.Unlock()
	defer func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previous
		common.OptionMapRWMutex.Unlock()
	}()

	patch, err := ClawBoxManagedOpenClawConfigPatch()
	if err != nil {
		t.Fatalf("missing option should still resolve default managed config: %v", err)
	}

	agents, ok := patch["agents"].(map[string]interface{})
	if !ok {
		t.Fatalf("resolved patch should include agents object")
	}
	defaults, ok := agents["defaults"].(map[string]interface{})
	if !ok {
		t.Fatalf("resolved patch should include agents.defaults object")
	}
	model, ok := defaults["model"].(map[string]interface{})
	if !ok {
		t.Fatalf("resolved patch should include agents.defaults.model object")
	}
	if got := model["primary"]; got != defaultClawBoxManagedPrimaryModel {
		t.Fatalf("unexpected default primary model: %v", got)
	}
	if got := patch["tools"]; got == nil {
		t.Fatalf("resolved patch should include tools object")
	}
	if got := patch["discovery"]; got == nil {
		t.Fatalf("resolved patch should include discovery object")
	}
}

func TestClawBoxManagedOpenClawConfigPatchPreservesExplicitBlankValue(t *testing.T) {
	common.OptionMapRWMutex.Lock()
	previous := common.OptionMap
	common.OptionMap = map[string]string{
		ClawBoxManagedOpenClawConfigOption: "",
	}
	common.OptionMapRWMutex.Unlock()
	defer func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previous
		common.OptionMapRWMutex.Unlock()
	}()

	patch, err := ClawBoxManagedOpenClawConfigPatch()
	if err != nil {
		t.Fatalf("explicit blank managed config should still parse: %v", err)
	}
	if len(patch) != 0 {
		t.Fatalf("explicit blank managed config should stay empty, got: %#v", patch)
	}
}
