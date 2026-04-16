package repository

import (
	"context"
	"errors"
	"sort"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	dbaccount "github.com/Wei-Shaw/sub2api/ent/account"
	dbaccountgroup "github.com/Wei-Shaw/sub2api/ent/accountgroup"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (r *accountRepository) PreviewBulkUpdateTargets(ctx context.Context, filter service.AccountBulkFilter) (*service.BulkAccountTargetPreview, error) {
	countQuery, err := r.buildAccountFilterQuery(filter)
	if err != nil {
		return nil, err
	}
	count, err := countQuery.Count(ctx)
	if err != nil {
		return nil, err
	}
	if count == 0 {
		return &service.BulkAccountTargetPreview{}, nil
	}

	var platformRows []struct {
		Platform string `json:"platform"`
	}
	platformQuery, err := r.buildAccountFilterQuery(filter)
	if err != nil {
		return nil, err
	}
	if err := platformQuery.Unique(true).Select(dbaccount.FieldPlatform).Scan(ctx, &platformRows); err != nil {
		return nil, err
	}
	platforms := make([]string, 0, len(platformRows))
	for _, row := range platformRows {
		if strings.TrimSpace(row.Platform) == "" {
			continue
		}
		platforms = append(platforms, row.Platform)
	}
	sort.Strings(platforms)

	var typeRows []struct {
		Type string `json:"type"`
	}
	typeQuery, err := r.buildAccountFilterQuery(filter)
	if err != nil {
		return nil, err
	}
	if err := typeQuery.Unique(true).Select(dbaccount.FieldType).Scan(ctx, &typeRows); err != nil {
		return nil, err
	}
	types := make([]string, 0, len(typeRows))
	for _, row := range typeRows {
		if strings.TrimSpace(row.Type) == "" {
			continue
		}
		types = append(types, row.Type)
	}
	sort.Strings(types)

	return &service.BulkAccountTargetPreview{Count: int64(count), Platforms: platforms, Types: types}, nil
}

func (r *accountRepository) ResolveBulkUpdateTargets(ctx context.Context, filter service.AccountBulkFilter) ([]service.BulkAccountTargetRef, error) {
	q, err := r.buildAccountFilterQuery(filter)
	if err != nil {
		return nil, err
	}
	var rows []struct {
		ID       int64  `json:"id"`
		Platform string `json:"platform"`
		Type     string `json:"type"`
	}
	if err := q.Select(dbaccount.FieldID, dbaccount.FieldPlatform, dbaccount.FieldType).Scan(ctx, &rows); err != nil {
		return nil, err
	}
	resolved := make([]service.BulkAccountTargetRef, 0, len(rows))
	for _, row := range rows {
		if row.ID <= 0 {
			continue
		}
		resolved = append(resolved, service.BulkAccountTargetRef{ID: row.ID, Platform: row.Platform, Type: row.Type})
	}
	return resolved, nil
}

func (r *accountRepository) BindGroupsBatch(ctx context.Context, accountIDs []int64, groupIDs []int64) error {
	accountIDs = normalizePositiveInt64s(accountIDs)
	groupIDs = normalizePositiveInt64s(groupIDs)
	if len(accountIDs) == 0 {
		return nil
	}

	existingGroupIDs, err := r.loadAccountGroupIDsByAccountIDs(ctx, accountIDs)
	if err != nil {
		return err
	}

	tx, err := r.client.Tx(ctx)
	if err != nil && !errors.Is(err, dbent.ErrTxStarted) {
		return err
	}

	var txClient *dbent.Client
	if err == nil {
		defer func() { _ = tx.Rollback() }()
		txClient = tx.Client()
	} else {
		txClient = r.client
	}

	if _, err := txClient.AccountGroup.Delete().Where(dbaccountgroup.AccountIDIn(accountIDs...)).Exec(ctx); err != nil {
		return err
	}
	if len(groupIDs) > 0 {
		builders := make([]*dbent.AccountGroupCreate, 0, len(accountIDs)*len(groupIDs))
		for _, accountID := range accountIDs {
			for index, groupID := range groupIDs {
				builders = append(builders, txClient.AccountGroup.Create().
					SetAccountID(accountID).
					SetGroupID(groupID).
					SetPriority(index+1),
				)
			}
		}
		if _, err := txClient.AccountGroup.CreateBulk(builders...).Save(ctx); err != nil {
			return err
		}
	}
	if tx != nil {
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	affectedGroupIDs := mergeGroupIDs(existingGroupIDs, groupIDs)
	payload := map[string]any{
		"account_ids": accountIDs,
		"group_ids":   affectedGroupIDs,
	}
	if err := enqueueSchedulerOutbox(ctx, r.sql, service.SchedulerOutboxEventAccountBulkChanged, nil, nil, payload); err != nil {
		logger.LegacyPrintf("repository.account", "[SchedulerOutbox] enqueue bulk bind groups failed: err=%v", err)
	}
	return nil
}
