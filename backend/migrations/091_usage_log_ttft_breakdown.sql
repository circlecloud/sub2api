ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS auth_latency_ms INTEGER,
    ADD COLUMN IF NOT EXISTS routing_latency_ms INTEGER,
    ADD COLUMN IF NOT EXISTS gateway_prepare_latency_ms INTEGER,
    ADD COLUMN IF NOT EXISTS upstream_latency_ms INTEGER,
    ADD COLUMN IF NOT EXISTS stream_first_event_ms INTEGER;

ALTER TABLE ops_error_logs
    ADD COLUMN IF NOT EXISTS gateway_prepare_latency_ms BIGINT,
    ADD COLUMN IF NOT EXISTS stream_first_event_latency_ms BIGINT;
