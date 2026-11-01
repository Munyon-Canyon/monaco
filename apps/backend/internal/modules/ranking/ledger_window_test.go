package ranking_test

import (
	"context"
	"math"
	"math/big"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func windowStart() time.Time { return ledgerDelivery().Add(-domain.LedgerWindow) }

func usdc(n int64) int64 { return n * 1_000_000 }

func micros(n int64) uint64 { return new(big.Int).SetInt64(usdc(n)).Uint64() }

type chainCabal struct {
	id         ids.CabalID
	start, end int64
}

type ledgerChain struct {
	app.Treasury
	owner            ids.UserID
	cabals           []chainCabal
	stakes0, stakes1 []treasury.MemberStake
	flows            []treasury.MemberFlow
	rows             []sqlc.SnapshotsAtRow
	history          []app.StakePoint
	curve            map[ids.CabalID][]domain.Snapshot
	fail             string
}

func stakeOf(owner ids.UserID, c ids.CabalID, shares uint64) treasury.MemberStake {
	return treasury.MemberStake{UserID: owner, CabalID: c, ShareUnits: money.SharesUnitsFromUint64(shares)}
}

func newChain(owner ids.UserID, cabals ...chainCabal) *ledgerChain {
	t0, t1 := windowStart(), ledgerDelivery()
	c := &ledgerChain{owner: owner, cabals: cabals, curve: map[ids.CabalID][]domain.Snapshot{}}
	for _, cabal := range cabals {
		c.stakes0 = append(c.stakes0, stakeOf(owner, cabal.id, 100))
		c.stakes1 = append(c.stakes1, stakeOf(owner, cabal.id, 100))
		c.rows = append(c.rows,
			sqlc.SnapshotsAtRow{
				Bucket: 0, CabalID: cabal.id.UUID(), At: t0, ValueMicros: usdc(cabal.start), TotalShares: 100,
			},
			sqlc.SnapshotsAtRow{
				Bucket: 1, CabalID: cabal.id.UUID(), At: t1, ValueMicros: usdc(cabal.end), TotalShares: 100,
			},
		)
		c.history = append(c.history, app.StakePoint{
			CabalID: cabal.id, At: t0.Add(-240 * time.Hour), ShareUnits: money.SharesUnitsFromUint64(100),
			NetContributed: money.SignedMicrosFromInt64(usdc(cabal.start)),
		})
		for h := -1; h <= 168; h++ {
			hours := new(big.Int).SetInt64(int64(max(h, 0))).Uint64()
			value := micros(cabal.start) + (micros(cabal.end)-micros(cabal.start))*hours/168
			c.curve[cabal.id] = append(c.curve[cabal.id], domain.Snapshot{
				At: t0.Add(time.Duration(h) * time.Hour), Value: money.MicrosFromUint64(value),
				TotalShares: money.SharesUnitsFromUint64(100),
			})
		}
	}
	return c
}

func (c *ledgerChain) fails(name string) error {
	if c.fail == name {
		return errs.New(errs.CodeInternal, "boom")
	}
	return nil
}

func (c *ledgerChain) MemberStakesAt(_ context.Context, at time.Time) ([]treasury.MemberStake, error) {
	if at.Equal(windowStart()) {
		return c.stakes0, c.fails("stakes0")
	}
	return c.stakes1, c.fails("stakes1")
}

func (c *ledgerChain) MemberFlowsBetween(context.Context, time.Time, time.Time) ([]treasury.MemberFlow, error) {
	return c.flows, c.fails("flows")
}

func (c *ledgerChain) SnapshotsAt(context.Context, sqlc.SnapshotsAtParams) ([]sqlc.SnapshotsAtRow, error) {
	return c.rows, c.fails("snaps")
}

func (c *ledgerChain) UserStakeHistory(context.Context, ids.UserID) ([]app.StakePoint, error) {
	return c.history, c.fails("history")
}

func (c *ledgerChain) SnapshotsOfCabals(
	context.Context, []ids.CabalID, time.Time, time.Time,
) (map[ids.CabalID][]domain.Snapshot, error) {
	return c.curve, c.fails("curve")
}

func (c *ledgerChain) wire(in app.LedgerInputs) app.LedgerInputs {
	in.Ports = app.Ports{Treasury: c, Snapshots: c}
	in.Stakes, in.Snaps, in.Window = c, c, nil
	return in
}

func fixtureChain(w week42) *ledgerChain {
	return newChain(
		w.andres,
		chainCabal{w.degens, 500, 500}, chainCabal{w.techTitans, 500, 508}, chainCabal{w.acCapital, 1000, 1020},
	)
}

func build(t *testing.T, w week42, c *ledgerChain) (domain.Ledger, bool, error) {
	t.Helper()
	return app.BuildLedger(t.Context(), c.wire(w.inputs), w.andres, ledgerDelivery())
}

func wantBps(t *testing.T, l domain.Ledger, want int64) {
	t.Helper()
	if l.StakeReturnBps == nil || *l.StakeReturnBps < want-1 || *l.StakeReturnBps > want+1 {
		t.Fatalf("stake_return_bps = %v, want %d", l.StakeReturnBps, want)
	}
}

func TestLedgerWindow_Fixture140BpsOrderedByEquityWith42Points(t *testing.T) {
	t.Parallel()
	w := seedWeek42(t)
	got, ok, err := build(t, w, fixtureChain(w))
	if err != nil || !ok {
		t.Fatalf("BuildLedger = %v, %v", ok, err)
	}
	wantBps(t, got, 140)
	names := make([]string, 0, len(got.Cabals))
	for _, c := range got.Cabals {
		names = append(names, c.Name)
	}
	if len(names) != 3 || names[0] != "AC Capital" || names[1] != "Tech Titans" || names[2] != "Degens" {
		t.Fatalf("cabals = %v, want AC Capital, Tech Titans, Degens", names)
	}
	assertSeries(t, got)
}

func assertSeries(t *testing.T, got domain.Ledger) {
	t.Helper()
	if len(got.Series) != 42 || !got.Series[0].At.Equal(windowStart().Add(4*time.Hour)) ||
		!got.Series[41].At.Equal(ledgerDelivery()) {
		t.Fatalf("series = %d points, want 42 from window_start+4h to window_end", len(got.Series))
	}
	if last := got.Series[41].ReturnBps; last < 139 || last > 141 {
		t.Fatalf("last series point = %d bps, want 140", last)
	}
}

func TestLedgerWindow_TiesOrderByNameAndOneCabalIsAOneElementList(t *testing.T) {
	t.Parallel()
	w := seedWeek42(t)
	tied := newChain(w.andres, chainCabal{w.techTitans, 500, 500}, chainCabal{w.acCapital, 500, 500})
	got, _, err := build(t, w, tied)
	if err != nil || len(got.Cabals) != 2 || got.Cabals[0].Name != "AC Capital" {
		t.Fatalf("tied cabals = %+v, %v, want AC Capital first by name", got.Cabals, err)
	}
	one := newChain(w.andres, chainCabal{w.degens, 500, 510})
	got, _, err = build(t, w, one)
	if err != nil || len(got.Cabals) != 1 || got.Cabals[0].Name != "Degens" {
		t.Fatalf("one cabal = %+v, %v", got.Cabals, err)
	}
	wantBps(t, got, 200)
}

func flowAt(c *ledgerChain, at time.Time, amount int64) {
	c.flows = append(c.flows, treasury.MemberFlow{
		UserID: c.owner, CabalID: c.cabals[0].id, Amount: money.SignedMicrosFromInt64(usdc(amount)), At: at,
	})
}

func TestLedgerWindow_ADepositAtWindowStartIsOutsideAndAtWindowEndInside(t *testing.T) {
	t.Parallel()
	w := seedWeek42(t)
	atStart := newChain(w.andres, chainCabal{w.degens, 1000, 1100})
	flowAt(atStart, windowStart(), 500)
	got, _, err := build(t, w, atStart)
	if err != nil {
		t.Fatal(err)
	}
	wantBps(t, got, 1000)
	atEnd := newChain(w.andres, chainCabal{w.degens, 1000, 1100})
	flowAt(atEnd, ledgerDelivery(), 500)
	got, _, err = build(t, w, atEnd)
	if err != nil {
		t.Fatal(err)
	}
	wantBps(t, got, -2667)
}

func TestLedgerWindow_JoinMidWeekHasAZeroStart(t *testing.T) {
	t.Parallel()
	w := seedWeek42(t)
	c := newChain(w.andres, chainCabal{w.degens, 1000, 1100})
	c.stakes0 = nil
	c.history[0].At = windowStart().Add(24 * time.Hour)
	flowAt(c, windowStart().Add(24*time.Hour), 1000)
	got, ok, err := build(t, w, c)
	if err != nil || !ok {
		t.Fatalf("BuildLedger = %v, %v", ok, err)
	}
	if got.StakeReturnBps == nil || *got.StakeReturnBps <= 0 || len(got.Cabals) != 1 {
		t.Fatalf("joiner = %v bps, %d cabals, want a positive return on one cabal", got.StakeReturnBps, len(got.Cabals))
	}
}

func TestLedgerWindow_ACashOutMidWeekHasAZeroEnd(t *testing.T) {
	t.Parallel()
	w := seedWeek42(t)
	c := newChain(w.andres, chainCabal{w.degens, 1000, 1100})
	c.stakes1 = nil
	flowAt(c, windowStart().Add(72*time.Hour), -1050)
	got, ok, err := build(t, w, c)
	if err != nil || !ok {
		t.Fatalf("BuildLedger = %v, %v", ok, err)
	}
	if got.StakeReturnBps == nil || *got.StakeReturnBps <= 0 || len(got.Cabals) != 1 {
		t.Fatalf("cash-out = %v bps, %d cabals, want a positive return", got.StakeReturnBps, len(got.Cabals))
	}
}

func TestLedgerWindow_UnusableSnapshotsGiveANullReturn(t *testing.T) {
	t.Parallel()
	stale := func(c *ledgerChain) {
		c.rows[1].At = ledgerDelivery().Add(-domain.MaxSnapshotGap(domain.Range1W) - time.Minute)
	}
	future := func(c *ledgerChain) { c.rows[1].At = ledgerDelivery().Add(time.Minute) }
	shares := func(c *ledgerChain) { c.rows[1].TotalShares = 99 }
	missingStart := func(c *ledgerChain) { c.rows = c.rows[1:] }
	overStake := func(c *ledgerChain) { c.stakes1[0] = stakeOf(c.owner, c.cabals[0].id, 101) }
	for name, mutate := range map[string]func(*ledgerChain){
		"stale end": stale, "future end": future, "total shares differ": shares, "no start": missingStart,
		"stake above total": overStake,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			w := seedWeek42(t)
			c := newChain(w.andres, chainCabal{w.degens, 1000, 1100})
			mutate(c)
			got, ok, err := build(t, w, c)
			if err != nil || !ok || got.StakeReturnBps != nil {
				t.Fatalf("BuildLedger = %v bps, %v, %v, want a Ledger with a null return", got.StakeReturnBps, ok, err)
			}
		})
	}
}

