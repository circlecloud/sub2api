package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const (
	opsRealtimeAccountKeyPrefix      = "ops:rt:v1:account:"
	opsRealtimeAccountsReadyKey      = "ops:rt:v1:index:accounts:ready"
	opsRealtimeAccountsAllKey        = "ops:rt:v1:index:accounts:all"
	opsRealtimePlatformIndexPrefix   = "ops:rt:v1:index:accounts:platform:"
	opsRealtimeGroupIndexPrefix      = "ops:rt:v1:index:accounts:group:"
	opsRealtimePlatformIndexKeysKey  = "ops:rt:v1:index:accounts:platform_keys"
	opsRealtimeGroupIndexKeysKey     = "ops:rt:v1:index:accounts:group_keys"
	opsRealtimeWarmAccountKeyPrefix  = "ops:rt:v1:warm:account:"
	opsRealtimeWarmAccountIndexKey   = "ops:rt:v1:warm:index:accounts"
	opsRealtimeWarmBucketMetaPrefix  = "ops:rt:v1:warm:bucket:"
	opsRealtimeWarmBucketIndexKey    = "ops:rt:v1:warm:index:buckets"
	opsRealtimeWarmBucketMembersPart = ":members"
	opsRealtimeWarmBucketMetaPart    = ":meta"
	opsRealtimeWarmGlobalKey         = "ops:rt:v1:warm:global"
	opsRealtimeWarmOverviewKey       = "ops:rt:v1:warm:overview"
	opsRealtimeReconcileLeaderKey    = "ops:rt:v1:reconcile:leader"
)

var (
	opsRealtimeWarmAccountUpsertScript = redis.NewScript(`
		local key = KEYS[1]
		local updated = tonumber(ARGV[1])
		local current = tonumber(redis.call('HGET', key, 'updated_at_unix_ms') or '0')
		if current > updated then
			return 0
		end
		redis.call('HSET', key,
			'state', ARGV[2],
			'verified_at_unix_ms', ARGV[3],
			'expires_at_unix_ms', ARGV[4],
			'fail_until_unix_ms', ARGV[5],
			'network_error_at_unix_ms', ARGV[6],
			'network_error_until_unix_ms', ARGV[7],
			'updated_at_unix_ms', ARGV[8]
		)
		return 1
	`)
	opsRealtimeWarmHashMaxScript = redis.NewScript(`
		local key = KEYS[1]
		local field = ARGV[1]
		local incoming = tonumber(ARGV[2])
		local current = tonumber(redis.call('HGET', key, field) or '0')
		if incoming > current then
			redis.call('HSET', key, field, ARGV[2])
			return 1
		end
		return 0
	`)
	opsRealtimeWarmRemoveBucketAccountScript = redis.NewScript(`
		local key = KEYS[1]
		local suffix = ':' .. ARGV[1]
		local members = redis.call('ZRANGE', key, 0, -1)
		local removed = 0
		for _, member in ipairs(members) do
			if string.sub(member, -string.len(suffix)) == suffix then
				removed = removed + redis.call('ZREM', key, member)
			end
		end
		return removed
	`)
	opsRealtimeLeaderReleaseScript = redis.NewScript(`
		if redis.call('GET', KEYS[1]) == ARGV[1] then
			return redis.call('DEL', KEYS[1])
		end
		return 0
	`)
)

type opsRealtimeCache struct {
	rdb *redis.Client
}

func NewOpsRealtimeCache(rdb *redis.Client) service.OpsRealtimeCache {
	return &opsRealtimeCache{rdb: rdb}
}

// invalidateWarmPoolOverviewSnapshot 仅用于账号索引等结构性变化；
// warm state / bucket member 抖动依赖 snapshot TTL 自然收敛，避免 overview 重建风暴。
func (c *opsRealtimeCache) invalidateWarmPoolOverviewSnapshot(ctx context.Context) {
	if c == nil || c.rdb == nil {
		return
	}
	_, _ = c.rdb.Del(ctx, opsRealtimeWarmOverviewKey).Result()
}

