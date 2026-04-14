package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const (
	schedulerCompactSnapshotPrefix         = "sched:compact:"
	schedulerCompactSnapshotEncodingOpenAI = "openai_compact_v1"
)

var (
	schedulerCompactOpenAICredentialKeys = []string{
		"model_mapping",
	}
	schedulerCompactOpenAIExtraKeys = []string{
		"privacy_mode",
		"model_rate_limits",
		"openai_ws_force_http",
		"openai_ws_allow_store_recovery",
		"openai_oauth_responses_websockets_v2_enabled",
		"openai_apikey_responses_websockets_v2_enabled",
		"responses_websockets_v2_enabled",
		"openai_ws_enabled",
		"openai_oauth_responses_websockets_v2_mode",
		"openai_apikey_responses_websockets_v2_mode",
	}
)

type compactSnapshotMeta struct {
	Encoding   string `json:"encoding"`
	ChunkCount int    `json:"chunk_count"`
	Count      int    `json:"count"`
}

type compactOpenAIGroup struct {
	ID       int64  `json:"id"`
	Name     string `json:"name,omitempty"`
	Platform string `json:"platform,omitempty"`
}

type compactOpenAIAccount struct {
	ID                      int64                `json:"id"`
	Name                    string               `json:"name,omitempty"`
	Platform                string               `json:"platform,omitempty"`
	Type                    string               `json:"type,omitempty"`
	Status                  string               `json:"status,omitempty"`
	ErrorMessage            string               `json:"error_message,omitempty"`
	Concurrency             int                  `json:"concurrency,omitempty"`
	Priority                int                  `json:"priority,omitempty"`
	RateMultiplier          *float64             `json:"rate_multiplier,omitempty"`
	LoadFactor              *int                 `json:"load_factor,omitempty"`
	LastUsedAt              *time.Time           `json:"last_used_at,omitempty"`
	ExpiresAt               *time.Time           `json:"expires_at,omitempty"`
	AutoPauseOnExpired      bool                 `json:"auto_pause_on_expired,omitempty"`
	Schedulable             bool                 `json:"schedulable,omitempty"`
	RateLimitedAt           *time.Time           `json:"rate_limited_at,omitempty"`
	RateLimitResetAt        *time.Time           `json:"rate_limit_reset_at,omitempty"`
	OverloadUntil           *time.Time           `json:"overload_until,omitempty"`
	TempUnschedulableUntil  *time.Time           `json:"temp_unschedulable_until,omitempty"`
	TempUnschedulableReason string               `json:"temp_unschedulable_reason,omitempty"`
	GroupIDs                []int64              `json:"group_ids,omitempty"`
	Groups                  []compactOpenAIGroup `json:"groups,omitempty"`
	Credentials             map[string]any       `json:"credentials,omitempty"`
	Extra                   map[string]any       `json:"extra,omitempty"`
}

func isCompactOpenAISnapshotBucket(bucket service.SchedulerBucket) bool {
	return bucket.Platform == service.PlatformOpenAI
}

func schedulerCompactSnapshotBaseKey(bucket service.SchedulerBucket, version string) string {
	return fmt.Sprintf("%s%d:%s:%s:v%s", schedulerCompactSnapshotPrefix, bucket.GroupID, bucket.Platform, bucket.Mode, version)
}

func schedulerCompactSnapshotMetaKey(bucket service.SchedulerBucket, version string) string {
	return schedulerCompactSnapshotBaseKey(bucket, version) + ":meta"
}

func schedulerCompactSnapshotChunkKey(bucket service.SchedulerBucket, version string, chunkIndex int) string {
	return schedulerCompactSnapshotBaseKey(bucket, version) + ":chunk:" + strconv.Itoa(chunkIndex)
}

