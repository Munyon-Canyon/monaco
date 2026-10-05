package apns

import (
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"syscall"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

func TestDialTLS_returnsTheConnectionWithNoErrorOnAHandshakeAndTheCodeOnARefusal(t *testing.T) {
	t.Parallel()
	srv := httptest.NewTLSServer(http.NotFoundHandler())
	t.Cleanup(srv.Close)
	cfg := srv.Client().Transport.(*http.Transport).TLSClientConfig

	conn, err := dialTLS(t.Context(), "tcp", srv.Listener.Addr().String(), cfg)
	if err != nil || conn == nil {
		t.Fatalf("dialTLS to a TLS server = %v, %v; want a connection and no error", conn, err)
	}
	_ = conn.Close()

	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	closed := ln.Addr().String()
	_ = ln.Close()
	_, err = dialTLS(t.Context(), "tcp", closed, cfg)
	if errs.CodeOf(err) != errs.CodeAPNSUnavailable || !errors.Is(err, syscall.ECONNREFUSED) {
		t.Fatalf("dialTLS to a closed port = %v, want apns_unavailable wrapping the refusal", err)
	}
}
