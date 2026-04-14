package service

import (
	"context"
	"sort"
	"strings"
	"time"
)

type opsSharedRealtimeAccountsLoadResult struct {
	accounts  []Account
	hit       bool
	source    string
	sharedHit bool
}

func (s *OpsService) listAllAccountsForOpsFromRealtimeCache(ctx context.Context, platformFilter string, groupIDFilter *int64) ([]Account, bool, error) {
	result, err := s.loadSharedOpsRealtimeAccountsFromMirror(ctx, platformFilter, groupIDFilter)
	if err != nil {
		return nil, false, err
	}
	return result.accounts, result.hit, nil
}

func (s *OpsService) loadSharedOpsRealtimeAccountsFromMirror(ctx context.Context, platformFilter string, groupIDFilter *int64) (opsSharedRealtimeAccountsLoadResult, error) {
	cacheKey := buildOpsRealtimeAccountsCacheKey(platformFilter, groupIDFilter)
	start := time.Now()
	if cacheKey != "" {
		if cached, ok := s.getCachedRealtimeAccounts(cacheKey); ok {
			result := opsSharedRealtimeAccountsLoadResult{accounts: cached, hit: true, source: "shared_cache", sharedHit: true}
			s.logRealtimeSharedSnapshot("ops_accounts_mirror", cacheKey, result.sharedHit, result.source, len(result.accounts), time.Since(start))
			return result, nil
		}
		value, err, _ := s.realtimeSnapshotFlight.Do("ops_accounts_mirror:"+cacheKey, func() (any, error) {
			if cached, ok := s.getCachedRealtimeAccounts(cacheKey); ok {
				return opsSharedRealtimeAccountsLoadResult{accounts: cached, hit: true, source: "shared_cache", sharedHit: true}, nil
			}
			accounts, hit, err := s.listAllAccountsForOpsFromRealtimeCacheUncached(ctx, platformFilter, groupIDFilter)
			if err != nil {
				return nil, err
			}
			if !hit {
				return opsSharedRealtimeAccountsLoadResult{source: "ops_realtime_cache_miss"}, nil
			}
			return opsSharedRealtimeAccountsLoadResult{
				accounts:  s.setCachedRealtimeAccounts(cacheKey, accounts),
				hit:       true,
				source:    "ops_realtime_cache",
				sharedHit: false,
			}, nil
		})
		if err != nil {
			s.logRealtimeSharedSnapshot("ops_accounts_mirror", cacheKey, false, "ops_realtime_cache_error", 0, time.Since(start))
			return opsSharedRealtimeAccountsLoadResult{}, err
		}
		result, _ := value.(opsSharedRealtimeAccountsLoadResult)
		s.logRealtimeSharedSnapshot("ops_accounts_mirror", cacheKey, result.sharedHit, result.source, len(result.accounts), time.Since(start))
		return result, nil
	}

	accounts, hit, err := s.listAllAccountsForOpsFromRealtimeCacheUncached(ctx, platformFilter, groupIDFilter)
	if err != nil {
		s.logRealtimeSharedSnapshot("ops_accounts_mirror", cacheKey, false, "ops_realtime_cache_error", 0, time.Since(start))
		return opsSharedRealtimeAccountsLoadResult{}, err
	}
	result := opsSharedRealtimeAccountsLoadResult{accounts: accounts, hit: hit, source: "ops_realtime_cache", sharedHit: false}
	if !hit {
		result.source = "ops_realtime_cache_miss"
	}
	s.logRealtimeSharedSnapshot("ops_accounts_mirror", cacheKey, result.sharedHit, result.source, len(result.accounts), time.Since(start))
	return result, nil
}

