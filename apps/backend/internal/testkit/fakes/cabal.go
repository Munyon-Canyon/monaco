package fakes

import (
	"bytes"
	"context"
	"slices"
	"sync"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type CabalSeed struct {
	View   cabal.View
	Rules  cabal.Rules
	Wallet cabal.TreasuryWallet
}

type CabalMember struct {
	CabalID ids.CabalID
	Member  cabal.MemberView
}

type Cabal struct {
	testkit.Faults
	mu      sync.Mutex
	cabals  []CabalSeed
	members []CabalMember
}

var _ cabal.Queries = (*Cabal)(nil)

func NewCabal(cabals []CabalSeed, members []CabalMember) *Cabal {
	f := &Cabal{cabals: slices.Clone(cabals), members: slices.Clone(members)}
	for i := range f.cabals {
		f.cabals[i].View.CreatedAt = f.cabals[i].View.CreatedAt.UTC()
	}
	for i := range f.members {
		f.members[i].Member.JoinedAt = f.members[i].Member.JoinedAt.UTC()
	}
	slices.SortFunc(f.members, func(a, b CabalMember) int {
		if c := a.Member.JoinedAt.Compare(b.Member.JoinedAt); c != 0 {
			return c
		}
		if c := compareUUIDs(a.Member.UserID.UUID(), b.Member.UserID.UUID()); c != 0 {
			return c
		}
		return compareUUIDs(a.CabalID.UUID(), b.CabalID.UUID())
	})
	slices.SortFunc(f.cabals, func(a, b CabalSeed) int {
		return compareUUIDs(a.View.ID.UUID(), b.View.ID.UUID())
	})
	return f
}

func (f *Cabal) Cabal(_ context.Context, id ids.CabalID) (cabal.View, error) {
	const op = "Cabal"
	f.mu.Lock()
	defer f.mu.Unlock()
	seed, err := f.seed(op, id)
	if err != nil {
		return cabal.View{}, err
	}
	return f.viewOf(seed), nil
}

func (f *Cabal) Cabals(_ context.Context, cabalIDs []ids.CabalID) (map[ids.CabalID]cabal.View, error) {
	if err := f.Check("Cabals"); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	found := make(map[ids.CabalID]cabal.View, len(cabalIDs))
	for _, id := range cabalIDs {
		if i := f.index(id); i >= 0 {
			found[id] = f.viewOf(f.cabals[i])
		}
	}
	return found, nil
}

func (f *Cabal) IsMember(_ context.Context, id ids.CabalID, user ids.UserID) (bool, error) {
	if err := f.Check("IsMember"); err != nil {
		return false, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.ContainsFunc(f.members, func(m CabalMember) bool {
		return m.CabalID == id && m.Member.UserID == user
	}), nil
}

func (f *Cabal) Member(_ context.Context, id ids.CabalID, user ids.UserID) (cabal.MemberView, error) {
	const op = "Member"
	if err := f.Check(op); err != nil {
		return cabal.MemberView{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	i := slices.IndexFunc(f.members, func(m CabalMember) bool { return m.CabalID == id && m.Member.UserID == user })
	if i < 0 {
		return cabal.MemberView{}, errs.New(errs.CodeNotCabalMember, "fakes.Cabal."+op)
	}
	return f.members[i].Member, nil
}

func (f *Cabal) Members(_ context.Context, id ids.CabalID) ([]cabal.MemberView, error) {
	if err := f.Check("Members"); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.membersOf(id, func(cabal.MemberView) bool { return true }), nil
}

func (f *Cabal) VoterSet(_ context.Context, id ids.CabalID) ([]ids.UserID, error) {
	if err := f.Check("VoterSet"); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	voters := []ids.UserID{}
	for _, m := range f.membersOf(id, func(m cabal.MemberView) bool { return m.CanVote }) {
		voters = append(voters, m.UserID)
	}
	return voters, nil
}

func (f *Cabal) Rules(_ context.Context, id ids.CabalID) (cabal.Rules, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	seed, err := f.seed("Rules", id)
	return seed.Rules, err
}

func (f *Cabal) SlippageBps(_ context.Context, id ids.CabalID) (int32, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	seed, err := f.seed("SlippageBps", id)
	return seed.Rules.SlippageBps, err
}

func (f *Cabal) Status(_ context.Context, id ids.CabalID) (cabal.Status, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	seed, err := f.seed("Status", id)
	return seed.View.Status, err
}

func (f *Cabal) TreasuryWallet(_ context.Context, id ids.CabalID) (cabal.TreasuryWallet, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	seed, err := f.seed("TreasuryWallet", id)
	return seed.Wallet, err
}

func (f *Cabal) TreasuryWallets(context.Context) ([]cabal.TreasuryWallet, error) {
	if err := f.Check("TreasuryWallets"); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	wallets := make([]cabal.TreasuryWallet, len(f.cabals))
	for i, seed := range f.cabals {
		wallets[i] = seed.Wallet
	}
	return wallets, nil
}

func (f *Cabal) CabalsOf(_ context.Context, user ids.UserID) ([]ids.CabalID, error) {
	if err := f.Check("CabalsOf"); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	cabals := []ids.CabalID{}
	for _, m := range f.members {
		if m.Member.UserID == user {
			cabals = append(cabals, m.CabalID)
		}
	}
	return cabals, nil
}

func (f *Cabal) seed(op string, id ids.CabalID) (CabalSeed, error) {
	if err := f.Check(op); err != nil {
		return CabalSeed{}, err
	}
	i := f.index(id)
	if i < 0 {
		return CabalSeed{}, errs.New(errs.CodeCabalNotFound, "fakes.Cabal."+op)
	}
	return f.cabals[i], nil
}

func (f *Cabal) index(id ids.CabalID) int {
	return slices.IndexFunc(f.cabals, func(s CabalSeed) bool { return s.View.ID == id })
}

func (f *Cabal) membersOf(id ids.CabalID, keep func(cabal.MemberView) bool) []cabal.MemberView {
	members := []cabal.MemberView{}
	for _, m := range f.members {
		if m.CabalID == id && keep(m.Member) {
			members = append(members, m.Member)
		}
	}
	return members
}

func (f *Cabal) viewOf(seed CabalSeed) cabal.View {
	view := seed.View
	view.MemberCount = len(f.membersOf(view.ID, func(cabal.MemberView) bool { return true }))
	return view
}

func compareUUIDs(a, b [16]byte) int { return bytes.Compare(a[:], b[:]) }
