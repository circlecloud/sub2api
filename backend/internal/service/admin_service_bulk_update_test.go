//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type accountRepoStubForBulkUpdate struct {
	accountRepoStub
	bulkUpdateErr             error
	bulkUpdateIDs             []int64
	bindGroupErrByID          map[int64]error
	bindGroupsCalls           []int64
	bindGroupsBatchIDs        []int64
	bindGroupsBatchGroupIDs   []int64
	getByIDsAccounts          []*Account
	getByIDsErr               error
	getByIDsCalled            bool
	getByIDsIDs               []int64
	getByIDAccounts           map[int64]*Account
	getByIDErrByID            map[int64]error
	getByIDCalled             []int64
	listByGroupData           map[int64][]Account
	listByGroupErr            map[int64]error
	resolveBulkTargets        []BulkAccountTargetRef
	resolveBulkTargetsErr     error
	resolveBulkTargetsFilter  *AccountBulkFilter
	previewBulkTargets        *BulkAccountTargetPreview
	previewBulkTargetsErr     error
	previewBulkTargetsFilter  *AccountBulkFilter
}

func (s *accountRepoStubForBulkUpdate) BulkUpdate(_ context.Context, ids []int64, _ AccountBulkUpdate) (int64, error) {
	s.bulkUpdateIDs = append([]int64{}, ids...)
	if s.bulkUpdateErr != nil {
		return 0, s.bulkUpdateErr
	}
	return int64(len(ids)), nil
}

func (s *accountRepoStubForBulkUpdate) BindGroups(_ context.Context, accountID int64, _ []int64) error {
	s.bindGroupsCalls = append(s.bindGroupsCalls, accountID)
	if err, ok := s.bindGroupErrByID[accountID]; ok {
		return err
	}
	return nil
}

func (s *accountRepoStubForBulkUpdate) BindGroupsBatch(_ context.Context, accountIDs []int64, groupIDs []int64) error {
	s.bindGroupsBatchIDs = append([]int64{}, accountIDs...)
	s.bindGroupsBatchGroupIDs = append([]int64{}, groupIDs...)
	return nil
}

func (s *accountRepoStubForBulkUpdate) GetByIDs(_ context.Context, ids []int64) ([]*Account, error) {
	s.getByIDsCalled = true
	s.getByIDsIDs = append([]int64{}, ids...)
	if s.getByIDsErr != nil {
		return nil, s.getByIDsErr
	}
	return s.getByIDsAccounts, nil
}

func (s *accountRepoStubForBulkUpdate) GetByID(_ context.Context, id int64) (*Account, error) {
	s.getByIDCalled = append(s.getByIDCalled, id)
	if err, ok := s.getByIDErrByID[id]; ok {
		return nil, err
	}
	if account, ok := s.getByIDAccounts[id]; ok {
		return account, nil
	}
	return nil, errors.New("account not found")
}

func (s *accountRepoStubForBulkUpdate) ListByGroup(_ context.Context, groupID int64) ([]Account, error) {
	if err, ok := s.listByGroupErr[groupID]; ok {
		return nil, err
	}
	if rows, ok := s.listByGroupData[groupID]; ok {
		return rows, nil
	}
	return nil, nil
}

func (s *accountRepoStubForBulkUpdate) ResolveBulkUpdateTargets(_ context.Context, filter AccountBulkFilter) ([]BulkAccountTargetRef, error) {
	copied := filter
	s.resolveBulkTargetsFilter = &copied
	if s.resolveBulkTargetsErr != nil {
		return nil, s.resolveBulkTargetsErr
	}
	return append([]BulkAccountTargetRef(nil), s.resolveBulkTargets...), nil
}

func (s *accountRepoStubForBulkUpdate) PreviewBulkUpdateTargets(_ context.Context, filter AccountBulkFilter) (*BulkAccountTargetPreview, error) {
	copied := filter
	s.previewBulkTargetsFilter = &copied
	if s.previewBulkTargetsErr != nil {
		return nil, s.previewBulkTargetsErr
	}
	if s.previewBulkTargets == nil {
		return &BulkAccountTargetPreview{}, nil
	}
	preview := *s.previewBulkTargets
	preview.Platforms = append([]string(nil), preview.Platforms...)
	preview.Types = append([]string(nil), preview.Types...)
	return &preview, nil
}

