package market_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/adapters/coingecko"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

const budgetLimit = 100

type budgetRig struct {
	*historyRig
	budget *app.Budget
}

func newBudgetRig(t *testing.T) *budgetRig {
	t.Helper()
	r := newHistoryRig(t)
	return &budgetRig{historyRig: r, budget: app.NewBudget(r.pool, r.clock, budgetLimit)}
}

func (r *budgetRig) setCalls(t *testing.T, calls int) {
	t.Helper()
	now := r.clock.Now().UTC()
	r.exec(t, `INSERT INTO coingecko_usage (month, calls, updated_at) VALUES ($1, $2, $3)
		ON CONFLICT (month) DO UPDATE SET calls = excluded.calls`,
		time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC), calls, now)
}

func (r *budgetRig) usage(t *testing.T) app.BudgetUsage {
	t.Helper()
	u, err := r.budget.Usage(r.ctx(t))
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestBudget_everyCallCountsAndTwoClientsShareOneCount(t *testing.T) {
	t.Parallel()
	r := newBudgetRig(t)
	other := app.NewBudget(r.pool, r.clock, budgetLimit)
	ctx := r.ctx(t)
	r.budget.Spend(ctx)
	other.Spend(ctx)
	r.budget.Spend(ctx)
	for name, b := range map[string]*app.Budget{"first": r.budget, "second": other} {
		u, err := b.Usage(ctx)
		if err != nil || u.Calls != 3 || u.Limit != budgetLimit || u.Level != app.BudgetOpen {
			t.Fatalf("%s usage = %+v, %v, want 3 of %d and open", name, u, err, budgetLimit)
		}
	}
}

func TestBudget_theSoftCapStartsAt80PercentAndTheHardCapAt95(t *testing.T) {
	t.Parallel()
	r := newBudgetRig(t)
	ctx := r.ctx(t)
	for _, tc := range []struct {
		calls int
		want  app.BudgetLevel
	}{
		{0, app.BudgetOpen},
		{79, app.BudgetOpen},
		{80, app.BudgetSoft},
		{94, app.BudgetSoft},
		{95, app.BudgetHard},
		{100, app.BudgetHard},
		{250, app.BudgetHard},
	} {
		r.setCalls(t, tc.calls)
		if got := r.budget.Level(ctx); got != tc.want {
			t.Fatalf("level at %d calls = %s, want %s", tc.calls, got, tc.want)
		}
	}
}

func TestBudget_aZeroLimitNeverLimitsAndANilBudgetNeverCounts(t *testing.T) {
	t.Parallel()
	r := newBudgetRig(t)
	ctx := r.ctx(t)
	r.setCalls(t, 5_000)
	if got := app.NewBudget(r.pool, r.clock, 0).Level(ctx); got != app.BudgetOpen {
		t.Fatalf("level with no limit = %s, want open", got)
	}
	var none *app.Budget
	none.Spend(ctx)
	off := app.NewBudget(r.pool, r.clock, 0)
	off.Spend(ctx)
	if got := none.Level(ctx); got != app.BudgetOpen || r.usage(t).Calls != 5_000 {
		t.Fatalf("nil budget level = %s, calls = %d, want open and untouched", got, r.usage(t).Calls)
	}
}

func TestBudget_aNewMonthStartsFromZero(t *testing.T) {
	t.Parallel()
	r := newBudgetRig(t)
	r.setCalls(t, 99)
	if got := r.usage(t); got.Calls != 99 || got.Level != app.BudgetHard {
		t.Fatalf("usage = %+v, want 99 calls and the hard cap", got)
	}
	r.clock.Set(time.Date(r.clock.Now().Year()+1, time.January, 1, 0, 0, 0, 0, time.UTC))
	if got := r.usage(t); got.Calls != 0 || got.Level != app.BudgetOpen {
		t.Fatalf("usage next month = %+v, want 0 calls and open", got)
	}
	r.budget.Spend(r.ctx(t))
	if got := r.usage(t); got.Calls != 1 {
		t.Fatalf("calls after one spend next month = %d, want 1", got.Calls)
	}
	var rows int
	if err := r.pool.QueryRow(t.Context(), `SELECT count(*) FROM coingecko_usage`).
		Scan(&rows); err != nil ||
		rows != 2 {
		t.Fatalf("usage rows = %d, %v, want one per month", rows, err)
	}
}

func TestBudget_warnsOnceAtEachOfFiftyEightyAndNinetyFivePercent(t *testing.T) {
	t.Parallel()
	r := newBudgetRig(t)
	r.setCalls(t, 40)
	ctx := r.ctx(t)
	for range 60 {
		r.budget.Spend(ctx)
	}
	got := r.logs.Lines("market.coingecko.budget", "percent", "calls")
	want := []string{"50 50", "80 80", "95 95"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("warnings = %v, want %v", got, want)
	}
}

func TestBudget_aCounterThatCannotBeReadOrWrittenNeverStopsWork(t *testing.T) {
	t.Parallel()
	r := newBudgetRig(t)
	r.exec(t, `ALTER TABLE coingecko_usage RENAME TO coingecko_usage_gone`)
	ctx := r.ctx(t)
	r.budget.Spend(ctx)
	if got := r.budget.Level(ctx); got != app.BudgetOpen {
		t.Fatalf("level = %s, want open when the count cannot be read", got)
	}
	if n := len(r.logs.Lines("market.coingecko.usage_failed", "err")); n != 2 {
		t.Fatalf("usage_failed logs = %d, want 2", n)
	}
}

func TestVolumes_stopAtTheSoftCap(t *testing.T) {
	t.Parallel()
	aapl := marketfake.AAPLx()
	r := newBudgetRig(t)
	seedAssets(t, r.pool, r.clock.Now(), aapl)
	r.history.PutSpotVolume(aapl.Mint, usd(7_000_000))
	volumes := app.NewVolumes(db.New(r.pool, testkit.NewIDs(6), r.clock), r.pool, r.clock, r.history).
		WithBudget(r.budget)
	for _, tc := range []struct {
		calls  int
		called bool
	}{{79, true}, {80, false}, {95, false}} {
		r.setCalls(t, tc.calls)
		before := len(r.history.VolumeBatches())
		report, err := volumes.Tick(r.ctx(t))
		if called := len(r.history.VolumeBatches()) - before; err != nil || (called == 1) != tc.called {
			t.Fatalf(
				"at %d calls: tick = %+v, %v with %d calls, want called = %t",
				tc.calls,
				report,
				err,
				called,
				tc.called,
			)
		}
	}
}

func TestReconcile_coldSlotsStopAtTheSoftCapAndEverythingAtTheHardCap(t *testing.T) {
	t.Parallel()
	aapl, tsla := marketfake.AAPLx(), marketfake.TSLAx()
	aapl.PopularRank, tsla.PopularRank = 0, 0
	hot := []app.HotMints{func(context.Context) ([]chain.SolanaAddress, error) {
		return []chain.SolanaAddress{aapl.Mint.Address()}, nil
	}}
	r := newBudgetRig(t)
	seedAssets(t, r.pool, r.clock.Now(), aapl, tsla)
	reconcile := app.NewReconcile(db.New(r.pool, testkit.NewIDs(5), r.clock), r.pool, r.clock, r.history, hot...).
		WithBudget(r.budget)
	for _, tc := range []struct {
		calls int
		want  int
	}{{0, 2}, {80, 1}, {94, 1}, {95, 0}} {
		r.setCalls(t, tc.calls)
		before := len(r.history.Calls())
		report, err := reconcile.Tick(r.ctx(t))
		calls := r.history.Calls()[before:]
		if err != nil || len(calls) != tc.want || report.Scanned != tc.want {
			t.Fatalf("at %d calls: tick = %+v, %v with %d calls, want %d", tc.calls, report, err, len(calls), tc.want)
		}
		if tc.calls == 80 && calls[0].Mint != aapl.Mint {
			t.Fatal("the soft cap kept the cold mint instead of the hot one")
		}
	}
}

type countingTransport struct{ hits atomic.Int32 }

func (c *countingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	c.hits.Add(1)
	return &http.Response{
		StatusCode: http.StatusOK, Header: http.Header{}, Request: r,
		Body: io.NopCloser(strings.NewReader(`{"prices":[[1700000000000,1.5]],"total_volumes":[]}`)),
	}, nil
}

func TestCoinGeckoClient_countsEachRequestAndRefusesOnceTheHardCapIsReached(t *testing.T) {
	t.Parallel()
	r := newBudgetRig(t)
	r.setCalls(t, 94)
	rt := &countingTransport{}
	cfg := config.Config{
		CoinGecko: config.CoinGecko{BaseURL: "http://coingecko.test"},
		Timeouts:  config.Timeouts{CoinGecko: time.Second},
	}
	client := coingecko.New(
		httpclient.New("coingecko", append(coingecko.Options(cfg), httpclient.WithTransport(rt))...), "key",
	).WithBudget(r.budget)
	aapl := marketfake.AAPLx()

	if _, err := client.MarketChart(r.ctx(t), aapl.Mint, 1); err != nil {
		t.Fatalf("call under the cap = %v, want it to go through", err)
	}
	if got := r.usage(t); got.Calls != 95 || got.Level != app.BudgetHard {
		t.Fatalf("usage = %+v, want 95 calls and the hard cap", got)
	}
	_, err := client.MarketChart(r.ctx(t), aapl.Mint, 1)
	if errs.CodeOf(err) != errs.CodeCoinGeckoRateLimited || rt.hits.Load() != 1 || r.usage(t).Calls != 95 {
		t.Fatalf("call over the cap = %v with %d upstream hits and %d counted, want a refusal before any call",
			err, rt.hits.Load(), r.usage(t).Calls)
	}
}

func TestBudgetLevel_namesEachLevelAndReadsAnUnknownOneAsOpen(t *testing.T) {
	t.Parallel()
	for level, want := range map[app.BudgetLevel]string{
		app.BudgetOpen: "open", app.BudgetSoft: "soft", app.BudgetHard: "hard", app.BudgetLevel(99): "open",
	} {
		if got := level.String(); got != want {
			t.Fatalf("level %d = %q, want %q", int(level), got, want)
		}
	}
}
