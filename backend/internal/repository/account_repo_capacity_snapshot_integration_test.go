//go:build integration

package repository

import "github.com/Wei-Shaw/sub2api/internal/service"

func (s *AccountRepoSuite) TestListGroupCapacitySnapshotAccounts_ToleratesDirtyExtraValues() {
	group := mustCreateGroup(s.T(), s.client, &service.Group{Name: "g-capacity-snapshot-dirty"})

	clean := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:        "acc-capacity-snapshot-clean",
		Schedulable: true,
		Concurrency: 2,
		Extra: map[string]any{
			"max_sessions":                 "4",
			"session_idle_timeout_minutes": "6",
			"base_rpm":                     "12",
		},
	})
	dirty := mustCreateAccount(s.T(), s.client, &service.Account{
		Name:        "acc-capacity-snapshot-dirty",
		Schedulable: true,
		Concurrency: 3,
		Extra: map[string]any{
			"max_sessions":                 "oops",
			"session_idle_timeout_minutes": "  ",
			"base_rpm":                     "12x",
		},
	})

	mustBindAccountToGroup(s.T(), s.client, clean.ID, group.ID, 1)
	mustBindAccountToGroup(s.T(), s.client, dirty.ID, group.ID, 2)

	records, err := s.repo.ListGroupCapacitySnapshotAccounts(s.ctx, group.ID)
	s.Require().NoError(err)
	s.Require().Equal([]service.GroupCapacitySnapshotAccountRecord{
		{AccountID: clean.ID, Concurrency: 2, MaxSessions: 4, SessionIdleTimeoutMinutes: 6, BaseRPM: 12},
		{AccountID: dirty.ID, Concurrency: 3, MaxSessions: 0, SessionIdleTimeoutMinutes: 0, BaseRPM: 0},
	}, records)
}
