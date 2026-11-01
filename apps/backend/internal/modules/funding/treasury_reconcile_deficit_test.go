package funding_test

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	treasuryport "github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

const deficitMessage = "funding.reconcile.deficit"

func (e *reconcileEnv) tickLogged(t *testing.T, p *app.TreasuryReconcilePoller, logs *bytes.Buffer) poller.Report {
	t.Helper()
	ctx := observability.WithLogger(t.Context(), slog.New(slog.NewJSONHandler(logs, nil)))
	ctx = observability.WithActor(ctx, "system:poller.funding.treasury-reconcile")
	report, err := p.Tick(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return report
}

func TestTreasuryReconcile_ADeficitThatPersistsAcrossTwoTicksAlerts(t *testing.T) {
	t.Parallel()
	env := newReconcileEnv(t)
	treasury := env.cabal.TreasuryAddress
	env.ledger[env.cabal.ID] = []treasuryport.Position{{Mint: env.usdc, Units: money.NewBaseUnits(100_000_000, 6)}}
	env.onChain(treasury, env.usdc, 33_000_000)
	p := app.NewTreasuryReconcilePoller(env.deps)
	var logs bytes.Buffer

	first := env.tickLogged(t, p, &logs)
	if strings.Contains(logs.String(), deficitMessage) || attr(first, "deficits") != 1 {
		t.Fatalf("first tick deficits = %d, logs = %s, want one deficit counted and no alert yet",
			attr(first, "deficits"), logs.String())
	}
	second := env.tickLogged(t, p, &logs)

	out := logs.String()
	if attr(second, "deficits") != 1 || strings.Count(out, deficitMessage) != 1 ||
		!strings.Contains(out, `"ledger_units":"100000000"`) || !strings.Contains(out, `"chain_units":"33000000"`) {
		t.Fatalf("second tick deficits = %d, logs = %s, want one alert naming 100000000 booked and 33000000 on chain",
			attr(second, "deficits"), out)
	}
	if len(env.chain.queries) != 0 {
		t.Fatalf("a deficit read %d signature pages, want none", len(env.chain.queries))
	}
}

func TestTreasuryReconcile_ADeficitThatHealsBetweenTicksNeverAlerts(t *testing.T) {
	t.Parallel()
	env := newReconcileEnv(t)
	treasury := env.cabal.TreasuryAddress
	env.ledger[env.cabal.ID] = []treasuryport.Position{{Mint: env.usdc, Units: money.NewBaseUnits(100_000_000, 6)}}
	p := app.NewTreasuryReconcilePoller(env.deps)
	var logs bytes.Buffer

	for _, onChain := range []uint64{33_000_000, 100_000_000, 33_000_000, 100_000_000} {
		env.onChain(treasury, env.usdc, onChain)
		env.tickLogged(t, p, &logs)
	}

	if strings.Contains(logs.String(), deficitMessage) {
		t.Fatalf("logs = %s, want no alert for a settlement lag that closed on the next tick", logs.String())
	}
}
