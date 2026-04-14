package service

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type schedulerSnapshotTimeoutCacheStub struct {
	mu           sync.Mutex
	readTimeout  time.Duration
	writeTimeout time.Duration
	blockRead    bool
	blockWrite   bool
}

func (s *schedulerSnapshotTimeoutCacheStub) captureDeadline(ctx context.Context) time.Duration {
	if ctx == nil {
		return 0
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		return 0
	}
	remaining := time.Until(deadline)
	if remaining < 0 {
		return 0
	}
	return remaining
}

func (s *schedulerSnapshotTimeoutCacheStub) GetSnapshot(ctx context.Context, _ SchedulerBucket) ([]*Account, bool, error) {
	s.mu.Lock()
	s.readTimeout = s.captureDeadline(ctx)
	blockRead := s.blockRead
	s.mu.Unlock()
	if blockRead {
		<-ctx.Done()
		return nil, false, ctx.Err()
	}
	return nil, false, nil
}

func (s *schedulerSnapshotTimeoutCacheStub) SetSnapshot(ctx context.Context, _ SchedulerBucket, _ []Account) error {
	s.mu.Lock()
	s.writeTimeout = s.captureDeadline(ctx)
	blockWrite := s.blockWrite
	s.mu.Unlock()
	if blockWrite {
		<-ctx.Done()
		return ctx.Err()
	}
	return nil
}

func (s *schedulerSnapshotTimeoutCacheStub) GetAccount(ctx context.Context, _ int64) (*Account, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (s *schedulerSnapshotTimeoutCacheStub) SetAccount(context.Context, *Account) error { return nil }
func (s *schedulerSnapshotTimeoutCacheStub) DeleteAccount(context.Context, int64) error { return nil }
func (s *schedulerSnapshotTimeoutCacheStub) UpdateLastUsed(context.Context, map[int64]time.Time) error {
	return nil
}
func (s *schedulerSnapshotTimeoutCacheStub) TryLockBucket(context.Context, SchedulerBucket, time.Duration) (bool, error) {
	return true, nil
}
func (s *schedulerSnapshotTimeoutCacheStub) ListBuckets(context.Context) ([]SchedulerBucket, error) {
	return nil, nil
}
func (s *schedulerSnapshotTimeoutCacheStub) GetOutboxWatermark(context.Context) (int64, error) {
	return 0, nil
}
func (s *schedulerSnapshotTimeoutCacheStub) SetOutboxWatermark(context.Context, int64) error {
	return nil
}

func (s *schedulerSnapshotTimeoutCacheStub) observedReadTimeout() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readTimeout
}

func (s *schedulerSnapshotTimeoutCacheStub) observedWriteTimeout() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeTimeout
}

type schedulerSnapshotOutboxCacheStub struct {
	mu                sync.Mutex
	watermark         int64
	setWatermarkCalls int
	buckets           []SchedulerBucket
}

func (s *schedulerSnapshotOutboxCacheStub) GetSnapshot(context.Context, SchedulerBucket) ([]*Account, bool, error) {
	return nil, false, nil
}

func (s *schedulerSnapshotOutboxCacheStub) SetSnapshot(context.Context, SchedulerBucket, []Account) error {
	return nil
}

func (s *schedulerSnapshotOutboxCacheStub) GetAccount(context.Context, int64) (*Account, error) {
	return nil, nil
}

func (s *schedulerSnapshotOutboxCacheStub) SetAccount(context.Context, *Account) error { return nil }
func (s *schedulerSnapshotOutboxCacheStub) DeleteAccount(context.Context, int64) error { return nil }
func (s *schedulerSnapshotOutboxCacheStub) UpdateLastUsed(context.Context, map[int64]time.Time) error {
	return nil
}
func (s *schedulerSnapshotOutboxCacheStub) TryLockBucket(context.Context, SchedulerBucket, time.Duration) (bool, error) {
	return true, nil
}
func (s *schedulerSnapshotOutboxCacheStub) ListBuckets(ctx context.Context) ([]SchedulerBucket, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]SchedulerBucket(nil), s.buckets...), nil
}
func (s *schedulerSnapshotOutboxCacheStub) GetOutboxWatermark(ctx context.Context) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.watermark, nil
}
func (s *schedulerSnapshotOutboxCacheStub) SetOutboxWatermark(ctx context.Context, id int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.watermark = id
	s.setWatermarkCalls++
	return nil
}

func (s *schedulerSnapshotOutboxCacheStub) observedWatermark() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.watermark
}

func (s *schedulerSnapshotOutboxCacheStub) observedSetWatermarkCalls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.setWatermarkCalls
}

