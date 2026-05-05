package service

import (
	"encoding/json"
	"fmt"
	"one-api/common"
	"one-api/dto"
	"strconv"
	"strings"
)

const responsesFunctionToolNameLimit = 64

type responsesToolNameMaps struct {
	originalToNormalized map[string]string
	normalizedToOriginal map[string]string
}

func buildClaudeResponsesToolNameMaps(claudeRequest dto.ClaudeRequest) (responsesToolNameMaps, error) {
	names, err := collectClaudeResponsesFunctionToolNames(claudeRequest)
	if err != nil {
		return responsesToolNameMaps{}, err
	}
	if len(names) == 0 {
		return responsesToolNameMaps{}, nil
	}

	allMappings := buildResponsesFunctionToolNameMap(names)
	if len(allMappings) == 0 {
		return responsesToolNameMaps{}, nil
	}

	originalToNormalized := make(map[string]string)
	normalizedToOriginal := make(map[string]string)
	for original, normalized := range allMappings {
		if original == normalized {
			continue
		}
		originalToNormalized[original] = normalized
		normalizedToOriginal[normalized] = original
	}
	if len(originalToNormalized) == 0 {
		return responsesToolNameMaps{}, nil
	}
	return responsesToolNameMaps{
		originalToNormalized: originalToNormalized,
		normalizedToOriginal: normalizedToOriginal,
	}, nil
}

func collectClaudeResponsesFunctionToolNames(claudeRequest dto.ClaudeRequest) ([]string, error) {
	names := make([]string, 0)
	seen := make(map[string]struct{})
	addName := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		if _, exists := seen[name]; exists {
			return
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}

	rawTools, err := decodeClaudeRawTools(claudeRequest.Tools)
	if err != nil {
		return nil, err
	}
	for _, rawTool := range rawTools {
		if rawTool == nil {
			continue
		}
		if builtInToolType := normalizeClaudeResponsesBuiltInToolType(common.Interface2String(rawTool["type"])); builtInToolType != "" {
			continue
		}
		addName(common.Interface2String(rawTool["name"]))
	}

	for _, msg := range claudeRequest.Messages {
		if strings.TrimSpace(msg.Role) != "assistant" || msg.IsStringContent() {
			continue
		}
		blocks, err := msg.ParseContent()
		if err != nil {
			return nil, err
		}
		for _, block := range blocks {
			if strings.TrimSpace(block.Type) != "tool_use" {
				continue
			}
			addName(block.Name)
		}
	}

	if toolChoiceName := collectClaudeResponsesToolChoiceFunctionName(claudeRequest.ToolChoice, claudeRequest.Tools); toolChoiceName != "" {
		addName(toolChoiceName)
	}

	return names, nil
}

func decodeClaudeRawTools(toolsAny any) ([]map[string]any, error) {
	if toolsAny == nil {
		return nil, nil
	}

	b, err := common.Marshal(toolsAny)
	if err != nil {
		return nil, fmt.Errorf("invalid tools: %w", err)
	}

	var rawTools []map[string]any
	if err := common.Unmarshal(b, &rawTools); err != nil {
		return nil, fmt.Errorf("invalid tools: %w", err)
	}
	return rawTools, nil
}

func collectClaudeResponsesToolChoiceFunctionName(toolChoiceAny any, toolsAny any) string {
	if toolChoiceAny == nil {
		return ""
	}

	choice, err := common.Any2Type[dto.ClaudeToolChoice](toolChoiceAny)
	if err != nil {
		return ""
	}
	if strings.TrimSpace(choice.Type) != "tool" {
		return ""
	}
	if builtInToolType := resolveClaudeBuiltInToolChoiceType(choice.Name, toolsAny); builtInToolType != "" {
		return ""
	}
	return strings.TrimSpace(choice.Name)
}

func buildResponsesFunctionToolNameMap(names []string) map[string]string {
	mapping := make(map[string]string, len(names))
	used := make(map[string]struct{}, len(names))

	for _, original := range names {
		original = strings.TrimSpace(original)
		if original == "" {
			continue
		}
		if _, exists := mapping[original]; exists {
			continue
		}

		candidate := shortenResponsesFunctionToolName(original)
		normalized := makeUniqueResponsesFunctionToolName(candidate, used)
		used[normalized] = struct{}{}
		mapping[original] = normalized
	}

	return mapping
}

func shortenResponsesFunctionToolName(name string) string {
	if len(name) <= responsesFunctionToolNameLimit {
		return name
	}
	if strings.HasPrefix(name, "mcp__") {
		if idx := strings.LastIndex(name, "__"); idx > 0 {
			candidate := "mcp__" + name[idx+2:]
			if len(candidate) > responsesFunctionToolNameLimit {
				return candidate[:responsesFunctionToolNameLimit]
			}
			return candidate
		}
	}
	return name[:responsesFunctionToolNameLimit]
}

