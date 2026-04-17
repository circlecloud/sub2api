package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type gatewayForwardingSettingRepoStub struct {
	values map[string]string
}

func (s *gatewayForwardingSettingRepoStub) Get(ctx context.Context, key string) (*Setting, error) {
	if value, ok := s.values[key]; ok {
		return &Setting{Key: key, Value: value}, nil
	}
	return nil, nil
}

func (s *gatewayForwardingSettingRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	if value, ok := s.values[key]; ok {
		return value, nil
	}
	return "", nil
}

func (s *gatewayForwardingSettingRepoStub) Set(ctx context.Context, key, value string) error {
	if s.values == nil {
		s.values = map[string]string{}
	}
	s.values[key] = value
	return nil
}

func (s *gatewayForwardingSettingRepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	result := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			result[key] = value
		}
	}
	return result, nil
}

func (s *gatewayForwardingSettingRepoStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	if s.values == nil {
		s.values = map[string]string{}
	}
	for key, value := range settings {
		s.values[key] = value
	}
	return nil
}

func (s *gatewayForwardingSettingRepoStub) GetAll(ctx context.Context) (map[string]string, error) {
	result := make(map[string]string, len(s.values))
	for key, value := range s.values {
		result[key] = value
	}
	return result, nil
}

func (s *gatewayForwardingSettingRepoStub) Delete(ctx context.Context, key string) error {
	delete(s.values, key)
	return nil
}

func TestSettingService_IsOpenAIStreamRectifierEnabled(t *testing.T) {
	t.Run("默认回退到配置值", func(t *testing.T) {
		gatewayForwardingCache.Store((*cachedGatewayForwardingSettings)(nil))
		repo := &gatewayForwardingSettingRepoStub{values: map[string]string{}}
		svc := NewSettingService(repo, &config.Config{Gateway: config.GatewayConfig{OpenAIStreamRectifierEnabled: false}})
		require.False(t, svc.IsOpenAIStreamRectifierEnabled(t.Context()))
	})

	t.Run("数据库设置覆盖配置", func(t *testing.T) {
		gatewayForwardingCache.Store((*cachedGatewayForwardingSettings)(nil))
		repo := &gatewayForwardingSettingRepoStub{values: map[string]string{SettingKeyEnableOpenAIStreamRectifier: "true"}}
		svc := NewSettingService(repo, &config.Config{Gateway: config.GatewayConfig{OpenAIStreamRectifierEnabled: false}})
		require.True(t, svc.IsOpenAIStreamRectifierEnabled(t.Context()))
	})
}

func TestSettingService_GetOpenAIStreamRectifierTimeouts(t *testing.T) {
	t.Run("默认回退到配置数组", func(t *testing.T) {
		gatewayForwardingCache.Store((*cachedGatewayForwardingSettings)(nil))
		repo := &gatewayForwardingSettingRepoStub{values: map[string]string{}}
		svc := NewSettingService(repo, &config.Config{Gateway: config.GatewayConfig{
			OpenAIStreamResponseHeaderRectifierTimeouts: []int{8, 10, 12},
			OpenAIStreamFirstTokenRectifierTimeouts:     []int{5, 8, 10},
		}})
		header, first := svc.GetOpenAIStreamRectifierTimeouts(t.Context())
		require.Equal(t, []int{8, 10, 12}, header)
		require.Equal(t, []int{5, 8, 10}, first)
	})

	t.Run("数据库设置覆盖配置数组", func(t *testing.T) {
		gatewayForwardingCache.Store((*cachedGatewayForwardingSettings)(nil))
		repo := &gatewayForwardingSettingRepoStub{values: map[string]string{
			SettingKeyOpenAIStreamResponseHeaderRectifierTimeouts: `[5,8,10]`,
			SettingKeyOpenAIStreamFirstTokenRectifierTimeouts:     `[6,7,9]`,
		}}
		svc := NewSettingService(repo, &config.Config{Gateway: config.GatewayConfig{
			OpenAIStreamResponseHeaderRectifierTimeouts: []int{8, 10, 12},
			OpenAIStreamFirstTokenRectifierTimeouts:     []int{5, 8, 10},
		}})
		header, first := svc.GetOpenAIStreamRectifierTimeouts(t.Context())
		require.Equal(t, []int{5, 8, 10}, header)
		require.Equal(t, []int{6, 7, 9}, first)
	})
}

