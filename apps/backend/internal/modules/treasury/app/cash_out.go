package app

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const cashOutMinimumMicros = 100_000

type CashOut struct {
	IdempotencyKey string
	CabalID        ids.CabalID
	UserID         ids.UserID
	PayoutMicros   money.Micros
	All            bool
}
type CashOutResult struct {
	ID           uuid.UUID
	ShareUnits   money.SharesUnits
	PayoutMicros money.Micros
	CreatedAt    time.Time
}
type CashOutPreview struct {
	SliceMicros money.Micros
	ShareUnits  money.SharesUnits
	Pause       CashOutPause
}
type CashOutJob struct {
	ID             uuid.UUID
	CabalID        ids.CabalID
	UserID         ids.UserID
	ShareUnits     money.SharesUnits
	PayoutMicros   money.Micros
	SellUSDCMicros money.Micros
	Status         domain.CashOutStatus
	ResultCode     *string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}
type CashOutPauses interface {
	IsPaused(context.Context, ids.CabalID) (CashOutPause, error)
}
type cashOutReads interface {
	CashOutJobByID(context.Context, sqlc.CashOutJobByIDParams) (sqlc.CashOutJobByIDRow, error)
	CashOutLiveJob(context.Context, sqlc.CashOutLiveJobParams) (bool, error)
	CashOutShares(context.Context, sqlc.CashOutSharesParams) (sqlc.CashOutSharesRow, error)
}
type cashOutReservationQueries interface {
	CashOutLiveJob(context.Context, sqlc.CashOutLiveJobParams) (bool, error)
	CashOutShares(context.Context, sqlc.CashOutSharesParams) (sqlc.CashOutSharesRow, error)
}
type cashOutRecordQueries interface {
	CashOutShortfall(context.Context, sqlc.CashOutShortfallParams) (string, error)
	InsertCashOutJob(context.Context, sqlc.InsertCashOutJobParams) error
}
type CashOutPause struct {
	Paused  bool
	Reasons []string
	Since   time.Time
}
type CashOutPauseFunc func(context.Context, ids.CabalID) (CashOutPause, error)

func (f CashOutPauseFunc) IsPaused(ctx context.Context, cabal ids.CabalID) (CashOutPause, error) {
	return f(ctx, cabal)
}

type CashOutHandler struct {
	uow    *db.UnitOfWork
	locker func(context.Context, db.Tx, ids.CabalID) error
	post   func(context.Context, db.Tx, domain.UserTxn) error
	usdc   domain.Asset
	values port.PositionsReader
	pauses CashOutPauses
	clock  clock.Clock
	ids    ids.Generator
	reads  cashOutReads
}

func NewCashOutHandler(
	uow *db.UnitOfWork,
	ledger Ledger,
	values port.PositionsReader,
	pauses CashOutPauses,
	c clock.Clock,
	g ids.Generator,
	readDB sqlc.DBTX,
) *CashOutHandler {
	return &CashOutHandler{
		uow:    uow,
		locker: ledger.LockCabal,
		post:   ledger.PostUserTxn,
		usdc:   ledger.usdc,
		values: values,
		pauses: pauses,
		clock:  c,
		ids:    g,
		reads:  sqlc.New(readDB),
	}
}

func (h *CashOutHandler) Handle(ctx context.Context, cmd CashOut) (result CashOutResult, err error) {
	err = h.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		result, err = h.start(ctx, tx, cmd)
		return err
	})
	return result, err
}

func (h *CashOutHandler) Preview(
	ctx context.Context,
	cabal ids.CabalID,
	user ids.UserID,
) (CashOutPreview, error) {
	paused, err := h.pauses.IsPaused(ctx, cabal)
	if err != nil {
		return CashOutPreview{}, err
	}
	shares, err := h.reads.CashOutShares(
		ctx,
		sqlc.CashOutSharesParams{CabalID: cabal.UUID(), UserID: user.UUID()},
	)
	if err != nil {
		return CashOutPreview{}, err
	}
	member, err := money.ParseSharesUnits(shares.Shares)
	if err != nil {
		return CashOutPreview{}, err
	}
	if member.IsZero() {
		return CashOutPreview{Pause: paused}, nil
	}
	total, err := money.ParseSharesUnits(shares.Total)
	if err != nil {
		return CashOutPreview{}, err
	}
	pot, err := h.values.PotValue(ctx, cabal)
	if err != nil {
		return CashOutPreview{}, err
	}
	slice, err := domain.PayoutFor(member, total, pot)
	if err != nil {
		return CashOutPreview{}, err
	}
	return CashOutPreview{SliceMicros: slice, ShareUnits: member, Pause: paused}, nil
}

