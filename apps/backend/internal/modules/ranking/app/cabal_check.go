package app

import (
	"context"

	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type CabalStatus interface {
	Status(context.Context, ids.CabalID) (cabalport.Status, error)
}

type CabalCheck func(context.Context, ids.CabalID) error

func CheckCabal(cabals CabalStatus) CabalCheck {
	return func(ctx context.Context, id ids.CabalID) error {
		_, err := cabals.Status(ctx, id)
		return err
	}
}
