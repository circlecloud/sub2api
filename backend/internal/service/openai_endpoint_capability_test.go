package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAccount_SupportsOpenAIEndpointCapability(t *testing.T) {
	chatOnly := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Extra: map[string]any{
			"openai_apikey_upstream_protocol": OpenAIUpstreamProtocolChatCompletions,
		},
	}
	responsesAPIKey := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Extra:    map[string]any{},
	}
	oauth := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
	}

	require.True(t, chatOnly.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityResponses))
	require.False(t, chatOnly.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityResponsesNative))
	require.False(t, chatOnly.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityMessages))
	require.False(t, chatOnly.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityResponsesCompact))
	require.False(t, chatOnly.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityResponsesWebSocket))

	for _, capability := range []OpenAIEndpointCapability{
		OpenAIEndpointCapabilityResponses,
		OpenAIEndpointCapabilityResponsesNative,
		OpenAIEndpointCapabilityMessages,
		OpenAIEndpointCapabilityResponsesCompact,
		OpenAIEndpointCapabilityResponsesWebSocket,
	} {
		require.True(t, responsesAPIKey.SupportsOpenAIEndpointCapability(capability))
		require.True(t, oauth.SupportsOpenAIEndpointCapability(capability))
	}
}

func TestNormalizeOpenAIEndpointCapability_PreservesResponsesNative(t *testing.T) {
	require.Equal(t,
		OpenAIEndpointCapabilityResponsesNative,
		normalizeOpenAIEndpointCapability(OpenAIEndpointCapabilityResponsesNative),
	)
}

func TestResolveOpenAIEndpointCapabilityClientMessage(t *testing.T) {
	require.Equal(t,
		"No available OpenAI accounts support /v1/messages; chat_completions upstream accounts are incompatible.",
		ResolveOpenAIEndpointCapabilityClientMessage(OpenAISelectionFailureReasonEndpointIncompatible, OpenAIEndpointCapabilityMessages),
	)
	require.Equal(t,
		"No available OpenAI accounts support native /v1/responses semantics; chat_completions upstream accounts are incompatible.",
		ResolveOpenAIEndpointCapabilityClientMessage(OpenAISelectionFailureReasonEndpointIncompatible, OpenAIEndpointCapabilityResponsesNative),
	)
	require.Equal(t,
		"No available OpenAI accounts support /v1/responses/compact; chat_completions upstream accounts are incompatible.",
		ResolveOpenAIEndpointCapabilityClientMessage(OpenAISelectionFailureReasonEndpointIncompatible, OpenAIEndpointCapabilityResponsesCompact),
	)
	require.Empty(t,
		ResolveOpenAIEndpointCapabilityClientMessage("transport_incompatible", OpenAIEndpointCapabilityMessages),
	)
}
