package funding_test

import (
	"context"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/adapters"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func (f onrampFixture) http() adapters.HTTP {
	return adapters.HTTP{Create: f.create, IDs: testkit.NewIDs(41)}
}

func (f onrampFixture) caller(t *testing.T) context.Context {
	t.Helper()
	return auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: f.user.ID.String()})
}

func TestOnrampHTTP_createAnswersTheSessionAndItsURL(t *testing.T) {
	t.Parallel()
	f := newOnrampFixture(t)
	amount := "25000000"
	resp, err := f.http().CreateOnrampSession(f.caller(t), api.CreateOnrampSessionRequestObject{
		Body: &api.CreateOnrampSessionJSONRequestBody{SuggestedAmountMicros: &amount},
	})
	if err != nil {
		t.Fatal(err)
	}
	created, ok := resp.(api.CreateOnrampSession201JSONResponse)
	if !ok {
		t.Fatalf("create response = %T", resp)
	}
	tokenOf(t, created.Url)
	evs := onrampEvents(t, f.pool)
	if len(evs) != 1 || evs[0].SessionID != created.SessionId || evs[0].SuggestedAmountMicros.String() != amount {
		t.Fatalf("events = %+v, want one created event for %s", evs, created.SessionId)
	}
}

func TestOnrampHTTP_createRefusesAnAmountPastUint64(t *testing.T) {
	t.Parallel()
	f := newOnrampFixture(t)
	h := f.http()
	tooBig := "99999999999999999999"
	if _, err := h.CreateOnrampSession(f.caller(t), api.CreateOnrampSessionRequestObject{
		Body: &api.CreateOnrampSessionJSONRequestBody{SuggestedAmountMicros: &tooBig},
	}); errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("create with %s = %v, want invalid_input", tooBig, err)
	}
	if got := onrampEvents(t, f.pool); len(got) != 0 {
		t.Fatalf("events = %+v, want none", got)
	}
}

func TestOnrampHTTP_createNeedsAUserCaller(t *testing.T) {
	t.Parallel()
	f := newOnrampFixture(t)
	h := f.http()
	tests := []struct {
		name  string
		actor *auth.Actor
		want  errs.Code
	}{
		{"anonymous", nil, errs.CodeUnauthorized},
		{"a system actor", &auth.Actor{Kind: auth.ActorSystem, ID: "ops"}, errs.CodeForbidden},
		{"a malformed user id", &auth.Actor{Kind: auth.ActorUser, ID: "nope"}, errs.CodeUnauthorized},
	}
	for _, tt := range tests {
		ctx := t.Context()
		if tt.actor != nil {
			ctx = auth.WithActor(ctx, *tt.actor)
		}
		_, err := h.CreateOnrampSession(ctx, api.CreateOnrampSessionRequestObject{
			Body: &api.CreateOnrampSessionJSONRequestBody{},
		})
		if errs.CodeOf(err) != tt.want {
			t.Errorf("%s: create = %v, want %s", tt.name, err, tt.want)
		}
	}
}

func TestOnrampHTTP_createSurfacesAWriteFailure(t *testing.T) {
	t.Parallel()
	f := newOnrampFixture(t)
	f.exec(t, `ALTER TABLE onramp_sessions RENAME TO onramp_sessions_gone`)
	if _, err := f.http().CreateOnrampSession(f.caller(t), api.CreateOnrampSessionRequestObject{
		Body: &api.CreateOnrampSessionJSONRequestBody{},
	}); err == nil {
		t.Fatal("create error = nil")
	}
}