func (c *schedulerCache) getCompactSnapshot(ctx context.Context, bucket service.SchedulerBucket, version string) ([]*service.Account, bool, bool, error) {
	if !isCompactOpenAISnapshotBucket(bucket) {
		return nil, false, false, nil
	}
	metaRaw, err := c.rdb.Get(ctx, schedulerCompactSnapshotMetaKey(bucket, version)).Result()
	if err == redis.Nil {
		return nil, false, false, nil
	}
	if err != nil {
		return nil, true, false, err
	}
	var meta compactSnapshotMeta
	if err := json.Unmarshal([]byte(metaRaw), &meta); err != nil {
		return nil, true, false, err
	}
	if meta.Encoding != schedulerCompactSnapshotEncodingOpenAI {
		return nil, true, false, fmt.Errorf("unexpected compact snapshot encoding: %s", meta.Encoding)
	}
	if meta.Count <= 0 || meta.ChunkCount <= 0 {
		return nil, true, false, nil
	}
	keys := make([]string, 0, meta.ChunkCount)
	for idx := 0; idx < meta.ChunkCount; idx++ {
		keys = append(keys, schedulerCompactSnapshotChunkKey(bucket, version, idx))
	}
	values, err := c.rdb.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, true, false, err
	}
	accounts := make([]*service.Account, 0, meta.Count)
	for _, val := range values {
		if val == nil {
			return nil, true, false, nil
		}
		chunkAccounts, err := decodeCompactOpenAIChunk(val)
		if err != nil {
			return nil, true, false, err
		}
		accounts = append(accounts, chunkAccounts...)
	}
	if len(accounts) == 0 {
		return nil, true, false, nil
	}
	return accounts, true, true, nil
}

func (c *schedulerCache) setCompactSnapshot(ctx context.Context, bucket service.SchedulerBucket, version string, accounts []service.Account) error {
	meta := compactSnapshotMeta{
		Encoding: schedulerCompactSnapshotEncodingOpenAI,
		Count:    len(accounts),
	}
	chunkPayloads := make([][]byte, 0, (len(accounts)+schedulerCompactSnapshotWriteChunkSize-1)/schedulerCompactSnapshotWriteChunkSize)
	for start := 0; start < len(accounts); start += schedulerCompactSnapshotWriteChunkSize {
		end := start + schedulerCompactSnapshotWriteChunkSize
		if end > len(accounts) {
			end = len(accounts)
		}
		payload, err := encodeCompactOpenAIChunk(accounts[start:end])
		if err != nil {
			return err
		}
		chunkPayloads = append(chunkPayloads, payload)
	}
	meta.ChunkCount = len(chunkPayloads)
	metaPayload, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	cleanupKeys := make([]string, 0, len(chunkPayloads)+1)
	pipe := c.rdb.Pipeline()
	for idx, payload := range chunkPayloads {
		key := schedulerCompactSnapshotChunkKey(bucket, version, idx)
		cleanupKeys = append(cleanupKeys, key)
		pipe.Set(ctx, key, payload, schedulerBucketMetaTTL)
	}
	metaKey := schedulerCompactSnapshotMetaKey(bucket, version)
	cleanupKeys = append(cleanupKeys, metaKey)
	pipe.Set(ctx, metaKey, metaPayload, schedulerBucketMetaTTL)
	_, err = pipe.Exec(ctx)
	if err != nil {
		_ = c.rdb.Del(context.Background(), cleanupKeys...).Err()
	}
	return err
}

func (c *schedulerCache) deleteCompactSnapshot(ctx context.Context, bucket service.SchedulerBucket, version string) error {
	if version == "" || !isCompactOpenAISnapshotBucket(bucket) {
		return nil
	}
	metaKey := schedulerCompactSnapshotMetaKey(bucket, version)
	metaRaw, err := c.rdb.Get(ctx, metaKey).Result()
	if err != nil && err != redis.Nil {
		return err
	}
	keys := []string{metaKey}
	if err == nil {
		var meta compactSnapshotMeta
		if json.Unmarshal([]byte(metaRaw), &meta) == nil {
			for idx := 0; idx < meta.ChunkCount; idx++ {
				keys = append(keys, schedulerCompactSnapshotChunkKey(bucket, version, idx))
			}
		}
	}
	return c.rdb.Del(ctx, keys...).Err()
}

