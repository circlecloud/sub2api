//go:build unit

package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
)

// --- shared test helpers ---

type queuedHTTPUpstream struct {
	responses     []*http.Response
	requests      []*http.Request
	requestBodies [][]byte
	tlsFlags      []bool
}

func (u *queuedHTTPUpstream) Do(_ *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	return nil, fmt.Errorf("unexpected Do call")
}

func (u *queuedHTTPUpstream) DoWithTLS(req *http.Request, _ string, _ int64, _ int, profile *tlsfingerprint.Profile) (*http.Response, error) {
	u.requests = append(u.requests, req)
	if req != nil && req.Body != nil {
		body, _ := io.ReadAll(req.Body)
		u.requestBodies = append(u.requestBodies, body)
	} else {
		u.requestBodies = append(u.requestBodies, nil)
	}
	u.tlsFlags = append(u.tlsFlags, profile != nil)
	if len(u.responses) == 0 {
		return nil, fmt.Errorf("no mocked response")
	}
	resp := u.responses[0]
	u.responses = u.responses[1:]
	return resp, nil
}

func newJSONResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

// --- test functions ---

func newTestContext() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/1/test", nil)
	return c, rec
}

type openAIAccountTestRepo struct {
	mockAccountRepoForGemini
	updatedExtra      map[string]any
	rateLimitedID     int64
	rateLimitedAt     *time.Time
	errorID           int64
	errorMessage      string
	tempUnschedID     int64
	tempUnschedUntil  *time.Time
	tempUnschedReason string
}

func (r *openAIAccountTestRepo) UpdateExtra(_ context.Context, _ int64, updates map[string]any) error {
	r.updatedExtra = updates
	return nil
}

func (r *openAIAccountTestRepo) SetRateLimited(_ context.Context, id int64, resetAt time.Time) error {
	r.rateLimitedID = id
	r.rateLimitedAt = &resetAt
	return nil
}

func (r *openAIAccountTestRepo) SetError(_ context.Context, id int64, errorMsg string) error {
	r.errorID = id
	r.errorMessage = errorMsg
	return nil
}

func (r *openAIAccountTestRepo) SetTempUnschedulable(_ context.Context, id int64, until time.Time, reason string) error {
	r.tempUnschedID = id
	r.tempUnschedUntil = &until
	r.tempUnschedReason = reason
	return nil
}

func TestAccountTestService_OpenAISuccessPersistsSnapshotFromHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, recorder := newTestContext()

	resp := newJSONResponse(http.StatusOK, "")
	resp.Body = io.NopCloser(strings.NewReader(`data: {"type":"response.completed"}

`))
	resp.Header.Set("x-codex-primary-used-percent", "88")
	resp.Header.Set("x-codex-primary-reset-after-seconds", "604800")
	resp.Header.Set("x-codex-primary-window-minutes", "10080")
	resp.Header.Set("x-codex-secondary-used-percent", "42")
	resp.Header.Set("x-codex-secondary-reset-after-seconds", "18000")
	resp.Header.Set("x-codex-secondary-window-minutes", "300")

	repo := &openAIAccountTestRepo{}
	upstream := &queuedHTTPUpstream{responses: []*http.Response{resp}}
	svc := &AccountTestService{accountRepo: repo, httpUpstream: upstream}
	account := &Account{
		ID:          89,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{"access_token": "test-token"},
	}

	err := svc.testOpenAIAccountConnection(ctx, account, "gpt-5.4")
	require.NoError(t, err)
	require.NotEmpty(t, repo.updatedExtra)
	require.Equal(t, 42.0, repo.updatedExtra["codex_5h_used_percent"])
	require.Equal(t, 88.0, repo.updatedExtra["codex_7d_used_percent"])
	require.Contains(t, recorder.Body.String(), "test_complete")
}

