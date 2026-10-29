package ranking_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/modules/social"
	socialport "github.com/monaco/monaco/apps/backend/internal/modules/social/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	apibase "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/rankingapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const (
	stockMint        = "So11111111111111111111111111111111111111112"
	insertLedgerCall = `INSERT INTO calls (proposal_id, cabal_id, kind, symbol, mint, opened_at, decided_at, outcome, state,
  move_bps, resolved_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, 'passed', 'resolved', $8, $9, $9)`
	insertLedgerVoter = `INSERT INTO call_voters (proposal_id, user_id, cabal_id, choice, choice_at, is_right)
VALUES ($1, $2, $3, $4, $5, $6)`
)

func ledgerDelivery() time.Time { return time.Date(2026, 10, 18, 17, 0, 0, 0, time.UTC) }

func idOf(name string) uuid.UUID { return uuid.NewSHA1(uuid.Nil, []byte(name)) }

type week42 struct {
	andres, christelle ids.UserID
	acCapital          ids.CabalID
	techTitans, degens ids.CabalID
	amzn, tsla, nvda   uuid.UUID
	inputs             app.LedgerInputs
	replies            *fakeReplies
	trades             *fakeTrades
}

type tripwire struct {
	calls, failOn int
}

func (w *tripwire) trip() error {
	w.calls++
	if w.calls == w.failOn {
		return errs.New(errs.CodeInternal, "boom")
	}
	return nil
}

type fakeMembers struct {
	cabals []ids.CabalID
	wire   *tripwire
}

func (f fakeMembers) CabalsOf(context.Context, ids.UserID) ([]ids.CabalID, error) {
	return f.cabals, f.wire.trip()
}

type fakeCards struct {
	names map[ids.CabalID]string
	wire  *tripwire
}

func (f fakeCards) Cabals(_ context.Context, in []ids.CabalID) (map[ids.CabalID]app.CabalView, error) {
	out := map[ids.CabalID]app.CabalView{}
	for _, id := range in {
		out[id] = app.CabalView{ID: id, Name: f.names[id]}
	}
	return out, f.wire.trip()
}

type fakeUsers struct {
	names map[ids.UserID]string
	wire  *tripwire
}

func (f fakeUsers) UsersByID(_ context.Context, in []ids.UserID) (map[ids.UserID]identity.UserCard, error) {
	out := map[ids.UserID]identity.UserCard{}
	for _, id := range in {
		out[id] = identity.UserCard{ID: id, DisplayName: f.names[id]}
	}
	return out, f.wire.trip()
}

type fakeMarket struct {
	app.Market
	wire *tripwire
}

func (f fakeMarket) AssetsByMints(context.Context, []string) ([]market.Asset, error) {
	mint, _ := market.ParseMint(stockMint)
	return []market.Asset{{Mint: mint, LogoURL: "https://logo.test/amzn.png"}}, f.wire.trip()
}

type fakeTrades struct {
	buys []trading.FilledBuy
	wire *tripwire
}

func (f *fakeTrades) FilledBuysBetween(
	context.Context, []ids.CabalID, time.Time, time.Time,
) ([]trading.FilledBuy, error) {
	return f.buys, f.wire.trip()
}

type fakeReplies struct {
	top  *socialport.TopReply
	wire *tripwire
}

func (f *fakeReplies) TopReply(
	context.Context, []ids.ProposalID, time.Time, time.Time,
) (*socialport.TopReply, error) {
	return f.top, f.wire.trip()
}

type failStore struct {
	app.LedgerStore
	wire *tripwire
}

func (f failStore) UserTurnoutBetween(
	ctx context.Context, arg sqlc.UserTurnoutBetweenParams,
) (sqlc.UserTurnoutBetweenRow, error) {
	if err := f.wire.trip(); err != nil {
		return sqlc.UserTurnoutBetweenRow{}, err
	}
	return f.LedgerStore.UserTurnoutBetween(ctx, arg)
}

type failCalls struct{ failStore }

func (f failCalls) UserCallsResolvedBetween(
	context.Context, sqlc.UserCallsResolvedBetweenParams,
) ([]sqlc.UserCallsResolvedBetweenRow, error) {
	return nil, errs.New(errs.CodeInternal, "boom")
}

type failMates struct{ failStore }

