package service

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type countingWarmPoolAccountRepo struct {
	stubOpenAIAccountRepo
	listByPlatformCalls atomic.Int32
	listByGroupCalls    atomic.Int32
	getByIDsCalls       atomic.Int32
}

func (r *countingWarmPoolAccountRepo) ListSchedulableByPlatform(ctx context.Context, platform string) ([]Account, error) {
	r.listByPlatformCalls.Add(1)
	return r.stubOpenAIAccountRepo.ListSchedulableByPlatform(ctx, platform)
}

func (r *countingWarmPoolAccountRepo) ListSchedulableByGroupIDAndPlatform(ctx context.Context, groupID int64, platform string) ([]Account, error) {
	r.listByGroupCalls.Add(1)
	return r.stubOpenAIAccountRepo.ListSchedulableByGroupIDAndPlatform(ctx, groupID, platform)
}

func (r *countingWarmPoolAccountRepo) GetByIDs(ctx context.Context, ids []int64) ([]*Account, error) {
	r.getByIDsCalls.Add(1)
	return r.stubOpenAIAccountRepo.GetByIDs(ctx, ids)
}

func TestOpsServiceGetOpenAIWarmPoolStats_DoesNotDoubleCountSharedGlobalReadyAccount(t *testing.T) {
	ctx := context.Background()
	cfg := newOpenAIWarmPoolTestConfig()
	cfg.Ops.Enabled = true
	groupA := &Group{ID: 9101, Name: "A", Platform: PlatformOpenAI}
	groupB := &Group{ID: 9102, Name: "B", Platform: PlatformOpenAI}
	accounts := []Account{{
		ID:          83001,
		Name:        "shared-openai",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Priority:    0,
		Groups:      []*Group{groupA, groupB},
	}}
	svc := newOpenAIWarmPoolTestService(cfg, accounts, stubConcurrencyCache{})
	svc.SetOpenAIWarmPoolUsageReader(&openAIWarmPoolUsageReaderStub{})

	now := time.Now()
	pool := svc.getOpenAIWarmPool()
	pool.accountState(83001).finishSuccess(now, pool.config().GlobalEntryTTL)
	require.True(t, pool.bucketState(groupA.ID).promote(83001, now, pool.config().BucketEntryTTL))
	require.True(t, pool.bucketState(groupB.ID).promote(83001, now, pool.config().BucketEntryTTL))
	pool.bucketState(groupA.ID).lastAccess.Store(now.UnixNano())
	pool.bucketState(groupB.ID).lastAccess.Store(now.UnixNano())
	pool.recordTake(&groupA.ID)
	pool.recordTake(&groupA.ID)

	opsSvc := &OpsService{cfg: cfg, openAIGatewayService: svc}
	stats, err := opsSvc.GetOpenAIWarmPoolStats(ctx, nil)
	require.NoError(t, err)
	require.NotNil(t, stats)
	require.NotNil(t, stats.Summary)
	require.Equal(t, 1, stats.Summary.TrackedAccountCount)
	require.Equal(t, 1, stats.Summary.GlobalReadyAccountCount)
	require.Equal(t, 2, stats.Summary.BucketReadyAccountCount)
	require.Equal(t, int64(2), stats.Summary.TakeCount)
	require.Len(t, stats.Buckets, 2)
	for _, bucket := range stats.Buckets {
		require.Equal(t, 1, bucket.SchedulableAccounts)
		require.Equal(t, 1, bucket.BucketReadyAccounts)
		require.Equal(t, cfg.Gateway.OpenAIWS.AccountWarmPool.BucketTargetSize, bucket.BucketTargetSize)
		require.Equal(t, cfg.Gateway.OpenAIWS.AccountWarmPool.BucketRefillBelow, bucket.BucketRefillBelow)
	}
}

func TestOpsServiceGetOpenAIWarmPoolStats_UsesGlobalReadyCoverageBeyondBucketMembers(t *testing.T) {
	ctx := context.Background()
	cfg := newOpenAIWarmPoolTestConfig()
	cfg.Ops.Enabled = true
	cfg.Gateway.OpenAIWS.AccountWarmPool.BucketTargetSize = 1
	cfg.Gateway.OpenAIWS.AccountWarmPool.BucketRefillBelow = 1
	cfg.Gateway.OpenAIWS.AccountWarmPool.GlobalTargetSize = 3
	cfg.Gateway.OpenAIWS.AccountWarmPool.GlobalRefillBelow = 1
	group := &Group{ID: 9151, Name: "Global Coverage Group", Platform: PlatformOpenAI}
	accounts := []Account{
		{ID: 91501, Name: "global-ready-1", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0, Groups: []*Group{group}},
		{ID: 91502, Name: "global-ready-2", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1, Groups: []*Group{group}},
		{ID: 91503, Name: "global-ready-3", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 2, Groups: []*Group{group}},
	}
	svc := newOpenAIWarmPoolTestService(cfg, accounts, stubConcurrencyCache{})
	svc.SetOpenAIWarmPoolUsageReader(&openAIWarmPoolUsageReaderStub{})

	now := time.Now()
	pool := svc.getOpenAIWarmPool()
	for _, accountID := range []int64{91501, 91502, 91503} {
		pool.accountState(accountID).finishSuccess(now, pool.config().GlobalEntryTTL)
	}
	require.True(t, pool.bucketState(group.ID).promote(91501, now, pool.config().BucketEntryTTL))
	pool.bucketState(group.ID).lastAccess.Store(now.UnixNano())

	opsSvc := &OpsService{cfg: cfg, openAIGatewayService: svc}
	stats, err := opsSvc.GetOpenAIWarmPoolStats(ctx, nil)
	require.NoError(t, err)
	require.NotNil(t, stats)
	require.NotNil(t, stats.Summary)
	require.Equal(t, 3, stats.Summary.GlobalReadyAccountCount)
	require.Len(t, stats.GlobalCoverages, 1)
	require.Equal(t, int64(group.ID), stats.GlobalCoverages[0].GroupID)
	require.Equal(t, 3, stats.GlobalCoverages[0].CoverageCount)
	require.Len(t, stats.Buckets, 1)
	require.Equal(t, 1, stats.Buckets[0].BucketReadyAccounts)
}

