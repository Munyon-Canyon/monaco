package app

import (
	"context"
	"encoding/base64"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	governanceport "github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	rankingport "github.com/monaco/monaco/apps/backend/internal/modules/ranking/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const (
	UserActivityPageDefault = 20
	UserActivityPageMax     = 50
	activityBatches         = 5
	activityBodyGraphemes   = 140
)

const (
	rankVoted = iota
	rankProposed
	rankReplied
	rankToppedBoard
)

type ActivityKind string

const (
	ActivityVoted       ActivityKind = "voted"
	ActivityProposed    ActivityKind = "proposed"
	ActivityReplied     ActivityKind = "replied"
	ActivityToppedBoard ActivityKind = "topped_board"
)

type MemberActivity interface {
	VotesBy(
		ctx context.Context,
		user ids.UserID,
		after *governanceport.Cursor,
		limit int,
	) ([]governanceport.CastVote, error)
	ProposalsBy(
		ctx context.Context, user ids.UserID, after *governanceport.Cursor, limit int,
	) ([]governanceport.ProposedRow, error)
	ProposalSubjects(ctx context.Context, proposals []ids.ProposalID) (map[ids.ProposalID]governanceport.Subject, error)
}

type ClosedTops interface {
	ClosedMonthTops(ctx context.Context, user ids.UserID) ([]rankingport.ClosedTop, error)
}

type MonthTops struct{ Boards rankingport.CallBoards }

var _ ClosedTops = MonthTops{}

func (t MonthTops) ClosedMonthTops(ctx context.Context, user ids.UserID) ([]rankingport.ClosedTop, error) {
	return t.Boards.ClosedTops(ctx, user.UUID(), "people", rankingport.PeriodMonth)
}

type UnwiredMemberActivity struct{}

var _ MemberActivity = UnwiredMemberActivity{}

func (UnwiredMemberActivity) VotesBy(
	context.Context, ids.UserID, *governanceport.Cursor, int,
) ([]governanceport.CastVote, error) {
	return nil, errs.New(errs.CodeUpstreamUnavailable, "social.UnwiredMemberActivity.VotesBy")
}

func (UnwiredMemberActivity) ProposalsBy(
	context.Context, ids.UserID, *governanceport.Cursor, int,
) ([]governanceport.ProposedRow, error) {
	return nil, errs.New(errs.CodeUpstreamUnavailable, "social.UnwiredMemberActivity.ProposalsBy")
}

func (UnwiredMemberActivity) ProposalSubjects(
	context.Context, []ids.ProposalID,
) (map[ids.ProposalID]governanceport.Subject, error) {
	return nil, errs.New(errs.CodeUpstreamUnavailable, "social.UnwiredMemberActivity.ProposalSubjects")
}

type UnwiredClosedTops struct{}

var _ ClosedTops = UnwiredClosedTops{}

func (UnwiredClosedTops) ClosedMonthTops(context.Context, ids.UserID) ([]rankingport.ClosedTop, error) {
	return nil, errs.New(errs.CodeUpstreamUnavailable, "social.UnwiredClosedTops.ClosedMonthTops")
}

type ReplyRow struct {
	ID         uuid.UUID
	ProposalID ids.ProposalID
	Body       string
	At         time.Time
}

type ActivityStore interface {
	Replies(ctx context.Context, user ids.UserID, after *governanceport.Cursor, limit int) ([]ReplyRow, error)
	BlockedEitherWay(ctx context.Context, a, b ids.UserID) (bool, error)
}

type SQLActivityStore struct{ db sqlc.DBTX }

var _ ActivityStore = SQLActivityStore{}

func NewActivityStore(db sqlc.DBTX) SQLActivityStore { return SQLActivityStore{db: db} }

func (s SQLActivityStore) Replies(
	ctx context.Context, user ids.UserID, after *governanceport.Cursor, limit int,
) ([]ReplyRow, error) {
	if limit < 1 || limit > UserActivityPageMax {
		return nil, errs.New(errs.CodeInvalidInput, "social.ActivityStore.Replies", slog.Int("limit", limit))
	}
	params := sqlc.UserRepliesParams{AuthorID: user.UUID(), RowLimit: int32(limit)}
	if after != nil {
		params.HasCursor, params.CursorAt, params.CursorID = true, after.At, after.ID
	}
	rows, err := sqlc.New(s.db).UserReplies(ctx, params)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, "social.ActivityStore.Replies")
	}
	out := make([]ReplyRow, len(rows))
	for i, row := range rows {
		out[i] = ReplyRow{ID: row.ID, ProposalID: ids.ProposalIDFrom(row.ProposalID), Body: row.Body, At: row.CreatedAt}
	}
	return out, nil
}

