package service

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtractOpenAIWhamUsageSnapshotParsesUsageWindows(t *testing.T) {
	t.Parallel()

	updates, resetAt, err := extractOpenAIWhamUsageSnapshot(&http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body: io.NopCloser(strings.NewReader(`{
			"plan_type": "plus",
			"rate_limit": {
				"primary_window": {
					"used_percent": 100,
					"limit_window_seconds": 604800,
					"reset_after_seconds": 86400
				},
				"secondary_window": {
					"used_percent": 25,
					"limit_window_seconds": 18000,
					"reset_after_seconds": 3600
				}
			}
		}`)),
	})
	require.NoError(t, err)
	require.NotNil(t, updates)
	require.Equal(t, 100.0, updates["codex_7d_used_percent"])
	require.Equal(t, 25.0, updates["codex_5h_used_percent"])
	require.Equal(t, 10080, updates["codex_7d_window_minutes"])
	require.Equal(t, 300, updates["codex_5h_window_minutes"])
	require.NotNil(t, resetAt)
}