func TestOpsServiceGetOpenAIWarmPoolStats_GroupFilterIncludesAccountStates(t *testing.T) {
	ctx := context.Background()
	cfg := newOpenAIWarmPoolTestConfig()
	cfg.Ops.Enabled = true
	groupID := int64(9201)
	group := &Group{ID: groupID, Name: "Warm Pool Group", Platform: PlatformOpenAI}
	accounts := []Account{
		{ID: 92001, Name: "ready-account", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0, Groups: []*Group{group}},
		{ID: 92002, Name: "probing-account", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1, Groups: []*Group{group}},
		{ID: 92003, Name: "cooling-account", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 2, Groups: []*Group{group}},
		{ID: 92004, Name: "network-error-account", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 3, Groups: []*Group{group}},
	}
	svc := newOpenAIWarmPoolTestService(cfg, accounts, stubConcurrencyCache{})
	svc.SetOpenAIWarmPoolUsageReader(&openAIWarmPoolUsageReaderStub{})

	now := time.Now()
	pool := svc.getOpenAIWarmPool()
	pool.accountState(92001).finishSuccess(now, pool.config().GlobalEntryTTL)
	require.True(t, pool.bucketState(groupID).promote(92001, now, pool.config().BucketEntryTTL))
	require.True(t, pool.accountState(92002).tryStartProbe(now, false))
	pool.accountState(92003).finishFailure(now, pool.config().FailureCooldown)
	pool.accountState(92004).markNetworkError(now, pool.config().NetworkErrorEntryTTL)
	pool.bucketState(groupID).lastAccess.Store(now.UnixNano())
	pool.bucketState(groupID).lastRefill.Store(now.UnixNano())

	opsSvc := &OpsService{cfg: cfg, openAIGatewayService: svc}
	stats, err := opsSvc.GetOpenAIWarmPoolStats(ctx, &groupID)
	require.NoError(t, err)
	require.Len(t, stats.Buckets, 1)
	require.Equal(t, 1, stats.Buckets[0].BucketReadyAccounts)
	require.Equal(t, cfg.Gateway.OpenAIWS.AccountWarmPool.BucketTargetSize, stats.Buckets[0].BucketTargetSize)
	require.Equal(t, cfg.Gateway.OpenAIWS.AccountWarmPool.BucketRefillBelow, stats.Buckets[0].BucketRefillBelow)
	require.Equal(t, 1, stats.Buckets[0].ProbingAccounts)
	require.Equal(t, 1, stats.Buckets[0].CoolingAccounts)
	require.Len(t, stats.Accounts, 4)

	states := make(map[int64]string, len(stats.Accounts))
	for _, account := range stats.Accounts {
		states[account.AccountID] = account.State
	}
	require.Equal(t, "ready", states[92001])
	require.Equal(t, "probing", states[92002])
	require.Equal(t, "cooling", states[92003])
	require.Equal(t, "network_error", states[92004])
}

