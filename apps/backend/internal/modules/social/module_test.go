package social_test

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/social"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/socialapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

func TestModule_servesTheFollowRoutesAndRegistersItsConsumers(t *testing.T) {
	t.Parallel()
	m := social.New(module.Deps{})
	if m.Name() != "social" || !testkit.Serves(m.Mount, "GET", "/v1/feed") || m.Pollers() != nil ||
		len(m.Consumers()) != 3 {
		t.Fatalf("module = %s, %v pollers, %d consumers",
			m.Name(), m.Pollers(), len(m.Consumers()))
	}
}

func TestModule_readsAccountStatusFromIdentityByDefault(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	active := testkit.SeedUser(t, f.pool, testkit.UserOpts{Handle: "carol"})
	banned := testkit.SeedUser(t, f.pool, testkit.UserOpts{Handle: "dave", AccountStatus: "banned"})
	deps := module.Deps{Pool: f.pool, UoW: db.New(f.pool, f.gen, f.clock), IDs: f.gen, Clock: f.clock}
	routes := social.HTTPOf(social.New(deps))
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

func TestModule_publishesToAblyOnceItHasAKey(t *testing.T) {
	t.Parallel()
	upstreams := fakes.New()
	srv := httptest.NewServer(upstreams)
	t.Cleanup(srv.Close)
	f := newChatFixture(t)
	deps := module.Deps{
		Pool: f.pool, UoW: f.deps.UoW, IDs: f.deps.IDs, Clock: f.clock, HTTPClient: httpclient.New,
		Config: config.Config{
			Ably:     config.Ably{APIKey: "app.key:secret", RESTHost: srv.URL + "/ably"},
			Timeouts: config.Timeouts{Ably: time.Minute},
		},
	}
	users := fakes.NewIdentity([]identity.UserCard{{ID: f.member(0), Handle: "kai", DisplayName: "Kai"}}, nil)
	routes := social.HTTPOf(social.New(deps, social.WithUsers(users)))
	res, err := routes.PostChatMessage(asUser(t.Context(), f.member(0)), api.PostChatMessageRequestObject{
		Id: f.cabal.ID.UUID(), Body: &api.PostChatMessageRequest{Body: "gm"},
	})
	if err != nil {
		t.Fatal(err)
	}
	posted := res.(api.PostChatMessage201JSONResponse)
	got := upstreams.AblyPublishes()
	if len(got) != 1 || got[0].Channel != "cabal:"+f.cabal.ID.String() || got[0].Name != "message.created" ||
		got[0].Data.(map[string]any)["id"] != posted.Id.String() {
		t.Fatalf("ably saw %+v, want message.created for %s on the cabal channel", got, posted.Id)
	}
}

func TestModule_refusesAMalformedAblyKeyAtBoot(t *testing.T) {
	t.Parallel()
	defer func() {
		if got := errs.CodeOf(recoveredError(recover())); got != errs.CodeInvalidInput {
			t.Fatalf("New panicked with code %q, want invalid_input", got)
		}
	}()
	social.New(module.Deps{
		HTTPClient: httpclient.New,
		Config: config.Config{
			Ably:     config.Ably{APIKey: "no-secret", RESTHost: "rest.ably.io"},
			Timeouts: config.Timeouts{Ably: time.Second},
		},
	})
	t.Fatal("New accepted a key with no secret")
}

func recoveredError(v any) error {
	err, _ := v.(error)
	return err
}
