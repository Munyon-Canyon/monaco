package app_test

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type fakeUsers struct {
	known map[ids.UserID]identity.UserCard
	err   error
}

func (f fakeUsers) UsersByID(context.Context, []ids.UserID) (map[ids.UserID]identity.UserCard, error) {
	return f.known, f.err
}

type fakeMembers struct {
	byUser map[ids.UserID][]ids.CabalID
	failOn *ids.UserID
}

func (f fakeMembers) CabalsOf(_ context.Context, user ids.UserID) ([]ids.CabalID, error) {
	if f.failOn != nil && *f.failOn == user {
		return nil, errs.New(errs.CodeInternal, "test")
	}
	return f.byUser[user], nil
}

type sharedFixture struct {
	viewer, other        ids.UserID
	big, small, mine     ids.CabalID
	theirs               ids.CabalID
	ports                app.SharedPorts
	latest               *fakeLatest
	cards                *fakeCards
	members              fakeMembers
	contributionsAtSnaps fakeContributions
}

func newSharedFixture() sharedFixture {
	at := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	newID := func() ids.CabalID { return ids.CabalIDFrom(ids.Real{}.NewV7()) }
	f := sharedFixture{
		viewer: ids.UserIDFrom(ids.Real{}.NewV7()), other: ids.UserIDFrom(ids.Real{}.NewV7()),
		big: newID(), small: newID(), mine: newID(), theirs: newID(),
	}
	snap := func(value uint64) domain.Snapshot {
		return domain.Snapshot{
			At:          at,
			Value:       money.MicrosFromUint64(value),
			TotalShares: money.SharesUnitsFromUint64(1),
		}
	}
	f.latest = &fakeLatest{
		values: map[ids.CabalID]domain.Snapshot{f.big: snap(900), f.small: snap(300), f.mine: snap(1)},
	}
	f.cards = &fakeCards{cards: map[ids.CabalID]app.CabalView{
		f.big: {ID: f.big, Name: "Big"}, f.small: {ID: f.small, Name: "Small"},
	}}
	f.members = fakeMembers{byUser: map[ids.UserID][]ids.CabalID{
		f.viewer: {f.small, f.mine, f.big}, f.other: {f.big, f.theirs, f.small},
	}}
	f.contributionsAtSnaps = fakeContributions{points: []app.ContributionPoint{
		{At: at.Add(-time.Hour), NetContributed: money.SignedMicrosFromInt64(600)},
	}}
	f.ports = app.SharedPorts{
		Users:   fakeUsers{known: map[ids.UserID]identity.UserCard{f.other: {ID: f.other}}},
		Members: f.members, Latest: f.latest, Ledger: f.contributionsAtSnaps, Cards: f.cards,
	}
	return f
}

func (f sharedFixture) run(t *testing.T) ([]app.SharedCabal, error) {
	t.Helper()
	return app.ReadSharedCabals{Viewer: f.viewer, Other: f.other}.Run(t.Context(), f.ports)
}

func TestReadSharedCabals_OnlyTheSharedOnesLargestPotFirst(t *testing.T) {
	t.Parallel()
	f := newSharedFixture()
	got, err := f.run(t)
	if err != nil || len(got) != 2 || got[0].Cabal.Name != "Big" || got[1].Cabal.Name != "Small" {
		t.Fatalf("Run = %+v, %v, want Big then Small", got, err)
	}
	if got[0].Value.Uint64() != 900 || got[0].PnL.Int64() != 300 || got[0].Return == nil || *got[0].Return != 5000 {
		t.Fatalf("Big = %+v, want value 900, P&L 300 and a 5000 bps return", got[0])
	}
}

func TestReadSharedCabals_IsEmptyWithoutASharedOrValuedCabalAndReadsNothingMore(t *testing.T) {
	t.Parallel()
	f := newSharedFixture()
	f.ports.Members = fakeMembers{byUser: map[ids.UserID][]ids.CabalID{f.viewer: {f.mine}, f.other: {f.theirs}}}
	got, err := f.run(t)
	if err != nil || got == nil || len(got) != 0 || f.latest.queries != 0 {
		t.Fatalf("no shared cabal = %#v, %v, snapshot reads %d", got, err, f.latest.queries)
	}
	f = newSharedFixture()
	f.latest.values = nil
	if got, err = f.run(t); err != nil || got == nil || len(got) != 0 || f.cards.queries != 0 {
		t.Fatalf("no snapshot = %#v, %v, name reads %d", got, err, f.cards.queries)
	}
}

func TestReadSharedCabals_Failures(t *testing.T) {
	t.Parallel()
	boom := errs.New(errs.CodeInternal, "test")
	for name, tc := range map[string]struct {
		change func(*sharedFixture)
		want   errs.Code
	}{
		"unknown user": {func(f *sharedFixture) { f.ports.Users = fakeUsers{} }, errs.CodeUserNotFound},
		"users":        {func(f *sharedFixture) { f.ports.Users = fakeUsers{err: boom} }, errs.CodeInternal},
		"my cabals": {func(f *sharedFixture) {
			f.ports.Members = fakeMembers{byUser: f.members.byUser, failOn: &f.viewer}
		}, errs.CodeInternal},
		"their cabals": {func(f *sharedFixture) {
			f.ports.Members = fakeMembers{byUser: f.members.byUser, failOn: &f.other}
		}, errs.CodeInternal},
		"snapshots": {func(f *sharedFixture) { f.latest.err = boom }, errs.CodeInternal},
		"ledger":    {func(f *sharedFixture) { f.ports.Ledger = fakeContributions{err: boom} }, errs.CodeInternal},
		"pnl": {func(f *sharedFixture) {
			f.ports.Ledger = fakeContributions{points: []app.ContributionPoint{
				{At: time.Unix(0, 0), NetContributed: money.SignedMicrosFromInt64(math.MinInt64)},
			}}
		}, errs.CodeInternal},
		"names":        {func(f *sharedFixture) { f.cards.err = boom }, errs.CodeInternal},
		"missing name": {func(f *sharedFixture) { f.cards.cards = nil }, errs.CodeInternal},
	} {
		f := newSharedFixture()
		tc.change(&f)
		if _, err := f.run(t); errs.CodeOf(err) != tc.want {
			t.Errorf("%s: err = %v, want %s", name, err, tc.want)
		}
	}
}
