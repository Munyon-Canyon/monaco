package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

const (
	chooseBridgeArrivalOp = "trading.ChooseBridgeArrival"
	actionChooseArrival   = "choose_arrival"
)

var txHashPattern = regexp.MustCompile(`^0x[0-9a-f]{64}$`)

type ChooseBridgeArrival struct {
	SwapID         ids.SwapID
	Network        chain.Network
	TxHash         string
	Actor          string
	Reason         string
	IdempotencyKey string
}

type BridgeArrivalChoice struct {
	SwapID  ids.SwapID
	CabalID ids.CabalID
	Network chain.Network
	TxHash  string
	Amount  money.Micros
	Fee     money.Micros
	Changed bool
}

type EVMTransfers interface {
	Transfer(
		ctx context.Context, network chain.Network, token, owner chain.EVMAddress, txHash string,
	) (EVMArrival, bool, error)
}

type ChooseBridgeArrivalHandler struct {
	uow       *db.UnitOfWork
	reads     sqlc.DBTX
	clock     clock.Clock
	ids       ids.Generator
	wallets   TreasuryEVM
	transfers EVMTransfers
}

func NewChooseBridgeArrivalHandler(
	uow *db.UnitOfWork, reads sqlc.DBTX, c clock.Clock, g ids.Generator, wallets TreasuryEVM, transfers EVMTransfers,
) *ChooseBridgeArrivalHandler {
	return &ChooseBridgeArrivalHandler{
		uow: uow, reads: reads, clock: c, ids: g, wallets: wallets, transfers: transfers,
	}
}

func (cmd ChooseBridgeArrival) validate() (ChooseBridgeArrival, error) {
	cmd.TxHash = strings.ToLower(cmd.TxHash)
	n := utf8.RuneCountInString(cmd.Reason)
	if cmd.Actor == "" || cmd.IdempotencyKey == "" || n < 1 || n > 500 || !cmd.Network.IsEVM() ||
		!txHashPattern.MatchString(cmd.TxHash) {
		return cmd, errs.New(errs.CodeInvalidInput, chooseBridgeArrivalOp)
	}
	return cmd, nil
}

func chooseRefusal(code errs.Code, swap ids.SwapID, reason string, attrs ...slog.Attr) error {
	return errs.New(code, chooseBridgeArrivalOp,
		append([]slog.Attr{slog.String("swap_id", swap.String()), slog.String("reason", reason)}, attrs...)...)
}

func (h *ChooseBridgeArrivalHandler) Plan(ctx context.Context, cmd ChooseBridgeArrival) (BridgeArrivalChoice, error) {
	cmd, err := cmd.validate()
	if err != nil {
		return BridgeArrivalChoice{}, err
	}
	choice, _, err := h.plan(observability.WithActor(ctx, "system:"+cmd.Actor), cmd)
	return choice, err
}

func (h *ChooseBridgeArrivalHandler) Handle(ctx context.Context, cmd ChooseBridgeArrival) (BridgeArrivalChoice, error) {
	cmd, err := cmd.validate()
	if err != nil {
		return BridgeArrivalChoice{}, err
	}
	ctx = observability.WithActor(ctx, "system:"+cmd.Actor)
	choice, step, err := h.plan(ctx, cmd)
	if err != nil || !choice.Changed {
		return choice, err
	}
	err = h.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		var err error
		choice, err = h.book(ctx, tx, cmd, step, choice)
		return err
	})
	if err != nil {
		return BridgeArrivalChoice{}, err
	}
	if choice.Changed {
		h.committed(ctx, cmd, choice)
	}
	return choice, nil
}

