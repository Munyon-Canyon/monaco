package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	fundingport "github.com/monaco/monaco/apps/backend/internal/modules/funding/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/sqlc"
	treasuryport "github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	dbsqlc "github.com/monaco/monaco/apps/backend/internal/platform/db/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

const DustCeilingMicros uint64 = 10_000

type Balances interface {
	Observe(ctx context.Context, user ids.UserID) (fundingport.Observation, error)
	AvailableIn(ctx context.Context, q dbsqlc.DBTX, obs fundingport.Observation) (fundingport.Balance, error)
}

type Arriving = fundingport.DepositsArriving

type Leaders = cabalport.CabalLeaders

type Stakes interface {
	StakesOf(ctx context.Context, user ids.UserID) ([]treasuryport.Stake, error)
}

type DeletingUsers interface {
	LockWalletOutflow(ctx context.Context, q sqlc.DBTX, id ids.UserID) error
	LockByID(ctx context.Context, q sqlc.DBTX, id ids.UserID) (domain.User, error)
	Delete(ctx context.Context, q sqlc.DBTX, id ids.UserID, expected domain.AccountStatus, at time.Time) error
}

type DeleteAccountDeps struct {
	UoW      *db.UnitOfWork
	Users    DeletingUsers
	Balances Balances
	Stakes   Stakes
	Arriving Arriving
	Leaders  Leaders
	Clock    clock.Clock
	Hints    Hints
}

type DeleteAccount struct{ d DeleteAccountDeps }

func NewDeleteAccount(d DeleteAccountDeps) *DeleteAccount { return &DeleteAccount{d: d} }

func (h *DeleteAccount) Handle(ctx context.Context, id ids.UserID) error {
	obs, err := h.d.Balances.Observe(ctx, id)
	if err != nil {
		return err
	}
	return h.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		if err := h.d.Users.LockWalletOutflow(ctx, tx.Queries(), id); err != nil {
			return err
		}
		user, err := h.d.Users.LockByID(ctx, tx.Queries(), id)
		if err != nil {
			return err
		}
		if _, err := domain.NextAccountStatus(user.AccountStatus, domain.AccountDelete); err != nil {
			return err
		}
		at := h.d.Clock.Now()
		forfeited, err := h.empty(ctx, tx, id, obs, at)
		if err != nil {
			return err
		}
		if err := h.d.Users.Delete(ctx, tx.Queries(), id, user.AccountStatus, at); err != nil {
			return err
		}
		deleted := deletedEvent(id, user, forfeited, at)
		if err := tx.Events.Append(ctx, deleted); err != nil {
			return err
		}
		tx.AfterCommit(func(ctx context.Context) {
			h.d.Hints.PublishHint(ctx, "user."+id.String()+".me_changed", nil)
			logDeleted(ctx, id, deleted)
		})
		return nil
	})
}

func deletedEvent(id ids.UserID, user domain.User, forfeited money.Micros, at time.Time) events.UserDeleted {
	deleted := events.UserDeleted{V: 1, UserID: id.UUID(), At: at}
	if forfeited.IsZero() {
		return deleted
	}
	deleted.ForfeitedMicros = &forfeited
	if user.Wallet != nil {
		deleted.ForfeitedWallet = MaskAddress(string(user.Wallet.Address))
	}
	return deleted
}

func logDeleted(ctx context.Context, id ids.UserID, deleted events.UserDeleted) {
	if deleted.ForfeitedMicros == nil {
		observability.Info(ctx, observability.IdentityAccountDeleted, slog.String("user_id", id.String()))
		return
	}
	observability.Info(ctx, observability.IdentityAccountDeleted,
		slog.String("user_id", id.String()),
		slog.String("forfeited_micros", deleted.ForfeitedMicros.String()),
		slog.String("forfeited_wallet", deleted.ForfeitedWallet))
}

func (h *DeleteAccount) empty(
	ctx context.Context, tx db.Tx, id ids.UserID, obs fundingport.Observation, at time.Time,
) (money.Micros, error) {
	const op = "identity.DeleteAccount"
	balance, err := h.d.Balances.AvailableIn(ctx, tx.Queries(), obs)
	if err != nil {
		return money.Micros{}, err
	}
	stakes, err := h.d.Stakes.StakesOf(ctx, id)
	if err != nil {
		return money.Micros{}, err
	}
	if len(stakes) > 0 {
		return money.Micros{}, errs.New(errs.CodeAccountHasPositions, op, slog.Int("cabal_count", len(stakes)))
	}
	if !balance.InFlightFundMicros.IsZero() || !balance.InFlightWithdrawalMicros.IsZero() ||
		balance.OnChainMicros.Cmp(money.MicrosFromUint64(DustCeilingMicros)) >= 0 {
		return money.Micros{}, errs.New(errs.CodeAccountHasBalance, op)
	}
	arriving, err := h.d.Arriving.DepositArriving(ctx, tx.Queries(), id, at)
	if err != nil {
		return money.Micros{}, err
	}
	if arriving {
		return money.Micros{}, errs.New(errs.CodeAccountHasArrivingDeposit, op)
	}
	leads, err := h.d.Leaders.LeadsCabalWithMembers(ctx, tx.Queries(), id)
	if err != nil {
		return money.Micros{}, err
	}
	if leads {
		return money.Micros{}, errs.New(errs.CodeLeaveCreatorWithMembers, op)
	}
	return balance.OnChainMicros, nil
}

func MaskAddress(address string) string {
	const edge = 4
	if len(address) <= 2*edge {
		return "..."
	}
	return address[:edge] + "..." + address[len(address)-edge:]
}
