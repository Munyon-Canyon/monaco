package app

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const DustLamports = 5_000_000

type DevUserDeletion struct {
	Handle        string
	PrivyID       string
	WalletAddress string
}

type DevUserEraser interface {
	DevUserForDelete(ctx context.Context, q sqlc.DBTX, id ids.UserID) (DevUserDeletion, bool, error)
	DeleteDevUser(ctx context.Context, q sqlc.DBTX, id ids.UserID) error
}

type WalletBalances interface {
	Balances(ctx context.Context, address chain.SolanaAddress) (usdcMicros, lamports uint64, err error)
}

type FundedError struct {
	Address    string
	USDCMicros uint64
	Lamports   uint64
}

func (e *FundedError) Error() string {
	return fmt.Sprintf("wallet %s holds %d USDC micros and %d lamports: sweep it with scripts/sweep-wallets.sh first",
		e.Address, e.USDCMicros, e.Lamports)
}

type DeleteDevUserDeps struct {
	UoW      *db.UnitOfWork
	Users    DevUserEraser
	Privy    PrivyDevUsers
	Balances WalletBalances
}

func DeleteDevUser(ctx context.Context, d DeleteDevUserDeps, id ids.UserID) error {
	return d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		q := tx.Queries()
		row, found, err := d.Users.DevUserForDelete(ctx, q, id)
		if err != nil {
			return err
		}
		if !found {
			return errs.New(errs.CodeNotFound, "identity.DeleteDevUser")
		}
		privyID, privyFound, err := d.checkDevUser(ctx, row)
		if err != nil {
			return err
		}
		if err := d.Users.DeleteDevUser(ctx, q, id); err != nil {
			return err
		}
		if !privyFound {
			return nil
		}
		return d.Privy.Delete(ctx, privyID)
	})
}

func (d DeleteDevUserDeps) checkDevUser(ctx context.Context, row DevUserDeletion) (PrivyUserID, bool, error) {
	const op = "identity.DeleteDevUser"
	if !strings.HasPrefix(row.Handle, domain.DevHandlePrefix) {
		return "", false, errs.New(errs.CodeInvalidInput, op, slog.String("reason", "not_a_dev_handle"))
	}
	privyID := PrivyUserID(row.PrivyID)
	devOnly, privyFound, err := d.Privy.DevOnly(ctx, privyID)
	if err != nil {
		return "", false, err
	}
	if privyFound && !devOnly {
		return "", false, errs.New(errs.CodeInvalidInput, op, slog.String("reason", "privy_user_is_not_dev_only"))
	}
	if row.WalletAddress == "" {
		return privyID, privyFound, nil
	}
	usdc, lamports, err := d.Balances.Balances(ctx, chain.SolanaAddress(row.WalletAddress))
	if err != nil {
		return "", false, err
	}
	if usdc > 0 || lamports > DustLamports {
		return "", false, &FundedError{Address: row.WalletAddress, USDCMicros: usdc, Lamports: lamports}
	}
	return privyID, privyFound, nil
}
