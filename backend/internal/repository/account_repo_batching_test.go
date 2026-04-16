package repository

import (
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestNormalizePositiveInt64s_RemovesInvalidAndDuplicates(t *testing.T) {
	ids := []int64{0, -1, 5, 3, 5, 7, 3, 9}

	normalized := normalizePositiveInt64s(ids)

	require.Equal(t, []int64{5, 3, 7, 9}, normalized)
}

func TestChunkPositiveInt64s_SplitsLargeInput(t *testing.T) {
	ids := make([]int64, 0, accountRepoIDBatchChunkSize+3)
	for i := 1; i <= accountRepoIDBatchChunkSize+3; i++ {
		ids = append(ids, int64(i))
	}
	ids = append(ids, 1, 2, 0, -10)

	chunks := chunkPositiveInt64s(ids, accountRepoIDBatchChunkSize)

	require.Len(t, chunks, 2)
	require.Len(t, chunks[0], accountRepoIDBatchChunkSize)
	require.Len(t, chunks[1], 3)
	require.Equal(t, int64(1), chunks[0][0])
	require.Equal(t, int64(accountRepoIDBatchChunkSize), chunks[0][accountRepoIDBatchChunkSize-1])
	require.Equal(t, []int64{int64(accountRepoIDBatchChunkSize + 1), int64(accountRepoIDBatchChunkSize + 2), int64(accountRepoIDBatchChunkSize + 3)}, chunks[1])
}

func TestOpsRealtimeAccountsToService_AttachesLoadedGroups(t *testing.T) {
	repo := &accountRepository{}
	accounts := []*dbent.Account{{
		ID:          42,
		Name:        "ops-account",
		Platform:    service.PlatformOpenAI,
		Concurrency: 3,
		Status:      service.StatusActive,
		Schedulable: true,
	}}
	groupsByAccount := map[int64][]*service.Group{
		42: {{ID: 7, Name: "group-7", Platform: service.PlatformOpenAI}},
	}
	groupIDsByAccount := map[int64][]int64{42: {7}}

	out := repo.opsRealtimeAccountsToService(accounts, groupsByAccount, groupIDsByAccount)

	require.Len(t, out, 1)
	require.Equal(t, int64(42), out[0].ID)
	require.Equal(t, []int64{7}, out[0].GroupIDs)
	require.Len(t, out[0].Groups, 1)
	require.Equal(t, int64(7), out[0].Groups[0].ID)
	require.Equal(t, "group-7", out[0].Groups[0].Name)
}

func TestAccountEntityToOpsRealtimeService_DropsHeavyFields(t *testing.T) {
	errMsg := "ops error"
	note := "keep out"
	proxyID := int64(99)
	loadFactor := 12
	sessionStatus := "window-open"
	rateLimitResetAt := time.Now().UTC().Truncate(time.Second)
	overloadUntil := rateLimitResetAt.Add(5 * time.Minute)
	tempUnschedulableUntil := rateLimitResetAt.Add(10 * time.Minute)

	out := accountEntityToOpsRealtimeService(&dbent.Account{
		ID:                     11,
		Name:                   "ops-lightweight",
		Notes:                  &note,
		Platform:               service.PlatformOpenAI,
		Type:                   service.AccountTypeOAuth,
		Credentials:            map[string]any{"refresh_token": "secret"},
		Extra:                  map[string]any{"privacy_mode": service.PrivacyModeTrainingOff},
		ProxyID:                &proxyID,
		Concurrency:            7,
		LoadFactor:             &loadFactor,
		Priority:               88,
		RateMultiplier:         1.5,
		Status:                 service.StatusError,
		ErrorMessage:           &errMsg,
		RateLimitResetAt:       &rateLimitResetAt,
		OverloadUntil:          &overloadUntil,
		TempUnschedulableUntil: &tempUnschedulableUntil,
		SessionWindowStatus:    &sessionStatus,
	})

	require.NotNil(t, out)
	require.Equal(t, int64(11), out.ID)
	require.Equal(t, "ops-lightweight", out.Name)
	require.Equal(t, service.PlatformOpenAI, out.Platform)
	require.Equal(t, 7, out.Concurrency)
	require.Equal(t, service.StatusError, out.Status)
	require.Equal(t, "ops error", out.ErrorMessage)
	require.False(t, out.Schedulable)
	require.NotNil(t, out.RateLimitResetAt)
	require.WithinDuration(t, rateLimitResetAt, *out.RateLimitResetAt, time.Second)
	require.NotNil(t, out.OverloadUntil)
	require.WithinDuration(t, overloadUntil, *out.OverloadUntil, time.Second)
	require.NotNil(t, out.TempUnschedulableUntil)
	require.WithinDuration(t, tempUnschedulableUntil, *out.TempUnschedulableUntil, time.Second)

	require.Nil(t, out.Notes)
	require.Empty(t, out.Type)
	require.Nil(t, out.Credentials)
	require.Nil(t, out.Extra)
	require.Nil(t, out.ProxyID)
	require.Zero(t, out.Priority)
	require.Nil(t, out.RateMultiplier)
	require.Nil(t, out.Proxy)
	require.Empty(t, out.Groups)
	require.Empty(t, out.GroupIDs)
	require.Empty(t, out.AccountGroups)
	require.Empty(t, out.SessionWindowStatus)
}