func (s SQLActivityStore) BlockedEitherWay(ctx context.Context, a, b ids.UserID) (bool, error) {
	blocked, err := sqlc.New(s.db).
		IsBlockedEitherWay(ctx, sqlc.IsBlockedEitherWayParams{UserA: a.UUID(), UserB: b.UUID()})
	if err != nil {
		return false, errs.Wrap(err, errs.CodeInternal, "social.ActivityStore.BlockedEitherWay")
	}
	return blocked, nil
}

type ActivityCabal struct {
	ID         ids.CabalID
	Name       string
	PictureURL string
}

type ActivityResult struct{ Yes, No int }

type ActivityRow struct {
	Kind        ActivityKind
	At          time.Time
	Cabal       ActivityCabal
	ProposalID  ids.ProposalID
	CommentID   uuid.UUID
	Subject     string
	Symbol      string
	USDCMicros  *uint64
	Choice      string
	Status      string
	Result      *ActivityResult
	Body        string
	PeriodStart time.Time
	Right       int
	Resolved    int
}

type ActivityPosition struct {
	At   time.Time
	Rank int
	ID   string
}

type ActivityPage struct {
	Items []ActivityRow
	Next  *ActivityPosition
}

func (p ActivityPosition) before(o ActivityPosition) bool {
	switch {
	case !p.At.Equal(o.At):
		return p.At.After(o.At)
	case p.Rank != o.Rank:
		return p.Rank < o.Rank
	default:
		return p.ID > o.ID
	}
}

func (p ActivityPosition) Encode() string {
	raw := strconv.FormatInt(p.At.UnixMicro(), 10) + "." + strconv.Itoa(p.Rank) + "." + p.ID
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func ParseActivityPosition(raw string) (ActivityPosition, error) {
	const op = "social.ParseActivityPosition"
	invalid := errs.New(errs.CodeInvalidInput, op, slog.String("cursor", raw))
	body, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return ActivityPosition{}, invalid
	}
	parts := strings.SplitN(string(body), ".", 3)
	if len(parts) != 3 {
		return ActivityPosition{}, invalid
	}
	micros, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return ActivityPosition{}, invalid
	}
	rank, err := strconv.Atoi(parts[1])
	if err != nil || rank < rankVoted || rank > rankToppedBoard {
		return ActivityPosition{}, invalid
	}
	pos := ActivityPosition{At: time.UnixMicro(micros).UTC(), Rank: rank, ID: parts[2]}
	if rank == rankToppedBoard {
		if _, err := time.Parse(time.DateOnly, pos.ID); err != nil {
			return ActivityPosition{}, invalid
		}
	} else if _, err := uuid.Parse(pos.ID); err != nil {
		return ActivityPosition{}, invalid
	}
	return pos, nil
}

func maxActivityID() uuid.UUID {
	var id uuid.UUID
	for i := range id {
		id[i] = 0xff
	}
	return id
}

func (p ActivityPosition) after(rank int) *governanceport.Cursor {
	switch {
	case rank == p.Rank:
		id, _ := uuid.Parse(p.ID)
		return &governanceport.Cursor{At: p.At, ID: id}
	case rank > p.Rank:
		return &governanceport.Cursor{At: p.At, ID: maxActivityID()}
	default:
		return &governanceport.Cursor{At: p.At, ID: uuid.Nil}
	}
}