func (c *opsRealtimeCache) IsAccountIndexReady(ctx context.Context) (bool, error) {
	count, err := c.rdb.Exists(ctx, opsRealtimeAccountsReadyKey).Result()
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (c *opsRealtimeCache) ReplaceAccounts(ctx context.Context, accounts []*service.OpsRealtimeAccountCacheEntry) error {
	oldIDs, err := c.rdb.SMembers(ctx, opsRealtimeAccountsAllKey).Result()
	if err != nil && err != redis.Nil {
		return err
	}
	platformIndexKeys, err := c.rdb.SMembers(ctx, opsRealtimePlatformIndexKeysKey).Result()
	if err != nil && err != redis.Nil {
		return err
	}
	groupIndexKeys, err := c.rdb.SMembers(ctx, opsRealtimeGroupIndexKeysKey).Result()
	if err != nil && err != redis.Nil {
		return err
	}

	cleanupKeys := make([]string, 0, len(oldIDs)+len(platformIndexKeys)+len(groupIndexKeys)+4)
	for _, id := range oldIDs {
		cleanupKeys = append(cleanupKeys, opsRealtimeAccountKey(id))
	}
	cleanupKeys = append(cleanupKeys, opsRealtimeAccountsReadyKey, opsRealtimeAccountsAllKey, opsRealtimePlatformIndexKeysKey, opsRealtimeGroupIndexKeysKey)
	cleanupKeys = append(cleanupKeys, platformIndexKeys...)
	cleanupKeys = append(cleanupKeys, groupIndexKeys...)
	if len(cleanupKeys) > 0 {
		if err := c.rdb.Del(ctx, cleanupKeys...).Err(); err != nil {
			return err
		}
	}

	pipe := c.rdb.Pipeline()
	for _, entry := range accounts {
		if entry == nil || entry.AccountID <= 0 {
			continue
		}
		payload, err := json.Marshal(entry)
		if err != nil {
			return err
		}
		accountID := strconv.FormatInt(entry.AccountID, 10)
		pipe.Set(ctx, opsRealtimeAccountKey(accountID), payload, 0)
		pipe.SAdd(ctx, opsRealtimeAccountsAllKey, accountID)
		platformKey := opsRealtimePlatformIndexKey(entry.Platform)
		pipe.SAdd(ctx, platformKey, accountID)
		pipe.SAdd(ctx, opsRealtimePlatformIndexKeysKey, platformKey)
		for _, groupID := range normalizeOpsRealtimeGroupIDs(entry.GroupIDs, entry.Groups) {
			groupKey := opsRealtimeGroupIndexKey(groupID)
			pipe.SAdd(ctx, groupKey, accountID)
			pipe.SAdd(ctx, opsRealtimeGroupIndexKeysKey, groupKey)
		}
	}
	pipe.Set(ctx, opsRealtimeAccountsReadyKey, "1", 0)
	_, err = pipe.Exec(ctx)
	if err != nil {
		return err
	}
	c.invalidateWarmPoolOverviewSnapshot(ctx)
	return nil
}

func (c *opsRealtimeCache) UpsertAccount(ctx context.Context, account *service.OpsRealtimeAccountCacheEntry) error {
	if account == nil || account.AccountID <= 0 {
		return nil
	}
	previous, err := c.getAccount(ctx, account.AccountID)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(account)
	if err != nil {
		return err
	}
	accountID := strconv.FormatInt(account.AccountID, 10)
	pipe := c.rdb.Pipeline()
	pipe.Set(ctx, opsRealtimeAccountKey(accountID), payload, 0)
	pipe.SAdd(ctx, opsRealtimeAccountsAllKey, accountID)
	platformKey := opsRealtimePlatformIndexKey(account.Platform)
	pipe.SAdd(ctx, platformKey, accountID)
	pipe.SAdd(ctx, opsRealtimePlatformIndexKeysKey, platformKey)
	if previous != nil && strings.TrimSpace(previous.Platform) != "" && previous.Platform != account.Platform {
		pipe.SRem(ctx, opsRealtimePlatformIndexKey(previous.Platform), accountID)
	}
	previousGroups := normalizeOpsRealtimeGroupIDs(nil, nil)
	if previous != nil {
		previousGroups = normalizeOpsRealtimeGroupIDs(previous.GroupIDs, previous.Groups)
	}
	currentGroups := normalizeOpsRealtimeGroupIDs(account.GroupIDs, account.Groups)
	currentGroupSet := make(map[int64]struct{}, len(currentGroups))
	for _, groupID := range currentGroups {
		if groupID <= 0 {
			continue
		}
		currentGroupSet[groupID] = struct{}{}
		groupKey := opsRealtimeGroupIndexKey(groupID)
		pipe.SAdd(ctx, groupKey, accountID)
		pipe.SAdd(ctx, opsRealtimeGroupIndexKeysKey, groupKey)
	}
	for _, groupID := range previousGroups {
		if groupID <= 0 {
			continue
		}
		if _, ok := currentGroupSet[groupID]; ok {
			continue
		}
		pipe.SRem(ctx, opsRealtimeGroupIndexKey(groupID), accountID)
	}
	_, err = pipe.Exec(ctx)
	if err != nil {
		return err
	}
	c.invalidateWarmPoolOverviewSnapshot(ctx)
	return nil
}

func (c *opsRealtimeCache) DeleteAccount(ctx context.Context, accountID int64) error {
	if accountID <= 0 {
		return nil
	}
	previous, err := c.getAccount(ctx, accountID)
	if err != nil {
		return err
	}
	accountIDStr := strconv.FormatInt(accountID, 10)
	pipe := c.rdb.Pipeline()
	pipe.Del(ctx, opsRealtimeAccountKey(accountIDStr))
	pipe.SRem(ctx, opsRealtimeAccountsAllKey, accountIDStr)
	if previous != nil {
		if strings.TrimSpace(previous.Platform) != "" {
			pipe.SRem(ctx, opsRealtimePlatformIndexKey(previous.Platform), accountIDStr)
		}
		for _, groupID := range normalizeOpsRealtimeGroupIDs(previous.GroupIDs, previous.Groups) {
			pipe.SRem(ctx, opsRealtimeGroupIndexKey(groupID), accountIDStr)
		}
	}
	_, err = pipe.Exec(ctx)
	if err != nil {
		return err
	}
	c.invalidateWarmPoolOverviewSnapshot(ctx)
	return nil
}

func (c *opsRealtimeCache) ListAccountIDs(ctx context.Context, platformFilter string, groupIDFilter *int64) ([]int64, error) {
	key := opsRealtimeAccountsAllKey
	if groupIDFilter != nil && *groupIDFilter > 0 {
		key = opsRealtimeGroupIndexKey(*groupIDFilter)
	} else if strings.TrimSpace(platformFilter) != "" {
		key = opsRealtimePlatformIndexKey(platformFilter)
	}
	values, err := c.rdb.SMembers(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return []int64{}, nil
		}
		return nil, err
	}
	ids := make([]int64, 0, len(values))
	for _, value := range values {
		id, err := strconv.ParseInt(value, 10, 64)
		if err != nil || id <= 0 {
			continue
		}
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids, nil
}

func (c *opsRealtimeCache) GetAccounts(ctx context.Context, accountIDs []int64) (map[int64]*service.OpsRealtimeAccountCacheEntry, error) {
	if len(accountIDs) == 0 {
		return map[int64]*service.OpsRealtimeAccountCacheEntry{}, nil
	}
	keys := make([]string, 0, len(accountIDs))
	for _, accountID := range accountIDs {
		if accountID <= 0 {
			continue
		}
		keys = append(keys, opsRealtimeAccountKey(strconv.FormatInt(accountID, 10)))
	}
	values, err := c.rdb.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, err
	}
	result := make(map[int64]*service.OpsRealtimeAccountCacheEntry, len(values))
	for _, value := range values {
		if value == nil {
			continue
		}
		entry, err := decodeOpsRealtimeAccountCacheEntry(value)
		if err != nil || entry == nil || entry.AccountID <= 0 {
			continue
		}
		result[entry.AccountID] = entry
	}
	return result, nil
}

