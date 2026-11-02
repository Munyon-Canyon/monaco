package app

import (
	"context"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/sqlc"
)

type sinceRecorder struct {
	dayBaseReader
	since time.Time
}

func (r *sinceRecorder) FirstSamplesSince(
	_ context.Context, p sqlc.FirstSamplesSinceParams,
) ([]sqlc.FirstSamplesSinceRow, error) {
	r.since = p.Since
	return nil, nil
}

func TestOpenedSamples_startsThePreIPODayAtEasternMidnight(t *testing.T) {
	t.Parallel()
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]struct{ now, want time.Time }{
		"winter evening, already tomorrow in UTC": {
			now:  time.Date(2026, 12, 8, 21, 30, 0, 0, ny),
			want: time.Date(2026, 12, 8, 0, 0, 0, 0, ny),
		},
		"summer evening, already tomorrow in UTC": {
			now:  time.Date(2026, 7, 14, 20, 30, 0, 0, ny),
			want: time.Date(2026, 7, 14, 0, 0, 0, 0, ny),
		},
		"a minute after midnight in New York": {
			now:  time.Date(2026, 7, 15, 0, 1, 0, 0, ny),
			want: time.Date(2026, 7, 15, 0, 0, 0, 0, ny),
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			rec := &sinceRecorder{}
			_, since, err := openedSamples(t.Context(), rec, []string{"mint"}, tc.now)
			if err != nil {
				t.Fatal(err)
			}
			if !since.Equal(tc.want) || !rec.since.Equal(tc.want) {
				t.Fatalf("day starts at %s (query %s), want %s", since, rec.since, tc.want)
			}
		})
	}
}

func splitAsset(step time.Time, to domain.Multiplier) domain.Asset {
	return domain.Asset{
		Kind:             domain.KindEquity,
		UIMultiplier:     domain.Multiplier{Num: 1, Den: 1},
		NextUIMultiplier: domain.MultiplierStep{To: to, At: step},
	}
}

func changeAcross(t *testing.T, asset domain.Asset, closed, now uint64, closeAt, at time.Time) *int32 {
	t.Helper()
	mint := asset.Mint.String()
	session := domain.SessionInfo{State: domain.StateOpen, LastClose: closeAt}
	book := quotes{
		newest:  map[string][]domain.Sample{mint: {{Micros: micros(now), ObservedAt: at}}},
		closed:  map[string][]domain.Sample{mint: {{Micros: micros(closed), ObservedAt: closeAt}}},
		closeAt: closeAt,
	}
	return summary(asset, session, book, at).Change
}

func TestSummary_aTwoForOneSplitIsNotAMinusFiftyPercentDay(t *testing.T) {
	t.Parallel()
	closeAt := time.Date(2026, 11, 2, 21, 0, 0, 0, time.UTC)
	step := closeAt.Add(2 * time.Hour)
	at := closeAt.Add(14 * time.Hour)
	split := splitAsset(step, domain.Multiplier{Num: 2, Den: 1})
	got := changeAcross(t, split, 200_000_000, 101_000_000, closeAt, at)
	if got == nil || *got != 100 {
		t.Fatalf("change after a 2-for-1 split that closed at 200 and trades at 101 = %v, want +100 bps", got)
	}
	unsplit := changeAcross(t, domain.Asset{Kind: domain.KindEquity, Mint: split.Mint}, 200_000_000, 101_000_000,
		closeAt, at)
	if unsplit == nil || *unsplit != -4950 {
		t.Fatalf("change with no split on record = %v, want -4950 bps, the move the split rescale removes", unsplit)
	}
}

func TestSummary_aReverseSplitRescalesTheCloseUp(t *testing.T) {
	t.Parallel()
	closeAt := time.Date(2026, 11, 2, 21, 0, 0, 0, time.UTC)
	reverse := splitAsset(closeAt.Add(time.Hour), domain.Multiplier{Num: 1, Den: 10})
	got := changeAcross(t, reverse, 5_000_000, 49_000_000, closeAt, closeAt.Add(14*time.Hour))
	if got == nil || *got != -200 {
		t.Fatalf("change after a 1-for-10 reverse split that closed at 5 and trades at 49 = %v, want -200 bps", got)
	}
}

func TestSummary_aDividendSizedMultiplierStepLeavesTheCloseAlone(t *testing.T) {
	t.Parallel()
	closeAt := time.Date(2026, 11, 2, 21, 0, 0, 0, time.UTC)
	dividend := splitAsset(closeAt.Add(time.Hour), domain.Multiplier{Num: 1_003, Den: 1_000})
	got := changeAcross(t, dividend, 200_000_000, 198_000_000, closeAt, closeAt.Add(14*time.Hour))
	if got == nil || *got != -100 {
		t.Fatalf("change across a 0.3%% multiplier step = %v, want the plain -100 bps", got)
	}
}

func TestSummary_aSplitAfterNowLeavesTheCloseAlone(t *testing.T) {
	t.Parallel()
	closeAt := time.Date(2026, 11, 2, 21, 0, 0, 0, time.UTC)
	later := splitAsset(closeAt.Add(48*time.Hour), domain.Multiplier{Num: 2, Den: 1})
	got := changeAcross(t, later, 200_000_000, 202_000_000, closeAt, closeAt.Add(14*time.Hour))
	if got == nil || *got != 100 {
		t.Fatalf("change before a scheduled split takes effect = %v, want the plain +100 bps", got)
	}
}