type activityItem struct {
	pos   ActivityPosition
	row   ActivityRow
	cabal ids.CabalID
	drop  bool
}

type activitySource struct {
	rank  int
	fetch func(ctx context.Context, after *ActivityPosition, limit int) ([]activityItem, error)
}

type UserActivityQuery struct {
	Store    ActivityStore
	Users    Users
	Cabals   CabalPictures
	Activity MemberActivity
	Tops     ClosedTops
}

func (q *UserActivityQuery) Run(
	ctx context.Context, viewer, user ids.UserID, cursor *ActivityPosition, limit int,
) (ActivityPage, error) {
	const op = "social.UserActivity"
	if limit < 1 || limit > UserActivityPageMax {
		return ActivityPage{}, errs.New(errs.CodeInvalidInput, op, slog.Int("limit", limit))
	}
	if err := q.requireUser(ctx, user); err != nil {
		return ActivityPage{}, err
	}
	if viewer != user {
		blocked, err := q.Store.BlockedEitherWay(ctx, viewer, user)
		if err != nil {
			return ActivityPage{}, err
		}
		if blocked {
			return ActivityPage{Items: []ActivityRow{}}, nil
		}
	}
	cabals := cabalBook{port: q.Cabals, views: map[ids.CabalID]*cabalport.CabalView{}}
	sources := []activitySource{
		{rankVoted, q.votes(user, viewer == user)},
		{rankProposed, q.proposals(user)},
		{rankReplied, q.replies(user)},
		{rankToppedBoard, q.tops(user)},
	}
	var pulled []pulledSource
	for _, source := range sources {
		got, err := pullSource(ctx, source, &cabals, cursor, limit)
		if err != nil {
			return ActivityPage{}, errs.Wrap(err, errs.CodeOf(err), op)
		}
		pulled = append(pulled, got)
	}
	return mergeActivity(pulled, limit), nil
}

func (q *UserActivityQuery) requireUser(ctx context.Context, user ids.UserID) error {
	status, err := statusOf(ctx, q.Users, user)
	if err != nil {
		return err
	}
	if status == domain.AccountUnknown || status == domain.AccountDeleted || status == domain.AccountBanned {
		return errs.New(errs.CodeUserNotFound, "social.UserActivity")
	}
	return nil
}

type pulledSource struct {
	items     []activityItem
	frontier  *ActivityPosition
	exhausted bool
}

func pullSource(
	ctx context.Context, source activitySource, cabals *cabalBook, cursor *ActivityPosition, limit int,
) (pulledSource, error) {
	var out pulledSource
	after := cursor
	for range activityBatches {
		raw, err := source.fetch(ctx, after, limit)
		if err != nil {
			return pulledSource{}, err
		}
		visible, err := cabals.visible(ctx, raw)
		if err != nil {
			return pulledSource{}, err
		}
		out.items = append(out.items, visible...)
		if len(raw) > 0 {
			last := raw[len(raw)-1].pos
			out.frontier, after = &last, &last
		}
		if len(raw) < limit {
			out.exhausted = true
			return out, nil
		}
		if len(out.items) >= limit {
			break
		}
	}
	if len(out.items) > limit {
		out.items = out.items[:limit]
		last := out.items[limit-1].pos
		out.frontier = &last
	}
	return out, nil
}

func mergeActivity(pulled []pulledSource, limit int) ActivityPage {
	var all []activityItem
	for _, source := range pulled {
		all = append(all, source.items...)
	}
	slices.SortFunc(all, compareActivity)
	frontier := earliestFrontier(pulled)
	more := frontier != nil
	if more {
		all = slices.DeleteFunc(all, func(item activityItem) bool { return frontier.before(item.pos) })
	}
	if len(all) > limit {
		all, more = all[:limit], true
	}
	page := ActivityPage{Items: make([]ActivityRow, len(all))}
	for i, item := range all {
		page.Items[i] = item.row
	}
	if more {
		page.Next = frontier
		if len(all) == limit {
			page.Next = &all[limit-1].pos
		}
	}
	return page
}

