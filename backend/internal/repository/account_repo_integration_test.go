//go:build integration

package repository

import (
	"context"
	"strconv"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/accountgroup"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/suite"
)

type AccountRepoSuite struct {
	suite.Suite
	ctx    context.Context
	client *dbent.Client
	repo   *accountRepository
}

type schedulerCacheRecorder struct {
	setAccounts []*service.Account
	accounts    map[int64]*service.Account
}

func (s *schedulerCacheRecorder) GetSnapshot(ctx context.Context, bucket service.SchedulerBucket) ([]*service.Account, bool, error) {
	return nil, false, nil
}

func (s *schedulerCacheRecorder) SetSnapshot(ctx context.Context, bucket service.SchedulerBucket, accounts []service.Account) error {
	return nil
}

func (s *schedulerCacheRecorder) GetAccount(ctx context.Context, accountID int64) (*service.Account, error) {
	if s.accounts == nil {
		return nil, nil
	}
	return s.accounts[accountID], nil
}

func (s *schedulerCacheRecorder) SetAccount(ctx context.Context, account *service.Account) error {
	s.setAccounts = append(s.setAccounts, account)
	if s.accounts == nil {
		s.accounts = make(map[int64]*service.Account)
	}
	if account != nil {
		s.accounts[account.ID] = account
	}
	return nil
}

func (s *schedulerCacheRecorder) DeleteAccount(ctx context.Context, accountID int64) error {
	return nil
}

func (s *schedulerCacheRecorder) UpdateLastUsed(ctx context.Context, updates map[int64]time.Time) error {
	return nil
}

func (s *schedulerCacheRecorder) TryLockBucket(ctx context.Context, bucket service.SchedulerBucket, ttl time.Duration) (bool, error) {
	return true, nil
}

func (s *schedulerCacheRecorder) ListBuckets(ctx context.Context) ([]service.SchedulerBucket, error) {
	return nil, nil
}

func (s *schedulerCacheRecorder) GetOutboxWatermark(ctx context.Context) (int64, error) {
	return 0, nil
}

func (s *schedulerCacheRecorder) SetOutboxWatermark(ctx context.Context, id int64) error {
	return nil
}

func (s *AccountRepoSuite) SetupTest() {
	s.ctx = context.Background()
	tx := testEntTx(s.T())
	s.client = tx.Client()
	s.repo = newAccountRepositoryWithSQL(s.client, tx, nil)
}

func TestAccountRepoSuite(t *testing.T) {
	suite.Run(t, new(AccountRepoSuite))
}

// --- Create / GetByID / Update / Delete ---

func (s *AccountRepoSuite) TestCreate() {
	account := &service.Account{
		Name:        "test-create",
		Platform:    service.PlatformAnthropic,
		Type:        service.AccountTypeOAuth,
		Status:      service.StatusActive,
		Credentials: map[string]any{},
		Extra:       map[string]any{},
		Concurrency: 3,
		Priority:    50,
		Schedulable: true,
	}

	err := s.repo.Create(s.ctx, account)
	s.Require().NoError(err, "Create")
	s.Require().NotZero(account.ID, "expected ID to be set")

	got, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err, "GetByID")
	s.Require().Equal("test-create", got.Name)
}

func (s *AccountRepoSuite) TestGetByID_NotFound() {
	_, err := s.repo.GetByID(s.ctx, 999999)
	s.Require().Error(err, "expected error for non-existent ID")
}

func (s *AccountRepoSuite) TestUpdate() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "original"})

	account.Name = "updated"
	err := s.repo.Update(s.ctx, account)
	s.Require().NoError(err, "Update")

	got, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err, "GetByID after update")
	s.Require().Equal("updated", got.Name)
}

func (s *AccountRepoSuite) TestUpdate_SyncSchedulerSnapshotOnDisabled() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "sync-update", Status: service.StatusActive, Schedulable: true})
	cacheRecorder := &schedulerCacheRecorder{}
	s.repo.schedulerCache = cacheRecorder

	account.Status = service.StatusDisabled
	err := s.repo.Update(s.ctx, account)
	s.Require().NoError(err, "Update")

	s.Require().Len(cacheRecorder.setAccounts, 1)
	s.Require().Equal(account.ID, cacheRecorder.setAccounts[0].ID)
	s.Require().Equal(service.StatusDisabled, cacheRecorder.setAccounts[0].Status)
}

func (s *AccountRepoSuite) TestUpdate_SyncSchedulerSnapshotOnCredentialsChange() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:        "sync-credentials-update",
		Status:      service.StatusActive,
		Schedulable: true,
		Credentials: map[string]any{
			"model_mapping": map[string]any{
				"gpt-5": "gpt-5.1",
			},
		},
	})
	cacheRecorder := &schedulerCacheRecorder{}
	s.repo.schedulerCache = cacheRecorder

	account.Credentials = map[string]any{
		"model_mapping": map[string]any{
			"gpt-5": "gpt-5.2",
		},
	}
	err := s.repo.Update(s.ctx, account)
	s.Require().NoError(err, "Update")

	s.Require().Len(cacheRecorder.setAccounts, 1)
	s.Require().Equal(account.ID, cacheRecorder.setAccounts[0].ID)
	mapping, ok := cacheRecorder.setAccounts[0].Credentials["model_mapping"].(map[string]any)
	s.Require().True(ok)
	s.Require().Equal("gpt-5.2", mapping["gpt-5"])
}

func (s *AccountRepoSuite) TestDelete() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "to-delete"})

	err := s.repo.Delete(s.ctx, account.ID)
	s.Require().NoError(err, "Delete")

	_, err = s.repo.GetByID(s.ctx, account.ID)
	s.Require().Error(err, "expected error after delete")
}

func (s *AccountRepoSuite) TestDelete_WithGroupBindings() {
	group := mustCreateGroup(s.T(), s.client, &service.Group{Name: "g-del"})
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-del"})
	mustBindAccountToGroup(s.T(), s.client, account.ID, group.ID, 1)

	err := s.repo.Delete(s.ctx, account.ID)
	s.Require().NoError(err, "Delete should cascade remove bindings")

	count, err := s.client.AccountGroup.Query().Where(accountgroup.AccountIDEQ(account.ID)).Count(s.ctx)
	s.Require().NoError(err)
	s.Require().Zero(count, "expected bindings to be removed")
}

// --- List / ListWithFilters ---

func (s *AccountRepoSuite) TestList() {
	mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc1"})
	mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc2"})

	accounts, page, err := s.repo.List(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10})
	s.Require().NoError(err, "List")
	s.Require().Len(accounts, 2)
	s.Require().Equal(int64(2), page.Total)
}

