package app

import (
	"errors"
	"math"
	"math/rand/v2"
	"slices"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type skipLog struct {
	cabal  ids.CabalID
	rng    domain.Range
	reason string
}

type rangedRig struct {
	boardRig
	book  *rangedBook
	skips []skipLog
}

func newRangedRig(rng domain.Range) *rangedRig {
	r := &rangedRig{boardRig: newBoardRig()}
	t0, _ := rng.Start(r.at)
	r.book = &rangedBook{
		Window: Window{Range: rng, T0: t0, T1: r.at}, stakes: map[MemberKey]money.SharesUnits{},
		shares: map[ids.CabalID]money.SharesUnits{}, users: map[ids.CabalID][]ids.UserID{},
		snaps: map[ids.CabalID]*Snapshot{}, flows: map[MemberKey][]domain.Flow{},
		cabalFlows: map[ids.CabalID][]domain.Flow{},
	}
	r.input.ranged = []rangedBook{*r.book}
	r.input.skipped = func(id ids.CabalID, w Window, reason string) {
		r.skips = append(r.skips, skipLog{cabal: id, rng: w.Range, reason: reason})
	}
	return r
}

func (r *rangedRig) alphaOnly(value, total uint64, net int64) {
	r.input.valued = []CabalValue{{
		CabalID: r.alpha, Value: money.MicrosFromUint64(value), TotalShares: money.SharesUnitsFromUint64(total),
	}}
	r.input.stakes = []treasury.MemberStake{{
		CabalID: r.alpha, UserID: r.u1, ShareUnits: money.SharesUnitsFromUint64(total),
		NetContributedMicros: money.SignedMicrosFromInt64(net),
	}}
}

func (r *rangedRig) startedWith(cabalID ids.CabalID, user ids.UserID, held, total, value uint64) {
	key := MemberKey{UserID: user, CabalID: cabalID}
	r.book.stakes[key] = money.SharesUnitsFromUint64(held)
	r.book.shares[cabalID] = money.SharesUnitsFromUint64(total)
	r.book.users[cabalID] = append(r.book.users[cabalID], user)
	r.book.snaps[cabalID] = &Snapshot{
		At: r.book.T0.Add(-time.Minute), Value: money.MicrosFromUint64(value),
		TotalShares: money.SharesUnitsFromUint64(total),
	}
}

func (r *rangedRig) flow(cabalID ids.CabalID, user ids.UserID, micros int64, at time.Time) {
	key := MemberKey{UserID: user, CabalID: cabalID}
	f := domain.Flow{Amount: money.SignedMicrosFromInt64(micros), At: at}
	r.book.flows[key] = append(r.book.flows[key], f)
	r.book.cabalFlows[cabalID] = append(r.book.cabalFlows[cabalID], f)
	r.book.users[cabalID] = append(r.book.users[cabalID], user)
}

func (r *rangedRig) build(t *testing.T) map[string][]Entry {
	t.Helper()
	r.input.ranged = []rangedBook{*r.book}
	entries, err := buildEntries(r.input)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]Entry{}
	for _, entry := range entries {
		key := entry.Board + "/" + entry.Range
		out[key] = append(out[key], entry)
	}
	return out
}

func wantRow(t *testing.T, rows []Entry, subject ids.UserID, pnl int64, bps int64) {
	t.Helper()
	if len(rows) != 1 || rows[0].PnLMicros != pnl || rows[0].ReturnBps == nil || *rows[0].ReturnBps != bps ||
		rows[0].SubjectID != subject.UUID() {
		t.Fatalf("rows = %+v, want one row for %v with %d micros and %d bps", rows, subject, pnl, bps)
	}
}

func TestRangedBoard_MidRangeDepositShowsNoFakeGain(t *testing.T) {
	t.Parallel()
	r := newRangedRig(domain.Range1W)
	r.alphaOnly(200_000_000, 200, 200_000_000)
	r.startedWith(r.alpha, r.u1, 100, 100, 100_000_000)
	r.flow(r.alpha, r.u1, 100_000_000, r.at.Add(-3*24*time.Hour))
	boards := r.build(t)
	wantRow(t, boards["people/1W"], r.u1, 0, 0)
	wantRow(t, boards["cabal_members:"+r.alpha.UUID().String()+"/1W"], r.u1, 0, 0)
	cabals := boards["cabals/1W"]
	if len(cabals) != 1 || cabals[0].PnLMicros != 0 || *cabals[0].ReturnBps != 0 ||
		cabals[0].ValueMicros != 200_000_000 {
		t.Fatalf("cabals 1W = %+v, want a flat row at the current value", cabals)
	}
	if len(boards["people/ALL"]) != 1 || *boards["people/ALL"][0].ReturnBps != 0 {
		t.Fatalf("people ALL = %+v, want the lifetime row beside the range", boards["people/ALL"])
	}
	if len(r.skips) != 0 {
		t.Fatalf("skips = %+v, want none", r.skips)
	}
}

