//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type groupCapacitySnapshotProviderStub struct {
	getSnapshotFn func(ctx context.Context, groupID int64) (GroupCapacityStaticSnapshot, error)
	snapshot      GroupCapacityStaticSnapshot
	err           error
	calls         []int64
}

func (s *groupCapacitySnapshotProviderStub) GetGroupCapacityStaticSnapshot(ctx context.Context, groupID int64) (GroupCapacityStaticSnapshot, error) {
	s.calls = append(s.calls, groupID)
	if s.getSnapshotFn != nil {
		return s.getSnapshotFn(ctx, groupID)
	}
	return s.snapshot, s.err
}

type groupCapacityRuntimeProviderStub struct {
	getUsageFn    func(ctx context.Context, snapshot GroupCapacityStaticSnapshot) (GroupCapacityRuntimeUsage, error)
	usage         GroupCapacityRuntimeUsage
	err           error
	calls         int
	lastSnapshot  GroupCapacityStaticSnapshot
	seenSnapshots []GroupCapacityStaticSnapshot
}

func (s *groupCapacityRuntimeProviderStub) GetGroupCapacityRuntimeUsage(ctx context.Context, snapshot GroupCapacityStaticSnapshot) (GroupCapacityRuntimeUsage, error) {
	s.calls++
	s.lastSnapshot = snapshot
	s.seenSnapshots = append(s.seenSnapshots, snapshot)
	if s.getUsageFn != nil {
		return s.getUsageFn(ctx, snapshot)
	}
	return s.usage, s.err
}

type groupCapacitySessionLimitCacheStub struct {
	counts         map[int64]int
	lastAccountIDs []int64
	lastTimeouts   map[int64]time.Duration
}

func (s *groupCapacitySessionLimitCacheStub) RegisterSession(ctx context.Context, accountID int64, sessionUUID string, maxSessions int, idleTimeout time.Duration) (bool, error) {
	panic("unexpected RegisterSession call")
}

func (s *groupCapacitySessionLimitCacheStub) RefreshSession(ctx context.Context, accountID int64, sessionUUID string, idleTimeout time.Duration) error {
	panic("unexpected RefreshSession call")
}

func (s *groupCapacitySessionLimitCacheStub) GetActiveSessionCount(ctx context.Context, accountID int64) (int, error) {
	panic("unexpected GetActiveSessionCount call")
}

func (s *groupCapacitySessionLimitCacheStub) GetActiveSessionCountBatch(ctx context.Context, accountIDs []int64, idleTimeouts map[int64]time.Duration) (map[int64]int, error) {
	s.lastAccountIDs = append([]int64(nil), accountIDs...)
	s.lastTimeouts = idleTimeouts
	return s.counts, nil
}

func (s *groupCapacitySessionLimitCacheStub) IsSessionActive(ctx context.Context, accountID int64, sessionUUID string) (bool, error) {
	panic("unexpected IsSessionActive call")
}

func (s *groupCapacitySessionLimitCacheStub) GetWindowCost(ctx context.Context, accountID int64) (float64, bool, error) {
	panic("unexpected GetWindowCost call")
}

func (s *groupCapacitySessionLimitCacheStub) SetWindowCost(ctx context.Context, accountID int64, cost float64) error {
	panic("unexpected SetWindowCost call")
}

func (s *groupCapacitySessionLimitCacheStub) GetWindowCostBatch(ctx context.Context, accountIDs []int64) (map[int64]float64, error) {
	panic("unexpected GetWindowCostBatch call")
}

type groupCapacityRPMCacheStub struct {
	counts         map[int64]int
	lastAccountIDs []int64
}

func (s *groupCapacityRPMCacheStub) IncrementRPM(ctx context.Context, accountID int64) (int, error) {
	panic("unexpected IncrementRPM call")
}

func (s *groupCapacityRPMCacheStub) GetRPM(ctx context.Context, accountID int64) (int, error) {
	panic("unexpected GetRPM call")
}