func (s *AccountRepoSuite) TestListWithFilters() {
	singleGroupFilter := ""
	multipleGroupFilter := ""
	excludeGroupFilter := ""
	exactExcludeGroupFilter := ""
	searchFilter := ""
	tests := []struct {
		name                      string
		setup                     func(client *dbent.Client)
		platform                  string
		accType                   string
		status                    string
		search                    string
		resolveSearch             func(client *dbent.Client) string
		groupFilter               string
		groupExcludeFilter        string
		groupExact                bool
		resolveGroupFilter        func(client *dbent.Client) string
		resolveGroupExcludeFilter func(client *dbent.Client) string
		privacyMode               string
		wantCount                 int
		validate                  func(accounts []service.Account)
	}{

		{
			name: "filter_by_platform",
			setup: func(client *dbent.Client) {
				mustCreateAccount(s.T(), client, &service.Account{Name: "a1", Platform: service.PlatformAnthropic})
				mustCreateAccount(s.T(), client, &service.Account{Name: "a2", Platform: service.PlatformOpenAI})
			},
			platform:  service.PlatformOpenAI,
			wantCount: 1,
			validate: func(accounts []service.Account) {
				s.Require().Equal(service.PlatformOpenAI, accounts[0].Platform)
			},
		},
		{
			name: "filter_by_type",
			setup: func(client *dbent.Client) {
				mustCreateAccount(s.T(), client, &service.Account{Name: "t1", Type: service.AccountTypeOAuth})
				mustCreateAccount(s.T(), client, &service.Account{Name: "t2", Type: service.AccountTypeAPIKey})
			},
			accType:   service.AccountTypeAPIKey,
			wantCount: 1,
			validate: func(accounts []service.Account) {
				s.Require().Equal(service.AccountTypeAPIKey, accounts[0].Type)
			},
		},
		{
			name: "filter_by_status",
			setup: func(client *dbent.Client) {
				mustCreateAccount(s.T(), client, &service.Account{Name: "s1", Status: service.StatusActive})
				mustCreateAccount(s.T(), client, &service.Account{Name: "s2", Status: service.StatusDisabled})
			},
			status:    service.StatusDisabled,
			wantCount: 1,
			validate: func(accounts []service.Account) {
				s.Require().Equal(service.StatusDisabled, accounts[0].Status)
			},
		},
		{
			name: "filter_by_status_active_excludes_runtime_blocked_accounts",
			setup: func(client *dbent.Client) {
				mustCreateAccount(s.T(), client, &service.Account{Name: "active-normal", Status: service.StatusActive})
				rateLimited := mustCreateAccount(s.T(), client, &service.Account{Name: "active-rate-limited", Status: service.StatusActive})
				err := client.Account.UpdateOneID(rateLimited.ID).
					SetRateLimitResetAt(time.Now().Add(10 * time.Minute)).
					Exec(context.Background())
				s.Require().NoError(err)
				tempUnsched := mustCreateAccount(s.T(), client, &service.Account{Name: "active-temp-unsched", Status: service.StatusActive})
				err = client.Account.UpdateOneID(tempUnsched.ID).
					SetTempUnschedulableUntil(time.Now().Add(15 * time.Minute)).
					Exec(context.Background())
				s.Require().NoError(err)
				unsched := mustCreateAccount(s.T(), client, &service.Account{Name: "active-unsched", Status: service.StatusActive})
				err = client.Account.UpdateOneID(unsched.ID).
					SetSchedulable(false).
					Exec(context.Background())
				s.Require().NoError(err)
			},
			status:    service.StatusActive,
			wantCount: 1,
			validate: func(accounts []service.Account) {
				s.Require().Equal("active-normal", accounts[0].Name)
			},
		},
		{
			name: "filter_by_status_unschedulable_excludes_rate_limited_and_temp_unschedulable",
			setup: func(client *dbent.Client) {
				mustCreateAccount(s.T(), client, &service.Account{Name: "active-normal", Status: service.StatusActive, Schedulable: true})
				unsched := mustCreateAccount(s.T(), client, &service.Account{Name: "active-unsched", Status: service.StatusActive})
				err := client.Account.UpdateOneID(unsched.ID).
					SetSchedulable(false).
					Exec(context.Background())
				s.Require().NoError(err)
				rateLimited := mustCreateAccount(s.T(), client, &service.Account{Name: "active-rate-limited", Status: service.StatusActive})
				err = client.Account.UpdateOneID(rateLimited.ID).
					SetSchedulable(false).
					SetRateLimitResetAt(time.Now().Add(10 * time.Minute)).
					Exec(context.Background())
				s.Require().NoError(err)
				tempUnsched := mustCreateAccount(s.T(), client, &service.Account{Name: "active-temp-unsched", Status: service.StatusActive})
				err = client.Account.UpdateOneID(tempUnsched.ID).
					SetSchedulable(false).
					SetTempUnschedulableUntil(time.Now().Add(15 * time.Minute)).
					Exec(context.Background())
				s.Require().NoError(err)
			},
			status:    "unschedulable",
			wantCount: 1,
			validate: func(accounts []service.Account) {
				s.Require().Equal("active-unsched", accounts[0].Name)
			},
		},
		{
			name: "filter_by_status_rate_limited_excludes_temp_unschedulable",
			setup: func(client *dbent.Client) {
				rateLimited := mustCreateAccount(s.T(), client, &service.Account{Name: "active-rate-limited", Status: service.StatusActive})
				err := client.Account.UpdateOneID(rateLimited.ID).
					SetRateLimitResetAt(time.Now().Add(10 * time.Minute)).
					Exec(context.Background())
				s.Require().NoError(err)
				tempUnsched := mustCreateAccount(s.T(), client, &service.Account{Name: "active-temp-unsched", Status: service.StatusActive})
				err = client.Account.UpdateOneID(tempUnsched.ID).
					SetRateLimitResetAt(time.Now().Add(20 * time.Minute)).
					SetTempUnschedulableUntil(time.Now().Add(15 * time.Minute)).
					Exec(context.Background())
				s.Require().NoError(err)
			},
			status:    "rate_limited",
			wantCount: 1,
			validate: func(accounts []service.Account) {
				s.Require().Equal("active-rate-limited", accounts[0].Name)
			},
		},
		{
			name: "filter_by_status_temp_unschedulable_excludes_manually_unschedulable",
			setup: func(client *dbent.Client) {
				tempUnsched := mustCreateAccount(s.T(), client, &service.Account{Name: "active-temp-unsched", Status: service.StatusActive, Schedulable: true})
				err := client.Account.UpdateOneID(tempUnsched.ID).
					SetTempUnschedulableUntil(time.Now().Add(15 * time.Minute)).
					Exec(context.Background())
				s.Require().NoError(err)
				unsched := mustCreateAccount(s.T(), client, &service.Account{Name: "active-unsched", Status: service.StatusActive})
				err = client.Account.UpdateOneID(unsched.ID).
					SetSchedulable(false).
					Exec(context.Background())
				s.Require().NoError(err)
			},
			status:    "temp_unschedulable",
			wantCount: 1,
			validate: func(accounts []service.Account) {
				s.Require().Equal("active-temp-unsched", accounts[0].Name)
			},
		},
		{
			name: "filter_by_search",
			setup: func(client *dbent.Client) {
				mustCreateAccount(s.T(), client, &service.Account{Name: "alpha-account"})
				mustCreateAccount(s.T(), client, &service.Account{Name: "beta-account"})
			},
			search:    "alpha",
			wantCount: 1,
			validate: func(accounts []service.Account) {
				s.Require().Contains(accounts[0].Name, "alpha")
			},
		},
		{
			name: "filter_by_search_id_prefix",
			setup: func(client *dbent.Client) {
				target := mustCreateAccount(s.T(), client, &service.Account{Name: "non-matching-name"})
				mustCreateAccount(s.T(), client, &service.Account{Name: "id:" + strconv.FormatInt(target.ID, 10) + "-in-name"})
				searchFilter = "id:" + strconv.FormatInt(target.ID, 10)
			},
			resolveSearch: func(_ *dbent.Client) string {
				return searchFilter
			},
			wantCount: 1,
			validate: func(accounts []service.Account) {
				s.Require().Equal("non-matching-name", accounts[0].Name)
			},
		},
		{
			name: "filter_by_active_excludes_rate_limited_accounts",
			setup: func(client *dbent.Client) {
				futureReset := time.Now().Add(30 * time.Minute)
				mustCreateAccount(s.T(), client, &service.Account{Name: "normal-active", Status: service.StatusActive, Schedulable: true})
				mustCreateAccount(s.T(), client, &service.Account{Name: "rate-limited-active", Status: service.StatusActive, Schedulable: true, RateLimitResetAt: &futureReset})
			},
			status:    service.StatusActive,
			wantCount: 1,
			validate: func(accounts []service.Account) {
				s.Require().Equal("normal-active", accounts[0].Name)
			},
		},
		{
			name: "filter_by_ungrouped",
			setup: func(client *dbent.Client) {
				group := mustCreateGroup(s.T(), client, &service.Group{Name: "g-ungrouped"})
				grouped := mustCreateAccount(s.T(), client, &service.Account{Name: "grouped-account"})
				mustCreateAccount(s.T(), client, &service.Account{Name: "ungrouped-account"})
				mustBindAccountToGroup(s.T(), client, grouped.ID, group.ID, 1)
			},
			groupFilter: service.AccountListGroupUngroupedQueryValue,
			wantCount:   1,
			validate: func(accounts []service.Account) {
				s.Require().Equal("ungrouped-account", accounts[0].Name)
				s.Require().Empty(accounts[0].GroupIDs)
			},
		},
		{
			name: "filter_by_exact_single_group",
			setup: func(client *dbent.Client) {
				groupA := mustCreateGroup(s.T(), client, &service.Group{Name: "group-a"})
				groupB := mustCreateGroup(s.T(), client, &service.Group{Name: "group-b"})
				onlyA := mustCreateAccount(s.T(), client, &service.Account{Name: "only-a"})
				aAndB := mustCreateAccount(s.T(), client, &service.Account{Name: "a-and-b"})
				mustBindAccountToGroup(s.T(), client, onlyA.ID, groupA.ID, 1)
				mustBindAccountToGroup(s.T(), client, aAndB.ID, groupA.ID, 1)
				mustBindAccountToGroup(s.T(), client, aAndB.ID, groupB.ID, 1)
				singleGroupFilter = strconv.FormatInt(groupA.ID, 10)
			},
			groupExact: true,
			resolveGroupFilter: func(_ *dbent.Client) string {
				return singleGroupFilter
			},
			wantCount: 1,
			validate: func(accounts []service.Account) {
				s.Require().Equal("only-a", accounts[0].Name)
			},
		},
		{
			name: "filter_by_contains_multiple_groups",
			setup: func(client *dbent.Client) {
				groupA := mustCreateGroup(s.T(), client, &service.Group{Name: "group-a"})
				groupB := mustCreateGroup(s.T(), client, &service.Group{Name: "group-b"})
				groupC := mustCreateGroup(s.T(), client, &service.Group{Name: "group-c"})
				groupD := mustCreateGroup(s.T(), client, &service.Group{Name: "group-d"})
				exact := mustCreateAccount(s.T(), client, &service.Account{Name: "exact-abc"})
				extra := mustCreateAccount(s.T(), client, &service.Account{Name: "extra-abcd"})
				partial := mustCreateAccount(s.T(), client, &service.Account{Name: "partial-ab"})
				mustBindAccountToGroup(s.T(), client, exact.ID, groupA.ID, 1)
				mustBindAccountToGroup(s.T(), client, exact.ID, groupB.ID, 1)
				mustBindAccountToGroup(s.T(), client, exact.ID, groupC.ID, 1)
				mustBindAccountToGroup(s.T(), client, extra.ID, groupA.ID, 1)
				mustBindAccountToGroup(s.T(), client, extra.ID, groupB.ID, 1)
				mustBindAccountToGroup(s.T(), client, extra.ID, groupC.ID, 1)
				mustBindAccountToGroup(s.T(), client, extra.ID, groupD.ID, 1)
				mustBindAccountToGroup(s.T(), client, partial.ID, groupA.ID, 1)
				mustBindAccountToGroup(s.T(), client, partial.ID, groupB.ID, 1)
				multipleGroupFilter, _ = service.NormalizeAccountGroupFilter(strconv.FormatInt(groupC.ID, 10) + "," + strconv.FormatInt(groupA.ID, 10) + "," + strconv.FormatInt(groupB.ID, 10))
			},
			resolveGroupFilter: func(_ *dbent.Client) string {
				return multipleGroupFilter
			},
			wantCount: 3,
			validate: func(accounts []service.Account) {
				names := []string{accounts[0].Name, accounts[1].Name, accounts[2].Name}
				s.ElementsMatch([]string{"exact-abc", "extra-abcd", "partial-ab"}, names)
			},
		},
		{
			name: "filter_by_exact_multiple_groups",
			setup: func(client *dbent.Client) {
				groupA := mustCreateGroup(s.T(), client, &service.Group{Name: "group-a"})
				groupB := mustCreateGroup(s.T(), client, &service.Group{Name: "group-b"})
				groupC := mustCreateGroup(s.T(), client, &service.Group{Name: "group-c"})
				groupD := mustCreateGroup(s.T(), client, &service.Group{Name: "group-d"})
				exact := mustCreateAccount(s.T(), client, &service.Account{Name: "exact-abc"})
				extra := mustCreateAccount(s.T(), client, &service.Account{Name: "extra-abcd"})
				partial := mustCreateAccount(s.T(), client, &service.Account{Name: "partial-ab"})
				mustBindAccountToGroup(s.T(), client, exact.ID, groupA.ID, 1)
				mustBindAccountToGroup(s.T(), client, exact.ID, groupB.ID, 1)
				mustBindAccountToGroup(s.T(), client, exact.ID, groupC.ID, 1)
				mustBindAccountToGroup(s.T(), client, extra.ID, groupA.ID, 1)
				mustBindAccountToGroup(s.T(), client, extra.ID, groupB.ID, 1)
				mustBindAccountToGroup(s.T(), client, extra.ID, groupC.ID, 1)
				mustBindAccountToGroup(s.T(), client, extra.ID, groupD.ID, 1)
				mustBindAccountToGroup(s.T(), client, partial.ID, groupA.ID, 1)
				mustBindAccountToGroup(s.T(), client, partial.ID, groupB.ID, 1)
				multipleGroupFilter, _ = service.NormalizeAccountGroupFilter(strconv.FormatInt(groupC.ID, 10) + "," + strconv.FormatInt(groupA.ID, 10) + "," + strconv.FormatInt(groupB.ID, 10))
			},
			groupExact: true,
			resolveGroupFilter: func(_ *dbent.Client) string {
				return multipleGroupFilter
			},
			wantCount: 1,
			validate: func(accounts []service.Account) {
				s.Require().Equal("exact-abc", accounts[0].Name)
			},
		},
		{
			name: "filter_by_exact_with_excluded_groups",
			setup: func(client *dbent.Client) {
				groupA := mustCreateGroup(s.T(), client, &service.Group{Name: "group-a"})
				groupB := mustCreateGroup(s.T(), client, &service.Group{Name: "group-b"})
				groupC := mustCreateGroup(s.T(), client, &service.Group{Name: "group-c"})
				exactKeep := mustCreateAccount(s.T(), client, &service.Account{Name: "exact-keep"})
				exactBlocked := mustCreateAccount(s.T(), client, &service.Account{Name: "exact-blocked"})
				mustBindAccountToGroup(s.T(), client, exactKeep.ID, groupA.ID, 1)
				mustBindAccountToGroup(s.T(), client, exactKeep.ID, groupB.ID, 1)
				mustBindAccountToGroup(s.T(), client, exactBlocked.ID, groupA.ID, 1)
				mustBindAccountToGroup(s.T(), client, exactBlocked.ID, groupB.ID, 1)
				mustBindAccountToGroup(s.T(), client, exactBlocked.ID, groupC.ID, 1)
				exactExcludeGroupFilter, _ = service.NormalizeAccountGroupFilter(strconv.FormatInt(groupA.ID, 10) + "," + strconv.FormatInt(groupB.ID, 10))
				excludeGroupFilter, _ = service.NormalizeAccountGroupExcludeFilter(strconv.FormatInt(groupB.ID, 10))
			},
			groupExact: true,
			resolveGroupFilter: func(_ *dbent.Client) string {
				return exactExcludeGroupFilter
			},
			resolveGroupExcludeFilter: func(_ *dbent.Client) string {
				return excludeGroupFilter
			},
			wantCount: 0,
		},
		{
			name: "filter_by_excluded_groups_only",
			setup: func(client *dbent.Client) {
				groupA := mustCreateGroup(s.T(), client, &service.Group{Name: "group-a"})
				groupB := mustCreateGroup(s.T(), client, &service.Group{Name: "group-b"})
				onlyA := mustCreateAccount(s.T(), client, &service.Account{Name: "only-a"})
				onlyB := mustCreateAccount(s.T(), client, &service.Account{Name: "only-b"})
				aAndB := mustCreateAccount(s.T(), client, &service.Account{Name: "a-and-b"})
				mustCreateAccount(s.T(), client, &service.Account{Name: "ungrouped"})
				mustBindAccountToGroup(s.T(), client, onlyA.ID, groupA.ID, 1)
				mustBindAccountToGroup(s.T(), client, onlyB.ID, groupB.ID, 1)
				mustBindAccountToGroup(s.T(), client, aAndB.ID, groupA.ID, 1)
				mustBindAccountToGroup(s.T(), client, aAndB.ID, groupB.ID, 1)
				excludeGroupFilter, _ = service.NormalizeAccountGroupExcludeFilter(strconv.FormatInt(groupB.ID, 10))
			},
			resolveGroupExcludeFilter: func(_ *dbent.Client) string {
				return excludeGroupFilter
			},
			wantCount: 2,
			validate: func(accounts []service.Account) {
				names := []string{accounts[0].Name, accounts[1].Name}
				s.ElementsMatch([]string{"only-a", "ungrouped"}, names)
			},
		},
		{
			name: "filter_by_contains_with_excluded_groups",
			setup: func(client *dbent.Client) {
				groupA := mustCreateGroup(s.T(), client, &service.Group{Name: "group-a"})
				groupB := mustCreateGroup(s.T(), client, &service.Group{Name: "group-b"})
				groupC := mustCreateGroup(s.T(), client, &service.Group{Name: "group-c"})
				onlyA := mustCreateAccount(s.T(), client, &service.Account{Name: "only-a"})
				aAndB := mustCreateAccount(s.T(), client, &service.Account{Name: "a-and-b"})
				aAndC := mustCreateAccount(s.T(), client, &service.Account{Name: "a-and-c"})
				mustBindAccountToGroup(s.T(), client, onlyA.ID, groupA.ID, 1)
				mustBindAccountToGroup(s.T(), client, aAndB.ID, groupA.ID, 1)
				mustBindAccountToGroup(s.T(), client, aAndB.ID, groupB.ID, 1)
				mustBindAccountToGroup(s.T(), client, aAndC.ID, groupA.ID, 1)
				mustBindAccountToGroup(s.T(), client, aAndC.ID, groupC.ID, 1)
				singleGroupFilter = strconv.FormatInt(groupA.ID, 10)
				excludeGroupFilter, _ = service.NormalizeAccountGroupExcludeFilter(strconv.FormatInt(groupB.ID, 10))
			},
			resolveGroupFilter: func(_ *dbent.Client) string {
				return singleGroupFilter
			},
			resolveGroupExcludeFilter: func(_ *dbent.Client) string {
				return excludeGroupFilter
			},
			wantCount: 2,
			validate: func(accounts []service.Account) {
				names := []string{accounts[0].Name, accounts[1].Name}
				s.ElementsMatch([]string{"only-a", "a-and-c"}, names)
			},
		},
		{
			name: "filter_by_privacy_mode",
			setup: func(client *dbent.Client) {
				mustCreateAccount(s.T(), client, &service.Account{Name: "privacy-ok", Extra: map[string]any{"privacy_mode": service.PrivacyModeTrainingOff}})
				mustCreateAccount(s.T(), client, &service.Account{Name: "privacy-fail", Extra: map[string]any{"privacy_mode": service.PrivacyModeFailed}})
			},
			privacyMode: service.PrivacyModeTrainingOff,
			wantCount:   1,
			validate: func(accounts []service.Account) {
				s.Require().Equal("privacy-ok", accounts[0].Name)
			},
		},
		{
			name: "filter_by_privacy_mode_unset",
			setup: func(client *dbent.Client) {
				mustCreateAccount(s.T(), client, &service.Account{Name: "privacy-unset", Extra: nil})
				mustCreateAccount(s.T(), client, &service.Account{Name: "privacy-empty", Extra: map[string]any{"privacy_mode": ""}})
				mustCreateAccount(s.T(), client, &service.Account{Name: "privacy-set", Extra: map[string]any{"privacy_mode": service.PrivacyModeTrainingOff}})
			},
			privacyMode: service.AccountPrivacyModeUnsetFilter,
			wantCount:   2,
			validate: func(accounts []service.Account) {
				names := []string{accounts[0].Name, accounts[1].Name}
				s.ElementsMatch([]string{"privacy-unset", "privacy-empty"}, names)
			},
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			// 每个 case 重新获取隔离资源
			tx := testEntTx(s.T())
			client := tx.Client()
			repo := newAccountRepositoryWithSQL(client, tx, nil)
			ctx := context.Background()

			tt.setup(client)
			groupFilter := tt.groupFilter
			if tt.resolveGroupFilter != nil {
				groupFilter = tt.resolveGroupFilter(client)
			}
			groupExcludeFilter := tt.groupExcludeFilter
			if tt.resolveGroupExcludeFilter != nil {
				groupExcludeFilter = tt.resolveGroupExcludeFilter(client)
			}
			groupExact := tt.groupExact
			search := tt.search
			if tt.resolveSearch != nil {
				search = tt.resolveSearch(client)
			}

			accounts, _, err := repo.ListWithFilters(ctx, pagination.PaginationParams{Page: 1, PageSize: 10, SortBy: "id", SortOrder: "desc"}, service.AccountListFilters{
				Platform:        tt.platform,
				AccountType:     tt.accType,
				Status:          tt.status,
				Search:          search,
				GroupIDs:        groupFilter,
				GroupExcludeIDs: groupExcludeFilter,
				GroupExact:      groupExact,
				PrivacyMode:     tt.privacyMode,
			})
			s.Require().NoError(err)
			s.Require().Len(accounts, tt.wantCount)
			if tt.validate != nil {
				tt.validate(accounts)
			}
		})
	}
}

