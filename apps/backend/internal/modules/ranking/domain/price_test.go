package domain_test

import (
	"slices"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
)

func TestPriceForBoard_ClosedUsesClose(t *testing.T) {
	t.Parallel()
	now := clock.Real{}.Now().UTC()
	closed := domain.Session{LastClose: now.Add(-40 * time.Hour)}
	atClose := domain.Sample{Price: usd(200_000_000), At: closed.LastClose.Add(-time.Minute)}
	latest := domain.Sample{Price: usd(260_000_000), At: now}
	wantPrice(t, closed, &latest, &atClose, now, atClose)
}

func TestPriceForBoard_OpenUsesLatest(t *testing.T) {
	t.Parallel()
	now := clock.Real{}.Now().UTC()
	open := domain.Session{Open: true, LastClose: now.Add(-16 * time.Hour)}
	latest := domain.Sample{Price: usd(260_000_000), At: now.Add(-time.Minute)}
	atClose := domain.Sample{Price: usd(200_000_000), At: open.LastClose}
	wantPrice(t, open, &latest, &atClose, now, latest)
}

func TestPriceForBoard_PreIPOUsesLatest(t *testing.T) {
	t.Parallel()
	now := clock.Real{}.Now().UTC()
	preIPO := domain.Session{Continuous: true, LastClose: now.Add(-40 * time.Hour)}
	latest := domain.Sample{Price: usd(9_000_000), At: now.Add(-2 * time.Minute)}
	wantPrice(t, preIPO, &latest, nil, now, latest)
}

func TestPriceForBoard_StaleAfterFiveMinutes(t *testing.T) {
	t.Parallel()
	now := clock.Real{}.Now().UTC()
	lastClose := now.Add(-40 * time.Hour)
	tests := map[string]struct {
		session domain.Session
		age     time.Duration
		stale   bool
	}{
		"open at five minutes": {session: domain.Session{Open: true}, age: domain.StaleAfter},
		"open past five minutes": {
			session: domain.Session{Open: true},
			age:     domain.StaleAfter + time.Nanosecond,
			stale:   true,
		},
		"pre-IPO past five minutes": {
			session: domain.Session{Continuous: true},
			age:     domain.StaleAfter + time.Nanosecond,
			stale:   true,
		},
		"closed at five before close": {session: domain.Session{LastClose: lastClose}, age: domain.StaleAfter},
		"closed past five before close": {
			session: domain.Session{LastClose: lastClose},
			age:     domain.StaleAfter + time.Nanosecond,
			stale:   true,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			asOf := now
			if !tt.session.Open && !tt.session.Continuous {
				asOf = lastClose
			}
			sample := domain.Sample{Price: usd(1), At: asOf.Add(-tt.age)}
			got, flags, fellBack := domain.PriceForBoard(tt.session, &sample, &sample, now)
			var want []domain.Flag
			if tt.stale {
				want = []domain.Flag{domain.FlagStalePrices}
			}
			if got != sample || !slices.Equal(flags, want) || fellBack {
				t.Fatalf(
					"PriceForBoard = %+v, %v, %v, want %+v, %v and no fallback",
					got,
					flags,
					fellBack,
					sample,
					want,
				)
			}
		})
	}
}

func TestPriceForBoard_Unpriced(t *testing.T) {
	t.Parallel()
	now := clock.Real{}.Now().UTC()
	fresh := domain.Sample{Price: usd(1), At: now}
	tests := map[string]struct {
		session         domain.Session
		latest, atClose *domain.Sample
	}{
		"open with no sample":         {session: domain.Session{Open: true}, atClose: &fresh},
		"closed with no close sample": {session: domain.Session{LastClose: now}, latest: &fresh},
		"pre-IPO with no sample":      {session: domain.Session{Continuous: true}, atClose: &fresh},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, flags, fellBack := domain.PriceForBoard(tt.session, tt.latest, tt.atClose, now)
			if got != (domain.Sample{}) || !slices.Equal(flags, []domain.Flag{domain.FlagUnpricedAssets}) || fellBack {
				t.Fatalf("PriceForBoard = %+v, %v, %v, want unpriced", got, flags, fellBack)
			}
		})
	}
}

func wantPrice(t *testing.T, s domain.Session, latest, atClose *domain.Sample, now time.Time, want domain.Sample) {
	t.Helper()
	got, flags, fellBack := domain.PriceForBoard(s, latest, atClose, now)
	if got != want || flags != nil || fellBack {
		t.Fatalf("PriceForBoard = %+v, %v, %v, want %+v, no flags and no fallback", got, flags, fellBack, want)
	}
}

func TestPriceForBoard_MissingCloseSampleUsesTheNewestOfTheLastSession(t *testing.T) {
	t.Parallel()
	now := clock.Real{}.Now().UTC()
	lastClose := now.Add(-40 * time.Hour)
	lastOpen := lastClose.Add(-6*time.Hour - 30*time.Minute)
	session := domain.Session{LastClose: lastClose, LastSessionOpen: lastOpen}
	tests := map[string]struct {
		sample       domain.Sample
		want         []domain.Flag
		wantFellBack bool
	}{
		"just past the five minute window": {
			sample: domain.Sample{
				Price: usd(5),
				At:    lastClose.Add(-domain.StaleAfter - time.Nanosecond),
			},
			wantFellBack: true,
		},
		"at the open": {sample: domain.Sample{Price: usd(5), At: lastOpen}, wantFellBack: true},
		"a minute before the open": {
			sample: domain.Sample{
				Price: usd(5),
				At:    lastOpen.Add(-time.Minute),
			},
			want: []domain.Flag{domain.FlagStalePrices},
		},
		"from an earlier session": {
			sample: domain.Sample{
				Price: usd(5),
				At:    lastOpen.Add(-24 * time.Hour),
			},
			want: []domain.Flag{domain.FlagStalePrices},
		},
		"at a zero price": {
			sample: domain.Sample{At: lastClose.Add(-time.Hour)}, want: []domain.Flag{domain.FlagStalePrices},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, flags, fellBack := domain.PriceForBoard(session, nil, &tt.sample, now)
			if got != tt.sample || !slices.Equal(flags, tt.want) || fellBack != tt.wantFellBack {
				t.Fatalf("PriceForBoard = %+v, %v, %v, want %+v, %v, %v",
					got, flags, fellBack, tt.sample, tt.want, tt.wantFellBack)
			}
		})
	}
}

func TestPriceForBoard_PresentCloseSampleNeverFallsBack(t *testing.T) {
	t.Parallel()
	now := clock.Real{}.Now().UTC()
	lastClose := now.Add(-40 * time.Hour)
	session := domain.Session{LastClose: lastClose, LastSessionOpen: lastClose.Add(-6*time.Hour - 30*time.Minute)}
	atClose := domain.Sample{Price: usd(7), At: lastClose.Add(-domain.StaleAfter)}
	wantPrice(t, session, nil, &atClose, now, atClose)
}

func TestPriceForBoard_OpenSessionStaysStaleWithAnOldLatestSample(t *testing.T) {
	t.Parallel()
	now := clock.Real{}.Now().UTC()
	session := domain.Session{Open: true, LastSessionOpen: now.Add(-24 * time.Hour)}
	latest := domain.Sample{Price: usd(7), At: now.Add(-time.Hour)}
	got, flags, fellBack := domain.PriceForBoard(session, &latest, nil, now)
	if got != latest || !slices.Equal(flags, []domain.Flag{domain.FlagStalePrices}) || fellBack {
		t.Fatalf("PriceForBoard = %+v, %v, %v, want a stale flag and no fallback", got, flags, fellBack)
	}
}
