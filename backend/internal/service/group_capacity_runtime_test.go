//go:build unit

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

type groupCapacityRuntimeSessionCacheStub struct {
	counts         map[int64]int
	err            error
	mu             sync.Mutex
	calls          [][]int64
	timeoutsByCall []map[int64]time.Duration
	started        atomic.Int64
	release        chan struct{}
}

func (s *groupCapacityRuntimeSessionCacheStub) RegisterSession(ctx context.Context, accountID int64, sessionUUID string, maxSessions int, idleTimeout time.Duration) (bool, error) {
	panic("unexpected RegisterSession call")
}

func (s *groupCapacityRuntimeSessionCacheStub) RefreshSession(ctx context.Context, accountID int64, sessionUUID string, idleTimeout time.Duration) error {
	panic("unexpected RefreshSession call")
}

func (s *groupCapacityRuntimeSessionCacheStub) GetActiveSessionCount(ctx context.Context, accountID int64) (int, error) {
	panic("unexpected GetActiveSessionCount call")
}

func (s *groupCapacityRuntimeSessionCacheStub) GetActiveSessionCountBatch(ctx context.Context, accountIDs []int64, idleTimeouts map[int64]time.Duration) (map[int64]int, error) {
	s.started.Add(1)
	if s.release != nil {
		<-s.release
	}
	s.mu.Lock()
	s.calls = append(s.calls, append([]int64(nil), accountIDs...))
	copyTimeouts := make(map[int64]time.Duration, len(idleTimeouts))
	for k, v := range idleTimeouts {
		copyTimeouts[k] = v
	}
	s.timeoutsByCall = append(s.timeoutsByCall, copyTimeouts)
	s.mu.Unlock()
	if s.err != nil {
		return nil, s.err
	}
	result := make(map[int64]int, len(accountIDs))
	for _, id := range accountIDs {
		result[id] = s.counts[id]
	}
	return result, nil
}

func (s *groupCapacityRuntimeSessionCacheStub) IsSessionActive(ctx context.Context, accountID int64, sessionUUID string) (bool, error) {
	panic("unexpected IsSessionActive call")
}

func (s *groupCapacityRuntimeSessionCacheStub) GetWindowCost(ctx context.Context, accountID int64) (float64, bool, error) {
	panic("unexpected GetWindowCost call")
}

func (s *groupCapacityRuntimeSessionCacheStub) SetWindowCost(ctx context.Context, accountID int64, cost float64) error {
	panic("unexpected SetWindowCost call")
}

func (s *groupCapacityRuntimeSessionCacheStub) GetWindowCostBatch(ctx context.Context, accountIDs []int64) (map[int64]float64, error) {
	panic("unexpected GetWindowCostBatch call")
}

type groupCapacityRuntimeRPMCacheStub struct {
	counts  map[int64]int
	err     error
	mu      sync.Mutex
	calls   [][]int64
	started atomic.Int64
	release chan struct{}
}

type groupCapacityRuntimeConcurrencyCacheStub struct {
	stubConcurrencyCacheForTest
	mu    sync.Mutex
	calls [][]AccountWithConcurrency
}

func (s *groupCapacityRuntimeConcurrencyCacheStub) GetAccountsLoadBatchFast(ctx context.Context, accounts []AccountWithConcurrency) (map[int64]*AccountLoadInfo, error) {
	s.mu.Lock()
	s.calls = append(s.calls, append([]AccountWithConcurrency(nil), accounts...))
	s.mu.Unlock()
	return s.stubConcurrencyCacheForTest.GetAccountsLoadBatchFast(ctx, accounts)
}

func (s *groupCapacityRuntimeRPMCacheStub) IncrementRPM(ctx context.Context, accountID int64) (int, error) {
	panic("unexpected IncrementRPM call")
}

func (s *groupCapacityRuntimeRPMCacheStub) GetRPM(ctx context.Context, accountID int64) (int, error) {
	panic("unexpected GetRPM call")
}

func (s *groupCapacityRuntimeRPMCacheStub) GetRPMBatch(ctx context.Context, accountIDs []int64) (map[int64]int, error) {
	s.started.Add(1)
	if s.release != nil {
		<-s.release
	}
	s.mu.Lock()
	s.calls = append(s.calls, append([]int64(nil), accountIDs...))
	s.mu.Unlock()
	if s.err != nil {
		return nil, s.err
	}
	result := make(map[int64]int, len(accountIDs))
	for _, id := range accountIDs {
		result[id] = s.counts[id]
	}
	return result, nil
}

type groupCapacityRuntimeAccountRepoStub struct {
	groupCapacityAccountRepoStub
	accountsByID map[int64]*Account
	getByIDCalls atomic.Int64
}

