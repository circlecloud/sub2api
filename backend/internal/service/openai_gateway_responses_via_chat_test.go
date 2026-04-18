package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIGatewayService_ForwardAsResponsesViaChatUpstream_UsesChatCompletionsEndpointForAPIKey(t *testing.T) {
	gin.SetMode(gin.TestMode)

	requestBody := []byte(`{"model":"gpt-5.2","instructions":"You are helpful.","input":[{"role":"user","content":"hi"}],"stream":false,"service_tier":"flex","reasoning":{"effort":"high"}}`)
	upstreamResponseBody := `{"id":"chatcmpl-1","object":"chat.completion","created":1730000000,"model":"gpt-5.2-mini","choices":[{"index":0,"message":{"role":"assistant","content":"pong"},"finish_reason":"stop"}],"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18,"prompt_tokens_details":{"cached_tokens":3}}}`

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(requestBody))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("User-Agent", "curl/8.0")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"application/json"},
			"X-Request-Id": []string{"rid-responses-chat"},
		},
		Body: io.NopCloser(strings.NewReader(upstreamResponseBody)),
	}}

	svc := &OpenAIGatewayService{
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
		httpUpstream: upstream,
	}

	account := &Account{
		ID:          790,
		Name:        "responses-chat-upstream-account",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-chat-upstream",
			"base_url": "https://example.com",
			"model_mapping": map[string]any{
				"gpt-5.2": "gpt-5.2-mini",
			},
		},
		Extra: map[string]any{
			"openai_apikey_upstream_protocol": OpenAIUpstreamProtocolChatCompletions,
		},
		Status:         StatusActive,
		Schedulable:    true,
		RateMultiplier: f64p(1),
	}

	result, err := svc.ForwardAsResponsesViaChatUpstream(context.Background(), c, account, requestBody)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, upstream.lastReq)
	require.Equal(t, "https://example.com/v1/chat/completions", upstream.lastReq.URL.String())
	require.NotContains(t, upstream.lastReq.URL.String(), "/v1/responses")
	require.Equal(t, "Bearer sk-chat-upstream", upstream.lastReq.Header.Get("Authorization"))
	require.Equal(t, "curl/8.0", upstream.lastReq.Header.Get("User-Agent"))
	require.Equal(t, "gpt-5.2-mini", gjson.GetBytes(upstream.lastBody, "model").String())
	require.Equal(t, "system", gjson.GetBytes(upstream.lastBody, "messages.0.role").String())
	require.Equal(t, "You are helpful.", gjson.GetBytes(upstream.lastBody, "messages.0.content").String())
	require.Equal(t, "user", gjson.GetBytes(upstream.lastBody, "messages.1.role").String())
	require.Equal(t, "response", gjson.GetBytes(rec.Body.Bytes(), "object").String())
	require.True(t, strings.HasPrefix(gjson.GetBytes(rec.Body.Bytes(), "id").String(), "resp_"))
	require.Equal(t, "gpt-5.2", gjson.GetBytes(rec.Body.Bytes(), "model").String())
	require.Equal(t, "pong", gjson.GetBytes(rec.Body.Bytes(), "output.0.content.0.text").String())
	require.Equal(t, 11, result.Usage.InputTokens)
	require.Equal(t, 7, result.Usage.OutputTokens)
	require.Equal(t, 3, result.Usage.CacheReadInputTokens)
	require.NotNil(t, result.ServiceTier)
	require.Equal(t, "flex", *result.ServiceTier)
	require.NotNil(t, result.ReasoningEffort)
	require.Equal(t, "high", *result.ReasoningEffort)
}

func TestOpenAIGatewayService_ForwardAsResponsesViaChatUpstream_RejectsResponsesNativeStateFeatures(t *testing.T) {
	gin.SetMode(gin.TestMode)

	requestBody := []byte(`{"model":"gpt-5.2","previous_response_id":"resp_123","store":true,"input":[{"role":"user","content":"hi"}],"stream":false}`)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(requestBody))
	c.Request.Header.Set("Content-Type", "application/json")

	svc := &OpenAIGatewayService{
		cfg: &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
	}

	account := &Account{
		ID:          794,
		Name:        "responses-chat-upstream-native-state-account",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-chat-upstream",
			"base_url": "https://example.com",
		},
		Extra: map[string]any{
			"openai_apikey_upstream_protocol": OpenAIUpstreamProtocolChatCompletions,
		},
		Status:         StatusActive,
		Schedulable:    true,
		RateMultiplier: f64p(1),
	}

	result, err := svc.ForwardAsResponsesViaChatUpstream(context.Background(), c, account, requestBody)
	require.Error(t, err)
	require.Nil(t, result)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, "invalid_request_error", gjson.GetBytes(rec.Body.Bytes(), "error.code").String())
	require.Contains(t, gjson.GetBytes(rec.Body.Bytes(), "error.message").String(), "stateless /v1/responses")
	require.Contains(t, gjson.GetBytes(rec.Body.Bytes(), "error.message").String(), "previous_response_id")
	require.Contains(t, gjson.GetBytes(rec.Body.Bytes(), "error.message").String(), "store")
}

