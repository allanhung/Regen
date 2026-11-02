package services

import (
	"fmt"

	"github.com/FluidifyAI/Regen/backend/internal/models"
	"github.com/google/uuid"
)

// Action.Submit payload keys. Teams delivers the data object back to the bot as the
// "value" of a message activity, handled by TeamsEventHandler.handleCardAction.
const (
	teamsCardActionKey = "regen_action"
	teamsActionAck     = "ack"
	teamsActionResolve = "resolve"
	teamsActionStatus  = "status"
)

// teamsActionButtons builds the Acknowledge / Resolve / Status buttons for a card.
// Either incidentID or alertID identifies the target; the handler resolves an alert to
// its linked incident. A status of "" shows every button (state unknown at render time).
func teamsActionButtons(incidentID, alertID, status string) []map[string]interface{} {
	button := func(title, action, style string) map[string]interface{} {
		data := map[string]interface{}{teamsCardActionKey: action}
		if incidentID != "" {
			data["incident_id"] = incidentID
		}
		if alertID != "" {
			data["alert_id"] = alertID
		}
		b := map[string]interface{}{"type": "Action.Submit", "title": title, "data": data}
		if style != "" {
			b["style"] = style
		}
		return b
	}

	terminal := status == string(models.IncidentStatusResolved) || status == string(models.IncidentStatusCanceled)
	var buttons []map[string]interface{}
	if status == "" || status == string(models.IncidentStatusTriggered) {
		buttons = append(buttons, button("Acknowledge", teamsActionAck, "positive"))
	}
	if !terminal {
		buttons = append(buttons, button("Resolve", teamsActionResolve, ""))
	}
	return append(buttons, button("Status", teamsActionStatus, ""))
}

// teamsIncidentCard returns an Adaptive Card payload (as a Go map, to be JSON-serialised
// when posting via Graph API) representing the current incident state.
// The card is designed to show the same key information as the Slack incident card.
func teamsIncidentCard(incident *models.Incident) map[string]interface{} {
	statusEmoji := teamsStatusEmoji(string(incident.Status))
	severityColor := teamsSeverityColor(string(incident.Severity))

	body := []map[string]interface{}{
		{
			"type":   "TextBlock",
			"size":   "Large",
			"weight": "Bolder",
			"text":   fmt.Sprintf("%s INC-%d: %s", statusEmoji, incident.IncidentNumber, incident.Title),
			"wrap":   true,
			"color":  severityColor,
		},
		{
			"type": "FactSet",
			"facts": []map[string]interface{}{
				{"title": "Status", "value": string(incident.Status)},
				{"title": "Severity", "value": string(incident.Severity)},
				{"title": "Triggered", "value": incident.TriggeredAt.Format("2006-01-02 15:04 UTC")},
			},
		},
	}

	if incident.Summary != "" {
		body = append(body, map[string]interface{}{
			"type":      "TextBlock",
			"text":      incident.Summary,
			"wrap":      true,
			"isSubtle":  true,
			"separator": true,
		})
	}

	return map[string]interface{}{
		"$schema": "http://adaptivecards.io/schemas/adaptive-card.json",
		"type":    "AdaptiveCard",
		"version": "1.4",
		"body":    body,
		"actions": teamsActionButtons(incident.ID.String(), "", string(incident.Status)),
	}
}

// teamsStatusUpdateCard builds a compact card for status change notifications.
func teamsStatusUpdateCard(incident *models.Incident, changedBy string) map[string]interface{} {
	emoji := teamsStatusEmoji(string(incident.Status))
	return map[string]interface{}{
		"$schema": "http://adaptivecards.io/schemas/adaptive-card.json",
		"type":    "AdaptiveCard",
		"version": "1.4",
		"body": []map[string]interface{}{
			{
				"type":   "TextBlock",
				"text":   fmt.Sprintf("%s INC-%d status changed to **%s**", emoji, incident.IncidentNumber, incident.Status),
				"wrap":   true,
				"weight": "Bolder",
			},
			{
				"type":     "TextBlock",
				"text":     fmt.Sprintf("Updated by: %s", changedBy),
				"isSubtle": true,
			},
		},
	}
}

