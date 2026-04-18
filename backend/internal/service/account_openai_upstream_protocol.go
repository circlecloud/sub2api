package service

import "strings"

const (
	OpenAIUpstreamProtocolResponses       = "responses"
	OpenAIUpstreamProtocolChatCompletions = "chat_completions"
)

func normalizeOpenAIUpstreamProtocol(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case OpenAIUpstreamProtocolChatCompletions:
		return OpenAIUpstreamProtocolChatCompletions
	case OpenAIUpstreamProtocolResponses:
		fallthrough
	default:
		return OpenAIUpstreamProtocolResponses
	}
}

// ResolveOpenAIUpstreamProtocol 返回 OpenAI 账号当前应使用的上游协议。
//
// 规则：
// - 默认 responses
// - OAuth 固定 responses
// - 仅 OpenAI API Key 账号读取 accounts.extra.openai_apikey_upstream_protocol
func (a *Account) ResolveOpenAIUpstreamProtocol() string {
	if a == nil || !a.IsOpenAI() {
		return OpenAIUpstreamProtocolResponses
	}
	if !a.IsOpenAIApiKey() {
		return OpenAIUpstreamProtocolResponses
	}
	if a.Extra == nil {
		return OpenAIUpstreamProtocolResponses
	}
	raw, ok := a.Extra["openai_apikey_upstream_protocol"].(string)
	if !ok {
		return OpenAIUpstreamProtocolResponses
	}
	return normalizeOpenAIUpstreamProtocol(raw)
}

func (a *Account) UsesOpenAIChatCompletionsUpstream() bool {
	return a.ResolveOpenAIUpstreamProtocol() == OpenAIUpstreamProtocolChatCompletions
}
