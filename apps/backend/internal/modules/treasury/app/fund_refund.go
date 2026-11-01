package app

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/relayer"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

const (
	FundRefundWait = 90 * time.Second
	FundRefundPoll = 2 * time.Second
)

type FundRefundBalances interface {
	TokenBalance(ctx context.Context, owner chain.SolanaAddress, mint chain.Mint) (money.BaseUnits, error)
}

type FundRefundPot interface {
	PotValue(ctx context.Context, cabalID ids.CabalID) (money.Micros, error)
}

type FundRefundDeps struct {
	Reads     sqlc.DBTX
	UoW       *db.UnitOfWork
	IDs       ids.Generator
	Clock     clock.Clock
	Ledger    Ledger
	Pot       FundRefundPot
	Chain     FundChain
	Transfers func() (PayoutTransfers, error)
	Wallets   PayoutWallets
	Balances  FundRefundBalances
	USDC      chain.Mint
	Hints     HintPublisher
	Wait      time.Duration
	Poll      time.Duration
}

type FundRefunds struct{ d FundRefundDeps }

func NewFundRefunds(d FundRefundDeps) *FundRefunds { return &FundRefunds{d: d} }

type RefundFundTransfer struct {
	ID     uuid.UUID
	Actor  string
	Reason string
}

type FundRefundPlan struct {
	TransferID uuid.UUID
	CabalID    ids.CabalID
	UserID     ids.UserID
	Amount     money.Micros
	From       chain.SolanaAddress
	To         chain.SolanaAddress
	Treasury   money.Micros
	Signature  chain.Signature
	Refunded   bool
	wallet     chain.Wallet
}

type FundRefundState string

const (
	FundRefundDone    FundRefundState = "refunded"
	FundRefundAlready FundRefundState = "already_refunded"
	FundRefundPending FundRefundState = "pending"
)

type FundRefundResult struct {
	Plan  FundRefundPlan
	State FundRefundState
}

type fundRefundRow struct {
	id        uuid.UUID
	user      ids.UserID
	cabal     ids.CabalID
	amount    money.Micros
	status    domain.FundStatus
	signed    relayer.SignedTx
	attempted bool
}

func (r *FundRefunds) Plan(ctx context.Context, id uuid.UUID) (FundRefundPlan, error) {
	row, err := r.load(ctx, id)
	if err != nil {
		return FundRefundPlan{}, err
	}
	return r.plan(ctx, row, row.status == domain.FundRefunded)
}

func (r *FundRefunds) Refund(ctx context.Context, cmd RefundFundTransfer) (FundRefundResult, error) {
	row, err := r.load(ctx, cmd.ID)
	if err != nil {
		return FundRefundResult{}, err
	}
	if row.status == domain.FundRefunded {
		plan, err := r.plan(ctx, row, true)
		return FundRefundResult{Plan: plan, State: FundRefundAlready}, err
	}
	plan, err := r.plan(ctx, row, false)
	if err != nil {
		return FundRefundResult{}, err
	}
	if !row.attempted {
		if row, err = r.start(ctx, row, plan); err != nil {
			return FundRefundResult{}, err
		}
		plan.Signature = row.signed.Signature
	}
	r.broadcast(ctx, row)
	faultpoint.Hit(ctx, faultpoint.AfterBroadcast)
	state, err := r.confirm(ctx, row, cmd)
	return FundRefundResult{Plan: plan, State: state}, err
}

func (r *FundRefunds) plan(ctx context.Context, row fundRefundRow, refunded bool) (FundRefundPlan, error) {
	const op = "treasury.FundRefunds.plan"
	plan := FundRefundPlan{
		TransferID: row.id, CabalID: row.cabal, UserID: row.user, Amount: row.amount,
		Signature: row.signed.Signature, Refunded: refunded,
	}
	if refunded {
		return plan, nil
	}
	if row.status != domain.FundLanded {
		return plan, errs.New(errs.CodeFundRefundNotLanded, op, slog.String("transfer_id", row.id.String()),
			slog.String("status", string(row.status)))
	}
	from, err := r.d.Wallets.TreasuryWallet(ctx, row.cabal)
	if err != nil {
		return plan, err
	}
	to, err := r.d.Wallets.MemberAddress(ctx, row.user)
	if err != nil {
		return plan, err
	}
	plan.wallet, plan.From, plan.To = from, from.Address, to
	if row.attempted {
		return plan, nil
	}
	if err := r.unpriceable(ctx, row); err != nil {
		return plan, err
	}
	plan.Treasury, err = r.treasuryCovers(ctx, row, from.Address)
	return plan, err
}

func (r *FundRefunds) unpriceable(ctx context.Context, row fundRefundRow) error {
	_, err := r.d.Pot.PotValue(ctx, row.cabal)
	switch {
	case errs.CodeOf(err) == errs.CodePriceUnavailable:
		return nil
	case err != nil:
		return err
	}
	return errs.New(errs.CodeFundRefundPriceable, "treasury.FundRefunds.unpriceable",
		slog.String("transfer_id", row.id.String()), slog.String("cabal_id", row.cabal.String()))
}