func makeUniqueResponsesFunctionToolName(candidate string, used map[string]struct{}) string {
	if _, exists := used[candidate]; !exists {
		return candidate
	}

	base := candidate
	for i := 1; ; i++ {
		suffix := "_" + strconv.Itoa(i)
		allowed := responsesFunctionToolNameLimit - len(suffix)
		if allowed < 0 {
			allowed = 0
		}
		unique := base
		if len(unique) > allowed {
			unique = unique[:allowed]
		}
		unique += suffix
		if _, exists := used[unique]; !exists {
			return unique
		}
	}
}

func applyResponsesFunctionToolNameMapping(req *dto.OpenAIResponsesRequest, originalToNormalized map[string]string) error {
	if req == nil || len(originalToNormalized) == 0 {
		return nil
	}

	if err := applyResponsesToolArrayNameMapping(req, originalToNormalized); err != nil {
		return err
	}
	if err := applyResponsesToolChoiceNameMapping(req, originalToNormalized); err != nil {
		return err
	}
	if err := applyResponsesInputFunctionCallNameMapping(req, originalToNormalized); err != nil {
		return err
	}
	return nil
}

func applyResponsesToolArrayNameMapping(req *dto.OpenAIResponsesRequest, originalToNormalized map[string]string) error {
	if len(req.Tools) == 0 {
		return nil
	}

	var tools []map[string]any
	if err := common.Unmarshal(req.Tools, &tools); err != nil {
		return fmt.Errorf("failed to unmarshal responses tools for tool-name mapping: %w", err)
	}

	changed := false
	for _, tool := range tools {
		if tool == nil || strings.TrimSpace(common.Interface2String(tool["type"])) != "function" {
			continue
		}
		if rewriteResponsesFunctionToolName(tool, "name", originalToNormalized) {
			changed = true
		}
	}
	if !changed {
		return nil
	}

	toolsRaw, err := common.Marshal(tools)
	if err != nil {
		return fmt.Errorf("failed to marshal responses tools after tool-name mapping: %w", err)
	}
	req.Tools = json.RawMessage(toolsRaw)
	return nil
}

func applyResponsesToolChoiceNameMapping(req *dto.OpenAIResponsesRequest, originalToNormalized map[string]string) error {
	if len(req.ToolChoice) == 0 {
		return nil
	}

	var toolChoice any
	if err := common.Unmarshal(req.ToolChoice, &toolChoice); err != nil {
		return fmt.Errorf("failed to unmarshal responses tool_choice for tool-name mapping: %w", err)
	}

	toolChoiceMap, ok := toolChoice.(map[string]any)
	if !ok || strings.TrimSpace(common.Interface2String(toolChoiceMap["type"])) != "function" {
		return nil
	}

	functionMap, _ := toolChoiceMap["function"].(map[string]any)
	if functionMap == nil {
		return nil
	}
	if !rewriteResponsesFunctionToolName(functionMap, "name", originalToNormalized) {
		return nil
	}

	toolChoiceRaw, err := common.Marshal(toolChoiceMap)
	if err != nil {
		return fmt.Errorf("failed to marshal responses tool_choice after tool-name mapping: %w", err)
	}
	req.ToolChoice = json.RawMessage(toolChoiceRaw)
	return nil
}

func applyResponsesInputFunctionCallNameMapping(req *dto.OpenAIResponsesRequest, originalToNormalized map[string]string) error {
	if len(req.Input) == 0 {
		return nil
	}

	var inputItems []map[string]any
	if err := common.Unmarshal(req.Input, &inputItems); err != nil {
		return fmt.Errorf("failed to unmarshal responses input for tool-name mapping: %w", err)
	}

	changed := false
	for _, item := range inputItems {
		if item == nil || strings.TrimSpace(common.Interface2String(item["type"])) != "function_call" {
			continue
		}
		if rewriteResponsesFunctionToolName(item, "name", originalToNormalized) {
			changed = true
		}
	}
	if !changed {
		return nil
	}

	inputRaw, err := common.Marshal(inputItems)
	if err != nil {
		return fmt.Errorf("failed to marshal responses input after tool-name mapping: %w", err)
	}
	req.Input = json.RawMessage(inputRaw)
	return nil
}

func rewriteResponsesFunctionToolName(target map[string]any, field string, originalToNormalized map[string]string) bool {
	if target == nil {
		return false
	}

	current := strings.TrimSpace(common.Interface2String(target[field]))
	if current == "" {
		return false
	}

	normalized, ok := originalToNormalized[current]
	if !ok {
		return false
	}
	target[field] = normalized
	return true
}