type schedulerSnapshotOutboxRepoStub struct {
	events []SchedulerOutboxEvent
	maxID  int64
}

func (s *schedulerSnapshotOutboxRepoStub) ListAfter(ctx context.Context, afterID int64, limit int) ([]SchedulerOutboxEvent, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if limit <= 0 {
		return nil, nil
	}
	out := make([]SchedulerOutboxEvent, 0, len(s.events))
	for _, event := range s.events {
		if event.ID <= afterID {
			continue
		}
		out = append(out, event)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (s *schedulerSnapshotOutboxRepoStub) MaxID(ctx context.Context) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	return s.maxID, nil
}

type delayedSchedulerSnapshotAccountRepo struct {
	stubOpenAIAccountRepo
	delay time.Duration
}

func (r *delayedSchedulerSnapshotAccountRepo) wait(ctx context.Context) error {
	if r.delay <= 0 {
		return nil
	}
	select {
	case <-time.After(r.delay):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *delayedSchedulerSnapshotAccountRepo) ListSchedulableByPlatform(ctx context.Context, platform string) ([]Account, error) {
	if err := r.wait(ctx); err != nil {
		return nil, err
	}
	return r.stubOpenAIAccountRepo.ListSchedulableByPlatform(ctx, platform)
}

func (r *delayedSchedulerSnapshotAccountRepo) ListSchedulableUngroupedByPlatform(ctx context.Context, platform string) ([]Account, error) {
	if err := r.wait(ctx); err != nil {
		return nil, err
	}
	return r.stubOpenAIAccountRepo.ListSchedulableUngroupedByPlatform(ctx, platform)
}

type warmPoolFreshLookupErrorRepo struct {
	stubOpenAIAccountRepo
	err error
}

func (r *warmPoolFreshLookupErrorRepo) GetByID(ctx context.Context, id int64) (*Account, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.stubOpenAIAccountRepo.GetByID(ctx, id)
}

type schedulerSnapshotAccountSyncCacheStub struct {
	setAccounts []*Account
}

func (s *schedulerSnapshotAccountSyncCacheStub) GetSnapshot(context.Context, SchedulerBucket) ([]*Account, bool, error) {
	return nil, false, nil
}
func (s *schedulerSnapshotAccountSyncCacheStub) SetSnapshot(context.Context, SchedulerBucket, []Account) error {
	return nil
}
func (s *schedulerSnapshotAccountSyncCacheStub) GetAccount(context.Context, int64) (*Account, error) {
	return nil, nil
}
func (s *schedulerSnapshotAccountSyncCacheStub) SetAccount(_ context.Context, account *Account) error {
	if account != nil {
		copied := *account
		s.setAccounts = append(s.setAccounts, &copied)
	}
	return nil
}
func (s *schedulerSnapshotAccountSyncCacheStub) DeleteAccount(context.Context, int64) error {
	return nil
}
func (s *schedulerSnapshotAccountSyncCacheStub) UpdateLastUsed(context.Context, map[int64]time.Time) error {
	return nil
}
func (s *schedulerSnapshotAccountSyncCacheStub) TryLockBucket(context.Context, SchedulerBucket, time.Duration) (bool, error) {
	return true, nil
}
func (s *schedulerSnapshotAccountSyncCacheStub) ListBuckets(context.Context) ([]SchedulerBucket, error) {
	return nil, nil
}
func (s *schedulerSnapshotAccountSyncCacheStub) GetOutboxWatermark(context.Context) (int64, error) {
	return 0, nil
}
func (s *schedulerSnapshotAccountSyncCacheStub) SetOutboxWatermark(context.Context, int64) error {
	return nil
}

type noRebuildSchedulerSnapshotRepo struct {
	stubOpenAIAccountRepo
	listCalls atomic.Int32
}

func (r *noRebuildSchedulerSnapshotRepo) ListSchedulableByGroupIDAndPlatform(ctx context.Context, groupID int64, platform string) ([]Account, error) {
	r.listCalls.Add(1)
	return r.stubOpenAIAccountRepo.ListSchedulableByGroupIDAndPlatform(ctx, groupID, platform)
}

func (r *noRebuildSchedulerSnapshotRepo) ListSchedulableByGroupIDAndPlatforms(ctx context.Context, groupID int64, platforms []string) ([]Account, error) {
	r.listCalls.Add(1)
	return r.stubOpenAIAccountRepo.ListSchedulableByGroupIDAndPlatforms(ctx, groupID, platforms)
}

func (r *noRebuildSchedulerSnapshotRepo) ListSchedulableByPlatform(ctx context.Context, platform string) ([]Account, error) {
	r.listCalls.Add(1)
	return r.stubOpenAIAccountRepo.ListSchedulableByPlatform(ctx, platform)
}

func (r *noRebuildSchedulerSnapshotRepo) ListSchedulableByPlatforms(ctx context.Context, platforms []string) ([]Account, error) {
	r.listCalls.Add(1)
	return r.stubOpenAIAccountRepo.ListSchedulableByPlatforms(ctx, platforms)
}

func (r *noRebuildSchedulerSnapshotRepo) ListSchedulableUngroupedByPlatform(ctx context.Context, platform string) ([]Account, error) {
	r.listCalls.Add(1)
	return r.stubOpenAIAccountRepo.ListSchedulableUngroupedByPlatform(ctx, platform)
}

func (r *noRebuildSchedulerSnapshotRepo) ListSchedulableUngroupedByPlatforms(ctx context.Context, platforms []string) ([]Account, error) {
	r.listCalls.Add(1)
	return r.stubOpenAIAccountRepo.ListSchedulableUngroupedByPlatforms(ctx, platforms)
}

type failingSchedulerObserver struct {
	calls atomic.Int32
	err   error
}

func (o *failingSchedulerObserver) HandleSchedulerOutboxEvent(ctx context.Context, event SchedulerOutboxEvent) error {
	o.calls.Add(1)
	return o.err
}

func TestSchedulerSnapshotService_HandleAccountEvent_SyncsAccountAndRebuildsBuckets(t *testing.T) {
	cfg := newOpenAIWarmPoolTestConfig()
	groupID := int64(9901)
	accountID := int64(99001)
	repo := &noRebuildSchedulerSnapshotRepo{stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: []Account{{
		ID:          accountID,
		Name:        "acc-99001",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		GroupIDs:    []int64{groupID},
		Groups:      []*Group{{ID: groupID, Name: "G9901", Platform: PlatformOpenAI}},
	}}}}
	cache := &schedulerSnapshotAccountSyncCacheStub{}
	svc := NewSchedulerSnapshotService(cache, nil, repo, nil, cfg)

	err := svc.handleAccountEvent(context.Background(), &accountID, nil, nil)
	require.NoError(t, err)
	require.Len(t, cache.setAccounts, 1)
	require.Equal(t, accountID, cache.setAccounts[0].ID)
	require.Greater(t, repo.listCalls.Load(), int32(0), "普通 account_changed 事件应触发相关 bucket 重建")
}

func TestSchedulerSnapshotService_GetAccount_FallbackSyncsSingleAccountCache(t *testing.T) {
	cfg := newOpenAIWarmPoolTestConfig()
	cfg.Gateway.Scheduling.DbFallbackEnabled = true
	cfg.Gateway.Scheduling.DbFallbackTimeoutSeconds = 5
	accountID := int64(99011)
	cache := &schedulerSnapshotAccountSyncCacheStub{}
	repo := &warmPoolFreshLookupErrorRepo{stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: []Account{{
		ID:          accountID,
		Name:        "acc-99011",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-test-secret"},
		Extra:       map[string]any{"privacy_mode": PrivacyModeTrainingOff},
	}}}}
	svc := NewSchedulerSnapshotService(cache, nil, repo, nil, cfg)

	account, err := svc.GetAccount(context.Background(), accountID)
	require.NoError(t, err)
	require.NotNil(t, account)
	require.Equal(t, accountID, account.ID)
	require.Equal(t, "sk-test-secret", account.GetCredential("api_key"))
	require.Len(t, cache.setAccounts, 1)
	require.Equal(t, accountID, cache.setAccounts[0].ID)
	require.Equal(t, "sk-test-secret", cache.setAccounts[0].GetCredential("api_key"))
}

