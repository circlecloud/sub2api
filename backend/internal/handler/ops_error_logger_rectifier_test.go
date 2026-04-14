package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type opsErrorLoggerSettingRepoStub struct {
	values map[string]string
}

func (s *opsErrorLoggerSettingRepoStub) Get(ctx context.Context, key string) (*service.Setting, error) {
	if v, ok := s.values[key]; ok {
		return &service.Setting{Key: key, Value: v}, nil
	}
	return nil, service.ErrSettingNotFound
}

func (s *opsErrorLoggerSettingRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	if v, ok := s.values[key]; ok {
		return v, nil
	}
	return "", service.ErrSettingNotFound
}

func (s *opsErrorLoggerSettingRepoStub) Set(ctx context.Context, key, value string) error {
	if s.values == nil {
		s.values = map[string]string{}
	}
	s.values[key] = value
	return nil
}

func (s *opsErrorLoggerSettingRepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if v, ok := s.values[key]; ok {
			out[key] = v
		}
	}
	return out, nil
}

func (s *opsErrorLoggerSettingRepoStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	if s.values == nil {
		s.values = map[string]string{}
	}
	for k, v := range settings {
		s.values[k] = v
	}
	return nil
}

func (s *opsErrorLoggerSettingRepoStub) GetAll(ctx context.Context) (map[string]string, error) {
	out := make(map[string]string, len(s.values))
	for k, v := range s.values {
		out[k] = v
	}
	return out, nil
}

func (s *opsErrorLoggerSettingRepoStub) Delete(ctx context.Context, key string) error {
	if s.values == nil {
		return nil
	}
	delete(s.values, key)
	return nil
}

func newOpsServiceWithRectifierTimeoutIgnore(t *testing.T, ignore bool) *service.OpsService {
	t.Helper()
	cfg := map[string]any{
		"data_retention": map[string]any{
			"cleanup_enabled":               false,
			"cleanup_schedule":              "0 2 * * *",
			"error_log_retention_days":      30,
			"minute_metrics_retention_days": 30,
			"hourly_metrics_retention_days": 30,
		},
		"aggregation": map[string]any{
			"aggregation_enabled": false,
		},
		"ignore_count_tokens_errors":              true,
		"ignore_context_canceled":                 true,
		"ignore_no_available_accounts":            false,
		"ignore_invalid_api_key_errors":           false,
		"ignore_insufficient_balance_errors":      false,
		"ignore_openai_stream_rectifier_timeouts": ignore,
		"display_openai_token_stats":              false,
		"display_alert_events":                    true,
		"auto_refresh_enabled":                    false,
		"auto_refresh_interval_seconds":           30,
	}
	raw, err := json.Marshal(cfg)
	require.NoError(t, err)
	repo := &opsErrorLoggerSettingRepoStub{values: map[string]string{
		service.SettingKeyOpsAdvancedSettings: string(raw),
	}}
	return service.NewOpsService(nil, repo, nil, nil, nil, nil, nil, nil, nil, nil, nil)
}

func TestShouldSkipOpsErrorLog_IgnoresRectifierTimeoutWhenEnabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	service.MarkOpsRectifierTimeoutExhausted(c)

	ops := newOpsServiceWithRectifierTimeoutIgnore(t, true)
	require.True(t, shouldSkipOpsErrorLog(c, ops, "Gateway timeout while waiting for upstream response", "", "/openai/v1/responses"))
}

func TestShouldSkipOpsErrorLog_KeepsRectifierTimeoutWhenDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	service.MarkOpsRectifierTimeoutExhausted(c)

	ops := newOpsServiceWithRectifierTimeoutIgnore(t, false)
	require.False(t, shouldSkipOpsErrorLog(c, ops, "Gateway timeout while waiting for upstream response", "", "/openai/v1/responses"))
}

func TestShouldSkipOpsErrorLog_NonRectifierErrorUnaffected(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)

	ops := newOpsServiceWithRectifierTimeoutIgnore(t, true)
	require.False(t, shouldSkipOpsErrorLog(c, ops, "generic upstream error", "", "/openai/v1/responses"))
}

var _ service.SettingRepository = (*opsErrorLoggerSettingRepoStub)(nil)
