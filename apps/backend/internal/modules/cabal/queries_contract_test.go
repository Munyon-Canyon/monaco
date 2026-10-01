package cabal_test

import (
	"bytes"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type contractMember struct {
	cabalID ids.CabalID
	member  cabal.MemberView
}

type contractCabal struct {
	view   cabal.View
	rules  cabal.Rules
	wallet cabal.TreasuryWallet
}

type contractWorld struct {
	users   []ids.UserID
	cabals  map[string]contractCabal
	members []contractMember
}

func (w contractWorld) id(key string) ids.CabalID { return w.cabals[key].view.ID }

func (w contractWorld) membersOf(key string) []cabal.MemberView {
	var out []cabal.MemberView
	for _, m := range w.members {
		if m.cabalID == w.id(key) {
			out = append(out, m.member)
		}
	}
	return out
}

func compareBytes(a, b [16]byte) int { return bytes.Compare(a[:], b[:]) }

func newContractWorld(t *testing.T) contractWorld {
	t.Helper()
	g := testkit.NewIDs(testkit.RandSeed(t))
	newUser := func() ids.UserID { return ids.NewUserID(g) }
	newCabalID := func() ids.CabalID { return ids.CabalIDFrom(g.NewV7()) }
	t0 := clock.Real{}.Now().UTC().Truncate(time.Microsecond).Add(-72 * time.Hour)
	w := contractWorld{cabals: map[string]contractCabal{}}
	for range 6 {
		w.users = append(w.users, newUser())
	}
	idB, idA, idC, idD, idE := newCabalID(), newCabalID(), newCabalID(), newCabalID(), newCabalID()
	u := w.users
	add := func(key string, id ids.CabalID, name, picture string, creator ids.UserID, status cabal.Status,
		created time.Time, rules cabal.Rules,
	) {
		w.cabals[key] = contractCabal{
			view: cabal.View{
				ID: id, Name: name, PictureURL: picture, CreatorID: creator, Status: status, CreatedAt: created,
			},
			rules: rules,
			wallet: cabal.TreasuryWallet{
				CabalID: id, PrivyWalletID: "treasury-" + key, Address: chain.SolanaAddress("address-" + key),
			},
		}
	}
	day := 24 * time.Hour
	add("a", idA, "Friends pot", "https://pictures.test/a.png", u[0], cabal.StatusActive, t0, cabal.Rules{
		JoinMode: cabal.JoinOpen, VoterMode: cabal.VotersAll, Threshold: cabal.ThresholdMajority,
		ProposalExpiry: day, SlippageBps: 100,
	})
	add("b", idB, "Banned pot", "", u[2], cabal.StatusBanned, t0.Add(2*time.Hour), cabal.Rules{
		JoinMode: cabal.JoinRequest, VoterMode: cabal.VotersList, Threshold: cabal.ThresholdUnanimous,
		ProposalExpiry: 7 * day, SlippageBps: 300,
	})
	small := cabal.Rules{
		JoinMode: cabal.JoinOpen, VoterMode: cabal.VotersAll, Threshold: cabal.ThresholdMajority,
		ProposalExpiry: time.Hour, SlippageBps: 1,
	}
	add("c", idC, "Solo pot", "", u[5], cabal.StatusActive, t0.Add(4*time.Hour), small)
	add("d", idD, "Tied pot one", "", u[3], cabal.StatusActive, t0.Add(5*time.Hour), small)
	add("e", idE, "Tied pot two", "", u[3], cabal.StatusActive, t0.Add(5*time.Hour), small)
	member := func(key string, user ids.UserID, role cabal.Role, canVote bool, joined time.Time) {
		w.members = append(w.members, contractMember{
			cabalID: w.id(key), member: cabal.MemberView{UserID: user, Role: role, CanVote: canVote, JoinedAt: joined},
		})
	}
	low, high := u[1], u[2]
	if compareBytes(low.UUID(), high.UUID()) > 0 {
		low, high = high, low
	}
	member("a", u[0], cabal.RoleCreator, true, t0)
	member("a", low, cabal.RoleMember, true, t0.Add(time.Hour))
	member("a", high, cabal.RoleMember, true, t0.Add(time.Hour))
	member("b", u[2], cabal.RoleCreator, true, t0.Add(2*time.Hour))
	member("b", u[0], cabal.RoleMember, false, t0.Add(3*time.Hour))
	member("b", u[4], cabal.RoleMember, true, t0.Add(4*time.Hour))
	member("b", u[1], cabal.RoleMember, false, t0.Add(4*time.Hour+time.Minute))
	member("c", u[5], cabal.RoleCreator, true, t0.Add(4*time.Hour))
	member("d", u[3], cabal.RoleCreator, true, t0.Add(5*time.Hour))
	member("e", u[3], cabal.RoleCreator, true, t0.Add(5*time.Hour))
	member("d", u[4], cabal.RoleMember, true, t0.Add(6*time.Hour))
	for key, c := range w.cabals {
		c.view.MemberCount = len(w.membersOf(key))
		w.cabals[key] = c
	}
	return w
}

func insertContractUsers(t *testing.T, pool *pgxpool.Pool, users []ids.UserID) {
	t.Helper()
	now := clock.Real{}.Now().UTC()
	for _, id := range users {
		if _, err := pool.Exec(t.Context(), `INSERT INTO users (id, privy_user_id, login_provider,
			auth_state_changed_at, created_at, updated_at) VALUES ($1, $2, 'sms', $3, $3, $3)`,
			id.UUID(), "did:privy:"+id.String(), now); err != nil {
			t.Fatalf("insert user: %v", err)
		}
	}
}

func insertContractCabal(t *testing.T, pool *pgxpool.Pool, c contractCabal, code string) {
	t.Helper()
	if _, err := pool.Exec(t.Context(), `INSERT INTO cabals (id, name, picture_url, creator_id, join_mode,
		voter_mode, threshold, proposal_expiry_seconds, slippage_bps, invite_code, status, created_at, updated_at)
		VALUES ($1, $2, NULLIF($3::text, ''), $4, $5, $6, $7, $8, $9, $10, $11, $12, $12)`,
		c.view.ID.UUID(), c.view.Name, c.view.PictureURL, c.view.CreatorID.UUID(), string(c.rules.JoinMode),
		string(c.rules.VoterMode), string(c.rules.Threshold), int64(c.rules.ProposalExpiry/time.Second),
		c.rules.SlippageBps, code, string(c.view.Status), c.view.CreatedAt); err != nil {
		t.Fatalf("insert cabal %s: %v", c.view.Name, err)
	}
	if _, err := pool.Exec(t.Context(), `INSERT INTO treasury_wallets (cabal_id, privy_wallet_id, address, created_at)
		VALUES ($1, $2, $3, $4)`, c.view.ID.UUID(), c.wallet.PrivyWalletID, string(c.wallet.Address),
		c.view.CreatedAt); err != nil {
		t.Fatalf("insert treasury wallet %s: %v", c.view.Name, err)
	}
}

func seedContractWorld(t *testing.T, pool *pgxpool.Pool, w contractWorld) {
	t.Helper()
	insertContractUsers(t, pool, w.users)
	for i, key := range []string{"a", "b", "c", "d", "e"} {
		insertContractCabal(t, pool, w.cabals[key], fmt.Sprintf("A%09d", i))
	}
	q := sqlc.New(pool)
	for _, m := range slices.Backward(w.members) {
		if _, err := q.InsertMember(t.Context(), sqlc.InsertMemberParams{
			CabalID: m.cabalID.UUID(), UserID: m.member.UserID.UUID(), Role: string(m.member.Role),
			CanVote: m.member.CanVote, JoinedAt: m.member.JoinedAt,
		}); err != nil {
			t.Fatalf("insert member: %v", err)
		}
	}
}

type contractSubject struct {
	name  string
	build func(t *testing.T, w contractWorld) cabal.Queries
}

func contractSubjects() []contractSubject {
	return []contractSubject{
		{name: "postgres", build: func(t *testing.T, w contractWorld) cabal.Queries {
			t.Helper()
			pool := testkit.DB(t)
			seedContractWorld(t, pool, w)
			return cabal.New(module.Deps{Pool: pool}).Queries()
		}},
	}
}

type contractEnv struct {
	t *testing.T
	q cabal.Queries
	w contractWorld
}

func wantNoErr(t *testing.T, what string, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: %v", what, err)
	}
}

