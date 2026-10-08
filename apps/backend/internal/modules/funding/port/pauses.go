package port

import (
	"context"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/funding/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type Pauses interface {
	IsPaused(ctx context.Context, cabalID ids.CabalID) (Pause, error)
	PausedCabals(ctx context.Context) (PausedSet, error)
}

type PauseScope string

const (
	PauseScopeGlobal PauseScope = "global"
	PauseScopeCabal  PauseScope = "cabal"
)

type OpenPause struct {
	Reason domain.PauseReason
	Scope  PauseScope
	Since  time.Time
}

type Pause struct {
	Paused  bool
	Reasons []domain.PauseReason
	Since   time.Time
	Open    []OpenPause
}

type PausedSet struct {
	Global bool
	Cabals map[ids.CabalID][]domain.PauseReason
}
