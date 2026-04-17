package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const chatgptWhamUsageURL = "https://chatgpt.com/backend-api/wham/usage"

func (s *AccountUsageService) probeOpenAICodexSnapshotViaWham(ctx context.Context, account *Account) (map[string]any, *time.Time, error) {
	accessToken := account.GetOpenAIAccessToken()
	if accessToken == "" {
		return nil, nil, fmt.Errorf("no access token available")
	}

	reqCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, chatgptWhamUsageURL, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("create openai wham usage request: %w", err)
	}
	s.applyOpenAIUsageProbeHeaders(reqCtx, req, account, accessToken, "application/json")

	resp, err := s.doOpenAIUsageProbe(reqCtx, account, req)
	if err != nil {
		return nil, nil, fmt.Errorf("openai wham usage request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusUnauthorized {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		probeErr := buildOpenAIUsageProbeUnauthorizedError(body)
		s.handleOpenAIUsageProbeUnauthorized(reqCtx, account, body, probeErr)
		return nil, nil, probeErr
	}
	if resp.StatusCode == http.StatusForbidden {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		probeErr := buildOpenAIUsageProbeForbiddenError(body)
		s.handleOpenAIUsageProbeForbidden(reqCtx, account, body, probeErr)
		return nil, nil, probeErr
	}

	updates, resetAt, err := extractOpenAIWhamUsageSnapshot(resp)
	if err != nil {
		return nil, nil, err
	}
	if len(updates) > 0 || resetAt != nil {
		return updates, resetAt, nil
	}
	return nil, nil, nil
}

type openAIWhamUsageResponse struct {
	PlanType  string               `json:"plan_type"`
	RateLimit *openAIWhamRateLimit `json:"rate_limit"`
}

type openAIWhamRateLimit struct {
	PrimaryWindow   *openAIWhamUsageWindow `json:"primary_window"`
	SecondaryWindow *openAIWhamUsageWindow `json:"secondary_window"`
}

type openAIWhamUsageWindow struct {
	UsedPercent        *float64 `json:"used_percent"`
	LimitWindowSeconds *int     `json:"limit_window_seconds"`
	ResetAfterSeconds  *int     `json:"reset_after_seconds"`
	ResetAt            *int64   `json:"reset_at"`
}

func extractOpenAIWhamUsageSnapshot(resp *http.Response) (map[string]any, *time.Time, error) {
	if resp == nil {
		return nil, nil, nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 128*1024))
	if err != nil {
		return nil, nil, fmt.Errorf("read openai wham usage body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := sanitizeUpstreamErrorMessage(strings.TrimSpace(extractUpstreamErrorMessage(body)))
		if msg != "" {
			msg = truncateForLog([]byte(msg), 512)
			return nil, nil, fmt.Errorf("openai wham usage returned status %d: %s", resp.StatusCode, msg)
		}
		trimmedBody := truncateForLog(body, 512)
		if trimmedBody != "" {
			return nil, nil, fmt.Errorf("openai wham usage returned status %d: %s", resp.StatusCode, trimmedBody)
		}
		return nil, nil, fmt.Errorf("openai wham usage returned status %d", resp.StatusCode)
	}

	var payload openAIWhamUsageResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, nil, fmt.Errorf("decode openai wham usage body: %w", err)
	}

	now := time.Now().UTC()
	snapshot := buildOpenAICodexSnapshotFromWhamUsage(payload.RateLimit, now)
	if snapshot == nil {
		return nil, nil, nil
	}
	updates := buildCodexUsageExtraUpdates(snapshot, now)
	resetAt := codexRateLimitResetAtFromSnapshot(snapshot, now)
	return updates, resetAt, nil
}

func buildOpenAICodexSnapshotFromWhamUsage(rateLimit *openAIWhamRateLimit, now time.Time) *OpenAICodexUsageSnapshot {
	if rateLimit == nil {
		return nil
	}
	snapshot := &OpenAICodexUsageSnapshot{
		UpdatedAt: now.UTC().Format(time.RFC3339),
	}
	hasData := false
	if rateLimit.PrimaryWindow != nil {
		snapshot.PrimaryUsedPercent = cloneOptionalFloat64(rateLimit.PrimaryWindow.UsedPercent)
		snapshot.PrimaryResetAfterSeconds = whamResetAfterSeconds(rateLimit.PrimaryWindow.ResetAfterSeconds, rateLimit.PrimaryWindow.ResetAt, now)
		snapshot.PrimaryWindowMinutes = whamWindowMinutes(rateLimit.PrimaryWindow.LimitWindowSeconds)
		hasData = hasData || snapshot.PrimaryUsedPercent != nil || snapshot.PrimaryResetAfterSeconds != nil || snapshot.PrimaryWindowMinutes != nil
	}
	if rateLimit.SecondaryWindow != nil {
		snapshot.SecondaryUsedPercent = cloneOptionalFloat64(rateLimit.SecondaryWindow.UsedPercent)
		snapshot.SecondaryResetAfterSeconds = whamResetAfterSeconds(rateLimit.SecondaryWindow.ResetAfterSeconds, rateLimit.SecondaryWindow.ResetAt, now)
		snapshot.SecondaryWindowMinutes = whamWindowMinutes(rateLimit.SecondaryWindow.LimitWindowSeconds)
		hasData = hasData || snapshot.SecondaryUsedPercent != nil || snapshot.SecondaryResetAfterSeconds != nil || snapshot.SecondaryWindowMinutes != nil
	}
	if !hasData {
		return nil
	}
	return snapshot
}

func cloneOptionalFloat64(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

func whamWindowMinutes(limitWindowSeconds *int) *int {
	if limitWindowSeconds == nil || *limitWindowSeconds <= 0 {
		return nil
	}
	minutes := *limitWindowSeconds / 60
	if minutes <= 0 {
		return nil
	}
	return &minutes
}

func whamResetAfterSeconds(resetAfter *int, resetAt *int64, now time.Time) *int {
	if resetAfter != nil {
		copied := *resetAfter
		return &copied
	}
	if resetAt == nil {
		return nil
	}
	seconds := int(time.Until(time.Unix(*resetAt, 0).UTC()).Seconds())
	if seconds < 0 {
		seconds = 0
	}
	return &seconds
}
