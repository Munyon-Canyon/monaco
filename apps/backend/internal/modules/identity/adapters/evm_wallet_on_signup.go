package adapters

import (
	"context"
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

type EVMWalletOnSignup struct {
	Wallets  DepositWallets
	UoW      *db.UnitOfWork
	Networks []chain.Network
}

func (h EVMWalletOnSignup) Handle(ctx context.Context, d bus.Delivery, e events.UserCreated) error {
	ctx = auth.WithActor(ctx, auth.Actor{Kind: auth.ActorSystem, ID: d.Handler})
	defer bus.KeepAlive(ctx)()
	outcome := "ok"
	if len(h.Networks) == 0 {
		outcome = "deferred"
	} else if _, err := h.Wallets.EnsureEVMDepositWallet(ctx, ids.UserIDFrom(e.UserID)); err != nil {
		code := errs.CodeOf(err)
		if code != errs.CodeUserNotFound && code != errs.CodeNotImplemented {
			return err
		}
		observability.Info(ctx, observability.IdentityEVMWalletSkipped,
			slog.String("user_id", e.UserID.String()), slog.String("code", string(code)))
		outcome = string(code)
	}
	return h.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		_, err := d.RecordAs(ctx, tx, outcome)
		return err
	})
}
