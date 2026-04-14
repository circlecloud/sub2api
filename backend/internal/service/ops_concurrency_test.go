package service

import (
	"context"
	"sort"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type countingOpsConcurrencyCache struct {
	stubConcurrencyCache
	loadBatchCalls          atomic.Int32
	loadBatchFastCalls      atomic.Int32
	usersLoadBatchCalls     atomic.Int32
	usersLoadBatchFastCalls atomic.Int32
}

func (c *countingOpsConcurrencyCache) GetAccountsLoadBatch(ctx context.Context, accounts []AccountWithConcurrency) (map[int64]*AccountLoadInfo, error) {
	c.loadBatchCalls.Add(1)
	return c.stubConcurrencyCache.GetAccountsLoadBatch(ctx, accounts)
}

func (c *countingOpsConcurrencyCache) GetAccountsLoadBatchFast(ctx context.Context, accounts []AccountWithConcurrency) (map[int64]*AccountLoadInfo, error) {
	c.loadBatchFastCalls.Add(1)
	return c.stubConcurrencyCache.GetAccountsLoadBatch(ctx, accounts)
}

func (c *countingOpsConcurrencyCache) GetUsersLoadBatch(ctx context.Context, users []UserWithConcurrency) (map[int64]*UserLoadInfo, error) {
	c.usersLoadBatchCalls.Add(1)
	return c.stubConcurrencyCache.GetUsersLoadBatch(ctx, users)
}

func (c *countingOpsConcurrencyCache) GetUsersLoadBatchFast(ctx context.Context, users []UserWithConcurrency) (map[int64]*UserLoadInfo, error) {
	c.usersLoadBatchFastCalls.Add(1)
	return c.stubConcurrencyCache.GetUsersLoadBatch(ctx, users)
}

func (c *countingOpsConcurrencyCache) accountLoadCalls() int32 {
	return c.loadBatchCalls.Load() + c.loadBatchFastCalls.Load()
}

//nolint:unused // 预留给后续用户并发加载断言
func (c *countingOpsConcurrencyCache) userLoadCalls() int32 {
	return c.usersLoadBatchCalls.Load() + c.usersLoadBatchFastCalls.Load()
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

type blockingOpsRealtimeMirrorCache struct {
	stubOpsRealtimeCache
	listCalls        atomic.Int32
	getCalls         atomic.Int32
	listStarted      chan struct{}
	releaseFirstList chan struct{}
}

type countingOpsRealtimeUserRepo struct {
	UserRepository
	users                []User
	listOpsCalls         atomic.Int32
	listWithFiltersCalls atomic.Int32
}

var _ UserRepository = (*countingOpsRealtimeUserRepo)(nil)

func (r *countingOpsRealtimeUserRepo) ListOpsRealtimeUsers(ctx context.Context) ([]User, error) {
	r.listOpsCalls.Add(1)
	return append([]User(nil), r.users...), nil
}

func (r *countingOpsRealtimeUserRepo) ListWithFilters(ctx context.Context, params pagination.PaginationParams, filters UserListFilters) ([]User, *pagination.PaginationResult, error) {
	r.listWithFiltersCalls.Add(1)
	out := append([]User(nil), r.users...)
	return out, &pagination.PaginationResult{Page: params.Page, PageSize: params.PageSize, Total: int64(len(out))}, nil
}

type countingLegacyOpsUserRepo struct {
	UserRepository
	users                []User
	listWithFiltersCalls atomic.Int32
}

var _ UserRepository = (*countingLegacyOpsUserRepo)(nil)

func (r *countingLegacyOpsUserRepo) ListWithFilters(ctx context.Context, params pagination.PaginationParams, filters UserListFilters) ([]User, *pagination.PaginationResult, error) {
	r.listWithFiltersCalls.Add(1)
	out := append([]User(nil), r.users...)
	return out, &pagination.PaginationResult{Page: params.Page, PageSize: params.PageSize, Total: int64(len(out))}, nil
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

func (c *blockingOpsRealtimeMirrorCache) ListAccountIDs(ctx context.Context, platformFilter string, groupIDFilter *int64) ([]int64, error) {
	call := c.listCalls.Add(1)
	if call == 1 {
		if c.listStarted != nil {
			close(c.listStarted)
		}
		if c.releaseFirstList != nil {
			<-c.releaseFirstList
		}
	}
	ids := make([]int64, 0, len(c.accounts))
	for accountID, entry := range c.accounts {
		if entry == nil {
			continue
		}
		if platformFilter != "" && entry.Platform != platformFilter {
			continue
		}
		if groupIDFilter != nil && *groupIDFilter > 0 && !opsRealtimeEntryHasGroup(entry, *groupIDFilter) {
			continue
		}
		ids = append(ids, accountID)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids, nil
}

func (c *blockingOpsRealtimeMirrorCache) GetAccounts(ctx context.Context, accountIDs []int64) (map[int64]*OpsRealtimeAccountCacheEntry, error) {
	c.getCalls.Add(1)
	result := make(map[int64]*OpsRealtimeAccountCacheEntry, len(accountIDs))
	for _, accountID := range accountIDs {
		if entry := c.accounts[accountID]; entry != nil {
			result[accountID] = entry
		}
	}
	return result, nil
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

func TestOpsServiceRealtimeMirrorSharedSnapshot_DedupesConcurrentConcurrencyAndAvailability(t *testing.T) {
	ctx := context.Background()
	cfg := newOpenAIWarmPoolTestConfig()
	cfg.Ops.Enabled = true
	group := &Group{ID: 9411, Name: "Mirror Group", Platform: PlatformOpenAI}
	cache := &blockingOpsRealtimeMirrorCache{
		stubOpsRealtimeCache: stubOpsRealtimeCache{
			ready: true,
			accounts: map[int64]*OpsRealtimeAccountCacheEntry{
				94101: BuildOpsRealtimeAccountCacheEntry(&Account{ID: 94101, Name: "acc-1", Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 2, Groups: []*Group{group}, GroupIDs: []int64{group.ID}}),
				94102: BuildOpsRealtimeAccountCacheEntry(&Account{ID: 94102, Name: "acc-2", Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Groups: []*Group{group}, GroupIDs: []int64{group.ID}}),
			},
		},
		listStarted:      make(chan struct{}),
		releaseFirstList: make(chan struct{}),
	}
	svc := ProvideOpsService(
		&opsRepoMock{},
		nil,
		cfg,
		stubOpenAIAccountRepo{},
		nil,
		NewConcurrencyService(stubConcurrencyCache{}),
		nil,
		nil,
		nil,
		nil,
		cache,
		nil,
	)

	errCh := make(chan error, 2)
	go func() {
		_, _, _, _, err := svc.GetConcurrencyStatsWithOptions(ctx, "", nil, NormalizeOpsRealtimeScope("platform", "", nil, false))
		errCh <- err
	}()
	<-cache.listStarted
	go func() {
		_, _, _, _, err := svc.GetAccountAvailabilityStatsWithOptions(ctx, "", nil, NormalizeOpsRealtimeScope("platform", "", nil, false))
		errCh <- err
	}()
	require.Never(t, func() bool { return cache.listCalls.Load() > 1 }, 50*time.Millisecond, time.Millisecond, "共享快照 singleflight 生效时，第二个并发请求不应在首个请求未完成前再次扫描 Redis 账号索引")
	close(cache.releaseFirstList)

	require.NoError(t, <-errCh)
	require.NoError(t, <-errCh)
	require.EqualValues(t, 1, cache.listCalls.Load(), "冷启动并发请求应共享同一轮 account ID 扫描")
	require.EqualValues(t, 1, cache.getCalls.Load(), "冷启动并发请求应共享同一轮 account 详情加载")
}

func TestOpsServiceGetAccountAvailabilityStats_CachesResultByFilterAndScope(t *testing.T) {
	ctx := context.Background()
	cfg := newOpenAIWarmPoolTestConfig()
	cfg.Ops.Enabled = true
	group := &Group{ID: 9421, Name: "Availability Group", Platform: PlatformOpenAI}
	cache := &stubOpsRealtimeCache{
		ready: true,
		accounts: map[int64]*OpsRealtimeAccountCacheEntry{
			94201: BuildOpsRealtimeAccountCacheEntry(&Account{ID: 94201, Name: "acc-1", Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 2, Groups: []*Group{group}, GroupIDs: []int64{group.ID}}),
		},
	}
	svc := ProvideOpsService(
		&opsRepoMock{},
		nil,
		cfg,
		stubOpenAIAccountRepo{},
		nil,
		NewConcurrencyService(stubConcurrencyCache{}),
		nil,
		nil,
		nil,
		nil,
		cache,
		nil,
	)

	scope := NormalizeOpsRealtimeScope("account", PlatformOpenAI, &group.ID, true)
	_, _, accountStats, collectedAt, err := svc.GetAccountAvailabilityStatsWithOptions(ctx, PlatformOpenAI, &group.ID, scope)
	require.NoError(t, err)
	require.NotNil(t, collectedAt)
	require.Len(t, accountStats, 1)

	cache.accounts = map[int64]*OpsRealtimeAccountCacheEntry{}
	_, _, accountStats2, collectedAt2, err := svc.GetAccountAvailabilityStatsWithOptions(ctx, PlatformOpenAI, &group.ID, scope)
	require.NoError(t, err)
	require.Len(t, accountStats2, 1, "短 TTL 结果缓存命中后应直接复用上一轮可用性聚合结果")
	require.True(t, collectedAt == collectedAt2, "结果缓存命中后应复用同一份 collectedAt 指针")
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
	require.EqualValues(t, 1, cache.accountLoadCalls())

	platformStats, groupStats, accountStats, _, err = svc.GetConcurrencyStatsWithOptions(ctx, "", nil, scope)
	require.NoError(t, err)
	require.Len(t, platformStats, 1)
	require.Empty(t, groupStats)
	require.Empty(t, accountStats)
	require.EqualValues(t, 3, platformStats[PlatformOpenAI].MaxCapacity)
	require.EqualValues(t, 1, repo.schedulableCalls.Load(), "结果缓存命中后不应重复加载 schedulable 账号")
	require.EqualValues(t, 0, repo.opsRealtimeCalls.Load())
	require.EqualValues(t, 1, cache.accountLoadCalls(), "结果缓存命中后不应重复查询并发负载")
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

func TestOpsServiceGetConcurrencyStats_AccountScopeCachesResultAndUsesFastLoadPath(t *testing.T) {
	ctx := context.Background()
	cfg := newOpenAIWarmPoolTestConfig()
	cfg.Ops.Enabled = true
	group := &Group{ID: 9811, Name: "Account Scope Group", Platform: PlatformOpenAI}
	accounts := []Account{
		{ID: 98101, Name: "acc-1", Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 3, Groups: []*Group{group}},
		{ID: 98102, Name: "acc-2", Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 4, Groups: []*Group{group}},
	}
	repo := &countingOpsRealtimeAccountRepo{stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: accounts}}
	cache := &countingOpsConcurrencyCache{
		stubConcurrencyCache: stubConcurrencyCache{
			loadMap: map[int64]*AccountLoadInfo{
				98101: &AccountLoadInfo{AccountID: 98101, CurrentConcurrency: 2, WaitingCount: 1},
				98102: &AccountLoadInfo{AccountID: 98102, CurrentConcurrency: 0, WaitingCount: 0},
			},
			skipDefaultLoad: true,
		},
	}
	svc := &OpsService{
		cfg:                cfg,
		accountRepo:        repo,
		concurrencyService: NewConcurrencyService(cache),
	}

	platformStats, groupStats, accountStats, _, err := svc.GetConcurrencyStatsWithOptions(ctx, PlatformOpenAI, &group.ID, opsRealtimeScopeAccount)
	require.NoError(t, err)
	require.Empty(t, platformStats)
	require.Empty(t, groupStats)
	require.Len(t, accountStats, 2)
	require.EqualValues(t, 2, accountStats[98101].CurrentInUse)
	require.EqualValues(t, 1, accountStats[98101].WaitingInQueue)
	require.EqualValues(t, 1, repo.opsRealtimeCalls.Load())
	require.EqualValues(t, 1, cache.loadBatchFastCalls.Load(), "account scope should prefer the fast path")
	require.EqualValues(t, 0, cache.loadBatchCalls.Load(), "account scope fast path should avoid the cleanup read path")

	platformStats, groupStats, accountStats, _, err = svc.GetConcurrencyStatsWithOptions(ctx, PlatformOpenAI, &group.ID, opsRealtimeScopeAccount)
	require.NoError(t, err)
	require.Empty(t, platformStats)
	require.Empty(t, groupStats)
	require.Len(t, accountStats, 2)
	require.EqualValues(t, 2, accountStats[98101].CurrentInUse)
	require.EqualValues(t, 1, accountStats[98101].WaitingInQueue)
	require.EqualValues(t, 1, repo.opsRealtimeCalls.Load(), "account scope result cache should skip the repo on hit")
	require.EqualValues(t, 1, cache.loadBatchFastCalls.Load(), "account scope result cache should skip load lookups on hit")
	require.EqualValues(t, 0, cache.loadBatchCalls.Load())
}

func TestOpsServiceGetUserConcurrencyStats_CachesResultAndPrefersLightweightLister(t *testing.T) {
	ctx := context.Background()
	cfg := newOpenAIWarmPoolTestConfig()
	cfg.Ops.Enabled = true
	users := []User{
		{ID: 99201, Email: "user1@example.com", Username: "user1", Concurrency: 4, Status: StatusActive},
		{ID: 99202, Email: "user2@example.com", Username: "user2", Concurrency: 2, Status: StatusActive},
	}
	userRepo := &countingOpsRealtimeUserRepo{users: users}
	cache := &countingOpsConcurrencyCache{
		stubConcurrencyCache: stubConcurrencyCache{
			usersLoadBatch: map[int64]*UserLoadInfo{
				99201: &UserLoadInfo{UserID: 99201, CurrentConcurrency: 2, WaitingCount: 1},
				99202: &UserLoadInfo{UserID: 99202, CurrentConcurrency: 1, WaitingCount: 0},
			},
			skipDefaultLoad: true,
		},
	}
	svc := &OpsService{
		cfg:                cfg,
		userRepo:           userRepo,
		concurrencyService: NewConcurrencyService(cache),
	}

	stats, collectedAt, err := svc.GetUserConcurrencyStats(ctx)
	require.NoError(t, err)
	require.NotNil(t, collectedAt)
	require.Len(t, stats, 2)
	require.EqualValues(t, 2, stats[99201].CurrentInUse)
	require.EqualValues(t, 1, stats[99201].WaitingInQueue)
	require.EqualValues(t, 1, userRepo.listOpsCalls.Load(), "lightweight user lister should be preferred")
	require.EqualValues(t, 0, userRepo.listWithFiltersCalls.Load(), "ops user concurrency should not go through the heavy filtered list path")
	require.EqualValues(t, 1, cache.usersLoadBatchFastCalls.Load(), "user concurrency should prefer the fast load path")
	require.EqualValues(t, 0, cache.usersLoadBatchCalls.Load())

	stats2, collectedAt2, err := svc.GetUserConcurrencyStats(ctx)
	require.NoError(t, err)
	require.NotNil(t, collectedAt2)
	require.Len(t, stats2, 2)
	require.EqualValues(t, 2, stats2[99201].CurrentInUse)
	require.EqualValues(t, 1, stats2[99201].WaitingInQueue)
	require.EqualValues(t, 1, userRepo.listOpsCalls.Load(), "result cache should skip the repo on hit")
	require.EqualValues(t, 0, userRepo.listWithFiltersCalls.Load())
	require.EqualValues(t, 1, cache.usersLoadBatchFastCalls.Load(), "result cache should skip load lookups on hit")
	require.EqualValues(t, 0, cache.usersLoadBatchCalls.Load())
	require.Equal(t, collectedAt, collectedAt2, "cached result should preserve the collected timestamp")
}

func TestOpsServiceGetUserConcurrencyStats_FallsBackToLegacyUserListWhenLightweightListerUnavailable(t *testing.T) {
	ctx := context.Background()
	cfg := newOpenAIWarmPoolTestConfig()
	cfg.Ops.Enabled = true
	users := []User{{ID: 99301, Email: "legacy@example.com", Username: "legacy", Concurrency: 3, Status: StatusActive}}
	userRepo := &countingLegacyOpsUserRepo{users: users}
	cache := &countingOpsConcurrencyCache{
		stubConcurrencyCache: stubConcurrencyCache{
			usersLoadBatch: map[int64]*UserLoadInfo{
				99301: &UserLoadInfo{UserID: 99301, CurrentConcurrency: 1, WaitingCount: 0},
			},
			skipDefaultLoad: true,
		},
	}
	svc := &OpsService{
		cfg:                cfg,
		userRepo:           userRepo,
		concurrencyService: NewConcurrencyService(cache),
	}

	stats, _, err := svc.GetUserConcurrencyStats(ctx)
	require.NoError(t, err)
	require.Len(t, stats, 1)
	require.EqualValues(t, 1, stats[99301].CurrentInUse)
	require.EqualValues(t, 1, userRepo.listWithFiltersCalls.Load(), "legacy repo should still be used as fallback")
	require.EqualValues(t, 1, cache.usersLoadBatchFastCalls.Load(), "fallback user listing should still use the fast load path")
}
