package scenario

import (
	"context"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

func TestSeededAdmin_grantsTheRoleToAnActiveUser(t *testing.T) {
	t.Parallel()
	a := start(t, options{})
	s := newScenario(t, a.backend())
	s.Given(SeededAdmin("mod", "moderator"))
	var role, status string
	err := a.pool.QueryRow(t.Context(), `SELECT a.role, u.account_status FROM admins a
		JOIN users u ON u.id = a.user_id WHERE a.user_id = $1::uuid`, s.Recall("mod")).Scan(&role, &status)
	if err != nil || role != "moderator" || status != "active" {
		t.Fatalf("admin = %q %q, %v, want an active user with the moderator role", role, status, err)
	}
}

func TestSeededAdmin_failsOnAnUnknownRole(t *testing.T) {
	t.Parallel()
	a := start(t, options{})
	got := failure(t, t.Context, func(r T) *Scenario { return newScenario(r, a.backend()) }, SeededAdmin("mod", "root"))
	const want = `scenario: grant mod the root role: ERROR: new row for relation "admins" violates check constraint`
	if len(got) < len(want) || got[:len(want)] != want {
		t.Fatalf("failure = %q, want it to start with %q", got, want)
	}
}

func TestExpectAdminAction_wantsExactlyOneActionByAnAdmin(t *testing.T) {
	t.Parallel()
	a := start(t, options{})
	appendAction := func(actor, target string) {
		ctx := observability.WithActor(t.Context(), actor)
		err := a.db.Do(ctx, func(ctx context.Context, tx db.Tx) error {
			return tx.Events.Append(ctx, events.AdminAction{
				V: 1, ActionID: a.ids.NewV7(), AdminID: a.ids.NewV7(), Action: events.AdminActionPingFlag,
				TargetType: events.AdminTargetSystemPing, TargetID: target, Reason: "spam",
			})
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	appendAction("admin:a", "one")
	appendAction("user:u", "by-a-user")
	appendAction("admin:a", "twice")
	appendAction("admin:a", "twice")
	inProcess := func(r T) *Scenario { return newScenario(r, a.backend()) }
	for _, tc := range []struct{ target, want string }{
		{"one", ""},
		{"missing", "scenario: 0 admin.action events for ping_flag on missing by actors [], want exactly one by an admin"},
		{"by-a-user", "scenario: 1 admin.action events for ping_flag on by-a-user by actors [user], want exactly one by an admin"},
		{"twice", "scenario: 2 admin.action events for ping_flag on twice by actors [admin admin], want exactly one by an admin"},
	} {
		if got := failure(
			t,
			t.Context,
			inProcess,
			ExpectAdminAction(events.AdminActionPingFlag, tc.target),
		); got != tc.want {
			t.Errorf("%s: failure = %q, want %q", tc.target, got, tc.want)
		}
	}
}
