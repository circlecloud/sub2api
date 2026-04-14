package repository

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestCompactOpenAIChunkRoundTrip_PreservesSchedulingFieldsAndStripsSecrets(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	rateReset := now.Add(3 * time.Minute)
	overloadUntil := now.Add(2 * time.Minute)
	loadFactor := 7
	rateMultiplier := 1.5
	groupID := int64(321)
	account := service.Account{
		ID:               1001,
		Name:             "openai-compact",
		Platform:         service.PlatformOpenAI,
		Type:             service.AccountTypeOAuth,
		Status:           service.StatusActive,
		Schedulable:      true,
		Concurrency:      3,
		Priority:         2,
		RateMultiplier:   &rateMultiplier,
		LoadFactor:       &loadFactor,
		LastUsedAt:       &now,
		RateLimitResetAt: &rateReset,
		OverloadUntil:    &overloadUntil,
		GroupIDs:         []int64{groupID},
		Groups:           []*service.Group{{ID: groupID, Name: "OpenAI G321", Platform: service.PlatformOpenAI}},
		Credentials: map[string]any{
			"model_mapping": map[string]any{"gpt-5.4": "gpt-5.4"},
			"api_key":       "sk-secret-should-not-be-cached",
			"access_token":  "oauth-secret-should-not-be-cached",
		},
		Extra: map[string]any{
			"privacy_mode":                              service.PrivacyModeTrainingOff,
			"model_rate_limits":                         map[string]any{"gpt-5.4": map[string]any{"rate_limit_reset_at": rateReset.Format(time.RFC3339)}},
			"openai_ws_force_http":                      true,
			"openai_oauth_responses_websockets_v2_mode": service.OpenAIWSIngressModeCtxPool,
			"codex_cli_only":                            true,
			"refresh_token":                             "should-not-be-cached",
		},
	}

	payload, err := encodeCompactOpenAIChunk([]service.Account{account})
	require.NoError(t, err)
	require.NotContains(t, string(payload), "sk-secret-should-not-be-cached")
	require.NotContains(t, string(payload), "oauth-secret-should-not-be-cached")
	require.NotContains(t, string(payload), "should-not-be-cached")

	decoded, err := decodeCompactOpenAIChunk(payload)
	require.NoError(t, err)
	require.Len(t, decoded, 1)
	got := decoded[0]
	require.NotNil(t, got)
	require.Equal(t, account.ID, got.ID)
	require.Equal(t, account.Name, got.Name)
	require.Equal(t, account.Platform, got.Platform)
	require.Equal(t, account.Type, got.Type)
	require.Equal(t, account.Concurrency, got.Concurrency)
	require.Equal(t, account.Priority, got.Priority)
	require.Equal(t, account.GroupIDs, got.GroupIDs)
	require.Len(t, got.Groups, 1)
	require.Equal(t, groupID, got.Groups[0].ID)
	require.Equal(t, "OpenAI G321", got.Groups[0].Name)
	require.NotNil(t, got.Credentials)
	require.True(t, got.IsModelSupported("gpt-5.4"))
	require.Equal(t, service.PrivacyModeTrainingOff, got.GetExtraString("privacy_mode"))
	require.True(t, got.IsOpenAIWSForceHTTPEnabled())
	require.Equal(t, service.OpenAIWSIngressModeCtxPool, got.ResolveOpenAIResponsesWebSocketV2Mode(service.OpenAIWSIngressModeOff))
	require.True(t, got.IsPrivacySet())
	require.Equal(t, rateReset, *got.RateLimitResetAt)
	require.Equal(t, overloadUntil, *got.OverloadUntil)
	require.Empty(t, got.GetCredential("api_key"))
	require.Empty(t, got.GetCredential("access_token"))
	require.Empty(t, got.GetExtraString("refresh_token"))
	require.False(t, got.IsCodexCLIOnlyEnabled(), "非调度字段不应进入快照")
}

func TestCompactOpenAIAccount_SynthesizesGroupIDsFromAccountGroups(t *testing.T) {
	account := service.Account{
		ID:            2001,
		Platform:      service.PlatformOpenAI,
		Type:          service.AccountTypeOAuth,
		Status:        service.StatusActive,
		Schedulable:   true,
		AccountGroups: []service.AccountGroup{{GroupID: 11}, {GroupID: 12}, {GroupID: 11}},
	}

	compact := newCompactOpenAIAccount(account)
	require.ElementsMatch(t, []int64{11, 12}, compact.GroupIDs)
}

func TestCompactOpenAISnapshotMeta_JSONRoundTrip(t *testing.T) {
	meta := compactSnapshotMeta{
		Encoding:   schedulerCompactSnapshotEncodingOpenAI,
		ChunkCount: 3,
		Count:      2048,
	}
	payload, err := json.Marshal(meta)
	require.NoError(t, err)
	var decoded compactSnapshotMeta
	require.NoError(t, json.Unmarshal(payload, &decoded))
	require.Equal(t, meta, decoded)
}
