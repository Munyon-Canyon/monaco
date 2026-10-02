package app

import (
	"context"
	"encoding/hex"
	"io"
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
}

func CreateDevUser(ctx context.Context, d CreateDevUserDeps) (DevUser, error) {
	const op = "identity.CreateDevUser"
	if d.Env == config.EnvProduction {
		return DevUser{}, errs.New(errs.CodeInvalidInput, op)
	}
	suffix, err := devSuffix(d.Rand)
	if err != nil {
		return DevUser{}, err
	}
	return settleDevUser(ctx, d, suffix)
}

func devSuffix(r io.Reader) (string, error) {
	var buf [4]byte
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return "", errs.Wrap(err, errs.CodeInternal, "identity.CreateDevUser")
	}
	return hex.EncodeToString(buf[:]), nil
}

func settleDevUser(ctx context.Context, d CreateDevUserDeps, suffix string) (DevUser, error) {
	handle := "dev_" + suffix
	privyID, err := d.Privy.Create(ctx, "dev-"+suffix+"@example.com")
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
