//go:build faultpoints

package ranking_test

import (
	"math/rand/v2"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

type chaosWorld struct {
	cabal                           uuid.UUID
	bought, withdrawn, voided, late uuid.UUID
	alice, bob, carol               uuid.UUID
	epoch, older, newer             time.Time
	tsla                            chain.SolanaAddress
}

func newChaosWorld() chaosWorld {
	id := func(name string) uuid.UUID { return uuid.NewSHA1(uuid.Nil, []byte("calls-"+name)) }
	epoch := time.Date(2026, time.March, 1, 12, 0, 0, 0, time.UTC)
	return chaosWorld{
		cabal: id("cabal"), bought: id("bought"), withdrawn: id("withdrawn"), voided: id("voided"), late: id("late"),
		alice: id("alice"), bob: id("bob"), carol: id("carol"),
		epoch: epoch, older: epoch.Add(-2 * time.Minute), newer: epoch.Add(-time.Minute),
		tsla: chain.SolanaAddress(marketfake.TSLAx().Mint.String()),
	}
}

func (w chaosWorld) consumer(t *testing.T, h testkit.Harness) bus.Consumer {
	t.Helper()
	prices := &marketfake.PricesFake{}
	prices.Set(marketfake.TSLAx().ID, money.MicrosFromUint64(700_000_000), h.Clock.Now().Add(-time.Hour))
	ports := app.Ports{
		Market: callsMarket{marketfake.NewCatalog(marketfake.Fixtures()...), prices},
		Ballots: ballotsFake{
			w.bought: {ballot(w.alice, "no", w.newer), ballot(w.bob, "yes", w.older), ballot(w.carol, "", time.Time{})},
			w.late:   {ballot(w.carol, "no", w.older)},
		},
	}
	deps := module.Deps{Pool: h.Pool, IDs: h.IDs, Clock: h.Clock, Bus: h.Bus}
	for _, c := range ranking.New(deps, ranking.WithPorts(ports)).Consumers() {
		if c.Durable == "ranking_calls" {
			return c
		}
	}
	t.Fatal("ranking_calls is not registered")
	return bus.Consumer{}
}

func (w chaosWorld) vote(proposal, voter uuid.UUID, choice string, at time.Time) events.Event {
	return events.VoteCast{V: 1, ProposalID: proposal, CabalID: w.cabal, VoterID: voter, Choice: choice, CastAt: at}
}

func (w chaosWorld) created(proposal uuid.UUID) events.Event {
	return events.ProposalCreated{
		V: 1, ProposalID: proposal, CabalID: w.cabal, ProposerID: w.alice, Kind: "buy", Symbol: "TSLAx",
		Mint: w.tsla, USDCMicros: money.MicrosFromUint64(25_000_000), VoterCount: 3,
	}
}

func (w chaosWorld) passed(proposal uuid.UUID) events.Event {
	return events.ProposalPassed{
		V: 1, ProposalID: proposal, CabalID: w.cabal, Kind: "buy", Symbol: "TSLAx", Mint: w.tsla,
	}
}

func (w chaosWorld) fill(proposal uuid.UUID, price uint64, at time.Time) events.Event {
	return events.TradeConfirmed{
		V: 1, SwapID: proposal, CabalID: w.cabal, Source: events.TradeSource{Kind: "proposal", ID: proposal},
		Action: "buy", Symbol: "TSLAx", InMint: usdcMint, InAmount: price, OutMint: w.tsla, OutAmount: 100_000_000,
		USDCMicros: money.MicrosFromUint64(price), ConfirmedAt: at,
	}
}

func (w chaosWorld) history() []events.Event {
	return []events.Event{
		w.created(w.bought),
		w.vote(w.bought, w.alice, "yes", w.older),
		w.vote(w.bought, w.alice, "no", w.newer),
		w.vote(w.bought, w.bob, "yes", w.older),
		w.passed(w.bought),
		w.fill(w.bought, 716_520_000, w.newer),
		w.created(w.withdrawn),
		events.ProposalWithdrawn{V: 1, ProposalID: w.withdrawn, CabalID: w.cabal, ProposerID: w.alice},
		w.passed(w.late),
		w.fill(w.late, 700_000_000, w.older),
		w.vote(w.late, w.carol, "no", w.older),
		events.ProposalVoided{V: 1, ProposalID: w.voided, CabalID: w.cabal, ActorType: "admin", Reason: "Banned."},
	}
}

func TestCalls_convergeUnderChaos(t *testing.T) {
	t.Parallel()
	w := newChaosWorld()
	history := w.history()
	testkit.ConsumerSuite(t, func(h testkit.Harness) bus.Consumer {
		return w.consumer(t, h)
	}, func(_ *rand.Rand, i int) events.Event { return history[i] })
}
