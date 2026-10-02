package market_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

func TestPriceMoved_UpTen(t *testing.T) {
	t.Parallel()
	aapl := marketfake.AAPLx()
	at := openInstant(t, clock.Real{}.Now())
	rig, q := newMoveRig(t, at, aapl)
	info := equitySession(t, rig.clock.Now())
	seedPrice(t, rig.pool, aapl.Mint.String(), info.LastClose, 200_000_000)
	q.quote(aapl, 220_000_000)
	var logs testkit.Logs
	ctx := loggedPollerCtx(t, &logs)
	if _, err := rig.poller.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	got := movedEvents(t, rig.pool)
	if len(got) != 2 {
		t.Fatalf("events = %d, want +500 and +1000", len(got))
	}
	wantMove(t, got[0], aapl, 500, 1000, 220_000_000, info.TradingDay.String(), rig.bucket)
	wantMove(t, got[1], aapl, 1000, 1000, 220_000_000, info.TradingDay.String(), rig.bucket)
	if strings.Count(string(logs.Bytes()), `"msg":"market.price_moved"`) != 2 {
		t.Fatalf("log = %s, want two market.price_moved lines", logs.Bytes())
	}
	rig.clock.Advance(domain.SampleBucket)
	if _, err := rig.poller.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if n := len(movedEvents(t, rig.pool)); n != 2 {
		t.Fatalf("events after a later tick the same day = %d, want 2", n)
	}
	if strings.Count(string(logs.Bytes()), `"msg":"market.price_moved"`) != 2 {
		t.Fatalf("log = %s, want no new market.price_moved line", logs.Bytes())
	}
}

func TestPriceMoved_NextDayAgain(t *testing.T) {
	t.Parallel()
	aapl := marketfake.AAPLx()
	at := openInstant(t, clock.Real{}.Now())
	rig, q := newMoveRig(t, at, aapl)
	info := equitySession(t, rig.clock.Now())
	seedPrice(t, rig.pool, aapl.Mint.String(), info.LastClose, 200_000_000)
	q.quote(aapl, 220_000_000)
	ctx := pollerCtx(t)
	if _, err := rig.poller.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	nextDay := time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, 1)
	pinMoveClock(rig, openInstant(t, nextDay))
	next := equitySession(t, rig.clock.Now())
	seedPrice(t, rig.pool, aapl.Mint.String(), next.LastClose, 200_000_000)
	if _, err := rig.poller.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	got := movedEvents(t, rig.pool)
	if len(got) != 4 {
		t.Fatalf("events = %d, want the same move again on %s", len(got), next.TradingDay)
	}
	wantMove(t, got[2], aapl, 500, 1000, 220_000_000, next.TradingDay.String(), rig.bucket)
	wantMove(t, got[3], aapl, 1000, 1000, 220_000_000, next.TradingDay.String(), rig.bucket)
}

func TestPriceMoved_ClosedMarketSkipsEquity(t *testing.T) {
	t.Parallel()
	aapl := marketfake.AAPLx()
	rig, q := newMoveRig(t, sundayInstant(t, clock.Real{}.Now()), aapl)
	info := equitySession(t, rig.clock.Now())
	if info.State == domain.StateOpen {
		t.Fatalf("session at %s is open", rig.clock.Now())
	}
	seedPrice(t, rig.pool, aapl.Mint.String(), info.LastClose, 200_000_000)
	q.quote(aapl, 220_000_000)
	if _, err := rig.poller.Tick(pollerCtx(t)); err != nil {
		t.Fatal(err)
	}
	if n := len(movedEvents(t, rig.pool)); n != 0 {
		t.Fatalf("events while closed = %d, want 0", n)
	}
}

func TestPriceMoved_PreIPOAnyTime(t *testing.T) {
	t.Parallel()
	aapl := marketfake.AAPLx()
	aapl.Kind = domain.KindPreIPO
	sunday := sundayInstant(t, clock.Real{}.Now())
	rig, q := newMoveRig(t, sunday, aapl)
	day := time.Date(sunday.Year(), sunday.Month(), sunday.Day(), 0, 0, 0, 0, time.UTC)
	seedPrice(t, rig.pool, aapl.Mint.String(), day.Add(-6*time.Hour), 200_000_000)
	seedPrice(t, rig.pool, aapl.Mint.String(), day.Add(12*time.Hour), 200_000_000)
	q.quote(aapl, 220_000_000)
	if _, err := rig.poller.Tick(pollerCtx(t)); err != nil {
		t.Fatal(err)
	}
	got := movedEvents(t, rig.pool)
	if len(got) != 2 {
		t.Fatalf("events = %d, want the sunday move", len(got))
	}
	info := equitySession(t, rig.clock.Now())
	wantMove(t, got[0], aapl, 500, 1000, 220_000_000, info.TradingDay.String(), rig.bucket)
	wantMove(t, got[1], aapl, 1000, 1000, 220_000_000, info.TradingDay.String(), rig.bucket)
}

