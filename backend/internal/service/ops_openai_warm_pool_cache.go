package service

import (
	"context"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"
)

type cachedWarmBucketSnapshot struct {
	groupID      int64
	lastAccessAt *time.Time
	lastRefillAt *time.Time
	takeCount    int64
	readyIDs     []int64
}

func warmInspectionFromCachedState(now time.Time, state *OpsRealtimeWarmAccountState) openAIWarmAccountInspection {
	inspection := openAIWarmAccountInspection{}
	if state == nil {
		return inspection
	}
	if strings.EqualFold(strings.TrimSpace(state.State), "probing") {
		inspection.Probing = true
	}
	inspection.VerifiedAt = cloneTimePtr(state.VerifiedAt)
	inspection.ExpiresAt = cloneTimePtr(state.ExpiresAt)
	inspection.FailUntil = cloneTimePtr(state.FailUntil)
	inspection.NetworkErrorAt = cloneTimePtr(state.NetworkErrorAt)
	inspection.NetworkErrorUntil = cloneTimePtr(state.NetworkErrorUntil)
	if inspection.FailUntil != nil && now.Before(*inspection.FailUntil) {
		inspection.Cooling = true
	}
	if inspection.NetworkErrorUntil != nil && now.Before(*inspection.NetworkErrorUntil) {
		inspection.NetworkError = true
	}
	if inspection.VerifiedAt != nil && !inspection.Cooling && !inspection.NetworkError {
		inspection.Ready = true
		if inspection.ExpiresAt != nil && !now.Before(*inspection.ExpiresAt) {
			inspection.Expired = true
		}
	}
	return inspection
}

func buildWarmInspectionCacheFromRealtimeState(now time.Time, stateEntries map[int64]*OpsRealtimeWarmAccountState) map[int64]openAIWarmAccountInspection {
	cache := make(map[int64]openAIWarmAccountInspection, len(stateEntries))
	for accountID, state := range stateEntries {
		if accountID <= 0 || state == nil {
			continue
		}
		cache[accountID] = warmInspectionFromCachedState(now, state)
	}
	return cache
}

func sortCachedWarmBucketSnapshots(snapshots []*cachedWarmBucketSnapshot) {
	sort.SliceStable(snapshots, func(i, j int) bool {
		left := snapshots[i]
		right := snapshots[j]
		switch {
		case left.lastAccessAt == nil && right.lastAccessAt != nil:
			return false
		case left.lastAccessAt != nil && right.lastAccessAt == nil:
			return true
		case left.lastAccessAt != nil && right.lastAccessAt != nil:
			if !left.lastAccessAt.Equal(*right.lastAccessAt) {
				return left.lastAccessAt.After(*right.lastAccessAt)
			}
		}
		return left.groupID < right.groupID
	})
}

func appendStartupCachedWarmBucketSnapshots(pool *openAIAccountWarmPoolService, snapshots []*cachedWarmBucketSnapshot) []*cachedWarmBucketSnapshot {
	if pool == nil {
		return snapshots
	}
	seen := make(map[int64]struct{}, len(snapshots))
	for _, snapshot := range snapshots {
		if snapshot == nil || snapshot.groupID <= 0 {
			continue
		}
		seen[snapshot.groupID] = struct{}{}
	}
	startupLastRefillAt := pool.startupOpsLastRefillAt()
	for _, groupID := range pool.startupOpsGroupIDs() {
		if groupID <= 0 {
			continue
		}
		if _, exists := seen[groupID]; exists {
			continue
		}
		snapshots = append(snapshots, &cachedWarmBucketSnapshot{
			groupID:      groupID,
			lastRefillAt: cloneTimePtr(startupLastRefillAt),
		})
	}
	sortCachedWarmBucketSnapshots(snapshots)
	return snapshots
}

func buildCachedWarmBucketSnapshots(now time.Time, activeBucketTTL time.Duration, metas map[int64]*OpsRealtimeWarmBucketMeta, readyIDsByGroup map[int64][]int64) []*cachedWarmBucketSnapshot {
	snapshots := make([]*cachedWarmBucketSnapshot, 0, len(metas))
	for groupID, meta := range metas {
		readyIDs := append([]int64(nil), readyIDsByGroup[groupID]...)
		var lastAccessAt *time.Time
		var lastRefillAt *time.Time
		var takeCount int64
		if meta != nil {
			lastAccessAt = cloneTimePtr(meta.LastAccessAt)
			lastRefillAt = cloneTimePtr(meta.LastRefillAt)
			takeCount = meta.TakeCount
		}
		if !warmBucketTimeWithinTTL(now, lastAccessAt, activeBucketTTL) && !warmBucketTimeWithinTTL(now, lastRefillAt, activeBucketTTL) && len(readyIDs) == 0 {
			continue
		}
		snapshots = append(snapshots, &cachedWarmBucketSnapshot{
			groupID:      groupID,
			lastAccessAt: lastAccessAt,
			lastRefillAt: lastRefillAt,
			takeCount:    takeCount,
			readyIDs:     readyIDs,
		})
	}
	sortCachedWarmBucketSnapshots(snapshots)
	return snapshots
}

