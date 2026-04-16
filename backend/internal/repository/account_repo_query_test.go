package repository

import (
	"testing"

	entsql "entgo.io/ent/dialect/sql"
	dbaccount "github.com/Wei-Shaw/sub2api/ent/account"
	"github.com/stretchr/testify/require"
)

func TestBuildAccountOrderOptions_Usage7dRemainingAscUsesSnapshotOrder(t *testing.T) {
	selector := entsql.Dialect("postgres").Select("*").From(entsql.Table(dbaccount.Table))

	for _, option := range buildAccountOrderOptions("usage_7d_remaining", "asc") {
		option(selector)
	}

	query, args := selector.Query()
	require.Empty(t, args)
	require.Contains(t, query, `ORDER BY CASE WHEN "accounts"."platform" = 'openai' AND "accounts"."type" = 'oauth'`)
	require.Contains(t, query, `"accounts"."extra"->>'codex_7d_used_percent'`)
	require.Contains(t, query, `"accounts"."extra"->>'passive_usage_7d_utilization'`)
	require.Contains(t, query, `NULLS LAST, "accounts"."id" DESC`)
}

func TestBuildAccountOrderOptions_Usage7dRemainingDescUsesSnapshotOrder(t *testing.T) {
	selector := entsql.Dialect("postgres").Select("*").From(entsql.Table(dbaccount.Table))

	for _, option := range buildAccountOrderOptions("usage_7d_remaining", "desc") {
		option(selector)
	}

	query, args := selector.Query()
	require.Empty(t, args)
	require.Contains(t, query, `ORDER BY CASE WHEN "accounts"."platform" = 'openai' AND "accounts"."type" = 'oauth'`)
	require.Contains(t, query, `DESC NULLS LAST, "accounts"."id" DESC`)
}

func TestBuildAccountOrderOptions_StatusAscUsesRuntimeStatusOrder(t *testing.T) {
	selector := entsql.Dialect("postgres").Select("*").From(entsql.Table(dbaccount.Table))

	for _, option := range buildAccountOrderOptions("status", "asc") {
		option(selector)
	}

	query, args := selector.Query()
	require.Empty(t, args)
	require.Contains(t, query, `ORDER BY CASE WHEN "accounts"."status" = 'active' AND "accounts"."rate_limit_reset_at" IS NOT NULL AND "accounts"."rate_limit_reset_at" > NOW() THEN 0`)
	require.Contains(t, query, `WHEN "accounts"."status" = 'active' AND "accounts"."overload_until" IS NOT NULL AND "accounts"."overload_until" > NOW() THEN 1`)
	require.Contains(t, query, `WHEN "accounts"."status" = 'active' AND "accounts"."temp_unschedulable_until" IS NOT NULL AND "accounts"."temp_unschedulable_until" > NOW() THEN 2`)
	require.Contains(t, query, `WHEN "accounts"."status" = 'active' AND NOT "accounts"."schedulable" THEN 3`)
	require.Contains(t, query, `"accounts"."rate_limit_reset_at" NULLS LAST`)
	require.Contains(t, query, `"accounts"."overload_until" NULLS LAST`)
	require.Contains(t, query, `"accounts"."temp_unschedulable_until" NULLS LAST`)
}

func TestBuildAccountOrderOptions_StatusDescUsesRuntimeStatusOrder(t *testing.T) {
	selector := entsql.Dialect("postgres").Select("*").From(entsql.Table(dbaccount.Table))

	for _, option := range buildAccountOrderOptions("status", "desc") {
		option(selector)
	}

	query, args := selector.Query()
	require.Empty(t, args)
	require.Contains(t, query, `ORDER BY CASE WHEN "accounts"."status" = 'active' AND "accounts"."rate_limit_reset_at" IS NOT NULL AND "accounts"."rate_limit_reset_at" > NOW() THEN 0`)
	require.Contains(t, query, `"accounts"."rate_limit_reset_at" DESC NULLS LAST`)
	require.Contains(t, query, `"accounts"."overload_until" DESC NULLS LAST`)
	require.Contains(t, query, `"accounts"."temp_unschedulable_until" DESC NULLS LAST`)
}
