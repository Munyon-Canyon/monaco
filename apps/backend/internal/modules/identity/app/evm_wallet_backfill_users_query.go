package app

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type EVMWalletBackfillUser struct {
	User ids.UserID
	Held bool
}

func EVMWalletBackfillUsers(ctx context.Context, q sqlc.DBTX) ([]EVMWalletBackfillUser, error) {
	rows, err := sqlc.New(q).EVMWalletBackfillUsers(ctx)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, "identity.EVMWalletBackfillUsers")
	}
	out := make([]EVMWalletBackfillUser, len(rows))
	for i, r := range rows {
		out[i] = EVMWalletBackfillUser{User: ids.UserIDFrom(r.UserID), Held: r.HasEvmWallet}
	}
	return out, nil
}
