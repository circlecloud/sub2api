package service

import (
	"context"
	"encoding/json"
	"sync"
)

type GroupCapacitySnapshotProjector struct {
	mu sync.RWMutex

	globalDirty        bool
	globalDirtyVersion uint64
	dirtyByID          map[int64]struct{}
	cleanByID          map[int64]struct{}
	dirtyVersionByID   map[int64]uint64
}

func NewGroupCapacitySnapshotProjector() *GroupCapacitySnapshotProjector {
	return &GroupCapacitySnapshotProjector{
		dirtyByID:        make(map[int64]struct{}),
		cleanByID:        make(map[int64]struct{}),
		dirtyVersionByID: make(map[int64]uint64),
	}
}

func (p *GroupCapacitySnapshotProjector) HandleSchedulerOutboxEvent(ctx context.Context, event SchedulerOutboxEvent) error {
	_ = ctx
	if p == nil {
		return nil
	}
	switch event.EventType {
	case SchedulerOutboxEventAccountChanged, SchedulerOutboxEventAccountGroupsChanged, SchedulerOutboxEventAccountBulkChanged:
		groupIDs := parseProjectorInt64Slice(nil)
		if event.Payload != nil {
			groupIDs = parseProjectorInt64Slice(event.Payload["group_ids"])
		}
		if len(groupIDs) == 0 {
			p.MarkAllDirty()
			return nil
		}
		for _, groupID := range groupIDs {
			p.MarkGroupDirty(groupID)
		}
	case SchedulerOutboxEventGroupChanged:
		if event.GroupID == nil || *event.GroupID <= 0 {
			p.MarkAllDirty()
			return nil
		}
		p.MarkGroupDirty(*event.GroupID)
	case SchedulerOutboxEventFullRebuild:
		p.MarkAllDirty()
	}
	return nil
}

func (p *GroupCapacitySnapshotProjector) MarkGroupDirty(groupID int64) {
	if p == nil || groupID <= 0 {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	p.ensureMapsLocked()
	p.globalDirtyVersion++
	p.dirtyVersionByID[groupID] = p.globalDirtyVersion
	p.dirtyByID[groupID] = struct{}{}
	delete(p.cleanByID, groupID)
}

func (p *GroupCapacitySnapshotProjector) MarkGroupClean(groupID int64) {
	if p == nil || groupID <= 0 {
		return
	}
	p.MarkGroupCleanIfVersion(groupID, p.GroupDirtyVersion(groupID))
}

func (p *GroupCapacitySnapshotProjector) MarkGroupCleanIfVersion(groupID int64, version uint64) {
	if p == nil || groupID <= 0 {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	p.ensureMapsLocked()
	if p.currentDirtyVersionLocked(groupID) != version {
		return
	}
	delete(p.dirtyByID, groupID)
	if p.globalDirty {
		p.cleanByID[groupID] = struct{}{}
		return
	}
	delete(p.cleanByID, groupID)
}

func (p *GroupCapacitySnapshotProjector) MarkAllDirty() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	p.globalDirtyVersion++
	p.globalDirty = true
	p.dirtyByID = make(map[int64]struct{})
	p.cleanByID = make(map[int64]struct{})
	p.dirtyVersionByID = make(map[int64]uint64)
}

func (p *GroupCapacitySnapshotProjector) ResetAllDirty() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	p.globalDirty = false
	p.dirtyByID = make(map[int64]struct{})
	p.cleanByID = make(map[int64]struct{})
	p.dirtyVersionByID = make(map[int64]uint64)
}

func (p *GroupCapacitySnapshotProjector) IsGroupDirty(groupID int64) bool {
	if p == nil || groupID <= 0 {
		return false
	}
	p.mu.RLock()
	defer p.mu.RUnlock()

	if _, ok := p.dirtyByID[groupID]; ok {
		return true
	}
	if p.globalDirty {
		_, clean := p.cleanByID[groupID]
		return !clean
	}
	return false
}

func (p *GroupCapacitySnapshotProjector) GroupDirtyVersion(groupID int64) uint64 {
	if p == nil || groupID <= 0 {
		return 0
	}
	p.mu.RLock()
	defer p.mu.RUnlock()

	return p.currentDirtyVersionLocked(groupID)
}

func (p *GroupCapacitySnapshotProjector) currentDirtyVersionLocked(groupID int64) uint64 {
	if _, ok := p.dirtyByID[groupID]; ok {
		if version, exists := p.dirtyVersionByID[groupID]; exists {
			return version
		}
	}
	if p.globalDirty {
		if _, clean := p.cleanByID[groupID]; !clean {
			return p.globalDirtyVersion
		}
	}
	return 0
}

func (p *GroupCapacitySnapshotProjector) ensureMapsLocked() {
	if p.dirtyByID == nil {
		p.dirtyByID = make(map[int64]struct{})
	}
	if p.cleanByID == nil {
		p.cleanByID = make(map[int64]struct{})
	}
	if p.dirtyVersionByID == nil {
		p.dirtyVersionByID = make(map[int64]uint64)
	}
}

func parseProjectorInt64Slice(value any) []int64 {
	items, ok := value.([]any)
	if !ok {
		if ints, ok := value.([]int64); ok {
			out := make([]int64, 0, len(ints))
			for _, item := range ints {
				if item > 0 {
					out = append(out, item)
				}
			}
			return out
		}
		return nil
	}
	out := make([]int64, 0, len(items))
	for _, item := range items {
		if v, ok := projectorToInt64(item); ok && v > 0 {
			out = append(out, v)
		}
	}
	return out
}

func projectorToInt64(value any) (int64, bool) {
	switch v := value.(type) {
	case float64:
		return int64(v), true
	case int:
		return int64(v), true
	case int8:
		return int64(v), true
	case int16:
		return int64(v), true
	case int32:
		return int64(v), true
	case int64:
		return v, true
	case uint:
		return int64(v), true
	case uint8:
		return int64(v), true
	case uint16:
		return int64(v), true
	case uint32:
		return int64(v), true
	case json.Number:
		parsed, err := v.Int64()
		return parsed, err == nil
	default:
		return 0, false
	}
}
