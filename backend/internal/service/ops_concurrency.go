package service

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

const (
	opsAccountsPageSize          = 100
	opsConcurrencyBatchChunkSize = 10000
	opsRealtimeSnapshotCacheTTL  = 3 * time.Second
	opsRealtimeResultCacheTTL    = 5 * time.Second
)

type opsRealtimeConcurrencyCacheEntry struct {
	platform    map[string]*PlatformConcurrencyInfo
	group       map[int64]*GroupConcurrencyInfo
	account     map[int64]*AccountConcurrencyInfo
	collectedAt *time.Time
	expiresAt   time.Time
}

type OpsRealtimeScope string

const (
	opsRealtimeScopePlatform OpsRealtimeScope = "platform"
	opsRealtimeScopeGroup    OpsRealtimeScope = "group"
	opsRealtimeScopeAccount  OpsRealtimeScope = "account"
)

type opsRealtimeAccountsCacheEntry struct {
	accounts  []Account
	expiresAt time.Time
}

type opsRealtimeUsersCacheEntry struct {
	users     []User
	expiresAt time.Time
}

type opsRealtimeAccountLister interface {
	ListOpsRealtimeAccounts(ctx context.Context, platformFilter string, groupIDFilter *int64) ([]Account, error)
}

func NormalizeOpsRealtimeScope(raw string, platformFilter string, groupIDFilter *int64, includeAccount bool) OpsRealtimeScope {
	scope := OpsRealtimeScope(strings.ToLower(strings.TrimSpace(raw)))
	switch scope {
	case opsRealtimeScopePlatform, opsRealtimeScopeGroup, opsRealtimeScopeAccount:
		return scope
	}
	if groupIDFilter != nil && *groupIDFilter > 0 {
		if includeAccount {
			return opsRealtimeScopeAccount
		}
		return opsRealtimeScopeGroup
	}
	if strings.TrimSpace(platformFilter) != "" {
		return opsRealtimeScopeGroup
	}
	return opsRealtimeScopePlatform
}

func (s OpsRealtimeScope) includePlatform() bool {
	return s == opsRealtimeScopePlatform
}

func (s OpsRealtimeScope) includeGroup() bool {
	return s == opsRealtimeScopeGroup
}

func (s OpsRealtimeScope) includeAccount() bool {
	return s == opsRealtimeScopeAccount
}

func buildOpsRealtimeAccountsCacheKey(platformFilter string, groupIDFilter *int64) string {
	groupID := int64(0)
	if groupIDFilter != nil && *groupIDFilter > 0 {
		groupID = *groupIDFilter
	}
	return fmt.Sprintf("%s|%d", strings.ToLower(strings.TrimSpace(platformFilter)), groupID)
}

func (s *OpsService) initRealtimeSnapshotCache() {
	if s == nil {
		return
	}
	s.realtimeSnapshotOnce.Do(func() {
		s.realtimeAccountsCache = make(map[string]opsRealtimeAccountsCacheEntry)
		s.realtimeConcurrencyCache = make(map[string]opsRealtimeConcurrencyCacheEntry)
		s.realtimeWarmPoolCache = make(map[string]opsWarmPoolResultCacheEntry)
		s.realtimeCacheMissLogAt = make(map[string]time.Time)
	})
}

func (s *OpsService) getCachedRealtimeAccounts(key string) ([]Account, bool) {
	if s == nil || key == "" {
		return nil, false
	}
	s.initRealtimeSnapshotCache()
	now := time.Now()
	s.realtimeSnapshotMu.RLock()
	entry, ok := s.realtimeAccountsCache[key]
	s.realtimeSnapshotMu.RUnlock()
	if !ok {
		return nil, false
	}
	if now.After(entry.expiresAt) {
		s.realtimeSnapshotMu.Lock()
		delete(s.realtimeAccountsCache, key)
		s.realtimeSnapshotMu.Unlock()
		return nil, false
	}
	return entry.accounts, true
}

func (s *OpsService) setCachedRealtimeAccounts(key string, accounts []Account) []Account {
	if s == nil || key == "" {
		return accounts
	}
	s.initRealtimeSnapshotCache()
	s.realtimeSnapshotMu.Lock()
	s.realtimeAccountsCache[key] = opsRealtimeAccountsCacheEntry{accounts: accounts, expiresAt: time.Now().Add(opsRealtimeSnapshotCacheTTL)}
	s.realtimeSnapshotMu.Unlock()
	return accounts
}

