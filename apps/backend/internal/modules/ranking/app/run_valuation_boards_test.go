package app

import (
	"math"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func TestRunValuation_failsTheTickWhenThePreviousRowsCannotBeRead(t *testing.T) {
	t.Parallel()
	f, usdc := heldAssetPorts(t, 1)
	f.previousErr = errs.New(errs.CodeInternal, "test")
	delete(f.latestRows, f.assetRows[0].ID)
	delete(f.asOfRows, f.assetRows[0].ID)
	if _, err := NewRunValuation(
		Ports{Market: f, Treasury: f, Funding: f, Cabals: f, Users: f, Previous: f}, usdc,
	).Run(t.Context(), valuationTime()); err == nil {
		t.Fatal("Run() error = nil")
	}
}

func TestBuildEntries_refusesLifetimeAndSumsThatDoNotFit(t *testing.T) {
	t.Parallel()
	micros, shares, signed := money.MicrosFromUint64, money.SharesUnitsFromUint64, money.SignedMicrosFromInt64
	for name, tc := range map[string]func(r *boardRig){
		"a cabal's gain beyond int64": func(r *boardRig) {
			r.input.stakes = nil
			r.input.valued[0].Value = micros(math.MaxUint64)
		},
		"a member's gain beyond int64": func(r *boardRig) {
			r.input.valued[0].Value = micros(math.MaxInt64 + 5)
			r.input.stakes = []treasury.MemberStake{
				{CabalID: r.alpha, UserID: r.u1, ShareUnits: shares(100)},
				{CabalID: r.alpha, UserID: r.u2, NetContributedMicros: signed(math.MaxInt64)},
			}
			r.input.members[r.alpha] = append(r.input.members[r.alpha], cabalport.MemberView{UserID: r.u2})
		},
		"a person's gain beyond int64": func(r *boardRig) {
			half := micros(1<<62 + 2_000_000)
			r.input.valued = r.input.valued[:2]
			r.input.valued[0].Value, r.input.valued[1].Value = half, half
			r.input.stakes = []treasury.MemberStake{
				{CabalID: r.alpha, UserID: r.u1, ShareUnits: shares(100), NetContributedMicros: signed(1_000_000)},
				{CabalID: r.bet, UserID: r.u1, ShareUnits: shares(100), NetContributedMicros: signed(1_000_000)},
			}
		},
		"a person's value beyond uint64": func(r *boardRig) {
			third := ids.CabalIDFrom(ids.Real{}.NewV7())
			r.input.valued = []CabalValue{
				{CabalID: r.alpha, Value: micros(math.MaxInt64), TotalShares: shares(100)},
				{CabalID: r.bet, Value: micros(math.MaxInt64), TotalShares: shares(100)},
				{CabalID: third, Value: micros(math.MaxInt64), TotalShares: shares(100)},
			}
			r.input.stakes = []treasury.MemberStake{
				{CabalID: r.alpha, UserID: r.u1, ShareUnits: shares(100), NetContributedMicros: signed(1_000_000)},
				{CabalID: r.bet, UserID: r.u1, ShareUnits: shares(100), NetContributedMicros: signed(1_000_000)},
				{CabalID: third, UserID: r.u1, ShareUnits: shares(100), NetContributedMicros: signed(1_000_000)},
			}
		},
		"a cabal's value beyond int64": func(r *boardRig) {
			r.input.valued[0].Value = micros(math.MaxInt64 + 1)
			r.input.stakes = []treasury.MemberStake{
				{CabalID: r.alpha, UserID: r.u1, ShareUnits: shares(1), NetContributedMicros: signed(math.MaxInt64)},
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := newBoardRig()
			tc(&r)
			if _, err := buildEntries(r.input); err == nil {
				t.Fatal("buildEntries() error = nil")
			}
		})
	}
}
