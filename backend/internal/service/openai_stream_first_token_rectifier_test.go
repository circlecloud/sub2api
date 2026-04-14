package service

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestResolveOpenAIStreamRectifierPolicyFromConfig(t *testing.T) {
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{
		OpenAIStreamRectifierEnabled:                true,
		OpenAIStreamResponseHeaderRectifierTimeouts: []int{8, 10, 12},
		OpenAIStreamFirstTokenRectifierTimeouts:     []int{5, 8, 10},
	}}}

	policy := svc.ResolveOpenAIStreamRectifierPolicy(t.Context())
	require.True(t, policy.Enabled)
	require.Equal(t, 8*time.Second, policy.ResponseHeaderTimeoutForAttempt(1))
	require.Equal(t, 10*time.Second, policy.ResponseHeaderTimeoutForAttempt(2))
	require.Equal(t, 12*time.Second, policy.ResponseHeaderTimeoutForAttempt(3))
	require.Equal(t, 5*time.Second, policy.FirstTokenTimeoutForAttempt(1))
	require.Equal(t, 8*time.Second, policy.FirstTokenTimeoutForAttempt(2))
	require.Equal(t, 10*time.Second, policy.FirstTokenTimeoutForAttempt(3))
	require.Zero(t, policy.ResponseHeaderTimeoutForAttempt(4))
	require.Zero(t, policy.FirstTokenTimeoutForAttempt(4))
}

func TestPrepareOpenAIStreamFirstTokenRectifierContext(t *testing.T) {
	t.Run("非流式请求不注入", func(t *testing.T) {
		svc := &OpenAIGatewayService{}
		ctx := svc.PrepareOpenAIStreamFirstTokenRectifierContext(t.Context(), false, 1, 1)
		_, ok := getOpenAIStreamFirstTokenRectifier(ctx)
		require.False(t, ok)
	})

	t.Run("关闭开关时不注入", func(t *testing.T) {
		svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{OpenAIStreamRectifierEnabled: false}}}
		ctx := svc.PrepareOpenAIStreamFirstTokenRectifierContext(t.Context(), true, 1, 1)
		_, ok := getOpenAIStreamFirstTokenRectifier(ctx)
		require.False(t, ok)
	})

	t.Run("按双阶段独立 attempt 注入预算", func(t *testing.T) {
		svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{
			OpenAIStreamRectifierEnabled:                true,
			OpenAIStreamResponseHeaderRectifierTimeouts: []int{5, 8, 10},
			OpenAIStreamFirstTokenRectifierTimeouts:     []int{6, 7, 9},
		}}}
		ctx := svc.PrepareOpenAIStreamFirstTokenRectifierContext(t.Context(), true, 2, 1)
		value, ok := getOpenAIStreamFirstTokenRectifier(ctx)
		require.True(t, ok)
		require.Equal(t, 2, value.HeaderAttempt)
		require.Equal(t, 1, value.FirstTokenAttempt)
		require.Equal(t, 8*time.Second, value.ResponseHeaderTimeout)
		require.Equal(t, 6*time.Second, value.FirstTokenTimeout)
	})

	t.Run("超出某一阶段数组长度时只保留另一阶段预算", func(t *testing.T) {
		svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{
			OpenAIStreamRectifierEnabled:                true,
			OpenAIStreamResponseHeaderRectifierTimeouts: []int{5, 8},
			OpenAIStreamFirstTokenRectifierTimeouts:     []int{6, 7, 9},
		}}}
		ctx := svc.PrepareOpenAIStreamFirstTokenRectifierContext(t.Context(), true, 3, 2)
		value, ok := getOpenAIStreamFirstTokenRectifier(ctx)
		require.True(t, ok)
		require.Equal(t, 3, value.HeaderAttempt)
		require.Equal(t, 2, value.FirstTokenAttempt)
		require.Zero(t, value.ResponseHeaderTimeout)
		require.Equal(t, 7*time.Second, value.FirstTokenTimeout)
	})
}