func (c *opsRealtimeCache) SetWarmAccountState(ctx context.Context, state *service.OpsRealtimeWarmAccountState) error {
	if state == nil || state.AccountID <= 0 {
		return nil
	}
	updatedAt := state.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	_, err := opsRealtimeWarmAccountUpsertScript.Run(ctx, c.rdb, []string{opsRealtimeWarmAccountKey(state.AccountID)},
		updatedAt.UnixMilli(),
		strings.TrimSpace(state.State),
		formatOpsRealtimeUnixMilli(state.VerifiedAt),
		formatOpsRealtimeUnixMilli(state.ExpiresAt),
		formatOpsRealtimeUnixMilli(state.FailUntil),
		formatOpsRealtimeUnixMilli(state.NetworkErrorAt),
		formatOpsRealtimeUnixMilli(state.NetworkErrorUntil),
		updatedAt.UnixMilli(),
	).Result()
	if err != nil {
		return err
	}
	if err := c.rdb.SAdd(ctx, opsRealtimeWarmAccountIndexKey, strconv.FormatInt(state.AccountID, 10)).Err(); err != nil {
		return err
	}
	return nil
}

func (c *opsRealtimeCache) DeleteWarmAccountState(ctx context.Context, accountID int64) error {
	if accountID <= 0 {
		return nil
	}
	accountIDStr := strconv.FormatInt(accountID, 10)
	pipe := c.rdb.Pipeline()
	pipe.Del(ctx, opsRealtimeWarmAccountKey(accountID))
	pipe.SRem(ctx, opsRealtimeWarmAccountIndexKey, accountIDStr)
	_, err := pipe.Exec(ctx)
	if err != nil {
		return err
	}
	return nil
}

