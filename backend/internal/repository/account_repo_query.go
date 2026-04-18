package repository

import (
	"context"
	"strconv"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	dbaccount "github.com/Wei-Shaw/sub2api/ent/account"
	dbaccountgroup "github.com/Wei-Shaw/sub2api/ent/accountgroup"
	dbpredicate "github.com/Wei-Shaw/sub2api/ent/predicate"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqljson"
)

func parseAccountSearchFilter(raw string) (string, *int64) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil
	}
	if len(trimmed) > 3 && strings.EqualFold(trimmed[:3], "id:") {
		candidate := strings.TrimSpace(trimmed[3:])
		if candidate != "" {
			if id, err := strconv.ParseInt(candidate, 10, 64); err == nil && id > 0 {
				return "", &id
			}
		}
	}
	return trimmed, nil
}

func buildAccountOrderOptions(sortBy, sortOrder string) []dbaccount.OrderOption {
	fieldMap := map[string]string{
		"id":              dbaccount.FieldID,
		"name":            dbaccount.FieldName,
		"status":          dbaccount.FieldStatus,
		"schedulable":     dbaccount.FieldSchedulable,
		"priority":        dbaccount.FieldPriority,
		"rate_multiplier": dbaccount.FieldRateMultiplier,
		"last_used_at":    dbaccount.FieldLastUsedAt,
		"expires_at":      dbaccount.FieldExpiresAt,
		"created_at":      dbaccount.FieldCreatedAt,
	}

	parseList := func(raw string) []string {
		parts := strings.Split(raw, ",")
		result := make([]string, 0, len(parts))
		for _, part := range parts {
			trimmed := strings.ToLower(strings.TrimSpace(part))
			if trimmed != "" {
				result = append(result, trimmed)
			}
		}
		return result
	}

	sortFields := parseList(sortBy)
	sortOrders := parseList(sortOrder)
	options := make([]dbaccount.OrderOption, 0, len(sortFields)+1)
	seen := make(map[string]struct{}, len(sortFields))
	idIncluded := false

	for index, sortField := range sortFields {
		if _, exists := seen[sortField]; exists {
			continue
		}

		order := "asc"
		if index < len(sortOrders) && sortOrders[index] == "desc" {
			order = "desc"
		}
		if sortField == "status" {
			seen[sortField] = struct{}{}
			options = append(options, buildAccountStatusOrderOptions(order)...)
			continue
		}
		if sortField == "usage_7d_remaining" {
			seen[sortField] = struct{}{}
			options = append(options, buildAccountUsage7dRemainingOrderOptions(order)...)
			continue
		}

		field, ok := fieldMap[sortField]
		if !ok {
			continue
		}
		seen[sortField] = struct{}{}
		if sortField == "id" {
			idIncluded = true
		}
		if order == "desc" {
			options = append(options, dbent.Desc(field))
		} else {
			options = append(options, dbent.Asc(field))
		}
	}

	if len(options) == 0 {
		return []dbaccount.OrderOption{dbent.Asc(dbaccount.FieldName), dbent.Asc(dbaccount.FieldID)}
	}
	if !idIncluded {
		options = append(options, dbent.Desc(dbaccount.FieldID))
	}
	return options
}

func buildAccountStatusOrderOptions(order string) []dbaccount.OrderOption {
	statusOrderExpr := entsql.Raw(`CASE WHEN "accounts"."status" = 'active' AND "accounts"."rate_limit_reset_at" IS NOT NULL AND "accounts"."rate_limit_reset_at" > NOW() THEN 0 WHEN "accounts"."status" = 'active' AND "accounts"."overload_until" IS NOT NULL AND "accounts"."overload_until" > NOW() THEN 1 WHEN "accounts"."status" = 'active' AND "accounts"."temp_unschedulable_until" IS NOT NULL AND "accounts"."temp_unschedulable_until" > NOW() THEN 2 WHEN "accounts"."status" = 'active' AND NOT "accounts"."schedulable" THEN 3 WHEN "accounts"."status" = 'active' THEN 4 WHEN "accounts"."status" = 'disabled' THEN 5 WHEN "accounts"."status" = 'error' THEN 6 ELSE 7 END`)
	orderExpr := statusOrderExpr
	resetAtOrder := dbaccount.ByRateLimitResetAt(entsql.OrderNullsLast())
	overloadOrder := dbaccount.ByOverloadUntil(entsql.OrderNullsLast())
	tempUnschedOrder := dbaccount.ByTempUnschedulableUntil(entsql.OrderNullsLast())
	if order == "desc" {
		orderExpr = entsql.DescExpr(statusOrderExpr)
		resetAtOrder = dbaccount.ByRateLimitResetAt(entsql.OrderDesc(), entsql.OrderNullsLast())
		overloadOrder = dbaccount.ByOverloadUntil(entsql.OrderDesc(), entsql.OrderNullsLast())
		tempUnschedOrder = dbaccount.ByTempUnschedulableUntil(entsql.OrderDesc(), entsql.OrderNullsLast())
	}
	return []dbaccount.OrderOption{
		func(selector *entsql.Selector) {
			selector.OrderExpr(orderExpr)
		},
		resetAtOrder,
		overloadOrder,
		tempUnschedOrder,
	}
}

