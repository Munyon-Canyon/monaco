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
			got, flags := domain.PriceForBoard(tt.session, &sample, &sample, now)
			var want []domain.Flag
			if tt.stale {
				want = []domain.Flag{domain.FlagStalePrices}
			}
			if got != sample || !slices.Equal(flags, want) {
				t.Fatalf("PriceForBoard = %+v, %v, want %+v, %v", got, flags, sample, want)
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
			got, flags := domain.PriceForBoard(tt.session, tt.latest, tt.atClose, now)
			if got != (domain.Sample{}) || !slices.Equal(flags, []domain.Flag{domain.FlagUnpricedAssets}) {
				t.Fatalf("PriceForBoard = %+v, %v, want unpriced", got, flags)
			}
		})
	}
}

func wantPrice(t *testing.T, s domain.Session, latest, atClose *domain.Sample, now time.Time, want domain.Sample) {
	t.Helper()
	got, flags := domain.PriceForBoard(s, latest, atClose, now)
	if got != want || flags != nil {
		t.Fatalf("PriceForBoard = %+v, %v, want %+v and no flags", got, flags, want)
	}
}
