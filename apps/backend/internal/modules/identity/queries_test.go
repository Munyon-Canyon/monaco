package identity_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/adapters"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type portFixture struct {
	pool *pgxpool.Pool
	port identity.Queries
	ids  *testkit.IDs
	now  time.Time
}

func newPortFixture(t *testing.T) portFixture {
	t.Helper()
	pool := testkit.DB(t)
	return portFixture{
		pool: pool,
		port: adapters.NewQueries(pool),
		ids:  testkit.NewIDs(testkit.RandSeed(t)),
		now:  clock.Real{}.Now().UTC().Truncate(time.Microsecond),
	}
}

func (f portFixture) newID(t *testing.T) ids.UserID {
	t.Helper()
	id, err := ids.ParseUserID(f.ids.NewV7().String())
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (f portFixture) created() time.Time { return f.now.Add(-72 * time.Hour) }

type portSeed struct {
	handle, name, photo, authState, status, xUserID string
	phoneHash                                       []byte
	phoneVerified, wallet, softDeleted              bool
	firstDeposit                                    *time.Time
}

func (f portFixture) seed(t *testing.T, s portSeed) testkit.SeededUser {
	t.Helper()
	u := testkit.SeedUser(t, f.pool, testkit.UserOpts{
		Handle: s.handle, AuthState: s.authState, AccountStatus: s.status, WithWallet: s.wallet,
	})
	var verifiedAt, deletedAt *time.Time
	if s.phoneVerified {
		verifiedAt = &f.now
	}
	if s.softDeleted {
		deletedAt = &f.now
	}
	if _, err := f.pool.Exec(t.Context(), `UPDATE users SET display_name = $2, photo_url = NULLIF($3::text, ''),
		phone_hash = $4, phone_verified_at = $5, x_user_id = NULLIF($6::text, ''), first_deposit_at = $7,
		created_at = $8, deleted_at = COALESCE(deleted_at, $9) WHERE id = $1`,
		u.ID.UUID(), s.name, s.photo, s.phoneHash, verifiedAt, s.xUserID, s.firstDeposit, f.created(),
		deletedAt); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return u
}

func (f portFixture) insertWalletUser(t *testing.T, id ids.UserID) {
	t.Helper()
	key := sha256.Sum256([]byte(id.String()))
	if _, err := f.pool.Exec(t.Context(), `INSERT INTO users (id, privy_user_id, login_provider, auth_state_changed_at,
		created_at, updated_at) VALUES ($1, $2, 'sms', $3, $3, $3)`,
		id.UUID(), "did:privy:"+id.String(), f.now); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, err := f.pool.Exec(t.Context(), `INSERT INTO user_wallets (user_id, privy_wallet_id, address, created_at)
		VALUES ($1, $2, $3, $4)`, id.UUID(), "wallet-"+id.String(), string(chain.AddressOf(key[:])), f.now); err != nil {
		t.Fatalf("insert wallet: %v", err)
	}
}

func portHash(label string) []byte {
	sum := sha256.Sum256([]byte(label))
	return sum[:]
}

func portOK(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("port call: %v", err)
	}
}

func portFound(t *testing.T, err error) bool {
	t.Helper()
	if err == nil {
		return true
	}
	wantCode(t, err, errs.CodeUserNotFound)
	return false
}

func portSameCard(t *testing.T, got, want identity.UserCard) {
	t.Helper()
	deposit := func(c identity.UserCard) string {
		if c.FirstDepositAt == nil {
			return "none"
		}
		return c.FirstDepositAt.Format(time.RFC3339Nano) + " " + c.FirstDepositAt.Location().String()
	}
	gotDeposit, wantDeposit := deposit(got), deposit(want)
	gotAt, wantAt := got.CreatedAt, want.CreatedAt
	got.FirstDepositAt, want.FirstDepositAt, got.CreatedAt, want.CreatedAt = nil, nil, time.Time{}, time.Time{}
	if got != want || gotDeposit != wantDeposit || !gotAt.Equal(wantAt) || gotAt.Location() != time.UTC {
		t.Fatalf("card = %+v created %s (%s) first deposit %s, want %+v created %s first deposit %s",
			got, gotAt, gotAt.Location(), gotDeposit, want, wantAt, wantDeposit)
	}
}