func (h *CashOutHandler) Job(
	ctx context.Context,
	cabal ids.CabalID,
	user ids.UserID,
	id uuid.UUID,
) (CashOutJob, error) {
	row, err := h.reads.CashOutJobByID(
		ctx,
		sqlc.CashOutJobByIDParams{ID: id, CabalID: cabal.UUID(), UserID: user.UUID()},
	)
	if errors.Is(err, sql.ErrNoRows) {
		return CashOutJob{}, errs.Wrap(err, errs.CodeNotFound, "treasury.CashOut.Job")
	}
	if err != nil {
		return CashOutJob{}, err
	}
	shares, err := money.ParseSharesUnits(row.ShareUnits)
	if err != nil {
		return CashOutJob{}, err
	}
	payout, err := money.ParseMicros(row.PayoutMicros)
	if err != nil {
		return CashOutJob{}, err
	}
	sell, err := money.ParseMicros(row.SellUsdcMicros)
	if err != nil {
		return CashOutJob{}, err
	}
	status, err := domain.ParseCashOutStatus(row.Status)
	if err != nil {
		return CashOutJob{}, err
	}
	var code *string
	if row.ResultCode.Valid {
		code = &row.ResultCode.String
	}
	return CashOutJob{
		ID:             row.ID,
		CabalID:        cabal,
		UserID:         user,
		ShareUnits:     shares,
		PayoutMicros:   payout,
		SellUSDCMicros: sell,
		Status:         status,
		ResultCode:     code,
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
	}, nil
}

func (h *CashOutHandler) start(ctx context.Context, tx db.Tx, cmd CashOut) (CashOutResult, error) {
	if err := h.locker(ctx, tx, cmd.CabalID); err != nil {
		return CashOutResult{}, err
	}
	paused, err := h.pauses.IsPaused(ctx, cmd.CabalID)
	if err != nil {
		return CashOutResult{}, err
	}
	if paused.Paused {
		return CashOutResult{}, errs.New(
			errs.CodeCabalPaused,
			"treasury.CashOut",
			slog.String("cabal_id", cmd.CabalID.String()),
		)
	}
	return h.reserve(ctx, tx, cmd)
}

func (h *CashOutHandler) reserve(
	ctx context.Context,
	tx db.Tx,
	cmd CashOut,
) (CashOutResult, error) {
	q := sqlc.New(tx.Queries())
	units, payout, err := h.amount(ctx, q, cmd)
	if err != nil {
		return CashOutResult{}, err
	}
	return h.record(ctx, tx, q, cmd, units, payout)
}

func (h *CashOutHandler) amount(
	ctx context.Context,
	q cashOutReservationQueries,
	cmd CashOut,
) (money.SharesUnits, money.Micros, error) {
	shares, err := q.CashOutShares(ctx, sqlc.CashOutSharesParams{
		CabalID: cmd.CabalID.UUID(), UserID: cmd.UserID.UUID(),
	})
	if err != nil {
		return money.SharesUnits{}, money.Micros{}, err
	}
	memberUnits, err := money.ParseSharesUnits(shares.Shares)
	if err != nil || memberUnits.IsZero() {
		return money.SharesUnits{}, money.Micros{}, errs.New(
			errs.CodeInsufficientShares,
			"treasury.CashOut",
		)
	}
	totalUnits, err := money.ParseSharesUnits(shares.Total)
	if err != nil {
		return money.SharesUnits{}, money.Micros{}, err
	}
	live, err := q.CashOutLiveJob(
		ctx,
		sqlc.CashOutLiveJobParams{CabalID: cmd.CabalID.UUID(), UserID: cmd.UserID.UUID()},
	)
	if err != nil {
		return money.SharesUnits{}, money.Micros{}, err
	}
	if live {
		return money.SharesUnits{}, money.Micros{}, errs.New(
			errs.CodeCashOutInProgress,
			"treasury.CashOut",
		)
	}
	pot, err := h.values.PotValue(ctx, cmd.CabalID)
	if err != nil {
		return money.SharesUnits{}, money.Micros{}, err
	}
	units, payout, err := cashOutAmount(cmd, memberUnits, totalUnits, pot)
	if err != nil {
		return money.SharesUnits{}, money.Micros{}, err
	}
	return units, payout, nil
}

