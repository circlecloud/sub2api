package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type opsIgnoreSettingRepoStub struct {
	value string
}

func (s *opsIgnoreSettingRepoStub) Get(ctx context.Context, key string) (*service.Setting, error) {
	return nil, nil
}

func (s *opsIgnoreSettingRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	if key == service.SettingKeyOpsAdvancedSettings {
		return s.value, nil
	}
	return "", nil
}

func (s *opsIgnoreSettingRepoStub) Set(ctx context.Context, key, value string) error {
	if key == service.SettingKeyOpsAdvancedSettings {
		s.value = value
	}
	return nil
}

func (s *opsIgnoreSettingRepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	result := make(map[string]string, len(keys))
	for _, key := range keys {
		value, _ := s.GetValue(ctx, key)
		result[key] = value
	}
	return result, nil
}

func (s *opsIgnoreSettingRepoStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	for key, value := range settings {
		if err := s.Set(ctx, key, value); err != nil {
			return err
		}
	}
	return nil
}

func (s *opsIgnoreSettingRepoStub) GetAll(ctx context.Context) (map[string]string, error) {
	return map[string]string{service.SettingKeyOpsAdvancedSettings: s.value}, nil
}

func (s *opsIgnoreSettingRepoStub) Delete(ctx context.Context, key string) error {
	if key == service.SettingKeyOpsAdvancedSettings {
		s.value = ""
	}
	return nil
}

func resetOpsErrorLoggerStateForTest(t *testing.T) {
	t.Helper()

	opsErrorLogMu.Lock()
	ch := opsErrorLogQueue
	opsErrorLogQueue = nil
	opsErrorLogStopping = true
	opsErrorLogMu.Unlock()

	if ch != nil {
		close(ch)
	}
	opsErrorLogWorkersWg.Wait()

	opsErrorLogOnce = sync.Once{}
	opsErrorLogStopOnce = sync.Once{}
	opsErrorLogWorkersWg = sync.WaitGroup{}
	opsErrorLogMu = sync.RWMutex{}
	opsErrorLogStopping = false

	opsErrorLogQueueLen.Store(0)
	opsErrorLogEnqueued.Store(0)
	opsErrorLogDropped.Store(0)
	opsErrorLogProcessed.Store(0)
	opsErrorLogSanitized.Store(0)
	opsErrorLogLastDropLogAt.Store(0)

	opsErrorLogShutdownCh = make(chan struct{})
	opsErrorLogShutdownOnce = sync.Once{}
	opsErrorLogDrained.Store(false)
}

func TestAttachOpsRequestBodyToEntry_SanitizeAndTrim(t *testing.T) {
	resetOpsErrorLoggerStateForTest(t)
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	raw := []byte(`{"access_token":"secret-token","messages":[{"role":"user","content":"hello"}]}`)
	setOpsRequestContext(c, "claude-3", false, raw)

	entry := &service.OpsInsertErrorLogInput{}
	attachOpsRequestBodyToEntry(c, entry)

	require.NotNil(t, entry.RequestBodyBytes)
	require.Equal(t, len(raw), *entry.RequestBodyBytes)
	require.NotNil(t, entry.RequestBodyJSON)
	require.NotContains(t, *entry.RequestBodyJSON, "secret-token")
	require.Contains(t, *entry.RequestBodyJSON, "[REDACTED]")
	require.Equal(t, int64(1), OpsErrorLogSanitizedTotal())
}

func TestAttachOpsRequestBodyToEntry_InvalidJSONKeepsSize(t *testing.T) {
	resetOpsErrorLoggerStateForTest(t)
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	raw := []byte("not-json")
	setOpsRequestContext(c, "claude-3", false, raw)

	entry := &service.OpsInsertErrorLogInput{}
	attachOpsRequestBodyToEntry(c, entry)

	require.Nil(t, entry.RequestBodyJSON)
	require.NotNil(t, entry.RequestBodyBytes)
	require.Equal(t, len(raw), *entry.RequestBodyBytes)
	require.False(t, entry.RequestBodyTruncated)
	require.Equal(t, int64(1), OpsErrorLogSanitizedTotal())
}

func TestAttachOpsOpenAIAccountSelectFailure_MergesStructuredDetailsIntoErrorBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	service.SetOpsOpenAIAccountSelectFailure(c, service.OpenAIAccountScheduleDecision{
		Layer:                  "load_balance",
		FailureReason:          "warm_pool_empty",
		FailureDetail:          "schedulable_accounts=0",
		CandidateCount:         0,
		WarmPoolTried:          true,
		WarmPoolCandidateCount: 0,
	}, service.ErrNoAvailableAccounts, 2)

	entry := &service.OpsInsertErrorLogInput{ErrorBody: `{"error":{"message":"Service temporarily unavailable"}}`}
	attachOpsOpenAIAccountSelectFailure(c, entry)

	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(entry.ErrorBody), &payload))
	selection, ok := payload["openai_account_select_failure"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "warm_pool_empty", selection["failure_reason"])
	require.Equal(t, "schedulable_accounts=0", selection["failure_detail"])
	require.EqualValues(t, 2, selection["excluded_account_count"])
}