func (s *OpsService) loadSharedOpsRealtimeAccountsFromRepo(ctx context.Context, platformFilter string, groupIDFilter *int64) (opsSharedRealtimeAccountsLoadResult, error) {
	cacheKey := buildOpsRealtimeAccountsCacheKey(platformFilter, groupIDFilter)
	start := time.Now()
	if cacheKey != "" {
		if cached, ok := s.getCachedRealtimeAccounts(cacheKey); ok {
			result := opsSharedRealtimeAccountsLoadResult{accounts: cached, hit: true, source: "shared_cache", sharedHit: true}
			s.logRealtimeSharedSnapshot("ops_accounts_repo", cacheKey, result.sharedHit, result.source, len(result.accounts), time.Since(start))
			return result, nil
		}
		value, err, _ := s.realtimeSnapshotFlight.Do("ops_accounts_repo:"+cacheKey, func() (any, error) {
			if cached, ok := s.getCachedRealtimeAccounts(cacheKey); ok {
				return opsSharedRealtimeAccountsLoadResult{accounts: cached, hit: true, source: "shared_cache", sharedHit: true}, nil
			}
			loaded, err := s.listAllAccountsForOpsUncached(ctx, platformFilter, groupIDFilter)
			if err != nil {
				return nil, err
			}
			return opsSharedRealtimeAccountsLoadResult{
				accounts:  s.setCachedRealtimeAccounts(cacheKey, loaded),
				hit:       true,
				source:    "repository",
				sharedHit: false,
			}, nil
		})
		if err != nil {
			return opsSharedRealtimeAccountsLoadResult{}, err
		}
		result, _ := value.(opsSharedRealtimeAccountsLoadResult)
		s.logRealtimeSharedSnapshot("ops_accounts_repo", cacheKey, result.sharedHit, result.source, len(result.accounts), time.Since(start))
		return result, nil
	}

	loaded, err := s.listAllAccountsForOpsUncached(ctx, platformFilter, groupIDFilter)
	if err != nil {
		return opsSharedRealtimeAccountsLoadResult{}, err
	}
	result := opsSharedRealtimeAccountsLoadResult{accounts: loaded, hit: true, source: "repository", sharedHit: false}
	s.logRealtimeSharedSnapshot("ops_accounts_repo", cacheKey, result.sharedHit, result.source, len(result.accounts), time.Since(start))
	return result, nil
}

func (s *OpsService) listAllAccountsForOpsFromRealtimeCacheUncached(ctx context.Context, platformFilter string, groupIDFilter *int64) ([]Account, bool, error) {
	if s == nil || s.opsRealtimeCache == nil {
		return nil, false, nil
	}
	ready, err := s.opsRealtimeCache.IsAccountIndexReady(ctx)
	if err != nil || !ready {
		return nil, false, err
	}
	ids, err := s.opsRealtimeCache.ListAccountIDs(ctx, platformFilter, groupIDFilter)
	if err != nil {
		return nil, false, err
	}
	entries, err := s.opsRealtimeCache.GetAccounts(ctx, ids)
	if err != nil {
		return nil, false, err
	}
	if len(ids) > 0 && len(entries) < len(ids) {
		return nil, false, nil
	}
	accounts := make([]Account, 0, len(entries))
	for _, accountID := range ids {
		entry := entries[accountID]
		if entry == nil {
			return nil, false, nil
		}
		if strings.TrimSpace(platformFilter) != "" && entry.Platform != platformFilter {
			return nil, false, nil
		}
		if groupIDFilter != nil && *groupIDFilter > 0 && !opsRealtimeEntryHasGroup(entry, *groupIDFilter) {
			return nil, false, nil
		}
		account := entry.ToAccount()
		if account == nil {
			return nil, false, nil
		}
		accounts = append(accounts, *account)
	}
	sort.SliceStable(accounts, func(i, j int) bool { return accounts[i].ID > accounts[j].ID })
	return accounts, true, nil
}

func (s *OpsService) listSchedulableAccountsForConcurrencyFromRealtimeCache(ctx context.Context, platformFilter string, groupIDFilter *int64) ([]Account, bool, error) {
	result, err := s.loadSharedOpsRealtimeAccountsFromMirror(ctx, platformFilter, groupIDFilter)
	if err != nil || !result.hit {
		return nil, result.hit, err
	}
	filtered := make([]Account, 0, len(result.accounts))
	for _, account := range result.accounts {
		if !account.IsSchedulable() {
			continue
		}
		filtered = append(filtered, account)
	}
	return filtered, true, nil
}

func opsRealtimeEntryHasGroup(entry *OpsRealtimeAccountCacheEntry, groupID int64) bool {
	if entry == nil || groupID <= 0 {
		return false
	}
	for _, gid := range entry.GroupIDs {
		if gid == groupID {
			return true
		}
	}
	for _, group := range entry.Groups {
		if group != nil && group.GroupID == groupID {
			return true
		}
	}
	return false
}
