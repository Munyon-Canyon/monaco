package social_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/social"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func withSocial() scenario.Option {
	return scenario.WithModules(func(d module.Deps) module.Module { return social.New(d) })
}

func TestFlow20_Follow_OK(t *testing.T) {
	t.Parallel()
	flows.F20FollowOK(scenario.New(t, withSocial(), scenario.WithPostHog(t)))
}

func TestFlow20_Follow_CannotFollowSelf(t *testing.T) {
	t.Parallel()
	flows.F20FollowCannotFollowSelf(scenario.New(t, withSocial()))
}

func TestFlow20_Follow_UserNotFound(t *testing.T) {
	t.Parallel()
	flows.F20FollowUserNotFound(scenario.New(t, withSocial()))
}

func TestFlow20_Follow_UserBanned(t *testing.T) {
	t.Parallel()
	flows.F20FollowUserBanned(scenario.New(t, withSocial()))
}

func TestFlow20_Follow_FollowBlocked(t *testing.T) {
	t.Parallel()
	flows.F20FollowFollowBlocked(scenario.New(t, withSocial()))
}

func TestFlow20_Follow_Unauthorized(t *testing.T) {
	t.Parallel()
	flows.F20FollowUnauthorized(scenario.New(t, withSocial()))
}

func TestFlow20Unfollow_OK(t *testing.T) {
	t.Parallel()
	flows.F20UnfollowOK(scenario.New(t, withSocial()))
}
