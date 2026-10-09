package adapters

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"pgregory.net/rapid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

const maxUint64 = "18446744073709551615"

type potFake struct {
	snapshot    []sqlc.CabalPotSnapshotRow
	members     []sqlc.CabalMemberSharesRow
	snapshotErr error
	membersErr  error
}

func (f potFake) CabalPotSnapshot(context.Context, uuid.UUID) ([]sqlc.CabalPotSnapshotRow, error) {
	return f.snapshot, f.snapshotErr
}

func (f potFake) CabalMemberShares(context.Context, uuid.UUID) ([]sqlc.CabalMemberSharesRow, error) {
	return f.members, f.membersErr
}

func potViewer() ids.UserID { return ids.UserIDFrom(testkit.NewIDs(3).NewV7()) }

func potRow(asset, units, cost string) sqlc.CabalPotSnapshotRow {
	return sqlc.CabalPotSnapshotRow{
		TotalShares: "10", NetContributedMicros: "0", CashOutReservedMicros: "0",
		Asset: pgtype.Text{String: asset, Valid: asset != ""}, Units: units, CostBasisMicros: cost,
	}
}

func member(units, contributed, withdrawn string) sqlc.CabalMemberSharesRow {
	return sqlc.CabalMemberSharesRow{
		UserID: potViewer().UUID(), ShareUnits: units, ContributedMicros: contributed, WithdrawnMicros: withdrawn,
	}
}

func potQueries(store potFake, pricesErr error) *Queries {
	aapl := marketfake.AAPLx()
	now := time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)
	return &Queries{
		pot: store, usdc: usdcMint, clock: testkit.NewClock(now),
		catalog: app.MintResolver(func(context.Context, chain.SolanaAddress) (app.Asset, error) {
			return app.Asset{ID: aapl.ID.UUID(), Symbol: aapl.Symbol, Decimals: aapl.Decimals, ChainChecked: true}, nil
		}),
		prices: app.PriceReader(func(context.Context) (map[uuid.UUID]app.Price, error) {
			return map[uuid.UUID]app.Price{
					aapl.ID.UUID(): {Micros: money.MicrosFromUint64(100_000_000), ObservedAt: now},
				},
				pricesErr
		}),
	}
}

