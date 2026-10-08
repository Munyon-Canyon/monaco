package fakes_test

import (
	"bytes"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

type cabalWorld struct {
	fake                  *fakes.Cabal
	seeds                 []fakes.CabalSeed
	members               []fakes.CabalMember
	alpha, omega          fakes.CabalSeed
	creator, guest, other ids.UserID
	joined                time.Time
}

func cabalIDs(t *testing.T) (func() ids.UserID, func() ids.CabalID) {
	t.Helper()
	g := testkit.NewIDs(testkit.RandSeed(t))
	return func() ids.UserID { return ids.NewUserID(g) }, func() ids.CabalID { return ids.CabalIDFrom(g.NewV7()) }
}

func newCabalWorld(t *testing.T) cabalWorld {
	t.Helper()
	newUser, newCabal := cabalIDs(t)
	joined := clock.Real{}.Now().Truncate(time.Second).In(time.FixedZone("x", 3600))
	w := cabalWorld{creator: newUser(), guest: newUser(), other: newUser(), joined: joined}
	seed := func(name string, status cabal.Status, bps int32) fakes.CabalSeed {
		id := newCabal()
		return fakes.CabalSeed{
			View: cabal.View{ID: id, Name: name, CreatorID: w.creator, Status: status, CreatedAt: w.joined},
			Rules: cabal.Rules{
				JoinMode: cabal.JoinOpen, VoterMode: cabal.VotersList, Threshold: cabal.ThresholdMajority,
				ProposalExpiry: time.Hour, SlippageBps: bps,
			},
			Wallet: cabal.TreasuryWallet{
				CabalID: id, PrivyWalletID: "wallet-" + name, Address: chain.SolanaAddress("addr-" + name),
			},
		}
	}
	w.alpha, w.omega = seed("alpha", cabal.StatusActive, 50), seed("omega", cabal.StatusBanned, 300)
	w.seeds = []fakes.CabalSeed{w.omega, w.alpha}
	join := func(c fakes.CabalSeed, user ids.UserID, role cabal.Role, canVote bool, at time.Time) fakes.CabalMember {
		return fakes.CabalMember{
			CabalID: c.View.ID, Member: cabal.MemberView{UserID: user, Role: role, CanVote: canVote, JoinedAt: at},
		}
	}
	w.members = []fakes.CabalMember{
		join(w.omega, w.guest, cabal.RoleMember, false, w.joined),
		join(w.omega, w.other, cabal.RoleMember, false, w.joined),
		join(w.alpha, w.guest, cabal.RoleMember, false, w.joined),
		join(w.alpha, w.creator, cabal.RoleCreator, true, w.joined.Add(-time.Hour)),
	}
	w.fake = fakes.NewCabal(w.seeds, w.members)
	return w
}

func compareCabalIDs(a, b ids.CabalID) int {
	x, y := a.UUID(), b.UUID()
	return bytes.Compare(x[:], y[:])
}

func cabalCode(t *testing.T, err error, want errs.Code) {
	t.Helper()
	if got := errs.CodeOf(err); err == nil || got != want {
		t.Fatalf("err = %v (code %s), want %s", err, got, want)
	}
}

func TestCabal_derivesTheMemberCountAndShowsTimesInUTC(t *testing.T) {
	t.Parallel()
	w := newCabalWorld(t)
	got, err := w.fake.Cabal(t.Context(), w.alpha.View.ID)
	want := w.alpha.View
	want.MemberCount, want.CreatedAt = 2, w.joined.UTC()
	if err != nil || got != want || got.CreatedAt.Location() != time.UTC {
		t.Fatalf("Cabal = %+v, %v, want %+v in UTC", got, err, want)
	}
	byID, err := w.fake.Cabals(t.Context(), []ids.CabalID{w.omega.View.ID, {}})
	if err != nil || len(byID) != 1 || byID[w.omega.View.ID].MemberCount != 2 {
		t.Fatalf("Cabals = %+v, %v, want only omega with two members", byID, err)
	}
	_, err = w.fake.Cabal(t.Context(), ids.CabalID{})
	cabalCode(t, err, errs.CodeCabalNotFound)
}

func TestCabal_membersComeBackInJoinOrderAndVotersAreTheOnesWhoCanVote(t *testing.T) {
	t.Parallel()
	w := newCabalWorld(t)
	members, err := w.fake.Members(t.Context(), w.alpha.View.ID)
	if err != nil || len(members) != 2 || members[0].UserID != w.creator || members[1].UserID != w.guest ||
		members[0].JoinedAt.Location() != time.UTC {
		t.Fatalf("Members = %+v, %v, want the creator and then the guest, joined in UTC", members, err)
	}
	voters, err := w.fake.VoterSet(t.Context(), w.alpha.View.ID)
	if err != nil || !slices.Equal(voters, []ids.UserID{w.creator}) {
		t.Fatalf("VoterSet = %v, %v, want only the creator", voters, err)
	}
	member, err := w.fake.Member(t.Context(), w.omega.View.ID, w.guest)
	if err != nil || member.UserID != w.guest {
		t.Fatalf("Member = %+v, %v, want the guest", member, err)
	}
	_, err = w.fake.Member(t.Context(), w.omega.View.ID, w.creator)
	cabalCode(t, err, errs.CodeNotCabalMember)
}

func TestCabal_isMemberIsTrueOnlyForAMemberOfThatCabal(t *testing.T) {
	t.Parallel()
	w := newCabalWorld(t)
	for _, tc := range []struct {
		cabal ids.CabalID
		user  ids.UserID
		want  bool
	}{{w.alpha.View.ID, w.creator, true}, {w.omega.View.ID, w.creator, false}, {ids.CabalID{}, w.guest, false}} {
		if got, err := w.fake.IsMember(t.Context(), tc.cabal, tc.user); err != nil || got != tc.want {
			t.Errorf("IsMember(%v, %v) = %v, %v, want %v", tc.cabal, tc.user, got, err, tc.want)
		}
	}
}

func TestCabal_cabalsOfOrdersByJoinTimeThenCabalIDAndWalletsByCabalID(t *testing.T) {
	t.Parallel()
	w := newCabalWorld(t)
	want := []ids.CabalID{w.alpha.View.ID, w.omega.View.ID}
	slices.SortFunc(want, compareCabalIDs)
	got, err := w.fake.CabalsOf(t.Context(), w.guest)
	if err != nil || !slices.Equal(got, want) {
		t.Fatalf("CabalsOf(guest) = %v, %v, want both joined together by cabal id %v", got, err, want)
	}
	if none, err := w.fake.CabalsOf(t.Context(), ids.UserID{}); err != nil || none == nil || len(none) != 0 {
		t.Fatalf("CabalsOf(nobody) = %v, %v, want an empty list", none, err)
	}
	wallets, err := w.fake.TreasuryWallets(t.Context())
	if err != nil || len(wallets) != 2 || compareCabalIDs(wallets[0].CabalID, wallets[1].CabalID) > 0 {
		t.Fatalf("TreasuryWallets = %+v, %v, want both by cabal id", wallets, err)
	}
	one, err := w.fake.TreasuryWallet(t.Context(), w.omega.View.ID)
	if err != nil || one != w.omega.Wallet {
		t.Fatalf("TreasuryWallet = %+v, %v, want %+v", one, err, w.omega.Wallet)
	}
	_, err = w.fake.TreasuryWallet(t.Context(), ids.CabalID{})
	cabalCode(t, err, errs.CodeCabalNotFound)
}

func TestCabal_rulesSlippageAndStatusComeFromTheSeed(t *testing.T) {
	t.Parallel()
	w := newCabalWorld(t)
	rules, err := w.fake.Rules(t.Context(), w.omega.View.ID)
	bps, bpsErr := w.fake.SlippageBps(t.Context(), w.omega.View.ID)
	status, statusErr := w.fake.Status(t.Context(), w.omega.View.ID)
	failed := errors.Join(err, bpsErr, statusErr)
	if failed != nil || rules != w.omega.Rules || bps != 300 || status != cabal.StatusBanned {
		t.Fatalf("Rules = %+v, SlippageBps = %d, Status = %q, err %v", rules, bps, status, failed)
	}
	_, err = w.fake.Rules(t.Context(), ids.CabalID{})
	cabalCode(t, err, errs.CodeCabalNotFound)
	_, err = w.fake.SlippageBps(t.Context(), ids.CabalID{})
	cabalCode(t, err, errs.CodeCabalNotFound)
	_, err = w.fake.Status(t.Context(), ids.CabalID{})
	cabalCode(t, err, errs.CodeCabalNotFound)
}

func TestCabal_keepsACopyOfTheSeedsItWasBuiltFrom(t *testing.T) {
	t.Parallel()
	w := newCabalWorld(t)
	w.seeds[0].View.Name, w.members[0].Member.CanVote = "changed", true
	got, err := w.fake.Cabal(t.Context(), w.omega.View.ID)
	if err != nil || got.Name != "omega" {
		t.Fatalf("Cabal after the caller changed its slice = %+v, %v, want the original", got, err)
	}
	if voters, err := w.fake.VoterSet(t.Context(), w.omega.View.ID); err != nil || len(voters) != 0 {
		t.Fatalf("VoterSet after the caller changed its slice = %v, %v, want none", voters, err)
	}
}

func cabalCalls(t *testing.T) map[string]func(f *fakes.Cabal) error {
	t.Helper()
	ctx := t.Context()
	newUser, newCabal := cabalIDs(t)
	user, id := newUser(), newCabal()
	return map[string]func(f *fakes.Cabal) error{
		"Cabal":           func(f *fakes.Cabal) error { _, err := f.Cabal(ctx, id); return err },
		"Cabals":          func(f *fakes.Cabal) error { _, err := f.Cabals(ctx, []ids.CabalID{id}); return err },
		"IsMember":        func(f *fakes.Cabal) error { _, err := f.IsMember(ctx, id, user); return err },
		"Member":          func(f *fakes.Cabal) error { _, err := f.Member(ctx, id, user); return err },
		"Members":         func(f *fakes.Cabal) error { _, err := f.Members(ctx, id); return err },
		"VoterSet":        func(f *fakes.Cabal) error { _, err := f.VoterSet(ctx, id); return err },
		"Rules":           func(f *fakes.Cabal) error { _, err := f.Rules(ctx, id); return err },
		"SlippageBps":     func(f *fakes.Cabal) error { _, err := f.SlippageBps(ctx, id); return err },
		"Status":          func(f *fakes.Cabal) error { _, err := f.Status(ctx, id); return err },
		"TreasuryWallet":  func(f *fakes.Cabal) error { _, err := f.TreasuryWallet(ctx, id); return err },
		"TreasuryWallets": func(f *fakes.Cabal) error { _, err := f.TreasuryWallets(ctx); return err },
		"CabalsOf":        func(f *fakes.Cabal) error { _, err := f.CabalsOf(ctx, user); return err },
	}
}

func cabalOthersUnaffected(
	t *testing.T, f *fakes.Cabal, calls map[string]func(f *fakes.Cabal) error, failing string, down error,
) {
	t.Helper()
	for name, call := range calls {
		if name != failing && errors.Is(call(f), down) {
			t.Errorf("%s failed because %s was told to", name, failing)
		}
	}
}

func TestCabal_failOnceFailsTheNextCallOfThatMethodOnly(t *testing.T) {
	t.Parallel()
	down := errs.New(errs.CodeDBUnavailable, "test")
	calls := cabalCalls(t)
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := fakes.NewCabal(nil, nil)
			f.FailOnce(name, down)
			cabalOthersUnaffected(t, f, calls, name, down)
			if err := call(f); !errors.Is(err, down) {
				t.Fatalf("first call after FailOnce err = %v, want the injected error", err)
			}
			if err := call(f); errors.Is(err, down) {
				t.Fatal("second call after FailOnce failed again")
			}
		})
	}
}