func (s *OpsService) getCachedRealtimeUsers() ([]User, bool) {
	if s == nil {
		return nil, false
	}
	s.initRealtimeSnapshotCache()
	now := time.Now()
	s.realtimeSnapshotMu.RLock()
	entry := s.realtimeUsersCache
	s.realtimeSnapshotMu.RUnlock()
	if entry == nil {
		return nil, false
	}
	if now.After(entry.expiresAt) {
		s.realtimeSnapshotMu.Lock()
		s.realtimeUsersCache = nil
		s.realtimeSnapshotMu.Unlock()
		return nil, false
	}
	return entry.users, true
}

func (s *OpsService) setCachedRealtimeUsers(users []User) []User {
	if s == nil {
		return users
	}
	s.initRealtimeSnapshotCache()
	s.realtimeSnapshotMu.Lock()
	s.realtimeUsersCache = &opsRealtimeUsersCacheEntry{users: users, expiresAt: time.Now().Add(opsRealtimeSnapshotCacheTTL)}
	s.realtimeSnapshotMu.Unlock()
	return users
}

func buildOpsRealtimeConcurrencyCacheKey(platformFilter string, groupIDFilter *int64, scope OpsRealtimeScope) string {
	groupID := int64(0)
	if groupIDFilter != nil && *groupIDFilter > 0 {
		groupID = *groupIDFilter
	}
	return fmt.Sprintf("%s|%d|%s", strings.ToLower(strings.TrimSpace(platformFilter)), groupID, scope)
}

func (s *OpsService) getCachedRealtimeConcurrencyStats(key string) (map[string]*PlatformConcurrencyInfo, map[int64]*GroupConcurrencyInfo, map[int64]*AccountConcurrencyInfo, *time.Time, bool) {
	if s == nil || key == "" {
		return nil, nil, nil, nil, false
	}
	s.initRealtimeSnapshotCache()
	now := time.Now()
	s.realtimeSnapshotMu.RLock()
	entry, ok := s.realtimeConcurrencyCache[key]
	s.realtimeSnapshotMu.RUnlock()
	if !ok {
		return nil, nil, nil, nil, false
	}
	if now.After(entry.expiresAt) {
		s.realtimeSnapshotMu.Lock()
		delete(s.realtimeConcurrencyCache, key)
		s.realtimeSnapshotMu.Unlock()
		return nil, nil, nil, nil, false
	}
	return entry.platform, entry.group, entry.account, entry.collectedAt, true
}

func (s *OpsService) setCachedRealtimeConcurrencyStats(key string, platform map[string]*PlatformConcurrencyInfo, group map[int64]*GroupConcurrencyInfo, account map[int64]*AccountConcurrencyInfo, collectedAt *time.Time) {
	if s == nil || key == "" {
		return
	}
	s.initRealtimeSnapshotCache()
	s.realtimeSnapshotMu.Lock()
	s.realtimeConcurrencyCache[key] = opsRealtimeConcurrencyCacheEntry{
		platform:    platform,
		group:       group,
		account:     account,
		collectedAt: collectedAt,
		expiresAt:   time.Now().Add(opsRealtimeResultCacheTTL),
	}
	s.realtimeSnapshotMu.Unlock()
}

func (s *OpsService) listConcurrencyAccounts(ctx context.Context, platformFilter string, groupIDFilter *int64, scope OpsRealtimeScope) ([]Account, error) {
	if s == nil || s.accountRepo == nil {
		return []Account{}, nil
	}
	if scope.includePlatform() && !scope.includeGroup() && !scope.includeAccount() {
		if accounts, hit, err := s.listSchedulableAccountsForConcurrencyFromRealtimeCache(ctx, platformFilter, groupIDFilter); err != nil {
			s.logRealtimeCacheMiss("ops_concurrency_platform_scope", err)
		} else if hit {
			return accounts, nil
		}
		if groupIDFilter != nil && *groupIDFilter > 0 {
			if strings.TrimSpace(platformFilter) != "" {
				return s.accountRepo.ListSchedulableByGroupIDAndPlatform(ctx, *groupIDFilter, platformFilter)
			}
			return s.accountRepo.ListSchedulableByGroupID(ctx, *groupIDFilter)
		}
		if strings.TrimSpace(platformFilter) != "" {
			return s.accountRepo.ListSchedulableByPlatform(ctx, platformFilter)
		}
		return s.accountRepo.ListSchedulable(ctx)
	}
	return s.listAllAccountsForOps(ctx, platformFilter, groupIDFilter)
}