func TestSchedulerSnapshotService_ListSchedulableAccounts_FastFailsCacheTimeouts(t *testing.T) {
	oldCacheReadTimeout := schedulerCacheReadTimeout
	oldCacheWriteTimeout := schedulerCacheWriteTimeout
	oldSnapshotReadTimeout := schedulerSnapshotReadTimeout
	schedulerCacheReadTimeout = 10 * time.Millisecond
	schedulerCacheWriteTimeout = 20 * time.Millisecond
	schedulerSnapshotReadTimeout = 40 * time.Millisecond
	defer func() {
		schedulerCacheReadTimeout = oldCacheReadTimeout
		schedulerCacheWriteTimeout = oldCacheWriteTimeout
		schedulerSnapshotReadTimeout = oldSnapshotReadTimeout
	}()

	cfg := newOpenAIWarmPoolTestConfig()
	cfg.Gateway.Scheduling.DbFallbackEnabled = true
	cfg.Gateway.Scheduling.DbFallbackTimeoutSeconds = 5
	groupID := int64(9801)
	group := &Group{ID: groupID, Name: "G9801", Platform: PlatformOpenAI}
	accounts := []Account{{ID: 98001, Name: "acc-98001", Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Groups: []*Group{group}, GroupIDs: []int64{groupID}}}
	cache := &schedulerSnapshotTimeoutCacheStub{blockRead: true, blockWrite: true}
	svc := NewSchedulerSnapshotService(cache, nil, &warmPoolFreshLookupErrorRepo{stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: accounts}}, nil, cfg)

	start := time.Now()
	result, useMixed, err := svc.ListSchedulableAccounts(context.Background(), &groupID, PlatformOpenAI, false)
	elapsed := time.Since(start)

	require.NoError(t, err)
	require.False(t, useMixed)
	require.Len(t, result, 1)
	require.Equal(t, int64(98001), result[0].ID)
	require.Less(t, elapsed, 500*time.Millisecond, "Redis 卡住时应快速降级到 DB，而不是等待多秒 socket timeout")
	require.NotZero(t, cache.observedReadTimeout())
	require.NotZero(t, cache.observedWriteTimeout())
	require.Greater(t, cache.observedReadTimeout(), schedulerCacheReadTimeout, "快照读取应使用专用读超时，而不是单键缓存读超时")
	require.LessOrEqual(t, cache.observedReadTimeout(), schedulerSnapshotReadTimeout+20*time.Millisecond)
	require.LessOrEqual(t, cache.observedWriteTimeout(), schedulerCacheWriteTimeout+20*time.Millisecond)
}

