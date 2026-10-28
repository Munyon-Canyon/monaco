package treasury_test

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/treasuryapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

type member bool

func (m member) IsMember(context.Context, ids.CabalID, ids.UserID) (bool, error) { return bool(m), nil }

type failingNames struct{}

func (failingNames) AssetNames(context.Context) (map[domain.Asset]app.AssetName, error) {
	return nil, errs.New(errs.CodeDBUnavailable, "test.AssetNames")
}

func TestActivityReads_aPageCostsAtMostFourQueries(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	c := testkit.NewCabal(t, pool, testkit.WithMembers(2))
	seedAAPLx(t, pool)
	mint := "XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp"
	g, base := testkit.NewIDs(41), clock.Real{}.Now().UTC().Truncate(time.Second)
	for i := range 40 {
		seedActivity(t, pool, c.ID, seededActivity{
			id: g.NewV7(), at: base.Add(-time.Duration(i) * time.Second), status: "confirmed", asset: &mint,
			actor: &c.Members[i%2].ID,
		})
	}
	d := module.Deps{Pool: pool}
	reads := app.NewActivityReads(pool, cabal.New(d).Queries(), identity.New(d).Queries(),
		treasury.CatalogNames{Catalog: market.New(module.Deps{Pool: pool}).Catalog()})
	testkit.AssertQueries(t, "activity page", func() {
		page, err := reads.List(t.Context(), app.ListActivity{CabalID: c.ID, Caller: c.Creator.ID})
		if err != nil || len(page.Items) != 30 || page.Items[0].Actor == nil || page.Items[0].Asset == nil {
			t.Fatalf("page = %d items (first %+v), err %v; want 30 with actor and asset", len(page.Items),
				page.Items, err)
		}
	})
}