func TestQueries_usersByIDCarriesEveryFieldOfTheCard(t *testing.T) {
	t.Parallel()
	f := newPortFixture(t)
	deposit := f.now.Add(-24 * time.Hour)
	full := f.seed(t, portSeed{
		handle: "kaicenat", name: "Kai Cenat", photo: "https://img.example/kai.png", authState: "ONBOARDING_COMPLETED",
		status: "suspended", phoneHash: portHash("kai"), phoneVerified: true, xUserID: "x-kai", firstDeposit: &deposit,
	})
	bare := f.seed(t, portSeed{})
	cards, err := f.port.UsersByID(t.Context(), []ids.UserID{full.ID, f.newID(t), bare.ID, full.ID})
	portOK(t, err)
	if len(cards) != 2 {
		t.Fatalf("UsersByID returned %d cards, want one per known user", len(cards))
	}
	portSameCard(t, cards[full.ID], identity.UserCard{
		ID: full.ID, Handle: "kaicenat", DisplayName: "Kai Cenat", PhotoURL: "https://img.example/kai.png",
		AuthState: identity.AuthOnboardingCompleted, AccountStatus: identity.AccountSuspended, PhoneVerified: true,
		XLinked: true, CreatedAt: f.created(), FirstDepositAt: &deposit,
	})
	portSameCard(t, cards[bare.ID], identity.UserCard{
		ID: bare.ID, AuthState: identity.AuthCreated, AccountStatus: identity.AccountActive, CreatedAt: f.created(),
	})
}

func TestQueries_theDisplayNameFallsBackToTheHandle(t *testing.T) {
	t.Parallel()
	f := newPortFixture(t)
	named := f.seed(t, portSeed{handle: "named_one", name: "Real Name"})
	quiet := f.seed(t, portSeed{handle: "quiet_one"})
	cards, err := f.port.UsersByID(t.Context(), []ids.UserID{named.ID, quiet.ID})
	portOK(t, err)
	if got := cards[named.ID].DisplayName; got != "Real Name" {
		t.Fatalf("DisplayName of a named user = %q, want their name", got)
	}
	if got := cards[quiet.ID].DisplayName; got != "quiet_one" {
		t.Fatalf("DisplayName of a user without a name = %q, want their handle", got)
	}
	byHandle, err := f.port.UserByHandle(t.Context(), "QUIET_one")
	portOK(t, err)
	if byHandle.DisplayName != "quiet_one" {
		t.Fatalf("UserByHandle DisplayName = %q, want the handle", byHandle.DisplayName)
	}
}

type portKinds struct {
	names   []string
	users   map[string]testkit.SeededUser
	kindOf  map[ids.UserID]string
	all     []ids.UserID
	handles []string
	hashes  [][]byte
	xIDs    []string
}

func seedPortKinds(t *testing.T, f portFixture) portKinds {
	t.Helper()
	k := portKinds{
		names:  []string{"active", "suspended", "banned", "deleted", "soft_deleted"},
		users:  map[string]testkit.SeededUser{},
		kindOf: map[ids.UserID]string{},
	}
	for _, kind := range k.names {
		status := kind
		if kind == "soft_deleted" {
			status = "active"
		}
		u := f.seed(t, portSeed{
			handle: "user_" + kind, name: "Name " + kind, photo: "https://img.example/" + kind + ".png",
			status: status, softDeleted: kind == "soft_deleted", phoneHash: portHash(kind), phoneVerified: true,
			xUserID: "x-" + kind, wallet: true,
		})
		k.users[kind], k.kindOf[u.ID] = u, kind
		k.all, k.handles = append(k.all, u.ID), append(k.handles, "user_"+kind)
		k.hashes, k.xIDs = append(k.hashes, portHash(kind)), append(k.xIDs, "x-"+kind)
	}
	return k
}

