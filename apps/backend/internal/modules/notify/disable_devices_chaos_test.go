//go:build faultpoints

package notify_test

import (
	"math/rand/v2"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestDisableDevices_convergesUnderChaos(t *testing.T) {
	t.Parallel()
	seeded := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	testkit.ConsumerSuite(t, func(h testkit.Harness) bus.Consumer {
		for i := range chaosUsers {
			if _, err := h.Pool.Exec(t.Context(), `INSERT INTO users
				(id, privy_user_id, login_provider, auth_state_changed_at, account_status, created_at, updated_at)
				VALUES ($1, $2, 'sms', $3, 'active', $3, $3)`,
				chaosRecipient(i), "did:privy:disable-chaos-"+chaosRecipient(i).String(), seeded); err != nil {
				t.Fatalf("seed user %d: %v", i, err)
			}
			if _, err := h.Pool.Exec(
				t.Context(),
				`INSERT INTO device_tokens
				(id, user_id, token, environment, created_at, last_seen_at) VALUES ($1, $2, $3, 'sandbox', $4, $4)`,
				chaosRecipient(
					i+100,
				),
				chaosRecipient(i),
				"disable-chaos-"+chaosRecipient(i).String(),
				seeded,
			); err != nil {
				t.Fatalf("seed token %d: %v", i, err)
			}
		}
		deps := module.Deps{Pool: h.Pool, IDs: h.IDs, Clock: h.Clock, UoW: db.New(h.Pool, h.IDs, h.Clock)}
		for _, c := range notify.New(deps, notify.WithSender(&testkit.FakeSender{})).Consumers() {
			if c.Durable == "notify_disable_devices" {
				return c
			}
		}
		t.Fatal("notify_disable_devices is not registered")
		return bus.Consumer{}
	}, func(rng *rand.Rand, i int) events.Event {
		user := chaosRecipient(i % chaosUsers)
		if rng.IntN(2) == 0 {
			return events.UserSessionsRevoked{V: 1, UserID: user, Scope: "all_sessions", At: seeded}
		}
		return events.UserDeleted{V: 1, UserID: user, At: seeded}
	})
}
