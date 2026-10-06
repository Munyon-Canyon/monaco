package social_test

import (
	"context"
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/social"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/adapters"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

type proposalRig struct {
	fixture
	conn     testkit.Bus
	cabal    uuid.UUID
	proposal uuid.UUID
	feed     adapters.Feed
}

func newProposalRig(t *testing.T) proposalRig {
	t.Helper()
	f := newFixture(t)
	b := testkit.NATS(t)
	r := proposalRig{
		fixture:  f,
		conn:     b,
		proposal: f.gen.NewV7(),
		feed: adapters.Feed{
			Bus:    b.Conn,
			Users:  f.users,
			Assets: social.NewAssets(marketfake.NewCatalog(marketfake.AAPLx())),
			IDs:    f.gen,
		},
	}
	r.cabal = createFeedCabal(t, f)
	return r
}

func (r proposalRig) created() events.ProposalCreated {
	return events.ProposalCreated{
		V: 1, ProposalID: r.proposal, CabalID: r.cabal, ProposerID: r.bob.UUID(), Kind: "buy", Symbol: "AAPLx",
		Mint: marketfake.AAPLx().Mint.Address(), USDCMicros: money.MicrosFromUint64(500_000_000),
		ExpiresAt: r.now.Add(24 * time.Hour), VoterCount: 3,
	}
}

func (r proposalRig) deliver(t *testing.T, fn func(context.Context, db.Tx) error) error {
	t.Helper()
	ctx := observability.WithEventID(t.Context(), ids.EventIDFrom(r.gen.NewV7()))
	return db.New(r.pool, r.gen, r.clock).Do(ctx, fn)
}

func (r proposalRig) create(t *testing.T) error {
	t.Helper()
	return r.deliver(t, func(ctx context.Context, tx db.Tx) error {
		e := r.created()
		card, err := r.feed.FetchProposal(ctx, e)
		if err != nil {
			return err
		}
		return r.feed.ApplyProposal(ctx, tx, e, card, r.clock.Now())
	})
}

func (r proposalRig) executed(t *testing.T) error {
	t.Helper()
	return r.deliver(t, func(ctx context.Context, tx db.Tx) error {
		return r.feed.ProposalExecuted(ctx, tx, events.ProposalExecuted{V: 1, ProposalID: r.proposal}, r.clock.Now())
	})
}

func (r proposalRig) passed(t *testing.T) error {
	t.Helper()
	return r.deliver(t, func(ctx context.Context, tx db.Tx) error {
		return r.feed.ProposalPassed(ctx, tx, events.ProposalPassed{V: 1, ProposalID: r.proposal}, r.clock.Now())
	})
}

func (r proposalRig) tradeFailed(t *testing.T, kind string) error {
	t.Helper()
	return r.deliver(t, func(ctx context.Context, tx db.Tx) error {
		return r.feed.TradeFailed(ctx, tx, events.TradeFailed{
			V: 1, Source: events.TradeSource{Kind: kind, ID: r.proposal}, FailureCode: string(errs.CodeFeedItemPending),
		}, r.clock.Now())
	})
}

type proposalRow struct {
	Status, Title, Body, Symbol string
	Payload                     map[string]any
	Updated                     time.Time
}

func (r proposalRig) row(t *testing.T) proposalRow {
	t.Helper()
	var (
		row proposalRow
		raw []byte
	)
	err := r.pool.QueryRow(t.Context(), `SELECT status, title, coalesce(body, ''), coalesce(symbol, ''), payload, updated_at
		FROM feed_objects WHERE kind = 'proposal' AND ref_id = $1`, r.proposal).
		Scan(&row.Status, &row.Title, &row.Body, &row.Symbol, &raw, &row.Updated)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &row.Payload); err != nil {
		t.Fatal(err)
	}
	return row
}