func TestRangedBoard_CabalYoungerThanRangeStartsFromZeroAndRanksByDietz(t *testing.T) {
	t.Parallel()
	r := newRangedRig(domain.Range1W)
	r.alphaOnly(110_000_000, 100, 100_000_000)
	r.flow(r.alpha, r.u1, 100_000_000, r.at.Add(-24*time.Hour))
	boards := r.build(t)
	wantRow(t, boards["people/1W"], r.u1, 10_000_000, 7_000)
	wantRow(t, boards["cabal_members:"+r.alpha.UUID().String()+"/1W"], r.u1, 10_000_000, 7_000)
	if cabals := boards["cabals/1W"]; len(cabals) != 1 || *cabals[0].ReturnBps != 7_000 {
		t.Fatalf("cabals 1W = %+v, want the Dietz return from a zero start", cabals)
	}
	if len(r.skips) != 0 {
		t.Fatalf("skips = %+v, a young cabal is not a skip", r.skips)
	}
}

func TestRangedBoard_CashOutInRangeIsNotALoss(t *testing.T) {
	t.Parallel()
	r := newRangedRig(domain.Range1W)
	r.alphaOnly(100_000_000, 50, 100_000_000)
	r.startedWith(r.alpha, r.u1, 100, 100, 200_000_000)
	r.flow(r.alpha, r.u1, -100_000_000, r.at.Add(-2*24*time.Hour))
	boards := r.build(t)
	wantRow(t, boards["people/1W"], r.u1, 0, 0)
	if cabals := boards["cabals/1W"]; len(cabals) != 1 || cabals[0].PnLMicros != 0 {
		t.Fatalf("cabals 1W = %+v, want no loss for a cash out", cabals)
	}
}

func TestRangedBoard_AMemberWithoutAUsableStartIsSkippedAndFlagged(t *testing.T) {
	t.Parallel()
	for name, setup := range map[string]func(*rangedRig){
		"no snapshot": func(*rangedRig) {},
		"snapshot too old": func(r *rangedRig) {
			r.book.snaps[r.alpha] = &Snapshot{
				At: r.book.T0.Add(-time.Hour), Value: money.MicrosFromUint64(100_000_000),
				TotalShares: money.SharesUnitsFromUint64(100),
			}
		},
		"shares above total": func(r *rangedRig) {
			r.startedWith(r.alpha, r.u1, 100, 100, 100_000_000)
			r.book.snaps[r.alpha].TotalShares = money.SharesUnitsFromUint64(10)
		},
		"snapshot after t0": func(r *rangedRig) {
			r.startedWith(r.alpha, r.u1, 100, 100, 100_000_000)
			r.book.snaps[r.alpha].At = r.book.T0.Add(time.Second)
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := newRangedRig(domain.Range1D)
			r.alphaOnly(150_000_000, 100, 100_000_000)
			r.book.stakes[MemberKey{UserID: r.u1, CabalID: r.alpha}] = money.SharesUnitsFromUint64(100)
			r.book.shares[r.alpha] = money.SharesUnitsFromUint64(100)
			setup(r)
			wantSkippedMember(t, r, name != "shares above total")
		})
	}
}