func TestActivityReads_refusesAPageOutOfRangeAndPassesPortErrorsOn(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	cabalID, user := ids.CabalIDFrom(testkit.NewIDs(51).NewV7()), ids.UserIDFrom(testkit.NewIDs(52).NewV7())
	mint, at := "XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp", clock.Real{}.Now().UTC()
	seedActivity(t, pool, cabalID, seededActivity{
		id: testkit.NewIDs(53).NewV7(), at: at, status: "pending",
		asset: &mint, actor: &user,
	})
	names := treasury.CatalogNames{Catalog: market.New(module.Deps{Pool: pool}).Catalog()}
	tests := map[string]struct {
		reads *app.ActivityReads
		req   app.ListActivity
		code  errs.Code
	}{
		"limit over 100": {
			app.NewActivityReads(pool, member(true), app.UnwiredReads{}, names),
			app.ListActivity{Limit: 101},
			errs.CodeInvalidInput,
		},
		"members unwired": {
			app.NewActivityReads(pool, app.UnwiredReads{}, app.UnwiredReads{}, names),
			app.ListActivity{},
			errs.CodeUpstreamUnavailable,
		},
		"users unwired": {
			app.NewActivityReads(pool, member(true), app.UnwiredReads{}, names),
			app.ListActivity{},
			errs.CodeUpstreamUnavailable,
		},
		"names down": {
			app.NewActivityReads(pool, member(true), app.UnwiredReads{}, failingNames{}),
			app.ListActivity{},
			errs.CodeDBUnavailable,
		},
		"names down for a symbol": {
			app.NewActivityReads(pool, member(true), app.UnwiredReads{}, failingNames{}),
			app.ListActivity{Symbol: "AAPLx"},
			errs.CodeDBUnavailable,
		},
	}
	for name, tc := range tests {
		tc.req.CabalID, tc.req.Caller = cabalID, user
		if _, err := tc.reads.List(t.Context(), tc.req); errs.CodeOf(err) != tc.code {
			t.Errorf("%s: err = %v, want %s", name, err, tc.code)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := app.NewActivityReads(pool, member(true), app.UnwiredReads{}, names).List(ctx,
		app.ListActivity{CabalID: cabalID, Caller: user})
	if err == nil {
		t.Fatal("a canceled read succeeded")
	}
	if _, err := names.AssetNames(ctx); err == nil {
		t.Fatal("a canceled catalog read succeeded")
	}
}

func TestActivityHTTP_refusesACallerThatIsNotASignedInUser(t *testing.T) {
	t.Parallel()
	h := adapters.HTTP{}
	tests := map[string]struct {
		actor *auth.Actor
		code  errs.Code
	}{
		"no actor": {nil, errs.CodeUnauthorized},
		"system":   {&auth.Actor{Kind: auth.ActorSystem, ID: "ops"}, errs.CodeForbidden},
		"bad user": {&auth.Actor{Kind: auth.ActorUser, ID: "nope"}, errs.CodeUnauthorized},
	}
	for name, tc := range tests {
		ctx := t.Context()
		if tc.actor != nil {
			ctx = auth.WithActor(ctx, *tc.actor)
		}
		for _, call := range []func(context.Context) error{
			func(ctx context.Context) error {
				_, err := h.GetCabalActivity(ctx, api.GetCabalActivityRequestObject{})
				return err
			},
			func(ctx context.Context) error {
				_, err := h.GetMyTxns(ctx, api.GetMyTxnsRequestObject{})
				return err
			},
		} {
			if err := call(ctx); errs.CodeOf(err) != tc.code {
				t.Errorf("%s: err = %v, want %s", name, err, tc.code)
			}
		}
	}
}

type trade struct {
	id          uuid.UUID
	kind        string
	asset       *string
	usdc, units *int64
	at          time.Time
}

type pricedItem struct {
	ID          uuid.UUID `json:"id"`
	Kind        string    `json:"kind"`
	Status      string    `json:"status"`
	PriceMicros *int64    `json:"price_micros"`
}

func seedTrades(t *testing.T, pool *pgxpool.Pool, cabal ids.CabalID, rows ...trade) {
	t.Helper()
	for _, r := range rows {
		if _, err := pool.Exec(t.Context(), `INSERT INTO cabal_activity (id, cabal_id, kind, status, asset,
			usdc_micros, units, occurred_at, updated_at) VALUES ($1, $2, $3, 'confirmed', $4, $5, $6, $7, $7)`,
			r.id, cabal.UUID(), r.kind, r.asset, r.usdc, r.units, r.at); err != nil {
			t.Fatal(err)
		}
	}
}

func seedSixDecimalSplit(t *testing.T, pool *pgxpool.Pool, at time.Time) {
	t.Helper()
	a := marketfake.TSLAx()
	if _, err := pool.Exec(t.Context(), `INSERT INTO assets (id, symbol, mint, decimals, issuer, kind, display_name,
		issuer_tradable, company_key, ui_multiplier_next_num, ui_multiplier_next_den, ui_multiplier_next_at,
		first_seen_at, updated_at) VALUES ($1, $2, $3, 6, $4, $5, $6, true, $7, 3, 2, $8, $8, $8)`,
		a.ID.UUID(), a.Symbol, a.Mint.String(), string(a.Issuer), string(a.Kind), a.DisplayName, a.CompanyKey,
		at); err != nil {
		t.Fatal(err)
	}
}

func expectPriced(t *testing.T, want ...pricedItem) scenario.Step {
	t.Helper()
	return scenario.ExpectField("items", func(_ *scenario.Scenario, raw json.RawMessage) {
		got := []pricedItem{}
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("items = %s, want %+v", raw, want)
		}
	})
}

func priced(id uuid.UUID, kind, status string, price int64) pricedItem {
	item := pricedItem{ID: id, Kind: kind, Status: status}
	if price > 0 {
		item.PriceMicros = &price
	}
	return item
}

func TestActivityRoute_filtersOneStockAndPricesEachTradeAtItsMultiplier(t *testing.T) {
	t.Parallel()
	s := scenario.New(t, withActivity())
	c := testkit.NewCabal(t, s.DB())
	g, base := testkit.NewIDs(91), clock.Real{}.Now().UTC().Truncate(time.Second)
	split := base.Add(-150 * time.Second)
	seedAAPLx(t, s.DB())
	seedSixDecimalSplit(t, s.DB(), split)
	aapl, tsla := marketfake.AAPLx().Mint.String(), marketfake.TSLAx().Mint.String()
	n := func(v int64) *int64 { return &v }
	rows := []trade{
		{g.NewV7(), "buy", &aapl, n(181_200_000), n(100_000_000), base.Add(-6 * time.Minute)},
		{g.NewV7(), "fund", nil, n(100_000_000), nil, base.Add(-5 * time.Minute)},
		{g.NewV7(), "buy", &tsla, n(10_000_000), n(4_000_000), base.Add(-4 * time.Minute)},
		{g.NewV7(), "sell", &aapl, n(428_200_000), n(200_000_000), base.Add(-3 * time.Minute)},
		{g.NewV7(), "cash_out", nil, n(50_000_000), nil, base.Add(-2 * time.Minute)},
		{g.NewV7(), "buy", &tsla, n(10_000_000), n(4_000_000), base.Add(-time.Minute)},
		{g.NewV7(), "buy", &aapl, n(196_750_000), n(100_000_000), base},
	}
	seedTrades(t, s.DB(), c.ID, rows...)
	seedRoutedSwap(t, s.DB(), c.ID, rows[6].id, base)
	path := "/v1/cabals/" + c.ID.String() + "/activity"
	s.Given(scenario.AsSeededUser("creator", c.Creator.ID)).
		When(
			scenario.Get(path+"?symbol=AAPLx&limit=2"),
			scenario.ExpectStatus(http.StatusOK),
			expectPriced(t,
				priced(rows[6].id, "buy", "pending", 0), priced(rows[3].id, "sell", "confirmed", 214_100_000)),
			scenario.Remember("next_cursor", "cursor"),
			scenario.Get(path+"?symbol=AAPLx&limit=2&cursor={cursor}"),
			scenario.ExpectStatus(http.StatusOK),
			expectPriced(t, priced(rows[0].id, "buy", "confirmed", 181_200_000)),
			scenario.ExpectJSON("next_cursor", nil),
			scenario.Get(path+"?symbol=TSLAx"),
			expectPriced(t,
				priced(rows[5].id, "buy", "confirmed", 1_666_666), priced(rows[2].id, "buy", "confirmed", 2_500_000)),
			scenario.Get(path),
			expectPriced(t, priced(rows[6].id, "buy", "pending", 0), priced(rows[5].id, "buy", "confirmed", 1_666_666),
				priced(rows[4].id, "cash_out", "confirmed", 0), priced(rows[3].id, "sell", "confirmed", 214_100_000),
				priced(rows[2].id, "buy", "confirmed", 2_500_000), priced(rows[1].id, "fund", "confirmed", 0),
				priced(rows[0].id, "buy", "confirmed", 181_200_000)),
			scenario.Get(path+"?symbol=NOPEx"),
			scenario.ExpectStatus(http.StatusOK),
		).
		Then(scenario.ExpectJSON("items", []any{}), scenario.ExpectJSON("next_cursor", nil))
}

func TestActivityHTTP_refusesAPriceTheWireCannotHold(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	c := testkit.NewCabal(t, pool)
	seedAAPLx(t, pool)
	aapl, huge, one := marketfake.AAPLx().Mint.String(), int64(9_000_000_000_000_000_000), int64(1)
	seedTrades(t, pool, c.ID, trade{testkit.NewIDs(95).NewV7(), "buy", &aapl, &huge, &one, clock.Real{}.Now().UTC()})
	reads := app.NewActivityReads(pool, member(true), app.UnwiredReads{},
		treasury.CatalogNames{Catalog: market.New(module.Deps{Pool: pool}).Catalog()})
	ctx := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: c.Creator.ID.String()})
	symbol := "AAPLx"
	_, err := adapters.HTTP{Reads: reads}.GetCabalActivity(ctx, api.GetCabalActivityRequestObject{
		Id: c.ID.UUID(), Params: api.GetCabalActivityParams{Symbol: &symbol},
	})
	if errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("a price past uint64 = %v, want invalid_input", err)
	}
}
