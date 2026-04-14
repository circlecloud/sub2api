package repository

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAccountRepository_ListGroupCapacitySnapshotAccounts_UsesTolerantLightweightQuery(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	repo := newAccountRepositoryWithSQL(nil, db, nil)
	rows := sqlmock.NewRows([]string{"account_id", "concurrency", "max_sessions", "session_idle_timeout_minutes", "base_rpm"}).
		AddRow(int64(11), 2, 4, 5, 10).
		AddRow(int64(12), 3, 0, 7, 0)

	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT
			ag.account_id,
			a.concurrency,
			`+expectedCapacitySnapshotExtraIntExpr("max_sessions")+` AS max_sessions,
			`+expectedCapacitySnapshotExtraIntExpr("session_idle_timeout_minutes")+` AS session_idle_timeout_minutes,
			`+expectedCapacitySnapshotExtraIntExpr("base_rpm")+` AS base_rpm
		FROM account_groups ag
		JOIN accounts a ON a.id = ag.account_id
		WHERE ag.group_id = $1
			AND a.deleted_at IS NULL
			AND a.status = $2
			AND a.schedulable = TRUE
			AND (a.temp_unschedulable_until IS NULL OR a.temp_unschedulable_until <= NOW())
			AND (a.expires_at IS NULL OR a.expires_at > $3 OR a.auto_pause_on_expired = FALSE)
			AND (a.overload_until IS NULL OR a.overload_until <= $3)
			AND (a.rate_limit_reset_at IS NULL OR a.rate_limit_reset_at <= $3)
		ORDER BY ag.priority ASC, a.priority ASC, ag.account_id ASC
	`)).
		WithArgs(int64(7), service.StatusActive, sqlmock.AnyArg()).
		WillReturnRows(rows)

	records, err := repo.ListGroupCapacitySnapshotAccounts(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, []service.GroupCapacitySnapshotAccountRecord{
		{AccountID: 11, Concurrency: 2, MaxSessions: 4, SessionIdleTimeoutMinutes: 5, BaseRPM: 10},
		{AccountID: 12, Concurrency: 3, MaxSessions: 0, SessionIdleTimeoutMinutes: 7, BaseRPM: 0},
	}, records)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGroupRepository_GetGroupCapacitySnapshotGroup_UsesLightweightQuery(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	repo := newGroupRepositoryWithSQL(nil, db)
	rows := sqlmock.NewRows([]string{"id", "status"}).AddRow(int64(7), service.StatusActive)

	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT id, status
		FROM groups
		WHERE id = $1
			AND deleted_at IS NULL
	`)).
		WithArgs(int64(7)).
		WillReturnRows(rows)

	record, err := repo.GetGroupCapacitySnapshotGroup(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, &service.GroupCapacitySnapshotGroupRecord{GroupID: 7, Status: service.StatusActive}, record)
	require.NoError(t, mock.ExpectationsWereMet())
}

func expectedCapacitySnapshotExtraIntExpr(key string) string {
	return fmt.Sprintf(`CASE
				WHEN jsonb_typeof(a.extra->'%[1]s') = 'number' THEN trunc((a.extra->>'%[1]s')::numeric)::int
				WHEN btrim(COALESCE(a.extra->>'%[1]s', '')) ~ '^[+-]?[0-9]+$' THEN btrim(COALESCE(a.extra->>'%[1]s', ''))::int
				ELSE 0
			END`, key)
}