// TestAdminService_BulkUpdateAccounts_AllSuccessIDs 验证批量更新成功时返回 success_ids/failed_ids。
func TestAdminService_BulkUpdateAccounts_AllSuccessIDs(t *testing.T) {
	repo := &accountRepoStubForBulkUpdate{}
	svc := &adminServiceImpl{accountRepo: repo}

	schedulable := true
	input := &BulkUpdateAccountsInput{
		AccountIDs:  []int64{1, 2, 3},
		Schedulable: &schedulable,
	}

	result, err := svc.BulkUpdateAccounts(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, 3, result.Success)
	require.Equal(t, 0, result.Failed)
	require.ElementsMatch(t, []int64{1, 2, 3}, result.SuccessIDs)
	require.Empty(t, result.FailedIDs)
	require.Len(t, result.Results, 3)
}

// TestAdminService_BulkUpdateAccounts_IDScopeUsesBatchBindGroups 验证 ID 范围批量更新会优先走 batch bind。
func TestAdminService_BulkUpdateAccounts_IDScopeUsesBatchBindGroups(t *testing.T) {
	repo := &accountRepoStubForBulkUpdate{}
	svc := &adminServiceImpl{
		accountRepo: repo,
		groupRepo:   &groupRepoStubForAdmin{getByID: &Group{ID: 10, Name: "g10"}},
	}

	groupIDs := []int64{10}
	schedulable := false
	input := &BulkUpdateAccountsInput{
		AccountIDs:            []int64{1, 2, 3},
		GroupIDs:              &groupIDs,
		Schedulable:           &schedulable,
		SkipMixedChannelCheck: true,
	}

	result, err := svc.BulkUpdateAccounts(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, 3, result.Success)
	require.Equal(t, []int64{1, 2, 3}, result.SuccessIDs)
	require.Equal(t, []int64{1, 2, 3}, repo.bindGroupsBatchIDs)
	require.Equal(t, []int64{10}, repo.bindGroupsBatchGroupIDs)
	require.Empty(t, repo.bindGroupsCalls, "支持 batch bind 时不应再逐账号绑定")
}

func TestAdminService_BulkUpdateAccounts_NilGroupRepoReturnsError(t *testing.T) {
	repo := &accountRepoStubForBulkUpdate{}
	svc := &adminServiceImpl{accountRepo: repo}

	groupIDs := []int64{10}
	input := &BulkUpdateAccountsInput{
		AccountIDs: []int64{1},
		GroupIDs:   &groupIDs,
	}

	result, err := svc.BulkUpdateAccounts(context.Background(), input)
	require.Nil(t, result)
	require.Error(t, err)
	require.Contains(t, err.Error(), "group repository not configured")
}

func TestAdminService_BulkUpdateAccounts_FilterScopeUsesResolvedTargetsAndBatchBindGroups(t *testing.T) {
	repo := &accountRepoStubForBulkUpdate{
		resolveBulkTargets: []BulkAccountTargetRef{
			{ID: 11, Platform: PlatformAnthropic},
			{ID: 22, Platform: PlatformAnthropic},
		},
	}
	svc := &adminServiceImpl{
		accountRepo: repo,
		groupRepo:   &groupRepoStubForAdmin{getByID: &Group{ID: 10, Name: "g10"}},
	}

	groupIDs := []int64{10}
	schedulable := true
	input := &BulkUpdateAccountsInput{
		Filters: &AccountBulkFilter{
			Platform: PlatformAnthropic,
			Search:   "claude",
			GroupIDs: "1,2,3",
		},
		GroupIDs:              &groupIDs,
		Schedulable:           &schedulable,
		SkipMixedChannelCheck: true,
	}

	result, err := svc.BulkUpdateAccounts(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, 2, result.Success)
	require.Equal(t, []int64{11, 22}, repo.bulkUpdateIDs)
	require.Equal(t, []int64{11, 22}, repo.bindGroupsBatchIDs)
	require.Equal(t, []int64{10}, repo.bindGroupsBatchGroupIDs)
	require.False(t, repo.getByIDsCalled, "filter scope 应直接解析轻量目标，而不是预拉完整账号")
	require.NotNil(t, repo.resolveBulkTargetsFilter)
	require.Equal(t, PlatformAnthropic, repo.resolveBulkTargetsFilter.Platform)
	require.Equal(t, "claude", repo.resolveBulkTargetsFilter.Search)
}