func (k portKinds) visible(seen func(ids.UserID) bool) []string {
	var out []string
	for _, kind := range k.names {
		if seen(k.users[kind].ID) {
			out = append(out, kind)
		}
	}
	return out
}

func portVisibleByMethod(
	t *testing.T,
	f portFixture,
	k portKinds,
) (map[string][]string, map[ids.UserID]identity.UserCard) {
	t.Helper()
	ctx := t.Context()
	got := map[string][]string{}
	cards, err := f.port.UsersByID(ctx, k.all)
	portOK(t, err)
	got["UsersByID"] = k.visible(func(id ids.UserID) bool { _, ok := cards[id]; return ok })
	got["UserByHandle"] = k.visible(func(id ids.UserID) bool {
		_, err := f.port.UserByHandle(ctx, "user_"+k.kindOf[id])
		return portFound(t, err)
	})
	byHandles, err := f.port.UserIDsByHandles(ctx, k.handles)
	portOK(t, err)
	got["UserIDsByHandles"] = k.visible(func(id ids.UserID) bool { return byHandles["user_"+k.kindOf[id]] == id })
	byPhone, err := f.port.UsersByPhoneHashes(ctx, k.hashes)
	portOK(t, err)
	got["UsersByPhoneHashes"] = k.visible(func(id ids.UserID) bool {
		return byPhone[hex.EncodeToString(portHash(k.kindOf[id]))] == id
	})
	byX, err := f.port.UsersByXUserIDs(ctx, k.xIDs)
	portOK(t, err)
	got["UsersByXUserIDs"] = k.visible(func(id ids.UserID) bool { return byX["x-"+k.kindOf[id]] == id })
	got["MemberWallet"] = k.visible(func(id ids.UserID) bool {
		_, err := f.port.MemberWallet(ctx, id)
		return portFound(t, err)
	})
	page, err := f.port.MemberWallets(ctx, ids.UserID{}, identity.MaxWalletPage)
	portOK(t, err)
	got["MemberWallets"] = k.visible(func(id ids.UserID) bool {
		return slices.ContainsFunc(page, func(w identity.MemberWallet) bool { return w.UserID == id })
	})
	return got, cards
}

func TestQueries_eachMethodShowsOnlyTheUsersItsRuleAllows(t *testing.T) {
	t.Parallel()
	f := newPortFixture(t)
	k := seedPortKinds(t, f)
	got, cards := portVisibleByMethod(t, f, k)
	want := map[string][]string{
		"UsersByID":          {"active", "suspended", "banned", "deleted", "soft_deleted"},
		"UserByHandle":       {"active", "suspended", "banned"},
		"UserIDsByHandles":   {"active", "suspended", "banned"},
		"UsersByPhoneHashes": {"active"},
		"UsersByXUserIDs":    {"active"},
		"MemberWallet":       {"active", "suspended", "banned"},
		"MemberWallets":      {"active", "suspended", "banned"},
	}
	if !maps.EqualFunc(got, want, slices.Equal) {
		t.Fatalf("users visible per method = %v, want %v", got, want)
	}
	for _, kind := range []string{"deleted", "soft_deleted"} {
		if gone := cards[k.users[kind].ID]; !gone.Deleted || gone.Handle != "" || gone.DisplayName != "" ||
			gone.PhotoURL != "" {
			t.Fatalf("%s user card = %+v, want Deleted with no handle, name or photo", kind, gone)
		}
	}
	if status := cards[k.users["deleted"].ID].AccountStatus; status != identity.AccountDeleted {
		t.Fatalf("deleted user status = %q, want deleted", status)
	}
	if live := cards[k.users["banned"].ID]; live.Deleted || live.DisplayName != "Name banned" || live.PhotoURL == "" {
		t.Fatalf("banned user card = %+v, want the name and photo kept", live)
	}
}

