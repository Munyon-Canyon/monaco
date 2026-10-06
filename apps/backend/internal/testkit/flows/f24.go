package flows

import (
	"context"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func F24NotifyOK(s *scenario.Scenario) {
	var user ids.UserID
	s.Given(func(s *scenario.Scenario) { user = seedPushDevice(s) }).
		When(func(s *scenario.Scenario) { requestTestPush(s, user) }).
		Then(
			scenario.Eventually("the test push delivered", func(s *scenario.Scenario) bool {
				return testPushDelivered(s, user)
			}),
			func(s *scenario.Scenario) { wantOneNotificationSent(s, user) },
		)
}

func seedPushDevice(s *scenario.Scenario) ids.UserID {
	user := testkit.SeedUser(seedT{s}, s.DB(), testkit.UserOpts{}).ID
	token := strings.Repeat(strings.ReplaceAll(user.String(), "-", ""), 2)
	if _, err := s.DB().Exec(s.Context(), `INSERT INTO device_tokens
		(id, user_id, token, environment, created_at, last_seen_at) VALUES ($1, $2, $3, 'sandbox', now(), now())`,
		ids.Real{}.NewV7(), user.UUID(), token); err != nil {
		s.Fatalf("flows: register a sandbox device for %s: %v", user, err)
	}
	return user
}

func requestTestPush(s *scenario.Scenario, user ids.UserID) {
	ctx := auth.WithActor(s.Context(), auth.Actor{Kind: auth.ActorSystem, ID: "monacoctl"})
	if err := db.New(s.DB(), ids.Real{}, clock.Real{}).Do(ctx, func(ctx context.Context, tx db.Tx) error {
		return tx.Events.Append(ctx, events.NotifyTestRequested{V: 1, UserID: user.UUID()})
	}); err != nil {
		s.Fatalf("flows: request a test push for %s: %v", user, err)
	}
}

func testPushDelivered(s *scenario.Scenario, user ids.UserID) bool {
	var delivered bool
	if err := s.DB().QueryRow(s.Context(), `SELECT EXISTS (SELECT 1 FROM notifications
		WHERE user_id = $1 AND kind = 'test' AND state = 'delivered')`, user.UUID()).Scan(&delivered); err != nil {
		s.Fatalf("flows: read the test push of %s: %v", user, err)
	}
	return delivered
}

func wantOneNotificationSent(s *scenario.Scenario, user ids.UserID) {
	var sent int
	if err := s.DB().QueryRow(s.Context(), `SELECT count(*) FROM events
		WHERE type = 'notification.sent' AND payload->>'user_id' = $1`, user.String()).Scan(&sent); err != nil {
		s.Fatalf("flows: count notification.sent for %s: %v", user, err)
	}
	if sent != 1 {
		s.Fatalf("flows: %d notification.sent events for %s, want 1", sent, user)
	}
}
