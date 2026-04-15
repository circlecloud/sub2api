package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

type openAIHandlerAccountRepoStub struct {
	service.AccountRepository
	accounts []service.Account
}

func (r openAIHandlerAccountRepoStub) ListByGroup(ctx context.Context, groupID int64) ([]service.Account, error) {
	result := make([]service.Account, 0, len(r.accounts))
	for _, acc := range r.accounts {
		matched := groupID == 0
		for _, id := range acc.GroupIDs {
			if id == groupID {
				matched = true
				break
			}
		}
		if matched {
			result = append(result, acc)
		}
	}
	return result, nil
}

func (r openAIHandlerAccountRepoStub) ListByPlatform(ctx context.Context, platform string) ([]service.Account, error) {
	result := make([]service.Account, 0, len(r.accounts))
	for _, acc := range r.accounts {
		if acc.Platform == platform {
			result = append(result, acc)
		}
	}
	return result, nil
}

func (r openAIHandlerAccountRepoStub) ListWithFilters(ctx context.Context, params pagination.PaginationParams, filters service.AccountListFilters) ([]service.Account, *pagination.PaginationResult, error) {
	result := make([]service.Account, 0, len(r.accounts))
	for _, acc := range r.accounts {
		if filters.Platform != "" && acc.Platform != filters.Platform {
			continue
		}
		if filters.GroupIDs != "" {
			matched := false
			for _, id := range acc.GroupIDs {
				if filters.GroupIDs == fmt.Sprintf("%d", id) {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		result = append(result, acc)
	}
	return result, &pagination.PaginationResult{Total: int64(len(result)), Page: params.Page, PageSize: params.PageSize, Pages: 1}, nil
}

func TestOpenAIHandleStreamingAwareError_JSONEscaping(t *testing.T) {
	tests := []struct {
		name    string
		errType string
		message string
	}{
		{
			name:    "包含双引号的消息",
			errType: "server_error",
			message: `upstream returned "invalid" response`,
		},
		{
			name:    "包含反斜杠的消息",
			errType: "server_error",
			message: `path C:\Users\test\file.txt not found`,
		},
		{
			name:    "包含双引号和反斜杠的消息",
			errType: "upstream_error",
			message: `error parsing "key\value": unexpected token`,
		},
		{
			name:    "包含换行符的消息",
			errType: "server_error",
			message: "line1\nline2\ttab",
		},
		{
			name:    "普通消息",
			errType: "upstream_error",
			message: "Upstream service temporarily unavailable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

			h := &OpenAIGatewayHandler{}
			h.handleStreamingAwareError(c, http.StatusBadGateway, tt.errType, tt.message, true)

			body := w.Body.String()

			// 验证 SSE 格式：event: error\ndata: {JSON}\n\n
			assert.True(t, strings.HasPrefix(body, "event: error\n"), "应以 'event: error\\n' 开头")
			assert.True(t, strings.HasSuffix(body, "\n\n"), "应以 '\\n\\n' 结尾")

			// 提取 data 部分
			lines := strings.Split(strings.TrimSuffix(body, "\n\n"), "\n")
			require.Len(t, lines, 2, "应有 event 行和 data 行")
			dataLine := lines[1]
			require.True(t, strings.HasPrefix(dataLine, "data: "), "第二行应以 'data: ' 开头")
			jsonStr := strings.TrimPrefix(dataLine, "data: ")

			// 验证 JSON 合法性
			var parsed map[string]any
			err := json.Unmarshal([]byte(jsonStr), &parsed)
			require.NoError(t, err, "JSON 应能被成功解析，原始 JSON: %s", jsonStr)

			// 验证结构
			errorObj, ok := parsed["error"].(map[string]any)
			require.True(t, ok, "应包含 error 对象")
			assert.Equal(t, tt.errType, errorObj["type"])
			assert.Equal(t, tt.message, errorObj["message"])
		})
	}
}

func TestOpenAIHandleStreamingAwareError_NonStreaming(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	h := &OpenAIGatewayHandler{}
	h.handleStreamingAwareError(c, http.StatusBadGateway, "upstream_error", "test error", false)

	// 非流式应返回 JSON 响应
	assert.Equal(t, http.StatusBadGateway, w.Code)

	var parsed map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &parsed)
	require.NoError(t, err)
	errorObj, ok := parsed["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "upstream_error", errorObj["type"])
	assert.Equal(t, "test error", errorObj["message"])
}

func TestHandleRectifierTimeoutExhausted_WritesOpenAIErrorAndMarksOps(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	h := &OpenAIGatewayHandler{}
	timeoutErr := &service.OpenAIRectifierTimeoutError{
		StatusCode:        http.StatusGatewayTimeout,
		Phase:             "first_token",
		Attempt:           3,
		HeaderAttempt:     1,
		FirstTokenAttempt: 3,
	}
	h.handleRectifierTimeoutExhausted(c, timeoutErr, false)

	require.True(t, service.IsOpsRectifierTimeoutExhausted(c))
	require.Equal(t, http.StatusGatewayTimeout, w.Code)

	var parsed map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &parsed)
	require.NoError(t, err)
	errorObj, ok := parsed["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "upstream_error", errorObj["type"])
	assert.Equal(t, "Gateway timeout while waiting for upstream first token", errorObj["message"])
}

func TestHandleAnthropicRectifierTimeoutExhausted_WritesAnthropicErrorAndMarksOps(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	h := &OpenAIGatewayHandler{}
	timeoutErr := &service.OpenAIRectifierTimeoutError{
		StatusCode:        http.StatusGatewayTimeout,
		Phase:             "response_header",
		Attempt:           2,
		HeaderAttempt:     2,
		FirstTokenAttempt: 1,
	}
	h.handleAnthropicRectifierTimeoutExhausted(c, timeoutErr, false)

	require.True(t, service.IsOpsRectifierTimeoutExhausted(c))
	require.Equal(t, http.StatusGatewayTimeout, w.Code)

	var parsed map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &parsed)
	require.NoError(t, err)
	assert.Equal(t, "error", parsed["type"])
	errorObj, ok := parsed["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "upstream_error", errorObj["type"])
	assert.Equal(t, "Gateway timeout while waiting for upstream response headers", errorObj["message"])
}

func TestLogOpenAIRectifierTimeoutRetry_EmitsMergedLogWithTimeoutAndRetryAttempt(t *testing.T) {
	core, logs := observer.New(zap.WarnLevel)
	reqLog := zap.New(core)
	timeoutErr := &service.OpenAIRectifierTimeoutError{
		Phase:             "response_header",
		Attempt:           1,
		HeaderAttempt:     1,
		FirstTokenAttempt: 1,
		Timeout:           8 * time.Second,
	}
	retryState := newOpenAIRectifierAttemptState()
	retryState.advance(timeoutErr.Phase)

	logOpenAIRectifierTimeoutRetry(reqLog, "openai.rectifier_timeout_retry", 42, "7:openai:single", timeoutErr, retryState, 1, 2)

	entries := logs.All()
	require.Len(t, entries, 1)
	require.Equal(t, "openai.rectifier_timeout_retry", entries[0].Message)
	fields := entries[0].ContextMap()
	require.EqualValues(t, 42, fields["account_id"])
	require.Equal(t, "7:openai:single", fields["scheduler_bucket"])
	require.Equal(t, "response_header", fields["phase"])
	require.EqualValues(t, 2, fields["attempt"])
	require.EqualValues(t, 2, fields["retry_attempt"])
	require.EqualValues(t, 1, fields["timed_out_attempt"])
	require.EqualValues(t, 3, fields["max_attempts"])
	require.EqualValues(t, 1, fields["retry_count"])
	require.EqualValues(t, 2, fields["retry_limit"])
	require.EqualValues(t, 2, fields["header_attempt"])
	require.EqualValues(t, 1, fields["first_token_attempt"])
	require.EqualValues(t, 8000, fields["timeout_ms"])
	require.EqualValues(t, 8, fields["timeout_seconds"])
}

func TestLogOpenAIRectifierTimeoutExhausted_EmitsSchedulerBucketAndRetryFields(t *testing.T) {
	core, logs := observer.New(zap.WarnLevel)
	reqLog := zap.New(core)
	timeoutErr := &service.OpenAIRectifierTimeoutError{
		Phase:             "first_token",
		Attempt:           3,
		HeaderAttempt:     2,
		FirstTokenAttempt: 3,
		Timeout:           10 * time.Second,
	}

	logOpenAIRectifierTimeoutExhausted(reqLog, "openai.rectifier_timeout_exhausted", 42, "0:openai:single", timeoutErr, 2, 2)

	entries := logs.All()
	require.Len(t, entries, 1)
	require.Equal(t, "openai.rectifier_timeout_exhausted", entries[0].Message)
	fields := entries[0].ContextMap()
	require.EqualValues(t, 42, fields["account_id"])
	require.Equal(t, "0:openai:single", fields["scheduler_bucket"])
	require.Equal(t, "first_token", fields["phase"])
	require.EqualValues(t, 3, fields["attempt"])
	require.EqualValues(t, 2, fields["retry_count"])
	require.EqualValues(t, 2, fields["retry_limit"])
	require.EqualValues(t, 3, fields["max_attempts"])
	require.EqualValues(t, 2, fields["header_attempt"])
	require.EqualValues(t, 3, fields["first_token_attempt"])
	require.EqualValues(t, 10000, fields["timeout_ms"])
	require.EqualValues(t, 10, fields["timeout_seconds"])
}

func TestLogOpenAIAccountSelectFailure_EmitsFailureReasonFields(t *testing.T) {
	core, logs := observer.New(zap.WarnLevel)
	reqLog := zap.New(core)
	decision := service.OpenAIAccountScheduleDecision{
		Layer:                  "load_balance",
		CandidateCount:         0,
		WarmPoolTried:          true,
		WarmPoolCandidateCount: 0,
		FailureReason:          "warm_pool_empty",
		FailureDetail:          "schedulable_accounts=0 warm_pool_candidates=0",
	}

	logOpenAIAccountSelectFailure(nil, reqLog, "openai.account_select_failed", decision, service.ErrNoAvailableAccounts, 2)

	entries := logs.All()
	require.Len(t, entries, 1)
	require.Equal(t, "openai.account_select_failed", entries[0].Message)
	fields := entries[0].ContextMap()
	require.Equal(t, "load_balance", fields["layer"])
	require.EqualValues(t, 0, fields["candidate_count"])
	require.EqualValues(t, 2, fields["excluded_account_count"])
	require.EqualValues(t, true, fields["warm_pool_tried"])
	require.EqualValues(t, 0, fields["warm_pool_candidate_count"])
	require.Equal(t, "warm_pool_empty", fields["failure_reason"])
	require.Equal(t, "schedulable_accounts=0 warm_pool_candidates=0", fields["failure_detail"])
}

func TestLogOpenAIAccountSelectFailure_InfersReasonFromErrorWhenDecisionMissing(t *testing.T) {
	core, logs := observer.New(zap.WarnLevel)
	reqLog := zap.New(core)
	err := fmt.Errorf("no available OpenAI accounts supporting model: gpt-5.4: %w", service.ErrNoAvailableAccounts)

	logOpenAIAccountSelectFailure(nil, reqLog, "openai.account_select_failed", service.OpenAIAccountScheduleDecision{}, err, 0)

	entries := logs.All()
	require.Len(t, entries, 1)
	fields := entries[0].ContextMap()
	require.Equal(t, "no_available_accounts", fields["failure_reason"])
	require.Equal(t, err.Error(), fields["failure_detail"])
	require.Equal(t, err.Error(), fields["error"])
}

func TestBuildOpenAIAccountSelectClientMessage_PrefersScheduleDecisionDetails(t *testing.T) {
	decision := service.OpenAIAccountScheduleDecision{
		FailureReason: "warm_pool_empty",
		FailureDetail: "schedulable_accounts=0 warm_pool_candidates=0",
	}

	message := buildOpenAIAccountSelectClientMessage(decision, service.ErrNoAvailableAccounts)
	require.Equal(t, "schedulable_accounts=0 warm_pool_candidates=0", message)
}

func TestBuildOpenAIAccountSelectClientMessage_FallsBackToErrorText(t *testing.T) {
	err := fmt.Errorf("no available OpenAI accounts supporting model: gpt-5.4: %w", service.ErrNoAvailableAccounts)

	message := buildOpenAIAccountSelectClientMessage(service.OpenAIAccountScheduleDecision{}, err)
	require.Equal(t, err.Error(), message)
}

func TestBuildOpenAIAccountSelectClientMessage_DefaultsToGenericMessage(t *testing.T) {
	message := buildOpenAIAccountSelectClientMessage(service.OpenAIAccountScheduleDecision{}, nil)
	require.Equal(t, "Service temporarily unavailable", message)
}

func TestBuildOpenAIAccountSelectClientMessage_ExposesNonSchedulingInternalError(t *testing.T) {
	err := errors.New("redis unavailable")

	message := buildOpenAIAccountSelectClientMessage(service.OpenAIAccountScheduleDecision{}, err)
	require.Equal(t, err.Error(), message)
}

func TestResolveOpenAINoAvailableAccountMessage_PrefersAuthFailureFromTempUnsched401(t *testing.T) {
	future := time.Now().Add(5 * time.Minute)
	repo := openAIHandlerAccountRepoStub{accounts: []service.Account{{
		ID:                      1,
		Platform:                service.PlatformOpenAI,
		Type:                    service.AccountTypeOAuth,
		Status:                  service.StatusActive,
		Schedulable:             true,
		GroupIDs:                []int64{10},
		TempUnschedulableUntil:  &future,
		TempUnschedulableReason: "OAuth 401: session expired",
	}}}
	h := &OpenAIGatewayHandler{gatewayService: service.NewOpenAIGatewayService(
		repo,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		&config.Config{},
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)}
	groupID := int64(10)

	message := h.resolveOpenAINoAvailableAccountMessage(
		context.Background(),
		&groupID,
		"",
		service.OpenAIAccountScheduleDecision{},
		fmt.Errorf("no available OpenAI accounts: %w", service.ErrNoAvailableAccounts),
		false,
	)
	require.Equal(t, "Upstream authentication failed, please contact administrator", message)
}

func TestResolveOpenAINoAvailableAccountMessage_ExposesSchedulingErrorWhenNoAuthSignal(t *testing.T) {
	future := time.Now().Add(5 * time.Minute)
	repo := openAIHandlerAccountRepoStub{accounts: []service.Account{{
		ID:                      2,
		Platform:                service.PlatformOpenAI,
		Type:                    service.AccountTypeOAuth,
		Status:                  service.StatusActive,
		Schedulable:             true,
		GroupIDs:                []int64{10},
		TempUnschedulableUntil:  &future,
		TempUnschedulableReason: "temporary overload",
	}}}
	h := &OpenAIGatewayHandler{gatewayService: service.NewOpenAIGatewayService(
		repo,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		&config.Config{},
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)}
	groupID := int64(10)
	err := fmt.Errorf("no available OpenAI accounts: %w", service.ErrNoAvailableAccounts)

	message := h.resolveOpenAINoAvailableAccountMessage(
		context.Background(),
		&groupID,
		"",
		service.OpenAIAccountScheduleDecision{},
		err,
		false,
	)
	require.Equal(t, err.Error(), message)
}

func TestReadRequestBodyWithPrealloc(t *testing.T) {
	payload := `{"model":"gpt-5","input":"hello"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(payload))
	req.ContentLength = int64(len(payload))

	body, err := pkghttputil.ReadRequestBodyWithPrealloc(req)
	require.NoError(t, err)
	require.Equal(t, payload, string(body))
}

func TestReadRequestBodyWithPrealloc_MaxBytesError(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(strings.Repeat("x", 8)))
	req.Body = http.MaxBytesReader(rec, req.Body, 4)

	_, err := pkghttputil.ReadRequestBodyWithPrealloc(req)
	require.Error(t, err)
	var maxErr *http.MaxBytesError
	require.ErrorAs(t, err, &maxErr)
}

func TestOpenAIEnsureForwardErrorResponse_WritesFallbackWhenNotWritten(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	h := &OpenAIGatewayHandler{}
	wrote := h.ensureForwardErrorResponse(c, false)

	require.True(t, wrote)
	require.Equal(t, http.StatusBadGateway, w.Code)

	var parsed map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &parsed)
	require.NoError(t, err)
	errorObj, ok := parsed["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "upstream_error", errorObj["type"])
	assert.Equal(t, "Upstream request failed", errorObj["message"])
}

func TestOpenAIEnsureForwardErrorResponse_DoesNotOverrideWrittenResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.String(http.StatusTeapot, "already written")

	h := &OpenAIGatewayHandler{}
	wrote := h.ensureForwardErrorResponse(c, false)

	require.False(t, wrote)
	require.Equal(t, http.StatusTeapot, w.Code)
	assert.Equal(t, "already written", w.Body.String())
}

func TestShouldLogOpenAIForwardFailureAsWarn(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("fallback_written_should_not_downgrade", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		require.False(t, shouldLogOpenAIForwardFailureAsWarn(c, true))
	})

	t.Run("context_nil_should_not_downgrade", func(t *testing.T) {
		require.False(t, shouldLogOpenAIForwardFailureAsWarn(nil, false))
	})

	t.Run("response_not_written_should_not_downgrade", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		require.False(t, shouldLogOpenAIForwardFailureAsWarn(c, false))
	})

	t.Run("response_already_written_should_downgrade", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		c.String(http.StatusForbidden, "already written")
		require.True(t, shouldLogOpenAIForwardFailureAsWarn(c, false))
	})
}

func TestOpenAIRecoverResponsesPanic_WritesFallbackResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	h := &OpenAIGatewayHandler{}
	streamStarted := false
	require.NotPanics(t, func() {
		func() {
			defer h.recoverResponsesPanic(c, &streamStarted)
			panic("test panic")
		}()
	})

	require.Equal(t, http.StatusBadGateway, w.Code)

	var parsed map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &parsed)
	require.NoError(t, err)

	errorObj, ok := parsed["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "upstream_error", errorObj["type"])
	assert.Equal(t, "Upstream request failed", errorObj["message"])
}

func TestOpenAIRecoverResponsesPanic_NoPanicNoWrite(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	h := &OpenAIGatewayHandler{}
	streamStarted := false
	require.NotPanics(t, func() {
		func() {
			defer h.recoverResponsesPanic(c, &streamStarted)
		}()
	})

	require.False(t, c.Writer.Written())
	assert.Equal(t, "", w.Body.String())
}

func TestOpenAIRecoverResponsesPanic_DoesNotOverrideWrittenResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.String(http.StatusTeapot, "already written")

	h := &OpenAIGatewayHandler{}
	streamStarted := false
	require.NotPanics(t, func() {
		func() {
			defer h.recoverResponsesPanic(c, &streamStarted)
			panic("test panic")
		}()
	})

	require.Equal(t, http.StatusTeapot, w.Code)
	assert.Equal(t, "already written", w.Body.String())
}

func TestOpenAIMissingResponsesDependencies(t *testing.T) {
	t.Run("nil_handler", func(t *testing.T) {
		var h *OpenAIGatewayHandler
		require.Equal(t, []string{"handler"}, h.missingResponsesDependencies())
	})

	t.Run("all_dependencies_missing", func(t *testing.T) {
		h := &OpenAIGatewayHandler{}
		require.Equal(t,
			[]string{"gatewayService", "billingCacheService", "apiKeyService", "concurrencyHelper"},
			h.missingResponsesDependencies(),
		)
	})

	t.Run("all_dependencies_present", func(t *testing.T) {
		h := &OpenAIGatewayHandler{
			gatewayService:      &service.OpenAIGatewayService{},
			billingCacheService: &service.BillingCacheService{},
			apiKeyService:       &service.APIKeyService{},
			concurrencyHelper: &ConcurrencyHelper{
				concurrencyService: &service.ConcurrencyService{},
			},
		}
		require.Empty(t, h.missingResponsesDependencies())
	})
}

func TestOpenAIEnsureResponsesDependencies(t *testing.T) {
	t.Run("missing_dependencies_returns_503", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

		h := &OpenAIGatewayHandler{}
		ok := h.ensureResponsesDependencies(c, nil)

		require.False(t, ok)
		require.Equal(t, http.StatusServiceUnavailable, w.Code)
		var parsed map[string]any
		err := json.Unmarshal(w.Body.Bytes(), &parsed)
		require.NoError(t, err)
		errorObj, exists := parsed["error"].(map[string]any)
		require.True(t, exists)
		assert.Equal(t, "api_error", errorObj["type"])
		assert.Equal(t, "Service temporarily unavailable", errorObj["message"])
	})

	t.Run("already_written_response_not_overridden", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		c.String(http.StatusTeapot, "already written")

		h := &OpenAIGatewayHandler{}
		ok := h.ensureResponsesDependencies(c, nil)

		require.False(t, ok)
		require.Equal(t, http.StatusTeapot, w.Code)
		assert.Equal(t, "already written", w.Body.String())
	})

	t.Run("dependencies_ready_returns_true_and_no_write", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

		h := &OpenAIGatewayHandler{
			gatewayService:      &service.OpenAIGatewayService{},
			billingCacheService: &service.BillingCacheService{},
			apiKeyService:       &service.APIKeyService{},
			concurrencyHelper: &ConcurrencyHelper{
				concurrencyService: &service.ConcurrencyService{},
			},
		}
		ok := h.ensureResponsesDependencies(c, nil)

		require.True(t, ok)
		require.False(t, c.Writer.Written())
		assert.Equal(t, "", w.Body.String())
	})
}

func TestResolveOpenAIForwardDefaultMappedModel(t *testing.T) {
	t.Run("prefers_explicit_fallback_model", func(t *testing.T) {
		apiKey := &service.APIKey{
			Group: &service.Group{DefaultMappedModel: "gpt-5.4"},
		}
		require.Equal(t, "gpt-5.2", resolveOpenAIForwardDefaultMappedModel(apiKey, " gpt-5.2 "))
	})

	t.Run("uses_group_default_when_explicit_fallback_absent", func(t *testing.T) {
		apiKey := &service.APIKey{
			Group: &service.Group{DefaultMappedModel: "gpt-5.4"},
		}
		require.Equal(t, "gpt-5.4", resolveOpenAIForwardDefaultMappedModel(apiKey, ""))
	})

	t.Run("returns_empty_without_group_default", func(t *testing.T) {
		require.Empty(t, resolveOpenAIForwardDefaultMappedModel(nil, ""))
		require.Empty(t, resolveOpenAIForwardDefaultMappedModel(&service.APIKey{}, ""))
		require.Empty(t, resolveOpenAIForwardDefaultMappedModel(&service.APIKey{
			Group: &service.Group{},
		}, ""))
	})
}

func TestResolveOpenAIMessagesDispatchMappedModel(t *testing.T) {
	t.Run("exact_claude_model_override_wins", func(t *testing.T) {
		apiKey := &service.APIKey{
			Group: &service.Group{
				MessagesDispatchModelConfig: service.OpenAIMessagesDispatchModelConfig{
					SonnetMappedModel: "gpt-5.2",
					ExactModelMappings: map[string]string{
						"claude-sonnet-4-5-20250929": "gpt-5.4-mini-high",
					},
				},
			},
		}
		require.Equal(t, "gpt-5.4-mini", resolveOpenAIMessagesDispatchMappedModel(apiKey, "claude-sonnet-4-5-20250929"))
	})

	t.Run("uses_family_default_when_no_override", func(t *testing.T) {
		apiKey := &service.APIKey{Group: &service.Group{}}
		require.Equal(t, "gpt-5.4", resolveOpenAIMessagesDispatchMappedModel(apiKey, "claude-opus-4-6"))
		require.Equal(t, "gpt-5.3-codex", resolveOpenAIMessagesDispatchMappedModel(apiKey, "claude-sonnet-4-5-20250929"))
		require.Equal(t, "gpt-5.4-mini", resolveOpenAIMessagesDispatchMappedModel(apiKey, "claude-haiku-4-5-20251001"))
	})

	t.Run("returns_empty_for_non_claude_or_missing_group", func(t *testing.T) {
		require.Empty(t, resolveOpenAIMessagesDispatchMappedModel(nil, "claude-sonnet-4-5-20250929"))
		require.Empty(t, resolveOpenAIMessagesDispatchMappedModel(&service.APIKey{}, "claude-sonnet-4-5-20250929"))
		require.Empty(t, resolveOpenAIMessagesDispatchMappedModel(&service.APIKey{Group: &service.Group{}}, "gpt-5.4"))
	})

	t.Run("does_not_fall_back_to_group_default_mapped_model", func(t *testing.T) {
		apiKey := &service.APIKey{
			Group: &service.Group{
				DefaultMappedModel: "gpt-5.4",
			},
		}
		require.Empty(t, resolveOpenAIMessagesDispatchMappedModel(apiKey, "gpt-5.4"))
		require.Equal(t, "gpt-5.3-codex", resolveOpenAIMessagesDispatchMappedModel(apiKey, "claude-sonnet-4-5-20250929"))
	})
}

func TestOpenAIAcquireResponsesAccountSlot_RectifierRetryDetachesCanceledRequestContext(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	baseCtx, cancel := context.WithCancel(context.Background())
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil).WithContext(baseCtx)
	cancel()

	cache := &concurrencyCacheMock{
		acquireAccountSlotFn: func(ctx context.Context, accountID int64, maxConcurrency int, requestID string) (bool, error) {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			return true, nil
		},
	}
	h := &OpenAIGatewayHandler{
		gatewayService:    &service.OpenAIGatewayService{},
		concurrencyHelper: NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatNone, time.Second),
	}

	selection := h.newRectifierRetrySelection(&service.Account{ID: 911, Concurrency: 10})
	streamStarted := false

	release, acquired := h.acquireResponsesAccountSlot(c, nil, "", selection, true, &streamStarted, zap.NewNop())
	require.True(t, acquired, "rectifier internal retry should not fail just because the original request context was canceled")
	require.NotNil(t, release)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Empty(t, rec.Body.String())

	release()
}

func TestOpenAIResponses_MissingDependencies_ReturnsServiceUnavailable(t *testing.T) {

	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-5","stream":false}`))
	c.Request.Header.Set("Content-Type", "application/json")

	groupID := int64(2)
	c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{
		ID:      10,
		GroupID: &groupID,
	})
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{
		UserID:      1,
		Concurrency: 1,
	})

	// 故意使用未初始化依赖，验证快速失败而不是崩溃。
	h := &OpenAIGatewayHandler{}
	require.NotPanics(t, func() {
		h.Responses(c)
	})

	require.Equal(t, http.StatusServiceUnavailable, w.Code)

	var parsed map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &parsed)
	require.NoError(t, err)

	errorObj, ok := parsed["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "api_error", errorObj["type"])
	assert.Equal(t, "Service temporarily unavailable", errorObj["message"])
}

