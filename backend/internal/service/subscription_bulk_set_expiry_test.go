package service

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type statusUpdateCall struct {
	id     int64
	status string
}

type batchSubscriptionRepoStub struct {
	userSubRepoNoop

	subs        map[int64]*UserSubscription
	orderedIDs  []int64
	listErr     error
	extendErr   error
	statusErr   error
	deleteErr   error
	extendCalls []int64
	statusCalls []statusUpdateCall
	deleteCalls []int64
}

func newBatchSubscriptionRepoStub(subs ...*UserSubscription) *batchSubscriptionRepoStub {
	stub := &batchSubscriptionRepoStub{
		subs:       make(map[int64]*UserSubscription, len(subs)),
		orderedIDs: make([]int64, 0, len(subs)),
	}
	for _, sub := range subs {
		if sub == nil {
			continue
		}
		cp := *sub
		stub.subs[cp.ID] = &cp
		stub.orderedIDs = append(stub.orderedIDs, cp.ID)
	}
	sort.Slice(stub.orderedIDs, func(i, j int) bool {
		return stub.orderedIDs[i] < stub.orderedIDs[j]
	})
	return stub
}

func (s *batchSubscriptionRepoStub) List(_ context.Context, params pagination.PaginationParams, userID, groupID *int64, status, platform, _, _ string) ([]UserSubscription, *pagination.PaginationResult, error) {
	if s.listErr != nil {
		return nil, nil, s.listErr
	}

	now := time.Now()
	filtered := make([]UserSubscription, 0, len(s.orderedIDs))
	for _, id := range s.orderedIDs {
		sub := s.subs[id]
		if sub == nil {
			continue
		}
		if userID != nil && sub.UserID != *userID {
			continue
		}
		if groupID != nil && sub.GroupID != *groupID {
			continue
		}
		if platform != "" {
			if sub.Group == nil || sub.Group.Platform != platform {
				continue
			}
		}
		switch status {
		case SubscriptionStatusActive:
			if sub.Status != SubscriptionStatusActive || !sub.ExpiresAt.After(now) {
				continue
			}
		case SubscriptionStatusExpired:
			if sub.Status != SubscriptionStatusExpired && (sub.Status != SubscriptionStatusActive || sub.ExpiresAt.After(now)) {
				continue
			}
		case SubscriptionStatusSuspended:
			if sub.Status != SubscriptionStatusSuspended {
				continue
			}
		case "":
			// no-op
		default:
			if sub.Status != status {
				continue
			}
		}
		filtered = append(filtered, *sub)
	}

	page := params.Page
	if page < 1 {
		page = 1
	}
	pageSize := params.Limit()
	start := params.Offset()
	pages := calcPages(len(filtered), pageSize)
	if start >= len(filtered) {
		return []UserSubscription{}, &pagination.PaginationResult{
			Total:    int64(len(filtered)),
			Page:     page,
			PageSize: pageSize,
			Pages:    pages,
		}, nil
	}
	end := start + pageSize
	if end > len(filtered) {
		end = len(filtered)
	}
	items := append([]UserSubscription(nil), filtered[start:end]...)
	return items, &pagination.PaginationResult{
		Total:    int64(len(filtered)),
		Page:     page,
		PageSize: pageSize,
		Pages:    pages,
	}, nil
}

func (s *batchSubscriptionRepoStub) ExtendExpiry(_ context.Context, subscriptionID int64, newExpiresAt time.Time) error {
	if s.extendErr != nil {
		return s.extendErr
	}
	sub := s.subs[subscriptionID]
	if sub == nil {
		return ErrSubscriptionNotFound
	}
	sub.ExpiresAt = newExpiresAt
	s.extendCalls = append(s.extendCalls, subscriptionID)
	return nil
}

func (s *batchSubscriptionRepoStub) UpdateStatus(_ context.Context, subscriptionID int64, status string) error {
	if s.statusErr != nil {
		return s.statusErr
	}
	sub := s.subs[subscriptionID]
	if sub == nil {
		return ErrSubscriptionNotFound
	}
	sub.Status = status
	s.statusCalls = append(s.statusCalls, statusUpdateCall{id: subscriptionID, status: status})
	return nil
}

func (s *batchSubscriptionRepoStub) Delete(_ context.Context, subscriptionID int64) error {
	if s.deleteErr != nil {
		return s.deleteErr
	}
	if _, ok := s.subs[subscriptionID]; !ok {
		return ErrSubscriptionNotFound
	}
	delete(s.subs, subscriptionID)
	for i, id := range s.orderedIDs {
		if id == subscriptionID {
			s.orderedIDs = append(s.orderedIDs[:i], s.orderedIDs[i+1:]...)
			break
		}
	}
	s.deleteCalls = append(s.deleteCalls, subscriptionID)
	return nil
}

