package adapters

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"math/big"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const usdcDecimals = 6

const usdcMint = "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v"

type Queries struct {
	q       queryStore
	history historyStore
	catalog app.MintResolver
	prices  app.PriceReader
	clock   clock.Clock
	usdc    chain.SolanaAddress
}

type queryStore interface {
	CabalPositions(context.Context, uuid.UUID) ([]sqlc.CabalPositionsRow, error)
	CabalTotalShares(context.Context, uuid.UUID) (string, error)
	CabalUserPosition(context.Context, sqlc.CabalUserPositionParams) (sqlc.CabalUserPositionRow, error)
	CabalStakeSnapshot(context.Context, sqlc.CabalStakeSnapshotParams) ([]sqlc.CabalStakeSnapshotRow, error)
	UserStakes(context.Context, uuid.UUID) ([]sqlc.UserStakesRow, error)
}

type historyStore interface {
	CabalUserShareUnitsAt(context.Context, sqlc.CabalUserShareUnitsAtParams) (string, error)
	CabalPositionSnapshotsAt(
		context.Context, sqlc.CabalPositionSnapshotsAtParams,
	) ([]sqlc.CabalPositionSnapshotsAtRow, error)
	MemberStakesAt(context.Context, time.Time) ([]sqlc.MemberStakesAtRow, error)
}

var _ port.Queries = (*Queries)(nil)

func NewQueries(
	db sqlc.DBTX, catalog app.MintResolver, prices app.PriceReader, c clock.Clock, usdc chain.SolanaAddress,
) *Queries {
	queries := sqlc.New(db)
	return &Queries{q: queries, history: queries, catalog: catalog, prices: prices, clock: c, usdc: usdc}
}

func (q *Queries) Positions(ctx context.Context, cabalID ids.CabalID) ([]port.Position, error) {
	rows, err := q.q.CabalPositions(ctx, cabalID.UUID())
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeOf(err), "treasury.Queries.Positions")
	}
	assets := map[chain.SolanaAddress]app.Asset{}
	positions := make([]port.Position, 0, len(rows))
	for _, row := range rows {
		position, err := q.position(ctx, row.Asset, row.Units, row.CostBasisMicros, assets)
		if err != nil {
			return nil, err
		}
		positions = append(positions, position)
	}
	return positions, nil
}

func (q *Queries) PotValue(ctx context.Context, cabalID ids.CabalID) (money.Micros, error) {
	rows, err := q.q.CabalPositions(ctx, cabalID.UUID())
	if err != nil {
		return money.Micros{}, errs.Wrap(err, errs.CodeOf(err), "treasury.Queries.PotValue")
	}
	assets := map[chain.SolanaAddress]app.Asset{}
	positions := make([]port.Position, 0, len(rows))
	for _, row := range rows {
		position, err := q.position(ctx, row.Asset, row.Units, row.CostBasisMicros, assets)
		if err != nil {
			return money.Micros{}, err
		}
		positions = append(positions, position)
	}
	return q.potValuePositions(ctx, positions, assets)
}

func (q *Queries) potValuePositions(
	ctx context.Context, positions []port.Position, assets map[chain.SolanaAddress]app.Asset,
) (money.Micros, error) {
	needsPrices := false
	for _, position := range positions {
		if position.Mint != q.usdc {
			needsPrices = true
			break
		}
	}
	prices := map[uuid.UUID]app.Price{}
	if needsPrices {
		var err error
		prices, err = q.prices.LatestPrices(ctx)
		if err != nil {
			return money.Micros{}, errs.Wrap(err, errs.CodeOf(err), "treasury.Queries.PotValue")
		}
	}
	var total money.Micros
	for _, position := range positions {
		value, err := q.positionValue(ctx, position, prices, assets)
		if err != nil {
			return money.Micros{}, err
		}
		total, err = total.Add(value)
		if err != nil {
			return money.Micros{}, errs.Wrap(err, errs.CodeOf(err), "treasury.Queries.PotValue")
		}
	}
	return total, nil
}

