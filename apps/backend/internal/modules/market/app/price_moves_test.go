package app

import (
	"context"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type fakeBook struct {
	asOf    map[domain.AssetID]domain.Sample
	asOfErr error
	days    []domain.Sample
	dayErr  error
}

func (fakeBook) PointsCount(context.Context) (int64, error) { return 0, nil }

func (fakeBook) RecentSamples(context.Context) (map[domain.AssetID][]domain.Sample, error) {
	return map[domain.AssetID][]domain.Sample{}, nil
}

func (fakeBook) SamplesOf(context.Context, []domain.AssetID) (map[domain.AssetID][]domain.Sample, error) {
	return map[domain.AssetID][]domain.Sample{}, nil
}

func (f fakeBook) PricesAsOf(context.Context, []domain.AssetID, time.Time) (map[domain.AssetID]domain.Sample, error) {
	return f.asOf, f.asOfErr
}

func (f fakeBook) DaySamples(context.Context, domain.AssetID, time.Time, time.Time) ([]domain.Sample, error) {
	return f.days, f.dayErr
}

func TestRecordMoves_reportsAReferenceReadFailure(t *testing.T) {
	t.Parallel()
	at := equityOpenAt(t, clock.Real{}.Now())
	id := domain.NewAssetID(testkit.NewIDs(7))
	asset := domain.Asset{ID: id, Kind: domain.KindEquity, Symbol: "AAPLx"}
	mark := domain.Sample{Micros: money.MicrosFromUint64(220_000_000), ObservedAt: at}
	p := &SamplePrices{
		book:  fakeBook{asOfErr: errs.New(errs.CodeDBUnavailable, "test.prices")},
		clock: testkit.NewClock(at),
	}
	if err := p.recordMoves(
		t.Context(),
		[]domain.Asset{asset},
		at,
		map[domain.AssetID]domain.Sample{id: mark},
	); errs.CodeOf(
		err,
	) != errs.CodeDBUnavailable {
		t.Fatalf("recordMoves = %v, want db_unavailable", err)
	}
}

func TestRecordMoves_reportsAPreIPOWindowFailure(t *testing.T) {
	t.Parallel()
	at := clock.Real{}.Now()
	id := domain.NewAssetID(testkit.NewIDs(8))
	asset := domain.Asset{ID: id, Kind: domain.KindPreIPO, Symbol: "SPACx"}
	mark := domain.Sample{Micros: money.MicrosFromUint64(220_000_000), ObservedAt: at}
	p := &SamplePrices{
		book:  fakeBook{dayErr: errs.New(errs.CodeDBUnavailable, "test.day")},
		clock: testkit.NewClock(at),
	}
	if err := p.recordMoves(
		t.Context(),
		[]domain.Asset{asset},
		at,
		map[domain.AssetID]domain.Sample{id: mark},
	); errs.CodeOf(
		err,
	) != errs.CodeDBUnavailable {
		t.Fatalf("recordMoves = %v, want db_unavailable", err)
	}
}

func equityOpenAt(t *testing.T, from time.Time) time.Time {
	t.Helper()
	day := time.Date(from.UTC().Year(), from.UTC().Month(), from.UTC().Day(), 15, 0, 0, 0, time.UTC)
	for i := range 14 {
		at := day.AddDate(0, 0, i)
		if at.Before(from.UTC()) {
			continue
		}
		info, err := domain.Session(domain.KindEquity, at)
		if err != nil {
			t.Fatal(err)
		}
		if info.State == domain.StateOpen {
			return at
		}
	}
	t.Fatal("no open equity session")
	return time.Time{}
}