func TestFeedProposal_createdWritesTheOpenItem(t *testing.T) {
	t.Parallel()
	r := newProposalRig(t)
	sub := testkit.SubscribeCore(t, r.conn, hintSubject)
	if err := r.create(t); err != nil {
		t.Fatal(err)
	}
	row := r.row(t)
	if row.Status != "open" || row.Body != "" || row.Symbol != "AAPLx" ||
		row.Title != "bob proposed buying $500.00 of AAPLx in Alpha" {
		t.Fatalf("row = %+v", row)
	}
	for key, want := range map[string]any{
		"status": "open", "actor_name": "bob", "cabal_name": "Alpha", "asset_name": "Apple", "action": "buy",
		"usdc_micros": "500000000", "expires_at": r.created().ExpiresAt.UTC().Format(time.RFC3339),
	} {
		if row.Payload[key] != want {
			t.Errorf("payload[%s] = %v, want %v", key, row.Payload[key], want)
		}
	}
	var asset uuid.UUID
	err := r.pool.QueryRow(t.Context(), `SELECT asset_id FROM feed_objects WHERE ref_id = $1`, r.proposal).Scan(&asset)
	if err != nil || asset != marketfake.AAPLx().ID.UUID() {
		t.Fatalf("asset_id = %s, %v", asset, err)
	}
	wantHints(t, r.conn.Conn, sub, 1)
}

func TestFeedProposal_sellRendersSelling(t *testing.T) {
	t.Parallel()
	r := newProposalRig(t)
	sell := r.created()
	sell.Kind, sell.USDCMicros, sell.TokenAmount = "sell", money.Micros{}, 7
	if err := r.deliver(t, func(ctx context.Context, tx db.Tx) error {
		card, err := r.feed.FetchProposal(ctx, sell)
		if err != nil {
			return err
		}
		return r.feed.ApplyProposal(ctx, tx, sell, card, r.clock.Now())
	}); err != nil {
		t.Fatal(err)
	}
	if got := r.row(t); got.Title != "bob proposed selling AAPLx in Alpha" || got.Payload["token_amount"] != "7" {
		t.Fatalf("row = %+v", got)
	}
}

func TestFeedProposal_StatusOrder(t *testing.T) {
	t.Parallel()
	r := newProposalRig(t)
	sub := testkit.SubscribeCore(t, r.conn, hintSubject)
	if err := r.create(t); err != nil {
		t.Fatal(err)
	}
	r.clock.Advance(time.Minute)
	if err := r.executed(t); err != nil {
		t.Fatal(err)
	}
	executedAt := r.clock.Now()
	r.clock.Advance(time.Minute)
	if err := r.passed(t); err != nil {
		t.Fatal(err)
	}
	row := r.row(t)
	if row.Status != "executed" || row.Payload["status"] != "executed" || !row.Updated.Equal(executedAt) {
		t.Fatalf("after executed then passed: %+v, want executed at %s", row, executedAt)
	}
	wantHints(t, r.conn.Conn, sub, 2)
}

func TestFeedProposal_aFailedTradeThenARetryEndsExecuted(t *testing.T) {
	t.Parallel()
	r := newProposalRig(t)
	if err := r.create(t); err != nil {
		t.Fatal(err)
	}
	for _, step := range []struct {
		name    string
		deliver func(*testing.T) error
		status  string
		code    any
	}{
		{"passed", r.passed, "passed", ""},
		{"failed trade", func(t *testing.T) error {
			t.Helper()
			return r.tradeFailed(t, "proposal")
		}, "execution_failed", "feed_item_pending"},
		{"retried trade", r.executed, "executed", ""},
	} {
		if err := step.deliver(t); err != nil {
			t.Fatalf("%s: %v", step.name, err)
		}
		row := r.row(t)
		if row.Status != step.status || row.Payload["status"] != step.status ||
			row.Payload["status_code"] != step.code {
			t.Fatalf("after %s: %+v, want %s %v", step.name, row, step.status, step.code)
		}
	}
}

func TestFeedProposal_aStatusBeforeTheCreateNaksThenConverges(t *testing.T) {
	t.Parallel()
	r := newProposalRig(t)
	if got := errs.CodeOf(r.executed(t)); got != errs.CodeFeedItemPending {
		t.Fatalf("status before create = %s, want %s", got, errs.CodeFeedItemPending)
	}
	if err := r.create(t); err != nil {
		t.Fatal(err)
	}
	if err := r.executed(t); err != nil {
		t.Fatal(err)
	}
	if got := r.row(t).Status; got != "executed" {
		t.Fatalf("status = %s, want executed", got)
	}
}