func calcPages(total, pageSize int) int {
	if total == 0 {
		return 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	pages := total / pageSize
	if total%pageSize != 0 {
		pages++
	}
	if pages < 1 {
		pages = 1
	}
	return pages
}

func TestSetFilteredSubscriptionsExpiry_InvalidExpiry(t *testing.T) {
	svc := NewSubscriptionService(groupRepoNoop{}, newBatchSubscriptionRepoStub(), nil, nil, nil)

	_, err := svc.SetFilteredSubscriptionsExpiry(context.Background(), SubscriptionBatchFilter{Status: SubscriptionStatusActive}, time.Time{})
	require.ErrorIs(t, err, ErrInvalidSubscriptionExpiry)

	_, err = svc.SetFilteredSubscriptionsExpiry(context.Background(), SubscriptionBatchFilter{Status: SubscriptionStatusActive}, MaxExpiresAt.Add(time.Second))
	require.ErrorIs(t, err, ErrInvalidSubscriptionExpiry)
}

func TestSetFilteredSubscriptionsExpiry_RequiresBatchFilter(t *testing.T) {
	svc := NewSubscriptionService(groupRepoNoop{}, newBatchSubscriptionRepoStub(), nil, nil, nil)

	_, err := svc.SetFilteredSubscriptionsExpiry(context.Background(), SubscriptionBatchFilter{}, time.Now().Add(24*time.Hour))

	require.ErrorIs(t, err, ErrSubscriptionBatchFilterNeeded)
}

func TestSetFilteredSubscriptionsExpiry_UsesCurrentFilters(t *testing.T) {
	groupID := int64(7)
	stub := newBatchSubscriptionRepoStub(
		&UserSubscription{ID: 1, UserID: 101, GroupID: groupID, Status: SubscriptionStatusExpired, ExpiresAt: time.Now().Add(-24 * time.Hour)},
		&UserSubscription{ID: 2, UserID: 102, GroupID: groupID, Status: SubscriptionStatusActive, ExpiresAt: time.Now().Add(-2 * time.Hour)},
		&UserSubscription{ID: 3, UserID: 103, GroupID: groupID, Status: SubscriptionStatusActive, ExpiresAt: time.Now().Add(24 * time.Hour)},
		&UserSubscription{ID: 4, UserID: 104, GroupID: 99, Status: SubscriptionStatusExpired, ExpiresAt: time.Now().Add(-24 * time.Hour)},
	)
	svc := NewSubscriptionService(groupRepoNoop{}, stub, nil, nil, nil)
	target := time.Now().Add(30 * 24 * time.Hour).UTC().Truncate(time.Second)

	result, err := svc.SetFilteredSubscriptionsExpiry(context.Background(), SubscriptionBatchFilter{
		GroupID: &groupID,
		Status:  SubscriptionStatusExpired,
	}, target)

	require.NoError(t, err)
	require.Equal(t, 2, result.UpdatedCount)
	require.Equal(t, SubscriptionStatusActive, result.Status)
	require.Equal(t, target, stub.subs[1].ExpiresAt)
	require.Equal(t, target, stub.subs[2].ExpiresAt)
	require.Equal(t, SubscriptionStatusActive, stub.subs[1].Status)
	require.Equal(t, SubscriptionStatusActive, stub.subs[2].Status)
	require.NotEqual(t, target, stub.subs[3].ExpiresAt)
	require.NotEqual(t, target, stub.subs[4].ExpiresAt)
	require.ElementsMatch(t, []int64{1, 2}, stub.extendCalls)
	require.ElementsMatch(t, []statusUpdateCall{
		{id: 1, status: SubscriptionStatusActive},
		{id: 2, status: SubscriptionStatusActive},
	}, stub.statusCalls)
}

func TestSetFilteredSubscriptionsExpiry_PreservesSuspendedStatus(t *testing.T) {
	groupID := int64(8)
	stub := newBatchSubscriptionRepoStub(
		&UserSubscription{ID: 11, UserID: 201, GroupID: groupID, Status: SubscriptionStatusSuspended, ExpiresAt: time.Now().Add(48 * time.Hour)},
		&UserSubscription{ID: 12, UserID: 202, GroupID: groupID, Status: SubscriptionStatusExpired, ExpiresAt: time.Now().Add(-48 * time.Hour)},
	)
	svc := NewSubscriptionService(groupRepoNoop{}, stub, nil, nil, nil)
	target := time.Now().Add(14 * 24 * time.Hour).UTC().Truncate(time.Second)

	result, err := svc.SetFilteredSubscriptionsExpiry(context.Background(), SubscriptionBatchFilter{GroupID: &groupID}, target)

	require.NoError(t, err)
	require.Equal(t, 2, result.UpdatedCount)
	require.Equal(t, target, stub.subs[11].ExpiresAt)
	require.Equal(t, target, stub.subs[12].ExpiresAt)
	require.Equal(t, SubscriptionStatusSuspended, stub.subs[11].Status)
	require.Equal(t, SubscriptionStatusActive, stub.subs[12].Status)
	require.Equal(t, []statusUpdateCall{{id: 12, status: SubscriptionStatusActive}}, stub.statusCalls)
}

func TestRevokeFilteredSubscriptions_DeletesAcrossPages(t *testing.T) {
	groupID := int64(11)
	subs := make([]*UserSubscription, 0, 121)
	for i := 1; i <= 120; i++ {
		subs = append(subs, &UserSubscription{
			ID:        int64(i),
			UserID:    int64(1000 + i),
			GroupID:   groupID,
			Status:    SubscriptionStatusActive,
			ExpiresAt: time.Now().Add(72 * time.Hour),
		})
	}
	subs = append(subs, &UserSubscription{
		ID:        999,
		UserID:    9999,
		GroupID:   12,
		Status:    SubscriptionStatusActive,
		ExpiresAt: time.Now().Add(72 * time.Hour),
	})
	stub := newBatchSubscriptionRepoStub(subs...)
	svc := NewSubscriptionService(groupRepoNoop{}, stub, nil, nil, nil)

	result, err := svc.RevokeFilteredSubscriptions(context.Background(), SubscriptionBatchFilter{
		GroupID: &groupID,
		Status:  SubscriptionStatusActive,
	})

	require.NoError(t, err)
	require.Equal(t, 120, result.DeletedCount)
	require.Len(t, stub.deleteCalls, 120)
	_, remaining := stub.subs[999]
	require.True(t, remaining)
	_, deleted := stub.subs[1]
	require.False(t, deleted)
}
