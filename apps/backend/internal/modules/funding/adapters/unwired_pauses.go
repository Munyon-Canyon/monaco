package adapters

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type UnwiredPauses struct{}

var _ port.Pauses = UnwiredPauses{}

func (UnwiredPauses) IsPaused(context.Context, ids.CabalID) (port.Pause, error) {
	return port.Pause{}, errs.New(errs.CodeUpstreamUnavailable, "funding.UnwiredPauses.IsPaused")
}

func (UnwiredPauses) PausedCabals(context.Context) (port.PausedSet, error) {
	return port.PausedSet{}, errs.New(errs.CodeUpstreamUnavailable, "funding.UnwiredPauses.PausedCabals")
}
