package market_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
)

func TestSession_changeBaseKeepsThePreviousCloseFromTheCloseUntilTheNextOpen(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, at, wantBase, wantLastClose string
	}{
		{"open Monday", "2026-03-09 10:00", "2026-03-06 16:00", "2026-03-06 16:00"},
		{"Monday 17:00", "2026-03-09 17:00", "2026-03-06 16:00", "2026-03-09 16:00"},
		{"Tuesday pre-market", "2026-03-10 05:00", "2026-03-06 16:00", "2026-03-09 16:00"},
		{"Saturday", "2026-03-07 10:00", "2026-03-05 16:00", "2026-03-06 16:00"},
		{"Monday holiday", "2026-01-19 12:00", "2026-01-15 16:00", "2026-01-16 16:00"},
		{"early close day", "2026-11-27 14:00", "2026-11-25 16:00", "2026-11-27 13:00"},
	}
	for _, tc := range cases {
		session := equitySession(t, easternTime(t, tc.at))
		if got := session.ChangeBase(); !got.Equal(easternTime(t, tc.wantBase)) ||
			!session.LastClose.Equal(easternTime(t, tc.wantLastClose)) {
			t.Fatalf("%s: base %s last close %s, want base %s last close %s",
				tc.name, got, session.LastClose, tc.wantBase, tc.wantLastClose)
		}
	}
}

func TestSession_changeBaseFallsBackToTheLastCloseAtTheStartOfTheTable(t *testing.T) {
	t.Parallel()
	session := equitySession(t, easternTime(t, "2026-01-05 12:00"))
	if !session.PreviousClose.IsZero() || !session.ChangeBase().Equal(session.LastClose) ||
		session.State != domain.StateOpen {
		t.Fatalf("session = %+v", session)
	}
	closed := equitySession(t, easternTime(t, "2026-01-05 17:00"))
	if !closed.PreviousClose.Equal(easternTime(t, "2026-01-02 16:00")) ||
		!closed.ChangeBase().Equal(easternTime(t, "2026-01-02 16:00")) {
		t.Fatalf("closed session = %+v", closed)
	}
}
