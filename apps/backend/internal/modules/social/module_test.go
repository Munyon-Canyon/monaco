package social_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
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

func TestModule_readsAccountStatusFromIdentityByDefault(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	active := testkit.SeedUser(t, f.pool, testkit.UserOpts{Handle: "carol"})
	banned := testkit.SeedUser(t, f.pool, testkit.UserOpts{Handle: "dave", AccountStatus: "banned"})
	deps := module.Deps{Pool: f.pool, UoW: db.New(f.pool, f.gen, f.clock), IDs: f.gen, Clock: f.clock}
	var routes httpx.Routes
	social.New(deps).Routes(&routes)
	ctx := asUser(t.Context(), f.alice)
	if _, err := routes.PostUserFollow(ctx, followReq(active.ID, nil)); err != nil {
		t.Fatal(err)
	}
	_, err := routes.PostUserFollow(ctx, followReq(banned.ID, nil))
	wantCode(t, err, errs.CodeUserBanned)
	if live, _ := f.rows(t); live != 1 {
		t.Fatalf("live follows = %d, want 1", live)
	}
}
