package repository

import (
	"context"

	dbaccountgroup "github.com/Wei-Shaw/sub2api/ent/accountgroup"
)

func (r *accountRepository) loadAccountGroupIDsByAccountIDs(ctx context.Context, accountIDs []int64) ([]int64, error) {
	accountIDs = normalizePositiveInt64s(accountIDs)
	if len(accountIDs) == 0 {
		return nil, nil
	}

	groupIDs := make([]int64, 0)
	for _, chunk := range chunkPositiveInt64s(accountIDs, accountRepoIDBatchChunkSize) {
		entries, err := r.client.AccountGroup.Query().
			Where(dbaccountgroup.AccountIDIn(chunk...)).
			Order(dbaccountgroup.ByAccountID(), dbaccountgroup.ByPriority()).
			All(ctx)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			groupIDs = append(groupIDs, entry.GroupID)
		}
	}
	return normalizePositiveInt64s(groupIDs), nil
}

func mergeGroupIDs(a []int64, b []int64) []int64 {
	seen := make(map[int64]struct{}, len(a)+len(b))
	out := make([]int64, 0, len(a)+len(b))
	for _, id := range a {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	for _, id := range b {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