func TestOpenAIResponses_SetsClientTransportHTTP(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", strings.NewReader(`{"model":"gpt-5"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	h := &OpenAIGatewayHandler{}
	h.Responses(c)

	require.Equal(t, http.StatusUnauthorized, w.Code)
	require.Equal(t, service.OpenAIClientTransportHTTP, service.GetOpenAIClientTransport(c))
}

func TestOpenAIResponses_RejectsMessageIDAsPreviousResponseID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", strings.NewReader(
		`{"model":"gpt-5.1","stream":false,"previous_response_id":"msg_123456","input":[{"type":"input_text","text":"hello"}]}`,
	))
	c.Request.Header.Set("Content-Type", "application/json")

	groupID := int64(2)
	c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{
		ID:      101,
		GroupID: &groupID,
		User:    &service.User{ID: 1},
	})
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{
		UserID:      1,
		Concurrency: 1,
	})

	h := newOpenAIHandlerForPreviousResponseIDValidation(t, nil)
	h.Responses(c)

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "previous_response_id must be a response.id")
}

func TestOpenAIResponsesWebSocket_SetsClientTransportWSWhenUpgradeValid(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/openai/v1/responses", nil)
	c.Request.Header.Set("Upgrade", "websocket")
	c.Request.Header.Set("Connection", "Upgrade")

	h := &OpenAIGatewayHandler{}
	h.ResponsesWebSocket(c)

	require.Equal(t, http.StatusUnauthorized, w.Code)
	require.Equal(t, service.OpenAIClientTransportWS, service.GetOpenAIClientTransport(c))
}

