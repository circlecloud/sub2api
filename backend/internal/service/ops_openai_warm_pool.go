package service

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

type openAIWarmBucketRuntimeSnapshot struct {
	groupID      int64
	lastAccessAt *time.Time
	lastRefillAt *time.Time
}

type openAIWarmAccountInspection struct {
	Ready             bool
	Expired           bool
	Cooling           bool
	Probing           bool
	NetworkError      bool
	VerifiedAt        *time.Time
	ExpiresAt         *time.Time
	FailUntil         *time.Time
	NetworkErrorAt    *time.Time
	NetworkErrorUntil *time.Time
}

var (
	ErrOpenAIWarmPoolUnavailable       = infraerrors.ServiceUnavailable("OPENAI_WARM_POOL_UNAVAILABLE", "OpenAI warm pool service is not available")
	ErrOpenAIWarmPoolDisabled          = infraerrors.BadRequest("OPENAI_WARM_POOL_DISABLED", "OpenAI warm pool is disabled")
	ErrOpenAIWarmPoolReaderUnavailable = infraerrors.ServiceUnavailable("OPENAI_WARM_POOL_READER_UNAVAILABLE", "OpenAI warm pool usage reader is not ready")
)

func buildOpsWarmPoolStatsCacheKey(groupID *int64, includeAccounts bool, accountStateFilter string, accountsOnly bool, readerReady bool, bootstrapping bool, revision int64) string {
	group := int64(0)
	if groupID != nil && *groupID > 0 {
		group = *groupID
	}
	return strings.Join([]string{
		strconv.FormatInt(group, 10),
		strconv.FormatBool(includeAccounts),
		strings.ToLower(strings.TrimSpace(accountStateFilter)),
		strconv.FormatBool(accountsOnly),
		strconv.FormatBool(readerReady),
		strconv.FormatBool(bootstrapping),
		strconv.FormatInt(revision, 10),
	}, "|")
}

func (s *OpsService) getCachedWarmPoolStats(key string) (*OpsOpenAIWarmPoolStats, bool) {
	if s == nil || key == "" {
		return nil, false
	}
	return s.realtimeWarmPoolCache.get(key, time.Now())
}

func (s *OpsService) setCachedWarmPoolStats(key string, stats *OpsOpenAIWarmPoolStats) {
	if s == nil || key == "" || stats == nil {
		return
	}
	s.realtimeWarmPoolCache.set(key, stats, opsRealtimeResultCacheTTL, time.Now())
}

func (s *OpsService) invalidateCachedWarmPoolStats(ctx context.Context) {
	if s == nil {
		return
	}
	s.realtimeWarmPoolCache.clear()
	if s.opsRealtimeCache != nil {
		_ = s.opsRealtimeCache.DeleteWarmPoolOverviewSnapshot(ctx)
	}
}

func (s *OpsService) TriggerOpenAIWarmPoolGlobalRefill(ctx context.Context) error {
	if s == nil {
		return ErrOpenAIWarmPoolUnavailable
	}
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return err
	}
	if s.openAIGatewayService == nil {
		return ErrOpenAIWarmPoolUnavailable
	}
	pool := s.openAIGatewayService.getOpenAIWarmPool()
	if pool == nil {
		return ErrOpenAIWarmPoolUnavailable
	}
	if !pool.config().Enabled {
		return ErrOpenAIWarmPoolDisabled
	}
	if pool.getUsageReader() == nil {
		return ErrOpenAIWarmPoolReaderUnavailable
	}
	if err := pool.triggerManualGlobalRefill(ctx); err != nil {
		return err
	}
	s.invalidateCachedWarmPoolStats(ctx)
	return nil
}

func (s *OpsService) GetOpenAIWarmPoolStats(ctx context.Context, groupID *int64) (*OpsOpenAIWarmPoolStats, error) {
	return s.GetOpenAIWarmPoolStatsWithOptions(ctx, groupID, groupID != nil, "", false)
}

