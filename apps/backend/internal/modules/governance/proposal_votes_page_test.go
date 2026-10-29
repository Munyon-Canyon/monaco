package governance_test

import (
	"encoding/base64"
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	identityport "github.com/monaco/monaco/apps/backend/internal/modules/identity/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/governanceapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func (d readsDB) votePage(t *testing.T, proposal uuid.UUID, limit int, cursor string) app.VotePage {
	t.Helper()
	page, err := d.reads().Votes(t.Context(), app.ListVotes{
		ID: ids.ProposalIDFrom(proposal), Caller: ids.UserIDFrom(d.caller), Limit: limit, Cursor: cursor,
	})
	if err != nil {
		t.Fatalf("Votes = %v", err)
	}
	return page
}

func (d readsDB) walkVotes(t *testing.T, proposal uuid.UUID, limit int) (got []app.VoterEntry, pages int) {
	t.Helper()
	cursor := ""
	for {
		page := d.votePage(t, proposal, limit, cursor)
		got = append(got, page.Items...)
		pages++
		if cursor = page.NextCursor; cursor == "" {
			return got, pages
		}
	}
}

func TestProposals_Votes_walkEveryVoterOnceOrderedByUserID(t *testing.T) {
	t.Parallel()
	d := newReadsDB(t)
	voters := make([]uuid.UUID, 0, 24)
	for range 24 {
		voters = append(voters, d.ids.NewV7())
	}
	p := d.proposedBy(t, d.caller, voters...)
	d.ballot(t, p, voters[3], "yes")
	d.ballot(t, p, voters[11], "no")
	got, pages := d.walkVotes(t, p, 10)
	order := make([]string, 0, len(got))
	voted := map[ids.UserID]bool{}
	for _, entry := range got {
		order = append(order, entry.UserID.String())
		voted[entry.UserID] = entry.Voted
	}
	if pages != 3 || len(got) != 25 || !slices.IsSorted(order) || len(slices.Compact(slices.Clone(order))) != 25 {
		t.Fatalf("pages %d, voters %d, sorted %v", pages, len(got), slices.IsSorted(order))
	}
	if !voted[ids.UserIDFrom(voters[3])] || !voted[ids.UserIDFrom(voters[11])] || voted[ids.UserIDFrom(voters[0])] {
		t.Fatalf("voted = %+v", voted)
	}
}

func TestProposals_Votes_showOnlyTheCallersOwnSide(t *testing.T) {
	t.Parallel()
	d := newReadsDB(t)
	other := d.ids.NewV7()
	p := d.proposedBy(t, d.caller, other)
	d.ballot(t, p, other, "yes")
	d.ballot(t, p, d.caller, "no")
	sides := map[uuid.UUID]domain.Choice{}
	for _, entry := range d.votePage(t, p, 10, "").Items {
		sides[entry.UserID.UUID()] = entry.Choice
		if !entry.Voted {
			t.Fatalf("%s did not show as voted", entry.UserID)
		}
	}
	if sides[d.caller] != domain.ChoiceNo || sides[other] != "" {
		t.Fatalf("sides = %+v, want only the caller's own", sides)
	}
}

func TestProposals_Votes_showEverySideOnceClosedWithTheRuleOn(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		rule   bool
		status domain.Status
		reveal bool
	}{
		{"passed with the rule on", true, domain.StatusPassed, true},
		{"passed with the rule off", false, domain.StatusPassed, false},
		{"open with the rule on", true, domain.StatusOpen, false},
		{"withdrawn with the rule on", true, domain.StatusWithdrawn, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := newSideRoom(t, tc.rule, tc.status)
			sides := map[uuid.UUID]domain.Choice{}
			for _, entry := range r.votePage(t, r.proposal, 10, "").Items {
				sides[entry.UserID.UUID()] = entry.Choice
			}
			shown := sides[r.mia] == domain.ChoiceYes && sides[r.kai] == domain.ChoiceNo
			if shown != tc.reveal || sides[r.caller] != domain.ChoiceYes || sides[r.jordan] != "" {
				t.Fatalf("sides = %+v, want others shown %v", sides, tc.reveal)
			}
		})
	}
}

func TestProposals_Votes_carryTheUserCard(t *testing.T) {
	t.Parallel()
	d := newReadsDB(t)
	member := ids.UserIDFrom(d.ids.NewV7())
	p := d.proposedBy(t, d.caller, member.UUID())
	cards := &cardBook{cards: map[ids.UserID]identityport.UserCard{
		member: {ID: member, Handle: "mia", DisplayName: "Mia", PhotoURL: "https://cdn.example/mia.jpg"},
	}}
	reads := app.NewProposalReads(d.pool, d.swaps, d.names, d.assets, cards, &returnBook{}, &replyBook{})
	page, err := reads.Votes(t.Context(), app.ListVotes{
		ID: ids.ProposalIDFrom(p), Caller: ids.UserIDFrom(d.caller), Limit: 10,
	})
	if err != nil || len(page.Items) != 2 || page.NextCursor != "" {
		t.Fatalf("Votes = %+v, %v", page, err)
	}
	for _, entry := range page.Items {
		if entry.UserID == member && (entry.Card.Handle != "mia" || entry.Card.DisplayName != "Mia") {
			t.Fatalf("card = %+v", entry.Card)
		}
	}
	if len(cards.calls) != 1 || len(cards.calls[0]) != 2 {
		t.Fatalf("card lookups = %v, want one for the page", cards.calls)
	}
}