func (c *opsRealtimeCache) ClearWarmPoolState(ctx context.Context) error {
	if c == nil || c.rdb == nil {
		return nil
	}
	keys := []string{
		opsRealtimeWarmAccountIndexKey,
		opsRealtimeWarmBucketIndexKey,
		opsRealtimeWarmGlobalKey,
		opsRealtimeWarmOverviewKey,
	}
	patterns := []string{
		opsRealtimeWarmAccountKeyPrefix + "*",
		opsRealtimeWarmBucketMetaPrefix + "*",
	}
	for _, pattern := range patterns {
		iter := c.rdb.Scan(ctx, 0, pattern, 0).Iterator()
		for iter.Next(ctx) {
			keys = append(keys, iter.Val())
		}
		if err := iter.Err(); err != nil {
			return err
		}
	}
	if len(keys) == 0 {
		return nil
	}
	return c.rdb.Del(ctx, keys...).Err()
}

func (c *opsRealtimeCache) GetWarmAccountStates(ctx context.Context, accountIDs []int64) (map[int64]*service.OpsRealtimeWarmAccountState, error) {
	if len(accountIDs) == 0 {
		members, err := c.rdb.SMembers(ctx, opsRealtimeWarmAccountIndexKey).Result()
		if err != nil && err != redis.Nil {
			return nil, err
		}
		accountIDs = make([]int64, 0, len(members))
		for _, member := range members {
			accountID, err := strconv.ParseInt(member, 10, 64)
			if err != nil || accountID <= 0 {
				continue
			}
			accountIDs = append(accountIDs, accountID)
		}
	}
	if len(accountIDs) == 0 {
		return map[int64]*service.OpsRealtimeWarmAccountState{}, nil
	}
	pipe := c.rdb.Pipeline()
	cmds := make(map[int64]*redis.MapStringStringCmd, len(accountIDs))
	for _, accountID := range accountIDs {
		if accountID <= 0 {
			continue
		}
		cmds[accountID] = pipe.HGetAll(ctx, opsRealtimeWarmAccountKey(accountID))
	}
	if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
		return nil, err
	}
	result := make(map[int64]*service.OpsRealtimeWarmAccountState, len(cmds))
	for accountID, cmd := range cmds {
		state := decodeOpsRealtimeWarmAccountState(accountID, cmd.Val())
		if state == nil {
			continue
		}
		result[accountID] = state
	}
	return result, nil
}

