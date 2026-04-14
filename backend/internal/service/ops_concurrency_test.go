package service

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

type countingOpsConcurrencyCache struct {
	stubConcurrencyCache
	loadBatchCalls atomic.Int32
}

func (c *countingOpsConcurrencyCache) GetAccountsLoadBatch(ctx context.Context, accounts []AccountWithConcurrency) (map[int64]*AccountLoadInfo, error) {
	c.loadBatchCalls.Add(1)
	return c.stubConcurrencyCache.GetAccountsLoadBatch(ctx, accounts)
}

type countingOpsRealtimeAccountRepo struct {
	stubOpenAIAccountRepo
	opsRealtimeCalls atomic.Int32
	schedulableCalls atomic.Int32
}

type partialHitOpsRealtimeCache struct {
	stubOpsRealtimeCache
	listedIDs []int64
}

func (r *countingOpsRealtimeAccountRepo) ListOpsRealtimeAccounts(ctx context.Context, platformFilter string, groupIDFilter *int64) ([]Account, error) {
	r.opsRealtimeCalls.Add(1)
	result := make([]Account, 0, len(r.accounts))
	for _, acc := range r.accounts {
		if platformFilter != "" && acc.Platform != platformFilter {
			continue
		}
		if groupIDFilter != nil && *groupIDFilter > 0 && !opsRealtimeAccountMatchesGroup(acc, *groupIDFilter) {
			continue
		}
		result = append(result, acc)
	}
	return result, nil
}

func (r *countingOpsRealtimeAccountRepo) ListSchedulable(ctx context.Context) ([]Account, error) {
	r.schedulableCalls.Add(1)
	return r.stubOpenAIAccountRepo.ListSchedulable(ctx)
}

func (r *countingOpsRealtimeAccountRepo) ListSchedulableByPlatform(ctx context.Context, platform string) ([]Account, error) {
	r.schedulableCalls.Add(1)
	return r.stubOpenAIAccountRepo.ListSchedulableByPlatform(ctx, platform)
}

func (r *countingOpsRealtimeAccountRepo) ListSchedulableByGroupID(ctx context.Context, groupID int64) ([]Account, error) {
	r.schedulableCalls.Add(1)
	return r.stubOpenAIAccountRepo.ListSchedulableByGroupID(ctx, groupID)
}

func (r *countingOpsRealtimeAccountRepo) ListSchedulableByGroupIDAndPlatform(ctx context.Context, groupID int64, platform string) ([]Account, error) {
	r.schedulableCalls.Add(1)
	return r.stubOpenAIAccountRepo.ListSchedulableByGroupIDAndPlatform(ctx, groupID, platform)
}

func (c *partialHitOpsRealtimeCache) ListAccountIDs(ctx context.Context, platformFilter string, groupIDFilter *int64) ([]int64, error) {
	if len(c.listedIDs) > 0 {
		return append([]int64(nil), c.listedIDs...), nil
	}
	return c.stubOpsRealtimeCache.ListAccountIDs(ctx, platformFilter, groupIDFilter)
}

func opsRealtimeAccountMatchesGroup(acc Account, groupID int64) bool {
	for _, grp := range acc.Groups {
		if grp != nil && grp.ID == groupID {
			return true
		}
	}
	for _, id := range acc.GroupIDs {
		if id == groupID {
			return true
		}
	}
	for _, ag := range acc.AccountGroups {
		if ag.GroupID == groupID {
			return true
		}
	}
	return false
}

func TestOpsServiceRealtimeAccountsCache_ReusedAcrossConcurrencyAndAvailability(t *testing.T) {
	ctx := context.Background()
	cfg := newOpenAIWarmPoolTestConfig()
	cfg.Ops.Enabled = true
	group := &Group{ID: 9401, Name: "OpenAI Group", Platform: PlatformOpenAI}
	accounts := []Account{
		{ID: 94001, Name: "acc-1", Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 2, Groups: []*Group{group}},
		{ID: 94002, Name: "acc-2", Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Groups: []*Group{group}},
	}
	repo := &countingOpsRealtimeAccountRepo{stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: accounts}}
	svc := &OpsService{
		cfg:                cfg,
		accountRepo:        repo,
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{}),
	}

	scope := NormalizeOpsRealtimeScope("group", PlatformOpenAI, nil, false)
	_, _, _, _, err := svc.GetConcurrencyStatsWithOptions(ctx, PlatformOpenAI, nil, scope)
	require.NoError(t, err)
	_, _, _, _, err = svc.GetAccountAvailabilityStatsWithOptions(ctx, PlatformOpenAI, nil, scope)
	require.NoError(t, err)
	require.EqualValues(t, 1, repo.opsRealtimeCalls.Load(), "并发/可用性接口应复用同一份账号快照")
}

