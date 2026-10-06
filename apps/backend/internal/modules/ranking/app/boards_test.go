package app

import (
	"math"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type boardRig struct {
	at         time.Time
	alpha, bet ids.CabalID
	u1, u2, u3 ids.UserID
	input      boardInput
}

func newBoardRig() boardRig {
	at := valuationTime()
	r := boardRig{
		at:    at,
		alpha: ids.CabalIDFrom(ids.Real{}.NewV7()), bet: ids.CabalIDFrom(ids.Real{}.NewV7()),
		u1: ids.UserIDFrom(ids.Real{}.NewV7()), u2: ids.UserIDFrom(ids.Real{}.NewV7()),
		u3: ids.UserIDFrom(ids.Real{}.NewV7()),
	}
	micros, shares, signed := money.MicrosFromUint64, money.SharesUnitsFromUint64, money.SignedMicrosFromInt64
	r.input = boardInput{
		at: at,
		views: map[ids.CabalID]cabalport.CabalView{
			r.alpha: {ID: r.alpha, Name: "Alpha", PictureURL: "a.png", CreatedAt: at.Add(-48 * time.Hour)},
			r.bet:   {ID: r.bet, Name: "Beta", CreatedAt: at.Add(-24 * time.Hour)},
		},
		members: map[ids.CabalID][]cabalport.MemberView{
			r.alpha: {{UserID: r.u1}},
			r.bet:   {{UserID: r.u1}, {UserID: r.u2}, {UserID: r.u3}},
		},
		stakes: []treasury.MemberStake{
			{CabalID: r.alpha, UserID: r.u1, ShareUnits: shares(100), NetContributedMicros: signed(100_000_000)},
			{CabalID: r.bet, UserID: r.u1, ShareUnits: shares(50), NetContributedMicros: signed(50_000_000)},
			{CabalID: r.bet, UserID: r.u2, ShareUnits: shares(50), NetContributedMicros: signed(100_000_000)},
		},
		users: map[ids.UserID]identity.UserCard{
			r.u1: {
				ID:          r.u1,
				Handle:      "one",
				DisplayName: "One",
				PhotoURL:    "one.png",
				CreatedAt:   at.Add(-72 * time.Hour),
			},
			r.u2: {ID: r.u2, Handle: "two", CreatedAt: at.Add(-70 * time.Hour)},
			r.u3: {ID: r.u3, CreatedAt: at.Add(-60 * time.Hour)},
		},
		valued: []CabalValue{
			{CabalID: r.alpha, Value: micros(150_000_000), TotalShares: shares(100)},
			{CabalID: r.bet, Value: micros(120_000_000), TotalShares: shares(100)},
		},
	}
	return r
}

func byBoard(entries []Entry) map[string][]Entry {
	out := map[string][]Entry{}
	for _, entry := range entries {
		out[entry.Board] = append(out[entry.Board], entry)
	}
	return out
}

func assertOrder(t *testing.T, board string, got []Entry, want ...uuid.UUID) {
	t.Helper()
	order := make([]uuid.UUID, len(got))
	for i, entry := range got {
		order[i] = entry.SubjectID
		if entry.Rank != i+1 {
			t.Fatalf("%s row %d has rank %d, want %d", board, i, entry.Rank, i+1)
		}
	}
	if !slices.Equal(order, want) {
		t.Fatalf("%s subjects = %v, want %v", board, order, want)
	}
}

func TestBuildEntries_ranksThePeopleCabalsAndMembersBoards(t *testing.T) {
	t.Parallel()
	r := newBoardRig()
	entries, err := buildEntries(r.input)
	if err != nil {
		t.Fatal(err)
	}
	boards := byBoard(entries)
	if len(boards) != 4 {
		t.Fatalf("boards = %d, want cabals, people and a members board per cabal", len(boards))
	}
	assertOrder(t, "cabals", boards[cabalsBoard], r.alpha.UUID(), r.bet.UUID())
	assertOrder(t, "people", boards[peopleBoard], r.u1.UUID(), r.u2.UUID())
	assertOrder(t, "alpha members", boards[MembersBoard(r.alpha.UUID())], r.u1.UUID())
	assertOrder(t, "beta members", boards[MembersBoard(r.bet.UUID())], r.u1.UUID(), r.u2.UUID(), r.u3.UUID())
}

func TestBuildEntries_fillsEveryFieldOfARow(t *testing.T) {
	t.Parallel()
	r := newBoardRig()
	entries, err := buildEntries(r.input)
	if err != nil {
		t.Fatal(err)
	}
	boards := byBoard(entries)
	picture, bps := "a.png", int64(5000)
	want := Entry{
		Board: cabalsBoard, Range: allRange, Rank: 1, SubjectID: r.alpha.UUID(), SubjectName: "Alpha",
		SubjectPictureURL: &picture, SubjectCreatedAt: r.input.views[r.alpha].CreatedAt, ValueMicros: 150_000_000,
		PnLMicros: 50_000_000, ReturnBps: &bps, PricesAsOf: r.at, ComputedAt: r.at, Flags: []string{},
	}
	one, two := boards[peopleBoard][0], boards[peopleBoard][1]
	unranked := boards[MembersBoard(r.bet.UUID())][2]
	if !reflect.DeepEqual(boards[cabalsBoard][0], want) || one.SubjectName != "One" || *one.SubjectHandle != "one" ||
		one.ValueMicros != 210_000_000 || *one.ReturnBps != 4000 || two.SubjectName != "two" ||
		*two.ReturnBps != -4000 || unranked.ReturnBps != nil || unranked.ValueMicros != 0 {
		t.Fatalf("rows = %+v, %+v, %+v, %+v", boards[cabalsBoard][0], one, two, unranked)
	}
}

func TestBuildEntries_leavesAnEmptyBoardEmpty(t *testing.T) {
	t.Parallel()
	entries, err := buildEntries(boardInput{at: valuationTime()})
	if err != nil || entries == nil || len(entries) != 0 {
		t.Fatalf("buildEntries(nothing) = %v, %v, want an empty built slice", entries, err)
	}
}

func TestBuildEntries_skipsIneligibleDeletedAndUnknownUsers(t *testing.T) {
	t.Parallel()
	r := newBoardRig()
	r.input.users[r.u2] = identity.UserCard{ID: r.u2, Handle: "two", Deleted: true}
	delete(r.input.users, r.u1)
	r.input.stakes[0].NetContributedMicros = money.SignedMicrosFromInt64(0)
	entries, err := buildEntries(r.input)
	if err != nil {
		t.Fatal(err)
	}
	boards := byBoard(entries)
	if len(boards[peopleBoard]) != 0 || len(boards[MembersBoard(r.bet.UUID())]) != 1 {
		t.Fatalf("boards = %+v, want nobody ranked and only the unnamed member listed", boards)
	}
	assertOrder(t, "cabals", boards[cabalsBoard], r.bet.UUID())
}

func TestBuildEntries_keepsAFlaggedCabalsPreviousRowsWithTheFlag(t *testing.T) {
	t.Parallel()
	r := newBoardRig()
	stale := ids.CabalIDFrom(ids.Real{}.NewV7())
	bps := int64(7000)
	previous := []Entry{
		{
			Board: cabalsBoard, Range: allRange, Rank: 1, SubjectID: stale.UUID(), SubjectName: "Stale",
			SubjectCreatedAt: r.at.Add(-96 * time.Hour), ValueMicros: 9_000_000, PnLMicros: 3_000_000, ReturnBps: &bps,
			PricesAsOf: r.at.Add(-time.Hour), ComputedAt: r.at.Add(-time.Hour), Flags: []string{},
		},
		{
			Board: MembersBoard(stale.UUID()), Range: allRange, Rank: 1, SubjectID: r.u1.UUID(), SubjectName: "One",
			SubjectCreatedAt: r.at, ValueMicros: 4, PricesAsOf: r.at.Add(-time.Hour), ComputedAt: r.at.Add(-time.Hour),
			Flags: []string{string(domain.FlagStalePrices)},
		},
	}
	r.input.flagged = []CabalValue{{CabalID: stale, Flags: []domain.Flag{domain.FlagStalePrices}}}
	r.input.previous = previous
	entries, err := buildEntries(r.input)
	if err != nil {
		t.Fatal(err)
	}
	boards := byBoard(entries)
	assertOrder(t, "cabals", boards[cabalsBoard], stale.UUID(), r.alpha.UUID(), r.bet.UUID())
	carried := boards[cabalsBoard][0]
	if carried.SubjectName != "Stale" || carried.ValueMicros != 9_000_000 || carried.PnLMicros != 3_000_000 ||
		*carried.ReturnBps != 7000 || len(carried.Flags) != 1 || carried.Flags[0] != string(domain.FlagStalePrices) ||
		!carried.PricesAsOf.Equal(r.at) {
		t.Fatalf("carried cabal row = %+v, want the old values with the stale flag", carried)
	}
	members := boards[MembersBoard(stale.UUID())]
	if len(members) != 1 || len(members[0].Flags) != 1 || members[0].ValueMicros != 4 {
		t.Fatalf("carried members row = %+v, want one row with the flag merged once", members)
	}
}

func TestBuildEntries_flagsAPersonWhoHoldsAFlaggedCabal(t *testing.T) {
	t.Parallel()
	r := newBoardRig()
	r.input.flagged = []CabalValue{{CabalID: r.bet, Flags: []domain.Flag{domain.FlagUnpricedAssets}}}
	r.input.valued = r.input.valued[:1]
	entries, err := buildEntries(r.input)
	if err != nil {
		t.Fatal(err)
	}
	people := byBoard(entries)[peopleBoard]
	if len(people) != 1 || people[0].SubjectID != r.u1.UUID() || len(people[0].Flags) != 1 ||
		people[0].Flags[0] != string(domain.FlagUnpricedAssets) {
		t.Fatalf("people = %+v, want the holder of the flagged cabal flagged", people)
	}
}

func TestBuildEntries_refusesWhatCannotBeStored(t *testing.T) {
	t.Parallel()
	hugeNet := money.SignedMicrosFromInt64(math.MaxInt64)
	hugeValue := money.MicrosFromUint64(math.MaxInt64 + 1)
	for name, tc := range map[string]struct {
		change func(r *boardRig)
		want   errs.Code
	}{
		"a cabal's net contribution beyond int64": {func(r *boardRig) {
			r.input.stakes = append(r.input.stakes, treasury.MemberStake{
				CabalID: r.bet, UserID: r.u3, ShareUnits: money.SharesUnitsFromUint64(1), NetContributedMicros: hugeNet,
			}, treasury.MemberStake{CabalID: r.bet, UserID: r.u3, NetContributedMicros: hugeNet})
		}, errs.CodeInvalidInput},
		"a person's net contribution beyond int64": {func(r *boardRig) {
			r.input.stakes = []treasury.MemberStake{
				{CabalID: r.alpha, UserID: r.u1, ShareUnits: money.SharesUnitsFromUint64(1), NetContributedMicros: hugeNet},
				{CabalID: r.bet, UserID: r.u1, ShareUnits: money.SharesUnitsFromUint64(1), NetContributedMicros: hugeNet},
			}
		}, errs.CodeInvalidInput},
		"a value beyond int64": {func(r *boardRig) {
			r.input.valued[0].Value = hugeValue
			r.input.stakes = r.input.stakes[:1]
			r.input.stakes[0].NetContributedMicros = hugeNet
			r.input.stakes[0].ShareUnits = money.SharesUnitsFromUint64(100)
		}, errs.CodeInvalidInput},
		"more shares than the cabal has": {func(r *boardRig) {
			r.input.stakes[0].ShareUnits = money.SharesUnitsFromUint64(101)
		}, errs.CodeInvalidInput},
		"a previous row with a negative value": {func(r *boardRig) {
			r.input.previous = []Entry{{Board: cabalsBoard, SubjectID: ids.Real{}.NewV7(), ValueMicros: -1}}
		}, errs.CodeDecodeFailed},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := newBoardRig()
			tc.change(&r)
			if _, err := buildEntries(r.input); errs.CodeOf(err) != tc.want {
				t.Fatalf("buildEntries() = %v, want code %q", err, tc.want)
			}
		})
	}
}