func wantView(t *testing.T, what string, got, want cabal.View) {
	t.Helper()
	got.CreatedAt, want.CreatedAt = time.Time{}, time.Time{}
	if got != want {
		t.Errorf("%s = %+v, want %+v", what, got, want)
	}
}

func wantSameInstant(t *testing.T, what string, got, want time.Time) {
	t.Helper()
	if !got.Equal(want) || got.Location() != time.UTC {
		t.Errorf("%s = %s (%s), want %s in UTC", what, got, got.Location(), want)
	}
}

func wantMembers(t *testing.T, what string, got, want []cabal.MemberView) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s = %+v, want %+v", what, got, want)
		return
	}
	for i := range want {
		at := got[i].JoinedAt
		got[i].JoinedAt = want[i].JoinedAt
		if got[i] != want[i] {
			t.Errorf("%s[%d] = %+v, want %+v", what, i, got[i], want[i])
		}
		wantSameInstant(t, what+" joined_at", at, want[i].JoinedAt)
	}
}

func membersWhere(ms []cabal.MemberView, keep func(cabal.MemberView) bool) []ids.UserID {
	out := []ids.UserID{}
	for _, m := range ms {
		if keep(m) {
			out = append(out, m.UserID)
		}
	}
	return out
}

type contractCase struct {
	name string
	run  func(e contractEnv)
}

