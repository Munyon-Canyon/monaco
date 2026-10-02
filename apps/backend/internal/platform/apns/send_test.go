package apns_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/apns"
)

func TestSend_postsTheNotificationAPNsExpects(t *testing.T) {
	t.Parallel()
	u := &upstream{replies: []reply{accepted()}}
	c := newClient(t, u)

	res := mustSend(t, c, push(apns.Sandbox))

	if want := (apns.Result{Status: 200, APNsID: apnsID}); res != want {
		t.Fatalf("Send = %+v, want %+v", res, want)
	}
	req := u.requests()[0]
	if got, want := req.url.String(), "https://api.sandbox.push.apple.com/3/device/"+testToken; got != want {
		t.Fatalf("url = %s, want %s", got, want)
	}
	for header, want := range map[string]string{
		"Apns-Topic":       topic,
		"Apns-Collapse-Id": "trade-42",
		"Apns-Push-Type":   "alert",
		"Content-Type":     "application/json; charset=utf-8",
	} {
		if got := req.header.Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
	verifyBearer(t, req.header.Get("Authorization"))
	var got map[string]any
	if err := json.Unmarshal(req.body, &got); err != nil {
		t.Fatalf("body %q: %v", req.body, err)
	}
	want := map[string]any{
		"aps": map[string]any{
			"alert": map[string]any{"title": "Trade filled", "body": "Your cabal bought $50.00 of Tesla"},
		},
		"cabal_id": "c1",
		"txn_id":   "42",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("payload = %s, want custom keys at the top level and no badge: %v", req.body, want)
	}
}

func TestSend_picksTheHostFromThePushEnvironment(t *testing.T) {
	t.Parallel()
	for env, host := range map[apns.Environment]string{
		apns.Sandbox:    "api.sandbox.push.apple.com",
		apns.Production: "api.push.apple.com",
	} {
		t.Run(string(env), func(t *testing.T) {
			t.Parallel()
			u := &upstream{replies: []reply{accepted()}}
			mustSend(t, newClient(t, u), push(env))
			if got := u.requests()[0].url.Host; got != host {
				t.Fatalf("host = %s, want %s", got, host)
			}
		})
	}
}

func TestSend_withABaseURLSpeaksPlainHTTPToThatServerForBothEnvironments(t *testing.T) {
	t.Parallel()
	paths := make(chan string, 2)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths <- r.URL.Path
		w.Header().Set("apns-id", apnsID)
	}))
	t.Cleanup(srv.Close)
	cfg := testConfig()
	cfg.APNs.BaseURL = srv.URL + "/apns/"
	c, err := apns.New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	for _, env := range []apns.Environment{apns.Sandbox, apns.Production} {
		if res := mustSend(t, c, push(env)); res.Status != http.StatusOK || res.APNsID != apnsID {
			t.Fatalf("%s: Send = %+v, want 200 with the apns-id", env, res)
		}
		if got, want := <-paths, "/apns/3/device/"+testToken; got != want {
			t.Fatalf("%s: path = %s, want %s", env, got, want)
		}
	}
}

func TestSend_carriesRetryAfterOnlyInWholeSeconds(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		header string
		want   time.Duration
	}{
		{"seconds", "3", 3 * time.Second},
		{"zero", "0", 0},
		{"absent", "", 0},
		{"a http date", time.Unix(0, 0).UTC().Format(http.TimeFormat), 0},
		{"negative", "-5", 0},
		{"an hour", "3600", time.Hour},
		{"past an hour is capped", "86400", time.Hour},
		{"past any width is still capped", "99999999999", time.Hour},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			u := &upstream{replies: []reply{respond(http.StatusTooManyRequests, `{"reason":"TooManyRequests"}`,
				"Retry-After", tt.header)}}
			if got := mustSend(t, newClient(t, u), push(apns.Sandbox)).RetryAfter; got != tt.want {
				t.Fatalf("RetryAfter = %s, want %s for %q", got, tt.want, tt.header)
			}
		})
	}
}