func TestOpenAIResponsesWebSocket_InvalidUpgradeDoesNotSetTransport(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/openai/v1/responses", nil)

	h := &OpenAIGatewayHandler{}
	h.ResponsesWebSocket(c)

	require.Equal(t, http.StatusUpgradeRequired, w.Code)
	require.Equal(t, service.OpenAIClientTransportUnknown, service.GetOpenAIClientTransport(c))
}

func TestOpenAIResponsesWebSocket_RejectsMessageIDAsPreviousResponseID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := newOpenAIHandlerForPreviousResponseIDValidation(t, nil)
	wsServer := newOpenAIWSHandlerTestServer(t, h, middleware.AuthSubject{UserID: 1, Concurrency: 1})
	defer wsServer.Close()

	dialCtx, cancelDial := context.WithTimeout(context.Background(), 3*time.Second)
	clientConn, _, err := coderws.Dial(dialCtx, "ws"+strings.TrimPrefix(wsServer.URL, "http")+"/openai/v1/responses", nil)
	cancelDial()
	require.NoError(t, err)
	defer func() {
		_ = clientConn.CloseNow()
	}()

	writeCtx, cancelWrite := context.WithTimeout(context.Background(), 3*time.Second)
	err = clientConn.Write(writeCtx, coderws.MessageText, []byte(
		`{"type":"response.create","model":"gpt-5.1","stream":false,"previous_response_id":"msg_abc123"}`,
	))
	cancelWrite()
	require.NoError(t, err)

	readCtx, cancelRead := context.WithTimeout(context.Background(), 3*time.Second)
	_, _, err = clientConn.Read(readCtx)
	cancelRead()
	require.Error(t, err)
	var closeErr coderws.CloseError
	require.ErrorAs(t, err, &closeErr)
	require.Equal(t, coderws.StatusPolicyViolation, closeErr.Code)
	require.Contains(t, strings.ToLower(closeErr.Reason), "previous_response_id")
}