func (f failMates) CabalmatesRecordsBetween(
	context.Context, sqlc.CabalmatesRecordsBetweenParams,
) ([]sqlc.CabalmatesRecordsBetweenRow, error) {
	return nil, errs.New(errs.CodeInternal, "boom")
}

type failProposals struct{ failStore }

func (f failProposals) CallProposalsInCabals(
	context.Context, sqlc.CallProposalsInCabalsParams,
) ([]sqlc.CallProposalsInCabalsRow, error) {
	return nil, errs.New(errs.CodeInternal, "boom")
}

func seedWeek42(t *testing.T) week42 {
	t.Helper()
	pool := testkit.DB(t)
	w := week42{
		andres: ids.UserIDFrom(idOf("andres")), christelle: ids.UserIDFrom(idOf("christelle")),
		acCapital: ids.CabalIDFrom(idOf("ac")), techTitans: ids.CabalIDFrom(idOf("tt")),
		degens: ids.CabalIDFrom(idOf("degens")),
		amzn:   idOf("amzn"), tsla: idOf("tsla"), nvda: idOf("nvda"),
		replies: &fakeReplies{wire: &tripwire{}}, trades: &fakeTrades{wire: &tripwire{}},
	}
	sat := time.Date(2026, 10, 17, 15, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		id         uuid.UUID
		cabal      ids.CabalID
		kind, sym  string
		decided    time.Time
		move       int
		andresSide string
		andresOK   bool
	}{
		{w.amzn, w.degens, "sell", "AMZN", sat, -420, "yes", true},
		{w.tsla, w.techTitans, "sell", "TSLA", sat.Add(-24 * time.Hour), -240, "no", false},
		{w.nvda, w.acCapital, "buy", "NVDA", sat.Add(-48 * time.Hour), 100, "yes", true},
	} {
		resolved := c.decided.Add(24 * time.Hour)
		exec(t, pool, insertLedgerCall, c.id, c.cabal.UUID(), c.kind, c.sym, stockMint,
			c.decided.Add(-48*time.Hour), c.decided, c.move, resolved)
		exec(t, pool, insertLedgerVoter, c.id, w.andres.UUID(), c.cabal.UUID(), c.andresSide, c.decided, c.andresOK)
		exec(t, pool, insertLedgerVoter, c.id, w.christelle.UUID(), c.cabal.UUID(), "yes", c.decided, true)
	}
	w.inputs = app.LedgerInputs{
		Store:  failStore{LedgerStore: sqlc.New(pool), wire: &tripwire{}},
		Market: fakeMarket{wire: &tripwire{}},
		Users: fakeUsers{
			names: map[ids.UserID]string{w.andres: "Andres", w.christelle: "Christelle"}, wire: &tripwire{},
		},
		Members: fakeMembers{cabals: []ids.CabalID{w.degens, w.techTitans, w.acCapital}, wire: &tripwire{}},
		Cards: fakeCards{
			names: map[ids.CabalID]string{w.acCapital: "AC Capital", w.techTitans: "Tech Titans", w.degens: "Degens"},
			wire:  &tripwire{},
		},
		Trades: w.trades, Replies: w.replies,
	}
	return w
}

type week42Summary struct {
	Week, LabelStart, LabelEnd              string
	Voted, Eligible, Right, Resolved, Calls int
	Cabals                                  []string
	Sharpest, Best, Miss, Reply             string
	Markers                                 []string
	HasReturn                               bool
	Series                                  int
}

func callLine(c *domain.LedgerCall) string {
	if c == nil {
		return ""
	}
	logo := ""
	if c.AssetLogoURL != nil {
		logo = *c.AssetLogoURL
	}
	return fmt.Sprintf("%s %s %s %d %t %s", c.Symbol, c.Cabal.Name, c.YourChoice, c.MoveBps, c.Right, logo)
}

