package fakes_test

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"maps"
	"slices"
	"strconv"
	"sync"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

func identityID(t *testing.T, g *testkit.IDs) ids.UserID {
	t.Helper()
	id, err := ids.ParseUserID(g.NewV7().String())
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func identityHash(label string) []byte {
	sum := sha256.Sum256([]byte(label))
	return sum[:]
}

func identityCode(t *testing.T, err error, want errs.Code) {
	t.Helper()
	if got := errs.CodeOf(err); err == nil || got != want {
		t.Fatalf("err = %v (code %s), want %s", err, got, want)
	}
}

type identityWorld struct {
	fake    *fakes.Identity
	kinds   []string
	users   map[string]identity.UserCard
	kindOf  map[ids.UserID]string
	all     []ids.UserID
	handles []string
	hashes  [][]byte
	xIDs    []string
	wallets []identity.MemberWallet
}

func newIdentityWorld(t *testing.T) identityWorld {
	t.Helper()
	g := testkit.NewIDs(testkit.RandSeed(t))
	statuses := map[string]identity.AccountStatus{
		"active": identity.AccountActive, "suspended": identity.AccountSuspended, "banned": identity.AccountBanned,
		"deleted": identity.AccountDeleted, "soft_deleted": identity.AccountActive,
	}
	w := identityWorld{
		kinds:  []string{"active", "suspended", "banned", "deleted", "soft_deleted"},
		users:  map[string]identity.UserCard{},
		kindOf: map[ids.UserID]string{},
	}
	cards := make([]identity.UserCard, 0, len(w.kinds))
	for _, kind := range w.kinds {
		card := identity.UserCard{
			ID: identityID(t, g), Handle: "user_" + kind, DisplayName: "Name " + kind,
			PhotoURL: "https://photos.test/" + kind, AccountStatus: statuses[kind],
			PhoneVerified: true, XLinked: true, Deleted: kind == "deleted" || kind == "soft_deleted",
		}
		cards = append(cards, card)
		w.users[kind], w.kindOf[card.ID] = card, kind
		w.all, w.handles = append(w.all, card.ID), append(w.handles, card.Handle)
		w.hashes, w.xIDs = append(w.hashes, identityHash(kind)), append(w.xIDs, "x-"+kind)
		w.wallets = append(w.wallets, identity.MemberWallet{
			UserID: card.ID, PrivyWalletID: "wallet-" + kind, Address: chain.SolanaAddress("address-" + kind),
		})
	}
	slices.Reverse(w.wallets)
	w.fake = fakes.NewIdentity(cards, w.wallets)
	for kind, card := range w.users {
		w.fake.SetPhoneHash(card.ID, identityHash(kind))
		w.fake.SetXUserID(card.ID, "x-"+kind)
	}
	return w
}

func (w identityWorld) visible(seen func(ids.UserID) bool) []string {
	var out []string
	for _, kind := range w.kinds {
		if seen(w.users[kind].ID) {
			out = append(out, kind)
		}
	}
	return out
}

func (w identityWorld) visibleByMethod(t *testing.T) map[string][]string {
	t.Helper()
	ctx := t.Context()
	got := map[string][]string{}
	cards, err := w.fake.UsersByID(ctx, w.all)
	if err != nil {
		t.Fatal(err)
	}
	got["UsersByID"] = w.visible(func(id ids.UserID) bool { _, ok := cards[id]; return ok })
	got["UserByHandle"] = w.visible(func(id ids.UserID) bool {
		_, err := w.fake.UserByHandle(ctx, "user_"+w.kindOf[id])
		return err == nil
	})
	byHandles, err := w.fake.UserIDsByHandles(ctx, w.handles)
	if err != nil {
		t.Fatal(err)
	}
	got["UserIDsByHandles"] = w.visible(func(id ids.UserID) bool { return byHandles["user_"+w.kindOf[id]] == id })
	byPhone, err := w.fake.UsersByPhoneHashes(ctx, w.hashes)
	if err != nil {
		t.Fatal(err)
	}
	got["UsersByPhoneHashes"] = w.visible(func(id ids.UserID) bool {
		return byPhone[hex.EncodeToString(identityHash(w.kindOf[id]))] == id
	})
	byX, err := w.fake.UsersByXUserIDs(ctx, w.xIDs)
	if err != nil {
		t.Fatal(err)
	}
	got["UsersByXUserIDs"] = w.visible(func(id ids.UserID) bool { return byX["x-"+w.kindOf[id]] == id })
	got["MemberWallet"] = w.visible(func(id ids.UserID) bool {
		_, err := w.fake.MemberWallet(ctx, id)
		return err == nil
	})
	page, err := w.fake.MemberWallets(ctx, ids.UserID{}, identity.MaxWalletPage)
	if err != nil {
		t.Fatal(err)
	}
	got["MemberWallets"] = w.visible(func(id ids.UserID) bool {
		return slices.ContainsFunc(page, func(mw identity.MemberWallet) bool { return mw.UserID == id })
	})
	return got
}

func TestIdentity_showsOnlyTheUsersEachMethodsRuleAllows(t *testing.T) {
	t.Parallel()
	w := newIdentityWorld(t)
	want := map[string][]string{
		"UsersByID":          {"active", "suspended", "banned", "deleted", "soft_deleted"},
		"UserByHandle":       {"active", "suspended", "banned"},
		"UserIDsByHandles":   {"active", "suspended", "banned"},
		"UsersByPhoneHashes": {"active"},
		"UsersByXUserIDs":    {"active"},
		"MemberWallet":       {"active", "suspended", "banned"},
		"MemberWallets":      {"active", "suspended", "banned"},
	}
	if got := w.visibleByMethod(t); !maps.EqualFunc(got, want, slices.Equal) {
		t.Fatalf("users visible per method = %v, want %v", got, want)
	}
	banned, gone := w.users["banned"], w.users["deleted"]
	gone.Handle, gone.DisplayName, gone.PhotoURL = "", "", ""
	cards, err := w.fake.UsersByID(t.Context(), []ids.UserID{banned.ID, gone.ID})
	if err != nil || cards[banned.ID] != banned || cards[gone.ID] != gone {
		t.Fatalf("UsersByID = %+v, %v, want the banned card as built and the deleted one with no handle, name or photo",
			cards, err)
	}
}

func TestIdentity_cardsComeBackTheWayThePortShowsThem(t *testing.T) {
	t.Parallel()
	g := testkit.NewIDs(testkit.RandSeed(t))
	named := identity.UserCard{
		ID: identityID(t, g), Handle: "kaicenat", DisplayName: "Kai", PhotoURL: "https://photos.test/kai",
	}
	unnamed := identity.UserCard{ID: identityID(t, g), Handle: "speed", PhotoURL: "https://photos.test/speed"}
	gone := identity.UserCard{
		ID: identityID(t, g), Handle: "gone_user", DisplayName: "Gone", PhotoURL: "https://photos.test/gone",
		AccountStatus: identity.AccountDeleted, PhoneVerified: true, XLinked: true, Deleted: true,
	}
	given := []identity.UserCard{named, unnamed, gone}
	f := fakes.NewIdentity(given, nil)
	if !slices.Equal(given, []identity.UserCard{named, unnamed, gone}) {
		t.Fatalf("NewIdentity changed the cards it was given: %+v", given)
	}
	wantUnnamed, wantGone := unnamed, gone
	wantUnnamed.DisplayName = "speed"
	wantGone.Handle, wantGone.DisplayName, wantGone.PhotoURL = "", "", ""
	want := map[ids.UserID]identity.UserCard{named.ID: named, unnamed.ID: wantUnnamed, gone.ID: wantGone}
	got, err := f.UsersByID(t.Context(), []ids.UserID{named.ID, unnamed.ID, gone.ID})
	if err != nil || !maps.Equal(got, want) {
		t.Fatalf("UsersByID = %+v, %v, want %+v", got, err, want)
	}
	if card, err := f.UserByHandle(t.Context(), "speed"); err != nil || card != wantUnnamed {
		t.Fatalf("UserByHandle(speed) = %+v, %v, want its handle as the display name %+v", card, err, wantUnnamed)
	}
}

func TestIdentity_handlesMatchInAnyCaseAndNeverMatchAMalformedOne(t *testing.T) {
	t.Parallel()
	g := testkit.NewIDs(testkit.RandSeed(t))
	kai := identity.UserCard{ID: identityID(t, g), Handle: "kaicenat", DisplayName: "Kai"}
	f := fakes.NewIdentity([]identity.UserCard{kai, {ID: identityID(t, g), Handle: "speed"}}, nil)
	ctx := t.Context()
	if card, err := f.UserByHandle(ctx, "KaiCenat"); err != nil || card != kai {
		t.Fatalf("UserByHandle(KaiCenat) = %+v, %v, want %+v", card, err, kai)
	}
	misses := []string{"nobody_here", "ab", "bad handle!", "", "kaïcenat", "kaicenat_way_too_long_handle"}
	for _, handle := range misses {
		_, err := f.UserByHandle(ctx, handle)
		identityCode(t, err, errs.CodeUserNotFound)
	}
	found, err := f.UserIDsByHandles(ctx, append([]string{"KAICENAT", "kaicenat"}, misses...))
	if want := (map[string]ids.UserID{"KAICENAT": kai.ID, "kaicenat": kai.ID}); err != nil || !maps.Equal(found, want) {
		t.Fatalf("UserIDsByHandles = %v, %v, want each spelling asked for mapped to the user %v", found, err, want)
	}
}

func TestIdentity_contactsMatchOnlyVerifiedPhonesOfKnownActiveUsers(t *testing.T) {
	t.Parallel()
	g := testkit.NewIDs(testkit.RandSeed(t))
	verified := identity.UserCard{ID: identityID(t, g), AccountStatus: identity.AccountActive, PhoneVerified: true}
	unverified := identity.UserCard{ID: identityID(t, g), AccountStatus: identity.AccountActive}
	ghost := identityID(t, g)
	f := fakes.NewIdentity([]identity.UserCard{verified, unverified}, nil)
	for id, label := range map[ids.UserID]string{verified.ID: "verified", unverified.ID: "unverified", ghost: "ghost"} {
		f.SetPhoneHash(id, identityHash(label))
		f.SetXUserID(id, "x-"+label)
	}
	asked := [][]byte{identityHash("verified"), identityHash("unverified"), identityHash("ghost"), []byte("short"), nil}
	got, err := f.UsersByPhoneHashes(t.Context(), asked)
	if want := map[string]ids.UserID{hex.EncodeToString(identityHash("verified")): verified.ID}; err != nil ||
		!maps.Equal(got, want) {
		t.Fatalf("UsersByPhoneHashes = %v, %v, want only the verified phone of a known user %v", got, err, want)
	}
	byX, err := f.UsersByXUserIDs(t.Context(), []string{"x-verified", "x-unverified", "x-ghost", "x-none", ""})
	if want := map[string]ids.UserID{"x-verified": verified.ID, "x-unverified": unverified.ID}; err != nil ||
		!maps.Equal(byX, want) {
		t.Fatalf("UsersByXUserIDs = %v, %v, want the two known active users %v", byX, err, want)
	}
}

func TestIdentity_walletsPageInIDOrderWithoutTouchingTheCallersSlice(t *testing.T) {
	t.Parallel()
	g := testkit.NewIDs(testkit.RandSeed(t))
	wallets := make([]identity.MemberWallet, 0, 7)
	var cards []identity.UserCard
	for i := range 7 {
		id := identityID(t, g)
		wallets = append(wallets, identity.MemberWallet{UserID: id, PrivyWalletID: "wallet-" + strconv.Itoa(i)})
		if i == 3 {
			cards = append(cards, identity.UserCard{ID: id, Deleted: true})
		}
	}
	slices.Reverse(wallets)
	given := slices.Clone(wallets)
	f := fakes.NewIdentity(cards, wallets)
	if !slices.Equal(wallets, given) {
		t.Fatal("NewIdentity reordered the slice it was given")
	}
	var (
		paged []identity.MemberWallet
		sizes []int
		after ids.UserID
	)
	for {
		page, err := f.MemberWallets(t.Context(), after, 3)
		if err != nil {
			t.Fatal(err)
		}
		paged, sizes = append(paged, page...), append(sizes, len(page))
		if len(page) < 3 {
			break
		}
		after = page[len(page)-1].UserID
	}
	if !slices.Equal(sizes, []int{3, 3, 0}) {
		t.Fatalf("page sizes = %v, want two full pages of 3 and then an empty one", sizes)
	}
	want := slices.DeleteFunc(
		slices.Clone(given),
		func(w identity.MemberWallet) bool { return w.PrivyWalletID == "wallet-3" },
	)
	slices.Reverse(want)
	if !slices.Equal(paged, want) {
		t.Fatalf("paged wallets = %v, want every visible wallet once in id order %v", paged, want)
	}
	if got, err := f.MemberWallet(t.Context(), want[0].UserID); err != nil || got != want[0] {
		t.Fatalf("MemberWallet = %+v, %v, want %+v", got, err, want[0])
	}
	_, unknown := f.MemberWallet(t.Context(), identityID(t, g))
	identityCode(t, unknown, errs.CodeUserNotFound)
}

func TestIdentity_keepsACopyOfTheCardsItWasBuiltFrom(t *testing.T) {
	t.Parallel()
	g := testkit.NewIDs(testkit.RandSeed(t))
	cards := []identity.UserCard{{ID: identityID(t, g), Handle: "kaicenat"}}
	f := fakes.NewIdentity(cards, nil)
	cards[0].Handle = "changed"
	if card, err := f.UserByHandle(t.Context(), "kaicenat"); err != nil || card.ID != cards[0].ID {
		t.Fatalf("UserByHandle after the caller changed its slice = %+v, %v, want the original card", card, err)
	}
}

func TestIdentity_overLimitInputIsInvalidInput(t *testing.T) {
	t.Parallel()
	f := fakes.NewIdentity(nil, nil)
	ctx := t.Context()
	for _, tc := range []struct {
		name string
		max  int
		call func(n int) error
	}{
		{"UsersByID", 500, func(n int) error {
			_, err := f.UsersByID(ctx, make([]ids.UserID, n))
			return err
		}},
		{"UserIDsByHandles", 50, func(n int) error {
			_, err := f.UserIDsByHandles(ctx, make([]string, n))
			return err
		}},
		{"UsersByPhoneHashes", 2000, func(n int) error {
			_, err := f.UsersByPhoneHashes(ctx, make([][]byte, n))
			return err
		}},
		{"UsersByXUserIDs", 2000, func(n int) error {
			_, err := f.UsersByXUserIDs(ctx, make([]string, n))
			return err
		}},
		{"MemberWallets", 500, func(n int) error {
			_, err := f.MemberWallets(ctx, ids.UserID{}, n)
			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if err := tc.call(tc.max); err != nil {
				t.Fatalf("%d inputs err = %v, want the limit itself to be accepted", tc.max, err)
			}
			identityCode(t, tc.call(tc.max+1), errs.CodeInvalidInput)
		})
	}
	t.Run("MemberWallets page smaller than one", func(t *testing.T) {
		t.Parallel()
		for _, limit := range []int{0, -1} {
			_, err := f.MemberWallets(ctx, ids.UserID{}, limit)
			identityCode(t, err, errs.CodeInvalidInput)
		}
		if _, err := f.MemberWallets(ctx, ids.UserID{}, 1); err != nil {
			t.Fatalf("a page of one err = %v", err)
		}
	})
}

func identityCalls(t *testing.T) map[string]func(f *fakes.Identity) error {
	t.Helper()
	ctx := t.Context()
	id := identityID(t, testkit.NewIDs(testkit.RandSeed(t)))
	return map[string]func(f *fakes.Identity) error{
		"UsersByID": func(f *fakes.Identity) error {
			_, err := f.UsersByID(ctx, []ids.UserID{id})
			return err
		},
		"UserByHandle": func(f *fakes.Identity) error {
			_, err := f.UserByHandle(ctx, "someone")
			return err
		},
		"UserIDsByHandles": func(f *fakes.Identity) error {
			_, err := f.UserIDsByHandles(ctx, []string{"someone"})
			return err
		},
		"UsersByPhoneHashes": func(f *fakes.Identity) error {
			_, err := f.UsersByPhoneHashes(ctx, [][]byte{identityHash("someone")})
			return err
		},
		"UsersByXUserIDs": func(f *fakes.Identity) error {
			_, err := f.UsersByXUserIDs(ctx, []string{"x-someone"})
			return err
		},
		"MemberWallet": func(f *fakes.Identity) error {
			_, err := f.MemberWallet(ctx, id)
			return err
		},
		"MemberWallets": func(f *fakes.Identity) error {
			_, err := f.MemberWallets(ctx, id, 1)
			return err
		},
	}
}

func identityOthersUnaffected(
	t *testing.T, f *fakes.Identity, calls map[string]func(f *fakes.Identity) error, failing string, down error,
) {
	t.Helper()
	for name, call := range calls {
		if name != failing && errors.Is(call(f), down) {
			t.Errorf("%s failed because %s was told to", name, failing)
		}
	}
}

func TestIdentity_failOnceFailsTheNextCallOfThatMethodOnly(t *testing.T) {
	t.Parallel()
	down := errs.New(errs.CodeDBUnavailable, "test")
	calls := identityCalls(t)
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := fakes.NewIdentity(nil, nil)
			f.FailOnce(name, down)
			identityOthersUnaffected(t, f, calls, name, down)
			if err := call(f); !errors.Is(err, down) {
				t.Fatalf("first call after FailOnce err = %v, want the injected error", err)
			}
			if err := call(f); errors.Is(err, down) {
				t.Fatal("second call after FailOnce failed again")
			}
		})
	}
}