func TestQueries_anUnverifiedPhoneIsNeverMatched(t *testing.T) {
	t.Parallel()
	f := newPortFixture(t)
	verified := f.seed(t, portSeed{phoneHash: portHash("verified"), phoneVerified: true, xUserID: "x-verified"})
	f.seed(t, portSeed{phoneHash: portHash("unverified")})
	f.seed(t, portSeed{})
	asked := [][]byte{
		portHash("verified"), portHash("unverified"), portHash("unknown"), []byte("short"), {}, nil,
	}
	got, err := f.port.UsersByPhoneHashes(t.Context(), asked)
	portOK(t, err)
	if want := map[string]ids.UserID{hex.EncodeToString(portHash("verified")): verified.ID}; !maps.Equal(got, want) {
		t.Fatalf("UsersByPhoneHashes = %v, want only the verified phone under its hex key %v", got, want)
	}
	byX, err := f.port.UsersByXUserIDs(t.Context(), []string{"x-verified", "x-unknown", ""})
	portOK(t, err)
	if want := map[string]ids.UserID{"x-verified": verified.ID}; !maps.Equal(byX, want) {
		t.Fatalf("UsersByXUserIDs = %v, want %v", byX, want)
	}
}

func TestQueries_handlesMatchInAnyCaseAndNeverMatchAMalformedOne(t *testing.T) {
	t.Parallel()
	f := newPortFixture(t)
	kai := f.seed(t, portSeed{handle: "kaicenat"})
	f.seed(t, portSeed{handle: "speed"})
	ctx := t.Context()
	card, err := f.port.UserByHandle(ctx, "KaiCenat")
	portOK(t, err)
	if card.ID != kai.ID || card.Handle != "kaicenat" {
		t.Fatalf("UserByHandle(KaiCenat) = %+v, want the user whose handle is kaicenat", card)
	}
	misses := []string{"nobody_here", "ab", "bad handle!", "", "kaïcenat", "kaicenat_way_too_long_handle"}
	for _, handle := range misses {
		_, err := f.port.UserByHandle(ctx, handle)
		wantCode(t, err, errs.CodeUserNotFound)
	}
	found, err := f.port.UserIDsByHandles(ctx, append([]string{"KAICENAT", "kaicenat"}, misses...))
	portOK(t, err)
	if want := (map[string]ids.UserID{"KAICENAT": kai.ID, "kaicenat": kai.ID}); !maps.Equal(found, want) {
		t.Fatalf("UserIDsByHandles = %v, want each spelling asked for mapped to the user %v", found, want)
	}
}

func TestQueries_emptyInputMatchesNobodyAndIsNotAnError(t *testing.T) {
	t.Parallel()
	f := newPortFixture(t)
	f.seed(t, portSeed{handle: "someone", phoneHash: portHash("someone"), phoneVerified: true, xUserID: "x-someone"})
	ctx := t.Context()
	byID, idErr := f.port.UsersByID(ctx, nil)
	byHandle, handleErr := f.port.UserIDsByHandles(ctx, nil)
	byPhone, phoneErr := f.port.UsersByPhoneHashes(ctx, nil)
	byX, xErr := f.port.UsersByXUserIDs(ctx, nil)
	if err := errors.Join(idErr, handleErr, phoneErr, xErr); err != nil {
		t.Fatalf("empty lookups failed: %v", err)
	}
	if len(byID)+len(byHandle)+len(byPhone)+len(byX) != 0 {
		t.Fatalf("empty lookups matched %v %v %v %v", byID, byHandle, byPhone, byX)
	}
}

