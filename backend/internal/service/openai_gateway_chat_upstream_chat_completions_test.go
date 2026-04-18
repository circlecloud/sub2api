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

func TestOpenAIGatewayService_ForwardAsChatCompletionsViaChatUpstream_UsesChatCompletionsEndpointForAPIKey(t *testing.T) {
	gin.SetMode(gin.TestMode)

	requestBody := []byte(`{"model":"gpt-5.2","messages":[{"role":"user","content":"hi"}],"stream":false,"service_tier":"flex"}`)
	upstreamResponseBody := `{"id":"chatcmpl-1","object":"chat.completion","created":1730000000,"model":"gpt-5.2-mini","choices":[{"index":0,"message":{"role":"assistant","content":"pong"},"finish_reason":"stop"}],"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18,"prompt_tokens_details":{"cached_tokens":3}}}`

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(requestBody))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("User-Agent", "curl/8.0")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"application/json"},
			"X-Request-Id": []string{"rid-chat"},
		},
		Body: io.NopCloser(strings.NewReader(upstreamResponseBody)),
	}}

	svc := &OpenAIGatewayService{
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
		httpUpstream: upstream,
	}

	account := &Account{
		ID:          789,
		Name:        "chat-upstream-account",
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

	result, err := svc.ForwardAsChatCompletionsViaChatUpstream(context.Background(), c, account, requestBody, "", "")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, upstream.lastReq)
	require.Equal(t, "https://example.com/v1/chat/completions", upstream.lastReq.URL.String())
	require.NotContains(t, upstream.lastReq.URL.String(), "/v1/responses")
	require.Equal(t, "Bearer sk-chat-upstream", upstream.lastReq.Header.Get("Authorization"))
	require.Equal(t, "curl/8.0", upstream.lastReq.Header.Get("User-Agent"))
	require.Equal(t, "gpt-5.2-mini", gjson.GetBytes(upstream.lastBody, "model").String())
	require.Equal(t, "gpt-5.2", gjson.GetBytes(rec.Body.Bytes(), "model").String())
	require.Equal(t, 11, result.Usage.InputTokens)
	require.Equal(t, 7, result.Usage.OutputTokens)
	require.Equal(t, 3, result.Usage.CacheReadInputTokens)
	require.NotNil(t, result.ServiceTier)
	require.Equal(t, "flex", *result.ServiceTier)
}

func TestOpenAIGatewayService_ForwardAsChatCompletionsViaChatUpstream_NonStreamingSSEFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)

	requestBody := []byte(`{"model":"gpt-5.2","messages":[{"role":"user","content":"hi"}],"stream":false}`)
	upstreamResponseBody := strings.Join([]string{
		`data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1730000000,"model":"gpt-5.2-mini","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1730000001,"model":"gpt-5.2-mini","choices":[{"index":0,"delta":{"content":"pong"},"finish_reason":"stop"}]}`,
		`data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1730000002,"model":"gpt-5.2-mini","choices":[],"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18,"prompt_tokens_details":{"cached_tokens":3}}}`,
		`data: [DONE]`,
	}, "\n")

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(requestBody))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"text/event-stream"},
			"X-Request-Id": []string{"rid-chat-sse"},
		},
		Body: io.NopCloser(strings.NewReader(upstreamResponseBody)),
	}}

	svc := &OpenAIGatewayService{
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
		httpUpstream: upstream,
	}

	account := &Account{
		ID:          792,
		Name:        "chat-upstream-sse-account",
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

	result, err := svc.ForwardAsChatCompletionsViaChatUpstream(context.Background(), c, account, requestBody, "", "")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Contains(t, rec.Header().Get("Content-Type"), "application/json")
	require.Equal(t, "chat.completion", gjson.GetBytes(rec.Body.Bytes(), "object").String())
	require.Equal(t, "gpt-5.2", gjson.GetBytes(rec.Body.Bytes(), "model").String())
	require.Equal(t, "assistant", gjson.GetBytes(rec.Body.Bytes(), "choices.0.message.role").String())
	require.Equal(t, "pong", gjson.GetBytes(rec.Body.Bytes(), "choices.0.message.content").String())
	require.Equal(t, "stop", gjson.GetBytes(rec.Body.Bytes(), "choices.0.finish_reason").String())
	require.Equal(t, 11, int(gjson.GetBytes(rec.Body.Bytes(), "usage.prompt_tokens").Int()))
	require.NotContains(t, rec.Body.String(), "data:")
	require.Equal(t, 11, result.Usage.InputTokens)
	require.Equal(t, 7, result.Usage.OutputTokens)
	require.Equal(t, 3, result.Usage.CacheReadInputTokens)
}
