package adapters

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	dbsqlc "github.com/monaco/monaco/apps/backend/internal/platform/db/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const tradeSourceProposal = "proposal"

type Calls struct {
	Ballots app.Ballots
	Market  app.Market
	DB      dbsqlc.DBTX
	Clock   clock.Clock
}

type CallOpening struct {
	skip    bool
	assetID pgtype.UUID
	ballots []ballot
}

type CallDecision struct {
	skip    bool
	assetID pgtype.UUID
	ballots []ballot
	ref     *app.AssetPrice
}

type CallFill struct {
	skip  bool
	price pgtype.Int8
}

type ballot struct {
	voter  uuid.UUID
	choice pgtype.Text
	at     pgtype.Timestamptz
}

func tradeKind(kind string) bool { return kind == domain.CallBuy || kind == domain.CallSell }

func (c Calls) FetchCreated(ctx context.Context, e events.ProposalCreated) (CallOpening, error) {
	if !tradeKind(e.Kind) {
		return CallOpening{skip: true}, nil
	}
	asset, err := c.asset(ctx, string(e.Mint))
	if err != nil {
		return CallOpening{}, err
	}
	ballots, err := c.ballots(ctx, e.ProposalID)
	if err != nil {
		return CallOpening{}, err
	}
	return CallOpening{assetID: asset, ballots: ballots}, nil
}

func (Calls) ApplyCreated(ctx context.Context, tx db.Tx, e events.ProposalCreated, f CallOpening, at time.Time) error {
	if f.skip {
		return nil
	}
	q := sqlc.New(tx.Queries())
	if err := q.UpsertCallOpened(ctx, sqlc.UpsertCallOpenedParams{
		ProposalID: e.ProposalID, CabalID: e.CabalID, Kind: text(e.Kind), Symbol: text(e.Symbol),
		Mint: text(string(e.Mint)), AssetID: f.assetID, OpenedAt: at, At: at,
	}); err != nil {
		return err
	}
	return upsertBallots(ctx, q, e.ProposalID, e.CabalID, f.ballots)
}

func (Calls) VoteCast(ctx context.Context, tx db.Tx, e events.VoteCast, _ time.Time) error {
	return sqlc.New(tx.Queries()).UpsertCallVoter(ctx, sqlc.UpsertCallVoterParams{
		ProposalID: e.ProposalID, UserID: e.VoterID, CabalID: e.CabalID,
		Choice: text(e.Choice), ChoiceAt: timestamp(e.CastAt),
	})
}

func (Calls) VoteRetracted(ctx context.Context, tx db.Tx, e events.VoteRetracted, _ time.Time) error {
	return sqlc.New(tx.Queries()).ResetCallVoterChoice(ctx, sqlc.ResetCallVoterChoiceParams{
		ProposalID: e.ProposalID, UserID: e.VoterID, At: e.RetractedAt,
	})
}

func (c Calls) FetchPassed(ctx context.Context, e events.ProposalPassed) (CallDecision, error) {
	if !tradeKind(e.Kind) {
		return CallDecision{skip: true}, nil
	}
	asset, err := c.asset(ctx, string(e.Mint))
	if err != nil {
		return CallDecision{}, err
	}
	return c.decision(ctx, e.ProposalID, asset)
}

func (c Calls) FetchFailed(ctx context.Context, e events.ProposalFailed) (CallDecision, error) {
	return c.decisionOfOpened(ctx, e.ProposalID)
}

func (c Calls) FetchExpired(ctx context.Context, e events.ProposalExpired) (CallDecision, error) {
	return c.decisionOfOpened(ctx, e.ProposalID)
}

func (c Calls) decisionOfOpened(ctx context.Context, proposal uuid.UUID) (CallDecision, error) {
	asset, err := sqlc.New(c.DB).CallAssetOfProposal(ctx, proposal)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return CallDecision{}, errs.Wrap(err, errs.CodeOf(err), "ranking.Calls.decisionOfOpened")
	}
	return c.decision(ctx, proposal, asset)
}

func (c Calls) decision(ctx context.Context, proposal uuid.UUID, asset pgtype.UUID) (CallDecision, error) {
	ballots, err := c.ballots(ctx, proposal)
	if err != nil {
		return CallDecision{}, err
	}
	out := CallDecision{assetID: asset, ballots: ballots}
	if !asset.Valid {
		return out, nil
	}
	id := app.AssetIDFrom(asset.Bytes)
	prices, err := c.Market.PricesAsOf(ctx, []app.AssetID{id}, c.Clock.Now())
	if err != nil {
		return CallDecision{}, err
	}
	if price, ok := prices[id]; ok {
		out.ref = &price
	}
	return out, nil
}

