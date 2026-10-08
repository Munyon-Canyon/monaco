package cabal_test

import (
	"context"
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/adapters"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/cabalapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type storedCabal struct {
	name, joinMode, voterMode, threshold string
	expiry, slippage                     int32
	updatedAt                            time.Time
}

func (f accessFixture) stored(t *testing.T, c ids.CabalID) storedCabal {
	t.Helper()
	var s storedCabal
	if err := f.pool.QueryRow(t.Context(), `SELECT name, join_mode, voter_mode, threshold, proposal_expiry_seconds,
		slippage_bps, updated_at FROM cabals WHERE id = $1`, c.UUID()).Scan(
		&s.name, &s.joinMode, &s.voterMode, &s.threshold, &s.expiry, &s.slippage, &s.updatedAt); err != nil {
		t.Fatal(err)
	}
	s.updatedAt = s.updatedAt.UTC()
	return s
}

func (f accessFixture) voters(t *testing.T, c testkit.SeededCabal) []bool {
	t.Helper()
	out := make([]bool, 0, len(c.Members))
	for _, m := range c.Members {
		got, ok := f.membership(t, c.ID, m.ID)
		if !ok {
			t.Fatalf("member %s is gone", m.ID)
		}
		out = append(out, got.canVote)
	}
	return out
}

func (f accessFixture) updates(t *testing.T) []events.CabalUpdated {
	t.Helper()
	return decoded[events.CabalUpdated](t, f, events.TypeCabalUpdated)
}

func (f accessFixture) patch(
	ctx context.Context, actor ids.UserID, c ids.CabalID, body api.UpdateCabalRequest,
) (api.PatchCabalResponseObject, error) {
	return f.routes(nil).PatchCabal(as(ctx, actor), api.PatchCabalRequestObject{Id: c.UUID(), Body: &body})
}

func (f accessFixture) patchOK(t *testing.T, c testkit.SeededCabal, body api.UpdateCabalRequest) {
	t.Helper()
	if _, err := f.patch(t.Context(), c.Creator.ID, c.ID, body); err != nil {
		t.Fatal(err)
	}
}

func voterIDs(users ...testkit.SeededUser) *[]uuid.UUID {
	out := make([]uuid.UUID, 0, len(users))
	for _, u := range users {
		out = append(out, u.ID.UUID())
	}
	return &out
}

func TestUpdateCabal_Ok(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithMembers(2))
	f.clock.Advance(time.Minute)
	res, err := f.patch(t.Context(), c.Creator.ID, c.ID, api.UpdateCabalRequest{
		Name: ptr("  Work pot "), JoinMode: ptr("request"), Threshold: ptr("unanimous"),
		ProposalExpirySeconds: ptr(int32(3600)), SlippageBps: ptr(int32(50)),
	})
	view, ok := res.(api.PatchCabal200JSONResponse)
	rules := api.CabalRules{
		JoinMode: "request", VoterMode: "all", Threshold: "unanimous", ProposalExpirySeconds: 3600, SlippageBps: 50,
	}
	if err != nil || !ok || view.Name != "Work pot" || view.Rules != rules || view.Me == nil ||
		view.Me.Role != "creator" {
		t.Fatalf("PatchCabal = %+v, %v; want Work pot with rules %+v as its creator sees it", res, err, rules)
	}
	stored := storedCabal{
		name: "Work pot", joinMode: "request", voterMode: "all", threshold: "unanimous", expiry: 3600, slippage: 50,
		updatedAt: f.clock.Now(),
	}
	if after := f.stored(t, c.ID); after != stored {
		t.Fatalf("stored %+v, want %+v", after, stored)
	}
	updates := f.updates(t)
	got, err := json.Marshal(updates)
	if err != nil {
		t.Fatal(err)
	}
	want, err := json.Marshal([]events.CabalUpdated{{
		V: 1, CabalID: c.ID.UUID(), ActorID: c.Creator.ID.UUID(), Changes: events.CabalChanges{
			Name: ptr("Work pot"), JoinMode: ptr("request"), Threshold: ptr("unanimous"),
			ProposalExpirySeconds: ptr(int32(3600)), SlippageBps: ptr(int32(50)),
		},
	}})
	if err != nil || string(got) != string(want) {
		t.Fatalf("cabal.updated = %s, want only the changed fields %s", got, want)
	}
}

