package app

import (
	"cmp"
	"context"
	"errors"
	"log/slog"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/relayer"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

type SigningWallets interface {
	SigningWallet(context.Context, ids.UserID) (chain.Wallet, error)
}

type Transfers interface {
	Build(context.Context, relayer.TransferSpec) (relayer.SignedTx, error)
	Broadcast(context.Context, relayer.SignedTx) error
}

type AccountProbe interface {
	HasTokenAccount(context.Context, chain.SolanaAddress, chain.Mint) (bool, error)
}

const WithdrawOperation = "funding.withdraw"

const (
	FreshDestinationsPerDay = 5
	freshDestinationWindow  = 24 * time.Hour
)

type Withdraw struct {
	ID      uuid.UUID
	UserID  ids.UserID
	Request domain.WithdrawalRequest
	Intent  db.IntentKey

	SessionID     string
	Authorization uuid.UUID
}

type WithdrawResult struct {
	ID          uuid.UUID
	Status      domain.WithdrawalStatus
	TxSignature chain.Signature
}

type WithdrawDeps struct {
	UoW        *db.UnitOfWork
	Balances   port.Balances
	Wallets    SigningWallets
	Treasuries TreasuryWallets
	Transfers  func() (Transfers, error)
	Actions    multichain.WalletActions
	TopUp      SOLTopUp
	Hints      HintPublisher
	Clock      clock.Clock
	USDC       chain.Mint

	FreshDestinationsPerDay int
	Sponsor                 relayer.SponsorBudgets
}

type WithdrawHandler struct{ d WithdrawDeps }

func NewWithdrawHandler(d WithdrawDeps) *WithdrawHandler { return &WithdrawHandler{d: d} }

func (h *WithdrawHandler) Handle(ctx context.Context, cmd Withdraw) (WithdrawResult, error) {
	const op = "funding.Withdraw.Handle"
	wallet, err := h.d.Wallets.SigningWallet(ctx, cmd.UserID)
	if err != nil {
		return WithdrawResult{}, err
	}
	if cmd.Request.Kind() == domain.DestinationEVM {
		return h.handleEVM(ctx, cmd, wallet)
	}
	if wallet.Address == cmd.Request.To {
		return WithdrawResult{}, errs.New(errs.CodeWithdrawToOwnWallet, op)
	}
	if err := h.refuseTreasury(ctx, cmd.Request.To); err != nil {
		return WithdrawResult{}, err
	}
	transfers, err := h.d.Transfers()
	if err != nil {
		return WithdrawResult{}, err
	}
	prior, plan, replayed, err := h.create(ctx, cmd, h.destinationLacksAccount(ctx, transfers, cmd.Request.To))
	if err != nil {
		return WithdrawResult{}, err
	}
	if replayed {
		if domain.WithdrawalStatus(prior.Status) != domain.WithdrawalCreated {
			return replayResult(prior), nil
		}
		cmd.ID = prior.ID
	}
	signed, err := transfers.Build(ctx, relayer.TransferSpec{
		FromWallet: wallet, To: cmd.Request.To, Mint: h.d.USDC,
		Amount: money.NewBaseUnits(cmd.Request.Amount.Uint64(), h.d.USDC.Decimals), SkipCreate: !plan.CreateAccount,
	})
	if err != nil {
		return WithdrawResult{}, h.failUnsent(ctx, cmd, err)
	}
	faultpoint.Hit(ctx, faultpoint.AfterSign)
	if err := h.submit(ctx, cmd, signed); err != nil {
		return WithdrawResult{}, err
	}
	if err := transfers.Broadcast(ctx, signed); err != nil {
		observability.Degraded(ctx, observability.FundingWithdrawalBroadcastFailed,
			slog.String("withdrawal_id", cmd.ID.String()), slog.String("code", string(errs.CodeOf(err))))
	}
	faultpoint.Hit(ctx, faultpoint.AfterBroadcast)
	return WithdrawResult{ID: cmd.ID, Status: domain.WithdrawalSubmitted, TxSignature: signed.Signature}, nil
}

func (h *WithdrawHandler) destinationLacksAccount(
	ctx context.Context,
	transfers Transfers,
	to chain.SolanaAddress,
) bool {
	probe, ok := transfers.(AccountProbe)
	if !ok {
		return false
	}
	has, err := probe.HasTokenAccount(ctx, to, h.d.USDC)
	return err == nil && !has
}

func (h *WithdrawHandler) refuseTreasury(ctx context.Context, to chain.SolanaAddress) error {
	const op = "funding.Withdraw.refuseTreasury"
	treasuries, err := h.d.Treasuries.TreasuryWallets(ctx)
	if err != nil {
		return err
	}
	for _, t := range treasuries {
		if t.Address == to {
			return errs.New(errs.CodeInvalidAddress, op,
				slog.String("reason", "cabal_treasury"), slog.String("cabal_id", t.CabalID.String()))
		}
	}
	return nil
}

func replayResult(row sqlc.GetWithdrawalRow) WithdrawResult {
	return WithdrawResult{
		ID: row.ID, Status: domain.WithdrawalStatus(row.Status), TxSignature: chain.Signature(row.TxSignature),
	}
}

func (h *WithdrawHandler) create(
	ctx context.Context, cmd Withdraw, lacksAccount bool,
) (prior sqlc.GetWithdrawalRow, plan relayer.SponsorPlan, replayed bool, err error) {
	obs, observeErr := h.d.Balances.Observe(ctx, cmd.UserID)
	err = h.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		prior, plan, replayed = sqlc.GetWithdrawalRow{}, relayer.SponsorPlan{}, false
		if err := sqlc.New(tx.Queries()).LockWalletOutflow(ctx, cmd.UserID.UUID()); err != nil {
			return err
		}
		bound, again, err := cmd.Intent.Bind(ctx, tx.Queries(), cmd.ID, h.d.Clock.Now())
		if err != nil {
			return err
		}
		if again {
			replayed = true
			prior, plan, err = h.resume(ctx, tx, cmd, bound)
			return err
		}
		plan, err = h.open(ctx, tx, cmd, obs, observeErr, lacksAccount)
		return err
	})
	return prior, plan, replayed, err
}

