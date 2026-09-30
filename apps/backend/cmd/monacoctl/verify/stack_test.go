package verify

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/platform/config"
)

func TestUp_startsAHealthyStackAndDownStopsEveryProcess(t *testing.T) {
	t.Parallel()
	o := testOptions(t, "ok")
	o.CoverDir = t.TempDir()
	s, err := Up(t.Context(), o)
	if err != nil {
		t.Fatalf("Up: %v", err)
	}
	if got := healthz(t.Context(), s.API); got != http.StatusOK {
		t.Fatalf("api /healthz = %d", got)
	}
	if s.Pool == nil || s.NATS == nil || !strings.HasPrefix(s.TokenKey, "verify-") {
		t.Fatalf("stack = %+v, want a pool, NATS and a dev token key", s)
	}
	if err := s.Down(t.Context()); err != nil {
		t.Fatalf("Down: %v", err)
	}
	for name, p := range s.procs {
		if p.running() {
			t.Errorf("%s still running after Down", name)
		}
	}
	if got := healthz(t.Context(), s.API); got != 0 {
		t.Fatalf("api /healthz after Down = %d, want no answer", got)
	}
}

func TestUp_aStackThatNeverGetsHealthyFailsNamingStackUp(t *testing.T) {
	t.Parallel()
	o := testOptions(t, fakeSick)
	o.Budget.Stack = 2 * time.Second
	s, err := Up(t.Context(), o)
	defer func() { _ = s.Down(t.Context()) }()
	var over *OverBudgetError
	if !errors.As(err, &over) || over.Phase != PhaseStack || !strings.Contains(err.Error(), "stack-up") ||
		!strings.Contains(err.Error(), "answered 503") {
		t.Fatalf("Up = %v, want over budget in stack-up naming the sick health check", err)
	}
}

func TestUp_namesTheProcessThatExitsBeforeListening(t *testing.T) {
	t.Parallel()
	s, err := Up(t.Context(), testOptions(t, fakeMute))
	defer func() { _ = s.Down(t.Context()) }()
	if err == nil || !strings.Contains(err.Error(), "fakes exited before boot.listening") {
		t.Fatalf("Up = %v, want fakes named", err)
	}
}

func TestUp_namesTheFakesProcessThatNeverLogsListening(t *testing.T) {
	t.Parallel()
	o := testOptions(t, fakeQuiet)
	stopWaiting := errors.New("test stopped waiting")
	s, err := Up(stopWhenFakesStart(t, &o, stopWaiting), o)
	if downErr := s.Down(t.Context()); downErr != nil {
		t.Errorf("Down: %v", downErr)
	}
	if !errors.Is(err, stopWaiting) || !strings.Contains(err.Error(), "fakes never logged boot.listening") {
		t.Fatalf("Up = %v, want fakes named as never listening", err)
	}
}

func stopWhenFakesStart(t *testing.T, o *Options, cause error) context.Context {
	t.Helper()
	ln, err := new(net.ListenConfig).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancelCause(t.Context())
	t.Cleanup(func() {
		cancel(nil)
		_ = ln.Close()
	})
	o.Environ = append(o.Environ, fakeStartedEnv+"="+ln.Addr().String())
	go func() {
		if conn, err := ln.Accept(); err == nil {
			_ = conn.Close()
			cancel(cause)
		}
	}()
	return ctx
}

func TestUp_failsWhenTheAPIOrWorkerCannotStart(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		bins func(Binaries) Binaries
	}{
		{"api", func(b Binaries) Binaries { b.API = "/nonexistent/api"; return b }},
		{"worker", func(b Binaries) Binaries { b.Worker = "/nonexistent/worker"; return b }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			o := testOptions(t, "ok")
			o.Bins = tc.bins(o.Bins)
			s, err := Up(t.Context(), o)
			if downErr := s.Down(t.Context()); downErr != nil {
				t.Errorf("Down: %v", downErr)
			}
			if err == nil || !strings.Contains(err.Error(), "/nonexistent/"+tc.name) {
				t.Fatalf("Up = %v, want the missing %s binary named", err, tc.name)
			}
		})
	}
}

