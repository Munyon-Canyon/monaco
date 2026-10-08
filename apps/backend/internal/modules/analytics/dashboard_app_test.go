package analytics_test

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/analytics/app"
	rankingport "github.com/monaco/monaco/apps/backend/internal/modules/ranking/port"
	treasuryport "github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/bucket"
	dbsqlc "github.com/monaco/monaco/apps/backend/internal/platform/db/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type ledgerFake struct {
	buckets     []treasuryport.LedgerBucket
	bucketsErr  error
	platform    money.Micros
	platformErr error
}

func (f ledgerFake) LedgerTotals(context.Context, time.Time, time.Time, bucket.Size) (
	[]treasuryport.LedgerBucket, error,
) {
	return f.buckets, f.bucketsErr
}

func (f ledgerFake) PlatformBalanceTotal(context.Context) (money.Micros, error) {
	return f.platform, f.platformErr
}

type valuationFake struct {
	values    []rankingport.CabalValue
	valuesErr error
	run       rankingport.Run
	runErr    error
}

func (f valuationFake) LatestRun(context.Context) (rankingport.Run, error) { return f.run, f.runErr }

func (f valuationFake) LatestCabalValues(context.Context) ([]rankingport.CabalValue, error) {
	return f.values, f.valuesErr
}

func fakeMoney(l ledgerFake, v valuationFake) app.Money {
	return app.Money{
		Read: func(ctx context.Context, fn func(context.Context, dbsqlc.DBTX) error) error { return fn(ctx, nil) },
		Bind: func(dbsqlc.DBTX) app.MoneySources { return app.MoneySources{Ledger: l, Valuations: v} },
	}
}

func micros(v uint64) money.Micros { return money.MicrosFromUint64(v) }

func TestMoney_Dashboard_SumsPotsAndThePlatformBalance(t *testing.T) {
	t.Parallel()
	asOf := at(14, 12, 0)
	dash := fakeMoney(
		ledgerFake{platform: micros(20)},
		valuationFake{
			values: []rankingport.CabalValue{{Value: micros(70)}, {Value: micros(5)}},
			run:    rankingport.Run{AsOf: asOf.In(time.FixedZone("x", 3600))},
		},
	)
	got, err := dash.Dashboard(t.Context(), app.Window{})
	if err != nil || got.Pots != micros(75) || got.Platform != micros(20) || got.Total != micros(95) ||
		got.AsOf == nil || !got.AsOf.Equal(asOf) || got.AsOf.Location() != time.UTC {
		t.Fatalf("Dashboard() = %+v, %v, want 75 + 20 = 95 as of %v in UTC", got, err, asOf)
	}
}

func TestMoney_Dashboard_HasNoAsOfBeforeTheFirstRun(t *testing.T) {
	t.Parallel()
	dash := fakeMoney(ledgerFake{}, valuationFake{runErr: errs.New(errs.CodeNotFound, "test")})
	got, err := dash.Dashboard(t.Context(), app.Window{})
	if err != nil || got.AsOf != nil {
		t.Fatalf("Dashboard() = %+v, %v, want no as_of and no error", got, err)
	}
}

func TestMoney_Dashboard_Failures(t *testing.T) {
	t.Parallel()
	boom := errs.New(errs.CodeInternal, "test")
	big := []rankingport.CabalValue{{Value: micros(math.MaxUint64)}, {Value: micros(1)}}
	tests := map[string]struct {
		ledger ledgerFake
		vals   valuationFake
		read   error
	}{
		"buckets":          {ledger: ledgerFake{bucketsErr: boom}},
		"platform":         {ledger: ledgerFake{platformErr: boom}},
		"cabal values":     {vals: valuationFake{valuesErr: boom}},
		"pots overflow":    {vals: valuationFake{values: big}},
		"latest run":       {vals: valuationFake{runErr: boom}},
		"total overflow":   {ledger: ledgerFake{platform: micros(1)}, vals: valuationFake{values: big[:1]}},
		"read transaction": {read: boom},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dash := fakeMoney(tt.ledger, tt.vals)
			if tt.read != nil {
				dash.Read = func(context.Context, func(context.Context, dbsqlc.DBTX) error) error { return tt.read }
			}
			if got, err := dash.Dashboard(t.Context(), app.Window{}); err == nil {
				t.Fatalf("Dashboard() = %+v, want an error", got)
			}
		})
	}
}
