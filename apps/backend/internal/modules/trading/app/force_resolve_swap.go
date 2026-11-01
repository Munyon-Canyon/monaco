package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

const forceResolveSwapOp = "trading.ForceResolveSwap"

const (
	actionForceResolve = "force_resolve"
	actorOperatorTag   = "operator:"
)

type ForceResolveSwap struct {
	Signature      chain.Signature
	To             domain.Status
	OutAmount      money.BaseUnits
	Reason         string
	Actor          string
	IdempotencyKey string
	Override       bool
}

type ForceResolveSwapHandler struct {
	uow   *db.UnitOfWork
	reads sqlc.DBTX
	clock clock.Clock
	ids   ids.Generator
	hints Hints
	chain ChainReader

	receipts RoutedReceiptReader
}

func NewForceResolveSwapHandler(
	uow *db.UnitOfWork,
	reads sqlc.DBTX,
	c clock.Clock,
	g ids.Generator,
	hints Hints,
	reader ChainReader,
	receipts RoutedReceiptReader,
) *ForceResolveSwapHandler {
	return &ForceResolveSwapHandler{
		uow: uow, reads: reads, clock: c, ids: g, hints: hints, chain: reader, receipts: receipts,
	}
}

func (h *ForceResolveSwapHandler) Handle(ctx context.Context, cmd ForceResolveSwap) (SwapView, error) {
	if err := cmd.validate(); err != nil {
		return SwapView{}, err
	}
	ctx = observability.WithActor(ctx, "system:"+cmd.Actor)
	facts, err := h.verify(ctx, cmd)
	if err != nil {
		return SwapView{}, err
	}
	return h.resolve(ctx, cmd, facts)
}

type chainFacts struct {
	State         string  `json:"chain_state"`
	ChainOut      *uint64 `json:"chain_out_amount,omitempty"`
	Contradiction string  `json:"contradiction,omitempty"`
	Unverified    string  `json:"unverified,omitempty"`
}

func (h *ForceResolveSwapHandler) verify(ctx context.Context, cmd ForceResolveSwap) (chainFacts, error) {
	row, err := sqlc.New(h.reads).SwapForForceResolve(ctx, string(cmd.Signature))
	if err != nil {
		return chainFacts{}, forceResolveFound(err)
	}
	if row.Status != string(domain.StatusSubmitted) {
		return chainFacts{State: "skipped"}, nil
	}
	if row.Routed && !row.ReceiptMissing {
		return chainFacts{State: "skipped"}, nil
	}
	facts, err := h.facts(ctx, cmd, row)
	switch {
	case err == nil && facts.Contradiction != "" && !cmd.Override:
		return chainFacts{}, fmt.Errorf(
			"%w: the chain contradicts --to %s: %s; rerun with --override to book it anyway",
			errs.New(errs.CodeInvalidInput, forceResolveSwapOp), cmd.To, facts.Contradiction,
		)
	case err != nil && cmd.Override:
		return chainFacts{State: "unknown", Unverified: err.Error()}, nil
	}
	return facts, err
}

func (h *ForceResolveSwapHandler) facts(
	ctx context.Context, cmd ForceResolveSwap, row sqlc.SwapForForceResolveRow,
) (chainFacts, error) {
	if !row.Routed {
		return h.chainFacts(ctx, cmd, row)
	}
	if h.receipts == nil {
		return chainFacts{}, notImplemented("RoutedReceiptReader", "#7557")
	}
	fill, err := h.receipts.Receipt(ctx, ids.SwapIDFrom(row.ID))
	if err != nil {
		return chainFacts{}, err
	}
	return h.receiptFacts(cmd, fill), nil
}

func (h *ForceResolveSwapHandler) receiptFacts(cmd ForceResolveSwap, fill EVMFill) chainFacts {
	if fill.State == FillReverted {
		return h.verdict(cmd, chainFacts{State: "failed"}, "its receipt shows it reverted", domain.StatusFailed)
	}
	if fill.State != FillSucceeded {
		return chainFacts{State: "not_found"}
	}
	out := fill.Out
	facts := chainFacts{State: "landed", ChainOut: &out}
	switch {
	case cmd.To == domain.StatusFailed:
		facts.Contradiction = "its receipt shows it landed successfully"
	case out != cmd.OutAmount.Uint64():
		facts.Contradiction = fmt.Sprintf("the treasury received %d, not %d", out, cmd.OutAmount.Uint64())
	}
	return facts
}