func (s *groupCapacityRPMCacheStub) GetRPMBatch(ctx context.Context, accountIDs []int64) (map[int64]int, error) {
	s.lastAccountIDs = append([]int64(nil), accountIDs...)
	return s.counts, nil
}

type groupCapacityAccountRepoStub struct {
	listSchedulableByGroupIDFn func(ctx context.Context, groupID int64) ([]Account, error)
}

func (s *groupCapacityAccountRepoStub) Create(ctx context.Context, account *Account) error {
	panic("unexpected Create call")
}

func (s *groupCapacityAccountRepoStub) GetByID(ctx context.Context, id int64) (*Account, error) {
	panic("unexpected GetByID call")
}

func (s *groupCapacityAccountRepoStub) GetByIDs(ctx context.Context, ids []int64) ([]*Account, error) {
	panic("unexpected GetByIDs call")
}

func (s *groupCapacityAccountRepoStub) ExistsByID(ctx context.Context, id int64) (bool, error) {
	panic("unexpected ExistsByID call")
}

func (s *groupCapacityAccountRepoStub) GetByCRSAccountID(ctx context.Context, crsAccountID string) (*Account, error) {
	panic("unexpected GetByCRSAccountID call")
}

func (s *groupCapacityAccountRepoStub) FindByExtraField(ctx context.Context, key string, value any) ([]Account, error) {
	panic("unexpected FindByExtraField call")
}

func (s *groupCapacityAccountRepoStub) ListCRSAccountIDs(ctx context.Context) (map[string]int64, error) {
	panic("unexpected ListCRSAccountIDs call")
}

func (s *groupCapacityAccountRepoStub) Update(ctx context.Context, account *Account) error {
	panic("unexpected Update call")
}

func (s *groupCapacityAccountRepoStub) Delete(ctx context.Context, id int64) error {
	panic("unexpected Delete call")
}

func (s *groupCapacityAccountRepoStub) List(ctx context.Context, params pagination.PaginationParams) ([]Account, *pagination.PaginationResult, error) {
	panic("unexpected List call")
}

func (s *groupCapacityAccountRepoStub) ListWithFilters(ctx context.Context, params pagination.PaginationParams, filters AccountListFilters) ([]Account, *pagination.PaginationResult, error) {
	panic("unexpected ListWithFilters call")
}

func (s *groupCapacityAccountRepoStub) ListByGroup(ctx context.Context, groupID int64) ([]Account, error) {
	panic("unexpected ListByGroup call")
}

func (s *groupCapacityAccountRepoStub) ListActive(ctx context.Context) ([]Account, error) {
	panic("unexpected ListActive call")
}

func (s *groupCapacityAccountRepoStub) ListByPlatform(ctx context.Context, platform string) ([]Account, error) {
	panic("unexpected ListByPlatform call")
}

func (s *groupCapacityAccountRepoStub) UpdateLastUsed(ctx context.Context, id int64) error {
	panic("unexpected UpdateLastUsed call")
}

func (s *groupCapacityAccountRepoStub) BatchUpdateLastUsed(ctx context.Context, updates map[int64]time.Time) error {
	panic("unexpected BatchUpdateLastUsed call")
}

func (s *groupCapacityAccountRepoStub) SetError(ctx context.Context, id int64, errorMsg string) error {
	panic("unexpected SetError call")
}

func (s *groupCapacityAccountRepoStub) ClearError(ctx context.Context, id int64) error {
	panic("unexpected ClearError call")
}

func (s *groupCapacityAccountRepoStub) SetSchedulable(ctx context.Context, id int64, schedulable bool) error {
	panic("unexpected SetSchedulable call")
}

func (s *groupCapacityAccountRepoStub) AutoPauseExpiredAccounts(ctx context.Context, now time.Time) (int64, error) {
	panic("unexpected AutoPauseExpiredAccounts call")
}

func (s *groupCapacityAccountRepoStub) BindGroups(ctx context.Context, accountID int64, groupIDs []int64) error {
	panic("unexpected BindGroups call")
}

