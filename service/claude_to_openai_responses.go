package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"one-api/common"
	"one-api/dto"
	"strings"
)

func ClaudeToOpenAIResponsesRequest(claudeRequest dto.ClaudeRequest) (*dto.OpenAIResponsesRequest, error) {
	req, _, _, err := ClaudeToOpenAIResponsesRequestWithToolNameMapping(claudeRequest)
	return req, err
}

func ClaudeToOpenAIResponsesRequestWithToolNameMapping(claudeRequest dto.ClaudeRequest) (*dto.OpenAIResponsesRequest, map[string]string, map[string]string, error) {
	toolNameMaps, err := buildClaudeResponsesToolNameMaps(claudeRequest)
	if err != nil {
		return nil, nil, nil, err
	}

	overrideInstructions := false
	if _, v, err := GetCxCompatResponsesInstructionsSettings(); err != nil {
		return nil, nil, nil, err
	} else {
		overrideInstructions = v
	}

	instructions, err := resolveResponsesCompatInstructionsText(claudeSystemToInstructions(&claudeRequest), overrideInstructions)
	if err != nil {
		return nil, nil, nil, err
	}

	inputItems, err := claudeMessagesToResponsesInputItems(claudeRequest.Messages)
	if err != nil {
		return nil, nil, nil, err
	}
	inputRaw, err := common.Marshal(inputItems)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to marshal responses input: %w", err)
	}

	var instructionsRaw json.RawMessage
	instructionsRaw, err = common.Marshal(instructions)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to marshal responses instructions: %w", err)
	}

	toolsRaw, toolChoiceRaw, err := claudeToolsToResponsesToolsAndChoice(claudeRequest.Tools, claudeRequest.ToolChoice)
	if err != nil {
		return nil, nil, nil, err
	}
	parallelToolCallsRaw, err := claudeToolChoiceToParallelToolCalls(claudeRequest.ToolChoice)
	if err != nil {
		return nil, nil, nil, err
	}

	reasoning := claudeThinkingToResponsesReasoning(claudeRequest.Thinking)

	var includeRaw json.RawMessage
	includeRaw, err = common.Marshal([]string{"reasoning.encrypted_content"})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to marshal responses include: %w", err)
	}

	req := &dto.OpenAIResponsesRequest{
		Model:             claudeRequest.Model,
		Input:             inputRaw,
		Instructions:      instructionsRaw,
		Stream:            claudeRequest.IsStream(nil),
		Reasoning:         reasoning,
		ParallelToolCalls: parallelToolCallsRaw,
		ServiceTier:       normalizeCx2ccResponsesServiceTier(claudeRequest.ServiceTier),
		Store:             json.RawMessage("false"),
		Include:           includeRaw,
		Tools:             toolsRaw,
		ToolChoice:        toolChoiceRaw,
	}

	if err := applyResponsesFunctionToolNameMapping(req, toolNameMaps.originalToNormalized); err != nil {
		return nil, nil, nil, err
	}

	return req, toolNameMaps.originalToNormalized, toolNameMaps.normalizedToOriginal, nil
}

func ClaudeToolUseIDToResponsesCallID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" || strings.HasPrefix(id, "fc_") {
		return id
	}
	return "fc_" + id
}

func requiredClaudeToolUseIDToResponsesCallID(id string, field string) (string, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", fmt.Errorf("%s is empty", field)
	}
	return ClaudeToolUseIDToResponsesCallID(id), nil
}

func ResponsesCallIDToClaudeToolUseID(id string) string {
	id = strings.TrimSpace(id)
	if after, ok := strings.CutPrefix(id, "fc_"); ok {
		if strings.HasPrefix(after, "toolu_") || strings.HasPrefix(after, "call_") {
			return after
		}
	}
	return id
}

func claudeSystemToInstructions(req *dto.ClaudeRequest) string {
	if req == nil || req.System == nil {
		return ""
	}
	if req.IsStringSystem() {
		return req.GetStringSystem()
	}

	blocks := req.ParseSystem()
	if len(blocks) == 0 {
		return ""
	}

	var parts []string
	for _, b := range blocks {
		if t := strings.TrimSpace(b.GetText()); t != "" {
			parts = append(parts, t)
		}
	}
	return strings.Join(parts, "\n")
}

