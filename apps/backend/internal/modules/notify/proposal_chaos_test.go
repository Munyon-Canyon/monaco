//go:build faultpoints

package notify_test

import (
	"context"
	"math/rand/v2"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	governanceport "github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

const chaosMint = chain.SolanaAddress("XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp")

type everyProposal []governanceport.Ballot

func (v everyProposal) Ballots(context.Context, ids.ProposalID) ([]governanceport.Ballot, error) {
	return v, nil
}

type everyHead struct{}

func (everyHead) ProposalHead(context.Context, ids.ProposalID) (governanceport.ProposalHead, error) {
	return governanceport.ProposalHead{Kind: "buy", Symbol: "AAPLx", Thesis: "chaos"}, nil
}

func chaosProposal(i int) events.Event {
	round := i / 2
	id := uuid.NewSHA1(uuid.NameSpaceOID, []byte("notify-chaos-proposal-"+strconv.Itoa(i)))
	cabalID, proposer := chaosCabal(round%2), chaosRecipient(round)
	symbol := []string{"AAPLx", "TSLAx", "ZZZZx"}[round%3]
	usdc := money.MicrosFromUint64([]uint64{1_250_000, 12_500_000, 9_999}[round%3])
	kind, tokens, quote := "buy", uint64(0), uint64(105_000_000)
	if i%2 == 1 {
		kind, usdc, tokens, quote = "sell", money.Micros{}, 300_000_000, 71_250_000
	}
	return events.ProposalCreated{
		V: 1, ProposalID: id, CabalID: cabalID, ProposerID: proposer, Kind: kind, Symbol: symbol, Mint: chaosMint,
		USDCMicros: usdc, TokenAmount: tokens, QuoteOutAmount: quote,
		ExpiresAt: time.Date(2026, 3, 2, 12, 0, 0, 0, time.UTC), VoterCount: 3,
	}
}

func TestNotify_ProposalKinds_ConvergeUnderChaos(t *testing.T) {
	t.Parallel()
	testkit.ConsumerSuite(t, func(h testkit.Harness) bus.Consumer {
		seedChaosUsers(t, h)
		deps := module.Deps{Pool: h.Pool, IDs: h.IDs, Clock: h.Clock, UoW: db.New(h.Pool, h.IDs, h.Clock)}
		user := func(i int) ids.UserID { return ids.UserIDFrom(chaosRecipient(i)) }
		ballots := everyProposal{
			eligibleBallot(user(3)),
			eligibleBallot(user(2)),
			eligibleBallot(user(0)),
			eligibleBallot(user(1)),
		}
		m := notify.New(deps, notify.WithSender(&testkit.FakeSender{}), notify.WithCabals(chaosCabals()),
			notify.WithBallots(ballots), notify.WithProposalHeads(everyHead{}),
			notify.WithAssets(marketfake.NewCatalog(marketfake.Fixtures()...)))
		return m.Consumers()[0]
	}, func(_ *rand.Rand, i int) events.Event { return chaosProposal(i) })
}
