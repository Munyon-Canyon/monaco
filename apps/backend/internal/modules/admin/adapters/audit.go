package adapters

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/admin/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
)

type Audit struct{}

func (Audit) Handle(ctx context.Context, tx db.Tx, e events.AdminAction, at time.Time) error {
	return sqlc.New(tx.Queries()).InsertAdminAction(ctx, sqlc.InsertAdminActionParams{
		ID:         e.ActionID,
		AdminID:    e.AdminID,
		Action:     string(e.Action),
		TargetType: string(e.TargetType),
		TargetID:   e.TargetID,
		Reason:     e.Reason,
		Before:     e.Before,
		After:      e.After,
		ApprovedBy: nullableUUID(e.ApprovedBy),
		CreatedAt:  at,
	})
}

func nullableUUID(id *uuid.UUID) pgtype.UUID {
	if id == nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: *id, Valid: true}
}
