//go:build faultpoints

package governance_test

import (
	"math/rand/v2"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const chaosProposers = 4

func chaosProposer(i int) uuid.UUID {
	return uuid.NewSHA1(uuid.Nil, []byte("proposer-"+strconv.Itoa(i)))
}

func TestBanVoids_convergesUnderChaos(t *testing.T) {
	t.Parallel()
	testkit.ConsumerSuite(t, func(h testkit.Harness) bus.Consumer {
		seedBanProposals(t, h)
		for _, c := range governance.New(module.Deps{
			Pool: h.Pool, UoW: db.New(h.Pool, h.IDs, h.Clock), IDs: h.IDs, Clock: h.Clock, Bus: h.Bus,
		}).Consumers() {
			if c.Durable == "governance" {
				return c
			}
		}
		t.Fatal("governance is not registered")
		return bus.Consumer{}
	}, func(_ *rand.Rand, i int) events.Event {
		if i < chaosProposers {
			return events.UserAccountStatusChanged{
				V: 1, UserID: chaosProposer(i), From: "active", To: "banned", Cause: "admin",
				At: time.Date(2026, 3, 1, 12, 0, i, 0, time.UTC),
			}
		}
		return events.UserAccountStatusChanged{
			V: 1, UserID: chaosProposer(i % chaosProposers), From: "banned", To: "active", Cause: "admin",
			At: time.Date(2026, 3, 1, 12, 0, i, 0, time.UTC),
		}
	})
}

func seedBanProposals(t *testing.T, h testkit.Harness) {
	t.Helper()
	q := sqlc.New(h.Pool)
	for i := range chaosProposers * 2 {
		proposer := chaosProposer(i % chaosProposers)
		id := chaosProposal(300 + i)
		_, err := q.InsertProposal(t.Context(), sqlc.InsertProposalParams{
			VoterIds: []uuid.UUID{chaosProposal(400 + i)}, ID: id, CabalID: chaosProposal(500 + i),
			ProposerID: proposer, Kind: "buy", Symbol: "AAPLx", Mint: aaplxMint,
			UsdcMicros: pgtype.Int8{Int64: 25_000_000, Valid: true}, QuoteOutAmount: 105_000_000,
			ExpiresAt: h.Clock.Now().Add(24 * time.Hour), CreatedAt: h.Clock.Now(), Threshold: "majority",
		})
		if err != nil {
			t.Fatal(err)
		}
		if i%2 == 1 {
			pass := `UPDATE proposals SET status = 'passed' WHERE id = $1`
			if _, err := h.Pool.Exec(t.Context(), pass, id); err != nil {
				t.Fatal(err)
			}
		}
	}
}
