package analytics

import (
	"github.com/monaco/monaco/apps/backend/internal/modules/analytics/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
)

func NewWithExports(d module.Deps, r *Registry) *Module { return newModule(d, r) }

func RegisterProposalExports(r *Registry, proposers app.ProposerReader) {
	registerProposalExports(r, proposers)
}

func RegisterFundingExports(r *Registry) { registerFundingExports(r) }

func RegisterTreasuryExports(r *Registry) { registerTreasuryExports(r) }

func RegisterCabalExports(r *Registry, members app.MembershipReader) {
	registerCabalExports(r, members)
}

func RegisterSocialExports(r *Registry) { registerSocialExports(r) }

func RegisterIdentityExports(r *Registry) { registerIdentityExports(r) }