func (s *groupCapacityAccountRepoStub) ListSchedulable(ctx context.Context) ([]Account, error) {
	panic("unexpected ListSchedulable call")
}

func (s *groupCapacityAccountRepoStub) ListSchedulableByGroupID(ctx context.Context, groupID int64) ([]Account, error) {
	if s.listSchedulableByGroupIDFn != nil {
		return s.listSchedulableByGroupIDFn(ctx, groupID)
	}
	panic("unexpected ListSchedulableByGroupID call")
}

func (s *groupCapacityAccountRepoStub) ListSchedulableByPlatform(ctx context.Context, platform string) ([]Account, error) {
	panic("unexpected ListSchedulableByPlatform call")
}

func (s *groupCapacityAccountRepoStub) ListSchedulableByGroupIDAndPlatform(ctx context.Context, groupID int64, platform string) ([]Account, error) {
	panic("unexpected ListSchedulableByGroupIDAndPlatform call")
}

func (s *groupCapacityAccountRepoStub) ListSchedulableByPlatforms(ctx context.Context, platforms []string) ([]Account, error) {
	panic("unexpected ListSchedulableByPlatforms call")
}

func (s *groupCapacityAccountRepoStub) ListSchedulableByGroupIDAndPlatforms(ctx context.Context, groupID int64, platforms []string) ([]Account, error) {
	panic("unexpected ListSchedulableByGroupIDAndPlatforms call")
}

func (s *groupCapacityAccountRepoStub) ListSchedulableUngroupedByPlatform(ctx context.Context, platform string) ([]Account, error) {
	panic("unexpected ListSchedulableUngroupedByPlatform call")
}

func (s *groupCapacityAccountRepoStub) ListSchedulableUngroupedByPlatforms(ctx context.Context, platforms []string) ([]Account, error) {
	panic("unexpected ListSchedulableUngroupedByPlatforms call")
}

func (s *groupCapacityAccountRepoStub) SetRateLimited(ctx context.Context, id int64, resetAt time.Time) error {
	panic("unexpected SetRateLimited call")
}

func (s *groupCapacityAccountRepoStub) SetModelRateLimit(ctx context.Context, id int64, scope string, resetAt time.Time) error {
	panic("unexpected SetModelRateLimit call")
}

func (s *groupCapacityAccountRepoStub) SetOverloaded(ctx context.Context, id int64, until time.Time) error {
	panic("unexpected SetOverloaded call")
}

func (s *groupCapacityAccountRepoStub) SetTempUnschedulable(ctx context.Context, id int64, until time.Time, reason string) error {
	panic("unexpected SetTempUnschedulable call")
}

func (s *groupCapacityAccountRepoStub) ClearTempUnschedulable(ctx context.Context, id int64) error {
	panic("unexpected ClearTempUnschedulable call")
}

func (s *groupCapacityAccountRepoStub) ClearRateLimit(ctx context.Context, id int64) error {
	panic("unexpected ClearRateLimit call")
}

func (s *groupCapacityAccountRepoStub) ClearAntigravityQuotaScopes(ctx context.Context, id int64) error {
	panic("unexpected ClearAntigravityQuotaScopes call")
}

func (s *groupCapacityAccountRepoStub) ClearModelRateLimits(ctx context.Context, id int64) error {
	panic("unexpected ClearModelRateLimits call")
}

func (s *groupCapacityAccountRepoStub) UpdateSessionWindow(ctx context.Context, id int64, start, end *time.Time, status string) error {
	panic("unexpected UpdateSessionWindow call")
}

func (s *groupCapacityAccountRepoStub) UpdateExtra(ctx context.Context, id int64, updates map[string]any) error {
	panic("unexpected UpdateExtra call")
}

func (s *groupCapacityAccountRepoStub) BulkUpdate(ctx context.Context, ids []int64, updates AccountBulkUpdate) (int64, error) {
	panic("unexpected BulkUpdate call")
}

