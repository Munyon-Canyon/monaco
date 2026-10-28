package app_test

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	dbsqlc "github.com/monaco/monaco/apps/backend/internal/platform/db/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type resolverMarket struct {
	assets    []market.Asset
	prices    map[market.AssetID]market.Price
	assetsErr error
	pricesErr error
	onPrices  func()
}

func (m *resolverMarket) PricesAsOf(
	context.Context, []market.AssetID, time.Time,
) (map[market.AssetID]market.Price, error) {
	if m.onPrices != nil {
		m.onPrices()
	}
	return m.prices, m.pricesErr
}

func (m *resolverMarket) AssetsByMints(context.Context, []string) ([]market.Asset, error) {
	return m.assets, m.assetsErr
}

func (*resolverMarket) ListingPrices(
	context.Context, []market.ListingKey, time.Time,
) (map[market.ListingKey]market.ListingPrice, error) {
	return map[market.ListingKey]market.ListingPrice{}, nil
}

func pollerContext(t *testing.T) context.Context {
	t.Helper()
	return auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorSystem, ID: "poller.ranking.calls_resolver"})
}

type rig struct {
	dbsqlc.DBTX
	uow *db.UnitOfWork
}

func newRig(t *testing.T) rig {
	t.Helper()
	pool := testkit.DB(t)
	return rig{DBTX: pool, uow: db.New(pool, ids.Real{}, clock.Real{})}
}

type callRow struct {
	proposal, cabal, yes, no, asset uuid.UUID
	kind, outcome, side             string
	fill, marketRef                 int64
	hasAsset                        bool
}

func (c callRow) insert(t *testing.T, pool rig, resolveAt time.Time) {
	t.Helper()
	var asset any
	if c.hasAsset {
		asset = c.asset
	}
	var fill, ref any
	if c.fill != 0 {
		fill = c.fill
	}
	if c.marketRef != 0 {
		ref = c.marketRef
	}
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{
			`INSERT INTO calls (proposal_id, cabal_id, kind, symbol, mint, asset_id, opened_at, decided_at, outcome,
			market_ref_micros, fill_price_micros, resolve_at, state, updated_at)
			VALUES ($1, $2, $3, 'AAPLx', 'mint', $4, $7, $7, $5, $6, $8, $9, 'decided', $7)`,
			[]any{c.proposal, c.cabal, c.kind, asset, c.outcome, ref, resolveAt.Add(-24 * time.Hour), fill, resolveAt},
		},
		{
			`INSERT INTO call_voters (proposal_id, user_id, cabal_id, choice, choice_at)
			VALUES ($1, $2, $4, 'yes', now()), ($1, $3, $4, 'no', now()), ($1, $5, $4, NULL, NULL)`,
			[]any{c.proposal, c.yes, c.no, c.cabal, ids.Real{}.NewV7()},
		},
		{
			`INSERT INTO cabal_calls (proposal_id, cabal_id, side) VALUES ($1, $2, $3)`,
			[]any{c.proposal, c.cabal, c.side},
		},
	} {
		if _, err := pool.Exec(t.Context(), q.sql, q.args...); err != nil {
			t.Fatalf("seed the call: %v", err)
		}
	}
}

func newCallRow(kind, outcome, side string, fill, marketRef int64) callRow {
	g := ids.Real{}
	return callRow{
		proposal: g.NewV7(), cabal: g.NewV7(), yes: g.NewV7(), no: g.NewV7(), asset: g.NewV7(),
		kind: kind, outcome: outcome, side: side, fill: fill, marketRef: marketRef, hasAsset: true,
	}
}

func (c callRow) market(kind string, micros uint64, observed time.Time) *resolverMarket {
	id := market.AssetIDFrom(c.asset)
	asset := market.Asset{ID: id}
	if kind == "pre_ipo" {
		asset.Kind = "pre_ipo"
	}
	return &resolverMarket{
		assets: []market.Asset{asset},
		prices: map[market.AssetID]market.Price{id: {Micros: money.MicrosFromUint64(micros), ObservedAt: observed}},
	}
}

