package testkit

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"go.uber.org/goleak"

	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
)

const (
	DefaultAckWait = 100 * time.Millisecond
	MaxAckWait     = 250 * time.Millisecond
	natsReady      = 5 * time.Second
	natsDeadline   = 30 * time.Second
)

var natsCurrent atomic.Pointer[natsServer]

type natsServer struct {
	srv     *natsserver.Server
	dir     string
	admin   *nats.Conn
	js      jetstream.JetStream
	startup time.Duration
}

func NATSServer(m *testing.M) {
	s, err := startNATS()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "testkit.NATSServer: %v\n", err)
		os.Exit(1)
	}
	natsCurrent.Store(s)
	goleak.VerifyTestMain(runThenStop{m: m, s: s})
}

type runThenStop struct {
	m *testing.M
	s *natsServer
}

func (r runThenStop) Run() int {
	code := r.m.Run()
	r.s.stop()
	return code
}

func startNATS() (*natsServer, error) {
	dir, err := os.MkdirTemp("", "monaco-nats-")
	if err != nil {
		return nil, fmt.Errorf("store dir: %w", err)
	}
	began := clock.Real{}.Now()
	srv, err := natsserver.NewServer(&natsserver.Options{
		Host: "127.0.0.1", Port: natsserver.RANDOM_PORT, JetStream: true, StoreDir: dir, NoLog: true, NoSigs: true,
	})
	if err != nil {
		_ = os.RemoveAll(dir)
		return nil, fmt.Errorf("new server: %w", err)
	}
	srv.Start()
	if !srv.ReadyForConnections(natsReady) {
		srv.Shutdown()
		_ = os.RemoveAll(dir)
		return nil, setupError(fmt.Sprintf("nats-server not ready after %s", natsReady))
	}
	startup := clock.Real{}.Now().Sub(began)
	admin, err := nats.Connect(srv.ClientURL(), nats.Name("monaco-testkit"))
	if err != nil {
		srv.Shutdown()
		_ = os.RemoveAll(dir)
		return nil, fmt.Errorf("admin connect: %w", err)
	}
	js, err := jetstream.New(admin)
	if err != nil {
		admin.Close()
		srv.Shutdown()
		_ = os.RemoveAll(dir)
		return nil, fmt.Errorf("admin jetstream: %w", err)
	}
	return &natsServer{srv: srv, dir: dir, admin: admin, js: js, startup: startup}, nil
}

func (s *natsServer) stop() {
	s.admin.Close()
	s.srv.Shutdown()
	s.srv.WaitForShutdown()
	_ = os.RemoveAll(s.dir)
}

func NATSURL() string {
	if s := natsCurrent.Load(); s != nil {
		return s.srv.ClientURL()
	}
	return ""
}

type Bus struct {
	Conn       *bus.Conn
	JS         jetstream.JetStream
	Events     string
	DeadLetter string
	Consumer   jetstream.ConsumerConfig
}

type BusOption func(*busOptions)

type busOptions struct {
	ackWait time.Duration
	busOpts []bus.Option
}

func WithAckWait(d time.Duration) BusOption {
	return func(o *busOptions) { o.ackWait = d }
}

func WithBusOptions(opts ...bus.Option) BusOption {
	return func(o *busOptions) { o.busOpts = append(o.busOpts, opts...) }
}

func NATS(t *testing.T, opts ...BusOption) Bus {
	t.Helper()
	s := natsCurrent.Load()
	if s == nil {
		t.Fatal("testkit.NATS: call testkit.NATSServer(m) from this package's TestMain")
	}
	o := busOptions{ackWait: DefaultAckWait}
	for _, opt := range opts {
		opt(&o)
	}
	consumer, err := consumerConfig(o.ackWait)
	if err != nil {
		t.Fatalf("testkit.NATS: %v", err)
	}
	ns := natsNamespace(t.Name())
	conn, err := openBus(s.srv.ClientURL(), natsDeadline, append([]bus.Option{bus.WithNamespace(ns)}, o.busOpts...))
	if err != nil {
		t.Fatalf("testkit.NATS: %v", err)
	}
	b := Bus{
		Conn: conn, JS: s.js,
		Events:     conn.Stream(bus.StreamEvents),
		DeadLetter: conn.Stream(bus.StreamDeadLetter),
		Consumer:   consumer,
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), natsDeadline)
		defer cancel()
		conn.Close(ctx)
		for _, name := range []string{b.Events, b.DeadLetter} {
			if err := s.js.DeleteStream(ctx, name); err != nil {
				t.Errorf("testkit.NATS: delete stream %s: %v", name, err)
			}
		}
	})
	return b
}

func openBus(url string, deadline time.Duration, opts []bus.Option) (*bus.Conn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), deadline)
	defer cancel()
	conn, err := bus.Connect(ctx, config.NATS{URL: url}, "test", opts...)
	if err != nil {
		return nil, err
	}
	if _, err := conn.Apply(ctx); err != nil {
		conn.Close(ctx)
		return nil, err
	}
	return conn, nil
}

func consumerConfig(ackWait time.Duration) (jetstream.ConsumerConfig, error) {
	if ackWait <= 0 || ackWait > MaxAckWait {
		return jetstream.ConsumerConfig{}, setupError(fmt.Sprintf(
			"AckWait %s is outside (0, %s]; bus tests wait on real timers", ackWait, MaxAckWait))
	}
	return jetstream.ConsumerConfig{AckPolicy: jetstream.AckExplicitPolicy, AckWait: ackWait}, nil
}

func natsNamespace(testName string) string {
	var suffix [4]byte
	_, _ = rand.Read(suffix[:])
	safe := strings.Map(func(r rune) rune {
		if r == '-' || r == '_' || ('0' <= r && r <= '9') || ('a' <= r && r <= 'z') || ('A' <= r && r <= 'Z') {
			return r
		}
		return '_'
	}, testName)
	return fmt.Sprintf("t_%s_%x", safe, suffix)
}

func StandaloneNATS(t *testing.T) string {
	t.Helper()
	s, err := startNATS()
	if err != nil {
		t.Fatalf("testkit.StandaloneNATS: %v", err)
	}
	t.Cleanup(s.stop)
	return s.srv.ClientURL()
}
