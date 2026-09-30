package apns_test

import (
	"bytes"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/apns"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability/boundary"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func logged(t *testing.T, err error) []byte {
	t.Helper()
	logs := &testkit.Logs{}
	ctx := observability.WithLogger(t.Context(), observability.NewLogger(config.Config{Env: config.EnvTest}, logs))
	boundary.Stopped(ctx, "apns", err)
	if !bytes.Contains(logs.Bytes(), []byte(`"msg":"boot.stopped"`)) {
		t.Fatalf("nothing was logged for %v", err)
	}
	return logs.Bytes()
}

func TestARawTransportErrorWouldLeakTheTokenPastTheLogRedactor(t *testing.T) {
	t.Parallel()
	raw := &url.Error{
		Op:  "Post",
		URL: "https://api.push.apple.com/3/device/" + testToken,
		Err: errs.New(errs.CodeInternal, "test.dial"),
	}

	line := logged(t, errs.Wrap(raw, errs.CodeAPNSUnavailable, "apns.Send"))

	if !bytes.Contains(line, []byte(testToken)) {
		t.Fatalf("log line %s hides the token, so the redaction test below proves nothing", line)
	}
}

func TestSend_neverPutsTheDeviceTokenInAnErrorOrALogLine(t *testing.T) {
	t.Parallel()
	failures := map[string]func(t *testing.T) error{
		"network failure": func(t *testing.T) error {
			t.Helper()
			_, err := newClient(t, &upstream{replies: []reply{unreachable}}).Send(t.Context(), push(apns.Sandbox))
			return err
		},
		"body that is not json": func(t *testing.T) error {
			t.Helper()
			u := &upstream{replies: []reply{respond(http.StatusBadGateway, "<html>bad gateway</html>")}}
			_, err := newClient(t, u).Send(t.Context(), push(apns.Sandbox))
			return err
		},
		"refused push": func(t *testing.T) error {
			t.Helper()
			_, err := newClient(t, &upstream{replies: []reply{accepted()}}).Send(t.Context(), push("staging"))
			return err
		},
	}
	for name, fail := range failures {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			err := fail(t)
			if err == nil {
				t.Fatal("Send succeeded, want a failure to inspect")
			}
			for _, text := range []string{err.Error(), fmt.Sprint(errs.Detail(err)), string(logged(t, err))} {
				if strings.Contains(text, testToken) {
					t.Fatalf("%q contains the device token", text)
				}
			}
		})
	}
}
