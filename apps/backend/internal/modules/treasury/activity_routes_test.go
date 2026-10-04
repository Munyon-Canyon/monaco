package treasury_test

import (
	"context"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	identityport "github.com/monaco/monaco/apps/backend/internal/modules/identity/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	apibase "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

type readsOnly struct{ name string }

func (r readsOnly) Name() string { return r.name }

func (readsOnly) Mount(apibase.Mount) {}

func (readsOnly) Consumers() []bus.Consumer { return nil }

func (readsOnly) Pollers() []poller.Poller { return nil }

type cabalReads struct {
	readsOnly
	d module.Deps
}

func (c cabalReads) Queries() cabalport.Queries { return cabal.New(c.d).Queries() }

type identityReads struct {
	readsOnly
	d module.Deps
}

func (i identityReads) Queries() identityport.Queries { return identity.New(i.d).Queries() }

func withActivity() scenario.Option {
	return scenario.WithModules(
		func(d module.Deps) module.Module { return treasury.New(d) },
		func(d module.Deps) module.Module { return cabalReads{readsOnly{"cabal"}, d} },
		func(d module.Deps) module.Module { return identityReads{readsOnly{"identity"}, d} },
	)
}

func seedAAPLx(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	a, now := marketfake.AAPLx(), clock.Real{}.Now().UTC()
	if _, err := pool.Exec(t.Context(), `INSERT INTO assets (
		id, symbol, mint, decimals, issuer, kind, display_name, issuer_tradable, company_key, first_seen_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $10)`,
		a.ID.UUID(), a.Symbol, a.Mint.String(), int16(a.Decimals), string(a.Issuer), string(a.Kind), a.DisplayName,
		a.IssuerTradable, a.CompanyKey, now); err != nil {
		t.Fatal(err)
	}
}

type seededActivity struct {
	id     uuid.UUID
	at     time.Time
	status string
	asset  *string
	actor  *ids.UserID
	sig    *string
}

type seededUserTxn struct {
	id        uuid.UUID
	at        time.Time
	user      ids.UserID
	kind      string
	status    string
	amount    int64
	wallets   []int64
	cabal     *ids.CabalID
	signature *string
}

func seedUserTxns(t *testing.T, pool *pgxpool.Pool, rows ...seededUserTxn) {
	t.Helper()
	for _, row := range rows {
		var cabal any
		if row.cabal != nil {
			cabal = row.cabal.UUID()
		}
		if _, err := pool.Exec(t.Context(), `INSERT INTO user_txns
			(id, user_id, cabal_id, kind, status, tx_signature, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			row.id, row.user.UUID(), cabal, row.kind, row.status, row.signature, row.at); err != nil {
			t.Fatal(err)
		}
		wallets := row.wallets
		if len(wallets) == 0 {
			wallets = []int64{row.amount}
		}
		for seq, amount := range wallets {
			if _, err := pool.Exec(t.Context(), `INSERT INTO user_txn_entries (txn_id, seq, account, asset, amount)
				VALUES ($1, $2, 'wallet', $3, $4)`, row.id, seq, testkit.USDCMint, amount); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func wireUserTxn(r seededUserTxn, cabal any) map[string]any {
	return map[string]any{
		"id": r.id, "kind": r.kind, "status": r.status, "usdc_micros": strconv.FormatInt(r.amount, 10),
		"cabal": cabal, "tx_signature": r.signature, "created_at": r.at.Format(time.RFC3339),
	}
}

func seedActivity(t *testing.T, pool *pgxpool.Pool, cabal ids.CabalID, rows ...seededActivity) {
	t.Helper()
	for _, r := range rows {
		var actor *uuid.UUID
		if r.actor != nil {
			u := r.actor.UUID()
			actor = &u
		}
		if _, err := pool.Exec(t.Context(), `INSERT INTO cabal_activity (id, cabal_id, kind, status, actor_user_id,
			asset, usdc_micros, units, tx_signature, occurred_at, updated_at)
			VALUES ($1, $2, 'buy', $3, $4, $5, 25000000,
				CASE WHEN $7::text IS NULL THEN NULL ELSE 105000000 END, $7, $6, $6)`,
			r.id, cabal.UUID(), r.status, actor, r.asset, r.at, r.sig); err != nil {
			t.Fatal(err)
		}
	}
}

func units(r seededActivity) any {
	if r.sig == nil {
		return nil
	}
	return 105000000
}

func wireItem(r seededActivity, asset, actor any) map[string]any {
	return map[string]any{
		"id": r.id, "kind": "buy", "status": r.status, "asset": asset, "usdc_micros": 25000000, "units": units(r),
		"actor": actor, "tx_signature": r.sig, "occurred_at": r.at.Format(time.RFC3339),
	}
}

func TestActivityRoute_aMemberPagesStablyAndAStrangerIsRefused(t *testing.T) {
	t.Parallel()
	s := scenario.New(t, withActivity())
	c := testkit.NewCabal(t, s.DB(), testkit.WithMembers(2))
	seedAAPLx(t, s.DB())
	mint := marketfake.AAPLx().Mint.String()
	unknown := "So11111111111111111111111111111111111111112"
	actor := testkit.SeedUser(t, s.DB(), testkit.UserOpts{Handle: "ana"})
	if _, err := s.DB().Exec(t.Context(), `UPDATE users SET display_name = 'Ana' WHERE id = $1`,
		actor.ID.UUID()); err != nil {
		t.Fatal(err)
	}
	g, base := testkit.NewIDs(31), clock.Real{}.Now().UTC().Truncate(time.Second)
	newest := seededActivity{id: g.NewV7(), at: base, status: "pending", asset: &mint, actor: &actor.ID}
	middle := seededActivity{id: g.NewV7(), at: base.Add(-time.Minute), status: "failed", asset: &unknown}
	sig := string(swapSig)
	oldest := seededActivity{id: g.NewV7(), at: base.Add(-2 * time.Minute), status: "confirmed", sig: &sig}
	seedActivity(t, s.DB(), c.ID, oldest, newest, middle)
	other := testkit.NewCabal(t, s.DB())
	seedActivity(t, s.DB(), other.ID, seededActivity{id: g.NewV7(), at: base, status: "confirmed"})
	stranger := testkit.SeedUser(t, s.DB(), testkit.UserOpts{})
	path := "/v1/cabals/" + c.ID.String() + "/activity"
	s.Given(scenario.AsSeededUser("member", c.Members[0].ID)).
		When(
			scenario.Get(path+"?limit=2"),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("items", []any{
				wireItem(newest, map[string]string{"symbol": "AAPLx", "name": marketfake.AAPLx().DisplayName},
					map[string]any{"user_id": actor.ID, "handle": "ana", "display_name": "Ana"}),
				wireItem(middle, nil, nil),
			}),
			scenario.Remember("next_cursor", "cursor"),
			scenario.Get(path+"?limit=2&cursor={cursor}"),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("items", []any{wireItem(oldest, nil, nil)}),
			scenario.ExpectJSON("next_cursor", nil),
			scenario.AsSeededUser("stranger", stranger.ID),
			scenario.Get(path),
		).
		Then(scenario.ExpectStatus(http.StatusForbidden), scenario.ExpectProblem(errs.CodeNotCabalMember))
}

func TestActivityRoute_refusesABadCursorAndAnEmptyCabalReadsNoItems(t *testing.T) {
	t.Parallel()
	s := scenario.New(t, withActivity())
	c := testkit.NewCabal(t, s.DB())
	path := "/v1/cabals/" + c.ID.String() + "/activity"
	s.Given(scenario.AsSeededUser("creator", c.Creator.ID)).
		When(
			scenario.Get(path),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("items", []any{}),
			scenario.ExpectJSON("next_cursor", nil),
		)
	for _, cursor := range []string{"%25%25", "bm9jb2xvbg", "eDow", "MTow"} {
		s.When(scenario.Get(path+"?cursor="+cursor)).
			Then(scenario.ExpectStatus(http.StatusBadRequest), scenario.ExpectProblem(errs.CodeInvalidInput))
	}
}

func TestUserTxnRoute_listsOnlyTheCallerHistoryWithStablePages(t *testing.T) {
	t.Parallel()
	s := scenario.New(t, withActivity())
	c := testkit.NewCabal(t, s.DB(), testkit.WithMembers(2))
	g, base := testkit.NewIDs(61), clock.Real{}.Now().UTC().Truncate(time.Second)
	sig := string(swapSig)
	oldest := seededUserTxn{
		id: g.NewV7(), at: base.Add(-time.Minute), user: c.Creator.ID, kind: "deposit", status: "settled",
		amount: 25000000,
	}
	middle := seededUserTxn{
		id: g.NewV7(), at: base.Add(-time.Minute), user: c.Creator.ID, kind: "fund", status: "pending",
		amount: -5000000, wallets: []int64{-7000000, 2000000},
		cabal: &c.ID, signature: &sig,
	}
	newest := seededUserTxn{
		id: g.NewV7(), at: base, user: c.Creator.ID, kind: "withdrawal", status: "failed", amount: -1000000,
	}
	other := seededUserTxn{
		id: g.NewV7(), at: base.Add(time.Minute), user: c.Members[1].ID, kind: "deposit", status: "settled",
		amount: 99000000,
	}
	seedUserTxns(t, s.DB(), oldest, middle, newest, other)
	s.Given(scenario.AsSeededUser("creator", c.Creator.ID)).
		When(
			scenario.Get("/v1/me/txns?limit=2"),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("items", []any{
				wireUserTxn(newest, nil),
				wireUserTxn(middle, map[string]any{"id": c.ID, "name": "cabal " + c.ID.String()[24:]}),
			}),
			scenario.Remember("next_cursor", "cursor"),
			scenario.Get("/v1/me/txns?limit=2&cursor={cursor}"),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("items", []any{wireUserTxn(oldest, nil)}),
			scenario.ExpectJSON("next_cursor", nil),
		)
}

func TestUserTxnRoute_returnsNullForAFundWhoseCabalIsGone(t *testing.T) {
	t.Parallel()
	s := scenario.New(t, withActivity())
	user := testkit.SeedUser(t, s.DB(), testkit.UserOpts{})
	gone := ids.CabalIDFrom(testkit.NewIDs(63).NewV7())
	row := seededUserTxn{
		id: testkit.NewIDs(64).NewV7(), at: clock.Real{}.Now().UTC().Truncate(time.Second), user: user.ID,
		kind: "fund", status: "settled", amount: -5000000, cabal: &gone,
	}
	seedUserTxns(t, s.DB(), row)
	s.Given(scenario.AsSeededUser("member", user.ID)).
		When(scenario.Get("/v1/me/txns")).
		Then(
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("items", []any{wireUserTxn(row, nil)}),
			scenario.ExpectJSON("next_cursor", nil),
		)
}

func TestUserTxnRoute_refusesInvalidPagesAndUnwiredCabalReads(t *testing.T) {
	t.Parallel()
	s := scenario.New(t, withActivity())
	c := testkit.NewCabal(t, s.DB())
	row := seededUserTxn{
		id: testkit.NewIDs(62).NewV7(), at: clock.Real{}.Now().UTC(), user: c.Creator.ID,
		kind: "fund", status: "settled", amount: -1, cabal: &c.ID,
	}
	seedUserTxns(t, s.DB(), row)
	s.Given(scenario.AsSeededUser("creator", c.Creator.ID)).
		When(scenario.Get("/v1/me/txns?limit=101")).
		Then(scenario.ExpectStatus(http.StatusBadRequest), scenario.ExpectProblem(errs.CodeInvalidInput))
	for _, cursor := range []string{"%25%25", "bm9jb2xvbg", "eDow", "MTow"} {
		s.When(scenario.Get("/v1/me/txns?cursor="+cursor)).
			Then(scenario.ExpectStatus(http.StatusBadRequest), scenario.ExpectProblem(errs.CodeInvalidInput))
	}
	reads := app.NewUserTxnReads(s.DB(), app.UnwiredReads{}, domain.MintAsset(testkit.USDCMint))
	_, err := reads.List(t.Context(), app.ListUserTxns{UserID: c.Creator.ID, Limit: 101})
	if errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("invalid limit error = %v, want %s", err, errs.CodeInvalidInput)
	}
	_, err = reads.List(t.Context(), app.ListUserTxns{UserID: c.Creator.ID})
	if errs.CodeOf(err) != errs.CodeUpstreamUnavailable {
		t.Fatalf("unwired cabals error = %v, want %s", err, errs.CodeUpstreamUnavailable)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := reads.List(ctx, app.ListUserTxns{UserID: c.Creator.ID}); err == nil {
		t.Fatal("a canceled read succeeded")
	}
}
