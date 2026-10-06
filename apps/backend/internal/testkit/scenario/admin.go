package scenario

import "github.com/monaco/monaco/apps/backend/internal/events"

func SeededAdmin(name, role string) Step {
	return func(s *Scenario) {
		s.t.Helper()
		SeededUser(name, "active")(s)
		_, err := s.app.pool.Exec(s.t.Context(),
			`INSERT INTO admins (user_id, role, granted_at) VALUES ($1, $2, now())`, s.remember[name], role)
		if err != nil {
			s.t.Fatalf("scenario: grant %s the %s role: %v", name, role, err)
		}
	}
}

func ExpectAdminAction(action events.AdminActionKind, targetID string) Step {
	return func(s *Scenario) {
		s.t.Helper()
		var n int
		var actors []string
		err := s.app.pool.QueryRow(s.t.Context(), `SELECT count(*), coalesce(array_agg(actor_type), '{}') FROM events
			WHERE type = $1 AND payload->>'action' = $2 AND payload->>'target_id' = $3`,
			string(events.TypeAdminAction), string(action), targetID).Scan(&n, &actors)
		if err != nil {
			s.t.Fatalf("scenario: read the %s admin.action events: %v", action, err)
		}
		if n != 1 || actors[0] != "admin" {
			s.t.Fatalf("scenario: %d admin.action events for %s on %s by actors %v, want exactly one by an admin",
				n, action, targetID, actors)
		}
	}
}
