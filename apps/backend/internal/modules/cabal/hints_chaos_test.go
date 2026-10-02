//go:build faultpoints

package cabal_test

import (
	"math/rand/v2"
	"strconv"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestHints_convergesUnderChaos(t *testing.T) {
	t.Parallel()
	testkit.ConsumerSuite(t, func(h testkit.Harness) bus.Consumer {
		for _, c := range cabal.New(module.Deps{Pool: h.Pool, IDs: h.IDs, Clock: h.Clock, Bus: h.Bus}).Consumers() {
			if c.Durable == "cabal_hints" {
				return c
			}
		}
		t.Fatal("cabal_hints is not registered")
		return bus.Consumer{}
	}, func(rng *rand.Rand, _ int) events.Event {
		id := uuid.NewSHA1(uuid.Nil, strconv.AppendUint(nil, rng.Uint64(), 10))
		user := uuid.NewSHA1(uuid.Nil, strconv.AppendUint(nil, rng.Uint64(), 10))
		return events.CabalCreated{
			V: 1, CabalID: id, CreatorID: user, Name: "Friends pot",
			JoinMode: "open", VoterMode: "all", Threshold: "majority",
			ProposalExpirySeconds: domain.ExpiryDay, SlippageBps: domain.DefaultSlippageBps,
			TreasuryAddress: "Dht9c9YfstFWkNYXgqr8HZbhqVn563bCpNU6zL32Ftqf",
		}
	})
}
