package app

import (
	"context"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	rankingport "github.com/monaco/monaco/apps/backend/internal/modules/ranking/port"
	treasuryport "github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	dbsqlc "github.com/monaco/monaco/apps/backend/internal/platform/db/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type MoneySources struct {
	Ledger     treasuryport.Dashboard
	Valuations rankingport.Queries
}

type MoneyView struct {
	Window   Window
	Buckets  []treasuryport.LedgerBucket
	Pots     money.Micros
	Platform money.Micros
	Total    money.Micros
	AsOf     *time.Time
}

type Money struct {
	Read ReadOnly
	Bind func(db dbsqlc.DBTX) MoneySources
}

func (m Money) Dashboard(ctx context.Context, w Window) (MoneyView, error) {
	view := MoneyView{Window: w}
	err := m.Read(ctx, func(ctx context.Context, db dbsqlc.DBTX) error {
		src := m.Bind(db)
		steps := []func(context.Context) error{
			func(ctx context.Context) (err error) {
				view.Buckets, err = src.Ledger.LedgerTotals(ctx, w.From, w.To, w.Size)
				return err
			},
			func(ctx context.Context) (err error) {
				view.Platform, err = src.Ledger.PlatformBalanceTotal(ctx)
				return err
			},
			func(ctx context.Context) error { return view.sumPots(ctx, src.Valuations) },
			func(ctx context.Context) error { return view.stampValuation(ctx, src.Valuations) },
		}
		for _, step := range steps {
			if err := step(ctx); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return MoneyView{}, err
	}
	view.Total, err = view.Pots.Add(view.Platform)
	return view, err
}

func (v *MoneyView) sumPots(ctx context.Context, valuations rankingport.Queries) error {
	values, err := valuations.LatestCabalValues(ctx)
	if err != nil {
		return err
	}
	for _, value := range values {
		if v.Pots, err = v.Pots.Add(value.Value); err != nil {
			return err
		}
	}
	return nil
}

func (v *MoneyView) stampValuation(ctx context.Context, valuations rankingport.Queries) error {
	run, err := valuations.LatestRun(ctx)
	switch {
	case errs.CodeOf(err) == errs.CodeNotFound:
		return nil
	case err != nil:
		return err
	}
	at := run.AsOf.UTC()
	v.AsOf = &at
	return nil
}
