package handlers

import (
	"fmt"
	"strings"
)

func shouldTreatAsResponsesFormat(raw map[string]interface{}) bool {
	if raw == nil {
		return false
	}
	if _, ok := raw["messages"]; ok {
		return false
	}
	if _, ok := raw["input"]; ok {
		return true
	}
	if _, ok := raw["instructions"]; ok {
		return true
	}
	return false
}

func convertResponsesRequestToChatCompletions(raw map[string]interface{}) (chatCompletionsRequest, error) {
	req := chatCompletionsRequest{
		Model:          strings.TrimSpace(getMapString(raw, "model")),
		PromptCacheKey: strings.TrimSpace(getMapString(raw, "prompt_cache_key")),
	}

	if stream, ok := raw["stream"].(bool); ok {
		req.Stream = boolPtr(stream)
	}
	if tools, ok := raw["tools"].([]interface{}); ok && len(tools) > 0 {
		req.Tools = tools
	}
	if toolChoice, ok := raw["tool_choice"]; ok && toolChoice != nil {
		req.ToolChoice = toolChoice
	}
	if parallelToolCalls, ok := raw["parallel_tool_calls"].(bool); ok {
		req.ParallelToolCalls = boolPtr(parallelToolCalls)
	}
	if reasoning := getMap(raw, "reasoning"); reasoning != nil {
		if effort := strings.TrimSpace(getMapString(reasoning, "effort")); effort != "" {
			req.Reasoning = &chatReasoning{Effort: effort}
			req.ReasoningEffort = effort
		}
	}

	if instructions := strings.TrimSpace(getMapString(raw, "instructions")); instructions != "" {
		req.Messages = append(req.Messages, chatMessageInput{
			Role:    "system",
			Content: instructions,
		})
	}

	inputValue, hasInput := raw["input"]
	if !hasInput || inputValue == nil {
		return req, nil
	}

	switch input := inputValue.(type) {
	case string:
		if strings.TrimSpace(input) != "" {
			req.Messages = append(req.Messages, chatMessageInput{
				Role:    "user",
				Content: input,
			})
		}
	case []interface{}:
		for idx, item := range input {
			itemMap, ok := item.(map[string]interface{})
			if !ok {
				return chatCompletionsRequest{}, fmt.Errorf("unsupported responses input item at index %d", idx)
			}
			message, err := convertResponsesInputItemToChatMessage(itemMap)
			if err != nil {
				return chatCompletionsRequest{}, fmt.Errorf("convert responses input item %d: %w", idx, err)
			}
			if message == nil {
				continue
			}
			req.Messages = append(req.Messages, *message)
		}
	default:
		return chatCompletionsRequest{}, fmt.Errorf("unsupported responses input type %T", inputValue)
	}

	return req, nil
}

func convertResponsesInputItemToChatMessage(item map[string]interface{}) (*chatMessageInput, error) {
	itemType := strings.ToLower(strings.TrimSpace(getMapString(item, "type")))
	if itemType == "" && strings.TrimSpace(getMapString(item, "role")) != "" {
		itemType = "message"
	}

	switch itemType {
	case "message", "":
		role := strings.ToLower(strings.TrimSpace(getMapString(item, "role")))
		if role == "" {
			return nil, fmt.Errorf("message role is empty")
		}
		return &chatMessageInput{
			Role:    role,
			Content: convertResponsesContentToChatContent(item["content"]),
		}, nil
	case "function_call":
		name := strings.TrimSpace(getMapString(item, "name"))
		if name == "" {
			return nil, fmt.Errorf("function_call name is empty")
		}
		return &chatMessageInput{
			Role: "assistant",
			ToolCalls: []chatToolCall{
				{
					ID:   strings.TrimSpace(getMapString(item, "call_id")),
					Type: "function",
					Function: &chatToolFunction{
						Name:      name,
						Arguments: getStringFromRaw(item["arguments"]),
					},
				},
			},
		}, nil
	case "function_call_output":
		callID := strings.TrimSpace(getMapString(item, "call_id"))
		if callID == "" {
			return nil, fmt.Errorf("function_call_output call_id is empty")
		}
		return &chatMessageInput{
			Role:       "tool",
			ToolCallID: callID,
			Content:    getStringFromRaw(item["output"]),
		}, nil
	default:
		return nil, fmt.Errorf("unsupported responses input type %q", itemType)
	}
}

func convertResponsesContentToChatContent(content interface{}) interface{} {
	switch v := content.(type) {
	case nil:
		return nil
	case string:
		return strings.TrimSpace(v)
	case []interface{}:
		parts := make([]interface{}, 0, len(v))
		for _, item := range v {
			switch value := item.(type) {
			case string:
				text := strings.TrimSpace(value)
				if text != "" {
					parts = append(parts, map[string]interface{}{
						"type": "text",
						"text": text,
					})
				}
			case map[string]interface{}:
				part := convertResponsesContentPartToChatContent(value)
				if part != nil {
					parts = append(parts, part)
				}
			}
		}
		if len(parts) == 1 {
			if partMap, ok := parts[0].(map[string]interface{}); ok && strings.EqualFold(getMapString(partMap, "type"), "text") {
				return strings.TrimSpace(getMapString(partMap, "text"))
			}
		}
		if len(parts) == 0 {
			return nil
		}
		return parts
	case map[string]interface{}:
		if part := convertResponsesContentPartToChatContent(v); part != nil {
			if partMap, ok := part.(map[string]interface{}); ok && strings.EqualFold(getMapString(partMap, "type"), "text") {
				return strings.TrimSpace(getMapString(partMap, "text"))
			}
			return []interface{}{part}
		}
	}
	text := strings.TrimSpace(getStringFromRaw(content))
	if text == "" {
		return nil
	}
	return text
}

func convertResponsesContentPartToChatContent(item map[string]interface{}) interface{} {
	itemType := strings.ToLower(strings.TrimSpace(getMapString(item, "type")))
	switch itemType {
	case "input_text", "output_text", "text":
		text := strings.TrimSpace(getMapString(item, "text"))
		if text == "" {
			return nil
		}
		return map[string]interface{}{
			"type": "text",
			"text": text,
		}
	case "input_image", "image_url":
		imageURL := parseImageURL(item["image_url"])
		if imageURL == "" {
			imageURL = strings.TrimSpace(getMapString(item, "image_url"))
		}
		if imageURL == "" {
			return nil
		}
		return map[string]interface{}{
			"type": "image_url",
			"image_url": map[string]interface{}{
				"url": imageURL,
			},
		}
	default:
		return nil
	}
}

func boolPtr(v bool) *bool {
	return &v
}
