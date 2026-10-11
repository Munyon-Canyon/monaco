package adapters

import (
	"context"
	"encoding/json"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type Ban struct {
	IDs ids.Generator
}

const (
	activeStatus = `{"status":"active"}`
	bannedStatus = `{"status":"banned"}`
)

func (b Ban) Handle(ctx context.Context, tx db.Tx, e events.AdminCabalBanApproved, at time.Time) error {
	banned, err := sqlc.New(tx.Queries()).BanCabal(ctx, sqlc.BanCabalParams{ID: e.CabalID, Now: at})
	if err != nil || banned == 0 {
		return err
	}
	reason, err := events.NewReason(e.Reason)
	if err != nil {
		return err
	}
	action := events.AdminAction{
		V: 1, ActionID: b.IDs.NewV7(), AdminID: e.RequestedBy, Action: events.AdminActionCabalBan,
		TargetType: events.AdminTargetCabal, TargetID: e.CabalID.String(), Reason: reason.String(),
		Before: json.RawMessage(activeStatus), After: json.RawMessage(bannedStatus), ApprovedBy: &e.ApprovedBy,
	}
	if err := tx.Events.Append(ctx, events.CabalBanned{V: 1, CabalID: e.CabalID, Reason: reason.String()}); err != nil {
		return err
	}
	return tx.Events.Append(ctx, action)
}
