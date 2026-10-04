package notify_test

import (
	"context"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/notifyapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestHTTP_refusesCallersThatAreNotAUser(t *testing.T) {
	t.Parallel()
	h := adapters.HTTP{}
	body := &api.DeviceRegistration{Token: token('a'), Environment: api.Sandbox}
	for name, tc := range map[string]struct {
		actor *auth.Actor
		want  errs.Code
	}{
		"no actor":    {nil, errs.CodeUnauthorized},
		"agent":       {&auth.Actor{Kind: auth.ActorAgent, ID: "a1"}, errs.CodeForbidden},
		"bad user id": {&auth.Actor{Kind: auth.ActorUser, ID: "u1"}, errs.CodeUnauthorized},
	} {
		ctx := t.Context()
		if tc.actor != nil {
			ctx = auth.WithActor(ctx, *tc.actor)
		}
		if _, err := h.PostDevice(ctx, api.PostDeviceRequestObject{Body: body}); errs.CodeOf(err) != tc.want {
			t.Errorf("%s: PostDevice err = %v, want %s", name, err, tc.want)
		}
		if _, err := h.DeleteDevice(
			ctx,
			api.DeleteDeviceRequestObject{Token: token('a')},
		); errs.CodeOf(
			err,
		) != tc.want {
			t.Errorf("%s: DeleteDevice err = %v, want %s", name, err, tc.want)
		}
	}
}

func TestHTTP_refusesAnUnknownEnvironment(t *testing.T) {
	t.Parallel()
	user := ids.NewUserID(testkit.NewIDs(1))
	ctx := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: user.String()})
	body := &api.DeviceRegistration{Token: token('a'), Environment: "staging"}
	if _, err := (adapters.HTTP{}).PostDevice(
		ctx,
		api.PostDeviceRequestObject{Body: body},
	); errs.CodeOf(
		err,
	) != errs.CodeInvalidInput {
		t.Fatalf("PostDevice err = %v, want invalid_input", err)
	}
}

func TestHTTP_surfacesADatabaseFailureOnDelete(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	g := testkit.NewIDs(1)
	user := ids.NewUserID(g)
	h := adapters.HTTP{
		Unregister: app.NewUnregisterDeviceHandler(db.New(pool, g, testkit.NewClock(clock.Real{}.Now()))),
	}
	ctx, cancel := context.WithCancel(auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: user.String()}))
	cancel()
	if _, err := h.DeleteDevice(ctx, api.DeleteDeviceRequestObject{Token: token('a')}); err == nil {
		t.Fatal("DeleteDevice on a canceled context = nil, want an error")
	}
}

func TestDomain_parsesEnvironmentsAndRedactsTokens(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{"sandbox", "production"} {
		if env, err := domain.ParseEnvironment(raw); err != nil || string(env) != raw {
			t.Errorf("ParseEnvironment(%q) = %q, %v", raw, env, err)
		}
	}
	if _, err := domain.ParseEnvironment("staging"); errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Errorf("ParseEnvironment(staging) err = %v, want invalid_input", err)
	}
	tok, err := domain.ParseDeviceToken(token('a'))
	if err != nil || tok.String() != token('a') {
		t.Fatalf("ParseDeviceToken = %v, %v", tok, err)
	}
	if v := tok.LogValue().String(); strings.Contains(v, token('a')) || v != "[redacted]" {
		t.Errorf("LogValue = %q, want [redacted]", v)
	}
}