func (s *groupCapacityAccountRepoStub) IncrementQuotaUsed(ctx context.Context, id int64, amount float64) error {
	panic("unexpected IncrementQuotaUsed call")
}

func (s *groupCapacityAccountRepoStub) ResetQuotaUsed(ctx context.Context, id int64) error {
	panic("unexpected ResetQuotaUsed call")
}

type groupCapacityGroupRepoStub struct {
	listActiveFn func(ctx context.Context) ([]Group, error)
}

func (s *groupCapacityGroupRepoStub) Create(ctx context.Context, group *Group) error {
	panic("unexpected Create call")
}
func (s *groupCapacityGroupRepoStub) GetByID(ctx context.Context, id int64) (*Group, error) {
	panic("unexpected GetByID call")
}
func (s *groupCapacityGroupRepoStub) GetByIDLite(ctx context.Context, id int64) (*Group, error) {
	panic("unexpected GetByIDLite call")
}
func (s *groupCapacityGroupRepoStub) Update(ctx context.Context, group *Group) error {
	panic("unexpected Update call")
}
func (s *groupCapacityGroupRepoStub) Delete(ctx context.Context, id int64) error {
	panic("unexpected Delete call")
}
func (s *groupCapacityGroupRepoStub) DeleteCascade(ctx context.Context, id int64) ([]int64, error) {
	panic("unexpected DeleteCascade call")
}
func (s *groupCapacityGroupRepoStub) List(ctx context.Context, params pagination.PaginationParams) ([]Group, *pagination.PaginationResult, error) {
	panic("unexpected List call")
}
func (s *groupCapacityGroupRepoStub) ListWithFilters(ctx context.Context, params pagination.PaginationParams, platform, status, search string, isExclusive *bool) ([]Group, *pagination.PaginationResult, error) {
	panic("unexpected ListWithFilters call")
}
func (s *groupCapacityGroupRepoStub) ListActive(ctx context.Context) ([]Group, error) {
	if s.listActiveFn != nil {
		return s.listActiveFn(ctx)
	}
	panic("unexpected ListActive call")
}
func (s *groupCapacityGroupRepoStub) ListActiveByPlatform(ctx context.Context, platform string) ([]Group, error) {
	panic("unexpected ListActiveByPlatform call")
}
func (s *groupCapacityGroupRepoStub) ExistsByName(ctx context.Context, name string) (bool, error) {
	panic("unexpected ExistsByName call")
}
func (s *groupCapacityGroupRepoStub) GetAccountCount(ctx context.Context, groupID int64) (int64, int64, error) {
	panic("unexpected GetAccountCount call")
}
func (s *groupCapacityGroupRepoStub) DeleteAccountGroupsByGroupID(ctx context.Context, groupID int64) (int64, error) {
	panic("unexpected DeleteAccountGroupsByGroupID call")
}
func (s *groupCapacityGroupRepoStub) GetAccountIDsByGroupIDs(ctx context.Context, groupIDs []int64) ([]int64, error) {
	panic("unexpected GetAccountIDsByGroupIDs call")
}
func (s *groupCapacityGroupRepoStub) BindAccountsToGroup(ctx context.Context, groupID int64, accountIDs []int64) error {
	panic("unexpected BindAccountsToGroup call")
}
func (s *groupCapacityGroupRepoStub) UpdateSortOrders(ctx context.Context, updates []GroupSortOrderUpdate) error {
	panic("unexpected UpdateSortOrders call")
}

type groupCapacityConcurrencyCacheStub struct {
	counts         map[int64]int
	lastAccountIDs []int64
}

