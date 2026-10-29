package app

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const (
	callBoardPodium      = 3
	callBoardAbove       = 2
	callBoardBelow       = 4
	callBoardUnrankedMax = 10
	callBoardCabalsEmpty = 7
	callBoardCabalSpan   = 10
	callBoardPage        = 20
	callBoardUnrankedCap = 20
	climbFullBps         = 10_000
	leadChangeTTL        = 24 * time.Hour
)

type CallBoardRuns interface {
	CallBoardLastRun(ctx context.Context) (sqlc.CallBoardRun, error)
	CallBoardTop(ctx context.Context, arg sqlc.CallBoardTopParams) ([]sqlc.CallBoardEntry, error)
	CallBoardCount(ctx context.Context, arg sqlc.CallBoardCountParams) (int32, error)
}

type CallBoardRanks interface {
	CallBoardRow(ctx context.Context, arg sqlc.CallBoardRowParams) (sqlc.CallBoardEntry, error)
	CallBoardAt(ctx context.Context, arg sqlc.CallBoardAtParams) (sqlc.CallBoardEntry, error)
	CallBoardWindow(ctx context.Context, arg sqlc.CallBoardWindowParams) ([]sqlc.CallBoardEntry, error)
}

type CallBoardSubjects interface {
	CallBoardRanked(ctx context.Context, arg sqlc.CallBoardRankedParams) ([]sqlc.CallBoardEntry, error)
	CallBoardOf(ctx context.Context, arg sqlc.CallBoardOfParams) ([]sqlc.CallBoardEntry, error)
	CallBoardUnranked(ctx context.Context, arg sqlc.CallBoardUnrankedParams) ([]sqlc.CallBoardEntry, error)
}

type CallLeadChanges interface {
	LatestLeadChange(ctx context.Context, arg sqlc.LatestLeadChangeParams) (sqlc.LatestLeadChangeRow, error)
}

type PendingCallSymbols interface {
	PendingSymbols(
		ctx context.Context, users []ids.UserID, cabal *ids.CabalID, p domain.Period,
	) (map[ids.UserID][]string, error)
}

type CallBoardHeadKey struct {
	RunID  uuid.UUID
	Board  domain.Board
	Period domain.PeriodKind
}

type CallBoardHead struct {
	Top         []sqlc.CallBoardEntry
	Total       int
	PeriodStart time.Time
}

type CallBoardHeads interface {
	Load(context.Context, CallBoardHeadKey, func(context.Context) (CallBoardHead, error)) (CallBoardHead, error)
}

type CallMovement struct {
	Kind   string
	Places int
}

type CallLast struct {
	ProposalID uuid.UUID
	Kind       string
	Symbol     string
	Right      bool
}

type CallSubjectView struct {
	ID         uuid.UUID
	Cabal      bool
	Name       string
	Handle     *string
	PictureURL *string
}

type CallRow struct {
	Rank       *int
	Subject    CallSubjectView
	RateBps    *int
	RatePct    *int
	Right      int
	Resolved   int
	Voted      int
	Eligible   int
	TurnoutBps int
	Needed     *int
	Movement   CallMovement
	LastCall   *CallLast
	IsViewer   bool
	IsMine     bool
}

type CallGapView struct {
	Versus         domain.GapVersus
	Points         int
	CallsToPass    *int
	AboveIsPerfect bool
}

type CallUnrankedMe struct {
	Subject        CallSubjectView
	Resolved       int
	Needed         int
	Voted          int
	Eligible       int
	PendingSymbols []string
}

type CallBoardView struct {
	Board       domain.Board
	Friends     bool
	Period      domain.PeriodKind
	PeriodStart *time.Time
	PeriodEnd   *time.Time
	ComputedAt  time.Time
	TotalRanked int
	Top         []CallRow
	Me          *CallRow
	Above       *CallRow
	Below       *CallRow
	Gap         *CallGapView
	ClimbBps    int
	UnrankedMe  *CallUnrankedMe
	Around      []CallRow
	AfterCursor *string
	Unranked    []CallRow
	LeadChange  *CallLeadChange
}

