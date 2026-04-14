package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type openAIPublicLinkSettingRepoStub struct {
	values map[string]string
}

func (s *openAIPublicLinkSettingRepoStub) Get(ctx context.Context, key string) (*Setting, error) {
	if value, ok := s.values[key]; ok {
		return &Setting{Key: key, Value: value}, nil
	}
	return nil, ErrSettingNotFound
}

func (s *openAIPublicLinkSettingRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	if value, ok := s.values[key]; ok {
		return value, nil
	}
	return "", ErrSettingNotFound
}

func (s *openAIPublicLinkSettingRepoStub) Set(ctx context.Context, key, value string) error {
	if s.values == nil {
		s.values = make(map[string]string)
	}
	s.values[key] = value
	return nil
}

func (s *openAIPublicLinkSettingRepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			out[key] = value
		}
	}
	return out, nil
}

func (s *openAIPublicLinkSettingRepoStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	if s.values == nil {
		s.values = make(map[string]string)
	}
	for key, value := range settings {
		s.values[key] = value
	}
	return nil
}

func (s *openAIPublicLinkSettingRepoStub) GetAll(ctx context.Context) (map[string]string, error) {
	out := make(map[string]string, len(s.values))
	for key, value := range s.values {
		out[key] = value
	}
	return out, nil
}

func (s *openAIPublicLinkSettingRepoStub) Delete(ctx context.Context, key string) error {
	delete(s.values, key)
	return nil
}