// --- ListByGroup / ListActive / ListByPlatform ---

func (s *AccountRepoSuite) TestListWithFilters_MultiSort() {
	accA := mustCreateAccount(s.T(), s.client, &service.Account{Name: "sort-a", Status: service.StatusActive})
	accB := mustCreateAccount(s.T(), s.client, &service.Account{Name: "sort-b", Status: service.StatusActive})
	accC := mustCreateAccount(s.T(), s.client, &service.Account{Name: "sort-c", Status: service.StatusActive})

	recent := time.Now().Add(-5 * time.Minute)
	older := recent.Add(-10 * time.Minute)
	s.Require().NoError(s.client.Account.UpdateOneID(accA.ID).SetLastUsedAt(recent).Exec(s.ctx))
	s.Require().NoError(s.client.Account.UpdateOneID(accB.ID).SetLastUsedAt(recent).Exec(s.ctx))
	s.Require().NoError(s.client.Account.UpdateOneID(accC.ID).SetLastUsedAt(older).Exec(s.ctx))

	accounts, _, err := s.repo.ListWithFilters(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10, SortBy: "last_used_at,id", SortOrder: "desc,asc"}, service.AccountListFilters{Search: "sort-"})
	s.Require().NoError(err)
	s.Require().Len(accounts, 3)
	s.Require().Equal([]int64{accA.ID, accB.ID, accC.ID}, []int64{accounts[0].ID, accounts[1].ID, accounts[2].ID})
}

