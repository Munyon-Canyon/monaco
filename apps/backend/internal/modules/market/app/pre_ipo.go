package app

import (
	"context"
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type preIPOReader interface {
	PreIpoProfiles(context.Context, []string) ([]sqlc.PreIpoProfilesRow, error)
	PreIpoRounds(context.Context, []string) ([]sqlc.PreIpoRound, error)
}

var _ preIPOReader = (*sqlc.Queries)(nil)

func loadProfiles(ctx context.Context, read preIPOReader, assets []domain.Asset) (map[string]domain.Profile, error) {
	keys := preIPOKeys(assets)
	if read == nil || len(keys) == 0 {
		return map[string]domain.Profile{}, nil
	}
	rows, err := read.PreIpoProfiles(ctx, keys)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeOf(err), "market.PreIpoProfiles")
	}
	out := make(map[string]domain.Profile, len(rows))
	for _, row := range rows {
		profile, err := toProfile(row)
		if err != nil {
			return nil, err
		}
		out[row.CompanyKey] = profile
	}
	return out, nil
}

func loadRounds(ctx context.Context, read preIPOReader, key string) ([]domain.Round, error) {
	rows, err := read.PreIpoRounds(ctx, []string{key})
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeOf(err), "market.PreIpoRounds")
	}
	rounds, err := roundsByKey(rows)
	if err != nil {
		return nil, err
	}
	return rounds[key], nil
}

func preIPOKeys(assets []domain.Asset) []string {
	seen := map[string]bool{}
	var keys []string
	for _, asset := range assets {
		if asset.Kind != domain.KindPreIPO || seen[asset.CompanyKey] {
			continue
		}
		seen[asset.CompanyKey] = true
		keys = append(keys, asset.CompanyKey)
	}
	return keys
}

func roundsByKey(rows []sqlc.PreIpoRound) (map[string][]domain.Round, error) {
	out := map[string][]domain.Round{}
	for _, row := range rows {
		price, err := positiveMicros(row.PriceMicros, "market.PreIpoRounds", row.CompanyKey)
		if err != nil {
			return nil, err
		}
		round := domain.Round{On: domain.DateOn(row.RoundOn.Time), Price: price}
		out[row.CompanyKey] = append(out[row.CompanyKey], round)
	}
	return out, nil
}

func toProfile(row sqlc.PreIpoProfilesRow) (domain.Profile, error) {
	const op = "market.PreIpoProfiles"
	estimate, err := positiveMicros(row.EstimateMicros, op, row.CompanyKey)
	if err != nil {
		return domain.Profile{}, err
	}
	minOrder, err := positiveMicros(row.MinOrderMicros, op, row.CompanyKey)
	if err != nil {
		return domain.Profile{}, err
	}
	window, err := domain.ParseSellWindow(row.SellWindow)
	if err != nil {
		return domain.Profile{}, err
	}
	profile := domain.Profile{
		CompanyKey: row.CompanyKey,
		Estimate:   domain.Estimate{Price: estimate, AsOf: domain.DateOn(row.EstimateOn.Time)},
		Valuation:  row.ValuationMicros,
		MinOrder:   minOrder,
		SellWindow: window,
		ImageURL:   row.ImageUrl.String,
	}
	if row.NextTenderOn.Valid {
		next := domain.DateOn(row.NextTenderOn.Time)
		profile.NextTenderOn = &next
	}
	return profile, nil
}

func positiveMicros(v int64, op, key string) (money.Micros, error) {
	if v <= 0 {
		return money.Micros{}, errs.New(errs.CodeDecodeFailed, op,
			slog.String("company_key", key), slog.Int64("micros", v))
	}
	return money.MicrosFromUint64(uint64(v)), nil
}
