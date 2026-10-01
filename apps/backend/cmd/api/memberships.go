package main

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type cabalMemberships struct {
	cabals cabal.Queries
}

func (m cabalMemberships) CabalIDs(ctx context.Context, user ids.UserID) ([]ids.CabalID, error) {
	return m.cabals.CabalsOf(ctx, user)
}
