package ranking_test

import (
	"crypto/sha256"
	"fmt"
	"math/rand/v2"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/events"
	governanceport "github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/tools/ops/replay"
)

func backfillRunbook() []string {
	return []string{
		"proposal_created", "vote_cast", "vote_retracted", "proposal_passed", "proposal_failed",
		"proposal_expired", "proposal_withdrawn", "proposal_voided", "trade_confirmed",
	}
}

const (
	backfillDecidedAgo = 10 * 24 * time.Hour
	backfillDecimals   = 6
)

type backfillAsset struct {
	name, kind, issuer string
	samples            map[time.Duration]int64
}

type backfillStory struct {
	name, kind, asset, outcome string
	fill                       int64
	want                       backfillWant
}

type backfillWant struct {
	state, void, source string
	ref, end            int64
	bps                 int
	alice, bob          string
	cabal               string
}

func backfillAssets() []backfillAsset {
	return []backfillAsset{
		{
			"equity-up",
			"equity",
			"xstocks",
			map[time.Duration]int64{-20 * time.Hour: 100_000_000, 4 * time.Hour: 110_000_000},
		},
		{
			"equity-down",
			"equity",
			"xstocks",
			map[time.Duration]int64{-20 * time.Hour: 200_000_000, 4 * time.Hour: 180_000_000},
		},
		{
			"pre-ipo",
			"pre_ipo",
			"tessera",
			map[time.Duration]int64{-20 * time.Hour: 50_000_000, 4 * time.Hour: 50_000_000},
		},
		{"stale", "equity", "xstocks", map[time.Duration]int64{-100 * time.Hour: 30_000_000}},
	}
}

func backfillStories() []backfillStory {
	return []backfillStory{
		{
			name: "buy-passed-filled", kind: "buy", asset: "equity-up", outcome: "passed", fill: 101_000_000,
			want: backfillWant{
				"resolved", "", "fill", 101_000_000, 110_000_000, 891, "right", "wrong", "right",
			},
		},
		{
			name: "sell-failed", kind: "sell", asset: "equity-down", outcome: "failed",
			want: backfillWant{
				"resolved", "", "market", 200_000_000, 180_000_000, -1000, "right", "wrong", "wrong",
			},
		},
		{
			name: "buy-expired", kind: "buy", asset: "equity-up", outcome: "expired",
			want: backfillWant{
				"resolved", "", "market", 100_000_000, 110_000_000, 1000, "right", "wrong", "wrong",
			},
		},
		{
			name: "buy-withdrawn", kind: "buy", asset: "equity-up", outcome: "withdrawn",
			want: backfillWant{state: "void", void: "withdrawn", alice: "open", bob: "open", cabal: "none"},
		},
		{
			name: "buy-pre-ipo", kind: "buy", asset: "pre-ipo", outcome: "passed",
			want: backfillWant{
				"resolved", "", "market", 50_000_000, 50_000_000, 0, "wrong", "right", "wrong",
			},
		},
		{
			name: "buy-unpriced", kind: "buy", asset: "stale", outcome: "passed",
			want: backfillWant{state: "void", void: "unpriced", alice: "open", bob: "open", cabal: "open"},
		},
	}
}

type backfillEvent struct {
	id uuid.UUID
	ev events.Event
}

type backfillWorld struct {
	pool    *pgxpool.Pool
	clk     *testkit.Clock
	late    *testkit.Clock
	uow     *db.UnitOfWork
	module  *ranking.Module
	market  app.Market
	events  []backfillEvent
	decided time.Time
	now     time.Time
}

func backfillID(parts ...string) uuid.UUID {
	id := uuid.NewSHA1(uuid.NameSpaceOID, []byte(strings.Join(parts, "/")))
	id[6] = id[6]&0x0f | 0x70
	return id
}

func backfillMint(name string) chain.SolanaAddress {
	sum := sha256.Sum256([]byte("backfill-mint/" + name))
	return chain.AddressOf(sum[:])
}

