package adapters

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
)

type EVMSend struct {
	Wallets multichain.EVMWallets
}

func (s EVMSend) Send(
	ctx context.Context,
	walletID string,
	tx multichain.EVMTransaction,
	idemKey string,
) (string, error) {
	tx.IdempotencyKey = idemKey
	return s.Wallets.SendEVMTransaction(ctx, walletID, tx, true)
}
