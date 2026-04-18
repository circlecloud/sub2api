package apicompat

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ResponsesToChatCompletionsRequest converts a Responses API request into a Chat
// Completions request. It is used by the OpenAI API-key chat-upstream compat
// path so /v1/responses clients can be forwarded to an upstream
// /v1/chat/completions endpoint.
func ResponsesToChatCompletionsRequest(req *ResponsesRequest) (*ChatCompletionsRequest, error) {
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}
	if reasons := ResponsesRequestNativeResponsesReasons(req); len(reasons) > 0 {
		return nil, fmt.Errorf("%s", ResponsesRequestChatUpstreamUnsupportedMessage(reasons))
	}

	messages, err := convertResponsesInputToChatMessages(req.Input)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.Instructions) != "" {
		systemContent, err := json.Marshal(req.Instructions)
		if err != nil {
			return nil, fmt.Errorf("marshal instructions: %w", err)
		}
		messages = append([]ChatMessage{{Role: "system", Content: systemContent}}, messages...)
	}

	out := &ChatCompletionsRequest{
		Model:       req.Model,
		Messages:    messages,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		Stream:      req.Stream,
		ToolChoice:  req.ToolChoice,
		ServiceTier: req.ServiceTier,
	}
	if req.MaxOutputTokens != nil && *req.MaxOutputTokens > 0 {
		v := *req.MaxOutputTokens
		out.MaxCompletionTokens = &v
	}
	if req.Reasoning != nil {
		out.ReasoningEffort = strings.TrimSpace(req.Reasoning.Effort)
	}
	if len(req.Tools) > 0 {
		tools, err := convertResponsesToolsToChat(req.Tools)
		if err != nil {
			return nil, err
		}
		out.Tools = tools
	}

	return out, nil
}

func convertResponsesInputToChatMessages(inputRaw json.RawMessage) ([]ChatMessage, error) {
	if len(inputRaw) == 0 {
		return nil, nil
	}

	var inputStr string
	if err := json.Unmarshal(inputRaw, &inputStr); err == nil {
		content, err := json.Marshal(inputStr)
		if err != nil {
			return nil, fmt.Errorf("marshal string input: %w", err)
		}
		return []ChatMessage{{Role: "user", Content: content}}, nil
	}

	var items []ResponsesInputItem
	if err := json.Unmarshal(inputRaw, &items); err != nil {
		return nil, fmt.Errorf("parse responses input: %w", err)
	}

	messages := make([]ChatMessage, 0, len(items))
	for _, item := range items {
		converted, err := responsesInputItemToChatMessages(item)
		if err != nil {
			return nil, err
		}
		messages = append(messages, converted...)
	}
	return messages, nil
}

func responsesInputItemToChatMessages(item ResponsesInputItem) ([]ChatMessage, error) {
	switch {
	case item.Role != "":
		content, err := convertResponsesContentToChat(item.Content)
		if err != nil {
			return nil, fmt.Errorf("convert %s message content: %w", item.Role, err)
		}
		return []ChatMessage{{Role: item.Role, Content: content}}, nil

	case item.Type == "function_call":
		arguments := item.Arguments
		if strings.TrimSpace(arguments) == "" {
			arguments = "{}"
		}
		return []ChatMessage{{
			Role: "assistant",
			ToolCalls: []ChatToolCall{{
				ID:   item.CallID,
				Type: "function",
				Function: ChatFunctionCall{
					Name:      item.Name,
					Arguments: arguments,
				},
			}},
		}}, nil

	case item.Type == "function_call_output":
		output := item.Output
		if output == "" {
			output = "(empty)"
		}
		content, err := json.Marshal(output)
		if err != nil {
			return nil, fmt.Errorf("marshal tool output: %w", err)
		}
		return []ChatMessage{{
			Role:       "tool",
			ToolCallID: item.CallID,
			Content:    content,
		}}, nil

	case item.Type != "":
		return nil, fmt.Errorf("unsupported responses input item type: %s", item.Type)
	default:
		return nil, fmt.Errorf("responses input item missing role/type")
	}
}

func convertResponsesContentToChat(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return json.Marshal(s)
	}

	var parts []ResponsesContentPart
	if err := json.Unmarshal(raw, &parts); err != nil {
		return nil, fmt.Errorf("parse content as string or parts array")
	}

	chatParts := make([]ChatContentPart, 0, len(parts))
	for _, part := range parts {
		switch part.Type {
		case "input_text", "output_text", "text":
			chatParts = append(chatParts, ChatContentPart{
				Type: "text",
				Text: part.Text,
			})
		case "input_image":
			if strings.TrimSpace(part.ImageURL) == "" {
				continue
			}
			chatParts = append(chatParts, ChatContentPart{
				Type: "image_url",
				ImageURL: &ChatImageURL{
					URL: part.ImageURL,
				},
			})
		default:
			return nil, fmt.Errorf("unsupported responses content part type: %s", part.Type)
		}
	}
	return json.Marshal(chatParts)
}

func convertResponsesToolsToChat(tools []ResponsesTool) ([]ChatTool, error) {
	out := make([]ChatTool, 0, len(tools))
	for _, tool := range tools {
		if tool.Type != "function" {
			return nil, fmt.Errorf("unsupported responses tool type: %s", tool.Type)
		}
		out = append(out, ChatTool{
			Type: "function",
			Function: &ChatFunction{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  tool.Parameters,
				Strict:      tool.Strict,
			},
		})
	}
	return out, nil
}