func newBackfillWorld(t *testing.T, now time.Time) *backfillWorld {
	t.Helper()
	pool, gen := testkit.DB(t), testkit.NewIDs(6452)
	w := &backfillWorld{
		pool:    pool,
		clk:     testkit.NewClock(now.Add(-backfillDecidedAgo)),
		late:    testkit.NewClock(now),
		decided: now.Add(-backfillDecidedAgo),
		now:     now,
	}
	w.uow = db.New(pool, gen, w.clk)
	ballots := callsBallots{}
	for _, s := range backfillStories() {
		proposal := backfillID(s.name)
		ballots[proposal] = []governanceport.Ballot{
			ballotOf(backfillID("alice"), "yes", w.decided.Add(-3*time.Hour)),
			ballotOf(backfillID("bob"), "no", w.decided.Add(-160*time.Minute)),
			ballotOf(backfillID("carol"), "", time.Time{}),
		}
	}
	deps := module.Deps{Pool: pool, UoW: w.uow, IDs: gen, Clock: w.late, Config: testkit.Config()}
	markets := market.New(deps)
	w.market = liveMarket{
		Catalog: markets.Catalog(), Prices: markets.Prices(), Calendar: markets.Calendar(), listings: markets.Prices(),
	}
	w.module = ranking.New(deps, ranking.WithPorts(app.Ports{Market: w.market, Ballots: ballots}))
	w.seedMarket(t)
	w.seedEvents(t)
	return w
}

func (w *backfillWorld) exec(t *testing.T, query string, args ...any) {
	t.Helper()
	if _, err := w.pool.Exec(t.Context(), query, args...); err != nil {
		t.Fatalf("seed the history: %v", err)
	}
}

func (w *backfillWorld) seedMarket(t *testing.T) {
	t.Helper()
	for _, a := range backfillAssets() {
		mint := string(backfillMint(a.name))
		w.exec(
			t,
			`INSERT INTO assets (id, symbol, mint, decimals, issuer, kind, display_name, issuer_tradable,
			company_key, first_seen_at, updated_at, chain_checked_at)
			VALUES ($1, $2, $3, $4, $5, $6, $2, true, $2, $7, $7, $7)`,
			backfillID(
				"asset",
				a.name,
			),
			"BF"+strings.ToUpper(a.name),
			mint,
			backfillDecimals,
			a.issuer,
			a.kind,
			w.decided,
		)
		for offset, price := range a.samples {
			w.exec(t, `INSERT INTO price_points (mint, ts, price_micros, source) VALUES ($1, $2, $3, 'jupiter')`,
				mint, w.decided.Add(offset), price)
		}
	}
}

func (w *backfillWorld) seedEvents(t *testing.T) {
	t.Helper()
	d := deliverer{pool: w.pool, gen: testkit.NewIDs(6453), clock: w.clk, uow: w.uow}
	cabal := backfillID("cabal")
	add := func(ev events.Event) { w.events = append(w.events, backfillEvent{d.event(t, ev), ev}) }
	for _, s := range backfillStories() {
		proposal, mint, symbol := backfillID(s.name), backfillMint(s.asset), "BF"+strings.ToUpper(s.asset)
		add(
			events.ProposalCreated{
				V:          1,
				ProposalID: proposal,
				CabalID:    cabal,
				Kind:       s.kind,
				Symbol:     symbol,
				Mint:       mint,
			},
		)
		alice, bob, carol := backfillID("alice"), backfillID("bob"), backfillID("carol")
		vote := func(voter uuid.UUID, choice string, ago time.Duration) {
			add(events.VoteCast{
				V: 1, ProposalID: proposal, CabalID: cabal, VoterID: voter, Choice: choice, CastAt: w.decided.Add(-ago),
			})
		}
		vote(alice, "yes", 3*time.Hour)
		vote(bob, "yes", 170*time.Minute)
		vote(bob, "no", 160*time.Minute)
		vote(carol, "yes", 150*time.Minute)
		add(events.VoteRetracted{
			V: 1, ProposalID: proposal, CabalID: cabal, VoterID: carol, Choice: "yes",
			RetractedAt: w.decided.Add(-140 * time.Minute),
		})
		switch s.outcome {
		case "passed":
			add(
				events.ProposalPassed{
					V:          1,
					ProposalID: proposal,
					CabalID:    cabal,
					Kind:       s.kind,
					Symbol:     symbol,
					Mint:       mint,
				},
			)
		case "failed":
			add(events.ProposalFailed{V: 1, ProposalID: proposal, CabalID: cabal})
		case "expired":
			add(events.ProposalExpired{V: 1, ProposalID: proposal, CabalID: cabal})
		case "withdrawn":
			add(events.ProposalWithdrawn{V: 1, ProposalID: proposal, CabalID: cabal})
		}
		if s.fill > 0 {
			add(events.TradeConfirmed{
				V: 1, SwapID: backfillID("swap", s.name), CabalID: cabal,
				Source: events.TradeSource{Kind: "proposal", ID: proposal}, Action: s.kind, Symbol: symbol,
				InMint: chain.SolanaAddress(callsUSDC), InAmount: uint64(s.fill), OutMint: mint, OutAmount: 1_000_000,
				USDCMicros: money.MicrosFromUint64(uint64(s.fill)), ConfirmedAt: w.decided.Add(5 * time.Minute),
			})
		}
	}
}