func TestUp_reportsSchemaAndPostgresFailures(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		edit func(*Options)
		want string
	}{
		{"postgres", func(o *Options) {
			o.Postgres = func(context.Context, string) (string, func(context.Context) error, error) {
				return "", nil, errors.New("no docker")
			}
		}, "no docker"},
		{"unreachable", func(o *Options) {
			o.Budget.Stack = 300 * time.Millisecond
			o.Postgres = func(context.Context, string) (string, func(context.Context) error, error) {
				return "postgres://monaco:monaco@127.0.0.1:1/monaco?sslmode=disable", nil, nil
			}
		}, "postgres never accepted connections"},
		{"atlas", func(o *Options) { o.Atlas = "/nonexistent/atlas" }, "/nonexistent/atlas migrate apply"},
		{"docker", func(o *Options) {
			o.Postgres = nil
			o.Docker = Docker(writeScript(t, "docker", `[ "$1" = rm ] && exit 0; echo "no daemon" >&2; exit 1`))
		}, "docker run: exit status 1: no daemon"},
		{"budget", func(o *Options) {
			o.Budget.Stack = 100 * time.Millisecond
			o.Postgres = func(ctx context.Context, _ string) (string, func(context.Context) error, error) {
				<-ctx.Done()
				return "", nil, errors.New("gave up")
			}
		}, "over budget: stack-up took longer than 100ms: gave up"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			o := testOptions(t, "ok")
			tc.edit(&o)
			s, err := Up(t.Context(), o)
			if downErr := s.Down(t.Context()); downErr != nil {
				t.Errorf("Down: %v", downErr)
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Up = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestDown_namesTheProcessThatSpentTheTeardownBudget(t *testing.T) {
	t.Parallel()
	o := testOptions(t, fakeDeaf)
	o.Budget.Teardown = 300 * time.Millisecond
	s, err := Up(t.Context(), o)
	if err != nil {
		t.Fatalf("Up: %v", err)
	}
	err = s.Down(t.Context())
	var over *OverBudgetError
	if !errors.As(err, &over) || over.Phase != PhaseTeardown {
		t.Fatalf("Down = %v, want the teardown budget named", err)
	}
	for _, want := range []string{
		"worker did not exit within the budget after SIGTERM",
		"api was killed without a graceful stop because the budget was already spent",
		"fakes was killed without a graceful stop because the budget was already spent",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Down = %v, want %q", err, want)
		}
	}
	if strings.Contains(err.Error(), "ignored") {
		t.Errorf("Down = %v, want no process blamed for ignoring the signal", err)
	}
	for name, p := range s.procs {
		if p.running() {
			t.Errorf("%s still running after Down", name)
		}
	}
}

func TestHealthz_answersZeroWhenNothingAnswers(t *testing.T) {
	t.Parallel()
	for _, base := range []string{"http://127.0.0.1:1", "http://bad\x7fhost"} {
		if got := healthz(t.Context(), base); got != 0 {
			t.Errorf("healthz(%q) = %d, want 0", base, got)
		}
	}
}

func TestChildEnv_dropsEveryConfigKeySoChildrenOnlySeeWhatVerifySets(t *testing.T) {
	t.Parallel()
	got := childEnv([]string{"PATH=/bin", "MONACO_ENV=local", "OTEL_SERVICE_NAME=x", "DATABASE_URL=y", "HOME=/h"})
	if strings.Join(got, " ") != "PATH=/bin HOME=/h" {
		t.Fatalf("childEnv = %v", got)
	}
}

func TestOverBudget_namesThePhaseAndTheFlow(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		err  *OverBudgetError
		want string
	}{
		{&OverBudgetError{Phase: PhaseStack, Budget: 10 * time.Second}, "over budget: stack-up took longer than 10s"},
		{
			&OverBudgetError{Phase: PhaseFlow, Flow: "00 ok", Budget: 15 * time.Second},
			"over budget: flow 00 ok flow took longer than 15s",
		},
	} {
		if got := tc.err.Error(); got != tc.want {
			t.Errorf("Error() = %q, want %q", got, tc.want)
		}
	}
}

func TestWaitPostgres_returnsOnceTheServerAnswersEvenBeforeMigrations(t *testing.T) {
	t.Parallel()
	url := strings.Replace(config.TestDBURL(os.Environ()), "/monaco?", "/postgres?", 1)
	if err := waitPostgres(t.Context(), url); err != nil {
		t.Fatalf("waitPostgres on an unmigrated database = %v", err)
	}
	if err := waitPostgres(t.Context(), "::not a url"); err == nil {
		t.Fatal("waitPostgres accepted an unparsable url")
	}
}
