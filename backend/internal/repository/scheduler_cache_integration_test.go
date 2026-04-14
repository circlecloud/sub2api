//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestSchedulerCacheSetSnapshotSetsTTLAndPrunesInactiveBucket(t *testing.T) {
	ctx := context.Background()
	rdb := testRedis(t)
	cache := NewSchedulerCache(rdb).(*schedulerCache)
	bucket := service.SchedulerBucket{GroupID: 321, Platform: service.PlatformAnthropic, Mode: service.SchedulerModeSingle}
	account := service.Account{ID: 900001, Name: "ttl-account", Platform: service.PlatformAnthropic, Status: service.StatusActive, Schedulable: true}

	require.NoError(t, cache.SetSnapshot(ctx, bucket, []service.Account{account}))

	activeTTL := rdb.TTL(ctx, schedulerBucketKey(schedulerActivePrefix, bucket)).Val()
	readyTTL := rdb.TTL(ctx, schedulerBucketKey(schedulerReadyPrefix, bucket)).Val()
	versionTTL := rdb.TTL(ctx, schedulerBucketKey(schedulerVersionPrefix, bucket)).Val()
	accountTTL := rdb.TTL(ctx, schedulerAccountKey("900001")).Val()
	require.Greater(t, activeTTL, 0*time.Second)
	require.Greater(t, readyTTL, 0*time.Second)
	require.Greater(t, versionTTL, 0*time.Second)
	require.Greater(t, accountTTL, 0*time.Second)

	members := rdb.SMembers(ctx, schedulerBucketSetKey).Val()
	require.Contains(t, members, bucket.String())

	require.NoError(t, rdb.Del(ctx,
		schedulerBucketKey(schedulerActivePrefix, bucket),
		schedulerBucketKey(schedulerReadyPrefix, bucket),
		schedulerBucketKey(schedulerVersionPrefix, bucket),
	).Err())
	require.NoError(t, cache.pruneInactiveBucket(ctx, bucket))
	members = rdb.SMembers(ctx, schedulerBucketSetKey).Val()
	require.NotContains(t, members, bucket.String())
}

func TestSchedulerCacheSetSnapshotReadFlowWithoutSeparateAccountWrite(t *testing.T) {
	ctx := context.Background()
	rdb := testRedis(t)
	cache := NewSchedulerCache(rdb).(*schedulerCache)
	bucket := service.SchedulerBucket{GroupID: 654, Platform: service.PlatformOpenAI, Mode: service.SchedulerModeForced}
	accounts := make([]service.Account, 0, schedulerCompactSnapshotWriteChunkSize+5)
	for i := 0; i < schedulerCompactSnapshotWriteChunkSize+5; i++ {
		accounts = append(accounts, service.Account{
			ID:          900002 + int64(i),
			Name:        fmt.Sprintf("read-flow-account-%d", i),
			Platform:    service.PlatformOpenAI,
			Status:      service.StatusActive,
			Schedulable: true,
		})
	}

	require.NoError(t, cache.SetSnapshot(ctx, bucket, accounts))
	accounts[0].Name = "updated-first"
	accounts[len(accounts)-1].Name = "updated-last"
	require.NoError(t, cache.SetSnapshot(ctx, bucket, accounts))

	snapshot, hit, err := cache.GetSnapshot(ctx, bucket)
	require.NoError(t, err)
	require.True(t, hit)
	require.Len(t, snapshot, len(accounts))
	require.Equal(t, accounts[0].ID, snapshot[0].ID)
	require.Equal(t, accounts[0].Name, snapshot[0].Name)
	require.Equal(t, accounts[len(accounts)-1].ID, snapshot[len(snapshot)-1].ID)
	require.Equal(t, accounts[len(accounts)-1].Name, snapshot[len(snapshot)-1].Name)
}

func TestSchedulerCacheSetSnapshotOpenAIUsesCompactPayloadWithoutSingleAccountWrite(t *testing.T) {
	ctx := context.Background()
	rdb := testRedis(t)
	cache := NewSchedulerCache(rdb).(*schedulerCache)
	bucket := service.SchedulerBucket{GroupID: 655, Platform: service.PlatformOpenAI, Mode: service.SchedulerModeSingle}
	accounts := []service.Account{{
		ID:          900111,
		Name:        "compact-openai-account",
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeOAuth,
		Status:      service.StatusActive,
		Schedulable: true,
		Credentials: map[string]any{
			"model_mapping": map[string]any{"gpt-5.4": "gpt-5.4"},
			"access_token":  "secret-token",
		},
		Extra: map[string]any{
			"privacy_mode": service.PrivacyModeTrainingOff,
			"openai_oauth_responses_websockets_v2_mode": service.OpenAIWSIngressModeCtxPool,
		},
	}}

	require.NoError(t, cache.SetSnapshot(ctx, bucket, accounts))

	metaExists, err := rdb.Exists(ctx, schedulerCompactSnapshotMetaKey(bucket, "1")).Result()
	require.NoError(t, err)
	require.EqualValues(t, 1, metaExists)
	accountExists, err := rdb.Exists(ctx, schedulerAccountKey("900111")).Result()
	require.NoError(t, err)
	require.Zero(t, accountExists, "OpenAI 紧凑快照不应顺手写入完整单账号缓存")

	snapshot, hit, err := cache.GetSnapshot(ctx, bucket)
	require.NoError(t, err)
	require.True(t, hit)
	require.Len(t, snapshot, 1)
	require.True(t, snapshot[0].IsModelSupported("gpt-5.4"))
	require.True(t, snapshot[0].IsPrivacySet())
	require.Empty(t, snapshot[0].GetCredential("access_token"))
}

func TestSchedulerCacheSetOutboxWatermarkSetsTTL(t *testing.T) {
	ctx := context.Background()
	rdb := testRedis(t)
	cache := NewSchedulerCache(rdb).(*schedulerCache)

	require.NoError(t, cache.SetOutboxWatermark(ctx, 12345))
	ttl := rdb.TTL(ctx, schedulerOutboxWatermarkKey).Val()
	require.Greater(t, ttl, 0*time.Second)
}