func (s *OpsService) listAllAccountsForOps(ctx context.Context, platformFilter string, groupIDFilter *int64) ([]Account, error) {
	if s == nil || s.accountRepo == nil {
		return []Account{}, nil
	}
	if accounts, hit, err := s.listAllAccountsForOpsFromRealtimeCache(ctx, platformFilter, groupIDFilter); err != nil {
		s.logRealtimeCacheMiss("ops_account_list", err)
	} else if hit {

		return accounts, nil
	}
	cacheKey := buildOpsRealtimeAccountsCacheKey(platformFilter, groupIDFilter)
	if cached, ok := s.getCachedRealtimeAccounts(cacheKey); ok {
		return cached, nil
	}
	if cacheKey != "" {
		value, err, _ := s.realtimeSnapshotFlight.Do("ops_accounts:"+cacheKey, func() (any, error) {
			if cached, ok := s.getCachedRealtimeAccounts(cacheKey); ok {
				return cached, nil
			}
			loaded, err := s.listAllAccountsForOpsUncached(ctx, platformFilter, groupIDFilter)
			if err != nil {
				return nil, err
			}
			return s.setCachedRealtimeAccounts(cacheKey, loaded), nil
		})
		if err != nil {
			return nil, err
		}
		accounts, _ := value.([]Account)
		return accounts, nil
	}
	return s.listAllAccountsForOpsUncached(ctx, platformFilter, groupIDFilter)
}

func (s *OpsService) listAllAccountsForOpsUncached(ctx context.Context, platformFilter string, groupIDFilter *int64) ([]Account, error) {
	if s == nil || s.accountRepo == nil {
		return []Account{}, nil
	}
	if lister, ok := s.accountRepo.(opsRealtimeAccountLister); ok {
		return lister.ListOpsRealtimeAccounts(ctx, platformFilter, groupIDFilter)
	}

	resolvedGroupID := int64(0)
	if groupIDFilter != nil && *groupIDFilter > 0 {
		resolvedGroupID = *groupIDFilter
	}

	out := make([]Account, 0, 128)
	page := 1
	for {
		groupFilter := ""
		if resolvedGroupID > 0 {
			groupFilter = strconv.FormatInt(resolvedGroupID, 10)
		}
		accounts, pageInfo, err := s.accountRepo.ListWithFilters(ctx, pagination.PaginationParams{
			Page:      page,
			PageSize:  opsAccountsPageSize,
			SortBy:    "id",
			SortOrder: "desc",
		}, AccountListFilters{Platform: platformFilter, GroupIDs: groupFilter})
		if err != nil {
			return nil, err
		}
		if len(accounts) == 0 {
			break
		}

		out = append(out, accounts...)
		if pageInfo != nil && int64(len(out)) >= pageInfo.Total {
			break
		}
		if len(accounts) < opsAccountsPageSize {
			break
		}

		page++
		if page > 10_000 {
			log.Printf("[Ops] listAllAccountsForOps: aborting after too many pages (platform=%q group=%d)", platformFilter, resolvedGroupID)
			break
		}
	}

	return out, nil
}

func (s *OpsService) getAccountsLoadMapBestEffort(ctx context.Context, accounts []Account) map[int64]*AccountLoadInfo {
	if s == nil || s.concurrencyService == nil {
		return map[int64]*AccountLoadInfo{}
	}
	if len(accounts) == 0 {
		return map[int64]*AccountLoadInfo{}
	}

	// De-duplicate IDs (and keep the max concurrency to avoid under-reporting).
	unique := make(map[int64]int, len(accounts))
	for _, acc := range accounts {
		if acc.ID <= 0 {
			continue
		}
		lf := acc.EffectiveLoadFactor()
		if prev, ok := unique[acc.ID]; !ok || lf > prev {
			unique[acc.ID] = lf
		}
	}

	batch := make([]AccountWithConcurrency, 0, len(unique))
	for id, maxConc := range unique {
		batch = append(batch, AccountWithConcurrency{
			ID:             id,
			MaxConcurrency: maxConc,
		})
	}

	out := make(map[int64]*AccountLoadInfo, len(batch))
	for i := 0; i < len(batch); i += opsConcurrencyBatchChunkSize {
		end := i + opsConcurrencyBatchChunkSize
		if end > len(batch) {
			end = len(batch)
		}
		part, err := s.concurrencyService.GetAccountsLoadBatch(ctx, batch[i:end])
		if err != nil {
			// Best-effort: return zeros rather than failing the ops UI.
			log.Printf("[Ops] GetAccountsLoadBatch failed: %v", err)
			continue
		}
		for k, v := range part {
			out[k] = v
		}
	}

	return out
}

