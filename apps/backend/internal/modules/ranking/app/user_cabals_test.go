package app_test

import (
	"context"
	"errors"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type fakeBlocks struct {
	blocked map[[2]ids.UserID]bool
}

func (f fakeBlocks) BlockedByMe(_ context.Context, viewer, user ids.UserID) (bool, error) {
	return f.blocked[[2]ids.UserID{viewer, user}], nil
}

type fakeRolesByCabal map[ids.CabalID][]cabal.MemberView

func (f fakeRolesByCabal) MembersOf(context.Context, []ids.CabalID) (map[ids.CabalID][]cabal.MemberView, error) {
	return f, nil
}

type userCabalsFixture struct {
	viewer, other              ids.UserID
	acCapital, degens, ballers ids.CabalID
	quiet                      ids.CabalID
	card                       identity.UserCard
	ports                      app.UserCabalPorts
	owned                      map[ids.CabalID]int32
	blocks                     fakeBlocks
	roles                      fakeRolesByCabal
	cards                      *fakeCards
	theirs, mine               []ids.CabalID
}

func newUserCabalsFixture() *userCabalsFixture {
	newID := func() ids.CabalID { return ids.CabalIDFrom(ids.Real{}.NewV7()) }
	f := &userCabalsFixture{
		viewer: ids.UserIDFrom(ids.Real{}.NewV7()), other: ids.UserIDFrom(ids.Real{}.NewV7()),
		acCapital: newID(), degens: newID(), ballers: newID(), quiet: newID(),
	}
	f.card = identity.UserCard{ID: f.other, SharePerformance: true}
	f.theirs = []ids.CabalID{f.ballers, f.degens, f.quiet, f.acCapital}
	f.mine = []ids.CabalID{f.acCapital, f.degens}
	f.owned = map[ids.CabalID]int32{f.acCapital: 2000, f.degens: 1500, f.ballers: 800}
	f.roles = fakeRolesByCabal{
		f.acCapital: {{UserID: f.other, Role: cabal.RoleCreator, CanVote: true}},
		f.degens:    {{UserID: f.other, Role: cabal.RoleMember, CanVote: true}},
		f.ballers:   {{UserID: f.other, Role: cabal.RoleMember, CanVote: false}},
		f.quiet:     {{UserID: f.other, Role: cabal.RoleMember, CanVote: true}},
	}
	f.cards = &fakeCards{cards: map[ids.CabalID]app.CabalView{
		f.acCapital: {ID: f.acCapital, Name: "AC Capital", MemberCount: 8},
		f.degens:    {ID: f.degens, Name: "Degens", MemberCount: 6},
		f.ballers:   {ID: f.ballers, Name: "Big Ballers", MemberCount: 12},
		f.quiet:     {ID: f.quiet, Name: "Aardvarks", MemberCount: 3},
	}}
	f.blocks = fakeBlocks{blocked: map[[2]ids.UserID]bool{}}
	return f
}

func (f *userCabalsFixture) run(t *testing.T) ([]app.UserCabal, error) {
	t.Helper()
	f.ports = app.UserCabalPorts{
		Users:     fakeUsers{known: map[ids.UserID]identity.UserCard{f.card.ID: f.card}},
		Blocks:    f.blocks,
		Members:   fakeMembers{byUser: map[ids.UserID][]ids.CabalID{f.viewer: f.mine, f.other: f.theirs}},
		Cards:     f.cards,
		Ownership: fakeOwnership{bps: f.owned},
		Roles:     f.roles,
	}
	return app.ReadUserCabals{Viewer: f.viewer, Other: f.other}.Run(t.Context(), f.ports)
}

func TestReadUserCabals_OrdersSharedFirstThenShare(t *testing.T) {
	t.Parallel()
	f := newUserCabalsFixture()
	rows, err := f.run(t)
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		id      ids.CabalID
		shared  bool
		role    app.Role
		canVote bool
		bps     int32
	}{
		{f.acCapital, true, app.RoleAdmin, true, 2000},
		{f.degens, true, app.RoleMember, true, 1500},
		{f.ballers, false, app.RoleMember, false, 800},
		{f.quiet, false, app.RoleMember, true, 0},
	}
	if len(rows) != len(want) {
		t.Fatalf("rows = %+v, want %d", rows, len(want))
	}
	for i, w := range want {
		got := rows[i]
		if got.Cabal.ID != w.id || got.Shared != w.shared || got.Role != w.role || got.CanVote != w.canVote ||
			got.OwnershipBps == nil || *got.OwnershipBps != w.bps {
			t.Errorf("row %d = %+v (bps %v), want %+v", i, got, got.OwnershipBps, w)
		}
	}
}

func TestReadUserCabals_KeepsCabalWithoutStake(t *testing.T) {
	t.Parallel()
	f := newUserCabalsFixture()
	delete(f.owned, f.ballers)
	rows, err := f.run(t)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.Cabal.ID == f.ballers && (row.OwnershipBps == nil || *row.OwnershipBps != 0) {
			t.Fatalf("cabal without a stake = %v, want kept with 0", row.OwnershipBps)
		}
	}
	if len(rows) != 4 {
		t.Fatalf("rows = %d, want all 4 cabals", len(rows))
	}
}