func (Calls) ApplyPassed(ctx context.Context, tx db.Tx, e events.ProposalPassed, f CallDecision, at time.Time) error {
	if f.skip {
		return nil
	}
	return applyDecision(ctx, tx, decided{
		proposal: e.ProposalID, cabal: e.CabalID, outcome: "passed", side: domain.SideYes,
		kind: text(e.Kind), symbol: text(e.Symbol), mint: text(string(e.Mint)),
	}, f, at)
}

func (Calls) ApplyFailed(ctx context.Context, tx db.Tx, e events.ProposalFailed, f CallDecision, at time.Time) error {
	return applyDecision(ctx, tx, decided{
		proposal: e.ProposalID, cabal: e.CabalID, outcome: "failed", side: domain.SideNo,
	}, f, at)
}

func (Calls) ApplyExpired(ctx context.Context, tx db.Tx, e events.ProposalExpired, f CallDecision, at time.Time) error {
	return applyDecision(ctx, tx, decided{
		proposal: e.ProposalID, cabal: e.CabalID, outcome: "expired", side: domain.SideNo,
	}, f, at)
}

type decided struct {
	proposal, cabal    uuid.UUID
	outcome, side      string
	kind, symbol, mint pgtype.Text
}

func applyDecision(ctx context.Context, tx db.Tx, d decided, f CallDecision, at time.Time) error {
	q := sqlc.New(tx.Queries())
	if err := upsertBallots(ctx, q, d.proposal, d.cabal, f.ballots); err != nil {
		return err
	}
	params := sqlc.RecordCallDecisionParams{
		ProposalID: d.proposal, CabalID: d.cabal, Kind: d.kind, Symbol: d.symbol, Mint: d.mint,
		AssetID: f.assetID, DecidedAt: at, Outcome: d.outcome, At: at,
	}
	if f.ref != nil {
		micros, err := int8Of(f.ref.Micros)
		if err != nil {
			return err
		}
		params.MarketRefMicros, params.MarketRefObservedAt = micros, timestamp(f.ref.ObservedAt)
	}
	if _, err := q.RecordCallDecision(ctx, params); err != nil {
		return err
	}
	return q.UpsertCabalCall(ctx, sqlc.UpsertCabalCallParams{ProposalID: d.proposal, CabalID: d.cabal, Side: d.side})
}

func (c Calls) FetchTrade(ctx context.Context, e events.TradeConfirmed) (CallFill, error) {
	mint, ok := fillMint(e)
	if !ok {
		return CallFill{skip: true}, nil
	}
	assets, err := c.Market.AssetsByMints(ctx, []string{mint})
	if err != nil {
		return CallFill{}, err
	}
	if len(assets) == 0 {
		return CallFill{skip: true}, nil
	}
	price, err := domain.FillPrice(e, assets[0].Decimals)
	if err != nil {
		return CallFill{}, err
	}
	micros, err := int8Of(price)
	if err != nil {
		return CallFill{}, err
	}
	return CallFill{price: micros}, nil
}

func fillMint(e events.TradeConfirmed) (string, bool) {
	if e.Source.Kind != tradeSourceProposal {
		return "", false
	}
	switch e.Action {
	case domain.CallBuy:
		return string(e.OutMint), true
	case domain.CallSell:
		return string(e.InMint), true
	}
	return "", false
}

func (Calls) ApplyTrade(ctx context.Context, tx db.Tx, e events.TradeConfirmed, f CallFill, at time.Time) error {
	if f.skip {
		return nil
	}
	return sqlc.New(tx.Queries()).RecordCallFill(ctx, sqlc.RecordCallFillParams{
		ProposalID: e.Source.ID, CabalID: e.CabalID, FillPriceMicros: f.price.Int64,
		FilledAt: e.ConfirmedAt, At: at,
	})
}

func (Calls) Withdrawn(ctx context.Context, tx db.Tx, e events.ProposalWithdrawn, at time.Time) error {
	return voidCall(ctx, tx, e.ProposalID, e.CabalID, "withdrawn", at)
}

func (Calls) Voided(ctx context.Context, tx db.Tx, e events.ProposalVoided, at time.Time) error {
	reason := e.Reason
	if reason == "" {
		reason = "voided"
	}
	return voidCall(ctx, tx, e.ProposalID, e.CabalID, reason, at)
}

func voidCall(ctx context.Context, tx db.Tx, proposal, cabal uuid.UUID, reason string, at time.Time) error {
	return sqlc.New(tx.Queries()).VoidCall(ctx, sqlc.VoidCallParams{
		ProposalID: proposal, CabalID: cabal, Reason: reason, At: at,
	})
}

func (c Calls) asset(ctx context.Context, mint string) (pgtype.UUID, error) {
	assets, err := c.Market.AssetsByMints(ctx, []string{mint})
	if err != nil || len(assets) == 0 {
		return pgtype.UUID{}, err
	}
	return pgtype.UUID{Bytes: assets[0].ID.UUID(), Valid: true}, nil
}