func (q *Queries) positionValue(
	ctx context.Context,
	position port.Position,
	prices map[uuid.UUID]app.Price,
	assets map[chain.SolanaAddress]app.Asset,
) (money.Micros, error) {
	if position.Mint == q.usdc {
		return money.MicrosFromUint64(position.Units.Uint64()), nil
	}
	asset, err := q.assetByAddress(ctx, position.Mint, assets)
	if err != nil {
		return money.Micros{}, priceUnavailable(err, position.Mint)
	}
	if !asset.ChainChecked {
		return money.Micros{}, priceUnavailable(nil, position.Mint)
	}
	price, ok := prices[asset.ID]
	if !ok || q.clock.Now().Sub(price.ObservedAt) > 5*time.Minute {
		return money.Micros{}, priceUnavailable(nil, position.Mint)
	}
	scale, err := power10(position.Units.Decimals())
	if err != nil {
		return money.Micros{}, errs.Wrap(err, errs.CodeInvalidInput, "treasury.Queries.positionValue")
	}
	multiplierNum, multiplierDen := asset.UIMultiplierAt(q.clock.Now())
	if multiplierNum <= 0 || multiplierDen <= 0 {
		return money.Micros{}, errs.New(errs.CodeInvalidInput, "treasury.Queries.positionValue")
	}
	value, err := valueWithMultiplier(
		position.Units.Uint64(), price.Micros.Uint64(), uint64(multiplierNum), scale, uint64(multiplierDen),
	)
	if err != nil {
		return money.Micros{}, errs.Wrap(err, errs.CodeOf(err), "treasury.Queries.positionValue")
	}
	return money.MicrosFromUint64(value), nil
}

func valueWithMultiplier(units, price, multiplier, scale, divisor uint64) (uint64, error) {
	numerator := new(big.Int).SetUint64(units)
	numerator.Mul(numerator, new(big.Int).SetUint64(price))
	numerator.Mul(numerator, new(big.Int).SetUint64(multiplier))
	denominator := new(big.Int).SetUint64(scale)
	denominator.Mul(denominator, new(big.Int).SetUint64(divisor))
	numerator.Quo(numerator, denominator)
	if !numerator.IsUint64() {
		return 0, errs.New(errs.CodeInvalidInput, "treasury.Queries.valueWithMultiplier")
	}
	return numerator.Uint64(), nil
}

func (q *Queries) TotalShares(ctx context.Context, cabalID ids.CabalID) (money.SharesUnits, error) {
	value, err := q.q.CabalTotalShares(ctx, cabalID.UUID())
	if err != nil {
		return money.SharesUnits{}, errs.Wrap(err, errs.CodeOf(err), "treasury.Queries.TotalShares")
	}
	return shares(value)
}

func (q *Queries) ShareUnits(ctx context.Context, cabalID ids.CabalID, userID ids.UserID) (money.SharesUnits, error) {
	position, err := q.userPosition(ctx, cabalID, userID)
	if errors.Is(err, sql.ErrNoRows) {
		return money.SharesUnits{}, nil
	}
	if err != nil {
		return money.SharesUnits{}, err
	}
	return shares(position.ShareUnits)
}

func (q *Queries) Stake(ctx context.Context, cabalID ids.CabalID, userID ids.UserID) (port.Stake, error) {
	rows, err := q.q.CabalStakeSnapshot(ctx, sqlc.CabalStakeSnapshotParams{
		CabalID: cabalID.UUID(), UserID: userID.UUID(),
	})
	if err != nil {
		return port.Stake{}, errs.Wrap(err, errs.CodeOf(err), "treasury.Queries.Stake")
	}
	if len(rows) == 0 {
		return port.Stake{CabalID: cabalID, UserID: userID}, nil
	}
	row := rows[0]
	stake, err := stakeFields(
		cabalID, userID, row.ShareUnits, row.ContributedMicros, row.WithdrawnMicros, row.TotalShares,
	)
	if err != nil {
		return port.Stake{}, err
	}
	if stake.ShareUnits.IsZero() {
		return stake, nil
	}
	assets := map[chain.SolanaAddress]app.Asset{}
	positions := make([]port.Position, 0, len(rows))
	for _, row := range rows {
		if !row.Asset.Valid {
			continue
		}
		position, err := q.position(ctx, row.Asset.String, row.Units, row.CostBasisMicros, assets)
		if err != nil {
			return port.Stake{}, err
		}
		positions = append(positions, position)
	}
	pot, err := q.potValuePositions(ctx, positions, assets)
	if err != nil {
		return port.Stake{}, err
	}
	stake.ValueMicros, err = domain.PayoutFor(stake.ShareUnits, stake.TotalShares, pot)
	return stake, err
}

