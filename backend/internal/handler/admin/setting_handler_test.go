package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type settingHandlerRepoStub struct {
	values         map[string]string
	getMultipleErr error
}

func newSettingHandlerRepoStub(values map[string]string) *settingHandlerRepoStub {
	copied := make(map[string]string, len(values))
	for key, value := range values {
		copied[key] = value
	}
	return &settingHandlerRepoStub{values: copied}
}

func (s *settingHandlerRepoStub) Get(ctx context.Context, key string) (*service.Setting, error) {
	value, ok := s.values[key]
	if !ok {
		return nil, service.ErrSettingNotFound
	}
	return &service.Setting{Key: key, Value: value}, nil
}

func (s *settingHandlerRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	value, ok := s.values[key]
	if !ok {
		return "", service.ErrSettingNotFound
	}
	return value, nil
}

func (s *settingHandlerRepoStub) Set(ctx context.Context, key, value string) error {
	if s.values == nil {
		s.values = make(map[string]string)
	}
	s.values[key] = value
	return nil
}

func (s *settingHandlerRepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	if s.getMultipleErr != nil {
		return nil, s.getMultipleErr
	}
	result := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			result[key] = value
		}
	}
	return result, nil
}

func (s *settingHandlerRepoStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	if s.values == nil {
		s.values = make(map[string]string, len(settings))
	}
	for key, value := range settings {
		s.values[key] = value
	}
	return nil
}

func (s *settingHandlerRepoStub) GetAll(ctx context.Context) (map[string]string, error) {
	result := make(map[string]string, len(s.values))
	for key, value := range s.values {
		result[key] = value
	}
	return result, nil
}

func (s *settingHandlerRepoStub) Delete(ctx context.Context, key string) error {
	delete(s.values, key)
	return nil
}

func TestSettingHandler_GetSettings_ReturnsOpenAIStreamRectifierFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newSettingHandlerRepoStub(map[string]string{
		service.SettingKeyEnableOpenAIStreamRectifier:                 "false",
		service.SettingKeyOpenAIStreamResponseHeaderRectifierTimeouts: `[11,13]`,
		service.SettingKeyOpenAIStreamFirstTokenRectifierTimeouts:     `[7,9]`,
	})
	handler := NewSettingHandler(service.NewSettingService(repo, &config.Config{Gateway: config.GatewayConfig{
		OpenAIStreamRectifierEnabled:                true,
		OpenAIStreamResponseHeaderRectifierTimeouts: []int{8, 10, 12},
		OpenAIStreamFirstTokenRectifierTimeouts:     []int{5, 8, 10},
	}}), nil, nil, nil, nil, nil)
	router := gin.New()
	router.GET("/api/v1/admin/settings", handler.GetSettings)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp response.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)

	payloadBytes, err := json.Marshal(resp.Data)
	require.NoError(t, err)
	var settings dto.SystemSettings
	require.NoError(t, json.Unmarshal(payloadBytes, &settings))
	require.False(t, settings.EnableOpenAIStreamRectifier)
	require.Equal(t, []int{11, 13}, settings.OpenAIStreamResponseHeaderRectifierTimeouts)
	require.Equal(t, []int{7, 9}, settings.OpenAIStreamFirstTokenRectifierTimeouts)
}

func TestSettingHandler_GetSettings_StartupGroupIDsAlwaysArray(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newSettingHandlerRepoStub(map[string]string{})
	handler := NewSettingHandler(service.NewSettingService(repo, &config.Config{}), nil, nil, nil, nil, nil)
	router := gin.New()
	router.GET("/api/v1/admin/settings", handler.GetSettings)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp response.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)

	payload, err := json.Marshal(resp.Data)
	require.NoError(t, err)

	var settings map[string]any
	require.NoError(t, json.Unmarshal(payload, &settings))
	value, exists := settings["openai_warm_pool_startup_group_ids"]
	require.True(t, exists, "GET /admin/settings 必须始终返回启动预热分组字段，前端依赖它渲染 GroupSelector")

	ids, ok := value.([]any)
	require.True(t, ok, "openai_warm_pool_startup_group_ids 必须返回数组而不是 null")
	require.Len(t, ids, 0)
}

func TestSettingHandler_GetSettings_PaymentConfigErrorReturnsFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newSettingHandlerRepoStub(map[string]string{})
	repo.getMultipleErr = context.DeadlineExceeded
	svc := service.NewSettingService(repo, &config.Config{})
	paymentConfigService := service.NewPaymentConfigService(nil, repo, nil)
	handler := NewSettingHandler(svc, nil, nil, nil, paymentConfigService, nil)
	router := gin.New()
	router.GET("/api/v1/admin/settings", handler.GetSettings)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings", nil)
	router.ServeHTTP(rec, req)

	require.NotEqual(t, http.StatusOK, rec.Code)
}

