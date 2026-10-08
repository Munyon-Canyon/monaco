package analytics_test

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/analytics/app"
	governanceport "github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
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
		Bind: func(dbsqlc.DBTX) app.MoneySources {
			return app.MoneySources{Ledger: l, Valuations: v, Events: countFake{}}
		},
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

type proposalDashFake struct {
	buckets    []governanceport.ProposalBucket
	bucketsErr error
	passed     governanceport.PassTime
	passedErr  error
	rows       []governanceport.CabalParticipation
	rowsErr    error
	open       int64
	openErr    error
}

func (f proposalDashFake) ProposalCounts(context.Context, time.Time, time.Time, bucket.Size) (
	[]governanceport.ProposalBucket, error,
) {
	return f.buckets, f.bucketsErr
}

func (f proposalDashFake) MedianTimeToPass(context.Context, time.Time, time.Time) (governanceport.PassTime, error) {
	return f.passed, f.passedErr
}

func (f proposalDashFake) VoteParticipation(context.Context, time.Time, time.Time) (
	[]governanceport.CabalParticipation, error,
) {
	return f.rows, f.rowsErr
}

func (f proposalDashFake) OpenCount(context.Context) (int64, error) { return f.open, f.openErr }

func fakeGovernance(f proposalDashFake) app.Governance {
	return app.Governance{
		Read: func(ctx context.Context, fn func(context.Context, dbsqlc.DBTX) error) error { return fn(ctx, nil) },
		Bind: func(dbsqlc.DBTX) governanceport.Dashboard { return f },
	}
}

func TestGovernance_Dashboard_ComputesParticipationInBasisPoints(t *testing.T) {
	t.Parallel()
	got, err := fakeGovernance(proposalDashFake{
		rows: []governanceport.CabalParticipation{
			{Proposals: 1, Eligible: 3, Voted: 1},
			{Proposals: 1, Eligible: 0, Voted: 0},
			{Proposals: 2, Eligible: 4, Voted: 4},
		},
		open: 7,
	}).Dashboard(t.Context(), app.Window{})
	if err != nil || len(got.Participation) != 3 || got.Open != 7 {
		t.Fatalf("Dashboard() = %+v, %v, want three rows and 7 open", got, err)
	}
	bps := [3]int64{got.Participation[0].Bps, got.Participation[1].Bps, got.Participation[2].Bps}
	if bps != [3]int64{3333, 0, 10000} {
		t.Fatalf("participation bps = %v, want [3333 0 10000]", bps)
	}
}

func TestGovernance_Dashboard_Failures(t *testing.T) {
	t.Parallel()
	boom := errs.New(errs.CodeInternal, "test")
	tests := map[string]proposalDashFake{
		"buckets":       {bucketsErr: boom},
		"time to pass":  {passedErr: boom},
		"participation": {rowsErr: boom},
		"open count":    {openErr: boom},
	}
	for name, f := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got, err := fakeGovernance(f).Dashboard(t.Context(), app.Window{}); err == nil {
				t.Fatalf("Dashboard() = %+v, want an error", got)
			}
		})
	}
}

func TestMoney_Dashboard_FailsWhenTheOnrampCountFails(t *testing.T) {
	t.Parallel()
	dash := fakeMoney(ledgerFake{}, valuationFake{})
	dash.Bind = func(dbsqlc.DBTX) app.MoneySources {
		return app.MoneySources{
			Ledger:     ledgerFake{},
			Valuations: valuationFake{},
			Events:     countFake{err: errs.New(errs.CodeInternal, "test")},
		}
	}
	if got, err := dash.Dashboard(t.Context(), app.Window{}); err == nil {
		t.Fatalf("Dashboard() = %+v, want an error", got)
	}
}