func (s *groupCapacityRuntimeAccountRepoStub) GetByID(ctx context.Context, id int64) (*Account, error) {
	s.getByIDCalls.Add(1)
	if s.accountsByID == nil {
		return nil, ErrAccountNotFound
	}
	account, ok := s.accountsByID[id]
	if !ok || account == nil {
		return nil, ErrAccountNotFound
	}
	copyAccount := *account
	if account.GroupIDs != nil {
		copyAccount.GroupIDs = append([]int64(nil), account.GroupIDs...)
	}
	return &copyAccount, nil
}

func TestGroupCapacityRuntimeProvider_AggregatesRuntimeUsage(t *testing.T) {
	sessionCache := &groupCapacityRuntimeSessionCacheStub{
		counts: map[int64]int{11: 2, 13: 1},
	}
	rpmCache := &groupCapacityRuntimeRPMCacheStub{
		counts: map[int64]int{11: 40, 12: 25},
	}
	concurrencyCache := &groupCapacityRuntimeConcurrencyCacheStub{
		stubConcurrencyCacheForTest: stubConcurrencyCacheForTest{
			loadBatch: map[int64]*AccountLoadInfo{
				11: {AccountID: 11, CurrentConcurrency: 2},
				12: {AccountID: 12, CurrentConcurrency: 3},
				13: {AccountID: 13, CurrentConcurrency: 0},
			},
		},
	}
	concurrencyService := NewConcurrencyService(concurrencyCache)

	provider := NewGroupCapacityRuntimeProviderService(concurrencyService, sessionCache, rpmCache, 2*time.Second, 2)
	snapshot := GroupCapacityStaticSnapshot{
		GroupID:                  7,
		AllAccountIDs:            []int64{11, 12, 13},
		SessionLimitedAccountIDs: []int64{11, 13},
		RPMLimitedAccountIDs:     []int64{11, 12},
		SessionTimeouts:          map[int64]time.Duration{11: 5 * time.Minute, 13: 9 * time.Minute},
		ConcurrencyMax:           99,
		SessionsMax:              99,
		RPMMax:                   99,
	}

	usage, err := provider.GetGroupCapacityRuntimeUsage(context.Background(), snapshot)
	require.NoError(t, err)
	require.Equal(t, GroupCapacityRuntimeUsage{
		GroupID:         7,
		ConcurrencyUsed: 5,
		ActiveSessions:  3,
		CurrentRPM:      65,
	}, usage)
	require.Equal(t, [][]AccountWithConcurrency{{{ID: 11}, {ID: 12}}, {{ID: 13}}}, concurrencyCache.calls)
	require.Equal(t, [][]int64{{11, 13}}, sessionCache.calls)
	require.Equal(t, [][]int64{{11, 12}}, rpmCache.calls)
	require.Equal(t, map[int64]time.Duration{11: 5 * time.Minute, 13: 9 * time.Minute}, sessionCache.timeoutsByCall[0])
	require.EqualValues(t, 2, concurrencyCache.accountFastLoadCalls.Load())
}

func TestGroupCapacityRuntimeProvider_ChunksBatchCalls(t *testing.T) {
	sessionCache := &groupCapacityRuntimeSessionCacheStub{
		counts: map[int64]int{1: 1, 2: 2, 3: 3},
	}
	rpmCache := &groupCapacityRuntimeRPMCacheStub{
		counts: map[int64]int{4: 4, 5: 5, 6: 6},
	}
	concurrencyCache := &groupCapacityRuntimeConcurrencyCacheStub{
		stubConcurrencyCacheForTest: stubConcurrencyCacheForTest{
			loadBatch: map[int64]*AccountLoadInfo{
				10: {AccountID: 10, CurrentConcurrency: 1},
				11: {AccountID: 11, CurrentConcurrency: 2},
				12: {AccountID: 12, CurrentConcurrency: 3},
			},
		},
	}
	provider := NewGroupCapacityRuntimeProviderService(NewConcurrencyService(concurrencyCache), sessionCache, rpmCache, time.Second, 2)

	snapshot := GroupCapacityStaticSnapshot{
		GroupID:                  8,
		AllAccountIDs:            []int64{10, 11, 12},
		SessionLimitedAccountIDs: []int64{1, 2, 3},
		RPMLimitedAccountIDs:     []int64{4, 5, 6},
		SessionTimeouts:          map[int64]time.Duration{1: time.Minute, 2: 2 * time.Minute, 3: 3 * time.Minute},
	}

	usage, err := provider.GetGroupCapacityRuntimeUsage(context.Background(), snapshot)
	require.NoError(t, err)
	require.Equal(t, 6, usage.ConcurrencyUsed)
	require.Equal(t, 6, usage.ActiveSessions)
	require.Equal(t, 15, usage.CurrentRPM)
	require.Equal(t, [][]AccountWithConcurrency{{{ID: 10}, {ID: 11}}, {{ID: 12}}}, concurrencyCache.calls)
	require.Equal(t, [][]int64{{1, 2}, {3}}, sessionCache.calls)
	require.Equal(t, [][]int64{{4, 5}, {6}}, rpmCache.calls)
	require.Equal(t, map[int64]time.Duration{1: time.Minute, 2: 2 * time.Minute}, sessionCache.timeoutsByCall[0])
	require.Equal(t, map[int64]time.Duration{3: 3 * time.Minute}, sessionCache.timeoutsByCall[1])
}