func TestQueries_memberWalletReadsTheWalletOfAKnownUser(t *testing.T) {
	t.Parallel()
	f := newPortFixture(t)
	owner := f.seed(t, portSeed{wallet: true})
	noWallet := f.seed(t, portSeed{})
	ctx := t.Context()
	want := identity.MemberWallet{UserID: owner.ID, PrivyWalletID: owner.PrivyWalletID, Address: owner.Address}
	if got, err := f.port.MemberWallet(ctx, owner.ID); err != nil || got != want {
		t.Fatalf("MemberWallet = %+v, %v, want %+v", got, err, want)
	}
	if page, err := f.port.MemberWallets(
		ctx,
		ids.UserID{},
		10,
	); err != nil ||
		!slices.Equal(page, []identity.MemberWallet{want}) {
		t.Fatalf("MemberWallets = %+v, %v, want only %+v", page, err, want)
	}
	for name, id := range map[string]ids.UserID{"a user without a wallet": noWallet.ID, "an unknown user": f.newID(t)} {
		_, err := f.port.MemberWallet(ctx, id)
		if errs.CodeOf(err) != errs.CodeUserNotFound {
			t.Errorf("MemberWallet of %s err = %v, want user_not_found", name, err)
		}
	}
}

func TestQueries_walletPagingReturnsEveryWalletOnceWhileUsersAreInserted(t *testing.T) {
	t.Parallel()
	f := newPortFixture(t)
	ctx := t.Context()
	const pageSize = 3
	initial := make([]ids.UserID, 0, 7)
	for _, status := range []string{"active", "suspended", "banned", "active", "banned", "suspended", "active"} {
		initial = append(initial, f.seed(t, portSeed{status: status, wallet: true}).ID)
	}
	f.seed(t, portSeed{status: "deleted", wallet: true})
	f.seed(t, portSeed{})

	var (
		seen, ahead []ids.UserID
		behind      = f.newID(t)
		after       ids.UserID
	)
	for pageNumber := 1; ; pageNumber++ {
		page, err := f.port.MemberWallets(ctx, after, pageSize)
		portOK(t, err)
		for _, w := range page {
			seen = append(seen, w.UserID)
		}
		if len(page) < pageSize {
			break
		}
		after = page[len(page)-1].UserID
		switch pageNumber {
		case 1:
			ahead = append(ahead, f.seed(t, portSeed{wallet: true}).ID, f.seed(t, portSeed{wallet: true}).ID)
		case 2:
			f.insertWalletUser(t, behind)
		}
	}
	if want := append(slices.Clone(initial), ahead...); !slices.Equal(seen, want) {
		t.Fatalf("paged wallets = %v, want every wallet once in id order %v", seen, want)
	}
	fresh, err := f.port.MemberWallets(ctx, ids.UserID{}, identity.MaxWalletPage)
	portOK(t, err)
	if len(fresh) != len(seen)+1 || fresh[0].UserID != behind {
		t.Fatalf("a fresh scan returned %d wallets starting at %v, want %d starting at the late insert %v",
			len(fresh), fresh[0].UserID, len(seen)+1, behind)
	}
}

