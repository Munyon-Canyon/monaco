package app

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	identityport "github.com/monaco/monaco/apps/backend/internal/modules/identity/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type UnwiredReads struct{}

var (
	_ Members    = UnwiredReads{}
	_ Users      = UnwiredReads{}
	_ CabalViews = UnwiredReads{}
)

func (UnwiredReads) IsMember(context.Context, ids.CabalID, ids.UserID) (bool, error) {
	return false, errs.New(errs.CodeUpstreamUnavailable, "treasury.UnwiredReads.IsMember")
}

func (UnwiredReads) UsersByID(context.Context, []ids.UserID) (map[ids.UserID]identityport.UserCard, error) {
	return nil, errs.New(errs.CodeUpstreamUnavailable, "treasury.UnwiredReads.UsersByID")
}

func (UnwiredReads) Cabals(context.Context, []ids.CabalID) (map[ids.CabalID]cabalport.CabalView, error) {
	return nil, errs.New(errs.CodeUpstreamUnavailable, "treasury.UnwiredReads.Cabals")
}

func (UnwiredReads) TreasuryWallet(context.Context, ids.CabalID) (cabalport.TreasuryWallet, error) {
	return cabalport.TreasuryWallet{}, errs.New(errs.CodeUpstreamUnavailable, "treasury.UnwiredReads.TreasuryWallet")
}

func (UnwiredReads) MemberWallet(context.Context, ids.UserID) (identityport.MemberWallet, error) {
	return identityport.MemberWallet{}, errs.New(errs.CodeUpstreamUnavailable, "treasury.UnwiredReads.MemberWallet")
}
