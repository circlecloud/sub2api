package service

import (
	"context"
	"time"
)

// buildOpenAIWarmPoolReadyListStats is the shared ready-list provider used by
// ready-only and paginated ready queries.
func (s *OpsService) buildOpenAIWarmPoolReadyListStats(ctx context.Context, pool *openAIAccountWarmPoolService, groupID *int64, page, pageSize int) (*OpsOpenAIWarmPoolStats, error) {
	now := time.Now().UTC()
	enabled := false
	if s != nil {
		enabled = s.IsRealtimeMonitoringEnabled(ctx)
	}
	stats := &OpsOpenAIWarmPoolStats{
		Enabled:          enabled,
		Timestamp:        &now,
		Summary:          &OpsOpenAIWarmPoolSummary{},
		Buckets:          []*OpsOpenAIWarmPoolBucket{},
		Accounts:         []*OpsOpenAIWarmPoolAccount{},
		GlobalCoverages:  []*OpsOpenAIWarmPoolGroupCoverage{},
		NetworkErrorPool: &OpsOpenAIWarmPoolNetworkErrorPool{},
	}
	if !stats.Enabled || s == nil || s.openAIGatewayService == nil || pool == nil {
		if page > 0 || pageSize > 0 {
			return paginateOpsOpenAIWarmPoolStats(stats, page, pageSize), nil
		}
		return stats, nil
	}

	cfg := pool.config()
	stats.WarmPoolEnabled = cfg.Enabled
	stats.ReaderReady = pool.getUsageReader() != nil
	stats.Bootstrapping = pool.isStartupBootstrapping()
	if !cfg.Enabled {
		stats.NetworkErrorPool = &OpsOpenAIWarmPoolNetworkErrorPool{Capacity: cfg.NetworkErrorPoolSize}
		if page > 0 || pageSize > 0 {
			return paginateOpsOpenAIWarmPoolStats(stats, page, pageSize), nil
		}
		return stats, nil
	}

	stateFilter := "ready"
	cacheKey := buildOpsWarmPoolStatsCacheKey(groupID, true, stateFilter, true, stats.ReaderReady, stats.Bootstrapping, pool.opsWarmPoolStatsCacheRevision())
	if cached, ok := s.getCachedWarmPoolStats(cacheKey); ok {
		if page > 0 || pageSize > 0 {
			return paginateOpsOpenAIWarmPoolStats(cached, page, pageSize), nil
		}
		return cached, nil
	}

	value, err, _ := s.realtimeSnapshotFlight.Do("ops_warm_pool:"+cacheKey, func() (any, error) {
		if cached, ok := s.getCachedWarmPoolStats(cacheKey); ok {
			return cached, nil
		}

		computed := stats
		computed.NetworkErrorPool = pool.buildOpsWarmPoolNetworkErrorPool(now)

		readyAccounts, err := s.loadOpenAIWarmPoolReadyAccounts(ctx, pool, 0, now)
		if err != nil {
			return nil, err
		}
		filteredAccounts := readyAccounts
		normalizedGroupID := pool.normalizeGroupID(groupID)
		if normalizedGroupID > 0 {
			filteredAccounts = make([]Account, 0, len(readyAccounts))
			for i := range readyAccounts {
				acc := &readyAccounts[i]
				if warmPoolAccountBelongsToBucket(pool, acc, normalizedGroupID) {
					filteredAccounts = append(filteredAccounts, readyAccounts[i])
				}
			}
		}

		inspectionCache := buildOpenAIWarmPoolInspectionCache(pool, now, readyAccounts)
		computed.Accounts = buildOpenAIWarmPoolAccountRows(pool, filteredAccounts, now, inspectionCache, stateFilter)
		bucketSnapshots := pool.collectActiveOpsBuckets(now, cfg.ActiveBucketTTL)
		computedCoverage := buildOpenAIWarmPoolCoverageFromAccounts(pool, now, bucketSnapshots, readyAccounts)
		computed.Summary = pool.buildOpsWarmPoolSummary(now, cfg.ActiveBucketTTL, computedCoverage)
		computed.GlobalCoverages = buildOpenAIWarmPoolGroupCoverageRowsFromBuckets(computedCoverage, nil, cfg)
		s.setCachedWarmPoolStats(cacheKey, computed)
		return computed, nil
	})
	if err != nil {
		return nil, err
	}
	cached, _ := value.(*OpsOpenAIWarmPoolStats)
	if page > 0 || pageSize > 0 {
		return paginateOpsOpenAIWarmPoolStats(cached, page, pageSize), nil
	}
	return cached, nil
}
