package service

import (
	"context"
	"sort"
	"time"
)

type OpsRealtimeGroupRef struct {
	GroupID   int64  `json:"group_id"`
	GroupName string `json:"group_name"`
	Platform  string `json:"platform"`
}

type OpsRealtimeAccountCacheEntry struct {
	AccountID              int64                  `json:"account_id"`
	AccountName            string                 `json:"account_name"`
	Platform               string                 `json:"platform"`
	Concurrency            int                    `json:"concurrency"`
	Priority               int                    `json:"priority"`
	Status                 string                 `json:"status"`
	Schedulable            bool                   `json:"schedulable"`
	ErrorMessage           string                 `json:"error_message"`
	AutoPauseOnExpired     bool                   `json:"auto_pause_on_expired"`
	ExpiresAt              *time.Time             `json:"expires_at,omitempty"`
	RateLimitResetAt       *time.Time             `json:"rate_limit_reset_at,omitempty"`
	OverloadUntil          *time.Time             `json:"overload_until,omitempty"`
	TempUnschedulableUntil *time.Time             `json:"temp_unschedulable_until,omitempty"`
	GroupIDs               []int64                `json:"group_ids,omitempty"`
	Groups                 []*OpsRealtimeGroupRef `json:"groups,omitempty"`
}

func BuildOpsRealtimeAccountCacheEntry(account *Account) *OpsRealtimeAccountCacheEntry {
	if account == nil || account.ID <= 0 {
		return nil
	}
	entry := &OpsRealtimeAccountCacheEntry{
		AccountID:              account.ID,
		AccountName:            account.Name,
		Platform:               account.Platform,
		Concurrency:            account.Concurrency,
		Priority:               account.Priority,
		Status:                 account.Status,
		Schedulable:            account.Schedulable,
		ErrorMessage:           account.ErrorMessage,
		AutoPauseOnExpired:     account.AutoPauseOnExpired,
		ExpiresAt:              cloneTimePtr(account.ExpiresAt),
		RateLimitResetAt:       cloneTimePtr(account.RateLimitResetAt),
		OverloadUntil:          cloneTimePtr(account.OverloadUntil),
		TempUnschedulableUntil: cloneTimePtr(account.TempUnschedulableUntil),
		GroupIDs:               append([]int64(nil), account.GroupIDs...),
	}
	if len(account.Groups) > 0 {
		entry.Groups = make([]*OpsRealtimeGroupRef, 0, len(account.Groups))
		for _, group := range account.Groups {
			if group == nil || group.ID <= 0 {
				continue
			}
			entry.Groups = append(entry.Groups, &OpsRealtimeGroupRef{
				GroupID:   group.ID,
				GroupName: group.Name,
				Platform:  group.Platform,
			})
		}
	}
	if len(entry.GroupIDs) == 0 && len(entry.Groups) > 0 {
		entry.GroupIDs = make([]int64, 0, len(entry.Groups))
		for _, group := range entry.Groups {
			if group == nil || group.GroupID <= 0 {
				continue
			}
			entry.GroupIDs = append(entry.GroupIDs, group.GroupID)
		}
	}
	if len(entry.GroupIDs) > 1 {
		sort.Slice(entry.GroupIDs, func(i, j int) bool { return entry.GroupIDs[i] < entry.GroupIDs[j] })
	}
	return entry
}

func (e *OpsRealtimeAccountCacheEntry) ToAccount() *Account {
	if e == nil || e.AccountID <= 0 {
		return nil
	}
	account := &Account{
		ID:                     e.AccountID,
		Name:                   e.AccountName,
		Platform:               e.Platform,
		Concurrency:            e.Concurrency,
		Priority:               e.Priority,
		Status:                 e.Status,
		Schedulable:            e.Schedulable,
		ErrorMessage:           e.ErrorMessage,
		AutoPauseOnExpired:     e.AutoPauseOnExpired,
		ExpiresAt:              cloneTimePtr(e.ExpiresAt),
		RateLimitResetAt:       cloneTimePtr(e.RateLimitResetAt),
		OverloadUntil:          cloneTimePtr(e.OverloadUntil),
		TempUnschedulableUntil: cloneTimePtr(e.TempUnschedulableUntil),
		GroupIDs:               append([]int64(nil), e.GroupIDs...),
	}
	if len(e.Groups) > 0 {
		account.Groups = make([]*Group, 0, len(e.Groups))
		for _, group := range e.Groups {
			if group == nil || group.GroupID <= 0 {
				continue
			}
			account.Groups = append(account.Groups, &Group{
				ID:       group.GroupID,
				Name:     group.GroupName,
				Platform: group.Platform,
			})
		}
	}
	return account
}