func (r *FundRefunds) treasuryCovers(
	ctx context.Context, row fundRefundRow, treasury chain.SolanaAddress,
) (money.Micros, error) {
	const op = "treasury.FundRefunds.treasuryCovers"
	have, err := r.d.Balances.TokenBalance(ctx, treasury, r.d.USDC)
	if err != nil {
		return money.Micros{}, err
	}
	held := money.MicrosFromUint64(have.Uint64())
	if held.Cmp(row.amount) < 0 {
		return held, errs.New(errs.CodeFundRefundTreasuryLow, op, slog.String("transfer_id", row.id.String()),
			slog.String("held_micros", held.String()), slog.String("amount_micros", row.amount.String()))
	}
	return held, nil
}

func (r *FundRefunds) start(ctx context.Context, row fundRefundRow, plan FundRefundPlan) (fundRefundRow, error) {
	transfers, err := r.d.Transfers()
	if err != nil {
		return row, err
	}
	signed, err := transfers.Build(ctx, relayer.TransferSpec{
		FromWallet: plan.wallet, To: plan.To, Mint: r.d.USDC,
		Amount: money.NewBaseUnits(row.amount.Uint64(), r.d.USDC.Decimals), SkipCreate: true,
	})
	if err != nil {
		return row, err
	}
	faultpoint.Hit(ctx, faultpoint.AfterSign)
	if err := r.unpriceable(ctx, row); err != nil {
		return row, err
	}
	if err := r.store(ctx, row, signed); err != nil {
		return row, err
	}
	row.signed, row.attempted = signed, true
	return row, nil
}

func (r *FundRefunds) store(ctx context.Context, row fundRefundRow, signed relayer.SignedTx) error {
	err := r.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		locked := r.d.Ledger.LockCabal(ctx, tx, row.cabal)
		n, err := sqlc.New(tx.Queries()).StoreFundRefund(ctx, sqlc.StoreFundRefundParams{
			ID: row.id, SignedTx: signed.Bytes, Signature: string(signed.Signature),
			LastValidBlockHeight: strconv.FormatUint(signed.LastValidBlockHeight, 10), SubmittedAt: r.d.Clock.Now(),
		})
		if err = errors.Join(locked, err); err == nil && n == 0 {
			err = errs.New(errs.CodeFundRefundNotLanded, "treasury.FundRefunds.store",
				slog.String("transfer_id", row.id.String()), slog.String("reason", "moved while signing"))
		}
		return err
	})
	if err != nil {
		return err
	}
	observability.Info(ctx, observability.TreasuryFundRefundStored, slog.String("transfer_id", row.id.String()),
		slog.String("cabal_id", row.cabal.String()), slog.String("amount_micros", row.amount.String()),
		slog.String("status_before", string(domain.FundLanded)), slog.String("status_after", string(domain.FundLanded)))
	return nil
}

func (r *FundRefunds) broadcast(ctx context.Context, row fundRefundRow) {
	transfers, err := r.d.Transfers()
	if err == nil {
		err = transfers.Broadcast(ctx, row.signed)
	}
	if err != nil {
		observability.Degraded(ctx, observability.TreasuryFundRefundBroadcastFailed,
			slog.String("transfer_id", row.id.String()), slog.String("code", string(errs.CodeOf(err))))
	}
}

func (r *FundRefunds) confirm(ctx context.Context, row fundRefundRow, cmd RefundFundTransfer) (FundRefundState, error) {
	deadline := r.d.Clock.Now().Add(r.d.Wait)
	for {
		verdict, err := r.verdict(ctx, row)
		if err != nil {
			return FundRefundPending, err
		}
		switch verdict {
		case refundLanded:
			return r.complete(ctx, row, cmd)
		case refundRejected:
			return FundRefundPending, r.drop(ctx, row, errs.CodeFundRefundRejected)
		case refundLapsed:
			return FundRefundPending, r.drop(ctx, row, errs.CodeFundRefundExpired)
		case refundMissing:
			r.broadcast(ctx, row)
		case refundInFlight:
		}
		if !r.d.Clock.Now().Before(deadline) {
			return FundRefundPending, nil
		}
		select {
		case <-ctx.Done():
			return FundRefundPending, errs.Wrap(ctx.Err(), errs.CodeUpstreamUnavailable, "treasury.FundRefunds.confirm")
		case <-r.d.Clock.After(r.d.Poll):
		}
	}
}

type refundVerdict int

const (
	refundInFlight refundVerdict = iota
	refundLanded
	refundRejected
	refundLapsed
	refundMissing
)