func TestLedgerWindow_MatchesTheOneWeekBoardPieces(t *testing.T) {
	t.Parallel()
	w := seedWeek42(t)
	c := fixtureChain(w)
	got, _, err := build(t, w, c)
	if err != nil {
		t.Fatal(err)
	}
	win := app.Window{Range: domain.Range1W, T0: windowStart(), T1: ledgerDelivery()}
	parts := make([]app.Ranged, 0, len(c.cabals))
	for i, cabal := range c.cabals {
		snap0 := &app.Snapshot{
			At: win.T0, Value: money.MicrosFromUint64(micros(cabal.start)),
			TotalShares: money.SharesUnitsFromUint64(100),
		}
		end := money.MicrosFromUint64(micros(cabal.end))
		part, err := app.MemberRanged(
			win, money.SharesUnitsFromUint64(100), c.stakes0[i].ShareUnits, snap0, end, nil,
		)
		if err != nil {
			t.Fatal(err)
		}
		parts = append(parts, part)
	}
	sum, err := app.SumRanged(win.T0, win.T1, parts)
	if err != nil {
		t.Fatal(err)
	}
	_, board, err := sum.Gain()
	if err != nil || board == nil || got.StakeReturnBps == nil || *got.StakeReturnBps != int64(*board) {
		t.Fatalf("ledger %v, board %v, %v, want the same return", got.StakeReturnBps, board, err)
	}
}