func TestOpenAIGatewayService_ForwardAsResponsesViaChatUpstream_NonStreamingSSEFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)

	requestBody := []byte(`{"model":"gpt-5.2","input":[{"role":"user","content":"hi"}],"stream":false}`)
	upstreamResponseBody := strings.Join([]string{
		`data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1730000000,"model":"gpt-5.2-mini","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1730000001,"model":"gpt-5.2-mini","choices":[{"index":0,"delta":{"content":"pong"},"finish_reason":"stop"}]}`,
		`data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1730000002,"model":"gpt-5.2-mini","choices":[],"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18,"prompt_tokens_details":{"cached_tokens":3}}}`,
		`data: [DONE]`,
	}, "\n")

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(requestBody))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"text/event-stream"},
			"X-Request-Id": []string{"rid-responses-sse"},
		},
		Body: io.NopCloser(strings.NewReader(upstreamResponseBody)),
	}}

	svc := &OpenAIGatewayService{
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
		httpUpstream: upstream,
	}

	account := &Account{
		ID:          793,
		Name:        "responses-chat-upstream-sse-account",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-chat-upstream",
			"base_url": "https://example.com",
			"model_mapping": map[string]any{
				"gpt-5.2": "gpt-5.2-mini",
			},
		},
		Extra: map[string]any{
			"openai_apikey_upstream_protocol": OpenAIUpstreamProtocolChatCompletions,
		},
		Status:         StatusActive,
		Schedulable:    true,
		RateMultiplier: f64p(1),
	}

	result, err := svc.ForwardAsResponsesViaChatUpstream(context.Background(), c, account, requestBody)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Contains(t, rec.Header().Get("Content-Type"), "application/json")
	require.Equal(t, "response", gjson.GetBytes(rec.Body.Bytes(), "object").String())
	require.Equal(t, "gpt-5.2", gjson.GetBytes(rec.Body.Bytes(), "model").String())
	require.Equal(t, "completed", gjson.GetBytes(rec.Body.Bytes(), "status").String())
	require.Equal(t, "pong", gjson.GetBytes(rec.Body.Bytes(), "output.0.content.0.text").String())
	require.Equal(t, 11, int(gjson.GetBytes(rec.Body.Bytes(), "usage.input_tokens").Int()))
	require.Equal(t, 7, int(gjson.GetBytes(rec.Body.Bytes(), "usage.output_tokens").Int()))
	require.NotContains(t, rec.Body.String(), "data:")
	require.Equal(t, 7, result.Usage.OutputTokens)
	require.Equal(t, 3, result.Usage.CacheReadInputTokens)
}

func TestOpenAIGatewayService_ForwardAsResponsesViaChatUpstream_StreamRequiresTerminalSignal(t *testing.T) {
	gin.SetMode(gin.TestMode)

	requestBody := []byte(`{"model":"gpt-5.2","input":[{"role":"user","content":"hi"}],"stream":true}`)
	upstreamStreamBody := strings.Join([]string{
		`data: {"id":"chatcmpl-stream-1","object":"chat.completion.chunk","created":1730000000,"model":"gpt-5.2-mini","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl-stream-1","object":"chat.completion.chunk","created":1730000001,"model":"gpt-5.2-mini","choices":[{"index":0,"delta":{"content":"partial"},"finish_reason":null}]}`,
	}, "\n") + "\n"

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(requestBody))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"text/event-stream"},
			"X-Request-Id": []string{"rid-responses-chat-stream"},
		},
		Body: io.NopCloser(strings.NewReader(upstreamStreamBody)),
	}}

	svc := &OpenAIGatewayService{
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
		httpUpstream: upstream,
	}

	account := &Account{
		ID:          791,
		Name:        "responses-chat-upstream-stream-account",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-chat-upstream",
			"base_url": "https://example.com",
		},
		Extra: map[string]any{
			"openai_apikey_upstream_protocol": OpenAIUpstreamProtocolChatCompletions,
		},
		Status:         StatusActive,
		Schedulable:    true,
		RateMultiplier: f64p(1),
	}

	result, err := svc.ForwardAsResponsesViaChatUpstream(context.Background(), c, account, requestBody)
	require.Error(t, err)
	require.ErrorContains(t, err, "missing terminal event")
	require.NotNil(t, result)
	require.Contains(t, rec.Body.String(), `"type":"response.created"`)
}

func TestOpenAIGatewayService_ForwardAsResponsesViaChatUpstream_StreamClientDisconnectStillDrainsUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)

	requestBody := []byte(`{"model":"gpt-5.2","input":[{"role":"user","content":"hi"}],"stream":true}`)
	upstreamStreamBody := strings.Join([]string{
		`data: {"id":"chatcmpl-stream-2","object":"chat.completion.chunk","created":1730000000,"model":"gpt-5.2-mini","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl-stream-2","object":"chat.completion.chunk","created":1730000001,"model":"gpt-5.2-mini","choices":[{"index":0,"delta":{"content":"pong"},"finish_reason":"stop"}]}`,
		`data: {"id":"chatcmpl-stream-2","object":"chat.completion.chunk","created":1730000002,"model":"gpt-5.2-mini","choices":[],"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18,"prompt_tokens_details":{"cached_tokens":3}}}`,
		`data: [DONE]`,
	}, "\n") + "\n"

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(requestBody))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Writer = &failWriteResponseWriter{ResponseWriter: c.Writer}

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"text/event-stream"},
			"X-Request-Id": []string{"rid-responses-chat-stream-disconnect"},
		},
		Body: io.NopCloser(strings.NewReader(upstreamStreamBody)),
	}}

	svc := &OpenAIGatewayService{
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
		httpUpstream: upstream,
	}

	account := &Account{
		ID:          792,
		Name:        "responses-chat-upstream-stream-disconnect-account",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-chat-upstream",
			"base_url": "https://example.com",
		},
		Extra: map[string]any{
			"openai_apikey_upstream_protocol": OpenAIUpstreamProtocolChatCompletions,
		},
		Status:         StatusActive,
		Schedulable:    true,
		RateMultiplier: f64p(1),
	}

	result, err := svc.ForwardAsResponsesViaChatUpstream(context.Background(), c, account, requestBody)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 11, result.Usage.InputTokens)
	require.Equal(t, 7, result.Usage.OutputTokens)
	require.Equal(t, 3, result.Usage.CacheReadInputTokens)
}
