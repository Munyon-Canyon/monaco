package app

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type Cabals interface {
	Cabal(ctx context.Context, id ids.CabalID) (port.CabalView, error)
}
