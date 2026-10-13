package adapters

import (
	"context"
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

type TreasuryEVMWalletOnCreate struct {
	Wallets TreasuryEVMWallets
	UoW     *db.UnitOfWork
}

func (h TreasuryEVMWalletOnCreate) Handle(ctx context.Context, d bus.Delivery, e events.CabalCreated) error {
	ctx = auth.WithActor(ctx, auth.Actor{Kind: auth.ActorSystem, ID: d.Handler})
	defer bus.KeepAlive(ctx)()
	outcome := "ok"
	if _, err := h.Wallets.EnsureTreasuryEVMWallet(ctx, ids.CabalIDFrom(e.CabalID)); err != nil {
		if errs.CodeOf(err) != errs.CodeNotImplemented {
			return err
		}
		observability.Info(ctx, observability.CabalTreasuryEVMWalletSkipped,
			slog.String("cabal_id", e.CabalID.String()), slog.String("code", string(errs.CodeNotImplemented)))
		outcome = string(errs.CodeNotImplemented)
	}
	return h.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		_, err := d.RecordAs(ctx, tx, outcome)
		return err
	})
}