func TestPriceMoved_HeldSpikeIgnored(t *testing.T) {
	t.Parallel()
	aapl := marketfake.AAPLx()
	rig, q := newMoveRig(t, openInstant(t, clock.Real{}.Now()), aapl)
	info := equitySession(t, rig.clock.Now())
	seedPrice(t, rig.pool, aapl.Mint.String(), info.LastClose, 200_000_000)
	q.quote(aapl, 250_000_000)
	if _, err := rig.poller.Tick(pollerCtx(t)); err != nil {
		t.Fatal(err)
	}
	if n := len(movedEvents(t, rig.pool)); n != 0 {
		t.Fatalf("events for an unconfirmed spike = %d, want 0", n)
	}
}

func TestPriceMoved_usesTheSampleSessionWhenTheClockMoves(t *testing.T) {
	t.Parallel()
	aapl := marketfake.AAPLx()
	rig, q := newMoveRig(t, openInstant(t, clock.Real{}.Now()), aapl)
	info := equitySession(t, rig.clock.Now())
	seedPrice(t, rig.pool, aapl.Mint.String(), info.LastClose, 200_000_000)
	q.quote(aapl, 220_000_000)
	rig.ticks.after = func() { rig.clock.Advance(48 * time.Hour) }
	if _, err := rig.poller.Tick(pollerCtx(t)); err != nil {
		t.Fatal(err)
	}
	got := movedEvents(t, rig.pool)
	if len(got) != 2 {
		t.Fatalf("events = %d, want the move on the sample's session", len(got))
	}
	wantMove(t, got[0], aapl, 500, 1000, 220_000_000, info.TradingDay.String(), rig.bucket)
	wantMove(t, got[1], aapl, 1000, 1000, 220_000_000, info.TradingDay.String(), rig.bucket)
}

func TestPriceMoved_staleMarkDoesNotConsumeTheDay(t *testing.T) {
	t.Parallel()
	aapl := marketfake.AAPLx()
	rig, _ := newMoveRig(t, openInstant(t, clock.Real{}.Now()), aapl)
	info := equitySession(t, rig.clock.Now())
	seedPrice(t, rig.pool, aapl.Mint.String(), info.LastClose, 200_000_000)
	seedPrice(t, rig.pool, aapl.Mint.String(), rig.bucket.Add(-domain.SampleBucket), 220_000_000)
	if _, err := rig.poller.Tick(pollerCtx(t)); err != nil {
		t.Fatal(err)
	}
	if n := len(movedEvents(t, rig.pool)); n != 0 {
		t.Fatalf("events from a quote older than this tick = %d, want 0", n)
	}
}

func TestPriceMoved_noReferenceSkips(t *testing.T) {
	t.Parallel()
	aapl := marketfake.AAPLx()
	rig, q := newMoveRig(t, openInstant(t, clock.Real{}.Now()), aapl)
	q.quote(aapl, 220_000_000)
	if _, err := rig.poller.Tick(pollerCtx(t)); err != nil {
		t.Fatal(err)
	}
	if n := len(movedEvents(t, rig.pool)); n != 0 {
		t.Fatalf("events without a reference = %d, want 0", n)
	}
}

func TestPriceMoved_recordsTheMoveWhenPublishingTheTickFails(t *testing.T) {
	t.Parallel()
	aapl := marketfake.AAPLx()
	rig, q := newMoveRig(t, openInstant(t, clock.Real{}.Now()), aapl)
	info := equitySession(t, rig.clock.Now())
	seedPrice(t, rig.pool, aapl.Mint.String(), info.LastClose, 200_000_000)
	q.quote(aapl, 220_000_000)
	rig.ticks.err = errs.New(errs.CodeUpstreamUnavailable, "test.publish")
	_, err := rig.poller.Tick(pollerCtx(t))
	if errs.CodeOf(err) != errs.CodeUpstreamUnavailable {
		t.Fatalf("Tick = %v, want upstream_unavailable", err)
	}
	got := movedEvents(t, rig.pool)
	if len(got) != 2 {
		t.Fatalf("events after a failed tick publish = %d, want the move recorded", len(got))
	}
	wantMove(t, got[0], aapl, 500, 1000, 220_000_000, info.TradingDay.String(), rig.bucket)
	wantMove(t, got[1], aapl, 1000, 1000, 220_000_000, info.TradingDay.String(), rig.bucket)
}

