package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

const defaultGroupCapacitySnapshotTTL = time.Minute

type groupCapacitySnapshotAccountLister interface {
	ListGroupCapacitySnapshotAccounts(ctx context.Context, groupID int64) ([]GroupCapacitySnapshotAccountRecord, error)
}

type groupCapacitySnapshotGroupGetter interface {
	GetGroupCapacitySnapshotGroup(ctx context.Context, groupID int64) (*GroupCapacitySnapshotGroupRecord, error)
}

type groupCapacitySnapshotProviderService struct {
	accountRepo groupCapacitySnapshotAccountLister
	groupRepo   groupCapacitySnapshotGroupGetter
	projector   *GroupCapacitySnapshotProjector
	ttl         time.Duration
	now         func() time.Time

	mu        sync.RWMutex
	snapshots map[int64]GroupCapacityStaticSnapshot
	sf        singleflight.Group
}

func NewGroupCapacitySnapshotProviderService(
	accountRepo AccountRepository,
	groupRepo GroupRepository,
	projector *GroupCapacitySnapshotProjector,
	ttl time.Duration,
) *groupCapacitySnapshotProviderService {
	accountLister, _ := accountRepo.(groupCapacitySnapshotAccountLister)
	groupGetter, _ := groupRepo.(groupCapacitySnapshotGroupGetter)
	return newGroupCapacitySnapshotProviderService(accountLister, groupGetter, projector, ttl)
}

func newGroupCapacitySnapshotProviderService(
	accountRepo groupCapacitySnapshotAccountLister,
	groupRepo groupCapacitySnapshotGroupGetter,
	projector *GroupCapacitySnapshotProjector,
	ttl time.Duration,
) *groupCapacitySnapshotProviderService {
	if ttl <= 0 {
		ttl = defaultGroupCapacitySnapshotTTL
	}
	svc := &groupCapacitySnapshotProviderService{
		accountRepo: accountRepo,
		groupRepo:   groupRepo,
		projector:   projector,
		ttl:         ttl,
		now:         time.Now,
		snapshots:   make(map[int64]GroupCapacityStaticSnapshot),
	}
	return svc
}

func (s *groupCapacitySnapshotProviderService) GetGroupCapacityStaticSnapshot(ctx context.Context, groupID int64) (GroupCapacityStaticSnapshot, error) {
	if s == nil || s.accountRepo == nil || s.groupRepo == nil {
		return GroupCapacityStaticSnapshot{}, ErrGroupCapacityProviderUnavailable
	}
	if groupID <= 0 {
		return GroupCapacityStaticSnapshot{}, nil
	}
	if snapshot, ok := s.getCachedSnapshot(groupID); ok {
		return snapshot, nil
	}

	value, err, _ := s.sf.Do(fmt.Sprintf("group-capacity-static:%d", groupID), func() (any, error) {
		if snapshot, ok := s.getCachedSnapshot(groupID); ok {
			return snapshot, nil
		}
		dirtyVersion := uint64(0)
		if s.projector != nil {
			dirtyVersion = s.projector.GroupDirtyVersion(groupID)
		}
		snapshot, rebuildErr := s.rebuildSnapshot(ctx, groupID)
		if rebuildErr != nil {
			return nil, rebuildErr
		}
		s.setCachedSnapshot(snapshot)
		if s.projector != nil {
			s.projector.MarkGroupCleanIfVersion(groupID, dirtyVersion)
		}
		return snapshot, nil
	})
	if err != nil {
		return GroupCapacityStaticSnapshot{}, err
	}
	snapshot, ok := value.(GroupCapacityStaticSnapshot)
	if !ok {
		return GroupCapacityStaticSnapshot{}, fmt.Errorf("group capacity snapshot: unexpected singleflight value %T", value)
	}
	return cloneGroupCapacityStaticSnapshot(snapshot), nil
}

func (s *groupCapacitySnapshotProviderService) getCachedSnapshot(groupID int64) (GroupCapacityStaticSnapshot, bool) {
	if s == nil || groupID <= 0 {
		return GroupCapacityStaticSnapshot{}, false
	}
	now := s.nowTime()
	s.mu.RLock()
	snapshot, ok := s.snapshots[groupID]
	s.mu.RUnlock()
	if !ok {
		return GroupCapacityStaticSnapshot{}, false
	}
	if !snapshot.ExpiresAt.After(now) {
		return GroupCapacityStaticSnapshot{}, false
	}
	if s.projector != nil && s.projector.IsGroupDirty(groupID) {
		return GroupCapacityStaticSnapshot{}, false
	}
	return cloneGroupCapacityStaticSnapshot(snapshot), true
}