func (s *groupCapacityConcurrencyCacheStub) AcquireAccountSlot(ctx context.Context, accountID int64, maxConcurrency int, requestID string) (bool, error) {
	panic("unexpected AcquireAccountSlot call")
}
func (s *groupCapacityConcurrencyCacheStub) ReleaseAccountSlot(ctx context.Context, accountID int64, requestID string) error {
	panic("unexpected ReleaseAccountSlot call")
}
func (s *groupCapacityConcurrencyCacheStub) GetAccountConcurrency(ctx context.Context, accountID int64) (int, error) {
	panic("unexpected GetAccountConcurrency call")
}
func (s *groupCapacityConcurrencyCacheStub) GetAccountConcurrencyBatch(ctx context.Context, accountIDs []int64) (map[int64]int, error) {
	s.lastAccountIDs = append([]int64(nil), accountIDs...)
	return s.counts, nil
}
func (s *groupCapacityConcurrencyCacheStub) IncrementAccountWaitCount(ctx context.Context, accountID int64, maxWait int) (bool, error) {
	panic("unexpected IncrementAccountWaitCount call")
}
func (s *groupCapacityConcurrencyCacheStub) DecrementAccountWaitCount(ctx context.Context, accountID int64) error {
	panic("unexpected DecrementAccountWaitCount call")
}
func (s *groupCapacityConcurrencyCacheStub) GetAccountWaitingCount(ctx context.Context, accountID int64) (int, error) {
	panic("unexpected GetAccountWaitingCount call")
}
func (s *groupCapacityConcurrencyCacheStub) AcquireUserSlot(ctx context.Context, userID int64, maxConcurrency int, requestID string) (bool, error) {
	panic("unexpected AcquireUserSlot call")
}
func (s *groupCapacityConcurrencyCacheStub) ReleaseUserSlot(ctx context.Context, userID int64, requestID string) error {
	panic("unexpected ReleaseUserSlot call")
}
func (s *groupCapacityConcurrencyCacheStub) GetUserConcurrency(ctx context.Context, userID int64) (int, error) {
	panic("unexpected GetUserConcurrency call")
}
func (s *groupCapacityConcurrencyCacheStub) IncrementWaitCount(ctx context.Context, userID int64, maxWait int) (bool, error) {
	panic("unexpected IncrementWaitCount call")
}
func (s *groupCapacityConcurrencyCacheStub) DecrementWaitCount(ctx context.Context, userID int64) error {
	panic("unexpected DecrementWaitCount call")
}
func (s *groupCapacityConcurrencyCacheStub) GetAccountsLoadBatch(ctx context.Context, accounts []AccountWithConcurrency) (map[int64]*AccountLoadInfo, error) {
	panic("unexpected GetAccountsLoadBatch call")
}
func (s *groupCapacityConcurrencyCacheStub) GetUsersLoadBatch(ctx context.Context, users []UserWithConcurrency) (map[int64]*UserLoadInfo, error) {
	panic("unexpected GetUsersLoadBatch call")
}
func (s *groupCapacityConcurrencyCacheStub) CleanupExpiredActiveAccountSlots(ctx context.Context) error {
	panic("unexpected CleanupExpiredActiveAccountSlots call")
}
func (s *groupCapacityConcurrencyCacheStub) CleanupExpiredAccountSlots(ctx context.Context, accountID int64) error {
	panic("unexpected CleanupExpiredAccountSlots call")
}
func (s *groupCapacityConcurrencyCacheStub) CleanupStaleProcessSlots(ctx context.Context, activeRequestPrefix string) error {
	panic("unexpected CleanupStaleProcessSlots call")
}

func TestGroupCapacityService_GetGroupCapacity_UsesProvidersFirst(t *testing.T) {
	t.Parallel()

	snapshotProvider := &groupCapacitySnapshotProviderStub{
		snapshot: GroupCapacityStaticSnapshot{
			GroupID:        42,
			ConcurrencyMax: 12,
			SessionsMax:    8,
			RPMMax:         600,
		},
	}
	runtimeProvider := &groupCapacityRuntimeProviderStub{
		usage: GroupCapacityRuntimeUsage{
			GroupID:         42,
			ConcurrencyUsed: 3,
			ActiveSessions:  2,
			CurrentRPM:      90,
		},
	}
	accountRepo := &groupCapacityAccountRepoStub{}

	svc := NewGroupCapacityService(
		accountRepo,
		&groupCapacityGroupRepoStub{},
		nil,
		nil,
		nil,
		WithGroupCapacitySnapshotProvider(snapshotProvider),
		WithGroupCapacityRuntimeProvider(runtimeProvider),
	)

	summary, err := svc.getGroupCapacity(context.Background(), 42)
	require.NoError(t, err)
	require.Equal(t, GroupCapacitySummary{
		ConcurrencyUsed: 3,
		ConcurrencyMax:  12,
		SessionsUsed:    2,
		SessionsMax:     8,
		RPMUsed:         90,
		RPMMax:          600,
	}, summary)
	require.Equal(t, []int64{42}, snapshotProvider.calls)
	require.Equal(t, 1, runtimeProvider.calls)
	require.Equal(t, snapshotProvider.snapshot, runtimeProvider.lastSnapshot)
}