func TestSettingService_OpenAIPublicAddLinksLifecycle(t *testing.T) {
	t.Parallel()

	repo := &openAIPublicLinkSettingRepoStub{values: make(map[string]string)}
	svc := NewSettingService(repo, &config.Config{Server: config.ServerConfig{FrontendURL: "https://example.com/"}})
	ctx := context.Background()

	proxyID := int64(9)
	concurrency := 12
	loadFactor := 8
	priority := 3
	rateMultiplier := 0.5
	expiresAt := int64(1893456000)
	autoPauseOnExpired := false
	created, err := svc.CreateOpenAIPublicAddLink(ctx, "  Ops Link  ", []int64{3, 2, 2, -1}, &OpenAIPublicAddLinkAccountDefaults{
		ProxyID:            &proxyID,
		Concurrency:        &concurrency,
		LoadFactor:         &loadFactor,
		Priority:           &priority,
		RateMultiplier:     &rateMultiplier,
		ExpiresAt:          &expiresAt,
		AutoPauseOnExpired: &autoPauseOnExpired,
		Credentials: map[string]any{
			"model_mapping": map[string]any{"gpt-4.1": "gpt-4.1"},
		},
		Extra: map[string]any{
			"openai_passthrough": true,
		},
	})
	require.NoError(t, err)
	require.NotEmpty(t, created.Token)
	require.Equal(t, "Ops Link", created.Name)
	require.Equal(t, []int64{2, 3}, created.GroupIDs)
	require.NotNil(t, created.AccountDefaults)
	require.NotNil(t, created.AccountDefaults.ProxyID)
	require.Equal(t, proxyID, *created.AccountDefaults.ProxyID)
	require.NotNil(t, created.AccountDefaults.Concurrency)
	require.Equal(t, concurrency, *created.AccountDefaults.Concurrency)
	require.NotNil(t, created.AccountDefaults.LoadFactor)
	require.Equal(t, loadFactor, *created.AccountDefaults.LoadFactor)
	require.NotNil(t, created.AccountDefaults.Priority)
	require.Equal(t, priority, *created.AccountDefaults.Priority)
	require.NotNil(t, created.AccountDefaults.RateMultiplier)
	require.Equal(t, rateMultiplier, *created.AccountDefaults.RateMultiplier)
	require.NotNil(t, created.AccountDefaults.ExpiresAt)
	require.Equal(t, expiresAt, *created.AccountDefaults.ExpiresAt)
	require.NotNil(t, created.AccountDefaults.AutoPauseOnExpired)
	require.Equal(t, autoPauseOnExpired, *created.AccountDefaults.AutoPauseOnExpired)

	fetched, err := svc.GetOpenAIPublicAddLink(ctx, created.Token)
	require.NoError(t, err)
	require.Equal(t, created.Token, fetched.Token)
	require.Equal(t, []int64{2, 3}, fetched.GroupIDs)
	require.NotNil(t, fetched.AccountDefaults)
	require.NotNil(t, fetched.AccountDefaults.ProxyID)
	require.Equal(t, proxyID, *fetched.AccountDefaults.ProxyID)

	listed, err := svc.ListOpenAIPublicAddLinks(ctx)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	require.Equal(t, created.Token, listed[0].Token)

	updatedConcurrency := 4
	updatedPriority := 6
	updatedRateMultiplier := 1.25
	updatedAutoPauseOnExpired := true
	updated, err := svc.UpdateOpenAIPublicAddLink(ctx, created.Token, "  Updated Link  ", []int64{3}, &OpenAIPublicAddLinkAccountDefaults{
		Concurrency:        &updatedConcurrency,
		Priority:           &updatedPriority,
		RateMultiplier:     &updatedRateMultiplier,
		AutoPauseOnExpired: &updatedAutoPauseOnExpired,
		Extra: map[string]any{
			"codex_cli_only": true,
		},
	})
	require.NoError(t, err)
	require.Equal(t, created.Token, updated.Token)
	require.Equal(t, created.CreatedAt, updated.CreatedAt)
	require.Equal(t, "Updated Link", updated.Name)
	require.Equal(t, []int64{3}, updated.GroupIDs)
	require.NotNil(t, updated.AccountDefaults)
	require.Nil(t, updated.AccountDefaults.ProxyID)
	require.NotNil(t, updated.AccountDefaults.Concurrency)
	require.Equal(t, updatedConcurrency, *updated.AccountDefaults.Concurrency)
	require.NotNil(t, updated.AccountDefaults.Priority)
	require.Equal(t, updatedPriority, *updated.AccountDefaults.Priority)
	require.NotNil(t, updated.AccountDefaults.RateMultiplier)
	require.Equal(t, updatedRateMultiplier, *updated.AccountDefaults.RateMultiplier)
	require.NotNil(t, updated.AccountDefaults.AutoPauseOnExpired)
	require.Equal(t, updatedAutoPauseOnExpired, *updated.AccountDefaults.AutoPauseOnExpired)
	require.NotNil(t, updated.AccountDefaults.Extra)
	require.Equal(t, true, updated.AccountDefaults.Extra["codex_cli_only"])
	require.False(t, updated.UpdatedAt.Before(created.UpdatedAt))

	rotated, err := svc.RotateOpenAIPublicAddLink(ctx, created.Token)
	require.NoError(t, err)
	require.NotEqual(t, created.Token, rotated.Token)
	require.Equal(t, updated.Name, rotated.Name)
	require.Equal(t, updated.GroupIDs, rotated.GroupIDs)
	require.NotNil(t, rotated.AccountDefaults)
	require.Nil(t, rotated.AccountDefaults.ProxyID)
	require.NotNil(t, rotated.AccountDefaults.Concurrency)
	require.Equal(t, updatedConcurrency, *rotated.AccountDefaults.Concurrency)

	_, err = svc.GetOpenAIPublicAddLink(ctx, created.Token)
	require.ErrorIs(t, err, ErrOpenAIPublicLinkNotFound)

	rotatedFetched, err := svc.GetOpenAIPublicAddLink(ctx, rotated.Token)
	require.NoError(t, err)
	require.Equal(t, rotated.Token, rotatedFetched.Token)

	url := svc.BuildOpenAIPublicAddLinkURL(ctx, rotated.Token)
	require.Equal(t, "https://example.com/openai/connect/"+rotated.Token, url)

	err = svc.DeleteOpenAIPublicAddLink(ctx, rotated.Token)
	require.NoError(t, err)

	listed, err = svc.ListOpenAIPublicAddLinks(ctx)
	require.NoError(t, err)
	require.Empty(t, listed)
}
