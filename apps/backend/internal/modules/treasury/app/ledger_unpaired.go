package app

import (
	"context"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/platform/db"
)

func (l Ledger) FailUnpaired(ctx context.Context, tx db.Tx, transferID uuid.UUID) (bool, error) {
	row, err := l.failTransfer(ctx, tx, transferID, true)
	return row.UserRows > 0, err
}
