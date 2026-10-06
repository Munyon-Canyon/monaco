package governance_test

import (
	"context"
	"encoding/base64"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/governanceapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

type readsDB struct {
	proposalDB
	cabal  uuid.UUID
	caller uuid.UUID
	swaps  *fakes.Trading
}

func newReadsDB(t *testing.T) readsDB {
	t.Helper()
	d := newProposalDB(t)
	return readsDB{proposalDB: d, cabal: d.ids.NewV7(), caller: d.ids.NewV7(), swaps: fakes.NewTrading()}
}

func (d readsDB) reads() *app.ProposalReads {
	return app.NewProposalReads(d.pool, threshold{rule: domain.RuleMajority}, d.swaps)
}

func (d readsDB) seed(t *testing.T, at time.Time, voters ...uuid.UUID) uuid.UUID {
	t.Helper()
	p := d.buy(append([]uuid.UUID{d.caller}, voters...)...)
	p.CabalID, p.CreatedAt = d.cabal, at
	d.insert(t, p)
	return p.ID
}

func (d readsDB) setStatus(t *testing.T, id uuid.UUID, status, reason string) {
	t.Helper()
	_, err := d.pool.Exec(t.Context(), `UPDATE proposals SET status = $2, status_reason = $3 WHERE id = $1`,
		id, status, pgtype.Text{String: reason, Valid: reason != ""})
	if err != nil {
		t.Fatal(err)
	}
}

func (d readsDB) list(t *testing.T, filter app.Filter, limit int, cursor string) app.ProposalPage {
	t.Helper()
	page, err := d.reads().List(t.Context(), app.ListProposals{
		CabalID: ids.CabalIDFrom(d.cabal), Caller: ids.UserIDFrom(d.caller),
		Filter: filter, Limit: limit, Cursor: cursor,
	})
	if err != nil {
		t.Fatalf("List(%s, %d, %q) = %v", filter, limit, cursor, err)
	}
	return page
}

func viewIDs(items []app.ProposalView) []uuid.UUID {
	out := make([]uuid.UUID, len(items))
	for i, v := range items {
		out[i] = v.ID.UUID()
	}
	return out
}

func TestProposals_List_KeysetStable(t *testing.T) {
	t.Parallel()
	d := newReadsDB(t)
	seeded := make([]uuid.UUID, 45)
	for i := range seeded {
		seeded[i] = d.seed(t, d.now.Add(time.Duration(i/3)*time.Second))
	}
	var seen []uuid.UUID
	var sizes []int
	cursor := ""
	for page := range 4 {
		got := d.list(t, app.FilterAll, 20, cursor)
		sizes = append(sizes, len(got.Items))
		seen = append(seen, viewIDs(got.Items)...)
		d.seed(t, d.now.Add(time.Hour+time.Duration(page)*time.Second))
		if cursor = got.NextCursor; cursor == "" {
			break
		}
	}
	want := slices.Clone(seeded)
	slices.Reverse(want)
	if !slices.Equal(sizes, []int{20, 20, 5}) || !slices.Equal(seen, want) {
		t.Fatalf("pages = %v, ids = %v, want 20/20/5 over the 45 seeded ids newest first %v", sizes, seen, want)
	}
}

func TestProposals_List_FilterClosed(t *testing.T) {
	t.Parallel()
	d := newReadsDB(t)
	byStatus := map[domain.Status]uuid.UUID{}
	for i, status := range domain.Statuses() {
		id := d.seed(t, d.now.Add(time.Duration(i)*time.Second))
		d.setStatus(t, id, string(status), "")
		byStatus[status] = id
	}
	closed := d.list(t, app.FilterClosed, 0, "")
	open := d.list(t, app.FilterOpen, 0, "")
	all := d.list(t, "", 0, "")
	closedStatuses := make([]domain.Status, 0, len(closed.Items))
	for _, v := range closed.Items {
		closedStatuses = append(closedStatuses, v.Status)
	}
	slices.Sort(closedStatuses)
	want := slices.DeleteFunc(domain.Statuses(), func(s domain.Status) bool { return s == domain.StatusOpen })
	slices.Sort(want)
	if !slices.Equal(closedStatuses, want) {
		t.Errorf("closed statuses = %v, want every status but open %v", closedStatuses, want)
	}
	if !slices.Equal(viewIDs(open.Items), []uuid.UUID{byStatus[domain.StatusOpen]}) || len(all.Items) != len(byStatus) {
		t.Errorf("open = %v, all = %d items, want only %s and all %d", viewIDs(open.Items), len(all.Items),
			byStatus[domain.StatusOpen], len(byStatus))
	}
}

func TestProposals_List_itemCarriesTallyBallotAndBlockedReason(t *testing.T) {
	t.Parallel()
	d := newReadsDB(t)
	yes, no := d.ids.NewV7(), d.ids.NewV7()
	voted := d.seed(t, d.now, yes, no)
	d.ballot(t, voted, d.caller, "yes")
	d.ballot(t, voted, no, "no")
	blocked := d.seed(t, d.now.Add(time.Second))
	d.setStatus(t, blocked, string(domain.StatusExecutionBlocked), string(errs.CodeNoRoute))
	var page app.ProposalPage
	testkit.AssertQueries(t, "ListProposals one page", func() { page = d.list(t, app.FilterAll, 0, "") })
	base := app.ProposalView{
		CabalID: ids.CabalIDFrom(d.cabal), Kind: domain.KindBuy, Symbol: "AAPLx", USDCMicros: 25_000_000,
		QuoteOut: 105_000_000, Thesis: "Earnings next week.",
	}
	wantBlocked, wantVoted := base, base
	wantBlocked.Status, wantBlocked.StatusReason = domain.StatusExecutionBlocked, errs.CodeNoRoute
	wantBlocked.Tally = app.Tally{Voters: 1, Needed: 1}
	wantVoted.Status, wantVoted.MyBallot, wantVoted.CanVote = domain.StatusOpen, domain.ChoiceYes, true
	wantVoted.Tally = app.Tally{Yes: 1, No: 1, Voters: 3, Needed: 2}
	want := []app.ProposalView{wantBlocked, wantVoted}
	for i, got := range page.Items {
		want[i].ID, want[i].ProposerID, want[i].ExpiresAt, want[i].CreatedAt = got.ID, got.ProposerID,
			got.ExpiresAt, got.CreatedAt
	}
	if !slices.Equal(page.Items, want) || page.NextCursor != "" || !page.Items[1].CreatedAt.Equal(d.now) {
		t.Fatalf("page = %+v, want %+v and no cursor", page, want)
	}
	empty := readsDB{proposalDB: d.proposalDB, cabal: d.ids.NewV7(), caller: d.caller, swaps: d.swaps}
	testkit.AssertQueries(t, "ListProposals empty page", func() {
		if page := empty.list(t, app.FilterAll, 0, ""); len(page.Items) != 0 || page.Items == nil {
			t.Errorf("empty cabal page = %+v, want an empty, non-nil list", page)
		}
	})
}

func TestProposals_List_refusesBadInput(t *testing.T) {
	t.Parallel()
	d := newReadsDB(t)
	enc := func(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }
	for name, req := range map[string]app.ListProposals{
		"filter":           {Filter: "pending"},
		"limit over 50":    {Limit: 51},
		"negative limit":   {Limit: -1},
		"cursor encoding":  {Cursor: "%%%"},
		"cursor separator": {Cursor: enc("1772366400000000")},
		"cursor time":      {Cursor: enc("noon\x00" + d.ids.NewV7().String())},
		"cursor id":        {Cursor: enc("1772366400000000\x00nope")},
	} {
		req.CabalID = ids.CabalIDFrom(d.cabal)
		if _, err := d.reads().List(t.Context(), req); errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Errorf("%s: List err = %v, want invalid_input", name, err)
		}
	}
}