func summarize(l domain.Ledger) week42Summary {
	out := week42Summary{
		Week: l.Week, LabelStart: l.LabelStart, LabelEnd: l.LabelEnd, Voted: l.Voted, Eligible: l.Eligible,
		Right: l.Right, Resolved: l.Resolved, Calls: len(l.Calls), HasReturn: l.StakeReturnBps != nil,
		Series: len(l.Series), Best: callLine(l.BestCall), Miss: callLine(l.Miss),
	}
	for _, c := range l.Cabals {
		out.Cabals = append(out.Cabals, c.Name)
	}
	for _, m := range l.Markers {
		out.Markers = append(out.Markers, m.Kind+" "+m.Label)
	}
	if l.Sharpest != nil {
		out.Sharpest = fmt.Sprintf("%s %d", l.Sharpest.User.Name, l.Sharpest.RateBps)
	}
	if r := l.TopReply; r != nil {
		out.Reply = fmt.Sprintf(
			"%s %s %s %d %s %d",
			r.ProposalLabel,
			r.Author.Name,
			r.TopEmoji,
			r.TopEmojiCount,
			r.Excerpt,
			r.ReactionCount,
		)
	}
	return out
}

func TestBuildLedger_Week42(t *testing.T) {
	t.Parallel()
	w := seedWeek42(t)
	w.replies.top = &socialport.TopReply{
		CommentID: idOf("comment"), ProposalID: w.nvda, AuthorID: w.christelle, Excerpt: "Earnings are the catalyst.",
		ReactionCount: 5, TopEmoji: "fire", TopEmojiCount: 3,
	}
	got, ok, err := app.BuildLedger(t.Context(), w.inputs, w.andres, ledgerDelivery())
	if err != nil || !ok {
		t.Fatalf("BuildLedger = %v, %v", ok, err)
	}
	want := week42Summary{
		Week: "2026-W42", LabelStart: "2026-10-12", LabelEnd: "2026-10-18",
		Voted: 3, Eligible: 3, Right: 2, Resolved: 3, Calls: 3,
		Cabals:   []string{"AC Capital", "Degens", "Tech Titans"},
		Sharpest: "Christelle 10000",
		Best:     "AMZN Degens yes 420 true https://logo.test/amzn.png",
		Miss:     "TSLA Tech Titans no -240 false https://logo.test/amzn.png",
		Reply:    "Buy NVDA Christelle fire 3 Earnings are the catalyst. 5",
		Markers:  []string{"call Called AMZN · Sat"},
	}
	if diff := summarize(got); !reflect.DeepEqual(diff, want) {
		t.Fatalf("ledger = %+v, want %+v", diff, want)
	}
}

func TestBuildLedger_BuyMarkersAndNoTopReply(t *testing.T) {
	t.Parallel()
	w := seedWeek42(t)
	buyAt := time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC)
	w.trades.buys = []trading.FilledBuy{{CabalID: w.degens, Symbol: "NVDA", ConfirmedAt: buyAt}}
	got, ok, err := app.BuildLedger(t.Context(), w.inputs, w.andres, ledgerDelivery())
	if err != nil || !ok || got.TopReply != nil {
		t.Fatalf("BuildLedger = %v, %v, top reply %+v", ok, err, got.TopReply)
	}
	if len(got.Markers) != 2 || got.Markers[0].Kind != "buy" || got.Markers[0].Label != "B" ||
		!got.Markers[0].At.Equal(buyAt) || got.Markers[1].Kind != "call" {
		t.Fatalf("markers = %+v, want the buy before the call marker", got.Markers)
	}
}

func TestBuildLedger_NoLedgerForAMemberWithNothing(t *testing.T) {
	t.Parallel()
	w := seedWeek42(t)
	w.inputs.Members = fakeMembers{wire: &tripwire{}}
	_, ok, err := app.BuildLedger(t.Context(), w.inputs, ids.UserIDFrom(idOf("stranger")), ledgerDelivery())
	if err != nil || ok {
		t.Fatalf("BuildLedger = %v, %v, want no Ledger", ok, err)
	}
}

func TestBuildLedger_AMemberWithOnlyAQuietWeekStillGetsOne(t *testing.T) {
	t.Parallel()
	w := seedWeek42(t)
	got, ok, err := app.BuildLedger(t.Context(), w.inputs, ids.UserIDFrom(idOf("stranger")), ledgerDelivery())
	if err != nil || !ok || len(got.Calls) != 0 || got.BestCall != nil || got.Miss != nil || got.Eligible != 0 {
		t.Fatalf("BuildLedger = %+v, %v, %v, want an empty week for a member with cabals", got, ok, err)
	}
}

