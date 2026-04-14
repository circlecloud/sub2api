//go:build unit

package repository

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestOpsRepository_GetErrorLogByID_ReadsExtendedLatencyFields(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	repo := &opsRepository{db: db}
	now := time.Now().UTC()

	rows := sqlmock.NewRows([]string{
		"id",
		"created_at",
		"error_phase",
		"error_type",
		"error_owner",
		"error_source",
		"severity",
		"status_code",
		"platform",
		"model",
		"is_retryable",
		"retry_count",
		"resolved",
		"resolved_at",
		"resolved_by_user_id",
		"resolved_retry_id",
		"client_request_id",
		"request_id",
		"error_message",
		"error_body",
		"upstream_status_code",
		"upstream_error_message",
		"upstream_error_detail",
		"upstream_errors",
		"is_business_limited",
		"user_id",
		"user_email",
		"api_key_id",
		"account_id",
		"account_name",
		"group_id",
		"group_name",
		"client_ip",
		"request_path",
		"stream",
		"inbound_endpoint",
		"upstream_endpoint",
		"requested_model",
		"upstream_model",
		"request_type",
		"user_agent",
		"auth_latency_ms",
		"routing_latency_ms",
		"gateway_prepare_latency_ms",
		"upstream_latency_ms",
		"response_latency_ms",
		"time_to_first_token_ms",
		"stream_first_event_latency_ms",
		"request_body",
		"request_body_truncated",
		"request_body_bytes",
		"request_headers",
	}).AddRow(
		int64(1),
		now,
		"auth",
		"api_error",
		"upstream",
		"openai",
		"high",
		int64(401),
		"openai",
		"gpt-5",
		false,
		int64(0),
		false,
		sql.NullTime{},
		sql.NullInt64{},
		sql.NullInt64{},
		"client-req",
		"req-1",
		"boom",
		"body",
		sql.NullInt64{Int64: 401, Valid: true},
		"unauthorized",
		"detail",
		"[]",
		false,
		sql.NullInt64{Int64: 9, Valid: true},
		"user@example.com",
		sql.NullInt64{Int64: 8, Valid: true},
		sql.NullInt64{Int64: 7, Valid: true},
		"account-a",
		sql.NullInt64{Int64: 6, Valid: true},
		"group-a",
		sql.NullString{String: "127.0.0.1", Valid: true},
		"/v1/chat/completions",
		true,
		"/inbound",
		"/upstream",
		"gpt-5",
		"gpt-5-upstream",
		sql.NullInt64{Int64: 2, Valid: true},
		"ua",
		sql.NullInt64{Int64: 11, Valid: true},
		sql.NullInt64{Int64: 22, Valid: true},
		sql.NullInt64{Int64: 33, Valid: true},
		sql.NullInt64{Int64: 44, Valid: true},
		sql.NullInt64{Int64: 55, Valid: true},
		sql.NullInt64{Int64: 66, Valid: true},
		sql.NullInt64{Int64: 77, Valid: true},
		"{\"foo\":1}",
		false,
		sql.NullInt64{Int64: 123, Valid: true},
		"{\"x-test\":[\"1\"]}",
	)

	mock.ExpectQuery("SELECT").WithArgs(int64(1)).WillReturnRows(rows)

	out, err := repo.GetErrorLogByID(context.Background(), 1)
	require.NoError(t, err)
	require.NotNil(t, out.GatewayPrepareLatencyMs)
	require.Equal(t, int64(33), *out.GatewayPrepareLatencyMs)
	require.NotNil(t, out.StreamFirstEventLatencyMs)
	require.Equal(t, int64(77), *out.StreamFirstEventLatencyMs)
	require.NoError(t, mock.ExpectationsWereMet())
}