func encodeCompactOpenAIChunk(accounts []service.Account) ([]byte, error) {
	compact := make([]compactOpenAIAccount, 0, len(accounts))
	for _, account := range accounts {
		compact = append(compact, newCompactOpenAIAccount(account))
	}
	return json.Marshal(compact)
}

func decodeCompactOpenAIChunk(val any) ([]*service.Account, error) {
	payload, err := decodeCachedPayloadBytes(val)
	if err != nil {
		return nil, err
	}
	var compact []compactOpenAIAccount
	if err := json.Unmarshal(payload, &compact); err != nil {
		return nil, err
	}
	accounts := make([]*service.Account, 0, len(compact))
	for _, item := range compact {
		accounts = append(accounts, item.toServiceAccount())
	}
	return accounts, nil
}

func newCompactOpenAIAccount(account service.Account) compactOpenAIAccount {
	groupIDs := compactOpenAIGroupIDs(account)
	return compactOpenAIAccount{
		ID:                      account.ID,
		Name:                    account.Name,
		Platform:                account.Platform,
		Type:                    account.Type,
		Status:                  account.Status,
		ErrorMessage:            account.ErrorMessage,
		Concurrency:             account.Concurrency,
		Priority:                account.Priority,
		RateMultiplier:          cloneFloat64Pointer(account.RateMultiplier),
		LoadFactor:              cloneIntPointer(account.LoadFactor),
		LastUsedAt:              cloneTimePointer(account.LastUsedAt),
		ExpiresAt:               cloneTimePointer(account.ExpiresAt),
		AutoPauseOnExpired:      account.AutoPauseOnExpired,
		Schedulable:             account.Schedulable,
		RateLimitedAt:           cloneTimePointer(account.RateLimitedAt),
		RateLimitResetAt:        cloneTimePointer(account.RateLimitResetAt),
		OverloadUntil:           cloneTimePointer(account.OverloadUntil),
		TempUnschedulableUntil:  cloneTimePointer(account.TempUnschedulableUntil),
		TempUnschedulableReason: account.TempUnschedulableReason,
		GroupIDs:                groupIDs,
		Groups:                  compactOpenAIGroups(account.Groups),
		Credentials:             compactAllowedMap(account.Credentials, schedulerCompactOpenAICredentialKeys),
		Extra:                   compactAllowedMap(account.Extra, schedulerCompactOpenAIExtraKeys),
	}
}

func (a compactOpenAIAccount) toServiceAccount() *service.Account {
	account := &service.Account{
		ID:                      a.ID,
		Name:                    a.Name,
		Platform:                a.Platform,
		Type:                    a.Type,
		Status:                  a.Status,
		ErrorMessage:            a.ErrorMessage,
		Concurrency:             a.Concurrency,
		Priority:                a.Priority,
		RateMultiplier:          cloneFloat64Pointer(a.RateMultiplier),
		LoadFactor:              cloneIntPointer(a.LoadFactor),
		LastUsedAt:              cloneTimePointer(a.LastUsedAt),
		ExpiresAt:               cloneTimePointer(a.ExpiresAt),
		AutoPauseOnExpired:      a.AutoPauseOnExpired,
		Schedulable:             a.Schedulable,
		RateLimitedAt:           cloneTimePointer(a.RateLimitedAt),
		RateLimitResetAt:        cloneTimePointer(a.RateLimitResetAt),
		OverloadUntil:           cloneTimePointer(a.OverloadUntil),
		TempUnschedulableUntil:  cloneTimePointer(a.TempUnschedulableUntil),
		TempUnschedulableReason: a.TempUnschedulableReason,
		GroupIDs:                append([]int64(nil), a.GroupIDs...),
		Groups:                  restoreCompactOpenAIGroups(a.Groups),
		Credentials:             cloneCompactMap(a.Credentials),
		Extra:                   cloneCompactMap(a.Extra),
	}
	return account
}

