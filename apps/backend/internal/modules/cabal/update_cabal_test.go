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
		Name: ptr("  Work pot "), SlippageBps: ptr(int32(50)),
	})
	view, ok := res.(api.PatchCabal200JSONResponse)
	rules := api.CabalRules{
		JoinMode: "open", VoterMode: "all", Threshold: "majority", ProposalExpirySeconds: 86400, SlippageBps: 50,
		WhoCanPropose: "voters",
	}
	if err != nil || !ok || view.Name != "Work pot" || view.Rules != rules || view.Me == nil ||
		view.Me.Role != "creator" {
		t.Fatalf("PatchCabal = %+v, %v; want Work pot with rules %+v as its creator sees it", res, err, rules)
	}
	stored := storedCabal{
		name: "Work pot", joinMode: "open", voterMode: "all", threshold: "majority", expiry: 86400, slippage: 50,
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
			Name: ptr("Work pot"), SlippageBps: ptr(int32(50)),
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
	before := f.stored(t, c.ID)
	for _, tt := range []struct {
		name string
		body api.UpdateCabalRequest
	}{
		{"a short name", api.UpdateCabalRequest{Name: ptr(" a ")}},
		{"zero slippage", api.UpdateCabalRequest{SlippageBps: ptr(int32(0))}},
		{"slippage over the cap", api.UpdateCabalRequest{SlippageBps: ptr(int32(301))}},
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
	c := testkit.NewCabal(t, f.pool, testkit.WithMembers(2))
	before := f.stored(t, c.ID)
	f.clock.Advance(time.Minute)
	for _, body := range []api.UpdateCabalRequest{
		{},
		{
			Name: ptr(before.name), SlippageBps: ptr(before.slippage),
		},
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
			"the cabal write", "all", `ALTER TABLE cabals ADD CONSTRAINT no_rename CHECK (name <> 'Blocked')`,
			func(testkit.SeededCabal) api.UpdateCabalRequest { return api.UpdateCabalRequest{Name: ptr("Blocked")} },
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