func TestGroupCapacityRuntimeProvider_TTLCacheHitAndExpiry(t *testing.T) {
	sessionCache := &groupCapacityRuntimeSessionCacheStub{
		counts: map[int64]int{1: 1},
	}
	rpmCache := &groupCapacityRuntimeRPMCacheStub{
		counts: map[int64]int{2: 2},
	}
	provider := NewGroupCapacityRuntimeProviderService(NewConcurrencyService(&groupCapacityRuntimeConcurrencyCacheStub{}), sessionCache, rpmCache, 2*time.Second, 8)
	provider.now = func() time.Time { return time.Unix(100, 0) }

	snapshot := GroupCapacityStaticSnapshot{
		GroupID:                  9,
		SessionLimitedAccountIDs: []int64{1},
		RPMLimitedAccountIDs:     []int64{2},
		SessionTimeouts:          map[int64]time.Duration{1: time.Minute},
	}

	usage1, err := provider.GetGroupCapacityRuntimeUsage(context.Background(), snapshot)
	require.NoError(t, err)
	require.Equal(t, 1, usage1.ActiveSessions)
	require.Equal(t, 2, usage1.CurrentRPM)
	require.Len(t, sessionCache.calls, 1)
	require.Len(t, rpmCache.calls, 1)

	sessionCache.counts[1] = 9
	rpmCache.counts[2] = 8
	provider.now = func() time.Time { return time.Unix(101, 0) }

	usage2, err := provider.GetGroupCapacityRuntimeUsage(context.Background(), snapshot)
	require.NoError(t, err)
	require.Equal(t, usage1, usage2)
	require.Len(t, sessionCache.calls, 1)
	require.Len(t, rpmCache.calls, 1)

	provider.now = func() time.Time { return time.Unix(103, 0) }
	usage3, err := provider.GetGroupCapacityRuntimeUsage(context.Background(), snapshot)
	require.NoError(t, err)
	require.Equal(t, 9, usage3.ActiveSessions)
	require.Equal(t, 8, usage3.CurrentRPM)
	require.Len(t, sessionCache.calls, 2)
	require.Len(t, rpmCache.calls, 2)
}

func TestGroupCapacityRuntimeProvider_SingleflightMergesConcurrentLoads(t *testing.T) {
	release := make(chan struct{})
	sessionCache := &groupCapacityRuntimeSessionCacheStub{
		counts:  map[int64]int{1: 7},
		release: release,
	}
	rpmCache := &groupCapacityRuntimeRPMCacheStub{
		counts:  map[int64]int{2: 11},
		release: release,
	}
	provider := NewGroupCapacityRuntimeProviderService(NewConcurrencyService(&groupCapacityRuntimeConcurrencyCacheStub{}), sessionCache, rpmCache, time.Second, 16)
	provider.now = func() time.Time { return time.Unix(200, 0) }

	snapshot := GroupCapacityStaticSnapshot{
		GroupID:                  10,
		SessionLimitedAccountIDs: []int64{1},
		RPMLimitedAccountIDs:     []int64{2},
		SessionTimeouts:          map[int64]time.Duration{1: time.Minute},
	}

	const goroutines = 8
	start := make(chan struct{})
	var wg sync.WaitGroup
	errCh := make(chan error, goroutines)
	usageCh := make(chan GroupCapacityRuntimeUsage, goroutines)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			usage, err := provider.GetGroupCapacityRuntimeUsage(context.Background(), snapshot)
			errCh <- err
			usageCh <- usage
		}()
	}

	close(start)
	require.Eventually(t, func() bool {
		return sessionCache.started.Load() == 1
	}, time.Second, 10*time.Millisecond)
	close(release)
	wg.Wait()
	close(errCh)
	close(usageCh)

	for err := range errCh {
		require.NoError(t, err)
	}
	for usage := range usageCh {
		require.Equal(t, 7, usage.ActiveSessions)
		require.Equal(t, 11, usage.CurrentRPM)
	}
	require.Len(t, sessionCache.calls, 1)
	require.Len(t, rpmCache.calls, 1)
}

