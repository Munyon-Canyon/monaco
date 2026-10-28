package market_test

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/marketapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func (s marketAPI) profile(t *testing.T, key string) {
	t.Helper()
	_, err := s.pool.Exec(t.Context(), `INSERT INTO pre_ipo_profiles (company_key, estimate_micros, estimate_on,
		valuation_micros, min_order_micros, sell_window, next_tender_on, updated_at)
		VALUES ($1, 185000000, '2026-02-20', 350000000000000000, 10000000, 'tender_quarterly', '2027-01-15', $2)`,
		key, s.when)
	if err != nil {
		t.Fatal(err)
	}
}

func (s marketAPI) round(t *testing.T, key, onDay string, price int64) {
	t.Helper()
	_, err := s.pool.Exec(t.Context(),
		`INSERT INTO pre_ipo_rounds (company_key, round_on, price_micros) VALUES ($1, $2::date, $3)`,
		key, onDay, price)
	if err != nil {
		t.Fatal(err)
	}
}

func (s marketAPI) preIPO(t *testing.T, seed uint64, symbol, mint, name string) {
	t.Helper()
	s.insertClone(t, seed, symbol, mint, name, domain.KindPreIPO, 0)
}

const (
	mintOne = "TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA"
	mintTwo = "So11111111111111111111111111111111111111112"
)

func TestPreIPO_listCarriesTheEstimateOfACompanyWithAProfileAndNullForOneWithout(t *testing.T) {
	t.Parallel()
	s := newMarketAPI(t, marketWhen())
	s.seedFixtures(t)
	s.preIPO(t, 5, "ACME", mintOne, "Acme Rockets")
	s.preIPO(t, 6, "NOPE", mintTwo, "No Profile Co")
	s.profile(t, "acme rockets")

	page := pageOf(t, s.get(t, "/v1/assets?filter=pre_ipo"))
	got := map[string]api.AssetSummary{}
	for _, a := range page.Assets {
		got[a.Symbol] = a
	}
	acme := got["ACME"]
	if acme.Estimate == nil || acme.Estimate.PriceMicros != 185_000_000 ||
		acme.Estimate.AsOf.Format(time.DateOnly) != "2026-02-20" {
		t.Fatalf("ACME estimate = %+v, want 185e6 as of 2026-02-20", acme.Estimate)
	}
	if got["NOPE"].Estimate != nil {
		t.Fatalf("NOPE estimate = %+v, want none without a profile", got["NOPE"].Estimate)
	}
	if all := pageOf(t, s.get(t, "/v1/assets")); all.Assets[0].Estimate != nil {
		t.Fatalf("equity %s estimate = %+v, want none", all.Assets[0].Symbol, all.Assets[0].Estimate)
	}
}

func TestPreIPO_detailServesTheRoundsOldestFirstAndTheRoundStats(t *testing.T) {
	t.Parallel()
	s := newMarketAPI(t, marketWhen())
	s.seedFixtures(t)
	s.preIPO(t, 5, "ACME", mintOne, "Acme Rockets")
	s.profile(t, "acme rockets")
	s.round(t, "acme rockets", "2025-10-01", 150_000_000)
	s.round(t, "acme rockets", "2026-06-01", 165_000_000)
	s.round(t, "acme rockets", "2025-02-01", 120_000_000)

	pre := detailOf(t, s.get(t, "/v1/assets/ACME")).PreIpo
	if pre == nil {
		t.Fatal("ACME pre_ipo = nil, want the profile")
	}
	assertRounds(t, pre)
	assertRoundStats(t, pre)
	if equity := detailOf(t, s.get(t, "/v1/assets/AAPLx")); equity.PreIpo != nil {
		t.Fatalf("AAPLx pre_ipo = %+v, want none for an equity", equity.PreIpo)
	}
}