func compareActivity(a, b activityItem) int {
	if a.pos.before(b.pos) {
		return -1
	}
	return 1
}

func earliestFrontier(pulled []pulledSource) *ActivityPosition {
	var earliest *ActivityPosition
	for _, source := range pulled {
		if source.exhausted || source.frontier == nil {
			continue
		}
		if earliest == nil || source.frontier.before(*earliest) {
			earliest = source.frontier
		}
	}
	return earliest
}

type cabalBook struct {
	port  CabalPictures
	views map[ids.CabalID]*cabalport.CabalView
}

func (b *cabalBook) load(ctx context.Context, items []activityItem) error {
	var missing []ids.CabalID
	for _, item := range items {
		_, seen := b.views[item.cabal]
		if item.drop || item.cabal == (ids.CabalID{}) || seen || slices.Contains(missing, item.cabal) {
			continue
		}
		missing = append(missing, item.cabal)
	}
	if len(missing) == 0 {
		return nil
	}
	views, err := b.port.Cabals(ctx, missing)
	if err != nil {
		return err
	}
	for _, id := range missing {
		view, ok := views[id]
		if ok {
			b.views[id] = &view
		} else {
			b.views[id] = nil
		}
	}
	return nil
}

func (b *cabalBook) visible(ctx context.Context, items []activityItem) ([]activityItem, error) {
	if err := b.load(ctx, items); err != nil {
		return nil, err
	}
	out := make([]activityItem, 0, len(items))
	for _, item := range items {
		if item.drop {
			continue
		}
		if item.row.Kind != ActivityToppedBoard {
			view := b.views[item.cabal]
			if view == nil || view.Status == cabalport.StatusBanned {
				continue
			}
			item.row.Cabal = ActivityCabal{ID: view.ID, Name: view.Name, PictureURL: view.PictureURL}
		}
		out = append(out, item)
	}
	return out, nil
}

func (q *UserActivityQuery) votes(
	user ids.UserID,
	own bool,
) func(context.Context, *ActivityPosition, int) ([]activityItem, error) {
	return func(ctx context.Context, after *ActivityPosition, limit int) ([]activityItem, error) {
		rows, err := q.Activity.VotesBy(ctx, user, afterFor(after, rankVoted), limit)
		if err != nil {
			return nil, err
		}
		out := make([]activityItem, len(rows))
		for i, row := range rows {
			item := activityItem{
				pos: ActivityPosition{At: row.CastAt.UTC(), Rank: rankVoted, ID: row.ProposalID.UUID().String()},
				row: ActivityRow{
					Kind: ActivityVoted, At: row.CastAt, ProposalID: row.ProposalID,
					Subject: subjectOf(row.Kind, row.Symbol), Symbol: row.Symbol, USDCMicros: row.USDCMicros,
				},
				cabal: row.CabalID,
			}
			if own {
				item.row.Choice = row.Choice
			}
			out[i] = item
		}
		return out, nil
	}
}

func (q *UserActivityQuery) proposals(
	user ids.UserID,
) func(context.Context, *ActivityPosition, int) ([]activityItem, error) {
	return func(ctx context.Context, after *ActivityPosition, limit int) ([]activityItem, error) {
		rows, err := q.Activity.ProposalsBy(ctx, user, afterFor(after, rankProposed), limit)
		if err != nil {
			return nil, err
		}
		out := make([]activityItem, len(rows))
		for i, row := range rows {
			item := activityItem{
				pos: ActivityPosition{At: row.CreatedAt.UTC(), Rank: rankProposed, ID: row.ProposalID.UUID().String()},
				row: ActivityRow{
					Kind: ActivityProposed, At: row.CreatedAt, ProposalID: row.ProposalID,
					Subject: subjectOf(row.Kind, row.Symbol), Symbol: row.Symbol, USDCMicros: row.USDCMicros,
					Status: string(row.Status),
				},
				cabal: row.CabalID,
			}
			if row.Status != governanceport.StatusOpen {
				item.row.Result = &ActivityResult{Yes: row.Yes, No: row.No}
			}
			out[i] = item
		}
		return out, nil
	}
}

