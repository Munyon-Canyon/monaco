package testkit

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
)

const (
	DefaultAckWait = 100 * time.Millisecond
	MaxAckWait     = 250 * time.Millisecond
	natsReady      = 5 * time.Second
	natsDeadline   = 30 * time.Second
	natsMaxStore   = 1 << 40
)

var natsCurrent atomic.Pointer[natsServer]

type natsServer struct {
	srv     *natsserver.Server
	dir     string
	admin   *nats.Conn
	js      jetstream.JetStream
	startup time.Duration
}

func startNATS() (*natsServer, error) {
	dir, err := os.MkdirTemp("", "monaco-nats-")
	if err != nil {
		return nil, fmt.Errorf("store dir: %w", err)
	}
	began := clock.Real{}.Now()
	srv, err := natsserver.NewServer(&natsserver.Options{
		Host:              "127.0.0.1",
		Port:              natsserver.RANDOM_PORT,
		JetStream:         true,
		JetStreamMaxStore: natsMaxStore,
		StoreDir:          dir,
		NoLog:             true,
		NoSigs:            true,
	})
	if err != nil {
		_ = os.RemoveAll(dir)
		return nil, fmt.Errorf("new server: %w", err)
	}
	srv.SetLoggerV2(stderrProblems{}, false, false, false)
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
	if err := keepStreamsDirNonEmpty(js); err != nil {
		admin.Close()
		srv.Shutdown()
		_ = os.RemoveAll(dir)
		return nil, err
	}
	return &natsServer{srv: srv, dir: dir, admin: admin, js: js, startup: startup}, nil
}

func keepStreamsDirNonEmpty(js jetstream.JetStream) error {
	ctx, cancel := context.WithTimeout(context.Background(), natsReady)
	defer cancel()
	_, err := js.CreateStream(ctx, jetstream.StreamConfig{
		Name: "TESTKIT_KEEPALIVE", Subjects: []string{"testkit.keepalive"}, Storage: jetstream.FileStorage,
	})
	if err != nil {
		return fmt.Errorf("keepalive stream: %w", err)
	}
	return nil
}

type stderrProblems struct{}

func (stderrProblems) Noticef(string, ...any) {}
func (stderrProblems) Debugf(string, ...any)  {}
func (stderrProblems) Tracef(string, ...any)  {}
func (stderrProblems) Warnf(format string, v ...any) {
	_, _ = fmt.Fprintf(os.Stderr, "nats-server warning: "+format+"\n", v...)
}

func (stderrProblems) Errorf(format string, v ...any) {
	_, _ = fmt.Fprintf(os.Stderr, "nats-server error: "+format+"\n", v...)
}

func (stderrProblems) Fatalf(format string, v ...any) {
	_, _ = fmt.Fprintf(os.Stderr, "nats-server fatal: "+format+"\n", v...)
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
		t.Fatal("testkit.NATS: call testkit.Main(m, testkit.WithNATS()) from this package's TestMain")
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

func StoppableNATS(t *testing.T) (url string, stop func()) {
	t.Helper()
	s, err := startNATS()
	if err != nil {
		t.Fatalf("testkit.StoppableNATS: %v", err)
	}
	var once sync.Once
	stop = func() { once.Do(s.stop) }
	t.Cleanup(stop)
	return s.srv.ClientURL(), stop
}

func NATSSubscriptions(t *testing.T, subject string) int {
	t.Helper()
	s := natsCurrent.Load()
	if s == nil {
		t.Fatal("testkit.NATSSubscriptions: call testkit.Main(m, testkit.WithNATS()) from this package's TestMain")
	}
	subsz, err := s.srv.Subsz(&natsserver.SubszOptions{Subscriptions: true, Limit: 1 << 16})
	if err != nil {
		t.Fatalf("testkit.NATSSubscriptions: %v", err)
	}
	n := 0
	for _, sub := range subsz.Subs {
		if sub.Subject == subject {
			n++
		}
	}
	return n
}

func StandaloneNATSWithStream(t *testing.T, name string, subjects ...string) string {
	t.Helper()
	url := StandaloneNATS(t)
	nc, err := nats.Connect(url, nats.Name("monaco-testkit"))
	if err != nil {
		t.Fatalf("testkit.StandaloneNATSWithStream: %v", err)
	}
	defer nc.Close()
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatalf("testkit.StandaloneNATSWithStream: %v", err)
	}
	if _, err := js.CreateStream(t.Context(), jetstream.StreamConfig{Name: name, Subjects: subjects}); err != nil {
		t.Fatalf("testkit.StandaloneNATSWithStream: %v", err)
	}
	return url
}

type EmbeddedNATS struct {
	URL string
	JS  jetstream.JetStream
	s   *natsServer
}

func (e *EmbeddedNATS) Connect(name string) (*nats.Conn, error) {
	conn, err := nats.Connect(e.URL, nats.Name(name))
	if err != nil {
		return nil, fmt.Errorf("connect %s: %w", name, err)
	}
	return conn, nil
}

func StartEmbeddedNATS() (*EmbeddedNATS, error) {
	s, err := startNATS()
	if err != nil {
		return nil, err
	}
	conn, err := openBus(s.srv.ClientURL(), natsDeadline, nil)
	if err != nil {
		s.stop()
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), natsDeadline)
	defer cancel()
	conn.Close(ctx)
	return &EmbeddedNATS{URL: s.srv.ClientURL(), JS: s.js, s: s}, nil
}

func (e *EmbeddedNATS) Stop() {
	if e != nil {
		e.s.stop()
	}
}