func runResolver(t *testing.T, pool rig, m *resolverMarket, now time.Time) (app.ResolveReport, error) {
	t.Helper()
	return app.NewResolveCalls(pool, pool.uow, m).Run(pollerContext(t), now, app.CallsBatch)
}

type callState struct {
	state, source, reason                  string
	moveBps                                int
	end                                    int64
	resolvedAt                             time.Time
	yesRight, noRight, cabalRight, silence bool
}

func readCall(t *testing.T, pool rig, c callRow) callState {
	t.Helper()
	var got callState
	var source, reason *string
	var bps *int
	var end *int64
	var resolvedAt *time.Time
	err := pool.QueryRow(t.Context(), `SELECT c.state, c.ref_source, c.void_reason, c.move_bps, c.end_price_micros,
		c.resolved_at,
		coalesce((SELECT is_right FROM call_voters WHERE proposal_id = c.proposal_id AND user_id = $2), false),
		coalesce((SELECT is_right FROM call_voters WHERE proposal_id = c.proposal_id AND user_id = $3), false),
		coalesce((SELECT is_right FROM cabal_calls WHERE proposal_id = c.proposal_id), false),
		(SELECT is_right IS NULL FROM call_voters WHERE proposal_id = c.proposal_id AND choice IS NULL)
		FROM calls AS c WHERE c.proposal_id = $1`, c.proposal, c.yes, c.no).
		Scan(&got.state, &source, &reason, &bps, &end, &resolvedAt, &got.yesRight, &got.noRight, &got.cabalRight,
			&got.silence)
	if err != nil {
		t.Fatalf("read the call: %v", err)
	}
	if source != nil {
		got.source = *source
	}
	if reason != nil {
		got.reason = *reason
	}
	if bps != nil {
		got.moveBps = *bps
	}
	if end != nil {
		got.end = *end
	}
	if resolvedAt != nil {
		got.resolvedAt = *resolvedAt
	}
	return got
}

func expectCall(t *testing.T, got, want callState, resolveAt time.Time) {
	t.Helper()
	if !got.resolvedAt.Equal(resolveAt) {
		t.Fatalf("resolved_at = %v, want resolve_at %v", got.resolvedAt, resolveAt)
	}
	got.resolvedAt = time.Time{}
	if got != want {
		t.Fatalf("call = %+v, want %+v", got, want)
	}
}

