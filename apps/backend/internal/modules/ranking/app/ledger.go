package app

import (
	"cmp"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const ledgerCallMarker = "Called %s · %s"

var ledgerWeekPattern = regexp.MustCompile(`^\d{4}-W\d{2}$`)

type LedgerStore interface {
	UserCallsResolvedBetween(
		context.Context, sqlc.UserCallsResolvedBetweenParams,
	) ([]sqlc.UserCallsResolvedBetweenRow, error)
	UserTurnoutBetween(context.Context, sqlc.UserTurnoutBetweenParams) (sqlc.UserTurnoutBetweenRow, error)
	CabalmatesRecordsBetween(
		context.Context, sqlc.CabalmatesRecordsBetweenParams,
	) ([]sqlc.CabalmatesRecordsBetweenRow, error)
	CallProposalsInCabals(
		context.Context, sqlc.CallProposalsInCabalsParams,
	) ([]sqlc.CallProposalsInCabalsRow, error)
}

type LedgerInputs struct {
	Store   LedgerStore
	Market  Market
	Users   Users
	Cards   CabalCards
	Trades  Trades
	Replies Replies
	Ports   Ports
	Stakes  StakeHistory
	Snaps   CabalSnapshots
	Window  *LedgerWindow
}

func BuildLedger(
	ctx context.Context, in LedgerInputs, user ids.UserID, deliveryAt time.Time,
) (domain.Ledger, bool, error) {
	week, labelStart, labelEnd, start, end := domain.LedgerWeek(deliveryAt)
	ledger := domain.Ledger{
		Week: week, LabelStart: labelStart.Format(time.DateOnly), LabelEnd: labelEnd.Format(time.DateOnly),
		WindowStart: start, WindowEnd: end, Series: []domain.LedgerPoint{},
	}
	win, member, err := in.memberWindow(ctx, user, start, end)
	if err != nil {
		return domain.Ledger{}, false, err
	}
	ledger.StakeReturnBps = member.returnBps
	cabalIDs := member.ids()
	if ledger.Cabals, err = in.cabals(ctx, member.cabals); err != nil {
		return domain.Ledger{}, false, err
	}
	turnout, err := in.Store.UserTurnoutBetween(ctx, sqlc.UserTurnoutBetweenParams{
		UserID: user.UUID(), WindowStart: start, WindowEnd: end,
	})
	if err != nil {
		return domain.Ledger{}, false, errs.Wrap(err, errs.CodeInternal, "ranking.BuildLedger")
	}
	ledger.Voted, ledger.Eligible = int(turnout.Voted), int(turnout.Eligible)
	if len(cabalIDs) == 0 && ledger.Eligible == 0 {
		return domain.Ledger{}, false, nil
	}
	if ledger.Calls, err = in.calls(ctx, user, start, end); err != nil {
		return domain.Ledger{}, false, err
	}
	ledger.Tally()
	if ledger.Series, err = in.series(ctx, user, win, member); err != nil {
		return domain.Ledger{}, false, err
	}
	ledger.BestCall, ledger.Miss = domain.PickBest(ledger.Calls), domain.PickMiss(ledger.Calls)
	if ledger.Sharpest, err = in.sharpest(ctx, cabalIDs, start, end); err != nil {
		return domain.Ledger{}, false, err
	}
	if ledger.TopReply, err = in.topReply(ctx, cabalIDs, start, end); err != nil {
		return domain.Ledger{}, false, err
	}
	if ledger.Markers, err = in.markers(ctx, ledger.BestCall, cabalIDs, start, end); err != nil {
		return domain.Ledger{}, false, err
	}
	return ledger, true, nil
}

func (in LedgerInputs) memberWindow(
	ctx context.Context, user ids.UserID, start, end time.Time,
) (*LedgerWindow, memberWindow, error) {
	win, err := in.Window, error(nil)
	if win == nil {
		win, err = NewLedgerWindow(ctx, in.Ports, start, end)
	}
	if err != nil {
		return nil, memberWindow{}, err
	}
	member, err := win.member(user)
	return win, member, err
}

func (in LedgerInputs) cabals(ctx context.Context, held []heldCabal) ([]domain.LedgerCabal, error) {
	cabalIDs := make([]ids.CabalID, len(held))
	for i, h := range held {
		cabalIDs[i] = h.id
	}
	cards, err := in.Cards.Cabals(ctx, cabalIDs)
	if err != nil {
		return nil, err
	}
	slices.SortFunc(held, func(a, b heldCabal) int {
		return cmp.Or(
			b.equity.Cmp(a.equity),
			strings.Compare(cards[a.id].Name, cards[b.id].Name),
			strings.Compare(a.id.String(), b.id.String()),
		)
	})
	out := make([]domain.LedgerCabal, 0, len(held))
	for _, h := range held {
		card := cards[h.id]
		out = append(
			out,
			domain.LedgerCabal{ID: h.id.UUID(), Name: card.Name, PictureURL: textOrNilString(card.PictureURL)},
		)
	}
	return out, nil
}

func (in LedgerInputs) calls(
	ctx context.Context, user ids.UserID, start, end time.Time,
) ([]domain.LedgerCall, error) {
	rows, err := in.Store.UserCallsResolvedBetween(ctx, sqlc.UserCallsResolvedBetweenParams{
		UserID: user.UUID(), WindowStart: start, WindowEnd: end,
	})
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, "ranking.BuildLedger.calls")
	}
	mints := make([]string, 0, len(rows))
	cabalIDs := make([]ids.CabalID, 0, len(rows))
	for _, row := range rows {
		mints = append(mints, row.Mint.String)
		cabalIDs = append(cabalIDs, ids.CabalIDFrom(row.CabalID))
	}
	assets, err := in.Market.AssetsByMints(ctx, mints)
	if err != nil {
		return nil, err
	}
	logos := make(map[string]string, len(assets))
	for _, a := range assets {
		logos[a.Mint.String()] = a.LogoURL
	}
	cards, err := in.Cards.Cabals(ctx, cabalIDs)
	if err != nil {
		return nil, err
	}
	out := make([]domain.LedgerCall, 0, len(rows))
	for _, row := range rows {
		cabalID := ids.CabalIDFrom(row.CabalID)
		out = append(out, domain.LedgerCall{
			ProposalID:   row.ProposalID,
			Cabal:        domain.LedgerCallCabal{ID: row.CabalID, Name: cards[cabalID].Name},
			Kind:         row.Kind.String,
			Symbol:       row.Symbol.String,
			AssetLogoURL: textOrNilString(logos[row.Mint.String]),
			YourChoice:   row.Choice.String,
			MoveBps:      domain.YourMove(row.Kind.String, row.Choice.String, int(row.MoveBps.Int32)),
			Right:        row.IsRight.Bool,
			DecidedAt:    row.DecidedAt.Time,
		})
	}
	return out, nil
}