func (s *OpsService) GetOpenAIWarmPoolStatsWithOptions(ctx context.Context, groupID *int64, includeAccounts bool, accountStateFilter string, accountsOnly bool) (*OpsOpenAIWarmPoolStats, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	stats := &OpsOpenAIWarmPoolStats{
		Enabled:          s.IsRealtimeMonitoringEnabled(ctx),
		Timestamp:        &now,
		Summary:          &OpsOpenAIWarmPoolSummary{},
		Buckets:          []*OpsOpenAIWarmPoolBucket{},
		Accounts:         []*OpsOpenAIWarmPoolAccount{},
		NetworkErrorPool: &OpsOpenAIWarmPoolNetworkErrorPool{},
	}
	if !stats.Enabled {
		return stats, nil
	}
	if s == nil || s.openAIGatewayService == nil {
		return stats, nil
	}
	pool := s.openAIGatewayService.getOpenAIWarmPool()
	if pool == nil {
		return stats, nil
	}

	cfg := pool.config()
	stats.WarmPoolEnabled = cfg.Enabled
	stats.ReaderReady = pool.getUsageReader() != nil
	stats.Bootstrapping = pool.isStartupBootstrapping()
	if !cfg.Enabled {
		stats.NetworkErrorPool = &OpsOpenAIWarmPoolNetworkErrorPool{Capacity: cfg.NetworkErrorPoolSize}
		return stats, nil
	}
	stateFilter := normalizeWarmPoolStateFilter(accountStateFilter)
	cacheKey := buildOpsWarmPoolStatsCacheKey(groupID, includeAccounts, stateFilter, accountsOnly, stats.ReaderReady, stats.Bootstrapping, pool.opsWarmPoolStatsCacheRevision())
	if includeAccounts && accountsOnly && stateFilter == "ready" {
		if cached, ok := s.getCachedWarmPoolStats(cacheKey); ok {
			return cached, nil
		}
	}
	if cachedStats, hit, err := s.getOpenAIWarmPoolStatsFromRealtimeCache(ctx, pool, groupID, includeAccounts, accountStateFilter, accountsOnly, stats); err != nil {
		s.logRealtimeCacheMiss("ops_openai_warm_pool", err)
	} else if hit {
		if includeAccounts && accountsOnly && stateFilter == "ready" {
			s.setCachedWarmPoolStats(cacheKey, cachedStats)
		}
		return cachedStats, nil
	}

	normalizedGroupID := pool.normalizeGroupID(groupID)

	if includeAccounts && accountsOnly && stateFilter == "ready" {
		return s.buildOpenAIWarmPoolReadyListStats(ctx, pool, groupID, 0, 0)
	}
	if !includeAccounts {
		if cached, ok := s.getCachedWarmPoolStats(cacheKey); ok {
			return cached, nil
		}
	}

	bucketSnapshots := pool.collectActiveOpsBuckets(now, cfg.ActiveBucketTTL)
	stats.NetworkErrorPool = pool.buildOpsWarmPoolNetworkErrorPool(now)
	globalCoverage := openAIWarmPoolGlobalCoverageSnapshot{}
	stats.Summary = pool.buildOpsWarmPoolSummary(now, cfg.ActiveBucketTTL, globalCoverage)

	if !includeAccounts {
		value, err, _ := s.realtimeSnapshotFlight.Do("ops_warm_pool:"+cacheKey, func() (any, error) {
			if cached, ok := s.getCachedWarmPoolStats(cacheKey); ok {
				return cached, nil
			}
			computed := stats
			var (
				coverageAccounts []Account
				err              error
			)
			if groupID != nil {
				accounts, err := s.openAIGatewayService.listSchedulableAccounts(ctx, pool.groupIDPointer(normalizedGroupID))
				if err != nil {
					return nil, err
				}
				inspectionCache := buildOpenAIWarmPoolInspectionCache(pool, now, accounts)
				bucketState, _ := pool.bucketStates.Load(normalizedGroupID)
				bucket, _ := bucketState.(*openAIWarmPoolBucketState)
				bucketStats, _ := buildOpenAIWarmPoolBucketStats(pool, normalizedGroupID, bucket, accounts, now, inspectionCache, false, stateFilter)
				computed.Buckets = []*OpsOpenAIWarmPoolBucket{bucketStats}
				coverageAccounts, err = pool.listGlobalRefillAccounts(ctx)
				if err != nil {
					return nil, err
				}
			} else {
				coverageAccounts, err = pool.listGlobalRefillAccounts(ctx)
				if err != nil {
					return nil, err
				}
				inspectionCache := buildOpenAIWarmPoolInspectionCache(pool, now, coverageAccounts)
				accountsByBucket := groupSchedulableOpenAIAccountsForWarmPool(pool, coverageAccounts)
				computed.Buckets = make([]*OpsOpenAIWarmPoolBucket, 0, len(bucketSnapshots))
				for _, bucketSnapshot := range bucketSnapshots {
					if bucketSnapshot == nil {
						continue
					}
					bucketState, _ := pool.bucketStates.Load(bucketSnapshot.groupID)
					bucket, _ := bucketState.(*openAIWarmPoolBucketState)
					computed.Buckets = append(computed.Buckets, buildOpenAIWarmPoolBucketStatsWithoutAccounts(pool, bucketSnapshot, bucket, accountsByBucket[bucketSnapshot.groupID], now, inspectionCache))
				}
			}
			computedCoverage := buildOpenAIWarmPoolCoverageFromAccounts(pool, now, bucketSnapshots, coverageAccounts)
			computed.Summary = pool.buildOpsWarmPoolSummary(now, cfg.ActiveBucketTTL, computedCoverage)
			computed.GlobalCoverages = buildOpenAIWarmPoolGroupCoverageRowsFromBuckets(computedCoverage, computed.Buckets, cfg)
			s.setCachedWarmPoolStats(cacheKey, computed)
			return computed, nil
		})
		if err != nil {
			return nil, err
		}
		cached, _ := value.(*OpsOpenAIWarmPoolStats)
		return cached, nil
	}

	if includeAccounts && accountsOnly {
		var (
			coverageAccounts []Account
			err              error
		)
		if groupID != nil {
			accountRows, err := s.buildOpenAIWarmPoolAccountRowsOnly(ctx, pool, groupID, stateFilter)
			if err != nil {
				return nil, err
			}
			stats.Accounts = accountRows
			coverageAccounts, err = pool.listGlobalRefillAccounts(ctx)
			if err != nil {
				return nil, err
			}
		} else {
			coverageAccounts, err = s.openAIGatewayService.listAllSchedulableOpenAIAccounts(ctx)
			if err != nil {
				return nil, err
			}
			inspectionCache := buildOpenAIWarmPoolInspectionCache(pool, now, coverageAccounts)
			stats.Accounts = buildOpenAIWarmPoolAccountRows(pool, coverageAccounts, now, inspectionCache, stateFilter)
		}
		globalCoverage = buildOpenAIWarmPoolCoverageFromAccounts(pool, now, bucketSnapshots, coverageAccounts)
		stats.Summary = pool.buildOpsWarmPoolSummary(now, cfg.ActiveBucketTTL, globalCoverage)
		stats.GlobalCoverages = buildOpenAIWarmPoolGroupCoverageRowsFromBuckets(globalCoverage, nil, cfg)
		return stats, nil
	}
	if groupID != nil {
		accounts, err := s.openAIGatewayService.listSchedulableAccounts(ctx, pool.groupIDPointer(normalizedGroupID))
		if err != nil {
			return nil, err
		}
		inspectionCache := buildOpenAIWarmPoolInspectionCache(pool, now, accounts)
		bucketState, _ := pool.bucketStates.Load(normalizedGroupID)
		bucket, _ := bucketState.(*openAIWarmPoolBucketState)
		bucketStats, accountRows := buildOpenAIWarmPoolBucketStats(pool, normalizedGroupID, bucket, accounts, now, inspectionCache, includeAccounts, stateFilter)
		stats.Buckets = []*OpsOpenAIWarmPoolBucket{bucketStats}
		stats.Accounts = accountRows
		coverageAccounts, err := pool.listGlobalRefillAccounts(ctx)
		if err != nil {
			return nil, err
		}
		globalCoverage = buildOpenAIWarmPoolCoverageFromAccounts(pool, now, bucketSnapshots, coverageAccounts)
		stats.Summary = pool.buildOpsWarmPoolSummary(now, cfg.ActiveBucketTTL, globalCoverage)
		stats.GlobalCoverages = buildOpenAIWarmPoolGroupCoverageRowsFromBuckets(globalCoverage, stats.Buckets, cfg)
		return stats, nil
	}

	allAccounts, err := s.openAIGatewayService.listAllSchedulableOpenAIAccounts(ctx)
	if err != nil {
		return nil, err
	}
	inspectionCache := buildOpenAIWarmPoolInspectionCache(pool, now, allAccounts)
	accountsByBucket := groupSchedulableOpenAIAccountsForWarmPool(pool, allAccounts)
	for _, bucketSnapshot := range bucketSnapshots {
		bucketState, _ := pool.bucketStates.Load(bucketSnapshot.groupID)
		bucket, _ := bucketState.(*openAIWarmPoolBucketState)
		bucketStats, _ := buildOpenAIWarmPoolBucketStats(pool, bucketSnapshot.groupID, bucket, accountsByBucket[bucketSnapshot.groupID], now, inspectionCache, false, "")
		stats.Buckets = append(stats.Buckets, bucketStats)
	}
	stats.Accounts = buildOpenAIWarmPoolAccountRows(pool, allAccounts, now, inspectionCache, stateFilter)
	globalCoverage = buildOpenAIWarmPoolCoverageFromAccounts(pool, now, bucketSnapshots, allAccounts)
	stats.Summary = pool.buildOpsWarmPoolSummary(now, cfg.ActiveBucketTTL, globalCoverage)
	stats.GlobalCoverages = buildOpenAIWarmPoolGroupCoverageRowsFromBuckets(globalCoverage, stats.Buckets, cfg)
	return stats, nil
}