type OpsRealtimeWarmAccountState struct {
	AccountID         int64      `json:"account_id"`
	State             string     `json:"state"`
	VerifiedAt        *time.Time `json:"verified_at,omitempty"`
	ExpiresAt         *time.Time `json:"expires_at,omitempty"`
	FailUntil         *time.Time `json:"fail_until,omitempty"`
	NetworkErrorAt    *time.Time `json:"network_error_at,omitempty"`
	NetworkErrorUntil *time.Time `json:"network_error_until,omitempty"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

type OpsRealtimeWarmBucketMeta struct {
	GroupID      int64      `json:"group_id"`
	LastAccessAt *time.Time `json:"last_access_at,omitempty"`
	LastRefillAt *time.Time `json:"last_refill_at,omitempty"`
	TakeCount    int64      `json:"take_count"`
}

type OpsRealtimeWarmGlobalState struct {
	TakeCount               int64      `json:"take_count"`
	LastBucketMaintenanceAt *time.Time `json:"last_bucket_maintenance_at,omitempty"`
	LastGlobalMaintenanceAt *time.Time `json:"last_global_maintenance_at,omitempty"`
}

type OpsRealtimeCache interface {
	IsAccountIndexReady(ctx context.Context) (bool, error)
	ReplaceAccounts(ctx context.Context, accounts []*OpsRealtimeAccountCacheEntry) error
	UpsertAccount(ctx context.Context, account *OpsRealtimeAccountCacheEntry) error
	DeleteAccount(ctx context.Context, accountID int64) error
	ListAccountIDs(ctx context.Context, platformFilter string, groupIDFilter *int64) ([]int64, error)
	GetAccounts(ctx context.Context, accountIDs []int64) (map[int64]*OpsRealtimeAccountCacheEntry, error)

	SetWarmAccountState(ctx context.Context, state *OpsRealtimeWarmAccountState) error
	DeleteWarmAccountState(ctx context.Context, accountID int64) error
	GetWarmAccountStates(ctx context.Context, accountIDs []int64) (map[int64]*OpsRealtimeWarmAccountState, error)
	ClearWarmPoolState(ctx context.Context) error

	TouchWarmBucketAccess(ctx context.Context, groupID int64, at time.Time) error
	TouchWarmBucketRefill(ctx context.Context, groupID int64, at time.Time) error
	IncrementWarmBucketTake(ctx context.Context, groupID int64, delta int64) error
	TouchWarmBucketMember(ctx context.Context, groupID int64, memberToken string, touchedAt time.Time) error
	RemoveWarmBucketMember(ctx context.Context, groupID int64, memberToken string) error
	RemoveWarmBucketAccount(ctx context.Context, groupID, accountID int64) error
	ListWarmBucketGroupIDs(ctx context.Context) ([]int64, error)
	GetWarmBucketMetas(ctx context.Context, groupIDs []int64) (map[int64]*OpsRealtimeWarmBucketMeta, error)
	GetWarmBucketMemberTokens(ctx context.Context, groupID int64, minTouchedAt time.Time) ([]string, error)

	IncrementWarmGlobalTake(ctx context.Context, delta int64) error
	TouchWarmLastBucketMaintenance(ctx context.Context, at time.Time) error
	TouchWarmLastGlobalMaintenance(ctx context.Context, at time.Time) error
	GetWarmGlobalState(ctx context.Context) (*OpsRealtimeWarmGlobalState, bool, error)
	GetWarmPoolOverviewSnapshot(ctx context.Context) (*OpsOpenAIWarmPoolStats, bool, error)
	SetWarmPoolOverviewSnapshot(ctx context.Context, stats *OpsOpenAIWarmPoolStats, ttl time.Duration) error
	DeleteWarmPoolOverviewSnapshot(ctx context.Context) error
	TryAcquireReconcileLeaderLock(ctx context.Context, owner string, ttl time.Duration) (func(), bool, error)
}

func cloneTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copied := value.UTC()
	return &copied
}