func (h *ForceResolveSwapHandler) chainFacts(
	ctx context.Context, cmd ForceResolveSwap, row sqlc.SwapForForceResolveRow,
) (chainFacts, error) {
	if chain.IsStandInSignature(cmd.Signature) {
		return chainFacts{
			State: "unknown", Contradiction: "its real signature is not recorded, so the chain cannot confirm it",
		}, nil
	}
	statuses, err := h.chain.SignatureStatuses(ctx, []chain.Signature{cmd.Signature})
	if err != nil {
		return chainFacts{}, err
	}
	if len(statuses) != 1 {
		return chainFacts{}, errs.New(errs.CodeDecodeFailed, forceResolveSwapOp, slog.Int("statuses", len(statuses)))
	}
	status := statuses[0]
	switch {
	case status.State == SigFinalized && status.Failed:
		return h.verdict(cmd, chainFacts{State: "failed"}, "it failed on chain", domain.StatusFailed), nil
	case status.State == SigFinalized:
		return h.landed(ctx, cmd, row)
	case status.State == SigProcessing:
		return h.verdict(cmd, chainFacts{State: "processing"}, "it is still processing and may yet land", ""), nil
	}
	valid, err := h.chain.BlockhashValid(ctx, row.SignedTx)
	if err != nil {
		return chainFacts{}, err
	}
	facts := chainFacts{State: "not_found"}
	if valid {
		return h.verdict(cmd, facts, "it was not found but its blockhash is still valid, so it may yet land", ""), nil
	}
	return h.verdict(cmd, facts, "it was not found and its blockhash expired", domain.StatusFailed), nil
}

func (h *ForceResolveSwapHandler) verdict(
	cmd ForceResolveSwap, facts chainFacts, what string, agrees domain.Status,
) chainFacts {
	if cmd.To != agrees {
		facts.Contradiction = what
	}
	return facts
}

func (h *ForceResolveSwapHandler) landed(
	ctx context.Context, cmd ForceResolveSwap, row sqlc.SwapForForceResolveRow,
) (chainFacts, error) {
	facts := chainFacts{State: "landed"}
	if cmd.To == domain.StatusFailed {
		facts.Contradiction = "it landed successfully on chain"
		return facts, nil
	}
	mint, err := confirmedOutMint(row, cmd)
	if err != nil {
		return chainFacts{}, err
	}
	got, err := h.chain.InboundAmount(ctx, cmd.Signature, chain.SolanaAddress(row.TreasuryAddress), mint)
	if err != nil {
		return chainFacts{}, err
	}
	out := got.Uint64()
	facts.ChainOut = &out
	if out != cmd.OutAmount.Uint64() {
		facts.Contradiction = fmt.Sprintf("the treasury received %d, not %d", out, cmd.OutAmount.Uint64())
	}
	return facts, nil
}

func (h *ForceResolveSwapHandler) resolve(
	ctx context.Context, cmd ForceResolveSwap, facts chainFacts,
) (SwapView, error) {
	var resolved SwapView
	err := h.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		var err error
		resolved, err = h.resolveTx(ctx, tx, cmd, facts)
		return err
	})
	if err != nil {
		return SwapView{}, err
	}
	return resolved, nil
}

func (h *ForceResolveSwapHandler) resolveTx(
	ctx context.Context, tx db.Tx, cmd ForceResolveSwap, facts chainFacts,
) (SwapView, error) {
	row, err := sqlc.New(tx.Queries()).SwapForForceResolve(ctx, string(cmd.Signature))
	if err != nil {
		return SwapView{}, forceResolveFound(err)
	}
	if err := h.gate(ctx, sqlc.New(tx.Queries()), row, cmd.To); err != nil {
		return SwapView{}, err
	}
	req, err := sweepRequest(
		row.CabalID,
		row.SourceKind,
		row.SourceID,
		row.Action,
		row.Symbol,
		row.InMint,
		row.InAmount,
		row.SourceBatchSize,
	)
	if err != nil {
		return SwapView{}, err
	}
	if cmd.To == domain.StatusConfirmed {
		if req.OutMint, err = confirmedOutMint(row, cmd); err != nil {
			return SwapView{}, err
		}
	}
	s := cmd.step(req, row.ID)
	at := h.clock.Now()
	n, err := s.apply(ctx, sqlc.New(tx.Queries()), at)
	if err != nil {
		return SwapView{}, err
	}
	if n == 0 {
		return SwapView{}, errs.New(errs.CodeSwapNotStuck, forceResolveSwapOp)
	}
	if err := tx.Events.Append(ctx, s.event(at)); err != nil {
		return SwapView{}, err
	}
	if err := h.record(ctx, sqlc.New(tx.Queries()), row.ID, cmd, facts, at); err != nil {
		return SwapView{}, err
	}
	resolved, err := NewQueries(tx.Queries()).SwapBySignature(ctx, cmd.Signature)
	if err != nil {
		return SwapView{}, err
	}
	tx.AfterCommit(func(ctx context.Context) { h.committed(ctx, req, row.ID, cmd, s) })
	return resolved, nil
}