func TestCabalPot_refusesBadReadsAndOverflows(t *testing.T) {
	t.Parallel()
	aapl := marketfake.AAPLx().Mint.String()
	withRow := func(edit func(*sqlc.CabalPotSnapshotRow), rows ...sqlc.CabalPotSnapshotRow) []sqlc.CabalPotSnapshotRow {
		edit(&rows[0])
		return rows
	}
	keep := func(*sqlc.CabalPotSnapshotRow) {}
	usdc := potRow(usdcMint, "100", "100")
	cases := map[string]struct {
		store     potFake
		pricesErr error
		member    bool
	}{
		"snapshot read fails": {store: potFake{snapshotErr: errs.New(errs.CodeInternal, "test")}},
		"bad asset":           {store: potFake{snapshot: []sqlc.CabalPotSnapshotRow{potRow("bad", "1", "1")}}},
		"price read fails": {
			store:     potFake{snapshot: []sqlc.CabalPotSnapshotRow{potRow(aapl, "1", "1")}},
			pricesErr: errs.New(errs.CodeInternal, "test"),
		},
		"holding pnl overflows": {store: potFake{snapshot: []sqlc.CabalPotSnapshotRow{potRow(aapl, "1", maxUint64)}}},
		"bad reservation": {
			store: potFake{
				snapshot: withRow(func(r *sqlc.CabalPotSnapshotRow) { r.CashOutReservedMicros = "bad" }, usdc),
			},
		},
		"pot overflows": {
			store: potFake{
				snapshot: []sqlc.CabalPotSnapshotRow{potRow(usdcMint, maxUint64, "0"), potRow(aapl, "100000000", "0")},
			},
		},
		"bad net": {
			store: potFake{
				snapshot: withRow(func(r *sqlc.CabalPotSnapshotRow) { r.NetContributedMicros = "bad" }, usdc),
			},
		},
		"pot pnl overflows": {
			store: potFake{snapshot: []sqlc.CabalPotSnapshotRow{potRow(usdcMint, maxUint64, "0")}},
		},
		"bad total shares": {
			store:  potFake{snapshot: withRow(func(r *sqlc.CabalPotSnapshotRow) { r.TotalShares = "bad" }, usdc)},
			member: true,
		},
		"member read fails": {
			store:  potFake{snapshot: withRow(keep, usdc), membersErr: errs.New(errs.CodeInternal, "test")},
			member: true,
		},
		"bad member units": {
			store: potFake{
				snapshot: withRow(keep, usdc),
				members:  []sqlc.CabalMemberSharesRow{member("bad", "0", "0")},
			},
			member: true,
		},
		"member units overflow": {
			store: potFake{
				snapshot: withRow(keep, usdc),
				members:  []sqlc.CabalMemberSharesRow{member(maxUint64, "0", "0"), member("1", "0", "0")},
			},
			member: true,
		},
		"bad contributed": {
			store: potFake{
				snapshot: withRow(keep, usdc),
				members:  []sqlc.CabalMemberSharesRow{member("1", "bad", "0")},
			},
			member: true,
		},
		"bad withdrawn": {
			store: potFake{
				snapshot: withRow(keep, usdc),
				members:  []sqlc.CabalMemberSharesRow{member("1", "0", "bad")},
			},
			member: true,
		},
		"zero total shares": {
			store: potFake{
				snapshot: withRow(func(r *sqlc.CabalPotSnapshotRow) { r.TotalShares = "0" }, usdc),
				members:  []sqlc.CabalMemberSharesRow{member("1", "0", "0")},
			},
			member: true,
		},
		"member net overflows": {
			store: potFake{
				snapshot: withRow(keep, usdc),
				members:  []sqlc.CabalMemberSharesRow{member("1", maxUint64, "0")},
			},
			member: true,
		},
		"member pnl overflows": {
			store: potFake{
				snapshot: []sqlc.CabalPotSnapshotRow{potRow(usdcMint, "9223372036854775807", "0")},
				members:  []sqlc.CabalMemberSharesRow{member("10", "0", "1")},
			},
			member: true,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := potQueries(tc.store, tc.pricesErr).CabalPot(t.Context(), ids.CabalID{}, potViewer(), tc.member)
			if err == nil {
				t.Fatal("CabalPot error = nil")
			}
		})
	}
}

func waitingCashOut(cash, held, reserved uint64) *Queries {
	aapl := marketfake.AAPLx().Mint.String()
	owed := strconv.FormatUint(reserved, 10)
	snapshot := []sqlc.CabalPotSnapshotRow{
		potRow(aapl, strconv.FormatUint(held, 10), "0"), potRow(usdcMint, strconv.FormatUint(cash, 10), "0"),
	}
	positions := make([]sqlc.CabalPositionsRow, len(snapshot))
	for i := range snapshot {
		snapshot[i].NetContributedMicros, snapshot[i].CashOutReservedMicros = "100000000", owed
		positions[i] = sqlc.CabalPositionsRow{
			Asset:                 snapshot[i].Asset.String,
			Units:                 snapshot[i].Units,
			CostBasisMicros:       "0",
			CashOutReservedMicros: owed,
		}
	}
	members := []sqlc.CabalMemberSharesRow{member("10", "100000000", "0")}
	q := potQueries(potFake{snapshot: snapshot, members: members}, nil)
	q.q = stakeStore{positions: positions}
	return q
}

