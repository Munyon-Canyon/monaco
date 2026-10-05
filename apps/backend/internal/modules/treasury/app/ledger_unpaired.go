package app

import (
	"context"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
)

func (Ledger) FailUnpaired(ctx context.Context, tx db.Tx, transferID uuid.UUID) (bool, error) {
	n, err := sqlc.New(tx.Queries()).FailUnpairedUserTransfer(ctx, transferID)
	return n > 0, err
}
