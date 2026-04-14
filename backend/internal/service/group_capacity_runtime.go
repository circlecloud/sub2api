package service

import "context"

// GroupCapacityRuntimeUsage holds dynamic per-group usage metrics.
// It is intentionally small in Phase 0 so later modules can replace the legacy
// runtime aggregation without changing the service orchestration contract.
type GroupCapacityRuntimeUsage struct {
	GroupID         int64
	ConcurrencyUsed int
	ActiveSessions  int
	CurrentRPM      int
}

// GroupCapacityRuntimeProvider loads runtime usage for a group.
type GroupCapacityRuntimeProvider interface {
	GetGroupCapacityRuntimeUsage(ctx context.Context, snapshot GroupCapacityStaticSnapshot) (GroupCapacityRuntimeUsage, error)
}