func TestOpsServiceGetOpenAIWarmPoolStats_ReadyAccountRowsUseTrackedReadyFastPathAndCache(t *testing.T) {
	ctx := context.Background()
	cfg := newOpenAIWarmPoolTestConfig()
	cfg.Ops.Enabled = true
	groupA := &Group{ID: 9251, Name: "A", Platform: PlatformOpenAI}
	groupB := &Group{ID: 9252, Name: "B", Platform: PlatformOpenAI}
	accounts := []Account{{
		ID:          92501,
		Name:        "shared-ready",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Priority:    0,
		Groups:      []*Group{groupA, groupB},
	}}
	repo := &countingWarmPoolAccountRepo{stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: accounts}}
	svc := &OpenAIGatewayService{
		accountRepo:        repo,
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{}),
	}
	svc.openaiWarmPool = newOpenAIAccountWarmPoolService(svc)
	svc.SetOpenAIWarmPoolUsageReader(&openAIWarmPoolUsageReaderStub{})

	now := time.Now()
	pool := svc.getOpenAIWarmPool()
	waitForStartupBootstrapToSettle(t, pool)
	pool.accountState(92501).finishSuccess(now, pool.config().GlobalEntryTTL)
	require.True(t, pool.bucketState(groupA.ID).promote(92501, now, pool.config().BucketEntryTTL))
	require.True(t, pool.bucketState(groupB.ID).promote(92501, now, pool.config().BucketEntryTTL))
	pool.bucketState(groupA.ID).lastAccess.Store(now.UnixNano())
	pool.bucketState(groupB.ID).lastAccess.Store(now.UnixNano())
	baseListByPlatformCalls := repo.listByPlatformCalls.Load()
	baseListByGroupCalls := repo.listByGroupCalls.Load()
	baseGetByIDsCalls := repo.getByIDsCalls.Load()

	opsSvc := &OpsService{cfg: cfg, openAIGatewayService: svc}
	stats, err := opsSvc.GetOpenAIWarmPoolStatsWithOptions(ctx, nil, true, "ready", true)
	require.NoError(t, err)
	require.Len(t, stats.Accounts, 1)
	require.Len(t, stats.GlobalCoverages, 2)
	require.NotNil(t, stats.Summary)
	require.Equal(t, 1, stats.Summary.GlobalReadyAccountCount)
	require.EqualValues(t, baseListByPlatformCalls, repo.listByPlatformCalls.Load())
	require.EqualValues(t, baseListByGroupCalls, repo.listByGroupCalls.Load())
	require.EqualValues(t, baseGetByIDsCalls+1, repo.getByIDsCalls.Load())
	require.Equal(t, int64(92501), stats.Accounts[0].AccountID)
	require.Equal(t, "ready", stats.Accounts[0].State)
	require.Len(t, stats.Accounts[0].Groups, 2)

	cachedStats, err := opsSvc.GetOpenAIWarmPoolStatsWithOptions(ctx, nil, true, "ready", true)
	require.NoError(t, err)
	require.Len(t, cachedStats.Accounts, 1)
	require.Len(t, cachedStats.GlobalCoverages, 2)
	require.NotNil(t, cachedStats.Summary)
	require.Equal(t, 1, cachedStats.Summary.GlobalReadyAccountCount)
	require.EqualValues(t, baseListByPlatformCalls, repo.listByPlatformCalls.Load())
	require.EqualValues(t, baseListByGroupCalls, repo.listByGroupCalls.Load())
	require.EqualValues(t, baseGetByIDsCalls+1, repo.getByIDsCalls.Load(), "ready 列表第二次请求应命中缓存，不应重复回库")
	require.Equal(t, int64(92501), cachedStats.Accounts[0].AccountID)
	require.Equal(t, "ready", cachedStats.Accounts[0].State)
	require.Len(t, cachedStats.Accounts[0].Groups, 2)
}

func TestOpsServiceGetOpenAIWarmPoolStatsWithPage_PaginatesReadyAccountsAndPreservesSummary(t *testing.T) {
	ctx := context.Background()
	cfg := newOpenAIWarmPoolTestConfig()
	cfg.Ops.Enabled = true
	group := &Group{ID: 9311, Name: "Paged Warm Group", Platform: PlatformOpenAI}
	accounts := make([]Account, 0, 21)
	for i := 0; i < 21; i++ {
		accountID := int64(93101 + i)
		accounts = append(accounts, Account{
			ID:          accountID,
			Name:        fmt.Sprintf("ready-account-%d", i+1),
			Platform:    PlatformOpenAI,
			Type:        AccountTypeOAuth,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 1,
			Priority:    i,
			Groups:      []*Group{group},
		})
	}
	repo := &countingWarmPoolAccountRepo{stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: accounts}}
	svc := &OpenAIGatewayService{
		accountRepo:        repo,
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{}),
	}
	svc.openaiWarmPool = newOpenAIAccountWarmPoolService(svc)
	svc.SetOpenAIWarmPoolUsageReader(&openAIWarmPoolUsageReaderStub{})

	now := time.Now()
	pool := svc.getOpenAIWarmPool()
	waitForStartupBootstrapToSettle(t, pool)
	baseGetByIDsCalls := repo.getByIDsCalls.Load()
	for _, account := range accounts {
		pool.accountState(account.ID).finishSuccess(now, pool.config().GlobalEntryTTL)
	}
	require.True(t, pool.bucketState(group.ID).promote(accounts[0].ID, now, pool.config().BucketEntryTTL))
	pool.bucketState(group.ID).lastAccess.Store(now.UnixNano())

	opsSvc := &OpsService{cfg: cfg, openAIGatewayService: svc}
	page1, err := opsSvc.GetOpenAIWarmPoolStatsWithPage(ctx, nil, true, "ready", true, 1, 20)
	require.NoError(t, err)
	require.NotNil(t, page1)
	require.NotNil(t, page1.Summary)
	require.Equal(t, 21, page1.Summary.GlobalReadyAccountCount)
	require.Equal(t, 1, page1.Summary.ActiveGroupCount)
	require.Equal(t, 20, len(page1.Accounts))
	require.Equal(t, 1, page1.Page)
	require.Equal(t, 20, page1.PageSize)
	require.Equal(t, 21, page1.Total)
	require.Equal(t, 2, page1.Pages)
	require.Equal(t, int64(93101), page1.Accounts[0].AccountID)
	require.Equal(t, int64(93120), page1.Accounts[19].AccountID)
	require.Len(t, page1.GlobalCoverages, 1)
	require.Equal(t, int64(group.ID), page1.GlobalCoverages[0].GroupID)
	require.Equal(t, 21, page1.GlobalCoverages[0].CoverageCount)

	page2, err := opsSvc.GetOpenAIWarmPoolStatsWithPage(ctx, nil, true, "ready", true, 2, 20)
	require.NoError(t, err)
	require.NotNil(t, page2.Summary)
	require.Equal(t, 21, page2.Summary.GlobalReadyAccountCount)
	require.Len(t, page2.Accounts, 1)
	require.Equal(t, int64(93121), page2.Accounts[0].AccountID)
	require.Equal(t, 2, page2.Page)
	require.Equal(t, 20, page2.PageSize)
	require.Equal(t, 21, page2.Total)
	require.Equal(t, 2, page2.Pages)
	require.EqualValues(t, baseGetByIDsCalls+1, repo.getByIDsCalls.Load(), "分页 ready 列表应复用同一份缓存结果")
}