type CallBoardQuery struct {
	Viewer  ids.UserID
	Board   domain.Board
	Period  domain.PeriodKind
	Friends bool
	Cursor  string
}

type ReadCallBoard struct {
	Runs     CallBoardRuns
	Ranks    CallBoardRanks
	Subjects CallBoardSubjects
	Pending  PendingCallSymbols
	Users    Users
	Follows  Follows
	Members  MemberCabals
	Heads    CallBoardHeads
	Leads    CallLeadChanges
	Clock    clock.Clock
}

func (r ReadCallBoard) Run(ctx context.Context, q CallBoardQuery) (CallBoardView, error) {
	var cursor callCursor
	var err error
	if q.Cursor != "" {
		if cursor, err = parseCallCursor(q.Cursor); err != nil {
			return CallBoardView{}, err
		}
	}
	run, err := r.Runs.CallBoardLastRun(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return r.empty(q), nil
	}
	if err != nil {
		return CallBoardView{}, err
	}
	if q.Cursor != "" && cursor.run != run.RunID {
		return CallBoardView{}, errs.New(errs.CodeCursorStale, "ranking.ReadCallBoard.Run")
	}
	read := callBoardRead{ReadCallBoard: r, q: q, run: run}
	if err := read.load(ctx); err != nil {
		return CallBoardView{}, err
	}
	if q.Cursor != "" {
		return read.page(ctx, cursor.after)
	}
	return read.screen(ctx)
}

func (r ReadCallBoard) empty(q CallBoardQuery) CallBoardView {
	now := r.Clock.Now()
	view := CallBoardView{Board: q.Board, Friends: q.Friends, Period: q.Period, ComputedAt: now}
	view.PeriodStart, view.PeriodEnd = periodDates(q.Period, now, time.Time{})
	view.Top, view.Around, view.Unranked = []CallRow{}, []CallRow{}, []CallRow{}
	return view
}

type callBoardRead struct {
	ReadCallBoard
	q        CallBoardQuery
	run      sqlc.CallBoardRun
	head     CallBoardHead
	mine     map[uuid.UUID]bool
	mineRows []sqlc.CallBoardEntry
	ranked   []sqlc.CallBoardEntry
	self     *sqlc.CallBoardEntry
	subjects []uuid.UUID
}

func (c *callBoardRead) load(ctx context.Context) error {
	switch {
	case c.q.Board == domain.BoardCabals:
		return c.loadCabals(ctx)
	case c.q.Friends:
		return c.loadFriends(ctx)
	}
	return c.loadPeople(ctx)
}

func (c *callBoardRead) loadHead(ctx context.Context) error {
	key := CallBoardHeadKey{RunID: c.run.RunID, Board: c.q.Board, Period: c.q.Period}
	load := func(ctx context.Context) (CallBoardHead, error) {
		top, err := c.Runs.CallBoardTop(
			ctx,
			sqlc.CallBoardTopParams{Board: string(c.q.Board), Period: string(c.q.Period)},
		)
		if err != nil {
			return CallBoardHead{}, err
		}
		total, err := c.Runs.CallBoardCount(ctx, sqlc.CallBoardCountParams{
			Board: string(c.q.Board), Period: string(c.q.Period),
		})
		if err != nil {
			return CallBoardHead{}, err
		}
		head := CallBoardHead{Top: top, Total: int(total)}
		if len(top) > 0 {
			head.PeriodStart = top[0].PeriodStart.Time
		}
		return head, nil
	}
	var err error
	if c.Heads == nil {
		c.head, err = load(ctx)
	} else {
		c.head, err = c.Heads.Load(ctx, key, load)
	}
	return err
}

func (c *callBoardRead) loadPeople(ctx context.Context) error {
	if err := c.loadHead(ctx); err != nil {
		return err
	}
	row, err := c.Ranks.CallBoardRow(ctx, sqlc.CallBoardRowParams{
		Board: string(c.q.Board), Period: string(c.q.Period), Subject: c.q.Viewer.UUID(),
	})
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	c.self = &row
	return nil
}