func (h *ChooseBridgeArrivalHandler) plan(
	ctx context.Context, cmd ChooseBridgeArrival,
) (BridgeArrivalChoice, domain.TradeStep, error) {
	journal := Journal{UoW: h.uow, Reads: h.reads, Clock: h.clock, IDs: h.ids}
	row, err := sqlc.New(h.reads).RoutedSwap(ctx, cmd.SwapID.UUID())
	if errors.Is(err, sql.ErrNoRows) {
		return BridgeArrivalChoice{}, domain.TradeStep{}, errs.New(errs.CodeSwapNotFound, chooseBridgeArrivalOp,
			slog.String("swap_id", cmd.SwapID.String()))
	}
	var comparison multichain.Comparison
	if err == nil {
		err = json.Unmarshal(row.Comparison, &comparison)
	}
	var step domain.TradeStep
	var started bool
	if err == nil {
		step, started, err = journal.Step(ctx, cmd.SwapID, domain.StepBridgeOut)
	}
	if err != nil {
		return BridgeArrivalChoice{}, domain.TradeStep{}, errs.Wrap(err, errs.CodeInternal, chooseBridgeArrivalOp,
			slog.String("swap_id", cmd.SwapID.String()))
	}
	choice, err := h.admissible(ctx, journal, cmd, step, started, bridgeFee(comparison))
	return choice, step, err
}

func (h *ChooseBridgeArrivalHandler) admissible(
	ctx context.Context, journal Journal, cmd ChooseBridgeArrival, step domain.TradeStep, started bool,
	quotedFee money.Micros,
) (BridgeArrivalChoice, error) {
	switch {
	case !started || step.Status != domain.StepSucceeded:
		return BridgeArrivalChoice{}, chooseRefusal(errs.CodeSwapNotStuck, cmd.SwapID, "bridge_out_not_succeeded")
	case step.Network != cmd.Network:
		return BridgeArrivalChoice{}, chooseRefusal(errs.CodeInvalidInput, cmd.SwapID, "network_mismatch",
			slog.String("network", string(step.Network)))
	case step.Arrival.Observed() && step.Arrival.TxHash == cmd.TxHash:
		return choiceOf(step, step.Arrival, false), nil
	case step.Arrival.Observed():
		return BridgeArrivalChoice{}, chooseRefusal(errs.CodeSwapNotStuck, cmd.SwapID, "arrival_already_booked",
			slog.String("arrival_tx", step.Arrival.TxHash))
	}
	booked, err := journal.ArrivalTxs(ctx, cmd.Network)
	if err == nil && slices.Contains(booked, cmd.TxHash) {
		err = chooseRefusal(errs.CodeInvalidInput, cmd.SwapID, "arrival_tx_already_booked",
			slog.String("tx_hash", cmd.TxHash))
	}
	var arrival domain.Arrival
	if err == nil {
		arrival, err = h.transfer(ctx, cmd, step, quotedFee)
	}
	if err != nil {
		return BridgeArrivalChoice{}, err
	}
	return choiceOf(step, arrival, true), nil
}

func (h *ChooseBridgeArrivalHandler) transfer(
	ctx context.Context, cmd ChooseBridgeArrival, step domain.TradeStep, quotedFee money.Micros,
) (domain.Arrival, error) {
	wallet, err := h.wallets.EnsureTreasuryEVMWallet(ctx, step.CabalID)
	var token chain.EVMAddress
	if err == nil {
		token, err = chain.ParseEVMAddress(cmd.Network.Cash().Address)
	}
	var arrival domain.Arrival
	var found bool
	if err == nil {
		arrival, found, err = h.transfers.Transfer(ctx, cmd.Network, token, wallet.Address, cmd.TxHash)
	}
	if err != nil {
		return domain.Arrival{}, err
	}
	if !found {
		return domain.Arrival{}, chooseRefusal(errs.CodeInvalidInput, cmd.SwapID, "not_a_cash_transfer_to_treasury",
			slog.String("tx_hash", cmd.TxHash), slog.String("treasury", wallet.Address.String()))
	}
	floor := arrivalFloor(step.Amount, quotedFee)
	if arrival.Amount.IsZero() || arrival.Amount.Cmp(floor) < 0 || arrival.Amount.Cmp(step.Amount) > 0 {
		return domain.Arrival{}, chooseRefusal(errs.CodeInvalidInput, cmd.SwapID, "amount_out_of_range",
			slog.String("tx_hash", cmd.TxHash), slog.String("amount_micros", arrival.Amount.String()),
			slog.String("floor_micros", floor.String()), slog.String("ceiling_micros", step.Amount.String()))
	}
	return domain.Arrival{Amount: arrival.Amount, TxHash: cmd.TxHash}, nil
}

