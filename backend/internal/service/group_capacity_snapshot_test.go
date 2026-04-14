//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type groupCapacitySnapshotAccountRepoStub struct {
	listFn func(ctx context.Context, groupID int64) ([]GroupCapacitySnapshotAccountRecord, error)
	calls  []int64
}

func (s *groupCapacitySnapshotAccountRepoStub) ListGroupCapacitySnapshotAccounts(ctx context.Context, groupID int64) ([]GroupCapacitySnapshotAccountRecord, error) {
	s.calls = append(s.calls, groupID)
	if s.listFn == nil {
		return nil, nil
	}
	return s.listFn(ctx, groupID)
}

type groupCapacitySnapshotGroupRepoStub struct {
	getFn func(ctx context.Context, groupID int64) (*GroupCapacitySnapshotGroupRecord, error)
	calls []int64
}

func (s *groupCapacitySnapshotGroupRepoStub) GetGroupCapacitySnapshotGroup(ctx context.Context, groupID int64) (*GroupCapacitySnapshotGroupRecord, error) {
	s.calls = append(s.calls, groupID)
	if s.getFn == nil {
		return nil, nil
	}
	return s.getFn(ctx, groupID)
}

func TestGroupCapacitySnapshotProvider_AggregatesStaticSnapshot(t *testing.T) {
	t.Parallel()

	now := time.Unix(1710000000, 0)
	accountRepo := &groupCapacitySnapshotAccountRepoStub{
		listFn: func(ctx context.Context, groupID int64) ([]GroupCapacitySnapshotAccountRecord, error) {
			require.Equal(t, int64(7), groupID)
			return []GroupCapacitySnapshotAccountRecord{
				{AccountID: 11, Concurrency: 2, MaxSessions: 4, SessionIdleTimeoutMinutes: 0, BaseRPM: 10},
				{AccountID: 12, Concurrency: 3, MaxSessions: 0, SessionIdleTimeoutMinutes: 99, BaseRPM: 15},
				{AccountID: 13, Concurrency: 4, MaxSessions: 2, SessionIdleTimeoutMinutes: 7, BaseRPM: 0},
			}, nil
		},
	}
	groupRepo := &groupCapacitySnapshotGroupRepoStub{
		getFn: func(ctx context.Context, groupID int64) (*GroupCapacitySnapshotGroupRecord, error) {
			return &GroupCapacitySnapshotGroupRecord{GroupID: groupID, Status: StatusActive}, nil
		},
	}

	provider := newGroupCapacitySnapshotProviderService(accountRepo, groupRepo, nil, 10*time.Minute)
	provider.now = func() time.Time { return now }

	snapshot, err := provider.GetGroupCapacityStaticSnapshot(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, GroupCapacityStaticSnapshot{
		GroupID:                  7,
		AccountIDs:               []int64{11, 12, 13},
		AllAccountIDs:            []int64{11, 12, 13},
		SessionLimitedAccountIDs: []int64{11, 13},
		RPMLimitedAccountIDs:     []int64{11, 12},
		SessionTimeouts: map[int64]time.Duration{
			11: 5 * time.Minute,
			13: 7 * time.Minute,
		},
		ConcurrencyMax: 9,
		SessionsMax:    6,
		RPMMax:         25,
		RebuiltAt:      now,
		ExpiresAt:      now.Add(10 * time.Minute),
	}, snapshot)
	require.Equal(t, []int64{7}, accountRepo.calls)
	require.Equal(t, []int64{7}, groupRepo.calls)
}

func TestGroupCapacitySnapshotProvider_RebuildsOnDirtyMark(t *testing.T) {
	t.Parallel()

	now := time.Unix(1710000100, 0)
	listCalls := 0
	accountRepo := &groupCapacitySnapshotAccountRepoStub{
		listFn: func(ctx context.Context, groupID int64) ([]GroupCapacitySnapshotAccountRecord, error) {
			listCalls++
			if listCalls == 1 {
				return []GroupCapacitySnapshotAccountRecord{{AccountID: 21, Concurrency: 2, MaxSessions: 1, SessionIdleTimeoutMinutes: 5, BaseRPM: 10}}, nil
			}
			return []GroupCapacitySnapshotAccountRecord{{AccountID: 21, Concurrency: 5, MaxSessions: 3, SessionIdleTimeoutMinutes: 9, BaseRPM: 20}}, nil
		},
	}
	groupRepo := &groupCapacitySnapshotGroupRepoStub{
		getFn: func(ctx context.Context, groupID int64) (*GroupCapacitySnapshotGroupRecord, error) {
			return &GroupCapacitySnapshotGroupRecord{GroupID: groupID, Status: StatusActive}, nil
		},
	}
	projector := NewGroupCapacitySnapshotProjector()

	provider := newGroupCapacitySnapshotProviderService(accountRepo, groupRepo, projector, 10*time.Minute)
	provider.now = func() time.Time { return now }

	snapshot1, err := provider.GetGroupCapacityStaticSnapshot(context.Background(), 9)
	require.NoError(t, err)
	snapshot2, err := provider.GetGroupCapacityStaticSnapshot(context.Background(), 9)
	require.NoError(t, err)
	require.Equal(t, snapshot1, snapshot2)
	require.Equal(t, 1, listCalls)

	now = now.Add(2 * time.Minute)
	projector.MarkGroupDirty(9)

	snapshot3, err := provider.GetGroupCapacityStaticSnapshot(context.Background(), 9)
	require.NoError(t, err)
	require.Equal(t, 2, listCalls)
	require.Equal(t, 5, snapshot3.ConcurrencyMax)
	require.Equal(t, 3, snapshot3.SessionsMax)
	require.Equal(t, 20, snapshot3.RPMMax)
	require.Equal(t, now, snapshot3.RebuiltAt)
	require.False(t, projector.IsGroupDirty(9))
}