func wantSkippedMember(t *testing.T, r *rangedRig, cabalOmitted bool) {
	t.Helper()
	boards := r.build(t)
	people := boards["people/1D"]
	if len(people) != 1 || people[0].ReturnBps != nil || people[0].PnLMicros != 0 ||
		!slices.Equal(people[0].Flags, []string{"stale_prices"}) || (cabalOmitted && len(boards["cabals/1D"]) != 0) {
		t.Fatalf(
			"1D boards = %+v, %+v, want the person unranked and flagged, and no cabal row",
			people,
			boards["cabals/1D"],
		)
	}
	wantFlaggedMember(t, boards["cabal_members:"+r.alpha.UUID().String()+"/1D"])
	if len(r.skips) == 0 || r.skips[0] != (skipLog{cabal: r.alpha, rng: domain.Range1D, reason: unusableStart}) {
		t.Fatalf("skips = %+v, want the cabal logged for 1D", r.skips)
	}
	if len(boards["people/ALL"]) != 1 {
		t.Fatalf("people ALL = %+v, want the lifetime board untouched", boards["people/ALL"])
	}
}

func TestRangedBoard_ASkippedCabalLeavesThePersonsSumAndFlagsTheRow(t *testing.T) {
	t.Parallel()
	r := newRangedRig(domain.Range1W)
	r.startedWith(r.alpha, r.u1, 100, 100, 100_000_000)
	r.book.stakes[MemberKey{UserID: r.u1, CabalID: r.bet}] = money.SharesUnitsFromUint64(50)
	r.book.shares[r.bet] = money.SharesUnitsFromUint64(100)
	boards := r.build(t)
	people := boards["people/1W"]
	if len(people) != 1 || people[0].SubjectID != r.u1.UUID() || *people[0].ReturnBps != 5_000 ||
		!slices.Equal(people[0].Flags, []string{"stale_prices"}) {
		t.Fatalf("people 1W = %+v, want only the usable cabal, flagged", people)
	}
	if len(r.skips) == 0 {
		t.Fatal("skips = none, want the unusable cabal logged")
	}
}

func TestRangedBoard_ACashedOutCabalStillCountsInThePersonsSum(t *testing.T) {
	t.Parallel()
	r := newRangedRig(domain.Range1W)
	r.alphaOnly(100_000_000, 100, 100_000_000)
	r.input.valued = append(r.input.valued, CabalValue{
		CabalID: r.bet, Value: money.MicrosFromUint64(50_000_000), TotalShares: money.SharesUnitsFromUint64(50),
	})
	r.input.stakes = append(r.input.stakes, treasury.MemberStake{
		CabalID: r.bet, UserID: r.u2, ShareUnits: money.SharesUnitsFromUint64(50),
		NetContributedMicros: money.SignedMicrosFromInt64(50_000_000),
	})
	r.startedWith(r.alpha, r.u1, 100, 100, 100_000_000)
	r.startedWith(r.bet, r.u1, 10, 10, 20_000_000)
	r.flow(r.bet, r.u1, -30_000_000, r.at.Add(-24*time.Hour))
	boards := r.build(t)
	wantRow(t, boards["people/1W"], r.u1, 10_000_000, 864)
}

func TestRangedBoard_KeepsAFlaggedCabalsPreviousRowsInTheirOwnRange(t *testing.T) {
	t.Parallel()
	r := newRangedRig(domain.Range1W)
	stale := ids.CabalIDFrom(ids.Real{}.NewV7())
	row := func(rng string, value int64, bps int64) Entry {
		return Entry{
			Board: cabalsBoard, Range: rng, Rank: 1, SubjectID: stale.UUID(), SubjectName: "Stale",
			ValueMicros: value, ReturnBps: &bps,
		}
	}
	member := Entry{Board: MembersBoard(stale.UUID()), Range: "1W", Rank: 1, SubjectID: r.u1.UUID(), ValueMicros: 4}
	r.input.flagged = []CabalValue{{CabalID: stale, Flags: []domain.Flag{domain.FlagStalePrices}}}
	r.input.previous = []Entry{row(allRange, 9_000_000, 7_000), row("1W", 5_000_000, 300), member}
	boards := r.build(t)
	if all := boards["cabals/ALL"]; len(all) != 3 || all[0].SubjectID != stale.UUID() ||
		all[0].ValueMicros != 9_000_000 {
		t.Fatalf("cabals ALL = %+v, want the carried lifetime row first", all)
	}
	week := boards["cabals/1W"]
	if len(week) != 1 || week[0].SubjectID != stale.UUID() || week[0].ValueMicros != 5_000_000 ||
		week[0].Rank != 1 || !slices.Equal(week[0].Flags, []string{"stale_prices"}) {
		t.Fatalf("cabals 1W = %+v, want the carried week row alone and flagged", week)
	}
	if carried := boards["cabal_members:"+stale.UUID().String()+"/1W"]; len(carried) != 1 ||
		carried[0].ValueMicros != 4 {
		t.Fatalf("members 1W = %+v, want the carried members row in its range", carried)
	}
}