// teamsDMCard returns an Adaptive Card for a proactive on-call DM.
// Uses OpenUrl actions so responders can act even outside the Teams channel context.
func teamsDMCard(incident *models.Incident, appURL string) map[string]interface{} {
	statusEmoji := teamsStatusEmoji(string(incident.Status))
	severityColor := teamsSeverityColor(string(incident.Severity))

	incidentURL := fmt.Sprintf("%s/incidents/%s", appURL, incident.ID)

	return map[string]interface{}{
		"$schema": "http://adaptivecards.io/schemas/adaptive-card.json",
		"type":    "AdaptiveCard",
		"version": "1.4",
		"body": []map[string]interface{}{
			{
				"type":   "TextBlock",
				"size":   "Medium",
				"weight": "Bolder",
				"text":   "🚨 You're on-call — new incident",
				"wrap":   true,
			},
			{
				"type":   "TextBlock",
				"size":   "Large",
				"weight": "Bolder",
				"text":   fmt.Sprintf("%s INC-%d: %s", statusEmoji, incident.IncidentNumber, incident.Title),
				"wrap":   true,
				"color":  severityColor,
			},
			{
				"type": "FactSet",
				"facts": []map[string]interface{}{
					{"title": "Severity", "value": string(incident.Severity)},
					{"title": "Status", "value": string(incident.Status)},
					{"title": "Triggered", "value": incident.TriggeredAt.Format("2006-01-02 15:04 UTC")},
				},
			},
		},
		"actions": append([]map[string]interface{}{
			{
				"type":  "Action.OpenUrl",
				"title": "View Incident",
				"url":   incidentURL,
			},
		}, teamsActionButtons(incident.ID.String(), "", string(incident.Status))...),
	}
}

// TeamsEscalationCard builds the Adaptive Card sent to someone an escalation has reached.
// alert is nil for incident-sourced escalations, which get a card without alert details.
func TeamsEscalationCard(alert *models.Alert, tierIndex int, appURL string) map[string]interface{} {
	body := []map[string]interface{}{
		{
			"type":   "TextBlock",
			"size":   "Medium",
			"weight": "Bolder",
			"text":   fmt.Sprintf("🚨 You're being paged (Tier %d)", tierIndex+1),
			"wrap":   true,
		},
	}
	actions := []map[string]interface{}{}

	if alert == nil {
		body = append(body, map[string]interface{}{
			"type": "TextBlock",
			"text": fmt.Sprintf("An incident escalated to Tier %d of its escalation policy.", tierIndex+1),
			"wrap": true,
		})
		actions = append(actions, map[string]interface{}{
			"type": "Action.OpenUrl", "title": "Open Fluidify Regen", "url": appURL + "/incidents",
		})
	} else {
		body = append(body,
			map[string]interface{}{
				"type":   "TextBlock",
				"size":   "Large",
				"weight": "Bolder",
				"text":   alert.Title,
				"wrap":   true,
				"color":  teamsAlertSeverityColor(string(alert.Severity)),
			},
			map[string]interface{}{
				"type": "FactSet",
				"facts": []map[string]interface{}{
					{"title": "Severity", "value": string(alert.Severity)},
					{"title": "Source", "value": alert.Source},
					{"title": "Tier", "value": fmt.Sprintf("%d of escalation policy", tierIndex+1)},
				},
			},
		)
		actions = append(actions, map[string]interface{}{
			"type": "Action.OpenUrl", "title": "View Alert", "url": fmt.Sprintf("%s/alerts/%s", appURL, alert.ID),
		})
		actions = append(actions, teamsActionButtons("", alert.ID.String(), "")...)
	}

	return map[string]interface{}{
		"$schema": "http://adaptivecards.io/schemas/adaptive-card.json",
		"type":    "AdaptiveCard",
		"version": "1.4",
		"body":    body,
		"actions": actions,
	}
}

// TeamsIncidentEscalationCard is the page for an incident-sourced escalation, with the
// incident's Acknowledge / Resolve / Status buttons.
func TeamsIncidentEscalationCard(incidentID uuid.UUID, tierIndex int, appURL string) map[string]interface{} {
	card := TeamsEscalationCard(nil, tierIndex, appURL)
	actions, _ := card["actions"].([]map[string]interface{})
	card["actions"] = append(actions, teamsActionButtons(incidentID.String(), "", "")...)
	return card
}

func teamsAlertSeverityColor(severity string) string {
	switch severity {
	case "critical":
		return "Attention"
	case "warning":
		return "Warning"
	default:
		return "Default"
	}
}

func teamsStatusEmoji(status string) string {
	switch status {
	case "triggered":
		return "🔴"
	case "acknowledged":
		return "🟡"
	case "resolved":
		return "✅"
	case "canceled":
		return "⛔"
	default:
		return "⚪"
	}
}

func teamsSeverityColor(severity string) string {
	switch severity {
	case "critical":
		return "Attention"
	case "high":
		return "Warning"
	case "medium":
		return "Default"
	case "low":
		return "Good"
	default:
		return "Default"
	}
}
