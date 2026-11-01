package app

import (
	"context"
	"log/slog"
	"strconv"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	fundingport "github.com/monaco/monaco/apps/backend/internal/modules/funding/port"
	identityport "github.com/monaco/monaco/apps/backend/internal/modules/identity/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/relayer"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	dbsqlc "github.com/monaco/monaco/apps/backend/internal/platform/db/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

const FundCabalOperation = "treasury.fund_cabal"

type FundCabal struct {
	CabalID ids.CabalID
	UserID  ids.UserID
	Amount  money.Micros
	Intent  db.IntentKey
}

type FundCabals interface {
	IsMember(ctx context.Context, id ids.CabalID, user ids.UserID) (bool, error)
	LockMember(ctx context.Context, q dbsqlc.DBTX, id ids.CabalID, user ids.UserID) (bool, error)
	LockStatus(ctx context.Context, q dbsqlc.DBTX, id ids.CabalID) (cabalport.Status, error)
	Status(ctx context.Context, id ids.CabalID) (cabalport.Status, error)
	TreasuryWallet(ctx context.Context, id ids.CabalID) (cabalport.TreasuryWallet, error)
}

type FundWallets interface {
	MemberWallet(ctx context.Context, id ids.UserID) (identityport.MemberWallet, error)
}

type FundPotReads interface {
	TotalShares(ctx context.Context, cabalID ids.CabalID) (money.SharesUnits, error)
	PotBasis(ctx context.Context, cabalID ids.CabalID) (string, error)
	HoldsLiveAssets(ctx context.Context, cabalID ids.CabalID) (bool, error)
}

type FundPot interface {
	FundPotReads
	PotValue(ctx context.Context, cabalID ids.CabalID) (money.Micros, error)
}

type FundTransfers interface {
	Build(ctx context.Context, spec relayer.TransferSpec) (relayer.SignedTx, error)
	Broadcast(ctx context.Context, tx relayer.SignedTx) error
}

type FundCabalDeps struct {
	UoW       *db.UnitOfWork
	IDs       ids.Generator
	Clock     clock.Clock
	Cabals    FundCabals
	Wallets   FundWallets
	Balances  fundingport.Balances
	Pauses    func(db.Tx) fundingport.Pauses
	Pot       FundPot
	Transfers FundTransfers
	Hints     HintPublisher
	USDC      chain.Mint
}

type FundCabalHandler struct{ d FundCabalDeps }

func NewFundCabalHandler(d FundCabalDeps) *FundCabalHandler { return &FundCabalHandler{d: d} }

type fundRoute struct {
	from chain.Wallet
	to   chain.SolanaAddress
}

type fundReads struct {
	balance    fundingport.Observation
	balanceErr error
	potErr     error
}

func (h *FundCabalHandler) Handle(ctx context.Context, cmd FundCabal) (uuid.UUID, error) {
	route, err := h.admit(ctx, cmd)
	if err != nil {
		return uuid.Nil, err
	}
	id := h.d.IDs.NewV7()
	var prior sqlc.GetFundTransferRow
	replayed := false
	reads := h.read(ctx, cmd)
	if err := h.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		var err error
		prior, replayed, err = h.create(ctx, tx, id, cmd, route, reads)
		return err
	}); err != nil {
		return uuid.Nil, err
	}
	if replayed {
		if domain.FundStatus(prior.Status) != domain.FundCreated {
			return prior.ID, nil
		}
		id = prior.ID
	}
	signed, err := h.d.Transfers.Build(ctx, relayer.TransferSpec{
		FromWallet: route.from, To: route.to, Mint: h.d.USDC,
		Amount: money.NewBaseUnits(cmd.Amount.Uint64(), h.d.USDC.Decimals),
	})
	if err != nil {
		h.failUnsent(ctx, id, err)
		return uuid.Nil, err
	}
	faultpoint.Hit(ctx, faultpoint.AfterSign)
	if err := h.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		return h.submit(ctx, tx, id, cmd, signed)
	}); err != nil {
		return uuid.Nil, err
	}
	observability.Info(
		ctx,
		observability.TreasuryFundSubmitted,
		slog.String("transfer_id", id.String()),
		slog.String("cabal_id", cmd.CabalID.String()),
		slog.String("amount_micros", cmd.Amount.String()),
		slog.String(
			"status_before",
			string(domain.FundCreated),
		),
		slog.String("status_after", string(domain.FundSubmitted)),
	)
	if err := h.d.Transfers.Broadcast(ctx, signed); err != nil {
		observability.Info(ctx, observability.TreasuryFundBroadcastFailed, slog.String("transfer_id", id.String()),
			slog.String("code", string(errs.CodeOf(err))))
	}
	faultpoint.Hit(ctx, faultpoint.AfterBroadcast)
	return id, nil
}