func (p *openAIAccountWarmPoolService) buildOpsWarmPoolSummary(now time.Time, activeBucketTTL time.Duration, globalCoverage openAIWarmPoolGlobalCoverageSnapshot) *OpsOpenAIWarmPoolSummary {
	summary := &OpsOpenAIWarmPoolSummary{}
	if p == nil {
		return summary
	}
	readyAccountCount := 0
	p.accountStates.Range(func(_, value any) bool {
		state, _ := value.(*openAIWarmAccountState)
		if state == nil {
			return true
		}
		inspection := state.inspect(now)
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
		if inspection.Ready {
			readyAccountCount++
		}
		return true
	})
	summary.GlobalReadyAccountCount = globalCoverage.uniqueReadyCount
	if summary.GlobalReadyAccountCount == 0 && len(globalCoverage.activeGroupIDs) == 0 && p.startupBootstrapDone.Load() {
		summary.GlobalReadyAccountCount = readyAccountCount
	}
	summary.ActiveGroupCount = len(globalCoverage.activeGroupIDs)
	cfg := p.config()
	summary.GlobalTargetPerActiveGroup = cfg.GlobalTargetSize
	summary.GlobalRefillPerActiveGroup = cfg.GlobalRefillBelow
	for _, groupID := range globalCoverage.activeGroupIDs {
		if globalCoverage.coverageByGroup[groupID] < cfg.GlobalTargetSize {
			summary.GroupsBelowTargetCount++
		}
		if globalCoverage.coverageByGroup[groupID] < cfg.GlobalRefillBelow {
			summary.GroupsBelowRefillCount++
		}
	}
	for _, bucketSnapshot := range p.collectActiveOpsBuckets(now, activeBucketTTL) {
		bucketState, _ := p.bucketStates.Load(bucketSnapshot.groupID)
		bucket, _ := bucketState.(*openAIWarmPoolBucketState)
		if bucket == nil {
			continue
		}
		summary.BucketReadyAccountCount += bucket.readyCount()
	}
	summary.TakeCount = p.totalTakeCount.Load()
	if cfg.NetworkErrorPoolSize > 0 && summary.NetworkErrorPoolCount >= cfg.NetworkErrorPoolSize {
		summary.NetworkErrorPoolFull = true
	}
	if last := p.lastBucketMaintenance.Load(); last > 0 {
		ts := time.Unix(0, last).UTC()
		summary.LastBucketMaintenanceAt = &ts
	}
	if last := p.lastGlobalMaintenance.Load(); last > 0 {
		ts := time.Unix(0, last).UTC()
		summary.LastGlobalMaintenanceAt = &ts
	}
	return summary
}

