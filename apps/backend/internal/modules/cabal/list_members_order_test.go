package cabal_test

import (
	"context"
	"encoding/base64"
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking"
	rankingport "github.com/monaco/monaco/apps/backend/internal/modules/ranking/port"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/cabalapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type countingReads struct {
	bps       map[ids.UserID]int32
	records   map[ids.UserID]rankingport.CallRecord
	ownership int
	calls     int
	ownErr    error
	callErr   error
	asked     int
}

func (c *countingReads) Ownership(context.Context, ids.CabalID) (map[ids.UserID]int32, error) {
	c.ownership++
	return c.bps, c.ownErr
}

func (c *countingReads) Records(
	_ context.Context, userIDs []ids.UserID, _ *ids.CabalID, _ rankingport.Period,
) (map[ids.UserID]rankingport.CallRecord, error) {
	c.calls++
	c.asked = len(userIDs)
	return c.records, c.callErr
}

func (c *countingReads) reads(f createFixture) app.MemberReads {
	return app.MemberReads{Ownership: c, Calls: c, Now: f.clock.Now}
}

type walked struct {
	order  []uuid.UUID
	crowns []uuid.UUID
	items  map[uuid.UUID]app.Member
	pages  int
	totals []app.MemberPage
}

func walkRanked(
	t *testing.T, f createFixture, cabalID ids.CabalID, c *countingReads, order string,
) walked {
	t.Helper()
	out := walked{items: map[uuid.UUID]app.Member{}}
	cursor := ""
	for {
		page, err := app.ListMembersPage(t.Context(), f.pool, f.routes(nil).Users, c.reads(f), app.ListMembersQuery{
			CabalID: cabalID, Limit: 50, Cursor: cursor, Order: order,
		})
		if err != nil {
			t.Fatal(err)
		}
		out.pages++
		out.totals = append(out.totals, page)
		for _, m := range page.Items {
			out.order = append(out.order, m.UserID)
			out.items[m.UserID] = m
			if m.Crown {
				out.crowns = append(out.crowns, m.UserID)
			}
		}
		if page.NextCursor == "" {
			return out
		}
		cursor = page.NextCursor
	}
}

func largeCabal(t *testing.T, f createFixture) (ids.CabalID, []uuid.UUID) {
	t.Helper()
	seeded := testkit.NewCabal(t, f.pool, testkit.WithMembers(120))
	order := joinOrder(t, f, seeded.ID)
	return seeded.ID, order
}

func TestListMembersPage_ordersByRoleThenShareAcrossPages(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	cabalID, joined := largeCabal(t, f)
	lastAdmin, largest := joined[119], joined[118]
	execSQL(t, f.pool, `UPDATE cabal_members SET role = 'admin', can_vote = true WHERE cabal_id = $1 AND user_id = $2`,
		cabalID.UUID(), lastAdmin)
	execSQL(t, f.pool, `UPDATE cabal_members SET can_vote = false WHERE cabal_id = $1 AND user_id = $2`,
		cabalID.UUID(), joined[117])
	c := &countingReads{bps: map[ids.UserID]int32{
		ids.UserIDFrom(largest): 5000, ids.UserIDFrom(joined[3]): 3000, ids.UserIDFrom(joined[117]): 2000,
	}}
	got := walkRanked(t, f, cabalID, c, "")
	if got.pages != 3 || len(got.order) != 120 || len(toSet(got.order)) != 120 {
		t.Fatalf("pages %d, rows %d, want 3 pages of 120 unique members", got.pages, len(got.order))
	}
	creator := joined[0]
	want := []uuid.UUID{creator, lastAdmin, largest, joined[3]}
	if !slices.Equal(got.order[:4], want) {
		t.Fatalf("first rows = %v, want creator, last-joined admin, largest holder, next holder", got.order[:4])
	}
	if got.order[119] != joined[117] {
		t.Fatalf("last row = %v, want the non-voting holder after every voter", got.order[119])
	}
	if c.ownership != 3 || c.calls != 3 || c.asked != 120 {
		t.Fatalf("ownership calls %d, record calls %d over %d members, want one each per page over all 120",
			c.ownership, c.calls, c.asked)
	}
	checkSharesAndTotals(t, got, largest)
}

func checkSharesAndTotals(t *testing.T, got walked, largest uuid.UUID) {
	t.Helper()
	var sum int32
	for _, m := range got.items {
		sum += m.OwnershipBps
	}
	if sum > 10000 || got.items[largest].OwnershipBps != 5000 {
		t.Fatalf("ownership sum %d, largest %d", sum, got.items[largest].OwnershipBps)
	}
	for _, page := range got.totals {
		if page.Totals.Admins != 2 || page.Totals.Voters != 117 || page.Totals.Members != 1 {
			t.Fatalf("totals = %+v, want the whole cabal on every page", page.Totals)
		}
	}
}

func toSet(ids []uuid.UUID) map[uuid.UUID]bool {
	out := map[uuid.UUID]bool{}
	for _, id := range ids {
		out[id] = true
	}
	return out
}

func TestListMembersPage_crownsTheTopRecordWithFiveResolvedOnEveryPage(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	cabalID, joined := largeCabal(t, f)
	holder := ids.UserIDFrom(joined[60])
	c := &countingReads{records: map[ids.UserID]rankingport.CallRecord{
		holder:                     {Right: 6, Resolved: 6, Voted: 6, Eligible: 6},
		ids.UserIDFrom(joined[70]): {Right: 4, Resolved: 4, Voted: 4, Eligible: 4},
		ids.UserIDFrom(joined[0]):  {Right: 3, Resolved: 5, Voted: 5, Eligible: 6},
	}}
	got := walkRanked(t, f, cabalID, c, "")
	if !slices.Equal(got.crowns, []uuid.UUID{holder.UUID()}) {
		t.Fatalf("crowns = %v, want only the 6 of 6 holder", got.crowns)
	}
	if got.items[holder.UUID()].Month != (app.MemberMonth{Right: 6, Resolved: 6, Voted: 6, Eligible: 6}) {
		t.Fatalf("month = %+v", got.items[holder.UUID()].Month)
	}
	for _, m := range got.totals[0].Items {
		if m.Crown {
			t.Fatalf("page one wears a crown on %v though the holder sits further down", m.UserID)
		}
	}
	c = &countingReads{records: map[ids.UserID]rankingport.CallRecord{
		holder: {Right: 4, Resolved: 4, Voted: 4, Eligible: 4},
	}}
	if got = walkRanked(t, f, cabalID, c, ""); len(got.crowns) != 0 {
		t.Fatalf("crowns = %v, want none with four resolved", got.crowns)
	}
}

func TestListMembersPage_joinedOrderKeepsJoinOrder(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	cabalID, joined := largeCabal(t, f)
	c := &countingReads{bps: map[ids.UserID]int32{ids.UserIDFrom(joined[100]): 9000}}
	got := walkRanked(t, f, cabalID, c, app.OrderJoined)
	if !slices.Equal(got.order, joined) {
		t.Fatal("order=joined must list members oldest joiner first")
	}
}

func TestListMembersPage_refusesBadOrdersCursorsAndReads(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	seeded := testkit.NewCabal(t, f.pool, testkit.WithMembers(3))
	users := f.routes(nil).Users
	run := func(c *countingReads, req app.ListMembersQuery) error {
		req.CabalID, req.Limit = seeded.ID, 10
		_, err := app.ListMembersPage(t.Context(), f.pool, users, c.reads(f), req)
		return err
	}
	if err := run(&countingReads{}, app.ListMembersQuery{Order: "newest"}); errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("unknown order = %v", err)
	}
	id := seeded.Creator.ID.String()
	for name, raw := range map[string]string{
		"two parts":  "1\x00" + id,
		"bad rank":   "x\x000\x001\x00" + id,
		"bad bps":    "0\x00y\x001\x00" + id,
		"bad time":   "0\x000\x00z\x00" + id,
		"bad id":     "0\x000\x001\x00nope",
		"not base64": "",
	} {
		cursor := base64.RawURLEncoding.EncodeToString([]byte(raw))
		if name == "not base64" {
			cursor = "!!"
		}
		if err := run(
			&countingReads{},
			app.ListMembersQuery{Cursor: cursor},
		); errs.CodeOf(
			err,
		) != errs.CodeInvalidInput {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if err := run(
		&countingReads{ownErr: errs.New(errs.CodeDBUnavailable, "test")},
		app.ListMembersQuery{},
	); errs.CodeOf(
		err,
	) != errs.CodeInternal {
		t.Fatalf("ownership failure = %v", err)
	}
	if err := run(
		&countingReads{callErr: errs.New(errs.CodeDBUnavailable, "test")},
		app.ListMembersQuery{},
	); errs.CodeOf(
		err,
	) != errs.CodeInternal {
		t.Fatalf("records failure = %v", err)
	}
}

func TestGetCabalMembers_servesTheWiredShareAndRecord(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	deps := module.Deps{Pool: f.pool, UoW: f.uow, IDs: f.ids, Clock: f.clock}
	seeded := testkit.NewCabal(t, f.pool, testkit.WithMembers(2))
	joined := joinOrder(t, f, seeded.ID)
	for i, units := range []string{"3", "1"} {
		execSQL(t, f.pool, `INSERT INTO user_positions
			(user_id, cabal_id, share_units, contributed_micros, withdrawn_micros, updated_at)
			VALUES ($1, $2, $3::numeric, 0, 0, $4)`, joined[i], seeded.ID.UUID(), units, f.clock.Now())
	}
	m := cabal.New(deps, cabal.WithTreasuryWallets(adapters.AppWallets{Client: f.wallets}))
	m.Wire(module.NewSet(ranking.New(deps)))
	res, err := cabal.HTTPOf(m).GetCabalMembers(f.actor(t.Context()), api.GetCabalMembersRequestObject{
		Id: seeded.ID.UUID(),
	})
	page, ok := res.(api.GetCabalMembers200JSONResponse)
	if err != nil || !ok || len(page.Items) != 2 || page.Items[0].OwnershipBps != 7500 ||
		page.Items[1].OwnershipBps != 2500 {
		t.Fatalf("page = %+v, %v", res, err)
	}
	if page.Items[0].Crown || page.Totals.Admins != 1 || page.Totals.Voters != 1 {
		t.Fatalf("page = %+v", page)
	}
}

func TestGetCabalMembers_orderJoinedListsOldestFirst(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	seeded := testkit.NewCabal(t, f.pool, testkit.WithMembers(3))
	joined := joinOrder(t, f, seeded.ID)
	order := api.GetCabalMembersParamsOrder("joined")
	res, err := f.routes(nil).GetCabalMembers(f.actor(t.Context()), api.GetCabalMembersRequestObject{
		Id: seeded.ID.UUID(), Params: api.GetCabalMembersParams{Order: &order},
	})
	page, ok := res.(api.GetCabalMembers200JSONResponse)
	if err != nil || !ok || len(page.Items) != 3 {
		t.Fatalf("page = %+v, %v", res, err)
	}
	for i, item := range page.Items {
		if item.UserId != joined[i] {
			t.Fatalf("row %d = %v, want %v", i, item.UserId, joined[i])
		}
	}
}

func TestEVMWalletBackfillCabals_listsSeededCabalsAndRefusesBrokenReads(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	seeded := testkit.NewCabal(t, f.pool)
	deps := module.Deps{Pool: f.pool, UoW: f.uow, IDs: f.ids, Clock: f.clock}
	m := cabal.New(deps, cabal.WithTreasuryWallets(adapters.AppWallets{Client: f.wallets}))
	rows, err := m.EVMWalletBackfillCabals(t.Context())
	found := false
	for _, r := range rows {
		found = found || (r.Cabal == seeded.ID && !r.Held)
	}
	if err != nil || !found {
		t.Fatalf("rows = %+v, %v, want the seeded cabal without a wallet", rows, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := app.EVMWalletBackfillCabals(ctx, f.pool); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("canceled read = %v", err)
	}
}
