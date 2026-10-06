package agents_test

import (
	"context"
	"errors"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/agents"
	"github.com/monaco/monaco/apps/backend/internal/modules/agents/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestAgentOf_returnsTheLiveAgentAndFalseForNoneOrOnlyRemovedOnes(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	queries := agents.New(module.Deps{Pool: pool}).Queries()
	cabal, other := newID(), newID()
	ask := func(c ids.CabalID) (agents.Agent, bool) {
		got, ok, err := queries.AgentOf(t.Context(), c)
		if err != nil {
			t.Fatal(err)
		}
		return got, ok
	}
	if got, ok := ask(ids.CabalIDFrom(cabal)); ok || got != (agents.Agent{}) {
		t.Fatalf("AgentOf(a cabal with no agent) = %+v, %v; want zero and false", got, ok)
	}
	if err := insertAgent(t, pool, newID(), cabal, "removed"); err != nil {
		t.Fatal(err)
	}
	if got, ok := ask(ids.CabalIDFrom(cabal)); ok || got != (agents.Agent{}) {
		t.Fatalf("AgentOf(a cabal with only a removed agent) = %+v, %v; want zero and false", got, ok)
	}
	live := newID()
	if err := insertAgent(t, pool, live, cabal, "paused"); err != nil {
		t.Fatal(err)
	}
	if err := run(
		t,
		pool,
		`UPDATE agents SET name = 'Atlas', budget_usdc_micros = 42000000 WHERE id = $1`,
		live,
	); err != nil {
		t.Fatal(err)
	}
	if err := insertAgent(t, pool, newID(), other, "active"); err != nil {
		t.Fatal(err)
	}
	want := agents.Agent{
		ID: ids.AgentIDFrom(live), CabalID: ids.CabalIDFrom(cabal), Name: "Atlas",
		Status: domain.StatusPaused, BudgetMicros: money.MicrosFromUint64(42_000_000),
	}
	if got, ok := ask(ids.CabalIDFrom(cabal)); !ok || got != want {
		t.Fatalf("AgentOf(a cabal with a paused agent) = %+v, %v; want %+v, true", got, ok, want)
	}
}

func isError(err error, code errs.Code, op string) bool {
	var e *errs.Error
	return errors.As(err, &e) && e.Code == code && e.Op == op
}

func TestAgentOf_refusesARowItCannotRead(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	queries := agents.New(module.Deps{Pool: pool}).Queries()
	cabal := newID()
	if err := run(t, pool, `ALTER TABLE agents DROP CONSTRAINT agents_budget_usdc_micros_check`); err != nil {
		t.Fatal(err)
	}
	if err := insertAgent(t, pool, newID(), cabal, "active"); err != nil {
		t.Fatal(err)
	}
	if err := run(t, pool, `UPDATE agents SET budget_usdc_micros = -1`); err != nil {
		t.Fatal(err)
	}
	got, ok, err := queries.AgentOf(t.Context(), ids.CabalIDFrom(cabal))
	if !isError(err, errs.CodeDecodeFailed, "agents.AgentOf") || ok || got != (agents.Agent{}) {
		t.Fatalf("AgentOf(a negative budget) = %+v, %v, %v; want zero, false and decode_failed from agents.AgentOf",
			got, ok, err)
	}
}

func TestAgentOf_failsInternalWhenTheDatabaseDoes(t *testing.T) {
	t.Parallel()
	queries := agents.New(module.Deps{Pool: testkit.DB(t)}).Queries()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	got, ok, err := queries.AgentOf(ctx, ids.CabalIDFrom(newID()))
	if !isError(err, errs.CodeInternal, "agents.AgentOf") || !errors.Is(err, context.Canceled) || ok ||
		got != (agents.Agent{}) {
		t.Fatalf("AgentOf(a cancelled context) = %+v, %v, %v; want zero, false and internal from agents.AgentOf "+
			"wrapping context.Canceled", got, ok, err)
	}
}
