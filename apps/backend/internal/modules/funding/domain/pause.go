package domain

import (
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type PauseReason string

const (
	PauseReasonExternalDeposit PauseReason = "external_deposit"
	PauseReasonOps             PauseReason = "ops"
)

type Pause struct {
	ID        uuid.UUID
	CabalID   *ids.CabalID
	Reason    PauseReason
	CreatedAt time.Time
}

func IsPaused(open []Pause, cabalID ids.CabalID) (bool, []PauseReason) {
	var reasons []PauseReason
	for _, global := range []bool{true, false} {
		for _, p := range open {
			applies := global && p.CabalID == nil || !global && p.CabalID != nil && *p.CabalID == cabalID
			if applies && !slices.Contains(reasons, p.Reason) {
				reasons = append(reasons, p.Reason)
			}
		}
	}
	return len(reasons) > 0, reasons
}
