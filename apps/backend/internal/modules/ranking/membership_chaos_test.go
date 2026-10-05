//go:build faultpoints

package ranking_test

import (
	"math/rand/v2"
	"strconv"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func rankingConsumer(t *testing.T, h testkit.Harness, durable string) bus.Consumer {
	t.Helper()
	for _, c := range ranking.New(module.Deps{Pool: h.Pool, IDs: h.IDs, Clock: h.Clock, Bus: h.Bus}).Consumers() {
		if c.Durable == durable {
			return c
		}
	}
	t.Fatalf("%s is not registered", durable)
	return bus.Consumer{}
}

func TestMembership_convergesUnderChaos(t *testing.T) {
	t.Parallel()
	testkit.ConsumerSuite(t, func(h testkit.Harness) bus.Consumer {
		return rankingConsumer(t, h, "ranking_membership")
	}, func(rng *rand.Rand, i int) events.Event {
		cabal := uuid.NewSHA1(uuid.Nil, []byte("cabal"+strconv.Itoa(i)))
		user := uuid.NewSHA1(uuid.Nil, strconv.AppendUint(nil, rng.Uint64(), 10))
		if i%2 == 1 {
			return events.CabalMemberLeft{V: 1, CabalID: cabal, UserID: user}
		}
		return events.CabalMemberJoined{V: 1, CabalID: cabal, UserID: user, Role: "member", Via: "open"}
	})
}
