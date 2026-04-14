package repository

import (
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	dbaccount "github.com/Wei-Shaw/sub2api/ent/account"
	"github.com/stretchr/testify/require"
)

func TestExactAccountGroupSetPredicateUsesSubquery(t *testing.T) {
	selector := entsql.Dialect(dialect.Postgres).Select().From(entsql.Table(dbaccount.Table))

	exactAccountGroupSetPredicate([]int64{42, 42, 0, -1, 7})(selector)
	query, args := selector.Query()

	require.Contains(t, query, `FROM "accounts" WHERE "accounts"."id" IN (SELECT`)
	require.Contains(t, query, `FROM "account_groups"`)
	require.Contains(t, query, `GROUP BY "account_groups"."account_id"`)
	require.Contains(t, query, `COUNT(CASE WHEN group_id = ANY($2) THEN 1 END) = $3`)
	require.Len(t, args, 3)
	require.Equal(t, 2, args[0])
	require.Equal(t, 2, args[2])
}
