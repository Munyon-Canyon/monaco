package flows

import (
	"time"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

const nudgePoller = "identity.nudges"

func f28Scripts() map[string]Script {
	return map[string]Script{
		"F28EmitNudgesOK": F28EmitNudgesOK,
	}
}

func F28EmitNudgesOK(s *scenario.Scenario) {
	var user ids.UserID
	s.Given(seedAwaitingPhone(&user, 25*time.Hour)).
		When(
			scenario.AwaitTick(nudgePoller),
			scenario.AwaitTick(nudgePoller),
		).
		Then(expectOneNudge(&user, "add_phone"))
}

func seedAwaitingPhone(dst *ids.UserID, settled time.Duration) scenario.Step {
	return func(s *scenario.Scenario) {
		id, err := ids.ParseUserID(ids.Real{}.NewV7().String())
		if err != nil {
			s.Fatalf("flows: new user id: %v", err)
		}
		now := time.Now().UTC()
		if _, err := s.DB().Exec(s.Context(), `INSERT INTO users
			(id, privy_user_id, login_provider, auth_state, auth_state_changed_at, created_at, updated_at)
			VALUES ($1, $2, 'sms', 'AWAITING_PHONE', $3, $3, $3)`,
			id.UUID(), "did:privy:"+id.String(), now.Add(-settled)); err != nil {
			s.Fatalf("flows: seed a user awaiting phone: %v", err)
		}
		*dst = id
	}
}

func expectOneNudge(user *ids.UserID, kind string) scenario.Step {
	return func(s *scenario.Scenario) {
		var (
			nudges, number, count int
			got                   string
		)
		if err := s.DB().QueryRow(s.Context(), `SELECT count(e.id), coalesce(min(e.payload->>'kind'), ''),
			coalesce(min((e.payload->>'nudge_number')::int), 0), u.nudge_count
			FROM users u LEFT JOIN events e ON e.aggregate_id = u.id AND e.type = $2
			WHERE u.id = $1 GROUP BY u.nudge_count`,
			user.UUID(), string(events.TypeUserNudgeDue)).Scan(&nudges, &got, &number, &count); err != nil {
			s.Fatalf("flows: read nudges for %s: %v", user, err)
		}
		if nudges != 1 || got != kind || number != 1 || count != 1 {
			s.Fatalf("flows: %d nudges for %s, kind %q numbered %d, nudge_count %d; want one %s nudge numbered 1",
				nudges, user, got, number, count, kind)
		}
	}
}
