package social_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/social"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
)

func TestModule_servesTheFollowRoutesAndRunsNoConsumersOrPollers(t *testing.T) {
	t.Parallel()
	m := social.New(module.Deps{})
	var routes httpx.Routes
	m.Routes(&routes)
	if m.Name() != "social" || routes.SocialRoutes == nil || m.Pollers() != nil || len(m.Consumers()) != 0 {
		t.Fatalf("module = %s, routes %v, %v pollers, %d consumers",
			m.Name(), routes.SocialRoutes, m.Pollers(), len(m.Consumers()))
	}
}
