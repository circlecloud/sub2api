package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestOpsRepositoryGetDashboardOverviewRaw_IncludesRectifierRetryCount(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &opsRepository{db: db}

	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(10 * time.Minute)
	filter := &service.OpsDashboardFilter{
		StartTime: start,
		EndTime:   end,
		Platform:  service.PlatformOpenAI,
		QueryMode: service.OpsQueryModeRaw,
	}

	mock.ExpectQuery(`SELECT\s+COALESCE\(COUNT\(\*\), 0\) AS success_count`).
		WillReturnRows(sqlmock.NewRows([]string{"success_count", "token_consumed"}).AddRow(int64(5), int64(100)))

	mock.ExpectQuery(`SELECT\s+percentile_cont\(0\.50\).*FROM usage_logs ul`).
		WillReturnRows(sqlmock.NewRows([]string{
			"duration_p50", "duration_p90", "duration_p95", "duration_p99", "duration_avg", "duration_max",
			"ttft_p50", "ttft_p90", "ttft_p95", "ttft_p99", "ttft_avg", "ttft_max",
		}).AddRow(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil))

	mock.ExpectQuery(`SELECT[\s\S]*COALESCE\(retry_count, 0\) = 0[\s\S]*AS upstream_excl`).
		WillReturnRows(sqlmock.NewRows([]string{
			"error_total", "business_limited", "error_sla", "upstream_excl", "upstream_429", "upstream_529",
		}).AddRow(int64(2), int64(0), int64(2), int64(1), int64(1), int64(0)))

	mock.ExpectQuery(`SELECT\s+COALESCE\(SUM\(COALESCE\(retry_count, 0\)\), 0\) AS rectifier_retry_count`).
		WillReturnRows(sqlmock.NewRows([]string{"rectifier_retry_count"}).AddRow(int64(7)))

	mock.ExpectQuery(`SELECT\s+COALESCE\(COUNT\(\*\), 0\) AS success_count`).
		WillReturnRows(sqlmock.NewRows([]string{"success_count", "token_consumed"}).AddRow(int64(1), int64(20)))

	mock.ExpectQuery(`SELECT[\s\S]*COALESCE\(retry_count, 0\) = 0[\s\S]*AS upstream_excl`).
		WillReturnRows(sqlmock.NewRows([]string{
			"error_total", "business_limited", "error_sla", "upstream_excl", "upstream_429", "upstream_529",
		}).AddRow(int64(1), int64(0), int64(1), int64(0), int64(1), int64(0)))

	mock.ExpectQuery(`WITH usage_buckets AS \(`).
		WillReturnRows(sqlmock.NewRows([]string{"max_req_per_min", "max_tokens_per_min"}).AddRow(int64(6), int64(120)))

	overview, err := repo.GetDashboardOverview(context.Background(), filter)
	require.NoError(t, err)
	require.NotNil(t, overview)
	require.EqualValues(t, 7, overview.RectifierRetryCount)
	require.NoError(t, mock.ExpectationsWereMet())
}