func TestQueries_overLimitInputIsInvalidInput(t *testing.T) {
	t.Parallel()
	f := newPortFixture(t)
	ctx := t.Context()
	fill := func(n int, item func(i int) string) []string {
		out := make([]string, n)
		for i := range out {
			out[i] = item(i)
		}
		return out
	}
	for _, tc := range []struct {
		name string
		max  int
		call func(t *testing.T, n int) error
	}{
		{"UsersByID", 500, func(t *testing.T, n int) error {
			t.Helper()
			asked := make([]ids.UserID, n)
			for i := range asked {
				asked[i] = f.newID(t)
			}
			_, err := f.port.UsersByID(ctx, asked)
			return err
		}},
		{"UserIDsByHandles", 50, func(_ *testing.T, n int) error {
			_, err := f.port.UserIDsByHandles(ctx, fill(n, func(i int) string { return fmt.Sprintf("user_%04d", i) }))
			return err
		}},
		{"UsersByPhoneHashes", 2000, func(_ *testing.T, n int) error {
			asked := make([][]byte, n)
			for i := range asked {
				asked[i] = portHash(strconv.Itoa(i))
			}
			_, err := f.port.UsersByPhoneHashes(ctx, asked)
			return err
		}},
		{"UsersByXUserIDs", 2000, func(_ *testing.T, n int) error {
			_, err := f.port.UsersByXUserIDs(ctx, fill(n, func(i int) string { return "x-" + strconv.Itoa(i) }))
			return err
		}},
		{"MemberWallets", 500, func(_ *testing.T, n int) error {
			_, err := f.port.MemberWallets(ctx, ids.UserID{}, n)
			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if err := tc.call(t, tc.max); err != nil {
				t.Fatalf("%d inputs err = %v, want the limit itself to be accepted", tc.max, err)
			}
			wantCode(t, tc.call(t, tc.max+1), errs.CodeInvalidInput)
		})
	}
	t.Run("MemberWallets page smaller than one", func(t *testing.T) {
		t.Parallel()
		for _, limit := range []int{0, -1} {
			_, err := f.port.MemberWallets(ctx, ids.UserID{}, limit)
			wantCode(t, err, errs.CodeInvalidInput)
		}
		if _, err := f.port.MemberWallets(ctx, ids.UserID{}, 1); err != nil {
			t.Fatalf("a page of one err = %v", err)
		}
	})
}

func TestQueries_everyMethodIsOneRoundTrip(t *testing.T) {
	t.Parallel()
	f := newPortFixture(t)
	ctx := t.Context()
	u := f.seed(t, portSeed{
		handle: "counted", phoneHash: portHash("counted"), phoneVerified: true, xUserID: "x-counted", wallet: true,
	})
	asked := []ids.UserID{u.ID}
	for len(asked) < identity.MaxUsersByID {
		asked = append(asked, f.newID(t))
	}
	handles, xIDs := []string{"counted"}, []string{"x-counted"}
	hashes := [][]byte{portHash("counted")}
	for i := 1; i < identity.MaxPhoneHashes; i++ {
		hashes, xIDs = append(hashes, portHash(strconv.Itoa(i))), append(xIDs, "x-"+strconv.Itoa(i))
		if i < identity.MaxHandles {
			handles = append(handles, fmt.Sprintf("user_%04d", i))
		}
	}
	testkit.AssertQueries(t, "UsersByID 500 ids", func() {
		cards, err := f.port.UsersByID(ctx, asked)
		portOK(t, err)
		if len(cards) != 1 {
			t.Fatalf("UsersByID returned %d cards, want the one known user", len(cards))
		}
	})
	testkit.AssertQueries(t, "UserByHandle", func() {
		_, err := f.port.UserByHandle(ctx, "counted")
		portOK(t, err)
	})
	testkit.AssertQueries(t, "UserIDsByHandles 50 handles", func() {
		found, err := f.port.UserIDsByHandles(ctx, handles)
		portOK(t, err)
		if len(found) != 1 {
			t.Fatalf("UserIDsByHandles matched %d handles, want the one known handle", len(found))
		}
	})
	testkit.AssertQueries(t, "UsersByPhoneHashes 2000 hashes", func() {
		found, err := f.port.UsersByPhoneHashes(ctx, hashes)
		portOK(t, err)
		if len(found) != 1 {
			t.Fatalf("UsersByPhoneHashes matched %d hashes, want the one known hash", len(found))
		}
	})
	testkit.AssertQueries(t, "UsersByXUserIDs 2000 ids", func() {
		found, err := f.port.UsersByXUserIDs(ctx, xIDs)
		portOK(t, err)
		if len(found) != 1 {
			t.Fatalf("UsersByXUserIDs matched %d ids, want the one known id", len(found))
		}
	})
	testkit.AssertQueries(t, "MemberWallet", func() {
		_, err := f.port.MemberWallet(ctx, u.ID)
		portOK(t, err)
	})
	testkit.AssertQueries(t, "MemberWallets page of 500", func() {
		page, err := f.port.MemberWallets(ctx, ids.UserID{}, identity.MaxWalletPage)
		portOK(t, err)
		if len(page) != 1 {
			t.Fatalf("MemberWallets returned %d wallets, want the one known wallet", len(page))
		}
	})
}