func TestPriceMoved_appendNeedsAnActor(t *testing.T) {
	t.Parallel()
	aapl := marketfake.AAPLx()
	rig, q := newMoveRig(t, openInstant(t, clock.Real{}.Now()), aapl)
	info := equitySession(t, rig.clock.Now())
	seedPrice(t, rig.pool, aapl.Mint.String(), info.LastClose, 200_000_000)
	q.quote(aapl, 220_000_000)
	if _, err := rig.poller.Tick(t.Context()); err == nil {
		t.Fatal("Tick without an actor returned nil, want the append to fail")
	}
	if n := moveCount(t, rig.pool); n != 0 {
		t.Fatalf("move rows after a failed append = %d, want 0", n)
	}
}

func TestPriceMoved_aFailedMoveInsertFailsTheTick(t *testing.T) {
	t.Parallel()
	aapl := marketfake.AAPLx()
	rig, q := newMoveRig(t, openInstant(t, clock.Real{}.Now()), aapl)
	info := equitySession(t, rig.clock.Now())
	seedPrice(t, rig.pool, aapl.Mint.String(), info.LastClose, 200_000_000)
	q.quote(aapl, 220_000_000)
	rig.ticks.after = func() {
		_, err := rig.pool.Exec(t.Context(),
			`ALTER TABLE asset_price_moves RENAME TO asset_price_moves_hidden`)
		if err != nil {
			t.Errorf("rename asset_price_moves: %v", err)
		}
	}
	if _, err := rig.poller.Tick(pollerCtx(t)); err == nil {
		t.Fatal("Tick whose move insert fails returned nil")
	}
	if n := len(movedEvents(t, rig.pool)); n != 0 {
		t.Fatalf("events after a failed insert = %d, want 0", n)
	}
}

func TestPriceMoved_aFailedMarkReadFailsTheTick(t *testing.T) {
	t.Parallel()
	aapl := marketfake.AAPLx()
	rig, q := newMoveRig(t, openInstant(t, clock.Real{}.Now()), aapl)
	info := equitySession(t, rig.clock.Now())
	seedPrice(t, rig.pool, aapl.Mint.String(), info.LastClose, 200_000_000)
	q.quote(aapl, 220_000_000)
	ctx, cancel := context.WithCancel(pollerCtx(t))
	rig.ticks.after = cancel
	if _, err := rig.poller.Tick(ctx); err == nil {
		t.Fatal("Tick whose mark read is cancelled returned nil")
	}
	if n := len(movedEvents(t, rig.pool)); n != 0 {
		t.Fatalf("events after a cancelled mark read = %d, want 0", n)
	}
}

func TestPriceMoved_expiredCalendarFailsTheTickAfterTheSample(t *testing.T) {
	t.Parallel()
	aapl := marketfake.AAPLx()
	rig, q := newMoveRig(t, clock.Real{}.Now().AddDate(3, 0, 0), aapl)
	q.quote(aapl, 220_000_000)
	_, err := rig.poller.Tick(pollerCtx(t))
	if errs.CodeOf(err) != errs.CodeCalendarExpired {
		t.Fatalf("Tick = %v, want calendar_expired", err)
	}
	if n := len(movedEvents(t, rig.pool)); n != 0 {
		t.Fatalf("events = %d, want 0", n)
	}
	if got := pricePoints(t, rig.pool); len(got) != 1 {
		t.Fatalf("price_points = %+v, want the sample committed before the calendar failure", got)
	}
}

func TestDaySamples_rejectsANegativePriceAndACancelledRead(t *testing.T) {
	t.Parallel()
	aapl := marketfake.AAPLx()
	at := openInstant(t, clock.Real{}.Now())
	rig, _ := newMoveRig(t, at, aapl)
	book := app.NewPriceBook(rig.pool, rig.clock)
	seedPrice(t, rig.pool, aapl.Mint.String(), rig.bucket.Add(-time.Hour), -1)
	day := time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, time.UTC)
	_, err := book.DaySamples(t.Context(), aapl.ID, day, rig.clock.Now())
	if errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("DaySamples = %v, want decode_failed", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := book.DaySamples(ctx, aapl.ID, day, rig.clock.Now()); err == nil {
		t.Fatal("DaySamples on a cancelled context returned nil")
	}
}