func TestOpsServiceBuildOpenAIWarmPoolReadyListStats_SharesCacheAcrossPagedAndUnpagedRequests(t *testing.T) {
	ctx := context.Background()
	cfg := newOpenAIWarmPoolTestConfig()
	cfg.Ops.Enabled = true
	groupA := &Group{ID: 9321, Name: "Group A", Platform: PlatformOpenAI}
	groupB := &Group{ID: 9322, Name: "Group B", Platform: PlatformOpenAI}
	accounts := make([]Account, 0, 30)
	for i := 0; i < 25; i++ {
		accounts = append(accounts, Account{
			ID:          int64(93201 + i),
			Name:        fmt.Sprintf("group-a-ready-%02d", i+1),
			Platform:    PlatformOpenAI,
			Type:        AccountTypeOAuth,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 1,
			Priority:    i,
			Groups:      []*Group{groupA},
		})
	}
	for i := 0; i < 5; i++ {
		accounts = append(accounts, Account{
			ID:          int64(93301 + i),
			Name:        fmt.Sprintf("group-b-ready-%02d", i+1),
			Platform:    PlatformOpenAI,
			Type:        AccountTypeOAuth,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 1,
			Priority:    i,
			Groups:      []*Group{groupB},
		})
	}
	repo := &countingWarmPoolAccountRepo{stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: accounts}}
	svc := &OpenAIGatewayService{
		accountRepo:        repo,
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{}),
	}
	svc.openaiWarmPool = newOpenAIAccountWarmPoolService(svc)
	svc.SetOpenAIWarmPoolUsageReader(&openAIWarmPoolUsageReaderStub{})

	now := time.Now()
	pool := svc.getOpenAIWarmPool()
	waitForStartupBootstrapToSettle(t, pool)
	for _, account := range accounts {
		pool.accountState(account.ID).finishSuccess(now, pool.config().GlobalEntryTTL)
	}
	require.True(t, pool.bucketState(groupA.ID).promote(accounts[0].ID, now, pool.config().BucketEntryTTL))
	require.True(t, pool.bucketState(groupB.ID).promote(accounts[len(accounts)-1].ID, now, pool.config().BucketEntryTTL))
	pool.bucketState(groupA.ID).lastAccess.Store(now.UnixNano())
	pool.bucketState(groupB.ID).lastAccess.Store(now.UnixNano())

	opsSvc := &OpsService{cfg: cfg, openAIGatewayService: svc}
	unpaged, err := opsSvc.buildOpenAIWarmPoolReadyListStats(ctx, pool, &groupA.ID, 0, 0)
	require.NoError(t, err)
	require.NotNil(t, unpaged)
	require.NotNil(t, unpaged.Summary)
	require.Equal(t, 30, unpaged.Summary.GlobalReadyAccountCount)
	require.Equal(t, 25, len(unpaged.Accounts))
	require.Len(t, unpaged.GlobalCoverages, 2)
	require.Equal(t, int64(groupA.ID), unpaged.GlobalCoverages[0].GroupID)
	require.Equal(t, 25, unpaged.GlobalCoverages[0].CoverageCount)
	require.Equal(t, int64(groupB.ID), unpaged.GlobalCoverages[1].GroupID)
	require.Equal(t, 5, unpaged.GlobalCoverages[1].CoverageCount)
	require.EqualValues(t, 1, repo.getByIDsCalls.Load())

	paged, err := opsSvc.buildOpenAIWarmPoolReadyListStats(ctx, pool, &groupA.ID, 2, 10)
	require.NoError(t, err)
	require.NotNil(t, paged.Summary)
	require.Equal(t, 30, paged.Summary.GlobalReadyAccountCount)
	require.Equal(t, 2, paged.Page)
	require.Equal(t, 10, paged.PageSize)
	require.Equal(t, 25, paged.Total)
	require.Equal(t, 3, paged.Pages)
	require.Len(t, paged.Accounts, 10)
	require.Equal(t, int64(93211), paged.Accounts[0].AccountID)
	require.Equal(t, int64(93220), paged.Accounts[9].AccountID)
	require.EqualValues(t, 1, repo.getByIDsCalls.Load(), "分页 ready 列表应复用同一份 ready provider 结果")
}