func TestOpenAIResponsesWebSocket_PreviousResponseIDKindLoggedBeforeAcquireFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cache := &concurrencyCacheMock{
		acquireUserSlotFn: func(ctx context.Context, userID int64, maxConcurrency int, requestID string) (bool, error) {
			return false, errors.New("user slot unavailable")
		},
	}
	h := newOpenAIHandlerForPreviousResponseIDValidation(t, cache)
	wsServer := newOpenAIWSHandlerTestServer(t, h, middleware.AuthSubject{UserID: 1, Concurrency: 1})
	defer wsServer.Close()

	dialCtx, cancelDial := context.WithTimeout(context.Background(), 3*time.Second)
	clientConn, _, err := coderws.Dial(dialCtx, "ws"+strings.TrimPrefix(wsServer.URL, "http")+"/openai/v1/responses", nil)
	cancelDial()
	require.NoError(t, err)
	defer func() {
		_ = clientConn.CloseNow()
	}()

	writeCtx, cancelWrite := context.WithTimeout(context.Background(), 3*time.Second)
	err = clientConn.Write(writeCtx, coderws.MessageText, []byte(
		`{"type":"response.create","model":"gpt-5.1","stream":false,"previous_response_id":"resp_prev_123"}`,
	))
	cancelWrite()
	require.NoError(t, err)

	readCtx, cancelRead := context.WithTimeout(context.Background(), 3*time.Second)
	_, _, err = clientConn.Read(readCtx)
	cancelRead()
	require.Error(t, err)
	var closeErr coderws.CloseError
	require.ErrorAs(t, err, &closeErr)
	require.Equal(t, coderws.StatusInternalError, closeErr.Code)
	require.Contains(t, strings.ToLower(closeErr.Reason), "failed to acquire user concurrency slot")
}