func contractCases() []contractCase {
	return []contractCase{
		{"Cabal returns every field of the view", checkCabalView},
		{"Cabal on an unknown id is CabalNotFound", checkCabalUnknown},
		{"Cabals returns the known ids and omits the rest", checkCabalsBatch},
		{"Cabals of nothing is an empty map", checkCabalsEmpty},
		{"IsMember is true only for a member of that cabal", checkIsMember},
		{"Member returns the role, vote and join time", checkMember},
		{"Member on a non-member is NotCabalMember", checkMemberAbsent},
		{"Members come back by join time and then user id", checkMembers},
		{"Members of an unknown cabal is an empty list", checkMembersUnknown},
		{"VoterSet is the members who can vote in join order", checkVoterSet},
		{"Rules carries the join, voter, threshold, expiry and slippage rules", checkRules},
		{"SlippageBps reads the cabal's slippage", checkSlippage},
		{"Status is active or banned", checkStatus},
		{"TreasuryWallet returns the wallet of that cabal", checkTreasuryWallet},
		{"TreasuryWallets lists every cabal's wallet by cabal id", checkTreasuryWallets},
		{"CabalsOf lists a user's cabals by join time and then cabal id", checkCabalsOf},
	}
}

func contractKeys() []string { return []string{"a", "b", "c"} }

func checkCabalView(e contractEnv) {
	for _, key := range contractKeys() {
		got, err := e.q.Cabal(e.t.Context(), e.w.id(key))
		wantNoErr(e.t, "Cabal "+key, err)
		wantView(e.t, "Cabal "+key, got, e.w.cabals[key].view)
		wantSameInstant(e.t, "Cabal "+key+" created_at", got.CreatedAt, e.w.cabals[key].view.CreatedAt)
	}
}

func checkCabalUnknown(e contractEnv) {
	_, err := e.q.Cabal(e.t.Context(), ids.CabalID{})
	wantCode(e.t, "Cabal", err, errs.CodeCabalNotFound)
}