func claudeThinkingToResponsesReasoning(thinking *dto.Thinking) *dto.Reasoning {
	normalizeReasoningSummary := func(summary string) string {
		v := strings.TrimSpace(summary)
		if v == "" {
			return ""
		}
		if strings.EqualFold(v, "auto") {
			return ""
		}
		return v
	}

	optEffort := strings.TrimSpace(readOptionAny(Cx2ccReasoningEffortOpt, legacyCx2ccReasoningEffortOpt))
	optSummary := normalizeReasoningSummary(readOptionAny(Cx2ccReasoningSummaryOpt, legacyCx2ccReasoningSummaryOpt))

	thinkingEnabled := thinking != nil && strings.TrimSpace(thinking.Type) != "disabled"
	if !thinkingEnabled {
		// Keep behavior aligned with openai-claude-main: default to medium reasoning effort.
		out := &dto.Reasoning{Effort: "medium"}
		if optSummary != "" {
			out.Summary = optSummary
		}
		return out
	}

	switch strings.TrimSpace(thinking.Type) {
	case "enabled":
		budget := thinking.GetBudgetTokens()
		var out *dto.Reasoning
		switch {
		case budget < 4000:
			out = &dto.Reasoning{Effort: "low"}
		case budget <= 16000:
			out = &dto.Reasoning{Effort: "medium"}
		default:
			out = &dto.Reasoning{Effort: "high"}
		}
		if optEffort != "" {
			out.Effort = optEffort
		}
		if optSummary != "" {
			out.Summary = optSummary
		}
		return out
	case "adaptive":
		out := &dto.Reasoning{Effort: "medium"}
		if optEffort != "" {
			out.Effort = optEffort
		}
		if optSummary != "" {
			out.Summary = optSummary
		}
		return out
	case "disabled":
		// treated as not thinkingEnabled; keep medium.
		out := &dto.Reasoning{Effort: "medium"}
		if optSummary != "" {
			out.Summary = optSummary
		}
		return out
	default:
		out := &dto.Reasoning{Effort: "medium"}
		if optEffort != "" {
			out.Effort = optEffort
		}
		if optSummary != "" {
			out.Summary = optSummary
		}
		return out
	}
}

func claudeToolsToResponsesToolsAndChoice(toolsAny any, toolChoiceAny any) (toolsRaw json.RawMessage, toolChoiceRaw json.RawMessage, err error) {
	tools, err := claudeToolsToResponsesTools(toolsAny)
	if err != nil {
		return nil, nil, err
	}
	if len(tools) > 0 {
		toolsRaw, err = common.Marshal(tools)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to marshal responses tools: %w", err)
		}
	}

	toolChoice, err := claudeToolChoiceToOpenAIResponses(toolChoiceAny, toolsAny)
	if err != nil {
		return nil, nil, err
	}

	if toolChoice != nil {
		toolChoiceRaw, err = common.Marshal(toolChoice)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to marshal responses tool_choice: %w", err)
		}
	} else if len(toolsRaw) > 0 {
		// Tools provided without explicit choice: default to auto.
		toolChoiceRaw = json.RawMessage(`"auto"`)
	}

	return toolsRaw, toolChoiceRaw, nil
}

func claudeToolsToResponsesTools(toolsAny any) ([]map[string]any, error) {
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

	openAITools := make([]map[string]any, 0, len(rawTools))
	for _, rawTool := range rawTools {
		if rawTool == nil {
			continue
		}
		openAITool, err := claudeToolToResponsesTool(rawTool)
		if err != nil {
			return nil, err
		}
		if openAITool != nil {
			openAITools = append(openAITools, openAITool)
		}
	}
	return openAITools, nil
}

// Normalize Anthropic/Codex built-in web search aliases to the canonical
// Responses API tool type so cx2cc does not accidentally degrade them into
// regular function tools.
func normalizeClaudeResponsesBuiltInToolType(toolType string) string {
	toolType = strings.TrimSpace(toolType)
	switch {
	case strings.HasPrefix(toolType, dto.BuildInToolWebSearch):
		return dto.BuildInToolWebSearch
	case strings.EqualFold(toolType, "google_search"):
		return dto.BuildInToolWebSearch
	default:
		return ""
	}
}

