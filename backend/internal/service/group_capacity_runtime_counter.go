package service

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
)

type groupCapacityRuntimeAccountGroupResolver func(ctx context.Context, accountID int64) ([]int64, error)

// GroupCapacityRuntimeCounter keeps per-group in-process concurrency usage.
type GroupCapacityRuntimeCounter struct {
	accountRepo          AccountRepository
	accountGroupResolver groupCapacityRuntimeAccountGroupResolver
	groupUsage           sync.Map // map[int64]*atomic.Int64
}

func NewGroupCapacityRuntimeCounter(accountRepo AccountRepository) *GroupCapacityRuntimeCounter {
	counter := &GroupCapacityRuntimeCounter{accountRepo: accountRepo}
	counter.accountGroupResolver = counter.resolveAccountGroupIDs
	return counter
}

func (c *GroupCapacityRuntimeCounter) TrackAcquire(ctx context.Context, accountID int64) (func(), error) {
	if c == nil || accountID <= 0 {
		return func() {}, nil
	}
	groupIDs, err := c.getAccountGroupIDs(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if len(groupIDs) == 0 {
		return func() {}, nil
	}
	for _, groupID := range groupIDs {
		if groupID <= 0 {
			continue
		}
		c.groupCounter(groupID).Add(1)
	}

	var once sync.Once
	return func() {
		once.Do(func() {
			for _, groupID := range groupIDs {
				if groupID <= 0 {
					continue
				}
				counter := c.groupCounter(groupID)
				for {
					current := counter.Load()
					if current <= 0 {
						break
					}
					if counter.CompareAndSwap(current, current-1) {
						break
					}
				}
			}
		})
	}, nil
}

func (c *GroupCapacityRuntimeCounter) GetGroupConcurrencyUsed(groupID int64) int {
	if c == nil || groupID <= 0 {
		return 0
	}
	value, ok := c.groupUsage.Load(groupID)
	if !ok {
		return 0
	}
	counter, ok := value.(*atomic.Int64)
	if !ok || counter == nil {
		return 0
	}
	current := counter.Load()
	if current <= 0 {
		return 0
	}
	return int(current)
}

func (c *GroupCapacityRuntimeCounter) groupCounter(groupID int64) *atomic.Int64 {
	if value, ok := c.groupUsage.Load(groupID); ok {
		if counter, ok := value.(*atomic.Int64); ok && counter != nil {
			return counter
		}
	}
	counter := &atomic.Int64{}
	actual, _ := c.groupUsage.LoadOrStore(groupID, counter)
	if stored, ok := actual.(*atomic.Int64); ok && stored != nil {
		return stored
	}
	return counter
}

func (c *GroupCapacityRuntimeCounter) getAccountGroupIDs(ctx context.Context, accountID int64) ([]int64, error) {
	if c == nil || accountID <= 0 {
		return nil, nil
	}
	resolver := c.accountGroupResolver
	if resolver == nil {
		resolver = c.resolveAccountGroupIDs
	}
	groupIDs, err := resolver(ctx, accountID)
	if err != nil {
		return nil, err
	}
	copied := append([]int64(nil), groupIDs...)
	return append([]int64(nil), copied...), nil
}

func (c *GroupCapacityRuntimeCounter) resolveAccountGroupIDs(ctx context.Context, accountID int64) ([]int64, error) {
	if c == nil || c.accountRepo == nil || accountID <= 0 {
		return nil, nil
	}
	account, err := c.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("resolve account groups: %w", err)
	}
	if account == nil || len(account.GroupIDs) == 0 {
		return nil, nil
	}
	return append([]int64(nil), account.GroupIDs...), nil
}