func TestAccountTestService_OpenAIApiKeyUsesEndpointByUpstreamProtocol(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name            string
		protocol        string
		responseBody    string
		wantURL         string
		wantInputPath   string
		wantMessageRole string
	}{
		{
			name:          "responses protocol uses responses endpoint",
			protocol:      OpenAIUpstreamProtocolResponses,
			responseBody:  "data: {\"type\":\"response.output_text.delta\",\"delta\":\"Ok\"}\n\ndata: {\"type\":\"response.completed\"}\n\n",
			wantURL:       "https://example.com/v1/responses",
			wantInputPath: "input.0.role",
		},
		{
			name:            "chat completions protocol uses chat completions endpoint",
			protocol:        OpenAIUpstreamProtocolChatCompletions,
			responseBody:    "data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"created\":1730000000,\"model\":\"gpt-5.4\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Ok\"},\"finish_reason\":null}]}\n\ndata: [DONE]\n\n",
			wantURL:         "https://example.com/v1/chat/completions",
			wantMessageRole: "user",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, recorder := newTestContext()
			resp := newJSONResponse(http.StatusOK, "")
			resp.Header.Set("Content-Type", "text/event-stream")
			resp.Body = io.NopCloser(strings.NewReader(tt.responseBody))

			upstream := &queuedHTTPUpstream{responses: []*http.Response{resp}}
			svc := &AccountTestService{
				httpUpstream: upstream,
				cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
			}
			account := &Account{
				ID:          90,
				Platform:    PlatformOpenAI,
				Type:        AccountTypeAPIKey,
				Concurrency: 1,
				Credentials: map[string]any{
					"api_key":  "sk-test",
					"base_url": "https://example.com",
				},
				Extra: map[string]any{
					"openai_apikey_upstream_protocol": tt.protocol,
				},
			}

			err := svc.testOpenAIAccountConnection(ctx, account, "gpt-5.4")
			require.NoError(t, err)
			require.Len(t, upstream.requests, 1)
			require.Equal(t, tt.wantURL, upstream.requests[0].URL.String())
			require.Contains(t, recorder.Body.String(), "test_complete")
			require.Len(t, upstream.requestBodies, 1)
			require.Equal(t, "gpt-5.4", gjson.GetBytes(upstream.requestBodies[0], "model").String())
			if tt.wantInputPath != "" {
				require.Equal(t, "user", gjson.GetBytes(upstream.requestBodies[0], tt.wantInputPath).String())
				require.False(t, gjson.GetBytes(upstream.requestBodies[0], "messages").Exists())
			}
			if tt.wantMessageRole != "" {
				require.Equal(t, tt.wantMessageRole, gjson.GetBytes(upstream.requestBodies[0], "messages.0.role").String())
				require.False(t, gjson.GetBytes(upstream.requestBodies[0], "input").Exists())
			}
		})
	}
}

func TestAccountTestService_OpenAI429PersistsSnapshotWithoutRateLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := newTestContext()

	resp := newJSONResponse(http.StatusTooManyRequests, `{"error":{"type":"usage_limit_reached","message":"limit reached"}}`)
	resp.Header.Set("x-codex-primary-used-percent", "100")
	resp.Header.Set("x-codex-primary-reset-after-seconds", "604800")
	resp.Header.Set("x-codex-primary-window-minutes", "10080")
	resp.Header.Set("x-codex-secondary-used-percent", "100")
	resp.Header.Set("x-codex-secondary-reset-after-seconds", "18000")
	resp.Header.Set("x-codex-secondary-window-minutes", "300")

	repo := &openAIAccountTestRepo{}
	upstream := &queuedHTTPUpstream{responses: []*http.Response{resp}}
	rateLimitService := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	svc := &AccountTestService{accountRepo: repo, rateLimitService: rateLimitService, httpUpstream: upstream}
	account := &Account{
		ID:          88,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{"access_token": "test-token"},
	}

	err := svc.testOpenAIAccountConnection(ctx, account, "gpt-5.4")
	require.Error(t, err)
	require.NotEmpty(t, repo.updatedExtra)
	require.Equal(t, 100.0, repo.updatedExtra["codex_5h_used_percent"])
	require.Equal(t, int64(88), repo.rateLimitedID)
	require.NotNil(t, repo.rateLimitedAt)
	require.NotNil(t, account.RateLimitResetAt)
	require.Zero(t, repo.errorID)
	require.Zero(t, repo.tempUnschedID)
	if account.RateLimitResetAt != nil && repo.rateLimitedAt != nil {
		require.WithinDuration(t, *repo.rateLimitedAt, *account.RateLimitResetAt, time.Second)
	}
}

