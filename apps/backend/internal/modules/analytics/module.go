package analytics

import (
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/analytics/adapters/exports"
	"github.com/monaco/monaco/apps/backend/internal/modules/analytics/adapters/posthog"
	"github.com/monaco/monaco/apps/backend/internal/modules/analytics/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

type Module struct {
	deps    module.Deps
	exports *Registry
}

func New(d module.Deps) *Module { return newModule(d, productExports(d)) }

func newModule(d module.Deps, r *Registry) *Module { return &Module{deps: d, exports: r} }

func productExports(d module.Deps) *Registry {
	r := NewRegistry()
	registerProposalExports(r, governance.New(d).Queries())
	registerFundingExports(r)
	registerTreasuryExports(r)
	registerCabalExports(r, cabal.New(d).Queries())
	registerSocialExports(r)
	registerIdentityExports(r)
	registerReferralExports(r)
	return r
}

func registerFundingExports(r *Registry) {
	Export(r, string(events.TypeDepositCredited), exports.DepositCredited)
	Export(r, string(events.TypeOnrampStatusChanged), exports.OnrampStatusChanged)
	Export(r, string(events.TypeWithdrawalConfirmed), exports.WithdrawalConfirmed)
}

func registerTreasuryExports(r *Registry) {
	Export(r, string(events.TypeFunded), exports.CabalFunded)
	Export(r, string(events.TypeCashOutCompleted), exports.CashOutCompleted)
	Export(r, string(events.TypeCashOutPartial), exports.CashOutPartial)
	Export(r, string(events.TypeCashOutFailed), exports.CashOutFailed)
}

func registerCabalExports(r *Registry, members app.MembershipReader) {
	c := exports.Cabals{Members: members}
	Export(r, string(events.TypeCabalCreated), c.CabalCreated)
	Export(r, string(events.TypeCabalMemberJoined), c.CabalJoined)
	Export(r, string(events.TypeCabalMemberLeft), c.CabalLeft)
}

func registerSocialExports(r *Registry) {
	Export(r, string(events.TypeFollowCreated), exports.FollowCreated)
	Export(r, string(events.TypeCommentCreated), exports.CommentCreated)
}

func registerIdentityExports(r *Registry) {
	Export(r, string(events.TypeUserCreated), exports.UserSignedUp)
	Export(r, string(events.TypeUserAuthStateChanged), exports.AuthStateChanged)
}

func registerReferralExports(r *Registry) {
	Export(r, string(events.TypeReferralAttributed), exports.ReferralAttributed)
	Export(r, string(events.TypeReferralQualified), exports.ReferralQualified)
}

func registerProposalExports(r *Registry, proposers app.ProposerReader) {
	p := exports.Proposals{Proposers: proposers}
	Export(r, string(events.TypeProposalPassed), p.ProposalPassed)
	Export(r, string(events.TypeProposalFailed), p.ProposalFailed)
	Export(r, string(events.TypeProposalExpired), p.ProposalExpired)
	Export(r, string(events.TypeTradeConfirmed), p.TradeConfirmed)
	Export(r, string(events.TypeTradeBlocked), p.TradeBlocked)
	Export(r, string(events.TypeTradeFailed), p.TradeFailed)
}

func (*Module) Name() string { return "analytics" }

func (*Module) Mount(api.Mount) {}

func (m *Module) Consumers() []bus.Consumer {
	var port app.PostHog = posthog.Noop{}
	if m.deps.Config.PostHog.APIKey != "" {
		port = posthog.New(m.deps.Config, m.deps.HTTPClient)
	}
	if c, ok := m.exports.Consumer(port); ok {
		return []bus.Consumer{c}
	}
	return nil
}

func (*Module) Pollers() []poller.Poller { return nil }