func TestBuildLedger_PortFailuresSurface(t *testing.T) {
	t.Parallel()
	for name, mutate := range map[string]func(w *week42){
		"members": func(w *week42) { w.inputs.Members = fakeMembers{wire: &tripwire{failOn: 1}} },
		"cabal cards": func(w *week42) {
			w.inputs.Cards = fakeCards{wire: &tripwire{failOn: 1}}
		},
		"call cabal cards": func(w *week42) {
			w.inputs.Cards = fakeCards{wire: &tripwire{failOn: 2}}
		},
		"turnout": func(w *week42) {
			w.inputs.Store = failStore{LedgerStore: w.inputs.Store.(failStore).LedgerStore, wire: &tripwire{failOn: 1}}
		},
		"calls":     func(w *week42) { w.inputs.Store = failCalls{failStore: w.inputs.Store.(failStore)} },
		"market":    func(w *week42) { w.inputs.Market = fakeMarket{wire: &tripwire{failOn: 1}} },
		"mates":     func(w *week42) { w.inputs.Store = failMates{failStore: w.inputs.Store.(failStore)} },
		"users":     func(w *week42) { w.inputs.Users = fakeUsers{wire: &tripwire{failOn: 1}} },
		"proposals": func(w *week42) { w.inputs.Store = failProposals{failStore: w.inputs.Store.(failStore)} },
		"replies": func(w *week42) {
			w.replies.wire = &tripwire{failOn: 1}
		},
		"reply author": func(w *week42) {
			w.replies.top = &socialport.TopReply{AuthorID: w.christelle, ProposalID: w.nvda}
			w.inputs.Users = fakeUsers{wire: &tripwire{failOn: 2}}
		},
		"trades": func(w *week42) { w.trades.wire = &tripwire{failOn: 1} },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			w := seedWeek42(t)
			mutate(&w)
			if _, ok, err := app.BuildLedger(t.Context(), w.inputs, w.andres, ledgerDelivery()); err == nil || ok {
				t.Fatalf("BuildLedger = %v, %v, want the %s failure", ok, err, name)
			}
		})
	}
}