func checkCabalsBatch(e contractEnv) {
	got, err := e.q.Cabals(e.t.Context(), []ids.CabalID{e.w.id("b"), {}, e.w.id("a"), e.w.id("b")})
	wantNoErr(e.t, "Cabals", err)
	if len(got) != 2 {
		e.t.Fatalf("Cabals returned %d cabals, want a and b only: %+v", len(got), got)
	}
	for _, key := range []string{"a", "b"} {
		wantView(e.t, "Cabals "+key, got[e.w.id(key)], e.w.cabals[key].view)
	}
}

func checkCabalsEmpty(e contractEnv) {
	for _, asked := range [][]ids.CabalID{nil, {}} {
		got, err := e.q.Cabals(e.t.Context(), asked)
		if err != nil || got == nil || len(got) != 0 {
			e.t.Errorf("Cabals(%v) = %v, %v, want an empty map", asked, got, err)
		}
	}
}

func checkIsMember(e contractEnv) {
	for _, tc := range []struct {
		cabal string
		user  int
		want  bool
	}{{"a", 0, true}, {"a", 2, true}, {"a", 3, false}, {"b", 0, true}, {"b", 5, false}, {"c", 0, false}} {
		got, err := e.q.IsMember(e.t.Context(), e.w.id(tc.cabal), e.w.users[tc.user])
		if err != nil || got != tc.want {
			e.t.Errorf("IsMember(%s, user %d) = %v, %v, want %v", tc.cabal, tc.user, got, err, tc.want)
		}
	}
	got, err := e.q.IsMember(e.t.Context(), ids.CabalID{}, e.w.users[0])
	if err != nil || got {
		e.t.Errorf("IsMember(unknown cabal) = %v, %v, want false", got, err)
	}
}

func checkMember(e contractEnv) {
	for _, m := range e.w.members {
		got, err := e.q.Member(e.t.Context(), m.cabalID, m.member.UserID)
		wantNoErr(e.t, "Member", err)
		wantMembers(e.t, "Member", []cabal.MemberView{got}, []cabal.MemberView{m.member})
	}
}

func checkMemberAbsent(e contractEnv) {
	_, err := e.q.Member(e.t.Context(), e.w.id("a"), e.w.users[3])
	wantCode(e.t, "Member of a stranger", err, errs.CodeNotCabalMember)
	_, err = e.q.Member(e.t.Context(), ids.CabalID{}, e.w.users[0])
	wantCode(e.t, "Member of an unknown cabal", err, errs.CodeNotCabalMember)
}

func checkMembers(e contractEnv) {
	for _, key := range contractKeys() {
		got, err := e.q.Members(e.t.Context(), e.w.id(key))
		wantNoErr(e.t, "Members "+key, err)
		wantMembers(e.t, "Members "+key, got, e.w.membersOf(key))
	}
}

func checkMembersUnknown(e contractEnv) {
	got, err := e.q.Members(e.t.Context(), ids.CabalID{})
	if err != nil || got == nil || len(got) != 0 {
		e.t.Errorf("Members(unknown) = %v, %v, want an empty list", got, err)
	}
}

func checkVoterSet(e contractEnv) {
	for _, key := range contractKeys() {
		got, err := e.q.VoterSet(e.t.Context(), e.w.id(key))
		want := membersWhere(e.w.membersOf(key), func(m cabal.MemberView) bool { return m.CanVote })
		if err != nil || !slices.Equal(got, want) {
			e.t.Errorf("VoterSet %s = %v, %v, want %v", key, got, err, want)
		}
	}
	got, err := e.q.VoterSet(e.t.Context(), ids.CabalID{})
	if err != nil || got == nil || len(got) != 0 {
		e.t.Errorf("VoterSet(unknown) = %v, %v, want an empty list", got, err)
	}
}

func checkRules(e contractEnv) {
	for _, key := range contractKeys() {
		got, err := e.q.Rules(e.t.Context(), e.w.id(key))
		if err != nil || got != e.w.cabals[key].rules {
			e.t.Errorf("Rules %s = %+v, %v, want %+v", key, got, err, e.w.cabals[key].rules)
		}
	}
	_, err := e.q.Rules(e.t.Context(), ids.CabalID{})
	wantCode(e.t, "Rules", err, errs.CodeCabalNotFound)
}

