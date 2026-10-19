package governance_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/governanceapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func (d readsDB) proposalIn(
	t *testing.T, cabal uuid.UUID, at time.Time, status domain.Status, voters ...uuid.UUID,
) uuid.UUID {
	t.Helper()
	p := d.buy(voters...)
	p.CabalID, p.CreatedAt = cabal, at
	d.insert(t, p)
	d.setStatus(t, p.ID, string(status), "")
	return p.ID
}

func (d readsDB) mine(t *testing.T, filter app.Filter, limit int, cursor string) app.ProposalPage {
	t.Helper()
	page, err := d.reads().ListMine(t.Context(), app.ListMyProposals{
		Caller: ids.UserIDFrom(d.caller), Filter: filter, Limit: limit, Cursor: cursor,
	})
	if err != nil {
		t.Fatalf("ListMine(%s, %d, %q) = %v", filter, limit, cursor, err)
	}
	return page
}

func TestProposals_ListMine_AcrossCabalsNewestFirst(t *testing.T) {
	t.Parallel()
	d := newReadsDB(t)
	other, stranger := d.ids.NewV7(), d.ids.NewV7()
	d.names.set(d.cabal, "Friday Night Lights", "active")
	d.names.set(other, "Weekend Warriors", "active")
	oldest := d.proposalIn(t, d.cabal, d.now, domain.StatusExecuted, d.caller)
	middle := d.proposalIn(t, other, d.now.Add(time.Minute), domain.StatusFailed, d.caller, stranger)
	newest := d.proposalIn(t, d.cabal, d.now.Add(2*time.Minute), domain.StatusExpired, d.caller)
	d.proposalIn(t, other, d.now.Add(3*time.Minute), domain.StatusOpen, d.caller)
	page := d.mine(t, "", 0, "")
	if got, want := viewIDs(page.Items), []uuid.UUID{newest, middle, oldest}; !slices.Equal(got, want) {
		t.Fatalf("closed proposals = %v, want %v newest first", got, want)
	}
	if got, want := page.Items[1].CabalName, "Weekend Warriors"; got != want {
		t.Errorf("cabal name = %q, want %q", got, want)
	}
	if page.NextCursor != "" {
		t.Errorf("next cursor = %q, want none", page.NextCursor)
	}
	if got := len(d.names.calls); got != 1 {
		t.Errorf("cabal lookups = %d, want 1 whatever the number of cabals", got)
	}
}

func TestProposals_ListMine_OnlyFrozenVoterSet(t *testing.T) {
	t.Parallel()
	d := newReadsDB(t)
	stranger := d.ids.NewV7()
	mine := d.proposalIn(t, d.cabal, d.now, domain.StatusPassed, d.caller, stranger)
	d.proposalIn(t, d.ids.NewV7(), d.now.Add(time.Minute), domain.StatusPassed, stranger)
	d.proposalIn(t, d.cabal, d.now.Add(2*time.Minute), domain.StatusPassed, stranger)
	if got, want := viewIDs(d.mine(t, app.FilterAll, 0, "").Items), []uuid.UUID{mine}; !slices.Equal(got, want) {
		t.Errorf("proposals = %v, want only %v: the caller is not in the frozen voter set of the others", got, want)
	}
}

func TestProposals_ListMine_Paging(t *testing.T) {
	t.Parallel()
	d := newReadsDB(t)
	cabals := []uuid.UUID{d.cabal, d.ids.NewV7(), d.ids.NewV7()}
	seeded := make([]uuid.UUID, 0, 7)
	for i := range 7 {
		seeded = append(seeded, d.proposalIn(t, cabals[i%3], d.now.Add(time.Duration(i/2)*time.Second),
			domain.StatusExecuted, d.caller))
	}
	var seen []uuid.UUID
	var sizes []int
	cursor := ""
	for range 4 {
		page := d.mine(t, app.FilterClosed, 3, cursor)
		sizes = append(sizes, len(page.Items))
		seen = append(seen, viewIDs(page.Items)...)
		if cursor = page.NextCursor; cursor == "" {
			break
		}
	}
	slices.Reverse(seeded)
	if !slices.Equal(sizes, []int{3, 3, 1}) || !slices.Equal(seen, seeded) {
		t.Fatalf("pages = %v, ids = %v, want 3/3/1 over each row once newest first %v", sizes, seen, seeded)
	}
}