func TestIdentity_failFailsEveryCallOfThatMethodUntilCleared(t *testing.T) {
	t.Parallel()
	down := errs.New(errs.CodeDBUnavailable, "test")
	calls := identityCalls(t)
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := fakes.NewIdentity(nil, nil)
			f.Fail(name, down)
			identityOthersUnaffected(t, f, calls, name, down)
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

func TestIdentity_badInputBeatsAnInjectedFailureAndKeepsItQueued(t *testing.T) {
	t.Parallel()
	down := errs.New(errs.CodeDBUnavailable, "test")
	f := fakes.NewIdentity(nil, nil)
	f.Fail("UserByHandle", down)
	_, err := f.UserByHandle(t.Context(), "bad handle!")
	identityCode(t, err, errs.CodeUserNotFound)
	f.FailOnce("UsersByID", down)
	_, err = f.UsersByID(t.Context(), make([]ids.UserID, identity.MaxUsersByID+1))
	identityCode(t, err, errs.CodeInvalidInput)
	if _, err := f.UsersByID(t.Context(), nil); !errors.Is(err, down) {
		t.Fatalf("UsersByID after the rejected call err = %v, want the queued failure", err)
	}
}

func TestIdentity_isSafeForConcurrentUse(t *testing.T) {
	t.Parallel()
	w := newIdentityWorld(t)
	active := w.users["active"]
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() {
			key := "x-worker-" + strconv.Itoa(i)
			w.fake.SetXUserID(active.ID, key)
			w.fake.SetPhoneHash(active.ID, identityHash(key))
			byX, xErr := w.fake.UsersByXUserIDs(t.Context(), []string{key})
			byPhone, phoneErr := w.fake.UsersByPhoneHashes(t.Context(), [][]byte{identityHash(key)})
			cards, cardErr := w.fake.UsersByID(t.Context(), w.all)
			if xErr != nil || phoneErr != nil || cardErr != nil || byX[key] != active.ID ||
				byPhone[hex.EncodeToString(identityHash(key))] != active.ID || len(cards) != len(w.all) {
				t.Errorf("worker %d saw %v %v %v and %v %v", i, xErr, phoneErr, cardErr, byX, byPhone)
			}
		})
	}
	wg.Wait()
}
