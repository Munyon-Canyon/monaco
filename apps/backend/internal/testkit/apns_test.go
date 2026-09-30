package testkit_test

import (
	"errors"
	"net/http"
	"reflect"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/apns"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func pushTo(token string) apns.Push {
	return apns.Push{
		Token:       token,
		Environment: apns.Sandbox,
		Title:       "Trade filled",
		Data:        map[string]string{"txn_id": "42"},
	}
}

func TestFakeSender_answers200AndRecordsEveryPushInOrder(t *testing.T) {
	t.Parallel()
	var f testkit.FakeSender

	for _, token := range []string{"aa", "bb", "aa"} {
		res, err := f.Send(t.Context(), pushTo(token))
		if err != nil || res != (apns.Result{Status: http.StatusOK}) {
			t.Fatalf("Send(%s) = %+v, %v, want a plain 200", token, res, err)
		}
	}

	sent := f.Sent()
	got := make([]string, 0, len(sent))
	for _, p := range sent {
		got = append(got, p.Token)
	}
	if want := []string{"aa", "bb", "aa"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Sent tokens = %v, want %v", got, want)
	}
}

func TestFakeSender_repliesPerTokenWithAnyResult(t *testing.T) {
	t.Parallel()
	var f testkit.FakeSender
	dead := apns.Result{Status: http.StatusGone, Reason: "Unregistered"}
	f.Reply("dead", dead)

	if res, err := f.Send(t.Context(), pushTo("dead")); err != nil || res != dead {
		t.Fatalf("Send(dead) = %+v, %v, want the scripted result", res, err)
	}
	if res, err := f.Send(t.Context(), pushTo("live")); err != nil || res.Status != http.StatusOK {
		t.Fatalf("Send(live) = %+v, %v, want the default 200", res, err)
	}
}

func TestFakeSender_failsThroughFaultsAndStillRecordsTheAttempt(t *testing.T) {
	t.Parallel()
	var f testkit.FakeSender
	boom := errs.New(errs.CodeAPNSUnavailable, "test.apns")
	f.FailOnce("Send", boom)

	if res, err := f.Send(t.Context(), pushTo("aa")); !errors.Is(err, boom) || res != (apns.Result{}) {
		t.Fatalf("first Send = %+v, %v, want the queued failure", res, err)
	}
	if res, err := f.Send(t.Context(), pushTo("aa")); err != nil || res.Status != http.StatusOK {
		t.Fatalf("second Send = %+v, %v, want the failure gone", res, err)
	}
	f.Fail("Send", boom)
	if _, err := f.Send(t.Context(), pushTo("aa")); !errors.Is(err, boom) {
		t.Fatalf("third Send = %v, want the standing failure", err)
	}
	if n := len(f.Sent()); n != 3 {
		t.Fatalf("Sent has %d pushes, want every attempt recorded", n)
	}
}

func TestFakeSender_recordsACopyOfTheData(t *testing.T) {
	t.Parallel()
	var f testkit.FakeSender
	p := pushTo("aa")
	if _, err := f.Send(t.Context(), p); err != nil {
		t.Fatal(err)
	}

	p.Data["txn_id"] = "changed after the send"

	if got := f.Sent()[0].Data["txn_id"]; got != "42" {
		t.Fatalf("recorded data = %q, want the value at send time", got)
	}
}