func TestProposals_ListMine_Filters(t *testing.T) {
	t.Parallel()
	d := newReadsDB(t)
	open := d.proposalIn(t, d.cabal, d.now, domain.StatusOpen, d.caller)
	closed := d.proposalIn(t, d.cabal, d.now.Add(time.Minute), domain.StatusWithdrawn, d.caller)
	cases := []struct {
		filter app.Filter
		want   []uuid.UUID
	}{
		{"", []uuid.UUID{closed}},
		{app.FilterClosed, []uuid.UUID{closed}},
		{app.FilterOpen, []uuid.UUID{open}},
		{app.FilterAll, []uuid.UUID{closed, open}},
	}
	for _, c := range cases {
		page := d.mine(t, c.filter, 0, "")
		if got := viewIDs(page.Items); !slices.Equal(got, c.want) {
			t.Errorf("filter %q = %v, want %v", c.filter, got, c.want)
		}
	}
	page := d.mine(t, app.FilterOpen, 0, "")
	if v := page.Items[0]; !v.CanVote || v.Tally.Voters != 1 {
		t.Errorf("open proposal can_vote = %t with %d voters, want true with 1", v.CanVote, v.Tally.Voters)
	}
}

func TestProposals_ListMine_InvalidInput(t *testing.T) {
	t.Parallel()
	d := newReadsDB(t)
	for name, req := range map[string]app.ListMyProposals{
		"limit over the cap": {Limit: 51},
		"negative limit":     {Limit: -1},
		"bad cursor":         {Cursor: "not-a-cursor"},
		"unknown filter":     {Filter: "nope"},
		"per-cabal filter":   {Filter: app.FilterPassed},
	} {
		req.Caller = ids.UserIDFrom(d.caller)
		if _, err := d.reads().ListMine(t.Context(), req); errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Errorf("%s: err = %v, want invalid_input", name, err)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := d.reads().ListMine(ctx, app.ListMyProposals{Caller: ids.UserIDFrom(d.caller)}); errs.CodeOf(
		err) != errs.CodeInternal {
		t.Errorf("ListMine on a cancelled context err = %v, want internal", err)
	}
}

func (d readsDB) getMine(
	t *testing.T, h adapters.HTTP, params api.GetMyProposalsParams,
) api.GetMyProposals200JSONResponse {
	t.Helper()
	ctx := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: d.caller.String()})
	res, err := h.GetMyProposals(ctx, api.GetMyProposalsRequestObject{Params: params})
	page, ok := res.(api.GetMyProposals200JSONResponse)
	if err != nil || !ok {
		t.Fatalf("GetMyProposals(%+v) = %#v, %v", params, res, err)
	}
	return page
}

func TestHTTP_GetMyProposals_DefaultsToClosed(t *testing.T) {
	t.Parallel()
	d := newReadsDB(t)
	d.names.set(d.cabal, "Friday Night Lights", "active")
	closed := d.proposalIn(t, d.cabal, d.now, domain.StatusExecuted, d.caller)
	d.proposalIn(t, d.cabal, d.now.Add(time.Minute), domain.StatusOpen, d.caller)
	got := d.getMine(t, adapters.HTTP{Reads: d.reads()}, api.GetMyProposalsParams{})
	if len(got.Proposals) != 1 || got.NextCursor != nil {
		t.Fatalf("GetMyProposals = %+v, want the one closed proposal and no cursor", got)
	}
	if p := got.Proposals[0]; p.Id != closed || p.CabalName != "Friday Night Lights" {
		t.Errorf("proposal = %+v, want %s in Friday Night Lights", p, closed)
	}
}

func TestHTTP_GetMyProposals_Paging(t *testing.T) {
	t.Parallel()
	d := newReadsDB(t)
	older := d.proposalIn(t, d.cabal, d.now, domain.StatusExecuted, d.caller)
	newer := d.proposalIn(t, d.cabal, d.now.Add(time.Minute), domain.StatusOpen, d.caller)
	h := adapters.HTTP{Reads: d.reads()}
	filter, limit := api.GetMyProposalsParamsFilter("all"), 1
	first := d.getMine(t, h, api.GetMyProposalsParams{Filter: &filter, Limit: &limit})
	if len(first.Proposals) != 1 || first.Proposals[0].Id != newer || first.NextCursor == nil {
		t.Fatalf("first page = %+v, want %s and a cursor", first, newer)
	}
	second := d.getMine(t, h, api.GetMyProposalsParams{Filter: &filter, Limit: &limit, Cursor: first.NextCursor})
	if len(second.Proposals) != 1 || second.Proposals[0].Id != older || second.NextCursor != nil {
		t.Errorf("second page = %+v, want %s and no cursor", second, older)
	}
}

func TestHTTP_GetMyProposals_Failures(t *testing.T) {
	t.Parallel()
	d := newReadsDB(t)
	h := adapters.HTTP{Reads: d.reads()}
	if _, err := h.GetMyProposals(t.Context(), api.GetMyProposalsRequestObject{}); errs.CodeOf(
		err) != errs.CodeUnauthorized {
		t.Errorf("GetMyProposals with no actor err = %v, want unauthorized", err)
	}
	ctx := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: d.caller.String()})
	tooMany := 51
	if _, err := h.GetMyProposals(ctx, api.GetMyProposalsRequestObject{
		Params: api.GetMyProposalsParams{Limit: &tooMany},
	}); errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Errorf("GetMyProposals with limit 51 err = %v, want invalid_input", err)
	}
}
