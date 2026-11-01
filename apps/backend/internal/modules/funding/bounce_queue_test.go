package funding_test

import (
	"encoding/json"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/adapters"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/fundingapi"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

const bouncesPath = "/v1/admin/queues/bounces"

type seededDeposit struct {
	name, status, returnAddress string
	cabal                       uuid.UUID
	minutes                     int
}

func seedExternalDeposits(
	t *testing.T, pool *pgxpool.Pool, at time.Time, rows []seededDeposit,
) map[string]uuid.UUID {
	t.Helper()
	gen := testkit.NewIDs(31)
	ids := map[string]uuid.UUID{}
	for i, row := range rows {
		ids[row.name] = gen.NewV7()
		if _, err := pool.Exec(
			t.Context(),
			`INSERT INTO external_deposits (id, signature, cabal_id, sender, mint, amount,
			source, status, return_address, bounce_attempts, detected_at)
			VALUES ($1, $2, $3, 'sender', 'mint', 25000000, 'reconcile', $4, NULLIF($5, ''), $6, $7)`,
			ids[row.name],
			"sig-"+row.name,
			row.cabal,
			row.status,
			row.returnAddress,
			i,
			at.Add(time.Duration(row.minutes)*time.Minute),
		); err != nil {
			t.Fatal(err)
		}
	}
	return ids
}

func TestBounceQueue_ListsOpenExternalDepositsOldestFirstWithTheirIDs(t *testing.T) {
	t.Parallel()
	s := flow15Scenario(t)
	older, newer, bystander := testkit.NewCabal(t, s.DB()), testkit.NewCabal(t, s.DB()), testkit.NewCabal(t, s.DB())
	at := clock.Real{}.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	ids := seedExternalDeposits(t, s.DB(), at, []seededDeposit{
		{"newer", "detected", "", newer.ID.UUID(), 3},
		{"older", "bounce_failed", "ReturnAddress1111111111111111111111111111111", older.ID.UUID(), 1},
		{"bouncing", "bouncing", "", bystander.ID.UUID(), 2},
		{"returned", "returned", "", bystander.ID.UUID(), 0},
		{"held", "held", "", bystander.ID.UUID(), 0},
	})
	s.Given(scenario.SeededAdmin("admin", "viewer"), scenario.AsUser("admin")).
		When(scenario.Get(bouncesPath), scenario.ExpectStatus(http.StatusOK)).
		Then(scenario.ExpectField("items", func(s *scenario.Scenario, raw json.RawMessage) {
			var items []api.PendingBounce
			if err := json.Unmarshal(raw, &items); err != nil || len(items) != 3 {
				s.Fatalf("items = %s (%v), want the three open deposits", raw, err)
			}
			got := []uuid.UUID{items[0].ExternalDepositId, items[1].ExternalDepositId, items[2].ExternalDepositId}
			if want := []uuid.UUID{ids["older"], ids["bouncing"], ids["newer"]}; !slices.Equal(got, want) {
				s.Fatalf("ids = %v, want %v by age", got, want)
			}
			assertFailedBounceRow(s, items[0], older.ID.UUID(), at.Add(time.Minute))
			if items[2].ReturnAddress != nil {
				s.Fatalf("detected deposit = %+v, want no return address", items[2])
			}
		}))
	s.When(scenario.Get(bouncesPath+"?limit=1"), scenario.ExpectStatus(http.StatusOK)).
		Then(scenario.ExpectField("items", func(s *scenario.Scenario, raw json.RawMessage) {
			var items []api.PendingBounce
			if err := json.Unmarshal(raw, &items); err != nil || len(items) != 1 ||
				items[0].ExternalDepositId != ids["older"] {
				s.Fatalf("items = %s (%v), want only the oldest", raw, err)
			}
		}))
}

func assertFailedBounceRow(s *scenario.Scenario, got api.PendingBounce, cabal uuid.UUID, since time.Time) {
	s.Helper()
	if got.CabalId != cabal || got.Status != "bounce_failed" || got.Amount != "25000000" || got.Mint != "mint" ||
		got.Attempts != 1 || !got.Since.Equal(since) {
		s.Fatalf("failed bounce = %+v, want cabal %s, bounce_failed, 25000000 of mint, 1 attempt, since %s",
			got, cabal, since)
	}
	if got.ReturnAddress == nil || *got.ReturnAddress != "ReturnAddress1111111111111111111111111111111" {
		s.Fatalf("return address = %v, want the one set", got.ReturnAddress)
	}
}

func TestBounceQueue_IsEmptyWhenNothingWaitsForABounce(t *testing.T) {
	t.Parallel()
	s := flow15Scenario(t)
	s.Given(scenario.SeededAdmin("admin", "viewer"), scenario.AsUser("admin")).
		When(scenario.Get(bouncesPath), scenario.ExpectStatus(http.StatusOK)).
		Then(scenario.ExpectJSON("items", []any{}))
}

func TestBounceQueue_ReportsAFailedRead(t *testing.T) {
	t.Parallel()
	env := newPauseEnv(t)
	closed, err := pgxpool.NewWithConfig(t.Context(), env.pool.Config())
	if err != nil {
		t.Fatal(err)
	}
	closed.Close()
	_, err = adapters.HTTP{Reads: closed}.GetBounceQueue(t.Context(), api.GetBounceQueueRequestObject{})
	if errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Fatalf("GetBounceQueue err = %v, want %s", err, errs.CodeDBUnavailable)
	}
}