func TestOpsServiceGetConcurrencyAndAvailabilityStats_ScopeTrimsUnusedDimensions(t *testing.T) {
	ctx := context.Background()
	cfg := newOpenAIWarmPoolTestConfig()
	cfg.Ops.Enabled = true
	group := &Group{ID: 9501, Name: "Scoped Group", Platform: PlatformOpenAI}
	accounts := []Account{{
		ID:          95001,
		Name:        "scoped-account",
		Platform:    PlatformOpenAI,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 3,
		Groups:      []*Group{group},
	}}
	repo := &countingOpsRealtimeAccountRepo{stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: accounts}}
	svc := &OpsService{
		cfg:                cfg,
		accountRepo:        repo,
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{}),
	}

	platformScope := NormalizeOpsRealtimeScope("platform", "", nil, false)
	platformStats, groupStats, accountStats, _, err := svc.GetConcurrencyStatsWithOptions(ctx, "", nil, platformScope)
	require.NoError(t, err)
	require.Len(t, platformStats, 1)
	require.Empty(t, groupStats)
	require.Empty(t, accountStats)

	platformAvailability, groupAvailability, accountAvailability, _, err := svc.GetAccountAvailabilityStatsWithOptions(ctx, "", nil, platformScope)
	require.NoError(t, err)
	require.Len(t, platformAvailability, 1)
	require.Empty(t, groupAvailability)
	require.Empty(t, accountAvailability)

	accountScope := NormalizeOpsRealtimeScope("account", PlatformOpenAI, &group.ID, true)
	platformStats, groupStats, accountStats, _, err = svc.GetConcurrencyStatsWithOptions(ctx, PlatformOpenAI, &group.ID, accountScope)
	require.NoError(t, err)
	require.Empty(t, platformStats)
	require.Empty(t, groupStats)
	require.Len(t, accountStats, 1)

	platformAvailability, groupAvailability, accountAvailability, _, err = svc.GetAccountAvailabilityStatsWithOptions(ctx, PlatformOpenAI, &group.ID, accountScope)
	require.NoError(t, err)
	require.Empty(t, platformAvailability)
	require.Empty(t, groupAvailability)
	require.Len(t, accountAvailability, 1)
}

func TestOpsServiceGetConcurrencyStats_PlatformScopeUsesSchedulableAndCachesResult(t *testing.T) {
	ctx := context.Background()
	cfg := newOpenAIWarmPoolTestConfig()
	cfg.Ops.Enabled = true
	group := &Group{ID: 9601, Name: "Cached Group", Platform: PlatformOpenAI}
	accounts := []Account{
		{ID: 96001, Name: "schedulable", Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 3, Groups: []*Group{group}},
		{ID: 96002, Name: "unschedulable", Platform: PlatformOpenAI, Status: StatusActive, Schedulable: false, Concurrency: 50, Groups: []*Group{group}},
	}
	repo := &countingOpsRealtimeAccountRepo{stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: accounts}}
	cache := &countingOpsConcurrencyCache{stubConcurrencyCache: stubConcurrencyCache{skipDefaultLoad: true}}
	svc := &OpsService{
		cfg:                cfg,
		accountRepo:        repo,
		concurrencyService: NewConcurrencyService(cache),
	}

	scope := NormalizeOpsRealtimeScope("platform", "", nil, false)
	platformStats, groupStats, accountStats, _, err := svc.GetConcurrencyStatsWithOptions(ctx, "", nil, scope)
	require.NoError(t, err)
	require.Len(t, platformStats, 1)
	require.Empty(t, groupStats)
	require.Empty(t, accountStats)
	require.EqualValues(t, 3, platformStats[PlatformOpenAI].MaxCapacity)
	require.EqualValues(t, 1, repo.schedulableCalls.Load())
	require.EqualValues(t, 0, repo.opsRealtimeCalls.Load())
	require.EqualValues(t, 1, cache.loadBatchCalls.Load())

	platformStats, groupStats, accountStats, _, err = svc.GetConcurrencyStatsWithOptions(ctx, "", nil, scope)
	require.NoError(t, err)
	require.Len(t, platformStats, 1)
	require.Empty(t, groupStats)
	require.Empty(t, accountStats)
	require.EqualValues(t, 3, platformStats[PlatformOpenAI].MaxCapacity)
	require.EqualValues(t, 1, repo.schedulableCalls.Load(), "结果缓存命中后不应重复加载 schedulable 账号")
	require.EqualValues(t, 0, repo.opsRealtimeCalls.Load())
	require.EqualValues(t, 1, cache.loadBatchCalls.Load(), "结果缓存命中后不应重复查询并发负载")
}

func TestOpsServiceRealtimeAccountsCache_PartialHitFallsBackToRepo(t *testing.T) {
	ctx := context.Background()
	cfg := newOpenAIWarmPoolTestConfig()
	cfg.Ops.Enabled = true
	group := &Group{ID: 9701, Name: "Fallback Group", Platform: PlatformOpenAI}
	accounts := []Account{
		{ID: 97001, Name: "acc-1", Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 2, Groups: []*Group{group}},
		{ID: 97002, Name: "acc-2", Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Groups: []*Group{group}},
	}
	repo := &countingOpsRealtimeAccountRepo{stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: accounts}}
	opsCache := &partialHitOpsRealtimeCache{
		stubOpsRealtimeCache: stubOpsRealtimeCache{
			ready: true,
			accounts: map[int64]*OpsRealtimeAccountCacheEntry{
				97001: BuildOpsRealtimeAccountCacheEntry(&accounts[0]),
			},
		},
		listedIDs: []int64{97001, 97002},
	}
	svc := ProvideOpsService(
		&opsRepoMock{},
		nil,
		cfg,
		repo,
		nil,
		NewConcurrencyService(stubConcurrencyCache{}),
		nil,
		nil,
		nil,
		nil,
		opsCache,
		nil,
	)

	_, _, accountStats, _, err := svc.GetConcurrencyStatsWithOptions(ctx, PlatformOpenAI, &group.ID, opsRealtimeScopeAccount)
	require.NoError(t, err)
	require.Len(t, accountStats, 2, "Redis 账号索引局部缺项时应回退到仓储，而不是静默少账号")
	require.EqualValues(t, 1, repo.opsRealtimeCalls.Load())
}