func TestGroupCapacityRuntimeCounter_TracksAcquireAndRelease(t *testing.T) {
	accountRepo := &groupCapacityRuntimeAccountRepoStub{
		accountsByID: map[int64]*Account{
			101: {ID: 101, GroupIDs: []int64{3, 4}},
		},
	}
	counter := NewGroupCapacityRuntimeCounter(accountRepo)

	release, err := counter.TrackAcquire(context.Background(), 101)
	require.NoError(t, err)
	require.NotNil(t, release)
	require.Equal(t, 1, counter.GetGroupConcurrencyUsed(3))
	require.Equal(t, 1, counter.GetGroupConcurrencyUsed(4))
	require.EqualValues(t, 1, accountRepo.getByIDCalls.Load())

	release()
	require.Equal(t, 0, counter.GetGroupConcurrencyUsed(3))
	require.Equal(t, 0, counter.GetGroupConcurrencyUsed(4))
}

func TestGroupCapacityRuntimeCounter_UsesLatestGroupBindings(t *testing.T) {
	accountRepo := &groupCapacityRuntimeAccountRepoStub{
		accountsByID: map[int64]*Account{
			101: {ID: 101, GroupIDs: []int64{3}},
		},
	}
	counter := NewGroupCapacityRuntimeCounter(accountRepo)

	release1, err := counter.TrackAcquire(context.Background(), 101)
	require.NoError(t, err)
	require.Equal(t, 1, counter.GetGroupConcurrencyUsed(3))
	require.Equal(t, 0, counter.GetGroupConcurrencyUsed(4))
	release1()

	accountRepo.accountsByID[101] = &Account{ID: 101, GroupIDs: []int64{4}}

	release2, err := counter.TrackAcquire(context.Background(), 101)
	require.NoError(t, err)
	require.Equal(t, 0, counter.GetGroupConcurrencyUsed(3))
	require.Equal(t, 1, counter.GetGroupConcurrencyUsed(4))
	require.EqualValues(t, 2, accountRepo.getByIDCalls.Load())
	release2()
	require.Equal(t, 0, counter.GetGroupConcurrencyUsed(4))
}

func TestConcurrencyService_AcquireReleaseUpdatesGroupRuntimeCounter(t *testing.T) {
	cache := &stubConcurrencyCacheForTest{acquireResult: true}
	accountRepo := &groupCapacityRuntimeAccountRepoStub{
		accountsByID: map[int64]*Account{
			42: {ID: 42, GroupIDs: []int64{7}},
		},
	}
	counter := NewGroupCapacityRuntimeCounter(accountRepo)
	svc := NewConcurrencyService(cache)
	svc.SetGroupCapacityRuntimeCounter(counter)

	result, err := svc.AcquireAccountSlot(context.Background(), 42, 3)
	require.NoError(t, err)
	require.True(t, result.Acquired)
	require.Equal(t, 1, counter.GetGroupConcurrencyUsed(7))

	result.ReleaseFunc()
	require.Equal(t, 0, counter.GetGroupConcurrencyUsed(7))
}

func TestGroupCapacityRuntimeProvider_FallbackContractFields(t *testing.T) {
	sessionCache := &groupCapacityRuntimeSessionCacheStub{
		counts: map[int64]int{1: 3, 2: 4},
	}
	rpmCache := &groupCapacityRuntimeRPMCacheStub{
		counts: map[int64]int{1: 6, 2: 7},
	}
	provider := NewGroupCapacityRuntimeProviderService(NewConcurrencyService(&groupCapacityRuntimeConcurrencyCacheStub{}), sessionCache, rpmCache, time.Second, 8)

	snapshot := GroupCapacityStaticSnapshot{
		GroupID:         11,
		AllAccountIDs:   []int64{1, 2},
		SessionTimeouts: map[int64]time.Duration{1: time.Minute, 2: 2 * time.Minute},
	}

	usage, err := provider.GetGroupCapacityRuntimeUsage(context.Background(), snapshot)
	require.NoError(t, err)
	require.Equal(t, 7, usage.ActiveSessions)
	require.Equal(t, 13, usage.CurrentRPM)
	require.Equal(t, [][]int64{{1, 2}}, sessionCache.calls)
	require.Equal(t, [][]int64{{1, 2}}, rpmCache.calls)
}

func TestGroupCapacityRuntimeCounter_PropagatesResolveError(t *testing.T) {
	accountRepo := &groupCapacityRuntimeAccountRepoStub{}
	counter := NewGroupCapacityRuntimeCounter(accountRepo)
	counter.accountGroupResolver = func(ctx context.Context, accountID int64) ([]int64, error) {
		return nil, errors.New("boom")
	}

	release, err := counter.TrackAcquire(context.Background(), 1)
	require.Error(t, err)
	require.Nil(t, release)
}