func (c *callBoardRead) loadFriends(ctx context.Context) error {
	following, err := c.Follows.FollowingIDs(ctx, c.q.Viewer)
	if err != nil {
		return err
	}
	c.subjects = make([]uuid.UUID, 0, len(following)+1)
	for _, id := range following {
		c.subjects = append(c.subjects, id.UUID())
	}
	if !slices.Contains(c.subjects, c.q.Viewer.UUID()) {
		c.subjects = append(c.subjects, c.q.Viewer.UUID())
	}
	if c.ranked, err = c.Subjects.CallBoardRanked(ctx, sqlc.CallBoardRankedParams{
		Board: string(c.q.Board), Period: string(c.q.Period), Subjects: c.subjects,
	}); err != nil {
		return err
	}
	c.head = CallBoardHead{Total: len(c.ranked)}
	for i := range min(callBoardPodium, len(c.ranked)) {
		c.head.Top = append(c.head.Top, c.ranked[i])
	}
	if len(c.ranked) > 0 {
		c.head.PeriodStart = c.ranked[0].PeriodStart.Time
	}
	for i := range c.ranked {
		if c.ranked[i].SubjectID == c.q.Viewer.UUID() {
			c.self = &c.ranked[i]
		}
	}
	if c.self != nil {
		return nil
	}
	row, err := c.Ranks.CallBoardRow(ctx, sqlc.CallBoardRowParams{
		Board: string(c.q.Board), Period: string(c.q.Period), Subject: c.q.Viewer.UUID(),
	})
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	c.self = &row
	return nil
}

func (c *callBoardRead) loadCabals(ctx context.Context) error {
	if err := c.loadHead(ctx); err != nil {
		return err
	}
	cabals, err := c.Members.CabalsOf(ctx, c.q.Viewer)
	if err != nil {
		return err
	}
	c.subjects = make([]uuid.UUID, len(cabals))
	c.mine = make(map[uuid.UUID]bool, len(cabals))
	for i, id := range cabals {
		c.subjects[i] = id.UUID()
		c.mine[id.UUID()] = true
	}
	if len(c.subjects) == 0 {
		return nil
	}
	if c.mineRows, err = c.Subjects.CallBoardOf(ctx, sqlc.CallBoardOfParams{
		Board: string(c.q.Board), Period: string(c.q.Period), Subjects: c.subjects,
	}); err != nil {
		return err
	}
	if len(c.mineRows) > 0 {
		c.self = &c.mineRows[0]
	}
	return nil
}

func (c *callBoardRead) rankOf(e sqlc.CallBoardEntry) *int {
	if c.q.Friends {
		for i := range c.ranked {
			if c.ranked[i].SubjectID == e.SubjectID {
				rank := i + 1
				return &rank
			}
		}
		return nil
	}
	if !e.Rank.Valid {
		return nil
	}
	rank := int(e.Rank.Int32)
	return &rank
}

func (c *callBoardRead) row(e sqlc.CallBoardEntry) CallRow {
	row := CallRow{
		Rank: c.rankOf(e),
		Subject: CallSubjectView{
			ID: e.SubjectID, Cabal: e.SubjectKind == subjectCabal, Name: e.SubjectName,
			Handle: textOrNil(e.SubjectHandle.String), PictureURL: textOrNil(e.SubjectPictureUrl.String),
		},
		Right: int(e.RightCount), Resolved: int(e.Resolved), Voted: int(e.Voted), Eligible: int(e.Eligible),
		TurnoutBps: int(e.TurnoutBps), Movement: movementOf(e), LastCall: lastCallOf(e.LastCall),
	}
	if e.RateBps.Valid && row.Rank != nil {
		bps, pct := int(e.RateBps.Int32), domain.RatePct(int(e.RateBps.Int32))
		row.RateBps, row.RatePct = &bps, &pct
	}
	if row.Rank == nil {
		needed := max(domain.MinResolvedCalls-row.Resolved, 0)
		row.Needed = &needed
	}
	row.IsViewer = c.q.Board == domain.BoardPeople && e.SubjectID == c.q.Viewer.UUID()
	row.IsMine = c.mine[e.SubjectID]
	return row
}

