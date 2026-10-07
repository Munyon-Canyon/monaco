//go:build faultpoints

package governance_test

import (
	"math/rand/v2"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const chaosPassed = 8

func chaosProposal(i int) uuid.UUID {
	return uuid.NewSHA1(uuid.Nil, []byte("proposal-"+strconv.Itoa(i)))
}

func TestTradeOutcome_convergesUnderChaos(t *testing.T) {
	t.Parallel()
	testkit.ConsumerSuite(t, func(h testkit.Harness) bus.Consumer {
		seedChaosProposals(t, h)
		for _, c := range governance.New(module.Deps{Pool: h.Pool, IDs: h.IDs, Clock: h.Clock, Bus: h.Bus}).Consumers() {
			if c.Durable == "governance" {
				return c
			}
		}
		t.Fatal("governance is not registered")
		return bus.Consumer{}
	}, func(rng *rand.Rand, i int) events.Event {
		random := func() uuid.UUID { return uuid.NewSHA1(uuid.Nil, strconv.AppendUint(nil, rng.Uint64(), 10)) }
		source := events.TradeSource{Kind: "proposal", ID: chaosProposal(i)}
		switch {
		case i == chaosPassed || i == chaosPassed+1:
			source.ID = chaosProposal(chaosPassed)
		case i > chaosPassed+1:
			source = events.TradeSource{Kind: "cashout", ID: chaosProposal(i % chaosPassed)}
		}
		if i%2 == 0 {
			return events.TradeConfirmed{
				V: 1, SwapID: random(), CabalID: random(), Source: source, SourceBatchSize: 1, Action: "buy",
				Symbol: "AAPLx", InAmount: 25_000_000, OutAmount: rng.Uint64N(1e9),
				ConfirmedAt: time.Date(2026, 3, 1, 12, 0, i, 0, time.UTC),
			}
		}
		codes := []errs.Code{errs.CodeInsufficientFunds, errs.CodeSlippageExceeded, errs.CodeNoRoute}
		return events.TradeBlocked{
			V: 1, CabalID: random(), Source: source, SourceBatchSize: 1, Action: "buy", Symbol: "AAPLx",
			Code: codes[rng.IntN(len(codes))], Have: 10_000_000, Need: 25_000_000,
		}
	})
}

func seedChaosProposals(t *testing.T, h testkit.Harness) {
	t.Helper()
	q := sqlc.New(h.Pool)
	for i := range chaosPassed + 1 {
		_, err := q.InsertProposal(t.Context(), sqlc.InsertProposalParams{
			VoterIds: []uuid.UUID{chaosProposal(100 + i)}, ID: chaosProposal(i), CabalID: chaosProposal(200 + i),
			ProposerID: chaosProposal(100 + i), Kind: "buy", Symbol: "AAPLx", Mint: aaplxMint,
			UsdcMicros: pgtype.Int8{Int64: 25_000_000, Valid: true}, QuoteOutAmount: 105_000_000,
			ExpiresAt: h.Clock.Now().Add(24 * time.Hour), CreatedAt: h.Clock.Now(),
			Threshold: "majority",
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	_, err := h.Pool.Exec(t.Context(),
		`UPDATE proposals SET status = CASE WHEN id = $1 THEN 'voided' ELSE 'passed' END`, chaosProposal(chaosPassed))
	if err != nil {
		t.Fatal(err)
	}
}
