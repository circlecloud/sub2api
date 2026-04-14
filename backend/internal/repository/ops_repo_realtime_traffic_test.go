package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestOpsRepositoryGetRealtimeTrafficSummary_PopulatesRecentRequestCounts(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &opsRepository{db: db}

	start := time.Date(2026, 4, 9, 9, 0, 0, 0, time.UTC)
	end := start.Add(5 * time.Minute)
	filter := &service.OpsDashboardFilter{
		StartTime: start,
		EndTime:   end,
	}

	mock.ExpectQuery(`WITH usage_buckets AS`).
		WithArgs(start, end, start, end).
		WillReturnRows(sqlmock.NewRows([]string{
			"success_total",
			"error_total",
			"token_total",
			"peak_requests_per_min",
			"peak_tokens_per_min",
		}).AddRow(int64(12), int64(3), int64(600), int64(7), int64(300)))

	mock.ExpectQuery(`SELECT COALESCE\(COUNT\(\*\), 0\) AS success_count, COALESCE\(SUM\(input_tokens \+ output_tokens \+ cache_creation_tokens \+ cache_read_tokens\), 0\) AS token_consumed`).
		WithArgs(end.Add(-1*time.Minute), end).
		WillReturnRows(sqlmock.NewRows([]string{"count", "token_sum"}).AddRow(int64(2), int64(120)))

	mock.ExpectQuery(`SELECT COALESCE\(COUNT\(\*\) FILTER \(WHERE COALESCE\(status_code, 0\) >= 400\), 0\) AS error_total`).
		WithArgs(end.Add(-1*time.Minute), end).
		WillReturnRows(sqlmock.NewRows([]string{
			"error_total",
			"business_limited",
			"error_count_sla",
			"upstream_excl",
			"upstream_429",
			"upstream_529",
		}).AddRow(int64(1), int64(0), int64(1), int64(1), int64(0), int64(0)))

	summary, err := repo.GetRealtimeTrafficSummary(context.Background(), filter)
	require.NoError(t, err)
	require.NotNil(t, summary)
	require.Equal(t, int64(15), summary.RequestCountTotal)
	require.Equal(t, int64(3), summary.RecentRequestCount)
	require.Equal(t, int64(1), summary.RecentErrorCount)
	require.Equal(t, 0.1, summary.QPS.Current)
	require.Equal(t, 2.0, summary.TPS.Current)
	require.NoError(t, mock.ExpectationsWereMet())
}
