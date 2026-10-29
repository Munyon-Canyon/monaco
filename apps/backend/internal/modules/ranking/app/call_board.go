package app

import (
	"context"
	"encoding/json"
	"slices"
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
	cardPage         = 500
	baselineKeepDays = 40
	openInputs       = 6

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

type lastCall struct {
	ProposalID uuid.UUID `json:"proposal_id"`
	Kind       string    `json:"kind"`
	Symbol     string    `json:"symbol"`
	Right      bool      `json:"right"`
}

type rankedInput struct {
	callInput
	entries []domain.CallEntry
	kind    string
	cards   map[uuid.UUID]callCard
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
	BaselineRank     *int      `json:"baseline_rank"`
	Movement         *int      `json:"movement"`
	MovementKind     string    `json:"movement_kind"`
	LastCall         *lastCall `json:"last_call"`
	RunID            uuid.UUID `json:"run_id"`
	ComputedAt       time.Time `json:"computed_at"`
}

type boardRun struct {
	id                    uuid.UUID
	startedAt, finishedAt time.Time
	day                   time.Time
}

func (r RunCallBoard) Run(ctx context.Context, startedAt time.Time) (int, error) {
	inputs, closing, err := r.read(ctx, startedAt)
	if err != nil {
		return 0, err
	}
	people, cabals, err := r.cards(ctx, slices.Concat(inputs, closing))
	if err != nil {
		return 0, err
	}
	utc := startedAt.UTC()
	run := boardRun{
		id: r.ids.NewV7(), startedAt: startedAt, finishedAt: r.clock.Now(),
		day: time.Date(utc.Year(), utc.Month(), utc.Day(), 0, 0, 0, 0, time.UTC),
	}
	open, shut := rankInputs(inputs, people, cabals), rankInputs(closing, people, cabals)
	var written int
	err = r.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		q := sqlc.New(tx.Queries())
		var err error
		if written, err = writeBoards(ctx, q, run, open); err != nil {
			return err
		}
		if err := closePeriods(ctx, q, shut, run.finishedAt); err != nil {
			return err
		}
		if err := q.PruneBaselines(ctx, run.day.AddDate(0, 0, -baselineKeepDays)); err != nil {
			return err
		}
		if err := q.InsertCallBoardRun(ctx, sqlc.InsertCallBoardRunParams{
			RunID: run.id, StartedAt: run.startedAt, FinishedAt: run.finishedAt,
		}); err != nil {
			return err
		}
		if err := q.DeleteCallBoardTriggersThrough(ctx, run.startedAt); err != nil {
			return err
		}
		return tx.Events.Append(ctx, events.RankingCallBoardWritten{V: 1, RunID: run.id, ComputedAt: run.finishedAt})
	})
	return written, err
}

func writeBoards(ctx context.Context, q *sqlc.Queries, run boardRun, open []rankedInput) (int, error) {
	calls, err := lastCalls(ctx, q, open, run.day)
	if err != nil {
		return 0, err
	}
	ran, err := q.CallBoardRanSince(ctx, run.day)
	if err != nil {
		return 0, err
	}
	written := 0
	for _, in := range open {
		baseline, err := baselinesOf(ctx, q, in, run.day, !ran)
		if err != nil {
			return 0, err
		}
		if err := recordLeadChange(ctx, q, in, run); err != nil {
			return 0, err
		}
		rows := boardRows(in, domain.Movement(baseline, in.entries), calls, run.id, run.finishedAt)
		if err := replaceBoard(ctx, q, in.callInput, rows); err != nil {
			return 0, err
		}
		written += len(rows)
	}
	return written, nil
}

func (r RunCallBoard) read(ctx context.Context, at time.Time) (open, closing []callInput, err error) {
	pending, err := sqlc.New(r.db).ClosedPeriodsPending(ctx, at.Add(-unpricedAfter))
	if err != nil {
		return nil, nil, err
	}
	periods := make([]domain.Period, 0, 3+len(pending))
	for _, kind := range []domain.PeriodKind{domain.PeriodWeek, domain.PeriodMonth, domain.PeriodAll} {
		periods = append(periods, domain.PeriodContaining(kind, at))
	}
	for _, p := range pending {
		periods = append(periods, domain.PeriodContaining(domain.PeriodKind(p.Period), p.PeriodStart.Time))
	}
	var all []callInput
	for _, period := range periods {
		in, err := r.readPeriod(ctx, period)
		if err != nil {
			return nil, nil, err
		}
		all = append(all, in...)
	}
	return all[:openInputs], all[openInputs:], nil
}