//nolint:unused // 预留给后续 warm pool coverage 视图接线
func buildOpenAIWarmPoolGroupCoverageRows(pool *openAIAccountWarmPoolService, coverage openAIWarmPoolGlobalCoverageSnapshot, accounts []Account, cfg openAIWarmPoolConfigView) []*OpsOpenAIWarmPoolGroupCoverage {
	rows := make([]*OpsOpenAIWarmPoolGroupCoverage, 0, len(coverage.activeGroupIDs))
	for _, groupID := range coverage.activeGroupIDs {
		rows = append(rows, &OpsOpenAIWarmPoolGroupCoverage{
			GroupID:       groupID,
			GroupName:     resolveWarmPoolCoverageGroupName(accounts, groupID),
			CoverageCount: coverage.coverageByGroup[groupID],
			TargetSize:    cfg.GlobalTargetSize,
			RefillBelow:   cfg.GlobalRefillBelow,
		})
	}
	return rows
}

func buildOpenAIWarmPoolCoverageFromAccounts(pool *openAIAccountWarmPoolService, now time.Time, bucketSnapshots []*openAIWarmBucketRuntimeSnapshot, accounts []Account) openAIWarmPoolGlobalCoverageSnapshot {
	coverage := openAIWarmPoolGlobalCoverageSnapshot{
		activeGroupIDs:  make([]int64, 0, len(bucketSnapshots)),
		activeGroupSet:  make(map[int64]struct{}, len(bucketSnapshots)),
		coverageByGroup: make(map[int64]int, len(bucketSnapshots)),
	}
	for _, bucketSnapshot := range bucketSnapshots {
		if bucketSnapshot == nil {
			continue
		}
		groupID := bucketSnapshot.groupID
		if _, exists := coverage.activeGroupSet[groupID]; exists {
			continue
		}
		coverage.activeGroupSet[groupID] = struct{}{}
		coverage.activeGroupIDs = append(coverage.activeGroupIDs, groupID)
		coverage.coverageByGroup[groupID] = 0
	}
	if pool == nil || len(coverage.activeGroupIDs) == 0 {
		return coverage
	}
	return pool.buildGlobalCoverageSnapshot(now, accounts, coverage.activeGroupIDs)
}

func buildOpenAIWarmPoolGroupCoverageRowsFromBuckets(coverage openAIWarmPoolGlobalCoverageSnapshot, bucketRows []*OpsOpenAIWarmPoolBucket, cfg openAIWarmPoolConfigView) []*OpsOpenAIWarmPoolGroupCoverage {
	nameByGroupID := make(map[int64]string, len(bucketRows))
	for _, row := range bucketRows {
		if row == nil {
			continue
		}
		if strings.TrimSpace(row.GroupName) != "" {
			nameByGroupID[row.GroupID] = row.GroupName
		}
	}
	rows := make([]*OpsOpenAIWarmPoolGroupCoverage, 0, len(coverage.activeGroupIDs))
	for _, groupID := range coverage.activeGroupIDs {
		rows = append(rows, &OpsOpenAIWarmPoolGroupCoverage{
			GroupID:       groupID,
			GroupName:     strings.TrimSpace(nameByGroupID[groupID]),
			CoverageCount: coverage.coverageByGroup[groupID],
			TargetSize:    cfg.GlobalTargetSize,
			RefillBelow:   cfg.GlobalRefillBelow,
		})
	}
	return rows
}

//nolint:unused // 预留给后续 warm pool coverage 视图接线
func resolveWarmPoolCoverageGroupName(accounts []Account, groupID int64) string {
	for i := range accounts {
		name := resolveWarmPoolGroupName(&accounts[i], groupID)
		if strings.TrimSpace(name) != "" {
			return name
		}
	}
	return ""
}

func (p *openAIAccountWarmPoolService) buildOpsWarmPoolNetworkErrorPool(now time.Time) *OpsOpenAIWarmPoolNetworkErrorPool {
	pool := &OpsOpenAIWarmPoolNetworkErrorPool{}
	if p == nil {
		return pool
	}
	pool.Capacity = p.config().NetworkErrorPoolSize
	var oldest *time.Time
	p.accountStates.Range(func(_, value any) bool {
		state, _ := value.(*openAIWarmAccountState)
		if state == nil {
			return true
		}
		inspection := state.inspect(now)
		if !inspection.NetworkError {
			return true
		}
		pool.Count++
		if inspection.NetworkErrorAt != nil {
			if oldest == nil || inspection.NetworkErrorAt.Before(*oldest) {
				oldestCopy := inspection.NetworkErrorAt.UTC()
				oldest = &oldestCopy
			}
		}
		return true
	})
	pool.OldestEnteredAt = oldest
	return pool
}

