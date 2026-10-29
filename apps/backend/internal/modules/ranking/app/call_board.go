package app

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	dbsqlc "github.com/monaco/monaco/apps/backend/internal/platform/db/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const (
	cardPage = 500

	accountBanned  = "banned"
	accountDeleted = "deleted"

	subjectPerson = "person"
	subjectCabal  = "cabal"
)

type RunCallBoard struct {
	db     dbsqlc.DBTX
	uow    *db.UnitOfWork
	ids    ids.Generator
	clock  clock.Clock
	users  Users
	cabals Cabals
}

func NewRunCallBoard(
	pool dbsqlc.DBTX, uow *db.UnitOfWork, gen ids.Generator, clk clock.Clock, users Users, cabals Cabals,
) RunCallBoard {
	return RunCallBoard{db: pool, uow: uow, ids: gen, clock: clk, users: users, cabals: cabals}
}

type callInput struct {
	board  domain.Board
	period domain.Period
	rows   []callCount
}

type callCount struct {
	id                               uuid.UUID
	right, resolved, voted, eligible int
}

type callCard struct {
	name, handle, picture string
	createdAt             time.Time
}

type callBoardRow struct {
	Board            string    `json:"board"`
	Period           string    `json:"period"`
	PeriodStart      string    `json:"period_start"`
	SubjectID        uuid.UUID `json:"subject_id"`
	SubjectKind      string    `json:"subject_kind"`
	SubjectName      string    `json:"subject_name"`
	SubjectHandle    *string   `json:"subject_handle"`
	SubjectPicture   *string   `json:"subject_picture_url"`
	SubjectCreatedAt time.Time `json:"subject_created_at"`
	Rank             *int      `json:"rank"`
	RightCount       int       `json:"right_count"`
	Resolved         int       `json:"resolved"`
	Voted            int       `json:"voted"`
	Eligible         int       `json:"eligible"`
	RateBps          *int      `json:"rate_bps"`
	TurnoutBps       int       `json:"turnout_bps"`
	RunID            uuid.UUID `json:"run_id"`
	ComputedAt       time.Time `json:"computed_at"`
}

func (r RunCallBoard) Run(ctx context.Context, startedAt time.Time) (int, error) {
	inputs, err := r.read(ctx, startedAt)
	if err != nil {
		return 0, err
	}
	people, cabals, err := r.cards(ctx, inputs)
	if err != nil {
		return 0, err
	}
	runID, finishedAt := r.ids.NewV7(), r.clock.Now()
	var written int
	err = r.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		q := sqlc.New(tx.Queries())
		written = 0
		for _, in := range inputs {
			rows := rankedRows(in, people, cabals, runID, finishedAt)
			if err := replaceBoard(ctx, q, in, rows); err != nil {
				return err
			}
			written += len(rows)
		}
		if err := q.InsertCallBoardRun(ctx, sqlc.InsertCallBoardRunParams{
			RunID: runID, StartedAt: startedAt, FinishedAt: finishedAt,
		}); err != nil {
			return err
		}
		if err := q.DeleteCallBoardTriggersThrough(ctx, startedAt); err != nil {
			return err
		}
		return tx.Events.Append(ctx, events.RankingCallBoardWritten{V: 1, RunID: runID, ComputedAt: finishedAt})
	})
	return written, err
}

func (r RunCallBoard) read(ctx context.Context, at time.Time) ([]callInput, error) {
	q := sqlc.New(r.db)
	periods := []domain.PeriodKind{domain.PeriodWeek, domain.PeriodMonth, domain.PeriodAll}
	out := make([]callInput, 0, 2*len(periods))
	for _, kind := range periods {
		period := domain.PeriodContaining(kind, at)
		window := sqlc.PeopleCallInputsParams{WindowStart: period.Start, WindowEnd: period.End}
		people, err := q.PeopleCallInputs(ctx, window)
		if err != nil {
			return nil, err
		}
		counts := make([]callCount, len(people))
		for i, p := range people {
			counts[i] = callCount{p.SubjectID, int(p.RightCount), int(p.Resolved), int(p.Voted), int(p.Eligible)}
		}
		out = append(out, callInput{board: domain.BoardPeople, period: period, rows: counts})
		cabals, err := q.CabalCallInputs(ctx, sqlc.CabalCallInputsParams(window))
		if err != nil {
			return nil, err
		}
		counts = make([]callCount, len(cabals))
		for i, c := range cabals {
			counts[i] = callCount{c.SubjectID, int(c.RightCount), int(c.Resolved), int(c.Voted), int(c.Eligible)}
		}
		out = append(out, callInput{board: domain.BoardCabals, period: period, rows: counts})
	}
	return out, nil
}

