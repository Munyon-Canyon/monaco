package httpx

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

type closeFails struct {
	net.Listener
	err error
}

func (l closeFails) Close() error {
	_ = l.Listener.Close()
	return l.err
}

func listen(t *testing.T) net.Listener {
	t.Helper()
	ln, err := new(net.ListenConfig).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return ln
}

func getStatus(t *testing.T, url string) (int, error) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	return resp.StatusCode, nil
}

func serveUntilCancelled(t *testing.T, ln net.Listener) error {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	srv := &http.Server{Handler: http.NotFoundHandler(), ReadHeaderTimeout: time.Second}
	go func() { done <- Serve(ctx, ln, srv, time.Second) }()
	url := "http://" + ln.Addr().String() + "/"
	if code, err := getStatus(t, url); err != nil || code != http.StatusNotFound {
		t.Fatalf("GET before shutdown = %d, %v; want 404", code, err)
	}
	cancel()
	select {
	case err := <-done:
		if _, err := getStatus(t, url); err == nil {
			t.Fatal("GET succeeded after shutdown")
		}
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return within 5s of cancellation")
		return nil
	}
}

func TestServe_answersUntilTheContextEndsThenReturnsNil(t *testing.T) {
	t.Parallel()
	if err := serveUntilCancelled(t, listen(t)); err != nil {
		t.Fatalf("Serve = %v, want nil after a clean shutdown", err)
	}
}

func TestServe_reportsAShutdownThatFails(t *testing.T) {
	t.Parallel()
	closeErr := errs.New(errs.CodeInternal, "test.listenerClose")
	err := serveUntilCancelled(t, closeFails{listen(t), closeErr})
	if !errors.Is(err, closeErr) || err.Error() != "shutdown: "+closeErr.Error() {
		t.Fatalf("Serve = %v, want the shutdown error", err)
	}
}

func TestServe_returnsAtOnceWhenTheListenerIsUnusable(t *testing.T) {
	t.Parallel()
	ln := listen(t)
	_ = ln.Close()
	srv := &http.Server{Handler: http.NotFoundHandler(), ReadHeaderTimeout: time.Second}
	done := make(chan error, 1)
	go func() { done <- Serve(t.Context(), ln, srv, time.Second) }()
	select {
	case err := <-done:
		if !errors.Is(err, net.ErrClosed) {
			t.Fatalf("Serve = %v, want net.ErrClosed", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal(
			"Serve on a closed listener did not return within 5s: it swallowed the Serve error and waited for a shutdown",
		)
	}
}
