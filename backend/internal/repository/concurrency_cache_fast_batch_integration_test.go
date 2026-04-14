//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

type fastBatchFixture struct {
	id             int64
	maxConcurrency int
	activeSlots    int
	expiredSlots   int
	waitingCount   int
}

func TestConcurrencyCache_GetAccountsLoadBatchFast_MatchesLegacyBehavior(t *testing.T) {
	ctx := context.Background()
	rdb := testRedis(t)
	cache := newFastBatchTestCache(t, rdb)

	fixtures := []fastBatchFixture{
		{id: 5101, maxConcurrency: 5, activeSlots: 2, expiredSlots: 1, waitingCount: 1},
		{id: 5102, maxConcurrency: 4, activeSlots: 1, expiredSlots: 2, waitingCount: 0},
		{id: 5103, maxConcurrency: 0, activeSlots: 3, expiredSlots: 1, waitingCount: 2},
	}
	accounts := buildAccountWithConcurrencyInputs(fixtures)
	serverNow := redisServerTime(t, ctx, rdb)

	seedFastBatchFixture(t, ctx, rdb, serverNow, fixtures, accountSlotKey, accountWaitKey)
	fast, err := cache.GetAccountsLoadBatchFast(ctx, accounts)
	require.NoError(t, err)
	require.Equal(t, map[int64]*service.AccountLoadInfo{
		5101: &service.AccountLoadInfo{AccountID: 5101, CurrentConcurrency: 2, WaitingCount: 1, LoadRate: 60},
		5102: &service.AccountLoadInfo{AccountID: 5102, CurrentConcurrency: 1, WaitingCount: 0, LoadRate: 25},
		5103: &service.AccountLoadInfo{AccountID: 5103, CurrentConcurrency: 3, WaitingCount: 2, LoadRate: 0},
	}, fast)

	seedFastBatchFixture(t, ctx, rdb, serverNow, fixtures, accountSlotKey, accountWaitKey)
	legacy, err := cache.GetAccountsLoadBatch(ctx, accounts)
	require.NoError(t, err)
	require.Equal(t, fast, legacy)
}

func TestConcurrencyCache_GetUsersLoadBatchFast_MatchesLegacyBehavior(t *testing.T) {
	ctx := context.Background()
	rdb := testRedis(t)
	cache := newFastBatchTestCache(t, rdb)

	fixtures := []fastBatchFixture{
		{id: 6201, maxConcurrency: 5, activeSlots: 2, expiredSlots: 1, waitingCount: 1},
		{id: 6202, maxConcurrency: 4, activeSlots: 1, expiredSlots: 2, waitingCount: 0},
		{id: 6203, maxConcurrency: 0, activeSlots: 3, expiredSlots: 1, waitingCount: 2},
	}
	users := buildUserWithConcurrencyInputs(fixtures)
	serverNow := redisServerTime(t, ctx, rdb)

	seedFastBatchFixture(t, ctx, rdb, serverNow, fixtures, userSlotKey, waitQueueKey)
	fast, err := cache.GetUsersLoadBatchFast(ctx, users)
	require.NoError(t, err)
	require.Equal(t, map[int64]*service.UserLoadInfo{
		6201: &service.UserLoadInfo{UserID: 6201, CurrentConcurrency: 2, WaitingCount: 1, LoadRate: 60},
		6202: &service.UserLoadInfo{UserID: 6202, CurrentConcurrency: 1, WaitingCount: 0, LoadRate: 25},
		6203: &service.UserLoadInfo{UserID: 6203, CurrentConcurrency: 3, WaitingCount: 2, LoadRate: 0},
	}, fast)

	seedFastBatchFixture(t, ctx, rdb, serverNow, fixtures, userSlotKey, waitQueueKey)
	legacy, err := cache.GetUsersLoadBatch(ctx, users)
	require.NoError(t, err)
	require.Equal(t, fast, legacy)
}

func TestConcurrencyCache_GetAccountsLoadBatchFast_EmptyInput(t *testing.T) {
	rdb := testRedis(t)
	cache := newFastBatchTestCache(t, rdb)

	result, err := cache.GetAccountsLoadBatchFast(context.Background(), nil)
	require.NoError(t, err)
	require.Empty(t, result)
}

func TestConcurrencyCache_GetUsersLoadBatchFast_EmptyInput(t *testing.T) {
	rdb := testRedis(t)
	cache := newFastBatchTestCache(t, rdb)

	result, err := cache.GetUsersLoadBatchFast(context.Background(), nil)
	require.NoError(t, err)
	require.Empty(t, result)
}

func newFastBatchTestCache(t *testing.T, rdb *redis.Client) *concurrencyCache {
	t.Helper()
	cache, ok := NewConcurrencyCache(rdb, testSlotTTLMinutes, int(testSlotTTL.Seconds())).(*concurrencyCache)
	require.True(t, ok)
	return cache
}

func redisServerTime(t *testing.T, ctx context.Context, rdb *redis.Client) time.Time {
	t.Helper()

	now, err := rdb.Time(ctx).Result()
	require.NoError(t, err)
	return now
}

func seedFastBatchFixture(t *testing.T, ctx context.Context, rdb *redis.Client, now time.Time, fixtures []fastBatchFixture, slotKeyFn func(int64) string, waitKeyFn func(int64) string) {
	t.Helper()

	activeScore := float64(now.Unix())
	expiredScore := float64(now.Unix() - int64(testSlotTTL.Seconds()) - 10)

	for _, fx := range fixtures {
		slotKey := slotKeyFn(fx.id)
		waitKey := waitKeyFn(fx.id)
		require.NoError(t, rdb.Del(ctx, slotKey, waitKey).Err())

		members := make([]redis.Z, 0, fx.activeSlots+fx.expiredSlots)
		for i := 0; i < fx.activeSlots; i++ {
			members = append(members, redis.Z{
				Score:  activeScore,
				Member: fmt.Sprintf("active-%d-%d", fx.id, i),
			})
		}
		for i := 0; i < fx.expiredSlots; i++ {
			members = append(members, redis.Z{
				Score:  expiredScore,
				Member: fmt.Sprintf("expired-%d-%d", fx.id, i),
			})
		}
		if len(members) > 0 {
			require.NoError(t, rdb.ZAdd(ctx, slotKey, members...).Err())
			require.NoError(t, rdb.Expire(ctx, slotKey, testSlotTTL).Err())
		}

		if fx.waitingCount > 0 {
			require.NoError(t, rdb.Set(ctx, waitKey, fx.waitingCount, testSlotTTL).Err())
		}
	}
}

func buildAccountWithConcurrencyInputs(fixtures []fastBatchFixture) []service.AccountWithConcurrency {
	accounts := make([]service.AccountWithConcurrency, 0, len(fixtures))
	for _, fx := range fixtures {
		accounts = append(accounts, service.AccountWithConcurrency{ID: fx.id, MaxConcurrency: fx.maxConcurrency})
	}
	return accounts
}

func buildUserWithConcurrencyInputs(fixtures []fastBatchFixture) []service.UserWithConcurrency {
	users := make([]service.UserWithConcurrency, 0, len(fixtures))
	for _, fx := range fixtures {
		users = append(users, service.UserWithConcurrency{ID: fx.id, MaxConcurrency: fx.maxConcurrency})
	}
	return users
}