func TestAccountTestService_OpenAI401TokenRevokedMarksError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := newTestContext()

	resp := newJSONResponse(http.StatusUnauthorized, `{"error":{"code":"token_revoked","message":"revoked by upstream"}}`)

	repo := &openAIAccountTestRepo{}
	upstream := &queuedHTTPUpstream{responses: []*http.Response{resp}}
	rateLimitService := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	svc := &AccountTestService{accountRepo: repo, rateLimitService: rateLimitService, httpUpstream: upstream}
	account := &Account{
		ID:          77,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{"access_token": "test-token"},
	}

	err := svc.testOpenAIAccountConnection(ctx, account, "gpt-5.4")
	require.Error(t, err)
	require.Equal(t, int64(77), repo.errorID)
	require.Contains(t, repo.errorMessage, "revoked by upstream")
	require.Zero(t, repo.tempUnschedID)
	require.Zero(t, repo.rateLimitedID)
}

func TestAccountTestService_OpenAI401OAuthMarksTempUnschedulable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := newTestContext()

	resp := newJSONResponse(http.StatusUnauthorized, `{"error":{"message":"session expired"}}`)

	repo := &openAIAccountTestRepo{}
	upstream := &queuedHTTPUpstream{responses: []*http.Response{resp}}
	rateLimitService := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	svc := &AccountTestService{accountRepo: repo, rateLimitService: rateLimitService, httpUpstream: upstream}
	account := &Account{
		ID:          66,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{"access_token": "test-token"},
	}

	err := svc.testOpenAIAccountConnection(ctx, account, "gpt-5.4")
	require.Error(t, err)
	require.Zero(t, repo.errorID)
	require.Equal(t, int64(66), repo.tempUnschedID)
	require.NotNil(t, repo.tempUnschedUntil)
	require.Contains(t, repo.tempUnschedReason, "session expired")
	require.Zero(t, repo.rateLimitedID)
}

func TestAccountTestService_OpenAI401UnauthorizedDetailMarksError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := newTestContext()

	resp := newJSONResponse(http.StatusUnauthorized, `{"detail":"Unauthorized"}`)

	repo := &openAIAccountTestRepo{}
	upstream := &queuedHTTPUpstream{responses: []*http.Response{resp}}
	rateLimitService := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	svc := &AccountTestService{accountRepo: repo, rateLimitService: rateLimitService, httpUpstream: upstream}
	account := &Account{
		ID:          65,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{"access_token": "test-token"},
	}

	err := svc.testOpenAIAccountConnection(ctx, account, "gpt-5.4")
	require.Error(t, err)
	require.Equal(t, int64(65), repo.errorID)
	require.Zero(t, repo.tempUnschedID)
	require.Contains(t, repo.errorMessage, "Unauthorized")
}

func TestAccountTestService_OpenAI401AccountDeactivatedMarksError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := newTestContext()

	resp := newJSONResponse(http.StatusUnauthorized, `{"error":{"code":"account_deactivated","message":"account disabled by upstream"}}`)

	repo := &openAIAccountTestRepo{}
	upstream := &queuedHTTPUpstream{responses: []*http.Response{resp}}
	rateLimitService := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	svc := &AccountTestService{accountRepo: repo, rateLimitService: rateLimitService, httpUpstream: upstream}
	account := &Account{
		ID:          56,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{"access_token": "test-token"},
	}

	err := svc.testOpenAIAccountConnection(ctx, account, "gpt-5.4")
	require.Error(t, err)
	require.Equal(t, int64(56), repo.errorID)
	require.Equal(t, "Account deactivated (401): account disabled by upstream", repo.errorMessage)
	require.Zero(t, repo.tempUnschedID)
	require.Zero(t, repo.rateLimitedID)
}

func TestAccountTestService_OpenAI403MarksError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := newTestContext()

	resp := newJSONResponse(http.StatusForbidden, `{"error":{"message":"workspace forbidden"}}`)

	repo := &openAIAccountTestRepo{}
	upstream := &queuedHTTPUpstream{responses: []*http.Response{resp}}
	rateLimitService := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	svc := &AccountTestService{accountRepo: repo, rateLimitService: rateLimitService, httpUpstream: upstream}
	account := &Account{
		ID:          55,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{"access_token": "test-token"},
	}

	err := svc.testOpenAIAccountConnection(ctx, account, "gpt-5.4")
	require.Error(t, err)
	require.Equal(t, int64(55), repo.errorID)
	require.Contains(t, repo.errorMessage, "workspace forbidden")
	require.Zero(t, repo.tempUnschedID)
	require.Zero(t, repo.rateLimitedID)
}
