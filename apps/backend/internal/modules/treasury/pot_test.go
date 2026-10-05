package treasury_test

import (
	"context"
	"math/big"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/treasuryapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

type potCabals struct {
	known     ids.CabalID
	members   map[ids.UserID]bool
	cabalsErr error
	memberErr error
}

func (p potCabals) Cabals(_ context.Context, cabalIDs []ids.CabalID) (map[ids.CabalID]cabalport.CabalView, error) {
	out := map[ids.CabalID]cabalport.CabalView{}
	for _, id := range cabalIDs {
		if id == p.known {
			out[id] = cabalport.CabalView{ID: id, Status: cabalport.StatusBanned}
		}
	}
	return out, p.cabalsErr
}

func (p potCabals) IsMember(_ context.Context, _ ids.CabalID, user ids.UserID) (bool, error) {
	return p.members[user], p.memberErr
}

type potCase struct {
	f      fixture
	cabal  ids.CabalID
	funder ids.UserID
	prices *marketfake.PricesFake
	reads  *int
	q      *adapters.Queries
}

func newPotCase(t *testing.T) potCase {
	t.Helper()
	f := newFixture(t)
	funder, cabal := f.user(t), f.cabal(t)
	u, c, err := f.fund(funder, cabal, 100_000_000, 100, domain.TxnSettled)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.postPair(u, c); err != nil {
		t.Fatal(err)
	}
	prices := &marketfake.PricesFake{}
	reads := 0
	reader := priceReader(prices)
	q := adapterQueriesWithReader(f, func(ctx context.Context) (map[uuid.UUID]app.Price, error) {
		reads++
		return reader(ctx)
	})
	return potCase{f: f, cabal: cabal, funder: funder, prices: prices, reads: &reads, q: q}
}

func (p potCase) buy(t *testing.T, micros, units int64) {
	t.Helper()
	swap, err := p.f.swap(p.cabal, micros, units)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.f.postCabal(swap); err != nil {
		t.Fatal(err)
	}
}

func (p potCase) hold(t *testing.T, mint string, units, cost int64) {
	t.Helper()
	const insert = `INSERT INTO cabal_positions (cabal_id, asset, units, cost_basis_micros, updated_at)
VALUES ($1, $2, $3, $4, $5)`
	if _, err := p.f.pool.Exec(t.Context(), insert, p.cabal.UUID(), mint, units, cost, p.f.clock.Now()); err != nil {
		t.Fatal(err)
	}
}

func (p potCase) get(t *testing.T, viewer ids.UserID, members ...ids.UserID) (api.CabalPot, error) {
	t.Helper()
	isMember := map[ids.UserID]bool{}
	for _, m := range members {
		isMember[m] = true
	}
	cabals := potCabals{known: p.cabal, members: isMember}
	ctx := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: viewer.String()})
	res, err := adapters.HTTP{Pot: p.q, Cabals: cabals, Members: cabals}.GetCabalPot(
		ctx, api.GetCabalPotRequestObject{Id: p.cabal.UUID()},
	)
	if err != nil {
		return api.CabalPot{}, err
	}
	return api.CabalPot(res.(api.GetCabalPot200JSONResponse)), nil
}

func sumWeights(pot api.CabalPot) int32 {
	total := pot.CashWeightBps
	for _, h := range pot.Holdings {
		total += h.WeightBps
	}
	return total
}

func sameMicros(got int64, want money.Micros) bool {
	return big.NewInt(got).Cmp(new(big.Int).SetUint64(want.Uint64())) == 0
}

func TestGetCabalPot_Member(t *testing.T) {
	t.Parallel()
	p := newPotCase(t)
	p.buy(t, 40_000_000, 200_000_000)
	p.hold(t, marketfake.TSLAx().Mint.String(), 73_000_000, 50_000_000)
	now := p.f.clock.Now()
	p.prices.Set(marketfake.AAPLx().ID, money.MicrosFromUint64(30_000_000), now)
	p.prices.Set(marketfake.TSLAx().ID, money.MicrosFromUint64(100_000_000), now.Add(-time.Minute))
	got, err := p.get(t, p.funder, p.funder)
	if err != nil {
		t.Fatal(err)
	}
	want, err := p.q.PotValue(t.Context(), p.cabal)
	if err != nil || !sameMicros(got.PotValueMicros, want) {
		t.Fatalf("pot_value_micros = %d, PotValue = %v, %v", got.PotValueMicros, want, err)
	}
	assertMemberHoldings(t, got)
	if got.CashMicros != 60_000_000 || sumWeights(got) != 10000 || !got.PricesAsOf.Equal(now.Add(-time.Minute)) {
		t.Fatalf("pot = %#v", got)
	}
	if got.PnlMicros != got.PotValueMicros-100_000_000 || got.ReturnBps == nil {
		t.Fatalf("pnl = %d, return = %v", got.PnlMicros, got.ReturnBps)
	}
	assertFunderSlice(t, p, got.Me)
}

func assertFunderSlice(t *testing.T, p potCase, me *api.CabalPotSlice) {
	t.Helper()
	stake, err := p.q.Stake(t.Context(), p.cabal, p.funder)
	if err != nil || me == nil || !sameMicros(me.ValueMicros, stake.ValueMicros) {
		t.Fatalf("me = %#v, stake %#v, %v", me, stake, err)
	}
	if me.SliceBps != 10000 || me.NetContributedMicros != 100_000_000 {
		t.Fatalf("me = %#v", me)
	}
}