func checkSlippage(e contractEnv) {
	for _, key := range contractKeys() {
		got, err := e.q.SlippageBps(e.t.Context(), e.w.id(key))
		if err != nil || got != e.w.cabals[key].rules.SlippageBps {
			e.t.Errorf("SlippageBps %s = %d, %v, want %d", key, got, err, e.w.cabals[key].rules.SlippageBps)
		}
	}
	_, err := e.q.SlippageBps(e.t.Context(), ids.CabalID{})
	wantCode(e.t, "SlippageBps", err, errs.CodeCabalNotFound)
}

func checkStatus(e contractEnv) {
	for _, key := range []string{"a", "b"} {
		got, err := e.q.Status(e.t.Context(), e.w.id(key))
		if err != nil || got != e.w.cabals[key].view.Status {
			e.t.Errorf("Status %s = %q, %v, want %q", key, got, err, e.w.cabals[key].view.Status)
		}
	}
	_, err := e.q.Status(e.t.Context(), ids.CabalID{})
	wantCode(e.t, "Status", err, errs.CodeCabalNotFound)
}

func checkTreasuryWallet(e contractEnv) {
	for _, key := range contractKeys() {
		got, err := e.q.TreasuryWallet(e.t.Context(), e.w.id(key))
		if err != nil || got != e.w.cabals[key].wallet {
			e.t.Errorf("TreasuryWallet %s = %+v, %v, want %+v", key, got, err, e.w.cabals[key].wallet)
		}
	}
	_, err := e.q.TreasuryWallet(e.t.Context(), ids.CabalID{})
	wantCode(e.t, "TreasuryWallet", err, errs.CodeCabalNotFound)
}

func checkTreasuryWallets(e contractEnv) {
	got, err := e.q.TreasuryWallets(e.t.Context())
	wantNoErr(e.t, "TreasuryWallets", err)
	want := make([]cabal.TreasuryWallet, 0, len(e.w.cabals))
	for _, c := range e.w.cabals {
		want = append(want, c.wallet)
	}
	slices.SortFunc(want, func(a, b cabal.TreasuryWallet) int {
		return compareBytes(a.CabalID.UUID(), b.CabalID.UUID())
	})
	if !slices.Equal(got, want) {
		e.t.Errorf("TreasuryWallets = %+v, want %+v", got, want)
	}
}

func checkCabalsOf(e contractEnv) {
	wantByUser := map[int][]string{0: {"a", "b"}, 3: {"d", "e"}, 4: {"b", "d"}, 5: {"c"}}
	for user, keys := range wantByUser {
		want := make([]ids.CabalID, len(keys))
		for i, key := range keys {
			want[i] = e.w.id(key)
		}
		if user == 3 {
			slices.SortFunc(want, func(a, b ids.CabalID) int { return compareBytes(a.UUID(), b.UUID()) })
		}
		got, err := e.q.CabalsOf(e.t.Context(), e.w.users[user])
		if err != nil || !slices.Equal(got, want) {
			e.t.Errorf("CabalsOf(user %d) = %v, %v, want %v", user, got, err, want)
		}
	}
	got, err := e.q.CabalsOf(e.t.Context(), ids.UserID{})
	if err != nil || got == nil || len(got) != 0 {
		e.t.Errorf("CabalsOf(unknown user) = %v, %v, want an empty list", got, err)
	}
}

func TestQueriesContract(t *testing.T) {
	t.Parallel()
	for _, s := range contractSubjects() {
		for _, tc := range contractCases() {
			t.Run(s.name+"/"+tc.name, func(t *testing.T) {
				t.Parallel()
				w := newContractWorld(t)
				tc.run(contractEnv{t: t, q: s.build(t, w), w: w})
			})
		}
	}
}
