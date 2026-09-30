package domain_test

import (
	"slices"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
)

func TestRank_ReturnDescending(t *testing.T) {
	t.Parallel()
	now := clock.Real{}.Now().UTC()
	flags := []domain.Flag{domain.FlagStalePrices}
	rows := domain.Rank([]domain.Candidate{
		{SubjectID: "low", CreatedAt: now, Return: bps(-50)},
		{SubjectID: "high", CreatedAt: now, Return: bps(900), Value: usd(7), PnL: signed(3), Flags: flags},
		{SubjectID: "mid", CreatedAt: now, Return: bps(0)},
	}, false)
	wantRanks(t, rows, "high", "mid", "low")
	if rows[0].Value != usd(7) || rows[0].PnL != signed(3) || !slices.Equal(rows[0].Flags, flags) {
		t.Fatalf("row 1 = %+v, want the candidate's value, pnl and flags", rows[0])
	}
}

func TestRank_TiesBreakByCreatedAtThenID(t *testing.T) {
	t.Parallel()
	now := clock.Real{}.Now().UTC()
	rows := domain.Rank([]domain.Candidate{
		{SubjectID: "b", CreatedAt: now, Return: bps(100)},
		{SubjectID: "newer", CreatedAt: now.Add(time.Second), Return: bps(100)},
		{SubjectID: "a", CreatedAt: now, Return: bps(100)},
		{SubjectID: "older", CreatedAt: now.Add(-time.Second), Return: bps(100)},
	}, false)
	wantRanks(t, rows, "older", "a", "b", "newer")
}

func TestRank_KeepUnranked(t *testing.T) {
	t.Parallel()
	now := clock.Real{}.Now().UTC()
	cands := []domain.Candidate{
		{SubjectID: "no-deposit-late", CreatedAt: now.Add(time.Second)},
		{SubjectID: "loser", CreatedAt: now, Return: bps(-100)},
		{SubjectID: "no-deposit-b", CreatedAt: now},
		{SubjectID: "winner", CreatedAt: now, Return: bps(100)},
		{SubjectID: "no-deposit-a", CreatedAt: now},
	}
	wantRanks(t, domain.Rank(cands, true), "winner", "loser", "no-deposit-a", "no-deposit-b", "no-deposit-late")
	wantRanks(t, domain.Rank(cands, false), "winner", "loser")
}

func TestRank_Empty(t *testing.T) {
	t.Parallel()
	if rows := domain.Rank(nil, true); len(rows) != 0 {
		t.Fatalf("Rank(nil) = %+v, want no rows", rows)
	}
}

func wantRanks(t *testing.T, rows []domain.Row, ids ...string) {
	t.Helper()
	got := make([]string, len(rows))
	for i, r := range rows {
		got[i] = r.SubjectID
		if r.Rank != i+1 {
			t.Fatalf("row %d has rank %d, want %d", i, r.Rank, i+1)
		}
	}
	if !slices.Equal(got, ids) {
		t.Fatalf("order = %v, want %v", got, ids)
	}
}
