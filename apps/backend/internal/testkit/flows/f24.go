package flows

import (
	"context"
	"fmt"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/apns"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

const (
	pushHandler = "notify.notify_test_requested"
	apnsDevice  = "/apns/3/device/"
)

func F24NotifyOK(s *scenario.Scenario) {
	var user ids.UserID
	s.Given(func(s *scenario.Scenario) { user = seedPushDevices(s, apns.Sandbox, apns.Production) }).
		When(func(s *scenario.Scenario) { requestTestPush(s, user) }).
		Then(
			scenario.Eventually("the test push delivered", func(s *scenario.Scenario) bool {
				return testPushDelivered(s, user)
			}),
			func(s *scenario.Scenario) { wantNotificationsSent(s, user, 1) },
			func(s *scenario.Scenario) { pushAnswered(s, user, "200", 2) },
		)
}

func F24NotifyAPNSUnavailable(s *scenario.Scenario) {
	var user ids.UserID
	s.Given(func(s *scenario.Scenario) {
		user = seedPushDevices(s, apns.Sandbox)
		answerPushesWith(s, user, "/apns/too_many_requests_429", 1)
	}).
		When(func(s *scenario.Scenario) { requestTestPush(s, user) }).
		Then(
			func(s *scenario.Scenario) {
				busSaid(s, user, map[string]string{"outcome": "nak", "code": string(errs.CodeAPNSUnavailable)})
			},
			func(s *scenario.Scenario) { pushAnswered(s, user, "429", 1) },
			scenario.Eventually("the test push delivered", func(s *scenario.Scenario) bool {
				return testPushDelivered(s, user)
			}),
			func(s *scenario.Scenario) { wantNotificationsSent(s, user, 1) },
			func(s *scenario.Scenario) { pushAnswered(s, user, "200", 1) },
		)
}

func F24NotifyAPNSAuthFailed(s *scenario.Scenario) {
	var user ids.UserID
	s.Given(func(s *scenario.Scenario) {
		user = seedPushDevices(s, apns.Sandbox)
		answerPushesWith(s, user, "/apns/forbidden_403", 10)
	}).
		When(func(s *scenario.Scenario) { requestTestPush(s, user) }).
		Then(
			func(s *scenario.Scenario) {
				busSaid(s, user, map[string]string{
					"outcome": "term", "code": string(errs.CodeAPNSAuthFailed), "alert": "true",
				})
			},
			func(s *scenario.Scenario) { wantNotificationState(s, user, "pending") },
			func(s *scenario.Scenario) { wantDeliveryCode(s, user, errs.CodeAPNSAuthFailed) },
			func(s *scenario.Scenario) { wantNotificationsSent(s, user, 0) },
		)
}

func F24NotifyCrashBeforeCommit(s *scenario.Scenario) {
	var user ids.UserID
	s.Given(func(s *scenario.Scenario) { user = seedPushDevices(s, apns.Sandbox) }).
		When(
			func(s *scenario.Scenario) { requestTestPush(s, user) },
			scenario.PublishCrashingAt(faultpoint.BeforeCommit),
		).
		Then(
			scenario.Eventually("the test push delivered", func(s *scenario.Scenario) bool {
				return testPushDelivered(s, user)
			}),
			func(s *scenario.Scenario) { wantNotificationRows(s, user, 1) },
			func(s *scenario.Scenario) { wantNotificationsSent(s, user, 1) },
		)
}

func seedPushDevices(s *scenario.Scenario, environments ...apns.Environment) ids.UserID {
	user := testkit.SeedUser(seedT{s}, s.DB(), testkit.UserOpts{}).ID
	for i, environment := range environments {
		if _, err := s.DB().Exec(s.Context(), `INSERT INTO device_tokens
			(id, user_id, token, environment, created_at, last_seen_at) VALUES ($1, $2, $3, $4, now(), now())`,
			ids.Real{}.NewV7(), user.UUID(), deviceToken(user, i), string(environment)); err != nil {
			s.Fatalf("flows: register a %s device for %s: %v", environment, user, err)
		}
	}
	return user
}

func deviceToken(user ids.UserID, n int) string {
	return fmt.Sprintf("%s%032x", strings.ReplaceAll(user.String(), "-", ""), n)
}

func answerPushesWith(s *scenario.Scenario, user ids.UserID, fixture string, times int) {
	scenario.FakeUpstream(fakes.Step{
		Route: apnsDevice + deviceToken(user, 0), Action: fakes.ActionSucceed, Fixture: fixture, Times: times,
		Reset: true,
	})(s)
}

func requestTestPush(s *scenario.Scenario, user ids.UserID) {
	ctx := auth.WithActor(s.Context(), auth.Actor{Kind: auth.ActorSystem, ID: "monacoctl"})
	if err := db.New(s.DB(), ids.Real{}, clock.Real{}).Do(ctx, func(ctx context.Context, tx db.Tx) error {
		return tx.Events.Append(ctx, events.NotifyTestRequested{V: 1, UserID: user.UUID()})
	}); err != nil {
		s.Fatalf("flows: request a test push for %s: %v", user, err)
	}
}

func triggerEvent(s *scenario.Scenario, user ids.UserID) string {
	var id string
	if err := s.DB().QueryRow(s.Context(), `SELECT id::text FROM events WHERE type = $1 AND payload->>'user_id' = $2`,
		string(events.TypeNotifyTestRequested), user.String()).Scan(&id); err != nil {
		s.Fatalf("flows: read the test push request of %s: %v", user, err)
	}
	return id
}

func pushAnswered(s *scenario.Scenario, user ids.UserID, status string, n int) {
	scenario.EventuallyLogs(observability.NotifyPushResult,
		map[string]string{"user_id": user.String(), "status": status}, n)(s)
}

func busSaid(s *scenario.Scenario, user ids.UserID, want map[string]string) {
	want["handler"], want["event_id"] = pushHandler, triggerEvent(s, user)
	scenario.EventuallyLog(observability.BusDispatched, want)(s)
}

func testPushDelivered(s *scenario.Scenario, user ids.UserID) bool {
	var delivered bool
	if err := s.DB().QueryRow(s.Context(), `SELECT EXISTS (SELECT 1 FROM notifications
		WHERE user_id = $1 AND kind = 'test' AND state = 'delivered')`, user.UUID()).Scan(&delivered); err != nil {
		s.Fatalf("flows: read the test push of %s: %v", user, err)
	}
	return delivered
}

func wantNotificationState(s *scenario.Scenario, user ids.UserID, want string) {
	var state string
	if err := s.DB().QueryRow(s.Context(), `SELECT state FROM notifications WHERE user_id = $1 AND kind = 'test'`,
		user.UUID()).Scan(&state); err != nil {
		s.Fatalf("flows: read the test push of %s: %v", user, err)
	}
	if state != want {
		s.Fatalf("flows: the test push of %s is %s, want %s", user, state, want)
	}
}

func wantNotificationRows(s *scenario.Scenario, user ids.UserID, want int) {
	var rows int
	if err := s.DB().QueryRow(s.Context(), `SELECT count(*) FROM notifications WHERE user_id = $1`,
		user.UUID()).Scan(&rows); err != nil {
		s.Fatalf("flows: count the notifications of %s: %v", user, err)
	}
	if rows != want {
		s.Fatalf("flows: %d notifications for %s, want %d", rows, user, want)
	}
}

func wantDeliveryCode(s *scenario.Scenario, user ids.UserID, want errs.Code) {
	var code string
	if err := s.DB().QueryRow(s.Context(), `SELECT code FROM event_deliveries
		WHERE handler = $1 AND event_id::text = $2`, pushHandler, triggerEvent(s, user)).Scan(&code); err != nil {
		s.Fatalf("flows: read the delivery of the test push of %s: %v", user, err)
	}
	if code != string(want) {
		s.Fatalf("flows: the test push of %s was delivered as %s, want %s", user, code, want)
	}
}

func wantNotificationsSent(s *scenario.Scenario, user ids.UserID, want int) {
	var sent int
	if err := s.DB().QueryRow(s.Context(), `SELECT count(*) FROM events
		WHERE type = 'notification.sent' AND payload->>'user_id' = $1`, user.String()).Scan(&sent); err != nil {
		s.Fatalf("flows: count notification.sent for %s: %v", user, err)
	}
	if sent != want {
		s.Fatalf("flows: %d notification.sent events for %s, want %d", sent, user, want)
	}
}
