package main

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestMain(m *testing.M) {
	testkit.Main(m, testkit.NoDB(), testkit.WithChild(main))
}

func do(ctx context.Context, t *testing.T, method, url, body string) (int, string) {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, method, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, string(b)
}

func TestServe_replaysFixturesAcceptsScriptsAndStopsOnCancel(t *testing.T) {
	t.Parallel()
	ln, err := new(net.ListenConfig).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	base := "http://" + ln.Addr().String()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- serve(ctx, ln) }()

	if code, body := do(t.Context(), t, http.MethodGet, base+"/privy/_health", ""); code != http.StatusOK ||
		!strings.Contains(body, `"upstream": "privy"`) {
		t.Fatalf("GET /privy/_health = %d %q", code, body)
	}
	if code, _ := do(t.Context(), t, http.MethodPost, base+"/_script",
		`{"route":"/privy/_health","action":"hang"}`); code != http.StatusNoContent {
		t.Fatalf("POST /_script = %d, want 204", code)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serve returned %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serve did not stop within 5s")
	}
}

func TestRun_reportsAnAddressItCannotListenOn(t *testing.T) {
	t.Parallel()
	err := run(t.Context(), []string{"FAKES_ADDR=256.0.0.1:1"})
	if err == nil || !strings.Contains(err.Error(), "listen on 256.0.0.1:1") {
		t.Fatalf("run = %v, want a listen error", err)
	}
}

func TestMain_servesFixturesUntilSIGTERMThenExitsZero(t *testing.T) {
	t.Parallel()
	p := testkit.StartMain(t, []string{"FAKES_ADDR=127.0.0.1:0"})
	if code, body := testkit.Get(t, "http://"+p.Addr+"/privy/_health"); code != http.StatusOK ||
		!strings.Contains(body, `"upstream": "privy"`) {
		t.Fatalf("GET /privy/_health = %d %q", code, body)
	}
	if stderr, err := p.Terminate(); err != nil {
		t.Fatalf("fakes after SIGTERM: %v\n%s", err, stderr)
	}
}

func TestMain_exitsOneAndLogsWhyWhenItCannotListen(t *testing.T) {
	t.Parallel()
	out, err := testkit.MainCommand(t, []string{"FAKES_ADDR=256.0.0.1:1"}).CombinedOutput()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 1 ||
		!strings.Contains(
			string(out),
			`"msg":"boot.stopped"`,
		) || !strings.Contains(string(out), "listen on 256.0.0.1:1") {
		t.Fatalf("fakes on a bad address = %v\n%s", err, out)
	}
}

type closeFails struct {
	net.Listener
	err error
}

func (l closeFails) Close() error {
	_ = l.Listener.Close()
	return l.err
}

func TestServe_reportsACloseThatFails(t *testing.T) {
	t.Parallel()
	ln, err := new(net.ListenConfig).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	closeErr := errs.New(errs.CodeInternal, "test.listenerClose")
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- serve(ctx, closeFails{ln, closeErr}) }()
	if code, _ := do(
		t.Context(),
		t,
		http.MethodGet,
		"http://"+ln.Addr().String()+"/privy/_health",
		"",
	); code != http.StatusOK {
		t.Fatalf("GET /privy/_health = %d, want 200", code)
	}
	cancel()
	if err := <-done; !errors.Is(err, closeErr) || err.Error() != "close: "+closeErr.Error() {
		t.Fatalf("serve = %v, want the close error", err)
	}
}

func TestServe_returnsAtOnceWhenTheListenerIsUnusable(t *testing.T) {
	t.Parallel()
	ln, err := new(net.ListenConfig).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_ = ln.Close()
	if err := serve(t.Context(), ln); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("serve = %v, want net.ErrClosed", err)
	}
}