func TestSchedulerSnapshotService_ListSchedulableAccounts_SkipsFallbackCacheWriteForLargeBuckets(t *testing.T) {
	oldMaxItems := schedulerSnapshotSyncWriteMaxItems
	schedulerSnapshotSyncWriteMaxItems = 1
	defer func() { schedulerSnapshotSyncWriteMaxItems = oldMaxItems }()

	cfg := newOpenAIWarmPoolTestConfig()
	cfg.Gateway.Scheduling.DbFallbackEnabled = true
	cfg.Gateway.Scheduling.DbFallbackTimeoutSeconds = 5
	groupID := int64(98015)
	group := &Group{ID: groupID, Name: "G98015", Platform: PlatformOpenAI}
	accounts := []Account{
		{ID: 98011, Name: "acc-98011", Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Groups: []*Group{group}, GroupIDs: []int64{groupID}},
		{ID: 98012, Name: "acc-98012", Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Groups: []*Group{group}, GroupIDs: []int64{groupID}},
	}
	cache := &schedulerSnapshotTimeoutCacheStub{}
	svc := NewSchedulerSnapshotService(cache, nil, &warmPoolFreshLookupErrorRepo{stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: accounts}}, nil, cfg)

	result, useMixed, err := svc.ListSchedulableAccounts(context.Background(), &groupID, PlatformOpenAI, false)
	require.NoError(t, err)
	require.False(t, useMixed)
	require.Len(t, result, len(accounts))
	require.Zero(t, cache.observedWriteTimeout(), "超大 bucket 的请求链路回源不应同步回写快照缓存")
}

func TestSchedulerSnapshotService_RebuildBucket_UsesSnapshotWriteTimeout(t *testing.T) {
	oldCacheWriteTimeout := schedulerCacheWriteTimeout
	oldSnapshotWriteTimeout := schedulerSnapshotWriteTimeout
	oldRebuildLoadTimeout := schedulerSnapshotRebuildLoadTimeout
	schedulerCacheWriteTimeout = 10 * time.Millisecond
	schedulerSnapshotWriteTimeout = 40 * time.Millisecond
	schedulerSnapshotRebuildLoadTimeout = 100 * time.Millisecond
	defer func() {
		schedulerCacheWriteTimeout = oldCacheWriteTimeout
		schedulerSnapshotWriteTimeout = oldSnapshotWriteTimeout
		schedulerSnapshotRebuildLoadTimeout = oldRebuildLoadTimeout
	}()

	cfg := newOpenAIWarmPoolTestConfig()
	groupID := int64(9802)
	group := &Group{ID: groupID, Name: "G9802", Platform: PlatformOpenAI}
	accounts := []Account{{ID: 98002, Name: "acc-98002", Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1, Groups: []*Group{group}, GroupIDs: []int64{groupID}}}
	cache := &schedulerSnapshotTimeoutCacheStub{blockWrite: true}
	svc := NewSchedulerSnapshotService(cache, nil, &warmPoolFreshLookupErrorRepo{stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: accounts}}, nil, cfg)

	start := time.Now()
	err := svc.rebuildBucket(context.Background(), SchedulerBucket{GroupID: groupID, Platform: PlatformOpenAI, Mode: SchedulerModeSingle}, "test")
	elapsed := time.Since(start)

	require.Error(t, err)
	require.Less(t, elapsed, 500*time.Millisecond, "重建写缓存超时时应尽快返回，而不是被长时间卡住")
	require.NotZero(t, cache.observedWriteTimeout())
	require.Greater(t, cache.observedWriteTimeout(), schedulerCacheWriteTimeout, "重建写缓存应使用专用快照写超时")
	require.LessOrEqual(t, cache.observedWriteTimeout(), schedulerSnapshotWriteTimeout+20*time.Millisecond)
}