func (in LedgerInputs) sharpest(
	ctx context.Context, cabalIDs []ids.CabalID, start, end time.Time,
) (*domain.LedgerSharpest, error) {
	rows, err := in.Store.CabalmatesRecordsBetween(ctx, sqlc.CabalmatesRecordsBetweenParams{
		CabalIds: uuids(cabalIDs), WindowStart: start, WindowEnd: end,
	})
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, "ranking.BuildLedger.sharpest")
	}
	wanted := make([]ids.UserID, len(rows))
	for i, row := range rows {
		wanted[i] = ids.UserIDFrom(row.UserID)
	}
	cards, err := in.Users.UsersByID(ctx, wanted)
	if err != nil {
		return nil, err
	}
	records := make([]domain.LedgerSharpest, 0, len(rows))
	for _, row := range rows {
		card := cards[ids.UserIDFrom(row.UserID)]
		records = append(records, domain.LedgerSharpest{
			User: domain.LedgerUser{
				ID: row.UserID, Name: domain.SubjectName(card.DisplayName, card.Handle),
				PictureURL: textOrNilString(card.PhotoURL),
			},
			Right: int(row.RightCalls), Resolved: int(row.ResolvedCalls),
		})
	}
	return domain.PickSharpest(records), nil
}

func (in LedgerInputs) topReply(
	ctx context.Context, cabalIDs []ids.CabalID, start, end time.Time,
) (*domain.LedgerReply, error) {
	proposals, err := in.Store.CallProposalsInCabals(ctx, sqlc.CallProposalsInCabalsParams{
		CabalIds: uuids(cabalIDs), WindowStart: start, WindowEnd: end,
	})
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, "ranking.BuildLedger.topReply")
	}
	wanted := make([]ids.ProposalID, len(proposals))
	labels := make(map[uuid.UUID]string, len(proposals))
	for i, p := range proposals {
		wanted[i] = ids.ProposalIDFrom(p.ProposalID)
		labels[p.ProposalID] = proposalLabel(p.Kind.String, p.Symbol.String)
	}
	top, err := in.Replies.TopReply(ctx, wanted, start, end)
	if err != nil || top == nil {
		return nil, err
	}
	cards, err := in.Users.UsersByID(ctx, []ids.UserID{top.AuthorID})
	if err != nil {
		return nil, err
	}
	card := cards[top.AuthorID]
	return &domain.LedgerReply{
		CommentID:     top.CommentID,
		ProposalID:    top.ProposalID,
		ProposalLabel: labels[top.ProposalID],
		Author: domain.LedgerUser{
			ID: top.AuthorID.UUID(), Name: domain.SubjectName(card.DisplayName, card.Handle),
			PictureURL: textOrNilString(card.PhotoURL),
		},
		Excerpt:       top.Excerpt,
		ReactionCount: top.ReactionCount,
		TopEmoji:      top.TopEmoji,
		TopEmojiCount: top.TopEmojiCount,
	}, nil
}