func TestUpdateCabal_NotCabalCreator(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithMembers(2))
	before := f.stored(t, c.ID)
	for _, user := range []ids.UserID{c.Members[1].ID, f.user(t)} {
		_, err := f.patch(t.Context(), user, c.ID, api.UpdateCabalRequest{Name: ptr("Taken over")})
		wantErr(t, err, errs.CodeNotCabalCreator)
	}
	if after := f.stored(t, c.ID); after != before || len(f.updates(t)) != 0 {
		t.Fatalf("stored %+v (was %+v), events %d; want nothing written", after, before, len(f.updates(t)))
	}
}

func TestUpdateCabal_InvalidInput(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithMembers(2))
	outsider := testkit.SeedUser(t, f.pool, testkit.UserOpts{})
	before := f.stored(t, c.ID)
	for _, tt := range []struct {
		name string
		body api.UpdateCabalRequest
	}{
		{"a short name", api.UpdateCabalRequest{Name: ptr(" a ")}},
		{"an unknown join mode", api.UpdateCabalRequest{JoinMode: ptr("secret")}},
		{"an unknown voter mode", api.UpdateCabalRequest{VoterMode: ptr("some")}},
		{"an unknown threshold", api.UpdateCabalRequest{Threshold: ptr("most")}},
		{"an expiry off the menu", api.UpdateCabalRequest{ProposalExpirySeconds: ptr(int32(60))}},
		{"zero slippage", api.UpdateCabalRequest{SlippageBps: ptr(int32(0))}},
		{"slippage over the cap", api.UpdateCabalRequest{SlippageBps: ptr(int32(301))}},
		{"voter ids while everyone votes", api.UpdateCabalRequest{VoterIds: voterIDs(c.Creator)}},
		{"voter ids switching to all", api.UpdateCabalRequest{VoterMode: ptr("all"), VoterIds: voterIDs(c.Creator)}},
		{"a voter who is not a member", api.UpdateCabalRequest{
			VoterMode: ptr("list"), VoterIds: voterIDs(c.Creator, outsider),
		}},
	} {
		_, err := f.patch(t.Context(), c.Creator.ID, c.ID, tt.body)
		if errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Errorf("%s: err = %v, want invalid_input", tt.name, err)
		}
	}
	_, err := f.routes(nil).PatchCabal(as(t.Context(), c.Creator.ID), api.PatchCabalRequestObject{Id: c.ID.UUID()})
	wantErr(t, err, errs.CodeInvalidInput)
	if after := f.stored(t, c.ID); after != before || len(f.updates(t)) != 0 {
		t.Fatalf("stored %+v (was %+v); want nothing written", after, before)
	}
	if got := f.voters(t, c); !slices.Equal(got, []bool{true, true}) {
		t.Fatalf("voters = %v, want both still voting", got)
	}
}

func TestUpdateCabal_CabalBanned(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool)
	f.exec(t, `UPDATE cabals SET status = 'banned' WHERE id = $1`, c.ID.UUID())
	_, err := f.patch(t.Context(), c.Creator.ID, c.ID, api.UpdateCabalRequest{Name: ptr("Back again")})
	wantErr(t, err, errs.CodeCabalBanned)
	if got := f.stored(t, c.ID).name; got == "Back again" || len(f.updates(t)) != 0 {
		t.Fatalf("name = %q with %d events, want the banned cabal unchanged", got, len(f.updates(t)))
	}
}