func TestOpsServiceGetOpenAIWarmPoolStats_DoesNotCountExpiredReadyAsGlobalReady(t *testing.T) {
	ctx := context.Background()
	cfg := newOpenAIWarmPoolTestConfig()
	cfg.Ops.Enabled = true
	accounts := []Account{{ID: 92801, Name: "expired-ready", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0}}
	svc := newOpenAIWarmPoolTestService(cfg, accounts, stubConcurrencyCache{})
	svc.SetOpenAIWarmPoolUsageReader(&openAIWarmPoolUsageReaderStub{})
	pool := svc.getOpenAIWarmPool()
	pool.accountState(92801).finishSuccess(time.Now().Add(-10*time.Minute), time.Minute)

	opsSvc := &OpsService{cfg: cfg, openAIGatewayService: svc}
	stats, err := opsSvc.GetOpenAIWarmPoolStatsWithOptions(ctx, nil, true, "", true)
	require.NoError(t, err)
	require.NotNil(t, stats.Summary)
	require.Equal(t, 0, stats.Summary.GlobalReadyAccountCount)
	require.Len(t, stats.Accounts, 1)
	require.Equal(t, "idle", stats.Accounts[0].State)
}

func TestOpsServiceGetOpenAIWarmPoolStats_DefaultOverviewSkipsAccountListing(t *testing.T) {
	ctx := context.Background()
	cfg := newOpenAIWarmPoolTestConfig()
	cfg.Ops.Enabled = true
	groupID := int64(92901)
	group := &Group{ID: groupID, Name: "Overview Group", Platform: PlatformOpenAI}
	accounts := []Account{{ID: 92901, Name: "overview-account", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0, Groups: []*Group{group}}}
	repo := &countingWarmPoolAccountRepo{stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: accounts}}
	svc := &OpenAIGatewayService{
		accountRepo:        repo,
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(stubConcurrencyCache{}),
	}
	svc.openaiWarmPool = newOpenAIAccountWarmPoolService(svc)
	svc.SetOpenAIWarmPoolUsageReader(&openAIWarmPoolUsageReaderStub{})
	pool := svc.getOpenAIWarmPool()
	waitForStartupBootstrapToSettle(t, pool)
	clearWarmPoolReadyStates(pool, accounts)
	baseListByPlatformCalls := repo.listByPlatformCalls.Load()
	baseListByGroupCalls := repo.listByGroupCalls.Load()
	baseGetByIDsCalls := repo.getByIDsCalls.Load()
	now := time.Now()
	pool.accountState(92901).finishSuccess(now, pool.config().GlobalEntryTTL)
	require.True(t, pool.bucketState(groupID).promote(92901, now, pool.config().BucketEntryTTL))
	pool.bucketState(groupID).lastAccess.Store(now.UnixNano())

	opsSvc := &OpsService{cfg: cfg, openAIGatewayService: svc}
	stats, err := opsSvc.GetOpenAIWarmPoolStatsWithOptions(ctx, nil, false, "", false)
	require.NoError(t, err)
	require.Empty(t, stats.Accounts)
	require.Len(t, stats.Buckets, 1)
	require.Len(t, stats.GlobalCoverages, 1)
	require.EqualValues(t, baseListByPlatformCalls, repo.listByPlatformCalls.Load(), "默认概览不应再触发额外的全平台可调度账号扫描")
	require.EqualValues(t, baseListByGroupCalls+1, repo.listByGroupCalls.Load(), "默认概览应只额外加载活跃 bucket 对应的分组账号")
	require.EqualValues(t, baseGetByIDsCalls, repo.getByIDsCalls.Load())

	stats, err = opsSvc.GetOpenAIWarmPoolStatsWithOptions(ctx, nil, false, "", false)
	require.NoError(t, err)
	require.Empty(t, stats.Accounts)
	require.Len(t, stats.Buckets, 1)
	require.EqualValues(t, baseListByPlatformCalls, repo.listByPlatformCalls.Load())
	require.EqualValues(t, baseListByGroupCalls+1, repo.listByGroupCalls.Load(), "结果缓存命中后不应重复加载分组账号")
}

func TestOpsServiceGetOpenAIWarmPoolStats_BuildsNetworkErrorPoolSummary(t *testing.T) {
	ctx := context.Background()
	cfg := newOpenAIWarmPoolTestConfig()
	cfg.Ops.Enabled = true
	accounts := []Account{{ID: 93001, Name: "network-error", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0}}
	svc := newOpenAIWarmPoolTestService(cfg, accounts, stubConcurrencyCache{})
	svc.SetOpenAIWarmPoolUsageReader(&openAIWarmPoolUsageReaderStub{})
	pool := svc.getOpenAIWarmPool()
	pool.accountState(93001).markNetworkError(time.Now(), pool.config().NetworkErrorEntryTTL)

	opsSvc := &OpsService{cfg: cfg, openAIGatewayService: svc}
	stats, err := opsSvc.GetOpenAIWarmPoolStats(ctx, nil)
	require.NoError(t, err)
	require.NotNil(t, stats.NetworkErrorPool)
	require.Equal(t, 1, stats.NetworkErrorPool.Count)
	require.Equal(t, pool.config().NetworkErrorPoolSize, stats.NetworkErrorPool.Capacity)
	require.NotNil(t, stats.NetworkErrorPool.OldestEnteredAt)
}