func TestEnsureOpenAIStreamFirstTokenRectifierContext(t *testing.T) {
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{
		OpenAIStreamRectifierEnabled:                true,
		OpenAIStreamResponseHeaderRectifierTimeouts: []int{8, 10, 12},
		OpenAIStreamFirstTokenRectifierTimeouts:     []int{5, 8, 10},
	}}}

	t.Run("缺失时按双阶段 attempt 补注入", func(t *testing.T) {
		ctx := svc.EnsureOpenAIStreamFirstTokenRectifierContext(t.Context(), true, 2, 3)
		value, ok := getOpenAIStreamFirstTokenRectifier(ctx)
		require.True(t, ok)
		require.Equal(t, 2, value.HeaderAttempt)
		require.Equal(t, 3, value.FirstTokenAttempt)
		require.Equal(t, 10*time.Second, value.ResponseHeaderTimeout)
		require.Equal(t, 10*time.Second, value.FirstTokenTimeout)
	})

	t.Run("缺失且 attempt 非法时默认按第一次处理", func(t *testing.T) {
		ctx := svc.EnsureOpenAIStreamFirstTokenRectifierContext(t.Context(), true, 0, 0)
		value, ok := getOpenAIStreamFirstTokenRectifier(ctx)
		require.True(t, ok)
		require.Equal(t, 1, value.HeaderAttempt)
		require.Equal(t, 1, value.FirstTokenAttempt)
		require.Equal(t, 8*time.Second, value.ResponseHeaderTimeout)
		require.Equal(t, 5*time.Second, value.FirstTokenTimeout)
	})

	t.Run("已有预算时保持原值", func(t *testing.T) {
		original := WithOpenAIStreamFirstTokenRectifier(t.Context(), 2, 2, 10*time.Second, 8*time.Second)
		ctx := svc.EnsureOpenAIStreamFirstTokenRectifierContext(original, true, 1, 1)
		value, ok := getOpenAIStreamFirstTokenRectifier(ctx)
		require.True(t, ok)
		require.Equal(t, 2, value.HeaderAttempt)
		require.Equal(t, 2, value.FirstTokenAttempt)
		require.Equal(t, 10*time.Second, value.ResponseHeaderTimeout)
		require.Equal(t, 8*time.Second, value.FirstTokenTimeout)
	})
}

func TestApplyOpenAIStreamResponseHeaderRectifierContext(t *testing.T) {
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{
		OpenAIStreamRectifierEnabled:                true,
		OpenAIStreamResponseHeaderRectifierTimeouts: []int{8, 10, 12},
		OpenAIStreamFirstTokenRectifierTimeouts:     []int{5, 8, 10},
	}}}
	ctx := svc.PrepareOpenAIStreamFirstTokenRectifierContext(t.Context(), true, 2, 1)
	ctx = svc.ApplyOpenAIStreamResponseHeaderRectifierContext(ctx, true)
	timeout, ok := HTTPUpstreamResponseHeaderTimeoutFromContext(ctx)
	require.True(t, ok)
	require.Equal(t, 10*time.Second, timeout)
}