func (r *FundRefunds) verdict(ctx context.Context, row fundRefundRow) (refundVerdict, error) {
	statuses, err := r.d.Chain.SignatureStatuses(ctx, []chain.Signature{row.signed.Signature})
	if err != nil {
		return refundInFlight, err
	}
	if len(statuses) != 1 {
		return refundInFlight, errs.New(errs.CodeDecodeFailed, "treasury.FundRefunds.verdict",
			slog.Int("statuses", len(statuses)))
	}
	st := statuses[0]
	switch {
	case st.State == solana.StateFinalized && st.Failed:
		return refundRejected, nil
	case st.State == solana.StateFinalized:
		return refundLanded, nil
	case st.BlockhashExpired(row.signed.LastValidBlockHeight):
		return refundLapsed, nil
	case st.State == solana.StateNotFound:
		return refundMissing, nil
	}
	return refundInFlight, nil
}

func (r *FundRefunds) complete(
	ctx context.Context, row fundRefundRow, cmd RefundFundTransfer,
) (FundRefundState, error) {
	moved := false
	err := r.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		locked := r.d.Ledger.LockCabal(ctx, tx, row.cabal)
		n, err := sqlc.New(tx.Queries()).RefundFundTransfer(ctx, sqlc.RefundFundTransferParams{
			ID: row.id, Signature: string(row.signed.Signature), RefundedAt: r.d.Clock.Now(),
		})
		if err = errors.Join(locked, err); err != nil || n == 0 {
			return err
		}
		moved = true
		if err := r.post(ctx, tx, row); err != nil {
			return err
		}
		tx.AfterCommit(func(ctx context.Context) {
			if r.d.Hints != nil {
				r.d.Hints.PublishHint(ctx, events.UserBalanceChangedHint(row.user), nil)
			}
		})
		return tx.Events.Append(ctx, events.FundRefunded{
			V: 1, TransferID: row.id, CabalID: row.cabal.UUID(), UserID: row.user.UUID(), AmountMicros: row.amount,
			TxSignature: row.signed.Signature, Actor: cmd.Actor, Reason: cmd.Reason,
		})
	})
	if err != nil {
		return FundRefundPending, err
	}
	if !moved {
		return FundRefundAlready, nil
	}
	observability.Info(
		ctx,
		observability.TreasuryFundRefunded,
		slog.String("transfer_id", row.id.String()),
		slog.String("cabal_id", row.cabal.String()),
		slog.String("amount_micros", row.amount.String()),
		slog.String(
			"status_before",
			string(domain.FundLanded),
		),
		slog.String("status_after", string(domain.FundRefunded)),
	)
	return FundRefundDone, nil
}

func (r *FundRefunds) post(ctx context.Context, tx db.Tx, row fundRefundRow) error {
	txn, err := domain.NewFundRefundTxn(domain.UserTxnHeader{
		ID: r.d.IDs.NewV7(), UserID: row.user, TransferID: row.id, TxSignature: row.signed.Signature,
	}, domain.MintAsset(r.d.USDC.Address), row.amount)
	if err != nil {
		return err
	}
	return r.d.Ledger.PostUserTxn(ctx, tx, txn)
}

func (r *FundRefunds) drop(ctx context.Context, row fundRefundRow, code errs.Code) error {
	var cleared bool
	err := r.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		n, err := sqlc.New(tx.Queries()).ClearFundRefund(ctx, sqlc.ClearFundRefundParams{
			ID: row.id, Signature: string(row.signed.Signature),
		})
		cleared = n == 1
		return err
	})
	if err != nil {
		return err
	}
	if cleared {
		observability.Info(ctx, observability.TreasuryFundRefundDropped, slog.String("transfer_id", row.id.String()),
			slog.String("code", string(code)), slog.String("status_before", string(domain.FundLanded)),
			slog.String("status_after", string(domain.FundLanded)))
	}
	return errs.New(code, "treasury.FundRefunds.drop", slog.String("transfer_id", row.id.String()))
}

func (r *FundRefunds) load(ctx context.Context, id uuid.UUID) (fundRefundRow, error) {
	const op = "treasury.FundRefunds.load"
	got, err := sqlc.New(r.d.Reads).GetFundRefund(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return fundRefundRow{}, errs.Wrap(err, errs.CodeNotFound, op, slog.String("transfer_id", id.String()))
	}
	if err != nil {
		return fundRefundRow{}, errs.Wrap(err, errs.CodeInternal, op)
	}
	amount, err := money.ParseMicros(got.AmountMicros)
	if err != nil {
		return fundRefundRow{}, errs.Wrap(err, errs.CodeDecodeFailed, op, slog.String("transfer_id", id.String()))
	}
	if got.RefundLastValidBlockHeight < 0 {
		return fundRefundRow{}, errs.New(errs.CodeDecodeFailed, op, slog.String("transfer_id", id.String()))
	}
	return fundRefundRow{
		id:     got.ID,
		user:   ids.UserIDFrom(got.UserID),
		cabal:  ids.CabalIDFrom(got.CabalID),
		amount: amount,
		status: domain.FundStatus(got.Status),
		signed: relayer.SignedTx{
			Bytes: got.RefundSignedTx, Signature: chain.Signature(got.RefundSignature),
			LastValidBlockHeight: uint64(got.RefundLastValidBlockHeight),
		},
		attempted: got.RefundSignature != "",
	}, nil
}
