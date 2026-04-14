package service

import (
	"context"
	"sort"
	"strings"
)

func (s *OpsService) listAllAccountsForOpsFromRealtimeCache(ctx context.Context, platformFilter string, groupIDFilter *int64) ([]Account, bool, error) {
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
	accounts, hit, err := s.listAllAccountsForOpsFromRealtimeCache(ctx, platformFilter, groupIDFilter)
	if err != nil || !hit {
		return nil, hit, err
	}
	filtered := make([]Account, 0, len(accounts))
	for _, account := range accounts {
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