// GetConcurrencyStats returns real-time concurrency usage aggregated by platform/group/account.
//
// Optional filters:
// - platformFilter: only include accounts in that platform (best-effort reduces DB load)
// - groupIDFilter: only include accounts that belong to that group
func (s *OpsService) GetConcurrencyStats(
	ctx context.Context,
	platformFilter string,
	groupIDFilter *int64,
) (map[string]*PlatformConcurrencyInfo, map[int64]*GroupConcurrencyInfo, map[int64]*AccountConcurrencyInfo, *time.Time, error) {
	return s.GetConcurrencyStatsWithOptions(ctx, platformFilter, groupIDFilter, NormalizeOpsRealtimeScope("", platformFilter, groupIDFilter, true))
}

func (s *OpsService) GetConcurrencyStatsWithOptions(
	ctx context.Context,
	platformFilter string,
	groupIDFilter *int64,
	scope OpsRealtimeScope,
) (map[string]*PlatformConcurrencyInfo, map[int64]*GroupConcurrencyInfo, map[int64]*AccountConcurrencyInfo, *time.Time, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, nil, nil, nil, err
	}

	cacheKey := ""
	if !scope.includeAccount() {
		cacheKey = buildOpsRealtimeConcurrencyCacheKey(platformFilter, groupIDFilter, scope)
		if platform, group, account, collectedAt, ok := s.getCachedRealtimeConcurrencyStats(cacheKey); ok {
			return platform, group, account, collectedAt, nil
		}
	}

	compute := func() (map[string]*PlatformConcurrencyInfo, map[int64]*GroupConcurrencyInfo, map[int64]*AccountConcurrencyInfo, *time.Time, error) {
		accounts, err := s.listConcurrencyAccounts(ctx, platformFilter, groupIDFilter, scope)
		if err != nil {
			return nil, nil, nil, nil, err
		}

		collectedAt := time.Now()
		loadMap := s.getAccountsLoadMapBestEffort(ctx, accounts)

		platform := make(map[string]*PlatformConcurrencyInfo)
		group := make(map[int64]*GroupConcurrencyInfo)
		account := map[int64]*AccountConcurrencyInfo{}
		if scope.includeAccount() {
			account = make(map[int64]*AccountConcurrencyInfo)
		}

		for _, acc := range accounts {
			if acc.ID <= 0 {
				continue
			}

			var matchedGroup *Group
			if groupIDFilter != nil && *groupIDFilter > 0 {
				for _, grp := range acc.Groups {
					if grp == nil || grp.ID <= 0 {
						continue
					}
					if grp.ID == *groupIDFilter {
						matchedGroup = grp
						break
					}
				}
				// Fallback path may still return broader account data; keep this guard for correctness.
				if matchedGroup == nil {
					continue
				}
			}

			load := loadMap[acc.ID]
			currentInUse := int64(0)
			waiting := int64(0)
			if load != nil {
				currentInUse = int64(load.CurrentConcurrency)
				waiting = int64(load.WaitingCount)
			}

			// Account-level view picks one display group (the first group).
			displayGroupID := int64(0)
			displayGroupName := ""
			if matchedGroup != nil {
				displayGroupID = matchedGroup.ID
				displayGroupName = matchedGroup.Name
			} else if len(acc.Groups) > 0 && acc.Groups[0] != nil {
				displayGroupID = acc.Groups[0].ID
				displayGroupName = acc.Groups[0].Name
			}

			if scope.includeAccount() {
				if _, ok := account[acc.ID]; !ok {
					info := &AccountConcurrencyInfo{
						AccountID:      acc.ID,
						AccountName:    acc.Name,
						Platform:       acc.Platform,
						GroupID:        displayGroupID,
						GroupName:      displayGroupName,
						CurrentInUse:   currentInUse,
						MaxCapacity:    int64(acc.Concurrency),
						WaitingInQueue: waiting,
					}
					if info.MaxCapacity > 0 {
						info.LoadPercentage = float64(info.CurrentInUse) / float64(info.MaxCapacity) * 100
					}
					account[acc.ID] = info
				}
			}

			// Platform aggregation.
			if scope.includePlatform() && acc.Platform != "" {
				if _, ok := platform[acc.Platform]; !ok {
					platform[acc.Platform] = &PlatformConcurrencyInfo{
						Platform: acc.Platform,
					}
				}
				p := platform[acc.Platform]
				p.MaxCapacity += int64(acc.Concurrency)
				p.CurrentInUse += currentInUse
				p.WaitingInQueue += waiting
			}

			// Group aggregation (one account may contribute to multiple groups).
			if scope.includeGroup() {
				if matchedGroup != nil {
					grp := matchedGroup
					if _, ok := group[grp.ID]; !ok {
						group[grp.ID] = &GroupConcurrencyInfo{
							GroupID:   grp.ID,
							GroupName: grp.Name,
							Platform:  grp.Platform,
						}
					}
					g := group[grp.ID]
					if g.GroupName == "" && grp.Name != "" {
						g.GroupName = grp.Name
					}
					if g.Platform != "" && grp.Platform != "" && g.Platform != grp.Platform {
						// Groups are expected to be platform-scoped. If mismatch is observed, avoid misleading labels.
						g.Platform = ""
					}
					g.MaxCapacity += int64(acc.Concurrency)
					g.CurrentInUse += currentInUse
					g.WaitingInQueue += waiting
				} else {
					for _, grp := range acc.Groups {
						if grp == nil || grp.ID <= 0 {
							continue
						}
						if _, ok := group[grp.ID]; !ok {
							group[grp.ID] = &GroupConcurrencyInfo{
								GroupID:   grp.ID,
								GroupName: grp.Name,
								Platform:  grp.Platform,
							}
						}
						g := group[grp.ID]
						if g.GroupName == "" && grp.Name != "" {
							g.GroupName = grp.Name
						}
						if g.Platform != "" && grp.Platform != "" && g.Platform != grp.Platform {
							// Groups are expected to be platform-scoped. If mismatch is observed, avoid misleading labels.
							g.Platform = ""
						}
						g.MaxCapacity += int64(acc.Concurrency)
						g.CurrentInUse += currentInUse
						g.WaitingInQueue += waiting
					}
				}
			}
		}

		for _, info := range platform {
			if info.MaxCapacity > 0 {
				info.LoadPercentage = float64(info.CurrentInUse) / float64(info.MaxCapacity) * 100
			}
		}
		for _, info := range group {
			if info.MaxCapacity > 0 {
				info.LoadPercentage = float64(info.CurrentInUse) / float64(info.MaxCapacity) * 100
			}
		}

		return platform, group, account, &collectedAt, nil
	}
	if cacheKey == "" {
		return compute()
	}

	value, err, _ := s.realtimeSnapshotFlight.Do("ops_concurrency:"+cacheKey, func() (any, error) {
		if platform, group, account, collectedAt, ok := s.getCachedRealtimeConcurrencyStats(cacheKey); ok {
			return opsRealtimeConcurrencyCacheEntry{platform: platform, group: group, account: account, collectedAt: collectedAt}, nil
		}
		platform, group, account, collectedAt, err := compute()
		if err != nil {
			return nil, err
		}
		s.setCachedRealtimeConcurrencyStats(cacheKey, platform, group, account, collectedAt)
		return opsRealtimeConcurrencyCacheEntry{platform: platform, group: group, account: account, collectedAt: collectedAt}, nil
	})
	if err != nil {
		return nil, nil, nil, nil, err
	}
	entry, _ := value.(opsRealtimeConcurrencyCacheEntry)
	return entry.platform, entry.group, entry.account, entry.collectedAt, nil
}