func assertMemberHoldings(t *testing.T, got api.CabalPot) {
	t.Helper()
	if len(got.Holdings) != 2 || got.Holdings[0].Symbol != "TSLAx" || got.Holdings[1].Symbol != "AAPLx" {
		t.Fatalf("holdings = %#v", got.Holdings)
	}
	tsla := got.Holdings[0]
	if tsla.Units != "0.7300" || tsla.ValueMicros != 73_000_000 || tsla.PnlMicros != 23_000_000 {
		t.Fatalf("TSLAx holding = %#v", tsla)
	}
	if tsla.DisplayName != marketfake.TSLAx().DisplayName || tsla.PriceMicros != 100_000_000 {
		t.Fatalf("TSLAx names and price = %#v", tsla)
	}
	if tsla.TokenAmount != 73_000_000 || tsla.Kind != api.Equity {
		t.Fatalf("TSLAx token amount and kind = %#v", tsla)
	}
}

func TestGetCabalPot_NonMember(t *testing.T) {
	t.Parallel()
	p := newPotCase(t)
	got, err := p.get(t, p.f.user(t), p.funder)
	if err != nil || got.Me != nil || got.PotValueMicros != 100_000_000 || got.CashWeightBps != 10000 {
		t.Fatalf("pot = %#v, %v", got, err)
	}
	if !got.PricesAsOf.Equal(p.f.clock.Now()) || len(got.Holdings) != 0 || got.ReturnBps == nil || *got.ReturnBps != 0 {
		t.Fatalf("pot = %#v", got)
	}
}

func TestGetCabalPot_MemberNoShares(t *testing.T) {
	t.Parallel()
	p := newPotCase(t)
	joiner := p.f.user(t)
	got, err := p.get(t, joiner, p.funder, joiner)
	if err != nil || got.Me == nil || *got.Me != (api.CabalPotSlice{}) {
		t.Fatalf("me = %#v, %v", got.Me, err)
	}
}

func TestGetCabalPot_StalePrice(t *testing.T) {
	t.Parallel()
	p := newPotCase(t)
	p.buy(t, 40_000_000, 200_000_000)
	p.prices.Set(marketfake.AAPLx().ID, money.MicrosFromUint64(30_000_000), p.f.clock.Now().Add(-6*time.Minute))
	_, err := p.get(t, p.funder, p.funder)
	wantCode(t, err, errs.CodePriceUnavailable)
	if !errs.Retryable(errs.CodePriceUnavailable) {
		t.Fatal("price_unavailable is not retryable")
	}
}

func TestGetCabalPot_UnknownCabal(t *testing.T) {
	t.Parallel()
	p := newPotCase(t)
	cabals := potCabals{known: p.f.cabal(t)}
	ctx := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: p.funder.String()})
	_, err := adapters.HTTP{Pot: p.q, Cabals: cabals, Members: cabals}.GetCabalPot(
		ctx, api.GetCabalPotRequestObject{Id: p.cabal.UUID()},
	)
	wantCode(t, err, errs.CodeCabalNotFound)
}

func TestGetCabalPot_OnePriceRead(t *testing.T) {
	t.Parallel()
	p := newPotCase(t)
	if _, err := p.get(t, p.funder, p.funder); err != nil || *p.reads != 0 {
		t.Fatalf("USDC-only price reads = %d, %v", *p.reads, err)
	}
	p.buy(t, 40_000_000, 200_000_000)
	p.hold(t, marketfake.TSLAx().Mint.String(), 1, 1)
	p.hold(t, marketfake.JPSTx().Mint.String(), 1, 1)
	for _, asset := range marketfake.Fixtures() {
		p.prices.Set(asset.ID, money.MicrosFromUint64(30_000_000), p.f.clock.Now())
	}
	got, err := p.get(t, p.funder, p.funder)
	if err != nil || len(got.Holdings) != 3 || *p.reads != 1 {
		t.Fatalf("three holdings price reads = %d, holdings %d, %v", *p.reads, len(got.Holdings), err)
	}
}

func TestGetCabalPot_refusesFailedReads(t *testing.T) {
	t.Parallel()
	p := newPotCase(t)
	signedIn := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: p.funder.String()})
	fail := errs.New(errs.CodeUpstreamUnavailable, "test")
	req := api.GetCabalPotRequestObject{Id: p.cabal.UUID()}
	if _, err := (adapters.HTTP{Pot: p.q}).GetCabalPot(t.Context(), req); errs.CodeOf(err) != errs.CodeUnauthorized {
		t.Fatalf("signed out error = %v", err)
	}
	for _, cabals := range []potCabals{{known: p.cabal, cabalsErr: fail}, {known: p.cabal, memberErr: fail}} {
		_, err := adapters.HTTP{Pot: p.q, Cabals: cabals, Members: cabals}.GetCabalPot(signedIn, req)
		wantCode(t, err, errs.CodeUpstreamUnavailable)
	}
}

func TestPotRoute_servesAnyUserAndRefusesAnUnknownCabal(t *testing.T) {
	t.Parallel()
	s := scenario.New(t, withActivity())
	c := testkit.NewCabal(t, s.DB())
	stranger := testkit.SeedUser(t, s.DB(), testkit.UserOpts{})
	s.Given(scenario.AsSeededUser("stranger", stranger.ID)).
		When(
			scenario.Get("/v1/cabals/"+c.ID.String()+"/pot"),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("pot_value_micros", 0),
			scenario.ExpectJSON("me", nil),
		)
	s.When(scenario.Get("/v1/cabals/"+testkit.NewIDs(5).NewV7().String()+"/pot")).
		Then(scenario.ExpectStatus(http.StatusNotFound), scenario.ExpectProblem(errs.CodeCabalNotFound))
}
