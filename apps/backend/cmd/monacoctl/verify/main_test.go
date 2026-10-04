package verify

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	openapi "github.com/monaco/monaco/apps/backend/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

var (
	dispatcher string
	contract   *httpx.Contract
)

func TestMain(m *testing.M) {
	testkit.Main(m, testkit.WithChild(fakeMain), testkit.WithNATS(), testkit.WithSetup(setup))
}

func setup() (func(), error) {
	var err error
	if contract, err = scenario.LoadContract(openapi.Spec); err != nil {
		return nil, err
	}
	return writeDispatcher()
}

func writeDispatcher() (func(), error) {
	dir, err := os.MkdirTemp("", "verify-fakes-")
	if err != nil {
		return nil, err
	}
	dispatcher = filepath.Join(dir, "fake")
	if err := os.WriteFile(dispatcher, []byte("#!/bin/sh\n. \"$0.mode\"\n"), 0o755); err != nil {
		_ = os.RemoveAll(dir)
		return nil, err
	}
	return func() { _ = os.RemoveAll(dir) }, nil
}

const (
	fakeEnv   = "VERIFY_FAKE"
	fakeSick  = "sick"
	fakeDeaf  = "deaf"
	fakeMute  = "mute"
	fakeQuiet = "quiet"

	fakeHoldEnv      = "VERIFY_FAKE_HOLD"
	fakeStartedEnv   = "VERIFY_FAKE_STARTED"
	fakeProbedEnv    = "VERIFY_FAKE_PROBED"
	fakeNeedsEnv     = "VERIFY_FAKE_WORKER_NEEDS"
	fakeCrashWaitEnv = "VERIFY_FAKE_CRASH_WAIT"
	fakeCrashNowEnv  = "VERIFY_FAKE_CRASH_NOW"

	noRaceExitSleep = "GORACE=atexit_sleep_ms=0"
)

func fakeMain() {
	mode := os.Getenv(fakeEnv)
	stop := make(chan os.Signal, 1)
	if mode == fakeDeaf {
		signal.Ignore(syscall.SIGTERM)
	} else {
		signal.Notify(stop, syscall.SIGTERM)
	}
	if mode == fakeMute {
		os.Exit(3)
	}
	if workerLacksAVariable() {
		os.Exit(5)
	}
	ln, err := new(net.ListenConfig).Listen(context.Background(), "tcp", fakeAddr())
	if err != nil {
		os.Exit(4)
	}
	crashNow(ln)
	checked := serveFake(ln, mode)
	if mode != fakeQuiet {
		_, _ = fmt.Fprintf(os.Stderr, "partial ")
		for range 2 {
			_, _ = fmt.Fprintf(os.Stderr, "line\n{\"msg\":\"boot.listening\",\"addr\":%q}\n", ln.Addr().String())
		}
	}
	if os.Getenv(fakeHoldEnv) != "" {
		select {}
	}
	if addr := os.Getenv(fakeStartedEnv); addr != "" {
		if conn, err := new(net.Dialer).DialContext(context.Background(), "tcp", addr); err == nil {
			_ = conn.Close()
		}
	}
	if point := os.Getenv("MONACO_FAULTPOINT"); point != "" {
		go crashAfter(checked, point)
	}
	<-stop
}

func crashNow(ln net.Listener) {
	point := os.Getenv("MONACO_FAULTPOINT")
	if point == "" || os.Getenv(fakeCrashNowEnv) == "" {
		return
	}
	_, _ = fmt.Fprintf(os.Stderr, "{\"msg\":\"boot.listening\",\"addr\":%q}\n", ln.Addr().String())
	_, _ = fmt.Fprintf(os.Stderr, "panic: faultpoint: crash at %s\n", point)
	os.Exit(2)
}

func workerLacksAVariable() bool {
	need := os.Getenv(fakeNeedsEnv)
	return need != "" && os.Getenv("MONACO_WORKER_HEALTH_ADDR") != "" && os.Getenv(need) == ""
}

func fakeAddr() string {
	addr := ""
	for _, key := range []string{"FAKES_ADDR", "MONACO_HTTP_ADDR", "MONACO_WORKER_HEALTH_ADDR"} {
		if v := os.Getenv(key); v != "" {
			addr = v
		}
	}
	return addr
}

func serveFake(ln net.Listener, mode string) <-chan struct{} {
	status := http.StatusOK
	if mode == fakeSick {
		status = http.StatusServiceUnavailable
	}
	mux := http.NewServeMux()
	checked, once := make(chan struct{}), sync.Once{}
	var answered atomic.Int32
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = fmt.Fprintf(os.Stderr,
			`{"msg":"http.request","method":"GET","route":"/healthz","status":%d,"duration_ms":0}`+"\n", status)
		once.Do(func() { close(checked) })
		if addr := os.Getenv(fakeProbedEnv); addr != "" && answered.Add(1) == 2 {
			if conn, err := new(net.Dialer).DialContext(r.Context(), "tcp", addr); err == nil {
				_ = conn.Close()
			}
		}
	})
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: time.Second}
	go func() { _ = srv.Serve(ln) }()
	return checked
}

func crashAfter(checked <-chan struct{}, point string) {
	unchecked := time.NewTimer(fakeCrashWait())
	select {
	case <-checked:
		<-time.NewTimer(25 * time.Millisecond).C
	case <-unchecked.C:
	}
	_, _ = fmt.Fprintf(os.Stderr, "panic: faultpoint: crash at %s\n", point)
	os.Exit(2)
}

func fakeCrashWait() time.Duration {
	wait, err := time.ParseDuration(os.Getenv(fakeCrashWaitEnv))
	if err != nil || wait <= 0 {
		return time.Second
	}
	return wait
}

func fakeEnviron(mode string) []string {
	return []string{
		"TESTKIT_RUN_MAIN=1", fakeEnv + "=" + mode, "MONACO_ENV=local", "PATH=" + os.Getenv("PATH"), noRaceExitSleep,
		fakeCrashWaitEnv + "=25ms",
	}
}

func fakeBinaries(t *testing.T) Binaries {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return Binaries{API: exe, Worker: exe, Fakes: exe}
}

func testPostgres(t *testing.T) PostgresFunc {
	t.Helper()
	url := testkit.DB(t).Config().ConnString()
	return func(context.Context, string) (string, func(context.Context) error, error) {
		return url, func(context.Context) error { return nil }, nil
	}
}

func backendDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func testBudget() Budget {
	b := DefaultBudget()
	b.Total = 5 * time.Minute
	b.Stack = 4 * time.Minute
	b.Flow = time.Minute
	b.Converge = 2 * time.Minute
	b.Teardown = 20 * time.Second
	return b
}

func testOptions(t *testing.T, mode string) Options {
	t.Helper()
	dir := backendDir(t)
	return Options{
		Dir:       dir,
		Atlas:     "true",
		Environ:   fakeEnviron(mode),
		Bins:      fakeBinaries(t),
		Budget:    testBudget(),
		Postgres:  testPostgres(t),
		PollEvery: 5 * time.Millisecond,
	}
}

func writeScript(t *testing.T, name, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path+".mode", []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(dispatcher, path); err != nil {
		t.Fatal(err)
	}
	return path
}
