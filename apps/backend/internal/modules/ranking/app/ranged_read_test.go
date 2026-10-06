package app

import (
	"math"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func readRangedWith(t *testing.T, f *countingPorts) ([]rangedBook, error) {
	t.Helper()
	return NewRunValuation(Ports{Treasury: f, Snapshots: f}, "").readRanged(t.Context(), valuationTime())
}

func TestReadRanged_StartsEveryWindowFromTheRangeAndGroupsTheReads(t *testing.T) {
	t.Parallel()
	at := valuationTime()
	user, cabal := ids.UserIDFrom(ids.Real{}.NewV7()), ids.CabalIDFrom(ids.Real{}.NewV7())
	inDay := at.Add(-time.Hour)
	f := &countingPorts{
		stakeRows: []treasury.MemberStake{{UserID: user, CabalID: cabal, ShareUnits: money.SharesUnitsFromUint64(4)}},
		flowRows: []treasury.MemberFlow{
			{UserID: user, CabalID: cabal, Amount: money.SignedMicrosFromInt64(5), At: inDay},
			{UserID: user, CabalID: cabal, Amount: money.SignedMicrosFromInt64(7), At: at.Add(-3 * 24 * time.Hour)},
		},
		snapshotRows: []sqlc.SnapshotsAtRow{
			{Bucket: 1, CabalID: cabal.UUID(), At: at.Add(-25 * time.Hour), ValueMicros: 90, TotalShares: 9},
		},
	}
	books, err := readRangedWith(t, f)
	if err != nil || len(books) != len(rangedRanges()) {
		t.Fatalf("readRanged = %v, %v, want one book per ranged range", books, err)
	}
	if f.stakes != len(rangedRanges()) || f.flowCalls != 1 || f.snapshotCalls != 1 {
		t.Fatalf("calls = %d stakes, %d flows, %d snapshots, want one stakes read per range and one of the rest",
			f.stakes, f.flowCalls, f.snapshotCalls)
	}
	for i, book := range books {
		wantStart, _ := rangedRanges()[i].Start(at)
		if book.Range != rangedRanges()[i] || !book.T0.Equal(wantStart) || !book.T1.Equal(at) {
			t.Fatalf("book %d window = %+v, want %s from %v", i, book.Window, rangedRanges()[i], wantStart)
		}
		if book.shares[cabal] != money.SharesUnitsFromUint64(4) {
			t.Fatalf("book %d shares = %v, want the cabal's total at t0", i, book.shares[cabal])
		}
	}
	wantFlowsAndSnapshots(t, books, user, cabal)
}

func wantFlowsAndSnapshots(t *testing.T, books []rangedBook, user ids.UserID, cabal ids.CabalID) {
	t.Helper()
	key := MemberKey{UserID: user, CabalID: cabal}
	if len(books[0].flows[key]) != 0 || len(books[1].flows[key]) != 1 || len(books[2].flows[key]) != 2 {
		t.Fatalf("flows per range = %d, %d, %d, want 0, 1, 2",
			len(books[0].flows[key]), len(books[1].flows[key]), len(books[2].flows[key]))
	}
	snap := books[1].snaps[cabal]
	if snap == nil || snap.Value != money.MicrosFromUint64(90) || snap.TotalShares != money.SharesUnitsFromUint64(9) {
		t.Fatalf("1D snapshot = %+v, want the row for bucket 1", snap)
	}
	if books[0].snaps[cabal] != nil {
		t.Fatalf("1H snapshot = %+v, want none for a cabal with no row", books[0].snaps[cabal])
	}
}

func TestReadRanged_RefusesRowsItCannotDecode(t *testing.T) {
	t.Parallel()
	cabal := ids.CabalIDFrom(ids.Real{}.NewV7())
	huge := money.SharesUnitsFromUint64(math.MaxUint64)
	for name, setup := range map[string]func(*countingPorts){
		"bucket past the ranges": func(f *countingPorts) {
			f.snapshotRows = []sqlc.SnapshotsAtRow{{Bucket: 4}}
		},
		"negative bucket": func(f *countingPorts) { f.snapshotRows = []sqlc.SnapshotsAtRow{{Bucket: -1}} },
		"negative value":  func(f *countingPorts) { f.snapshotRows = []sqlc.SnapshotsAtRow{{ValueMicros: -1}} },
		"negative shares": func(f *countingPorts) { f.snapshotRows = []sqlc.SnapshotsAtRow{{TotalShares: -1}} },
		"shares past uint64": func(f *countingPorts) {
			f.stakeRows = []treasury.MemberStake{
				{CabalID: cabal, ShareUnits: huge}, {CabalID: cabal, ShareUnits: money.SharesUnitsFromUint64(1)},
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := &countingPorts{}
			setup(f)
			if _, err := readRangedWith(t, f); err == nil {
				t.Fatal("readRanged error = nil")
			}
		})
	}
}

func TestReadRanged_ReturnsPortErrors(t *testing.T) {
	t.Parallel()
	boom := errs.New(errs.CodeInternal, "test")
	for name, f := range map[string]*countingPorts{
		"stakes":    {stakesErr: boom},
		"flows":     {flowsErr: boom},
		"snapshots": {snapshotsErr: boom},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := readRangedWith(t, f); err == nil {
				t.Fatal("readRanged error = nil")
			}
		})
	}
}

func TestPreviousRows_SplitsFlaggedRowsFromTheFallbackForValuedCabals(t *testing.T) {
	t.Parallel()
	flagged, valued := ids.CabalIDFrom(ids.Real{}.NewV7()), ids.CabalIDFrom(ids.Real{}.NewV7())
	f := &countingPorts{previousRows: []sqlc.LeaderboardEntry{
		{Board: "cabals", Range: "ALL", SubjectID: flagged.UUID()},
		{Board: "cabals", Range: "1D", SubjectID: flagged.UUID()},
		{Board: MembersBoard(flagged.UUID()), Range: "1D", SubjectID: valued.UUID()},
		{Board: "cabals", Range: "1D", SubjectID: valued.UUID()},
		{Board: MembersBoard(valued.UUID()), Range: "1D", SubjectID: flagged.UUID()},
	}}
	previous, fallback, err := NewRunValuation(Ports{Previous: f}, "").previousRows(
		t.Context(), []ids.CabalID{flagged}, []CabalValue{{CabalID: valued}},
	)
	if err != nil || len(previous) != 3 || len(fallback) != 1 {
		t.Fatalf("previousRows = %d kept, %d fallback, %v, want the flagged cabal's three rows and one fallback row",
			len(previous), len(fallback), err)
	}
	if _, ok := fallback[fallbackKey{cabal: valued.UUID(), rng: "1D"}]; !ok {
		t.Fatalf("fallback = %+v, want the valued cabal's 1D row", fallback)
	}
	f.previousErr = errs.New(errs.CodeInternal, "test")
	if _, _, err := NewRunValuation(Ports{Previous: f}, "").previousRows(t.Context(), nil, nil); err != nil {
		t.Fatalf("previousRows with no cabals = %v, want no read", err)
	}
	if _, _, err := NewRunValuation(Ports{Previous: f}, "").previousRows(
		t.Context(), []ids.CabalID{flagged}, nil,
	); err == nil {
		t.Fatal("previousRows error = nil, want the read error")
	}
}
