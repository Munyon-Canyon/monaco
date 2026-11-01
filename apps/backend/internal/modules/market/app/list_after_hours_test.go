package app

import (
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestListAssets_reportsAFailedReadOfTheLastSessionsClose(t *testing.T) {
	t.Parallel()
	boom := errs.New(errs.CodeInternal, "market.test")
	apple := appleRow(t, "equity")
	saturday := time.Date(2026, 3, 7, 15, 0, 0, 0, time.UTC)
	sample := []sqlc.NewestSamplesRow{{Mint: apple.Mint, Ts: saturday, PriceMicros: 1}}
	calls := 0
	r := reader{
		symbol: func() ([]sqlc.Asset, error) { return []sqlc.Asset{apple}, nil },
		newest: func() ([]sqlc.NewestSamplesRow, error) {
			calls++
			if calls <= 2 {
				return sample, nil
			}
			return nil, boom
		},
	}
	var clk clock.Clock = testkit.NewClock(saturday)
	lister := &ListAssets{read: r, volumes: r, clock: clk}
	if _, err := lister.Handle(t.Context(), ListRequest{}); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("err = %v, want internal", err)
	}
}

func TestAfterHours_isAbsentWhenTheLastClosePriceIsZero(t *testing.T) {
	t.Parallel()
	asset := domain.Asset{Kind: domain.KindEquity}
	closed := domain.SessionInfo{State: domain.StateClosed}
	book := quotes{lastClosed: map[string][]domain.Sample{asset.Mint.String(): {{Micros: micros(0)}}}}
	if got := afterHours(asset, closed, book, micros(1_000_000)); got != nil {
		t.Fatalf("after hours = %d, want nil", *got)
	}
}

func TestChart_aHistoryChartReportsAFailedHeadlineRead(t *testing.T) {
	t.Parallel()
	boom := errs.New(errs.CodeInternal, "market.test")
	apple := appleRow(t, "equity")
	read := chartRead{
		asset:  func() (sqlc.Asset, error) { return apple, nil },
		newest: func() ([]sqlc.NewestSamplesRow, error) { return nil, boom },
	}
	hist := &fakeHistory{samples: historySamples()}
	chart := (&AssetChart{read: read, base: read, clock: testkit.NewClock(historyWhen())}).WithHistory(hist)
	if _, err := chart.Handle(t.Context(), "AAPLx", ChartRequest{Range: "1W"}); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("err = %v, want internal", err)
	}
}