func TestSetOpenAIClientTransportHTTP(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	setOpenAIClientTransportHTTP(c)
	require.Equal(t, service.OpenAIClientTransportHTTP, service.GetOpenAIClientTransport(c))
}

func TestSetOpenAIClientTransportWS(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	setOpenAIClientTransportWS(c)
	require.Equal(t, service.OpenAIClientTransportWS, service.GetOpenAIClientTransport(c))
}

// TestOpenAIHandler_GjsonExtraction 验证 gjson 从请求体中提取 model/stream 的正确性
func TestOpenAIHandler_GjsonExtraction(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantModel  string
		wantStream bool
	}{
		{"正常提取", `{"model":"gpt-4","stream":true,"input":"hello"}`, "gpt-4", true},
		{"stream false", `{"model":"gpt-4","stream":false}`, "gpt-4", false},
		{"无 stream 字段", `{"model":"gpt-4"}`, "gpt-4", false},
		{"model 缺失", `{"stream":true}`, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := []byte(tt.body)
			modelResult := gjson.GetBytes(body, "model")
			model := ""
			if modelResult.Type == gjson.String {
				model = modelResult.String()
			}
			stream := gjson.GetBytes(body, "stream").Bool()
			require.Equal(t, tt.wantModel, model)
			require.Equal(t, tt.wantStream, stream)
		})
	}
}