//nolint:unused // 保留统一的 settings 更新请求测试辅助函数
func TestSettingHandler_UpdateSettings_RoundTripsOpenAIStreamRectifierFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newSettingHandlerRepoStub(map[string]string{})
	svc := service.NewSettingService(repo, &config.Config{Gateway: config.GatewayConfig{
		OpenAIStreamRectifierEnabled:                true,
		OpenAIStreamResponseHeaderRectifierTimeouts: []int{8, 10, 12},
		OpenAIStreamFirstTokenRectifierTimeouts:     []int{5, 8, 10},
	}})
	handler := NewSettingHandler(svc, nil, nil, nil, nil, nil)
	router := gin.New()
	router.PUT("/api/v1/admin/settings", handler.UpdateSettings)

	settings := performUpdateSettingsRequest(t, router, map[string]any{
		"enable_openai_stream_rectifier":                   false,
		"openai_stream_response_header_rectifier_timeouts": []int{11, 13},
		"openai_stream_first_token_rectifier_timeouts":     []int{7, 9},
	})

	require.False(t, settings.EnableOpenAIStreamRectifier)
	require.Equal(t, []int{11, 13}, settings.OpenAIStreamResponseHeaderRectifierTimeouts)
	require.Equal(t, []int{7, 9}, settings.OpenAIStreamFirstTokenRectifierTimeouts)
	require.Equal(t, "false", repo.values[service.SettingKeyEnableOpenAIStreamRectifier])
	require.Equal(t, `[11,13]`, repo.values[service.SettingKeyOpenAIStreamResponseHeaderRectifierTimeouts])
	require.Equal(t, `[7,9]`, repo.values[service.SettingKeyOpenAIStreamFirstTokenRectifierTimeouts])
}

func TestSettingHandler_UpdateSettings_RoundTripsNotifyAndPaymentFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newSettingHandlerRepoStub(map[string]string{})
	svc := service.NewSettingService(repo, &config.Config{})
	paymentConfigService := service.NewPaymentConfigService(nil, repo, nil)
	handler := NewSettingHandler(svc, nil, nil, nil, paymentConfigService, nil)
	router := gin.New()
	router.PUT("/api/v1/admin/settings", handler.UpdateSettings)

	settings := performUpdateSettingsRequest(t, router, map[string]any{
		"balance_low_notify_enabled":          true,
		"balance_low_notify_threshold":        12.5,
		"balance_low_notify_recharge_url":     "https://example.com/recharge",
		"account_quota_notify_enabled":        true,
		"account_quota_notify_emails":         []map[string]any{{"email": "ops@example.com", "verified": true, "disabled": false}},
		"payment_balance_recharge_multiplier": 1.5,
		"payment_recharge_fee_rate":           2.25,
	})

	require.True(t, settings.BalanceLowNotifyEnabled)
	require.Equal(t, 12.5, settings.BalanceLowNotifyThreshold)
	require.Equal(t, "https://example.com/recharge", settings.BalanceLowNotifyRechargeURL)
	require.True(t, settings.AccountQuotaNotifyEnabled)
	require.Len(t, settings.AccountQuotaNotifyEmails, 1)
	require.Equal(t, "ops@example.com", settings.AccountQuotaNotifyEmails[0].Email)
	require.Equal(t, 1.5, settings.PaymentBalanceRechargeMultiplier)
	require.Equal(t, 2.25, settings.PaymentRechargeFeeRate)

	require.Equal(t, "true", repo.values[service.SettingKeyBalanceLowNotifyEnabled])
	require.Equal(t, "12.50000000", repo.values[service.SettingKeyBalanceLowNotifyThreshold])
	require.Equal(t, "https://example.com/recharge", repo.values[service.SettingKeyBalanceLowNotifyRechargeURL])
	require.Equal(t, "true", repo.values[service.SettingKeyAccountQuotaNotifyEnabled])
	require.Equal(t, "1.50", repo.values[service.SettingBalanceRechargeMult])
	require.Equal(t, "2.25", repo.values[service.SettingRechargeFeeRate])
	require.Equal(t, `[{"email":"ops@example.com","disabled":false,"verified":true}]`, repo.values[service.SettingKeyAccountQuotaNotifyEmails])
}

func TestSettingHandler_UpdateSettings_PaymentConfigReloadErrorReturnsFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newSettingHandlerRepoStub(map[string]string{})
	svc := service.NewSettingService(repo, &config.Config{})
	paymentConfigService := service.NewPaymentConfigService(nil, repo, nil)
	handler := NewSettingHandler(svc, nil, nil, nil, paymentConfigService, nil)
	router := gin.New()
	router.PUT("/api/v1/admin/settings", handler.UpdateSettings)

	repo.getMultipleErr = context.DeadlineExceeded
	body, err := json.Marshal(map[string]any{
		"payment_balance_recharge_multiplier": 1.5,
	})
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.NotEqual(t, http.StatusOK, rec.Code)
}

func performUpdateSettingsRequest(t *testing.T, router *gin.Engine, payload map[string]any) dto.SystemSettings {
	t.Helper()

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp response.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)

	payloadBytes, err := json.Marshal(resp.Data)
	require.NoError(t, err)

	var settings dto.SystemSettings
	require.NoError(t, json.Unmarshal(payloadBytes, &settings))
	return settings
}
