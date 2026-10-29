package cabal_test

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/domain"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/cabalapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type countingVotes struct {
	counts map[ids.CabalID]int
	calls  [][]ids.CabalID
}

func (c *countingVotes) OpenVoteCounts(_ context.Context, cabals []ids.CabalID) (map[ids.CabalID]int, error) {
	c.calls = append(c.calls, cabals)
	return c.counts, nil
}

func readWith(t *testing.T, h adapters.HTTP, c testkit.SeededCabal, user ids.UserID) api.Cabal {
	t.Helper()
	res, err := h.GetCabal(as(t.Context(), user), api.GetCabalRequestObject{Id: c.ID.UUID()})
	got, ok := res.(api.GetCabal200JSONResponse)
	if err != nil || !ok {
		t.Fatalf("GetCabal = %T, %v", res, err)
	}
	return api.Cabal(got)
}

func TestGetCabal_servesTheStartDateVoterCountOpenVotesAndWhoMayPropose(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithMembers(8))
	if _, err := f.pool.Exec(t.Context(), `UPDATE cabal_members SET can_vote = false
		WHERE cabal_id = $1 AND role = 'member' AND user_id = ANY($2)`,
		c.ID.UUID(), []uuid.UUID{c.Members[6].ID.UUID(), c.Members[7].ID.UUID()}); err != nil {
		t.Fatal(err)
	}
	votes := &countingVotes{counts: map[ids.CabalID]int{c.ID: 1}}
	h := f.routes(nil)
	h.Votes = votes
	voter := readWith(t, h, c, c.Members[1].ID)
	if voter.MemberCount != 8 || voter.VoterCount != 6 || voter.OpenProposalCount != 1 {
		t.Fatalf("counts = %d members, %d voters, %d open; want 8, 6, 1",
			voter.MemberCount, voter.VoterCount, voter.OpenProposalCount)
	}
	if len(votes.calls) != 1 || len(votes.calls[0]) != 1 || votes.calls[0][0] != c.ID {
		t.Fatalf("OpenVoteCounts calls = %v, want one call with the cabal", votes.calls)
	}
}

func TestGetCabal_servesTheStartDateAndWhoMayPropose(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithMembers(2), testkit.WithVoterMode("list"))
	h := f.routes(nil)
	voter := readWith(t, h, c, c.Creator.ID)
	var created time.Time
	if err := f.pool.QueryRow(t.Context(), `SELECT created_at FROM cabals WHERE id = $1`, c.ID.UUID()).
		Scan(&created); err != nil || !voter.CreatedAt.Equal(created) {
		t.Fatalf("created_at = %v, want the stored %v (%v)", voter.CreatedAt, created, err)
	}
	if voter.Me == nil || !voter.Me.CanPropose {
		t.Fatalf("a voter's me = %+v, want can_propose", voter.Me)
	}
	if bystander := readWith(t, h, c, c.Members[1].ID); bystander.Me == nil || bystander.Me.CanPropose {
		t.Fatalf("a non-voting member's me = %+v, want no can_propose", bystander.Me)
	}
}

func TestGetCabal_startsPrivateWithNoCapAndVotersOnlyProposing(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	c := testkit.NewCabal(t, f.pool)
	rules := readWith(t, f.routes(nil), c, c.Creator.ID).Rules
	if rules.WhoCanPropose != "voters" || rules.ShowSides || rules.ShowSidesSince != nil || rules.MaxTradeBps != nil {
		t.Fatalf("rules = %+v, want voters, sides off, never changed, no cap", rules)
	}
}

func TestGetCabal_failsWhenTheOpenVoteCountFails(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	c := testkit.NewCabal(t, f.pool)
	h := f.routes(nil)
	h.Votes = failingVotes{}
	_, err := h.GetCabal(as(t.Context(), c.Creator.ID), api.GetCabalRequestObject{Id: c.ID.UUID()})
	if errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("GetCabal = %v, want %s", err, errs.CodeInternal)
	}
}

type failingVotes struct{}

func (failingVotes) OpenVoteCounts(context.Context, []ids.CabalID) (map[ids.CabalID]int, error) {
	return nil, errs.New(errs.CodeInternal, "test.votes")
}