func accountExtraNumericExpr(key string) string {
	extraColumn := `"accounts"."extra"`
	trimmedValue := "btrim(COALESCE(" + extraColumn + "->>'" + key + "', ''))"
	return "CASE " +
		"WHEN jsonb_typeof(" + extraColumn + "->'" + key + "') = 'number' THEN (" + extraColumn + "->>'" + key + "')::numeric " +
		"WHEN " + trimmedValue + " ~ '^[+-]?[0-9]+(\\.[0-9]+)?$' THEN " + trimmedValue + "::numeric " +
		"ELSE NULL END"
}

func accountUsage7dRemainingExpr() string {
	openAIUsedExpr := accountExtraNumericExpr("codex_7d_used_percent")
	anthropicUtilExpr := accountExtraNumericExpr("passive_usage_7d_utilization")
	return "CASE " +
		"WHEN \"accounts\".\"platform\" = 'openai' AND \"accounts\".\"type\" = 'oauth' AND " + openAIUsedExpr + " IS NOT NULL THEN 100 - (" + openAIUsedExpr + ") " +
		"WHEN \"accounts\".\"platform\" = 'anthropic' AND (\"accounts\".\"type\" = 'oauth' OR \"accounts\".\"type\" = 'setup-token') AND " + anthropicUtilExpr + " IS NOT NULL THEN 100 - ((" + anthropicUtilExpr + ") * 100) " +
		"ELSE NULL END"
}

func buildAccountUsage7dRemainingOrderOptions(order string) []dbaccount.OrderOption {
	orderExpr := accountUsage7dRemainingExpr() + " NULLS LAST"
	if order == "desc" {
		orderExpr = accountUsage7dRemainingExpr() + " DESC NULLS LAST"
	}
	return []dbaccount.OrderOption{
		func(selector *entsql.Selector) {
			selector.OrderExpr(entsql.Raw(orderExpr))
		},
	}
}