func (q *UserActivityQuery) replies(
	user ids.UserID,
) func(context.Context, *ActivityPosition, int) ([]activityItem, error) {
	return func(ctx context.Context, after *ActivityPosition, limit int) ([]activityItem, error) {
		rows, err := q.Store.Replies(ctx, user, afterFor(after, rankReplied), limit)
		if err != nil {
			return nil, err
		}
		proposals := make([]ids.ProposalID, len(rows))
		for i, row := range rows {
			proposals[i] = row.ProposalID
		}
		subjects := map[ids.ProposalID]governanceport.Subject{}
		if len(proposals) > 0 {
			if subjects, err = q.Activity.ProposalSubjects(ctx, proposals); err != nil {
				return nil, err
			}
		}
		out := make([]activityItem, len(rows))
		for i, row := range rows {
			subject, ok := subjects[row.ProposalID]
			out[i] = activityItem{
				pos: ActivityPosition{At: row.At.UTC(), Rank: rankReplied, ID: row.ID.String()},
				row: ActivityRow{
					Kind:       ActivityReplied,
					At:         row.At,
					ProposalID: row.ProposalID,
					CommentID:  row.ID,
					Body: cutGraphemes(
						row.Body,
						activityBodyGraphemes,
					),
					Subject:    subjectOf(subject.Kind, subject.Symbol),
					USDCMicros: subject.USDCMicros,
				},
				cabal: subject.CabalID,
				drop:  !ok,
			}
		}
		return out, nil
	}
}

func (q *UserActivityQuery) tops(
	user ids.UserID,
) func(context.Context, *ActivityPosition, int) ([]activityItem, error) {
	return func(ctx context.Context, after *ActivityPosition, limit int) ([]activityItem, error) {
		tops, err := q.Tops.ClosedMonthTops(ctx, user)
		if err != nil {
			return nil, err
		}
		var out []activityItem
		for _, top := range tops {
			start := top.PeriodStart.UTC()
			pos := ActivityPosition{At: start.AddDate(0, 1, 0), Rank: rankToppedBoard, ID: start.Format(time.DateOnly)}
			if after != nil && !after.before(pos) {
				continue
			}
			out = append(out, activityItem{
				pos: pos,
				row: ActivityRow{
					Kind: ActivityToppedBoard, At: pos.At, PeriodStart: start, Right: top.Right, Resolved: top.Resolved,
				},
			})
		}
		slices.SortFunc(out, compareActivity)
		return out[:min(limit, len(out))], nil
	}
}

func afterFor(after *ActivityPosition, rank int) *governanceport.Cursor {
	if after == nil {
		return nil
	}
	return after.after(rank)
}

func subjectOf(kind, symbol string) string {
	if kind == "" {
		return symbol
	}
	return strings.ToUpper(kind[:1]) + kind[1:] + " " + symbol
}

func cutGraphemes(s string, limit int) string {
	count, prev, run := 0, rune(0), 0
	for i, r := range s {
		if isRegionalIndicator(r) {
			run++
		} else {
			run = 0
		}
		if !extendsCluster(prev, r, run) {
			if count == limit {
				return s[:i]
			}
			count++
		}
		prev = r
	}
	return s
}

func extendsCluster(prev, r rune, indicatorRun int) bool {
	switch {
	case prev == 0:
		return false
	case r == '\u200d', prev == '\u200d':
		return true
	case r >= 0x1F3FB && r <= 0x1F3FF, r >= 0xE0020 && r <= 0xE007F, r >= 0xFE00 && r <= 0xFE0F:
		return true
	case unicode.In(r, unicode.Mn, unicode.Me, unicode.Mc):
		return true
	}
	return indicatorRun%2 == 0 && indicatorRun > 0
}

func isRegionalIndicator(r rune) bool { return r >= 0x1F1E6 && r <= 0x1F1FF }
