package app

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type EVMWalletBackfillCabal struct {
	Cabal ids.CabalID
	Held  bool
}

func EVMWalletBackfillCabals(ctx context.Context, q sqlc.DBTX) ([]EVMWalletBackfillCabal, error) {
	rows, err := sqlc.New(q).EVMWalletBackfillCabals(ctx)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, "cabal.EVMWalletBackfillCabals")
	}
	out := make([]EVMWalletBackfillCabal, len(rows))
	for i, r := range rows {
		out[i] = EVMWalletBackfillCabal{Cabal: ids.CabalIDFrom(r.CabalID), Held: r.HasEvmWallet}
	}
	return out, nil
}
