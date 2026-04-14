//go:build unit

package service

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGroupCapacitySnapshotProjector_HandlesOutboxEvents(t *testing.T) {
	t.Parallel()

	projector := NewGroupCapacitySnapshotProjector()
	require.False(t, projector.IsGroupDirty(7))
	require.False(t, projector.IsGroupDirty(8))

	err := projector.HandleSchedulerOutboxEvent(context.Background(), SchedulerOutboxEvent{
		EventType: SchedulerOutboxEventAccountChanged,
		Payload: map[string]any{
			"group_ids": []any{int64(7), json.Number("8")},
		},
	})
	require.NoError(t, err)
	require.True(t, projector.IsGroupDirty(7))
	require.True(t, projector.IsGroupDirty(8))

	projector.MarkGroupClean(7)
	require.False(t, projector.IsGroupDirty(7))
	require.True(t, projector.IsGroupDirty(8))

	err = projector.HandleSchedulerOutboxEvent(context.Background(), SchedulerOutboxEvent{EventType: SchedulerOutboxEventAccountChanged})
	require.NoError(t, err)
	require.True(t, projector.IsGroupDirty(7))
	require.True(t, projector.IsGroupDirty(99))

	projector.MarkGroupClean(7)
	require.False(t, projector.IsGroupDirty(7))
	require.True(t, projector.IsGroupDirty(99))
}

func TestGroupCapacitySnapshotProjector_MarkAllDirtyClearsPreviousCleanMarks(t *testing.T) {
	t.Parallel()

	projector := NewGroupCapacitySnapshotProjector()
	projector.MarkAllDirty()
	projector.MarkGroupClean(7)
	require.False(t, projector.IsGroupDirty(7))

	projector.MarkAllDirty()
	require.True(t, projector.IsGroupDirty(7))
	require.True(t, projector.IsGroupDirty(99))
}

func TestGroupCapacitySnapshotProjector_AccountBulkChangedMarksGroupsDirty(t *testing.T) {
	t.Parallel()

	projector := NewGroupCapacitySnapshotProjector()

	err := projector.HandleSchedulerOutboxEvent(context.Background(), SchedulerOutboxEvent{
		EventType: SchedulerOutboxEventAccountBulkChanged,
		Payload: map[string]any{
			"group_ids": []any{int64(7), json.Number("8")},
		},
	})
	require.NoError(t, err)
	require.True(t, projector.IsGroupDirty(7))
	require.True(t, projector.IsGroupDirty(8))
	require.False(t, projector.IsGroupDirty(9))
}

func TestGroupCapacitySnapshotProjector_MarkGroupCleanIfVersionSkipsNewerDirty(t *testing.T) {
	t.Parallel()

	projector := NewGroupCapacitySnapshotProjector()
	projector.MarkGroupDirty(7)
	version := projector.GroupDirtyVersion(7)
	require.NotZero(t, version)

	projector.MarkGroupDirty(7)
	projector.MarkGroupCleanIfVersion(7, version)
	require.True(t, projector.IsGroupDirty(7))

	projector.MarkGroupCleanIfVersion(7, projector.GroupDirtyVersion(7))
	require.False(t, projector.IsGroupDirty(7))
}

func TestGroupCapacitySnapshotProjector_IsConcurrentSafe(t *testing.T) {
	t.Parallel()

	projector := NewGroupCapacitySnapshotProjector()

	const goroutines = 32
	const iterations = 200

	var wg sync.WaitGroup
	start := make(chan struct{})

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			<-start
			groupID := int64(worker%8 + 1)
			for j := 0; j < iterations; j++ {
				switch j % 4 {
				case 0:
					projector.MarkGroupDirty(groupID)
				case 1:
					projector.MarkGroupClean(groupID)
				case 2:
					projector.MarkAllDirty()
				case 3:
					_ = projector.IsGroupDirty(groupID)
				}
			}
		}(i)
	}

	close(start)
	wg.Wait()

	require.NotPanics(t, func() {
		for groupID := int64(1); groupID <= 8; groupID++ {
			_ = projector.IsGroupDirty(groupID)
		}
	})
}