func TestUpdateCabal_NoChangeNoEvent(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithMembers(2), testkit.WithVoterMode("list"))
	before := f.stored(t, c.ID)
	f.clock.Advance(time.Minute)
	for _, body := range []api.UpdateCabalRequest{
		{},
		{
			Name: ptr(before.name), JoinMode: ptr(before.joinMode), VoterMode: ptr(before.voterMode),
			Threshold: ptr(before.threshold), ProposalExpirySeconds: ptr(before.expiry),
			SlippageBps: ptr(before.slippage),
		},
		{VoterIds: voterIDs(c.Creator, c.Creator)},
	} {
		res, err := f.patch(t.Context(), c.Creator.ID, c.ID, body)
		if got, ok := res.(api.PatchCabal200JSONResponse); err != nil || !ok || got.Name != before.name {
			t.Fatalf("PatchCabal(%+v) = %+v, %v; want the cabal unchanged", body, res, err)
		}
	}
	if after := f.stored(t, c.ID); after != before || len(f.updates(t)) != 0 {
		t.Fatalf("stored %+v (was %+v), events %+v; want no write", after, before, f.updates(t))
	}
}

func TestUpdateCabal_VoterListMustIncludeCreator(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithMembers(3))
	for _, voters := range []*[]uuid.UUID{voterIDs(c.Members[1]), voterIDs()} {
		_, err := f.patch(t.Context(), c.Creator.ID, c.ID, api.UpdateCabalRequest{
			VoterMode: ptr("list"), VoterIds: voters,
		})
		wantErr(t, err, errs.CodeInvalidInput)
	}
	if got := f.voters(t, c); !slices.Equal(got, []bool{true, true, true}) || len(f.updates(t)) != 0 {
		t.Fatalf("voters = %v, events %d; want everyone still voting and no event", got, len(f.updates(t)))
	}
}

func TestUpdateCabal_setsTheVoterListThenOpensVotingToAll(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithMembers(3))
	f.patchOK(t, c, api.UpdateCabalRequest{
		VoterMode: ptr("list"), VoterIds: voterIDs(c.Members[2], c.Creator),
	})
	if got := f.voters(t, c); !slices.Equal(got, []bool{true, false, true}) {
		t.Fatalf("voters after the list = %v, want the creator and the third member", got)
	}
	f.patchOK(t, c, api.UpdateCabalRequest{
		VoterIds: voterIDs(c.Creator, c.Members[1]),
	})
	if got := f.voters(t, c); !slices.Equal(got, []bool{true, true, false}) {
		t.Fatalf("voters after a list-only patch = %v, want the creator and the second member", got)
	}
	f.patchOK(t, c, api.UpdateCabalRequest{VoterMode: ptr("all")})
	if got := f.voters(t, c); !slices.Equal(got, []bool{true, true, true}) {
		t.Fatalf("voters after all = %v, want everyone", got)
	}
	updates := f.updates(t)
	if len(updates) != 3 {
		t.Fatalf("cabal.updated events = %d, want 3", len(updates))
	}
	listed := []uuid.UUID{c.Creator.ID.UUID(), c.Members[2].ID.UUID()}
	slices.SortFunc(listed, func(a, b uuid.UUID) int { return slices.Compare(a[:], b[:]) })
	first, second, third := updates[0].Changes, updates[1].Changes, updates[2].Changes
	if *first.VoterMode != "list" || !slices.Equal(first.VoterIDs, listed) || second.VoterMode != nil ||
		len(second.VoterIDs) != 2 || *third.VoterMode != "all" || third.VoterIDs != nil {
		t.Fatalf("changes = %+v, %+v, %+v; want the list, the new list, then all", first, second, third)
	}
}

