package app

import (
	"context"
	"encoding/hex"
	"io"
	"log/slog"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

type DevUser struct {
	UserID        ids.UserID
	Handle        string
	WalletAddress chain.SolanaAddress
}

type DevUsers interface {
	InsertDevUser(
		ctx context.Context, q sqlc.DBTX, id ids.UserID, privyID, handle, displayName string, at time.Time,
	) error
	AttachWallet(ctx context.Context, q sqlc.DBTX, id ids.UserID, w domain.Wallet, at time.Time) (bool, error)
	UpdateAuthState(
		ctx context.Context, q sqlc.DBTX, id ids.UserID, expected, next domain.AuthState, at time.Time,
	) error
}

type DevUserRow struct {
	ID      ids.UserID
	PrivyID string
	Deleted bool
}

type DevPools interface {
	LockDevHandle(ctx context.Context, q sqlc.DBTX, handle string) error
	DevUserByHandle(ctx context.Context, q sqlc.DBTX, handle string) (DevUserRow, bool, error)
	RestoreDevUser(ctx context.Context, q sqlc.DBTX, id ids.UserID, displayName string, at time.Time) (bool, error)
}

type CreateDevUserDeps struct {
	Env     config.Env
	UoW     *db.UnitOfWork
	Users   DevUsers
	Privy   PrivyUsers
	Wallets MemberWallets
	IDs     ids.Generator
	Clock   clock.Clock
	Hints   Hints
	Rand    io.Reader
	Pools   DevPools
	Pool    string
}

func CreateDevUser(ctx context.Context, d CreateDevUserDeps) (DevUser, error) {
	const op = "identity.CreateDevUser"
	if d.Env == config.EnvProduction {
		return DevUser{}, errs.New(errs.CodeInvalidInput, op)
	}
	if d.Pool != "" {
		return poolDevUser(ctx, d)
	}
	suffix, err := devSuffix(d.Rand)
	if err != nil {
		return DevUser{}, err
	}
	return settleDevUser(ctx, d, suffix)
}

func poolDevUser(ctx context.Context, d CreateDevUserDeps) (DevUser, error) {
	const op = "identity.CreateDevUser"
	if !domain.ValidDevPool(d.Pool) {
		return DevUser{}, errs.New(errs.CodeInvalidInput, op, slog.String("reason", "pool_name"))
	}
	handle := domain.DevHandle(d.Pool)
	var user DevUser
	err := d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		q := tx.Queries()
		if err := d.Pools.LockDevHandle(ctx, q, handle); err != nil {
			return err
		}
		row, found, err := d.Pools.DevUserByHandle(ctx, q, handle)
		if err != nil {
			return err
		}
		if found && !row.Deleted {
			return poolWallet(ctx, d, &user, row.ID, PrivyUserID(row.PrivyID), handle)
		}
		privyID, err := adoptOrCreatePrivy(ctx, d, domain.DevEmail(d.Pool))
		if err != nil {
			return err
		}
		if found {
			return restorePoolUser(ctx, d, tx, row, privyID, &user)
		}
		return insertPoolUser(ctx, d, tx, handle, privyID, &user)
	})
	return user, err
}

func poolWallet(
	ctx context.Context, d CreateDevUserDeps, user *DevUser, id ids.UserID, privyID PrivyUserID, handle string,
) error {
	wallet, err := d.Wallets.FindOrCreate(ctx, privyID)
	if err != nil {
		return err
	}
	*user = DevUser{UserID: id, Handle: handle, WalletAddress: wallet.Address}
	return nil
}

func adoptOrCreatePrivy(ctx context.Context, d CreateDevUserDeps, email string) (PrivyUserID, error) {
	id, found, err := d.Privy.ByEmail(ctx, email)
	if err != nil || found {
		return id, err
	}
	id, err = d.Privy.Create(ctx, email)
	if errs.CodeOf(err) != errs.CodeInvalidInput {
		return id, err
	}
	again, found, lookupErr := d.Privy.ByEmail(ctx, email)
	if lookupErr != nil || !found {
		return "", err
	}
	return again, nil
}

