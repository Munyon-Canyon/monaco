package apns_test

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"syscall"
	"testing"
	"testing/synctest"
	"time"

	"github.com/sideshow/apns2"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/apns"
)

func silentPeer(done chan<- struct{}) func(context.Context, string, string, *tls.Config) (net.Conn, error) {
	return func(ctx context.Context, _, _ string, cfg *tls.Config) (net.Conn, error) {
		defer close(done)
		ctx, cancel := context.WithTimeout(ctx, apns2.TLSDialTimeout)
		defer cancel()
		client, server := net.Pipe()
		go func() { _, _ = io.Copy(io.Discard, server) }()
		conn := tls.Client(client, cfg)
		if err := conn.HandshakeContext(ctx); err != nil {
			return nil, err
		}
		return conn, nil
	}
}

func closedLoopback(t *testing.T) string {
	t.Helper()
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatal(err)
	}
	return "https://" + addr
}

func TestSend_aHandshakeThatNeverCompletesEndsAtTheSendDeadline(t *testing.T) {
	t.Parallel()
	host := closedLoopback(t)
	synctest.Test(t, func(t *testing.T) {
		dialGaveUp := make(chan struct{})
		c, err := apns.New(testConfig(), apns.WithDialTLS(silentPeer(dialGaveUp)))
		if err != nil {
			t.Fatal(err)
		}
		apns.Retarget(c, host)
		start := now()

		_, err = c.Send(t.Context(), push(apns.Sandbox))

		wantCode(t, err, errs.CodeAPNSUnavailable)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("err = %v, want the deadline in the chain", err)
		}
		if got := now().Sub(start); got != 10*time.Second {
			t.Fatalf("Send returned after %s, want the 10s MONACO_TIMEOUT_APNS, not apns2's 20s dial timeout", got)
		}
		<-dialGaveUp
	})
}

func TestSend_anAPNsThatRefusesTheConnectionIsUnavailable(t *testing.T) {
	t.Parallel()
	c, err := apns.New(testConfig())
	if err != nil {
		t.Fatal(err)
	}
	apns.Retarget(c, closedLoopback(t))

	_, err = c.Send(t.Context(), push(apns.Production))

	wantCode(t, err, errs.CodeAPNSUnavailable)
	if !errors.Is(err, syscall.ECONNREFUSED) {
		t.Fatalf("err = %v, want the refused dial in the chain", err)
	}
}