func newMoveRig(t *testing.T, at time.Time, assets ...market.Asset) (*sampleRig, *quotes) {
	t.Helper()
	pool := testkit.DB(t)
	bucket := at.UTC().Truncate(domain.SampleBucket)
	seedAssets(t, pool, bucket, assets...)
	clk := testkit.NewClock(bucket.Add(37 * time.Second))
	ids := testkit.NewIDs(56)
	q := &quotes{}
	ticks := &tickRecorder{}
	return &sampleRig{
		pool: pool, clock: clk, bucket: bucket, ticks: ticks,
		poller: app.NewSamplePrices(db.New(pool, ids, clk), pool, ids, clk, q, ticks, 2*time.Minute),
	}, q
}

func pinMoveClock(rig *sampleRig, at time.Time) {
	rig.bucket = at.UTC().Truncate(domain.SampleBucket)
	rig.clock.Set(rig.bucket.Add(37 * time.Second))
}

func pollerCtx(t *testing.T) context.Context {
	t.Helper()
	return auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorSystem, ID: "poller.market.prices"})
}

func loggedPollerCtx(t *testing.T, logs *testkit.Logs) context.Context {
	t.Helper()
	logger := observability.NewLogger(config.Config{Env: config.EnvTest}, logs)
	return observability.WithLogger(pollerCtx(t), logger)
}

func openInstant(t *testing.T, from time.Time) time.Time {
	t.Helper()
	day := time.Date(from.UTC().Year(), from.UTC().Month(), from.UTC().Day(), 15, 0, 0, 0, time.UTC)
	for i := range 14 {
		at := day.AddDate(0, 0, i)
		if at.Before(from.UTC()) {
			continue
		}
		if equitySession(t, at).State == domain.StateOpen {
			return at
		}
	}
	t.Fatal("no open equity session")
	return time.Time{}
}

func sundayInstant(t *testing.T, from time.Time) time.Time {
	t.Helper()
	day := time.Date(from.UTC().Year(), from.UTC().Month(), from.UTC().Day(), 15, 0, 0, 0, time.UTC)
	for i := range 14 {
		at := day.AddDate(0, 0, i)
		if at.Weekday() == time.Sunday && !at.Before(from.UTC()) {
			return at
		}
	}
	t.Fatal("no sunday")
	return time.Time{}
}

func seedPrice(t *testing.T, pool *pgxpool.Pool, mint string, at time.Time, micros int64) {
	t.Helper()
	_, err := pool.Exec(t.Context(),
		`INSERT INTO price_points (mint, ts, price_micros, source) VALUES ($1, $2, $3, 'jupiter')`,
		mint, at, micros)
	if err != nil {
		t.Fatal(err)
	}
}

func movedEvents(t *testing.T, pool *pgxpool.Pool) []events.AssetPriceMoved {
	t.Helper()
	rows, err := pool.Query(t.Context(),
		`SELECT payload FROM events WHERE type = 'asset.price_moved' ORDER BY created_at, id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []events.AssetPriceMoved
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			t.Fatal(err)
		}
		var ev events.AssetPriceMoved
		if err := json.Unmarshal(payload, &ev); err != nil {
			t.Fatal(err)
		}
		out = append(out, ev)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func moveCount(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM asset_price_moves`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func wantMove(
	t *testing.T, got events.AssetPriceMoved, a market.Asset, threshold, change int64, mark uint64, day string,
	observed time.Time,
) {
	t.Helper()
	if got.V != 1 || got.AssetID != a.ID.UUID() || got.Symbol != a.Symbol || got.AssetName != "Apple" ||
		got.ThresholdBps != threshold || got.ChangeBps != change || got.PrevCloseMicros != usd(200_000_000) ||
		got.MarkMicros != usd(mark) || got.TradingDay != day || !got.ObservedAt.Equal(observed) {
		t.Fatalf("event = %+v, want %s threshold %d change %d on %s at %s",
			got, a.Symbol, threshold, change, day, observed.Format(time.RFC3339))
	}
}