func TestCabalPot_aCashOutWaitingOnItsSaleTakesTheCashThenThePot(t *testing.T) {
	t.Parallel()
	type view struct {
		cash, value, me     money.Micros
		cashBps, holdingBps int32
		pnl, mePnL          int64
		returnBps           int32
	}
	q := waitingCashOut(10_000_000, 90_000_000, 50_000_000)
	pot, err := q.CabalPot(t.Context(), ids.CabalID{}, potViewer(), true)
	if err != nil {
		t.Fatalf("CabalPot() error = %v, want the pot while the sale runs", err)
	}
	got := view{
		pot.CashMicros, pot.PotValueMicros, pot.Me.ValueMicros, pot.CashWeightBps, pot.Holdings[0].WeightBps,
		pot.PnLMicros.Int64(), pot.Me.PnLMicros.Int64(), *pot.ReturnBps,
	}
	fifty := money.MicrosFromUint64(50_000_000)
	if want := (view{money.Micros{}, fifty, fifty, 0, 10000, -50_000_000, -50_000_000, -5000}); got != want {
		t.Fatalf("pot = %+v, want %+v: no cash, and the 100 USDC pot less the 50 USDC payout", got, want)
	}
	if value, err := q.PotValue(t.Context(), ids.CabalID{}); err != nil || value != pot.PotValueMicros {
		t.Fatalf("PotValue() = %v, %v, want CabalPot's %v", value, err, pot.PotValueMicros)
	}
}

func TestCabalPot_agreesWithPotValueWhileCashOutsWait(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	rapid.Check(t, func(t *rapid.T) {
		cash := rapid.Uint64Range(1, 1e12).Draw(t, "cash")
		held := rapid.Uint64Range(1, 1e12).Draw(t, "held")
		reserved := rapid.Uint64Range(0, 3e12).Draw(t, "reserved")
		q := waitingCashOut(cash, held, reserved)
		pot, err := q.CabalPot(ctx, ids.CabalID{}, potViewer(), true)
		value, valueErr := q.PotValue(ctx, ids.CabalID{})
		if reserved > cash+held {
			if errs.CodeOf(err) != errs.CodePotValueChanged || errs.CodeOf(valueErr) != errs.CodePotValueChanged {
				t.Fatalf("CabalPot() = %v, PotValue() = %v, want pot_value_changed from both", err, valueErr)
			}
			return
		}
		if err != nil || valueErr != nil || pot.PotValueMicros != value || value.Uint64() != cash+held-reserved {
			t.Fatalf("CabalPot() = %v, %v, PotValue() = %v, %v, want both at %d",
				pot.PotValueMicros, err, value, valueErr, cash+held-reserved)
		}
		if pot.CashMicros.Uint64() != cash-min(cash, reserved) || pot.Me.ValueMicros != value {
			t.Fatalf("cash = %v, me = %v, want the cash less what it pays and the sole member on the pot",
				pot.CashMicros, pot.Me.ValueMicros)
		}
		if pot.CashWeightBps+pot.Holdings[0].WeightBps != 10000 {
			t.Fatalf("weights = %d + %d, want 10000", pot.CashWeightBps, pot.Holdings[0].WeightBps)
		}
	})
}

func TestCabalPot_anEmptyPotHasZeroWeightsAndNoReturn(t *testing.T) {
	t.Parallel()
	pot, err := potQueries(potFake{snapshot: []sqlc.CabalPotSnapshotRow{potRow("", "0", "0")}}, nil).
		CabalPot(t.Context(), ids.CabalID{}, potViewer(), false)
	if err != nil || !pot.PotValueMicros.IsZero() || pot.CashWeightBps != 0 || pot.ReturnBps != nil || pot.Me != nil {
		t.Fatalf("CabalPot() = %#v, %v", pot, err)
	}
}

func TestDisplayUnits_roundsDownAtTheUIMultiplier(t *testing.T) {
	t.Parallel()
	cases := []struct {
		units    uint64
		num, den int64
		want     string
	}{
		{73_000_000, 1, 1, "0.7300"},
		{123_456_789, 1, 1, "1.2345"},
		{100_000_000, 3, 2, "1.5000"},
		{0, 1, 1, "0.0000"},
	}
	for _, c := range cases {
		if got := displayUnits(money.NewBaseUnits(c.units, 8), c.num, c.den); got != c.want {
			t.Errorf("displayUnits(%d, %d/%d) = %q, want %q", c.units, c.num, c.den, got, c.want)
		}
	}
}

func TestWirePot_refusesMicrosPastInt64(t *testing.T) {
	t.Parallel()
	if _, err := wirePot(port.CabalPot{PotValueMicros: money.MicrosFromUint64(1 << 63)}); err == nil {
		t.Fatal("wirePot error = nil")
	}
}
