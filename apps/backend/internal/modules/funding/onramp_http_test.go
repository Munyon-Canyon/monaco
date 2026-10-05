package funding_test

import (
	"context"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/adapters"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/fundingapi"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func (f onrampFixture) http() adapters.HTTP {
	return adapters.HTTP{Create: f.create, Exchange: f.exchange, IDs: testkit.NewIDs(41)}
}

func (f onrampFixture) caller(t *testing.T) context.Context {
	t.Helper()
	return auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: f.user.ID.String()})
}

func TestOnrampHTTP_createThenExchangeRoundTripsTheAmount(t *testing.T) {
	t.Parallel()
	f := newOnrampFixture(t)
	h := f.http()
	amount := "25000000"
	resp, err := h.CreateOnrampSession(f.caller(t), api.CreateOnrampSessionRequestObject{
		Body: &api.CreateOnrampSessionJSONRequestBody{SuggestedAmountMicros: &amount},
	})
	if err != nil {
		t.Fatal(err)
	}
	created, ok := resp.(api.CreateOnrampSession201JSONResponse)
	if !ok {
		t.Fatalf("create response = %T", resp)
	}
	token := tokenOf(t, created.Url)
	out, err := h.ExchangeOnrampToken(t.Context(), api.ExchangeOnrampTokenRequestObject{
		Body: &api.ExchangeOnrampTokenJSONRequestBody{Token: token.Encode()},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, ok := out.(api.ExchangeOnrampToken200JSONResponse)
	if !ok || got.SessionId != created.SessionId || got.WalletAddress != string(f.user.Address) ||
		got.SuggestedAmountMicros == nil || *got.SuggestedAmountMicros != amount || got.UsdcMint != usdcMint {
		t.Fatalf("exchange response = %+v", out)
	}
	if _, err := h.ExchangeOnrampToken(t.Context(), api.ExchangeOnrampTokenRequestObject{
		Body: &api.ExchangeOnrampTokenJSONRequestBody{Token: token.Encode()},
	}); errs.CodeOf(err) != errs.CodeOnrampLinkInvalid {
		t.Fatalf("second exchange = %v, want onramp_link_invalid", err)
	}
}

func TestOnrampHTTP_exchangeWithoutAnAmountAnswersNull(t *testing.T) {
	t.Parallel()
	f := newOnrampFixture(t)
	_, token := f.start(t, nil)
	out, err := f.http().ExchangeOnrampToken(t.Context(), api.ExchangeOnrampTokenRequestObject{
		Body: &api.ExchangeOnrampTokenJSONRequestBody{Token: token.Encode()},
	})
	if got, ok := out.(api.ExchangeOnrampToken200JSONResponse); err != nil || !ok || got.SuggestedAmountMicros != nil {
		t.Fatalf("exchange = %+v, %v, want a null amount", out, err)
	}
}

func TestOnrampHTTP_refusesBadInputBeforeTouchingTheDatabase(t *testing.T) {
	t.Parallel()
	f := newOnrampFixture(t)
	h := f.http()
	tooBig := "99999999999999999999"
	if _, err := h.CreateOnrampSession(f.caller(t), api.CreateOnrampSessionRequestObject{
		Body: &api.CreateOnrampSessionJSONRequestBody{SuggestedAmountMicros: &tooBig},
	}); errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("create with %s = %v, want invalid_input", tooBig, err)
	}
	if _, err := h.ExchangeOnrampToken(t.Context(), api.ExchangeOnrampTokenRequestObject{
		Body: &api.ExchangeOnrampTokenJSONRequestBody{Token: "short"},
	}); errs.CodeOf(err) != errs.CodeOnrampLinkInvalid {
		t.Fatalf("exchange of a malformed token = %v, want onramp_link_invalid", err)
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