func TestGroupCapacityService_GetGroupCapacity_FallsBackToLegacy(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name             string
		snapshotProvider GroupCapacitySnapshotProvider
		runtimeProvider  GroupCapacityRuntimeProvider
	}{
		{
			name:             "providers missing",
			snapshotProvider: nil,
			runtimeProvider:  nil,
		},
		{
			name:             "snapshot provider unavailable",
			snapshotProvider: &groupCapacitySnapshotProviderStub{err: ErrGroupCapacityProviderUnavailable},
			runtimeProvider:  &groupCapacityRuntimeProviderStub{},
		},
		{
			name: "runtime provider unavailable",
			snapshotProvider: &groupCapacitySnapshotProviderStub{snapshot: GroupCapacityStaticSnapshot{
				GroupID:        7,
				ConcurrencyMax: 99,
				SessionsMax:    99,
				RPMMax:         99,
			}},
			runtimeProvider: &groupCapacityRuntimeProviderStub{err: ErrGroupCapacityProviderUnavailable},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			accountRepo := &groupCapacityAccountRepoStub{
				listSchedulableByGroupIDFn: func(ctx context.Context, groupID int64) ([]Account, error) {
					require.Equal(t, int64(7), groupID)
					return []Account{
						{
							ID:          101,
							Concurrency: 4,
							Extra: map[string]any{
								"max_sessions":                 3,
								"session_idle_timeout_minutes": 10,
								"base_rpm":                     120,
							},
						},
						{
							ID:          102,
							Concurrency: 2,
							Extra: map[string]any{
								"max_sessions":                 1,
								"session_idle_timeout_minutes": 7,
								"base_rpm":                     30,
							},
						},
					}, nil
				},
			}
			concurrencyCache := &groupCapacityConcurrencyCacheStub{counts: map[int64]int{101: 2, 102: 1}}
			sessionCache := &groupCapacitySessionLimitCacheStub{counts: map[int64]int{101: 1, 102: 1}}
			rpmCache := &groupCapacityRPMCacheStub{counts: map[int64]int{101: 80, 102: 15}}

			svc := NewGroupCapacityService(
				accountRepo,
				&groupCapacityGroupRepoStub{},
				NewConcurrencyService(concurrencyCache),
				sessionCache,
				rpmCache,
				WithGroupCapacitySnapshotProvider(tt.snapshotProvider),
				WithGroupCapacityRuntimeProvider(tt.runtimeProvider),
			)

			summary, err := svc.getGroupCapacity(context.Background(), 7)
			require.NoError(t, err)
			require.Equal(t, GroupCapacitySummary{
				ConcurrencyUsed: 3,
				ConcurrencyMax:  6,
				SessionsUsed:    2,
				SessionsMax:     4,
				RPMUsed:         95,
				RPMMax:          150,
			}, summary)
			require.Equal(t, []int64{101, 102}, concurrencyCache.lastAccountIDs)
			require.Equal(t, []int64{101, 102}, sessionCache.lastAccountIDs)
			require.Equal(t, []int64{101, 102}, rpmCache.lastAccountIDs)
			require.Equal(t, map[int64]time.Duration{
				101: 10 * time.Minute,
				102: 7 * time.Minute,
			}, sessionCache.lastTimeouts)
		})
	}
}