func TestUpdateCabal_aRulesChangeReachesOnlyLaterVoterSetReads(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithMembers(3))
	reads := adapters.NewQueries(f.pool)
	frozen, err := reads.VoterSet(t.Context(), c.ID)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := slices.Clone(frozen)
	if _, err := f.patch(t.Context(), c.Creator.ID, c.ID, api.UpdateCabalRequest{
		VoterMode: ptr("list"), VoterIds: voterIDs(c.Creator),
	}); err != nil {
		t.Fatal(err)
	}
	later, err := reads.VoterSet(t.Context(), c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(frozen) != 3 || !slices.Equal(frozen, snapshot) || !slices.Equal(later, []ids.UserID{c.Creator.ID}) {
		t.Fatalf("voter set before = %v, after = %v; want all three frozen and only the creator later", frozen, later)
	}
}

func TestUpdateCabal_refusesAnUnknownCabalAndAnAnonymousCaller(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	user := f.user(t)
	_, err := f.patch(t.Context(), user, ids.CabalIDFrom(ids.Real{}.NewV7()), api.UpdateCabalRequest{})
	wantErr(t, err, errs.CodeCabalNotFound)
	_, err = f.routes(nil).PatchCabal(t.Context(), api.PatchCabalRequestObject{})
	wantErr(t, err, errs.CodeUnauthorized)
	c := testkit.NewCabal(t, f.pool)
	_, err = f.routes(failCards{}).PatchCabal(as(t.Context(), c.Creator.ID), api.PatchCabalRequestObject{
		Id: c.ID.UUID(), Body: &api.UpdateCabalRequest{},
	})
	wantErr(t, err, errs.CodeInternal)
}

func TestUpdateCabal_wrapsStoreFailuresAsInternal(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name      string
		voterMode string
		statement string
		body      func(c testkit.SeededCabal) api.UpdateCabalRequest
	}{
		{
			"the cabal lock", "all", `ALTER TABLE cabals RENAME TO cabals_gone`,
			func(testkit.SeededCabal) api.UpdateCabalRequest { return api.UpdateCabalRequest{} },
		},
		{
			"the cabal read", "all", `ALTER TABLE cabal_members RENAME TO members_gone`,
			func(testkit.SeededCabal) api.UpdateCabalRequest { return api.UpdateCabalRequest{} },
		},
		{
			"stored rules the domain refuses", "all", `ALTER TABLE cabals DROP CONSTRAINT cabals_join_mode_check;
			UPDATE cabals SET join_mode = 'secret'`,
			func(testkit.SeededCabal) api.UpdateCabalRequest { return api.UpdateCabalRequest{} },
		},
		{
			"the member list", "list", `ALTER TABLE cabal_members RENAME COLUMN can_vote TO votes`,
			func(c testkit.SeededCabal) api.UpdateCabalRequest {
				return api.UpdateCabalRequest{VoterIds: voterIDs(c.Creator)}
			},
		},
		{
			"the cabal write", "all", `ALTER TABLE cabals ADD CONSTRAINT no_rename CHECK (name <> 'Blocked')`,
			func(testkit.SeededCabal) api.UpdateCabalRequest { return api.UpdateCabalRequest{Name: ptr("Blocked")} },
		},
		{
			"the voter list write", "list", `ALTER TABLE cabal_members ADD CONSTRAINT creator_votes CHECK (can_vote = (role = 'creator'))`,
			func(c testkit.SeededCabal) api.UpdateCabalRequest {
				return api.UpdateCabalRequest{VoterIds: voterIDs(c.Creator, c.Members[1])}
			},
		},
		{
			"the vote for everyone", "list",
			`ALTER TABLE cabal_members ADD CONSTRAINT creator_votes CHECK (can_vote = (role = 'creator'))`,
			func(testkit.SeededCabal) api.UpdateCabalRequest { return api.UpdateCabalRequest{VoterMode: ptr("all")} },
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newAccess(t)
			c := testkit.NewCabal(t, f.pool, testkit.WithMembers(2), testkit.WithVoterMode(tt.voterMode))
			f.exec(t, tt.statement)
			_, err := f.patch(t.Context(), c.Creator.ID, c.ID, tt.body(c))
			wantErr(t, err, errs.CodeInternal)
		})
	}
}