func (s *AccountRepoSuite) TestListWithFilters_LastUsedFilters() {
	unused := mustCreateAccount(s.T(), s.client, &service.Account{Name: "last-used-unused", Status: service.StatusActive})
	inRange := mustCreateAccount(s.T(), s.client, &service.Account{Name: "last-used-in-range", Status: service.StatusActive})
	outOfRange := mustCreateAccount(s.T(), s.client, &service.Account{Name: "last-used-out-of-range", Status: service.StatusActive})

	now := time.Now().UTC().Truncate(time.Second)
	start := now.Add(-30 * time.Minute)
	end := now.Add(-5 * time.Minute)
	inRangeAt := now.Add(-10 * time.Minute)
	outOfRangeAt := now.Add(-2 * time.Hour)
	inRangeID := inRange.ID
	outOfRangeID := outOfRange.ID
	unusedID := unused.ID

	s.Require().NoError(s.client.Account.UpdateOneID(inRangeID).SetLastUsedAt(inRangeAt).Exec(s.ctx))
	s.Require().NoError(s.client.Account.UpdateOneID(outOfRangeID).SetLastUsedAt(outOfRangeAt).Exec(s.ctx))

	unusedAccounts, _, err := s.repo.ListWithFilters(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10, SortBy: "id", SortOrder: "asc"}, service.AccountListFilters{
		Search:         "last-used-",
		LastUsedFilter: "unused",
	})
	s.Require().NoError(err)
	s.Require().Len(unusedAccounts, 1)
	s.Require().Equal(unusedID, unusedAccounts[0].ID)

	rangeAccounts, _, err := s.repo.ListWithFilters(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10, SortBy: "id", SortOrder: "asc"}, service.AccountListFilters{
		Search:         "last-used-",
		LastUsedFilter: "range",
		LastUsedStart:  &start,
		LastUsedEnd:    &end,
	})
	s.Require().NoError(err)
	s.Require().Len(rangeAccounts, 1)
	s.Require().Equal(inRangeID, rangeAccounts[0].ID)
}

func (s *AccountRepoSuite) TestListByGroup() {
	group := mustCreateGroup(s.T(), s.client, &service.Group{Name: "g-list"})
	acc1 := mustCreateAccount(s.T(), s.client, &service.Account{Name: "a1", Status: service.StatusActive})
	acc2 := mustCreateAccount(s.T(), s.client, &service.Account{Name: "a2", Status: service.StatusActive})
	mustBindAccountToGroup(s.T(), s.client, acc1.ID, group.ID, 2)
	mustBindAccountToGroup(s.T(), s.client, acc2.ID, group.ID, 1)

	accounts, err := s.repo.ListByGroup(s.ctx, group.ID)
	s.Require().NoError(err, "ListByGroup")
	s.Require().Len(accounts, 2)
	// Should be ordered by priority
	s.Require().Equal(acc2.ID, accounts[0].ID, "expected acc2 first (priority=1)")
}

func (s *AccountRepoSuite) TestListActive() {
	mustCreateAccount(s.T(), s.client, &service.Account{Name: "active1", Status: service.StatusActive})
	mustCreateAccount(s.T(), s.client, &service.Account{Name: "inactive1", Status: service.StatusDisabled})

	accounts, err := s.repo.ListActive(s.ctx)
	s.Require().NoError(err, "ListActive")
	s.Require().Len(accounts, 1)
	s.Require().Equal("active1", accounts[0].Name)
}