func TestGroupCapacityService_GetGroupCapacity_ProviderErrorPropagates(t *testing.T) {
	t.Parallel()

	t.Run("snapshot provider error", func(t *testing.T) {
		t.Parallel()

		expectedErr := context.DeadlineExceeded
		svc := NewGroupCapacityService(
			&groupCapacityAccountRepoStub{},
			&groupCapacityGroupRepoStub{},
			nil,
			nil,
			nil,
			WithGroupCapacitySnapshotProvider(&groupCapacitySnapshotProviderStub{err: expectedErr}),
			WithGroupCapacityRuntimeProvider(&groupCapacityRuntimeProviderStub{}),
		)

		_, err := svc.getGroupCapacity(context.Background(), 42)
		require.ErrorIs(t, err, expectedErr)
	})

	t.Run("runtime provider error", func(t *testing.T) {
		t.Parallel()

		expectedErr := context.Canceled
		snapshotProvider := &groupCapacitySnapshotProviderStub{snapshot: GroupCapacityStaticSnapshot{
			GroupID:        42,
			ConcurrencyMax: 12,
			SessionsMax:    8,
			RPMMax:         600,
		}}
		runtimeProvider := &groupCapacityRuntimeProviderStub{err: expectedErr}
		svc := NewGroupCapacityService(
			&groupCapacityAccountRepoStub{},
			&groupCapacityGroupRepoStub{},
			nil,
			nil,
			nil,
			WithGroupCapacitySnapshotProvider(snapshotProvider),
			WithGroupCapacityRuntimeProvider(runtimeProvider),
		)

		_, err := svc.getGroupCapacity(context.Background(), 42)
		require.ErrorIs(t, err, expectedErr)
		require.Equal(t, 1, runtimeProvider.calls)
	})
}

func TestGroupCapacityService_GetAllGroupCapacity_SkipsErroredGroups(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name             string
		snapshotProvider GroupCapacitySnapshotProvider
		runtimeProvider  GroupCapacityRuntimeProvider
	}{
		{
			name:             "snapshot provider error",
			snapshotProvider: &groupCapacitySnapshotProviderStub{err: context.DeadlineExceeded},
			runtimeProvider:  &groupCapacityRuntimeProviderStub{},
		},
		{
			name: "runtime provider error",
			snapshotProvider: &groupCapacitySnapshotProviderStub{snapshot: GroupCapacityStaticSnapshot{
				GroupID:        42,
				ConcurrencyMax: 12,
				SessionsMax:    8,
				RPMMax:         600,
			}},
			runtimeProvider: &groupCapacityRuntimeProviderStub{err: context.Canceled},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			groupRepo := &groupCapacityGroupRepoStub{
				listActiveFn: func(ctx context.Context) ([]Group, error) {
					return []Group{{ID: 42}, {ID: 43}}, nil
				},
			}
			snapshotProvider := tt.snapshotProvider
			runtimeProvider := tt.runtimeProvider
			if snapshotProvider == nil {
				snapshotProvider = &groupCapacitySnapshotProviderStub{}
			}
			if runtimeProvider == nil {
				runtimeProvider = &groupCapacityRuntimeProviderStub{}
			}
			if snapshotStub, ok := snapshotProvider.(*groupCapacitySnapshotProviderStub); ok {
				snapshotStub.getSnapshotFn = func(ctx context.Context, groupID int64) (GroupCapacityStaticSnapshot, error) {
					if groupID == 43 {
						return GroupCapacityStaticSnapshot{GroupID: 43, ConcurrencyMax: 2, SessionsMax: 1, RPMMax: 50}, nil
					}
					return snapshotStub.snapshot, snapshotStub.err
				}
			}
			if runtimeStub, ok := runtimeProvider.(*groupCapacityRuntimeProviderStub); ok {
				runtimeStub.getUsageFn = func(ctx context.Context, snapshot GroupCapacityStaticSnapshot) (GroupCapacityRuntimeUsage, error) {
					if snapshot.GroupID == 43 {
						return GroupCapacityRuntimeUsage{GroupID: 43, ConcurrencyUsed: 1, ActiveSessions: 1, CurrentRPM: 20}, nil
					}
					return runtimeStub.usage, runtimeStub.err
				}
			}
			svc := NewGroupCapacityService(
				&groupCapacityAccountRepoStub{},
				groupRepo,
				nil,
				nil,
				nil,
				WithGroupCapacitySnapshotProvider(snapshotProvider),
				WithGroupCapacityRuntimeProvider(runtimeProvider),
			)

			results, err := svc.GetAllGroupCapacity(context.Background())
			require.NoError(t, err)
			require.Equal(t, []GroupCapacitySummary{{
				GroupID:         43,
				ConcurrencyUsed: 1,
				ConcurrencyMax:  2,
				SessionsUsed:    1,
				SessionsMax:     1,
				RPMUsed:         20,
				RPMMax:          50,
			}}, results)
		})
	}
}

