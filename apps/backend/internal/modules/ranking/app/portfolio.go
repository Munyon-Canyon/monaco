package app

import (
	"context"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type LatestSnapshots interface {
	LatestValuesOf(context.Context, []ids.CabalID) (map[ids.CabalID]domain.Snapshot, error)
}

type CabalView = cabal.View

type CabalCards interface {
	Cabals(context.Context, []ids.CabalID) (map[ids.CabalID]CabalView, error)
}

type PortfolioView struct {
	domain.Portfolio
	Cabals     map[ids.CabalID]CabalView
	ComputedAt *time.Time
	PricesAsOf *time.Time
}

type ReadPortfolio struct {
	User ids.UserID
}

func (r ReadPortfolio) Run(
	ctx context.Context, boards Boards, latest LatestSnapshots, stakes StakeHistory, cards CabalCards,
) (PortfolioView, error) {
	empty := PortfolioView{Portfolio: domain.Portfolio{Rows: []domain.PortfolioRow{}}}
	run, ok, err := boards.LatestRun(ctx)
	if err != nil || !ok {
		return empty, err
	}
	empty.ComputedAt, empty.PricesAsOf = &run.FinishedAt, &run.PricesAsOf
	history, err := stakes.UserStakeHistory(ctx, r.User)
	if err != nil || len(history) == 0 {
		return empty, err
	}
	points, cabals := stakePoints(history)
	values, err := latest.LatestValuesOf(ctx, cabals)
	if err != nil {
		return PortfolioView{}, err
	}
	portfolio, err := domain.NewPortfolio(points, values)
	if err != nil {
		return PortfolioView{}, errs.Wrap(err, errs.CodeInternal, "ranking.ReadPortfolio")
	}
	empty.Portfolio = portfolio
	if len(portfolio.Rows) == 0 {
		return empty, nil
	}
	held := make([]ids.CabalID, 0, len(portfolio.Rows))
	for _, row := range portfolio.Rows {
		held = append(held, row.CabalID)
	}
	if empty.Cabals, err = cards.Cabals(ctx, held); err != nil {
		return PortfolioView{}, err
	}
	for _, id := range held {
		if _, ok := empty.Cabals[id]; !ok {
			return PortfolioView{}, errs.New(errs.CodeInternal, "ranking.ReadPortfolio")
		}
	}
	return empty, nil
}
