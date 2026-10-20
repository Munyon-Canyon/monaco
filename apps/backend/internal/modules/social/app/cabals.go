package app

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type Cabals interface {
	Cabal(ctx context.Context, id ids.CabalID) (port.CabalView, error)
}

type CabalPictures interface {
	Cabals(ctx context.Context, cabalIDs []ids.CabalID) (map[ids.CabalID]port.CabalView, error)
}