func (r *accountRepository) buildAccountFilterQuery(filters service.AccountListFilters) (*dbent.AccountQuery, error) {
	q := r.client.Account.Query()

	if filters.Platform != "" {
		q = q.Where(dbaccount.PlatformEQ(filters.Platform))
	}
	if filters.AccountType != "" {
		q = q.Where(dbaccount.TypeEQ(filters.AccountType))
	}
	if filters.Status != "" {
		now := time.Now()
		switch filters.Status {
		case service.StatusActive:
			q = q.Where(
				dbaccount.StatusEQ(service.StatusActive),
				dbaccount.SchedulableEQ(true),
				tempUnschedulablePredicate(),
				notExpiredPredicate(now),
				dbaccount.Or(dbaccount.OverloadUntilIsNil(), dbaccount.OverloadUntilLTE(now)),
				dbaccount.Or(dbaccount.RateLimitResetAtIsNil(), dbaccount.RateLimitResetAtLTE(now)),
			)
		case "rate_limited":
			q = q.Where(dbaccount.RateLimitResetAtGT(now))
		case "temp_unschedulable":
			q = q.Where(
				dbaccount.StatusEQ(service.StatusActive),
				dbpredicate.Account(func(s *entsql.Selector) {
					col := s.C("temp_unschedulable_until")
					s.Where(entsql.And(
						entsql.Not(entsql.IsNull(col)),
						entsql.GT(col, entsql.Expr("NOW()")),
					))
				}),
			)
		case "unschedulable":
			q = q.Where(
				dbaccount.StatusEQ(service.StatusActive),
				dbaccount.SchedulableEQ(false),
				dbaccount.Or(
					dbaccount.RateLimitResetAtIsNil(),
					dbaccount.RateLimitResetAtLTE(time.Now()),
				),
				dbpredicate.Account(func(s *entsql.Selector) {
					col := s.C("temp_unschedulable_until")
					s.Where(entsql.Or(
						entsql.IsNull(col),
						entsql.LTE(col, entsql.Expr("NOW()")),
					))
				}),
			)
		default:
			q = q.Where(dbaccount.StatusEQ(filters.Status))
		}
	}
	if filters.Search != "" {
		searchText, searchID := parseAccountSearchFilter(filters.Search)
		if searchID != nil {
			q = q.Where(dbaccount.IDEQ(*searchID))
		} else if searchText != "" {
			q = q.Where(dbaccount.NameContainsFold(searchText))
		}
	}
	ungroupedOnly, groupIDs, err := service.ParseAccountGroupFilter(filters.GroupIDs)
	if err != nil {
		return nil, err
	}
	excludeGroupIDs, err := service.ParseAccountGroupExcludeFilter(filters.GroupExcludeIDs)
	if err != nil {
		return nil, err
	}
	if ungroupedOnly {
		q = q.Where(dbaccount.Not(dbaccount.HasAccountGroups()))
	} else if len(groupIDs) > 0 {
		if filters.GroupExact {
			q = q.Where(exactAccountGroupSetPredicate(groupIDs))
		} else {
			q = q.Where(dbaccount.HasAccountGroupsWith(dbaccountgroup.GroupIDIn(groupIDs...)))
		}
	}
	if len(excludeGroupIDs) > 0 {
		q = q.Where(dbaccount.Not(dbaccount.HasAccountGroupsWith(dbaccountgroup.GroupIDIn(excludeGroupIDs...))))
	}
	if filters.PrivacyMode != "" {
		q = q.Where(dbpredicate.Account(func(s *entsql.Selector) {
			path := sqljson.Path("privacy_mode")
			switch filters.PrivacyMode {
			case service.AccountPrivacyModeUnsetFilter:
				s.Where(entsql.Or(
					entsql.Not(sqljson.HasKey(dbaccount.FieldExtra, path)),
					sqljson.ValueEQ(dbaccount.FieldExtra, "", path),
				))
			default:
				s.Where(sqljson.ValueEQ(dbaccount.FieldExtra, filters.PrivacyMode, path))
			}
		}))
	}
	if filters.LastUsedFilter != "" {
		switch filters.LastUsedFilter {
		case "unused":
			q = q.Where(dbaccount.LastUsedAtIsNil())
		case "range":
			predicates := []dbpredicate.Account{dbaccount.LastUsedAtNotNil()}
			if filters.LastUsedStart != nil {
				predicates = append(predicates, dbaccount.LastUsedAtGTE(*filters.LastUsedStart))
			}
			if filters.LastUsedEnd != nil {
				predicates = append(predicates, dbaccount.LastUsedAtLT(*filters.LastUsedEnd))
			}
			q = q.Where(predicates...)
		}
	}

	return q, nil
}

func (r *accountRepository) ListWithFilters(ctx context.Context, params pagination.PaginationParams, filters service.AccountListFilters) ([]service.Account, *pagination.PaginationResult, error) {
	q, err := r.buildAccountFilterQuery(filters)
	if err != nil {
		return nil, nil, err
	}

	total, err := q.Count(ctx)
	if err != nil {
		return nil, nil, err
	}

	accounts, err := q.
		Offset(params.Offset()).
		Limit(params.Limit()).
		Order(buildAccountOrderOptions(params.SortBy, params.SortOrder)...).
		All(ctx)
	if err != nil {
		return nil, nil, err
	}

	outAccounts, err := r.accountsToService(ctx, accounts)
	if err != nil {
		return nil, nil, err
	}
	return outAccounts, paginationResultFromTotal(int64(total), params), nil
}

func exactAccountGroupSetPredicate(groupIDs []int64) dbpredicate.Account {
	normalized := normalizePositiveInt64s(groupIDs)
	return dbpredicate.Account(func(s *entsql.Selector) {
		if len(normalized) == 0 {
			s.Where(entsql.False())
			return
		}
		s.Where(entsql.In(s.C(dbaccount.FieldID), exactAccountGroupSetSubquery(normalized)))
	})
}

func exactAccountGroupSetSubquery(groupIDs []int64) *entsql.Selector {
	normalized := normalizePositiveInt64s(groupIDs)
	accountGroups := entsql.Table(dbaccountgroup.Table)
	return entsql.Dialect(dialect.Postgres).
		Select(accountGroups.C(dbaccountgroup.FieldAccountID)).
		From(accountGroups).
		GroupBy(accountGroups.C(dbaccountgroup.FieldAccountID)).
		Having(entsql.P(func(b *entsql.Builder) {
			b.WriteString("COUNT(*) = ").Arg(len(normalized))
			b.WriteString(" AND COUNT(CASE WHEN group_id = ANY(").Arg(pq.Array(normalized))
			b.WriteString(") THEN 1 END) = ").Arg(len(normalized))
		}))
}