func TestOpsServiceGetOpenAIWarmPoolStats_DoesNotTriggerBucketRefreshSideEffects(t *testing.T) {
	ctx := context.Background()
	cfg := newOpenAIWarmPoolTestConfig()
	cfg.Ops.Enabled = true
	cfg.Gateway.OpenAIWS.AccountWarmPool.BucketEntryTTLSeconds = 1
	groupID := int64(93002)
	group := &Group{ID: groupID, Name: "Ops Slow Group", Platform: PlatformOpenAI}
	accounts := []Account{{ID: 93002, Name: "ready-account", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0, Groups: []*Group{group}}}
	reader := &openAIWarmPoolUsageReaderStub{errs: map[int64]error{93002: context.DeadlineExceeded}}
	svc := newOpenAIWarmPoolTestService(cfg, accounts, stubConcurrencyCache{})
	svc.SetOpenAIWarmPoolUsageReader(reader)
	pool := svc.getOpenAIWarmPool()
	waitForStartupBootstrapToSettle(t, pool)
	baseReaderCalls := reader.CallCount()
	past := time.Now().Add(-2 * time.Second)
	pool.accountState(93002).finishSuccess(past, pool.config().GlobalEntryTTL)
	require.True(t, pool.bucketState(groupID).promote(93002, past, pool.config().BucketEntryTTL))
	pool.bucketState(groupID).lastAccess.Store(time.Now().UnixNano())

	opsSvc := &OpsService{cfg: cfg, openAIGatewayService: svc}
	stats, err := opsSvc.GetOpenAIWarmPoolStats(ctx, nil)
	require.NoError(t, err)
	require.Len(t, stats.Buckets, 1)
	require.Equal(t, 1, stats.Buckets[0].BucketReadyAccounts)
	require.Never(t, func() bool { return reader.CallCount() > baseReaderCalls }, 200*time.Millisecond, 20*time.Millisecond, "OPS 查询不应触发分组池复检探测")
}

func TestOpsServiceTriggerOpenAIWarmPoolGlobalRefill_ClearsWarmPoolCache(t *testing.T) {
	ctx := context.Background()
	cfg := newOpenAIWarmPoolTestConfig()
	cfg.Ops.Enabled = true
	cfg.Gateway.OpenAIWS.AccountWarmPool.GlobalTargetSize = 2
	cfg.Gateway.OpenAIWS.AccountWarmPool.GlobalRefillCooldownSeconds = 3600
	groupID := int64(93003)
	group := &Group{ID: groupID, Name: "Ops Trigger Group", Platform: PlatformOpenAI}
	accounts := []Account{
		{ID: 93031, Name: "trigger-1", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0, Groups: []*Group{group}},
		{ID: 93032, Name: "trigger-2", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1, Groups: []*Group{group}},
	}
	svc := newOpenAIWarmPoolTestService(cfg, accounts, stubConcurrencyCache{})
	svc.SetOpenAIWarmPoolUsageReader(&openAIWarmPoolUsageReaderStub{})
	pool := svc.getOpenAIWarmPool()
	pool.bucketState(groupID).lastAccess.Store(time.Now().UnixNano())
	pool.lastGlobalRefill.Store(time.Now().UnixNano())

	opsSvc := &OpsService{cfg: cfg, openAIGatewayService: svc}
	opsSvc.realtimeWarmPoolCache.set("cached", &OpsOpenAIWarmPoolStats{Summary: &OpsOpenAIWarmPoolSummary{GlobalReadyAccountCount: 0}}, time.Minute, time.Now())
	_, ok := opsSvc.realtimeWarmPoolCache.get("cached", time.Now())
	require.True(t, ok)

	require.NoError(t, opsSvc.TriggerOpenAIWarmPoolGlobalRefill(ctx))
	_, ok = opsSvc.realtimeWarmPoolCache.get("cached", time.Now())
	require.False(t, ok, "手动触发全局池补充后应清空 OPS warm-pool 结果缓存")
	require.Equal(t, 2, pool.countWarmReady(cloneAccounts(accounts)), "手动触发全局池补充时应立即刷新全局 ready 状态")
}

