package market_test

import (
	"context"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

type backfillRig struct {
	pool    *pgxpool.Pool
	clock   *testkit.Clock
	history *marketfake.PriceHistoryFake
	logs    *testkit.Logs
	poller  *app.Backfill
	day     time.Time
}

func newBackfillRig(t *testing.T) *backfillRig {
	t.Helper()
	pool := testkit.DB(t)
	clk := testkit.NewClock(clock.Real{}.Now().UTC().Truncate(time.Microsecond))
	history := &marketfake.PriceHistoryFake{}
	ids := testkit.NewIDs(3)
	return &backfillRig{
		pool: pool, clock: clk, history: history, logs: &testkit.Logs{},
		poller: app.NewBackfill(db.New(pool, ids, clk), pool, clk, history),
		day:    clk.Now().Truncate(24 * time.Hour).Add(-72 * time.Hour),
	}
}

func (r *backfillRig) ctx(t *testing.T) context.Context {
	t.Helper()
	return observability.WithLogger(t.Context(), observability.NewLogger(config.Config{Env: config.EnvTest}, r.logs))
}

func (r *backfillRig) pend(t *testing.T, mints ...market.Mint) {
	t.Helper()
	for i, m := range mints {
		r.exec(t, `INSERT INTO price_backfills (mint, requested_at) VALUES ($1, $2)
			ON CONFLICT (mint) DO UPDATE SET done_at = NULL, last_code = NULL`, m.String(),
			r.clock.Now().Add(time.Duration(i)*time.Second))
	}
}

func (r *backfillRig) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := r.pool.Exec(t.Context(), sql, args...); err != nil {
		t.Fatal(err)
	}
}

func (r *backfillRig) tick(t *testing.T) (poller.Report, error) {
	t.Helper()
	return r.poller.Tick(r.ctx(t))
}