func TestEnqueueOpsErrorLog_QueueFullDrop(t *testing.T) {
	resetOpsErrorLoggerStateForTest(t)

	// 禁止 enqueueOpsErrorLog 触发 workers，使用测试队列验证满队列降级。
	opsErrorLogOnce.Do(func() {})

	opsErrorLogMu.Lock()
	opsErrorLogQueue = make(chan opsErrorLogJob, 1)
	opsErrorLogMu.Unlock()

	ops := service.NewOpsService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	entry := &service.OpsInsertErrorLogInput{ErrorPhase: "upstream", ErrorType: "upstream_error"}

	enqueueOpsErrorLog(ops, entry)
	enqueueOpsErrorLog(ops, entry)

	require.Equal(t, int64(1), OpsErrorLogEnqueuedTotal())
	require.Equal(t, int64(1), OpsErrorLogDroppedTotal())
	require.Equal(t, int64(1), OpsErrorLogQueueLength())
}

func TestApplyOpsRetryCountFromContext_UsesRectifierRetryTotal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)

	service.IncrementOpsRectifierRetryCount(c)
	service.IncrementOpsRectifierRetryCount(c)

	entry := &service.OpsInsertErrorLogInput{}
	applyOpsRetryCountFromContext(c, entry)

	require.Equal(t, 2, entry.RetryCount)
}

func TestApplyOpsRetryCountFromContext_DefaultZero(t *testing.T) {
	entry := &service.OpsInsertErrorLogInput{RetryCount: 99}
	applyOpsRetryCountFromContext(nil, entry)
	require.Equal(t, 99, entry.RetryCount)

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)

	entry = &service.OpsInsertErrorLogInput{RetryCount: 99}
	applyOpsRetryCountFromContext(c, entry)
	require.Equal(t, 0, entry.RetryCount)
}

func TestAttachOpsRequestBodyToEntry_EarlyReturnBranches(t *testing.T) {
	resetOpsErrorLoggerStateForTest(t)
	gin.SetMode(gin.TestMode)

	entry := &service.OpsInsertErrorLogInput{}
	attachOpsRequestBodyToEntry(nil, entry)
	attachOpsRequestBodyToEntry(&gin.Context{}, nil)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	// 无请求体 key
	attachOpsRequestBodyToEntry(c, entry)
	require.Nil(t, entry.RequestBodyJSON)
	require.Nil(t, entry.RequestBodyBytes)
	require.False(t, entry.RequestBodyTruncated)

	// 错误类型
	c.Set(opsRequestBodyKey, "not-bytes")
	attachOpsRequestBodyToEntry(c, entry)
	require.Nil(t, entry.RequestBodyJSON)
	require.Nil(t, entry.RequestBodyBytes)

	// 空 bytes
	c.Set(opsRequestBodyKey, []byte{})
	attachOpsRequestBodyToEntry(c, entry)
	require.Nil(t, entry.RequestBodyJSON)
	require.Nil(t, entry.RequestBodyBytes)

	require.Equal(t, int64(0), OpsErrorLogSanitizedTotal())
}

func TestEnqueueOpsErrorLog_EarlyReturnBranches(t *testing.T) {
	resetOpsErrorLoggerStateForTest(t)

	ops := service.NewOpsService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	entry := &service.OpsInsertErrorLogInput{ErrorPhase: "upstream", ErrorType: "upstream_error"}

	// nil 入参分支
	enqueueOpsErrorLog(nil, entry)
	enqueueOpsErrorLog(ops, nil)
	require.Equal(t, int64(0), OpsErrorLogEnqueuedTotal())

	// shutdown 分支
	close(opsErrorLogShutdownCh)
	enqueueOpsErrorLog(ops, entry)
	require.Equal(t, int64(0), OpsErrorLogEnqueuedTotal())

	// stopping 分支
	resetOpsErrorLoggerStateForTest(t)
	opsErrorLogMu.Lock()
	opsErrorLogStopping = true
	opsErrorLogMu.Unlock()
	enqueueOpsErrorLog(ops, entry)
	require.Equal(t, int64(0), OpsErrorLogEnqueuedTotal())

	// queue nil 分支（防止启动 worker 干扰）
	resetOpsErrorLoggerStateForTest(t)
	opsErrorLogOnce.Do(func() {})
	opsErrorLogMu.Lock()
	opsErrorLogQueue = nil
	opsErrorLogMu.Unlock()
	enqueueOpsErrorLog(ops, entry)
	require.Equal(t, int64(0), OpsErrorLogEnqueuedTotal())
}