func resolvedEvents(t *testing.T, pool rig) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM events WHERE type = 'call.resolved'`).
		Scan(&n); err != nil {
		t.Fatalf("count the events: %v", err)
	}
	return n
}

func TestResolveCalls_aFilledBuyResolvesAgainstTheFillOnceAndAppendsOneEvent(t *testing.T) {
	t.Parallel()
	pool := newRig(t)
	now := clock.Real{}.Now().UTC()
	resolveAt := now.Add(-time.Hour).Truncate(time.Microsecond)
	call := newCallRow("buy", "passed", "yes", 716_520_000, 700_000_000)
	call.insert(t, pool, resolveAt)
	m := call.market("equity", 724_400_000, resolveAt.Add(-time.Minute))
	for run := range 2 {
		got, err := runResolver(t, pool, m, now)
		wantResolved := 1 - run
		if err != nil || got.Resolved != wantResolved || got.Voided != 0 {
			t.Fatalf("run %d = %+v, %v, want %d resolved", run, got, err, wantResolved)
		}
	}
	want := callState{
		state: "resolved", source: "fill", moveBps: 109, end: 724_400_000,
		yesRight: true, cabalRight: true, silence: true,
	}
	expectCall(t, readCall(t, pool, call), want, resolveAt)
	if n := resolvedEvents(t, pool); n != 1 {
		t.Fatalf("call.resolved events = %d, want 1", n)
	}
}

func TestResolveCalls_aFailedSellFallsBackToTheMarketReferenceAndMissesWhenThePriceDrops(t *testing.T) {
	t.Parallel()
	pool := newRig(t)
	now := clock.Real{}.Now().UTC()
	resolveAt := now.Add(-time.Hour).Truncate(time.Microsecond)
	call := newCallRow("sell", "failed", "no", 0, 200_000_000)
	call.insert(t, pool, resolveAt)
	if _, err := runResolver(t, pool, call.market("equity", 180_000_000, resolveAt), now); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	want := callState{
		state: "resolved", source: "market", moveBps: -1000, end: 180_000_000,
		yesRight: true, silence: true,
	}
	expectCall(t, readCall(t, pool, call), want, resolveAt)
}

func TestResolveCalls_aRiseBelowOneBasisPointKeepsTheYesVoterRight(t *testing.T) {
	t.Parallel()
	pool := newRig(t)
	now := clock.Real{}.Now().UTC()
	resolveAt := now.Add(-time.Hour).Truncate(time.Microsecond)
	call := newCallRow("buy", "passed", "yes", 0, 1_000_000_000)
	call.insert(t, pool, resolveAt)
	if _, err := runResolver(t, pool, call.market("equity", 1_000_000_001, resolveAt), now); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	got := readCall(t, pool, call)
	if got.state != "resolved" || got.moveBps != 0 || !got.yesRight || got.noRight {
		t.Fatalf("call = %+v, want resolved with move 0 and the Yes voter right", got)
	}
}

func TestResolveCalls_sampleAgesDependOnTheAssetKind(t *testing.T) {
	t.Parallel()
	now := clock.Real{}.Now().UTC()
	resolveAt := now.Add(-time.Hour).Truncate(time.Microsecond)
	for _, tc := range []struct {
		name, kind string
		age        time.Duration
		want       string
	}{
		{"a live sample at the limit", "equity", 2 * time.Hour, "resolved"},
		{"a live sample past the limit", "equity", 2*time.Hour + time.Second, "decided"},
		{"a pre-IPO sample at the limit", "pre_ipo", 36 * time.Hour, "resolved"},
		{"a pre-IPO sample past the limit", "pre_ipo", 36*time.Hour + time.Second, "decided"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pool := newRig(t)
			call := newCallRow("buy", "passed", "yes", 0, 50_000_000)
			call.insert(t, pool, resolveAt)
			if _, err := runResolver(
				t,
				pool,
				call.market(tc.kind, 50_000_000, resolveAt.Add(-tc.age)),
				now,
			); err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			got := readCall(t, pool, call)
			if got.state != tc.want || (tc.want == "resolved" && (got.moveBps != 0 || got.yesRight || !got.noRight)) {
				t.Fatalf("call = %+v, want %s", got, tc.want)
			}
		})
	}
}

func TestResolveCalls_aCallWithoutAnAcceptedPriceWaitsThenVoidsAsUnpriced(t *testing.T) {
	t.Parallel()
	now := clock.Real{}.Now().UTC()
	for _, tc := range []struct {
		name  string
		build func(callRow) (callRow, *resolverMarket)
	}{
		{"no sample", func(c callRow) (callRow, *resolverMarket) {
			return c, &resolverMarket{assets: []market.Asset{{ID: market.AssetIDFrom(c.asset)}}}
		}},
		{"no asset", func(c callRow) (callRow, *resolverMarket) {
			c.hasAsset = false
			return c, &resolverMarket{}
		}},
		{"a price too large to store", func(c callRow) (callRow, *resolverMarket) {
			return c, c.market("equity", math.MaxUint64, now.Add(-time.Hour))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pool := newRig(t)
			call, m := tc.build(newCallRow("buy", "passed", "yes", 0, 50_000_000))
			call.insert(t, pool, now.Add(-time.Hour))
			assertWaitsThenVoids(t, pool, call, m, now)
		})
	}
}

func assertWaitsThenVoids(t *testing.T, pool rig, call callRow, m *resolverMarket, now time.Time) {
	t.Helper()
	if _, err := runResolver(t, pool, m, now); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got := readCall(t, pool, call); got.state != "decided" {
		t.Fatalf("call = %+v, want it left for the next tick inside the window", got)
	}
	got, err := runResolver(t, pool, m, now.Add(6*time.Hour))
	if err != nil || got.Voided != 1 {
		t.Fatalf("Run() = %+v, %v, want one void", got, err)
	}
	if state := readCall(t, pool, call); state.state != "void" || state.reason != "unpriced" {
		t.Fatalf("call = %+v, want void unpriced", state)
	}
	if n := resolvedEvents(t, pool); n != 0 {
		t.Fatalf("call.resolved events = %d, want 0", n)
	}
}

func TestResolveCalls_aCallWithNoReferenceVoidsAtOnce(t *testing.T) {
	t.Parallel()
	pool := newRig(t)
	now := clock.Real{}.Now().UTC()
	call := newCallRow("buy", "failed", "no", 0, 0)
	call.insert(t, pool, now.Add(-time.Hour))
	if _, err := pool.Exec(
		t.Context(),
		`UPDATE calls SET mint = NULL WHERE proposal_id = $1`,
		call.proposal,
	); err != nil {
		t.Fatalf("clear the mint: %v", err)
	}
	got, err := runResolver(t, pool, &resolverMarket{}, now)
	if err != nil || got.Voided != 1 {
		t.Fatalf("Run() = %+v, %v, want one void", got, err)
	}
	if state := readCall(t, pool, call); state.state != "void" || state.reason != "unpriced" {
		t.Fatalf("call = %+v, want void unpriced", state)
	}
}

func TestResolveCalls_aCallThatIsNotDueYetIsLeftAlone(t *testing.T) {
	t.Parallel()
	pool := newRig(t)
	now := clock.Real{}.Now().UTC()
	call := newCallRow("buy", "passed", "yes", 100_000_000, 0)
	call.insert(t, pool, now.Add(time.Minute))
	got, err := runResolver(t, pool, call.market("equity", 110_000_000, now), now)
	if err != nil || got.Scanned != 0 {
		t.Fatalf("Run() = %+v, %v, want nothing scanned", got, err)
	}
}

func TestResolveCalls_aCallTakenByAnotherWorkerIsSkipped(t *testing.T) {
	t.Parallel()
	pool := newRig(t)
	now := clock.Real{}.Now().UTC()
	call := newCallRow("buy", "passed", "yes", 100_000_000, 0)
	call.insert(t, pool, now.Add(-time.Hour))
	m := call.market("equity", 110_000_000, now.Add(-time.Hour))
	m.onPrices = func() {
		if _, err := pool.Exec(t.Context(), `UPDATE calls SET state = 'resolved' WHERE proposal_id = $1`,
			call.proposal); err != nil {
			t.Errorf("take the call: %v", err)
		}
	}
	got, err := runResolver(t, pool, m, now)
	if err != nil || got.Resolved != 0 || got.Voided != 0 {
		t.Fatalf("Run() = %+v, %v, want nothing resolved", got, err)
	}
	if n := resolvedEvents(t, pool); n != 0 {
		t.Fatalf("call.resolved events = %d, want 0", n)
	}
}

func TestResolveCalls_aMarketErrorFailsTheRun(t *testing.T) {
	t.Parallel()
	errBoom := errs.New(errs.CodeInternal, "test.boom")
	now := clock.Real{}.Now().UTC()
	for name, tc := range map[string]func(*resolverMarket){
		"assets": func(m *resolverMarket) { m.assetsErr = errBoom },
		"prices": func(m *resolverMarket) { m.pricesErr = errBoom },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			pool := newRig(t)
			call := newCallRow("buy", "passed", "yes", 100_000_000, 0)
			call.insert(t, pool, now.Add(-time.Hour))
			m := call.market("equity", 110_000_000, now)
			tc(m)
			if _, err := runResolver(t, pool, m, now); !errors.Is(err, errBoom) {
				t.Fatalf("Run() error = %v, want %v", err, errBoom)
			}
			if got := readCall(t, pool, call); got.state != "decided" {
				t.Fatalf("call = %+v, want it left decided", got)
			}
		})
	}
}

func TestResolveCalls_aCanceledContextFailsTheRun(t *testing.T) {
	t.Parallel()
	pool := newRig(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := app.NewResolveCalls(pool, pool.uow, &resolverMarket{}).
		Run(ctx, clock.Real{}.Now(), app.CallsBatch); err == nil {
		t.Fatal("Run() error = nil, want the canceled context")
	}
}

func TestResolveCalls_aMoveTooLargeToStoreVoidsTheCall(t *testing.T) {
	t.Parallel()
	pool := newRig(t)
	now := clock.Real{}.Now().UTC()
	call := newCallRow("buy", "passed", "yes", 1, 0)
	call.insert(t, pool, now.Add(-time.Hour))
	got, err := runResolver(t, pool, call.market("equity", 1_000_000_000_000, now.Add(-time.Hour)), now)
	if err != nil || got.Voided != 1 {
		t.Fatalf("Run() = %+v, %v, want one void", got, err)
	}
	if state := readCall(t, pool, call); state.state != "void" || state.reason != "unpriced" {
		t.Fatalf("call = %+v, want void unpriced", state)
	}
}

func TestResolveCalls_aFailedEventAppendRollsTheWholeBatchBack(t *testing.T) {
	t.Parallel()
	pool := newRig(t)
	now := clock.Real{}.Now().UTC()
	call := newCallRow("buy", "passed", "yes", 100_000_000, 0)
	call.insert(t, pool, now.Add(-time.Hour))
	m := call.market("equity", 110_000_000, now.Add(-time.Hour))
	if _, err := app.NewResolveCalls(pool, pool.uow, m).Run(t.Context(), now, app.CallsBatch); err == nil {
		t.Fatal("Run() error = nil, want the missing actor to fail the event append")
	}
	if got := readCall(t, pool, call); got.state != "decided" || got.yesRight {
		t.Fatalf("call = %+v, want it rolled back to decided", got)
	}
}

func TestResolveCalls_aCallOlderThanAWeekAcceptsADailySample(t *testing.T) {
	t.Parallel()
	now := clock.Real{}.Now().UTC()
	for _, tc := range []struct {
		name      string
		resolveAt time.Duration
		age       time.Duration
		want      string
	}{
		{"a daily sample 20h before a call 10 days old", 10 * 24 * time.Hour, 20 * time.Hour, "resolved"},
		{"a sample at 36h before a call 10 days old", 10 * 24 * time.Hour, 36 * time.Hour, "resolved"},
		{"a sample past 36h before a call 10 days old", 10 * 24 * time.Hour, 36*time.Hour + time.Second, "void"},
		{"a call just past the week", 7*24*time.Hour + time.Second, 20 * time.Hour, "resolved"},
		{"a daily sample for a call at the week", 7 * 24 * time.Hour, 20 * time.Hour, "void"},
		{"a daily sample for a call 2 days old", 48 * time.Hour, 20 * time.Hour, "void"},
		{"a 2h sample for a call 2 days old", 48 * time.Hour, 2 * time.Hour, "resolved"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pool := newRig(t)
			resolveAt := now.Add(-tc.resolveAt).Truncate(time.Microsecond)
			call := newCallRow("buy", "passed", "yes", 0, 50_000_000)
			call.insert(t, pool, resolveAt)
			if _, err := runResolver(
				t, pool, call.market("equity", 55_000_000, resolveAt.Add(-tc.age)), now,
			); err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			got := readCall(t, pool, call)
			if got.state != tc.want || (tc.want == "void" && got.reason != "unpriced") ||
				(tc.want == "resolved" && !got.yesRight) {
				t.Fatalf("call = %+v, want %s", got, tc.want)
			}
		})
	}
}
