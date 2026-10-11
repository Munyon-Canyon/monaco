package port

import (
	"context"
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type TreasuryEVM struct {
	ensure func(ctx context.Context, cabal ids.CabalID) (multichain.EVMWallet, error)
}

func NewTreasuryEVM(ensure func(ctx context.Context, cabal ids.CabalID) (multichain.EVMWallet, error)) TreasuryEVM {
	return TreasuryEVM{ensure: ensure}
}

func (t TreasuryEVM) EnsureTreasuryEVMWallet(ctx context.Context, cabal ids.CabalID) (multichain.EVMWallet, error) {
	if t.ensure == nil {
		return multichain.EVMWallet{}, errs.New(errs.CodeNotImplemented, "cabal.TreasuryEVM",
			slog.String("cabal_id", cabal.String()))
	}
	return t.ensure(ctx, cabal)
}
