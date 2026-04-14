package service

import "time"

// OpsRealtimeTrafficSummary is a lightweight summary used by the Ops dashboard "Realtime Traffic" card.
// It reports QPS/TPS current/peak/avg for the requested time window, plus raw request counts for
// the last minute and the selected window.
type OpsRealtimeTrafficSummary struct {
	// Window is a normalized label (e.g. "1min", "5min", "30min", "1h").
	Window string `json:"window"`

	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`

	Platform string `json:"platform"`
	GroupID  *int64 `json:"group_id"`

	// RecentRequestCount / RecentErrorCount expose the raw last-1-minute counts
	// used to derive current QPS. They are intentionally unrounded so the frontend
	// can distinguish low-traffic activity from true idle state.
	RecentRequestCount int64 `json:"recent_request_count"`
	RecentErrorCount   int64 `json:"recent_error_count"`

	// RequestCountTotal exposes the raw request count for the selected window.
	// Frontends can use it to recompute a minute-based average without changing
	// the current/peak semantics below.
	RequestCountTotal int64 `json:"request_count_total"`

	QPS OpsRateSummary `json:"qps"`
	TPS OpsRateSummary `json:"tps"`
}
