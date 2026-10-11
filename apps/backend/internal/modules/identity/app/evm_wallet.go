package app

import (
	"context"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/identity/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type EVMWalletStore interface {
	LockUser(ctx context.Context, q sqlc.DBTX, user ids.UserID) error
	StoredEVMWallet(ctx context.Context, q sqlc.DBTX, user ids.UserID) (chain.EVMAddress, bool, error)
	MemberAddress(ctx context.Context, q sqlc.DBTX, user ids.UserID) (chain.SolanaAddress, error)
	SetEVMWallet(ctx context.Context, q sqlc.DBTX, user ids.UserID, wallet multichain.EVMWallet, now time.Time) error
}

type EVMDepositWalletDeps struct {
	UoW        *db.UnitOfWork
	Reads      sqlc.DBTX
	Store      EVMWalletStore
	Wallets    multichain.EVMWallets
	Forwarding multichain.DepositForwarding
	Clock      clock.Clock
}

type EVMDepositWallets struct{ d EVMDepositWalletDeps }

var _ port.EVMDepositWallets = (*EVMDepositWallets)(nil)

func NewEVMDepositWallets(d EVMDepositWalletDeps) *EVMDepositWallets { return &EVMDepositWallets{d: d} }

func (e *EVMDepositWallets) EnsureEVMDepositWallet(ctx context.Context, user ids.UserID) (chain.EVMAddress, error) {
	if got, found, err := e.d.Store.StoredEVMWallet(ctx, e.d.Reads, user); err != nil || found {
		return got, err
	}
	var out chain.EVMAddress
	err := e.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		q := tx.Queries()
		if err := e.d.Store.LockUser(ctx, q, user); err != nil {
			return err
		}
		got, found, err := e.d.Store.StoredEVMWallet(ctx, q, user)
		if err != nil || found {
			out = got
			return err
		}
		member, err := e.d.Store.MemberAddress(ctx, q, user)
		if err != nil {
			return err
		}
		wallet, err := e.d.Wallets.CreateAppEVMWallet(ctx, "evm-deposit:"+user.String())
		if err != nil {
			return err
		}
		if err := e.d.Forwarding.Attach(ctx, wallet.ID, member); err != nil {
			return err
		}
		if err := e.d.Store.SetEVMWallet(ctx, q, user, wallet, e.d.Clock.Now().UTC()); err != nil {
			return err
		}
		out = wallet.Address
		return nil
	})
	return out, err
}
