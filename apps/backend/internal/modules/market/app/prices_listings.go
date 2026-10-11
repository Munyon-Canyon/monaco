package app

import (
	"context"
	"log/slog"
	"math/big"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type ListingKey string

func ListingKeyOf(n chain.Network, token string) ListingKey {
	if n.IsEVM() {
		token = strings.ToLower(token)
	}
	return ListingKey(string(n) + ":" + token)
}

func ParseListingKey(asset string) (chain.Network, string, bool) {
	network, token, ok := strings.Cut(asset, ":")
	if !ok {
		return "", "", false
	}
	n, err := chain.ParseNetwork(network)
	if err != nil || !n.IsEVM() || token == "" {
		return "", "", false
	}
	return n, token, true
}

type HeldListing struct {
	multichain.Listing
	Key     ListingKey
	AssetID *domain.AssetID
	Asset   *ListedAsset
}

type ListedAsset struct {
	Mint        string
	Symbol      string
	DisplayName string
	Kind        string
}

type ListingPrice struct {
	HeldListing
	Price *domain.Sample
}

func (p ListingPrice) Value(units *big.Int) (money.Micros, error) {
	if p.Price == nil {
		return money.Micros{}, errs.New(errs.CodePricesStale, "market.ListingPrice.Value",
			slog.String("listing", string(p.Key)))
	}
	return ListingValue(units, p.Decimals, p.SharesPerToken, p.Price.Micros)
}

func ListingValue(
	units *big.Int, decimals uint8, sharesPerToken *big.Rat, perShare money.Micros,
) (money.Micros, error) {
	v := new(big.Rat).SetInt(units)
	v.Mul(v, sharesPerToken)
	v.Mul(v, new(big.Rat).SetInt(new(big.Int).SetUint64(perShare.Uint64())))
	v.Quo(v, new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)))
	floor := new(big.Int).Quo(v.Num(), v.Denom())
	if !floor.IsUint64() {
		return money.Micros{}, errs.New(
			errs.CodeInvalidInput,
			"market.ListingValue",
			slog.String("value", floor.String()),
		)
	}
	return money.MicrosFromUint64(floor.Uint64()), nil
}

func (b *PriceBook) ListingPrices(
	ctx context.Context, keys []ListingKey, at time.Time,
) (map[ListingKey]ListingPrice, error) {
	const op = "market.PriceBook.ListingPrices"
	asked := make([]string, len(keys))
	for i, k := range keys {
		asked[i] = string(k)
	}
	rows, err := b.q.ListingPriceSamples(ctx, sqlc.ListingPriceSamplesParams{
		Keys: asked, At: at, PerKey: domain.HoldWindow,
	})
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeOf(err), op)
	}
	out := make(map[ListingKey]ListingPrice, len(keys))
	recent := map[ListingKey][]domain.Sample{}
	for _, row := range rows {
		key := ListingKey(row.Key)
		if _, seen := out[key]; !seen {
			l, err := listingOf(fieldsOf(row))
			if err != nil {
				return nil, err
			}
			out[key] = ListingPrice{HeldListing: l}
		}
		if !row.Sampled {
			continue
		}
		micros, err := money.SignedMicrosFromInt64(row.PriceMicros).Micros()
		if err != nil {
			return nil, errs.Wrap(err, errs.CodeDecodeFailed, op,
				slog.String("listing", row.Key), slog.Int64("price_micros", row.PriceMicros))
		}
		recent[key] = append(recent[key], domain.Sample{Micros: micros, ObservedAt: row.Ts})
	}
	for key, samples := range recent {
		if s, ok := domain.Accept(samples); ok {
			price := out[key]
			price.Price = &s
			out[key] = price
		}
	}
	return out, nil
}

type listingFields struct {
	ID             uuid.UUID
	AssetID        *uuid.UUID
	Issuer         string
	Symbol         string
	Network        string
	TokenAddress   string
	Decimals       int16
	SharesPerToken string
	Venue          string
	Key            string
	Asset          ListedAsset
}

func fieldsOf(row sqlc.ListingPriceSamplesRow) listingFields {
	f := listingFields{
		ID: row.ID, Issuer: row.Issuer, Symbol: row.Symbol, Network: row.Network, TokenAddress: row.TokenAddress,
		Decimals: row.Decimals.Int16, SharesPerToken: row.SharesPerToken, Venue: row.Venue, Key: row.Key,
		Asset: ListedAsset{
			Mint: row.AssetMint.String, Symbol: row.AssetSymbol.String,
			DisplayName: row.AssetDisplayName.String, Kind: row.AssetKind.String,
		},
	}
	if row.AssetID.Valid {
		id := uuid.UUID(row.AssetID.Bytes)
		f.AssetID = &id
	}
	return f
}

func listingOf(row listingFields) (HeldListing, error) {
	const op = "market.listingOf"
	network, err := chain.ParseNetwork(row.Network)
	if err != nil {
		return HeldListing{}, errs.Wrap(err, errs.CodeDecodeFailed, op, slog.String("listing", row.Key))
	}
	decimals, err := decimalsOf(row.Decimals)
	if err != nil {
		return HeldListing{}, errs.Wrap(err, errs.CodeDecodeFailed, op, slog.String("listing", row.Key))
	}
	spt, ok := new(big.Rat).SetString(row.SharesPerToken)
	if !ok || spt.Sign() <= 0 {
		return HeldListing{}, errs.New(errs.CodeDecodeFailed, op, slog.String("listing", row.Key),
			slog.String("shares_per_token", row.SharesPerToken))
	}
	l := HeldListing{
		Listing: multichain.Listing{
			ID: multichain.ListingIDFrom(row.ID), Issuer: row.Issuer, Symbol: row.Symbol, Network: network,
			TokenAddress: row.TokenAddress, Decimals: decimals, SharesPerToken: spt,
			Venue: multichain.Venue(row.Venue),
		},
		Key: ListingKey(row.Key),
	}
	if row.AssetID != nil {
		id, err := domain.ParseAssetID(row.AssetID.String())
		if err != nil {
			return HeldListing{}, errs.Wrap(err, errs.CodeDecodeFailed, op, slog.String("listing", row.Key))
		}
		asset := row.Asset
		l.AssetID, l.Asset = &id, &asset
	}
	return l, nil
}
