package treasury_test

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/treasuryapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

const (
	potEVMAAPL     = "base:0xb200000000000000000000000000000000000a01"
	potEVMUnlinked = "base:0xb200000000000000000000000000000000000b02"
	sharePrice     = 30_000_000
)

type listingQuotes map[string]app.ListingQuote

func (l listingQuotes) read(_ context.Context, keys []string, _ time.Time) (map[string]app.ListingQuote, error) {
	out := map[string]app.ListingQuote{}
	for _, k := range keys {
		if q, ok := l[k]; ok {
			out[k] = q
		}
	}
	return out, nil
}

func quote(
	t *testing.T,
	id string,
	network chain.Network,
	asset *app.ListedAsset,
	symbol string,
	observed time.Time,
) app.ListingQuote {
	t.Helper()
	listingID, err := uuid.Parse(id)
	if err != nil {
		t.Fatal(err)
	}
	price := money.MicrosFromUint64(sharePrice)
	spt := big.NewRat(1, 1)
	return app.ListingQuote{
		ListingID: listingID, Asset: asset, Issuer: "xstocks", Symbol: symbol, Network: network,
		Decimals: 18, SharesPerToken: spt, Price: &app.Price{Micros: price, ObservedAt: observed},
		Value: func(units *big.Int) (money.Micros, error) {
			v := new(big.Int).Mul(units, big.NewInt(sharePrice))
			return money.MicrosFromUint64(v.Quo(v, new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)).Uint64()), nil
		},
	}
}

func (p potCase) multichain(t *testing.T) listingQuotes {
	t.Helper()
	now := p.f.clock.Now()
	aapl := marketfake.AAPLx()
	asset := &app.ListedAsset{
		Mint:        aapl.Mint.String(),
		Symbol:      aapl.Symbol,
		DisplayName: aapl.DisplayName,
		Kind:        "equity",
	}
	solana := quote(t, "b9103e13-4f65-59a1-a6c9-90159264e601", chain.NetworkSolana, asset, aapl.Symbol, now)
	quotes := listingQuotes{
		potEVMAAPL: quote(
			t,
			"b9103e13-4f65-59a1-a6c9-90159264e602",
			chain.NetworkBase,
			asset,
			"AAPLx",
			now,
		),
		"solana:" + aapl.Mint.String(): solana,
	}
	p.q.WithListings(quotes.read)
	p.prices.Set(aapl.ID, money.MicrosFromUint64(sharePrice), now)
	return quotes
}

func (p potCase) inTransit(t *testing.T, micros int64) {
	t.Helper()
	if _, err := p.f.pool.Exec(
		t.Context(),
		`INSERT INTO treasury_in_transit (cabal_id, trade_id, network, amount_micros,
		created_at) VALUES ($1, $2, 'base', $3, $4)`,
		p.cabal.UUID(),
		p.f.ids.NewV7(),
		micros,
		p.f.clock.Now(),
	); err != nil {
		t.Fatal(err)
	}
}

func TestGetCabalPot_totalsEveryChainAndShowsOneHoldingPerStock(t *testing.T) {
	t.Parallel()
	p := newPotCase(t)
	p.multichain(t)
	p.buy(t, 20_000_000, 100_000_000)
	p.hold(t, potEVMAAPL, 500_000_000_000_000_000, 15_000_000)
	p.hold(t, "base:"+chain.NetworkBase.Cash().Address, 7_000_000, 7_000_000)
	p.inTransit(t, 5_000_000)

	got, err := p.get(t, p.funder, p.funder)
	if err != nil {
		t.Fatal(err)
	}
	const wantTotal = 80_000_000 + 7_000_000 + 5_000_000 + 30_000_000 + 15_000_000
	if got.PotValueMicros != wantTotal || got.CashMicros != 92_000_000 || sumWeights(got) != 10000 {
		t.Fatalf("pot = %d, cash %d, weights %d, want %d, 92000000, 10000", got.PotValueMicros, got.CashMicros,
			sumWeights(got), wantTotal)
	}
	value, err := p.q.PotValue(t.Context(), p.cabal)
	if err != nil || value.Uint64() != wantTotal {
		t.Fatalf("PotValue = %v, %v, want %d", value, err, wantTotal)
	}
	wantOneAAPLHolding(t, got)
}

func wantOneAAPLHolding(t *testing.T, got api.CabalPot) {
	t.Helper()
	if len(got.Holdings) != 1 {
		t.Fatalf("holdings = %#v, want one AAPLx holding", got.Holdings)
	}
	h := got.Holdings[0]
	if h.Symbol != "AAPLx" || h.Units != "1.5000" || h.ValueMicros != 45_000_000 || h.CostBasisMicros != 35_000_000 ||
		h.PnlMicros != 10_000_000 || h.TokenAmount != 100_000_000 {
		t.Fatalf("AAPLx holding = %#v", h)
	}
	wantSolanaAndBase(t, h.Listings)
}

func wantSolanaAndBase(t *testing.T, listings *[]api.HoldingListing) {
	t.Helper()
	if listings == nil || len(*listings) != 2 {
		t.Fatalf("AAPLx listings = %#v, want Solana and Base", listings)
	}
	solana, base := (*listings)[0], (*listings)[1]
	if solana.Network != "solana" || solana.ValueMicros != 30_000_000 || base.Network != "base" ||
		base.TokenAmount != "500000000000000000" || base.ValueMicros != 15_000_000 {
		t.Fatalf("AAPLx listings = %#v", *listings)
	}
}

func TestGetCabalPot_anEVMListingTheCatalogDoesNotLinkIsItsOwnHolding(t *testing.T) {
	t.Parallel()
	p := newPotCase(t)
	quotes := p.multichain(t)
	quotes[potEVMUnlinked] = quote(t, "b9103e13-4f65-59a1-a6c9-90159264e603", chain.NetworkBase, nil, "COINc",
		p.f.clock.Now())
	p.hold(t, potEVMUnlinked, 2_000_000_000_000_000_000, 50_000_000)

	got, err := p.get(t, p.funder, p.funder)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Holdings) != 1 || got.Holdings[0].Symbol != "COINc" || got.Holdings[0].Units != "2.0000" ||
		got.Holdings[0].ValueMicros != 60_000_000 || got.Holdings[0].PriceMicros != sharePrice ||
		got.PotValueMicros != 160_000_000 {
		t.Fatalf("pot = %#v", got)
	}
}

func TestGetCabalPot_anEVMPositionWithoutAFreshPriceFailsThePot(t *testing.T) {
	t.Parallel()
	for name, prepare := range map[string]func(*testing.T, potCase, listingQuotes){
		"no listing": func(t *testing.T, p potCase, _ listingQuotes) {
			t.Helper()
			p.hold(t, potEVMUnlinked, 1, 1)
		},
		"stale price": func(t *testing.T, p potCase, q listingQuotes) {
			t.Helper()
			q[potEVMAAPL] = quote(t, "b9103e13-4f65-59a1-a6c9-90159264e602", chain.NetworkBase, nil, "AAPLx",
				p.f.clock.Now().Add(-6*time.Minute))
			p.hold(t, potEVMAAPL, 1, 1)
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			p := newPotCase(t)
			prepare(t, p, p.multichain(t))
			if _, err := p.get(t, p.funder, p.funder); errs.CodeOf(err) != errs.CodePriceUnavailable {
				t.Fatalf("pot err = %v, want price_unavailable", err)
			}
			if _, err := p.q.PotValue(t.Context(), p.cabal); errs.CodeOf(err) != errs.CodePriceUnavailable {
				t.Fatalf("PotValue err = %v, want price_unavailable", err)
			}
		})
	}
}