func (r RunCallBoard) readPeriod(ctx context.Context, period domain.Period) ([]callInput, error) {
	q := sqlc.New(r.db)
	window := sqlc.PeopleCallInputsParams{WindowStart: period.Start, WindowEnd: period.End}
	people, err := q.PeopleCallInputs(ctx, window)
	if err != nil {
		return nil, err
	}
	counts := make([]callCount, len(people))
	for i, p := range people {
		counts[i] = callCount{p.SubjectID, int(p.RightCount), int(p.Resolved), int(p.Voted), int(p.Eligible)}
	}
	out := make([]callInput, 0, 2)
	out = append(out, callInput{board: domain.BoardPeople, period: period, rows: counts})
	cabals, err := q.CabalCallInputs(ctx, sqlc.CabalCallInputsParams(window))
	if err != nil {
		return nil, err
	}
	counts = make([]callCount, len(cabals))
	for i, c := range cabals {
		counts[i] = callCount{c.SubjectID, int(c.RightCount), int(c.Resolved), int(c.Voted), int(c.Eligible)}
	}
	return append(out, callInput{board: domain.BoardCabals, period: period, rows: counts}), nil
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

func rankInputs(inputs []callInput, people, cabals map[uuid.UUID]callCard) []rankedInput {
	out := make([]rankedInput, len(inputs))
	for i, in := range inputs {
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
		out[i] = rankedInput{callInput: in, entries: domain.RankCalls(subjects), kind: kind, cards: cards}
	}
	return out
}

func boardRows(
	in rankedInput, entries []domain.CallEntry, calls map[uuid.UUID]*lastCall, runID uuid.UUID, computedAt time.Time,
) []callBoardRow {
	out := make([]callBoardRow, len(entries))
	for i, e := range entries {
		card := in.cards[e.ID]
		out[i] = callBoardRow{
			Board: string(in.board), Period: string(in.period.Kind), PeriodStart: in.period.Start.Format(time.DateOnly),
			SubjectID: e.ID, SubjectKind: in.kind, SubjectName: card.name, SubjectHandle: textOrNil(card.handle),
			SubjectPicture: textOrNil(card.picture), SubjectCreatedAt: card.createdAt, Rank: e.Rank,
			RightCount: e.Right, Resolved: e.Resolved, Voted: e.Voted, Eligible: e.Eligible, RateBps: e.RateBps,
			TurnoutBps: e.TurnoutBps, BaselineRank: e.BaselineRank, Movement: e.Movement,
			MovementKind: string(e.MovementKind), LastCall: calls[e.ID], RunID: runID, ComputedAt: computedAt,
		}
	}
	return out
}

func baselinesOf(
	ctx context.Context, q *sqlc.Queries, in rankedInput, day time.Time, firstOfDay bool,
) (map[uuid.UUID]int, error) {
	if firstOfDay {
		if err := writeBaselines(ctx, q, in, day); err != nil {
			return nil, err
		}
	}
	rows, err := q.BaselinesFor(ctx, sqlc.BaselinesForParams{
		Board: string(in.board), Period: string(in.period.Kind), Day: day,
	})
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID]int, len(rows))
	for _, row := range rows {
		out[row.SubjectID] = int(row.Rank)
	}
	return out, nil
}

func writeBaselines(ctx context.Context, q *sqlc.Queries, in rankedInput, day time.Time) error {
	var ranked []baselineRow
	for _, e := range in.entries {
		if e.Rank != nil {
			ranked = append(ranked, baselineRow{SubjectID: e.ID, Rank: *e.Rank})
		}
	}
	if len(ranked) == 0 {
		return nil
	}
	encoded, _ := json.Marshal(ranked)
	return q.UpsertBaselines(ctx, sqlc.UpsertBaselinesParams{
		Board: string(in.board), Period: string(in.period.Kind), PeriodStart: in.period.Start, Day: day, Rows: encoded,
	})
}

type baselineRow struct {
	SubjectID uuid.UUID `json:"subject_id"`
	Rank      int       `json:"rank"`
}

func lastCalls(
	ctx context.Context, q *sqlc.Queries, inputs []rankedInput, since time.Time,
) (map[uuid.UUID]*lastCall, error) {
	seen := map[uuid.UUID]bool{}
	var subjects []uuid.UUID
	for _, in := range inputs {
		for _, e := range in.entries {
			if e.Resolved > 0 && !seen[e.ID] {
				seen[e.ID] = true
				subjects = append(subjects, e.ID)
			}
		}
	}
	out := make(map[uuid.UUID]*lastCall, len(subjects))
	if len(subjects) == 0 {
		return out, nil
	}
	rows, err := q.LastCallsSince(ctx, sqlc.LastCallsSinceParams{SubjectIds: subjects, Since: since})
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.SubjectID] = &lastCall{
			ProposalID: row.ProposalID, Kind: row.Kind.String, Symbol: row.Symbol.String, Right: row.RightCall.Bool,
		}
	}
	return out, nil
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