func restorePoolUser(
	ctx context.Context, d CreateDevUserDeps, tx db.Tx, row DevUserRow, privyID PrivyUserID, user *DevUser,
) error {
	const op = "identity.CreateDevUser"
	if string(privyID) != row.PrivyID {
		return errs.New(errs.CodeWalletMismatch, op, slog.String("reason", "pool_privy_user_changed"))
	}
	handle := domain.DevHandle(d.Pool)
	restored, err := d.Pools.RestoreDevUser(ctx, tx.Queries(), row.ID, "Dev "+d.Pool, d.Clock.Now())
	if err != nil {
		return err
	}
	if !restored {
		return errs.New(errs.CodeInternal, op, slog.String("reason", "pool_user_not_deleted"))
	}
	return poolWallet(ctx, d, user, row.ID, privyID, handle)
}

func insertPoolUser(
	ctx context.Context, d CreateDevUserDeps, tx db.Tx, handle string, privyID PrivyUserID, user *DevUser,
) error {
	wallet, err := d.Wallets.FindOrCreate(ctx, privyID)
	if err != nil {
		return err
	}
	id := ids.NewUserID(d.IDs)
	err = writeDevUser(ctx, tx, d, devRow{
		id: id, privyID: string(privyID), handle: handle, display: "Dev " + d.Pool,
		wallet: wallet.Wallet, at: d.Clock.Now(),
	})
	if err != nil {
		return err
	}
	*user = DevUser{UserID: id, Handle: handle, WalletAddress: wallet.Address}
	return nil
}

func devSuffix(r io.Reader) (string, error) {
	var buf [4]byte
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return "", errs.Wrap(err, errs.CodeInternal, "identity.CreateDevUser")
	}
	return hex.EncodeToString(buf[:]), nil
}

func settleDevUser(ctx context.Context, d CreateDevUserDeps, suffix string) (DevUser, error) {
	handle := domain.DevHandle(suffix)
	privyID, err := d.Privy.Create(ctx, domain.DevEmail(suffix))
	if err != nil {
		return DevUser{}, err
	}
	wallet, err := d.Wallets.FindOrCreate(ctx, privyID)
	if err != nil {
		return DevUser{}, err
	}
	id := ids.NewUserID(d.IDs)
	now := d.Clock.Now()
	err = d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		return writeDevUser(ctx, tx, d, devRow{
			id: id, privyID: string(privyID), handle: handle, display: "Dev " + suffix,
			wallet: wallet.Wallet, at: now,
		})
	})
	if err != nil {
		return DevUser{}, err
	}
	return DevUser{UserID: id, Handle: handle, WalletAddress: wallet.Address}, nil
}

type devRow struct {
	id      ids.UserID
	privyID string
	handle  string
	display string
	wallet  domain.Wallet
	at      time.Time
}

func writeDevUser(ctx context.Context, tx db.Tx, d CreateDevUserDeps, row devRow) error {
	const op = "identity.CreateDevUser"
	ctx = observability.WithActor(ctx, auth.Actor{Kind: auth.ActorUser, ID: row.id.String()}.Key())
	q := tx.Queries()
	if err := d.Users.InsertDevUser(ctx, q, row.id, row.privyID, row.handle, row.display, row.at); err != nil {
		return err
	}
	attached, err := d.Users.AttachWallet(ctx, q, row.id, row.wallet, row.at)
	if err != nil {
		return err
	}
	if !attached {
		return errs.New(errs.CodeWalletMismatch, op)
	}
	err = tx.Events.Append(ctx, events.UserCreated{
		V: 1, UserID: row.id.UUID(), LoginProvider: string(domain.LoginEmail), CreatedAt: row.at,
	})
	if err != nil {
		return err
	}
	next, _ := domain.NextAuthState(domain.AuthCreated, domain.AuthEvent{Kind: domain.PhoneSkipped})
	if err := d.Users.UpdateAuthState(ctx, q, row.id, domain.AuthCreated, next, row.at); err != nil {
		return err
	}
	err = tx.Events.Append(ctx, events.UserAuthStateChanged{
		V: 1, UserID: row.id.UUID(), From: string(domain.AuthCreated), To: string(next),
		Cause: string(domain.CauseOnboarding), At: row.at,
	})
	if err != nil {
		return err
	}
	tx.AfterCommit(func(ctx context.Context) {
		d.Hints.PublishHint(ctx, "user."+row.id.String()+".me_changed", nil)
	})
	return nil
}