func storeTwice(t *testing.T, user ids.UserID, first, second domain.Ledger) *pgxpool.Pool {
	t.Helper()
	pool := testkit.DB(t)
	uow := db.New(pool, testkit.NewIDs(1), testkit.NewClock(ledgerDelivery()))
	var inserted []bool
	for _, l := range []domain.Ledger{first, second} {
		err := uow.Do(t.Context(), func(ctx context.Context, tx db.Tx) error {
			ok, err := app.StoreLedger(ctx, tx, user, l, ledgerDelivery())
			inserted = append(inserted, ok)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(inserted, []bool{true, false}) {
		t.Fatalf("inserted = %v, want true then false", inserted)
	}
	return pool
}

func TestStoreLedger_KeepsTheFirstPayloadAndReadsItBack(t *testing.T) {
	t.Parallel()
	user := ids.UserIDFrom(idOf("stored"))
	first := domain.Ledger{
		Week: "2026-W42", WindowStart: ledgerDelivery().Add(-7 * 24 * time.Hour), WindowEnd: ledgerDelivery(), Voted: 3,
	}
	second := first
	second.Voted = 1
	pool := storeTwice(t, user, first, second)
	reads := app.ReadLedger{Reads: sqlc.New(pool)}
	want := mustJSON(t, first)
	for name, read := range map[string]func() ([]byte, error){
		"week":   func() ([]byte, error) { return reads.Week(t.Context(), user, "2026-W42") },
		"latest": func() ([]byte, error) { return reads.Latest(t.Context(), user) },
	} {
		got, err := read()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		var decoded domain.Ledger
		if err := json.Unmarshal(got, &decoded); err != nil || mustJSON(t, decoded) != want {
			t.Fatalf("%s payload = %s, want the first build %s", name, got, want)
		}
	}
}

func TestReadLedger_NotFoundAndMalformed(t *testing.T) {
	t.Parallel()
	reads := app.ReadLedger{Reads: sqlc.New(testkit.DB(t))}
	user := ids.UserIDFrom(idOf("empty"))
	if _, err := reads.Week(t.Context(), user, "2026-W41"); errs.CodeOf(err) != errs.CodeLedgerNotFound {
		t.Fatalf("Week with none stored = %v, want ledger_not_found", err)
	}
	if _, err := reads.Latest(t.Context(), user); errs.CodeOf(err) != errs.CodeLedgerNotFound {
		t.Fatalf("Latest with none stored = %v, want ledger_not_found", err)
	}
	if _, err := reads.Week(t.Context(), user, "week-42"); errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("Week malformed = %v, want invalid_input", err)
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

type brokenReads struct{}

func (brokenReads) GetLedger(context.Context, sqlc.GetLedgerParams) ([]byte, error) {
	return nil, errs.New(errs.CodeInternal, "boom")
}

func (brokenReads) LatestLedger(context.Context, uuid.UUID) ([]byte, error) {
	return nil, errs.New(errs.CodeInternal, "boom")
}

func TestReadLedger_DatabaseErrorIsInternal(t *testing.T) {
	t.Parallel()
	reads := app.ReadLedger{Reads: brokenReads{}}
	user := ids.UserIDFrom(idOf("stranger"))
	if _, err := reads.Latest(t.Context(), user); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("Latest = %v, want internal", err)
	}
	if _, err := reads.Week(t.Context(), user, "2026-W42"); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("Week = %v, want internal", err)
	}
}

func TestStoreLedger_FailuresSurface(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	uow := db.New(pool, testkit.NewIDs(1), testkit.NewClock(ledgerDelivery()))
	user := ids.UserIDFrom(idOf("stranger"))
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	unmarshalable := domain.Ledger{Week: "2026-W42", WindowEnd: time.Date(10_000, 1, 1, 0, 0, 0, 0, time.UTC)}
	_ = uow.Do(t.Context(), func(ctx context.Context, tx db.Tx) error {
		if _, err := app.StoreLedger(
			ctx,
			tx,
			user,
			unmarshalable,
			ledgerDelivery(),
		); errs.CodeOf(
			err,
		) != errs.CodeInternal {
			t.Errorf("StoreLedger with an unencodable payload = %v, want internal", err)
		}
		if _, err := app.StoreLedger(
			cancelled,
			tx,
			user,
			domain.Ledger{Week: "2026-W42"},
			ledgerDelivery(),
		); errs.CodeOf(
			err,
		) != errs.CodeInternal {
			t.Errorf("StoreLedger on a cancelled context = %v, want internal", err)
		}
		if err := app.StoreSkip(cancelled, tx, user, "2026-W42"); errs.CodeOf(err) != errs.CodeInternal {
			t.Errorf("StoreSkip on a cancelled context = %v, want internal", err)
		}
		return errs.New(errs.CodeInternal, "rollback")
	})
}

func TestStoreSkip_IsIdempotent(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	uow := db.New(pool, testkit.NewIDs(1), testkit.NewClock(ledgerDelivery()))
	user := ids.UserIDFrom(idOf("stranger"))
	for range 2 {
		err := uow.Do(t.Context(), func(ctx context.Context, tx db.Tx) error {
			return app.StoreSkip(ctx, tx, user, "2026-W42")
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	var n int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM weekly_ledger_skips WHERE user_id = $1`, user.UUID()).
		Scan(&n); err != nil ||
		n != 1 {
		t.Fatalf("skips = %d, %v, want 1", n, err)
	}
}

func seedStoredLedger(t *testing.T, s server, user ids.UserID) {
	t.Helper()
	ledger := domain.Ledger{
		Week: "2026-W42", LabelStart: "2026-10-12", LabelEnd: "2026-10-18",
		WindowStart: ledgerDelivery().Add(-7 * 24 * time.Hour), WindowEnd: ledgerDelivery(), Voted: 3, Eligible: 3,
		Right: 2, Resolved: 3,
		Cabals: []domain.LedgerCabal{{ID: idOf("ac"), Name: "AC Capital"}}, Calls: []domain.LedgerCall{},
		Series: []domain.LedgerPoint{}, Markers: []domain.LedgerMarker{},
	}
	exec(t, s.pool, `INSERT INTO weekly_ledgers (user_id, week, window_start, window_end, payload, built_at)
VALUES ($1, $2, $3, $4, $5, $4)`, user.UUID(), ledger.Week, ledger.WindowStart, ledger.WindowEnd, mustJSON(t, ledger))
}

func TestLedgerRoutes_ServeTheStoredLedger(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	user := testkit.NewCabal(t, s.pool).Creator.ID
	seedStoredLedger(t, s, user)
	for _, path := range []string{"/v1/me/ledgers/latest", "/v1/me/ledgers/2026-W42"} {
		rec := s.get(t, path, user)
		var got api.Ledger
		decode(t, rec, &got)
		if rec.Code != http.StatusOK || got.Week != "2026-W42" || got.Voted != 3 || got.Right != 2 ||
			got.Cabals[0].Name != "AC Capital" || got.StakeReturnBps != nil || got.BestCall != nil {
			t.Fatalf("GET %s = %d %s", path, rec.Code, rec.Body)
		}
	}
	other := testkit.NewCabal(t, s.pool).Creator.ID
	if rec := s.get(t, "/v1/me/ledgers/latest", other); rec.Code != http.StatusNotFound {
		t.Fatalf("another member's latest = %d, want 404", rec.Code)
	}
}

func TestLedgerRoutes_Errors(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	user := testkit.NewCabal(t, s.pool).Creator.ID
	seedStoredLedger(t, s, user)
	for path, want := range map[string]struct {
		status int
		code   apibase.ErrorCode
	}{
		"/v1/me/ledgers/2026-W41": {http.StatusNotFound, apibase.LedgerNotFound},
		"/v1/me/ledgers/2026-W4":  {http.StatusBadRequest, apibase.InvalidInput},
	} {
		rec := s.get(t, path, user)
		if rec.Code != want.status || problemOf(t, rec) != want.code {
			t.Fatalf("GET %s = %d %s, want %d %s", path, rec.Code, rec.Body, want.status, want.code)
		}
	}
	for _, path := range []string{"/v1/me/ledgers/latest", "/v1/me/ledgers/2026-W42"} {
		if rec := s.get(t, path, ids.UserID{}); rec.Code != http.StatusUnauthorized {
			t.Fatalf("GET %s signed out = %d, want 401", path, rec.Code)
		}
	}
}

func TestLedgerRoutes_ACorruptStoredPayloadIsInternal(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	user := testkit.NewCabal(t, s.pool).Creator.ID
	exec(t, s.pool, `INSERT INTO weekly_ledgers (user_id, week, window_start, window_end, payload, built_at)
VALUES ($1, '2026-W42', $2, $2, '{"week": 1}', $2)`, user.UUID(), ledgerDelivery())
	for _, path := range []string{"/v1/me/ledgers/latest", "/v1/me/ledgers/2026-W42"} {
		if rec := s.get(t, path, user); rec.Code != http.StatusInternalServerError {
			t.Fatalf("GET %s = %d %s, want 500", path, rec.Code, rec.Body)
		}
	}
}

func TestModule_LedgerInputsAreWired(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	deps := module.Deps{Pool: pool, IDs: testkit.NewIDs(1), Clock: testkit.NewClock(ledgerDelivery())}
	m := ranking.New(deps)
	module.NewSet(
		m, cabal.New(deps), treasury.New(deps), identity.New(deps), social.New(deps), governance.New(deps),
		trading.New(deps),
	)
	in := m.LedgerInputs()
	if in.Trades == nil || in.Replies == nil || in.Members == nil || in.Cards == nil || in.Users == nil {
		t.Fatalf("LedgerInputs = %+v, want every port bound", in)
	}
	if _, ok, err := app.BuildLedger(
		t.Context(),
		in,
		ids.UserIDFrom(idOf("stranger")),
		ledgerDelivery(),
	); err != nil ||
		ok {
		t.Fatalf("BuildLedger for a stranger = %v, %v, want no Ledger", ok, err)
	}
}

func TestLedgerHandlers_RefuseACallerWithoutAnActor(t *testing.T) {
	t.Parallel()
	h := adapters.HTTP{}
	if _, err := h.GetMyLatestLedger(
		t.Context(),
		api.GetMyLatestLedgerRequestObject{},
	); errs.CodeOf(
		err,
	) != errs.CodeUnauthorized {
		t.Fatalf("GetMyLatestLedger = %v, want unauthorized", err)
	}
	if _, err := h.GetMyLedger(
		t.Context(),
		api.GetMyLedgerRequestObject{Week: "2026-W42"},
	); errs.CodeOf(
		err,
	) != errs.CodeUnauthorized {
		t.Fatalf("GetMyLedger = %v, want unauthorized", err)
	}
}
