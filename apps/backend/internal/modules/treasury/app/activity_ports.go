package app

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	identityport "github.com/monaco/monaco/apps/backend/internal/modules/identity/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type UnwiredReads struct{}

var (
	_ Members = UnwiredReads{}
	_ Users   = UnwiredReads{}
)

func (UnwiredReads) IsMember(context.Context, ids.CabalID, ids.UserID) (bool, error) {
	return false, errs.New(errs.CodeUpstreamUnavailable, "treasury.UnwiredReads.IsMember")
}

func (UnwiredReads) UsersByID(context.Context, []ids.UserID) (map[ids.UserID]identityport.UserCard, error) {
	return nil, errs.New(errs.CodeUpstreamUnavailable, "treasury.UnwiredReads.UsersByID")
}