// TestOpenAIHandler_GjsonValidation 验证修复后的 JSON 合法性和类型校验
func TestOpenAIHandler_GjsonValidation(t *testing.T) {
	// 非法 JSON 被 gjson.ValidBytes 拦截
	require.False(t, gjson.ValidBytes([]byte(`{invalid json`)))

	// model 为数字 → 类型不是 gjson.String，应被拒绝
	body := []byte(`{"model":123}`)
	modelResult := gjson.GetBytes(body, "model")
	require.True(t, modelResult.Exists())
	require.NotEqual(t, gjson.String, modelResult.Type)

	// model 为 null → 类型不是 gjson.String，应被拒绝
	body2 := []byte(`{"model":null}`)
	modelResult2 := gjson.GetBytes(body2, "model")
	require.True(t, modelResult2.Exists())
	require.NotEqual(t, gjson.String, modelResult2.Type)

	// stream 为 string → 类型既不是 True 也不是 False，应被拒绝
	body3 := []byte(`{"model":"gpt-4","stream":"true"}`)
	streamResult := gjson.GetBytes(body3, "stream")
	require.True(t, streamResult.Exists())
	require.NotEqual(t, gjson.True, streamResult.Type)
	require.NotEqual(t, gjson.False, streamResult.Type)

	// stream 为 int → 同上
	body4 := []byte(`{"model":"gpt-4","stream":1}`)
	streamResult2 := gjson.GetBytes(body4, "stream")
	require.True(t, streamResult2.Exists())
	require.NotEqual(t, gjson.True, streamResult2.Type)
	require.NotEqual(t, gjson.False, streamResult2.Type)
}