func TestAdminService_PreviewBulkUpdateTargets_UsesRepositoryPreview(t *testing.T) {
	repo := &accountRepoStubForBulkUpdate{
		previewBulkTargets: &BulkAccountTargetPreview{
			Count:     128,
			Platforms: []string{PlatformAnthropic},
			Types:     []string{AccountTypeAPIKey},
		},
	}
	svc := &adminServiceImpl{accountRepo: repo}

	preview, err := svc.PreviewBulkUpdateTargets(context.Background(), AccountBulkFilter{
		Platform: PlatformAnthropic,
		GroupIDs: "1,2,3",
	})
	require.NoError(t, err)
	require.NotNil(t, preview)
	require.Equal(t, int64(128), preview.Count)
	require.Equal(t, []string{PlatformAnthropic}, preview.Platforms)
	require.Equal(t, []string{AccountTypeAPIKey}, preview.Types)
	require.NotNil(t, repo.previewBulkTargetsFilter)
	require.Equal(t, "1,2,3", repo.previewBulkTargetsFilter.GroupIDs)
}

func TestAdminService_ResolveBulkUpdateTargets_UsesRepositoryResolver(t *testing.T) {
	repo := &accountRepoStubForBulkUpdate{
		resolveBulkTargets: []BulkAccountTargetRef{
			{ID: 11, Platform: PlatformAnthropic, Type: AccountTypeOAuth},
			{ID: 22, Platform: PlatformAnthropic, Type: AccountTypeAPIKey},
		},
	}
	svc := &adminServiceImpl{accountRepo: repo}

	resolved, err := svc.ResolveBulkUpdateTargets(context.Background(), AccountBulkFilter{
		Platform: PlatformAnthropic,
		GroupIDs: "1,2,3",
	})
	require.NoError(t, err)
	require.Equal(t, []BulkAccountTargetRef{
		{ID: 11, Platform: PlatformAnthropic, Type: AccountTypeOAuth},
		{ID: 22, Platform: PlatformAnthropic, Type: AccountTypeAPIKey},
	}, resolved)
	require.NotNil(t, repo.resolveBulkTargetsFilter)
	require.Equal(t, "1,2,3", repo.resolveBulkTargetsFilter.GroupIDs)
}

// TestAdminService_BulkUpdateAccounts_MixedChannelPreCheckBlocksOnExistingConflict verifies
// that the global pre-check detects a conflict with existing group members and returns an
// error before any DB write is performed.
func TestAdminService_BulkUpdateAccounts_MixedChannelPreCheckBlocksOnExistingConflict(t *testing.T) {
	repo := &accountRepoStubForBulkUpdate{
		getByIDsAccounts: []*Account{
			{ID: 1, Platform: PlatformAntigravity},
		},
		// Group 10 already contains an Anthropic account.
		listByGroupData: map[int64][]Account{
			10: {{ID: 99, Platform: PlatformAnthropic}},
		},
	}
	svc := &adminServiceImpl{
		accountRepo: repo,
		groupRepo:   &groupRepoStubForAdmin{getByID: &Group{ID: 10, Name: "target-group"}},
	}

	groupIDs := []int64{10}
	input := &BulkUpdateAccountsInput{
		AccountIDs: []int64{1},
		GroupIDs:   &groupIDs,
	}

	result, err := svc.BulkUpdateAccounts(context.Background(), input)
	require.Nil(t, result)
	require.Error(t, err)
	require.Contains(t, err.Error(), "mixed channel")
	// No BindGroups should have been called since the check runs before any write.
	require.Empty(t, repo.bindGroupsCalls)
}