func buildOpenAIWarmPoolCoverageFromCache(pool *openAIAccountWarmPoolService, snapshots []*cachedWarmBucketSnapshot, accounts []Account, inspectionCache map[int64]openAIWarmAccountInspection) openAIWarmPoolGlobalCoverageSnapshot {
	activeGroupIDs := make([]int64, 0, len(snapshots))
	activeGroupSet := make(map[int64]struct{}, len(snapshots))
	for _, snapshot := range snapshots {
		if snapshot == nil {
			continue
		}
		if _, exists := activeGroupSet[snapshot.groupID]; exists {
			continue
		}
		activeGroupSet[snapshot.groupID] = struct{}{}
		activeGroupIDs = append(activeGroupIDs, snapshot.groupID)
	}
	coverage := openAIWarmPoolGlobalCoverageSnapshot{
		activeGroupIDs:  activeGroupIDs,
		activeGroupSet:  activeGroupSet,
		coverageByGroup: make(map[int64]int, len(activeGroupIDs)),
	}
	for _, groupID := range activeGroupIDs {
		coverage.coverageByGroup[groupID] = 0
	}
	seenReady := make(map[int64]struct{}, len(accounts))
	for i := range accounts {
		account := &accounts[i]
		if account == nil || !account.IsOpenAI() || !account.IsSchedulable() {
			continue
		}
		bucketIDs := warmPoolBucketIDsForAccount(pool, account)
		activeOverlap := make([]int64, 0, len(bucketIDs))
		for _, bucketID := range bucketIDs {
			if _, exists := activeGroupSet[bucketID]; !exists {
				continue
			}
			activeOverlap = append(activeOverlap, bucketID)
		}
		if len(activeOverlap) == 0 {
			continue
		}
		inspection := inspectionCache[account.ID]
		if inspection.Cooling || inspection.NetworkError || !inspection.Ready || inspection.Expired {
			continue
		}
		if _, exists := seenReady[account.ID]; !exists {
			seenReady[account.ID] = struct{}{}
			coverage.uniqueReadyCount++
		}
		for _, groupID := range activeOverlap {
			coverage.coverageByGroup[groupID]++
		}
	}
	return coverage
}

func buildOpenAIWarmPoolBucketStatsFromCache(
	pool *openAIAccountWarmPoolService,
	groupID int64,
	snapshot *cachedWarmBucketSnapshot,
	accounts []Account,
	inspectionCache map[int64]openAIWarmAccountInspection,
	includeAccounts bool,
	accountStateFilter string,
) (*OpsOpenAIWarmPoolBucket, []*OpsOpenAIWarmPoolAccount) {
	cfg := pool.config()
	row := &OpsOpenAIWarmPoolBucket{
		GroupID:           groupID,
		BucketTargetSize:  cfg.BucketTargetSize,
		BucketRefillBelow: cfg.BucketRefillBelow,
	}
	if snapshot != nil {
		row.LastAccessAt = cloneTimePtr(snapshot.lastAccessAt)
		row.LastRefillAt = cloneTimePtr(snapshot.lastRefillAt)
		row.TakeCount = snapshot.takeCount
	}
	accountRows := make([]*OpsOpenAIWarmPoolAccount, 0, len(accounts))
	candidateIDs := make(map[int64]struct{}, len(accounts))
	for i := range accounts {
		acc := &accounts[i]
		if acc == nil || !acc.IsOpenAI() {
			continue
		}
		candidateIDs[acc.ID] = struct{}{}
		row.SchedulableAccounts++
		if row.GroupName == "" {
			row.GroupName = resolveWarmPoolGroupName(acc, groupID)
		}
		inspection := inspectionCache[acc.ID]
		if inspection.Probing {
			row.ProbingAccounts++
		}
		if inspection.Cooling {
			row.CoolingAccounts++
		}
		if includeAccounts {
			accountRow := buildOpenAIWarmPoolAccountRow(pool, acc, inspection)
			if warmPoolStateMatchesFilter(accountRow.State, accountStateFilter) {
				accountRows = append(accountRows, accountRow)
			}
		}
	}
	if snapshot != nil {
		for _, accountID := range snapshot.readyIDs {
			if len(candidateIDs) > 0 {
				if _, exists := candidateIDs[accountID]; !exists {
					continue
				}
			}
			inspection := inspectionCache[accountID]
			if inspection.Cooling || inspection.NetworkError || !inspection.Ready || inspection.Expired {
				continue
			}
			row.BucketReadyAccounts++
		}
	}
	if includeAccounts {
		sortWarmPoolAccountRows(accountRows)
	}
	return row, accountRows
}

