//go:build faultpoints

package ranking_test

import (
	"math/rand/v2"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	governanceport "github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type chaosCalls struct {
	cabal, alice, bob, p0, p1, p2, p3 uuid.UUID
	base                              time.Time
}

func newChaosCalls() chaosCalls {
	named := func(name string) uuid.UUID { return uuid.NewSHA1(uuid.Nil, []byte("calls/"+name)) }
	return chaosCalls{
		cabal: named("cabal"), alice: named("alice"), bob: named("bob"),
		p0: named("p0"), p1: named("p1"), p2: named("p2"), p3: named("p3"),
		base: time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC),
	}
}

func (c chaosCalls) ballots() callsBallots {
	out := callsBallots{}
	for _, p := range []uuid.UUID{c.p0, c.p1, c.p2, c.p3} {
		out[p] = []governanceport.Ballot{
			ballotOf(c.alice, "yes", c.base.Add(time.Minute)), ballotOf(c.bob, "no", c.base.Add(2*time.Minute)),
		}
	}
	return out
}

func (c chaosCalls) created(p uuid.UUID) events.Event {
	return events.ProposalCreated{V: 1, ProposalID: p, CabalID: c.cabal, Kind: "buy", Symbol: "AAPLx", Mint: callsMint}
}

func (c chaosCalls) cast(p, voter uuid.UUID, choice string, at time.Duration) events.Event {
	return events.VoteCast{
		V:          1,
		ProposalID: p,
		CabalID:    c.cabal,
		VoterID:    voter,
		Choice:     choice,
		CastAt:     c.base.Add(at),
	}
}

func (c chaosCalls) fill() events.Event {
	return events.TradeConfirmed{
		V: 1, SwapID: uuid.NewSHA1(uuid.Nil, []byte("calls/swap")), CabalID: c.cabal,
		Source: events.TradeSource{Kind: "proposal", ID: c.p0}, Action: "buy",
		InMint: chain.SolanaAddress(callsUSDC), InAmount: 716_520_000, OutMint: callsMint, OutAmount: 1_000_000,
		USDCMicros: money.MicrosFromUint64(716_520_000), ConfirmedAt: c.base.Add(time.Hour),
	}
}

func (c chaosCalls) event(i int) events.Event {
	return []events.Event{
		c.created(c.p0),
		c.cast(c.p0, c.alice, "yes", time.Minute),
		c.cast(c.p0, c.bob, "no", 2*time.Minute),
		events.ProposalPassed{V: 1, ProposalID: c.p0, CabalID: c.cabal, Kind: "buy", Symbol: "AAPLx", Mint: callsMint},
		c.fill(),
		c.cast(c.p1, c.bob, "yes", 3*time.Minute),
		events.ProposalFailed{V: 1, ProposalID: c.p1, CabalID: c.cabal},
		c.created(c.p2),
		events.ProposalWithdrawn{V: 1, ProposalID: c.p2, CabalID: c.cabal},
		c.cast(c.p3, c.alice, "yes", time.Minute),
		events.ProposalExpired{V: 1, ProposalID: c.p3, CabalID: c.cabal},
		c.cast(c.p0, c.alice, "no", 0),
	}[i%12]
}

func TestCalls_convergeUnderChaos(t *testing.T) {
	t.Parallel()
	c := newChaosCalls()
	testkit.ConsumerSuite(t, func(h testkit.Harness) bus.Consumer {
		ports := app.Ports{
			Market:  newCallsMarket(t, uuid.NewSHA1(uuid.Nil, []byte("calls/asset")), 700_000_000, c.base),
			Ballots: c.ballots(),
		}
		deps := module.Deps{Pool: h.Pool, IDs: h.IDs, Clock: h.Clock, Bus: h.Bus}
		for _, consumer := range ranking.New(deps, ranking.WithPorts(ports)).Consumers() {
			if consumer.Durable == "ranking_calls" {
				return consumer
			}
		}
		t.Fatal("ranking_calls is not registered")
		return bus.Consumer{}
	}, func(_ *rand.Rand, i int) events.Event { return c.event(i) })
}
