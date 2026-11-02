package services

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/FluidifyAI/Regen/backend/internal/models"
	"github.com/google/uuid"
)

func TestTeamsEscalationCard_AlertDetailsAndLink(t *testing.T) {
	id := uuid.New()
	alert := &models.Alert{ID: id, Title: "EscalationCheck", Severity: models.AlertSeverityCritical, Source: "prometheus"}

	raw, _ := json.Marshal(TeamsEscalationCard(alert, 1, "https://regen.example.com"))
	got := string(raw)

	for _, want := range []string{
		"Tier 2", "EscalationCheck", "critical", "prometheus", "Attention",
		"View Alert", "https://regen.example.com/alerts/" + id.String(),
	} {
		if !strings.Contains(got, want) {
			t.Errorf("card missing %q:\n%s", want, got)
		}
	}
}

func TestTeamsEscalationCard_NilAlertDoesNotPanic(t *testing.T) {
	raw, _ := json.Marshal(TeamsEscalationCard(nil, 0, "https://regen.example.com"))
	got := string(raw)
	if !strings.Contains(got, "Tier 1") || !strings.Contains(got, "https://regen.example.com/incidents") {
		t.Errorf("incident-sourced card incomplete:\n%s", got)
	}
}

func TestSendDirectMessage_SendsTeamsCardWhenSet(t *testing.T) {
	var calls []recordedCall
	svc := newDMTestService(t, http.StatusOK, &calls)
	svc.SetUserResolver(func(string) (string, bool) { return "alice-teams-id", true })

	msg := Message{Text: "fallback", TeamsCard: map[string]interface{}{"type": "AdaptiveCard"}}
	if err := svc.SendDirectMessage("alice@example.com", msg); err != nil {
		t.Fatal(err)
	}
	activity := calls[1].body
	atts, ok := activity["attachments"].([]interface{})
	if !ok || len(atts) != 1 {
		t.Fatalf("expected one card attachment, got %v", activity)
	}
	if ct := atts[0].(map[string]interface{})["contentType"]; ct != "application/vnd.microsoft.card.adaptive" {
		t.Errorf("contentType = %v", ct)
	}
	if _, hasText := activity["text"]; hasText {
		t.Error("card activity should not also carry text")
	}
}

func TestTeamsCards_ActionButtons(t *testing.T) {
	alertID := uuid.New()
	incident := &models.Incident{ID: uuid.New(), Status: models.IncidentStatusTriggered, Severity: models.IncidentSeverityHigh}

	cases := map[string]struct {
		card map[string]interface{}
		want []string
		not  []string
	}{
		"escalation": {TeamsEscalationCard(&models.Alert{ID: alertID}, 0, "https://x"), []string{"Acknowledge", "Resolve", "Status", alertID.String()}, nil},
		"incident":   {teamsIncidentCard(incident), []string{"Acknowledge", "Resolve", "Status", incident.ID.String()}, nil},
		"dm":         {teamsDMCard(incident, "https://x"), []string{"Acknowledge", "Resolve", "Status", "View Incident"}, nil},
		"acked":      {teamsIncidentCard(&models.Incident{ID: uuid.New(), Status: models.IncidentStatusAcknowledged}), []string{"Resolve", "Status"}, []string{"Acknowledge"}},
		"resolved":   {teamsIncidentCard(&models.Incident{ID: uuid.New(), Status: models.IncidentStatusResolved}), []string{"Status"}, []string{"Acknowledge", `"Resolve"`}},
	}
	for name, tc := range cases {
		raw, _ := json.Marshal(tc.card)
		got := string(raw)
		for _, w := range tc.want {
			if !strings.Contains(got, w) {
				t.Errorf("%s card missing %q:\n%s", name, w, got)
			}
		}
		for _, w := range tc.not {
			if strings.Contains(got, w) {
				t.Errorf("%s card should not contain %q:\n%s", name, w, got)
			}
		}
		if !strings.Contains(got, "Action.Submit") {
			t.Errorf("%s card has no Action.Submit", name)
		}
	}
}
