package domain_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
)

func ledgerCall(seed, kind string, move int, right bool, decided time.Time) domain.LedgerCall {
	return domain.LedgerCall{
		ProposalID: uuid.NewSHA1(uuid.Nil, []byte(seed)), Kind: kind, Symbol: seed, YourChoice: domain.SideYes,
		MoveBps: domain.YourMove(kind, domain.SideYes, move), Right: right, DecidedAt: decided,
	}
}

func TestLedgerWeek_Week42(t *testing.T) {
	t.Parallel()
	week, labelStart, labelEnd, start, end := domain.LedgerWeek(time.Date(2026, 10, 18, 17, 0, 0, 0, time.UTC))
	if week != "2026-W42" || labelStart.Day() != 12 || labelEnd.Day() != 18 ||
		!start.Equal(time.Date(2026, 10, 11, 17, 0, 0, 0, time.UTC)) ||
		!end.Equal(time.Date(2026, 10, 18, 17, 0, 0, 0, time.UTC)) {
		t.Fatalf("LedgerWeek = %s %v %v %v %v", week, labelStart, labelEnd, start, end)
	}
}

func TestYourMove(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		kind, choice string
		move, want   int
	}{
		"sell yes after a fall": {domain.CallSell, domain.SideYes, -420, 420},
		"buy yes after a rise":  {domain.CallBuy, domain.SideYes, 300, 300},
		"buy no after a rise":   {domain.CallBuy, domain.SideNo, 300, -300},
		"sell no after a fall":  {domain.CallSell, domain.SideNo, -240, -240},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := domain.YourMove(tc.kind, tc.choice, tc.move); got != tc.want {
				t.Fatalf("YourMove = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestPickBestAndMiss_Week42(t *testing.T) {
	t.Parallel()
	sat := time.Date(2026, 10, 17, 15, 0, 0, 0, time.UTC)
	calls := []domain.LedgerCall{
		ledgerCall("TSLA", domain.CallSell, 240, false, sat.Add(-24*time.Hour)),
		ledgerCall("AMZN", domain.CallSell, -420, true, sat),
		ledgerCall("NVDA", domain.CallBuy, 100, true, sat.Add(-48*time.Hour)),
	}
	best, miss := domain.PickBest(calls), domain.PickMiss(calls)
	if best == nil || best.Symbol != "AMZN" || best.MoveBps != 420 {
		t.Fatalf("PickBest = %+v, want AMZN +420", best)
	}
	if miss == nil || miss.Symbol != "TSLA" || miss.MoveBps != -240 {
		t.Fatalf("PickMiss = %+v, want TSLA -240", miss)
	}
}

func TestPickBestAndMiss_TiesAndNone(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 10, 17, 15, 0, 0, 0, time.UTC)
	a := ledgerCall("A", domain.CallBuy, 100, true, at)
	b := ledgerCall("B", domain.CallBuy, 100, true, at.Add(time.Hour))
	if got := domain.PickBest([]domain.LedgerCall{a, b}); got.Symbol != "B" {
		t.Fatalf("PickBest tie = %s, want the later call B", got.Symbol)
	}
	if got := domain.PickBest([]domain.LedgerCall{b, a}); got.Symbol != "B" {
		t.Fatalf("PickBest tie reversed = %s, want B", got.Symbol)
	}
	same := b
	same.Symbol = "C"
	same.ProposalID = uuid.NewSHA1(uuid.Nil, []byte("C"))
	if got := domain.PickBest([]domain.LedgerCall{b, same}); got.ProposalID.String() < b.ProposalID.String() {
		t.Fatalf("PickBest same-time tie = %s, want the larger proposal id", got.ProposalID)
	}
	if domain.PickBest(nil) != nil || domain.PickMiss([]domain.LedgerCall{a}) != nil {
		t.Fatal("no right call or no wrong call must pick nothing")
	}
}

func TestPickSharpest_Week42(t *testing.T) {
	t.Parallel()
	user := func(name string, right, resolved int) domain.LedgerSharpest {
		return domain.LedgerSharpest{
			User:  domain.LedgerUser{ID: uuid.NewSHA1(uuid.Nil, []byte(name)), Name: name},
			Right: right, Resolved: resolved,
		}
	}
	got := domain.PickSharpest([]domain.LedgerSharpest{
		user("Andres", 2, 3), user("Christelle", 3, 3), user("Priya", 1, 1), user("Zed", 3, 3),
	})
	if got == nil || got.User.Name != "Christelle" || got.RateBps != 10_000 || got.Right != 3 {
		t.Fatalf("PickSharpest = %+v, want Christelle at 10000 (Priya has one resolved call, Zed ties by name)", got)
	}
	more := domain.PickSharpest([]domain.LedgerSharpest{user("Ann", 2, 2), user("Bo", 3, 3)})
	if more.User.Name != "Bo" {
		t.Fatalf("PickSharpest tie on rate = %s, want the one with more right calls", more.User.Name)
	}
	if domain.PickSharpest([]domain.LedgerSharpest{user("Priya", 1, 1)}) != nil {
		t.Fatal("one resolved call must not be sharpest")
	}
}

func TestPickMiss_TheLargestMoveAgainstWins(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 10, 17, 15, 0, 0, 0, time.UTC)
	calls := []domain.LedgerCall{
		ledgerCall("SMALL", domain.CallBuy, -100, false, at),
		ledgerCall("BIG", domain.CallBuy, -240, false, at),
	}
	if got := domain.PickMiss(calls); got == nil || got.Symbol != "BIG" || got.MoveBps != -240 {
		t.Fatalf("PickMiss = %+v, want the -240 call", got)
	}
}
