package adapters

import (
	"context"
	"strconv"
	"testing"

	"github.com/google/uuid"
	"pgregory.net/rapid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type holdersFake struct {
	rows  []sqlc.MemberSharesInCabalsOfRow
	units []sqlc.ShareUnitsByCabalRow
	err   error
}

func (f holdersFake) ShareUnitsByCabal(context.Context, uuid.UUID) ([]sqlc.ShareUnitsByCabalRow, error) {
	return f.units, f.err
}

func (f holdersFake) MemberSharesInCabalsOf(context.Context, uuid.UUID) ([]sqlc.MemberSharesInCabalsOfRow, error) {
	return f.rows, f.err
}

func holderRow(cabal ids.CabalID, user ids.UserID, units string) sqlc.MemberSharesInCabalsOfRow {
	return sqlc.MemberSharesInCabalsOfRow{CabalID: cabal.UUID(), UserID: user.UUID(), ShareUnits: units}
}

func TestOwnershipBps_splitsEachCabalOnItsOwnHolders(t *testing.T) {
	t.Parallel()
	g := testkit.NewIDs(7)
	me, low, high := ids.UserIDFrom(g.NewV7()), ids.UserIDFrom(uuid.UUID{0}), ids.UserIDFrom(uuid.UUID{255})
	even, uneven := ids.CabalIDFrom(g.NewV7()), ids.CabalIDFrom(g.NewV7())
	q := &Queries{holders: holdersFake{rows: []sqlc.MemberSharesInCabalsOfRow{
		holderRow(even, low, "1"), holderRow(even, me, "1"), holderRow(even, high, "1"),
		holderRow(uneven, low, "33333333"), holderRow(uneven, me, "35000000"), holderRow(uneven, high, "31666667"),
	}}}
	got, err := q.OwnershipBps(t.Context(), me)
	if err != nil || len(got) != 2 || got[even] != 3333 || got[uneven] != 3501 {
		t.Fatalf("OwnershipBps = %v, %v, want 3333 behind the lower tied id and 3501 with the remainder", got, err)
	}
}

func TestOwnershipBps_refusesBadReads(t *testing.T) {
	t.Parallel()
	me, cabal := potViewer(), ids.CabalIDFrom(testkit.NewIDs(5).NewV7())
	for name, tt := range map[string]struct {
		store holdersFake
		want  errs.Code
	}{
		"read fails": {store: holdersFake{err: errs.New(errs.CodeDBUnavailable, "test")}, want: errs.CodeDBUnavailable},
		"bad units": {
			store: holdersFake{rows: []sqlc.MemberSharesInCabalsOfRow{holderRow(cabal, me, "bad")}},
			want:  errs.CodeDecodeFailed,
		},
		"units overflow": {
			store: holdersFake{rows: []sqlc.MemberSharesInCabalsOfRow{
				holderRow(cabal, me, maxUint64), holderRow(cabal, ids.UserIDFrom(uuid.UUID{255}), maxUint64),
			}},
			want: errs.CodeInvalidInput,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := (&Queries{holders: tt.store}).OwnershipBps(t.Context(), me)
			if got != nil || errs.CodeOf(err) != tt.want {
				t.Fatalf("OwnershipBps = %v, %v, want nil and %s", got, err, tt.want)
			}
		})
	}
}

func TestOwnershipBps_isEachHoldersSlicesBpsEntry(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		g := testkit.NewIDs(rapid.Uint64().Draw(t, "seed"))
		cabal := ids.CabalIDFrom(g.NewV7())
		units := rapid.SliceOfN(rapid.Uint64Range(0, 1e15), 1, 8).Draw(t, "units")
		units[rapid.IntRange(0, len(units)-1).Draw(t, "holder")]++
		users := make([]ids.UserID, len(units))
		shares := make([]money.SharesUnits, len(units))
		rows := make([]sqlc.MemberSharesInCabalsOfRow, len(units))
		for i, u := range units {
			users[i], shares[i] = ids.UserIDFrom(g.NewV7()), money.SharesUnitsFromUint64(u)
			rows[i] = holderRow(cabal, users[i], strconv.FormatUint(u, 10))
		}
		want, err := domain.SlicesBps(shares)
		if err != nil {
			t.Fatal(err)
		}
		q := &Queries{holders: holdersFake{rows: rows}}
		var sum int32
		for i, user := range users {
			if units[i] == 0 {
				continue
			}
			got, err := q.OwnershipBps(t.Context(), user)
			if err != nil || got[cabal] != want[i] {
				t.Fatalf("holder %d OwnershipBps = %v, %v, want %d", i, got, err, want[i])
			}
			sum += got[cabal]
		}
		if sum != 10000 {
			t.Fatalf("holders sum to %d bps, want 10000", sum)
		}
	})
}

func TestOwnership_splitsTheCabalAndLeavesNonHoldersOut(t *testing.T) {
	t.Parallel()
	a, b, c := ids.UserIDFrom(uuid.UUID{1}), ids.UserIDFrom(uuid.UUID{2}), ids.UserIDFrom(uuid.UUID{3})
	q := &Queries{holders: holdersFake{units: []sqlc.ShareUnitsByCabalRow{
		{UserID: a.UUID(), ShareUnits: "1"}, {UserID: b.UUID(), ShareUnits: "1"}, {UserID: c.UUID(), ShareUnits: "1"},
	}}}
	got, err := q.Ownership(t.Context(), ids.CabalIDFrom(uuid.UUID{9}))
	var sum int32
	for _, bps := range got {
		sum += bps
	}
	if err != nil || len(got) != 3 || sum != 10000 || got[a] != 3334 || got[b] != 3333 {
		t.Fatalf("Ownership = %v, %v, want three holders summing to 10000", got, err)
	}
	if _, ok := got[ids.UserIDFrom(uuid.UUID{4})]; ok {
		t.Fatal("a member without a position must be absent")
	}
}

func TestOwnership_refusesBadReads(t *testing.T) {
	t.Parallel()
	cabal := ids.CabalIDFrom(uuid.UUID{9})
	big := sqlc.ShareUnitsByCabalRow{UserID: uuid.UUID{1}, ShareUnits: maxUint64}
	for name, tt := range map[string]struct {
		store holdersFake
		want  errs.Code
	}{
		"read fails": {store: holdersFake{err: errs.New(errs.CodeDBUnavailable, "test")}, want: errs.CodeDBUnavailable},
		"bad units": {
			store: holdersFake{units: []sqlc.ShareUnitsByCabalRow{{UserID: uuid.UUID{1}, ShareUnits: "bad"}}},
			want:  errs.CodeDecodeFailed,
		},
		"units overflow": {
			store: holdersFake{units: []sqlc.ShareUnitsByCabalRow{big, {UserID: uuid.UUID{2}, ShareUnits: maxUint64}}},
			want:  errs.CodeInvalidInput,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := (&Queries{holders: tt.store}).Ownership(t.Context(), cabal)
			if got != nil || errs.CodeOf(err) != tt.want {
				t.Fatalf("Ownership = %v, %v, want nil and %s", got, err, tt.want)
			}
		})
	}
}
