package adapters

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestPositionAndAmountDecodingFailures(t *testing.T) {
	t.Parallel()
	queries := &Queries{usdc: usdcMint}
	if _, err := queries.position(t.Context(), usdcMint, "bad", "1", map[chain.SolanaAddress]app.Asset{}); err == nil {
		t.Fatal("position with bad units error = nil")
	}
	if _, err := queries.position(t.Context(), usdcMint, "1", "bad", map[chain.SolanaAddress]app.Asset{}); err == nil {
		t.Fatal("position with bad cost error = nil")
	}
	if _, err := micros("bad"); err == nil {
		t.Fatal("micros error = nil")
	}
	if _, err := shares("bad"); err == nil {
		t.Fatal("shares error = nil")
	}
	if got, err := shares("1"); err != nil || got != money.SharesUnitsFromUint64(1) {
		t.Fatalf("shares() = %v, %v", got, err)
	}
	queries.catalog = app.MintResolver(func(context.Context, chain.SolanaAddress) (app.Asset, error) {
		return app.Asset{}, errs.New(errs.CodeAssetNotFound, "test")
	})
	if _, err := queries.position(
		t.Context(), "XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp", "1", "1", map[chain.SolanaAddress]app.Asset{},
	); err == nil {
		t.Fatal("position with unavailable asset error = nil")
	}
	queries.catalog = app.MintResolver(func(context.Context, chain.SolanaAddress) (app.Asset, error) {
		return app.Asset{}, nil
	})
	if _, err := queries.position(
		t.Context(), "XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp", "1", "1", map[chain.SolanaAddress]app.Asset{},
	); err == nil {
		t.Fatal("position with unchecked asset error = nil")
	}
	if _, err := power10(20); err == nil {
		t.Fatal("power10 overflow error = nil")
	}
	queries.catalog = app.MintResolver(func(context.Context, chain.SolanaAddress) (app.Asset, error) {
		return app.Asset{Decimals: 20, ChainChecked: true}, nil
	})
	queries.clock = testkit.NewClock(time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC))
	if _, err := queries.positionValue(t.Context(), port.Position{
		Mint: "XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp", Units: money.NewBaseUnits(1, 20),
	}, map[uuid.UUID]app.Price{uuid.Nil: {Micros: money.MicrosFromUint64(1), ObservedAt: queries.clock.Now()}}, map[chain.SolanaAddress]app.Asset{}); err == nil {
		t.Fatal("positionValue scale error = nil")
	}
}

func TestQueryValueAndShareFailures(t *testing.T) {
	t.Parallel()
	prices := app.PriceReader(func(context.Context) (map[uuid.UUID]app.Price, error) {
		return map[uuid.UUID]app.Price{}, nil
	})
	q := newTestQueries(valueStore{}, prices)
	badRow := valueStore{rows: []sqlc.CabalPositionsRow{{Asset: "bad", Units: "1", CostBasisMicros: "0"}}}
	if _, err := newTestQueries(badRow, prices).PotValue(t.Context(), ids.CabalID{}); err == nil {
		t.Fatal("PotValue bad row error = nil")
	}
	positions := []port.Position{
		{Mint: chain.SolanaAddress(usdcMint), Units: money.NewBaseUnits(math.MaxUint64, usdcDecimals)},
		{Mint: chain.SolanaAddress(usdcMint), Units: money.NewBaseUnits(1, usdcDecimals)},
	}
	if _, err := q.potValuePositions(t.Context(), positions, map[chain.SolanaAddress]app.Asset{}); err == nil {
		t.Fatal("potValuePositions overflow error = nil")
	}
	assetMint := chain.SolanaAddress("XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp")
	q.catalog = app.MintResolver(func(context.Context, chain.SolanaAddress) (app.Asset, error) {
		return app.Asset{}, errs.New(errs.CodeAssetNotFound, "test")
	})
	if _, err := q.positionValue(
		t.Context(), port.Position{Mint: assetMint}, nil, map[chain.SolanaAddress]app.Asset{},
	); err == nil {
		t.Fatal("positionValue unavailable asset error = nil")
	}
	q.catalog = app.MintResolver(func(context.Context, chain.SolanaAddress) (app.Asset, error) {
		return app.Asset{Decimals: 0, ChainChecked: true}, nil
	})
	q.clock = testkit.NewClock(time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC))
	priceMap := map[uuid.UUID]app.Price{
		uuid.Nil: {
			Micros:     money.MicrosFromUint64(math.MaxUint64),
			ObservedAt: time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC),
		},
	}
	if _, err := q.positionValue(t.Context(), port.Position{
		Mint: assetMint, Units: money.NewBaseUnits(math.MaxUint64, 0),
	}, priceMap, map[chain.SolanaAddress]app.Asset{}); err == nil {
		t.Fatal("positionValue multiplication overflow error = nil")
	}
	q.catalog = app.MintResolver(func(context.Context, chain.SolanaAddress) (app.Asset, error) {
		return app.Asset{}, nil
	})
	assets := map[chain.SolanaAddress]app.Asset{}
	if _, err := q.positionValue(t.Context(), port.Position{Mint: assetMint}, priceMap, assets); err == nil {
		t.Fatal("positionValue unchecked asset error = nil")
	}
	failingReader := app.PriceReader(func(context.Context) (map[uuid.UUID]app.Price, error) {
		return nil, errs.New(errs.CodeDBUnavailable, "test")
	})
	priceFailure := newTestQueries(valueStore{}, failingReader)
	priceFailure.catalog = app.MintResolver(func(context.Context, chain.SolanaAddress) (app.Asset, error) {
		return app.Asset{ChainChecked: true}, nil
	})
	if _, err := priceFailure.potValuePositions(t.Context(), []port.Position{{Mint: assetMint}}, assets); err == nil {
		t.Fatal("potValuePositions price error = nil")
	}
	if _, err := newTestQueries(valueStore{total: "bad"}, prices).TotalShares(t.Context(), ids.CabalID{}); err == nil {
		t.Fatal("TotalShares malformed value error = nil")
	}
}

