package services

import (
	"testing"

	"github.com/FluidifyAI/Regen/backend/internal/models"
	"github.com/FluidifyAI/Regen/backend/internal/repository"
	"github.com/google/uuid"
)

type alertsOnlyIncidentRepo struct {
	repository.IncidentRepository
	alerts []models.Alert
}

func (r *alertsOnlyIncidentRepo) GetAlerts(_ uuid.UUID) ([]models.Alert, error) { return r.alerts, nil }

type recordingEscalation struct {
	mockEscalationEngine
	acked         []uuid.UUID
	ackedVia      models.AcknowledgmentVia
	incidentAcked []uuid.UUID
	incidentDone  []uuid.UUID
}

func (r *recordingEscalation) AcknowledgeAlert(id uuid.UUID, _ string, via models.AcknowledgmentVia) error {
	r.acked = append(r.acked, id)
	r.ackedVia = via
	return nil
}
func (r *recordingEscalation) AcknowledgeIncident(id uuid.UUID, _ string, _ models.AcknowledgmentVia) error {
	r.incidentAcked = append(r.incidentAcked, id)
	return nil
}
func (r *recordingEscalation) CompleteIncident(id uuid.UUID) error {
	r.incidentDone = append(r.incidentDone, id)
	return nil
}

func TestSyncEscalation(t *testing.T) {
	incidentID := uuid.New()
	a1, a2 := models.Alert{ID: uuid.New()}, models.Alert{ID: uuid.New()}

	newSvc := func() (*incidentService, *recordingEscalation) {
		esc := &recordingEscalation{}
		return &incidentService{incidentRepo: &alertsOnlyIncidentRepo{alerts: []models.Alert{a1, a2}}, escalation: esc}, esc
	}

	t.Run("acknowledge stops alert and incident escalations", func(t *testing.T) {
		svc, esc := newSvc()
		svc.syncEscalation(incidentID, models.IncidentStatusAcknowledged, "u", models.AcknowledgmentViaTeams)
		if len(esc.acked) != 2 || esc.ackedVia != models.AcknowledgmentViaTeams {
			t.Errorf("alerts acked = %v via %q", esc.acked, esc.ackedVia)
		}
		if len(esc.incidentAcked) != 1 || esc.incidentAcked[0] != incidentID {
			t.Errorf("incident acked = %v", esc.incidentAcked)
		}
		if len(esc.completed) != 0 {
			t.Errorf("ack must not complete: %v", esc.completed)
		}
	})

	t.Run("resolve completes alert and incident escalations", func(t *testing.T) {
		svc, esc := newSvc()
		svc.syncEscalation(incidentID, models.IncidentStatusResolved, "u", models.AcknowledgmentViaAPI)
		if len(esc.completed) != 2 || len(esc.incidentDone) != 1 {
			t.Errorf("completed = %v, incidentDone = %v", esc.completed, esc.incidentDone)
		}
		if len(esc.acked) != 0 {
			t.Errorf("resolve must not ack: %v", esc.acked)
		}
	})

	t.Run("triggered is ignored and nil engine is safe", func(t *testing.T) {
		svc, esc := newSvc()
		svc.syncEscalation(incidentID, models.IncidentStatusTriggered, "u", models.AcknowledgmentViaAPI)
		if len(esc.acked)+len(esc.completed)+len(esc.incidentAcked)+len(esc.incidentDone) != 0 {
			t.Error("triggered must not touch escalation")
		}
		(&incidentService{}).syncEscalation(incidentID, models.IncidentStatusAcknowledged, "u", models.AcknowledgmentViaAPI)
	})
}

func TestEscalationViaForActor(t *testing.T) {
	for actor, want := range map[string]models.AcknowledgmentVia{
		"teams_bot": models.AcknowledgmentViaTeams, "slack_reaction": models.AcknowledgmentViaSlack, "user": models.AcknowledgmentViaAPI,
	} {
		if got := escalationViaForActor(actor); got != want {
			t.Errorf("%s -> %q, want %q", actor, got, want)
		}
	}
}
