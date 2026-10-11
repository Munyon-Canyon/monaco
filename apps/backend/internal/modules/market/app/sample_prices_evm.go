package app

import (
	"context"
	"log/slog"
	"math"
	"math/big"
	"slices"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const (
	ListingGapBps  = 200
	evmPriceSource = string(multichain.VenueUniswap)
)

type EVMSampling struct {
	Source multichain.EVMPriceSource
	Gaps   metric.Int64Counter
}

func (p *SamplePrices) WithEVM(e EVMSampling) *SamplePrices {
	p.evm = e
	return p
}

type ListingGap struct {
	Listing HeldListing
	EVM     money.Micros
	Solana  money.Micros
	GapBps  int64
}

func (p *SamplePrices) sampleListings(
	ctx context.Context, at time.Time, solana map[domain.Mint]money.Micros,
) ([]slog.Attr, error) {
	if p.evm.Source == nil {
		return nil, nil
	}
	listings, err := p.heldListings(ctx)
	if err != nil || len(listings) == 0 {
		return nil, err
	}
	priced, sourceErr := p.priceListings(ctx, listings)
	if err := p.storeListingPrices(ctx, at, listings, priced); err != nil {
		return nil, err
	}
	gaps := listingGaps(listings, priced, solana)
	p.countGaps(ctx, gaps)
	attrs := []slog.Attr{
		slog.Int("evm_priced", len(priced)), slog.Int("evm_missing", len(listings)-len(priced)),
		slog.String("evm_gaps", gapNames(gaps)),
	}
	if sourceErr != nil {
		attrs = append(attrs, slog.String("evm_error", string(errs.CodeOf(sourceErr))))
	}
	return attrs, nil
}

func (p *SamplePrices) heldListings(ctx context.Context) (map[multichain.ListingID]HeldListing, error) {
	keys, err := p.heldListingKeys(ctx)
	if err != nil || len(keys) == 0 {
		return nil, err
	}
	byKey, err := p.listings.ListingPrices(ctx, keys, p.clock.Now())
	if err != nil {
		return nil, err
	}
	out := make(map[multichain.ListingID]HeldListing, len(byKey))
	for _, l := range byKey {
		out[l.ID] = l.HeldListing
	}
	return out, nil
}

func (p *SamplePrices) priceListings(
	ctx context.Context, listings map[multichain.ListingID]HeldListing,
) (map[multichain.ListingID]money.Micros, error) {
	asked := make([]multichain.Listing, 0, len(listings))
	for _, l := range listings {
		asked = append(asked, l.Listing)
	}
	fetchCtx, cancel := context.WithTimeout(ctx, p.fetchWindow())
	answered, err := p.evm.Source.Prices(fetchCtx, asked)
	cancel()
	priced := map[multichain.ListingID]money.Micros{}
	for id, price := range answered {
		if _, held := listings[id]; held && !price.PerShareMicros.IsZero() {
			if _, ok := storableMicros(price.PerShareMicros); ok {
				priced[id] = price.PerShareMicros
			}
		}
	}
	return priced, err
}

func (p *SamplePrices) storeListingPrices(
	ctx context.Context, at time.Time,
	listings map[multichain.ListingID]HeldListing, priced map[multichain.ListingID]money.Micros,
) error {
	rows := sqlc.InsertPricePointsParams{Ts: at, Source: evmPriceSource}
	for id, price := range priced {
		stored, _ := storableMicros(price)
		rows.Mints = append(rows.Mints, string(listings[id].Key))
		rows.PriceMicros = append(rows.PriceMicros, stored)
	}
	_, err := p.insert(ctx, rows)
	return err
}

func (p *SamplePrices) countGaps(ctx context.Context, gaps []ListingGap) {
	for _, g := range gaps {
		p.evm.Gaps.Add(ctx, 1, metric.WithAttributes(
			attribute.String("issuer", g.Listing.Issuer), attribute.String("network", string(g.Listing.Network)),
			attribute.String("symbol", g.Listing.Symbol)))
	}
}

func (p *SamplePrices) heldListingKeys(ctx context.Context) ([]ListingKey, error) {
	seen := map[ListingKey]bool{}
	for _, read := range p.hot {
		held, err := read(ctx)
		if err != nil {
			return nil, err
		}
		for _, asset := range held {
			if network, token, ok := ParseListingKey(string(asset)); ok && token != network.Cash().Address {
				seen[ListingKeyOf(network, token)] = true
			}
		}
	}
	keys := make([]ListingKey, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys, nil
}

func listingGaps(
	listings map[multichain.ListingID]HeldListing, priced map[multichain.ListingID]money.Micros,
	solana map[domain.Mint]money.Micros,
) []ListingGap {
	byMint := make(map[string]money.Micros, len(solana))
	for m, micros := range solana {
		byMint[m.String()] = micros
	}
	var gaps []ListingGap
	for id, evm := range priced {
		l := listings[id]
		if l.Asset == nil {
			continue
		}
		s := byMint[l.Asset.Mint]
		if g := GapBps(evm, s); !s.IsZero() && g > ListingGapBps {
			gaps = append(gaps, ListingGap{Listing: l, EVM: evm, Solana: s, GapBps: g})
		}
	}
	slices.SortFunc(gaps, func(a, b ListingGap) int {
		return strings.Compare(string(a.Listing.Key), string(b.Listing.Key))
	})
	return gaps
}

func GapBps(evm, solana money.Micros) int64 {
	e, s := evm.Uint64(), max(solana.Uint64(), 1)
	diff := new(big.Int).SetUint64(max(e, s) - min(e, s))
	bps := diff.Mul(diff, big.NewInt(10_000)).Quo(diff, new(big.Int).SetUint64(s))
	if !bps.IsInt64() {
		return math.MaxInt64
	}
	return bps.Int64()
}

func gapNames(gaps []ListingGap) string {
	names := make([]string, len(gaps))
	for i, g := range gaps {
		names[i] = g.Listing.Symbol + "@" + string(g.Listing.Network)
	}
	return strings.Join(names, ",")
}