func (h *CashOutHandler) record(
	ctx context.Context,
	tx db.Tx,
	q cashOutRecordQueries,
	cmd CashOut,
	units money.SharesUnits,
	payout money.Micros,
) (CashOutResult, error) {
	short, err := q.CashOutShortfall(ctx, sqlc.CashOutShortfallParams{
		CabalID: cmd.CabalID.UUID(), Asset: string(h.usdc), PayoutMicros: payout.String(),
	})
	if err != nil {
		return CashOutResult{}, err
	}
	sell, err := money.ParseMicros(short)
	if err != nil {
		return CashOutResult{}, err
	}
	result := CashOutResult{
		ID:           h.ids.NewV7(),
		ShareUnits:   units,
		PayoutMicros: payout,
		CreatedAt:    h.clock.Now(),
	}
	if err := q.InsertCashOutJob(ctx, sqlc.InsertCashOutJobParams{
		ID: result.ID, CabalID: cmd.CabalID.UUID(), UserID: cmd.UserID.UUID(), ShareUnits: units.String(),
		PayoutMicros: payout.String(), SellUsdcMicros: sell.String(), At: result.CreatedAt,
	}); err != nil {
		return CashOutResult{}, err
	}
	paid, err := payout.Delta(money.Micros{})
	if err != nil {
		return CashOutResult{}, err
	}
	shares := money.MicrosFromUint64(units.Uint64())
	burned, err := money.Micros{}.Delta(shares)
	if err != nil {
		return CashOutResult{}, err
	}
	txn, err := domain.NewUserTxn(domain.UserTxnHeader{
		ID: h.ids.NewV7(), UserID: cmd.UserID, CabalID: cmd.CabalID, Kind: domain.UserCashOut,
		Status: domain.TxnPending, TransferID: result.ID,
	}, []domain.UserEntry{
		{Account: domain.UserCabal, Asset: h.usdc, Amount: money.SignedMicrosFromInt64(-paid.Int64())},
		{Account: domain.UserWallet, Asset: h.usdc, Amount: paid},
		{Account: domain.UserHolder, Asset: domain.SharesAsset(cmd.CabalID), Amount: burned},
		{
			Account: domain.UserIssuer, Asset: domain.SharesAsset(cmd.CabalID),
			Amount: money.SignedMicrosFromInt64(-burned.Int64()),
		},
	})
	if err != nil {
		return CashOutResult{}, err
	}
	if err := h.post(ctx, tx, txn); err != nil {
		return CashOutResult{}, err
	}
	err = tx.Events.Append(
		ctx,
		events.CashOutStarted{
			V: 1, JobID: result.ID, CabalID: cmd.CabalID.UUID(),
			UserID: cmd.UserID.UUID(), ShareUnits: units.Uint64(), PayoutMicros: payout, SellUSDC: sell,
		},
	)
	return result, err
}

func cashOutAmount(
	cmd CashOut, member, total money.SharesUnits, pot money.Micros,
) (money.SharesUnits, money.Micros, error) {
	if pot.IsZero() {
		return money.SharesUnits{}, money.Micros{}, errs.New(errs.CodePriceUnavailable, "treasury.CashOut")
	}
	if cmd.All {
		payout, err := domain.PayoutFor(member, total, pot)
		if err == nil && payout.Cmp(money.MicrosFromUint64(cashOutMinimumMicros)) < 0 {
			err = errs.New(errs.CodePotValueChanged, "treasury.CashOut")
		}
		return member, payout, err
	}
	if cmd.PayoutMicros.Cmp(money.MicrosFromUint64(cashOutMinimumMicros)) < 0 {
		return money.SharesUnits{}, money.Micros{}, errs.New(
			errs.CodeInvalidInput,
			"treasury.CashOut",
		)
	}
	units, err := domain.CashOutUnitsFor(cmd.PayoutMicros, total, pot)
	if err != nil || units.Cmp(member) > 0 {
		return money.SharesUnits{}, money.Micros{}, errs.New(
			errs.CodeInsufficientShares,
			"treasury.CashOut",
		)
	}
	promote, err := promotesRemainder(member, units, total, pot)
	if err != nil {
		return money.SharesUnits{}, money.Micros{}, err
	}
	if promote {
		payout, err := domain.PayoutFor(member, total, pot)
		return member, payout, err
	}
	payout, err := domain.PayoutFor(units, total, pot)
	return units, payout, err
}

func promotesRemainder(member, units, total money.SharesUnits, pot money.Micros) (bool, error) {
	remaining, err := member.Sub(units)
	if err != nil {
		return false, err
	}
	if remaining.IsZero() {
		return false, nil
	}
	remainder, err := domain.PayoutFor(remaining, total, pot)
	if err != nil {
		return false, err
	}
	return remainder.Cmp(money.MicrosFromUint64(cashOutMinimumMicros)) < 0, nil
}
