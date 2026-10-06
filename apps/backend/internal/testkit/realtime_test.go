package testkit_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
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
