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

type failingVolume struct{ err error }

func (f failingVolume) Volume24h(context.Context, string, time.Time) (*money.Micros, error) {
	return nil, f.err
}

func TestRecordMoves_reportsAVolumeReadFailure(t *testing.T) {
	t.Parallel()
	at := equityOpenAt(t, clock.Real{}.Now())
	id := domain.NewAssetID(testkit.NewIDs(9))
	asset := domain.Asset{ID: id, Kind: domain.KindEquity, Symbol: "AAPLx"}
	ref := domain.Sample{Micros: money.MicrosFromUint64(200_000_000), ObservedAt: at.Add(-time.Hour)}
	mark := domain.Sample{Micros: money.MicrosFromUint64(220_000_000), ObservedAt: at}
	p := &SamplePrices{
		book:   fakeBook{asOf: map[domain.AssetID]domain.Sample{id: ref}},
		volume: failingVolume{err: errs.New(errs.CodeDBUnavailable, "test.volume")},
		ids:    testkit.NewIDs(3),
		clock:  testkit.NewClock(at),
	}
	err := p.recordMoves(t.Context(), []domain.Asset{asset}, at, map[domain.AssetID]domain.Sample{id: mark})
	if errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Fatalf("recordMoves = %v, want db_unavailable", err)
	}
}

type countingBook struct {
	fakeBook
	asOfCalls *int
	dayCalls  *int
}

func (c countingBook) PricesAsOf(
	ctx context.Context, ids []domain.AssetID, at time.Time,
) (map[domain.AssetID]domain.Sample, error) {
	*c.asOfCalls++
	return c.fakeBook.PricesAsOf(ctx, ids, at)
}

func (c countingBook) DaySamples(
	ctx context.Context, id domain.AssetID, from, to time.Time,
) ([]domain.Sample, error) {
	*c.dayCalls++
	return c.fakeBook.DaySamples(ctx, id, from, to)
}

func TestRecordMoves_readsEachCloseOncePerSession(t *testing.T) {
	t.Parallel()
	at := equityOpenAt(t, clock.Real{}.Now())
	ids := testkit.NewIDs(11)
	ref := domain.Sample{Micros: money.MicrosFromUint64(200_000_000), ObservedAt: at.Add(-time.Hour)}
	assets := make([]domain.Asset, 0, 3)
	refs := map[domain.AssetID]domain.Sample{}
	for _, sym := range []string{"AAPLx", "TSLAx", "NVDAx"} {
		id := domain.NewAssetID(ids)
		assets = append(assets, domain.Asset{ID: id, Kind: domain.KindEquity, Symbol: sym})
		refs[id] = ref
	}
	var asOf, day int
	p := &SamplePrices{
		book:  countingBook{fakeBook: fakeBook{asOf: refs}, asOfCalls: &asOf, dayCalls: &day},
		clock: testkit.NewClock(at),
	}
	for tick := range 2 {
		bucket := at.Add(time.Duration(tick) * domain.SampleBucket)
		marks := map[domain.AssetID]domain.Sample{}
		for _, a := range assets {
			marks[a.ID] = domain.Sample{Micros: ref.Micros, ObservedAt: bucket}
		}
		if err := p.recordMoves(t.Context(), assets, bucket, marks); err != nil {
			t.Fatal(err)
		}
	}
	if asOf != 1 {
		t.Fatalf("PricesAsOf calls = %d, want 1 for three equities over two ticks", asOf)
	}
}

func TestRecordMoves_readsAPreIPODayOpenOncePerDay(t *testing.T) {
	t.Parallel()
	now := clock.Real{}.Now().UTC()
	at := time.Date(now.Year(), now.Month(), now.Day(), 15, 0, 0, 0, time.UTC)
	id := domain.NewAssetID(testkit.NewIDs(12))
	asset := domain.Asset{ID: id, Kind: domain.KindPreIPO, Symbol: "SPACx"}
	var asOf, day int
	p := &SamplePrices{clock: testkit.NewClock(at)}
	tick := func(bucket time.Time) {
		t.Helper()
		start := time.Date(bucket.Year(), bucket.Month(), bucket.Day(), 0, 0, 0, 0, time.UTC)
		open := domain.Sample{Micros: money.MicrosFromUint64(50_000_000), ObservedAt: start.Add(time.Minute)}
		p.book = countingBook{fakeBook: fakeBook{days: []domain.Sample{open}}, asOfCalls: &asOf, dayCalls: &day}
		mark := domain.Sample{Micros: open.Micros, ObservedAt: bucket}
		if err := p.recordMoves(
			t.Context(), []domain.Asset{asset}, bucket, map[domain.AssetID]domain.Sample{id: mark},
		); err != nil {
			t.Fatal(err)
		}
	}
	tick(at)
	tick(at.Add(domain.SampleBucket))
	if day != 1 {
		t.Fatalf("DaySamples calls = %d, want 1 across two ticks on one day", day)
	}
	tick(at.AddDate(0, 0, 1))
	if day != 2 {
		t.Fatalf("DaySamples calls = %d, want 2 after the next trading day starts", day)
	}
}