func TestLedgerWindow_ASharedWindowIsLoadedOnce(t *testing.T) {
	t.Parallel()
	w := seedWeek42(t)
	c := fixtureChain(w)
	in := c.wire(w.inputs)
	win, err := app.NewLedgerWindow(t.Context(), in.Ports, windowStart(), ledgerDelivery())
	if err != nil {
		t.Fatal(err)
	}
	in.Ports = app.Ports{}
	in.Window = win
	got, ok, err := app.BuildLedger(t.Context(), in, w.andres, ledgerDelivery())
	if err != nil || !ok {
		t.Fatalf("BuildLedger = %v, %v", ok, err)
	}
	wantBps(t, got, 140)
}

func TestLedgerWindow_NoStakeAndNoProposalGetsNoLedger(t *testing.T) {
	t.Parallel()
	w := seedWeek42(t)
	c := newChain(ids.UserIDFrom(idOf("stranger")))
	if _, ok, err := app.BuildLedger(
		t.Context(), c.wire(w.inputs), ids.UserIDFrom(idOf("stranger")), ledgerDelivery(),
	); err != nil || ok {
		t.Fatalf("BuildLedger = %v, %v, want no Ledger", ok, err)
	}
}

func TestLedgerWindow_ANoStakeMemberWithNoHistoryHasAnEmptySeries(t *testing.T) {
	t.Parallel()
	w := seedWeek42(t)
	c := fixtureChain(w)
	c.history = nil
	got, ok, err := build(t, w, c)
	if err != nil || !ok || len(got.Series) != 0 {
		t.Fatalf("BuildLedger = %d points, %v, %v, want an empty series", len(got.Series), ok, err)
	}
}

