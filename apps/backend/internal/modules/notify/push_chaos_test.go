//go:build faultpoints

package notify_test

import (
	"math/rand/v2"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func chaosRecipient(i int) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte("notify-chaos-user-"+strconv.Itoa(i)))
}

func TestPush_convergesUnderChaosWithATokenlessAndADeletedUser(t *testing.T) {
	t.Parallel()
	const users, tokenless, deleted = 4, 2, 3
	seeded := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	testkit.ConsumerSuite(t, func(h testkit.Harness) bus.Consumer {
		for i := range users {
			status := "active"
			if i == deleted {
				status = "deleted"
			}
			if _, err := h.Pool.Exec(t.Context(), `INSERT INTO users
				(id, privy_user_id, login_provider, auth_state_changed_at, account_status, created_at, updated_at,
				 deleted_at)
				VALUES ($1, $2, 'sms', $3, $4, $3, $3, CASE WHEN $4 = 'deleted' THEN $3::timestamptz END)`,
				chaosRecipient(i), "did:privy:notify-chaos-"+strconv.Itoa(i), seeded, status); err != nil {
				t.Fatalf("seed user %d: %v", i, err)
			}
			if i == tokenless {
				continue
			}
			if _, err := h.Pool.Exec(t.Context(), `INSERT INTO device_tokens
				(id, user_id, token, environment, created_at, last_seen_at) VALUES ($1, $2, $3, 'sandbox', $4, $4)`,
				uuid.NewSHA1(uuid.NameSpaceOID, []byte("notify-chaos-token-"+strconv.Itoa(i))), chaosRecipient(i),
				strings.Repeat(strconv.Itoa(i), 64), seeded); err != nil {
				t.Fatalf("seed token %d: %v", i, err)
			}
		}
		seedChaosNudgeStates(t, h)
		deps := module.Deps{Pool: h.Pool, IDs: h.IDs, Clock: h.Clock, UoW: db.New(h.Pool, h.IDs, h.Clock)}
		return notify.New(deps, notify.WithSender(&testkit.FakeSender{})).Consumers()[0]
	}, func(_ *rand.Rand, i int) events.Event { return chaosPushEvent(i) })
}

func chaosPushEvent(i int) events.Event {
	if i < chaosUsers {
		return events.NotifyTestRequested{V: 1, UserID: chaosRecipient(i)}
	}
	return chaosNudge(i)
}