func (c *opsRealtimeCache) TouchWarmBucketAccess(ctx context.Context, groupID int64, at time.Time) error {
	if groupID < 0 {
		return nil
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	pipe := c.rdb.Pipeline()
	pipe.HSet(ctx, opsRealtimeWarmBucketMetaKey(groupID), "group_id", groupID)
	pipe.SAdd(ctx, opsRealtimeWarmBucketIndexKey, strconv.FormatInt(groupID, 10))
	if _, err := pipe.Exec(ctx); err != nil {
		return err
	}
	_, err := opsRealtimeWarmHashMaxScript.Run(ctx, c.rdb, []string{opsRealtimeWarmBucketMetaKey(groupID)}, "last_access_at_unix_ms", at.UTC().UnixMilli()).Result()
	return err
}

func (c *opsRealtimeCache) TouchWarmBucketRefill(ctx context.Context, groupID int64, at time.Time) error {
	if groupID < 0 {
		return nil
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	pipe := c.rdb.Pipeline()
	pipe.HSet(ctx, opsRealtimeWarmBucketMetaKey(groupID), "group_id", groupID)
	pipe.SAdd(ctx, opsRealtimeWarmBucketIndexKey, strconv.FormatInt(groupID, 10))
	if _, err := pipe.Exec(ctx); err != nil {
		return err
	}
	_, err := opsRealtimeWarmHashMaxScript.Run(ctx, c.rdb, []string{opsRealtimeWarmBucketMetaKey(groupID)}, "last_refill_at_unix_ms", at.UTC().UnixMilli()).Result()
	return err
}

func (c *opsRealtimeCache) IncrementWarmBucketTake(ctx context.Context, groupID int64, delta int64) error {
	if groupID < 0 || delta == 0 {
		return nil
	}
	pipe := c.rdb.Pipeline()
	pipe.HSet(ctx, opsRealtimeWarmBucketMetaKey(groupID), "group_id", groupID)
	pipe.HIncrBy(ctx, opsRealtimeWarmBucketMetaKey(groupID), "take_count", delta)
	pipe.SAdd(ctx, opsRealtimeWarmBucketIndexKey, strconv.FormatInt(groupID, 10))
	_, err := pipe.Exec(ctx)
	return err
}

func (c *opsRealtimeCache) TouchWarmBucketMember(ctx context.Context, groupID int64, memberToken string, touchedAt time.Time) error {
	if groupID < 0 || strings.TrimSpace(memberToken) == "" {
		return nil
	}
	if touchedAt.IsZero() {
		touchedAt = time.Now().UTC()
	}
	pipe := c.rdb.Pipeline()
	pipe.ZAdd(ctx, opsRealtimeWarmBucketMembersKey(groupID), redis.Z{Score: float64(touchedAt.UTC().UnixMilli()), Member: strings.TrimSpace(memberToken)})
	pipe.SAdd(ctx, opsRealtimeWarmBucketIndexKey, strconv.FormatInt(groupID, 10))
	_, err := pipe.Exec(ctx)
	if err != nil {
		return err
	}
	return nil
}

func (c *opsRealtimeCache) RemoveWarmBucketMember(ctx context.Context, groupID int64, memberToken string) error {
	if groupID < 0 || strings.TrimSpace(memberToken) == "" {
		return nil
	}
	if err := c.rdb.ZRem(ctx, opsRealtimeWarmBucketMembersKey(groupID), strings.TrimSpace(memberToken)).Err(); err != nil {
		return err
	}
	return nil
}

func (c *opsRealtimeCache) RemoveWarmBucketAccount(ctx context.Context, groupID, accountID int64) error {
	if groupID < 0 || accountID <= 0 {
		return nil
	}
	_, err := opsRealtimeWarmRemoveBucketAccountScript.Run(ctx, c.rdb, []string{opsRealtimeWarmBucketMembersKey(groupID)}, accountID).Result()
	if err != nil {
		return err
	}
	return nil
}

func (c *opsRealtimeCache) ListWarmBucketGroupIDs(ctx context.Context) ([]int64, error) {
	values, err := c.rdb.SMembers(ctx, opsRealtimeWarmBucketIndexKey).Result()
	if err != nil {
		if err == redis.Nil {
			return []int64{}, nil
		}
		return nil, err
	}
	groupIDs := make([]int64, 0, len(values))
	for _, value := range values {
		groupID, err := strconv.ParseInt(value, 10, 64)
		if err != nil || groupID < 0 {
			continue
		}
		groupIDs = append(groupIDs, groupID)
	}
	sort.Slice(groupIDs, func(i, j int) bool { return groupIDs[i] < groupIDs[j] })
	return groupIDs, nil
}

func (c *opsRealtimeCache) GetWarmBucketMetas(ctx context.Context, groupIDs []int64) (map[int64]*service.OpsRealtimeWarmBucketMeta, error) {
	if len(groupIDs) == 0 {
		listed, err := c.ListWarmBucketGroupIDs(ctx)
		if err != nil {
			return nil, err
		}
		groupIDs = listed
	}
	if len(groupIDs) == 0 {
		return map[int64]*service.OpsRealtimeWarmBucketMeta{}, nil
	}
	pipe := c.rdb.Pipeline()
	cmds := make(map[int64]*redis.MapStringStringCmd, len(groupIDs))
	for _, groupID := range groupIDs {
		if groupID < 0 {
			continue
		}
		cmds[groupID] = pipe.HGetAll(ctx, opsRealtimeWarmBucketMetaKey(groupID))
	}
	if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
		return nil, err
	}
	result := make(map[int64]*service.OpsRealtimeWarmBucketMeta, len(cmds))
	for groupID, cmd := range cmds {
		meta := decodeOpsRealtimeWarmBucketMeta(groupID, cmd.Val())
		if meta == nil {
			continue
		}
		result[groupID] = meta
	}
	return result, nil
}

func (c *opsRealtimeCache) GetWarmBucketMemberTokens(ctx context.Context, groupID int64, minTouchedAt time.Time) ([]string, error) {
	if groupID < 0 {
		return []string{}, nil
	}
	minScore := "-inf"
	if !minTouchedAt.IsZero() {
		minScore = strconv.FormatInt(minTouchedAt.UTC().UnixMilli(), 10)
	}
	return c.rdb.ZRangeByScore(ctx, opsRealtimeWarmBucketMembersKey(groupID), &redis.ZRangeBy{Min: minScore, Max: "+inf"}).Result()
}

func (c *opsRealtimeCache) GetWarmBucketMemberTokensByGroups(ctx context.Context, groupIDs []int64, minTouchedAt time.Time) (map[int64][]string, error) {
	result := make(map[int64][]string, len(groupIDs))
	if len(groupIDs) == 0 {
		return result, nil
	}
	minScore := "-inf"
	if !minTouchedAt.IsZero() {
		minScore = strconv.FormatInt(minTouchedAt.UTC().UnixMilli(), 10)
	}
	pipe := c.rdb.Pipeline()
	cmds := make(map[int64]*redis.StringSliceCmd, len(groupIDs))
	for _, groupID := range groupIDs {
		if groupID < 0 {
			continue
		}
		cmds[groupID] = pipe.ZRangeByScore(ctx, opsRealtimeWarmBucketMembersKey(groupID), &redis.ZRangeBy{Min: minScore, Max: "+inf"})
	}
	if len(cmds) == 0 {
		return result, nil
	}
	if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
		return nil, err
	}
	for groupID, cmd := range cmds {
		result[groupID] = append([]string(nil), cmd.Val()...)
	}
	return result, nil
}

