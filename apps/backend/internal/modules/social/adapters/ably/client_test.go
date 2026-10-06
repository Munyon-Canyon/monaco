package ably_test

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/adapters/ably"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

const (
	keyName   = "appid.keyid"
	keySecret = "super-secret-key-material"
	apiKey    = keyName + ":" + keySecret
	channel   = "cabal:0190a5d0-0000-7000-8000-000000000001"
)

func newClient(t *testing.T, host string, timeout time.Duration) ably.Client {
	t.Helper()
	cfg := config.Config{
		Ably:     config.Ably{APIKey: apiKey, RESTHost: host},
		Timeouts: config.Timeouts{Ably: timeout},
	}
	client, err := ably.New(cfg, httpclient.New)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func fakeServer(t *testing.T) (*fakes.Server, *httptest.Server) {
	t.Helper()
	srv := fakes.New()
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)
	return srv, ts
}

func script(t *testing.T, base string, step fakes.Step) {
	t.Helper()
	raw, err := json.Marshal(step)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "/_script", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	hc := httpclient.New("fakes", httpclient.WithBaseURL(base), httpclient.WithTimeout(time.Minute))
	resp, err := hc.Do(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("POST /_script = %d, want 204", resp.StatusCode)
	}
}

func TestClient_Publish_sendsTheEventToTheChannel(t *testing.T) {
	t.Parallel()
	srv, ts := fakeServer(t)
	client := newClient(t, ts.URL+"/ably", time.Minute)
	data := map[string]any{"id": "m1", "body": "gm"}
	if err := client.Publish(t.Context(), channel, "message.created", data); err != nil {
		t.Fatal(err)
	}
	got := srv.AblyPublishes()
	want := fakes.AblyPublish{Channel: channel, Name: "message.created", Data: data}
	if len(got) != 1 || fmt.Sprint(got[0]) != fmt.Sprint(want) {
		t.Fatalf("published = %+v, want [%+v]", got, want)
	}
}

func TestClient_Publish_mapsAFailureToUpstreamUnavailable(t *testing.T) {
	t.Parallel()
	for _, status := range []int{http.StatusInternalServerError, http.StatusTooManyRequests, http.StatusUnauthorized} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			t.Parallel()
			srv, ts := fakeServer(t)
			script(t, ts.URL, fakes.Step{
				Route: "/ably/channels/messages", Action: fakes.ActionFail, Status: status, Times: 1,
				Body: json.RawMessage(`{"error":{"code":50000,"statusCode":500,"message":"down"}}`),
			})
			err := newClient(t, ts.URL+"/ably", time.Minute).Publish(t.Context(), channel, "message.created", "x")
			if errs.CodeOf(err) != errs.CodeUpstreamUnavailable || len(srv.AblyPublishes()) != 0 {
				t.Fatalf("Publish = %v (%s) with %d published, want upstream_unavailable and none published",
					err, errs.CodeOf(err), len(srv.AblyPublishes()))
			}
		})
	}
}

func TestClient_Publish_hitsTheDeadline(t *testing.T) {
	t.Parallel()
	_, ts := fakeServer(t)
	script(t, ts.URL, fakes.Step{Route: "/ably/channels/messages", Action: fakes.ActionHang, Times: 1})
	err := newClient(t, ts.URL+"/ably", 50*time.Millisecond).Publish(t.Context(), channel, "message.created", "x")
	if errs.CodeOf(err) != errs.CodeUpstreamTimeout {
		t.Fatalf("Publish = %v (%s), want upstream_timeout", err, errs.CodeOf(err))
	}
}

func TestClient_Publish_triesTheConfiguredHostOnce(t *testing.T) {
	t.Parallel()
	var hits atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(ts.Close)
	err := newClient(t, ts.URL, time.Minute).Publish(t.Context(), channel, "message.created", "x")
	if hits.Load() != 1 || errs.CodeOf(err) != errs.CodeUpstreamUnavailable {
		t.Fatalf("Publish = %v after %d calls, want upstream_unavailable after one", err, hits.Load())
	}
}

func TestClient_Publish_stopsCallingAnUpstreamThatKeepsFailing(t *testing.T) {
	t.Parallel()
	var hits atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(ts.Close)
	client := newClient(t, ts.URL, time.Minute)
	for range 20 {
		_ = client.Publish(t.Context(), channel, "message.created", "x")
	}
	before := hits.Load()
	err := client.Publish(t.Context(), channel, "message.created", "x")
	if hits.Load() != before || errs.CodeOf(err) != errs.CodeUpstreamUnavailable || before >= 20 {
		t.Fatalf("breaker: %d of 20 calls reached the upstream, then %v; want it open and no further call", before, err)
	}
}