func (h *WithdrawHandler) resume(
	ctx context.Context, tx db.Tx, cmd Withdraw, bound uuid.UUID,
) (prior sqlc.GetWithdrawalRow, plan relayer.SponsorPlan, err error) {
	prior, err = GetWithdrawal(ctx, tx.Queries(), bound, cmd.UserID)
	if err != nil || domain.WithdrawalStatus(prior.Status) != domain.WithdrawalCreated {
		return prior, plan, err
	}
	plan, err = h.reserve(ctx, tx, cmd, prior.ID, true)
	return prior, plan, err
}

func (h *WithdrawHandler) open(
	ctx context.Context, tx db.Tx, cmd Withdraw, obs port.Observation, observeErr error, lacksAccount bool,
) (relayer.SponsorPlan, error) {
	q := sqlc.New(tx.Queries())
	if err := h.requireStepUp(ctx, q, cmd); err != nil {
		return relayer.SponsorPlan{}, err
	}
	landed, err := h.admit(ctx, tx, cmd, obs, observeErr)
	if err != nil {
		return relayer.SponsorPlan{}, err
	}
	if err := q.InsertWithdrawal(ctx, insertParams(cmd, h.d.Clock.Now())); err != nil {
		return relayer.SponsorPlan{}, err
	}
	if err := h.consumeAuthorization(ctx, q, cmd); err != nil {
		return relayer.SponsorPlan{}, err
	}
	plan, err := h.reserve(ctx, tx, cmd, cmd.ID, !landed || lacksAccount)
	if err != nil {
		return relayer.SponsorPlan{}, err
	}
	h.afterCommit(tx, cmd, "", domain.WithdrawalCreated)
	return plan, nil
}

func (h *WithdrawHandler) reserve(
	ctx context.Context, tx db.Tx, cmd Withdraw, ref uuid.UUID, canCreate bool,
) (relayer.SponsorPlan, error) {
	if cmd.Request.Kind() == domain.DestinationEVM {
		return relayer.SponsorPlan{}, nil
	}
	return h.d.Sponsor.Reserve(ctx, tx.Queries(), relayer.SponsorRequest{
		Ref: ref, User: cmd.UserID, Destination: cmd.Request.To, CanCreate: canCreate, Now: h.d.Clock.Now(),
	})
}

func (h *WithdrawHandler) admit(
	ctx context.Context, tx db.Tx, cmd Withdraw, obs port.Observation, observeErr error,
) (landed bool, err error) {
	const op = "funding.Withdraw.admit"
	if landed, err = h.refuseFreshDestinationFlood(ctx, sqlc.New(tx.Queries()), cmd); err != nil {
		return false, err
	}
	if observeErr != nil {
		return false, observeErr
	}
	balance, err := h.d.Balances.AvailableIn(ctx, tx.Queries(), obs)
	if err != nil {
		return false, err
	}
	if balance.AvailableMicros.Cmp(cmd.Request.Amount) < 0 {
		return false, errs.New(errs.CodeInsufficientFunds, op, slog.String("available_micros",
			balance.AvailableMicros.String()), slog.String("amount_micros", cmd.Request.Amount.String()))
	}
	return landed, nil
}

