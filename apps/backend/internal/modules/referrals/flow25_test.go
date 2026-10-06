package referrals_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/referrals"
	"github.com/monaco/monaco/apps/backend/internal/modules/social"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func referralScenario(t *testing.T, extra ...scenario.Option) *scenario.Scenario {
	t.Helper()
	return scenario.New(t, append([]scenario.Option{
		scenario.WithModules(func(d module.Deps) module.Module { return referrals.New(d) }),
	}, extra...)...)
}

func TestFlow25_AttachReferral_OK(t *testing.T) {
	t.Parallel()
	flows.F25AttachReferralOK(referralScenario(t, scenario.WithPostHog(t)))
}

func TestFlow25_AttachReferral_ReferralCodeUnknown(t *testing.T) {
	t.Parallel()
	flows.F25AttachReferralReferralCodeUnknown(referralScenario(t))
}

func TestFlow25_AttachReferral_ReferralSelf(t *testing.T) {
	t.Parallel()
	flows.F25AttachReferralReferralSelf(referralScenario(t))
}

func TestFlow25_AttachReferral_ReferralAlreadyAttached(t *testing.T) {
	t.Parallel()
	flows.F25AttachReferralReferralAlreadyAttached(referralScenario(t))
}

func TestFlow25_AttachReferral_ReferralWindowClosed(t *testing.T) {
	t.Parallel()
	flows.F25AttachReferralReferralWindowClosed(referralScenario(t))
}

func TestFlow25_AttachReferral_Unauthorized(t *testing.T) {
	t.Parallel()
	flows.F25AttachReferralUnauthorized(referralScenario(t))
}

func TestFlow25_AttachReferral_RateLimited(t *testing.T) {
	t.Parallel()
	flows.F25AttachReferralRateLimited(referralScenario(t))
}

func TestAttachReferral_Ok_SocialFollowsBothWays(t *testing.T) {
	t.Parallel()
	flows.F25AttachReferralSocialFollowsBothWays(referralScenario(t,
		scenario.WithModules(func(d module.Deps) module.Module { return social.New(d) })))
}