func (r RunCallBoard) cards(
	ctx context.Context, inputs []callInput,
) (people, cabals map[uuid.UUID]callCard, err error) {
	if people, err = r.peopleCards(ctx, inputs); err != nil {
		return nil, nil, err
	}
	cabals, err = r.cabalCards(ctx)
	return people, cabals, err
}

func (r RunCallBoard) peopleCards(ctx context.Context, inputs []callInput) (map[uuid.UUID]callCard, error) {
	var wanted []ids.UserID
	seen := map[uuid.UUID]bool{}
	for _, in := range inputs {
		for _, row := range in.rows {
			if in.board == domain.BoardPeople && !seen[row.id] {
				seen[row.id] = true
				wanted = append(wanted, ids.UserIDFrom(row.id))
			}
		}
	}
	people := make(map[uuid.UUID]callCard, len(wanted))
	for len(wanted) > 0 {
		page, err := r.users.UsersByID(ctx, wanted[:min(len(wanted), cardPage)])
		if err != nil {
			return nil, err
		}
		for id, u := range page {
			if listsOnCallBoard(u) {
				people[id.UUID()] = callCard{
					name: domain.SubjectName(u.DisplayName, u.Handle), handle: u.Handle, picture: u.PhotoURL,
					createdAt: u.CreatedAt,
				}
			}
		}
		wanted = wanted[min(len(wanted), cardPage):]
	}
	return people, nil
}

func (r RunCallBoard) cabalCards(ctx context.Context) (map[uuid.UUID]callCard, error) {
	all, err := r.cabals.AllCabals(ctx)
	if err != nil {
		return nil, err
	}
	cabals := make(map[uuid.UUID]callCard, len(all))
	for _, c := range all {
		if c.Status != cabalport.StatusBanned {
			cabals[c.ID.UUID()] = callCard{name: c.Name, picture: c.PictureURL, createdAt: c.CreatedAt}
		}
	}
	return cabals, nil
}

func listsOnCallBoard(u identity.UserCard) bool {
	status := string(u.AccountStatus)
	return !u.Deleted && status != accountBanned && status != accountDeleted
}

func rankedRows(
	in callInput, people, cabals map[uuid.UUID]callCard, runID uuid.UUID, computedAt time.Time,
) []callBoardRow {
	cards, kind := people, subjectPerson
	if in.board == domain.BoardCabals {
		cards, kind = cabals, subjectCabal
	}
	subjects := make([]domain.CallSubject, 0, len(in.rows))
	for _, row := range in.rows {
		card, ok := cards[row.id]
		if !ok {
			continue
		}
		subjects = append(subjects, domain.CallSubject{
			ID: row.id, Right: row.right, Resolved: row.resolved, Voted: row.voted, Eligible: row.eligible,
			CreatedAt: card.createdAt,
		})
	}
	entries := domain.RankCalls(subjects)
	out := make([]callBoardRow, len(entries))
	for i, e := range entries {
		card := cards[e.ID]
		out[i] = callBoardRow{
			Board: string(in.board), Period: string(in.period.Kind), PeriodStart: in.period.Start.Format(time.DateOnly),
			SubjectID: e.ID, SubjectKind: kind, SubjectName: card.name, SubjectHandle: textOrNil(card.handle),
			SubjectPicture: textOrNil(card.picture), SubjectCreatedAt: card.createdAt, Rank: e.Rank,
			RightCount: e.Right, Resolved: e.Resolved, Voted: e.Voted, Eligible: e.Eligible, RateBps: e.RateBps,
			TurnoutBps: e.TurnoutBps, RunID: runID, ComputedAt: computedAt,
		}
	}
	return out
}

func textOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func replaceBoard(ctx context.Context, q *sqlc.Queries, in callInput, rows []callBoardRow) error {
	if err := q.DeleteCallBoardEntries(ctx, sqlc.DeleteCallBoardEntriesParams{
		Board: string(in.board), Period: string(in.period.Kind),
	}); err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	encoded, _ := json.Marshal(rows)
	return q.InsertCallBoardEntries(ctx, encoded)
}