func (s *groupCapacitySnapshotProviderService) setCachedSnapshot(snapshot GroupCapacityStaticSnapshot) {
	if s == nil || snapshot.GroupID <= 0 {
		return
	}
	s.mu.Lock()
	s.snapshots[snapshot.GroupID] = cloneGroupCapacityStaticSnapshot(snapshot)
	s.mu.Unlock()
}

func (s *groupCapacitySnapshotProviderService) rebuildSnapshot(ctx context.Context, groupID int64) (GroupCapacityStaticSnapshot, error) {
	group, err := s.groupRepo.GetGroupCapacitySnapshotGroup(ctx, groupID)
	if err != nil {
		return GroupCapacityStaticSnapshot{}, err
	}
	rebuiltAt := s.nowTime()
	snapshot := GroupCapacityStaticSnapshot{
		GroupID:   groupID,
		RebuiltAt: rebuiltAt,
		ExpiresAt: rebuiltAt.Add(s.ttl),
	}
	if group == nil || group.Status != StatusActive {
		return snapshot, nil
	}

	records, err := s.accountRepo.ListGroupCapacitySnapshotAccounts(ctx, groupID)
	if err != nil {
		return GroupCapacityStaticSnapshot{}, err
	}

	seen := make(map[int64]struct{}, len(records))
	allIDs := make([]int64, 0, len(records))
	sessionIDs := make([]int64, 0, len(records))
	rpmIDs := make([]int64, 0, len(records))
	timeouts := make(map[int64]time.Duration)

	for _, record := range records {
		if record.AccountID <= 0 {
			continue
		}
		if _, ok := seen[record.AccountID]; ok {
			continue
		}
		seen[record.AccountID] = struct{}{}
		allIDs = append(allIDs, record.AccountID)
		snapshot.ConcurrencyMax += record.Concurrency

		if record.MaxSessions > 0 {
			snapshot.SessionsMax += record.MaxSessions
			sessionIDs = append(sessionIDs, record.AccountID)
			timeout := time.Duration(record.SessionIdleTimeoutMinutes) * time.Minute
			if timeout <= 0 {
				timeout = 5 * time.Minute
			}
			timeouts[record.AccountID] = timeout
		}
		if record.BaseRPM > 0 {
			snapshot.RPMMax += record.BaseRPM
			rpmIDs = append(rpmIDs, record.AccountID)
		}
	}

	snapshot.AccountIDs = append([]int64(nil), allIDs...)
	snapshot.AllAccountIDs = append([]int64(nil), allIDs...)
	snapshot.SessionLimitedAccountIDs = append([]int64(nil), sessionIDs...)
	snapshot.RPMLimitedAccountIDs = append([]int64(nil), rpmIDs...)
	if len(timeouts) > 0 {
		snapshot.SessionTimeouts = timeouts
	}
	return snapshot, nil
}

func (s *groupCapacitySnapshotProviderService) nowTime() time.Time {
	if s == nil || s.now == nil {
		return time.Now()
	}
	return s.now()
}

func cloneGroupCapacityStaticSnapshot(snapshot GroupCapacityStaticSnapshot) GroupCapacityStaticSnapshot {
	cloned := snapshot
	if snapshot.AccountIDs != nil {
		cloned.AccountIDs = append([]int64(nil), snapshot.AccountIDs...)
	}
	if snapshot.AllAccountIDs != nil {
		cloned.AllAccountIDs = append([]int64(nil), snapshot.AllAccountIDs...)
	}
	if snapshot.SessionLimitedAccountIDs != nil {
		cloned.SessionLimitedAccountIDs = append([]int64(nil), snapshot.SessionLimitedAccountIDs...)
	}
	if snapshot.RPMLimitedAccountIDs != nil {
		cloned.RPMLimitedAccountIDs = append([]int64(nil), snapshot.RPMLimitedAccountIDs...)
	}
	if snapshot.SessionTimeouts != nil {
		cloned.SessionTimeouts = make(map[int64]time.Duration, len(snapshot.SessionTimeouts))
		for accountID, timeout := range snapshot.SessionTimeouts {
			cloned.SessionTimeouts[accountID] = timeout
		}
	}
	return cloned
}