func (c *opsRealtimeCache) IncrementWarmGlobalTake(ctx context.Context, delta int64) error {
	if delta == 0 {
		return nil
	}
	return c.rdb.HIncrBy(ctx, opsRealtimeWarmGlobalKey, "take_count", delta).Err()
}

func (c *opsRealtimeCache) TouchWarmLastBucketMaintenance(ctx context.Context, at time.Time) error {
	if at.IsZero() {
		at = time.Now().UTC()
	}
	_, err := opsRealtimeWarmHashMaxScript.Run(ctx, c.rdb, []string{opsRealtimeWarmGlobalKey}, "last_bucket_maintenance_at_unix_ms", at.UTC().UnixMilli()).Result()
	return err
}

func (c *opsRealtimeCache) TouchWarmLastGlobalMaintenance(ctx context.Context, at time.Time) error {
	if at.IsZero() {
		at = time.Now().UTC()
	}
	_, err := opsRealtimeWarmHashMaxScript.Run(ctx, c.rdb, []string{opsRealtimeWarmGlobalKey}, "last_global_maintenance_at_unix_ms", at.UTC().UnixMilli()).Result()
	return err
}

func (c *opsRealtimeCache) GetWarmGlobalState(ctx context.Context) (*service.OpsRealtimeWarmGlobalState, bool, error) {
	values, err := c.rdb.HGetAll(ctx, opsRealtimeWarmGlobalKey).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, false, nil
		}
		return nil, false, err
	}
	if len(values) == 0 {
		return nil, false, nil
	}
	state := decodeOpsRealtimeWarmGlobalState(values)
	if state == nil {
		return nil, false, nil
	}
	return state, true, nil
}

