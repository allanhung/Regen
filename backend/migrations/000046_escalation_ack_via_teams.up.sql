-- Allow Teams as an acknowledgment channel for escalation states.
ALTER TABLE escalation_states DROP CONSTRAINT IF EXISTS escalation_states_acknowledged_via_check;
ALTER TABLE escalation_states
    ADD CONSTRAINT escalation_states_acknowledged_via_check
    CHECK (acknowledged_via IS NULL OR acknowledged_via IN ('slack', 'api', 'cli', 'teams'));