func TestRangedBoard_RefusesGainsThatDoNotFit(t *testing.T) {
	t.Parallel()
	maxInt, cashOut := uint64(math.MaxInt64), int64(math.MaxInt64)
	for name, setup := range map[string]func(*rangedRig){
		"a cabal's gain": func(r *rangedRig) {
			r.alphaOnly(10, 100, 1_000_000)
			r.book.cabalFlows[r.alpha] = []domain.Flow{
				{Amount: money.SignedMicrosFromInt64(-cashOut), At: r.at.Add(-time.Hour)},
			}
		},
		"a member's gain": func(r *rangedRig) {
			r.alphaOnly(10, 100, 1_000_000)
			r.startedWith(r.alpha, r.u2, 100, 100, 10)
			r.book.flows[MemberKey{UserID: r.u1, CabalID: r.alpha}] = []domain.Flow{
				{Amount: money.SignedMicrosFromInt64(-cashOut), At: r.at.Add(-time.Hour)},
			}
		},
		"a person's start sum": func(r *rangedRig) {
			r.twoCabals(10)
			for _, id := range []ids.CabalID{r.alpha, r.bet} {
				r.startedWith(id, r.u1, 100, 100, maxInt+1)
				r.flow(id, r.u1, -cashOut+1_000_000, r.at.Add(-time.Hour))
			}
		},
		"a person's gain": func(r *rangedRig) {
			r.twoCabals(0)
			for _, id := range []ids.CabalID{r.alpha, r.bet} {
				r.flow(id, r.u1, -cashOut, r.at.Add(-time.Hour))
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := newRangedRig(domain.Range1W)
			setup(r)
			r.input.ranged = []rangedBook{*r.book}
			if _, err := buildEntries(r.input); err == nil {
				t.Fatal("buildEntries error = nil")
			}
		})
	}
}

func (r *rangedRig) twoCabals(value uint64) {
	r.input.valued = nil
	r.input.stakes = nil
	for _, id := range []ids.CabalID{r.alpha, r.bet} {
		r.input.valued = append(r.input.valued, CabalValue{
			CabalID: id, Value: money.MicrosFromUint64(value), TotalShares: money.SharesUnitsFromUint64(100),
		})
		r.input.stakes = append(r.input.stakes, treasury.MemberStake{
			CabalID: id, UserID: r.u1, ShareUnits: money.SharesUnitsFromUint64(100),
			NetContributedMicros: money.SignedMicrosFromInt64(1_000_000),
		})
	}
}

func TestBuildEntries_refusesAPersonValueBeyondInt64(t *testing.T) {
	t.Parallel()
	r := newBoardRig()
	half := money.MicrosFromUint64(5_000_000_000_000_000_000)
	r.input.valued = []CabalValue{
		{CabalID: r.alpha, Value: half, TotalShares: money.SharesUnitsFromUint64(100)},
		{CabalID: r.bet, Value: half, TotalShares: money.SharesUnitsFromUint64(100)},
	}
	r.input.stakes = []treasury.MemberStake{
		{
			CabalID: r.alpha, UserID: r.u1, ShareUnits: money.SharesUnitsFromUint64(100),
			NetContributedMicros: money.SignedMicrosFromInt64(1_000_000_000_000_000_000),
		},
		{
			CabalID: r.bet, UserID: r.u1, ShareUnits: money.SharesUnitsFromUint64(100),
			NetContributedMicros: money.SignedMicrosFromInt64(1_000_000_000_000_000_000),
		},
	}
	if _, err := buildEntries(r.input); err == nil {
		t.Fatal("buildEntries error = nil, want a person worth more than int64 refused")
	}
}

func TestRangedBoard_DenseRanksOverRandomSeeds(t *testing.T) {
	t.Parallel()
	for seed := uint64(1); seed <= 40; seed++ {
		rnd := rand.New(rand.NewPCG(seed, 7))
		entries := randomRangedBoards(t, rnd)
		groups := map[string][]int{}
		for _, entry := range entries {
			key := entry.Board + "/" + entry.Range
			groups[key] = append(groups[key], entry.Rank)
		}
		for key, ranks := range groups {
			for i, rank := range ranks {
				if rank != i+1 {
					t.Fatalf("seed %d: %s ranks = %v, want dense from 1", seed, key, ranks)
				}
			}
		}
	}
}