func (q *Queries) StakesOf(ctx context.Context, userID ids.UserID) ([]port.Stake, error) {
	rows, err := q.q.UserStakes(ctx, userID.UUID())
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeOf(err), "treasury.Queries.StakesOf")
	}
	out := make([]port.Stake, 0, len(rows))
	for _, row := range rows {
		cabalID := ids.CabalIDFrom(row.CabalID)
		stake, err := q.Stake(ctx, cabalID, userID)
		if err != nil {
			return nil, err
		}
		out = append(out, stake)
	}
	return out, nil
}

func (q *Queries) ShareUnitsAt(
	ctx context.Context, cabalID ids.CabalID, userID ids.UserID, at time.Time,
) (money.SharesUnits, error) {
	value, err := q.history.CabalUserShareUnitsAt(ctx, sqlc.CabalUserShareUnitsAtParams{
		At: at, CabalID: cabalID.UUID(), UserID: userID.UUID(),
	})
	if err != nil {
		return money.SharesUnits{}, errs.Wrap(err, errs.CodeOf(err), "treasury.Queries.ShareUnitsAt")
	}
	return shares(value)
}

func (q *Queries) CabalPositionsAt(ctx context.Context, at time.Time) ([]port.CabalPositions, error) {
	rows, err := q.history.CabalPositionSnapshotsAt(ctx, sqlc.CabalPositionSnapshotsAtParams{
		At: at, Usdc: string(q.usdc),
	})
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeOf(err), "treasury.Queries.CabalPositionsAt")
	}
	out := []port.CabalPositions{}
	assets := map[chain.SolanaAddress]app.Asset{}
	for _, row := range rows {
		cabalID := ids.CabalIDFrom(row.CabalID)
		if len(out) == 0 || out[len(out)-1].CabalID != cabalID {
			shareUnits, err := shares(row.ShareUnits)
			if err != nil {
				return nil, err
			}
			out = append(out, port.CabalPositions{CabalID: cabalID, TotalShares: shareUnits})
		}
		position, err := q.position(ctx, row.Asset, row.Units, row.CostBasisMicros, assets)
		if err != nil {
			return nil, err
		}
		out[len(out)-1].Holdings = append(out[len(out)-1].Holdings, position)
	}
	return out, nil
}

func (q *Queries) MemberStakesAt(ctx context.Context, at time.Time) ([]port.MemberStake, error) {
	rows, err := q.history.MemberStakesAt(ctx, at)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeOf(err), "treasury.Queries.MemberStakesAt")
	}
	out := make([]port.MemberStake, 0, len(rows))
	for _, row := range rows {
		shareUnits, err := shares(row.ShareUnits)
		if err != nil {
			return nil, err
		}
		net, err := money.ParseSignedMicros(row.NetContributedMicros)
		if err != nil {
			return nil, errs.Wrap(err, errs.CodeDecodeFailed, "treasury.Queries.MemberStakesAt")
		}
		out = append(out, port.MemberStake{
			UserID: ids.UserIDFrom(row.UserID), CabalID: ids.CabalIDFrom(uuid.UUID(row.CabalID.Bytes)),
			ShareUnits:           shareUnits,
			NetContributedMicros: net,
		})
	}
	return out, nil
}

