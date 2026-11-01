package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	fundingport "github.com/monaco/monaco/apps/backend/internal/modules/funding/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/sqlc"
	treasuryport "github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

const DustCeilingMicros uint64 = 10_000

type Balances interface {
	Available(ctx context.Context, user ids.UserID) (fundingport.Balance, error)
}

type Stakes interface {
	StakesOf(ctx context.Context, user ids.UserID) ([]treasuryport.Stake, error)
}

type DeletingUsers interface {
	LockByID(ctx context.Context, q sqlc.DBTX, id ids.UserID) (domain.User, error)
	Delete(ctx context.Context, q sqlc.DBTX, id ids.UserID, expected domain.AccountStatus, at time.Time) error
}

type DeleteAccountDeps struct {
	UoW      *db.UnitOfWork
	Users    DeletingUsers
	Balances Balances
	Stakes   Stakes
	Clock    clock.Clock
	Hints    Hints
}

type DeleteAccount struct{ d DeleteAccountDeps }

func NewDeleteAccount(d DeleteAccountDeps) *DeleteAccount { return &DeleteAccount{d: d} }

func (h *DeleteAccount) Handle(ctx context.Context, id ids.UserID) error {
	forfeited, err := h.empty(ctx, id)
	if err != nil {
		return err
	}
	return h.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		user, err := h.d.Users.LockByID(ctx, tx.Queries(), id)
		if err != nil {
			return err
		}
		if _, err := domain.NextAccountStatus(user.AccountStatus, domain.AccountDelete); err != nil {
			return err
		}
		at := h.d.Clock.Now()
		if err := h.d.Users.Delete(ctx, tx.Queries(), id, user.AccountStatus, at); err != nil {
			return err
		}
		deleted := events.UserDeleted{V: 1, UserID: id.UUID(), At: at}
		attrs := []slog.Attr{slog.String("user_id", id.String())}
		if !forfeited.IsZero() {
			deleted.ForfeitedMicros = &forfeited
			if user.Wallet != nil {
				deleted.ForfeitedWallet = MaskAddress(string(user.Wallet.Address))
			}
			attrs = append(
				attrs,
				slog.String(
					"forfeited_micros",
					forfeited.String(),
				),
				slog.String("forfeited_wallet", deleted.ForfeitedWallet),
			)
		}
		if err := tx.Events.Append(ctx, deleted); err != nil {
			return err
		}
		tx.AfterCommit(func(ctx context.Context) {
			h.d.Hints.PublishHint(ctx, "user."+id.String()+".me_changed", nil)
			observability.Info(ctx, observability.IdentityAccountDeleted, attrs...)
		})
		return nil
	})
}

func (h *DeleteAccount) empty(ctx context.Context, id ids.UserID) (money.Micros, error) {
	const op = "identity.DeleteAccount"
	stakes, err := h.d.Stakes.StakesOf(ctx, id)
	if err != nil {
		return money.Micros{}, err
	}
	if len(stakes) > 0 {
		return money.Micros{}, errs.New(errs.CodeAccountHasPositions, op, slog.Int("cabal_count", len(stakes)))
	}
	balance, err := h.d.Balances.Available(ctx, id)
	if err != nil {
		return money.Micros{}, err
	}
	if !balance.InFlightFundMicros.IsZero() || !balance.InFlightWithdrawalMicros.IsZero() ||
		balance.OnChainMicros.Cmp(money.MicrosFromUint64(DustCeilingMicros)) >= 0 {
		return money.Micros{}, errs.New(errs.CodeAccountHasBalance, op)
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