func (s *AccountRepoSuite) TestListOpsRealtimeAccounts_FiltersAndReturnsLightweightProjection() {
	proxy := mustCreateProxy(s.T(), s.client, &service.Proxy{Name: "ops-proxy"})
	targetGroup := mustCreateGroup(s.T(), s.client, &service.Group{Name: "ops-target-group"})
	otherGroup := mustCreateGroup(s.T(), s.client, &service.Group{Name: "ops-other-group"})

	rateLimitResetAt := time.Now().UTC().Add(20 * time.Minute).Truncate(time.Second)
	overloadUntil := rateLimitResetAt.Add(5 * time.Minute)
	tempUnschedulableUntil := rateLimitResetAt.Add(10 * time.Minute)

	matchingOlder := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:             "ops-match-older",
		Platform:         service.PlatformOpenAI,
		ProxyID:          &proxy.ID,
		Concurrency:      7,
		Status:           service.StatusError,
		ErrorMessage:     "ops-error",
		RateLimitResetAt: &rateLimitResetAt,
		OverloadUntil:    &overloadUntil,
		Credentials:      map[string]any{"refresh_token": "secret"},
		Extra:            map[string]any{"privacy_mode": service.PrivacyModeTrainingOff},
	})
	s.Require().NoError(s.client.Account.UpdateOneID(matchingOlder.ID).SetTempUnschedulableUntil(tempUnschedulableUntil).Exec(s.ctx))
	mustBindAccountToGroup(s.T(), s.client, matchingOlder.ID, targetGroup.ID, 1)

	matchingNewer := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:        "ops-match-newer",
		Platform:    service.PlatformOpenAI,
		Concurrency: 5,
		Priority:    10,
	})
	mustBindAccountToGroup(s.T(), s.client, matchingNewer.ID, targetGroup.ID, 2)

	wrongPlatform := mustCreateAccount(s.T(), s.client, &service.Account{Name: "ops-wrong-platform", Platform: service.PlatformAnthropic})
	mustBindAccountToGroup(s.T(), s.client, wrongPlatform.ID, targetGroup.ID, 1)

	wrongGroup := mustCreateAccount(s.T(), s.client, &service.Account{Name: "ops-wrong-group", Platform: service.PlatformOpenAI})
	mustBindAccountToGroup(s.T(), s.client, wrongGroup.ID, otherGroup.ID, 1)

	targetGroupID := targetGroup.ID
	accounts, err := s.repo.ListOpsRealtimeAccounts(s.ctx, service.PlatformOpenAI, &targetGroupID)
	s.Require().NoError(err)
	s.Require().Equal([]int64{matchingNewer.ID, matchingOlder.ID}, idsOfAccounts(accounts))

	byID := make(map[int64]service.Account, len(accounts))
	for _, account := range accounts {
		byID[account.ID] = account
	}

	got := byID[matchingOlder.ID]
	s.Require().Equal("ops-match-older", got.Name)
	s.Require().Equal(service.PlatformOpenAI, got.Platform)
	s.Require().Equal(7, got.Concurrency)
	s.Require().Equal(service.StatusError, got.Status)
	s.Require().Equal("ops-error", got.ErrorMessage)
	s.Require().NotNil(got.RateLimitResetAt)
	s.Require().WithinDuration(rateLimitResetAt, *got.RateLimitResetAt, time.Second)
	s.Require().NotNil(got.OverloadUntil)
	s.Require().WithinDuration(overloadUntil, *got.OverloadUntil, time.Second)
	s.Require().NotNil(got.TempUnschedulableUntil)
	s.Require().WithinDuration(tempUnschedulableUntil, *got.TempUnschedulableUntil, time.Second)
	s.Require().Equal([]int64{targetGroup.ID}, got.GroupIDs)
	s.Require().Len(got.Groups, 1)
	s.Require().Equal(targetGroup.ID, got.Groups[0].ID)
	s.Require().Nil(got.Proxy)
	s.Require().Nil(got.ProxyID)
	s.Require().Nil(got.Credentials)
	s.Require().Nil(got.Extra)
	s.Require().Empty(got.AccountGroups)
	s.Require().Nil(got.RateMultiplier)
	s.Require().Empty(got.Type)
	_, hasWrongPlatform := byID[wrongPlatform.ID]
	s.Require().False(hasWrongPlatform)
	_, hasWrongGroup := byID[wrongGroup.ID]
	s.Require().False(hasWrongGroup)
}

func (s *AccountRepoSuite) TestListActiveForTokenRefresh_ReturnsCoreFieldsWithoutRelationHydration() {
	proxy := mustCreateProxy(s.T(), s.client, &service.Proxy{Name: "refresh-proxy"})
	group := mustCreateGroup(s.T(), s.client, &service.Group{Name: "refresh-group"})

	refreshable := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:        "refreshable",
		Platform:    service.PlatformOpenAI,
		ProxyID:     &proxy.ID,
		Priority:    10,
		Credentials: map[string]any{"refresh_token": "rt-1"},
		Extra:       map[string]any{"privacy_mode": service.PrivacyModeTrainingOff},
	})
	mustBindAccountToGroup(s.T(), s.client, refreshable.ID, group.ID, 1)

	second := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:     "refreshable-second",
		Platform: service.PlatformAnthropic,
		Priority: 20,
	})
	mustCreateAccount(s.T(), s.client, &service.Account{Name: "disabled-refresh", Status: service.StatusDisabled, Priority: 5})

	accounts, err := s.repo.ListActiveForTokenRefresh(s.ctx)
	s.Require().NoError(err)
	s.Require().Equal([]int64{refreshable.ID, second.ID}, idsOfAccounts(accounts))

	got := accounts[0]
	s.Require().Equal(refreshable.ID, got.ID)
	s.Require().NotNil(got.ProxyID)
	s.Require().Equal(proxy.ID, *got.ProxyID)
	s.Require().Equal("rt-1", got.Credentials["refresh_token"])
	s.Require().Equal(service.PrivacyModeTrainingOff, got.Extra["privacy_mode"])
	s.Require().Nil(got.Proxy)
	s.Require().Empty(got.Groups)
	s.Require().Empty(got.GroupIDs)
	s.Require().Empty(got.AccountGroups)
}

func (s *AccountRepoSuite) TestListByPlatform() {
	mustCreateAccount(s.T(), s.client, &service.Account{Name: "p1", Platform: service.PlatformAnthropic, Status: service.StatusActive})
	mustCreateAccount(s.T(), s.client, &service.Account{Name: "p2", Platform: service.PlatformOpenAI, Status: service.StatusActive})

	accounts, err := s.repo.ListByPlatform(s.ctx, service.PlatformAnthropic)
	s.Require().NoError(err, "ListByPlatform")
	s.Require().Len(accounts, 1)
	s.Require().Equal(service.PlatformAnthropic, accounts[0].Platform)
}

// --- Preload and VirtualFields ---

func (s *AccountRepoSuite) TestPreload_And_VirtualFields() {
	proxy := mustCreateProxy(s.T(), s.client, &service.Proxy{Name: "p1"})
	group := mustCreateGroup(s.T(), s.client, &service.Group{Name: "g1"})

	account := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:    "acc1",
		ProxyID: &proxy.ID,
	})
	mustBindAccountToGroup(s.T(), s.client, account.ID, group.ID, 1)

	got, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err, "GetByID")
	s.Require().NotNil(got.Proxy, "expected Proxy preload")
	s.Require().Equal(proxy.ID, got.Proxy.ID)
	s.Require().Len(got.GroupIDs, 1, "expected GroupIDs to be populated")
	s.Require().Equal(group.ID, got.GroupIDs[0])
	s.Require().Len(got.Groups, 1, "expected Groups to be populated")
	s.Require().Equal(group.ID, got.Groups[0].ID)

	accounts, page, err := s.repo.ListWithFilters(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 10, SortBy: "id", SortOrder: "desc"}, service.AccountListFilters{Search: "acc"})
	s.Require().NoError(err, "ListWithFilters")
	s.Require().Equal(int64(1), page.Total)
	s.Require().Len(accounts, 1)
	s.Require().NotNil(accounts[0].Proxy, "expected Proxy preload in list")
	s.Require().Equal(proxy.ID, accounts[0].Proxy.ID)
	s.Require().Len(accounts[0].GroupIDs, 1, "expected GroupIDs in list")
	s.Require().Equal(group.ID, accounts[0].GroupIDs[0])
}

// --- GroupBinding / AddToGroup / RemoveFromGroup / BindGroups / GetGroups ---