func TestOpsServiceGetOpenAIWarmPoolStats_ReportsBootstrappingDuringStartupBootstrap(t *testing.T) {
	ctx := context.Background()
	cfg := newOpenAIWarmPoolTestConfig()
	cfg.Ops.Enabled = true
	cfg.Gateway.OpenAIWS.AccountWarmPool.GlobalTargetSize = 1
	cfg.Gateway.OpenAIWS.AccountWarmPool.GlobalRefillBelow = 1
	cfg.Gateway.OpenAIWS.AccountWarmPool.GlobalRefillCooldownSeconds = 0
	cfg.Gateway.OpenAIWS.AccountWarmPool.GlobalRefillIntervalSeconds = 3600
	groupID := int64(93004)
	group := &Group{ID: groupID, Name: "Ops Bootstrap Group", Platform: PlatformOpenAI}
	accounts := []Account{{ID: 93041, Name: "startup-ready", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0, Groups: []*Group{group}}}
	svc := newOpenAIWarmPoolTestService(cfg, accounts, stubConcurrencyCache{})
	svc.settingService = NewSettingService(&warmPoolSettingRepoStub{values: map[string]string{
		SettingKeyOpenAIWarmPoolStartupGroupIDs: "93004",
	}}, cfg)
	openAIWarmPoolSF.Forget("openai_warm_pool")
	openAIWarmPoolCache.Store((*cachedOpenAIWarmPoolSettings)(nil))
	defer func() {
		openAIWarmPoolSF.Forget("openai_warm_pool")
		openAIWarmPoolCache.Store((*cachedOpenAIWarmPoolSettings)(nil))
	}()
	pool := svc.getOpenAIWarmPool()
	defer pool.Stop()
	reader := newBlockingWarmPoolUsageReaderStub()
	svc.SetOpenAIWarmPoolUsageReader(reader)

	select {
	case <-reader.Started():
	case <-time.After(time.Second):
		t.Fatal("expected startup bootstrap to start probing after usage reader became ready")
	}

	opsSvc := &OpsService{cfg: cfg, openAIGatewayService: svc}
	stats, err := opsSvc.GetOpenAIWarmPoolStats(ctx, nil)
	require.NoError(t, err)
	require.True(t, stats.Bootstrapping, "首轮 startup bootstrap 进行中时，OPS stats 应显式返回 bootstrapping=true")

	reader.Release()
	require.Eventually(t, func() bool {
		return pool.countWarmReady(cloneAccounts(accounts)) == 1
	}, time.Second, 10*time.Millisecond, "startup bootstrap 释放后应完成首轮全局池补充")
	require.Eventually(t, func() bool {
		stats, err := opsSvc.GetOpenAIWarmPoolStats(ctx, nil)
		require.NoError(t, err)
		return !stats.Bootstrapping
	}, time.Second, 10*time.Millisecond, "startup bootstrap 稳定后，OPS stats 应切回 bootstrapping=false")
}

func TestOpsServiceGetOpenAIWarmPoolStats_ReportsGlobalReadyAfterStartupBootstrapWithoutActiveBuckets(t *testing.T) {
	ctx := context.Background()
	cfg := newOpenAIWarmPoolTestConfig()
	cfg.Ops.Enabled = true
	cfg.Gateway.OpenAIWS.AccountWarmPool.GlobalTargetSize = 1
	cfg.Gateway.OpenAIWS.AccountWarmPool.GlobalRefillBelow = 1
	cfg.Gateway.OpenAIWS.AccountWarmPool.GlobalRefillCooldownSeconds = 0
	cfg.Gateway.OpenAIWS.AccountWarmPool.GlobalRefillIntervalSeconds = 3600
	groupID := int64(93014)
	group := &Group{ID: groupID, Name: "Ops Startup Summary Group", Platform: PlatformOpenAI}
	accounts := []Account{{ID: 93141, Name: "startup-summary", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0, Groups: []*Group{group}}}
	svc := newOpenAIWarmPoolTestService(cfg, accounts, stubConcurrencyCache{})
	svc.settingService = NewSettingService(&warmPoolSettingRepoStub{values: map[string]string{
		SettingKeyOpenAIWarmPoolStartupGroupIDs: "93014",
	}}, cfg)
	openAIWarmPoolSF.Forget("openai_warm_pool")
	openAIWarmPoolCache.Store((*cachedOpenAIWarmPoolSettings)(nil))
	defer func() {
		openAIWarmPoolSF.Forget("openai_warm_pool")
		openAIWarmPoolCache.Store((*cachedOpenAIWarmPoolSettings)(nil))
	}()
	pool := svc.getOpenAIWarmPool()
	defer pool.Stop()

	svc.SetOpenAIWarmPoolUsageReader(&openAIWarmPoolUsageReaderStub{})
	require.Eventually(t, func() bool {
		return pool.countWarmReady(cloneAccounts(accounts)) == 1
	}, time.Second, 10*time.Millisecond)

	opsSvc := &OpsService{cfg: cfg, openAIGatewayService: svc}
	stats, err := opsSvc.GetOpenAIWarmPoolStats(ctx, nil)
	require.NoError(t, err)
	require.False(t, stats.Bootstrapping)
	require.NotNil(t, stats.Summary)
	require.Equal(t, 1, stats.Summary.GlobalReadyAccountCount, "startup bootstrap 完成后，即使还没有 active bucket，OPS summary 也应反映已 ready 的全局账号")
}

func TestOpsServiceGetOpenAIWarmPoolStats_ShowsBucketDuringRefillWithoutAccess(t *testing.T) {
	ctx := context.Background()
	cfg := newOpenAIWarmPoolTestConfig()
	cfg.Ops.Enabled = true
	groupID := int64(93024)
	group := &Group{ID: groupID, Name: "Refill Visible Group", Platform: PlatformOpenAI}
	accounts := []Account{{ID: 93241, Name: "refill-visible", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0, Groups: []*Group{group}}}
	svc := newOpenAIWarmPoolTestService(cfg, accounts, stubConcurrencyCache{})
	svc.SetOpenAIWarmPoolUsageReader(&openAIWarmPoolUsageReaderStub{})
	pool := svc.getOpenAIWarmPool()
	defer pool.Stop()
	waitForStartupBootstrapToSettle(t, pool)

	now := time.Now()
	pool.bucketState(groupID).lastRefill.Store(now.UnixNano())

	opsSvc := &OpsService{cfg: cfg, openAIGatewayService: svc}
	stats, err := opsSvc.GetOpenAIWarmPoolStats(ctx, nil)
	require.NoError(t, err)
	require.Len(t, stats.Buckets, 1, "最近补池过的分组在尚无真实访问时也应可见")
	require.Equal(t, groupID, stats.Buckets[0].GroupID)
	require.Equal(t, 1, stats.Buckets[0].SchedulableAccounts)
	require.Zero(t, stats.Buckets[0].BucketReadyAccounts)
	require.Nil(t, stats.Buckets[0].LastAccessAt)
	require.NotNil(t, stats.Buckets[0].LastRefillAt)
}