func (c *callBoardRead) rows(entries []sqlc.CallBoardEntry) []CallRow {
	out := make([]CallRow, len(entries))
	for i, e := range entries {
		out[i] = c.row(e)
	}
	return out
}

func (c *callBoardRead) view(computedAt time.Time) CallBoardView {
	view := CallBoardView{
		Board: c.q.Board, Friends: c.q.Friends, Period: c.q.Period, ComputedAt: computedAt,
		TotalRanked: c.head.Total, Top: []CallRow{}, Around: []CallRow{}, Unranked: []CallRow{},
	}
	view.PeriodStart, view.PeriodEnd = periodDates(c.q.Period, c.Clock.Now(), c.head.PeriodStart)
	return view
}

func (c *callBoardRead) window(ctx context.Context, from, to int) ([]sqlc.CallBoardEntry, error) {
	to = min(to, c.head.Total)
	if from > to {
		return nil, nil
	}
	if c.q.Friends {
		return slices.Clone(c.ranked[from-1 : to]), nil
	}
	return c.Ranks.CallBoardWindow(ctx, sqlc.CallBoardWindowParams{
		Board: string(c.q.Board), Period: string(c.q.Period), FromRank: rankArg(from), ToRank: rankArg(to),
	})
}

func (c *callBoardRead) page(ctx context.Context, after int) (CallBoardView, error) {
	view := c.view(c.run.FinishedAt)
	entries, err := c.window(ctx, after+1, after+callBoardPage)
	if err != nil {
		return CallBoardView{}, err
	}
	view.Around = c.rows(entries)
	view.AfterCursor = c.afterCursor(entries)
	return view, nil
}

func (c *callBoardRead) afterCursor(entries []sqlc.CallBoardEntry) *string {
	if len(entries) == 0 {
		return nil
	}
	last := c.rankOf(entries[len(entries)-1])
	if last == nil || *last >= c.head.Total {
		return nil
	}
	next := encodeCallCursor(c.run.RunID, *last)
	return &next
}

func (c *callBoardRead) screen(ctx context.Context) (CallBoardView, error) {
	view := c.view(c.run.FinishedAt)
	view.Top = c.rows(c.head.Top)
	if err := c.leadChange(ctx, &view); err != nil {
		return view, err
	}
	if c.q.Board == domain.BoardCabals {
		return view, c.cabalsScreen(ctx, &view)
	}
	return view, c.peopleScreen(ctx, &view)
}

func (c *callBoardRead) peopleScreen(ctx context.Context, view *CallBoardView) error {
	if c.self == nil || c.rankOf(*c.self) == nil {
		return c.unrankedScreen(ctx, view)
	}
	me := *c.rankOf(*c.self)
	entries, err := c.window(ctx, max(callBoardPodium+1, me-callBoardAbove), me+callBoardBelow)
	if err != nil {
		return err
	}
	return c.neighbours(ctx, view, me, entries)
}

func (c *callBoardRead) neighbours(
	ctx context.Context, view *CallBoardView, me int, entries []sqlc.CallBoardEntry,
) error {
	known := make(map[int]sqlc.CallBoardEntry, len(entries)+callBoardPodium)
	for _, e := range slices.Concat(c.head.Top, entries) {
		if rank := c.rankOf(e); rank != nil {
			known[*rank] = e
		}
	}
	above, aboveOK := known[me-1]
	below, belowOK := known[me+1]
	if !aboveOK && me > 1 {
		var err error
		if above, err = c.Ranks.CallBoardAt(ctx, sqlc.CallBoardAtParams{
			Board: string(c.q.Board), Period: string(c.q.Period), Rank: rankArg(me - 1),
		}); err != nil {
			return err
		}
		aboveOK = true
	}
	view.Around, view.AfterCursor = c.rows(entries), c.afterCursor(entries)
	mine := c.row(*c.self)
	view.Me = &mine
	view.ClimbBps = *mine.RateBps
	view.Above, view.Below = c.optRow(above, aboveOK), c.optRow(below, belowOK)
	gap := domain.ComputeGap(callEntry(*c.self), optEntry(above, aboveOK), optEntry(below, belowOK))
	if gap.Versus != domain.VersusNone {
		view.Gap = &CallGapView{
			Versus: gap.Versus, Points: gap.Points, CallsToPass: gap.CallsToPass, AboveIsPerfect: gap.AboveIsPerfect,
		}
	}
	return nil
}

