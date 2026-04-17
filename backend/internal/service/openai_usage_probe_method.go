package service

import "strings"

type OpenAIUsageProbeMethod string

const (
	OpenAIUsageProbeMethodResponses OpenAIUsageProbeMethod = "responses"
	OpenAIUsageProbeMethodWham      OpenAIUsageProbeMethod = "wham"
)

func NormalizeOpenAIUsageProbeMethod(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(OpenAIUsageProbeMethodResponses):
		return string(OpenAIUsageProbeMethodResponses)
	case string(OpenAIUsageProbeMethodWham):
		return string(OpenAIUsageProbeMethodWham)
	default:
		return string(OpenAIUsageProbeMethodWham)
	}
}

func normalizeOpenAIUsageProbeMethod(raw string) string {
	return NormalizeOpenAIUsageProbeMethod(raw)
}
