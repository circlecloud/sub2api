package repository

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func capacitySnapshotExtraIntExpr(key string) string {
	trimmedValue := "btrim(COALESCE(a.extra->>'" + key + "', ''))"
	return "CASE " +
		"WHEN jsonb_typeof(a.extra->'" + key + "') = 'number' THEN trunc((a.extra->>'" + key + "')::numeric)::int " +
		"WHEN " + trimmedValue + " ~ '^[+-]?[0-9]+$' THEN " + trimmedValue + "::int " +
		"ELSE 0 END"
}

func (r *accountRepository) listGroupCapacitySnapshotAccounts(ctx context.Context, groupID int64) ([]service.GroupCapacitySnapshotAccountRecord, error) {
	if r == nil || r.sql == nil || groupID <= 0 {
		return []service.GroupCapacitySnapshotAccountRecord{}, nil
	}
	now := time.Now()
	query := `
		SELECT
			ag.account_id,
			a.concurrency,
			` + capacitySnapshotExtraIntExpr("max_sessions") + ` AS max_sessions,
			` + capacitySnapshotExtraIntExpr("session_idle_timeout_minutes") + ` AS session_idle_timeout_minutes,
			` + capacitySnapshotExtraIntExpr("base_rpm") + ` AS base_rpm
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
	`
	rows, err := r.sql.QueryContext(ctx, query, groupID, service.StatusActive, now)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	records := make([]service.GroupCapacitySnapshotAccountRecord, 0)
	for rows.Next() {
		var record service.GroupCapacitySnapshotAccountRecord
		if err := rows.Scan(
			&record.AccountID,
			&record.Concurrency,
			&record.MaxSessions,
			&record.SessionIdleTimeoutMinutes,
			&record.BaseRPM,
		); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return records, nil
}