func TestNewOpenAIStreamFirstTokenRectifierTimeoutError_WritesOpsUpstreamContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	core, logs := observer.New(zap.WarnLevel)
	ctx := logger.IntoContext(t.Context(), zap.New(core))

	svc := &OpenAIGatewayService{}
	account := &Account{ID: 42, Name: "sticky-openai", Platform: PlatformOpenAI}
	rectifier := openAIStreamFirstTokenRectifierContextValue{HeaderAttempt: 2, FirstTokenAttempt: 1, ResponseHeaderTimeout: 8 * time.Second, FirstTokenTimeout: 6 * time.Second}
	err := svc.newOpenAIStreamFirstTokenRectifierTimeoutError(ctx, c, account, rectifier, "response_header")
	require.Error(t, err)
	timeoutErr, ok := AsOpenAIRectifierTimeoutError(err)
	require.True(t, ok)
	require.Equal(t, http.StatusBadGateway, timeoutErr.StatusCode)
	require.Equal(t, "response_header", timeoutErr.Phase)
	require.Equal(t, 2, timeoutErr.Attempt)
	require.Equal(t, 8*time.Second, timeoutErr.Timeout)
	require.Len(t, logs.All(), 0, "timeout error should be logged by the retry/exhausted handler path only")

	upstreamStatus, ok := c.Get(OpsUpstreamStatusCodeKey)
	require.True(t, ok)
	require.Equal(t, http.StatusBadGateway, upstreamStatus)

	upstreamMessage, ok := c.Get(OpsUpstreamErrorMessageKey)
	require.True(t, ok)
	require.Equal(t, "OpenAI response header timeout after 8s on header attempt 2", upstreamMessage)

	upstreamDetail, ok := c.Get(OpsUpstreamErrorDetailKey)
	require.True(t, ok)
	require.Equal(t, "phase=response_header timeout_ms=8000 attempt=2 header_attempt=2 first_token_attempt=1", upstreamDetail)

	eventsValue, ok := c.Get(OpsUpstreamErrorsKey)
	require.True(t, ok)
	events, ok := eventsValue.([]*OpsUpstreamErrorEvent)
	require.True(t, ok)
	require.Len(t, events, 1)
	require.NotNil(t, events[0])
	require.Equal(t, PlatformOpenAI, events[0].Platform)
	require.Equal(t, int64(42), events[0].AccountID)
	require.Equal(t, "sticky-openai", events[0].AccountName)
	require.Equal(t, http.StatusBadGateway, events[0].UpstreamStatusCode)
	require.Equal(t, "request_error:rectifier_timeout", events[0].Kind)
	require.Equal(t, "OpenAI response header timeout after 8s on header attempt 2", events[0].Message)
	require.Equal(t, "phase=response_header timeout_ms=8000 attempt=2 header_attempt=2 first_token_attempt=1", events[0].Detail)
}

func TestHandleStreamingResponse_FirstTokenRectifierTimeout(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	svc := &OpenAIGatewayService{
		cfg:                  &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}},
		responseHeaderFilter: compileResponseHeaderFilter(&config.Config{}),
	}
	account := &Account{ID: 42, Name: "slow-openai", Platform: PlatformOpenAI}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       delayedEOFReadCloser{delay: 20 * time.Millisecond},
	}
	ctx := WithOpenAIStreamFirstTokenRectifier(t.Context(), 2, 1, 8*time.Second, 5*time.Millisecond)

	result, err := svc.handleStreamingResponse(ctx, resp, c, account, time.Now(), "gpt-5.4", "gpt-5.4")
	require.Nil(t, result)
	require.Error(t, err)
	timeoutErr, ok := AsOpenAIRectifierTimeoutError(err)
	require.True(t, ok)
	require.Equal(t, http.StatusBadGateway, timeoutErr.StatusCode)
	require.Equal(t, "first_token", timeoutErr.Phase)
	require.Equal(t, 1, timeoutErr.Attempt)
	require.Equal(t, 0, rec.Body.Len())
}

func TestHandleStreamingResponse_FirstTokenRectifierAllowsLongStreamAfterFirstToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	svc := &OpenAIGatewayService{
		cfg:                  &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}},
		responseHeaderFilter: compileResponseHeaderFilter(&config.Config{}),
	}
	account := &Account{ID: 43, Name: "good-openai", Platform: PlatformOpenAI}
	body := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\n" +
		"data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n"
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       ioNopCloserString(body),
	}
	ctx := WithOpenAIStreamFirstTokenRectifier(t.Context(), 2, 2, 8*time.Second, 50*time.Millisecond)

	result, err := svc.handleStreamingResponse(ctx, resp, c, account, time.Now(), "gpt-5.4", "gpt-5.4")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.firstTokenMs)
	require.Contains(t, rec.Body.String(), "response.output_text.delta")
}

type delayedEOFReadCloser struct {
	delay time.Duration
}

func (d delayedEOFReadCloser) Read(p []byte) (int, error) {
	time.Sleep(d.delay)
	return 0, io.EOF
}

func (d delayedEOFReadCloser) Close() error { return nil }

type stringReadCloser struct {
	*strings.Reader
}

func (s stringReadCloser) Close() error { return nil }

func ioNopCloserString(value string) stringReadCloser {
	return stringReadCloser{Reader: strings.NewReader(value)}
}
