//go:build integration

package repository

import (
	"encoding/json"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (s *AccountRepoSuite) readLatestBulkAccountChangedPayload() map[string]any {
	var payloadRaw []byte
	s.Require().NoError(scanSingleRow(
		s.ctx,
		s.repo.sql,
		`SELECT payload
		 FROM scheduler_outbox
		 WHERE event_type = $1
		 ORDER BY id DESC
		 LIMIT 1`,
		[]any{service.SchedulerOutboxEventAccountBulkChanged},
		&payloadRaw,
	))
	s.Require().NotEmpty(payloadRaw)

	var payload map[string]any
	s.Require().NoError(json.Unmarshal(payloadRaw, &payload))
	return payload
}

func (s *AccountRepoSuite) requirePayloadGroupIDs(payload map[string]any, expected []int64) {
	rawIDs, ok := payload["group_ids"].([]any)
	s.Require().True(ok, "expected group_ids array in payload")
	actual := make([]int64, 0, len(rawIDs))
	for _, rawID := range rawIDs {
		number, ok := rawID.(float64)
		s.Require().True(ok, "expected numeric group id in payload")
		actual = append(actual, int64(number))
	}
	s.Require().ElementsMatch(expected, actual)
}

func (s *AccountRepoSuite) requirePayloadAccountIDs(payload map[string]any, expected []int64) {
	rawIDs, ok := payload["account_ids"].([]any)
	s.Require().True(ok, "expected account_ids array in payload")
	actual := make([]int64, 0, len(rawIDs))
	for _, rawID := range rawIDs {
		number, ok := rawID.(float64)
		s.Require().True(ok, "expected numeric account id in payload")
		actual = append(actual, int64(number))
	}
	s.Require().Equal(expected, actual)
}

func (s *AccountRepoSuite) TestBindGroupsBatch_ReplacesGroupsForAllTargets() {
	g1 := mustCreateGroup(s.T(), s.client, &service.Group{Name: "batch-g1"})
	g2 := mustCreateGroup(s.T(), s.client, &service.Group{Name: "batch-g2"})
	g3 := mustCreateGroup(s.T(), s.client, &service.Group{Name: "batch-g3"})
	a1 := mustCreateAccount(s.T(), s.client, &service.Account{Name: "batch-a1"})
	a2 := mustCreateAccount(s.T(), s.client, &service.Account{Name: "batch-a2"})
	mustBindAccountToGroup(s.T(), s.client, a1.ID, g1.ID, 1)
	mustBindAccountToGroup(s.T(), s.client, a2.ID, g1.ID, 1)

	s.Require().NoError(s.repo.BindGroupsBatch(s.ctx, []int64{a1.ID, a2.ID}, []int64{g2.ID, g3.ID}))

	groupsA1, err := s.repo.GetGroups(s.ctx, a1.ID)
	s.Require().NoError(err)
	groupsA2, err := s.repo.GetGroups(s.ctx, a2.ID)
	s.Require().NoError(err)
	s.Require().Len(groupsA1, 2)
	s.Require().Len(groupsA2, 2)
	s.Require().Equal([]int64{g2.ID, g3.ID}, []int64{groupsA1[0].ID, groupsA1[1].ID})
	s.Require().Equal([]int64{g2.ID, g3.ID}, []int64{groupsA2[0].ID, groupsA2[1].ID})
}

func (s *AccountRepoSuite) TestBindGroupsBatch_NormalizesIDsBeforePersistingAndEnqueueing() {
	_, err := s.repo.sql.ExecContext(s.ctx, "TRUNCATE scheduler_outbox RESTART IDENTITY")
	s.Require().NoError(err)

	groupOld := mustCreateGroup(s.T(), s.client, &service.Group{Name: "batch-normalize-old"})
	groupNewA := mustCreateGroup(s.T(), s.client, &service.Group{Name: "batch-normalize-new-a"})
	groupNewB := mustCreateGroup(s.T(), s.client, &service.Group{Name: "batch-normalize-new-b"})
	a1 := mustCreateAccount(s.T(), s.client, &service.Account{Name: "batch-normalize-a1"})
	a2 := mustCreateAccount(s.T(), s.client, &service.Account{Name: "batch-normalize-a2"})
	mustBindAccountToGroup(s.T(), s.client, a1.ID, groupOld.ID, 1)
	mustBindAccountToGroup(s.T(), s.client, a2.ID, groupOld.ID, 1)
	_, err = s.repo.sql.ExecContext(s.ctx, "TRUNCATE scheduler_outbox RESTART IDENTITY")
	s.Require().NoError(err)

	s.Require().NoError(s.repo.BindGroupsBatch(s.ctx, []int64{a1.ID, 0, a2.ID, a1.ID, -1}, []int64{groupNewB.ID, 0, groupNewA.ID, groupNewB.ID, -2}))

	groupsA1, err := s.repo.GetGroups(s.ctx, a1.ID)
	s.Require().NoError(err)
	groupsA2, err := s.repo.GetGroups(s.ctx, a2.ID)
	s.Require().NoError(err)
	s.Require().Len(groupsA1, 2)
	s.Require().Len(groupsA2, 2)
	s.Require().ElementsMatch([]int64{groupNewB.ID, groupNewA.ID}, []int64{groupsA1[0].ID, groupsA1[1].ID})
	s.Require().ElementsMatch([]int64{groupNewB.ID, groupNewA.ID}, []int64{groupsA2[0].ID, groupsA2[1].ID})

	payload := s.readLatestBulkAccountChangedPayload()
	s.requirePayloadAccountIDs(payload, []int64{a1.ID, a2.ID})
	s.requirePayloadGroupIDs(payload, []int64{groupOld.ID, groupNewB.ID, groupNewA.ID})
}

func (s *AccountRepoSuite) TestBindGroupsBatch_OutboxPayloadIncludesOldAndNewGroups() {
	_, err := s.repo.sql.ExecContext(s.ctx, "TRUNCATE scheduler_outbox RESTART IDENTITY")
	s.Require().NoError(err)

	groupA := mustCreateGroup(s.T(), s.client, &service.Group{Name: "batch-old-a"})
	groupB := mustCreateGroup(s.T(), s.client, &service.Group{Name: "batch-old-b"})
	groupC := mustCreateGroup(s.T(), s.client, &service.Group{Name: "batch-new-c"})
	groupD := mustCreateGroup(s.T(), s.client, &service.Group{Name: "batch-new-d"})
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "batch-payload-account"})
	mustBindAccountToGroup(s.T(), s.client, account.ID, groupA.ID, 1)
	mustBindAccountToGroup(s.T(), s.client, account.ID, groupB.ID, 2)
	_, err = s.repo.sql.ExecContext(s.ctx, "TRUNCATE scheduler_outbox RESTART IDENTITY")
	s.Require().NoError(err)

	s.Require().NoError(s.repo.BindGroupsBatch(s.ctx, []int64{account.ID}, []int64{groupC.ID, groupD.ID}))

	payload := s.readLatestBulkAccountChangedPayload()
	s.requirePayloadGroupIDs(payload, []int64{groupA.ID, groupB.ID, groupC.ID, groupD.ID})
}

func (s *AccountRepoSuite) TestBindGroupsBatch_OutboxPayloadIncludesOldGroupsWhenClearing() {
	_, err := s.repo.sql.ExecContext(s.ctx, "TRUNCATE scheduler_outbox RESTART IDENTITY")
	s.Require().NoError(err)

	groupA := mustCreateGroup(s.T(), s.client, &service.Group{Name: "batch-clear-a"})
	groupB := mustCreateGroup(s.T(), s.client, &service.Group{Name: "batch-clear-b"})
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "batch-clear-account"})
	mustBindAccountToGroup(s.T(), s.client, account.ID, groupA.ID, 1)
	mustBindAccountToGroup(s.T(), s.client, account.ID, groupB.ID, 2)
	_, err = s.repo.sql.ExecContext(s.ctx, "TRUNCATE scheduler_outbox RESTART IDENTITY")
	s.Require().NoError(err)

	s.Require().NoError(s.repo.BindGroupsBatch(s.ctx, []int64{account.ID}, nil))

	payload := s.readLatestBulkAccountChangedPayload()
	s.requirePayloadGroupIDs(payload, []int64{groupA.ID, groupB.ID})
}