func TestGroupCapacitySnapshotProvider_RebuildsWhenTTLExpires(t *testing.T) {
	t.Parallel()

	now := time.Unix(1710000200, 0)
	listCalls := 0
	accountRepo := &groupCapacitySnapshotAccountRepoStub{
		listFn: func(ctx context.Context, groupID int64) ([]GroupCapacitySnapshotAccountRecord, error) {
			listCalls++
			return []GroupCapacitySnapshotAccountRecord{{AccountID: 31, Concurrency: listCalls}}, nil
		},
	}
	groupRepo := &groupCapacitySnapshotGroupRepoStub{
		getFn: func(ctx context.Context, groupID int64) (*GroupCapacitySnapshotGroupRecord, error) {
			return &GroupCapacitySnapshotGroupRecord{GroupID: groupID, Status: StatusActive}, nil
		},
	}

	provider := newGroupCapacitySnapshotProviderService(accountRepo, groupRepo, nil, time.Minute)
	provider.now = func() time.Time { return now }

	snapshot1, err := provider.GetGroupCapacityStaticSnapshot(context.Background(), 11)
	require.NoError(t, err)
	require.Equal(t, 1, snapshot1.ConcurrencyMax)
	require.Equal(t, 1, listCalls)

	now = now.Add(30 * time.Second)
	snapshot2, err := provider.GetGroupCapacityStaticSnapshot(context.Background(), 11)
	require.NoError(t, err)
	require.Equal(t, snapshot1, snapshot2)
	require.Equal(t, 1, listCalls)

	now = now.Add(31 * time.Second)
	snapshot3, err := provider.GetGroupCapacityStaticSnapshot(context.Background(), 11)
	require.NoError(t, err)
	require.Equal(t, 2, snapshot3.ConcurrencyMax)
	require.Equal(t, 2, listCalls)
}

func TestGroupCapacitySnapshotProvider_DirtyDuringRebuildStaysDirty(t *testing.T) {
	t.Parallel()

	now := time.Unix(1710000300, 0)
	projector := NewGroupCapacitySnapshotProjector()
	rebuildStarted := make(chan struct{})
	allowFirstRebuildFinish := make(chan struct{})
	listCalls := 0
	accountRepo := &groupCapacitySnapshotAccountRepoStub{
		listFn: func(ctx context.Context, groupID int64) ([]GroupCapacitySnapshotAccountRecord, error) {
			listCalls++
			switch listCalls {
			case 1:
				close(rebuildStarted)
				<-allowFirstRebuildFinish
				return []GroupCapacitySnapshotAccountRecord{{AccountID: 41, Concurrency: 1}}, nil
			case 2:
				return []GroupCapacitySnapshotAccountRecord{{AccountID: 41, Concurrency: 2}}, nil
			default:
				return nil, nil
			}
		},
	}
	groupRepo := &groupCapacitySnapshotGroupRepoStub{
		getFn: func(ctx context.Context, groupID int64) (*GroupCapacitySnapshotGroupRecord, error) {
			return &GroupCapacitySnapshotGroupRecord{GroupID: groupID, Status: StatusActive}, nil
		},
	}
	provider := newGroupCapacitySnapshotProviderService(accountRepo, groupRepo, projector, 10*time.Minute)
	provider.now = func() time.Time { return now }

	type result struct {
		snapshot GroupCapacityStaticSnapshot
		err      error
	}
	firstResult := make(chan result, 1)
	go func() {
		snapshot, err := provider.GetGroupCapacityStaticSnapshot(context.Background(), 13)
		firstResult <- result{snapshot: snapshot, err: err}
	}()

	<-rebuildStarted
	projector.MarkGroupDirty(13)
	close(allowFirstRebuildFinish)

	first := <-firstResult
	require.NoError(t, first.err)
	require.Equal(t, 1, first.snapshot.ConcurrencyMax)
	require.True(t, projector.IsGroupDirty(13))

	now = now.Add(time.Minute)
	second, err := provider.GetGroupCapacityStaticSnapshot(context.Background(), 13)
	require.NoError(t, err)
	require.Equal(t, 2, second.ConcurrencyMax)
	require.Equal(t, 2, listCalls)
	require.False(t, projector.IsGroupDirty(13))
}
