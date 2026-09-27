package testkit

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
)

func stallingProxy(t *testing.T, upstream string, stall time.Duration) string {
	t.Helper()
	ln, err := new(net.ListenConfig).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var (
		wg    sync.WaitGroup
		mu    sync.Mutex
		conns []net.Conn
	)
	track := func(c net.Conn) {
		mu.Lock()
		defer mu.Unlock()
		conns = append(conns, c)
	}
	wg.Go(func() {
		for {
			client, err := ln.Accept()
			if err != nil {
				return
			}
			server, err := new(net.Dialer).DialContext(t.Context(), "tcp", upstream)
			if err != nil {
				_ = client.Close()
				return
			}
			track(client)
			track(server)
			wg.Go(func() { _, _ = io.Copy(server, client) })
			wg.Go(func() { delayAfterFirstPong(client, server, stall) })
		}
	})
	t.Cleanup(func() {
		_ = ln.Close()
		mu.Lock()
		for _, c := range conns {
			_ = c.Close()
		}
		mu.Unlock()
		wg.Wait()
	})
	return "nats://" + ln.Addr().String()
}

func delayAfterFirstPong(dst io.Writer, src io.Reader, stall time.Duration) {
	buf := make([]byte, 32<<10)
	stalled, ponged := false, false
	for {
		n, err := src.Read(buf)
		if n > 0 {
			if ponged && !stalled {
				stalled = true
				<-time.After(stall)
			}
			ponged = ponged || bytes.Contains(buf[:n], []byte("PONG\r\n"))
			if _, werr := dst.Write(buf[:n]); werr != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}

func TestOpenBus_usesItsOwnDeadlineNotTheJetStreamDefault(t *testing.T) {
	t.Parallel()
	if natsDeadline < 30*time.Second {
		t.Fatalf(
			"natsDeadline = %s, want at least 30s so a loaded host does not hit the 5s client default",
			natsDeadline,
		)
	}
	s, err := startNATS()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.stop)
	upstream := strings.TrimPrefix(s.srv.ClientURL(), "nats://")

	wall := clock.Real{}
	began := wall.Now()
	if _, err := openBus(
		stallingProxy(t, upstream, 300*time.Millisecond),
		100*time.Millisecond,
		nil,
	); !errors.Is(
		err,
		context.DeadlineExceeded,
	) {
		t.Fatalf("openBus with a 100ms deadline behind a 300ms stall = %v, want deadline exceeded", err)
	}
	if took := wall.Now().Sub(began); took >= time.Second {
		t.Fatalf("openBus gave up after %s, want the 100ms deadline", took)
	}
	conn, err := openBus(stallingProxy(t, upstream, 300*time.Millisecond), 2*time.Second, nil)
	if err != nil {
		t.Fatalf("openBus with a 2s deadline behind a 300ms stall = %v, want connected", err)
	}
	conn.Close(t.Context())
}