func TestSend_refusesAPushThatCannotBeBuilt(t *testing.T) {
	t.Parallel()
	withAPS := push(apns.Sandbox)
	withAPS.Data = map[string]string{"aps": "clobbered"}
	tests := map[string]apns.Push{
		"unknown environment": push("staging"),
		"no environment":      push(""),
		"data that sets aps":  withAPS,
	}
	for name, p := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			u := &upstream{replies: []reply{accepted()}}
			res, err := newClient(t, u).Send(t.Context(), p)
			wantCode(t, err, errs.CodeInvalidInput)
			if res != (apns.Result{}) || u.count() != 0 {
				t.Fatalf("Send = %+v after %d requests, want nothing sent", res, u.count())
			}
		})
	}
}

func TestSend_answersATokenThatCannotBeADeviceTokenAsAPNsWould(t *testing.T) {
	t.Parallel()
	for _, token := range []string{"", "abc-def", "../3/device/x?y#z", "12 34", "zz", "0x1f"} {
		t.Run(token, func(t *testing.T) {
			t.Parallel()
			u := &upstream{replies: []reply{accepted()}}
			p := push(apns.Sandbox)
			p.Token = token
			res := mustSend(t, newClient(t, u), p)
			if want := (apns.Result{Status: 400, Reason: "BadDeviceToken"}); res != want {
				t.Fatalf("Send = %+v, want %+v", res, want)
			}
			if apns.Classify(res) != apns.TokenDead || u.count() != 0 {
				t.Fatalf(
					"outcome %d after %d requests, want a dead token and nothing sent",
					apns.Classify(res),
					u.count(),
				)
			}
		})
	}
}

func TestSend_acceptsAnyHexToken(t *testing.T) {
	t.Parallel()
	for _, token := range []string{strings.ToUpper(testToken), "abc", "0"} {
		t.Run(token, func(t *testing.T) {
			t.Parallel()
			u := &upstream{replies: []reply{accepted()}}
			p := push(apns.Sandbox)
			p.Token = token
			mustSend(t, newClient(t, u), p)
			if got, want := u.requests()[0].url.Path, "/3/device/"+token; got != want {
				t.Fatalf("path = %s, want %s", got, want)
			}
		})
	}
}

func TestSend_aBodyThatIsNotJSONIsUnavailableNotAnAnswer(t *testing.T) {
	t.Parallel()
	u := &upstream{replies: []reply{respond(http.StatusBadGateway, "<html>bad gateway</html>")}}

	res, err := newClient(t, u).Send(t.Context(), push(apns.Sandbox))

	wantCode(t, err, errs.CodeAPNSUnavailable)
	if !errs.Retryable(errs.CodeOf(err)) || res != (apns.Result{}) {
		t.Fatalf("Send = %+v, %v, want a retryable failure with no result", res, err)
	}
}

func TestSend_answersACollapseIDAPNSWouldRefuseLocally(t *testing.T) {
	t.Parallel()
	for name, id := range map[string]string{
		"over 64 bytes":     strings.Repeat("a", 65),
		"a control byte":    "trade-\n42",
		"a carriage return": "trade-42\r",
		"a leading control": "\x00trade-42",
		"only a control":    "\t",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			u := &upstream{replies: []reply{accepted()}}
			p := push(apns.Sandbox)
			p.CollapseID = id

			res := mustSend(t, newClient(t, u), p)

			if want := (apns.Result{Status: 400, Reason: "BadCollapseId"}); res != want {
				t.Fatalf("Send = %+v, want %+v", res, want)
			}
			if apns.Classify(res) != apns.Rejected || u.count() != 0 {
				t.Fatalf(
					"outcome %d after %d requests, want a rejected push and nothing sent",
					apns.Classify(res),
					u.count(),
				)
			}
		})
	}
}

func TestSend_sendsACollapseIDOfExactly64Bytes(t *testing.T) {
	t.Parallel()
	u := &upstream{replies: []reply{accepted()}}
	p := push(apns.Sandbox)
	p.CollapseID = strings.Repeat("a", 64)

	mustSend(t, newClient(t, u), p)

	if got := u.requests()[0].header.Get("Apns-Collapse-Id"); got != p.CollapseID {
		t.Fatalf("apns-collapse-id = %q, want the 64 byte id sent as is", got)
	}
}