func (w *backfillWorld) handler(t *testing.T, name string) bus.HandlerSpec {
	t.Helper()
	for _, c := range w.module.Consumers() {
		for _, h := range c.Handlers {
			if h.Name == name {
				return h
			}
		}
	}
	t.Fatalf("handler %s is not registered", name)
	return bus.HandlerSpec{}
}

func (w *backfillWorld) dispatch(t *testing.T, order []backfillEvent) {
	t.Helper()
	byType := map[events.Type]bus.HandlerSpec{}
	for _, c := range w.module.Consumers() {
		if c.Durable != "ranking_calls" {
			continue
		}
		for _, h := range c.Handlers {
			byType[h.Type()] = h
		}
	}
	for _, e := range order {
		ctx := observability.WithEventID(t.Context(), ids.EventIDFrom(e.id))
		if _, err := bus.Deliver(ctx, w.uow, w.late, byType[e.ev.Type()], ids.EventIDFrom(e.id), e.ev); err != nil {
			t.Fatalf("Deliver(%s) = %v", e.ev.Type(), err)
		}
	}
}

func (w *backfillWorld) resolve(t *testing.T) app.ResolveReport {
	t.Helper()
	ctx := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorSystem, ID: "poller.ranking.calls_resolver"})
	rep, err := app.NewResolveCalls(w.pool, w.uow, w.market).Run(ctx, w.now, app.CallsBatch)
	if err != nil {
		t.Fatalf("resolver tick = %v", err)
	}
	return rep
}

func (w *backfillWorld) backfill(t *testing.T, name string) replay.Report {
	t.Helper()
	h := w.handler(t, "ranking.calls."+name)
	rep, err := replay.Backfill(t.Context(), replay.BackfillOptions{
		Pool: w.pool, UoW: w.uow, Clock: &replay.Clock{}, Now: w.late, Handler: h,
		Types: []events.Type{events.Type(strings.Replace(name, "_", ".", 1))},
	})
	if err != nil {
		t.Fatalf("backfill %s = %v", name, err)
	}
	return rep
}

func (w *backfillWorld) rows(t *testing.T) []string {
	t.Helper()
	var out []string
	for _, q := range []string{
		`SELECT (to_jsonb(t) - 'updated_at')::text FROM calls t ORDER BY proposal_id`,
		`SELECT to_jsonb(t)::text FROM call_voters t ORDER BY proposal_id, user_id`,
		`SELECT to_jsonb(t)::text FROM cabal_calls t ORDER BY proposal_id`,
	} {
		rows, _ := w.pool.Query(t.Context(), q)
		got, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, got...)
	}
	return out
}

