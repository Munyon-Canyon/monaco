package app

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/modules/admin/sqlc"
	treasuryport "github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const recentLimit = 20

func recentActions(ctx context.Context, log ActionLog, targetType, targetID string) ([]sqlc.AdminAction, error) {
	return log.Recent(ctx, targetType, targetID, recentLimit)
}

func ShortAddress(address string) string {
	const edge = 4
	if len(address) <= 2*edge {
		return address
	}
	return address[:edge] + "…" + address[len(address)-edge:]
}

type TxnHeader struct {
	ID          uuid.UUID
	Scope       string
	Kind        string
	Status      string
	CabalID     *ids.CabalID
	TxSignature string
	CreatedAt   time.Time
}

func txnHeaders(rows []treasuryport.TxnHeader) []TxnHeader {
	out := make([]TxnHeader, len(rows))
	for i, r := range rows {
		out[i] = txnHeader(r)
	}
	return out
}

func txnHeader(r treasuryport.TxnHeader) TxnHeader {
	return TxnHeader{
		ID: r.ID, Scope: string(r.Scope), Kind: r.Kind, Status: r.Status, CabalID: r.CabalID,
		TxSignature: string(r.TxSignature), CreatedAt: r.CreatedAt,
	}
}
