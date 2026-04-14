package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestSettingService_OpenAIWarmPoolStartupGroupIDsPersistAndLoad(t *testing.T) {
	repo := &warmPoolSettingRepoStub{values: map[string]string{
		SettingKeyOpenAIWarmPoolStartupGroupIDs: "5,2,5",
	}}
	svc := NewSettingService(repo, &config.Config{})
	openAIWarmPoolSF.Forget("openai_warm_pool")
	openAIWarmPoolCache.Store((*cachedOpenAIWarmPoolSettings)(nil))
	defer func() {
		openAIWarmPoolSF.Forget("openai_warm_pool")
		openAIWarmPoolCache.Store((*cachedOpenAIWarmPoolSettings)(nil))
	}()

	initial := svc.GetOpenAIWarmPoolSettings(context.Background())
	require.Equal(t, []int64{2, 5}, initial.StartupGroupIDs)

	err := svc.UpdateSettings(context.Background(), &SystemSettings{
		OpenAIWarmPoolEnabled:                     true,
		OpenAIWarmPoolBucketTargetSize:            10,
		OpenAIWarmPoolBucketRefillBelow:           3,
		OpenAIWarmPoolBucketSyncFillMin:           1,
		OpenAIWarmPoolBucketEntryTTLSeconds:       30,
		OpenAIWarmPoolBucketRefillCooldownSeconds: 15,
		OpenAIWarmPoolBucketRefillIntervalSeconds: 30,
		OpenAIWarmPoolGlobalTargetSize:            30,
		OpenAIWarmPoolGlobalRefillBelow:           10,
		OpenAIWarmPoolGlobalEntryTTLSeconds:       300,
		OpenAIWarmPoolGlobalRefillCooldownSeconds: 60,
		OpenAIWarmPoolGlobalRefillIntervalSeconds: 300,
		OpenAIWarmPoolNetworkErrorPoolSize:        3,
		OpenAIWarmPoolNetworkErrorEntryTTLSeconds: 120,
		OpenAIWarmPoolProbeMaxCandidates:          24,
		OpenAIWarmPoolProbeConcurrency:            4,
		OpenAIWarmPoolProbeTimeoutSeconds:         15,
		OpenAIWarmPoolProbeFailureCooldownSeconds: 120,
		OpenAIWarmPoolStartupGroupIDs:             []int64{9, 3, 9},
	})
	require.NoError(t, err)
	require.Equal(t, "3,9", repo.values[SettingKeyOpenAIWarmPoolStartupGroupIDs])

	updated := svc.GetOpenAIWarmPoolSettings(context.Background())
	require.Equal(t, []int64{3, 9}, updated.StartupGroupIDs)
}