func TestFeedProposal_aCreateBeforeItsCabalNaks(t *testing.T) {
	t.Parallel()
	r := newProposalRig(t)
	r.cabal = r.gen.NewV7()
	if got := errs.CodeOf(r.create(t)); got != errs.CodeFeedItemPending {
		t.Fatalf("create before cabal = %s, want %s", got, errs.CodeFeedItemPending)
	}
}

func TestFeedProposal_aCashoutTradeFailureLeavesProposalItemsAlone(t *testing.T) {
	t.Parallel()
	r := newProposalRig(t)
	if err := r.create(t); err != nil {
		t.Fatal(err)
	}
	before := r.row(t)
	r.clock.Advance(time.Minute)
	if err := r.tradeFailed(t, "cashout"); err != nil {
		t.Fatal(err)
	}
	if after := r.row(t); after.Status != "open" || !after.Updated.Equal(before.Updated) {
		t.Fatalf("row = %+v, want %+v", after, before)
	}
}

func TestFeedProposal_everyStatusEventSetsItsStatus(t *testing.T) {
	t.Parallel()
	cases := map[string]func(adapters.Feed, context.Context, db.Tx, uuid.UUID, time.Time) error{
		"failed": func(h adapters.Feed, ctx context.Context, tx db.Tx, id uuid.UUID, at time.Time) error {
			return h.ProposalFailed(ctx, tx, events.ProposalFailed{ProposalID: id}, at)
		},
		"expired": func(h adapters.Feed, ctx context.Context, tx db.Tx, id uuid.UUID, at time.Time) error {
			return h.ProposalExpired(ctx, tx, events.ProposalExpired{ProposalID: id}, at)
		},
		"withdrawn": func(h adapters.Feed, ctx context.Context, tx db.Tx, id uuid.UUID, at time.Time) error {
			return h.ProposalWithdrawn(ctx, tx, events.ProposalWithdrawn{ProposalID: id}, at)
		},
		"voided": func(h adapters.Feed, ctx context.Context, tx db.Tx, id uuid.UUID, at time.Time) error {
			return h.ProposalVoided(ctx, tx, events.ProposalVoided{ProposalID: id}, at)
		},
		"execution_blocked": func(h adapters.Feed, ctx context.Context, tx db.Tx, id uuid.UUID, at time.Time) error {
			return h.ProposalBlocked(ctx, tx, events.ProposalExecutionBlocked{
				ProposalID: id, Code: errs.CodeFeedItemPending,
			}, at)
		},
	}
	for status, apply := range cases {
		t.Run(status, func(t *testing.T) {
			t.Parallel()
			r := newProposalRig(t)
			if err := r.create(t); err != nil {
				t.Fatal(err)
			}
			if err := r.deliver(t, func(ctx context.Context, tx db.Tx) error {
				return apply(r.feed, ctx, tx, r.proposal, r.clock.Now())
			}); err != nil {
				t.Fatal(err)
			}
			if got := r.row(t).Status; got != status {
				t.Fatalf("status = %s, want %s", got, status)
			}
		})
	}
}

func TestFeedProposal_fetchRefusesWhatItCannotName(t *testing.T) {
	t.Parallel()
	for name, prepare := range map[string]func(*proposalRig, *events.ProposalCreated){
		"bad mint":      func(_ *proposalRig, e *events.ProposalCreated) { e.Mint = "not-a-mint" },
		"unknown asset": func(_ *proposalRig, e *events.ProposalCreated) { e.Mint = marketfake.TSLAx().Mint.Address() },
		"identity down": func(r *proposalRig, _ *events.ProposalCreated) {
			r.users.Fail("UsersByID", errs.New(errs.CodeDBUnavailable, "test"))
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := newProposalRig(t)
			e := r.created()
			prepare(&r, &e)
			if _, err := r.feed.FetchProposal(t.Context(), e); err == nil {
				t.Fatal("FetchProposal = nil, want an error")
			}
		})
	}
}