func TestLedgerWindow_FailuresSurface(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"stakes0", "stakes1", "snaps", "flows", "history", "curve"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			w := seedWeek42(t)
			c := fixtureChain(w)
			c.fail = name
			if _, ok, err := build(t, w, c); err == nil || ok {
				t.Fatalf("BuildLedger = %v, %v, want the %s failure", ok, err, name)
			}
		})
	}
}

func TestLedgerWindow_MalformedAndOverflowingInputsSurface(t *testing.T) {
	t.Parallel()
	huge := int64(1<<63 - 1)
	for name, mutate := range map[string]func(*ledgerChain){
		"negative snapshot": func(c *ledgerChain) { c.rows[0].ValueMicros = -1 },
		"shares overflow": func(c *ledgerChain) {
			c.stakes0 = append(c.stakes0, treasury.MemberStake{
				UserID: c.owner, CabalID: c.cabals[0].id, ShareUnits: money.SharesUnitsFromUint64(1<<64 - 1),
			}, treasury.MemberStake{
				UserID: ids.UserIDFrom(idOf("other")), CabalID: c.cabals[0].id,
				ShareUnits: money.SharesUnitsFromUint64(2),
			})
		},
		"start sum overflow": func(c *ledgerChain) {
			for i := range c.rows {
				c.rows[i].ValueMicros = huge
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			w := seedWeek42(t)
			c := newChain(
				w.andres,
				chainCabal{w.degens, 1, 1}, chainCabal{w.techTitans, 1, 1}, chainCabal{w.acCapital, 1, 1},
			)
			mutate(c)
			if _, ok, err := build(t, w, c); err == nil || ok {
				t.Fatalf("BuildLedger = %v, %v, want an error", ok, err)
			}
		})
	}
}

func TestLedgerWindow_ACurveThatOverflowsSurfaces(t *testing.T) {
	t.Parallel()
	w := seedWeek42(t)
	c := fixtureChain(w)
	for id, snaps := range c.curve {
		for i := range snaps {
			snaps[i].Value = money.MicrosFromUint64(1<<63 - 1)
		}
		c.curve[id] = snaps
	}
	if _, ok, err := build(t, w, c); err == nil || ok {
		t.Fatalf("BuildLedger = %v, %v, want the curve overflow", ok, err)
	}
}

func TestLedgerWindow_AStakeWithNoFlowsAndNoStartHasANullReturn(t *testing.T) {
	t.Parallel()
	w := seedWeek42(t)
	c := newChain(w.andres, chainCabal{w.degens, 1000, 1100})
	c.stakes0 = nil
	got, ok, err := build(t, w, c)
	if err != nil || !ok || got.StakeReturnBps != nil {
		t.Fatalf("BuildLedger = %v bps, %v, %v, want a null return", got.StakeReturnBps, ok, err)
	}
}

func TestLedgerWindow_ADegenerateWindowSurfaces(t *testing.T) {
	t.Parallel()
	w := seedWeek42(t)
	c := fixtureChain(w)
	for i := range c.rows {
		c.rows[i].At = ledgerDelivery()
	}
	in := c.wire(w.inputs)
	win, err := app.NewLedgerWindow(t.Context(), in.Ports, ledgerDelivery(), ledgerDelivery())
	if err != nil {
		t.Fatal(err)
	}
	in.Window = win
	if _, ok, err := app.BuildLedger(t.Context(), in, w.andres, ledgerDelivery()); err == nil || ok {
		t.Fatalf("BuildLedger = %v, %v, want the window failure", ok, err)
	}
}

func TestLedgerWindow_AGainTooLargeForSignedMicrosSurfaces(t *testing.T) {
	t.Parallel()
	w := seedWeek42(t)
	c := newChain(w.andres, chainCabal{w.degens, 1000, 1100})
	snaps := c.curve[w.degens]
	for i := range snaps {
		value := uint64(1)
		if snaps[i].At.After(windowStart().Add(time.Hour)) {
			value = math.MaxUint64
		}
		snaps[i].Value = money.MicrosFromUint64(value)
	}
	if _, ok, err := build(t, w, c); err == nil || ok {
		t.Fatalf("BuildLedger = %v, %v, want the gain overflow", ok, err)
	}
}

func TestLedgerWindow_AMidWeekGainTooLargeForSignedMicrosSurfaces(t *testing.T) {
	t.Parallel()
	w := seedWeek42(t)
	c := newChain(w.andres, chainCabal{w.degens, 1000, 1100})
	snaps := c.curve[w.degens]
	for i := range snaps {
		if snaps[i].At.Equal(windowStart().Add(8 * time.Hour)) {
			snaps[i].Value = money.MicrosFromUint64(math.MaxUint64)
		}
	}
	if _, ok, err := build(t, w, c); err == nil || ok {
		t.Fatalf("BuildLedger = %v, %v, want the series point's gain overflow", ok, err)
	}
}
