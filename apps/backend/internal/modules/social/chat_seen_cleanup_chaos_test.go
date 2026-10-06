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

const seenChaosPairs = 6

func seenChaosID(label string) uuid.UUID { return uuid.NewSHA1(uuid.Nil, []byte(label)) }

func seenChaosPair(i int) (cabal, user uuid.UUID) {
	return seenChaosID("cabal"), seenChaosID("user" + strconv.Itoa(i))
}

func seenChaosConsumer(t *testing.T, h testkit.Harness) bus.Consumer {
	t.Helper()
	for i := range seenChaosPairs {
		cabal, user := seenChaosPair(i)
		if _, err := h.Pool.Exec(t.Context(),
			`INSERT INTO chat_seen (cabal_id, user_id, last_seen_at) VALUES ($1, $2, $3)`,
			cabal, user, h.Clock.Now()); err != nil {
			t.Fatalf("seed the watermark %d: %v", i, err)
		}
	}
	deps := module.Deps{Pool: h.Pool, UoW: db.New(h.Pool, h.IDs, h.Clock), IDs: h.IDs, Clock: h.Clock, Bus: h.Bus}
	for _, c := range social.New(deps).Consumers() {
		if c.Durable == "social_chat_seen_cleanup" {
			return c
		}
	}
	t.Fatal("social_chat_seen_cleanup is not registered")
	return bus.Consumer{}
}

func seenChaosEvent(_ *rand.Rand, i int) events.Event {
	cabal, user := seenChaosPair(i % (seenChaosPairs - 1))
	return events.CabalMemberLeft{V: 1, CabalID: cabal, UserID: user}
}

func TestChatSeenCleanup_Redelivery_NoDuplicate(t *testing.T) {
	t.Parallel()
	testkit.ConsumerSuite(t, func(h testkit.Harness) bus.Consumer {
		return seenChaosConsumer(t, h)
	}, seenChaosEvent)
}
