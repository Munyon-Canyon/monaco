package fakes_test

import (
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

func TestTreasury_Positions(t *testing.T) {
	t.Parallel()
	w := newTreasuryWorld()
	positions, err := w.fake.Positions(t.Context(), w.cabalID)
	if err != nil || !reflect.DeepEqual(positions, []treasury.Position{w.position}) {
		t.Fatalf("Positions = (%+v, %v), want (%+v, nil)", positions, err, []treasury.Position{w.position})
	}
}

func TestTreasury_PotAndShares(t *testing.T) {
	t.Parallel()
	w := newTreasuryWorld()
	if got, err := w.fake.PotValue(t.Context(), w.cabalID); err != nil || got != w.stake.ValueMicros {
		t.Fatalf("PotValue = (%v, %v), want (%v, nil)", got, err, w.stake.ValueMicros)
	}
	if got, err := w.fake.TotalShares(t.Context(), w.cabalID); err != nil || got != w.stake.TotalShares {
		t.Fatalf("TotalShares = (%v, %v), want (%v, nil)", got, err, w.stake.TotalShares)
	}
}

func TestTreasury_Stakes(t *testing.T) {
	t.Parallel()
	w := newTreasuryWorld()
	if got, err := w.fake.ShareUnits(t.Context(), w.cabalID, w.userID); err != nil || got != w.stake.ShareUnits {
		t.Fatalf("ShareUnits = (%v, %v), want (%v, nil)", got, err, w.stake.ShareUnits)
	}
	if got, err := w.fake.Stake(t.Context(), w.cabalID, w.userID); err != nil || got != w.stake {
		t.Fatalf("Stake = (%+v, %v), want (%+v, nil)", got, err, w.stake)
	}
	got, err := w.fake.StakesOf(t.Context(), w.userID)
	if err != nil || !reflect.DeepEqual(got, []treasury.Stake{w.stake}) {
		t.Fatalf("StakesOf = (%+v, %v), want (%+v, nil)", got, err, []treasury.Stake{w.stake})
	}
	units, err := w.fake.ShareUnitsAt(t.Context(), w.cabalID, w.userID, time.Time{})
	if err != nil || units != w.stake.ShareUnits {
		t.Fatalf("ShareUnitsAt = (%v, %v), want (%v, nil)", units, err, w.stake.ShareUnits)
	}
}

func TestTreasury_Snapshots(t *testing.T) {
	t.Parallel()
	w := newTreasuryWorld()
	positions, err := w.fake.CabalPositionsAt(t.Context(), time.Time{})
	if err != nil || !reflect.DeepEqual(positions, w.cabalPositions) {
		t.Fatalf("CabalPositionsAt = (%+v, %v), want (%+v, nil)", positions, err, w.cabalPositions)
	}
	stakes, err := w.fake.MemberStakesAt(t.Context(), time.Time{})
	if err != nil || !reflect.DeepEqual(stakes, w.memberStakes) {
		t.Fatalf("MemberStakesAt = (%+v, %v), want (%+v, nil)", stakes, err, w.memberStakes)
	}
}

func TestTreasury_Unknown(t *testing.T) {
	t.Parallel()
	w := newTreasuryWorld()
	if positions, err := w.fake.Positions(t.Context(), w.otherCabalID); err != nil || len(positions) != 0 {
		t.Fatalf("unknown Positions = (%+v, %v), want (empty, nil)", positions, err)
	}
	if got, err := w.fake.Stake(t.Context(), w.cabalID, w.otherUserID); err != nil || got != (treasury.Stake{}) {
		t.Fatalf("unknown Stake = (%+v, %v), want (zero, nil)", got, err)
	}
}

type treasuryWorld struct {
	fake           *fakes.Treasury
	cabalID        ids.CabalID
	otherCabalID   ids.CabalID
	userID         ids.UserID
	otherUserID    ids.UserID
	position       treasury.Position
	stake          treasury.Stake
	cabalPositions []treasury.CabalPositions
	memberStakes   []treasury.MemberStake
}

func newTreasuryWorld() treasuryWorld {
	fake := fakes.NewTreasury()
	cabalID := ids.CabalIDFrom(ids.Real{}.NewV7())
	otherCabalID := ids.CabalIDFrom(ids.Real{}.NewV7())
	userID := ids.UserIDFrom(ids.Real{}.NewV7())
	otherUserID := ids.UserIDFrom(ids.Real{}.NewV7())
	position := treasury.Position{
		Mint: chain.SystemProgram, Units: money.NewBaseUnits(12, 6), CostBasis: money.MicrosFromUint64(34),
	}
	stake := treasury.Stake{
		CabalID: cabalID, UserID: userID, ShareUnits: money.SharesUnitsFromUint64(5),
		TotalShares: money.SharesUnitsFromUint64(8), ValueMicros: money.MicrosFromUint64(9),
		ContributedMicros: money.MicrosFromUint64(10), WithdrawnMicros: money.MicrosFromUint64(1),
	}
	cabalPositions := []treasury.CabalPositions{{
		CabalID: cabalID, Holdings: []treasury.Position{position}, TotalShares: stake.TotalShares,
	}}
	memberStakes := []treasury.MemberStake{{
		UserID: userID, CabalID: cabalID, ShareUnits: stake.ShareUnits,
		NetContributedMicros: money.SignedMicrosFromInt64(9),
	}}
	fake.SetPositions(cabalID, []treasury.Position{position})
	fake.SetPotValue(cabalID, stake.ValueMicros)
	fake.SetTotalShares(cabalID, stake.TotalShares)
	fake.SetStake(stake)
	fake.SetStake(stake)
	fake.SetStake(treasury.Stake{CabalID: otherCabalID, UserID: userID})
	fake.SetCabalPositionsAt(cabalPositions)
	fake.SetMemberStakesAt(memberStakes)
	return treasuryWorld{
		fake: fake, cabalID: cabalID, otherCabalID: otherCabalID, userID: userID, otherUserID: otherUserID,
		position: position, stake: stake, cabalPositions: cabalPositions, memberStakes: memberStakes,
	}
}

func TestTreasury_Faults(t *testing.T) {
	t.Parallel()
	fake := fakes.NewTreasury()
	cabalID := ids.CabalIDFrom(ids.Real{}.NewV7())
	userID := ids.UserIDFrom(ids.Real{}.NewV7())
	calls := map[string]func() error{
		"Positions":        func() error { _, err := fake.Positions(t.Context(), cabalID); return err },
		"PotValue":         func() error { _, err := fake.PotValue(t.Context(), cabalID); return err },
		"TotalShares":      func() error { _, err := fake.TotalShares(t.Context(), cabalID); return err },
		"ShareUnits":       func() error { _, err := fake.ShareUnits(t.Context(), cabalID, userID); return err },
		"Stake":            func() error { _, err := fake.Stake(t.Context(), cabalID, userID); return err },
		"StakesOf":         func() error { _, err := fake.StakesOf(t.Context(), userID); return err },
		"ShareUnitsAt":     func() error { _, err := fake.ShareUnitsAt(t.Context(), cabalID, userID, time.Time{}); return err },
		"CabalPositionsAt": func() error { _, err := fake.CabalPositionsAt(t.Context(), time.Time{}); return err },
		"MemberStakesAt":   func() error { _, err := fake.MemberStakesAt(t.Context(), time.Time{}); return err },
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			once := errors.New("once")
			always := errors.New("always")
			fake.FailOnce(name, once)
			fake.Fail(name, always)
			if err := call(); !errors.Is(err, once) {
				t.Fatalf("first call error = %v, want %v", err, once)
			}
			if err := call(); !errors.Is(err, always) {
				t.Fatalf("second call error = %v, want %v", err, always)
			}
			fake.Fail(name, nil)
			if err := call(); err != nil {
				t.Fatalf("call after clearing error = %v, want nil", err)
			}
		})
	}
}

func TestTreasury_isSafeForConcurrentUse(t *testing.T) {
	t.Parallel()
	fake := fakes.NewTreasury()
	cabalID := ids.CabalIDFrom(ids.Real{}.NewV7())
	userID := ids.UserIDFrom(ids.Real{}.NewV7())
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			fake.SetPotValue(cabalID, money.MicrosFromUint64(1))
			_, _ = fake.PotValue(t.Context(), cabalID)
			fake.SetStake(treasury.Stake{CabalID: cabalID, UserID: userID})
			_, _ = fake.StakesOf(t.Context(), userID)
		})
	}
	wg.Wait()
}

func TestTreasury_PotValueCanFailForMissingPrice(t *testing.T) {
	t.Parallel()
	fake := fakes.NewTreasury()
	fake.Fail("PotValue", errs.New(errs.CodePriceUnavailable, "test"))
	_, err := fake.PotValue(t.Context(), ids.CabalIDFrom(ids.Real{}.NewV7()))
	if errs.CodeOf(err) != errs.CodePriceUnavailable {
		t.Fatalf("PotValue code = %s, want %s", errs.CodeOf(err), errs.CodePriceUnavailable)
	}
}
