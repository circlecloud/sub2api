package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type accountUsageCodexProbeRepo struct {
	stubOpenAIAccountRepo
	updateExtraCh   chan map[string]any
	rateLimitCh     chan time.Time
	errorID         int64
	errorMessage    string
	clearErrorID    int64
	clearErrorCalls int
}

func (r *accountUsageCodexProbeRepo) UpdateExtra(_ context.Context, _ int64, updates map[string]any) error {
	if r.updateExtraCh != nil {
		copied := make(map[string]any, len(updates))
		for k, v := range updates {
			copied[k] = v
		}
		r.updateExtraCh <- copied
	}
	return nil
}

func (r *accountUsageCodexProbeRepo) SetRateLimited(_ context.Context, _ int64, resetAt time.Time) error {
	if r.rateLimitCh != nil {
		r.rateLimitCh <- resetAt
	}
	return nil
}

func (r *accountUsageCodexProbeRepo) SetError(_ context.Context, id int64, errorMsg string) error {
	r.errorID = id
	r.errorMessage = errorMsg
	return nil
}

func (r *accountUsageCodexProbeRepo) ClearError(_ context.Context, id int64) error {
	r.clearErrorID = id
	r.clearErrorCalls++
	for i := range r.accounts {
		if r.accounts[i].ID == id {
			r.accounts[i].Status = StatusActive
			r.accounts[i].ErrorMessage = ""
			break
		}
	}
	return nil
}

func TestShouldRefreshOpenAICodexSnapshot(t *testing.T) {
	t.Parallel()

	rateLimitedUntil := time.Now().Add(5 * time.Minute)
	now := time.Now()
	usage := &UsageInfo{
		FiveHour: &UsageProgress{Utilization: 0},
		SevenDay: &UsageProgress{Utilization: 0},
	}

	if !shouldRefreshOpenAICodexSnapshot(&Account{RateLimitResetAt: &rateLimitedUntil}, usage, now) {
		t.Fatal("expected rate-limited account to force codex snapshot refresh")
	}

	if shouldRefreshOpenAICodexSnapshot(&Account{}, usage, now) {
		t.Fatal("expected complete non-rate-limited usage to skip codex snapshot refresh")
	}

	if !shouldRefreshOpenAICodexSnapshot(&Account{}, &UsageInfo{FiveHour: nil, SevenDay: &UsageProgress{}}, now) {
		t.Fatal("expected missing 5h snapshot to require refresh")
	}

	staleAt := now.Add(-(openAIProbeCacheTTL + time.Minute)).Format(time.RFC3339)
	if !shouldRefreshOpenAICodexSnapshot(&Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			"openai_oauth_responses_websockets_v2_enabled": true,
			"codex_usage_updated_at":                       staleAt,
		},
	}, usage, now) {
		t.Fatal("expected stale ws snapshot to trigger refresh")
	}
}

func TestExtractOpenAICodexProbeUpdatesAccepts429WithCodexHeaders(t *testing.T) {
	t.Parallel()

	headers := make(http.Header)
	headers.Set("x-codex-primary-used-percent", "100")
	headers.Set("x-codex-primary-reset-after-seconds", "604800")
	headers.Set("x-codex-primary-window-minutes", "10080")
	headers.Set("x-codex-secondary-used-percent", "100")
	headers.Set("x-codex-secondary-reset-after-seconds", "18000")
	headers.Set("x-codex-secondary-window-minutes", "300")

	updates, err := extractOpenAICodexProbeUpdates(&http.Response{StatusCode: http.StatusTooManyRequests, Header: headers})
	if err != nil {
		t.Fatalf("extractOpenAICodexProbeUpdates() error = %v", err)
	}
	if len(updates) == 0 {
		t.Fatal("expected codex probe updates from 429 headers")
	}
	if got := updates["codex_5h_used_percent"]; got != 100.0 {
		t.Fatalf("codex_5h_used_percent = %v, want 100", got)
	}
	if got := updates["codex_7d_used_percent"]; got != 100.0 {
		t.Fatalf("codex_7d_used_percent = %v, want 100", got)
	}
}

func TestExtractOpenAICodexProbeSnapshotAccepts429WithResetAt(t *testing.T) {
	t.Parallel()

	headers := make(http.Header)
	headers.Set("x-codex-primary-used-percent", "100")
	headers.Set("x-codex-primary-reset-after-seconds", "604800")
	headers.Set("x-codex-primary-window-minutes", "10080")
	headers.Set("x-codex-secondary-used-percent", "100")
	headers.Set("x-codex-secondary-reset-after-seconds", "18000")
	headers.Set("x-codex-secondary-window-minutes", "300")

	updates, resetAt, err := extractOpenAICodexProbeSnapshot(&http.Response{StatusCode: http.StatusTooManyRequests, Header: headers})
	if err != nil {
		t.Fatalf("extractOpenAICodexProbeSnapshot() error = %v", err)
	}
	if len(updates) == 0 {
		t.Fatal("expected codex probe updates from 429 headers")
	}
	if resetAt == nil {
		t.Fatal("expected resetAt from exhausted codex headers")
	}
}