func TestSchedulerSnapshotService_PollOutbox_WritesWatermarkWithFreshContext(t *testing.T) {
	oldTimeout := schedulerOutboxPollTimeout
	schedulerOutboxPollTimeout = 20 * time.Millisecond
	defer func() { schedulerOutboxPollTimeout = oldTimeout }()

	cfg := newOpenAIWarmPoolTestConfig()
	accounts := []Account{{ID: 98201, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1}}
	cache := &schedulerSnapshotOutboxCacheStub{
		buckets: []SchedulerBucket{{GroupID: 0, Platform: PlatformOpenAI, Mode: SchedulerModeSingle}},
	}
	outboxRepo := &schedulerSnapshotOutboxRepoStub{
		events: []SchedulerOutboxEvent{{ID: 7, EventType: SchedulerOutboxEventFullRebuild, CreatedAt: time.Now()}},
		maxID:  7,
	}
	accountRepo := &delayedSchedulerSnapshotAccountRepo{
		stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: accounts},
		delay:                 30 * time.Millisecond,
	}
	svc := NewSchedulerSnapshotService(cache, outboxRepo, accountRepo, nil, cfg)

	svc.pollOutbox()

	require.Equal(t, int64(7), cache.observedWatermark(), "即使事件处理耗时超过本轮 poll 超时，watermark 也应使用新的上下文成功写入")
	require.Equal(t, 1, cache.observedSetWatermarkCalls())
}

func TestSchedulerSnapshotService_PollOutbox_ObserverFailureDoesNotBlockWatermark(t *testing.T) {
	cfg := newOpenAIWarmPoolTestConfig()
	accounts := []Account{{ID: 98211, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 1}}
	cache := &schedulerSnapshotOutboxCacheStub{
		buckets: []SchedulerBucket{{GroupID: 0, Platform: PlatformOpenAI, Mode: SchedulerModeSingle}},
	}
	outboxRepo := &schedulerSnapshotOutboxRepoStub{
		events: []SchedulerOutboxEvent{{ID: 8, EventType: SchedulerOutboxEventFullRebuild, CreatedAt: time.Now()}},
		maxID:  8,
	}
	svc := NewSchedulerSnapshotService(cache, outboxRepo, &delayedSchedulerSnapshotAccountRepo{stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: accounts}}, nil, cfg)
	observer := &failingSchedulerObserver{err: errors.New("observer down")}
	svc.RegisterObserver(observer)

	require.NotPanics(t, func() { svc.pollOutbox() })
	require.Equal(t, int32(1), observer.calls.Load())
	require.Equal(t, int64(8), cache.observedWatermark(), "observer 失败时也不应阻断主 scheduler watermark 推进")
	require.Equal(t, 1, cache.observedSetWatermarkCalls())
}

func TestOpenAIWarmPoolProbeCandidate_FreshLookupErrorDoesNotPanic(t *testing.T) {
	cfg := newOpenAIWarmPoolTestConfig()
	accounts := []Account{{ID: 98101, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0}}
	svc := newOpenAIWarmPoolTestService(cfg, accounts, stubConcurrencyCache{})
	svc.accountRepo = &warmPoolFreshLookupErrorRepo{
		stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: accounts},
		err:                   errors.New("fresh lookup failed"),
	}
	svc.SetOpenAIWarmPoolUsageReader(&openAIWarmPoolUsageReaderStub{})
	pool := svc.getOpenAIWarmPool()

	require.NotPanics(t, func() {
		result := pool.probeCandidate(context.Background(), 0, &accounts[0])
		require.Equal(t, openAIWarmPoolProbeFailedOutcome, result.Outcome)
		require.Equal(t, "fresh_account_lookup_failed", result.Reason)
		require.Error(t, result.Err)
	})
}