func (h *FundCabalHandler) admit(ctx context.Context, cmd FundCabal) (fundRoute, error) {
	const op = "treasury.FundCabal"
	member, err := h.d.Cabals.IsMember(ctx, cmd.CabalID, cmd.UserID)
	if err != nil {
		return fundRoute{}, err
	}
	if !member {
		return fundRoute{}, errs.New(errs.CodeNotCabalMember, op, slog.String("cabal_id", cmd.CabalID.String()))
	}
	status, err := h.d.Cabals.Status(ctx, cmd.CabalID)
	if err != nil {
		return fundRoute{}, err
	}
	if status == cabalport.StatusBanned {
		return fundRoute{}, errs.New(errs.CodeCabalBanned, op, slog.String("cabal_id", cmd.CabalID.String()))
	}
	wallet, err := h.d.Wallets.MemberWallet(ctx, cmd.UserID)
	if err != nil {
		return fundRoute{}, err
	}
	treasury, err := h.d.Cabals.TreasuryWallet(ctx, cmd.CabalID)
	if err != nil {
		return fundRoute{}, err
	}
	return fundRoute{
		from: chain.Wallet{ID: wallet.PrivyWalletID, Address: wallet.Address, HasAppSigner: true},
		to:   treasury.Address,
	}, nil
}

func (h *FundCabalHandler) read(ctx context.Context, cmd FundCabal) fundReads {
	obs, balanceErr := h.d.Balances.Observe(ctx, cmd.UserID)
	return fundReads{balance: obs, balanceErr: balanceErr, potErr: h.checkPot(ctx, cmd.CabalID)}
}

func (h *FundCabalHandler) create(
	ctx context.Context, tx db.Tx, id uuid.UUID, cmd FundCabal, route fundRoute, reads fundReads,
) (sqlc.GetFundTransferRow, bool, error) {
	const op = "treasury.FundCabal"
	member, err := h.d.Cabals.LockMember(ctx, tx.Queries(), cmd.CabalID, cmd.UserID)
	if err != nil {
		return sqlc.GetFundTransferRow{}, false, err
	}
	if !member {
		return sqlc.GetFundTransferRow{}, false, errs.New(
			errs.CodeNotCabalMember, op, slog.String("cabal_id", cmd.CabalID.String()))
	}
	status, err := h.d.Cabals.LockStatus(ctx, tx.Queries(), cmd.CabalID)
	if err != nil {
		return sqlc.GetFundTransferRow{}, false, err
	}
	if status == cabalport.StatusBanned {
		return sqlc.GetFundTransferRow{}, false, errs.New(
			errs.CodeCabalBanned, op, slog.String("cabal_id", cmd.CabalID.String()))
	}
	q := sqlc.New(tx.Queries())
	var bound uuid.UUID
	var again bool
	err = q.LockWalletOutflow(ctx, cmd.UserID.UUID())
	if err == nil {
		bound, again, err = cmd.Intent.Bind(ctx, tx.Queries(), id, h.d.Clock.Now())
	}
	if err != nil {
		return sqlc.GetFundTransferRow{}, false, err
	}
	if again {
		prior, err := q.GetFundTransfer(ctx, bound)
		return prior, true, err
	}
	return sqlc.GetFundTransferRow{}, false, h.insert(ctx, tx, q, id, cmd, route, reads)
}