func TestExtractOpenAICodexProbeSnapshotIncludesBadRequestBody(t *testing.T) {
	t.Parallel()

	updates, resetAt, err := extractOpenAICodexProbeSnapshot(&http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"probe payload rejected"}}`)),
	})
	require.Error(t, err)
	require.Nil(t, updates)
	require.Nil(t, resetAt)
	require.Contains(t, err.Error(), "status 400")
	require.Contains(t, err.Error(), "probe payload rejected")
}

func TestAccountUsageService_PersistOpenAICodexProbeSnapshotOnlyUpdatesExtra(t *testing.T) {
	t.Parallel()

	repo := &accountUsageCodexProbeRepo{
		updateExtraCh: make(chan map[string]any, 1),
		rateLimitCh:   make(chan time.Time, 1),
	}
	svc := &AccountUsageService{accountRepo: repo}
	svc.persistOpenAICodexProbeSnapshot(321, map[string]any{
		"codex_7d_used_percent": 100.0,
		"codex_7d_reset_at":     time.Now().Add(2 * time.Hour).UTC().Truncate(time.Second).Format(time.RFC3339),
	}, nil)

	select {
	case updates := <-repo.updateExtraCh:
		if got := updates["codex_7d_used_percent"]; got != 100.0 {
			t.Fatalf("codex_7d_used_percent = %v, want 100", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("等待 codex 探测快照写入 extra 超时")
	}

	select {
	case got := <-repo.rateLimitCh:
		t.Fatalf("不应将探测快照写入运行时限流状态: %v", got)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestAccountUsageService_GetOpenAIUsage_PromotesCodexExtraToRateLimit(t *testing.T) {
	t.Parallel()

	resetAt := time.Now().Add(6 * 24 * time.Hour).UTC().Truncate(time.Second)
	repo := &accountUsageCodexProbeRepo{
		rateLimitCh: make(chan time.Time, 1),
	}
	svc := &AccountUsageService{accountRepo: repo}
	account := &Account{
		ID:       123,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			"codex_5h_used_percent": 1.0,
			"codex_5h_reset_at":     time.Now().Add(2 * time.Hour).UTC().Truncate(time.Second).Format(time.RFC3339),
			"codex_7d_used_percent": 100.0,
			"codex_7d_reset_at":     resetAt.Format(time.RFC3339),
		},
	}

	usage, err := svc.getOpenAIUsage(context.Background(), account, false)
	if err != nil {
		t.Fatalf("getOpenAIUsage() error = %v", err)
	}
	if usage.SevenDay == nil || usage.SevenDay.Utilization != 100.0 {
		t.Fatalf("预期 7 天用量仍然可见，实际为 %#v", usage.SevenDay)
	}
	if account.RateLimitResetAt == nil || !account.RateLimitResetAt.Equal(resetAt) {
		t.Fatalf("预期 codex extra 同步到运行时限流状态，实际为 %v", account.RateLimitResetAt)
	}
	select {
	case got := <-repo.rateLimitCh:
		if !got.Equal(resetAt) {
			t.Fatalf("SetRateLimited() = %v, want %v", got, resetAt)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("等待 SetRateLimited 调用超时")
	}
}

func TestAccountUsageService_HandleOpenAIUsageProbeUnauthorizedMarksError(t *testing.T) {
	t.Parallel()

	repo := &accountUsageCodexProbeRepo{}
	svc := &AccountUsageService{accountRepo: repo}
	account := &Account{ID: 901, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive}
	body := []byte(`{"error":{"code":"token_revoked","message":"revoked by upstream"}}`)

	svc.handleOpenAIUsageProbeUnauthorized(context.Background(), account, body, nil)

	require.Equal(t, int64(901), repo.errorID)
	require.Equal(t, StatusError, account.Status)
	require.Contains(t, repo.errorMessage, "revoked by upstream")
	require.Equal(t, repo.errorMessage, account.ErrorMessage)
}

func TestBuildOpenAIUsageProbeUnauthorizedErrorIncludesUpstreamMessage(t *testing.T) {
	t.Parallel()

	err := buildOpenAIUsageProbeUnauthorizedError([]byte(`{"error":{"message":"session expired"}}`))
	require.Error(t, err)
	require.True(t, isOpenAIUsageProbeUnauthorizedError(err))
	require.Contains(t, err.Error(), "session expired")
}

func TestAccountUsageService_GetUsage_DoesNotClearRecoverableErrorOnDegradedOpenAIUsage(t *testing.T) {
	t.Parallel()

	repo := &accountUsageCodexProbeRepo{stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: []Account{{
		ID:           902,
		Platform:     PlatformOpenAI,
		Type:         AccountTypeOAuth,
		Status:       StatusError,
		ErrorMessage: "unauthenticated",
	}}}}
	svc := &AccountUsageService{accountRepo: repo}

	usage, err := svc.GetUsage(context.Background(), 902, true)
	require.NoError(t, err)
	require.NotNil(t, usage)
	require.Equal(t, errorCodeNetworkError, usage.ErrorCode)
	require.Zero(t, repo.clearErrorCalls, "降级 usage 返回不应清理账号错误状态")
	require.Equal(t, StatusError, repo.accounts[0].Status)
	require.Equal(t, "unauthenticated", repo.accounts[0].ErrorMessage)
}

func TestAccountUsageService_GetUsage_ClearsRecoverableErrorAfterHealthyOpenAIUsage(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	repo := &accountUsageCodexProbeRepo{stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: []Account{{
		ID:           903,
		Platform:     PlatformOpenAI,
		Type:         AccountTypeOAuth,
		Status:       StatusError,
		ErrorMessage: "unauthenticated",
		Extra: map[string]any{
			"openai_oauth_responses_websockets_v2_enabled": true,
			"codex_usage_updated_at":                       now.Format(time.RFC3339),
			"codex_5h_used_percent":                        12.0,
			"codex_5h_reset_at":                            now.Add(2 * time.Hour).Format(time.RFC3339),
			"codex_7d_used_percent":                        34.0,
			"codex_7d_reset_at":                            now.Add(24 * time.Hour).Format(time.RFC3339),
		},
	}}}}
	svc := &AccountUsageService{accountRepo: repo}

	usage, err := svc.GetUsage(context.Background(), 903, false)
	require.NoError(t, err)
	require.NotNil(t, usage)
	require.Empty(t, usage.ErrorCode)
	require.Empty(t, usage.Error)
	require.Equal(t, 1, repo.clearErrorCalls)
	require.Equal(t, int64(903), repo.clearErrorID)
	require.Equal(t, StatusActive, repo.accounts[0].Status)
	require.Empty(t, repo.accounts[0].ErrorMessage)
}

func TestBuildCodexUsageProgressFromExtra_ZerosExpiredWindow(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 16, 12, 0, 0, 0, time.UTC)

	t.Run("expired 5h window zeroes utilization", func(t *testing.T) {
		extra := map[string]any{
			"codex_5h_used_percent": 42.0,
			"codex_5h_reset_at":     "2026-03-16T10:00:00Z", // 2h ago
		}
		progress := buildCodexUsageProgressFromExtra(extra, "5h", now)
		if progress == nil {
			t.Fatal("expected non-nil progress")
		}
		if progress.Utilization != 0 {
			t.Fatalf("expected Utilization=0 for expired window, got %v", progress.Utilization)
		}
		if progress.RemainingSeconds != 0 {
			t.Fatalf("expected RemainingSeconds=0, got %v", progress.RemainingSeconds)
		}
	})

	t.Run("active 5h window keeps utilization", func(t *testing.T) {
		resetAt := now.Add(2 * time.Hour).Format(time.RFC3339)
		extra := map[string]any{
			"codex_5h_used_percent": 42.0,
			"codex_5h_reset_at":     resetAt,
		}
		progress := buildCodexUsageProgressFromExtra(extra, "5h", now)
		if progress == nil {
			t.Fatal("expected non-nil progress")
		}
		if progress.Utilization != 42.0 {
			t.Fatalf("expected Utilization=42, got %v", progress.Utilization)
		}
	})

	t.Run("expired 7d window zeroes utilization", func(t *testing.T) {
		extra := map[string]any{
			"codex_7d_used_percent": 88.0,
			"codex_7d_reset_at":     "2026-03-15T00:00:00Z", // yesterday
		}
		progress := buildCodexUsageProgressFromExtra(extra, "7d", now)
		if progress == nil {
			t.Fatal("expected non-nil progress")
		}
		if progress.Utilization != 0 {
			t.Fatalf("expected Utilization=0 for expired 7d window, got %v", progress.Utilization)
		}
	})
}
