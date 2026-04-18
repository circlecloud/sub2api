package service

import "strings"

// OpenAISelectionFailureReasonEndpointIncompatible 表示账号与当前入口能力不兼容。
const OpenAISelectionFailureReasonEndpointIncompatible = "endpoint_incompatible"

// OpenAIEndpointCapability 表示某个 OpenAI 入口所要求的上游协议能力。
type OpenAIEndpointCapability string

const (
	OpenAIEndpointCapabilityAny                OpenAIEndpointCapability = ""
	OpenAIEndpointCapabilityResponses          OpenAIEndpointCapability = "responses"
	OpenAIEndpointCapabilityResponsesNative    OpenAIEndpointCapability = "responses_native"
	OpenAIEndpointCapabilityMessages           OpenAIEndpointCapability = "messages"
	OpenAIEndpointCapabilityResponsesCompact   OpenAIEndpointCapability = "responses_compact"
	OpenAIEndpointCapabilityResponsesWebSocket OpenAIEndpointCapability = "responses_websocket"
)

func normalizeOpenAIEndpointCapability(capability OpenAIEndpointCapability) OpenAIEndpointCapability {
	switch capability {
	case OpenAIEndpointCapabilityResponses,
		OpenAIEndpointCapabilityResponsesNative,
		OpenAIEndpointCapabilityMessages,
		OpenAIEndpointCapabilityResponsesCompact,
		OpenAIEndpointCapabilityResponsesWebSocket:
		return capability
	default:
		return OpenAIEndpointCapabilityAny
	}
}

// SupportsOpenAIEndpointCapability 返回账号是否兼容指定 OpenAI 入口。
//
// 说明：
// - 常规 /v1/responses 允许 chat_completions 上游（由兼容层兜底）
// - 依赖原生 Responses 状态语义的 /v1/responses 请求不允许 chat-only 账号
// - /v1/messages、/v1/responses/compact、Responses WebSocket 不允许 chat-only 账号
func (a *Account) SupportsOpenAIEndpointCapability(capability OpenAIEndpointCapability) bool {
	capability = normalizeOpenAIEndpointCapability(capability)
	if capability == OpenAIEndpointCapabilityAny {
		return true
	}
	if a == nil || !a.IsOpenAI() {
		return false
	}
	if !a.UsesOpenAIChatCompletionsUpstream() {
		return true
	}
	switch capability {
	case OpenAIEndpointCapabilityResponses:
		return true
	case OpenAIEndpointCapabilityResponsesNative,
		OpenAIEndpointCapabilityMessages,
		OpenAIEndpointCapabilityResponsesCompact,
		OpenAIEndpointCapabilityResponsesWebSocket:
		return false
	default:
		return true
	}
}

func ResolveOpenAIEndpointCapabilityClientMessage(failureReason string, capability OpenAIEndpointCapability) string {
	if strings.TrimSpace(failureReason) != OpenAISelectionFailureReasonEndpointIncompatible {
		return ""
	}
	switch normalizeOpenAIEndpointCapability(capability) {
	case OpenAIEndpointCapabilityResponsesNative:
		return "No available OpenAI accounts support native /v1/responses semantics; chat_completions upstream accounts are incompatible."
	case OpenAIEndpointCapabilityMessages:
		return "No available OpenAI accounts support /v1/messages; chat_completions upstream accounts are incompatible."
	case OpenAIEndpointCapabilityResponsesCompact:
		return "No available OpenAI accounts support /v1/responses/compact; chat_completions upstream accounts are incompatible."
	case OpenAIEndpointCapabilityResponsesWebSocket:
		return "No available OpenAI accounts support Responses WebSocket; chat_completions upstream accounts are incompatible."
	default:
		return ""
	}
}

func ResolveOpenAIEndpointCapabilityCloseReason(failureReason string, capability OpenAIEndpointCapability) string {
	if strings.TrimSpace(failureReason) != OpenAISelectionFailureReasonEndpointIncompatible {
		return ""
	}
	switch normalizeOpenAIEndpointCapability(capability) {
	case OpenAIEndpointCapabilityResponsesWebSocket:
		return "Responses WebSocket is not supported for chat_completions upstream accounts"
	default:
		return ResolveOpenAIEndpointCapabilityClientMessage(failureReason, capability)
	}
}