func (r *backfillRig) points(t *testing.T, mint market.Mint) map[time.Time]string {
	t.Helper()
	rows, err := r.pool.Query(t.Context(),
		`SELECT ts, price_micros::text || '/' || source FROM price_points WHERE mint = $1`, mint.String())
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[time.Time]string{}
	for rows.Next() {
		var ts time.Time
		var v string
		if err := rows.Scan(&ts, &v); err != nil {
			t.Fatal(err)
		}
		out[ts.UTC()] = v
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func (r *backfillRig) status(t *testing.T, mint market.Mint) (done bool, code string) {
	t.Helper()
	var doneAt *time.Time
	var last *string
	err := r.pool.QueryRow(t.Context(),
		`SELECT done_at, last_code FROM price_backfills WHERE mint = $1`, mint.String()).Scan(&doneAt, &last)
	if err != nil {
		t.Fatal(err)
	}
	if last != nil {
		code = *last
	}
	return doneAt != nil, code
}

func (r *backfillRig) putChart(m market.Mint) {
	at := func(d time.Duration) time.Time { return r.day.Add(d) }
	r.history.Put(m, 1, sample(at(10*time.Hour+7*time.Minute+31*time.Second), 105_000_000))
	r.history.Put(m, 90,
		sample(at(11*time.Hour+22*time.Minute), 110_000_000), sample(at(12*time.Hour+59*time.Minute), 111_000_000))
	r.history.Put(m, 365, sample(at(13*time.Hour+41*time.Second), 120_000_000))
}

func sample(at time.Time, micros uint64) app.Sample { return app.Sample{At: at, Price: usd(micros)} }

func TestBackfill_ThreeCallsPerMint(t *testing.T) {
	t.Parallel()
	r := newBackfillRig(t)
	aapl, tsla := marketfake.AAPLx().Mint, marketfake.TSLAx().Mint
	r.putChart(aapl)
	r.pend(t, aapl, tsla)

	report, err := r.tick(t)
	if err != nil {
		t.Fatal(err)
	}
	want := make([]marketfake.HistoryCall, 0, 6)
	for _, m := range []market.Mint{aapl, tsla} {
		for _, days := range []int{1, 90, 365} {
			want = append(want, marketfake.HistoryCall{Mint: m, Days: days})
		}
	}
	if got := r.history.Calls(); !slices.Equal(got, want) {
		t.Fatalf("calls = %v, want %v", got, want)
	}
	wantPoints := map[time.Time]string{
		r.day.Add(10*time.Hour + 5*time.Minute): "105000000/coingecko",
		r.day.Add(11 * time.Hour):               "110000000/coingecko",
		r.day.Add(12 * time.Hour):               "111000000/coingecko",
		r.day:                                   "120000000/coingecko",
	}
	if got := r.points(t, aapl); !maps.Equal(got, wantPoints) {
		t.Fatalf("points = %v, want %v", got, wantPoints)
	}
	if report.Scanned != 2 || report.Changed != 4 {
		t.Fatalf("report = %+v, want 2 mints scanned and 4 rows", report)
	}
	for _, m := range []market.Mint{aapl, tsla} {
		if done, code := r.status(t, m); !done || code != "" {
			t.Fatalf("%v done = %v, last_code = %q, want done with no code", m, done, code)
		}
	}
}

func TestBackfill_Idempotent(t *testing.T) {
	t.Parallel()
	r := newBackfillRig(t)
	aapl := marketfake.AAPLx().Mint
	r.putChart(aapl)
	r.pend(t, aapl)
	if report, err := r.tick(t); err != nil || report.Changed != 4 {
		t.Fatalf("first run = %+v, %v, want 4 rows", report, err)
	}
	r.pend(t, aapl)
	report, err := r.tick(t)
	if err != nil || report.Changed != 0 || report.Scanned != 1 {
		t.Fatalf("second run = %+v, %v, want the mint scanned and zero rows inserted", report, err)
	}
	if got := len(r.points(t, aapl)); got != 4 {
		t.Fatalf("%d points after the second run, want 4", got)
	}
}

func TestBackfill_doneMintsAreNotAskedAgain(t *testing.T) {
	t.Parallel()
	r := newBackfillRig(t)
	aapl := marketfake.AAPLx().Mint
	r.pend(t, aapl)
	for range 2 {
		if _, err := r.tick(t); err != nil {
			t.Fatal(err)
		}
	}
	if got := len(r.history.Calls()); got != 3 {
		t.Fatalf("%d calls over two ticks, want the 3 of the first only", got)
	}
}

func TestBackfill_LiveSampleWins(t *testing.T) {
	t.Parallel()
	r := newBackfillRig(t)
	aapl := marketfake.AAPLx().Mint
	r.putChart(aapl)
	r.pend(t, aapl)
	live := r.day.Add(11 * time.Hour)
	r.exec(t, `INSERT INTO price_points (mint, ts, price_micros, source) VALUES ($1, $2, 99000000, 'jupiter')`,
		aapl.String(), live)

	if _, err := r.tick(t); err != nil {
		t.Fatal(err)
	}
	got := r.points(t, aapl)
	if got[live] != "99000000/jupiter" || len(got) != 4 {
		t.Fatalf("points = %v, want the live sample kept at %v and the other 3 backfilled", got, live)
	}
}

func TestBackfill_finestWindowWinsASharedBucket(t *testing.T) {
	t.Parallel()
	r := newBackfillRig(t)
	aapl := marketfake.AAPLx().Mint
	r.history.Put(aapl, 90, sample(r.day.Add(30*time.Second), 130_000_000))
	r.history.Put(aapl, 365, sample(r.day.Add(13*time.Hour), 140_000_000))
	r.pend(t, aapl)
	if _, err := r.tick(t); err != nil {
		t.Fatal(err)
	}
	if got := r.points(t, aapl); len(got) != 1 || got[r.day] != "130000000/coingecko" {
		t.Fatalf("points = %v, want the hourly 130 in the midnight bucket both windows share", got)
	}
}

func TestBackfill_UnlistedMint(t *testing.T) {
	t.Parallel()
	r := newBackfillRig(t)
	tsla := marketfake.TSLAx().Mint
	r.pend(t, tsla)
	report, err := r.tick(t)
	if err != nil || report.Changed != 0 {
		t.Fatalf("tick = %+v, %v, want no rows and no error", report, err)
	}
	if done, code := r.status(t, tsla); !done || code != "" || len(r.points(t, tsla)) != 0 {
		t.Fatalf("done = %v, last_code = %q, want done with no code and no points", done, code)
	}
}

func TestBackfill_NoKeySkips(t *testing.T) {
	t.Parallel()
	r := newBackfillRig(t)
	aapl := marketfake.AAPLx().Mint
	r.history.WithoutKey()
	r.putChart(aapl)
	r.pend(t, aapl)
	report, err := r.tick(t)
	if err != nil || report.Scanned != 0 || report.Changed != 0 {
		t.Fatalf("tick = %+v, %v, want nothing done", report, err)
	}
	if got := len(r.history.Calls()); got != 0 {
		t.Fatalf("%d calls with no key, want none", got)
	}
	if done, _ := r.status(t, aapl); done || len(r.points(t, aapl)) != 0 {
		t.Fatalf("done = %v with %d points, want the mint still pending", done, len(r.points(t, aapl)))
	}
	if _, err := r.tick(t); err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(r.logs.Bytes()), `"msg":"market.backfill.skipped_no_key"`); got != 2 {
		t.Fatalf("skipped_no_key logged %d times over 2 ticks, want once per tick", got)
	}
}

func TestBackfill_aFailedMintKeepsItsCodeAndNothingIsWritten(t *testing.T) {
	t.Parallel()
	r := newBackfillRig(t)
	aapl := marketfake.AAPLx().Mint
	r.putChart(aapl)
	r.pend(t, aapl)
	r.history.FailOnce("MarketChart", nil)
	r.history.FailOnce("MarketChart", errs.New(errs.CodeUpstreamTimeout, "test"))

	_, err := r.tick(t)
	if errs.CodeOf(err) != errs.CodeUpstreamTimeout {
		t.Fatalf("tick err = %v, want upstream_timeout", err)
	}
	if done, code := r.status(t, aapl); done || code != "upstream_timeout" || len(r.points(t, aapl)) != 0 {
		t.Fatalf("done = %v, last_code = %q, points = %d, want pending with the code and no rows",
			done, code, len(r.points(t, aapl)))
	}
	if _, err := r.tick(t); err != nil {
		t.Fatal(err)
	}
	if done, code := r.status(t, aapl); !done || code != "" {
		t.Fatalf("after a clean retry done = %v, last_code = %q, want done with the code cleared", done, code)
	}
}

func TestBackfill_aRateLimitStopsTheTickAndFreshMintsGoFirst(t *testing.T) {
	t.Parallel()
	r := newBackfillRig(t)
	aapl, tsla, jpst := marketfake.AAPLx().Mint, marketfake.TSLAx().Mint, marketfake.JPSTx().Mint
	r.pend(t, aapl, tsla, jpst)
	r.exec(t, `UPDATE price_backfills SET last_code = 'upstream_timeout' WHERE mint = $1`, aapl.String())
	r.history.Fail("MarketChart", errs.New(errs.CodeCoinGeckoRateLimited, "test"))

	report, err := r.tick(t)
	if errs.CodeOf(err) != errs.CodeCoinGeckoRateLimited {
		t.Fatalf("tick err = %v, want coin_gecko_rate_limited", err)
	}
	if calls := r.history.Calls(); len(calls) != 1 || calls[0].Mint != tsla {
		t.Fatalf("calls = %v, want one call, for the never-tried TSLAx, before the tick stopped", calls)
	}
	if report.Scanned != 1 {
		t.Fatalf("report = %+v, want 1 mint scanned", report)
	}
	for m, want := range map[market.Mint]string{tsla: "coin_gecko_rate_limited", jpst: "", aapl: "upstream_timeout"} {
		if _, code := r.status(t, m); code != want {
			t.Fatalf("%v last_code = %q, want %q", m, code, want)
		}
	}
}

func TestBackfill_aBadMintInTheTableIsRecordedNotFatal(t *testing.T) {
	t.Parallel()
	r := newBackfillRig(t)
	aapl := marketfake.AAPLx().Mint
	r.exec(t, `INSERT INTO price_backfills (mint, requested_at) VALUES ('not-a-mint', $1)`,
		r.clock.Now().Add(-time.Hour))
	r.pend(t, aapl)
	_, err := r.tick(t)
	if errs.CodeOf(err) != errs.CodeInvalidAddress {
		t.Fatalf("tick err = %v, want invalid_address", err)
	}
	if done, _ := r.status(t, aapl); !done {
		t.Fatal("the good mint behind the bad one was not backfilled")
	}
	var code string
	err = r.pool.QueryRow(t.Context(), `SELECT last_code FROM price_backfills WHERE mint = 'not-a-mint'`).Scan(&code)
	if err != nil || code != "invalid_address" {
		t.Fatalf("bad mint last_code = %q, %v, want invalid_address", code, err)
	}
}

func TestBackfill_takesTenPendingMintsATick(t *testing.T) {
	t.Parallel()
	r := newBackfillRig(t)
	mints := make([]market.Mint, 0, 11)
	for _, raw := range []string{
		"XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp", "XsDoVfqeBukxuZHWhdvWHBhgEHjGNst4MLodqsJHzoB",
		"XsCAXu7xTaZMG9b9KJhNWYapuvNjxPuE4SysZq8uvMq", "TSPXcLV76s6V2zDiZQ18kBfcbnjaE2ZzNT3ga2Pd99v",
		"PreANxuXjsy2pvisWWMNB6YaJNzr7681wJJr2rHsfTh", "PresTj4Yc2bAR197Er7wz4UUKSfqt6FryBEdAriBoQB",
		"Pren1FvFX6J3E4kXhJuCiAD5aDmGEb7qJRncwA8Lkhw", "PreZad18qfPtbxNpMtMuAuX2zVpvkEU8DnJx56faCWd",
		"PreLWGkkeqG1s4HEfFZSy9moCrJ7btsHuUtfcCeoRua", "PrekqLJvJ3qVdXmBGDiexvwUTF4rLFDa6HWS4HJbw9S",
		"EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v",
	} {
		m, err := market.ParseMint(raw)
		if err != nil {
			t.Fatal(err)
		}
		mints = append(mints, m)
	}
	r.pend(t, mints...)
	report, err := r.tick(t)
	if err != nil || report.Scanned != 10 || len(r.history.Calls()) != 30 {
		t.Fatalf("tick = %+v, %v with %d calls, want 10 mints and 30 calls", report, err, len(r.history.Calls()))
	}
	if done, _ := r.status(t, mints[10]); done {
		t.Fatal("the eleventh mint was backfilled in the same tick")
	}
	if _, err := r.tick(t); err != nil {
		t.Fatal(err)
	}
	if done, _ := r.status(t, mints[10]); !done {
		t.Fatal("the eleventh mint was not backfilled on the next tick")
	}
}

func TestBackfill_pricesTooBigToStoreAreLeftOut(t *testing.T) {
	t.Parallel()
	r := newBackfillRig(t)
	aapl := marketfake.AAPLx().Mint
	r.history.Put(aapl, 1, sample(r.day, 1<<63), sample(r.day.Add(5*time.Minute), 7))
	r.pend(t, aapl)
	if _, err := r.tick(t); err != nil {
		t.Fatal(err)
	}
	if got := r.points(t, aapl); len(got) != 1 || got[r.day.Add(5*time.Minute)] != "7/coingecko" {
		t.Fatalf("points = %v, want only the storable price", got)
	}
}

func TestBackfill_aDatabaseFailureIsReportedAndTheMintStaysPending(t *testing.T) {
	t.Parallel()
	r := newBackfillRig(t)
	aapl := marketfake.AAPLx().Mint
	r.pend(t, aapl)
	ctx, cancel := context.WithCancel(r.ctx(t))
	r.history.During(cancel)
	_, err := r.poller.Tick(ctx)
	if err == nil {
		t.Fatal("tick with a cancelled context succeeded")
	}
	if done, _ := r.status(t, aapl); done {
		t.Fatal("a mint whose write failed was marked done")
	}
	if _, err := r.poller.Tick(ctx); err == nil {
		t.Fatal("tick on a cancelled context succeeded")
	}
}

func TestBackfill_isNamedAndRunsEveryFiveMinutes(t *testing.T) {
	t.Parallel()
	b := app.NewBackfill(nil, nil, clock.Real{}, &marketfake.PriceHistoryFake{})
	if b.Name() != "market.backfill" || b.Interval() != 5*time.Minute {
		t.Fatalf("Backfill = %s every %v, want market.backfill every 5m", b.Name(), b.Interval())
	}
}

func TestBackfill_AMintAddedByTheCatalogPollerIsBackfilledOnTheNextTick(t *testing.T) {
	t.Parallel()
	aapl := marketfake.AAPLx()
	p := &provider{issuer: domain.IssuerXStocks}
	p.serve(nil, listed(aapl))
	rig := newRig(t, p)
	rig.tick(t)

	history := &marketfake.PriceHistoryFake{}
	backfill := app.NewBackfill(db.New(rig.pool, rig.ids, rig.clock), rig.pool, rig.clock, history)
	history.Put(aapl.Mint, 365, sample(rig.clock.Now().Truncate(24*time.Hour).Add(-48*time.Hour), 120_000_000))
	report, err := backfill.Tick(rig.ctx(t))
	if err != nil || report.Scanned != 1 || report.Changed != 1 || len(history.Calls()) != 3 {
		t.Fatalf("backfill tick = %+v, %v with %d calls, want the new mint backfilled with 3 calls",
			report, err, len(history.Calls()))
	}
	if _, err := rig.poller.Tick(rig.ctx(t)); err != nil {
		t.Fatal(err)
	}
	if report, err := backfill.Tick(rig.ctx(t)); err != nil || report.Scanned != 0 {
		t.Fatalf("second tick = %+v, %v, want nothing pending after a catalog tick that saw no new mint", report, err)
	}
}

func TestBackfill_runRequeuesDoneMintsAndDrainsThem(t *testing.T) {
	t.Parallel()
	r := newBackfillRig(t)
	aapl, tsla := marketfake.AAPLx().Mint, marketfake.TSLAx().Mint
	r.putChart(aapl)
	r.pend(t, aapl)
	if _, err := r.tick(t); err != nil {
		t.Fatal(err)
	}
	res, err := r.poller.Run(r.ctx(t), []string{aapl.String(), tsla.String()})
	if err != nil || res != (app.BackfillResult{Mints: 2, Calls: 6, Rows: 0}) {
		t.Fatalf("Run = %+v, %v, want 2 mints, 6 calls and no new rows", res, err)
	}
	for _, m := range []market.Mint{aapl, tsla} {
		if done, code := r.status(t, m); !done || code != "" {
			t.Fatalf("%v done = %v, last_code = %q, want done and clean", m, done, code)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := r.poller.Run(ctx, []string{aapl.String()}); err == nil {
		t.Fatal("Run on a cancelled context succeeded")
	}
}

func TestBackfill_runAllTakesEveryCatalogMint(t *testing.T) {
	t.Parallel()
	aapl, tsla := marketfake.AAPLx(), marketfake.TSLAx()
	r := newBackfillRig(t)
	seedAssets(t, r.pool, r.clock.Now(), aapl, tsla)
	r.putChart(aapl.Mint)
	res, err := r.poller.RunAll(r.ctx(t))
	if err != nil || res != (app.BackfillResult{Mints: 2, Calls: 6, Rows: 4}) {
		t.Fatalf("RunAll = %+v, %v, want 2 mints, 6 calls and 4 rows", res, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := r.poller.RunAll(ctx); err == nil {
		t.Fatal("RunAll on a cancelled context read the catalog")
	}
}

func TestBackfill_aRowThePointsTableRejectsFailsTheWholeMint(t *testing.T) {
	t.Parallel()
	r := newBackfillRig(t)
	aapl := marketfake.AAPLx().Mint
	r.history.Put(aapl, 1, sample(r.day, 105_000_000))
	r.history.Put(aapl, 365, sample(time.Date(300_000, time.January, 1, 0, 0, 0, 0, time.UTC), 120_000_000))
	r.pend(t, aapl)
	if _, err := r.tick(t); err == nil {
		t.Fatal("tick with a sample Postgres cannot store succeeded")
	}
	if done, code := r.status(t, aapl); done || code == "" || len(r.points(t, aapl)) != 0 {
		t.Fatalf("done = %v, last_code = %q, points = %d, want pending with a code and no rows from any window",
			done, code, len(r.points(t, aapl)))
	}
}