func TestPositionValueUsesEffectiveUIMultiplier(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)
	assetMint := chain.SolanaAddress("XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp")
	asset := app.Asset{
		ID: uuid.Nil, Decimals: 0, ChainChecked: true, UIMultiplierNum: 5, UIMultiplierDen: 1,
		NextUIMultiplierNum: 2, NextUIMultiplierDen: 1, NextUIMultiplierAt: at,
	}
	q := newTestQueries(valueStore{}, nil)
	assets := map[chain.SolanaAddress]app.Asset{}
	q.catalog = app.MintResolver(func(context.Context, chain.SolanaAddress) (app.Asset, error) { return asset, nil })
	price := map[uuid.UUID]app.Price{uuid.Nil: {Micros: money.MicrosFromUint64(3), ObservedAt: at}}
	position := port.Position{Mint: assetMint, Units: money.NewBaseUnits(4, 0)}
	q.clock = testkit.NewClock(at.Add(-time.Nanosecond))
	if got, err := q.positionValue(
		t.Context(), position, price, assets,
	); err != nil || got != money.MicrosFromUint64(60) {
		t.Fatalf("positionValue before step = %v, %v", got, err)
	}
	q.clock = testkit.NewClock(at)
	assets = map[chain.SolanaAddress]app.Asset{}
	got, err := q.positionValue(t.Context(), position, price, assets)
	if err != nil || got != money.MicrosFromUint64(24) {
		t.Fatalf("positionValue at step = %v, %v", got, err)
	}
	asset.UIMultiplierNum = 0
	asset.NextUIMultiplierDen = 0
	assets = map[chain.SolanaAddress]app.Asset{}
	if _, err := q.positionValue(t.Context(), position, price, assets); err == nil {
		t.Fatal("positionValue invalid multiplier error = nil")
	}
	asset.UIMultiplierNum, asset.UIMultiplierDen = 2, 1
	assets = map[chain.SolanaAddress]app.Asset{}
	overflowPrice := map[uuid.UUID]app.Price{
		uuid.Nil: {Micros: money.MicrosFromUint64(1), ObservedAt: at},
	}
	if _, err := q.positionValue(t.Context(), port.Position{
		Mint: assetMint, Units: money.NewBaseUnits(math.MaxUint64, 0),
	}, overflowPrice, assets); err == nil {
		t.Fatal("positionValue multiplier overflow error = nil")
	}
	fractionalPrice := map[uuid.UUID]app.Price{
		uuid.Nil: {Micros: money.MicrosFromUint64(30_000_000), ObservedAt: at},
	}
	if got, err := q.positionValue(t.Context(), port.Position{
		Mint: assetMint, Units: money.NewBaseUnits(5, 8),
	}, fractionalPrice, assets); err != nil || got != money.MicrosFromUint64(3) {
		t.Fatalf("positionValue fractional multiplier = %v, %v", got, err)
	}
}

func newTestQueries(store queryStore, prices app.PriceReader) *Queries {
	return &Queries{
		q:      store,
		prices: prices,
		clock:  testkit.NewClock(time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)),
		usdc:   usdcMint,
	}
}

type valueStore struct {
	total string
	rows  []sqlc.CabalPositionsRow
}

func (s valueStore) CabalPositions(context.Context, uuid.UUID) ([]sqlc.CabalPositionsRow, error) {
	return s.rows, nil
}

func (s valueStore) CabalTotalShares(context.Context, uuid.UUID) (string, error) { return s.total, nil }