func claudeToolToResponsesTool(tool map[string]any) (map[string]any, error) {
	toolType := strings.TrimSpace(common.Interface2String(tool["type"]))
	name := strings.TrimSpace(common.Interface2String(tool["name"]))
	if builtInToolType := normalizeClaudeResponsesBuiltInToolType(toolType); builtInToolType != "" {
		return map[string]any{"type": builtInToolType}, nil
	}

	if name == "" {
		return nil, fmt.Errorf("responses tool name is required")
	}

	out := map[string]any{
		"type":       "function",
		"name":       name,
		"parameters": cleanOpenAIResponsesToolParameters(tool["input_schema"]),
		"strict":     false,
	}
	if description := strings.TrimSpace(common.Interface2String(tool["description"])); description != "" {
		out["description"] = description
	}
	return out, nil
}

func resolveClaudeBuiltInToolChoiceType(choiceName string, toolsAny any) string {
	choiceName = strings.TrimSpace(choiceName)
	if choiceName == "" || toolsAny == nil {
		return ""
	}

	b, err := common.Marshal(toolsAny)
	if err != nil {
		return ""
	}

	var rawTools []map[string]any
	if err := common.Unmarshal(b, &rawTools); err != nil {
		return ""
	}

	for _, rawTool := range rawTools {
		if rawTool == nil {
			continue
		}
		builtInToolType := normalizeClaudeResponsesBuiltInToolType(common.Interface2String(rawTool["type"]))
		if builtInToolType == "" {
			continue
		}

		toolName := strings.TrimSpace(common.Interface2String(rawTool["name"]))
		toolType := strings.TrimSpace(common.Interface2String(rawTool["type"]))
		switch {
		case strings.EqualFold(choiceName, toolName):
			return builtInToolType
		case strings.EqualFold(choiceName, toolType):
			return builtInToolType
		}
	}

	return ""
}

func claudeToolChoiceToOpenAIResponses(choiceAny any, toolsAny any) (any, error) {
	if choiceAny == nil {
		return nil, nil
	}

	if s, ok := choiceAny.(string); ok {
		switch strings.TrimSpace(s) {
		case "auto":
			return "auto", nil
		case "any":
			return "required", nil
		case "none":
			return "none", nil
		}
		if builtInToolType := normalizeClaudeResponsesBuiltInToolType(s); builtInToolType != "" {
			return map[string]any{"type": builtInToolType}, nil
		}
	}

	choice, err := common.Any2Type[dto.ClaudeToolChoice](choiceAny)
	if err != nil {
		return nil, fmt.Errorf("invalid tool_choice: %w", err)
	}

	switch strings.TrimSpace(choice.Type) {
	case "auto":
		return "auto", nil
	case "any":
		return "required", nil
	case "none":
		return "none", nil
	case "tool":
		if strings.TrimSpace(choice.Name) == "" {
			return nil, fmt.Errorf("tool_choice.name is required for tool choice")
		}
		if builtInToolType := resolveClaudeBuiltInToolChoiceType(choice.Name, toolsAny); builtInToolType != "" {
			return map[string]any{"type": builtInToolType}, nil
		}
		return map[string]any{
			"type":     "function",
			"function": map[string]any{"name": choice.Name},
		}, nil
	default:
		if builtInToolType := normalizeClaudeResponsesBuiltInToolType(choice.Type); builtInToolType != "" {
			return map[string]any{"type": builtInToolType}, nil
		}
		return "auto", nil
	}
}

func claudeToolChoiceToParallelToolCalls(choiceAny any) (json.RawMessage, error) {
	if choiceAny == nil {
		return json.RawMessage("true"), nil
	}

	if _, ok := choiceAny.(string); ok {
		return json.RawMessage("true"), nil
	}

	choice, err := common.Any2Type[dto.ClaudeToolChoice](choiceAny)
	if err != nil {
		return nil, fmt.Errorf("invalid tool_choice: %w", err)
	}
	if choice.DisableParallelToolUse {
		return json.RawMessage("false"), nil
	}
	return json.RawMessage("true"), nil
}