func randomRangedBoards(t *testing.T, rnd *rand.Rand) []Entry {
	t.Helper()
	at := valuationTime()
	in := boardInput{
		at: at, views: map[ids.CabalID]cabalport.CabalView{}, members: map[ids.CabalID][]cabalport.MemberView{},
		users: map[ids.UserID]identity.UserCard{},
	}
	for _, rng := range rangedRanges() {
		t0, _ := rng.Start(at)
		in.ranged = append(in.ranged, rangedBook{
			Window: Window{Range: rng, T0: t0, T1: at}, stakes: map[MemberKey]money.SharesUnits{},
			shares: map[ids.CabalID]money.SharesUnits{}, users: map[ids.CabalID][]ids.UserID{},
			snaps: map[ids.CabalID]*Snapshot{}, flows: map[MemberKey][]domain.Flow{},
			cabalFlows: map[ids.CabalID][]domain.Flow{},
		})
	}
	people := make([]ids.UserID, 2+rnd.IntN(6))
	for i := range people {
		people[i] = ids.UserIDFrom(ids.Real{}.NewV7())
		in.users[people[i]] = identity.UserCard{ID: people[i], CreatedAt: at.Add(-time.Duration(i+1) * time.Hour)}
	}
	for range 1 + rnd.IntN(4) {
		id := ids.CabalIDFrom(ids.Real{}.NewV7())
		in.views[id] = cabal.View{ID: id, CreatedAt: at.Add(-time.Hour)}
		addRandomCabal(&in, rnd, id, people)
	}
	entries, err := buildEntries(in)
	if err != nil {
		t.Fatal(err)
	}
	return entries
}

func addRandomCabal(in *boardInput, rnd *rand.Rand, id ids.CabalID, people []ids.UserID) {
	var total uint64
	for _, user := range people {
		if rnd.IntN(3) == 0 {
			continue
		}
		held := 1 + rnd.Uint64N(50)
		total += held
		in.members[id] = append(in.members[id], cabalport.MemberView{UserID: user})
		in.stakes = append(in.stakes, treasury.MemberStake{
			CabalID: id, UserID: user, ShareUnits: money.SharesUnitsFromUint64(held),
			NetContributedMicros: money.SignedMicrosFromInt64(1_000_000 * (1 + rnd.Int64N(50))),
		})
		for i := range in.ranged {
			book := &in.ranged[i]
			if rnd.IntN(4) == 0 {
				continue
			}
			key := MemberKey{UserID: user, CabalID: id}
			book.stakes[key] = money.SharesUnitsFromUint64(held)
			book.shares[id] = money.SharesUnitsFromUint64(total)
			book.users[id] = append(book.users[id], user)
			flow := domain.Flow{
				Amount: money.SignedMicrosFromInt64(int64(rnd.IntN(5_000_000)) - 1_000_000),
				At:     book.T0.Add(time.Duration(1+rnd.IntN(59)) * time.Second),
			}
			book.flows[key] = append(book.flows[key], flow)
			book.cabalFlows[id] = append(book.cabalFlows[id], flow)
		}
	}
	in.valued = append(in.valued, CabalValue{
		CabalID: id, Value: money.MicrosFromUint64(total * (500_000 + rnd.Uint64N(2_000_000))),
		TotalShares: money.SharesUnitsFromUint64(total),
	})
	for i := range in.ranged {
		book := &in.ranged[i]
		book.snaps[id] = &Snapshot{
			At: book.T0.Add(-time.Minute), Value: money.MicrosFromUint64(book.shares[id].Uint64() * 1_000_000),
			TotalShares: book.shares[id],
		}
	}
}

func wantFlaggedMember(t *testing.T, members []Entry) {
	t.Helper()
	if len(members) != 1 || members[0].ReturnBps != nil || members[0].PnLMicros != 0 ||
		members[0].ValueMicros != 150_000_000 || !slices.Equal(members[0].Flags, []string{"stale_prices"}) {
		t.Fatalf("members 1D = %+v, want the member kept, unranked, flagged and at its current value", members)
	}
}

