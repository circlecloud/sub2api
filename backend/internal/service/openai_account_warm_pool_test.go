package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"

	"github.com/stretchr/testify/require"
)

type openAIWarmPoolUsageReaderStub struct {
	mu        sync.Mutex
	calls     int
	results   map[int64]*UsageInfo
	resultSeq map[int64][]*UsageInfo
	errs      map[int64]error
	errSeq    map[int64][]error
}

type warmPoolSettingRepoStub struct {
	values map[string]string
}

type warmPoolProxyRepoStub struct {
	proxy *Proxy
	err   error
}

type warmPoolProxyProberStub struct {
	mu       sync.Mutex
	calls    int
	lastHost string //nolint:unused // 预留给后续断言最近探测目标
	exitInfo *ProxyExitInfo
	err      error
}

type blockingWarmMirrorCache struct {
	stubOpsRealtimeCache
	started chan struct{}
	release chan struct{}
}

//nolint:unused // 预留给后续 fallback 禁用路径测试桩
type noFallbackWarmPoolAccountRepo struct {
	stubOpenAIAccountRepo
	groupPlatformCalls int
}

func (s *warmPoolSettingRepoStub) Get(ctx context.Context, key string) (*Setting, error) {
	if s == nil {
		return nil, ErrSettingNotFound
	}
	value, ok := s.values[key]
	if !ok {
		return nil, ErrSettingNotFound
	}
	return &Setting{Key: key, Value: value}, nil
}

func (s *warmPoolSettingRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	setting, err := s.Get(ctx, key)
	if err != nil {
		return "", err
	}
	return setting.Value, nil
}

func (s *warmPoolSettingRepoStub) Set(ctx context.Context, key, value string) error {
	if s.values == nil {
		s.values = make(map[string]string)
	}
	s.values[key] = value
	return nil
}

func (s *warmPoolSettingRepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	result := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			result[key] = value
		}
	}
	return result, nil
}

func (s *warmPoolSettingRepoStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	if s.values == nil {
		s.values = make(map[string]string)
	}
	for key, value := range settings {
		s.values[key] = value
	}
	return nil
}

func (s *warmPoolSettingRepoStub) GetAll(ctx context.Context) (map[string]string, error) {
	result := make(map[string]string, len(s.values))
	for key, value := range s.values {
		result[key] = value
	}
	return result, nil
}

func (s *warmPoolSettingRepoStub) Delete(ctx context.Context, key string) error {
	if s == nil || s.values == nil {
		return nil
	}
	delete(s.values, key)
	return nil
}

func (s *openAIWarmPoolUsageReaderStub) GetUsage(ctx context.Context, accountID int64, forceRefresh bool) (*UsageInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	if s.errSeq != nil {
		if seq, ok := s.errSeq[accountID]; ok && len(seq) > 0 {
			err := seq[0]
			s.errSeq[accountID] = seq[1:]
			if err != nil {
				return nil, err
			}
		}
	}
	if s.resultSeq != nil {
		if seq, ok := s.resultSeq[accountID]; ok && len(seq) > 0 {
			result := seq[0]
			s.resultSeq[accountID] = seq[1:]
			if result != nil {
				return result, nil
			}
		}
	}
	if s.errs != nil {
		if err, ok := s.errs[accountID]; ok {
			return nil, err
		}
	}
	if s.results != nil {
		if result, ok := s.results[accountID]; ok {
			return result, nil
		}
	}
	now := time.Now()
	return &UsageInfo{UpdatedAt: &now}, nil
}

func (s *openAIWarmPoolUsageReaderStub) CallCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func (s *openAIWarmPoolUsageReaderStub) ResetCalls() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = 0
}

type blockingWarmPoolUsageReaderStub struct {
	mu      sync.Mutex
	calls   int
	started chan struct{}
	release chan struct{}
}