func TestClient_Publish_neverLeaksTheKeySecret(t *testing.T) {
	t.Parallel()
	_, ts := fakeServer(t)
	script(t, ts.URL, fakes.Step{
		Route: "/ably/channels/messages", Action: fakes.ActionFail, Status: http.StatusUnauthorized, Times: 1,
		Body: json.RawMessage(`{"error":{"code":40101,"statusCode":401,"message":"invalid key"}}`),
	})
	err := newClient(t, ts.URL+"/ably", time.Minute).Publish(t.Context(), channel, "message.created", "x")
	if err == nil || strings.Contains(fmt.Sprintf("%v %+v", err, errs.Detail(err)), keySecret) {
		t.Fatalf("error %v carries the key secret or is nil", err)
	}
}

func TestClient_TokenRequest_signsLocallyForTheGivenChannels(t *testing.T) {
	t.Parallel()
	var hits atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits.Add(1) }))
	t.Cleanup(ts.Close)
	client := newClient(t, ts.URL, time.Minute)
	user := ids.UserIDFrom(testkit.NewIDs(1).NewV7())
	other := "cabal:0190a5d0-0000-7000-8000-000000000002"
	req, err := client.TokenRequest(t.Context(), user, []string{channel, other}, 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	assertSubscribeOnly(t, req.Capability, channel, other)
	if req.KeyName != keyName || req.ClientID != user.String() || req.TTL != (15*time.Minute).Milliseconds() ||
		req.Nonce == "" || req.Timestamp == 0 || hits.Load() != 0 {
		t.Fatalf("request = %+v with %d network calls, want key %s, client %s, 15m ttl and none",
			req, hits.Load(), keyName, user)
	}
	if want := macOf(req); req.MAC != want {
		t.Fatalf("mac = %q, want %q", req.MAC, want)
	}
}

func assertSubscribeOnly(t *testing.T, raw string, channels ...string) {
	t.Helper()
	var capability map[string][]string
	if err := json.Unmarshal([]byte(raw), &capability); err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{}
	for _, c := range channels {
		want[c] = []string{"subscribe"}
	}
	if !reflect.DeepEqual(capability, want) {
		t.Fatalf("capability = %v, want subscribe on exactly %v", capability, channels)
	}
}

func macOf(req app.TokenRequest) string {
	mac := hmac.New(sha256.New, []byte(keySecret))
	for _, field := range []any{req.KeyName, req.TTL, req.Capability, req.ClientID, req.Timestamp, req.Nonce} {
		_, _ = fmt.Fprintln(mac, field)
	}
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func TestClient_TokenRequest_usesAFreshNonceEachTime(t *testing.T) {
	t.Parallel()
	client := newClient(t, "http://127.0.0.1:1", time.Minute)
	user := ids.UserIDFrom(testkit.NewIDs(1).NewV7())
	first, err := client.TokenRequest(t.Context(), user, []string{channel}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	second, err := client.TokenRequest(t.Context(), user, []string{channel}, time.Minute)
	if err != nil || first.Nonce == second.Nonce {
		t.Fatalf("nonces %q and %q (err %v), want them to differ", first.Nonce, second.Nonce, err)
	}
}

func TestNew_refusesAnEmptyKey(t *testing.T) {
	t.Parallel()
	cfg := config.Config{Ably: config.Ably{RESTHost: "rest.ably.io"}, Timeouts: config.Timeouts{Ably: time.Second}}
	if _, err := ably.New(cfg, httpclient.New); errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("New with no key = %v, want invalid_input", err)
	}
}

func TestNew_refusesAKeyWithoutASecret(t *testing.T) {
	t.Parallel()
	for _, key := range []string{"no-colon-here", "name:"} {
		cfg := config.Config{
			Ably:     config.Ably{APIKey: key, RESTHost: "rest.ably.io"},
			Timeouts: config.Timeouts{Ably: time.Second},
		}
		if _, err := ably.New(cfg, httpclient.New); errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Fatalf("New with key %q = %v, want invalid_input", key, err)
		}
	}
}

func TestNew_refusesAHostThatIsNotAURL(t *testing.T) {
	t.Parallel()
	cfg := config.Config{
		Ably:     config.Ably{APIKey: apiKey, RESTHost: "http://bad host"},
		Timeouts: config.Timeouts{Ably: time.Second},
	}
	if _, err := ably.New(cfg, httpclient.New); errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("New with a bad host = %v, want invalid_input", err)
	}
}

func TestNoop_publishesNothingAndIssuesNoToken(t *testing.T) {
	t.Parallel()
	var port app.Realtime = ably.Noop{}
	if err := port.Publish(t.Context(), channel, "x", nil); err != nil {
		t.Fatal(err)
	}
	_, err := port.TokenRequest(t.Context(), ids.UserID{}, []string{channel}, time.Minute)
	if errs.CodeOf(err) != errs.CodeUpstreamUnavailable {
		t.Fatalf("Noop token = %v, want upstream_unavailable", err)
	}
}
