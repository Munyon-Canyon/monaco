package funding_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/adapters"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/fundingapi"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

const bouncesPath = "/v1/admin/queues/bounces"

func TestBounceQueue_ListsOpenExternalDepositPausesOldestFirst(t *testing.T) {
	t.Parallel()
	s := flow15Scenario(t)
	older, newer, bystander := testkit.NewCabal(t, s.DB()), testkit.NewCabal(t, s.DB()), testkit.NewCabal(t, s.DB())
	at := clock.Real{}.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	for i, row := range []struct {
		cabal    any
		reason   string
		resolved *time.Time
	}{
		{newer.ID.UUID(), "external_deposit", nil},
		{older.ID.UUID(), "external_deposit", nil},
		{bystander.ID.UUID(), "ops", nil},
		{bystander.ID.UUID(), "external_deposit", &at},
		{nil, "external_deposit", nil},
	} {
		if _, err := s.DB().Exec(t.Context(), `INSERT INTO cabal_pauses (id, cabal_id, reason, created_at, resolved_at)
			VALUES (gen_random_uuid(), $1, $2, $3, $4)`, row.cabal, row.reason, at.Add(time.Duration(2-i)*time.Minute), row.resolved,
		); err != nil {
			t.Fatal(err)
		}
	}
	s.Given(scenario.SeededAdmin("admin", "viewer"), scenario.AsUser("admin")).
		When(scenario.Get(bouncesPath), scenario.ExpectStatus(http.StatusOK)).
		Then(scenario.ExpectField("items", func(s *scenario.Scenario, raw json.RawMessage) {
			var items []api.PendingBounce
			if err := json.Unmarshal(raw, &items); err != nil || len(items) != 2 ||
				items[0].CabalId != older.ID.UUID() || items[1].CabalId != newer.ID.UUID() ||
				!items[0].Since.Equal(at.Add(time.Minute)) || !items[1].Since.Equal(at.Add(2*time.Minute)) {
				s.Fatalf("items = %s (%v), want the older cabal first and only open external-deposit pauses", raw, err)
			}
		}))
	s.When(scenario.Get(bouncesPath+"?limit=1"), scenario.ExpectStatus(http.StatusOK)).
		Then(scenario.ExpectField("items", func(s *scenario.Scenario, raw json.RawMessage) {
			var items []api.PendingBounce
			if err := json.Unmarshal(
				raw,
				&items,
			); err != nil || len(items) != 1 ||
				items[0].CabalId != older.ID.UUID() {
				s.Fatalf("items = %s (%v), want only the oldest", raw, err)
			}
		}))
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