func (h *WithdrawHandler) refuseFreshDestinationFlood(
	ctx context.Context, q *sqlc.Queries, cmd Withdraw,
) (landed bool, err error) {
	const op = "funding.Withdraw.refuseFreshDestinationFlood"
	limit := cmp.Or(h.d.FreshDestinationsPerDay, FreshDestinationsPerDay)
	seen, err := q.FreshWithdrawalDestinations(ctx, sqlc.FreshWithdrawalDestinationsParams{
		UserID: cmd.UserID.UUID(), ToAddress: string(cmd.Request.To),
		Since: h.d.Clock.Now().Add(-freshDestinationWindow),
	})
	if err != nil {
		return false, err
	}
	if !seen.DestinationKnown && seen.FreshToday >= int64(limit) {
		return false, errs.New(errs.CodeRateLimited, op, slog.String("reason", "fresh_destinations"),
			slog.String("user_id", cmd.UserID.String()), slog.Int64("fresh_today", seen.FreshToday))
	}
	return seen.DestinationLanded, nil
}

func insertParams(cmd Withdraw, now time.Time) sqlc.InsertWithdrawalParams {
	r := cmd.Request
	params := sqlc.InsertWithdrawalParams{
		ID: cmd.ID, UserID: cmd.UserID.UUID(), ToAddress: r.Address(), CreatedAt: now,
		AmountMicros: r.Amount.String(), DestinationKind: string(r.Kind()),
		Network: string(cmp.Or(r.Network, chain.NetworkSolana)),
	}
	if r.Kind() == domain.DestinationEVM {
		params.EvmAddress = r.EVM.String()
	}
	return params
}

func (h *WithdrawHandler) failUnsent(ctx context.Context, cmd Withdraw, cause error) error {
	code := errs.CodeOf(cause)
	err := h.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		moved, err := sqlc.New(tx.Queries()).FailUnsentWithdrawal(ctx, sqlc.FailUnsentWithdrawalParams{
			ID: cmd.ID, FailCode: string(code), CompletedAt: h.d.Clock.Now(),
		})
		if err != nil || moved == 0 {
			return err
		}
		if err := h.d.Sponsor.Release(ctx, tx.Queries(), cmd.ID, h.d.Clock.Now()); err != nil {
			return err
		}
		err = db.UnbindIntent(ctx, tx.Queries(), WithdrawOperation, cmd.ID)
		if err == nil {
			h.afterCommit(tx, cmd, domain.WithdrawalCreated, domain.WithdrawalFailed)
		}
		return err
	})
	if err != nil {
		return errors.Join(cause, err)
	}
	return cause
}

func (h *WithdrawHandler) submit(ctx context.Context, cmd Withdraw, signed relayer.SignedTx) error {
	const op = "funding.Withdraw.submit"
	return h.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		moved, err := sqlc.New(tx.Queries()).SubmitWithdrawal(ctx, sqlc.SubmitWithdrawalParams{
			ID: cmd.ID, SignedTx: signed.Bytes, TxSignature: string(signed.Signature),
			LastValidBlockHeight: strconv.FormatUint(signed.LastValidBlockHeight, 10), SubmittedAt: h.d.Clock.Now(),
		})
		if err != nil {
			return err
		}
		if moved == 0 {
			return errs.New(errs.CodeInternal, op, slog.String("withdrawal_id", cmd.ID.String()),
				slog.String("reason", "withdrawal left created before submit"))
		}
		if err := tx.Events.Append(ctx, events.WithdrawalSubmitted{
			V: 1, WithdrawalID: cmd.ID, UserID: cmd.UserID.UUID(), AmountMicros: cmd.Request.Amount,
			ToAddress: cmd.Request.To, TxSignature: signed.Signature,
		}); err != nil {
			return err
		}
		h.afterCommit(tx, cmd, domain.WithdrawalCreated, domain.WithdrawalSubmitted)
		return nil
	})
}

func (h *WithdrawHandler) afterCommit(tx db.Tx, cmd Withdraw, from, to domain.WithdrawalStatus) {
	tx.AfterCommit(func(ctx context.Context) {
		h.d.Hints.PublishHint(ctx, events.UserBalanceChangedHint(cmd.UserID), nil)
		observability.Info(ctx, observability.FundingWithdrawalMoved,
			slog.String("withdrawal_id", cmd.ID.String()), slog.String("user_id", cmd.UserID.String()),
			slog.String("amount_micros", cmd.Request.Amount.String()),
			slog.String("status_before", string(from)), slog.String("status_after", string(to)))
	})
}