func TestCabal_failFailsEveryCallOfThatMethodUntilCleared(t *testing.T) {
	t.Parallel()
	down := errs.New(errs.CodeDBUnavailable, "test")
	calls := cabalCalls(t)
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := fakes.NewCabal(nil, nil)
			f.Fail(name, down)
			cabalOthersUnaffected(t, f, calls, name, down)
			for range 2 {
				if err := call(f); !errors.Is(err, down) {
					t.Fatalf("call after Fail err = %v, want the injected error", err)
				}
			}
			f.Fail(name, nil)
			if err := call(f); errors.Is(err, down) {
				t.Fatal("call after Fail(nil) still failed")
			}
		})
	}
}

func TestCabal_isSafeForConcurrentUse(t *testing.T) {
	t.Parallel()
	w := newCabalWorld(t)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			view, viewErr := w.fake.Cabal(t.Context(), w.alpha.View.ID)
			mine, mineErr := w.fake.CabalsOf(t.Context(), w.guest)
			w.fake.FailOnce("Status", nil)
			if viewErr != nil || mineErr != nil || view.MemberCount != 2 || len(mine) != 2 {
				t.Errorf("worker saw %+v %v and %v %v", view, viewErr, mine, mineErr)
			}
		})
	}
	wg.Wait()
}
