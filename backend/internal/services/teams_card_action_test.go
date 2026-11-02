package services

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/FluidifyAI/Regen/backend/internal/models"
	"github.com/FluidifyAI/Regen/backend/internal/repository"
	"github.com/google/uuid"
)

type cardActionIncidentSvc struct {
	IncidentService
	incident *models.Incident
	alerts   []models.Alert
	acked    int
	resolved int
}

func (s *cardActionIncidentSvc) GetIncident(uuid.UUID, int) (*models.Incident, error) {
	if s.incident == nil {
		return nil, &repository.NotFoundError{Resource: "incident"}
	}
	return s.incident, nil
}
func (s *cardActionIncidentSvc) GetIncidentAlerts(uuid.UUID) ([]models.Alert, error) {
	return s.alerts, nil
}
func (s *cardActionIncidentSvc) AcknowledgeIncident(uuid.UUID, string, string) error {
	s.acked++
	return nil
}
func (s *cardActionIncidentSvc) ResolveIncident(uuid.UUID, string, string) error {
	s.resolved++
	return nil
}

type cardActionIncidentRepo struct {
	repository.IncidentRepository
	byAlert *models.Incident
}

func (r *cardActionIncidentRepo) GetIncidentByAlertID(id uuid.UUID) (*models.Incident, error) {
	if r.byAlert == nil {
		return nil, &repository.NotFoundError{Resource: "incident", ID: id.String()}
	}
	return r.byAlert, nil
}

func cardActivity(t *testing.T, value map[string]string) BotActivity {
	t.Helper()
	raw, _ := json.Marshal(value)
	return BotActivity{
		Type: "message", Value: raw,
		From:         BotAccount{ID: "29:user", Name: "Alice"},
		Conversation: BotConversation{ID: "conv-1"},
	}
}

func newCardHandler(t *testing.T, svc *cardActionIncidentSvc, repo *cardActionIncidentRepo) (*TeamsEventHandler, *recordingEscalation) {
	t.Helper()
	var calls []recordedCall
	h := NewTeamsEventHandler("app", svc, repo, nil, newDMTestService(t, http.StatusOK, &calls))
	esc := &recordingEscalation{}
	h.SetEscalationEngine(esc)
	return h, esc
}

func TestCardAction_AckAlertWithoutIncident_StopsAlertEscalation(t *testing.T) {
	alertID := uuid.New()
	h, esc := newCardHandler(t, &cardActionIncidentSvc{}, &cardActionIncidentRepo{})

	h.Handle(t.Context(), cardActivity(t, map[string]string{"regen_action": "ack", "alert_id": alertID.String()}))

	if len(esc.acked) != 1 || esc.acked[0] != alertID || esc.ackedVia != models.AcknowledgmentViaTeams {
		t.Fatalf("acked = %v via %q", esc.acked, esc.ackedVia)
	}
}

func TestCardAction_AckAlertOfAcknowledgedIncident_StillStopsEscalation(t *testing.T) {
	alertID := uuid.New()
	incident := &models.Incident{ID: uuid.New(), Status: models.IncidentStatusAcknowledged}
	svc := &cardActionIncidentSvc{incident: incident, alerts: []models.Alert{{ID: alertID}}}
	h, esc := newCardHandler(t, svc, &cardActionIncidentRepo{byAlert: incident})

	h.Handle(t.Context(), cardActivity(t, map[string]string{"regen_action": "ack", "alert_id": alertID.String()}))

	if svc.acked != 0 {
		t.Error("an already-acknowledged incident must not be acknowledged again")
	}
	if len(esc.acked) == 0 || len(esc.incidentAcked) != 1 {
		t.Fatalf("escalations not stopped: alerts %v, incident %v", esc.acked, esc.incidentAcked)
	}
}

func TestCardAction_AckIncidentButton_AcknowledgesTriggeredIncident(t *testing.T) {
	incident := &models.Incident{ID: uuid.New(), Status: models.IncidentStatusTriggered}
	svc := &cardActionIncidentSvc{incident: incident}
	h, _ := newCardHandler(t, svc, &cardActionIncidentRepo{})

	h.Handle(t.Context(), cardActivity(t, map[string]string{"regen_action": "ack", "incident_id": incident.ID.String()}))

	if svc.acked != 1 {
		t.Errorf("AcknowledgeIncident calls = %d, want 1", svc.acked)
	}
}

func TestCardAction_ResolveButton(t *testing.T) {
	for status, want := range map[models.IncidentStatus]int{
		models.IncidentStatusTriggered:    1,
		models.IncidentStatusAcknowledged: 1,
		models.IncidentStatusResolved:     0,
		models.IncidentStatusCanceled:     0,
	} {
		incident := &models.Incident{ID: uuid.New(), Status: status}
		svc := &cardActionIncidentSvc{incident: incident}
		h, _ := newCardHandler(t, svc, &cardActionIncidentRepo{})

		h.Handle(t.Context(), cardActivity(t, map[string]string{"regen_action": "resolve", "incident_id": incident.ID.String()}))

		if svc.resolved != want {
			t.Errorf("status %s: ResolveIncident calls = %d, want %d", status, svc.resolved, want)
		}
	}
}

func TestCardAction_InvalidPayloadsAreIgnored(t *testing.T) {
	svc := &cardActionIncidentSvc{}
	h, esc := newCardHandler(t, svc, &cardActionIncidentRepo{})

	for _, v := range []map[string]string{
		{},
		{"regen_action": "ack", "incident_id": "not-a-uuid"},
		{"regen_action": "ack", "alert_id": "not-a-uuid"},
		{"regen_action": "bogus", "incident_id": uuid.New().String()},
	} {
		h.Handle(t.Context(), cardActivity(t, v))
	}
	if svc.acked+svc.resolved+len(esc.acked) != 0 {
		t.Errorf("invalid payloads must not act: %+v", esc)
	}
}