func TestProposals_List_failures(t *testing.T) {
	t.Parallel()
	d := newReadsDB(t)
	req := app.ListProposals{CabalID: ids.CabalIDFrom(d.cabal), Caller: ids.UserIDFrom(d.caller)}
	gone := app.NewProposalReads(d.pool, threshold{err: errs.New(errs.CodeCabalNotFound, "t")}, d.swaps)
	if _, err := gone.List(t.Context(), req); errs.CodeOf(err) != errs.CodeCabalNotFound {
		t.Errorf("List of an unknown cabal err = %v, want cabal_not_found", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := d.reads().List(ctx, req); errs.CodeOf(err) != errs.CodeInternal {
		t.Errorf("List on a cancelled context err = %v, want internal", err)
	}
	bad := d.seed(t, d.now)
	for _, stored := range [][2]string{{"swap", "open"}, {"buy", "gremlins"}} {
		_, err := d.pool.Exec(t.Context(), `UPDATE proposals SET kind = $2, status = $3 WHERE id = $1`,
			bad, stored[0], stored[1])
		if err != nil {
			t.Fatal(err)
		}
		if _, err := d.reads().List(t.Context(), req); errs.CodeOf(err) != errs.CodeDecodeFailed {
			t.Errorf("List with a stored kind and status of %v err = %v, want decode_failed", stored, err)
		}
	}
	if _, err := d.pool.Exec(t.Context(), `ALTER TABLE proposal_voters RENAME TO proposal_voters_gone`); err != nil {
		t.Fatal(err)
	}
	if _, err := d.reads().List(t.Context(), req); errs.CodeOf(err) != errs.CodeInternal {
		t.Errorf("List whose tally read fails err = %v, want internal", err)
	}
}

func (d readsDB) getProposals(
	t *testing.T, h adapters.HTTP, params api.GetCabalProposalsParams,
) api.GetCabalProposals200JSONResponse {
	t.Helper()
	ctx := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: d.caller.String()})
	res, err := h.GetCabalProposals(ctx, api.GetCabalProposalsRequestObject{Id: d.cabal, Params: params})
	page, ok := res.(api.GetCabalProposals200JSONResponse)
	if err != nil || !ok {
		t.Fatalf("GetCabalProposals(%+v) = %#v, %v", params, res, err)
	}
	return page
}

