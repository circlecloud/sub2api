package repository

import (
	"context"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	dbaccount "github.com/Wei-Shaw/sub2api/ent/account"
	dbaccountgroup "github.com/Wei-Shaw/sub2api/ent/accountgroup"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// ListOpsRealtimeAccounts returns a lightweight account projection for ops realtime dashboards.
//
// It avoids the generic paginated list path so high-cardinality ops panels do not spend time on
// repeated COUNT/OFFSET queries or loading heavyweight fields like credentials / proxy metadata.
func (r *accountRepository) ListOpsRealtimeAccounts(ctx context.Context, platformFilter string, groupIDFilter *int64) ([]service.Account, error) {
	q := r.client.Account.Query().
		Select(
			dbaccount.FieldID,
			dbaccount.FieldName,
			dbaccount.FieldPlatform,
			dbaccount.FieldConcurrency,
			dbaccount.FieldStatus,
			dbaccount.FieldSchedulable,
			dbaccount.FieldErrorMessage,
			dbaccount.FieldRateLimitResetAt,
			dbaccount.FieldOverloadUntil,
			dbaccount.FieldTempUnschedulableUntil,
		).
		Order(dbent.Desc(dbaccount.FieldID))

	if platformFilter != "" {
		q = q.Where(dbaccount.PlatformEQ(platformFilter))
	}
	if groupIDFilter != nil && *groupIDFilter > 0 {
		q = q.Where(dbaccount.HasAccountGroupsWith(dbaccountgroup.GroupIDEQ(*groupIDFilter)))
	}

	accounts, err := q.All(ctx)
	if err != nil {
		return nil, err
	}

	accountIDs := make([]int64, 0, len(accounts))
	for _, acc := range accounts {
		if acc == nil || acc.ID <= 0 {
			continue
		}
		accountIDs = append(accountIDs, acc.ID)
	}
	groupsByAccount, groupIDsByAccount, _, err := r.loadAccountGroups(ctx, accountIDs)
	if err != nil {
		return nil, err
	}

	return r.opsRealtimeAccountsToService(accounts, groupsByAccount, groupIDsByAccount), nil
}

// ListActiveForTokenRefresh returns active accounts without loading proxy/group relations.
// Token refresh only needs account core fields and will resolve proxy details lazily when privacy
// checks run, so skipping relation hydration avoids huge ID fan-out queries on large account sets.
func (r *accountRepository) ListActiveForTokenRefresh(ctx context.Context) ([]service.Account, error) {
	accounts, err := r.client.Account.Query().
		Where(dbaccount.StatusEQ(service.StatusActive)).
		Order(dbent.Asc(dbaccount.FieldPriority)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]service.Account, 0, len(accounts))
	for _, acc := range accounts {
		mapped := accountEntityToService(acc)
		if mapped == nil {
			continue
		}
		out = append(out, *mapped)
	}
	return out, nil
}

// GetModelProjectionByIDs returns a lightweight account projection for available model calculations.
// It only loads the fields needed to build model lists and intentionally skips proxy/group hydration.
func (r *accountRepository) GetModelProjectionByIDs(ctx context.Context, ids []int64) ([]*service.Account, error) {
	uniqueIDs := make([]int64, 0, len(ids))
	seen := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		uniqueIDs = append(uniqueIDs, id)
	}
	if len(uniqueIDs) == 0 {
		return []*service.Account{}, nil
	}

	const modelProjectionChunkSize = 1000
	accountsByID := make(map[int64]*service.Account, len(uniqueIDs))
	for _, chunk := range chunkPositiveInt64s(uniqueIDs, modelProjectionChunkSize) {
		accounts, err := r.client.Account.Query().
			Select(
				dbaccount.FieldID,
				dbaccount.FieldPlatform,
				dbaccount.FieldType,
				dbaccount.FieldCredentials,
				dbaccount.FieldExtra,
			).
			Where(dbaccount.IDIn(chunk...)).
			All(ctx)
		if err != nil {
			return nil, err
		}
		for _, account := range accounts {
			mapped := accountEntityToService(account)
			if mapped == nil || mapped.ID <= 0 {
				continue
			}
			accountsByID[mapped.ID] = mapped
		}
	}

	out := make([]*service.Account, 0, len(uniqueIDs))
	for _, id := range uniqueIDs {
		if account, ok := accountsByID[id]; ok && account != nil {
			out = append(out, account)
		}
	}
	return out, nil
}

func (r *accountRepository) opsRealtimeAccountsToService(accounts []*dbent.Account, groupsByAccount map[int64][]*service.Group, groupIDsByAccount map[int64][]int64) []service.Account {
	if len(accounts) == 0 {
		return []service.Account{}
	}

	outAccounts := make([]service.Account, 0, len(accounts))
	for _, acc := range accounts {
		out := accountEntityToOpsRealtimeService(acc)
		if out == nil {
			continue
		}
		if groups, ok := groupsByAccount[acc.ID]; ok && len(groups) > 0 {
			out.Groups = groups
		}
		if groupIDs, ok := groupIDsByAccount[acc.ID]; ok && len(groupIDs) > 0 {
			out.GroupIDs = groupIDs
		}
		outAccounts = append(outAccounts, *out)
	}

	return outAccounts
}

func accountEntityToOpsRealtimeService(m *dbent.Account) *service.Account {
	if m == nil {
		return nil
	}
	return &service.Account{
		ID:                     m.ID,
		Name:                   m.Name,
		Platform:               m.Platform,
		Concurrency:            m.Concurrency,
		Status:                 m.Status,
		ErrorMessage:           derefString(m.ErrorMessage),
		Schedulable:            m.Schedulable,
		RateLimitResetAt:       m.RateLimitResetAt,
		OverloadUntil:          m.OverloadUntil,
		TempUnschedulableUntil: m.TempUnschedulableUntil,
	}
}