func TestGroupCapacityService_GetAllGroupCapacity_AggregatesByGroup(t *testing.T) {
	t.Parallel()

	groupRepo := &groupCapacityGroupRepoStub{
		listActiveFn: func(ctx context.Context) ([]Group, error) {
			return []Group{{ID: 11}, {ID: 12}}, nil
		},
	}
	snapshotProvider := &groupCapacitySnapshotProviderStub{
		getSnapshotFn: func(ctx context.Context, groupID int64) (GroupCapacityStaticSnapshot, error) {
			switch groupID {
			case 11:
				return GroupCapacityStaticSnapshot{GroupID: 11, ConcurrencyMax: 2, SessionsMax: 3, RPMMax: 100}, nil
			case 12:
				return GroupCapacityStaticSnapshot{GroupID: 12, ConcurrencyMax: 5, SessionsMax: 4, RPMMax: 150}, nil
			default:
				return GroupCapacityStaticSnapshot{}, nil
			}
		},
	}
	runtimeProvider := &groupCapacityRuntimeProviderStub{
		getUsageFn: func(ctx context.Context, snapshot GroupCapacityStaticSnapshot) (GroupCapacityRuntimeUsage, error) {
			switch snapshot.GroupID {
			case 11:
				return GroupCapacityRuntimeUsage{GroupID: 11, ConcurrencyUsed: 1, ActiveSessions: 2, CurrentRPM: 40}, nil
			case 12:
				return GroupCapacityRuntimeUsage{GroupID: 12, ConcurrencyUsed: 3, ActiveSessions: 1, CurrentRPM: 60}, nil
			default:
				return GroupCapacityRuntimeUsage{}, nil
			}
		},
	}

	svc := NewGroupCapacityService(
		&groupCapacityAccountRepoStub{},
		groupRepo,
		nil,
		nil,
		nil,
		WithGroupCapacitySnapshotProvider(snapshotProvider),
		WithGroupCapacityRuntimeProvider(runtimeProvider),
	)

	results, err := svc.GetAllGroupCapacity(context.Background())
	require.NoError(t, err)
	require.Equal(t, []GroupCapacitySummary{
		{
			GroupID:         11,
			ConcurrencyUsed: 1,
			ConcurrencyMax:  2,
			SessionsUsed:    2,
			SessionsMax:     3,
			RPMUsed:         40,
			RPMMax:          100,
		},
		{
			GroupID:         12,
			ConcurrencyUsed: 3,
			ConcurrencyMax:  5,
			SessionsUsed:    1,
			SessionsMax:     4,
			RPMUsed:         60,
			RPMMax:          150,
		},
	}, results)
	require.Equal(t, []int64{11, 12}, snapshotProvider.calls)
	require.Len(t, runtimeProvider.seenSnapshots, 2)
	require.Equal(t, int64(11), runtimeProvider.seenSnapshots[0].GroupID)
	require.Equal(t, int64(12), runtimeProvider.seenSnapshots[1].GroupID)
}