func ptr[T any](v T) *T { return &v }

func TestHTTP_GetCabalProposals(t *testing.T) {
	t.Parallel()
	d := newReadsDB(t)
	older := d.seed(t, d.now)
	sell := d.buy(d.caller)
	sell.CabalID, sell.CreatedAt, sell.Kind = d.cabal, d.now.Add(time.Second), "sell"
	sell.UsdcMicros, sell.TokenAmount, sell.Thesis = pgtype.Int8{}, pgtype.Int8{Int64: 7, Valid: true}, pgtype.Text{}
	d.insert(t, sell)
	d.setStatus(t, sell.ID, string(domain.StatusExecutionBlocked), string(errs.CodeNoRoute))
	d.ballot(t, sell.ID, d.caller, "no")
	h := adapters.HTTP{Reads: d.reads()}
	first := d.getProposals(t, h, api.GetCabalProposalsParams{
		Filter: ptr(api.GetCabalProposalsParamsFilterAll), Limit: ptr(1),
	})
	second := d.getProposals(t, h, api.GetCabalProposalsParams{Cursor: first.NextCursor})
	if len(first.Proposals) != 1 || first.NextCursor == nil || len(second.Proposals) != 1 || second.NextCursor != nil {
		t.Fatalf("pages = %+v then %+v, want one item each and a cursor only on the first", first, second)
	}
	base := api.Proposal{
		CabalId: d.cabal, ProposerId: sell.ProposerID, Symbol: "AAPLx", QuoteOutAmount: 105_000_000,
		Tally: api.Tally{Voters: 1, Needed: 1},
	}
	wantSell, wantBuy := base, base
	wantSell.Id, wantSell.Kind, wantSell.TokenAmount = sell.ID, "sell", ptr(int64(7))
	wantSell.Status, wantSell.MyBallot = api.ProposalStatus(domain.StatusExecutionBlocked), ptr(api.BallotChoice("no"))
	wantSell.StatusReason, wantSell.StatusMessage = ptr("no_route"), ptr(errs.Message(errs.CodeNoRoute))
	wantSell.Tally.No = 1
	wantBuy.Id, wantBuy.Kind, wantBuy.UsdcMicros = older, "buy", ptr(int64(25_000_000))
	wantBuy.Status, wantBuy.Thesis, wantBuy.CanVote = "open", ptr("Earnings next week."), true
	for _, pair := range [][2]*api.Proposal{{&wantSell, &first.Proposals[0]}, {&wantBuy, &second.Proposals[0]}} {
		want, got := pair[0], pair[1]
		want.ProposerId, want.ExpiresAt, want.CreatedAt = got.ProposerId, got.ExpiresAt, got.CreatedAt
		if !reflect.DeepEqual(got, want) {
			t.Errorf("item = %+v, want %+v", *got, *want)
		}
	}
	if _, err := h.GetCabalProposals(t.Context(), api.GetCabalProposalsRequestObject{Id: d.cabal}); errs.CodeOf(
		err) != errs.CodeUnauthorized {
		t.Errorf("GetCabalProposals with no actor err = %v, want unauthorized", err)
	}
	ctx := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: d.caller.String()})
	if _, err := h.GetCabalProposals(ctx, api.GetCabalProposalsRequestObject{
		Id: d.cabal, Params: api.GetCabalProposalsParams{Limit: ptr(51)},
	}); errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Errorf("GetCabalProposals over the page limit err = %v, want invalid_input", err)
	}
}