func TestOpsCaptureWriterPool_ResetOnRelease(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/test", nil)

	writer := acquireOpsCaptureWriter(c.Writer)
	require.NotNil(t, writer)
	_, err := writer.buf.WriteString("temp-error-body")
	require.NoError(t, err)

	releaseOpsCaptureWriter(writer)

	reused := acquireOpsCaptureWriter(c.Writer)
	defer releaseOpsCaptureWriter(reused)

	require.Zero(t, reused.buf.Len(), "writer should be reset before reuse")
}

func TestOpsErrorLoggerMiddleware_RecoveredRectifierRetryCountEnqueuesRecoveredLog(t *testing.T) {
	resetOpsErrorLoggerStateForTest(t)
	gin.SetMode(gin.TestMode)

	opsErrorLogOnce.Do(func() {})
	opsErrorLogMu.Lock()
	opsErrorLogQueue = make(chan opsErrorLogJob, 1)
	opsErrorLogMu.Unlock()

	ops := service.NewOpsService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	r := gin.New()
	r.Use(OpsErrorLoggerMiddleware(ops))
	r.GET("/v1/responses", func(c *gin.Context) {
		c.Set(service.OpsUpstreamErrorsKey, []*service.OpsUpstreamErrorEvent{{
			Platform:           service.PlatformOpenAI,
			AccountID:          42,
			AccountName:        "sticky-openai",
			UpstreamStatusCode: http.StatusBadGateway,
			Kind:               "request_error:rectifier_timeout",
			Message:            "OpenAI response header timeout after 8s on header attempt 2",
			Detail:             "phase=response_header timeout_ms=8000 attempt=2 header_attempt=2 first_token_attempt=1",
		}})
		service.SetOpsRectifierRetryCount(c, 2)
		c.Status(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	select {
	case job := <-opsErrorLogQueue:
		require.NotNil(t, job.entry)
		require.Equal(t, 2, job.entry.RetryCount)
		require.Equal(t, http.StatusOK, job.entry.StatusCode)
		require.Equal(t, "Recovered upstream error 502: OpenAI response header timeout after 8s on header attempt 2", job.entry.ErrorMessage)
		require.Len(t, job.entry.UpstreamErrors, 1)
		require.Equal(t, "request_error:rectifier_timeout", job.entry.UpstreamErrors[0].Kind)
	default:
		t.Fatal("expected recovered upstream error log to be enqueued")
	}
}

func TestOpsErrorLoggerMiddleware_DoesNotBreakOuterMiddlewares(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(middleware2.Recovery())
	r.Use(middleware2.RequestLogger())
	r.Use(middleware2.Logger())
	r.GET("/v1/messages", OpsErrorLoggerMiddleware(nil), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/messages", nil)

	require.NotPanics(t, func() {
		r.ServeHTTP(rec, req)
	})
	require.Equal(t, http.StatusNoContent, rec.Code)
}

func TestIsKnownOpsErrorType(t *testing.T) {
	known := []string{
		"invalid_request_error",
		"authentication_error",
		"rate_limit_error",
		"billing_error",
		"subscription_error",
		"upstream_error",
		"overloaded_error",
		"api_error",
		"not_found_error",
		"forbidden_error",
	}
	for _, k := range known {
		require.True(t, isKnownOpsErrorType(k), "expected known: %s", k)
	}

	unknown := []string{"<nil>", "null", "", "random_error", "some_new_type", "<nil>\u003e"}
	for _, u := range unknown {
		require.False(t, isKnownOpsErrorType(u), "expected unknown: %q", u)
	}
}

func TestNormalizeOpsErrorType(t *testing.T) {
	tests := []struct {
		name    string
		errType string
		code    string
		want    string
	}{
		// Known types pass through.
		{"known invalid_request_error", "invalid_request_error", "", "invalid_request_error"},
		{"known rate_limit_error", "rate_limit_error", "", "rate_limit_error"},
		{"known upstream_error", "upstream_error", "", "upstream_error"},

		// Unknown/garbage types are rejected and fall through to code-based or default.
		{"nil literal from upstream", "<nil>", "", "api_error"},
		{"null string", "null", "", "api_error"},
		{"random string", "something_weird", "", "api_error"},

		// Unknown type but known code still maps correctly.
		{"nil with INSUFFICIENT_BALANCE code", "<nil>", "INSUFFICIENT_BALANCE", "billing_error"},
		{"nil with USAGE_LIMIT_EXCEEDED code", "<nil>", "USAGE_LIMIT_EXCEEDED", "subscription_error"},
		{"nil with API_KEY_QUOTA_EXHAUSTED code", "<nil>", "API_KEY_QUOTA_EXHAUSTED", "subscription_error"},

		// Empty type falls through to code-based mapping.
		{"empty type with balance code", "", "INSUFFICIENT_BALANCE", "billing_error"},
		{"empty type with subscription code", "", "SUBSCRIPTION_NOT_FOUND", "subscription_error"},
		{"empty type no code", "", "", "api_error"},

		// Known type overrides conflicting code-based mapping.
		{"known type overrides conflicting code", "rate_limit_error", "INSUFFICIENT_BALANCE", "rate_limit_error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeOpsErrorType(tt.errType, tt.code)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestClassifyOpsIsBusinessLimited(t *testing.T) {
	require.True(t, classifyOpsIsBusinessLimited("api_error", "internal", "API_KEY_QUOTA_EXHAUSTED", 429, "quota exhausted"))
	require.True(t, classifyOpsIsBusinessLimited("subscription_error", "request", "USAGE_LIMIT_EXCEEDED", 429, "usage limit exceeded"))
	require.False(t, classifyOpsIsBusinessLimited("api_error", "routing", "", 503, "No available accounts"))
	require.False(t, classifyOpsIsBusinessLimited("rate_limit_error", "upstream", "", 429, "upstream rate limit"))
}

func TestShouldSkipOpsErrorLog_IgnoresNoAvailableAccountsWhenEnabled(t *testing.T) {
	repo := &opsIgnoreSettingRepoStub{value: `{"ignore_no_available_accounts":true}`}
	ops := service.NewOpsService(nil, repo, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	require.True(t, shouldSkipOpsErrorLog(nil, ops, "No available accounts", "", "/v1/chat/completions"))
	require.True(t, shouldSkipOpsErrorLog(nil, ops, "", `{"error":"No available accounts"}`, "/v1/chat/completions"))
}

func TestUsageLimitExceededNestedErrorCodeExcludedFromSLA(t *testing.T) {
	body := []byte(`{"error":{"code":"USAGE_LIMIT_EXCEEDED","message":"error: code=429 reason=\"DAILY_LIMIT_EXCEEDED\" message=\"daily usage limit exceeded\" metadata=map[]"}}`)

	parsed := parseOpsErrorResponse(body)
	normalizedType := normalizeOpsErrorType(parsed.ErrorType, parsed.Code)
	phase := classifyOpsPhase(normalizedType, parsed.Message, parsed.Code)

	require.True(t, classifyOpsIsBusinessLimited(normalizedType, phase, parsed.Code, 429, parsed.Message))
}

func TestUsageLimitExceededNestedErrorTypeExcludedFromSLA(t *testing.T) {
	body := []byte(`{"error":{"type":"USAGE_LIMIT_EXCEEDED","message":"error: code=429 reason=\"DAILY_LIMIT_EXCEEDED\" message=\"daily usage limit exceeded\" metadata=map[]"}}`)

	parsed := parseOpsErrorResponse(body)
	normalizedType := normalizeOpsErrorType(parsed.ErrorType, parsed.Code)
	phase := classifyOpsPhase(normalizedType, parsed.Message, parsed.Code)

	require.True(t, classifyOpsIsBusinessLimited(normalizedType, phase, parsed.Code, 429, parsed.Message))
}

func TestSetOpsEndpointContext_SetsContextKeys(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	setOpsEndpointContext(c, "claude-3-5-sonnet-20241022", int16(2)) // stream

	v, ok := c.Get(opsUpstreamModelKey)
	require.True(t, ok)
	vStr, ok := v.(string)
	require.True(t, ok)
	require.Equal(t, "claude-3-5-sonnet-20241022", vStr)

	rt, ok := c.Get(opsRequestTypeKey)
	require.True(t, ok)
	rtVal, ok := rt.(int16)
	require.True(t, ok)
	require.Equal(t, int16(2), rtVal)
}

func TestSetOpsEndpointContext_EmptyModelNotStored(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	setOpsEndpointContext(c, "", int16(1))

	_, ok := c.Get(opsUpstreamModelKey)
	require.False(t, ok, "empty upstream model should not be stored")

	rt, ok := c.Get(opsRequestTypeKey)
	require.True(t, ok)
	rtVal, ok := rt.(int16)
	require.True(t, ok)
	require.Equal(t, int16(1), rtVal)
}

func TestSetOpsEndpointContext_NilContext(t *testing.T) {
	require.NotPanics(t, func() {
		setOpsEndpointContext(nil, "model", int16(1))
	})
}