func buildOpsWarmPoolSummaryFromCache(
	now time.Time,
	cfg openAIWarmPoolConfigView,
	stateEntries map[int64]*OpsRealtimeWarmAccountState,
	coverage openAIWarmPoolGlobalCoverageSnapshot,
	bucketRows []*OpsOpenAIWarmPoolBucket,
	globalState *OpsRealtimeWarmGlobalState,
	openAIAccountSet map[int64]struct{},
) *OpsOpenAIWarmPoolSummary {
	summary := &OpsOpenAIWarmPoolSummary{
		GlobalReadyAccountCount:    coverage.uniqueReadyCount,
		ActiveGroupCount:           len(coverage.activeGroupIDs),
		GlobalTargetPerActiveGroup: cfg.GlobalTargetSize,
		GlobalRefillPerActiveGroup: cfg.GlobalRefillBelow,
	}
	for accountID, state := range stateEntries {
		if len(openAIAccountSet) > 0 {
			if _, exists := openAIAccountSet[accountID]; !exists {
				continue
			}
		}
		inspection := warmInspectionFromCachedState(now, state)
		summary.TrackedAccountCount++
		if inspection.Probing {
			summary.ProbingAccountCount++
		}
		if inspection.Cooling {
			summary.CoolingAccountCount++
		}
		if inspection.NetworkError {
			summary.NetworkErrorPoolCount++
		}
	}
	for _, groupID := range coverage.activeGroupIDs {
		if coverage.coverageByGroup[groupID] < cfg.GlobalTargetSize {
			summary.GroupsBelowTargetCount++
		}
		if coverage.coverageByGroup[groupID] < cfg.GlobalRefillBelow {
			summary.GroupsBelowRefillCount++
		}
	}
	if len(bucketRows) > 0 {
		for _, row := range bucketRows {
			if row == nil {
				continue
			}
			summary.BucketReadyAccountCount += row.BucketReadyAccounts
		}
	} else {
		for _, groupID := range coverage.activeGroupIDs {
			summary.BucketReadyAccountCount += coverage.coverageByGroup[groupID]
		}
	}
	if globalState != nil {
		summary.TakeCount = globalState.TakeCount
		summary.LastBucketMaintenanceAt = cloneTimePtr(globalState.LastBucketMaintenanceAt)
		summary.LastGlobalMaintenanceAt = cloneTimePtr(globalState.LastGlobalMaintenanceAt)
	}
	if cfg.NetworkErrorPoolSize > 0 && summary.NetworkErrorPoolCount >= cfg.NetworkErrorPoolSize {
		summary.NetworkErrorPoolFull = true
	}
	return summary
}

func buildOpsWarmPoolNetworkErrorPoolFromCache(
	now time.Time,
	cfg openAIWarmPoolConfigView,
	stateEntries map[int64]*OpsRealtimeWarmAccountState,
	openAIAccountSet map[int64]struct{},
) *OpsOpenAIWarmPoolNetworkErrorPool {
	pool := &OpsOpenAIWarmPoolNetworkErrorPool{Capacity: cfg.NetworkErrorPoolSize}
	var oldest *time.Time
	for accountID, state := range stateEntries {
		if len(openAIAccountSet) > 0 {
			if _, exists := openAIAccountSet[accountID]; !exists {
				continue
			}
		}
		inspection := warmInspectionFromCachedState(now, state)
		if !inspection.NetworkError {
			continue
		}
		pool.Count++
		if inspection.NetworkErrorAt != nil && (oldest == nil || inspection.NetworkErrorAt.Before(*oldest)) {
			oldest = cloneTimePtr(inspection.NetworkErrorAt)
		}
	}
	pool.OldestEnteredAt = oldest
	return pool
}