func (h *ForceResolveSwapHandler) record(
	ctx context.Context, q *sqlc.Queries, id uuid.UUID, cmd ForceResolveSwap, facts chainFacts, at time.Time,
) error {
	detail, err := json.Marshal(struct {
		To        domain.Status `json:"to"`
		OutAmount uint64        `json:"out_amount"`
		Override  bool          `json:"override"`
		chainFacts
	}{cmd.To, cmd.OutAmount.Uint64(), cmd.Override, facts})
	return errors.Join(err, q.InsertSwapOperatorAction(ctx, sqlc.InsertSwapOperatorActionParams{
		ID: h.ids.NewV7(), SwapID: id, Action: actionForceResolve, Actor: cmd.Actor,
		Reason: cmd.Reason, Detail: detail, CreatedAt: at,
	}))
}

func confirmedOutMint(row sqlc.SwapForForceResolveRow, cmd ForceResolveSwap) (chain.Mint, error) {
	if row.OutDecimals < 0 || row.OutDecimals > 255 {
		return chain.Mint{}, errs.New(errs.CodeDecodeFailed, forceResolveSwapOp)
	}
	if int16(cmd.OutAmount.Decimals()) != row.OutDecimals {
		return chain.Mint{}, errs.New(errs.CodeInvalidInput, forceResolveSwapOp)
	}
	return chain.Mint{Address: chain.SolanaAddress(row.OutMint), Decimals: uint8(row.OutDecimals)}, nil
}

func validOperator(actor string) bool {
	return strings.HasPrefix(actor, actorOperatorTag) && len(actor) > len(actorOperatorTag)
}

func (cmd ForceResolveSwap) validate() error {
	if cmd.Signature == "" || cmd.IdempotencyKey == "" || !validOperator(cmd.Actor) ||
		utf8.RuneCountInString(cmd.Reason) < 1 || utf8.RuneCountInString(cmd.Reason) > 500 {
		return errs.New(errs.CodeInvalidInput, forceResolveSwapOp)
	}
	switch cmd.To {
	case domain.StatusConfirmed:
		if cmd.OutAmount.IsZero() {
			return errs.New(errs.CodeInvalidInput, forceResolveSwapOp)
		}
		if _, ok := domain.Column(cmd.OutAmount.Uint64()); !ok {
			return errs.New(errs.CodeInvalidInput, forceResolveSwapOp)
		}
	case domain.StatusFailed:
	case domain.StatusCreated, domain.StatusSubmitted:
		return errs.New(errs.CodeInvalidInput, forceResolveSwapOp)
	default:
		return errs.New(errs.CodeInvalidInput, forceResolveSwapOp)
	}
	return nil
}

func (cmd ForceResolveSwap) step(req SwapRequest, id uuid.UUID) step {
	if cmd.To == domain.StatusConfirmed {
		out, _ := domain.Column(cmd.OutAmount.Uint64())
		return confirmed(req, id, cmd.Signature, cmd.OutAmount.Uint64(), out)
	}
	return failed(req, id, domain.FailureForceResolved, "")
}

func forceResolvable(row sqlc.SwapForForceResolveRow) error {
	if row.Status != string(domain.StatusSubmitted) || row.Routed && !row.ReceiptMissing {
		return errs.New(errs.CodeSwapNotStuck, forceResolveSwapOp, slog.Bool("routed", row.Routed))
	}
	return nil
}

func forceResolveFound(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return errs.New(errs.CodeSwapNotFound, forceResolveSwapOp)
	}
	return errs.Wrap(err, errs.CodeInternal, forceResolveSwapOp)
}

func (h *ForceResolveSwapHandler) committed(
	ctx context.Context,
	req SwapRequest,
	id uuid.UUID,
	cmd ForceResolveSwap,
	s step,
) {
	h.hints.PublishHint(
		ctx,
		"cabal."+req.CabalID.UUID().String()+".swap_updated",
		fmt.Appendf(nil, `{"swap_id":%q}`, id),
	)
	observability.Info(ctx, observability.TradingSwapForceResolved,
		slog.String("swap_id", id.String()),
		slog.String("signature", string(cmd.Signature)),
		slog.String("from", string(s.from)),
		slog.String("to", string(s.status)),
		slog.String("reason", cmd.Reason),
		slog.String("actor", cmd.Actor),
		slog.Bool("override", cmd.Override),
	)
}

func (h *ForceResolveSwapHandler) gate(
	ctx context.Context, q *sqlc.Queries, row sqlc.SwapForForceResolveRow, to domain.Status,
) error {
	if err := forceResolvable(row); err != nil || !row.Routed {
		return err
	}
	return closeSwapStep(ctx, q, row.ID, to, h.clock.Now())
}

func closeSwapStep(ctx context.Context, q *sqlc.Queries, id uuid.UUID, to domain.Status, at time.Time) error {
	params := sqlc.CloseSwapStepParams{TradeID: id, Status: string(domain.StepSucceeded), At: at}
	if to == domain.StatusFailed {
		params.Status, params.Reason = string(domain.StepFailed), string(domain.FailureForceResolved)
	}
	_, err := q.CloseSwapStep(ctx, params)
	return err
}