func (w *backfillWorld) assertStories(t *testing.T) {
	t.Helper()
	right := func(b *bool) string {
		switch {
		case b == nil:
			return "open"
		case *b:
			return "right"
		}
		return "wrong"
	}
	for _, s := range backfillStories() {
		var got backfillWant
		var source, void *string
		var ref, end *int64
		var bps *int
		var alice, bob, cabal *bool
		var cabalRows int
		proposal := backfillID(s.name)
		if err := w.pool.QueryRow(t.Context(), `SELECT c.state, c.void_reason, c.ref_source, c.ref_price_micros,
			c.end_price_micros, c.move_bps,
			(SELECT is_right FROM call_voters WHERE proposal_id = c.proposal_id AND user_id = $2),
			(SELECT is_right FROM call_voters WHERE proposal_id = c.proposal_id AND user_id = $3),
			(SELECT is_right FROM cabal_calls WHERE proposal_id = c.proposal_id),
			(SELECT count(*) FROM cabal_calls WHERE proposal_id = c.proposal_id)
			FROM calls c WHERE c.proposal_id = $1`, proposal, backfillID("alice"), backfillID("bob")).
			Scan(&got.state, &void, &source, &ref, &end, &bps, &alice, &bob, &cabal, &cabalRows); err != nil {
			t.Fatalf("%s: read the call: %v", s.name, err)
		}
		got.void, got.source = deref(void), deref(source)
		got.ref, got.end = derefInt(ref), derefInt(end)
		if bps != nil {
			got.bps = *bps
		}
		got.alice, got.bob, got.cabal = right(alice), right(bob), right(cabal)
		if cabalRows == 0 {
			got.cabal = "none"
		}
		if got != s.want {
			t.Errorf("%s: call = %+v, want %+v", s.name, got, s.want)
		}
		var silent, retracted int
		if err := w.pool.QueryRow(t.Context(), `SELECT
			count(*) FILTER (WHERE user_id = $2 AND choice IS NULL AND is_right IS NULL),
			count(*) FILTER (WHERE user_id = $3 AND choice = 'no')
			FROM call_voters WHERE proposal_id = $1`, proposal, backfillID("carol"), backfillID("bob")).
			Scan(&silent, &retracted); err != nil || silent != 1 || retracted != 1 {
			t.Errorf(
				"%s: carol retracted and bob changed to no = %d, %d, %v, want 1, 1",
				s.name,
				silent,
				retracted,
				err,
			)
		}
	}
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func derefInt(n *int64) int64 {
	if n == nil {
		return 0
	}
	return *n
}

func backfillNow() time.Time { return clock.Real{}.Now().UTC().Truncate(time.Hour) }

func (w *backfillWorld) runRunbook(t *testing.T, second bool) {
	t.Helper()
	for _, name := range backfillRunbook() {
		stored := 0
		for _, e := range w.events {
			if string(e.ev.Type()) == strings.Replace(name, "_", ".", 1) {
				stored++
			}
		}
		rep := w.backfill(t, name)
		applied, duplicates := stored, 0
		if second {
			applied, duplicates = 0, stored
		}
		if rep.Events != stored || rep.Applied != applied || rep.Duplicates != duplicates {
			t.Fatalf("backfill %s (second pass %v) = %+v, want %d applied and %d duplicate",
				name, second, rep, applied, duplicates)
		}
	}
}

func TestCallsBackfill_theRunbookCommandsReplayTheHistoryAndAreIdempotent(t *testing.T) {
	t.Parallel()
	w := newBackfillWorld(t, backfillNow())
	w.runRunbook(t, false)
	if rep := w.resolve(t); rep.Resolved != 4 || rep.Voided != 1 {
		t.Fatalf("resolver tick = %+v, want 4 resolved and the unpriced call voided", rep)
	}
	w.assertStories(t)
	first := w.rows(t)
	w.runRunbook(t, true)
	if rep := w.resolve(t); rep.Scanned != 0 {
		t.Fatalf("second resolver tick = %+v, want nothing left to judge", rep)
	}
	if second := w.rows(t); !slices.Equal(first, second) {
		t.Fatalf("a second pass changed the rows:\nfirst  %q\nsecond %q", first, second)
	}
}

func TestCallsBackfill_theRunbookNamesEveryHandlerOfTheConsumerAndTheDocListsEachCommand(t *testing.T) {
	t.Parallel()
	w := newBackfillWorld(t, backfillNow())
	var registered []string
	for _, c := range w.module.Consumers() {
		if c.Durable != "ranking_calls" {
			continue
		}
		for _, h := range c.Handlers {
			registered = append(registered, strings.TrimPrefix(h.Name, "ranking.calls."))
		}
	}
	slices.Sort(registered)
	if want := slices.Sorted(slices.Values(backfillRunbook())); !slices.Equal(registered, want) {
		t.Fatalf("ranking_calls handlers = %q, runbook = %q", registered, want)
	}
	doc, err := os.ReadFile("../../../../../docs/architecture/right-calls.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range backfillRunbook() {
		command := fmt.Sprintf("monacoctl backfill --consumer ranking.calls.%s --types %s",
			name, strings.Replace(name, "_", ".", 1))
		if !strings.Contains(string(doc), command) {
			t.Errorf("right-calls.md lacks the command %q", command)
		}
	}
}

func pinnedAfterCreated(t events.Type) bool {
	return t == events.TypeProposalFailed || t == events.TypeProposalExpired
}

func shuffledBackfill(history []backfillEvent, seed uint64) []backfillEvent {
	r := rand.New(rand.NewPCG(seed, seed))
	var free, pinned []backfillEvent
	for _, e := range history {
		if pinnedAfterCreated(e.ev.Type()) {
			pinned = append(pinned, e)
		} else {
			free = append(free, e)
		}
	}
	r.Shuffle(len(free), func(i, j int) { free[i], free[j] = free[j], free[i] })
	for _, p := range pinned {
		created := slices.IndexFunc(free, func(e backfillEvent) bool {
			return e.ev.Type() == events.TypeProposalCreated && e.ev.AggregateID() == p.ev.AggregateID()
		})
		at := created + 1 + r.IntN(len(free)-created)
		free = slices.Insert(free, at, p)
	}
	return free
}

func TestCallsBackfill_theStoreConvergesInAnyDeliveryOrder(t *testing.T) {
	t.Parallel()
	now := backfillNow()
	byType := func(history []backfillEvent, first ...events.Type) []backfillEvent {
		out := slices.Clone(history)
		slices.SortStableFunc(out, func(a, b backfillEvent) int {
			return slices.Index(first, b.ev.Type()) - slices.Index(first, a.ev.Type())
		})
		return out
	}
	orders := map[string]func([]backfillEvent) []backfillEvent{
		"event order": slices.Clone[[]backfillEvent],
		"fills before their decisions": func(h []backfillEvent) []backfillEvent {
			return byType(h, events.TypeTradeConfirmed)
		},
		"votes in reverse": func(h []backfillEvent) []backfillEvent {
			votes := slices.DeleteFunc(slices.Clone(h), func(e backfillEvent) bool {
				return e.ev.Type() != events.TypeVoteCast && e.ev.Type() != events.TypeVoteRetracted
			})
			slices.Reverse(votes)
			rest := slices.DeleteFunc(slices.Clone(h), func(e backfillEvent) bool {
				return e.ev.Type() == events.TypeVoteCast || e.ev.Type() == events.TypeVoteRetracted
			})
			return append(votes, rest...)
		},
		"shuffle 1": func(h []backfillEvent) []backfillEvent { return shuffledBackfill(h, 1) },
		"shuffle 2": func(h []backfillEvent) []backfillEvent { return shuffledBackfill(h, 2) },
		"shuffle 3": func(h []backfillEvent) []backfillEvent { return shuffledBackfill(h, 3) },
	}
	var mu sync.Mutex
	snapshots := map[string][]string{}
	t.Cleanup(func() {
		for name, got := range snapshots {
			if want := snapshots["event order"]; !slices.Equal(got, want) {
				t.Errorf("%s left different rows from event order:\n got %q\nwant %q", name, got, want)
			}
		}
	})
	for name, order := range orders {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			w := newBackfillWorld(t, now)
			w.dispatch(t, order(w.events))
			if rep := w.resolve(t); rep.Resolved != 4 || rep.Voided != 1 {
				t.Fatalf("resolver tick = %+v, want 4 resolved and the unpriced call voided", rep)
			}
			w.assertStories(t)
			mu.Lock()
			defer mu.Unlock()
			snapshots[name] = w.rows(t)
		})
	}
}

func TestCallsBackfill_aMintWithNoSampleWithin36HoursVoidsUnpriced(t *testing.T) {
	t.Parallel()
	w := newBackfillWorld(t, backfillNow())
	w.dispatch(t, w.events)
	w.resolve(t)
	var state, reason string
	if err := w.pool.QueryRow(t.Context(), `SELECT state, void_reason FROM calls WHERE proposal_id = $1`,
		backfillID("buy-unpriced")).Scan(&state, &reason); err != nil || state != "void" || reason != "unpriced" {
		t.Fatalf("call on the stale mint = %s %s, %v, want void unpriced", state, reason, err)
	}
}