func (s *AccountRepoSuite) TestGroupBinding_And_BindGroups() {
	g1 := mustCreateGroup(s.T(), s.client, &service.Group{Name: "g1"})
	g2 := mustCreateGroup(s.T(), s.client, &service.Group{Name: "g2"})
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc"})

	s.Require().NoError(s.repo.AddToGroup(s.ctx, account.ID, g1.ID, 10), "AddToGroup")
	groups, err := s.repo.GetGroups(s.ctx, account.ID)
	s.Require().NoError(err, "GetGroups")
	s.Require().Len(groups, 1, "expected 1 group")
	s.Require().Equal(g1.ID, groups[0].ID)

	s.Require().NoError(s.repo.RemoveFromGroup(s.ctx, account.ID, g1.ID), "RemoveFromGroup")
	groups, err = s.repo.GetGroups(s.ctx, account.ID)
	s.Require().NoError(err, "GetGroups after remove")
	s.Require().Empty(groups, "expected 0 groups after remove")

	s.Require().NoError(s.repo.BindGroups(s.ctx, account.ID, []int64{g1.ID, g2.ID}), "BindGroups")
	groups, err = s.repo.GetGroups(s.ctx, account.ID)
	s.Require().NoError(err, "GetGroups after bind")
	s.Require().Len(groups, 2, "expected 2 groups after bind")
}

func (s *AccountRepoSuite) TestBindGroups_EmptyList() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-empty"})
	group := mustCreateGroup(s.T(), s.client, &service.Group{Name: "g-empty"})
	mustBindAccountToGroup(s.T(), s.client, account.ID, group.ID, 1)

	s.Require().NoError(s.repo.BindGroups(s.ctx, account.ID, []int64{}), "BindGroups empty")

	groups, err := s.repo.GetGroups(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().Empty(groups, "expected 0 groups after binding empty list")
}

func (s *AccountRepoSuite) TestPreviewAndResolveBulkUpdateTargets() {
	g1 := mustCreateGroup(s.T(), s.client, &service.Group{Name: "preview-g1"})
	a1 := mustCreateAccount(s.T(), s.client, &service.Account{Name: "preview-a1", Platform: service.PlatformAnthropic, Type: service.AccountTypeOAuth})
	a2 := mustCreateAccount(s.T(), s.client, &service.Account{Name: "preview-a2", Platform: service.PlatformAnthropic, Type: service.AccountTypeAPIKey})
	mustCreateAccount(s.T(), s.client, &service.Account{Name: "preview-a3", Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth})
	mustBindAccountToGroup(s.T(), s.client, a1.ID, g1.ID, 1)
	mustBindAccountToGroup(s.T(), s.client, a2.ID, g1.ID, 1)

	filter := service.AccountBulkFilter{
		Platform: service.PlatformAnthropic,
		GroupIDs: strconv.FormatInt(g1.ID, 10),
	}
	preview, err := s.repo.PreviewBulkUpdateTargets(s.ctx, filter)
	s.Require().NoError(err)
	s.Require().Equal(int64(2), preview.Count)
	s.Require().Equal([]string{service.PlatformAnthropic}, preview.Platforms)
	s.Require().Equal([]string{service.AccountTypeAPIKey, service.AccountTypeOAuth}, preview.Types)

	resolved, err := s.repo.ResolveBulkUpdateTargets(s.ctx, filter)
	s.Require().NoError(err)
	s.Require().Len(resolved, 2)
	resolvedIDs := make([]int64, 0, len(resolved))
	for _, target := range resolved {
		resolvedIDs = append(resolvedIDs, target.ID)
		s.Require().Equal(service.PlatformAnthropic, target.Platform)
	}
	s.Require().ElementsMatch([]int64{a1.ID, a2.ID}, resolvedIDs)
}

func (s *AccountRepoSuite) TestPreviewAndResolveBulkUpdateTargets_EmptyResult() {
	mustCreateAccount(s.T(), s.client, &service.Account{Name: "bulk-preview-existing", Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth})

	filter := service.AccountBulkFilter{Search: "bulk-preview-missing"}
	preview, err := s.repo.PreviewBulkUpdateTargets(s.ctx, filter)
	s.Require().NoError(err)
	s.Require().NotNil(preview)
	s.Require().Zero(preview.Count)
	s.Require().Empty(preview.Platforms)
	s.Require().Empty(preview.Types)

	resolved, err := s.repo.ResolveBulkUpdateTargets(s.ctx, filter)
	s.Require().NoError(err)
	s.Require().Empty(resolved)
}

// --- Schedulable ---

func (s *AccountRepoSuite) TestListSchedulable() {
	now := time.Now()
	group := mustCreateGroup(s.T(), s.client, &service.Group{Name: "g-sched"})

	okAcc := mustCreateAccount(s.T(), s.client, &service.Account{Name: "ok", Schedulable: true})
	mustBindAccountToGroup(s.T(), s.client, okAcc.ID, group.ID, 1)

	future := now.Add(10 * time.Minute)
	overloaded := mustCreateAccount(s.T(), s.client, &service.Account{Name: "over", Schedulable: true, OverloadUntil: &future})
	mustBindAccountToGroup(s.T(), s.client, overloaded.ID, group.ID, 1)

	sched, err := s.repo.ListSchedulable(s.ctx)
	s.Require().NoError(err, "ListSchedulable")
	ids := idsOfAccounts(sched)
	s.Require().Contains(ids, okAcc.ID)
	s.Require().NotContains(ids, overloaded.ID)
}

func (s *AccountRepoSuite) TestListSchedulableByGroupID_TimeBoundaries_And_StatusUpdates() {
	now := time.Now()
	group := mustCreateGroup(s.T(), s.client, &service.Group{Name: "g-sched"})

	okAcc := mustCreateAccount(s.T(), s.client, &service.Account{Name: "ok", Schedulable: true})
	mustBindAccountToGroup(s.T(), s.client, okAcc.ID, group.ID, 1)

	future := now.Add(10 * time.Minute)
	overloaded := mustCreateAccount(s.T(), s.client, &service.Account{Name: "over", Schedulable: true, OverloadUntil: &future})
	mustBindAccountToGroup(s.T(), s.client, overloaded.ID, group.ID, 1)

	rateLimited := mustCreateAccount(s.T(), s.client, &service.Account{Name: "rl", Schedulable: true})
	mustBindAccountToGroup(s.T(), s.client, rateLimited.ID, group.ID, 1)
	s.Require().NoError(s.repo.SetRateLimited(s.ctx, rateLimited.ID, now.Add(10*time.Minute)), "SetRateLimited")

	s.Require().NoError(s.repo.SetError(s.ctx, overloaded.ID, "boom"), "SetError")

	sched, err := s.repo.ListSchedulableByGroupID(s.ctx, group.ID)
	s.Require().NoError(err, "ListSchedulableByGroupID")
	s.Require().Len(sched, 1, "expected only ok account schedulable")
	s.Require().Equal(okAcc.ID, sched[0].ID)

	s.Require().NoError(s.repo.ClearRateLimit(s.ctx, rateLimited.ID), "ClearRateLimit")
	sched2, err := s.repo.ListSchedulableByGroupID(s.ctx, group.ID)
	s.Require().NoError(err, "ListSchedulableByGroupID after ClearRateLimit")
	s.Require().Len(sched2, 2, "expected 2 schedulable accounts after ClearRateLimit")
}

func (s *AccountRepoSuite) TestListSchedulableByPlatform() {
	mustCreateAccount(s.T(), s.client, &service.Account{Name: "a1", Platform: service.PlatformAnthropic, Schedulable: true})
	mustCreateAccount(s.T(), s.client, &service.Account{Name: "a2", Platform: service.PlatformOpenAI, Schedulable: true})

	accounts, err := s.repo.ListSchedulableByPlatform(s.ctx, service.PlatformAnthropic)
	s.Require().NoError(err)
	s.Require().Len(accounts, 1)
	s.Require().Equal(service.PlatformAnthropic, accounts[0].Platform)
}

func (s *AccountRepoSuite) TestListSchedulableByGroupIDAndPlatform() {
	group := mustCreateGroup(s.T(), s.client, &service.Group{Name: "g-sp"})
	a1 := mustCreateAccount(s.T(), s.client, &service.Account{Name: "a1", Platform: service.PlatformAnthropic, Schedulable: true})
	a2 := mustCreateAccount(s.T(), s.client, &service.Account{Name: "a2", Platform: service.PlatformOpenAI, Schedulable: true})
	mustBindAccountToGroup(s.T(), s.client, a1.ID, group.ID, 1)
	mustBindAccountToGroup(s.T(), s.client, a2.ID, group.ID, 2)

	accounts, err := s.repo.ListSchedulableByGroupIDAndPlatform(s.ctx, group.ID, service.PlatformAnthropic)
	s.Require().NoError(err)
	s.Require().Len(accounts, 1)
	s.Require().Equal(a1.ID, accounts[0].ID)
}