func TestProposals_Votes_refuseABadLimitCursorAndMissingProposal(t *testing.T) {
	t.Parallel()
	d := newReadsDB(t)
	p := d.proposedBy(t, d.caller)
	for name, req := range map[string]app.ListVotes{
		"limit zero": {ID: ids.ProposalIDFrom(p), Limit: 0},
		"limit over": {ID: ids.ProposalIDFrom(p), Limit: 201},
		"cursor":     {ID: ids.ProposalIDFrom(p), Limit: 10, Cursor: "not-a-cursor!"},
	} {
		if _, err := d.reads().Votes(t.Context(), req); errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Fatalf("%s: %v", name, err)
		}
	}
	_, err := d.reads().Votes(t.Context(), app.ListVotes{ID: ids.ProposalIDFrom(d.ids.NewV7()), Limit: 10})
	if errs.CodeOf(err) != errs.CodeProposalNotFound {
		t.Fatalf("missing proposal: %v", err)
	}
}

func TestProposals_Votes_pastTheLastVoterIsAnEmptyPage(t *testing.T) {
	t.Parallel()
	d := newReadsDB(t)
	p := d.proposedBy(t, d.caller, d.ids.NewV7())
	last := base64.RawURLEncoding.EncodeToString([]byte(uuid.Max.String()))
	if page := d.votePage(t, p, 10, last); len(page.Items) != 0 || page.NextCursor != "" {
		t.Fatalf("page = %+v, want empty", page)
	}
}

func TestProposals_Votes_reportABrokenReadAndABrokenCardLookup(t *testing.T) {
	t.Parallel()
	d := newReadsDB(t)
	p := d.proposedBy(t, d.caller)
	down := errs.New(errs.CodeInternal, "test.cards")
	cards := &cardBook{err: down}
	broken := app.NewProposalReads(d.pool, d.swaps, d.names, d.assets, cards, &returnBook{}, &replyBook{})
	req := app.ListVotes{ID: ids.ProposalIDFrom(p), Caller: ids.UserIDFrom(d.caller), Limit: 10}
	if _, err := broken.Votes(t.Context(), req); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("card lookup failure = %v", err)
	}
	if _, err := d.pool.Exec(t.Context(), `ALTER TABLE proposal_voters RENAME TO proposal_voters_gone`); err != nil {
		t.Fatal(err)
	}
	if _, err := d.reads().Votes(t.Context(), req); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("voters read failure = %v", err)
	}
	req.Cursor = base64.RawURLEncoding.EncodeToString([]byte("not-a-uuid"))
	if _, err := d.reads().Votes(t.Context(), req); errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("cursor that is not a uuid = %v", err)
	}
}

func TestHTTP_GetProposalVotes_pagesWithTheCardAndTheCallersOwnSide(t *testing.T) {
	t.Parallel()
	d := newReadsDB(t)
	other := ids.UserIDFrom(d.ids.NewV7())
	p := d.proposedBy(t, d.caller, other.UUID())
	d.ballot(t, p, other.UUID(), "yes")
	d.ballot(t, p, d.caller, "no")
	cards := &cardBook{cards: map[ids.UserID]identityport.UserCard{
		other: {ID: other, Handle: "mia", DisplayName: "Mia", PhotoURL: "https://cdn.example/mia.jpg"},
	}}
	reads := app.NewProposalReads(d.pool, d.swaps, d.names, d.assets, cards, &returnBook{}, &replyBook{})
	h := adapters.HTTP{Reads: reads}
	ctx := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: d.caller.String()})
	limit := 1
	first, err := h.GetProposalVotes(ctx, api.GetProposalVotesRequestObject{
		Id: p, Params: api.GetProposalVotesParams{Limit: &limit},
	})
	page, ok := first.(api.GetProposalVotes200JSONResponse)
	if err != nil || !ok || len(page.Items) != 1 || page.NextCursor == nil {
		t.Fatalf("first page = %#v, %v", first, err)
	}
	second, err := h.GetProposalVotes(ctx, api.GetProposalVotesRequestObject{
		Id: p, Params: api.GetProposalVotesParams{Limit: &limit, Cursor: page.NextCursor},
	})
	next, ok := second.(api.GetProposalVotes200JSONResponse)
	if err != nil || !ok || len(next.Items) != 1 || next.NextCursor != nil {
		t.Fatalf("second page = %#v, %v", second, err)
	}
	checkVoteItems(t, append(page.Items, next.Items...), d.caller)
	if _, err := h.GetProposalVotes(ctx, api.GetProposalVotesRequestObject{Id: d.ids.NewV7()}); err == nil {
		t.Fatal("a missing proposal read as a page")
	}
}

func checkVoteItems(t *testing.T, items []api.ProposalVoteItem, caller uuid.UUID) {
	t.Helper()
	for _, item := range items {
		mine := item.UserId == caller
		if !item.Voted || (item.Choice != nil) != mine || (item.CastAt != nil) != mine {
			t.Fatalf("item = %+v, want voted with a side only on the caller's row", item)
		}
		if !mine && (item.DisplayName != "Mia" || item.Handle == nil || item.PhotoUrl == nil) {
			t.Fatalf("item = %+v, want the card", item)
		}
	}
}

func TestHTTP_GetProposalVotes_rejectsACallerThatIsNotAUser(t *testing.T) {
	t.Parallel()
	d := newReadsDB(t)
	h := adapters.HTTP{Reads: d.reads()}
	service := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorService, ID: d.caller.String()})
	if _, err := h.GetProposalVotes(service, api.GetProposalVotesRequestObject{Id: d.ids.NewV7()}); err == nil {
		t.Fatal("a service caller read the votes")
	}
}