func (c *callBoardRead) optRow(e sqlc.CallBoardEntry, ok bool) *CallRow {
	if !ok {
		return nil
	}
	row := c.row(e)
	return &row
}

func (c *callBoardRead) unrankedScreen(ctx context.Context, view *CallBoardView) error {
	entries, err := c.window(ctx, callBoardPodium+1, callBoardUnrankedMax)
	if err != nil {
		return err
	}
	view.Around, view.AfterCursor = c.rows(entries), c.afterCursor(entries)
	if err := c.unrankedCard(ctx, view); err != nil {
		return err
	}
	return c.unrankedPeople(ctx, view)
}

func (c *callBoardRead) unrankedPeople(ctx context.Context, view *CallBoardView) error {
	subjects := c.subjects
	if !c.q.Friends {
		following, err := c.Follows.FollowingIDs(ctx, c.q.Viewer)
		if err != nil {
			return err
		}
		subjects = []uuid.UUID{c.q.Viewer.UUID()}
		for _, id := range following {
			subjects = append(subjects, id.UUID())
		}
	}
	entries, err := c.Subjects.CallBoardUnranked(ctx, sqlc.CallBoardUnrankedParams{
		Board: string(c.q.Board), Period: string(c.q.Period), MinResolved: 1, Subjects: subjects,
		RowLimit: callBoardUnrankedCap,
	})
	if err != nil {
		return err
	}
	view.Unranked = c.rows(entries)
	return nil
}

func (c *callBoardRead) unrankedCard(ctx context.Context, view *CallBoardView) error {
	card, ok, err := c.unrankedSubject(ctx)
	if err != nil || !ok {
		return err
	}
	card.Needed = max(domain.MinResolvedCalls-card.Resolved, 0)
	view.ClimbBps = min(card.Resolved*climbFullBps/domain.MinResolvedCalls, climbFullBps)
	if c.q.Board == domain.BoardPeople {
		if card.PendingSymbols, err = c.pendingSymbols(ctx, view); err != nil {
			return err
		}
	}
	view.UnrankedMe = &card
	return nil
}

func (c *callBoardRead) unrankedSubject(ctx context.Context) (CallUnrankedMe, bool, error) {
	card := CallUnrankedMe{PendingSymbols: []string{}}
	switch {
	case c.self != nil:
		row := c.row(*c.self)
		card.Subject, card.Resolved, card.Voted, card.Eligible = row.Subject, row.Resolved, row.Voted, row.Eligible
		return card, true, nil
	case c.q.Board != domain.BoardPeople:
		return card, false, nil
	}
	cards, err := c.Users.UsersByID(ctx, []ids.UserID{c.q.Viewer})
	if err != nil {
		return card, false, err
	}
	user, ok := cards[c.q.Viewer]
	card.Subject = CallSubjectView{
		ID: c.q.Viewer.UUID(), Name: domain.SubjectName(user.DisplayName, user.Handle),
		Handle: textOrNil(user.Handle), PictureURL: textOrNil(user.PhotoURL),
	}
	return card, ok, nil
}

func (c *callBoardRead) pendingSymbols(ctx context.Context, view *CallBoardView) ([]string, error) {
	period := domain.PeriodContaining(c.q.Period, c.periodAt(view))
	pending, err := c.Pending.PendingSymbols(ctx, []ids.UserID{c.q.Viewer}, nil, period)
	if err != nil {
		return nil, err
	}
	return append([]string{}, pending[c.q.Viewer]...), nil
}

