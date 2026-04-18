package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAccount_ResolveOpenAIUpstreamProtocol(t *testing.T) {
	t.Run("默认走 responses", func(t *testing.T) {
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeAPIKey,
			Extra:    map[string]any{},
		}
		require.Equal(t, OpenAIUpstreamProtocolResponses, account.ResolveOpenAIUpstreamProtocol())
	})

	t.Run("OAuth 固定走 responses", func(t *testing.T) {
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra: map[string]any{
				"openai_apikey_upstream_protocol": OpenAIUpstreamProtocolChatCompletions,
			},
		}
		require.Equal(t, OpenAIUpstreamProtocolResponses, account.ResolveOpenAIUpstreamProtocol())
		require.False(t, account.UsesOpenAIChatCompletionsUpstream())
	})

	t.Run("OpenAI API Key 支持 chat_completions", func(t *testing.T) {
		account := &Account{
			Platform: PlatformOpenAI,
			Type:     AccountTypeAPIKey,
			Extra: map[string]any{
				"openai_apikey_upstream_protocol": OpenAIUpstreamProtocolChatCompletions,
			},
		}
		require.Equal(t, OpenAIUpstreamProtocolChatCompletions, account.ResolveOpenAIUpstreamProtocol())
		require.True(t, account.UsesOpenAIChatCompletionsUpstream())
	})
}
