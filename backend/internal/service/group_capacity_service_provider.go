package service

import (
	"context"
	"errors"
)

func (s *GroupCapacityService) getGroupCapacityFromProviders(ctx context.Context, groupID int64) (GroupCapacitySummary, bool, error) {
	if s.snapshotProvider == nil || s.runtimeProvider == nil {
		return GroupCapacitySummary{}, false, nil
	}

	snapshot, err := s.snapshotProvider.GetGroupCapacityStaticSnapshot(ctx, groupID)
	if err != nil {
		if errors.Is(err, ErrGroupCapacityProviderUnavailable) {
			return GroupCapacitySummary{}, false, nil
		}
		return GroupCapacitySummary{}, true, err
	}

	runtimeUsage, err := s.runtimeProvider.GetGroupCapacityRuntimeUsage(ctx, snapshot)
	if err != nil {
		if errors.Is(err, ErrGroupCapacityProviderUnavailable) {
			return GroupCapacitySummary{}, false, nil
		}
		return GroupCapacitySummary{}, true, err
	}

	return GroupCapacitySummary{
		ConcurrencyUsed: runtimeUsage.ConcurrencyUsed,
		ConcurrencyMax:  snapshot.ConcurrencyMax,
		SessionsUsed:    runtimeUsage.ActiveSessions,
		SessionsMax:     snapshot.SessionsMax,
		RPMUsed:         runtimeUsage.CurrentRPM,
		RPMMax:          snapshot.RPMMax,
	}, true, nil
}