func TestOpsServiceGetOpenAIWarmPoolStats_ShowsStartupGroupCoverageWithoutAccess(t *testing.T) {
	ctx := context.Background()
	cfg := newOpenAIWarmPoolTestConfig()
	cfg.Ops.Enabled = true
	cfg.Gateway.OpenAIWS.AccountWarmPool.GlobalTargetSize = 1
	cfg.Gateway.OpenAIWS.AccountWarmPool.GlobalRefillBelow = 1
	cfg.Gateway.OpenAIWS.AccountWarmPool.GlobalRefillCooldownSeconds = 0
	cfg.Gateway.OpenAIWS.AccountWarmPool.GlobalRefillIntervalSeconds = 3600
	groupID := int64(93034)
	group := &Group{ID: groupID, Name: "Startup Visible Group", Platform: PlatformOpenAI}
	accounts := []Account{{ID: 93341, Name: "startup-visible", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0, Groups: []*Group{group}}}
	svc := newOpenAIWarmPoolTestService(cfg, accounts, stubConcurrencyCache{})
	svc.settingService = NewSettingService(&warmPoolSettingRepoStub{values: map[string]string{
		SettingKeyOpenAIWarmPoolStartupGroupIDs: "93034",
	}}, cfg)
	openAIWarmPoolSF.Forget("openai_warm_pool")
	openAIWarmPoolCache.Store((*cachedOpenAIWarmPoolSettings)(nil))
	defer func() {
		openAIWarmPoolSF.Forget("openai_warm_pool")
		openAIWarmPoolCache.Store((*cachedOpenAIWarmPoolSettings)(nil))
	}()
	pool := svc.getOpenAIWarmPool()
	defer pool.Stop()

	svc.SetOpenAIWarmPoolUsageReader(&openAIWarmPoolUsageReaderStub{})
	require.Eventually(t, func() bool {
		return pool.countWarmReady(cloneAccounts(accounts)) == 1
	}, time.Second, 10*time.Millisecond)

	opsSvc := &OpsService{cfg: cfg, openAIGatewayService: svc}
	stats, err := opsSvc.GetOpenAIWarmPoolStats(ctx, nil)
	require.NoError(t, err)
	require.NotNil(t, stats.Summary)
	require.Equal(t, 1, stats.Summary.GlobalReadyAccountCount)
	require.Equal(t, 1, stats.Summary.ActiveGroupCount, "startup 预热组在无人访问时也应继续参与 OPS 覆盖统计")
	require.Len(t, stats.GlobalCoverages, 1)
	require.Equal(t, groupID, stats.GlobalCoverages[0].GroupID)
	require.Equal(t, 1, stats.GlobalCoverages[0].CoverageCount)
	require.Len(t, stats.Buckets, 1, "startup 预热组在无人访问时也应出现在分组池列表中")
	require.Equal(t, groupID, stats.Buckets[0].GroupID)
	require.Equal(t, 1, stats.Buckets[0].SchedulableAccounts)
	require.GreaterOrEqual(t, stats.Buckets[0].BucketReadyAccounts, cfg.Gateway.OpenAIWS.AccountWarmPool.BucketSyncFillMin)
}

func TestOpsServiceGetOpenAIWarmPoolStats_DoesNotReuseReaderReadyCacheAcrossReaderChanges(t *testing.T) {
	ctx := context.Background()
	cfg := newOpenAIWarmPoolTestConfig()
	cfg.Ops.Enabled = true
	groupID := int64(93005)
	group := &Group{ID: groupID, Name: "Reader Ready Group", Platform: PlatformOpenAI}
	accounts := []Account{{ID: 93051, Name: "reader-ready-account", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0, Groups: []*Group{group}}}
	svc := newOpenAIWarmPoolTestService(cfg, accounts, stubConcurrencyCache{})
	svc.SetOpenAIWarmPoolUsageReader(&openAIWarmPoolUsageReaderStub{})
	pool := svc.getOpenAIWarmPool()
	defer pool.Stop()
	waitForStartupBootstrapToSettle(t, pool)

	now := time.Now()
	pool.accountState(93051).finishSuccess(now, pool.config().GlobalEntryTTL)
	require.True(t, pool.bucketState(groupID).promote(93051, now, pool.config().BucketEntryTTL))
	pool.bucketState(groupID).lastAccess.Store(now.UnixNano())

	opsSvc := &OpsService{cfg: cfg, openAIGatewayService: svc}
	stats, err := opsSvc.GetOpenAIWarmPoolStats(ctx, nil)
	require.NoError(t, err)
	require.True(t, stats.ReaderReady)

	svc.SetOpenAIWarmPoolUsageReader(nil)
	stats, err = opsSvc.GetOpenAIWarmPoolStats(ctx, nil)
	require.NoError(t, err)
	require.False(t, stats.ReaderReady, "reader 状态变化后不应继续复用旧的 warm-pool 结果缓存")
}