func collectReadyBucketAccountIDsFromTokens(tokens []string) []int64 {
	if len(tokens) == 0 {
		return nil
	}
	seen := make(map[int64]struct{}, len(tokens))
	ids := make([]int64, 0, len(tokens))
	for _, token := range tokens {
		_, accountID, ok := parseWarmBucketMemberToken(token)
		if !ok {
			continue
		}
		if _, exists := seen[accountID]; exists {
			continue
		}
		seen[accountID] = struct{}{}
		ids = append(ids, accountID)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func buildOpenAIAccountSet(accounts []Account) map[int64]struct{} {
	set := make(map[int64]struct{}, len(accounts))
	for i := range accounts {
		acc := &accounts[i]
		if acc == nil || !acc.IsOpenAI() {
			continue
		}
		set[acc.ID] = struct{}{}
	}
	return set
}

func cloneWarmPoolStatsShallow(stats *OpsOpenAIWarmPoolStats) *OpsOpenAIWarmPoolStats {
	if stats == nil {
		return nil
	}
	cloned := *stats
	cloned.Buckets = append([]*OpsOpenAIWarmPoolBucket(nil), stats.Buckets...)
	cloned.Accounts = append([]*OpsOpenAIWarmPoolAccount(nil), stats.Accounts...)
	cloned.GlobalCoverages = append([]*OpsOpenAIWarmPoolGroupCoverage(nil), stats.GlobalCoverages...)
	return &cloned
}

func findWarmPoolBucketRow(stats *OpsOpenAIWarmPoolStats, groupID int64) *OpsOpenAIWarmPoolBucket {
	if stats == nil {
		return nil
	}
	for _, row := range stats.Buckets {
		if row != nil && row.GroupID == groupID {
			return row
		}
	}
	return nil
}

func subsetWarmPoolOverviewStatsByGroup(overview *OpsOpenAIWarmPoolStats, groupID int64, accountsOnly bool) *OpsOpenAIWarmPoolStats {
	stats := cloneWarmPoolStatsShallow(overview)
	if stats == nil {
		return nil
	}
	stats.Accounts = []*OpsOpenAIWarmPoolAccount{}
	if accountsOnly {
		stats.Buckets = []*OpsOpenAIWarmPoolBucket{}
		return stats
	}
	bucketRow := findWarmPoolBucketRow(overview, groupID)
	if bucketRow != nil {
		stats.Buckets = []*OpsOpenAIWarmPoolBucket{bucketRow}
	} else {
		stats.Buckets = []*OpsOpenAIWarmPoolBucket{}
	}
	return stats
}

func collectAccountIDs(accounts []Account) []int64 {
	ids := make([]int64, 0, len(accounts))
	for i := range accounts {
		if accounts[i].ID > 0 {
			ids = append(ids, accounts[i].ID)
		}
	}
	return ids
}

func warmPoolSnapshotFreshnessWindow(cfg openAIWarmPoolConfigView) time.Duration {
	freshnessWindow := cfg.BucketEntryTTL * 3
	if freshnessWindow > 0 {
		return freshnessWindow
	}
	freshnessWindow = time.Minute
	if cfg.ActiveBucketTTL > 0 && cfg.ActiveBucketTTL < freshnessWindow {
		freshnessWindow = cfg.ActiveBucketTTL
	}
	return freshnessWindow
}

func loadWarmBucketReadyIDsByGroup(ctx context.Context, cache OpsRealtimeCache, bucketMetas map[int64]*OpsRealtimeWarmBucketMeta, minTouchedAt time.Time) (map[int64][]int64, error) {
	readyIDsByGroup := make(map[int64][]int64, len(bucketMetas))
	if cache == nil || len(bucketMetas) == 0 {
		return readyIDsByGroup, nil
	}
	groupIDs := make([]int64, 0, len(bucketMetas))
	for groupID := range bucketMetas {
		groupIDs = append(groupIDs, groupID)
	}
	sort.Slice(groupIDs, func(i, j int) bool { return groupIDs[i] < groupIDs[j] })
	tokensByGroup, err := cache.GetWarmBucketMemberTokensByGroups(ctx, groupIDs, minTouchedAt)
	if err != nil {
		return nil, err
	}
	for _, groupID := range groupIDs {
		readyIDsByGroup[groupID] = collectReadyBucketAccountIDsFromTokens(tokensByGroup[groupID])
	}
	return readyIDsByGroup, nil
}

func (s *OpsService) buildOpenAIWarmPoolOverviewFromRealtimeCache(
	ctx context.Context,
	pool *openAIAccountWarmPoolService,
	base *OpsOpenAIWarmPoolStats,
) (*OpsOpenAIWarmPoolStats, bool, error) {
	if s == nil || s.opsRealtimeCache == nil || pool == nil || base == nil {
		return nil, false, nil
	}
	allAccounts, hit, err := s.listAllAccountsForOpsFromRealtimeCache(ctx, "", nil)
	if err != nil || !hit {
		return nil, hit, err
	}
	now := time.Now().UTC()
	cfg := pool.config()
	warmStates, err := s.opsRealtimeCache.GetWarmAccountStates(ctx, nil)
	if err != nil {
		return nil, false, err
	}
	inspectionCache := buildWarmInspectionCacheFromRealtimeState(now, warmStates)
	groupIDs, err := s.opsRealtimeCache.ListWarmBucketGroupIDs(ctx)
	if err != nil {
		return nil, false, err
	}
	bucketMetas, err := s.opsRealtimeCache.GetWarmBucketMetas(ctx, groupIDs)
	if err != nil {
		return nil, false, err
	}
	readyIDsByGroup, err := loadWarmBucketReadyIDsByGroup(ctx, s.opsRealtimeCache, bucketMetas, now.Add(-warmPoolSnapshotFreshnessWindow(cfg)))
	if err != nil {
		return nil, false, err
	}
	snapshots := buildCachedWarmBucketSnapshots(now, cfg.ActiveBucketTTL, bucketMetas, readyIDsByGroup)
	snapshots = appendStartupCachedWarmBucketSnapshots(pool, snapshots)
	globalCoverage := buildOpenAIWarmPoolCoverageFromCache(pool, snapshots, allAccounts, inspectionCache)
	globalState, _, err := s.opsRealtimeCache.GetWarmGlobalState(ctx)
	if err != nil {
		return nil, false, err
	}
	stats := &OpsOpenAIWarmPoolStats{
		Enabled:         base.Enabled,
		WarmPoolEnabled: base.WarmPoolEnabled,
		ReaderReady:     base.ReaderReady,
		Bootstrapping:   base.Bootstrapping,
		Timestamp:       base.Timestamp,
		Buckets:         []*OpsOpenAIWarmPoolBucket{},
		Accounts:        []*OpsOpenAIWarmPoolAccount{},
		GlobalCoverages: []*OpsOpenAIWarmPoolGroupCoverage{},
	}
	openAIAccountSet := buildOpenAIAccountSet(allAccounts)
	stats.NetworkErrorPool = buildOpsWarmPoolNetworkErrorPoolFromCache(now, cfg, warmStates, openAIAccountSet)
	accountsByBucket := groupSchedulableOpenAIAccountsForWarmPool(pool, allAccounts)
	bucketRows := make([]*OpsOpenAIWarmPoolBucket, 0, len(snapshots))
	for _, snapshot := range snapshots {
		if snapshot == nil {
			continue
		}
		bucketRow, _ := buildOpenAIWarmPoolBucketStatsFromCache(pool, snapshot.groupID, snapshot, accountsByBucket[snapshot.groupID], inspectionCache, false, "")
		bucketRows = append(bucketRows, bucketRow)
	}
	stats.Buckets = bucketRows
	stats.GlobalCoverages = buildOpenAIWarmPoolGroupCoverageRowsFromBuckets(globalCoverage, bucketRows, cfg)
	stats.Summary = buildOpsWarmPoolSummaryFromCache(now, cfg, warmStates, globalCoverage, bucketRows, globalState, openAIAccountSet)
	return stats, true, nil
}

func (s *OpsService) getOrBuildOpenAIWarmPoolOverviewSnapshot(
	ctx context.Context,
	pool *openAIAccountWarmPoolService,
	base *OpsOpenAIWarmPoolStats,
	logOverview bool,
) (*OpsOpenAIWarmPoolStats, bool, error) {
	if s == nil || s.opsRealtimeCache == nil || pool == nil || base == nil {
		return nil, false, nil
	}
	startedAt := time.Now()
	if cached, hit, err := s.opsRealtimeCache.GetWarmPoolOverviewSnapshot(ctx); err != nil {
		return nil, false, err
	} else if hit && cached != nil && cached.Bootstrapping == base.Bootstrapping && cached.ReaderReady == base.ReaderReady {
		if logOverview {
			newOpsTraceWithComponent(openAIWarmPoolLogComponent, "warm_pool_overview").
				WithBranch("redis_snapshot_hit").
				WithCacheHit(true).
				WithDurationSince(startedAt).
				Info("ops_warm_pool_overview")
		}
		return cached, true, nil
	}
	singleflightKey := strings.Join([]string{
		"ops_warm_pool_overview_snapshot",
		strconv.FormatBool(base.ReaderReady),
		strconv.FormatBool(base.Bootstrapping),
		strconv.FormatInt(pool.opsWarmPoolStatsCacheRevision(), 10),
	}, ":")
	value, err, shared := s.realtimeSnapshotFlight.Do(singleflightKey, func() (any, error) {
		if cached, hit, err := s.opsRealtimeCache.GetWarmPoolOverviewSnapshot(ctx); err != nil {
			return nil, err
		} else if hit && cached != nil && cached.Bootstrapping == base.Bootstrapping && cached.ReaderReady == base.ReaderReady {
			return cached, nil
		}
		stats, hit, err := s.buildOpenAIWarmPoolOverviewFromRealtimeCache(ctx, pool, base)
		if err != nil || !hit || stats == nil {
			return nil, err
		}
		if err := s.opsRealtimeCache.SetWarmPoolOverviewSnapshot(ctx, stats, opsRealtimeResultCacheTTL); err != nil {
			return nil, err
		}
		return stats, nil
	})
	if err != nil {
		return nil, false, err
	}
	stats, _ := value.(*OpsOpenAIWarmPoolStats)
	if stats == nil {
		return nil, false, nil
	}
	if logOverview {
		newOpsTraceWithComponent(openAIWarmPoolLogComponent, "warm_pool_overview").
			WithBranch("redis_snapshot_rebuild").
			WithCacheHit(false).
			WithSingleflightShared(shared).
			WithAttrs(slog.Int("bucket_count", len(stats.Buckets))).
			WithDurationSince(startedAt).
			Info("ops_warm_pool_overview")
	}
	return stats, true, nil
}

func (s *OpsService) buildWarmPoolGroupSnapshotFromRealtimeCache(
	ctx context.Context,
	pool *openAIAccountWarmPoolService,
	groupID int64,
	now time.Time,
	cfg openAIWarmPoolConfigView,
) (*cachedWarmBucketSnapshot, error) {
	if s == nil || s.opsRealtimeCache == nil || groupID <= 0 {
		return nil, nil
	}
	bucketMetas, err := s.opsRealtimeCache.GetWarmBucketMetas(ctx, []int64{groupID})
	if err != nil {
		return nil, err
	}
	if len(bucketMetas) == 0 {
		return nil, nil
	}
	readyIDsByGroup, err := loadWarmBucketReadyIDsByGroup(ctx, s.opsRealtimeCache, bucketMetas, now.Add(-warmPoolSnapshotFreshnessWindow(cfg)))
	if err != nil {
		return nil, err
	}
	snapshots := buildCachedWarmBucketSnapshots(now, cfg.ActiveBucketTTL, bucketMetas, readyIDsByGroup)
	snapshots = appendStartupCachedWarmBucketSnapshots(pool, snapshots)
	for _, snapshot := range snapshots {
		if snapshot != nil && snapshot.groupID == groupID {
			return snapshot, nil
		}
	}
	return nil, nil
}

func (s *OpsService) buildOpenAIWarmPoolGroupStatsFromRealtimeCache(
	ctx context.Context,
	pool *openAIAccountWarmPoolService,
	normalizedGroupID int64,
	includeAccounts bool,
	stateFilter string,
	accountsOnly bool,
	overview *OpsOpenAIWarmPoolStats,
	base *OpsOpenAIWarmPoolStats,
) (*OpsOpenAIWarmPoolStats, bool, error) {
	if s == nil || s.opsRealtimeCache == nil || pool == nil {
		return nil, false, nil
	}
	groupPtr := pool.groupIDPointer(normalizedGroupID)
	accounts, hit, err := s.listAllAccountsForOpsFromRealtimeCache(ctx, PlatformOpenAI, groupPtr)
	if err != nil || !hit {
		return nil, hit, err
	}
	now := time.Now().UTC()
	warmStates, err := s.opsRealtimeCache.GetWarmAccountStates(ctx, collectAccountIDs(accounts))
	if err != nil {
		return nil, false, err
	}
	inspectionCache := buildWarmInspectionCacheFromRealtimeState(now, warmStates)
	stats := subsetWarmPoolOverviewStatsByGroup(overview, normalizedGroupID, accountsOnly)
	if stats == nil {
		stats = &OpsOpenAIWarmPoolStats{
			Buckets:         []*OpsOpenAIWarmPoolBucket{},
			Accounts:        []*OpsOpenAIWarmPoolAccount{},
			GlobalCoverages: []*OpsOpenAIWarmPoolGroupCoverage{},
		}
		if base != nil {
			stats.Enabled = base.Enabled
			stats.WarmPoolEnabled = base.WarmPoolEnabled
			stats.ReaderReady = base.ReaderReady
			stats.Bootstrapping = base.Bootstrapping
			stats.Timestamp = base.Timestamp
		}
	}
	if accountsOnly {
		stats.Accounts = buildOpenAIWarmPoolAccountRows(pool, accounts, now, inspectionCache, stateFilter)
		return stats, true, nil
	}
	if includeAccounts {
		stats.Accounts = buildOpenAIWarmPoolAccountRows(pool, accounts, now, inspectionCache, stateFilter)
	}
	if bucketRow := findWarmPoolBucketRow(overview, normalizedGroupID); bucketRow != nil {
		stats.Buckets = []*OpsOpenAIWarmPoolBucket{bucketRow}
		return stats, true, nil
	}
	cfg := pool.config()
	groupSnapshot, err := s.buildWarmPoolGroupSnapshotFromRealtimeCache(ctx, pool, normalizedGroupID, now, cfg)
	if err != nil {
		return nil, false, err
	}
	bucketRow, accountRows := buildOpenAIWarmPoolBucketStatsFromCache(pool, normalizedGroupID, groupSnapshot, accounts, inspectionCache, includeAccounts, stateFilter)
	stats.Buckets = []*OpsOpenAIWarmPoolBucket{bucketRow}
	if includeAccounts {
		stats.Accounts = accountRows
	}
	return stats, true, nil
}

func (s *OpsService) buildOpenAIWarmPoolStatsFromRealtimeRaw(
	ctx context.Context,
	pool *openAIAccountWarmPoolService,
	groupID *int64,
	includeAccounts bool,
	stateFilter string,
	accountsOnly bool,
	base *OpsOpenAIWarmPoolStats,
) (*OpsOpenAIWarmPoolStats, bool, error) {
	if s == nil || s.opsRealtimeCache == nil || pool == nil || base == nil {
		return nil, false, nil
	}
	allAccounts, hit, err := s.listAllAccountsForOpsFromRealtimeCache(ctx, "", nil)
	if err != nil {
		return nil, false, err
	}
	if !hit {
		return nil, false, nil
	}
	startedAt := time.Now()
	now := startedAt.UTC()
	cfg := pool.config()
	warmStates, err := s.opsRealtimeCache.GetWarmAccountStates(ctx, nil)
	if err != nil {
		return nil, false, err
	}
	inspectionCache := buildWarmInspectionCacheFromRealtimeState(now, warmStates)
	groupIDs, err := s.opsRealtimeCache.ListWarmBucketGroupIDs(ctx)
	if err != nil {
		return nil, false, err
	}
	bucketMetas, err := s.opsRealtimeCache.GetWarmBucketMetas(ctx, groupIDs)
	if err != nil {
		return nil, false, err
	}
	readyIDsByGroup, err := loadWarmBucketReadyIDsByGroup(ctx, s.opsRealtimeCache, bucketMetas, now.Add(-warmPoolSnapshotFreshnessWindow(cfg)))
	if err != nil {
		return nil, false, err
	}
	snapshots := buildCachedWarmBucketSnapshots(now, cfg.ActiveBucketTTL, bucketMetas, readyIDsByGroup)
	snapshots = appendStartupCachedWarmBucketSnapshots(pool, snapshots)
	globalCoverage := buildOpenAIWarmPoolCoverageFromCache(pool, snapshots, allAccounts, inspectionCache)
	globalState, _, err := s.opsRealtimeCache.GetWarmGlobalState(ctx)
	if err != nil {
		return nil, false, err
	}
	stats := &OpsOpenAIWarmPoolStats{
		Enabled:         base.Enabled,
		WarmPoolEnabled: base.WarmPoolEnabled,
		ReaderReady:     base.ReaderReady,
		Bootstrapping:   base.Bootstrapping,
		Timestamp:       base.Timestamp,
		Buckets:         []*OpsOpenAIWarmPoolBucket{},
		Accounts:        []*OpsOpenAIWarmPoolAccount{},
		GlobalCoverages: []*OpsOpenAIWarmPoolGroupCoverage{},
	}
	openAIAccountSet := buildOpenAIAccountSet(allAccounts)
	stats.NetworkErrorPool = buildOpsWarmPoolNetworkErrorPoolFromCache(now, cfg, warmStates, openAIAccountSet)
	accountsByBucket := groupSchedulableOpenAIAccountsForWarmPool(pool, allAccounts)
	if accountsOnly {
		targetAccounts := allAccounts
		if groupID != nil {
			targetAccounts = accountsByBucket[pool.normalizeGroupID(groupID)]
		}
		stats.Accounts = buildOpenAIWarmPoolAccountRows(pool, targetAccounts, now, inspectionCache, stateFilter)
		stats.GlobalCoverages = buildOpenAIWarmPoolGroupCoverageRowsFromBuckets(globalCoverage, nil, cfg)
		stats.Summary = buildOpsWarmPoolSummaryFromCache(now, cfg, warmStates, globalCoverage, nil, globalState, openAIAccountSet)
		return stats, true, nil
	}
	bucketRows := make([]*OpsOpenAIWarmPoolBucket, 0, len(snapshots))
	if groupID != nil {
		normalizedGroupID := pool.normalizeGroupID(groupID)
		var groupSnapshot *cachedWarmBucketSnapshot
		for _, snapshot := range snapshots {
			if snapshot != nil && snapshot.groupID == normalizedGroupID {
				groupSnapshot = snapshot
				break
			}
		}
		bucketRow, accountRows := buildOpenAIWarmPoolBucketStatsFromCache(pool, normalizedGroupID, groupSnapshot, accountsByBucket[normalizedGroupID], inspectionCache, includeAccounts, stateFilter)
		bucketRows = append(bucketRows, bucketRow)
		if includeAccounts {
			stats.Accounts = accountRows
		}
	} else {
		for _, snapshot := range snapshots {
			if snapshot == nil {
				continue
			}
			bucketRow, _ := buildOpenAIWarmPoolBucketStatsFromCache(pool, snapshot.groupID, snapshot, accountsByBucket[snapshot.groupID], inspectionCache, false, "")
			bucketRows = append(bucketRows, bucketRow)
		}
		if includeAccounts {
			stats.Accounts = buildOpenAIWarmPoolAccountRows(pool, allAccounts, now, inspectionCache, stateFilter)
		}
	}
	stats.Buckets = bucketRows
	stats.GlobalCoverages = buildOpenAIWarmPoolGroupCoverageRowsFromBuckets(globalCoverage, bucketRows, cfg)
	stats.Summary = buildOpsWarmPoolSummaryFromCache(now, cfg, warmStates, globalCoverage, bucketRows, globalState, openAIAccountSet)
	if groupID == nil && !includeAccounts && !accountsOnly {
		newOpsTraceWithComponent(openAIWarmPoolLogComponent, "warm_pool_overview").
			WithBranch("realtime_raw_rebuild").
			WithCacheHit(false).
			WithAttrs(slog.Int("bucket_count", len(bucketRows))).
			WithDurationSince(startedAt).
			Info("ops_warm_pool_overview")
	}
	return stats, true, nil
}

func (s *OpsService) getOpenAIWarmPoolStatsFromRealtimeCache(
	ctx context.Context,
	pool *openAIAccountWarmPoolService,
	groupID *int64,
	includeAccounts bool,
	accountStateFilter string,
	accountsOnly bool,
	base *OpsOpenAIWarmPoolStats,
) (*OpsOpenAIWarmPoolStats, bool, error) {
	if s == nil || s.opsRealtimeCache == nil || pool == nil || base == nil {
		return nil, false, nil
	}
	ready, err := s.opsRealtimeCache.IsAccountIndexReady(ctx)
	if err != nil {
		return nil, false, err
	}
	if !ready {
		return nil, false, nil
	}
	stateFilter := normalizeWarmPoolStateFilter(accountStateFilter)
	logOverview := groupID == nil && !includeAccounts && !accountsOnly
	overview, overviewHit, err := s.getOrBuildOpenAIWarmPoolOverviewSnapshot(ctx, pool, base, logOverview)
	if err != nil {
		return nil, false, err
	}
	if overviewHit {
		if groupID != nil {
			normalizedGroupID := pool.normalizeGroupID(groupID)
			if !includeAccounts && !accountsOnly && findWarmPoolBucketRow(overview, normalizedGroupID) != nil {
				return subsetWarmPoolOverviewStatsByGroup(overview, normalizedGroupID, false), true, nil
			}
			return s.buildOpenAIWarmPoolGroupStatsFromRealtimeCache(ctx, pool, normalizedGroupID, includeAccounts, stateFilter, accountsOnly, overview, base)
		}
		if !includeAccounts {
			return cloneWarmPoolStatsShallow(overview), true, nil
		}
	}
	return s.buildOpenAIWarmPoolStatsFromRealtimeRaw(ctx, pool, groupID, includeAccounts, stateFilter, accountsOnly, base)
}
