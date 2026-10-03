package app

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"math"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/sqlc"
)

type Catalog struct {
	q *sqlc.Queries
}

func NewCatalog(db sqlc.DBTX) *Catalog { return &Catalog{q: sqlc.New(db)} }

func (c *Catalog) AssetByID(ctx context.Context, id domain.AssetID) (domain.Asset, error) {
	row, err := c.q.AssetByID(ctx, id.UUID())
	return one(row, err, "market.Catalog.AssetByID", slog.String("id", id.String()))
}

func (c *Catalog) AssetByMint(ctx context.Context, mint domain.Mint) (domain.Asset, error) {
	row, err := c.q.AssetByMint(ctx, mint.String())
	return one(row, err, "market.Catalog.AssetByMint", slog.String("mint", mint.String()))
}

func (c *Catalog) AssetBySymbol(ctx context.Context, symbol string) (domain.Asset, error) {
	row, err := c.q.AssetBySymbol(ctx, symbol)
	return one(row, err, "market.Catalog.AssetBySymbol", slog.String("symbol", symbol))
}

func (c *Catalog) ListTradable(ctx context.Context) ([]domain.Asset, error) {
	rows, err := c.q.ListTradableAssets(ctx)
	return many(rows, err, "market.Catalog.ListTradable")
}

func (c *Catalog) ListAll(ctx context.Context) ([]domain.Asset, error) {
	rows, err := c.q.ListAssets(ctx)
	return many(rows, err, "market.Catalog.ListAll")
}

func one(row sqlc.Asset, err error, op string, key slog.Attr) (domain.Asset, error) {
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return domain.Asset{}, errs.New(errs.CodeAssetNotFound, op, key)
	case err != nil:
		return domain.Asset{}, errs.Wrap(err, errs.CodeOf(err), op, key)
	}
	return toAsset(row)
}

func many(rows []sqlc.Asset, err error, op string) ([]domain.Asset, error) {
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeOf(err), op)
	}
	out := make([]domain.Asset, len(rows))
	for i, row := range rows {
		if out[i], err = toAsset(row); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func toAsset(row sqlc.Asset) (domain.Asset, error) {
	const op = "market.toAsset"
	id, idErr := domain.ParseAssetID(row.ID.String())
	mint, mintErr := domain.ParseMint(row.Mint)
	issuer, issuerErr := domain.ParseIssuer(row.Issuer)
	kind, kindErr := domain.ParseKind(row.Kind)
	decimals, decimalsErr := decimalsOf(row.Decimals)
	if err := errors.Join(idErr, mintErr, issuerErr, kindErr, decimalsErr); err != nil {
		return domain.Asset{}, errs.Wrap(err, errs.CodeDecodeFailed, op, slog.String("symbol", row.Symbol))
	}
	override := domain.OverrideAuto
	switch {
	case !row.TradableOverride.Valid:
	case row.TradableOverride.Bool:
		override = domain.OverrideOn
	default:
		override = domain.OverrideOff
	}
	return domain.Asset{
		ID:               id,
		Symbol:           row.Symbol,
		Mint:             mint,
		Decimals:         decimals,
		Issuer:           issuer,
		Kind:             kind,
		DisplayName:      row.DisplayName,
		LogoURL:          row.LogoUrl.String,
		UIMultiplier:     domain.Multiplier{Num: row.UiMultiplierNum, Den: row.UiMultiplierDen},
		NextUIMultiplier: nextMultiplier(row),
		ChainChecked:     row.ChainCheckedAt.Valid,
		IssuerTradable:   row.IssuerTradable,
		Override:         override,
		PopularRank:      row.PopularRank.Int16,
		CompanyKey:       row.CompanyKey,
		FirstSeenAt:      row.FirstSeenAt,
		UpdatedAt:        row.UpdatedAt,
	}, nil
}

func nextMultiplier(row sqlc.Asset) domain.MultiplierStep {
	if !row.UiMultiplierNextNum.Valid {
		return domain.MultiplierStep{}
	}
	return domain.MultiplierStep{
		To: domain.Multiplier{Num: row.UiMultiplierNextNum.Int64, Den: row.UiMultiplierNextDen.Int64},
		At: row.UiMultiplierNextAt.Time,
	}
}

func decimalsOf(v int16) (uint8, error) {
	if v < 0 || v > math.MaxUint8 {
		return 0, errs.New(errs.CodeDecodeFailed, "market.decimalsOf", slog.Int("decimals", int(v)))
	}
	return uint8(v), nil
}
