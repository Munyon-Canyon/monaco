//go:build faultpoints

package social_test

import (
	"math/rand/v2"
	"strconv"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/social"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func revokeChaosConsumer(t *testing.T, h testkit.Harness) bus.Consumer {
	t.Helper()
	deps := module.Deps{Pool: h.Pool, UoW: db.New(h.Pool, h.IDs, h.Clock), IDs: h.IDs, Clock: h.Clock, Bus: h.Bus}
	for _, c := range social.New(deps, social.WithRealtime(&testkit.FakeRealtime{})).Consumers() {
		if c.Durable == "social_realtime_revoke" {
			return c
		}
	}
	t.Fatal("social_realtime_revoke is not registered")
	return bus.Consumer{}
}

func revokeChaosEvent(_ *rand.Rand, i int) events.Event {
	user := uuid.NewSHA1(uuid.Nil, []byte(strconv.Itoa(i%3)))
	switch i % 3 {
	case 0:
		return events.CabalMemberLeft{V: 1, CabalID: uuid.NewSHA1(uuid.Nil, []byte("cabal")), UserID: user}
	case 1:
		return events.UserAccountStatusChanged{V: 1, UserID: user, From: "active", To: "banned", Cause: "admin"}
	default:
		return events.UserDeleted{V: 1, UserID: user}
	}
}

func TestRealtimeRevoke_Redelivery_NoDuplicate(t *testing.T) {
	t.Parallel()
	testkit.ConsumerSuite(t, func(h testkit.Harness) bus.Consumer {
		return revokeChaosConsumer(t, h)
	}, revokeChaosEvent)
}