// TestOpenAIHandler_InstructionsInjection 验证 instructions 的 gjson/sjson 注入逻辑
func TestOpenAIHandler_InstructionsInjection(t *testing.T) {
	// 测试 1：无 instructions → 注入
	body := []byte(`{"model":"gpt-4"}`)
	existing := gjson.GetBytes(body, "instructions").String()
	require.Empty(t, existing)
	newBody, err := sjson.SetBytes(body, "instructions", "test instruction")
	require.NoError(t, err)
	require.Equal(t, "test instruction", gjson.GetBytes(newBody, "instructions").String())

	// 测试 2：已有 instructions → 不覆盖
	body2 := []byte(`{"model":"gpt-4","instructions":"existing"}`)
	existing2 := gjson.GetBytes(body2, "instructions").String()
	require.Equal(t, "existing", existing2)

	// 测试 3：空白 instructions → 注入
	body3 := []byte(`{"model":"gpt-4","instructions":"   "}`)
	existing3 := strings.TrimSpace(gjson.GetBytes(body3, "instructions").String())
	require.Empty(t, existing3)

	// 测试 4：sjson.SetBytes 返回错误时不应 panic
	// 正常 JSON 不会产生 sjson 错误，验证返回值被正确处理
	validBody := []byte(`{"model":"gpt-4"}`)
	result, setErr := sjson.SetBytes(validBody, "instructions", "hello")
	require.NoError(t, setErr)
	require.True(t, gjson.ValidBytes(result))
}