func (c *callBoardRead) periodAt(view *CallBoardView) time.Time {
	if view.PeriodStart != nil {
		return *view.PeriodStart
	}
	return c.Clock.Now()
}

func (c *callBoardRead) cabalsScreen(ctx context.Context, view *CallBoardView) error {
	for _, e := range c.mineRows {
		if row := c.row(e); row.Rank == nil {
			view.Unranked = append(view.Unranked, row)
		}
	}
	if c.self == nil || c.rankOf(*c.self) == nil {
		entries, err := c.window(ctx, 1, callBoardCabalsEmpty)
		if err != nil {
			return err
		}
		view.Around, view.AfterCursor = c.rows(entries), c.afterCursor(entries)
		return c.unrankedCard(ctx, view)
	}
	me := *c.rankOf(*c.self)
	last := me + callBoardBelow
	for _, e := range c.mineRows {
		if rank := c.rankOf(e); rank != nil && *rank <= me+callBoardCabalSpan {
			last = max(last, *rank)
		}
	}
	entries, err := c.window(ctx, me, last)
	if err != nil {
		return err
	}
	return c.neighbours(ctx, view, me, entries)
}

func rankArg(n int) int32 { return int32(min(max(n, 0), math.MaxInt32)) }

func callEntry(e sqlc.CallBoardEntry) domain.CallEntry {
	out := domain.CallEntry{
		CallSubject: domain.CallSubject{
			ID: e.SubjectID, Right: int(e.RightCount), Resolved: int(e.Resolved), Voted: int(e.Voted),
			Eligible: int(e.Eligible), CreatedAt: e.SubjectCreatedAt,
		},
		TurnoutBps: int(e.TurnoutBps),
	}
	if e.RateBps.Valid {
		rate := int(e.RateBps.Int32)
		out.RateBps = &rate
	}
	return out
}

func optEntry(e sqlc.CallBoardEntry, ok bool) *domain.CallEntry {
	if !ok {
		return nil
	}
	out := callEntry(e)
	return &out
}

func movementOf(e sqlc.CallBoardEntry) CallMovement {
	out := CallMovement{Kind: e.MovementKind}
	if e.Movement.Valid &&
		(e.MovementKind == string(domain.MovementUp) || e.MovementKind == string(domain.MovementDown)) {
		out.Places = int(max(e.Movement.Int32, -e.Movement.Int32))
	}
	return out
}

func lastCallOf(raw []byte) *CallLast {
	if len(raw) == 0 {
		return nil
	}
	var parsed lastCall
	if err := json.Unmarshal(raw, &parsed); err != nil || parsed.ProposalID == uuid.Nil {
		return nil
	}
	return &CallLast{ProposalID: parsed.ProposalID, Kind: parsed.Kind, Symbol: parsed.Symbol, Right: parsed.Right}
}

func periodDates(kind domain.PeriodKind, now, start time.Time) (first, last *time.Time) {
	if kind == domain.PeriodAll {
		return nil, nil
	}
	if start.IsZero() {
		start = now
	}
	p := domain.PeriodContaining(kind, start)
	end := p.End.AddDate(0, 0, -1)
	return &p.Start, &end
}

func encodeCallCursor(run uuid.UUID, lastRank int) string {
	return base64.RawURLEncoding.EncodeToString([]byte(run.String() + ":" + strconv.Itoa(lastRank)))
}

type callCursor struct {
	run   uuid.UUID
	after int
}

func parseCallCursor(raw string) (callCursor, error) {
	const op = "ranking.ReadCallBoard.parseCallCursor"
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return callCursor{}, errs.Wrap(err, errs.CodeInvalidInput, op)
	}
	id, rank, ok := strings.Cut(string(decoded), ":")
	run, runErr := uuid.Parse(id)
	last, rankErr := strconv.Atoi(rank)
	if !ok || runErr != nil || rankErr != nil || last < 0 {
		return callCursor{}, errs.New(errs.CodeInvalidInput, op, slog.String("cursor", raw))
	}
	return callCursor{run: run, after: last}, nil
}
