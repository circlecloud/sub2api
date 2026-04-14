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