func (in LedgerInputs) markers(
	ctx context.Context, best *domain.LedgerCall, cabalIDs []ids.CabalID, start, end time.Time,
) ([]domain.LedgerMarker, error) {
	out := []domain.LedgerMarker{}
	if best != nil {
		out = append(out, domain.LedgerMarker{
			At: best.DecidedAt, Kind: domain.LedgerMarkerCall, Symbol: best.Symbol,
			Label: fmt.Sprintf(ledgerCallMarker, best.Symbol, best.DecidedAt.UTC().Format("Mon")),
		})
	}
	buys, err := in.Trades.FilledBuysBetween(ctx, cabalIDs, start, end)
	if err != nil {
		return nil, err
	}
	for _, b := range buys {
		out = append(out, domain.LedgerMarker{
			At: b.ConfirmedAt, Kind: domain.LedgerMarkerBuy, Symbol: b.Symbol, Label: domain.LedgerBuyMarkerLabel,
		})
	}
	slices.SortStableFunc(out, func(a, b domain.LedgerMarker) int { return a.At.Compare(b.At) })
	return out, nil
}

func proposalLabel(kind, symbol string) string {
	return strings.ToUpper(kind[:1]) + kind[1:] + " " + symbol
}

func textOrNilString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func uuids(in []ids.CabalID) []uuid.UUID {
	out := make([]uuid.UUID, len(in))
	for i, id := range in {
		out[i] = id.UUID()
	}
	return out
}

func StoreLedger(
	ctx context.Context, tx db.Tx, user ids.UserID, ledger domain.Ledger, builtAt time.Time,
) (bool, error) {
	payload, err := json.Marshal(ledger)
	if err != nil {
		return false, errs.Wrap(err, errs.CodeInternal, "ranking.StoreLedger")
	}
	n, err := sqlc.New(tx.Queries()).InsertLedger(ctx, sqlc.InsertLedgerParams{
		UserID: user.UUID(), Week: ledger.Week, WindowStart: ledger.WindowStart, WindowEnd: ledger.WindowEnd,
		Payload: payload, BuiltAt: builtAt,
	})
	if err != nil {
		return false, errs.Wrap(err, errs.CodeInternal, "ranking.StoreLedger")
	}
	return n == 1, nil
}

func StoreSkip(ctx context.Context, tx db.Tx, user ids.UserID, week string) error {
	err := sqlc.New(tx.Queries()).InsertLedgerSkip(ctx, sqlc.InsertLedgerSkipParams{UserID: user.UUID(), Week: week})
	if err != nil {
		return errs.Wrap(err, errs.CodeInternal, "ranking.StoreSkip")
	}
	return nil
}

type LedgerReads interface {
	GetLedger(context.Context, sqlc.GetLedgerParams) ([]byte, error)
	LatestLedger(context.Context, uuid.UUID) ([]byte, error)
}

type ReadLedger struct{ Reads LedgerReads }

func (r ReadLedger) Latest(ctx context.Context, user ids.UserID) ([]byte, error) {
	payload, err := r.Reads.LatestLedger(ctx, user.UUID())
	return ledgerFound("ranking.ReadLedger.Latest", payload, err)
}

func (r ReadLedger) Week(ctx context.Context, user ids.UserID, week string) ([]byte, error) {
	const op = "ranking.ReadLedger.Week"
	if !ledgerWeekPattern.MatchString(week) {
		return nil, errs.New(errs.CodeInvalidInput, op)
	}
	payload, err := r.Reads.GetLedger(ctx, sqlc.GetLedgerParams{UserID: user.UUID(), Week: week})
	return ledgerFound(op, payload, err)
}

func ledgerFound(op string, payload []byte, err error) ([]byte, error) {
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errs.New(errs.CodeLedgerNotFound, op)
	}
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, op)
	}
	return payload, nil
}
