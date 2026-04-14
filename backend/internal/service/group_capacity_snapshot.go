package service

import (
	"context"
	"errors"
	"time"
)

var ErrGroupCapacityProviderUnavailable = errors.New("group capacity provider unavailable")

// GroupCapacityStaticSnapshot holds static per-group capacity metadata.
//
// AccountIDs is kept for backward compatibility; new readers should prefer the
// narrower *_AccountIDs subsets when available.
type GroupCapacityStaticSnapshot struct {
	GroupID int64
	// AccountIDs is the legacy all-accounts field kept for compatibility.
	AccountIDs []int64
	// AllAccountIDs includes all accounts that belong to the group.
	AllAccountIDs []int64
	// SessionLimitedAccountIDs includes only accounts that contribute to sessions_used.
	SessionLimitedAccountIDs []int64
	// RPMLimitedAccountIDs includes only accounts that contribute to rpm_used.
	RPMLimitedAccountIDs []int64
	// SessionTimeouts stores per-account idle timeouts used for session runtime reads.
	SessionTimeouts map[int64]time.Duration
	ConcurrencyMax  int
	SessionsMax     int
	RPMMax          int
	RebuiltAt       time.Time
	ExpiresAt       time.Time
}

// GroupCapacitySnapshotAccountRecord is the lightweight account row used by the
// static snapshot rebuild path.
type GroupCapacitySnapshotAccountRecord struct {
	AccountID                 int64
	Concurrency               int
	MaxSessions               int
	SessionIdleTimeoutMinutes int
	BaseRPM                   int
}

// GroupCapacitySnapshotGroupRecord is the lightweight group row used by the
// static snapshot rebuild path.
type GroupCapacitySnapshotGroupRecord struct {
	GroupID int64
	Status  string
}

// GroupCapacitySnapshotProvider loads static capacity data for a group.
type GroupCapacitySnapshotProvider interface {
	GetGroupCapacityStaticSnapshot(ctx context.Context, groupID int64) (GroupCapacityStaticSnapshot, error)
}