func TestPostCabal_readsBackTheMaxPerTradeAndStartsWithSidesOff(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	limit := int32(2000)
	req := cabalPost("c1", "Friends pot", "request", "list", "unanimous", domain.ExpiryWeek, nil)
	req.Body.MaxTradeBps = &limit
	res, err := f.routes(nil).PostCabal(f.actor(t.Context()), req)
	created, ok := res.(api.PostCabal201JSONResponse)
	if err != nil || !ok {
		t.Fatalf("PostCabal = %T, %v", res, err)
	}
	if got := created.Rules.MaxTradeBps; got == nil || *got != 2000 || created.Rules.ShowSides {
		t.Fatalf("rules = %+v, want cap 2000 and sides off", created.Rules)
	}
	read := readCabal(t, f, f.user.ID, ids.CabalIDFrom(created.Id))
	if read.Rules.MaxTradeBps == nil || *read.Rules.MaxTradeBps != 2000 {
		t.Fatalf("read back cap = %v, want 2000", read.Rules.MaxTradeBps)
	}
}

func TestPostCabal_refusesAMaxPerTradeOutsideItsRange(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	for _, bad := range []int32{0, 99, 150, 10100, -100} {
		req := cabalPost("c1", "Friends pot", "request", "list", "unanimous", domain.ExpiryWeek, nil)
		req.Body.MaxTradeBps = &bad
		_, err := f.routes(nil).PostCabal(f.actor(t.Context()), req)
		if errs.CodeOf(err) != errs.CodeInvalidInput || f.count(t.Context(), t, "cabals") != 0 {
			t.Fatalf("max_trade_bps %d: PostCabal = %v, want %s and no cabal", bad, err, errs.CodeInvalidInput)
		}
	}
}

func TestPatchCabal_keepsTheSidesAndMaxPerTradeRules(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool)
	if _, err := f.pool.Exec(t.Context(), `UPDATE cabals SET show_sides = true, show_sides_changed_at = $2,
		max_trade_bps = 2000 WHERE id = $1`, c.ID.UUID(), f.clock.Now()); err != nil {
		t.Fatal(err)
	}
	name := "Renamed pot"
	f.patchOK(t, c, api.UpdateCabalRequest{Name: &name})
	var on bool
	var capBps *int32
	var at *string
	if err := f.pool.QueryRow(t.Context(),
		`SELECT show_sides, max_trade_bps, show_sides_changed_at::text FROM cabals WHERE id = $1`, c.ID.UUID()).
		Scan(&on, &capBps, &at); err != nil {
		t.Fatal(err)
	}
	if !on || capBps == nil || *capBps != 2000 || at == nil {
		t.Fatalf("after a rename: sides %v, cap %v, changed_at %v; want on, 2000, set", on, capBps, at)
	}
}

func TestGetCabal_clampsAnOpenVoteCountPastInt32(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	c := testkit.NewCabal(t, f.pool)
	h := f.routes(nil)
	h.Votes = &countingVotes{counts: map[ids.CabalID]int{c.ID: math.MaxInt32 + 1}}
	if got := readWith(t, h, c, c.Creator.ID).OpenProposalCount; got != math.MaxInt32 {
		t.Fatalf("open_proposal_count = %d, want %d", got, int32(math.MaxInt32))
	}
}

func TestPortRules_carriesTheSidesAndMaxPerTradeRules(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	c := testkit.NewCabal(t, f.pool)
	if _, err := f.pool.Exec(t.Context(),
		`UPDATE cabals SET show_sides = true, max_trade_bps = 2000 WHERE id = $1`, c.ID.UUID()); err != nil {
		t.Fatal(err)
	}
	got, err := cabal.New(module.Deps{Pool: f.pool}).Queries().Rules(t.Context(), c.ID)
	if err != nil || !got.ShowSides || got.MaxTradeBps == nil || *got.MaxTradeBps != 2000 {
		t.Fatalf("Rules = %+v, %v; want sides on and cap 2000", got, err)
	}
}

func TestCreateCabal_aRetryWithAMaxPerTradeReturnsTheSameCabal(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	cmd := f.command(t, "c1")
	limit := int32(2000)
	rules, err := cmd.Rules.WithMaxTradeBps(&limit)
	if err != nil {
		t.Fatal(err)
	}
	cmd.Rules = rules
	first, err := f.handler().Handle(f.actor(t.Context()), cmd)
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.handler().Handle(f.actor(t.Context()), cmd)
	if err != nil || second != first {
		t.Fatalf("retry = %+v, %v; want %+v", second, err, first)
	}
	other := int32(3000)
	if cmd.Rules, err = cmd.Rules.WithMaxTradeBps(&other); err != nil {
		t.Fatal(err)
	}
	if _, err = f.handler().Handle(f.actor(t.Context()), cmd); errs.CodeOf(err) != errs.CodeIdempotencyMismatch {
		t.Fatalf("a retry with another cap = %v, want %s", err, errs.CodeIdempotencyMismatch)
	}
}
