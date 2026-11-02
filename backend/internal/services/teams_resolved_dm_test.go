package services

import (
	"net/http"
	"testing"

	"github.com/FluidifyAI/Regen/backend/internal/database"
	"github.com/FluidifyAI/Regen/backend/internal/models"
	"github.com/FluidifyAI/Regen/backend/internal/repository"
	"github.com/google/uuid"
)

func TestSendResolvedDM_PostsResolvedCard(t *testing.T) {
	var calls []recordedCall
	svc := newDMTestService(t, http.StatusOK, &calls)

	inc := &models.Incident{ID: uuid.New(), IncidentNumber: 7, Title: "db down"}
	if err := svc.SendResolvedDM("abc123", inc, "alice", "http://app"); err != nil {
		t.Fatalf("SendResolvedDM: %v", err)
	}
	if len(calls) != 2 {
		t.Fatalf("want 2 calls, got %d", len(calls))
	}
	members := calls[0].body["members"].([]interface{})
	if id := members[0].(map[string]interface{})["id"]; id != "29:abc123" {
		t.Errorf("member id = %v, want 29:abc123", id)
	}
	if calls[1].body["attachments"] == nil {
		t.Errorf("activity body missing card: %v", calls[1].body)
	}
}

func TestSendResolvedDM_ReportsActivityFailure(t *testing.T) {
	var calls []recordedCall
	svc := newDMTestService(t, http.StatusForbidden, &calls)
	if err := svc.SendResolvedDM("abc123", &models.Incident{}, "alice", "http://app"); err == nil {
		t.Fatal("a failed activity post must not be reported as success")
	}
}

func strPtr(s string) *string { return &s }

func newResolvedDMTestService(t *testing.T) (*incidentService, repository.UserRepository) {
	t.Helper()
	repo := repository.NewUserRepository(database.SetupTestDB(t))
	return &incidentService{userRepo: repo}, repo
}

func mkUser(t *testing.T, repo repository.UserRepository, name, teamsID string) *models.User {
	t.Helper()
	u := &models.User{
		ID: uuid.New(), Name: name, Email: name + "@example.com",
		AuthSource: "local", Active: true, Role: models.UserRoleMember,
	}
	if teamsID != "" {
		u.TeamsUserID = strPtr(teamsID)
	}
	if err := repo.CreateLocal(u); err != nil {
		t.Fatalf("create user %s: %v", name, err)
	}
	return u
}

func TestResolvedDMRecipients(t *testing.T) {
	s, repo := newResolvedDMTestService(t)
	oncall := mkUser(t, repo, "oncall", "29:aaa")
	commander := mkUser(t, repo, "commander", "bbb") // stored without the 29: prefix
	noTeams := mkUser(t, repo, "noteams", "")

	cases := []struct {
		name       string
		onCall     *uuid.UUID
		commander  *uuid.UUID
		resolvedBy string
		want       []string
	}{
		{"on-call and commander", &oncall.ID, &commander.ID, "alertmanager", []string{"29:aaa", "bbb"}},
		{"same person is sent once", &oncall.ID, &oncall.ID, "alertmanager", []string{"29:aaa"}},
		{"no on-call", nil, &commander.ID, "alertmanager", []string{"bbb"}},
		{"no commander", &oncall.ID, nil, "alertmanager", []string{"29:aaa"}},
		{"user without teams_user_id is skipped", &noTeams.ID, &commander.ID, "alertmanager", []string{"bbb"}},
		{"unknown user id is skipped", func() *uuid.UUID { id := uuid.New(); return &id }(), &commander.ID, "alertmanager", []string{"bbb"}},
		{"resolver by uuid excluded", &oncall.ID, &commander.ID, oncall.ID.String(), []string{"bbb"}},
		{"resolver by name excluded", &oncall.ID, &commander.ID, "Commander", []string{"29:aaa"}},
		{"resolver by teams id excluded", &oncall.ID, &commander.ID, "29:bbb", []string{"29:aaa"}},
		{"resolver by email excluded", &oncall.ID, &commander.ID, "oncall@example.com", []string{"bbb"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			inc := &models.Incident{CommanderID: tc.commander}
			got := s.resolvedDMRecipientsFor(inc, tc.onCall, tc.resolvedBy)
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("got %v, want %v", got, tc.want)
				}
			}
		})
	}
}

func TestResolvedDMRecipients_NilUserRepo(t *testing.T) {
	s := &incidentService{}
	id := uuid.New()
	if got := s.resolvedDMRecipientsFor(&models.Incident{CommanderID: &id}, &id, "x"); got != nil {
		t.Errorf("want nil without userRepo, got %v", got)
	}
}

func TestNotifyResolvedViaTeamsDM_SendsWithoutConversationID(t *testing.T) {
	var calls []recordedCall
	teams := newDMTestService(t, http.StatusOK, &calls)
	s, repo := newResolvedDMTestService(t)
	s.teamsSvc = teams
	commander := mkUser(t, repo, "commander", "bbb")

	// TeamsConversationID is nil: the channel update is skipped, the DM must still go out.
	inc := &models.Incident{ID: uuid.New(), CommanderID: &commander.ID, Title: "x"}
	s.notifyResolvedViaTeamsDM(inc, "alertmanager")
	if len(calls) != 2 {
		t.Fatalf("want 2 Bot Framework calls for one DM, got %d", len(calls))
	}
}