func newBlockingWarmPoolUsageReaderStub() *blockingWarmPoolUsageReaderStub {
	return &blockingWarmPoolUsageReaderStub{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
}

func (s *blockingWarmPoolUsageReaderStub) GetUsage(ctx context.Context, accountID int64, forceRefresh bool) (*UsageInfo, error) {
	s.mu.Lock()
	s.calls++
	started := s.started
	release := s.release
	s.mu.Unlock()
	select {
	case <-started:
	default:
		close(started)
	}
	if release != nil {
		select {
		case <-release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	now := time.Now()
	return &UsageInfo{UpdatedAt: &now}, nil
}

func (s *blockingWarmPoolUsageReaderStub) Release() {
	if s == nil || s.release == nil {
		return
	}
	select {
	case <-s.release:
	default:
		close(s.release)
	}
}

func (s *blockingWarmPoolUsageReaderStub) Started() <-chan struct{} {
	if s == nil {
		return nil
	}
	return s.started
}

func (s *warmPoolProxyRepoStub) Create(ctx context.Context, proxy *Proxy) error {
	panic("unexpected Create call")
}

func (s *warmPoolProxyRepoStub) GetByID(ctx context.Context, id int64) (*Proxy, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.proxy == nil || s.proxy.ID != id {
		return nil, ErrProxyNotFound
	}
	return s.proxy, nil
}

func (s *warmPoolProxyRepoStub) ListByIDs(ctx context.Context, ids []int64) ([]Proxy, error) {
	panic("unexpected ListByIDs call")
}

func (s *warmPoolProxyRepoStub) Update(ctx context.Context, proxy *Proxy) error {
	panic("unexpected Update call")
}

func (s *warmPoolProxyRepoStub) Delete(ctx context.Context, id int64) error {
	panic("unexpected Delete call")
}

func (s *warmPoolProxyRepoStub) List(ctx context.Context, params pagination.PaginationParams) ([]Proxy, *pagination.PaginationResult, error) {
	panic("unexpected List call")
}

func (s *warmPoolProxyRepoStub) ListWithFilters(ctx context.Context, params pagination.PaginationParams, protocol, status, search string) ([]Proxy, *pagination.PaginationResult, error) {
	panic("unexpected ListWithFilters call")
}

func (s *warmPoolProxyRepoStub) ListActive(ctx context.Context) ([]Proxy, error) {
	panic("unexpected ListActive call")
}

func (s *warmPoolProxyRepoStub) ListActiveWithAccountCount(ctx context.Context) ([]ProxyWithAccountCount, error) {
	panic("unexpected ListActiveWithAccountCount call")
}

func (s *warmPoolProxyRepoStub) ListWithFiltersAndAccountCount(ctx context.Context, params pagination.PaginationParams, protocol, status, search string) ([]ProxyWithAccountCount, *pagination.PaginationResult, error) {
	panic("unexpected ListWithFiltersAndAccountCount call")
}

func (s *warmPoolProxyRepoStub) ExistsByHostPortAuth(ctx context.Context, host string, port int, username, password string) (bool, error) {
	panic("unexpected ExistsByHostPortAuth call")
}

func (s *warmPoolProxyRepoStub) CountAccountsByProxyID(ctx context.Context, proxyID int64) (int64, error) {
	panic("unexpected CountAccountsByProxyID call")
}

func (s *warmPoolProxyRepoStub) ListAccountSummariesByProxyID(ctx context.Context, proxyID int64) ([]ProxyAccountSummary, error) {
	panic("unexpected ListAccountSummariesByProxyID call")
}

func (s *warmPoolProxyProberStub) ProbeProxy(ctx context.Context, proxyURL string) (*ProxyExitInfo, int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	if s.err != nil {
		return nil, 0, s.err
	}
	if s.exitInfo != nil {
		return s.exitInfo, 25, nil
	}
	return &ProxyExitInfo{IP: "1.1.1.1", Country: "US"}, 25, nil
}

func (s *warmPoolProxyProberStub) CallCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func (c *blockingWarmMirrorCache) SetWarmAccountState(ctx context.Context, state *OpsRealtimeWarmAccountState) error {
	select {
	case <-c.started:
	default:
		close(c.started)
	}
	select {
	case <-c.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

//nolint:unused // 预留给后续 fallback 禁用路径测试桩
func (r *noFallbackWarmPoolAccountRepo) ListSchedulableByGroupIDAndPlatform(ctx context.Context, groupID int64, platform string) ([]Account, error) {
	r.groupPlatformCalls++
	return r.stubOpenAIAccountRepo.ListSchedulableByGroupIDAndPlatform(ctx, groupID, platform)
}

func newOpenAIWarmPoolTestConfig() *config.Config {

	cfg := &config.Config{}
	cfg.Gateway.Scheduling.LoadBatchEnabled = true
	cfg.Gateway.OpenAIWS.AccountWarmPool.Enabled = true
	cfg.Gateway.OpenAIWS.AccountWarmPool.BucketTargetSize = 2
	cfg.Gateway.OpenAIWS.AccountWarmPool.BucketRefillBelow = 1
	cfg.Gateway.OpenAIWS.AccountWarmPool.BucketSyncFillMin = 1
	cfg.Gateway.OpenAIWS.AccountWarmPool.BucketEntryTTLSeconds = 30
	cfg.Gateway.OpenAIWS.AccountWarmPool.BucketRefillCooldownSeconds = 0
	cfg.Gateway.OpenAIWS.AccountWarmPool.BucketRefillIntervalSeconds = 0
	cfg.Gateway.OpenAIWS.AccountWarmPool.GlobalTargetSize = 4
	cfg.Gateway.OpenAIWS.AccountWarmPool.GlobalRefillBelow = 2
	cfg.Gateway.OpenAIWS.AccountWarmPool.GlobalEntryTTLSeconds = 300
	cfg.Gateway.OpenAIWS.AccountWarmPool.GlobalRefillCooldownSeconds = 0
	cfg.Gateway.OpenAIWS.AccountWarmPool.GlobalRefillIntervalSeconds = 0
	cfg.Gateway.OpenAIWS.AccountWarmPool.NetworkErrorPoolSize = 3
	cfg.Gateway.OpenAIWS.AccountWarmPool.NetworkErrorEntryTTLSeconds = 120
	cfg.Gateway.OpenAIWS.AccountWarmPool.ProbeMaxCandidates = 8
	cfg.Gateway.OpenAIWS.AccountWarmPool.ProbeConcurrency = 1
	cfg.Gateway.OpenAIWS.AccountWarmPool.ProbeTimeoutSeconds = 5
	cfg.Gateway.OpenAIWS.AccountWarmPool.ProbeFailureCooldownSeconds = 30
	return cfg
}

func newOpenAIWarmPoolTestService(cfg *config.Config, accounts []Account, cache ConcurrencyCache) *OpenAIGatewayService {
	svc := &OpenAIGatewayService{
		accountRepo:        stubOpenAIAccountRepo{accounts: accounts},
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(cache),
	}
	svc.openaiWarmPool = newOpenAIAccountWarmPoolService(svc)
	return svc
}

func waitForStartupBootstrapToSettle(t *testing.T, pool *openAIAccountWarmPoolService) {
	t.Helper()
	require.Eventually(t, func() bool {
		return pool == nil || (!pool.isStartupBootstrapping() && !pool.startupBootstrapRunning.Load())
	}, time.Second, 10*time.Millisecond)
}

func clearWarmPoolReadyStates(pool *openAIAccountWarmPoolService, accounts []Account) {
	if pool == nil {
		return
	}
	for i := range accounts {
		if accounts[i].ID <= 0 {
			continue
		}
		state := pool.accountState(accounts[i].ID)
		if state == nil {
			continue
		}
		state.mu.Lock()
		state.verifiedAt = time.Time{}
		state.expiresAt = time.Time{}
		state.failUntil = time.Time{}
		state.networkErrorAt = time.Time{}
		state.networkErrorUntil = time.Time{}
		state.probing = false
		state.mu.Unlock()
	}
}

func TestOpenAIWarmPoolMirrorWritesAreAsync(t *testing.T) {
	cfg := newOpenAIWarmPoolTestConfig()
	svc := newOpenAIWarmPoolTestService(cfg, []Account{{ID: 80001, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1}}, stubConcurrencyCache{})
	cache := &blockingWarmMirrorCache{started: make(chan struct{}), release: make(chan struct{})}
	svc.opsRealtimeCache = cache
	pool := svc.getOpenAIWarmPool()
	defer pool.Stop()

	start := time.Now()
	pool.syncWarmAccountState(80001, pool.accountState(80001), time.Now())
	elapsed := time.Since(start)
	require.Less(t, elapsed, 100*time.Millisecond, "镜像 Redis 写入应异步执行，不能阻塞实时选号路径")

	select {
	case <-cache.started:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected async mirror worker to pick up queued state write")
	}
	close(cache.release)
}

func TestOpenAIWarmPoolPrefersBucketAccountInScheduler(t *testing.T) {
	ctx := context.Background()
	groupID := int64(8101)
	cfg := newOpenAIWarmPoolTestConfig()
	accounts := []Account{
		{ID: 81001, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0},
		{ID: 81002, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 10},
	}
	svc := newOpenAIWarmPoolTestService(cfg, accounts, stubConcurrencyCache{})
	pool := svc.getOpenAIWarmPool()
	now := time.Now()
	pool.accountState(81002).finishSuccess(now, pool.config().GlobalEntryTTL)
	require.True(t, pool.bucketState(groupID).promote(81002, now, pool.config().BucketEntryTTL))

	selection, decision, err := svc.SelectAccountWithScheduler(ctx, &groupID, "", "session_bucket_prefer", "gpt-5.1", nil, OpenAIUpstreamTransportAny)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, int64(81002), selection.Account.ID)
	require.Equal(t, openAIAccountScheduleLayerLoadBalance, decision.Layer)
	coverage := pool.buildGlobalCoverageSnapshot(time.Now(), cloneAccounts(accounts), pool.globalCoverageGroupIDs(time.Now(), cloneAccounts(accounts)))
	require.Equal(t, int64(1), pool.buildOpsWarmPoolSummary(time.Now(), pool.config().ActiveBucketTTL, coverage).TakeCount)
}

func TestOpenAIWarmPoolBusyReadyAccountsDoNotFallbackToColdAccounts(t *testing.T) {
	ctx := context.Background()
	groupID := int64(8102)
	group := &Group{ID: groupID, Name: "G8102", Platform: PlatformOpenAI}
	cfg := newOpenAIWarmPoolTestConfig()
	cfg.Gateway.Scheduling.FallbackMaxWaiting = 3
	cfg.Gateway.Scheduling.FallbackWaitTimeout = 30 * time.Second
	accounts := []Account{
		{ID: 81101, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0, Groups: []*Group{group}, Credentials: map[string]any{"model_mapping": map[string]any{"gpt-5.4": "gpt-5.4"}}},
		{ID: 81102, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 10, Groups: []*Group{group}, Credentials: map[string]any{"model_mapping": map[string]any{"gpt-5.4": "gpt-5.4"}}},
	}
	concurrencyCache := stubConcurrencyCache{
		acquireResults: map[int64]bool{
			81101: false,
			81102: true,
		},
		waitCounts: map[int64]int{
			81101: 0,
		},
		loadMap: map[int64]*AccountLoadInfo{
			81101: {AccountID: 81101, LoadRate: 0, WaitingCount: 0},
			81102: {AccountID: 81102, LoadRate: 0, WaitingCount: 0},
		},
	}
	svc := newOpenAIWarmPoolTestService(cfg, accounts, concurrencyCache)
	pool := svc.getOpenAIWarmPool()
	now := time.Now()
	pool.accountState(81101).finishSuccess(now, pool.config().GlobalEntryTTL)
	require.True(t, pool.bucketState(groupID).promote(81101, now, pool.config().BucketEntryTTL))

	selection, decision, err := svc.SelectAccountWithScheduler(ctx, &groupID, "", "session_bucket_busy", "gpt-5.4", nil, OpenAIUpstreamTransportAny)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, int64(81101), selection.Account.ID, "只要预热池未空，应继续优先使用 warm 账号而不是切到冷账号")
	require.False(t, selection.Acquired)
	require.NotNil(t, selection.WaitPlan)
	require.Equal(t, int64(81101), selection.WaitPlan.AccountID)
	require.Equal(t, openAIAccountScheduleLayerLoadBalance, decision.Layer)
}

func TestOpenAIWarmPoolPromotesGlobalReadyIntoBucketWithoutReprobe(t *testing.T) {
	groupID := int64(8201)
	group := &Group{ID: groupID, Name: "G8201", Platform: PlatformOpenAI}
	cfg := newOpenAIWarmPoolTestConfig()
	accounts := []Account{{ID: 82001, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0, Groups: []*Group{group}}}
	svc := newOpenAIWarmPoolTestService(cfg, accounts, stubConcurrencyCache{})
	reader := &openAIWarmPoolUsageReaderStub{}
	svc.SetOpenAIWarmPoolUsageReader(reader)
	pool := svc.getOpenAIWarmPool()
	waitForStartupBootstrapToSettle(t, pool)
	clearWarmPoolReadyStates(pool, accounts)
	reader.ResetCalls()
	pool.accountState(82001).finishSuccess(time.Now(), pool.config().GlobalEntryTTL)

	warmed := pool.WarmCandidates(context.Background(), &groupID, cloneAccounts(accounts), "gpt-5.1", nil)
	require.Len(t, warmed, 1)
	require.Equal(t, int64(82001), warmed[0].ID)
	require.Zero(t, reader.CallCount(), "已有 Global ready 时应直接提升到 Bucket")
	require.Equal(t, 1, pool.countBucketWarmReady(groupID, cloneAccounts(accounts)))
}

func TestOpenAIWarmPoolLowWaterRefillsBucketFromGlobalReadyWithoutModelFilter(t *testing.T) {
	groupID := int64(8202)
	group := &Group{ID: groupID, Name: "G8202", Platform: PlatformOpenAI}
	cfg := newOpenAIWarmPoolTestConfig()
	cfg.Gateway.OpenAIWS.AccountWarmPool.BucketTargetSize = 3
	cfg.Gateway.OpenAIWS.AccountWarmPool.BucketRefillBelow = 2
	cfg.Gateway.OpenAIWS.AccountWarmPool.BucketSyncFillMin = 1
	accounts := []Account{
		{ID: 82021, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0, Groups: []*Group{group}, Credentials: map[string]any{"model_mapping": map[string]any{"gpt-4o": "gpt-4o"}}},
		{ID: 82022, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1, Groups: []*Group{group}, Credentials: map[string]any{"model_mapping": map[string]any{"gpt-4o": "gpt-4o"}}},
		{ID: 82023, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 2, Groups: []*Group{group}, Credentials: map[string]any{"model_mapping": map[string]any{"gpt-5.1": "gpt-5.1"}}},
	}
	svc := newOpenAIWarmPoolTestService(cfg, accounts, stubConcurrencyCache{})
	reader := &openAIWarmPoolUsageReaderStub{}
	svc.SetOpenAIWarmPoolUsageReader(reader)
	pool := svc.getOpenAIWarmPool()
	waitForStartupBootstrapToSettle(t, pool)
	clearWarmPoolReadyStates(pool, accounts)
	reader.ResetCalls()
	pool.accountState(82021).finishSuccess(time.Now(), pool.config().GlobalEntryTTL)
	pool.accountState(82022).finishSuccess(time.Now(), pool.config().GlobalEntryTTL)

	warmed := pool.WarmCandidates(context.Background(), &groupID, cloneAccounts(accounts), "gpt-5.1", nil)
	require.Len(t, warmed, 1)
	require.Equal(t, int64(82023), warmed[0].ID, "首个请求仍应优先补出当前模型可用账号")
	require.Eventually(t, func() bool {
		return pool.countBucketWarmReady(groupID, cloneAccounts(accounts)) >= 2
	}, time.Second, 10*time.Millisecond, "分组池低于低水位时应直接从已有 global ready 回填 bucket，而不是受当前请求模型过滤限制")
	require.Equal(t, 1, reader.CallCount(), "低水位回填应优先复用已有 global ready，只为首个模型命中账号做一次探测")
}

func TestOpenAIWarmPoolSharesGlobalReadyAcrossGroupsWithoutDuplicateProbe(t *testing.T) {
	cfg := newOpenAIWarmPoolTestConfig()
	groupA := &Group{ID: 9101, Name: "A", Platform: PlatformOpenAI}
	groupB := &Group{ID: 9102, Name: "B", Platform: PlatformOpenAI}
	accounts := []Account{{ID: 83001, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0, Groups: []*Group{groupA, groupB}}}
	svc := newOpenAIWarmPoolTestService(cfg, accounts, stubConcurrencyCache{})
	reader := &openAIWarmPoolUsageReaderStub{}
	svc.SetOpenAIWarmPoolUsageReader(reader)
	pool := svc.getOpenAIWarmPool()
	waitForStartupBootstrapToSettle(t, pool)
	clearWarmPoolReadyStates(pool, accounts)
	reader.ResetCalls()

	first := pool.WarmCandidates(context.Background(), &groupA.ID, cloneAccounts(accounts), "gpt-5.1", nil)
	require.Len(t, first, 1)
	require.Equal(t, 1, reader.CallCount(), "首次访问应触发一次探测")

	second := pool.WarmCandidates(context.Background(), &groupB.ID, cloneAccounts(accounts), "gpt-5.1", nil)
	require.Len(t, second, 1)
	require.Equal(t, 1, reader.CallCount(), "同一账号被其他分组复用时不应重复探测")
	require.Equal(t, 1, pool.countBucketWarmReady(groupA.ID, cloneAccounts(accounts)))
	require.Equal(t, 1, pool.countBucketWarmReady(groupB.ID, cloneAccounts(accounts)))
}

func TestOpenAIWarmPoolPrefersOwnerAccountBeforeBorrowingSharedReady(t *testing.T) {
	cfg := newOpenAIWarmPoolTestConfig()
	cfg.Gateway.OpenAIWS.AccountWarmPool.BucketTargetSize = 1
	cfg.Gateway.OpenAIWS.AccountWarmPool.BucketRefillBelow = 1
	cfg.Gateway.OpenAIWS.AccountWarmPool.BucketSyncFillMin = 1
	groupA := &Group{ID: 9201, Name: "A", Platform: PlatformOpenAI}
	groupB := &Group{ID: 9202, Name: "B", Platform: PlatformOpenAI}
	accounts := []Account{
		{ID: 92011, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0, Groups: []*Group{groupA, groupB}, Credentials: map[string]any{"model_mapping": map[string]any{"gpt-5.1": "gpt-5.1"}}},
		{ID: 92012, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1, Groups: []*Group{groupA, groupB}, Credentials: map[string]any{"model_mapping": map[string]any{"gpt-5.1": "gpt-5.1"}}},
		{ID: 92013, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 2, Groups: []*Group{groupA, groupB}, Credentials: map[string]any{"model_mapping": map[string]any{"gpt-5.1": "gpt-5.1"}}},
		{ID: 92014, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 3, Groups: []*Group{groupA, groupB}, Credentials: map[string]any{"model_mapping": map[string]any{"gpt-5.1": "gpt-5.1"}}},
	}
	svc := newOpenAIWarmPoolTestService(cfg, accounts, stubConcurrencyCache{})
	reader := &openAIWarmPoolUsageReaderStub{}
	svc.SetOpenAIWarmPoolUsageReader(reader)
	pool := svc.getOpenAIWarmPool()
	waitForStartupBootstrapToSettle(t, pool)
	clearWarmPoolReadyStates(pool, accounts)
	reader.ResetCalls()

	var ownerA, ownerB *Account
	for i := range accounts {
		acc := &accounts[i]
		switch warmPoolBucketOwnerGroupID(pool, acc) {
		case groupA.ID:
			if ownerA == nil {
				ownerA = acc
			}
		case groupB.ID:
			if ownerB == nil {
				ownerB = acc
			}
		}
	}
	require.NotNil(t, ownerA)
	require.NotNil(t, ownerB)
	pool.accountState(ownerB.ID).finishSuccess(time.Now(), pool.config().GlobalEntryTTL)

	warmed := pool.WarmCandidates(context.Background(), &groupA.ID, cloneAccounts(accounts), "gpt-5.1", nil)
	require.Len(t, warmed, 1)
	require.Equal(t, ownerA.ID, warmed[0].ID, "当前组 owner 容量充足时，应优先补自己的 owner 账号，而不是直接借用别组已 ready 的共享账号")
	require.Equal(t, 1, reader.CallCount(), "应先探测 owner 账号进入全局池")
	require.Equal(t, 1, pool.countBucketWarmReady(groupA.ID, cloneAccounts(accounts)))
}

func TestOpenAIWarmPoolBorrowsSharedGlobalReadyWhenOwnerCapacityInsufficient(t *testing.T) {
	cfg := newOpenAIWarmPoolTestConfig()
	cfg.Gateway.OpenAIWS.AccountWarmPool.BucketTargetSize = 1
	cfg.Gateway.OpenAIWS.AccountWarmPool.BucketRefillBelow = 1
	cfg.Gateway.OpenAIWS.AccountWarmPool.BucketSyncFillMin = 1
	groupA := &Group{ID: 9211, Name: "A", Platform: PlatformOpenAI}
	groupB := &Group{ID: 9212, Name: "B", Platform: PlatformOpenAI}
	account := Account{ID: 92101, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0, Groups: []*Group{groupA, groupB}, Credentials: map[string]any{"model_mapping": map[string]any{"gpt-5.1": "gpt-5.1"}}}
	svc := newOpenAIWarmPoolTestService(cfg, []Account{account}, stubConcurrencyCache{})
	reader := &openAIWarmPoolUsageReaderStub{}
	svc.SetOpenAIWarmPoolUsageReader(reader)
	pool := svc.getOpenAIWarmPool()
	waitForStartupBootstrapToSettle(t, pool)
	clearWarmPoolReadyStates(pool, []Account{account})
	reader.ResetCalls()
	pool.accountState(account.ID).finishSuccess(time.Now(), pool.config().GlobalEntryTTL)

	ownerGroupID := warmPoolBucketOwnerGroupID(pool, &account)
	borrowGroupID := groupA.ID
	if ownerGroupID == borrowGroupID {
		borrowGroupID = groupB.ID
	}

	warmed := pool.WarmCandidates(context.Background(), &borrowGroupID, cloneAccounts([]Account{account}), "gpt-5.1", nil)
	require.Len(t, warmed, 1)
	require.Equal(t, account.ID, warmed[0].ID, "当前组 owner 容量不足时，应允许直接借用全局池里已 ready 的共享账号")
	require.Zero(t, reader.CallCount(), "借用已在全局池 ready 的共享账号时不应重复探测")
	require.Equal(t, 1, pool.countBucketWarmReady(borrowGroupID, cloneAccounts([]Account{account})))
}

func TestOpenAIWarmPoolDistributesSharedAccountsAcrossOwnerBuckets(t *testing.T) {
	cfg := newOpenAIWarmPoolTestConfig()
	cfg.Gateway.OpenAIWS.AccountWarmPool.BucketTargetSize = 2
	cfg.Gateway.OpenAIWS.AccountWarmPool.BucketRefillBelow = 1
	cfg.Gateway.OpenAIWS.AccountWarmPool.BucketSyncFillMin = 1
	groupA := &Group{ID: 9221, Name: "A", Platform: PlatformOpenAI}
	groupB := &Group{ID: 9222, Name: "B", Platform: PlatformOpenAI}
	seedPool := newOpenAIWarmPoolTestService(cfg, nil, stubConcurrencyCache{}).getOpenAIWarmPool()
	accounts := make([]Account, 0, 8)
	ownerCountA := 0
	ownerCountB := 0
	for accountID := int64(92201); accountID < 92232 && (ownerCountA < 2 || ownerCountB < 2); accountID++ {
		account := Account{ID: accountID, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: int(accountID - 92201), Groups: []*Group{groupA, groupB}}
		accounts = append(accounts, account)
		if warmPoolBucketOwnerGroupID(seedPool, &account) == groupA.ID {
			ownerCountA++
		} else {
			ownerCountB++
		}
	}
	require.GreaterOrEqual(t, ownerCountA, 2)
	require.GreaterOrEqual(t, ownerCountB, 2)

	svc := newOpenAIWarmPoolTestService(cfg, accounts, stubConcurrencyCache{})
	svc.SetOpenAIWarmPoolUsageReader(&openAIWarmPoolUsageReaderStub{})
	pool := svc.getOpenAIWarmPool()
	for i := range accounts {
		pool.accountState(accounts[i].ID).finishSuccess(time.Now(), pool.config().GlobalEntryTTL)
	}

	pool.ensureBucketReady(context.Background(), groupA.ID, cloneAccounts(accounts), "", nil, true)
	pool.ensureBucketReady(context.Background(), groupB.ID, cloneAccounts(accounts), "", nil, true)
	first := pool.collectBucketWarmCandidates(groupA.ID, cloneAccounts(accounts), "", nil)
	second := pool.collectBucketWarmCandidates(groupB.ID, cloneAccounts(accounts), "", nil)
	require.Len(t, first, 2)
	require.Len(t, second, 2)
	require.Equal(t, 2, pool.countBucketWarmReady(groupA.ID, cloneAccounts(accounts)), "owner 容量足够时，A 组 bucket 应只使用自己 owner 到的共享账号")
	require.Equal(t, 2, pool.countBucketWarmReady(groupB.ID, cloneAccounts(accounts)), "owner 容量足够时，B 组 bucket 应只使用自己 owner 到的共享账号")
}

func TestOpenAIWarmPoolInitialFillFillsBucketThenGlobal(t *testing.T) {
	cfg := newOpenAIWarmPoolTestConfig()
	cfg.Gateway.OpenAIWS.AccountWarmPool.BucketTargetSize = 1
	cfg.Gateway.OpenAIWS.AccountWarmPool.GlobalTargetSize = 2
	groupID := int64(9301)
	group := &Group{ID: groupID, Name: "G9301", Platform: PlatformOpenAI}
	accounts := []Account{
		{ID: 93001, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0, Groups: []*Group{group}},
		{ID: 93002, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1, Groups: []*Group{group}},
	}
	svc := newOpenAIWarmPoolTestService(cfg, accounts, stubConcurrencyCache{})
	reader := &openAIWarmPoolUsageReaderStub{}
	svc.SetOpenAIWarmPoolUsageReader(reader)
	pool := svc.getOpenAIWarmPool()

	warmed := pool.WarmCandidates(context.Background(), &groupID, cloneAccounts(accounts), "", nil)
	require.Len(t, warmed, 1)
	require.Equal(t, 1, pool.countBucketWarmReady(groupID, cloneAccounts(accounts)))
	require.Eventually(t, func() bool {
		return pool.countWarmReady(cloneAccounts(accounts)) == 2
	}, time.Second, 10*time.Millisecond, "初始化补号应先放行首个 Bucket 账号，再继续后台补满 Global")
	require.Eventually(t, func() bool {
		return reader.CallCount() == 2
	}, time.Second, 10*time.Millisecond)
}

func TestOpenAIWarmPoolColdStartReturnsFirstReadyThenContinuesAsyncFill(t *testing.T) {
	cfg := newOpenAIWarmPoolTestConfig()
	cfg.Gateway.OpenAIWS.AccountWarmPool.BucketTargetSize = 3
	cfg.Gateway.OpenAIWS.AccountWarmPool.BucketSyncFillMin = 3
	cfg.Gateway.OpenAIWS.AccountWarmPool.GlobalTargetSize = 3
	groupID := int64(9341)
	group := &Group{ID: groupID, Name: "G9341", Platform: PlatformOpenAI}
	accounts := []Account{
		{ID: 93401, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0, Groups: []*Group{group}},
		{ID: 93402, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1, Groups: []*Group{group}},
		{ID: 93403, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 2, Groups: []*Group{group}},
	}
	svc := newOpenAIWarmPoolTestService(cfg, accounts, stubConcurrencyCache{})
	reader := &openAIWarmPoolUsageReaderStub{}
	svc.SetOpenAIWarmPoolUsageReader(reader)
	pool := svc.getOpenAIWarmPool()

	warmed := pool.WarmCandidates(context.Background(), &groupID, cloneAccounts(accounts), "", nil)
	require.Len(t, warmed, 1, "冷启动首个请求拿到 1 个可用账号后就应立即返回")
	require.Equal(t, 1, pool.countBucketWarmReady(groupID, cloneAccounts(accounts)))
	require.Eventually(t, func() bool {
		return pool.countBucketWarmReady(groupID, cloneAccounts(accounts)) == 3
	}, time.Second, 10*time.Millisecond, "首个账号放行后，后台仍应继续把分组池补满")
}

func TestOpenAIWarmPoolBootstrapGlobalAfterBucketEventuallyBecomesFull(t *testing.T) {
	cfg := newOpenAIWarmPoolTestConfig()
	cfg.Gateway.OpenAIWS.AccountWarmPool.BucketTargetSize = 2
	cfg.Gateway.OpenAIWS.AccountWarmPool.BucketSyncFillMin = 2
	cfg.Gateway.OpenAIWS.AccountWarmPool.GlobalTargetSize = 3
	cfg.Gateway.OpenAIWS.AccountWarmPool.NetworkErrorEntryTTLSeconds = 0
	cfg.Gateway.OpenAIWS.AccountWarmPool.ProbeFailureCooldownSeconds = 0
	groupID := int64(9351)
	group := &Group{ID: groupID, Name: "G9351", Platform: PlatformOpenAI}
	accounts := []Account{
		{ID: 93501, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0, Groups: []*Group{group}},
		{ID: 93502, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1, Groups: []*Group{group}},
		{ID: 93503, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 2, Groups: []*Group{group}},
	}
	svc := newOpenAIWarmPoolTestService(cfg, accounts, stubConcurrencyCache{})
	reader := &openAIWarmPoolUsageReaderStub{errs: map[int64]error{
		93502: errors.New("temporary usage error"),
		93503: errors.New("temporary usage error"),
	}}
	svc.SetOpenAIWarmPoolUsageReader(reader)
	pool := svc.getOpenAIWarmPool()

	first := pool.WarmCandidates(context.Background(), &groupID, cloneAccounts(accounts), "", nil)
	require.Len(t, first, 1)
	require.Equal(t, 1, pool.countBucketWarmReady(groupID, cloneAccounts(accounts)))
	require.Equal(t, 1, pool.countWarmReady(cloneAccounts(accounts)))

	reader.errs = nil
	require.Eventually(t, func() bool {
		return pool.countBucketWarmReady(groupID, cloneAccounts(accounts)) == 2 && pool.countWarmReady(cloneAccounts(accounts)) == 3
	}, time.Second, 10*time.Millisecond, "分组池后续补满后也应继续触发全局池补充")
	second := pool.WarmCandidates(context.Background(), &groupID, cloneAccounts(accounts), "", nil)
	require.Len(t, second, 2)
}

func TestOpenAIWarmPoolBootstrapGlobalIgnoresGlobalCooldown(t *testing.T) {
	cfg := newOpenAIWarmPoolTestConfig()
	cfg.Gateway.OpenAIWS.AccountWarmPool.BucketTargetSize = 1
	cfg.Gateway.OpenAIWS.AccountWarmPool.GlobalTargetSize = 2
	cfg.Gateway.OpenAIWS.AccountWarmPool.GlobalRefillCooldownSeconds = 3600
	groupID := int64(9361)
	group := &Group{ID: groupID, Name: "G9361", Platform: PlatformOpenAI}
	accounts := []Account{
		{ID: 93601, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0, Groups: []*Group{group}},
		{ID: 93602, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1, Groups: []*Group{group}},
	}
	svc := newOpenAIWarmPoolTestService(cfg, accounts, stubConcurrencyCache{})
	reader := &openAIWarmPoolUsageReaderStub{}
	svc.SetOpenAIWarmPoolUsageReader(reader)
	pool := svc.getOpenAIWarmPool()
	pool.lastGlobalRefill.Store(time.Now().UnixNano())

	warmed := pool.WarmCandidates(context.Background(), &groupID, cloneAccounts(accounts), "", nil)
	require.Len(t, warmed, 1)
	require.Eventually(t, func() bool {
		return pool.countWarmReady(cloneAccounts(accounts)) == 2
	}, time.Second, 10*time.Millisecond, "分组池首次补满触发的全局池补充不应被全局冷却阻塞")
}

func TestOpenAIWarmPoolBootstrapGlobalUpdatesLastGlobalMaintenance(t *testing.T) {
	cfg := newOpenAIWarmPoolTestConfig()
	cfg.Gateway.OpenAIWS.AccountWarmPool.BucketTargetSize = 1
	cfg.Gateway.OpenAIWS.AccountWarmPool.GlobalTargetSize = 2
	cfg.Gateway.OpenAIWS.AccountWarmPool.GlobalRefillCooldownSeconds = 3600
	groupID := int64(9362)
	group := &Group{ID: groupID, Name: "G9362", Platform: PlatformOpenAI}
	accounts := []Account{
		{ID: 93621, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0, Groups: []*Group{group}},
		{ID: 93622, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1, Groups: []*Group{group}},
	}
	svc := newOpenAIWarmPoolTestService(cfg, accounts, stubConcurrencyCache{})
	reader := &openAIWarmPoolUsageReaderStub{}
	svc.SetOpenAIWarmPoolUsageReader(reader)
	pool := svc.getOpenAIWarmPool()
	pool.lastGlobalMaintenance.Store(0)

	warmed := pool.WarmCandidates(context.Background(), &groupID, cloneAccounts(accounts), "", nil)
	require.Len(t, warmed, 1)
	require.Eventually(t, func() bool {
		return pool.countWarmReady(cloneAccounts(accounts)) == 2
	}, time.Second, 10*time.Millisecond, "分组池补满触发的全局池补充仍应正常执行")
	require.NotZero(t, pool.lastGlobalMaintenance.Load(), "全局池真实补池动作仍应更新最近全局维护时间")
}

func TestOpenAIWarmPoolManualGlobalRefillIgnoresCooldown(t *testing.T) {
	cfg := newOpenAIWarmPoolTestConfig()
	cfg.Gateway.OpenAIWS.AccountWarmPool.GlobalTargetSize = 2
	cfg.Gateway.OpenAIWS.AccountWarmPool.GlobalRefillCooldownSeconds = 3600
	groupID := int64(9364)
	group := &Group{ID: groupID, Name: "G9364", Platform: PlatformOpenAI}
	accounts := []Account{
		{ID: 93641, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0, Groups: []*Group{group}},
		{ID: 93642, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1, Groups: []*Group{group}},
	}
	svc := newOpenAIWarmPoolTestService(cfg, accounts, stubConcurrencyCache{})
	reader := &openAIWarmPoolUsageReaderStub{}
	svc.SetOpenAIWarmPoolUsageReader(reader)
	pool := svc.getOpenAIWarmPool()
	pool.bucketState(groupID).lastAccess.Store(time.Now().UnixNano())
	pool.lastGlobalRefill.Store(time.Now().UnixNano())

	require.NoError(t, pool.triggerManualGlobalRefill(context.Background()))
	require.Equal(t, 2, pool.countWarmReady(cloneAccounts(accounts)), "手动触发全局池补充时应忽略全局冷却并立即补池")
}

func TestOpenAIWarmPoolRefillWorkerAlsoMaintainsGlobalPool(t *testing.T) {
	cfg := newOpenAIWarmPoolTestConfig()
	cfg.Gateway.OpenAIWS.AccountWarmPool.BucketRefillIntervalSeconds = 0
	cfg.Gateway.OpenAIWS.AccountWarmPool.GlobalTargetSize = 1
	cfg.Gateway.OpenAIWS.AccountWarmPool.GlobalRefillBelow = 1
	cfg.Gateway.OpenAIWS.AccountWarmPool.GlobalRefillCooldownSeconds = 0
	cfg.Gateway.OpenAIWS.AccountWarmPool.GlobalRefillIntervalSeconds = 1
	groupID := int64(9365)
	group := &Group{ID: groupID, Name: "G9365", Platform: PlatformOpenAI}
	accounts := []Account{{ID: 93651, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0, Groups: []*Group{group}}}
	svc := newOpenAIWarmPoolTestService(cfg, accounts, stubConcurrencyCache{})
	reader := &openAIWarmPoolUsageReaderStub{}
	svc.SetOpenAIWarmPoolUsageReader(reader)
	pool := svc.getOpenAIWarmPool()
	defer pool.Stop()
	pool.bucketState(groupID).lastAccess.Store(time.Now().UnixNano())

	require.Eventually(t, func() bool {
		return pool.countWarmReady(cloneAccounts(accounts)) == 1
	}, 8*time.Second, 100*time.Millisecond, "refill worker 应周期性触发全局池维护，而不只是分组池维护")
	require.NotZero(t, pool.lastGlobalMaintenance.Load(), "worker 触发的全局维护应更新最近全局维护时间")
}

func TestOpenAIWarmPoolStartupBootstrapSkipsWhenStartupGroupsUnset(t *testing.T) {
	cfg := newOpenAIWarmPoolTestConfig()
	cfg.Gateway.OpenAIWS.AccountWarmPool.BucketRefillIntervalSeconds = 3600
	cfg.Gateway.OpenAIWS.AccountWarmPool.GlobalTargetSize = 1
	cfg.Gateway.OpenAIWS.AccountWarmPool.GlobalRefillBelow = 1
	cfg.Gateway.OpenAIWS.AccountWarmPool.GlobalRefillCooldownSeconds = 0
	cfg.Gateway.OpenAIWS.AccountWarmPool.GlobalRefillIntervalSeconds = 3600
	groupID := int64(9366)
	group := &Group{ID: groupID, Name: "G9366", Platform: PlatformOpenAI}
	accounts := []Account{{ID: 93661, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0, Groups: []*Group{group}}}
	svc := newOpenAIWarmPoolTestService(cfg, accounts, stubConcurrencyCache{})
	pool := svc.getOpenAIWarmPool()
	defer pool.Stop()

	require.Zero(t, pool.countWarmReady(cloneAccounts(accounts)))
	svc.SetOpenAIWarmPoolUsageReader(&openAIWarmPoolUsageReaderStub{})

	require.Never(t, func() bool {
		return pool.countWarmReady(cloneAccounts(accounts)) > 0
	}, 300*time.Millisecond, 10*time.Millisecond, "未配置启动预热分组时不应自动触发 startup bootstrap")
	require.False(t, pool.isStartupBootstrapping())
	require.Zero(t, pool.lastGlobalMaintenance.Load())
}

func TestOpenAIWarmPoolStartupBootstrapUsesConfiguredGroups(t *testing.T) {
	cfg := newOpenAIWarmPoolTestConfig()
	cfg.Gateway.OpenAIWS.AccountWarmPool.BucketRefillIntervalSeconds = 3600
	cfg.Gateway.OpenAIWS.AccountWarmPool.GlobalTargetSize = 4
	cfg.Gateway.OpenAIWS.AccountWarmPool.GlobalRefillBelow = 1
	cfg.Gateway.OpenAIWS.AccountWarmPool.GlobalRefillCooldownSeconds = 0
	cfg.Gateway.OpenAIWS.AccountWarmPool.GlobalRefillIntervalSeconds = 3600
	selectedGroupID := int64(9366)
	selectedGroup := &Group{ID: selectedGroupID, Name: "G9366", Platform: PlatformOpenAI}
	otherGroup := &Group{ID: 9367, Name: "G9367", Platform: PlatformOpenAI}
	accounts := []Account{
		{ID: 93661, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0, Groups: []*Group{selectedGroup}},
		{ID: 93662, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1, Groups: []*Group{otherGroup}},
	}
	svc := newOpenAIWarmPoolTestService(cfg, accounts, stubConcurrencyCache{})
	svc.settingService = NewSettingService(&warmPoolSettingRepoStub{values: map[string]string{
		SettingKeyOpenAIWarmPoolStartupGroupIDs: "9366",
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
	}, time.Second, 10*time.Millisecond, "usage reader 挂好后应只预热配置分组中的账号")
	require.Equal(t, 1, pool.countWarmReady(cloneAccounts([]Account{accounts[0]})))
	require.Equal(t, 0, pool.countWarmReady(cloneAccounts([]Account{accounts[1]})))
	require.NotZero(t, pool.lastGlobalMaintenance.Load())
	require.GreaterOrEqual(t, pool.countBucketWarmReady(selectedGroupID, cloneAccounts(accounts)), cfg.Gateway.OpenAIWS.AccountWarmPool.BucketSyncFillMin, "startup bootstrap 应至少补到分组池最少可用账号数")
	require.Zero(t, pool.countBucketWarmReady(otherGroup.ID, cloneAccounts(accounts)), "未配置的 startup 分组不应被预热到分组池")
}

func TestOpenAIWarmPoolStartupBootstrapKeepsGlobalReadyDuringNoAccessMaintenance(t *testing.T) {
	cfg := newOpenAIWarmPoolTestConfig()
	cfg.Gateway.OpenAIWS.AccountWarmPool.BucketRefillIntervalSeconds = 3600
	cfg.Gateway.OpenAIWS.AccountWarmPool.GlobalTargetSize = 1
	cfg.Gateway.OpenAIWS.AccountWarmPool.GlobalRefillBelow = 1
	cfg.Gateway.OpenAIWS.AccountWarmPool.GlobalRefillCooldownSeconds = 0
	cfg.Gateway.OpenAIWS.AccountWarmPool.GlobalRefillIntervalSeconds = 3600
	groupID := int64(9368)
	group := &Group{ID: groupID, Name: "G9368", Platform: PlatformOpenAI}
	accounts := []Account{{ID: 93681, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0, Groups: []*Group{group}}}
	svc := newOpenAIWarmPoolTestService(cfg, accounts, stubConcurrencyCache{})
	svc.settingService = NewSettingService(&warmPoolSettingRepoStub{values: map[string]string{
		SettingKeyOpenAIWarmPoolStartupGroupIDs: "9368",
	}}, cfg)
	openAIWarmPoolSF.Forget("openai_warm_pool")
	openAIWarmPoolCache.Store((*cachedOpenAIWarmPoolSettings)(nil))
	defer func() {
		openAIWarmPoolSF.Forget("openai_warm_pool")
		openAIWarmPoolCache.Store((*cachedOpenAIWarmPoolSettings)(nil))
	}()
	pool := svc.getOpenAIWarmPool()
	defer pool.Stop()

	reader := &openAIWarmPoolUsageReaderStub{}
	svc.SetOpenAIWarmPoolUsageReader(reader)
	require.Eventually(t, func() bool {
		return pool.countWarmReady(cloneAccounts(accounts)) == 1
	}, time.Second, 10*time.Millisecond, "startup 预热应先把账号放进全局池")
	require.GreaterOrEqual(t, pool.countBucketWarmReady(groupID, cloneAccounts(accounts)), cfg.Gateway.OpenAIWS.AccountWarmPool.BucketSyncFillMin, "startup 预热在无人访问时也应至少补到分组池最少可用账号数")

	reader.ResetCalls()
	require.NoError(t, pool.refillGlobal(context.Background(), nil, "periodic_maintenance", true))
	require.Equal(t, 1, pool.countWarmReady(cloneAccounts(accounts)), "无人访问时的后续维护不应清空 startup 全局池 ready 账号")
	require.GreaterOrEqual(t, pool.countBucketWarmReady(groupID, cloneAccounts(accounts)), cfg.Gateway.OpenAIWS.AccountWarmPool.BucketSyncFillMin)
	require.Zero(t, reader.CallCount(), "已 ready 的 startup 账号在无人访问维护周期中不应被重复探测")
}

func TestOpenAIWarmPoolStartupBootstrapRetriesUntilStartupGatePasses(t *testing.T) {
	cfg := newOpenAIWarmPoolTestConfig()
	cfg.Gateway.OpenAIWS.AccountWarmPool.BucketRefillIntervalSeconds = 0
	cfg.Gateway.OpenAIWS.AccountWarmPool.GlobalTargetSize = 1
	cfg.Gateway.OpenAIWS.AccountWarmPool.GlobalRefillBelow = 1
	cfg.Gateway.OpenAIWS.AccountWarmPool.GlobalRefillCooldownSeconds = 0
	cfg.Gateway.OpenAIWS.AccountWarmPool.GlobalRefillIntervalSeconds = 3600
	groupID := int64(9367)
	group := &Group{ID: groupID, Name: "G9367", Platform: PlatformOpenAI}
	accounts := []Account{{ID: 93671, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0, Groups: []*Group{group}}}
	svc := newOpenAIWarmPoolTestService(cfg, accounts, stubConcurrencyCache{})
	svc.settingService = NewSettingService(&warmPoolSettingRepoStub{values: map[string]string{
		SettingKeyOpenAIWarmPoolStartupGroupIDs: "9367",
	}}, cfg)
	openAIWarmPoolSF.Forget("openai_warm_pool")
	openAIWarmPoolCache.Store((*cachedOpenAIWarmPoolSettings)(nil))
	defer func() {
		openAIWarmPoolSF.Forget("openai_warm_pool")
		openAIWarmPoolCache.Store((*cachedOpenAIWarmPoolSettings)(nil))
	}()
	svc.proxyRepo = &warmPoolProxyRepoStub{proxy: &Proxy{ID: defaultOpenAIWarmPoolStartupProxyID, Name: "default", Protocol: "http", Host: "127.0.0.1", Port: 7890, Status: StatusActive}}
	proxyProber := &warmPoolProxyProberStub{err: errors.New("dial tcp timeout")}
	svc.proxyProber = proxyProber
	pool := svc.getOpenAIWarmPool()
	defer pool.Stop()

	svc.SetOpenAIWarmPoolUsageReader(&openAIWarmPoolUsageReaderStub{})
	require.Eventually(t, func() bool {
		return proxyProber.CallCount() == 1
	}, time.Second, 10*time.Millisecond, "startup bootstrap 应先尝试启动门禁探测")
	require.True(t, pool.isStartupBootstrapping(), "启动门禁未通过时应继续保留 bootstrapping 状态")
	require.Zero(t, pool.countWarmReady(cloneAccounts(accounts)))

	proxyProber.err = nil
	pool.startupGateLastCheck.Store(0)
	require.Eventually(t, func() bool {
		return pool.countWarmReady(cloneAccounts(accounts)) == 1
	}, 2*time.Second, 10*time.Millisecond, "startup bootstrap 应在门禁恢复后自动重试，而不需要再次设置 usage reader")
	require.False(t, pool.isStartupBootstrapping())
	require.True(t, pool.startupBootstrapDone.Load())
}

func TestOpenAIWarmPoolBootstrapGlobalRunsAgainAfterBucketDropsAndRefills(t *testing.T) {
	cfg := newOpenAIWarmPoolTestConfig()
	cfg.Gateway.OpenAIWS.AccountWarmPool.BucketTargetSize = 1
	cfg.Gateway.OpenAIWS.AccountWarmPool.GlobalTargetSize = 2
	cfg.Gateway.OpenAIWS.AccountWarmPool.GlobalRefillCooldownSeconds = 3600
	groupID := int64(9363)
	group := &Group{ID: groupID, Name: "G9363", Platform: PlatformOpenAI}
	accounts := []Account{
		{ID: 93631, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0, Groups: []*Group{group}},
		{ID: 93632, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1, Groups: []*Group{group}},
	}
	svc := newOpenAIWarmPoolTestService(cfg, accounts, stubConcurrencyCache{})
	reader := &openAIWarmPoolUsageReaderStub{}
	svc.SetOpenAIWarmPoolUsageReader(reader)
	pool := svc.getOpenAIWarmPool()
	waitForStartupBootstrapToSettle(t, pool)
	clearWarmPoolReadyStates(pool, accounts)

	first := pool.WarmCandidates(context.Background(), &groupID, cloneAccounts(accounts), "", nil)
	require.Len(t, first, 1)
	require.Eventually(t, func() bool {
		return pool.countWarmReady(cloneAccounts(accounts)) == 2
	}, time.Second, 10*time.Millisecond, "首次分组池补满后应补全局池")

	pool.accountState(first[0].ID).clearReady()
	require.Eventually(t, func() bool {
		return pool.countBucketWarmReady(groupID, cloneAccounts(accounts)) == 0 && pool.countWarmReady(cloneAccounts(accounts)) == 1
	}, time.Second, 10*time.Millisecond, "当分组池成员失效后应观察到分组池重新掉到未满状态")

	second := pool.WarmCandidates(context.Background(), &groupID, cloneAccounts(accounts), "", nil)
	require.Len(t, second, 1)
	require.Eventually(t, func() bool {
		return pool.countWarmReady(cloneAccounts(accounts)) == 2
	}, time.Second, 10*time.Millisecond, "分组池重新补满后应再次触发全局池补充")
}

func TestOpenAIWarmPoolBucketRefreshDeadlineDoesNotEvictReadyAccount(t *testing.T) {
	cfg := newOpenAIWarmPoolTestConfig()
	cfg.Gateway.OpenAIWS.AccountWarmPool.BucketEntryTTLSeconds = 1
	groupID := int64(9381)
	accounts := []Account{{ID: 93801, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0}}
	svc := newOpenAIWarmPoolTestService(cfg, accounts, stubConcurrencyCache{})
	svc.SetOpenAIWarmPoolUsageReader(&openAIWarmPoolUsageReaderStub{})
	pool := svc.getOpenAIWarmPool()
	past := time.Now().Add(-2 * time.Second)
	pool.accountState(93801).finishSuccess(time.Now(), pool.config().GlobalEntryTTL)
	require.True(t, pool.bucketState(groupID).promote(93801, past, pool.config().BucketEntryTTL))

	ready := pool.collectBucketWarmCandidates(groupID, cloneAccounts(accounts), "", nil)
	require.Len(t, ready, 1, "分组池刷新时间到了也不应直接把账号踢出分组池")
	require.Equal(t, 1, pool.bucketState(groupID).readyCount())
}

func TestOpenAIWarmPoolBucketRefreshFailureRemovesBucketEntry(t *testing.T) {
	cfg := newOpenAIWarmPoolTestConfig()
	groupID := int64(9382)
	accounts := []Account{{ID: 93802, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0}}
	svc := newOpenAIWarmPoolTestService(cfg, accounts, stubConcurrencyCache{})
	svc.SetOpenAIWarmPoolUsageReader(&openAIWarmPoolUsageReaderStub{errs: map[int64]error{93802: errors.New("dial tcp timeout")}})
	pool := svc.getOpenAIWarmPool()
	bucket := pool.bucketState(groupID)
	pool.accountState(93802).finishSuccess(time.Now(), pool.config().GlobalEntryTTL)
	require.True(t, bucket.promote(93802, time.Now(), pool.config().BucketEntryTTL))

	pool.refreshBucketMember(groupID, bucket, &accounts[0])
	require.Equal(t, 0, bucket.readyCount(), "分组池账号只有在复检失败后才应被移出分组池")
	inspection := pool.accountState(93802).inspect(time.Now())
	require.True(t, inspection.NetworkError)
}

func TestOpenAIWarmPoolBucketRefreshRecentUseSkipsProbe(t *testing.T) {
	cfg := newOpenAIWarmPoolTestConfig()
	cfg.Gateway.OpenAIWS.AccountWarmPool.BucketEntryTTLSeconds = 30
	groupID := int64(9383)
	verifiedAt := time.Now().Add(-20 * time.Second)
	lastUsedAt := time.Now().Add(-5 * time.Second)
	accounts := []Account{{ID: 93803, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0, LastUsedAt: &lastUsedAt}}
	reader := &openAIWarmPoolUsageReaderStub{}
	svc := newOpenAIWarmPoolTestService(cfg, accounts, stubConcurrencyCache{})
	svc.SetOpenAIWarmPoolUsageReader(reader)
	pool := svc.getOpenAIWarmPool()
	waitForStartupBootstrapToSettle(t, pool)
	clearWarmPoolReadyStates(pool, accounts)
	reader.ResetCalls()
	bucket := pool.bucketState(groupID)
	pool.accountState(93803).finishSuccess(verifiedAt, pool.config().GlobalEntryTTL)
	require.True(t, bucket.promote(93803, verifiedAt, pool.config().BucketEntryTTL))

	pool.refreshBucketMember(groupID, bucket, &accounts[0])

	require.Zero(t, reader.CallCount(), "近期已被实际使用的分组池成员不应再次探测")
	after := pool.accountState(93803).inspect(time.Now())
	require.NotNil(t, after.VerifiedAt)
	require.False(t, after.NetworkError)
}

func TestOpenAIWarmPoolRetriesNetworkErrorOnceBeforeSuccess(t *testing.T) {
	cfg := newOpenAIWarmPoolTestConfig()
	accounts := []Account{{ID: 96000, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0}}
	reader := &openAIWarmPoolUsageReaderStub{errSeq: map[int64][]error{
		96000: {errors.New("dial tcp timeout")},
	}}
	svc := newOpenAIWarmPoolTestService(cfg, accounts, stubConcurrencyCache{})
	pool := svc.getOpenAIWarmPool()
	pool.usageReaderMu.Lock()
	pool.usageReader = reader
	pool.usageReaderMu.Unlock()

	result := pool.probeCandidate(context.Background(), 0, &accounts[0])
	require.Equal(t, openAIWarmPoolProbeReadyOutcome, result.Outcome)
	require.Equal(t, 2, reader.CallCount(), "网络异常后应立即重试一次")
	inspection := pool.accountState(96000).inspect(time.Now())
	require.True(t, inspection.Ready)
	require.False(t, inspection.NetworkError)
}

func TestOpenAIWarmPoolMovesNetworkErrorAccountIntoNetworkErrorPool(t *testing.T) {
	cfg := newOpenAIWarmPoolTestConfig()
	accounts := []Account{{ID: 96001, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0}}
	reader := &openAIWarmPoolUsageReaderStub{errSeq: map[int64][]error{
		96001: {errors.New("dial tcp timeout"), errors.New("dial tcp timeout")},
	}}
	svc := newOpenAIWarmPoolTestService(cfg, accounts, stubConcurrencyCache{})
	pool := svc.getOpenAIWarmPool()
	pool.usageReaderMu.Lock()
	pool.usageReader = reader
	pool.usageReaderMu.Unlock()

	result := pool.probeCandidate(context.Background(), 0, &accounts[0])
	require.Equal(t, openAIWarmPoolProbeNetworkErrorOutcome, result.Outcome)
	require.Equal(t, 2, reader.CallCount(), "进入网络异常池前应先重试一次")
	inspection := pool.accountState(96001).inspect(time.Now())
	require.True(t, inspection.NetworkError)
	require.NotNil(t, inspection.NetworkErrorUntil)
}

func TestOpenAIWarmPoolSelectProbeCandidatesPrioritizesNetworkErrorBeforeCold(t *testing.T) {
	cfg := newOpenAIWarmPoolTestConfig()
	accounts := []Account{
		{ID: 97001, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 10},
		{ID: 97002, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1},
	}
	svc := newOpenAIWarmPoolTestService(cfg, accounts, stubConcurrencyCache{})
	pool := svc.getOpenAIWarmPool()
	pool.accountState(97001).markNetworkError(time.Now(), pool.config().NetworkErrorEntryTTL)

	candidates := pool.selectProbeCandidates(cloneAccounts(accounts), "gpt-5.1", nil)
	require.Len(t, candidates, 2)
	require.Equal(t, int64(97001), candidates[0].ID, "NetworkError 账号应优先于冷账号被回收进 Global")
}

func TestOpenAIWarmPoolWaitsForStartupProxyGateBeforePrewarming(t *testing.T) {
	cfg := newOpenAIWarmPoolTestConfig()
	groupID := int64(9921)
	group := &Group{ID: groupID, Name: "G9921", Platform: PlatformOpenAI}
	accounts := []Account{{ID: 99201, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0, Groups: []*Group{group}}}
	svc := newOpenAIWarmPoolTestService(cfg, accounts, stubConcurrencyCache{})
	svc.proxyRepo = &warmPoolProxyRepoStub{proxy: &Proxy{ID: defaultOpenAIWarmPoolStartupProxyID, Name: "default", Protocol: "http", Host: "127.0.0.1", Port: 7890, Status: StatusActive}}
	proxyProber := &warmPoolProxyProberStub{err: errors.New("dial tcp timeout")}
	svc.proxyProber = proxyProber
	reader := &openAIWarmPoolUsageReaderStub{}
	svc.SetOpenAIWarmPoolUsageReader(reader)
	pool := svc.getOpenAIWarmPool()

	warmed := pool.WarmCandidates(context.Background(), &groupID, cloneAccounts(accounts), "gpt-5.1", nil)
	require.Len(t, warmed, 0)
	require.Zero(t, reader.CallCount())
	require.False(t, pool.startupGateReady.Load())
	require.Equal(t, 1, proxyProber.CallCount())

	proxyProber.err = nil
	pool.startupGateLastCheck.Store(0)
	warmed = pool.WarmCandidates(context.Background(), &groupID, cloneAccounts(accounts), "gpt-5.1", nil)
	require.Len(t, warmed, 1)
	require.Equal(t, 1, reader.CallCount())
	require.True(t, pool.startupGateReady.Load())
}
