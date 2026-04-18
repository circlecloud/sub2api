package apicompat

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tidwall/gjson"
)

// ResponsesRequestRequiresNativeResponsesUpstream reports whether a Responses
// request depends on semantics that cannot be faithfully served through the
// API-key chat-completions upstream compatibility path.
func ResponsesRequestRequiresNativeResponsesUpstream(req *ResponsesRequest) bool {
	return len(ResponsesRequestNativeResponsesReasons(req)) > 0
}

// ResponsesRequestNativeResponsesReasonsFromBody performs the same native
// semantics detection directly against raw request JSON so callers can decide
// account capability before choosing an upstream account.
func ResponsesRequestNativeResponsesReasonsFromBody(body []byte) []string {
	if len(body) == 0 {
		return nil
	}

	reasons := make([]string, 0, 6)
	if previousResponseID := strings.TrimSpace(gjson.GetBytes(body, "previous_response_id").String()); previousResponseID != "" {
		reasons = append(reasons, "previous_response_id")
	}
	if store := gjson.GetBytes(body, "store"); store.Exists() && store.Type == gjson.True {
		reasons = append(reasons, "store")
	}
	if include := gjson.GetBytes(body, "include"); include.Exists() && include.IsArray() && len(include.Array()) > 0 {
		reasons = append(reasons, "include")
	}
	if reasoningSummary := strings.TrimSpace(gjson.GetBytes(body, "reasoning.summary").String()); reasoningSummary != "" {
		reasons = append(reasons, "reasoning.summary")
	}
	if tools := gjson.GetBytes(body, "tools"); tools.Exists() && tools.IsArray() {
		for _, tool := range tools.Array() {
			if strings.TrimSpace(tool.Get("type").String()) != "function" {
				reasons = append(reasons, "tools")
				break
			}
		}
	}
	if input := strings.TrimSpace(gjson.GetBytes(body, "input").Raw); input != "" && input != "null" && responsesInputRequiresNativeResponses(json.RawMessage(input)) {
		reasons = append(reasons, "input")
	}
	return reasons
}

// ResponsesRequestChatUpstreamUnsupportedMessage formats the stable client error
// for requests that require native Responses semantics.
func ResponsesRequestChatUpstreamUnsupportedMessage(reasons []string) string {
	if len(reasons) == 0 {
		return ""
	}
	return fmt.Sprintf(
		"chat_completions upstream compatibility mode only supports stateless /v1/responses requests; unsupported features: %s",
		strings.Join(reasons, ", "),
	)
}

// ResponsesRequestNativeResponsesReasons returns the request features that
// require a native Responses-capable upstream.
func ResponsesRequestNativeResponsesReasons(req *ResponsesRequest) []string {
	if req == nil {
		return nil
	}

	reasons := make([]string, 0, 6)
	if strings.TrimSpace(req.PreviousResponseID) != "" {
		reasons = append(reasons, "previous_response_id")
	}
	if req.Store != nil && *req.Store {
		reasons = append(reasons, "store")
	}
	if len(req.Include) > 0 {
		reasons = append(reasons, "include")
	}
	if req.Reasoning != nil && strings.TrimSpace(req.Reasoning.Summary) != "" {
		reasons = append(reasons, "reasoning.summary")
	}
	if responsesToolsRequireNativeResponses(req.Tools) {
		reasons = append(reasons, "tools")
	}
	if responsesInputRequiresNativeResponses(req.Input) {
		reasons = append(reasons, "input")
	}
	return reasons
}

func responsesToolsRequireNativeResponses(tools []ResponsesTool) bool {
	for _, tool := range tools {
		if strings.TrimSpace(tool.Type) != "function" {
			return true
		}
	}
	return false
}

func responsesInputRequiresNativeResponses(inputRaw json.RawMessage) bool {
	if len(inputRaw) == 0 {
		return false
	}

	var inputStr string
	if err := json.Unmarshal(inputRaw, &inputStr); err == nil {
		return false
	}

	var items []ResponsesInputItem
	if err := json.Unmarshal(inputRaw, &items); err != nil {
		return false
	}
	for _, item := range items {
		if responsesInputItemRequiresNativeResponses(item) {
			return true
		}
	}
	return false
}

func responsesInputItemRequiresNativeResponses(item ResponsesInputItem) bool {
	switch strings.TrimSpace(item.Type) {
	case "":
		return responsesContentRequiresNativeResponses(item.Content)
	case "function_call", "function_call_output":
		return false
	default:
		return true
	}
}

func responsesContentRequiresNativeResponses(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}

	var contentStr string
	if err := json.Unmarshal(raw, &contentStr); err == nil {
		return false
	}

	var parts []ResponsesContentPart
	if err := json.Unmarshal(raw, &parts); err != nil {
		return false
	}
	for _, part := range parts {
		switch strings.TrimSpace(part.Type) {
		case "input_text", "output_text", "text", "input_image":
			continue
		default:
			return true
		}
	}
	return false
}