func TestPreIPO_aProfileWithoutRoundsServesAnEmptyListAndNoLastRound(t *testing.T) {
	t.Parallel()
	s := newMarketAPI(t, marketWhen())
	s.preIPO(t, 5, "ACME", mintOne, "Acme Rockets")
	s.profile(t, "acme rockets")

	rec := s.get(t, "/v1/assets/ACME")
	pre := detailOf(t, rec).PreIpo
	if pre == nil || pre.Rounds == nil || len(pre.Rounds) != 0 || pre.LastRound != nil {
		t.Fatalf("pre_ipo = %+v, want an empty rounds list and no last round", pre)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"rounds":[]`) || !strings.Contains(body, `"last_round":null`) {
		t.Fatalf("body = %s, want rounds [] and last_round null on the wire", rec.Body)
	}
}

func TestPreIPO_aResearchOnlyCompanyStillServesItsEstimateAndProfile(t *testing.T) {
	t.Parallel()
	s := newMarketAPI(t, marketWhen())
	s.preIPO(t, 5, "ACME", mintOne, "Acme Rockets")
	s.profile(t, "acme rockets")
	_, err := s.pool.Exec(t.Context(), `UPDATE assets SET tradable_override = false WHERE symbol = 'ACME'`)
	if err != nil {
		t.Fatal(err)
	}
	detail := detailOf(t, s.get(t, "/v1/assets/ACME"))
	if detail.Tradable || detail.PreIpo == nil || detail.PreIpo.Estimate.PriceMicros != 185_000_000 {
		t.Fatalf("detail tradable=%v pre_ipo=%+v, want research-only with its profile", detail.Tradable, detail.PreIpo)
	}
}

func assertRounds(t *testing.T, pre *api.PreIpoProfile) {
	t.Helper()
	if len(pre.Rounds) != 3 || pre.Rounds[0].PriceMicros != 120_000_000 || pre.Rounds[1].PriceMicros != 150_000_000 ||
		pre.Rounds[2].PriceMicros != 165_000_000 {
		t.Fatalf("rounds = %+v, want oldest first", pre.Rounds)
	}
	last := pre.LastRound
	if last == nil || *last != pre.Rounds[2] || last.On.Format(time.DateOnly) != "2026-06-01" {
		t.Fatalf("last_round = %+v, want the newest round", last)
	}
}

func assertRoundStats(t *testing.T, pre *api.PreIpoProfile) {
	t.Helper()
	if pre.Estimate.PriceMicros != 185_000_000 || pre.ValuationMicros != "350000000000000000" ||
		pre.MinOrderMicros != 10_000_000 || pre.SellWindow != api.TenderQuarterly || pre.ImageUrl != nil {
		t.Fatalf("pre_ipo = %+v, want the stats of the profile", pre)
	}
	if pre.NextTenderOn == nil || pre.NextTenderOn.Format(time.DateOnly) != "2027-01-15" {
		t.Fatalf("next_tender_on = %v, want 2027-01-15", pre.NextTenderOn)
	}
}

type queryCounter struct {
	profiles atomic.Int64
	rounds   atomic.Int64
}

func (c *queryCounter) TraceQueryStart(
	ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData,
) context.Context {
	switch {
	case strings.Contains(data.SQL, "FROM pre_ipo_profiles"):
		c.profiles.Add(1)
	case strings.Contains(data.SQL, "FROM pre_ipo_rounds"):
		c.rounds.Add(1)
	}
	return ctx
}

func (*queryCounter) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func (s marketAPI) counted(t *testing.T) (*pgxpool.Pool, *queryCounter) {
	t.Helper()
	counter := &queryCounter{}
	cfg := s.pool.Config().Copy()
	cfg.ConnConfig.Tracer = counter
	pool, err := pgxpool.NewWithConfig(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool, counter
}

func TestPreIPO_aPageOfTheListMakesOneProfileQueryAndNoRoundQuery(t *testing.T) {
	t.Parallel()
	s := newMarketAPI(t, marketWhen())
	for i, asset := range generatedCatalog(t, 12) {
		asset.Kind, asset.ChainChecked = domain.KindPreIPO, true
		asset.CompanyKey = fmt.Sprintf("company %02d", i)
		insert(t, s.pool, stamped(asset, s.when), nil)
		if i%2 == 0 {
			s.profile(t, asset.CompanyKey)
		}
	}
	pool, counter := s.counted(t)
	page, err := app.NewListAssets(pool, s.clock).Handle(t.Context(), app.ListRequest{Filter: app.FilterPreIPO})
	if err != nil || len(page.Items) != 12 {
		t.Fatalf("page = %d items, %v, want the 12 Pre-IPO assets", len(page.Items), err)
	}
	if counter.profiles.Load() != 1 || counter.rounds.Load() != 0 {
		t.Fatalf("profile queries = %d, round queries = %d, want 1 and 0",
			counter.profiles.Load(), counter.rounds.Load())
	}
	withProfile := 0
	for _, item := range page.Items {
		if item.Profile != nil {
			withProfile++
		}
	}
	if withProfile != 6 {
		t.Fatalf("%d items carry a profile, want 6", withProfile)
	}
}

type fixedDetail struct{ view app.AssetView }

func (f fixedDetail) Handle(context.Context, string) (app.AssetView, error) { return f.view, nil }

func TestPreIPO_aMicrosValuePastInt64DoesNotReachTheClient(t *testing.T) {
	t.Parallel()
	when := marketWhen()
	huge := money.MicrosFromUint64(uint64(math.MaxInt64) + 1)
	one := money.MicrosFromUint64(1)
	asset := domain.Asset{Symbol: "ACME", DisplayName: "Acme", Issuer: domain.IssuerTessera, Kind: domain.KindPreIPO}
	session := domain.SessionInfo{State: domain.StateOpen, Continuous: true}
	user := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: testkit.NewIDs(1).NewV7().String()})
	profile := func(estimate, minOrder, round money.Micros) *domain.Profile {
		return &domain.Profile{
			Estimate: domain.Estimate{Price: estimate}, MinOrder: minOrder, SellWindow: domain.SellWindowAnyTime,
			Rounds: []domain.Round{{Price: round}},
		}
	}
	h := adapters.HTTP{Clock: testkit.NewClock(when)}
	cases := map[string]*domain.Profile{
		"estimate": profile(huge, one, one), "minimum order": profile(one, huge, one),
		"round": profile(one, one, huge),
	}
	for name, p := range cases {
		h.List = hugeList{page: app.Page{Items: []app.Summary{{Asset: asset, Session: session, Profile: p}}}}
		h.Detail = fixedDetail{view: app.AssetView{Summary: app.Summary{Asset: asset, Session: session, Profile: p}}}
		_, listErr := h.GetAssets(user, api.GetAssetsRequestObject{})
		_, detailErr := h.GetAsset(user, api.GetAssetRequestObject{Symbol: "ACME"})
		wantList := name == "estimate"
		if (errs.CodeOf(listErr) == errs.CodeDecodeFailed) != wantList ||
			errs.CodeOf(detailErr) != errs.CodeDecodeFailed {
			t.Fatalf("%s: list = %v, detail = %v, want decode_failed (list only for the estimate)",
				name, listErr, detailErr)
		}
	}
}
