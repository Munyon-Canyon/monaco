package social_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/socialapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

func (f chatRoutes) token(t *testing.T, user ids.UserID) (api.RealtimeTokenRequest, error) {
	t.Helper()
	res, err := f.routes.CreateRealtimeToken(asUser(t.Context(), user), api.CreateRealtimeTokenRequestObject{})
	if err != nil {
		return api.RealtimeTokenRequest{}, err
	}
	token, ok := res.(api.CreateRealtimeToken200JSONResponse)
	if !ok {
		t.Fatalf("response = %#v", res)
	}
	return api.RealtimeTokenRequest(token), nil
}

func subscribeOnly(t *testing.T, capability string, cabals ...ids.CabalID) {
	t.Helper()
	var got map[string][]string
	if err := json.Unmarshal([]byte(capability), &got); err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{}
	for _, c := range cabals {
		want[app.CabalChannel(c)] = []string{"subscribe"}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("capability = %v, want subscribe on exactly %v", got, cabals)
	}
}

func TestRealtimeToken_CapabilityPerCabal(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	third := testkit.NewCabal(t, f.pool, testkit.WithCreator(f.member(0)))
	got, err := f.token(t, f.member(0))
	if err != nil {
		t.Fatal(err)
	}
	subscribeOnly(t, got.Capability, f.cabal.ID, third.ID)
	if got.ClientId != f.member(0).String() || got.Ttl != app.RealtimeTokenTTL.Milliseconds() {
		t.Fatalf("token = %+v, want client %s and a 15 minute ttl", got, f.member(0))
	}
	other, err := f.token(t, f.member(1))
	if err != nil {
		t.Fatal(err)
	}
	subscribeOnly(t, other.Capability, f.cabal.ID)
}

func TestRealtimeToken_aMemberWhoLeftIsAbsentFromTheNextToken(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	if _, err := f.pool.Exec(t.Context(), `DELETE FROM cabal_members WHERE cabal_id = $1 AND user_id = $2`,
		f.cabal.ID.UUID(), f.member(2).UUID()); err != nil {
		t.Fatal(err)
	}
	_, err := f.token(t, f.member(2))
	if errs.CodeOf(err) != errs.CodeNoRealtimeChannels {
		t.Fatalf("token after leaving = %v, want no_realtime_channels", err)
	}
}

func TestRealtimeToken_NoCabals(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	_, err := f.token(t, f.outsider)
	if errs.CodeOf(err) != errs.CodeNoRealtimeChannels || len(f.rt.Published()) != 0 {
		t.Fatalf("token for a user in no cabal = %v, want no_realtime_channels", err)
	}
	if kind := errs.KindOf(errs.CodeNoRealtimeChannels); kind != errs.KindConflict {
		t.Fatalf("no_realtime_channels is kind %v, want conflict", kind)
	}
}

func TestRealtimeToken_requiresASignedInUser(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	_, err := f.routes.CreateRealtimeToken(t.Context(), api.CreateRealtimeTokenRequestObject{})
	if errs.CodeOf(err) != errs.CodeUnauthorized {
		t.Fatalf("token without a caller = %v, want unauthorized", err)
	}
}

func TestRealtimeToken_returnsTheMembershipLookupFailure(t *testing.T) {
	t.Parallel()
	members := fakes.NewCabal(nil, nil)
	boom := errs.New(errs.CodeUpstreamUnavailable, "test")
	members.Fail("CabalsOf", boom)
	_, err := app.NewRealtimeTokenHandler(members, &testkit.FakeRealtime{}).Handle(t.Context(), ids.UserID{})
	if !errors.Is(err, boom) || errs.CodeOf(err) != errs.CodeUpstreamUnavailable {
		t.Fatalf("err = %v, want the lookup error", err)
	}
}

func TestRealtimeToken_withoutAnAblyKeyTheUpstreamIsUnavailable(t *testing.T) {
	t.Parallel()
	f := newChatFixture(t)
	deps := module.Deps{Pool: f.pool, UoW: f.deps.UoW, IDs: f.deps.IDs, Clock: f.clock}
	routes := social.HTTPOf(social.New(deps))
	_, err := routes.CreateRealtimeToken(asUser(t.Context(), f.member(0)), api.CreateRealtimeTokenRequestObject{})
	if errs.CodeOf(err) != errs.CodeUpstreamUnavailable {
		t.Fatalf("token with the no-op realtime = %v, want upstream_unavailable", err)
	}
}
