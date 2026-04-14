package service

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/stretchr/testify/require"
)

type openaiOAuthClientRefreshStub struct {
	refreshCalls int32
	refreshErr   error
}

func (s *openaiOAuthClientRefreshStub) ExchangeCode(ctx context.Context, code, codeVerifier, redirectURI, proxyURL, clientID string) (*openai.TokenResponse, error) {
	return nil, errors.New("not implemented")
}

func (s *openaiOAuthClientRefreshStub) RefreshToken(ctx context.Context, refreshToken, proxyURL string) (*openai.TokenResponse, error) {
	atomic.AddInt32(&s.refreshCalls, 1)
	return nil, errors.New("not implemented")
}

func (s *openaiOAuthClientRefreshStub) RefreshTokenWithClientID(ctx context.Context, refreshToken, proxyURL string, clientID string) (*openai.TokenResponse, error) {
	atomic.AddInt32(&s.refreshCalls, 1)
	if s.refreshErr != nil {
		return nil, s.refreshErr
	}
	return nil, errors.New("not implemented")
}

func TestOpenAIOAuthService_RefreshAccountToken_RefreshTokenReusedRequiresReauth(t *testing.T) {
	client := &openaiOAuthClientRefreshStub{refreshErr: errors.New(`error: code=502 reason="OPENAI_OAUTH_TOKEN_REFRESH_FAILED" message="token refresh failed: status 401, body: {\n \"error\": {\n \"message\": \"Your refresh token has already been used to generate a new access token. Please try signing in again.\",\n \"type\": \"invalid_request_error\",\n \"param\": null,
 \"code\": \"refresh_token_reused\"\n }\n}" metadata=map[]`)}
	svc := NewOpenAIOAuthService(nil, client)
	account := &Account{
		ID:       78,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"refresh_token": "used-refresh-token",
			"client_id":     "client-id-2",
		},
	}

	info, err := svc.RefreshAccountToken(context.Background(), account)
	require.Nil(t, info)
	require.Error(t, err)
	require.Contains(t, err.Error(), "please sign in again")
	require.Contains(t, err.Error(), "re-authorize")
	require.Equal(t, int32(1), atomic.LoadInt32(&client.refreshCalls))
}

func TestOpenAIOAuthService_RefreshAccountToken_AccountDeactivatedRequiresReauth(t *testing.T) {
	client := &openaiOAuthClientRefreshStub{refreshErr: errors.New(`error: code=502 reason="OPENAI_OAUTH_TOKEN_REFRESH_FAILED" message="token refresh failed: status 401, body: {\n \"error\": {\n \"message\": \"account disabled by upstream\",\n \"type\": \"invalid_request_error\",\n \"param\": null,\n \"code\": \"account_deactivated\"\n }\n}" metadata=map[]`)}
	svc := NewOpenAIOAuthService(nil, client)
	account := &Account{
		ID:       79,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"refresh_token": "dead-refresh-token",
			"client_id":     "client-id-3",
		},
	}

	info, err := svc.RefreshAccountToken(context.Background(), account)
	require.Nil(t, info)
	require.Error(t, err)
	require.Contains(t, err.Error(), "OPENAI_OAUTH_REAUTH_REQUIRED")
	require.Contains(t, err.Error(), "account has been deactivated")
	require.Equal(t, int32(1), atomic.LoadInt32(&client.refreshCalls))
}

func TestOpenAIOAuthService_RefreshAccountToken_NoRefreshTokenUsesExistingAccessToken(t *testing.T) {
	client := &openaiOAuthClientRefreshStub{}
	svc := NewOpenAIOAuthService(nil, client)

	expiresAt := time.Now().Add(30 * time.Minute).UTC().Format(time.RFC3339)
	account := &Account{
		ID:       77,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "existing-access-token",
			"expires_at":   expiresAt,
			"client_id":    "client-id-1",
		},
	}

	info, err := svc.RefreshAccountToken(context.Background(), account)
	require.NoError(t, err)
	require.NotNil(t, info)
	require.Equal(t, "existing-access-token", info.AccessToken)
	require.Equal(t, "client-id-1", info.ClientID)
	require.Zero(t, atomic.LoadInt32(&client.refreshCalls), "existing access token should be reused without calling refresh")
}
