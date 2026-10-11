package adapters

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type EVMDepositWallet struct {
	UserID   ids.UserID
	WalletID string
}

type EnrolledEVMWallets struct{ q *sqlc.Queries }

func NewEnrolledEVMWallets(db sqlc.DBTX) EnrolledEVMWallets {
	return EnrolledEVMWallets{q: sqlc.New(db)}
}

func (w EnrolledEVMWallets) EVMDepositWallets(
	ctx context.Context, after ids.UserID, limit int,
) ([]EVMDepositWallet, error) {
	rows, err := w.q.EVMDepositWalletsAfter(ctx, sqlc.EVMDepositWalletsAfterParams{
		After: after.UUID(), PageSize: int64(limit),
	})
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, "identity.EVMDepositWallets")
	}
	page := make([]EVMDepositWallet, len(rows))
	for i, row := range rows {
		page[i] = EVMDepositWallet{UserID: ids.UserIDFrom(row.UserID), WalletID: row.EvmPrivyWalletID}
	}
	return page, nil
}