func (c Calls) ballots(ctx context.Context, proposal uuid.UUID) ([]ballot, error) {
	rows, err := c.Ballots.Ballots(ctx, ids.ProposalIDFrom(proposal))
	if err != nil {
		return nil, err
	}
	out := make([]ballot, len(rows))
	for i, r := range rows {
		out[i] = ballot{voter: r.VoterID.UUID()}
		if r.Choice != nil && r.CastAt != nil {
			out[i].choice, out[i].at = text(*r.Choice), timestamp(*r.CastAt)
		}
	}
	return out, nil
}

func upsertBallots(ctx context.Context, q *sqlc.Queries, proposal, cabal uuid.UUID, ballots []ballot) error {
	for _, b := range ballots {
		if err := q.UpsertCallVoter(ctx, sqlc.UpsertCallVoterParams{
			ProposalID: proposal, UserID: b.voter, CabalID: cabal, Choice: b.choice, ChoiceAt: b.at,
		}); err != nil {
			return err
		}
	}
	return nil
}

func int8Of(m money.Micros) (pgtype.Int8, error) {
	v, err := m.Delta(money.Micros{})
	if err != nil {
		return pgtype.Int8{}, errs.Wrap(err, errs.CodeInvalidInput, "ranking.Calls.int8Of")
	}
	return pgtype.Int8{Int64: v.Int64(), Valid: true}, nil
}

func text(s string) pgtype.Text { return pgtype.Text{String: s, Valid: true} }

func timestamp(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }

type CallStore struct{ q *sqlc.Queries }

var _ app.CallStore = CallStore{}

func NewCallStore(db dbsqlc.DBTX) CallStore { return CallStore{q: sqlc.New(db)} }

func (s CallStore) UserRecords(
	ctx context.Context, users []ids.UserID, cabal *ids.CabalID, p port.Period,
) (map[ids.UserID]port.CallRecord, error) {
	rows, err := s.q.UserCallRecords(ctx, sqlc.UserCallRecordsParams{
		UserIds: userUUIDs(users), CabalID: cabalArg(cabal), WindowStart: p.Start, WindowEnd: p.End,
	})
	if err != nil {
		return nil, readFailed(err, "UserRecords")
	}
	out := make(map[ids.UserID]port.CallRecord, len(rows))
	for _, r := range rows {
		out[ids.UserIDFrom(r.UserID)] = port.CallRecord{
			Right: int(r.RightCalls), Resolved: int(r.ResolvedCalls),
			Voted: int(r.VotedCalls), Eligible: int(r.EligibleCalls),
		}
	}
	return out, nil
}

func (s CallStore) PendingSymbols(
	ctx context.Context, users []ids.UserID, cabal *ids.CabalID, p port.Period,
) (map[ids.UserID][]string, error) {
	rows, err := s.q.UserPendingCallSymbols(ctx, sqlc.UserPendingCallSymbolsParams{
		UserIds: userUUIDs(users), CabalID: cabalArg(cabal), WindowStart: p.Start, WindowEnd: p.End,
	})
	if err != nil {
		return nil, readFailed(err, "PendingSymbols")
	}
	out := map[ids.UserID][]string{}
	for _, r := range rows {
		id := ids.UserIDFrom(r.UserID)
		if !slices.Contains(out[id], r.Symbol.String) {
			out[id] = append(out[id], r.Symbol.String)
		}
	}
	return out, nil
}

func (s CallStore) CabalRecords(
	ctx context.Context, cabals []ids.CabalID, p port.Period,
) (map[ids.CabalID]port.CallRecord, error) {
	args := make([]uuid.UUID, len(cabals))
	out := make(map[ids.CabalID]port.CallRecord, len(cabals))
	for i, id := range cabals {
		args[i] = id.UUID()
		out[id] = port.CallRecord{}
	}
	rows, err := s.q.CabalCallRecords(ctx, sqlc.CabalCallRecordsParams{
		CabalIds: args, WindowStart: p.Start, WindowEnd: p.End,
	})
	if err != nil {
		return nil, readFailed(err, "CabalRecords")
	}
	for _, r := range rows {
		out[ids.CabalIDFrom(r.CabalID)] = port.CallRecord{Right: int(r.RightCalls), Resolved: int(r.ResolvedCalls)}
	}
	return out, nil
}

func userUUIDs(users []ids.UserID) []uuid.UUID {
	out := make([]uuid.UUID, len(users))
	for i, id := range users {
		out[i] = id.UUID()
	}
	return out
}

func cabalArg(cabal *ids.CabalID) pgtype.UUID {
	if cabal == nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: cabal.UUID(), Valid: true}
}

func readFailed(err error, op string) error {
	return errs.Wrap(err, errs.CodeOf(err), "ranking.CallStore."+op)
}