func compactOpenAIGroups(groups []*service.Group) []compactOpenAIGroup {
	if len(groups) == 0 {
		return nil
	}
	result := make([]compactOpenAIGroup, 0, len(groups))
	seen := make(map[int64]struct{}, len(groups))
	for _, group := range groups {
		if group == nil || group.ID <= 0 {
			continue
		}
		if _, exists := seen[group.ID]; exists {
			continue
		}
		seen[group.ID] = struct{}{}
		result = append(result, compactOpenAIGroup{ID: group.ID, Name: group.Name, Platform: group.Platform})
	}
	return result
}

func restoreCompactOpenAIGroups(groups []compactOpenAIGroup) []*service.Group {
	if len(groups) == 0 {
		return nil
	}
	result := make([]*service.Group, 0, len(groups))
	for _, group := range groups {
		g := group
		result = append(result, &service.Group{ID: g.ID, Name: g.Name, Platform: g.Platform})
	}
	return result
}

func compactOpenAIGroupIDs(account service.Account) []int64 {
	if len(account.Groups) == 0 && len(account.GroupIDs) == 0 && len(account.AccountGroups) == 0 {
		return nil
	}
	seen := make(map[int64]struct{}, len(account.Groups)+len(account.GroupIDs)+len(account.AccountGroups))
	result := make([]int64, 0, len(seen))
	appendGroupID := func(groupID int64) {
		if groupID <= 0 {
			return
		}
		if _, exists := seen[groupID]; exists {
			return
		}
		seen[groupID] = struct{}{}
		result = append(result, groupID)
	}
	for _, group := range account.Groups {
		if group != nil {
			appendGroupID(group.ID)
		}
	}
	for _, groupID := range account.GroupIDs {
		appendGroupID(groupID)
	}
	for _, accountGroup := range account.AccountGroups {
		appendGroupID(accountGroup.GroupID)
	}
	return result
}

func compactAllowedMap(source map[string]any, allowedKeys []string) map[string]any {
	if len(source) == 0 || len(allowedKeys) == 0 {
		return nil
	}
	result := make(map[string]any, len(allowedKeys))
	for _, key := range allowedKeys {
		value, ok := source[key]
		if !ok || value == nil {
			continue
		}
		result[key] = cloneCompactValue(value)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func cloneCompactMap(source map[string]any) map[string]any {
	if len(source) == 0 {
		return nil
	}
	result := make(map[string]any, len(source))
	for key, value := range source {
		result[key] = cloneCompactValue(value)
	}
	return result
}

func cloneCompactValue(value any) any {
	switch v := value.(type) {
	case nil:
		return nil
	case string, bool, float64, float32, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, json.Number:
		return v
	case map[string]any:
		copied := make(map[string]any, len(v))
		for key, item := range v {
			copied[key] = cloneCompactValue(item)
		}
		return copied
	case map[string]string:
		copied := make(map[string]any, len(v))
		for key, item := range v {
			copied[key] = item
		}
		return copied
	case []any:
		copied := make([]any, len(v))
		for idx, item := range v {
			copied[idx] = cloneCompactValue(item)
		}
		return copied
	case []string:
		copied := make([]any, len(v))
		for idx, item := range v {
			copied[idx] = item
		}
		return copied
	case []int64:
		copied := make([]any, len(v))
		for idx, item := range v {
			copied[idx] = item
		}
		return copied
	case []int:
		copied := make([]any, len(v))
		for idx, item := range v {
			copied[idx] = item
		}
		return copied
	default:
		return v
	}
}

func cloneTimePointer(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

func cloneFloat64Pointer(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

func cloneIntPointer(value *int) *int {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}
