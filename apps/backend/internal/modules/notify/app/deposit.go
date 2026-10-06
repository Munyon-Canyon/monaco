package app

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type DepositCredited struct{}

func (DepositCredited) Name() string { return "deposit_credited" }

func (DepositCredited) Recipients(_ context.Context, e events.DepositCredited) ([]ids.UserID, error) {
	return []ids.UserID{ids.UserIDFrom(e.UserID)}, nil
}

func (k DepositCredited) Render(_ context.Context, e events.DepositCredited, _ ids.UserID) (Message, error) {
	return Message{
		Title:      "Deposit received",
		Body:       usd(e.AmountMicros) + " is in your account balance.",
		Data:       map[string]string{"kind": k.Name()},
		CollapseID: "deposit-" + e.DepositID.String(),
	}, nil
}
