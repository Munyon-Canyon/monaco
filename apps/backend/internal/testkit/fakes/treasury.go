package fakes

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type Treasury struct {
	testkit.Faults
	mu               sync.Mutex
	positions        map[ids.CabalID][]treasury.Position
	potValues        map[ids.CabalID]money.Micros
	totalShares      map[ids.CabalID]money.SharesUnits
	stakes           []treasury.Stake
	cabalPositionsAt []treasury.CabalPositions
	memberStakesAt   []treasury.MemberStake
}

var (
	_ treasury.Queries        = (*Treasury)(nil)
	_ treasury.SignatureOwner = (*Treasury)(nil)
	_ treasury.WalletLedger   = (*Treasury)(nil)
)

func (f *Treasury) OwnsSignature(context.Context, chain.Signature) (bool, error) {
	return false, f.Check("OwnsSignature")
}

func (f *Treasury) WalletLedgerMicros(
	context.Context,
	ids.UserID,
	chain.SolanaAddress,
) (money.SignedMicros, int, error) {
	return money.SignedMicros{}, 0, f.Check("WalletLedgerMicros")
}

func NewTreasury() *Treasury {
	return &Treasury{
		positions:   map[ids.CabalID][]treasury.Position{},
		potValues:   map[ids.CabalID]money.Micros{},
		totalShares: map[ids.CabalID]money.SharesUnits{},
	}
}

func (f *Treasury) SetPositions(cabalID ids.CabalID, positions []treasury.Position) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.positions[cabalID] = slices.Clone(positions)
}

func (f *Treasury) SetPotValue(cabalID ids.CabalID, value money.Micros) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.potValues[cabalID] = value
}

func (f *Treasury) SetTotalShares(cabalID ids.CabalID, shares money.SharesUnits) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.totalShares[cabalID] = shares
}

func (f *Treasury) SetStake(stake treasury.Stake) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if i := slices.IndexFunc(f.stakes, func(current treasury.Stake) bool {
		return current.CabalID == stake.CabalID && current.UserID == stake.UserID
	}); i >= 0 {
		f.stakes[i] = stake
		return
	}
	f.stakes = append(f.stakes, stake)
}

func (f *Treasury) SetCabalPositionsAt(positions []treasury.CabalPositions) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cabalPositionsAt = cloneCabalPositions(positions)
}

func (f *Treasury) SetMemberStakesAt(stakes []treasury.MemberStake) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.memberStakesAt = slices.Clone(stakes)
}

func (f *Treasury) Positions(_ context.Context, cabalID ids.CabalID) ([]treasury.Position, error) {
	if err := f.Check("Positions"); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.positions[cabalID]), nil
}

func (f *Treasury) PotValue(_ context.Context, cabalID ids.CabalID) (money.Micros, error) {
	if err := f.Check("PotValue"); err != nil {
		return money.Micros{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.potValues[cabalID], nil
}

func (f *Treasury) TotalShares(_ context.Context, cabalID ids.CabalID) (money.SharesUnits, error) {
	if err := f.Check("TotalShares"); err != nil {
		return money.SharesUnits{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.totalShares[cabalID], nil
}

func (f *Treasury) ShareUnits(_ context.Context, cabalID ids.CabalID, user ids.UserID) (money.SharesUnits, error) {
	if err := f.Check("ShareUnits"); err != nil {
		return money.SharesUnits{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.stake(cabalID, user).ShareUnits, nil
}

func (f *Treasury) Stake(_ context.Context, cabalID ids.CabalID, user ids.UserID) (treasury.Stake, error) {
	if err := f.Check("Stake"); err != nil {
		return treasury.Stake{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.stake(cabalID, user), nil
}

func (f *Treasury) StakesOf(_ context.Context, user ids.UserID) ([]treasury.Stake, error) {
	if err := f.Check("StakesOf"); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	stakes := []treasury.Stake{}
	for _, stake := range f.stakes {
		if stake.UserID == user && !stake.ShareUnits.IsZero() {
			stakes = append(stakes, stake)
		}
	}
	return stakes, nil
}

func (f *Treasury) ShareUnitsAt(
	_ context.Context, cabalID ids.CabalID, user ids.UserID, _ time.Time,
) (money.SharesUnits, error) {
	if err := f.Check("ShareUnitsAt"); err != nil {
		return money.SharesUnits{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.stake(cabalID, user).ShareUnits, nil
}

func (f *Treasury) CabalPositionsAt(_ context.Context, _ time.Time) ([]treasury.CabalPositions, error) {
	if err := f.Check("CabalPositionsAt"); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return cloneCabalPositions(f.cabalPositionsAt), nil
}

func (f *Treasury) MemberStakesAt(_ context.Context, _ time.Time) ([]treasury.MemberStake, error) {
	if err := f.Check("MemberStakesAt"); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.memberStakesAt), nil
}

func (f *Treasury) stake(cabalID ids.CabalID, user ids.UserID) treasury.Stake {
	if i := slices.IndexFunc(f.stakes, func(stake treasury.Stake) bool {
		return stake.CabalID == cabalID && stake.UserID == user
	}); i >= 0 {
		return f.stakes[i]
	}
	return treasury.Stake{}
}

func cloneCabalPositions(in []treasury.CabalPositions) []treasury.CabalPositions {
	out := slices.Clone(in)
	for i := range out {
		out[i].Holdings = slices.Clone(out[i].Holdings)
	}
	return out
}
