package app_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func weekStart() time.Time { return time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC) }

func boardUser(name string) ids.UserID {
	return ids.UserIDFrom(uuid.NewSHA1(uuid.Nil, []byte("board/user/"+name)))
}

func seedStandings(t *testing.T) app.CallBoards {
	t.Helper()
	pool, cards := newRig(t), &boardCards{}
	cabal := cards.addCabal("Pilates", cabalport.StatusActive)
	seedPeople(t, pool, cards, cabal, weekStart().Add(9*time.Hour), []personSeed{
		{name: "Ada", right: 5, resolved: 5, openVotes: 2, openOf: 2},
		{name: "Kai", right: 4, resolved: 5, openVotes: 1, openOf: 2},
		{name: "Andres", right: 3, resolved: 3, openOf: 2},
	})
	if _, err := newBoardRun(t, pool, cards).Run(pollerContext(t), boardNow()); err != nil {
		t.Fatalf("Run = %v", err)
	}
	return app.NewCallBoards(pool)
}

func TestCallBoards_standingCarriesRankRecordAndPeriodStart(t *testing.T) {
	t.Parallel()
	kai, err := seedStandings(t).CallStanding(t.Context(), boardUser("Kai"), domain.PeriodWeek)
	if err != nil {
		t.Fatalf("CallStanding(Kai) = %v", err)
	}
	if kai.Rank == nil || *kai.Rank != 2 || kai.Of != 2 || kai.RateBps == nil || *kai.RateBps != 8000 {
		t.Errorf("Kai standing = %+v, want #2 of 2 at 8000 bps", kai)
	}
	if kai.Right != 4 || kai.Resolved != 5 || !kai.PeriodStart.Equal(weekStart()) {
		t.Errorf("Kai standing = %+v, want 4 of 5 in the week of %s", kai, weekStart())
	}
}

func TestCallBoards_standingOfAnUnrankedMemberHasNoRank(t *testing.T) {
	t.Parallel()
	andres, err := seedStandings(t).CallStanding(t.Context(), boardUser("Andres"), domain.PeriodWeek)
	if err != nil || andres.Rank != nil || andres.RateBps != nil || andres.Resolved != 3 || andres.Of != 2 {
		t.Errorf("Andres standing = %+v, %v; want unranked with 3 resolved of 2 ranked", andres, err)
	}
}

func TestCallBoards_standingOfAStrangerIsEmpty(t *testing.T) {
	t.Parallel()
	stranger, err := seedStandings(t).CallStanding(t.Context(), boardUser("Nobody"), domain.PeriodWeek)
	if err != nil || stranger.Rank != nil || !stranger.PeriodStart.IsZero() {
		t.Errorf("stranger standing = %+v, %v; want the zero standing", stranger, err)
	}
}

func TestCallBoards_leaderOfThePeopleBoardIsRankOne(t *testing.T) {
	t.Parallel()
	leader, err := seedStandings(t).CallLeader(t.Context(), domain.BoardPeople, domain.PeriodWeek)
	if err != nil || leader.Name != "Ada" || leader.RateBps != 10000 || leader.Handle != "Ada" {
		t.Errorf("people leader = %+v, %v; want Ada at 10000 bps", leader, err)
	}
	if leader.Right != 5 || leader.Resolved != 5 || leader.SubjectID != boardUser("Ada").UUID() {
		t.Errorf("people leader = %+v, want 5 of 5 for Ada", leader)
	}
	if !leader.PeriodStart.Equal(weekStart()) {
		t.Errorf("leader period start = %s, want %s", leader.PeriodStart, weekStart())
	}
}

func TestCallBoards_leaderOfTheCabalBoardHasNoHandle(t *testing.T) {
	t.Parallel()
	pilates, err := seedStandings(t).CallLeader(t.Context(), domain.BoardCabals, domain.PeriodMonth)
	if err != nil || pilates.Name != "Pilates" || pilates.Handle != "" || pilates.RateBps != 10000 {
		t.Errorf("cabal leader = %+v, %v; want Pilates at 10000 bps with no handle", pilates, err)
	}
}

func TestCallBoards_leaderIsNotFoundOnAnEmptyBoard(t *testing.T) {
	t.Parallel()
	_, err := app.NewCallBoards(newRig(t)).CallLeader(t.Context(), domain.BoardPeople, domain.PeriodAll)
	if errs.CodeOf(err) != errs.CodeNotFound {
		t.Fatalf("CallLeader on an empty board = %v, want not_found", err)
	}
}

func TestCallBoards_passTheReadErrorOn(t *testing.T) {
	t.Parallel()
	pool := newRig(t)
	execAll(t, pool, "ALTER TABLE call_board_entries RENAME TO call_board_entries_gone")
	boards := app.NewCallBoards(pool)
	if _, err := boards.CallStanding(t.Context(), boardUser("Kai"), domain.PeriodWeek); err == nil {
		t.Error("CallStanding error = nil, want the read to fail")
	}
	if _, err := boards.CallLeader(
		t.Context(),
		domain.BoardPeople,
		domain.PeriodWeek,
	); errs.CodeOf(
		err,
	) == errs.CodeNotFound ||
		err == nil {
		t.Errorf("CallLeader = %v, want a read failure that is not not_found", err)
	}
}
