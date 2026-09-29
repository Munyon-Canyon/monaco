package verify

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestMain(m *testing.M) {
	testkit.Main(m, testkit.WithChild(fakeMain))
}

const (
	fakeEnv   = "VERIFY_FAKE"
	fakeSick  = "sick"
	fakeDeaf  = "deaf"
	fakeMute  = "mute"
	fakeQuiet = "quiet"
)

func fakeMain() {
	mode := os.Getenv(fakeEnv)
	if mode == fakeMute {
		os.Exit(3)
	}
	addr := ""
	for _, key := range []string{"FAKES_ADDR", "MONACO_HTTP_ADDR", "MONACO_WORKER_HEALTH_ADDR"} {
		if v := os.Getenv(key); v != "" {
			addr = v
		}
	}
	ln, err := new(net.ListenConfig).Listen(context.Background(), "tcp", addr)
	if err != nil {
		os.Exit(4)
	}
	status := http.StatusOK
	if mode == fakeSick {
		status = http.StatusServiceUnavailable
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status) })
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: time.Second}
	go func() { _ = srv.Serve(ln) }()
	if mode != fakeQuiet {
		_, _ = fmt.Fprintf(os.Stderr, "partial ")
		for range 2 {
			_, _ = fmt.Fprintf(os.Stderr, "line\n{\"msg\":\"boot.listening\",\"addr\":%q}\n", ln.Addr().String())
		}
	}
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM)
	if mode == fakeDeaf {
		signal.Ignore(syscall.SIGTERM)
		select {}
	}
	<-stop
}

func fakeEnviron(mode string) []string {
	return []string{"TESTKIT_RUN_MAIN=1", fakeEnv + "=" + mode, "MONACO_ENV=local", "PATH=" + os.Getenv("PATH")}
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
	b.Stack = time.Minute
	b.Teardown = 5 * time.Second
	return b
}

func testOptions(t *testing.T, mode string) Options {
	t.Helper()
	dir := backendDir(t)
	return Options{
		Dir:      dir,
		Atlas:    "true",
		Environ:  fakeEnviron(mode),
		Bins:     fakeBinaries(t),
		Budget:   testBudget(),
		Postgres: testPostgres(t),
	}
}
