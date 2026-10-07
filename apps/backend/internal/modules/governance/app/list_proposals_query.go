package app

import (
	"context"
	"encoding/base64"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const (
	listDefault = 20
	listMax     = 50
)

type Filter string

const (
	FilterAll    Filter = "all"
	FilterOpen   Filter = "open"
	FilterClosed Filter = "closed"
)

type ListProposals struct {
	CabalID ids.CabalID
	Caller  ids.UserID
	Filter  Filter
	Limit   int
	Cursor  string
}

type ProposalView struct {
	ID           ids.ProposalID
	CabalID      ids.CabalID
	ProposerID   ids.UserID
	Kind         domain.Kind
	Symbol       string
	USDCMicros   int64
	TokenAmount  int64
	QuoteOut     int64
	Thesis       string
	Status       domain.Status
	StatusReason errs.Code
	ExpiresAt    time.Time
	CreatedAt    time.Time
	Tally        Tally
	MyBallot     domain.Choice
	CanVote      bool
}

type ProposalPage struct {
	Items      []ProposalView
	NextCursor string
}

type ProposalReads struct {
	q     *sqlc.Queries
	swaps Swaps
}

func NewProposalReads(db sqlc.DBTX, s Swaps) *ProposalReads {
	return &ProposalReads{q: sqlc.New(db), swaps: s}
}

type pageCursor struct {
	ok        bool
	createdAt time.Time
	id        uuid.UUID
}

func (r *ProposalReads) List(ctx context.Context, req ListProposals) (ProposalPage, error) {
	const op = "governance.ListProposals"
	filter, limit, cursor, err := parseList(req)
	if err != nil {
		return ProposalPage{}, err
	}
	rows, err := r.q.ListProposals(ctx, sqlc.ListProposalsParams{
		CallerID: req.Caller.UUID(), CabalID: req.CabalID.UUID(), Filter: string(filter),
		HasCursor: cursor.ok, CursorCreatedAt: cursor.createdAt, CursorID: cursor.id, RowLimit: limit + 1,
	})
	if err != nil {
		return ProposalPage{}, errs.Wrap(err, errs.CodeInternal, op)
	}
	page := ProposalPage{Items: []ProposalView{}}
	if len(rows) > int(limit) {
		rows = rows[:limit]
		last := rows[len(rows)-1]
		page.NextCursor = encodeCursor(last.CreatedAt, last.ID)
	}
	if len(rows) == 0 {
		return page, nil
	}
	tallies, err := r.tallies(ctx, req.Caller, rows)
	if err != nil {
		return ProposalPage{}, err
	}
	for _, row := range rows {
		view, err := proposalView(row, tallies[row.ID].tally)
		if err != nil {
			return ProposalPage{}, err
		}
		view.CanVote = view.Status == domain.StatusOpen && tallies[row.ID].callerVotes
		page.Items = append(page.Items, view)
	}
	return page, nil
}

type listedTally struct {
	tally       Tally
	callerVotes bool
}

func (r *ProposalReads) tallies(
	ctx context.Context, caller ids.UserID, rows []sqlc.ListProposalsRow,
) (map[uuid.UUID]listedTally, error) {
	proposals := make([]uuid.UUID, len(rows))
	rules := make(map[uuid.UUID]domain.ThresholdRule, len(rows))
	for i, row := range rows {
		proposals[i] = row.ID
		rules[row.ID] = domain.ThresholdRule(row.Threshold)
	}
	counts, err := r.q.TallyProposals(ctx, sqlc.TallyProposalsParams{ProposalIds: proposals, CallerID: caller.UUID()})
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, "governance.TallyProposals")
	}
	out := make(map[uuid.UUID]listedTally, len(counts))
	for _, c := range counts {
		voters := int(c.Voters)
		needed := rules[c.ProposalID].Needed(voters)
		out[c.ProposalID] = listedTally{
			tally:       Tally{Yes: int(c.Yes), No: int(c.No), Voters: voters, Needed: needed},
			callerVotes: c.CallerVotes,
		}
	}
	return out, nil
}

func proposalView(row sqlc.ListProposalsRow, tally Tally) (ProposalView, error) {
	kind, err := domain.ParseKind(row.Kind)
	if err != nil {
		return ProposalView{}, err
	}
	status, err := domain.ParseStatus(row.Status)
	if err != nil {
		return ProposalView{}, err
	}
	return ProposalView{
		ID: ids.ProposalIDFrom(row.ID), CabalID: ids.CabalIDFrom(row.CabalID),
		ProposerID: ids.UserIDFrom(row.ProposerID), Kind: kind, Symbol: row.Symbol, USDCMicros: row.UsdcMicros.Int64,
		TokenAmount: row.TokenAmount.Int64, QuoteOut: row.QuoteOutAmount, Thesis: row.Thesis.String,
		Status: status, StatusReason: errs.Code(row.StatusReason.String), ExpiresAt: row.ExpiresAt,
		CreatedAt: row.CreatedAt, Tally: tally, MyBallot: domain.Choice(row.MyBallot),
	}, nil
}

func parseList(req ListProposals) (Filter, int32, pageCursor, error) {
	const op = "governance.ListProposals"
	filter := req.Filter
	switch filter {
	case "":
		filter = FilterAll
	case FilterAll, FilterOpen, FilterClosed:
	default:
		return "", 0, pageCursor{}, errs.New(errs.CodeInvalidInput, op, slog.String("field", "filter"))
	}
	limit := req.Limit
	if limit == 0 {
		limit = listDefault
	}
	if limit < 1 || limit > listMax {
		return "", 0, pageCursor{}, errs.New(errs.CodeInvalidInput, op, slog.String("field", "limit"))
	}
	cursor, err := decodeCursor(req.Cursor)
	if err != nil {
		return "", 0, pageCursor{}, err
	}
	return filter, int32(limit), cursor, nil
}

func encodeCursor(createdAt time.Time, id uuid.UUID) string {
	raw := strconv.FormatInt(createdAt.UnixMicro(), 10) + "\x00" + id.String()
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeCursor(raw string) (pageCursor, error) {
	const op = "governance.cursor"
	if raw == "" {
		return pageCursor{}, nil
	}
	body, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return pageCursor{}, errs.Wrap(err, errs.CodeInvalidInput, op)
	}
	micros, id, ok := strings.Cut(string(body), "\x00")
	if !ok {
		return pageCursor{}, errs.New(errs.CodeInvalidInput, op)
	}
	at, err := strconv.ParseInt(micros, 10, 64)
	if err != nil {
		return pageCursor{}, errs.Wrap(err, errs.CodeInvalidInput, op)
	}
	parsed, err := uuid.Parse(id)
	if err != nil {
		return pageCursor{}, errs.Wrap(err, errs.CodeInvalidInput, op)
	}
	return pageCursor{ok: true, createdAt: time.UnixMicro(at).UTC(), id: parsed}, nil
}
