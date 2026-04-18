package service

import "strings"

func normalizeOpenAIUpstreamBaseURL(base string) string {
	return strings.TrimRight(strings.TrimSpace(base), "/")
}

// buildOpenAIChatCompletionsURL 组装 OpenAI Chat Completions 端点。
// - base 已是 /v1/chat/completions：原样返回
// - base 已是 /v1/responses：替换为 /v1/chat/completions
// - base 以 /v1 结尾：追加 /chat/completions
// - 其他情况：追加 /v1/chat/completions
func buildOpenAIChatCompletionsURL(base string) string {
	normalized := normalizeOpenAIUpstreamBaseURL(base)
	switch {
	case strings.HasSuffix(normalized, "/v1/chat/completions"), strings.HasSuffix(normalized, "/chat/completions"):
		return normalized
	case strings.HasSuffix(normalized, "/v1/responses"):
		return strings.TrimSuffix(normalized, "/v1/responses") + "/v1/chat/completions"
	case strings.HasSuffix(normalized, "/responses"):
		return strings.TrimSuffix(normalized, "/responses") + "/v1/chat/completions"
	case strings.HasSuffix(normalized, "/v1"):
		return normalized + "/chat/completions"
	default:
		return normalized + "/v1/chat/completions"
	}
}