// listAllActiveUsersForOps returns all active users with their concurrency settings.
func (s *OpsService) listAllActiveUsersForOps(ctx context.Context) ([]User, error) {
	if s == nil || s.userRepo == nil {
		return []User{}, nil
	}
	if cached, ok := s.getCachedRealtimeUsers(); ok {
		return cached, nil
	}
	value, err, _ := s.realtimeSnapshotFlight.Do("ops_users", func() (any, error) {
		if cached, ok := s.getCachedRealtimeUsers(); ok {
			return cached, nil
		}
		loaded, err := s.listAllActiveUsersForOpsUncached(ctx)
		if err != nil {
			return nil, err
		}
		return s.setCachedRealtimeUsers(loaded), nil
	})
	if err != nil {
		return nil, err
	}
	users, _ := value.([]User)
	return users, nil
}

func (s *OpsService) listAllActiveUsersForOpsUncached(ctx context.Context) ([]User, error) {
	if s == nil || s.userRepo == nil {
		return []User{}, nil
	}

	out := make([]User, 0, 128)
	page := 1
	for {
		users, pageInfo, err := s.userRepo.ListWithFilters(ctx, pagination.PaginationParams{
			Page:     page,
			PageSize: opsAccountsPageSize,
		}, UserListFilters{
			Status: StatusActive,
		})
		if err != nil {
			return nil, err
		}
		if len(users) == 0 {
			break
		}

		out = append(out, users...)
		if pageInfo != nil && int64(len(out)) >= pageInfo.Total {
			break
		}
		if len(users) < opsAccountsPageSize {
			break
		}

		page++
		if page > 10_000 {
			log.Printf("[Ops] listAllActiveUsersForOps: aborting after too many pages")
			break
		}
	}

	return out, nil
}