func (q *Queries) userPosition(
	ctx context.Context, cabalID ids.CabalID, userID ids.UserID,
) (sqlc.CabalUserPositionRow, error) {
	row, err := q.q.CabalUserPosition(ctx, sqlc.CabalUserPositionParams{
		CabalID: cabalID.UUID(), UserID: userID.UUID(),
	})
	if err != nil {
		return sqlc.CabalUserPositionRow{}, errs.Wrap(err, errs.CodeOf(err), "treasury.Queries.userPosition")
	}
	return row, nil
}

func stakeFields(
	cabalID ids.CabalID, userID ids.UserID, units, contributed, withdrawn, total string,
) (port.Stake, error) {
	shareUnits, err := shares(units)
	if err != nil {
		return port.Stake{}, err
	}
	contributedMicros, err := micros(contributed)
	if err != nil {
		return port.Stake{}, err
	}
	withdrawnMicros, err := micros(withdrawn)
	if err != nil {
		return port.Stake{}, err
	}
	totalShares, err := shares(total)
	if err != nil {
		return port.Stake{}, err
	}
	return port.Stake{
		CabalID: cabalID, UserID: userID, ShareUnits: shareUnits, TotalShares: totalShares,
		ContributedMicros: contributedMicros, WithdrawnMicros: withdrawnMicros,
	}, nil
}

func (q *Queries) position(
	ctx context.Context, assetRaw, unitsRaw, costRaw string, assets map[chain.SolanaAddress]app.Asset,
) (port.Position, error) {
	address, err := chain.ParseAddress(assetRaw)
	if err != nil {
		return port.Position{}, errs.Wrap(
			err, errs.CodeDecodeFailed, "treasury.Queries.position", slog.String("asset", assetRaw),
		)
	}
	decimals := uint8(usdcDecimals)
	if address != q.usdc {
		asset, err := q.assetByAddress(ctx, address, assets)
		if err != nil {
			return port.Position{}, priceUnavailable(err, address)
		}
		if !asset.ChainChecked {
			return port.Position{}, priceUnavailable(nil, address)
		}
		decimals = asset.Decimals
	}
	units, err := micros(unitsRaw)
	if err != nil {
		return port.Position{}, err
	}
	cost, err := micros(costRaw)
	if err != nil {
		return port.Position{}, err
	}
	return port.Position{Mint: address, Units: money.NewBaseUnits(units.Uint64(), decimals), CostBasis: cost}, nil
}

func (q *Queries) assetByAddress(
	ctx context.Context, address chain.SolanaAddress, assets map[chain.SolanaAddress]app.Asset,
) (app.Asset, error) {
	if asset, ok := assets[address]; ok {
		return asset, nil
	}
	asset, err := q.catalog.AssetByMint(ctx, address)
	if err != nil {
		return app.Asset{}, err
	}
	assets[address] = asset
	return asset, nil
}

func micros(raw string) (money.Micros, error) {
	value, err := money.ParseMicros(raw)
	if err != nil {
		return money.Micros{}, errs.Wrap(
			err, errs.CodeDecodeFailed, "treasury.Queries.micros", slog.String("value", raw),
		)
	}
	return value, nil
}

func shares(raw string) (money.SharesUnits, error) {
	value, err := money.ParseMicros(raw)
	if err != nil {
		return money.SharesUnits{}, errs.Wrap(
			err, errs.CodeDecodeFailed, "treasury.Queries.shares", slog.String("value", raw),
		)
	}
	return money.SharesUnitsFromUint64(value.Uint64()), nil
}

func priceUnavailable(cause error, mint chain.SolanaAddress) error {
	return errs.Wrap(
		cause, errs.CodePriceUnavailable, "treasury.Queries.PotValue", slog.String("mint", string(mint)),
	)
}

func power10(decimals uint8) (uint64, error) {
	if decimals > 19 {
		return 0, errs.New(errs.CodeInvalidInput, "treasury.Queries.power10", slog.Uint64("decimals", uint64(decimals)))
	}
	value := uint64(1)
	for range decimals {
		value *= 10
	}
	return value, nil
}