func cleanOpenAIResponsesToolParameters(schema any) any {
	m, ok := schema.(map[string]any)
	if !ok || m == nil {
		return map[string]any{
			"type":                 "object",
			"properties":           map[string]any{},
			"additionalProperties": false,
		}
	}

	out := make(map[string]any, len(m)+1)
	for k, v := range m {
		if k == "$schema" {
			continue
		}
		out[k] = v
	}

	if typ, ok := out["type"].(string); !ok || strings.TrimSpace(typ) == "" {
		out["type"] = "object"
	}

	// OpenAI tool schema validation requires object schemas to explicitly include `properties`
	// (even when empty). Some MCP tools declare an empty object schema as `{ "type": "object" }`
	// which is valid JSON Schema but rejected by OpenAI.
	if typ, ok := out["type"].(string); ok && strings.EqualFold(strings.TrimSpace(typ), "object") {
		if v, exists := out["properties"]; !exists || v == nil {
			out["properties"] = map[string]any{}
		}
	}

	if _, ok := out["additionalProperties"]; !ok {
		out["additionalProperties"] = false
	}
	return out
}

func normalizeCx2ccResponsesServiceTier(serviceTier string) string {
	switch strings.ToLower(strings.TrimSpace(serviceTier)) {
	case "fast", "priority":
		return "priority"
	default:
		return ""
	}
}

func claudeMessagesToResponsesInputItems(messages []dto.ClaudeMessage) ([]any, error) {
	items := make([]any, 0, len(messages))

	for _, msg := range messages {
		switch strings.TrimSpace(msg.Role) {
		case "user":
			if err := claudeUserMessageToResponses(&msg, &items); err != nil {
				return nil, err
			}
		case "assistant":
			if err := claudeAssistantMessageToResponses(&msg, &items); err != nil {
				return nil, err
			}
		default:
			continue
		}
	}

	return items, nil
}

func claudeUserMessageToResponses(msg *dto.ClaudeMessage, items *[]any) error {
	if msg == nil {
		return nil
	}

	if msg.IsStringContent() {
		text := msg.GetStringContent()
		*items = append(*items, map[string]any{
			"type": "message",
			"role": "user",
			"content": []any{
				map[string]any{"type": "input_text", "text": text},
			},
		})
		return nil
	}

	blocks, err := msg.ParseContent()
	if err != nil {
		return err
	}

	textParts := make([]any, 0, len(blocks))
	hasCompaction := false
	compactionText := ""
	toolResultImageParts := make([]any, 0)

	for _, block := range blocks {
		switch strings.TrimSpace(block.Type) {
		case "compaction":
			textParts = textParts[:0]
			hasCompaction = true
			compactionText = common.Interface2String(block.Content)
		case "text":
			textParts = append(textParts, map[string]any{"type": "input_text", "text": block.GetText()})
		case "image":
			imageURL := claudeSourceToDataURL(block.Source, "image/png")
			textParts = append(textParts, map[string]any{
				"type":      "input_image",
				"image_url": imageURL,
				"detail":    "auto",
			})
		case "document":
			if filePart := claudeDocumentSourceToInputFile(block.Source); filePart != nil {
				textParts = append(textParts, filePart)
			}
		case "tool_result":
			output, imageParts := claudeToolResultContentToResponses(block.Content)
			if block.IsError {
				output = "ERROR: " + output
			}
			callID, err := requiredClaudeToolUseIDToResponsesCallID(block.ToolUseId, "user tool_result tool_use_id")
			if err != nil {
				return err
			}
			*items = append(*items, map[string]any{
				"type":    "function_call_output",
				"call_id": callID,
				"output":  output,
			})
			toolResultImageParts = append(toolResultImageParts, imageParts...)
		default:
			continue
		}
	}

	if hasCompaction {
		*items = append(*items, map[string]any{
			"type": "message",
			"role": "user",
			"content": []any{
				map[string]any{"type": "input_text", "text": "[Previous conversation summary]\n" + compactionText},
			},
		})
	}

	if len(textParts) > 0 {
		textParts = append(textParts, toolResultImageParts...)
		*items = append(*items, map[string]any{
			"type":    "message",
			"role":    "user",
			"content": textParts,
		})
	} else if len(toolResultImageParts) > 0 {
		*items = append(*items, map[string]any{
			"type":    "message",
			"role":    "user",
			"content": toolResultImageParts,
		})
	}

	return nil
}