func TestRangedBoard_ANonSentinelErrorFailsTheRun(t *testing.T) {
	t.Parallel()
	boom := errs.New(errs.CodeInternal, "test")
	t.Run("member", func(t *testing.T) {
		t.Parallel()
		r := newRangedRig(domain.Range1W)
		r.alphaOnly(100_000_000, 100, 100_000_000)
		r.startedWith(r.alpha, r.u1, 100, 100, 100_000_000)
		r.input.member = func(
			Window, money.SharesUnits, money.SharesUnits, *Snapshot, money.Micros, []domain.Flow,
		) (Ranged, error) {
			return Ranged{}, boom
		}
		r.input.ranged = []rangedBook{*r.book}
		if _, err := buildEntries(r.input); !errors.Is(err, boom) {
			t.Fatalf("buildEntries error = %v, want the member's error to fail the run", err)
		}
	})
	t.Run("cabal", func(t *testing.T) {
		t.Parallel()
		r := newRangedRig(domain.Range1W)
		r.alphaOnly(100_000_000, 100, 100_000_000)
		r.book.T1 = r.book.T0
		r.input.ranged = []rangedBook{*r.book}
		if _, err := buildEntries(r.input); err == nil || errors.Is(err, domain.ErrUnusableStart) {
			t.Fatalf("buildEntries error = %v, want a failure that is not a skip", err)
		}
	})
}

func TestRangedBoard_ADepositBetweenSnapshotAndT0SkipsTheMember(t *testing.T) {
	t.Parallel()
	r := newRangedRig(domain.Range1D)
	r.alphaOnly(150_000_000, 150, 200_000_000)
	r.startedWith(r.alpha, r.u1, 100, 150, 100_000_000)
	r.book.snaps[r.alpha].TotalShares = money.SharesUnitsFromUint64(100)
	wantSkippedMember(t, r, true)
}

func TestRangedBoard_ASkippedCabalKeepsItsPreviousRow(t *testing.T) {
	t.Parallel()
	r := newRangedRig(domain.Range1D)
	r.alphaOnly(150_000_000, 100, 100_000_000)
	r.book.stakes[MemberKey{UserID: r.u1, CabalID: r.alpha}] = money.SharesUnitsFromUint64(100)
	r.book.shares[r.alpha] = money.SharesUnitsFromUint64(100)
	bps := int64(1200)
	row := func(rng string) Entry {
		return Entry{
			Board: cabalsBoard, Range: rng, Rank: 3, SubjectID: r.alpha.UUID(), SubjectName: "Alpha",
			ValueMicros: 140_000_000, PnLMicros: 10_000_000, ReturnBps: &bps,
		}
	}
	r.input.fallback = map[fallbackKey]Entry{
		{cabal: r.alpha.UUID(), rng: "1D"}: row("1D"), {cabal: r.alpha.UUID(), rng: "1W"}: row("1W"),
	}
	boards := r.build(t)
	day := boards["cabals/1D"]
	if len(day) != 1 || day[0].ValueMicros != 140_000_000 || day[0].Rank != 1 ||
		!slices.Equal(day[0].Flags, []string{"stale_prices"}) {
		t.Fatalf("cabals 1D = %+v, want the previous row carried, ranked and flagged", day)
	}
	if len(boards["cabals/1W"]) != 0 {
		t.Fatalf("cabals 1W = %+v, want no carry for a range the cabal was not skipped in", boards["cabals/1W"])
	}
	r.input.fallback[fallbackKey{cabal: r.alpha.UUID(), rng: "1D"}] = Entry{ValueMicros: -1}
	r.input.ranged = []rangedBook{*r.book}
	if _, err := buildEntries(r.input); err == nil {
		t.Fatal("buildEntries error = nil, want a negative carried value refused")
	}
}

func TestRangedPerson_APersonWithNoMemberResultsGetsNoRow(t *testing.T) {
	t.Parallel()
	r := newRangedRig(domain.Range1W)
	r.input.ranged = []rangedBook{*r.book}
	b := &boardBuilder{in: r.input}
	out := map[string][]domain.Candidate{}
	if err := b.rangedPerson(
		&person{sums: map[string]*rangedSum{}},
		domain.Candidate{},
		out,
	); err != nil ||
		len(out) != 0 {
		t.Fatalf("rangedPerson = %v, %+v, want no candidate", err, out)
	}
}
