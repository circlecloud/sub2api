package service

import "time"

type OpsOpenAIWarmPoolStats struct {
	Enabled          bool                               `json:"enabled"`
	WarmPoolEnabled  bool                               `json:"warm_pool_enabled"`
	ReaderReady      bool                               `json:"reader_ready"`
	Timestamp        *time.Time                         `json:"timestamp,omitempty"`
	Summary          *OpsOpenAIWarmPoolSummary          `json:"summary,omitempty"`
	Buckets          []*OpsOpenAIWarmPoolBucket         `json:"buckets"`
	Accounts         []*OpsOpenAIWarmPoolAccount        `json:"accounts"`
	GlobalCoverages  []*OpsOpenAIWarmPoolGroupCoverage  `json:"global_coverages"`
	NetworkErrorPool *OpsOpenAIWarmPoolNetworkErrorPool `json:"network_error_pool,omitempty"`
}

type OpsOpenAIWarmPoolSummary struct {
	TrackedAccountCount        int        `json:"tracked_account_count"`
	BucketReadyAccountCount    int        `json:"bucket_ready_account_count"`
	GlobalReadyAccountCount    int        `json:"global_ready_account_count"`
	ActiveGroupCount           int        `json:"active_group_count"`
	GlobalTargetPerActiveGroup int        `json:"global_target_per_active_group"`
	GlobalRefillPerActiveGroup int        `json:"global_refill_per_active_group"`
	GroupsBelowTargetCount     int        `json:"groups_below_target_count"`
	GroupsBelowRefillCount     int        `json:"groups_below_refill_count"`
	ProbingAccountCount        int        `json:"probing_account_count"`
	CoolingAccountCount        int        `json:"cooling_account_count"`
	NetworkErrorPoolCount      int        `json:"network_error_pool_count"`
	TakeCount                  int64      `json:"take_count"`
	NetworkErrorPoolFull       bool       `json:"network_error_pool_full"`
	LastBucketMaintenanceAt    *time.Time `json:"last_bucket_maintenance_at,omitempty"`
	LastGlobalMaintenanceAt    *time.Time `json:"last_global_maintenance_at,omitempty"`
}

type OpsOpenAIWarmPoolBucket struct {
	GroupID             int64      `json:"group_id"`
	GroupName           string     `json:"group_name"`
	SchedulableAccounts int        `json:"schedulable_accounts"`
	BucketReadyAccounts int        `json:"bucket_ready_accounts"`
	BucketTargetSize    int        `json:"bucket_target_size"`
	BucketRefillBelow   int        `json:"bucket_refill_below"`
	TakeCount           int64      `json:"take_count"`
	ProbingAccounts     int        `json:"probing_accounts"`
	CoolingAccounts     int        `json:"cooling_accounts"`
	LastAccessAt        *time.Time `json:"last_access_at,omitempty"`
	LastRefillAt        *time.Time `json:"last_refill_at,omitempty"`
}

type OpsOpenAIWarmPoolBucketRef struct {
	GroupID   int64  `json:"group_id"`
	GroupName string `json:"group_name"`
}

type OpsOpenAIWarmPoolGroupCoverage struct {
	GroupID       int64  `json:"group_id"`
	GroupName     string `json:"group_name"`
	CoverageCount int    `json:"coverage_count"`
	TargetSize    int    `json:"target_size"`
	RefillBelow   int    `json:"refill_below"`
}

type OpsOpenAIWarmPoolNetworkErrorPool struct {
	Count           int        `json:"count"`
	Capacity        int        `json:"capacity"`
	OldestEnteredAt *time.Time `json:"oldest_entered_at,omitempty"`
}

type OpsOpenAIWarmPoolAccount struct {
	AccountID         int64                         `json:"account_id"`
	AccountName       string                        `json:"account_name"`
	Platform          string                        `json:"platform"`
	Schedulable       bool                          `json:"schedulable"`
	Priority          int                           `json:"priority"`
	Concurrency       int                           `json:"concurrency"`
	State             string                        `json:"state"`
	Groups            []*OpsOpenAIWarmPoolBucketRef `json:"groups,omitempty"`
	VerifiedAt        *time.Time                    `json:"verified_at,omitempty"`
	ExpiresAt         *time.Time                    `json:"expires_at,omitempty"`
	FailUntil         *time.Time                    `json:"fail_until,omitempty"`
	NetworkErrorAt    *time.Time                    `json:"network_error_at,omitempty"`
	NetworkErrorUntil *time.Time                    `json:"network_error_until,omitempty"`
}