func claudeAssistantMessageToResponses(msg *dto.ClaudeMessage, items *[]any) error {
	if msg == nil {
		return nil
	}

	if msg.IsStringContent() {
		text := msg.GetStringContent()
		*items = append(*items, map[string]any{
			"type": "message",
			"role": "assistant",
			"content": []any{
				map[string]any{"type": "output_text", "text": text},
			},
		})
		return nil
	}

	blocks, err := msg.ParseContent()
	if err != nil {
		return err
	}

	textParts := make([]any, 0, len(blocks))
	functionCalls := make([]any, 0)

	for _, block := range blocks {
		switch strings.TrimSpace(block.Type) {
		case "text":
			textParts = append(textParts, map[string]any{"type": "output_text", "text": block.GetText()})
		case "tool_use":
			callID, err := requiredClaudeToolUseIDToResponsesCallID(block.Id, "assistant tool_use id")
			if err != nil {
				return err
			}
			argsJSON := "{}"
			if block.Input != nil {
				args, err := common.Marshal(block.Input)
				if err != nil {
					return fmt.Errorf("failed to marshal tool_use input: %w", err)
				}
				if trimmed := strings.TrimSpace(string(args)); trimmed != "" && trimmed != "null" {
					argsJSON = trimmed
				}
			}
			functionCalls = append(functionCalls, map[string]any{
				"type":      "function_call",
				"call_id":   callID,
				"name":      block.Name,
				"arguments": argsJSON,
			})
		case "thinking", "redacted_thinking":
			continue
		default:
			continue
		}
	}

	if len(textParts) > 0 {
		*items = append(*items, map[string]any{
			"type":    "message",
			"role":    "assistant",
			"content": textParts,
		})
	}

	if len(functionCalls) > 0 {
		*items = append(*items, functionCalls...)
	}

	return nil
}

func claudeSourceToDataURL(source *dto.ClaudeMessageSource, defaultMediaType string) string {
	if source == nil {
		return ""
	}
	switch strings.TrimSpace(source.Type) {
	case "base64":
		mediaType := strings.TrimSpace(source.MediaType)
		if mediaType == "" {
			mediaType = defaultMediaType
		}
		data := common.Interface2String(source.Data)
		return fmt.Sprintf("data:%s;base64,%s", mediaType, data)
	case "url":
		return strings.TrimSpace(source.Url)
	default:
		return ""
	}
}

func claudeDocumentSourceToInputFile(source *dto.ClaudeMessageSource) map[string]any {
	if source == nil {
		return nil
	}

	switch strings.TrimSpace(source.Type) {
	case "base64":
		mediaType := strings.TrimSpace(source.MediaType)
		if mediaType == "" {
			mediaType = "application/pdf"
		}
		data := common.Interface2String(source.Data)
		return map[string]any{
			"type":      "input_file",
			"file_data": fmt.Sprintf("data:%s;base64,%s", mediaType, data),
			"filename":  "document.pdf",
		}
	case "url":
		return map[string]any{
			"type":     "input_file",
			"file_url": strings.TrimSpace(source.Url),
			"filename": "document.pdf",
		}
	default:
		return nil
	}
}

func claudeToolResultContentToResponses(content any) (string, []any) {
	const emptyOutput = "(empty)"

	if content == nil {
		return emptyOutput, nil
	}
	if s, ok := content.(string); ok {
		if strings.TrimSpace(s) == "" {
			return emptyOutput, nil
		}
		return s, nil
	}

	blocks, err := common.Any2Type[[]dto.ClaudeMediaMessage](content)
	if err != nil {
		// Best-effort: represent structured tool output as JSON string.
		if b, mErr := common.Marshal(content); mErr == nil {
			if len(bytes.TrimSpace(b)) == 0 {
				return emptyOutput, nil
			}
			return string(b), nil
		}
		raw := common.Interface2String(content)
		if strings.TrimSpace(raw) == "" {
			return emptyOutput, nil
		}
		return raw, nil
	}

	var texts []string
	imageParts := make([]any, 0)
	for _, b := range blocks {
		switch strings.TrimSpace(b.Type) {
		case "text":
			if t := strings.TrimSpace(b.GetText()); t != "" {
				texts = append(texts, t)
			}
		case "image":
			imageURL := claudeSourceToDataURL(b.Source, "image/png")
			if imageURL != "" {
				imageParts = append(imageParts, map[string]any{
					"type":      "input_image",
					"image_url": imageURL,
					"detail":    "auto",
				})
			}
		}
	}
	output := strings.Join(texts, "\n")
	if strings.TrimSpace(output) == "" {
		output = emptyOutput
	}
	return output, imageParts
}