func (s *AccountRepoSuite) TestListSchedulableByGroupIDAndPlatform_PreservesOrderedIDs() {
	group := mustCreateGroup(s.T(), s.client, &service.Group{Name: "g-sp-ordered"})
	a1 := mustCreateAccount(s.T(), s.client, &service.Account{Name: "a1", Platform: service.PlatformAnthropic, Schedulable: true, Priority: 50})
	a2 := mustCreateAccount(s.T(), s.client, &service.Account{Name: "a2", Platform: service.PlatformAnthropic, Schedulable: true, Priority: 10})
	a3 := mustCreateAccount(s.T(), s.client, &service.Account{Name: "a3", Platform: service.PlatformAnthropic, Schedulable: true, Priority: 1})
	mustBindAccountToGroup(s.T(), s.client, a1.ID, group.ID, 2)
	mustBindAccountToGroup(s.T(), s.client, a2.ID, group.ID, 1)
	mustBindAccountToGroup(s.T(), s.client, a3.ID, group.ID, 1)

	accounts, err := s.repo.ListSchedulableByGroupIDAndPlatform(s.ctx, group.ID, service.PlatformAnthropic)
	s.Require().NoError(err)
	s.Require().Len(accounts, 3)
	s.Require().Equal([]int64{a3.ID, a2.ID, a1.ID}, []int64{accounts[0].ID, accounts[1].ID, accounts[2].ID})
}

func (s *AccountRepoSuite) TestSetSchedulable() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-sched", Schedulable: true})
	cacheRecorder := &schedulerCacheRecorder{}
	s.repo.schedulerCache = cacheRecorder

	s.Require().NoError(s.repo.SetSchedulable(s.ctx, account.ID, false))

	got, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().False(got.Schedulable)
	s.Require().Len(cacheRecorder.setAccounts, 1)
	s.Require().Equal(account.ID, cacheRecorder.setAccounts[0].ID)
}

func (s *AccountRepoSuite) TestBulkUpdate_SyncSchedulerSnapshotOnDisabled() {
	account1 := mustCreateAccount(s.T(), s.client, &service.Account{Name: "bulk-1", Status: service.StatusActive, Schedulable: true})
	account2 := mustCreateAccount(s.T(), s.client, &service.Account{Name: "bulk-2", Status: service.StatusActive, Schedulable: true})
	cacheRecorder := &schedulerCacheRecorder{}
	s.repo.schedulerCache = cacheRecorder

	disabled := service.StatusDisabled
	rows, err := s.repo.BulkUpdate(s.ctx, []int64{account1.ID, account2.ID}, service.AccountBulkUpdate{
		Status: &disabled,
	})
	s.Require().NoError(err)
	s.Require().Equal(int64(2), rows)

	s.Require().Len(cacheRecorder.setAccounts, 2)
	ids := map[int64]struct{}{}
	for _, acc := range cacheRecorder.setAccounts {
		ids[acc.ID] = struct{}{}
	}
	s.Require().Contains(ids, account1.ID)
	s.Require().Contains(ids, account2.ID)
}

// --- SetOverloaded / SetRateLimited / ClearRateLimit ---

func (s *AccountRepoSuite) TestSetOverloaded() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-over"})
	until := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)

	s.Require().NoError(s.repo.SetOverloaded(s.ctx, account.ID, until))

	got, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().NotNil(got.OverloadUntil)
	s.Require().WithinDuration(until, *got.OverloadUntil, time.Second)
}

func (s *AccountRepoSuite) TestSetRateLimited() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-rl"})
	resetAt := time.Date(2025, 6, 15, 14, 0, 0, 0, time.UTC)

	s.Require().NoError(s.repo.SetRateLimited(s.ctx, account.ID, resetAt))

	got, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().NotNil(got.RateLimitedAt)
	s.Require().NotNil(got.RateLimitResetAt)
	s.Require().WithinDuration(resetAt, *got.RateLimitResetAt, time.Second)
}

func (s *AccountRepoSuite) TestClearRateLimit() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-clear"})
	until := time.Now().Add(1 * time.Hour)
	s.Require().NoError(s.repo.SetOverloaded(s.ctx, account.ID, until))
	s.Require().NoError(s.repo.SetRateLimited(s.ctx, account.ID, until))

	s.Require().NoError(s.repo.ClearRateLimit(s.ctx, account.ID))

	got, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().Nil(got.RateLimitedAt)
	s.Require().Nil(got.RateLimitResetAt)
	s.Require().Nil(got.OverloadUntil)
}

func (s *AccountRepoSuite) TestTempUnschedulableFieldsLoadedByGetByIDAndGetByIDs() {
	acc1 := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-temp-1"})
	acc2 := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-temp-2"})

	until := time.Now().Add(15 * time.Minute).UTC().Truncate(time.Second)
	reason := `{"rule":"429","matched_keyword":"too many requests"}`
	s.Require().NoError(s.repo.SetTempUnschedulable(s.ctx, acc1.ID, until, reason))

	gotByID, err := s.repo.GetByID(s.ctx, acc1.ID)
	s.Require().NoError(err)
	s.Require().NotNil(gotByID.TempUnschedulableUntil)
	s.Require().WithinDuration(until, *gotByID.TempUnschedulableUntil, time.Second)
	s.Require().Equal(reason, gotByID.TempUnschedulableReason)

	gotByIDs, err := s.repo.GetByIDs(s.ctx, []int64{acc2.ID, acc1.ID})
	s.Require().NoError(err)
	s.Require().Len(gotByIDs, 2)
	s.Require().Equal(acc2.ID, gotByIDs[0].ID)
	s.Require().Nil(gotByIDs[0].TempUnschedulableUntil)
	s.Require().Equal("", gotByIDs[0].TempUnschedulableReason)
	s.Require().Equal(acc1.ID, gotByIDs[1].ID)
	s.Require().NotNil(gotByIDs[1].TempUnschedulableUntil)
	s.Require().WithinDuration(until, *gotByIDs[1].TempUnschedulableUntil, time.Second)
	s.Require().Equal(reason, gotByIDs[1].TempUnschedulableReason)

	s.Require().NoError(s.repo.ClearTempUnschedulable(s.ctx, acc1.ID))
	cleared, err := s.repo.GetByID(s.ctx, acc1.ID)
	s.Require().NoError(err)
	s.Require().Nil(cleared.TempUnschedulableUntil)
	s.Require().Equal("", cleared.TempUnschedulableReason)
}

// --- UpdateLastUsed ---

func (s *AccountRepoSuite) TestUpdateLastUsed() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-used"})
	s.Require().Nil(account.LastUsedAt)

	s.Require().NoError(s.repo.UpdateLastUsed(s.ctx, account.ID))

	got, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().NotNil(got.LastUsedAt)
}

// --- SetError ---

func (s *AccountRepoSuite) TestSetError() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-err", Status: service.StatusActive})

	s.Require().NoError(s.repo.SetError(s.ctx, account.ID, "something went wrong"))

	got, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().Equal(service.StatusError, got.Status)
	s.Require().Equal("something went wrong", got.ErrorMessage)
}

func (s *AccountRepoSuite) TestClearError_SyncSchedulerSnapshotOnRecovery() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:         "acc-clear-err",
		Status:       service.StatusError,
		ErrorMessage: "temporary error",
	})
	cacheRecorder := &schedulerCacheRecorder{}
	s.repo.schedulerCache = cacheRecorder

	s.Require().NoError(s.repo.ClearError(s.ctx, account.ID))

	got, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().Equal(service.StatusActive, got.Status)
	s.Require().Empty(got.ErrorMessage)
	s.Require().Len(cacheRecorder.setAccounts, 1)
	s.Require().Equal(account.ID, cacheRecorder.setAccounts[0].ID)
	s.Require().Equal(service.StatusActive, cacheRecorder.setAccounts[0].Status)
}

// --- UpdateSessionWindow ---

func (s *AccountRepoSuite) TestUpdateSessionWindow() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-win"})
	start := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	end := time.Date(2025, 6, 15, 15, 0, 0, 0, time.UTC)

	s.Require().NoError(s.repo.UpdateSessionWindow(s.ctx, account.ID, &start, &end, "active"))

	got, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().NotNil(got.SessionWindowStart)
	s.Require().NotNil(got.SessionWindowEnd)
	s.Require().Equal("active", got.SessionWindowStatus)
}

// --- UpdateExtra ---

func (s *AccountRepoSuite) TestUpdateExtra_MergesFields() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:  "acc-extra",
		Extra: map[string]any{"a": "1"},
	})
	s.Require().NoError(s.repo.UpdateExtra(s.ctx, account.ID, map[string]any{"b": "2"}), "UpdateExtra")

	got, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err, "GetByID")
	s.Require().Equal("1", got.Extra["a"])
	s.Require().Equal("2", got.Extra["b"])
}

