//go:build unit

package repository

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestSafeDateFormat(t *testing.T) {
	tests := []struct {
		name        string
		granularity string
		expected    string
	}{
		// 合法值
		{"hour", "hour", "YYYY-MM-DD HH24:00"},
		{"day", "day", "YYYY-MM-DD"},
		{"week", "week", "IYYY-IW"},
		{"month", "month", "YYYY-MM"},

		// 非法值回退到默认
		{"空字符串", "", "YYYY-MM-DD"},
		{"未知粒度 year", "year", "YYYY-MM-DD"},
		{"未知粒度 minute", "minute", "YYYY-MM-DD"},

		// 恶意字符串
		{"SQL 注入尝试", "'; DROP TABLE users; --", "YYYY-MM-DD"},
		{"带引号", "day'", "YYYY-MM-DD"},
		{"带括号", "day)", "YYYY-MM-DD"},
		{"Unicode", "日", "YYYY-MM-DD"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := safeDateFormat(tc.granularity)
			require.Equal(t, tc.expected, got, "safeDateFormat(%q)", tc.granularity)
		})
	}
}

func TestBuildUsageLogBatchInsertQuery_UsesConflictDoNothing(t *testing.T) {
	log := &service.UsageLog{
		UserID:       1,
		APIKeyID:     2,
		AccountID:    3,
		RequestID:    "req-batch-no-update",
		Model:        "gpt-5",
		InputTokens:  10,
		OutputTokens: 5,
		TotalCost:    1.2,
		ActualCost:   1.2,
		CreatedAt:    time.Now().UTC(),
	}
	prepared := prepareUsageLogInsert(log)

	query, _ := buildUsageLogBatchInsertQuery([]string{usageLogBatchKey(log.RequestID, log.APIKeyID)}, map[string]usageLogInsertPrepared{
		usageLogBatchKey(log.RequestID, log.APIKeyID): prepared,
	})

	require.Contains(t, query, "ON CONFLICT (request_id, api_key_id) DO NOTHING")
	require.NotContains(t, strings.ToUpper(query), "DO UPDATE")
}

func TestPrepareUsageLogInsert_IncludesAccountStatsCostAndSelectColumn(t *testing.T) {
	accountStatsCost := 1.23
	prepared := prepareUsageLogInsert(&service.UsageLog{
		UserID:           1,
		APIKeyID:         2,
		AccountID:        3,
		RequestID:        "req-account-stats-cost",
		Model:            "gpt-5",
		RequestedModel:   "gpt-5",
		InputTokens:      10,
		OutputTokens:     5,
		TotalCost:        1.2,
		ActualCost:       1.2,
		AccountStatsCost: &accountStatsCost,
		BillingType:      int8(service.BillingTypeBalance),
		RequestType:      service.RequestTypeSync,
		CreatedAt:        time.Now().UTC(),
	})

	require.Contains(t, usageLogSelectColumns, "account_stats_cost")
	require.Len(t, prepared.args, len(usageLogInsertArgTypes))

	got, ok := prepared.args[44].(sql.NullFloat64)
	require.True(t, ok)
	require.True(t, got.Valid)
	require.Equal(t, accountStatsCost, got.Float64)
}

func TestBuildUsageLogBatchInsertQuery_IncludesLatencyColumns(t *testing.T) {
	authLatency := 11
	routingLatency := 22
	gatewayPrepareLatency := 33
	upstreamLatency := 44
	streamFirstEvent := 55
	log := &service.UsageLog{
		UserID:                  1,
		APIKeyID:                2,
		AccountID:               3,
		RequestID:               "req-batch-latency",
		Model:                   "gpt-5",
		RequestedModel:          "gpt-5",
		InputTokens:             10,
		OutputTokens:            5,
		TotalCost:               1.2,
		ActualCost:              1.2,
		AuthLatencyMs:           &authLatency,
		RoutingLatencyMs:        &routingLatency,
		GatewayPrepareLatencyMs: &gatewayPrepareLatency,
		UpstreamLatencyMs:       &upstreamLatency,
		StreamFirstEventMs:      &streamFirstEvent,
		CreatedAt:               time.Now().UTC(),
	}
	prepared := prepareUsageLogInsert(log)
	batchQuery, args := buildUsageLogBatchInsertQuery([]string{usageLogBatchKey(log.RequestID, log.APIKeyID)}, map[string]usageLogInsertPrepared{
		usageLogBatchKey(log.RequestID, log.APIKeyID): prepared,
	})
	bestEffortQuery, bestEffortArgs := buildUsageLogBestEffortInsertQuery([]usageLogInsertPrepared{prepared})

	require.Contains(t, batchQuery, "account_stats_cost,\n\t\t\tauth_latency_ms")
	require.Contains(t, batchQuery, "stream_first_event_ms,\n\t\t\tcreated_at")
	require.Contains(t, bestEffortQuery, "account_stats_cost,\n\t\t\tauth_latency_ms")
	require.Contains(t, bestEffortQuery, "stream_first_event_ms,\n\t\t\tcreated_at")
	require.Len(t, args, 1+len(usageLogInsertArgTypes))
	require.Len(t, bestEffortArgs, len(usageLogInsertArgTypes))
}
