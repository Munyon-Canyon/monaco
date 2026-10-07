package adapters_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type pollerFake struct {
	run         *sqlc.LeaderboardRun
	trigger     *time.Time
	runErr      error
	triggerErr  error
	valueErr    error
	writeErr    error
	flags       []domain.Flag
	runs, saved int
}

func (f *pollerFake) LastLeaderboardRun(context.Context) (sqlc.LeaderboardRun, error) {
	if f.runErr != nil || f.run == nil {
		return sqlc.LeaderboardRun{}, orNoRows(f.runErr)
	}
	return *f.run, nil
}

func (f *pollerFake) OldestRankingTrigger(context.Context) (time.Time, error) {
	if f.triggerErr != nil || f.trigger == nil {
		return time.Time{}, orNoRows(f.triggerErr)
	}
	return *f.trigger, nil
}

func (f *pollerFake) Run(context.Context, time.Time) (app.Valuation, error) {
	f.runs++
	return app.Valuation{
		Cabals: []app.CabalValue{{}}, Flagged: []app.CabalValue{{Flags: f.flags}}, Entries: []app.Entry{{}, {}},
	}, f.valueErr
}

func (f *pollerFake) Write(context.Context, app.Valuation, time.Time, time.Time) (uuid.UUID, error) {
	f.saved++
	return uuid.Nil, f.writeErr
}

func orNoRows(err error) error {
	if err != nil {
		return err
	}
	return pgx.ErrNoRows
}

func TestValuationPoller_runsOnlyWhenTheScheduleSaysSo(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	ago := func(d time.Duration) *time.Time { at := now.Add(-d); return &at }
	runAt := func(finished, asOf time.Duration) *sqlc.LeaderboardRun {
		return &sqlc.LeaderboardRun{FinishedAt: now.Add(-finished), AsOf: now.Add(-asOf)}
	}
	for name, tc := range map[string]struct {
		fake *pollerFake
		runs int
	}{
		"the very first tick":                {&pollerFake{}, 1},
		"a recent run and no trigger":        {&pollerFake{run: runAt(time.Second, time.Second)}, 0},
		"a run two minutes ago":              {&pollerFake{run: runAt(2*time.Minute, 2*time.Minute)}, 1},
		"a trigger under a second old":       {&pollerFake{run: runAt(time.Minute, time.Minute), trigger: ago(900 * time.Millisecond)}, 0},
		"a trigger a second old":             {&pollerFake{run: runAt(time.Minute, time.Minute), trigger: ago(time.Second)}, 1},
		"a due run that already holds as_of": {&pollerFake{run: runAt(3*time.Minute, 0), trigger: ago(time.Minute)}, 0},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			report, err := tc.fake.poller(testkit.NewClock(now)).Tick(t.Context())
			if err != nil || tc.fake.runs != tc.runs || tc.fake.saved != tc.runs {
				t.Fatalf(
					"Tick() = %+v, %v with %d runs and %d writes, want %d",
					report,
					err,
					tc.fake.runs,
					tc.fake.saved,
					tc.runs,
				)
			}
			if tc.runs == 1 && (report.Scanned != 2 || report.Changed != 2) {
				t.Fatalf("report = %+v, want 2 cabals scanned and 2 rows changed", report)
			}
		})
	}
}

func (f *pollerFake) poller(clock *testkit.Clock) adapters.ValuationPoller {
	return adapters.ValuationPoller{Reads: f, Runner: f, Writer: f, Clock: clock}
}

func TestValuationPoller_returnsEveryFailure(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	boom := errs.New(errs.CodeUpstreamUnavailable, "test")
	for name, fake := range map[string]*pollerFake{
		"last run read":       {runErr: boom},
		"trigger read":        {triggerErr: boom},
		"valuation":           {valueErr: boom},
		"write":               {writeErr: boom},
		"valuation and write": {valueErr: boom, writeErr: boom},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := fake.poller(testkit.NewClock(now)).
				Tick(t.Context()); errs.CodeOf(
				err,
			) != errs.CodeUpstreamUnavailable {
				t.Fatalf("Tick() = %v, want the upstream error", err)
			}
		})
	}
}

func TestValuationPoller_namesItselfAndTicksEverySecond(t *testing.T) {
	t.Parallel()
	p := adapters.ValuationPoller{}
	if p.Name() != "ranking.valuation" || p.Interval() != time.Second {
		t.Fatalf("poller = %s every %v, want ranking.valuation every 1s", p.Name(), p.Interval())
	}
}

func TestValuationPoller_logsTheRealFlagForAnExcludedCabal(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	for name, tc := range map[string]struct {
		flags []domain.Flag
		want  string
	}{
		"an unpriced cabal": {[]domain.Flag{domain.FlagUnpricedAssets}, `"reason":"unpriced_assets"`},
		"a stale cabal":     {[]domain.Flag{domain.FlagStalePrices}, `"code":"prices_stale","reason":"stale_prices"`},
		"both flags": {
			[]domain.Flag{domain.FlagStalePrices, domain.FlagUnpricedAssets},
			`"code":"prices_stale","reason":"stale_prices,unpriced_assets"`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var logs bytes.Buffer
			ctx := observability.WithLogger(t.Context(), slog.New(slog.NewJSONHandler(&logs, nil)))
			fake := &pollerFake{flags: tc.flags}
			if _, err := fake.poller(testkit.NewClock(now)).Tick(ctx); err != nil {
				t.Fatalf("Tick() = %v", err)
			}
			out := logs.String()
			if !strings.Contains(tc.want, "prices_stale") && strings.Contains(out, "prices_stale") {
				t.Fatalf("logs = %s, want no prices_stale code for %s", out, name)
			}
			if !strings.Contains(out, "ranking.cabal.excluded") || !strings.Contains(out, tc.want) {
				t.Fatalf("logs = %s, want ranking.cabal.excluded with %s", out, tc.want)
			}
		})
	}
}