func (s *AccountRepoSuite) TestUpdateExtra_EmptyUpdates() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-extra-empty"})
	s.Require().NoError(s.repo.UpdateExtra(s.ctx, account.ID, map[string]any{}))
}

func (s *AccountRepoSuite) TestUpdateExtra_NilExtra() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-nil-extra", Extra: nil})
	s.Require().NoError(s.repo.UpdateExtra(s.ctx, account.ID, map[string]any{"key": "val"}))

	got, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().Equal("val", got.Extra["key"])
}

func (s *AccountRepoSuite) TestUpdateExtra_SchedulerNeutralSkipsOutboxAndSyncsFreshSnapshot() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:     "acc-extra-neutral",
		Platform: service.PlatformOpenAI,
		Extra:    map[string]any{"codex_usage_updated_at": "old"},
	})
	cacheRecorder := &schedulerCacheRecorder{
		accounts: map[int64]*service.Account{
			account.ID: {
				ID:       account.ID,
				Platform: account.Platform,
				Status:   service.StatusDisabled,
				Extra: map[string]any{
					"codex_usage_updated_at": "old",
				},
			},
		},
	}
	s.repo.schedulerCache = cacheRecorder

	updates := map[string]any{
		"codex_usage_updated_at":     "2026-03-11T10:00:00Z",
		"codex_5h_used_percent":      88.5,
		"session_window_utilization": 0.42,
	}
	s.Require().NoError(s.repo.UpdateExtra(s.ctx, account.ID, updates))

	got, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().Equal("2026-03-11T10:00:00Z", got.Extra["codex_usage_updated_at"])
	s.Require().Equal(88.5, got.Extra["codex_5h_used_percent"])
	s.Require().Equal(0.42, got.Extra["session_window_utilization"])

	var outboxCount int
	s.Require().NoError(scanSingleRow(s.ctx, s.repo.sql, "SELECT COUNT(*) FROM scheduler_outbox", nil, &outboxCount))
	s.Require().Zero(outboxCount)
	s.Require().Len(cacheRecorder.setAccounts, 1)
	s.Require().NotNil(cacheRecorder.accounts[account.ID])
	s.Require().Equal(service.StatusActive, cacheRecorder.accounts[account.ID].Status)
	s.Require().Equal("2026-03-11T10:00:00Z", cacheRecorder.accounts[account.ID].Extra["codex_usage_updated_at"])
}

func (s *AccountRepoSuite) TestUpdateExtra_ExhaustedCodexSnapshotSyncsSchedulerCache() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:     "acc-extra-codex-exhausted",
		Platform: service.PlatformOpenAI,
		Type:     service.AccountTypeOAuth,
		Extra:    map[string]any{},
	})
	cacheRecorder := &schedulerCacheRecorder{}
	s.repo.schedulerCache = cacheRecorder
	_, err := s.repo.sql.ExecContext(s.ctx, "TRUNCATE scheduler_outbox")
	s.Require().NoError(err)

	s.Require().NoError(s.repo.UpdateExtra(s.ctx, account.ID, map[string]any{
		"codex_7d_used_percent":        100.0,
		"codex_7d_reset_at":            "2026-03-12T13:00:00Z",
		"codex_7d_reset_after_seconds": 86400,
	}))

	var count int
	err = scanSingleRow(s.ctx, s.repo.sql, "SELECT COUNT(*) FROM scheduler_outbox", nil, &count)
	s.Require().NoError(err)
	s.Require().Equal(0, count)
	s.Require().Len(cacheRecorder.setAccounts, 1)
	s.Require().Equal(account.ID, cacheRecorder.setAccounts[0].ID)
	s.Require().Equal(service.StatusActive, cacheRecorder.setAccounts[0].Status)
	s.Require().Equal(100.0, cacheRecorder.setAccounts[0].Extra["codex_7d_used_percent"])
}

func (s *AccountRepoSuite) TestUpdateExtra_SchedulerRelevantStillEnqueuesOutbox() {
	account := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:     "acc-extra-mixed",
		Platform: service.PlatformAntigravity,
		Extra:    map[string]any{},
	})
	_, err := s.repo.sql.ExecContext(s.ctx, "TRUNCATE scheduler_outbox")
	s.Require().NoError(err)

	s.Require().NoError(s.repo.UpdateExtra(s.ctx, account.ID, map[string]any{
		"mixed_scheduling":       true,
		"codex_usage_updated_at": "2026-03-11T10:00:00Z",
	}))

	var count int
	err = scanSingleRow(s.ctx, s.repo.sql, "SELECT COUNT(*) FROM scheduler_outbox", nil, &count)
	s.Require().NoError(err)
	s.Require().Equal(1, count)
}

// --- GetByCRSAccountID ---

func (s *AccountRepoSuite) TestGetByCRSAccountID() {
	crsID := "crs-12345"
	mustCreateAccount(s.T(), s.client, &service.Account{
		Name:  "acc-crs",
		Extra: map[string]any{"crs_account_id": crsID},
	})

	got, err := s.repo.GetByCRSAccountID(s.ctx, crsID)
	s.Require().NoError(err)
	s.Require().NotNil(got)
	s.Require().Equal("acc-crs", got.Name)
}

func (s *AccountRepoSuite) TestGetByCRSAccountID_NotFound() {
	got, err := s.repo.GetByCRSAccountID(s.ctx, "non-existent")
	s.Require().NoError(err)
	s.Require().Nil(got)
}

func (s *AccountRepoSuite) TestGetByCRSAccountID_EmptyString() {
	got, err := s.repo.GetByCRSAccountID(s.ctx, "")
	s.Require().NoError(err)
	s.Require().Nil(got)
}

// --- BulkUpdate ---

func (s *AccountRepoSuite) TestBulkUpdate() {
	a1 := mustCreateAccount(s.T(), s.client, &service.Account{Name: "bulk1", Priority: 1})
	a2 := mustCreateAccount(s.T(), s.client, &service.Account{Name: "bulk2", Priority: 1})

	newPriority := 99
	affected, err := s.repo.BulkUpdate(s.ctx, []int64{a1.ID, a2.ID}, service.AccountBulkUpdate{
		Priority: &newPriority,
	})
	s.Require().NoError(err)
	s.Require().GreaterOrEqual(affected, int64(1), "expected at least one affected row")

	got1, _ := s.repo.GetByID(s.ctx, a1.ID)
	got2, _ := s.repo.GetByID(s.ctx, a2.ID)
	s.Require().Equal(99, got1.Priority)
	s.Require().Equal(99, got2.Priority)
}

func (s *AccountRepoSuite) TestBulkUpdate_MergeCredentials() {
	a1 := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:        "bulk-cred",
		Credentials: map[string]any{"existing": "value"},
	})

	_, err := s.repo.BulkUpdate(s.ctx, []int64{a1.ID}, service.AccountBulkUpdate{
		Credentials: map[string]any{"new_key": "new_value"},
	})
	s.Require().NoError(err)

	got, _ := s.repo.GetByID(s.ctx, a1.ID)
	s.Require().Equal("value", got.Credentials["existing"])
	s.Require().Equal("new_value", got.Credentials["new_key"])
}

func (s *AccountRepoSuite) TestBulkUpdate_MergeExtra() {
	a1 := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:  "bulk-extra",
		Extra: map[string]any{"existing": "val"},
	})

	_, err := s.repo.BulkUpdate(s.ctx, []int64{a1.ID}, service.AccountBulkUpdate{
		Extra: map[string]any{"new_key": "new_val"},
	})
	s.Require().NoError(err)

	got, _ := s.repo.GetByID(s.ctx, a1.ID)
	s.Require().Equal("val", got.Extra["existing"])
	s.Require().Equal("new_val", got.Extra["new_key"])
}

func (s *AccountRepoSuite) TestBulkUpdate_EmptyIDs() {
	affected, err := s.repo.BulkUpdate(s.ctx, []int64{}, service.AccountBulkUpdate{})
	s.Require().NoError(err)
	s.Require().Zero(affected)
}

func (s *AccountRepoSuite) TestBulkUpdate_EmptyUpdates() {
	a1 := mustCreateAccount(s.T(), s.client, &service.Account{Name: "bulk-empty"})

	affected, err := s.repo.BulkUpdate(s.ctx, []int64{a1.ID}, service.AccountBulkUpdate{})
	s.Require().NoError(err)
	s.Require().Zero(affected)
}

func idsOfAccounts(accounts []service.Account) []int64 {
	out := make([]int64, 0, len(accounts))
	for i := range accounts {
		out = append(out, accounts[i].ID)
	}
	return out
}