func (c *opsRealtimeCache) GetWarmPoolOverviewSnapshot(ctx context.Context) (*service.OpsOpenAIWarmPoolStats, bool, error) {
	value, err := c.rdb.Get(ctx, opsRealtimeWarmOverviewKey).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, false, nil
		}
		return nil, false, err
	}
	stats := &service.OpsOpenAIWarmPoolStats{}
	if err := json.Unmarshal([]byte(value), stats); err != nil {
		return nil, false, err
	}
	return stats, true, nil
}

func (c *opsRealtimeCache) SetWarmPoolOverviewSnapshot(ctx context.Context, stats *service.OpsOpenAIWarmPoolStats, ttl time.Duration) error {
	if stats == nil {
		return nil
	}
	payload, err := json.Marshal(stats)
	if err != nil {
		return err
	}
	return c.rdb.Set(ctx, opsRealtimeWarmOverviewKey, payload, ttl).Err()
}

func (c *opsRealtimeCache) DeleteWarmPoolOverviewSnapshot(ctx context.Context) error {
	return c.rdb.Del(ctx, opsRealtimeWarmOverviewKey).Err()
}

func (c *opsRealtimeCache) TryAcquireReconcileLeaderLock(ctx context.Context, owner string, ttl time.Duration) (func(), bool, error) {
	if strings.TrimSpace(owner) == "" {
		return nil, false, nil
	}
	if ttl <= 0 {
		ttl = time.Minute
	}
	ok, err := c.rdb.SetNX(ctx, opsRealtimeReconcileLeaderKey, strings.TrimSpace(owner), ttl).Result()
	if err != nil {
		return nil, false, err
	}
	if !ok {
		return nil, false, nil
	}
	release := func() {
		releaseCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_, _ = opsRealtimeLeaderReleaseScript.Run(releaseCtx, c.rdb, []string{opsRealtimeReconcileLeaderKey}, strings.TrimSpace(owner)).Result()
	}
	return release, true, nil
}

func (c *opsRealtimeCache) getAccount(ctx context.Context, accountID int64) (*service.OpsRealtimeAccountCacheEntry, error) {
	if accountID <= 0 {
		return nil, nil
	}
	value, err := c.rdb.Get(ctx, opsRealtimeAccountKey(strconv.FormatInt(accountID, 10))).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, err
	}
	return decodeOpsRealtimeAccountCacheEntry(value)
}

func opsRealtimeAccountKey(accountID string) string {
	return opsRealtimeAccountKeyPrefix + accountID
}

func opsRealtimePlatformIndexKey(platform string) string {
	return opsRealtimePlatformIndexPrefix + strings.TrimSpace(platform)
}

func opsRealtimeGroupIndexKey(groupID int64) string {
	return opsRealtimeGroupIndexPrefix + strconv.FormatInt(groupID, 10)
}

func opsRealtimeWarmAccountKey(accountID int64) string {
	return opsRealtimeWarmAccountKeyPrefix + strconv.FormatInt(accountID, 10)
}

func opsRealtimeWarmBucketMetaKey(groupID int64) string {
	return fmt.Sprintf("%s%d%s", opsRealtimeWarmBucketMetaPrefix, groupID, opsRealtimeWarmBucketMetaPart)
}

func opsRealtimeWarmBucketMembersKey(groupID int64) string {
	return fmt.Sprintf("%s%d%s", opsRealtimeWarmBucketMetaPrefix, groupID, opsRealtimeWarmBucketMembersPart)
}