func TestAppendOpenAISettingsUpdates_PersistsProbeMethodAndWarmPoolFields(t *testing.T) {
	updates := map[string]string{}
	settings := &SystemSettings{
		EnableFingerprintUnification:                true,
		EnableMetadataPassthrough:                   true,
		EnableCCHSigning:                            true,
		EnableOpenAIStreamRectifier:                 false,
		OpenAIStreamResponseHeaderRectifierTimeouts: []int{11, 13},
		OpenAIStreamFirstTokenRectifierTimeouts:     []int{7, 9},
		OpenAIUsageProbeMethod:                      "WHAM",
		OpenAIWarmPoolEnabled:                       true,
		OpenAIWarmPoolBucketTargetSize:              10,
		OpenAIWarmPoolBucketRefillBelow:             3,
		OpenAIWarmPoolBucketSyncFillMin:             1,
		OpenAIWarmPoolBucketEntryTTLSeconds:         30,
		OpenAIWarmPoolBucketRefillCooldownSeconds:   15,
		OpenAIWarmPoolBucketRefillIntervalSeconds:   30,
		OpenAIWarmPoolGlobalTargetSize:              30,
		OpenAIWarmPoolGlobalRefillBelow:             10,
		OpenAIWarmPoolGlobalEntryTTLSeconds:         300,
		OpenAIWarmPoolGlobalRefillCooldownSeconds:   60,
		OpenAIWarmPoolGlobalRefillIntervalSeconds:   300,
		OpenAIWarmPoolNetworkErrorPoolSize:          3,
		OpenAIWarmPoolNetworkErrorEntryTTLSeconds:   120,
		OpenAIWarmPoolProbeMaxCandidates:            24,
		OpenAIWarmPoolProbeConcurrency:              4,
		OpenAIWarmPoolProbeTimeoutSeconds:           15,
		OpenAIWarmPoolProbeFailureCooldownSeconds:   120,
		OpenAIWarmPoolStartupGroupIDs:               []int64{9, 3, 9},
	}

	require.NoError(t, appendOpenAISettingsUpdates(updates, settings))
	require.Equal(t, "wham", updates[SettingKeyOpenAIUsageProbeMethod])
	require.Equal(t, `[11,13]`, updates[SettingKeyOpenAIStreamResponseHeaderRectifierTimeouts])
	require.Equal(t, `[7,9]`, updates[SettingKeyOpenAIStreamFirstTokenRectifierTimeouts])
	require.Equal(t, "3,9", updates[SettingKeyOpenAIWarmPoolStartupGroupIDs])
}

func TestApplyOpenAISettingsFromMap_LoadsGatewayAndWarmPoolFields(t *testing.T) {
	result := &SystemSettings{}
	settings := map[string]string{
		SettingKeyEnableFingerprintUnification:                "false",
		SettingKeyEnableMetadataPassthrough:                   "true",
		SettingKeyEnableCCHSigning:                            "true",
		SettingKeyEnableOpenAIStreamRectifier:                 "false",
		SettingKeyOpenAIStreamResponseHeaderRectifierTimeouts: `[11,13]`,
		SettingKeyOpenAIStreamFirstTokenRectifierTimeouts:     `[7,9]`,
		SettingKeyOpenAIUsageProbeMethod:                      "WHAM",
		SettingKeyOpenAIWarmPoolEnabled:                       "true",
		SettingKeyOpenAIWarmPoolBucketTargetSize:              "10",
		SettingKeyOpenAIWarmPoolBucketRefillBelow:             "3",
		SettingKeyOpenAIWarmPoolBucketSyncFillMin:             "1",
		SettingKeyOpenAIWarmPoolBucketEntryTTLSeconds:         "30",
		SettingKeyOpenAIWarmPoolBucketRefillCooldownSeconds:   "15",
		SettingKeyOpenAIWarmPoolBucketRefillIntervalSeconds:   "30",
		SettingKeyOpenAIWarmPoolGlobalTargetSize:              "30",
		SettingKeyOpenAIWarmPoolGlobalRefillBelow:             "10",
		SettingKeyOpenAIWarmPoolGlobalEntryTTLSeconds:         "300",
		SettingKeyOpenAIWarmPoolGlobalRefillCooldownSeconds:   "60",
		SettingKeyOpenAIWarmPoolGlobalRefillIntervalSeconds:   "300",
		SettingKeyOpenAIWarmPoolNetworkErrorPoolSize:          "3",
		SettingKeyOpenAIWarmPoolNetworkErrorEntryTTLSeconds:   "120",
		SettingKeyOpenAIWarmPoolProbeMaxCandidates:            "24",
		SettingKeyOpenAIWarmPoolProbeConcurrency:              "4",
		SettingKeyOpenAIWarmPoolProbeTimeoutSeconds:           "15",
		SettingKeyOpenAIWarmPoolProbeFailureCooldownSeconds:   "120",
		SettingKeyOpenAIWarmPoolStartupGroupIDs:               "9,3,9",
	}

	applyOpenAISettingsFromMap(result, settings, &config.Config{})
	require.False(t, result.EnableFingerprintUnification)
	require.True(t, result.EnableMetadataPassthrough)
	require.True(t, result.EnableCCHSigning)
	require.False(t, result.EnableOpenAIStreamRectifier)
	require.Equal(t, []int{11, 13}, result.OpenAIStreamResponseHeaderRectifierTimeouts)
	require.Equal(t, []int{7, 9}, result.OpenAIStreamFirstTokenRectifierTimeouts)
	require.Equal(t, "wham", result.OpenAIUsageProbeMethod)
	require.True(t, result.OpenAIWarmPoolEnabled)
	require.Equal(t, []int64{3, 9}, result.OpenAIWarmPoolStartupGroupIDs)
}