func (p *openAIAccountWarmPoolService) collectActiveOpsBuckets(now time.Time, activeBucketTTL time.Duration) []*openAIWarmBucketRuntimeSnapshot {
	if p == nil {
		return nil
	}
	buckets := make([]*openAIWarmBucketRuntimeSnapshot, 0, 8)
	seen := make(map[int64]struct{}, 8)
	appendBucket := func(item *openAIWarmBucketRuntimeSnapshot) {
		if item == nil || item.groupID <= 0 {
			return
		}
		if _, exists := seen[item.groupID]; exists {
			return
		}
		seen[item.groupID] = struct{}{}
		buckets = append(buckets, item)
	}
	p.bucketStates.Range(func(_, value any) bool {
		bucket, _ := value.(*openAIWarmPoolBucketState)
		if !p.shouldRetainOpsBucket(bucket, now, activeBucketTTL) {
			return true
		}
		item := &openAIWarmBucketRuntimeSnapshot{groupID: bucket.groupID}
		if lastAccessUnix := bucket.lastAccess.Load(); lastAccessUnix > 0 {
			item.lastAccessAt = timePtrUTC(time.Unix(0, lastAccessUnix))
		}
		if lastRefillUnix := bucket.lastRefill.Load(); lastRefillUnix > 0 {
			item.lastRefillAt = timePtrUTC(time.Unix(0, lastRefillUnix))
		}
		appendBucket(item)
		return true
	})
	startupLastRefillAt := p.startupOpsLastRefillAt()
	for _, groupID := range p.startupOpsGroupIDs() {
		appendBucket(&openAIWarmBucketRuntimeSnapshot{
			groupID:      groupID,
			lastRefillAt: cloneTimePtr(startupLastRefillAt),
		})
	}
	sort.SliceStable(buckets, func(i, j int) bool {
		left := buckets[i]
		right := buckets[j]
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
	return buckets
}

func buildOpenAIWarmPoolInspectionCache(pool *openAIAccountWarmPoolService, now time.Time, accounts []Account) map[int64]openAIWarmAccountInspection {
	cache := make(map[int64]openAIWarmAccountInspection, len(accounts))
	if pool == nil {
		return cache
	}
	for i := range accounts {
		acc := &accounts[i]
		if acc == nil || acc.ID <= 0 {
			continue
		}
		if _, exists := cache[acc.ID]; exists {
			continue
		}
		cache[acc.ID] = pool.accountState(acc.ID).inspect(now)
	}
	return cache
}

func lookupOpenAIWarmPoolInspection(pool *openAIAccountWarmPoolService, now time.Time, cache map[int64]openAIWarmAccountInspection, accountID int64) openAIWarmAccountInspection {
	if cache != nil {
		if inspection, exists := cache[accountID]; exists {
			return inspection
		}
	}
	inspection := pool.accountState(accountID).inspect(now)
	if cache != nil {
		cache[accountID] = inspection
	}
	return inspection
}

func countBucketWarmReadyReadonly(pool *openAIAccountWarmPoolService, now time.Time, bucket *openAIWarmPoolBucketState, accountIDs map[int64]struct{}, inspectionCache map[int64]openAIWarmAccountInspection) int {
	if pool == nil || bucket == nil {
		return 0
	}
	count := 0
	for _, accountID := range bucket.readyIDs() {
		if accountIDs != nil {
			if _, exists := accountIDs[accountID]; !exists {
				continue
			}
		}
		inspection := lookupOpenAIWarmPoolInspection(pool, now, inspectionCache, accountID)
		if inspection.Cooling || inspection.NetworkError || !inspection.Ready || inspection.Expired {
			continue
		}
		count++
	}
	return count
}

func buildOpenAIWarmPoolBucketStatsWithoutAccounts(
	pool *openAIAccountWarmPoolService,
	bucketSnapshot *openAIWarmBucketRuntimeSnapshot,
	bucket *openAIWarmPoolBucketState,
	accounts []Account,
	now time.Time,
	inspectionCache map[int64]openAIWarmAccountInspection,
) *OpsOpenAIWarmPoolBucket {
	cfg := pool.config()
	out := &OpsOpenAIWarmPoolBucket{
		GroupID:           bucketSnapshot.groupID,
		BucketTargetSize:  cfg.BucketTargetSize,
		BucketRefillBelow: cfg.BucketRefillBelow,
		LastAccessAt:      bucketSnapshot.lastAccessAt,
		LastRefillAt:      bucketSnapshot.lastRefillAt,
	}
	accountIDs := make(map[int64]struct{}, len(accounts))
	for i := range accounts {
		acc := &accounts[i]
		if acc == nil || !acc.IsOpenAI() {
			continue
		}
		accountIDs[acc.ID] = struct{}{}
		out.SchedulableAccounts++
		if out.GroupName == "" {
			out.GroupName = resolveWarmPoolGroupName(acc, bucketSnapshot.groupID)
		}
		inspection := lookupOpenAIWarmPoolInspection(pool, now, inspectionCache, acc.ID)
		if inspection.Probing {
			out.ProbingAccounts++
		}
		if inspection.Cooling {
			out.CoolingAccounts++
		}
	}
	if bucket != nil {
		out.TakeCount = bucket.takeCount.Load()
		out.BucketReadyAccounts = countBucketWarmReadyReadonly(pool, now, bucket, accountIDs, inspectionCache)
	}
	return out
}

func buildOpenAIWarmPoolBucketStats(
	pool *openAIAccountWarmPoolService,
	groupID int64,
	bucket *openAIWarmPoolBucketState,
	accounts []Account,
	now time.Time,
	inspectionCache map[int64]openAIWarmAccountInspection,
	includeAccounts bool,
	accountStateFilter string,
) (*OpsOpenAIWarmPoolBucket, []*OpsOpenAIWarmPoolAccount) {
	cfg := pool.config()
	out := &OpsOpenAIWarmPoolBucket{
		GroupID:           groupID,
		BucketTargetSize:  cfg.BucketTargetSize,
		BucketRefillBelow: cfg.BucketRefillBelow,
	}
	if bucket != nil {
		out.TakeCount = bucket.takeCount.Load()
		if lastAccessUnix := bucket.lastAccess.Load(); lastAccessUnix > 0 {
			out.LastAccessAt = timePtrUTC(time.Unix(0, lastAccessUnix))
		}
		if lastRefillUnix := bucket.lastRefill.Load(); lastRefillUnix > 0 {
			out.LastRefillAt = timePtrUTC(time.Unix(0, lastRefillUnix))
		}
	}
	accountRows := make([]*OpsOpenAIWarmPoolAccount, 0, len(accounts))
	accountIDs := make(map[int64]struct{}, len(accounts))
	for i := range accounts {
		acc := &accounts[i]
		if acc == nil || !acc.IsOpenAI() {
			continue
		}
		accountIDs[acc.ID] = struct{}{}
		out.SchedulableAccounts++
		if out.GroupName == "" {
			out.GroupName = resolveWarmPoolGroupName(acc, groupID)
		}
		inspection := lookupOpenAIWarmPoolInspection(pool, now, inspectionCache, acc.ID)
		if inspection.Probing {
			out.ProbingAccounts++
		}
		if inspection.Cooling {
			out.CoolingAccounts++
		}
		if includeAccounts {
			accountRow := buildOpenAIWarmPoolAccountRow(pool, acc, inspection)
			if warmPoolStateMatchesFilter(accountRow.State, accountStateFilter) {
				accountRows = append(accountRows, accountRow)
			}
		}
	}
	out.BucketReadyAccounts = countBucketWarmReadyReadonly(pool, now, bucket, accountIDs, inspectionCache)
	if includeAccounts {
		sortWarmPoolAccountRows(accountRows)
	}
	return out, accountRows
}

func buildOpenAIWarmPoolAccountRows(pool *openAIAccountWarmPoolService, accounts []Account, now time.Time, inspectionCache map[int64]openAIWarmAccountInspection, accountStateFilter string) []*OpsOpenAIWarmPoolAccount {
	rows := make([]*OpsOpenAIWarmPoolAccount, 0, len(accounts))
	seen := make(map[int64]struct{}, len(accounts))
	for i := range accounts {
		acc := &accounts[i]
		if acc == nil || !acc.IsOpenAI() || !acc.IsSchedulable() {
			continue
		}
		if _, exists := seen[acc.ID]; exists {
			continue
		}
		seen[acc.ID] = struct{}{}
		inspection := lookupOpenAIWarmPoolInspection(pool, now, inspectionCache, acc.ID)
		accountRow := buildOpenAIWarmPoolAccountRow(pool, acc, inspection)
		if !warmPoolStateMatchesFilter(accountRow.State, accountStateFilter) {
			continue
		}
		rows = append(rows, accountRow)
	}
	sortWarmPoolAccountRows(rows)
	return rows
}

func (s *OpsService) loadOpenAIWarmPoolReadyAccounts(ctx context.Context, pool *openAIAccountWarmPoolService, normalizedGroupID int64, now time.Time) ([]Account, error) {
	if pool == nil {
		return []Account{}, nil
	}
	if s == nil || s.openAIGatewayService == nil {
		return []Account{}, nil
	}
	if s.openAIGatewayService.accountRepo == nil {
		var accounts []Account
		var err error
		if normalizedGroupID > 0 {
			accounts, err = s.openAIGatewayService.listSchedulableAccounts(ctx, pool.groupIDPointer(normalizedGroupID))
		} else {
			accounts, err = s.openAIGatewayService.listAllSchedulableOpenAIAccounts(ctx)
		}
		if err != nil {
			return nil, err
		}
		inspectionCache := buildOpenAIWarmPoolInspectionCache(pool, now, accounts)
		readyAccounts := make([]Account, 0, len(accounts))
		for i := range accounts {
			acc := &accounts[i]
			if acc == nil || !acc.IsOpenAI() || !acc.IsSchedulable() {
				continue
			}
			inspection := lookupOpenAIWarmPoolInspection(pool, now, inspectionCache, acc.ID)
			if !warmPoolReadyUsable(inspection) {
				continue
			}
			if normalizedGroupID > 0 && !warmPoolAccountBelongsToBucket(pool, acc, normalizedGroupID) {
				continue
			}
			readyAccounts = append(readyAccounts, *acc)
		}
		return readyAccounts, nil
	}
	ids := collectWarmPoolTrackedAccountIDsByState(pool, now, "ready")
	if len(ids) == 0 {
		return []Account{}, nil
	}
	accounts, err := s.openAIGatewayService.accountRepo.GetByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	readyAccounts := make([]Account, 0, len(accounts))
	for _, acc := range accounts {
		if acc == nil || !acc.IsOpenAI() || !acc.IsSchedulable() {
			continue
		}
		if normalizedGroupID > 0 && !warmPoolAccountBelongsToBucket(pool, acc, normalizedGroupID) {
			continue
		}
		readyAccounts = append(readyAccounts, *acc)
	}
	return readyAccounts, nil
}

func (s *OpsService) buildOpenAIWarmPoolAccountRowsOnly(ctx context.Context, pool *openAIAccountWarmPoolService, groupID *int64, accountStateFilter string) ([]*OpsOpenAIWarmPoolAccount, error) {
	if pool == nil {
		return []*OpsOpenAIWarmPoolAccount{}, nil
	}
	if s == nil || s.openAIGatewayService == nil {
		return []*OpsOpenAIWarmPoolAccount{}, nil
	}
	stateFilter := normalizeWarmPoolStateFilter(accountStateFilter)
	normalizedGroupID := pool.normalizeGroupID(groupID)
	if stateFilter == "ready" {
		stats, err := s.buildOpenAIWarmPoolReadyListStats(ctx, pool, groupID, 0, 0)
		if err != nil {
			return nil, err
		}
		return stats.Accounts, nil
	}
	if stateFilter == "" || stateFilter == "idle" {
		if groupID != nil {
			accounts, err := s.openAIGatewayService.listSchedulableAccounts(ctx, groupID)
			if err != nil {
				return nil, err
			}
			now := time.Now()
			inspectionCache := buildOpenAIWarmPoolInspectionCache(pool, now, accounts)
			return buildOpenAIWarmPoolAccountRows(pool, accounts, now, inspectionCache, stateFilter), nil
		}
		allAccounts, err := s.openAIGatewayService.listAllSchedulableOpenAIAccounts(ctx)
		if err != nil {
			return nil, err
		}
		now := time.Now()
		inspectionCache := buildOpenAIWarmPoolInspectionCache(pool, now, allAccounts)
		return buildOpenAIWarmPoolAccountRows(pool, allAccounts, now, inspectionCache, stateFilter), nil
	}
	if s == nil || s.openAIGatewayService == nil || s.openAIGatewayService.accountRepo == nil {
		return []*OpsOpenAIWarmPoolAccount{}, nil
	}

	ids := collectWarmPoolTrackedAccountIDsByState(pool, time.Now(), stateFilter)
	if len(ids) == 0 {
		return []*OpsOpenAIWarmPoolAccount{}, nil
	}
	accounts, err := s.openAIGatewayService.accountRepo.GetByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}

	rows := make([]*OpsOpenAIWarmPoolAccount, 0, len(accounts))
	now := time.Now()
	for _, acc := range accounts {
		if acc == nil || !acc.IsOpenAI() || !acc.IsSchedulable() {
			continue
		}
		if groupID != nil && !warmPoolAccountBelongsToBucket(pool, acc, normalizedGroupID) {
			continue
		}
		inspection := pool.accountState(acc.ID).inspect(now)
		accountRow := buildOpenAIWarmPoolAccountRow(pool, acc, inspection)
		if !warmPoolStateMatchesFilter(accountRow.State, stateFilter) {
			continue
		}
		rows = append(rows, accountRow)
	}
	sortWarmPoolAccountRows(rows)
	return rows, nil
}

func collectWarmPoolTrackedAccountIDsByState(pool *openAIAccountWarmPoolService, now time.Time, accountStateFilter string) []int64 {
	if pool == nil {
		return nil
	}
	filter := normalizeWarmPoolStateFilter(accountStateFilter)
	ids := make([]int64, 0, 16)
	pool.accountStates.Range(func(key, value any) bool {
		accountID, _ := key.(int64)
		state, _ := value.(*openAIWarmAccountState)
		if accountID <= 0 || state == nil {
			return true
		}
		if warmPoolStateMatchesFilter(warmPoolStateLabel(state.inspect(now)), filter) {
			ids = append(ids, accountID)
		}
		return true
	})
	return ids
}

func warmPoolAccountBelongsToBucket(pool *openAIAccountWarmPoolService, account *Account, groupID int64) bool {
	for _, bucketID := range warmPoolBucketIDsForAccount(pool, account) {
		if bucketID == groupID {
			return true
		}
	}
	return false
}

func buildOpenAIWarmPoolAccountRow(pool *openAIAccountWarmPoolService, acc *Account, inspection openAIWarmAccountInspection) *OpsOpenAIWarmPoolAccount {
	if acc == nil {
		return nil
	}
	return &OpsOpenAIWarmPoolAccount{
		AccountID:         acc.ID,
		AccountName:       acc.Name,
		Platform:          acc.Platform,
		Schedulable:       acc.IsSchedulable(),
		Priority:          acc.Priority,
		Concurrency:       acc.Concurrency,
		State:             warmPoolStateLabel(inspection),
		Groups:            warmPoolBucketRefsForAccount(pool, acc),
		VerifiedAt:        inspection.VerifiedAt,
		ExpiresAt:         inspection.ExpiresAt,
		FailUntil:         inspection.FailUntil,
		NetworkErrorAt:    inspection.NetworkErrorAt,
		NetworkErrorUntil: inspection.NetworkErrorUntil,
	}
}

func sortWarmPoolAccountRows(rows []*OpsOpenAIWarmPoolAccount) {
	sort.SliceStable(rows, func(i, j int) bool {
		left := rows[i]
		right := rows[j]
		if rankWarmPoolState(left.State) != rankWarmPoolState(right.State) {
			return rankWarmPoolState(left.State) < rankWarmPoolState(right.State)
		}
		if left.Priority != right.Priority {
			return left.Priority < right.Priority
		}
		return left.AccountID < right.AccountID
	})
}

func groupSchedulableOpenAIAccountsForWarmPool(pool *openAIAccountWarmPoolService, accounts []Account) map[int64][]Account {
	grouped := make(map[int64][]Account)
	for i := range accounts {
		account := accounts[i]
		if !account.IsOpenAI() || !account.IsSchedulable() {
			continue
		}
		bucketIDs := warmPoolBucketIDsForAccount(pool, &account)
		for _, bucketID := range bucketIDs {
			grouped[bucketID] = append(grouped[bucketID], account)
		}
	}
	return grouped
}

func warmPoolBucketIDsForAccount(pool *openAIAccountWarmPoolService, account *Account) []int64 {
	if account == nil {
		return []int64{0}
	}
	if pool != nil && pool.service != nil && pool.service.cfg != nil && pool.service.cfg.RunMode == config.RunModeSimple {
		return []int64{0}
	}
	seen := make(map[int64]struct{}, len(account.Groups)+len(account.GroupIDs)+len(account.AccountGroups))
	bucketCap := len(account.Groups) + len(account.GroupIDs) + len(account.AccountGroups)
	if bucketCap <= 0 {
		bucketCap = 1
	}
	bucketIDs := make([]int64, 0, bucketCap)
	appendGroupID := func(groupID int64) {
		if groupID <= 0 {
			return
		}
		if _, exists := seen[groupID]; exists {
			return
		}
		seen[groupID] = struct{}{}
		bucketIDs = append(bucketIDs, groupID)
	}
	for _, group := range account.Groups {
		if group == nil {
			continue
		}
		appendGroupID(group.ID)
	}
	for _, groupID := range account.GroupIDs {
		appendGroupID(groupID)
	}
	for _, accountGroup := range account.AccountGroups {
		appendGroupID(accountGroup.GroupID)
	}
	if len(bucketIDs) == 0 {
		return []int64{0}
	}
	sort.Slice(bucketIDs, func(i, j int) bool { return bucketIDs[i] < bucketIDs[j] })
	return bucketIDs
}

func warmPoolStableBucketIndex(accountID int64, size int) int {
	if size <= 1 {
		return 0
	}
	u := uint64(accountID)
	u ^= u >> 33
	u *= 0xff51afd7ed558ccd
	u ^= u >> 33
	u *= 0xc4ceb9fe1a85ec53
	u ^= u >> 33
	return int(u % uint64(size))
}

func warmPoolBucketOwnerGroupID(pool *openAIAccountWarmPoolService, account *Account) int64 {
	bucketIDs := warmPoolBucketIDsForAccount(pool, account)
	if len(bucketIDs) == 0 {
		return 0
	}
	if len(bucketIDs) == 1 {
		return bucketIDs[0]
	}
	return bucketIDs[warmPoolStableBucketIndex(account.ID, len(bucketIDs))]
}

func warmPoolBucketRefsForAccount(pool *openAIAccountWarmPoolService, account *Account) []*OpsOpenAIWarmPoolBucketRef {
	bucketIDs := warmPoolBucketIDsForAccount(pool, account)
	refs := make([]*OpsOpenAIWarmPoolBucketRef, 0, len(bucketIDs))
	for _, bucketID := range bucketIDs {
		refs = append(refs, &OpsOpenAIWarmPoolBucketRef{
			GroupID:   bucketID,
			GroupName: resolveWarmPoolGroupName(account, bucketID),
		})
	}
	return refs
}

func resolveWarmPoolGroupName(account *Account, groupID int64) string {
	if account == nil || groupID <= 0 {
		return ""
	}
	for _, group := range account.Groups {
		if group == nil || group.ID != groupID {
			continue
		}
		return group.Name
	}
	return ""
}

func normalizeWarmPoolStateFilter(state string) string {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "ready", "probing", "cooling", "network_error", "idle":
		return strings.ToLower(strings.TrimSpace(state))
	default:
		return ""
	}
}

func warmPoolStateMatchesFilter(state, filter string) bool {
	filter = normalizeWarmPoolStateFilter(filter)
	if filter == "" {
		return true
	}
	return state == filter
}

func warmPoolReadyUsable(inspection openAIWarmAccountInspection) bool {
	return inspection.Ready && !inspection.Expired
}

func warmPoolStateLabel(inspection openAIWarmAccountInspection) string {
	switch {
	case warmPoolReadyUsable(inspection):
		return "ready"
	case inspection.Probing:
		return "probing"
	case inspection.Cooling:
		return "cooling"
	case inspection.NetworkError:
		return "network_error"
	default:
		return "idle"
	}
}

func rankWarmPoolState(state string) int {
	switch state {
	case "ready":
		return 0
	case "probing":
		return 1
	case "cooling":
		return 2
	case "network_error":
		return 3
	default:
		return 4
	}
}

func timePtrUTC(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	utc := t.UTC()
	return &utc
}