func (h *FundCabalHandler) insert(
	ctx context.Context, tx db.Tx, q *sqlc.Queries, id uuid.UUID, cmd FundCabal, route fundRoute, reads fundReads,
) error {
	const op = "treasury.FundCabal"
	pause, err := h.d.Pauses(tx).IsPaused(ctx, cmd.CabalID)
	if err != nil {
		return err
	}
	if pause.Paused {
		return errs.New(errs.CodeCabalPaused, op, slog.String("cabal_id", cmd.CabalID.String()))
	}
	if reads.balanceErr != nil {
		return reads.balanceErr
	}
	balance, err := h.d.Balances.AvailableIn(ctx, tx.Queries(), reads.balance)
	if err != nil {
		return err
	}
	if balance.AvailableMicros.Cmp(cmd.Amount) < 0 {
		return errs.New(
			errs.CodeInsufficientFunds,
			op,
			slog.String("available_micros", balance.AvailableMicros.String()),
			slog.String("amount_micros", cmd.Amount.String()),
		)
	}
	if reads.potErr != nil {
		return reads.potErr
	}
	return q.InsertFundTransfer(ctx, sqlc.InsertFundTransferParams{
		ID: id, UserID: cmd.UserID.UUID(), CabalID: cmd.CabalID.UUID(), AmountMicros: cmd.Amount.String(),
		FromAddress: string(route.from.Address), ToAddress: string(route.to), CreatedAt: h.d.Clock.Now(),
	})
}

func (h *FundCabalHandler) checkPot(ctx context.Context, cabalID ids.CabalID) error {
	total, err := h.d.Pot.TotalShares(ctx, cabalID)
	if err != nil {
		return err
	}
	if total.IsZero() {
		return refuseUnpriceableFund(ctx, h.d.Pot, cabalID)
	}
	pot, err := h.d.Pot.PotValue(ctx, cabalID)
	if errs.CodeOf(err) == errs.CodePriceUnavailable {
		return nil
	}
	if err != nil {
		return err
	}
	if pot.IsZero() {
		return errs.New(errs.CodePotValueZero, "treasury.FundCabal", slog.String("cabal_id", cabalID.String()))
	}
	return nil
}

func refuseUnpriceableFund(ctx context.Context, reads FundPotReads, cabalID ids.CabalID) error {
	holds, err := reads.HoldsLiveAssets(ctx, cabalID)
	if err != nil || !holds {
		return err
	}
	return errs.New(errs.CodePotValueChanged, "treasury.FundCabal", slog.String("cabal_id", cabalID.String()),
		slog.String("reason", "no_shares_but_live_holdings"))
}

func (h *FundCabalHandler) submit(
	ctx context.Context, tx db.Tx, id uuid.UUID, cmd FundCabal, signed relayer.SignedTx,
) error {
	n, err := sqlc.New(tx.Queries()).SubmitFundTransfer(ctx, sqlc.SubmitFundTransferParams{
		ID: id, SignedTx: signed.Bytes, TxSignature: string(signed.Signature),
		LastValidBlockHeight: strconv.FormatUint(signed.LastValidBlockHeight, 10), SubmittedAt: h.d.Clock.Now(),
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return errs.New(errs.CodeFundNotSent, "treasury.FundCabal", slog.String("transfer_id", id.String()),
			slog.String("reason", "failed while signing"))
	}
	if err := tx.Events.Append(ctx, events.FundSubmitted{
		V: 1, TransferID: id, CabalID: cmd.CabalID.UUID(), UserID: cmd.UserID.UUID(), AmountMicros: cmd.Amount,
		TxSignature: signed.Signature,
	}); err != nil {
		return err
	}
	tx.AfterCommit(func(ctx context.Context) {
		h.d.Hints.PublishHint(ctx, events.UserBalanceChangedHint(cmd.UserID), nil)
	})
	return nil
}

func (h *FundCabalHandler) failUnsent(ctx context.Context, id uuid.UUID, cause error) {
	code := string(errs.CodeOf(cause))
	failed := false
	err := h.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		moved, err := sqlc.New(tx.Queries()).FailUnsentFundTransfer(ctx,
			sqlc.FailUnsentFundTransferParams{ID: id, FailCode: code})
		if err == nil && moved > 0 {
			err = db.UnbindIntent(ctx, tx.Queries(), FundCabalOperation, id)
		}
		failed = err == nil && moved > 0
		return err
	})
	after := string(domain.FundCreated)
	switch {
	case err != nil:
	case failed:
		after = string(domain.FundFailed)
	default:
		after = "unchanged"
	}
	observability.Info(ctx, observability.TreasuryFundSignFailed, slog.String("transfer_id", id.String()),
		slog.String("code", code), slog.String("status_before", string(domain.FundCreated)),
		slog.String("status_after", after))
}