func TestReadUserCabals_UnevenSplitMatchesOwnershipPort(t *testing.T) {
	t.Parallel()
	f := newUserCabalsFixture()
	f.owned = map[ids.CabalID]int32{f.acCapital: 3334, f.degens: 3333, f.ballers: 3333}
	rows, err := f.run(t)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if want, ok := f.owned[row.Cabal.ID]; ok && *row.OwnershipBps != want {
			t.Errorf("%s = %d, want the port's %d", row.Cabal.Name, *row.OwnershipBps, want)
		}
	}
}

func TestReadUserCabals_HiddenPerformance(t *testing.T) {
	t.Parallel()
	f := newUserCabalsFixture()
	f.card.SharePerformance = false
	delete(f.owned, f.ballers)
	rows, err := f.run(t)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 4 {
		t.Fatalf("rows = %d, want all 4", len(rows))
	}
	for _, row := range rows {
		if row.OwnershipBps != nil {
			t.Errorf("%s = %d, want null for another viewer, valued or not", row.Cabal.Name, *row.OwnershipBps)
		}
	}
	if !rows[0].Shared || !rows[1].Shared || rows[0].Cabal.Name != "AC Capital" || rows[1].Cabal.Name != "Degens" {
		t.Errorf("order = %+v, want shared first then by name", rows)
	}
}

func TestReadUserCabals_HiddenPerformanceShownToSelf(t *testing.T) {
	t.Parallel()
	f := newUserCabalsFixture()
	f.card.SharePerformance = false
	f.viewer = f.other
	f.mine = f.theirs
	rows, err := f.run(t)
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].OwnershipBps == nil || *rows[0].OwnershipBps != 2000 {
		t.Fatalf("own first row = %+v, want the real 2000", rows[0])
	}
}

func TestReadUserCabals_BlockEitherDirectionIsEmpty(t *testing.T) {
	t.Parallel()
	for name, blocked := range map[string]func(*userCabalsFixture) [2]ids.UserID{
		"viewer blocks other": func(f *userCabalsFixture) [2]ids.UserID { return [2]ids.UserID{f.viewer, f.other} },
		"other blocks viewer": func(f *userCabalsFixture) [2]ids.UserID { return [2]ids.UserID{f.other, f.viewer} },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newUserCabalsFixture()
			f.blocks.blocked[blocked(f)] = true
			rows, err := f.run(t)
			if err != nil || rows == nil || len(rows) != 0 {
				t.Fatalf("rows, err = %v, %v, want an empty list", rows, err)
			}
		})
	}
}

func TestReadUserCabals_UnknownUser(t *testing.T) {
	t.Parallel()
	f := newUserCabalsFixture()
	f.card.ID = ids.UserIDFrom(ids.Real{}.NewV7())
	_, err := f.run(t)
	var e *errs.Error
	if !errors.As(err, &e) || e.Code != errs.CodeUserNotFound {
		t.Fatalf("err = %v, want user_not_found", err)
	}
}

func TestReadUserCabals_UnwiredBlocksIsUnavailable(t *testing.T) {
	t.Parallel()
	f := newUserCabalsFixture()
	if _, err := f.run(t); err != nil {
		t.Fatal(err)
	}
	f.ports.Blocks = app.UnwiredBlocks{}
	_, err := app.ReadUserCabals{Viewer: f.viewer, Other: f.other}.Run(t.Context(), f.ports)
	if errs.CodeOf(err) != errs.CodeUpstreamUnavailable {
		t.Fatalf("err = %v, want upstream_unavailable", err)
	}
}

func TestReadUserCabals_PortFailuresSurface(t *testing.T) {
	t.Parallel()
	boom := errs.New(errs.CodeInternal, "test")
	cases := map[string]func(*app.UserCabalPorts, *userCabalsFixture){
		"users":  func(p *app.UserCabalPorts, _ *userCabalsFixture) { p.Users = fakeUsers{err: boom} },
		"theirs": func(p *app.UserCabalPorts, f *userCabalsFixture) { p.Members = fakeMembers{failOn: &f.other} },
		"mine": func(p *app.UserCabalPorts, f *userCabalsFixture) {
			p.Members = fakeMembers{byUser: map[ids.UserID][]ids.CabalID{f.other: f.theirs}, failOn: &f.viewer}
		},
		"cards": func(p *app.UserCabalPorts, _ *userCabalsFixture) { p.Cards = &fakeCards{err: boom} },
		"missing": func(p *app.UserCabalPorts, _ *userCabalsFixture) {
			p.Cards = &fakeCards{cards: map[ids.CabalID]app.CabalView{}}
		},
		"roles":     func(p *app.UserCabalPorts, _ *userCabalsFixture) { p.Roles = failingRoles{} },
		"ownership": func(p *app.UserCabalPorts, _ *userCabalsFixture) { p.Ownership = fakeOwnership{err: boom} },
	}
	for name, sabotage := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newUserCabalsFixture()
			if _, err := f.run(t); err != nil {
				t.Fatal(err)
			}
			sabotage(&f.ports, f)
			if _, err := (app.ReadUserCabals{Viewer: f.viewer, Other: f.other}).Run(t.Context(), f.ports); err == nil {
				t.Fatal("err = nil, want the port's failure")
			}
		})
	}
}

type failingRoles struct{}

func (failingRoles) MembersOf(context.Context, []ids.CabalID) (map[ids.CabalID][]cabal.MemberView, error) {
	return nil, errs.New(errs.CodeInternal, "test")
}