func TestFeedProposal_returnsEachWriteFailure(t *testing.T) {
	t.Parallel()
	for name, ddl := range map[string]string{
		"cabal lookup": "ALTER TABLE feed_cabals RENAME TO feed_cabals_gone",
		"insert":       "ALTER TABLE feed_objects RENAME TO feed_objects_gone",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := newProposalRig(t)
			if _, err := r.pool.Exec(t.Context(), ddl); err != nil {
				t.Fatal(err)
			}
			err := r.create(t)
			if err == nil || errs.CodeOf(err) == errs.CodeFeedItemPending {
				t.Fatalf("create = %v, want a database failure", err)
			}
		})
	}
	r := newProposalRig(t)
	if _, err := r.pool.Exec(t.Context(), "ALTER TABLE feed_objects RENAME TO feed_objects_gone"); err != nil {
		t.Fatal(err)
	}
	if err := r.executed(t); err == nil || errs.CodeOf(err) == errs.CodeFeedItemPending {
		t.Fatalf("status update = %v, want a database failure", err)
	}
}

func TestFeedProposal_theModuleReadsAssetsFromItsOption(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	for _, tc := range []struct {
		name    string
		catalog *marketfake.CatalogFake
		fails   bool
	}{
		{"known mint", marketfake.NewCatalog(marketfake.AAPLx()), false},
		{"empty catalog", marketfake.NewCatalog(), true},
	} {
		m := social.New(module.Deps{}, social.WithUsers(f.users), social.WithAssets(tc.catalog))
		for _, c := range m.Consumers() {
			for _, h := range c.Handlers {
				if h.Name != "social.feed.proposal_created" {
					continue
				}
				e := events.ProposalCreated{V: 1, ProposerID: f.bob.UUID(), Mint: marketfake.AAPLx().Mint.Address()}
				if _, err := h.Fetch(t.Context(), e); (err != nil) != tc.fails {
					t.Errorf("%s: Fetch err = %v, want failure %t", tc.name, err, tc.fails)
				}
			}
		}
	}
}

func TestSeed_feedTwoCabalsHoldsAnOpenAndAnExecutedProposal(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	alice, _ := ids.ParseUserID("01890a5d-ac96-774b-bcce-b302099a8101")
	bob, _ := ids.ParseUserID("01890a5d-ac96-774b-bcce-b302099a8103")
	users := fakes.NewIdentity([]identity.UserCard{
		{ID: alice, Handle: "alice"}, {ID: bob, Handle: "bob"},
	}, nil)
	m := social.New(
		module.Deps{Pool: f.pool, Bus: testkit.NATS(t).Conn, Clock: f.clock, IDs: f.gen},
		social.WithUsers(users), social.WithAssets(marketfake.NewCatalog(marketfake.AAPLx())),
	)
	testkit.Seed(t, f.pool, "feed-two-cabals", m.Consumers()...)
	rows, err := f.pool.Query(t.Context(), `SELECT cabal_name, status, title FROM feed_objects WHERE kind = 'proposal'
		ORDER BY cabal_name`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var cabal, status, title string
		if err := rows.Scan(&cabal, &status, &title); err != nil {
			t.Fatal(err)
		}
		got = append(got, cabal+"|"+status+"|"+title)
	}
	want := []string{
		"Alpha|open|alice proposed buying $5.00 of AAPLx in Alpha",
		"Beta|executed|bob proposed buying $7.50 of AAPLx in Beta",
	}
	if err := rows.Err(); err != nil || !slices.Equal(got, want) {
		t.Fatalf("proposal items = %v, %v, want %v", got, err, want)
	}
	var title string
	err = f.pool.QueryRow(t.Context(), `SELECT title FROM feed_objects WHERE kind = 'trade'`).Scan(&title)
	if err != nil || title != "Beta bought $7.50 of AAPLx" {
		t.Fatalf("trade item = %q, %v", title, err)
	}
	err = f.pool.QueryRow(t.Context(), `SELECT title FROM feed_objects WHERE kind = 'price_move'`).Scan(&title)
	if err != nil || title != "AAPLx is up 10% today" {
		t.Fatalf("price move item = %q, %v", title, err)
	}
}
