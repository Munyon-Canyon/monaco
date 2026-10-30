package app

import (
	"context"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type stubCharter struct {
	chart Chart
	err   error
	got   ChartRequest
}

func (s *stubCharter) Handle(_ context.Context, _ string, req ChartRequest) (Chart, error) {
	s.got = req
	return s.chart, s.err
}

func TestPositionPrices_mapsTheChartClosesOfTheRange(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	charter := &stubCharter{chart: Chart{Points: []Point{{At: at, Close: money.MicrosFromUint64(5)}}}}
	got, err := NewPositionPrices(charter).PositionPrices(t.Context(), "AAPLx", "1M")
	if err != nil || len(got) != 1 || !got[0].At.Equal(at) || got[0].PriceMicros.Uint64() != 5 ||
		charter.got.Range != "1M" {
		t.Fatalf("prices = %+v, %v, request %+v, want one close of 5 on 1M", got, err, charter.got)
	}
}

func TestPositionPrices_passesTheChartErrorThrough(t *testing.T) {
	t.Parallel()
	charter := &stubCharter{err: errs.New(errs.CodeDBUnavailable, "test")}
	if _, err := NewPositionPrices(
		charter,
	).PositionPrices(t.Context(), "AAPLx", "1M"); errs.CodeOf(
		err,
	) != errs.CodeDBUnavailable {
		t.Fatalf("chart failure = %v, want db_unavailable", err)
	}
}
