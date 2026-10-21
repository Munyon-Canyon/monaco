package testkit_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestFakeRealtime_recordsPublishesAndFailsOnDemand(t *testing.T) {
	t.Parallel()
	var f testkit.FakeRealtime
	boom := errs.New(errs.CodeUpstreamUnavailable, "test")
	f.FailOnce(boom)
	if err := f.Publish(t.Context(), "c", "first", 1); !errors.Is(err, boom) {
		t.Fatalf("first publish = %v, want the queued failure", err)
	}
	if err := f.Publish(t.Context(), "c", "second", map[string]int{"a": 1}); err != nil {
		t.Fatal(err)
	}
	want := []testkit.RealtimePublish{{Channel: "c", Name: "second", Data: json.RawMessage(`{"a":1}`)}}
	if got := f.Published(); !reflect.DeepEqual(got, want) {
		t.Fatalf("published = %+v, want %+v", got, want)
	}
	if err := f.Publish(t.Context(), "c", "unmarshalable", make(chan int)); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("publish of a channel value = %v, want internal", err)
	}
	f.Fail(boom)
	if err := f.Publish(t.Context(), "c", "third", nil); !errors.Is(err, boom) || len(f.Published()) != 1 {
		t.Fatalf("publish after Fail = %v with %d recorded, want the failure and one", err, len(f.Published()))
	}
}

func TestFakeRealtime_issuesASubscribeOnlyTokenRequest(t *testing.T) {
	t.Parallel()
	var f testkit.FakeRealtime
	user := ids.UserIDFrom(testkit.NewIDs(1).NewV7())
	req, err := f.TokenRequest(t.Context(), user, []string{"cabal:1"}, time.Minute)
	const capability = `{"cabal:1":["subscribe"]}`
	if err != nil || req.Capability != capability || req.ClientID != user.String() || req.TTL != 60000 {
		t.Fatalf("token request = %+v, %v, want a subscribe-only request for %s", req, err, user)
	}
}

func TestFakeRealtime_aRevokedClientHearsNothingMoreAndAFreshOneDoes(t *testing.T) {
	t.Parallel()
	var f testkit.FakeRealtime
	user := ids.UserIDFrom(testkit.NewIDs(1).NewV7())
	other := ids.UserIDFrom(testkit.NewIDs(2).NewV7())
	request := func(u ids.UserID, channels ...string) app.TokenRequest {
		req, err := f.TokenRequest(t.Context(), u, channels, time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		return req
	}
	connect := func(req app.TokenRequest) *testkit.RealtimeClient {
		c, err := f.Connect(req)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	old, bystander := connect(request(user, "a", "b")), connect(request(other, "a"))
	deaf := connect(request(user, "b"))
	publish := func(channel string) {
		if err := f.Publish(t.Context(), channel, "m", nil); err != nil {
			t.Fatal(err)
		}
	}
	publish("a")
	if err := f.RevokeTokens(t.Context(), user); err != nil {
		t.Fatal(err)
	}
	fresh := connect(request(user, "a"))
	publish("a")
	publish("b")
	got := []int{len(old.Received()), len(deaf.Received()), len(fresh.Received()), len(bystander.Received())}
	if want := []int{1, 0, 1, 2}; !reflect.DeepEqual(got, want) {
		t.Fatalf("received = %v, want %v for old, other channel, fresh and bystander", got, want)
	}
	if got := f.Revocations(); !reflect.DeepEqual(got, []ids.UserID{user}) {
		t.Fatalf("revocations = %v, want [%s]", got, user)
	}
	boom := errs.New(errs.CodeUpstreamUnavailable, "test")
	f.FailRevokeOnce(boom)
	if err := f.RevokeTokens(t.Context(), user); !errors.Is(err, boom) {
		t.Fatalf("revoke = %v, want the queued failure", err)
	}
	if _, err := f.Connect(app.TokenRequest{Capability: "nope"}); errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("connect with a broken capability = %v, want invalid_input", err)
	}
}