func choiceOf(step domain.TradeStep, arrival domain.Arrival, changed bool) BridgeArrivalChoice {
	fee, _ := step.Amount.Sub(arrival.Amount)
	return BridgeArrivalChoice{
		SwapID: step.SwapID, CabalID: step.CabalID, Network: step.Network, TxHash: arrival.TxHash,
		Amount: arrival.Amount, Fee: fee, Changed: changed,
	}
}

func (h *ChooseBridgeArrivalHandler) book(
	ctx context.Context, tx db.Tx, cmd ChooseBridgeArrival, planned domain.TradeStep, choice BridgeArrivalChoice,
) (BridgeArrivalChoice, error) {
	q := sqlc.New(tx.Queries())
	ref := domain.StepRef{
		SwapID: cmd.SwapID, CabalID: planned.CabalID, Step: domain.StepBridgeOut, Network: cmd.Network,
	}
	journal := Journal{UoW: h.uow, Reads: h.reads, Clock: h.clock, IDs: h.ids}
	err := q.LockStrandedCash(ctx, cmd.SwapID.UUID())
	var after domain.TradeStep
	var changed bool
	if err == nil {
		arrival := domain.Arrival{Amount: choice.Amount, TxHash: choice.TxHash}
		after, changed, err = journal.arrived(ctx, tx, ref, arrival)
	}
	switch {
	case err != nil:
		return BridgeArrivalChoice{}, err
	case changed:
		return choice, h.record(ctx, q, cmd, choice, h.clock.Now())
	case after.Arrival.TxHash == cmd.TxHash:
		choice.Changed = false
		return choice, nil
	}
	return BridgeArrivalChoice{}, chooseRefusal(errs.CodeSwapNotStuck, cmd.SwapID, "arrival_already_booked",
		slog.String("arrival_tx", after.Arrival.TxHash))
}

func (h *ChooseBridgeArrivalHandler) record(
	ctx context.Context, q *sqlc.Queries, cmd ChooseBridgeArrival, choice BridgeArrivalChoice, at time.Time,
) error {
	detail, err := json.Marshal(struct {
		Network        string `json:"network"`
		TxHash         string `json:"tx_hash"`
		ArrivedMicros  string `json:"arrived_micros"`
		FeeMicros      string `json:"fee_micros"`
		IdempotencyKey string `json:"idempotency_key"`
	}{string(choice.Network), choice.TxHash, choice.Amount.String(), choice.Fee.String(), cmd.IdempotencyKey})
	return errors.Join(err, q.InsertSwapOperatorAction(ctx, sqlc.InsertSwapOperatorActionParams{
		ID: h.ids.NewV7(), SwapID: cmd.SwapID.UUID(), Action: actionChooseArrival, Actor: cmd.Actor,
		Reason: cmd.Reason, Detail: detail, CreatedAt: at,
	}))
}

func (h *ChooseBridgeArrivalHandler) committed(ctx context.Context, cmd ChooseBridgeArrival, c BridgeArrivalChoice) {
	observability.Info(ctx, observability.TradingRoutedSwapResolved,
		slog.String("swap_id", c.SwapID.String()), slog.String("cabal_id", c.CabalID.String()),
		slog.String("action", actionChooseArrival), slog.String("actor", cmd.Actor),
		slog.String("reason", cmd.Reason), slog.String("network", string(c.Network)),
		slog.String("arrival_tx", c.TxHash), slog.String("arrived_micros", c.Amount.String()),
		slog.String("fee_micros", c.Fee.String()))
}