func normalizeOpsRealtimeGroupIDs(groupIDs []int64, groups []*service.OpsRealtimeGroupRef) []int64 {
	seen := make(map[int64]struct{}, len(groupIDs)+len(groups))
	out := make([]int64, 0, len(groupIDs)+len(groups))
	appendGroupID := func(groupID int64) {
		if groupID <= 0 {
			return
		}
		if _, exists := seen[groupID]; exists {
			return
		}
		seen[groupID] = struct{}{}
		out = append(out, groupID)
	}
	for _, groupID := range groupIDs {
		appendGroupID(groupID)
	}
	for _, group := range groups {
		if group == nil {
			continue
		}
		appendGroupID(group.GroupID)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func decodeOpsRealtimeAccountCacheEntry(value any) (*service.OpsRealtimeAccountCacheEntry, error) {
	payload, err := normalizeRedisPayload(value)
	if err != nil {
		return nil, err
	}
	var entry service.OpsRealtimeAccountCacheEntry
	if err := json.Unmarshal(payload, &entry); err != nil {
		return nil, err
	}
	return &entry, nil
}

func decodeOpsRealtimeWarmAccountState(accountID int64, values map[string]string) *service.OpsRealtimeWarmAccountState {
	if len(values) == 0 {
		return nil
	}
	updatedAt := parseOpsRealtimeUnixMilli(values["updated_at_unix_ms"])
	if updatedAt == nil {
		return nil
	}
	return &service.OpsRealtimeWarmAccountState{
		AccountID:         accountID,
		State:             strings.TrimSpace(values["state"]),
		VerifiedAt:        parseOpsRealtimeUnixMilli(values["verified_at_unix_ms"]),
		ExpiresAt:         parseOpsRealtimeUnixMilli(values["expires_at_unix_ms"]),
		FailUntil:         parseOpsRealtimeUnixMilli(values["fail_until_unix_ms"]),
		NetworkErrorAt:    parseOpsRealtimeUnixMilli(values["network_error_at_unix_ms"]),
		NetworkErrorUntil: parseOpsRealtimeUnixMilli(values["network_error_until_unix_ms"]),
		UpdatedAt:         updatedAt.UTC(),
	}
}

func decodeOpsRealtimeWarmBucketMeta(groupID int64, values map[string]string) *service.OpsRealtimeWarmBucketMeta {
	if len(values) == 0 {
		return nil
	}
	meta := &service.OpsRealtimeWarmBucketMeta{GroupID: groupID}
	if takeCount, err := strconv.ParseInt(values["take_count"], 10, 64); err == nil {
		meta.TakeCount = takeCount
	}
	meta.LastAccessAt = parseOpsRealtimeUnixMilli(values["last_access_at_unix_ms"])
	meta.LastRefillAt = parseOpsRealtimeUnixMilli(values["last_refill_at_unix_ms"])
	return meta
}

func decodeOpsRealtimeWarmGlobalState(values map[string]string) *service.OpsRealtimeWarmGlobalState {
	if len(values) == 0 {
		return nil
	}
	state := &service.OpsRealtimeWarmGlobalState{}
	if takeCount, err := strconv.ParseInt(values["take_count"], 10, 64); err == nil {
		state.TakeCount = takeCount
	}
	state.LastBucketMaintenanceAt = parseOpsRealtimeUnixMilli(values["last_bucket_maintenance_at_unix_ms"])
	state.LastGlobalMaintenanceAt = parseOpsRealtimeUnixMilli(values["last_global_maintenance_at_unix_ms"])
	return state
}

func normalizeRedisPayload(value any) ([]byte, error) {
	switch raw := value.(type) {
	case string:
		return []byte(raw), nil
	case []byte:
		return raw, nil
	default:
		return nil, fmt.Errorf("unexpected redis payload type: %T", value)
	}
}

func formatOpsRealtimeUnixMilli(value *time.Time) string {
	if value == nil {
		return "0"
	}
	return strconv.FormatInt(value.UTC().UnixMilli(), 10)
}

func parseOpsRealtimeUnixMilli(value string) *time.Time {
	value = strings.TrimSpace(value)
	if value == "" || value == "0" {
		return nil
	}
	unixMilli, err := strconv.ParseInt(value, 10, 64)
	if err != nil || unixMilli <= 0 {
		return nil
	}
	t := time.UnixMilli(unixMilli).UTC()
	return &t
}
