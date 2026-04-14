package service

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type stubOpsRealtimeListAccountCall struct {
	platformFilter string
	groupIDFilter  *int64
}

type stubOpsRealtimeCache struct {
	ready                       bool
	accounts                    map[int64]*OpsRealtimeAccountCacheEntry
	warmStates                  map[int64]*OpsRealtimeWarmAccountState
	warmBucketMetas             map[int64]*OpsRealtimeWarmBucketMeta
	warmBucketMembers           map[int64][]string
	warmGlobal                  *OpsRealtimeWarmGlobalState
	warmOverview                *OpsOpenAIWarmPoolStats
	listAccountCalls            []stubOpsRealtimeListAccountCall
	setWarmOverviewCalls        int
	getWarmOverviewCalls        int
	deleteWarmOverviewOps       int
	clearWarmStateCall          int
	getWarmBucketMemberCalls    int
	getWarmBucketMemberBatchOps int
}

func (s *stubOpsRealtimeCache) IsAccountIndexReady(ctx context.Context) (bool, error) {
	return s.ready, nil
}
func (s *stubOpsRealtimeCache) ReplaceAccounts(ctx context.Context, accounts []*OpsRealtimeAccountCacheEntry) error {
	s.accounts = make(map[int64]*OpsRealtimeAccountCacheEntry, len(accounts))
	for _, account := range accounts {
		if account == nil || account.AccountID <= 0 {
			continue
		}
		s.accounts[account.AccountID] = account
	}
	s.ready = true
	return nil
}
func (s *stubOpsRealtimeCache) UpsertAccount(ctx context.Context, account *OpsRealtimeAccountCacheEntry) error {
	if s.accounts == nil {
		s.accounts = map[int64]*OpsRealtimeAccountCacheEntry{}
	}
	if account != nil && account.AccountID > 0 {
		s.accounts[account.AccountID] = account
	}
	return nil
}
func (s *stubOpsRealtimeCache) DeleteAccount(ctx context.Context, accountID int64) error {
	delete(s.accounts, accountID)
	return nil
}
func (s *stubOpsRealtimeCache) ListAccountIDs(ctx context.Context, platformFilter string, groupIDFilter *int64) ([]int64, error) {
	var groupFilterCopy *int64
	if groupIDFilter != nil {
		copied := *groupIDFilter
		groupFilterCopy = &copied
	}
	s.listAccountCalls = append(s.listAccountCalls, stubOpsRealtimeListAccountCall{platformFilter: platformFilter, groupIDFilter: groupFilterCopy})
	ids := make([]int64, 0, len(s.accounts))
	for accountID, entry := range s.accounts {
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
func (s *stubOpsRealtimeCache) GetAccounts(ctx context.Context, accountIDs []int64) (map[int64]*OpsRealtimeAccountCacheEntry, error) {
	result := make(map[int64]*OpsRealtimeAccountCacheEntry, len(accountIDs))
	for _, accountID := range accountIDs {
		if entry := s.accounts[accountID]; entry != nil {
			result[accountID] = entry
		}
	}
	return result, nil
}
func (s *stubOpsRealtimeCache) SetWarmAccountState(ctx context.Context, state *OpsRealtimeWarmAccountState) error {
	if s.warmStates == nil {
		s.warmStates = map[int64]*OpsRealtimeWarmAccountState{}
	}
	if state != nil && state.AccountID > 0 {
		s.warmStates[state.AccountID] = state
	}
	return nil
}
func (s *stubOpsRealtimeCache) DeleteWarmAccountState(ctx context.Context, accountID int64) error {
	delete(s.warmStates, accountID)
	return nil
}
func (s *stubOpsRealtimeCache) GetWarmAccountStates(ctx context.Context, accountIDs []int64) (map[int64]*OpsRealtimeWarmAccountState, error) {
	if len(accountIDs) == 0 {
		result := make(map[int64]*OpsRealtimeWarmAccountState, len(s.warmStates))
		for accountID, state := range s.warmStates {
			result[accountID] = state
		}
		return result, nil
	}
	result := make(map[int64]*OpsRealtimeWarmAccountState, len(accountIDs))
	for _, accountID := range accountIDs {
		if state := s.warmStates[accountID]; state != nil {
			result[accountID] = state
		}
	}
	return result, nil
}
func (s *stubOpsRealtimeCache) ClearWarmPoolState(ctx context.Context) error {
	s.clearWarmStateCall++
	s.warmStates = map[int64]*OpsRealtimeWarmAccountState{}
	s.warmBucketMetas = map[int64]*OpsRealtimeWarmBucketMeta{}
	s.warmBucketMembers = map[int64][]string{}
	s.warmGlobal = nil
	s.warmOverview = nil
	return nil
}
func (s *stubOpsRealtimeCache) TouchWarmBucketAccess(ctx context.Context, groupID int64, at time.Time) error {
	return nil
}
func (s *stubOpsRealtimeCache) TouchWarmBucketRefill(ctx context.Context, groupID int64, at time.Time) error {
	return nil
}
func (s *stubOpsRealtimeCache) IncrementWarmBucketTake(ctx context.Context, groupID int64, delta int64) error {
	return nil
}
func (s *stubOpsRealtimeCache) TouchWarmBucketMember(ctx context.Context, groupID int64, memberToken string, touchedAt time.Time) error {
	if s.warmBucketMembers == nil {
		s.warmBucketMembers = map[int64][]string{}
	}
	members := append([]string(nil), s.warmBucketMembers[groupID]...)
	for _, item := range members {
		if item == memberToken {
			s.warmBucketMembers[groupID] = members
			return nil
		}
	}
	members = append(members, memberToken)
	s.warmBucketMembers[groupID] = members
	return nil
}
func (s *stubOpsRealtimeCache) RemoveWarmBucketMember(ctx context.Context, groupID int64, memberToken string) error {
	members := s.warmBucketMembers[groupID]
	filtered := members[:0]
	for _, item := range members {
		if item == memberToken {
			continue
		}
		filtered = append(filtered, item)
	}
	s.warmBucketMembers[groupID] = append([]string(nil), filtered...)
	return nil
}
func (s *stubOpsRealtimeCache) RemoveWarmBucketAccount(ctx context.Context, groupID, accountID int64) error {
	members := s.warmBucketMembers[groupID]
	filtered := members[:0]
	suffix := ":" + strconv.FormatInt(accountID, 10)
	for _, item := range members {
		if strings.HasSuffix(item, suffix) {
			continue
		}
		filtered = append(filtered, item)
	}
	s.warmBucketMembers[groupID] = append([]string(nil), filtered...)
	return nil
}
func (s *stubOpsRealtimeCache) ListWarmBucketGroupIDs(ctx context.Context) ([]int64, error) {
	ids := make([]int64, 0, len(s.warmBucketMetas))
	for groupID := range s.warmBucketMetas {
		ids = append(ids, groupID)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids, nil
}
func (s *stubOpsRealtimeCache) GetWarmBucketMetas(ctx context.Context, groupIDs []int64) (map[int64]*OpsRealtimeWarmBucketMeta, error) {
	if len(groupIDs) == 0 {
		result := make(map[int64]*OpsRealtimeWarmBucketMeta, len(s.warmBucketMetas))
		for groupID, meta := range s.warmBucketMetas {
			result[groupID] = meta
		}
		return result, nil
	}
	result := make(map[int64]*OpsRealtimeWarmBucketMeta, len(groupIDs))
	for _, groupID := range groupIDs {
		if meta := s.warmBucketMetas[groupID]; meta != nil {
			result[groupID] = meta
		}
	}
	return result, nil
}
func (s *stubOpsRealtimeCache) GetWarmBucketMemberTokens(ctx context.Context, groupID int64, minTouchedAt time.Time) ([]string, error) {
	s.getWarmBucketMemberCalls++
	return append([]string(nil), s.warmBucketMembers[groupID]...), nil
}
func (s *stubOpsRealtimeCache) GetWarmBucketMemberTokensByGroups(ctx context.Context, groupIDs []int64, minTouchedAt time.Time) (map[int64][]string, error) {
	s.getWarmBucketMemberBatchOps++
	result := make(map[int64][]string, len(groupIDs))
	for _, groupID := range groupIDs {
		result[groupID] = append([]string(nil), s.warmBucketMembers[groupID]...)
	}
	return result, nil
}
func (s *stubOpsRealtimeCache) IncrementWarmGlobalTake(ctx context.Context, delta int64) error {
	return nil
}
func (s *stubOpsRealtimeCache) TouchWarmLastBucketMaintenance(ctx context.Context, at time.Time) error {
	return nil
}
func (s *stubOpsRealtimeCache) TouchWarmLastGlobalMaintenance(ctx context.Context, at time.Time) error {
	return nil
}
func (s *stubOpsRealtimeCache) GetWarmGlobalState(ctx context.Context) (*OpsRealtimeWarmGlobalState, bool, error) {
	if s.warmGlobal == nil {
		return nil, false, nil
	}
	return s.warmGlobal, true, nil
}
func (s *stubOpsRealtimeCache) GetWarmPoolOverviewSnapshot(ctx context.Context) (*OpsOpenAIWarmPoolStats, bool, error) {
	s.getWarmOverviewCalls++
	if s.warmOverview == nil {
		return nil, false, nil
	}
	return s.warmOverview, true, nil
}
func (s *stubOpsRealtimeCache) SetWarmPoolOverviewSnapshot(ctx context.Context, stats *OpsOpenAIWarmPoolStats, ttl time.Duration) error {
	s.setWarmOverviewCalls++
	s.warmOverview = stats
	return nil
}
func (s *stubOpsRealtimeCache) DeleteWarmPoolOverviewSnapshot(ctx context.Context) error {
	s.deleteWarmOverviewOps++
	s.warmOverview = nil
	return nil
}
func (s *stubOpsRealtimeCache) TryAcquireReconcileLeaderLock(ctx context.Context, owner string, ttl time.Duration) (func(), bool, error) {
	return func() {}, true, nil
}

func TestOpsServiceRealtimeCache_DrivesConcurrencyAndAvailability(t *testing.T) {
	ctx := context.Background()
	group := &Group{ID: 7001, Name: "openai-group", Platform: PlatformOpenAI}
	cache := &stubOpsRealtimeCache{
		ready: true,
		accounts: map[int64]*OpsRealtimeAccountCacheEntry{
			1001: BuildOpsRealtimeAccountCacheEntry(&Account{ID: 1001, Name: "cached-openai", Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 3, Groups: []*Group{group}, GroupIDs: []int64{group.ID}}),
		},
	}
	svc := ProvideOpsService(
		&opsRepoMock{},
		nil,
		&config.Config{Ops: config.OpsConfig{Enabled: true}},
		stubOpenAIAccountRepo{},
		nil,
		NewConcurrencyService(stubConcurrencyCache{loadMap: map[int64]*AccountLoadInfo{1001: {AccountID: 1001, CurrentConcurrency: 2, WaitingCount: 1}}}),
		nil,
		nil,
		nil,
		nil,
		cache,
		nil,
	)

	_, _, accountStats, _, err := svc.GetConcurrencyStatsWithOptions(ctx, PlatformOpenAI, &group.ID, opsRealtimeScopeAccount)
	require.NoError(t, err)
	require.Contains(t, accountStats, int64(1001))
	require.Equal(t, int64(2), accountStats[1001].CurrentInUse)
	require.Equal(t, int64(1), accountStats[1001].WaitingInQueue)
	require.Equal(t, "cached-openai", accountStats[1001].AccountName)

	_, _, availabilityStats, _, err := svc.GetAccountAvailabilityStatsWithOptions(ctx, PlatformOpenAI, &group.ID, opsRealtimeScopeAccount)
	require.NoError(t, err)
	require.Contains(t, availabilityStats, int64(1001))
	require.True(t, availabilityStats[1001].IsAvailable)
	require.Equal(t, group.Name, availabilityStats[1001].GroupName)
}

func TestOpsServiceGetOpenAIWarmPoolStats_UsesRealtimeMirrorCache(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	group := &Group{ID: 8101, Name: "warm-group", Platform: PlatformOpenAI}
	account := &Account{ID: 81001, Name: "warm-openai", Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 2, Priority: 1, Groups: []*Group{group}, GroupIDs: []int64{group.ID}}
	cache := &stubOpsRealtimeCache{
		ready: true,
		accounts: map[int64]*OpsRealtimeAccountCacheEntry{
			account.ID: BuildOpsRealtimeAccountCacheEntry(account),
		},
		warmStates: map[int64]*OpsRealtimeWarmAccountState{
			account.ID: {
				AccountID:  account.ID,
				State:      "ready",
				VerifiedAt: cloneTimePtr(&now),
				ExpiresAt:  cloneTimePtr(warmTestTimePtr(now.Add(5 * time.Minute))),
				UpdatedAt:  now,
			},
		},
		warmBucketMetas: map[int64]*OpsRealtimeWarmBucketMeta{
			group.ID: {GroupID: group.ID, LastAccessAt: cloneTimePtr(&now), LastRefillAt: cloneTimePtr(&now), TakeCount: 4},
		},
		warmBucketMembers: map[int64][]string{
			group.ID: {warmBucketMemberToken("instance-a", account.ID)},
		},
		warmGlobal: &OpsRealtimeWarmGlobalState{TakeCount: 4, LastBucketMaintenanceAt: cloneTimePtr(&now), LastGlobalMaintenanceAt: cloneTimePtr(&now)},
	}
	cfg := newOpenAIWarmPoolTestConfig()
	cfg.Ops.Enabled = true
	gatewaySvc := &OpenAIGatewayService{cfg: cfg}
	gatewaySvc.openaiWarmPool = newOpenAIAccountWarmPoolService(gatewaySvc)
	svc := ProvideOpsService(&opsRepoMock{}, nil, cfg, stubOpenAIAccountRepo{}, nil, nil, nil, gatewaySvc, nil, nil, cache, nil)

	stats, err := svc.GetOpenAIWarmPoolStatsWithOptions(ctx, &group.ID, true, "ready", false)
	require.NoError(t, err)
	require.NotNil(t, stats)
	require.True(t, stats.Enabled)
	require.True(t, stats.WarmPoolEnabled)
	require.NotNil(t, stats.Summary)
	require.Equal(t, int64(4), stats.Summary.TakeCount)
	require.Equal(t, 1, stats.Summary.GlobalReadyAccountCount)
	require.Len(t, stats.Buckets, 1)
	require.Equal(t, 1, stats.Buckets[0].BucketReadyAccounts)
	require.Equal(t, group.Name, stats.Buckets[0].GroupName)
	require.Len(t, stats.Accounts, 1)
	require.Equal(t, account.ID, stats.Accounts[0].AccountID)
	require.Equal(t, "ready", stats.Accounts[0].State)
}

func TestOpsServiceGetOpenAIWarmPoolStats_RealtimeMirrorUsesGlobalReadyCoverageBeyondBucketMembers(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	group := &Group{ID: 8111, Name: "warm-group", Platform: PlatformOpenAI}
	accounts := map[int64]*OpsRealtimeAccountCacheEntry{}
	warmStates := map[int64]*OpsRealtimeWarmAccountState{}
	for idx, accountID := range []int64{81101, 81102, 81103} {
		account := &Account{ID: accountID, Name: "warm-openai-" + strconv.Itoa(idx+1), Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 2, Priority: idx, Groups: []*Group{group}, GroupIDs: []int64{group.ID}}
		accounts[account.ID] = BuildOpsRealtimeAccountCacheEntry(account)
		warmStates[account.ID] = &OpsRealtimeWarmAccountState{
			AccountID:  account.ID,
			State:      "ready",
			VerifiedAt: cloneTimePtr(&now),
			ExpiresAt:  cloneTimePtr(warmTestTimePtr(now.Add(5 * time.Minute))),
			UpdatedAt:  now,
		}
	}
	cache := &stubOpsRealtimeCache{
		ready:      true,
		accounts:   accounts,
		warmStates: warmStates,
		warmBucketMetas: map[int64]*OpsRealtimeWarmBucketMeta{
			group.ID: {GroupID: group.ID, LastAccessAt: cloneTimePtr(&now), LastRefillAt: cloneTimePtr(&now), TakeCount: 2},
		},
		warmBucketMembers: map[int64][]string{
			group.ID: {warmBucketMemberToken("instance-a", 81101)},
		},
		warmGlobal: &OpsRealtimeWarmGlobalState{TakeCount: 2, LastBucketMaintenanceAt: cloneTimePtr(&now), LastGlobalMaintenanceAt: cloneTimePtr(&now)},
	}
	cfg := newOpenAIWarmPoolTestConfig()
	cfg.Ops.Enabled = true
	cfg.Gateway.OpenAIWS.AccountWarmPool.BucketTargetSize = 1
	cfg.Gateway.OpenAIWS.AccountWarmPool.BucketRefillBelow = 1
	cfg.Gateway.OpenAIWS.AccountWarmPool.GlobalTargetSize = 3
	cfg.Gateway.OpenAIWS.AccountWarmPool.GlobalRefillBelow = 1
	gatewaySvc := &OpenAIGatewayService{cfg: cfg}
	gatewaySvc.openaiWarmPool = newOpenAIAccountWarmPoolService(gatewaySvc)
	svc := ProvideOpsService(&opsRepoMock{}, nil, cfg, stubOpenAIAccountRepo{}, nil, nil, nil, gatewaySvc, nil, nil, cache, nil)

	stats, err := svc.GetOpenAIWarmPoolStatsWithOptions(ctx, nil, false, "", false)
	require.NoError(t, err)
	require.NotNil(t, stats)
	require.NotNil(t, stats.Summary)
	require.Equal(t, 3, stats.Summary.GlobalReadyAccountCount)
	require.Len(t, stats.GlobalCoverages, 1)
	require.Equal(t, 3, stats.GlobalCoverages[0].CoverageCount)
	require.Len(t, stats.Buckets, 1)
	require.Equal(t, 1, stats.Buckets[0].BucketReadyAccounts)
}

func TestOpsServiceGetOpenAIWarmPoolStats_CachesRealtimeOverviewSnapshotInRedis(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	group := &Group{ID: 8121, Name: "warm-group", Platform: PlatformOpenAI}
	account := &Account{ID: 81201, Name: "warm-openai", Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Concurrency: 2, Priority: 1, Groups: []*Group{group}, GroupIDs: []int64{group.ID}}
	cache := &stubOpsRealtimeCache{
		ready: true,
		accounts: map[int64]*OpsRealtimeAccountCacheEntry{
			account.ID: BuildOpsRealtimeAccountCacheEntry(account),
		},
		warmStates: map[int64]*OpsRealtimeWarmAccountState{
			account.ID: {
				AccountID:  account.ID,
				State:      "ready",
				VerifiedAt: cloneTimePtr(&now),
				ExpiresAt:  cloneTimePtr(warmTestTimePtr(now.Add(5 * time.Minute))),
				UpdatedAt:  now,
			},
		},
		warmBucketMetas: map[int64]*OpsRealtimeWarmBucketMeta{
			group.ID: {GroupID: group.ID, LastAccessAt: cloneTimePtr(&now), LastRefillAt: cloneTimePtr(&now), TakeCount: 1},
		},
		warmBucketMembers: map[int64][]string{
			group.ID: {warmBucketMemberToken("instance-a", account.ID)},
		},
		warmGlobal: &OpsRealtimeWarmGlobalState{TakeCount: 1, LastBucketMaintenanceAt: cloneTimePtr(&now), LastGlobalMaintenanceAt: cloneTimePtr(&now)},
	}
	cfg := newOpenAIWarmPoolTestConfig()
	cfg.Ops.Enabled = true
	gatewaySvc := &OpenAIGatewayService{cfg: cfg}
	gatewaySvc.openaiWarmPool = newOpenAIAccountWarmPoolService(gatewaySvc)
	svc := ProvideOpsService(&opsRepoMock{}, nil, cfg, stubOpenAIAccountRepo{}, nil, nil, nil, gatewaySvc, nil, nil, cache, nil)

	stats, err := svc.GetOpenAIWarmPoolStatsWithOptions(ctx, nil, false, "", false)
	require.NoError(t, err)
	require.NotNil(t, stats)
	require.NotNil(t, stats.Summary)
	require.Equal(t, 1, cache.setWarmOverviewCalls)
	require.Len(t, cache.listAccountCalls, 1)

	cache.accounts = map[int64]*OpsRealtimeAccountCacheEntry{}
	cache.listAccountCalls = nil
	stats, err = svc.GetOpenAIWarmPoolStatsWithOptions(ctx, nil, false, "", false)
	require.NoError(t, err)
	require.NotNil(t, stats)
	require.NotNil(t, stats.Summary)
	require.Empty(t, cache.listAccountCalls, "命中 Redis 预聚合概览快照后不应再重新扫描账号索引")
}

func TestOpsServiceGetOpenAIWarmPoolStats_OverviewBuildBatchesWarmBucketMemberReads(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	groupA := &Group{ID: 8125, Name: "group-a", Platform: PlatformOpenAI}
	groupB := &Group{ID: 8126, Name: "group-b", Platform: PlatformOpenAI}
	cache := &stubOpsRealtimeCache{
		ready: true,
		accounts: map[int64]*OpsRealtimeAccountCacheEntry{
			81251: BuildOpsRealtimeAccountCacheEntry(&Account{ID: 81251, Name: "a-1", Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Groups: []*Group{groupA}, GroupIDs: []int64{groupA.ID}}),
			81261: BuildOpsRealtimeAccountCacheEntry(&Account{ID: 81261, Name: "b-1", Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Groups: []*Group{groupB}, GroupIDs: []int64{groupB.ID}}),
		},
		warmStates: map[int64]*OpsRealtimeWarmAccountState{
			81251: {AccountID: 81251, State: "ready", VerifiedAt: cloneTimePtr(&now), ExpiresAt: cloneTimePtr(warmTestTimePtr(now.Add(5 * time.Minute))), UpdatedAt: now},
			81261: {AccountID: 81261, State: "ready", VerifiedAt: cloneTimePtr(&now), ExpiresAt: cloneTimePtr(warmTestTimePtr(now.Add(5 * time.Minute))), UpdatedAt: now},
		},
		warmBucketMetas: map[int64]*OpsRealtimeWarmBucketMeta{
			groupA.ID: {GroupID: groupA.ID, LastAccessAt: cloneTimePtr(&now), LastRefillAt: cloneTimePtr(&now), TakeCount: 1},
			groupB.ID: {GroupID: groupB.ID, LastAccessAt: cloneTimePtr(&now), LastRefillAt: cloneTimePtr(&now), TakeCount: 1},
		},
		warmBucketMembers: map[int64][]string{
			groupA.ID: {warmBucketMemberToken("instance-a", 81251)},
			groupB.ID: {warmBucketMemberToken("instance-b", 81261)},
		},
		warmGlobal: &OpsRealtimeWarmGlobalState{TakeCount: 2, LastBucketMaintenanceAt: cloneTimePtr(&now), LastGlobalMaintenanceAt: cloneTimePtr(&now)},
	}
	cfg := newOpenAIWarmPoolTestConfig()
	cfg.Ops.Enabled = true
	gatewaySvc := &OpenAIGatewayService{cfg: cfg}
	gatewaySvc.openaiWarmPool = newOpenAIAccountWarmPoolService(gatewaySvc)
	svc := ProvideOpsService(&opsRepoMock{}, nil, cfg, stubOpenAIAccountRepo{}, nil, nil, nil, gatewaySvc, nil, nil, cache, nil)

	stats, err := svc.GetOpenAIWarmPoolStatsWithOptions(ctx, nil, false, "", false)
	require.NoError(t, err)
	require.NotNil(t, stats)
	require.Len(t, stats.Buckets, 2)
	require.Equal(t, 1, cache.getWarmBucketMemberBatchOps)
	require.Zero(t, cache.getWarmBucketMemberCalls)
}

func TestOpsServiceGetOpenAIWarmPoolStats_GroupOverviewUsesSnapshotWithoutFullScan(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	groupA := &Group{ID: 8131, Name: "group-a", Platform: PlatformOpenAI}
	groupB := &Group{ID: 8132, Name: "group-b", Platform: PlatformOpenAI}
	cache := &stubOpsRealtimeCache{
		ready: true,
		accounts: map[int64]*OpsRealtimeAccountCacheEntry{
			81301: BuildOpsRealtimeAccountCacheEntry(&Account{ID: 81301, Name: "a-1", Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Groups: []*Group{groupA}, GroupIDs: []int64{groupA.ID}}),
			81302: BuildOpsRealtimeAccountCacheEntry(&Account{ID: 81302, Name: "b-1", Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Groups: []*Group{groupB}, GroupIDs: []int64{groupB.ID}}),
		},
		warmOverview: &OpsOpenAIWarmPoolStats{
			Enabled:         true,
			WarmPoolEnabled: true,
			ReaderReady:     true,
			Timestamp:       cloneTimePtr(&now),
			Summary:         &OpsOpenAIWarmPoolSummary{ActiveGroupCount: 2, GlobalReadyAccountCount: 2},
			Buckets: []*OpsOpenAIWarmPoolBucket{
				{GroupID: groupA.ID, GroupName: groupA.Name, BucketReadyAccounts: 1, BucketTargetSize: 2, BucketRefillBelow: 1},
				{GroupID: groupB.ID, GroupName: groupB.Name, BucketReadyAccounts: 1, BucketTargetSize: 2, BucketRefillBelow: 1},
			},
			GlobalCoverages: []*OpsOpenAIWarmPoolGroupCoverage{
				{GroupID: groupA.ID, GroupName: groupA.Name, CoverageCount: 2, TargetSize: 2, RefillBelow: 1},
				{GroupID: groupB.ID, GroupName: groupB.Name, CoverageCount: 2, TargetSize: 2, RefillBelow: 1},
			},
			NetworkErrorPool: &OpsOpenAIWarmPoolNetworkErrorPool{Capacity: 3},
		},
	}
	cfg := newOpenAIWarmPoolTestConfig()
	cfg.Ops.Enabled = true
	gatewaySvc := &OpenAIGatewayService{cfg: cfg}
	gatewaySvc.openaiWarmPool = newOpenAIAccountWarmPoolService(gatewaySvc)
	gatewaySvc.SetOpenAIWarmPoolUsageReader(&openAIWarmPoolUsageReaderStub{})
	waitForStartupBootstrapToSettle(t, gatewaySvc.getOpenAIWarmPool())
	svc := ProvideOpsService(&opsRepoMock{}, nil, cfg, stubOpenAIAccountRepo{}, nil, nil, nil, gatewaySvc, nil, nil, cache, nil)

	stats, err := svc.GetOpenAIWarmPoolStatsWithOptions(ctx, &groupA.ID, false, "", false)
	require.NoError(t, err)
	require.NotNil(t, stats)
	require.Len(t, stats.Buckets, 1)
	require.Equal(t, groupA.ID, stats.Buckets[0].GroupID)
	require.Empty(t, cache.listAccountCalls, "分组概览命中 Redis 概览快照后不应再执行全量或分组账号扫描")
}

func TestOpsServiceGetOpenAIWarmPoolStats_GroupAccountsUseGroupScopedRealtimeScan(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	groupA := &Group{ID: 8141, Name: "group-a", Platform: PlatformOpenAI}
	groupB := &Group{ID: 8142, Name: "group-b", Platform: PlatformOpenAI}
	cache := &stubOpsRealtimeCache{
		ready: true,
		accounts: map[int64]*OpsRealtimeAccountCacheEntry{
			81401: BuildOpsRealtimeAccountCacheEntry(&Account{ID: 81401, Name: "a-1", Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Groups: []*Group{groupA}, GroupIDs: []int64{groupA.ID}}),
			81402: BuildOpsRealtimeAccountCacheEntry(&Account{ID: 81402, Name: "b-1", Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Groups: []*Group{groupB}, GroupIDs: []int64{groupB.ID}}),
		},
		warmStates: map[int64]*OpsRealtimeWarmAccountState{
			81401: {AccountID: 81401, State: "ready", VerifiedAt: cloneTimePtr(&now), ExpiresAt: cloneTimePtr(warmTestTimePtr(now.Add(5 * time.Minute))), UpdatedAt: now},
			81402: {AccountID: 81402, State: "ready", VerifiedAt: cloneTimePtr(&now), ExpiresAt: cloneTimePtr(warmTestTimePtr(now.Add(5 * time.Minute))), UpdatedAt: now},
		},
		warmOverview: &OpsOpenAIWarmPoolStats{
			Enabled:         true,
			WarmPoolEnabled: true,
			ReaderReady:     true,
			Timestamp:       cloneTimePtr(&now),
			Summary:         &OpsOpenAIWarmPoolSummary{ActiveGroupCount: 2, GlobalReadyAccountCount: 2},
			Buckets: []*OpsOpenAIWarmPoolBucket{
				{GroupID: groupA.ID, GroupName: groupA.Name, BucketReadyAccounts: 1, BucketTargetSize: 2, BucketRefillBelow: 1},
				{GroupID: groupB.ID, GroupName: groupB.Name, BucketReadyAccounts: 1, BucketTargetSize: 2, BucketRefillBelow: 1},
			},
			GlobalCoverages: []*OpsOpenAIWarmPoolGroupCoverage{
				{GroupID: groupA.ID, GroupName: groupA.Name, CoverageCount: 2, TargetSize: 2, RefillBelow: 1},
				{GroupID: groupB.ID, GroupName: groupB.Name, CoverageCount: 2, TargetSize: 2, RefillBelow: 1},
			},
			NetworkErrorPool: &OpsOpenAIWarmPoolNetworkErrorPool{Capacity: 3},
		},
	}
	cfg := newOpenAIWarmPoolTestConfig()
	cfg.Ops.Enabled = true
	gatewaySvc := &OpenAIGatewayService{cfg: cfg}
	gatewaySvc.openaiWarmPool = newOpenAIAccountWarmPoolService(gatewaySvc)
	gatewaySvc.SetOpenAIWarmPoolUsageReader(&openAIWarmPoolUsageReaderStub{})
	waitForStartupBootstrapToSettle(t, gatewaySvc.getOpenAIWarmPool())
	svc := ProvideOpsService(&opsRepoMock{}, nil, cfg, stubOpenAIAccountRepo{}, nil, nil, nil, gatewaySvc, nil, nil, cache, nil)

	stats, err := svc.GetOpenAIWarmPoolStatsWithOptions(ctx, &groupA.ID, true, "ready", false)
	require.NoError(t, err)
	require.NotNil(t, stats)
	require.Len(t, stats.Accounts, 1)
	require.Equal(t, int64(81401), stats.Accounts[0].AccountID)
	require.Len(t, cache.listAccountCalls, 1)
	require.Equal(t, PlatformOpenAI, cache.listAccountCalls[0].platformFilter)
	require.NotNil(t, cache.listAccountCalls[0].groupIDFilter)
	require.Equal(t, groupA.ID, *cache.listAccountCalls[0].groupIDFilter)
}

func TestProvideSchedulerSnapshotService_ClearsWarmPoolCacheOnStartup(t *testing.T) {
	cfg := &config.Config{}
	repo := &countingOpsRealtimeAccountRepo{stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: []Account{{ID: 83001, Name: "openai-live", Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true}}}}
	cache := &stubOpsRealtimeCache{
		warmStates: map[int64]*OpsRealtimeWarmAccountState{
			83001: {AccountID: 83001, State: "ready", UpdatedAt: time.Now().UTC()},
		},
		warmBucketMetas: map[int64]*OpsRealtimeWarmBucketMeta{
			5: {GroupID: 5, LastAccessAt: warmTestTimePtr(time.Now().UTC())},
		},
		warmBucketMembers: map[int64][]string{
			5: {warmBucketMemberToken("instance-a", 83001)},
		},
		warmGlobal: &OpsRealtimeWarmGlobalState{TakeCount: 1},
	}

	svc := ProvideSchedulerSnapshotService(nil, nil, repo, nil, cfg, cache, nil)
	defer svc.Stop()

	require.Equal(t, 1, cache.clearWarmStateCall)
	require.Empty(t, cache.warmStates)
	require.Empty(t, cache.warmBucketMetas)
	require.Empty(t, cache.warmBucketMembers)
	require.Nil(t, cache.warmGlobal)
}

func TestOpsRealtimeProjectorRebuildAll_CleansStaleWarmMirror(t *testing.T) {
	ctx := context.Background()
	group := &Group{ID: 8201, Name: "warm-group", Platform: PlatformOpenAI}
	cache := &stubOpsRealtimeCache{
		warmStates: map[int64]*OpsRealtimeWarmAccountState{
			82001: {AccountID: 82001, State: "ready", UpdatedAt: time.Now().UTC()},
			82002: {AccountID: 82002, State: "ready", UpdatedAt: time.Now().UTC()},
		},
		warmBucketMetas: map[int64]*OpsRealtimeWarmBucketMeta{
			group.ID: {GroupID: group.ID},
		},
		warmBucketMembers: map[int64][]string{
			group.ID: {
				warmBucketMemberToken("instance-a", 82001),
				warmBucketMemberToken("instance-b", 82002),
			},
		},
	}
	repo := &countingOpsRealtimeAccountRepo{stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: []Account{{ID: 82001, Name: "openai-live", Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Groups: []*Group{group}}}}}
	projector := NewOpsRealtimeProjector(cache, repo)

	require.NoError(t, projector.RebuildAll(ctx))
	require.Contains(t, cache.accounts, int64(82001))
	require.NotContains(t, cache.warmStates, int64(82002), "全量 reconcile 后应清理已删除或已不再是 OpenAI 的 warm state")
	require.Len(t, cache.warmBucketMembers[group.ID], 1)
	require.Equal(t, warmBucketMemberToken("instance-a", 82001), cache.warmBucketMembers[group.ID][0])
}

func warmTestTimePtr(t time.Time) *time.Time { return &t }