func TestSetContextLatencyMsIfAbsent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	setContextLatencyMsIfAbsent(c, service.OpsRoutingLatencyMsKey, 12)
	setContextLatencyMsIfAbsent(c, service.OpsRoutingLatencyMsKey, 99)

	ms, ok := getContextInt64(c, service.OpsRoutingLatencyMsKey)
	require.True(t, ok)
	require.EqualValues(t, 12, ms)
}

func TestAttachOpenAITimingBreakdownFromContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	service.SetOpsLatencyMs(c, service.OpsAuthLatencyMsKey, 11)
	service.SetOpsLatencyMs(c, service.OpsRoutingLatencyMsKey, 22)
	service.SetOpsLatencyMs(c, service.OpsGatewayPrepareLatencyMsKey, 33)
	service.SetOpsLatencyMs(c, service.OpsUpstreamLatencyMsKey, 44)
	service.SetOpsLatencyMs(c, service.OpsStreamFirstEventLatencyMsKey, 55)

	result := &service.OpenAIForwardResult{}
	attachOpenAITimingBreakdownFromContext(c, result)

	require.NotNil(t, result.AuthLatencyMs)
	require.NotNil(t, result.RoutingLatencyMs)
	require.NotNil(t, result.GatewayPrepareMs)
	require.NotNil(t, result.UpstreamLatencyMs)
	require.NotNil(t, result.StreamFirstEventMs)
	assert.Equal(t, 11, *result.AuthLatencyMs)
	assert.Equal(t, 22, *result.RoutingLatencyMs)
	assert.Equal(t, 33, *result.GatewayPrepareMs)
	assert.Equal(t, 44, *result.UpstreamLatencyMs)
	assert.Equal(t, 55, *result.StreamFirstEventMs)
}

func newOpenAIHandlerForPreviousResponseIDValidation(t *testing.T, cache *concurrencyCacheMock) *OpenAIGatewayHandler {
	t.Helper()
	if cache == nil {
		cache = &concurrencyCacheMock{
			acquireUserSlotFn: func(ctx context.Context, userID int64, maxConcurrency int, requestID string) (bool, error) {
				return true, nil
			},
			acquireAccountSlotFn: func(ctx context.Context, accountID int64, maxConcurrency int, requestID string) (bool, error) {
				return true, nil
			},
		}
	}
	return &OpenAIGatewayHandler{
		gatewayService:      &service.OpenAIGatewayService{},
		billingCacheService: &service.BillingCacheService{},
		apiKeyService:       &service.APIKeyService{},
		concurrencyHelper:   NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatNone, time.Second),
	}
}

func newOpenAIWSHandlerTestServer(t *testing.T, h *OpenAIGatewayHandler, subject middleware.AuthSubject) *httptest.Server {
	t.Helper()
	groupID := int64(2)
	apiKey := &service.APIKey{
		ID:      101,
		GroupID: &groupID,
		User:    &service.User{ID: subject.UserID},
	}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyAPIKey), apiKey)
		c.Set(string(middleware.ContextKeyUser), subject)
		c.Next()
	})
	router.GET("/openai/v1/responses", h.ResponsesWebSocket)
	return httptest.NewServer(router)
}