func TestQueries_aRowWhoseIDIsNotV7IsDecodeFailed(t *testing.T) {
	t.Parallel()
	f := newPortFixture(t)
	ctx := t.Context()
	var v4 uuid.UUID
	if err := f.pool.QueryRow(ctx, `INSERT INTO users (id, privy_user_id, handle, login_provider, phone_hash,
		phone_verified_at, x_user_id, auth_state_changed_at, created_at, updated_at)
		VALUES (gen_random_uuid(), 'did:privy:v4', 'v4_user', 'sms', $1, $2, 'x-v4', $2, $2, $2) RETURNING id`,
		portHash("v4"), f.now).Scan(&v4); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO user_wallets (user_id, privy_wallet_id, address, created_at)
		VALUES ($1, 'wallet-v4', $2, $3)`, v4, string(chain.AddressOf(portHash("v4"))), f.now); err != nil {
		t.Fatal(err)
	}
	_, byHandle := f.port.UserByHandle(ctx, "v4_user")
	_, byHandles := f.port.UserIDsByHandles(ctx, []string{"v4_user"})
	_, byPhone := f.port.UsersByPhoneHashes(ctx, [][]byte{portHash("v4")})
	_, byX := f.port.UsersByXUserIDs(ctx, []string{"x-v4"})
	_, wallets := f.port.MemberWallets(ctx, ids.UserID{}, 10)
	for name, err := range map[string]error{
		"UserByHandle": byHandle, "UserIDsByHandles": byHandles, "UsersByPhoneHashes": byPhone,
		"UsersByXUserIDs": byX, "MemberWallets": wallets,
	} {
		if errs.CodeOf(err) != errs.CodeDecodeFailed {
			t.Errorf("%s err = %v, want decode_failed", name, err)
		}
	}
}

func TestQueries_databaseFailuresAreInternalAndKeepTheirCause(t *testing.T) {
	t.Parallel()
	f := newPortFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	id := f.newID(t)
	_, byID := f.port.UsersByID(ctx, []ids.UserID{id})
	_, byHandle := f.port.UserByHandle(ctx, "someone")
	_, byHandles := f.port.UserIDsByHandles(ctx, []string{"someone"})
	_, byPhone := f.port.UsersByPhoneHashes(ctx, [][]byte{portHash("someone")})
	_, byX := f.port.UsersByXUserIDs(ctx, []string{"x-someone"})
	_, wallet := f.port.MemberWallet(ctx, id)
	_, wallets := f.port.MemberWallets(ctx, id, 1)
	for name, err := range map[string]error{
		"UsersByID": byID, "UserByHandle": byHandle, "UserIDsByHandles": byHandles, "UsersByPhoneHashes": byPhone,
		"UsersByXUserIDs": byX, "MemberWallet": wallet, "MemberWallets": wallets,
	} {
		if errs.CodeOf(err) != errs.CodeInternal || !errors.Is(err, context.Canceled) {
			t.Errorf("%s err = %v, want internal wrapping the context error", name, err)
		}
	}
}

func TestQueries_theModuleServesThePortOverItsOwnPool(t *testing.T) {
	t.Parallel()
	f := newPortFixture(t)
	seeded := f.seed(t, portSeed{handle: "module_reader"})
	card, err := identity.New(module.Deps{Pool: f.pool}).Queries().UserByHandle(t.Context(), "module_reader")
	portOK(t, err)
	if card.ID != seeded.ID {
		t.Fatalf("Module.Queries().UserByHandle = %+v, want the user seeded in the module's pool %v", card, seeded.ID)
	}
}