// getUsersLoadMapBestEffort returns user load info for the given users.
func (s *OpsService) getUsersLoadMapBestEffort(ctx context.Context, users []User) map[int64]*UserLoadInfo {
	if s == nil || s.concurrencyService == nil {
		return map[int64]*UserLoadInfo{}
	}
	if len(users) == 0 {
		return map[int64]*UserLoadInfo{}
	}

	// De-duplicate IDs (and keep the max concurrency to avoid under-reporting).
	unique := make(map[int64]int, len(users))
	for _, u := range users {
		if u.ID <= 0 {
			continue
		}
		if prev, ok := unique[u.ID]; !ok || u.Concurrency > prev {
			unique[u.ID] = u.Concurrency
		}
	}

	batch := make([]UserWithConcurrency, 0, len(unique))
	for id, maxConc := range unique {
		batch = append(batch, UserWithConcurrency{
			ID:             id,
			MaxConcurrency: maxConc,
		})
	}

	out := make(map[int64]*UserLoadInfo, len(batch))
	for i := 0; i < len(batch); i += opsConcurrencyBatchChunkSize {
		end := i + opsConcurrencyBatchChunkSize
		if end > len(batch) {
			end = len(batch)
		}
		part, err := s.concurrencyService.GetUsersLoadBatch(ctx, batch[i:end])
		if err != nil {
			// Best-effort: return zeros rather than failing the ops UI.
			log.Printf("[Ops] GetUsersLoadBatch failed: %v", err)
			continue
		}
		for k, v := range part {
			out[k] = v
		}
	}

	return out
}

// GetUserConcurrencyStats returns real-time concurrency usage for all active users.
func (s *OpsService) GetUserConcurrencyStats(ctx context.Context) (map[int64]*UserConcurrencyInfo, *time.Time, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, nil, err
	}

	users, err := s.listAllActiveUsersForOps(ctx)
	if err != nil {
		return nil, nil, err
	}

	collectedAt := time.Now()
	loadMap := s.getUsersLoadMapBestEffort(ctx, users)

	result := make(map[int64]*UserConcurrencyInfo)

	for _, u := range users {
		if u.ID <= 0 {
			continue
		}

		load := loadMap[u.ID]
		currentInUse := int64(0)
		waiting := int64(0)
		if load != nil {
			currentInUse = int64(load.CurrentConcurrency)
			waiting = int64(load.WaitingCount)
		}

		// Skip users with no concurrency activity
		if currentInUse == 0 && waiting == 0 {
			continue
		}

		info := &UserConcurrencyInfo{
			UserID:         u.ID,
			UserEmail:      u.Email,
			Username:       u.Username,
			CurrentInUse:   currentInUse,
			MaxCapacity:    int64(u.Concurrency),
			WaitingInQueue: waiting,
		}
		if info.MaxCapacity > 0 {
			info.LoadPercentage = float64(info.CurrentInUse) / float64(info.MaxCapacity) * 100
		}
		result[u.ID] = info
	}

	return result, &collectedAt, nil
}
