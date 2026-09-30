package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel/metric/noop"

	openapi "github.com/monaco/monaco/apps/backend/api"
	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestMain(m *testing.M) {
	testkit.Main(m, testkit.WithChild(main), testkit.WithNATS())
}

func bootEnv(t *testing.T, extra ...string) []string {
	t.Helper()
	url := testkit.StandaloneNATS(t)
	conn, err := bus.Connect(t.Context(), config.NATS{URL: url}, bus.ProcessMonacoctl)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Apply(t.Context()); err != nil {
		t.Fatal(err)
	}
	conn.Close(t.Context())
	return append([]string{
		"MONACO_ENV=test", "DATABASE_URL=" + testkit.DB(t).Config().ConnString(), "NATS_URL=" + url,
		"MONACO_DEV_TOKEN_KEY=test-only",
	}, extra...)
}

func TestMain_servesHealthzUntilSIGTERMThenExitsZero(t *testing.T) {
	t.Parallel()
	p := testkit.StartMain(t, bootEnv(t, "MONACO_HTTP_ADDR=127.0.0.1:0"))
	if code, body := testkit.Get(t, "http://"+p.Addr+"/healthz"); code != http.StatusOK || body != "ok\n" {
		t.Fatalf("GET /healthz = %d %q, want 200 %q", code, body, "ok\n")
	}
	if stderr, err := p.Terminate(); err != nil {
		t.Fatalf("api after SIGTERM: %v\n%s", err, stderr)
	}
}

func TestMain_exitsOneAndLogsWhyWhenConfigIsMissing(t *testing.T) {
	t.Parallel()
	out, err := testkit.MainCommand(t, nil).CombinedOutput()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 1 ||
		!strings.Contains(string(out), `"msg":"boot.stopped"`) || !strings.Contains(string(out), "missing MONACO_ENV") {
		t.Fatalf("api without config = %v\n%s", err, out)
	}
}

func TestRun_refusesToBootWithoutRequiredConfig(t *testing.T) {
	t.Parallel()
	err := run(t.Context(), io.Discard, []string{"PATH=/usr/bin"}, openapi.Spec, noop.NewMeterProvider())
	want := "config.Load: invalid_input: missing MONACO_ENV, DATABASE_URL, NATS_URL"
	if err == nil || err.Error() != want || errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("run = %v, want %q", err, want)
	}
}

func TestRun_refusesToBootInProductionWithoutAPostHogKey(t *testing.T) {
	t.Parallel()
	err := run(t.Context(), io.Discard, append([]string{
		"MONACO_ENV=production", "DATABASE_URL=postgres://localhost/monaco", "NATS_URL=nats://localhost:4222",
	}, testkit.APNsEnv()...), openapi.Spec, noop.NewMeterProvider())
	want := "config.Load: invalid_input: missing POSTHOG_API_KEY"
	if err == nil || err.Error() != want || errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("run = %v, want %q", err, want)
	}
}

func TestRun_refusesToBootWithMalformedOTelEndpoint(t *testing.T) {
	t.Parallel()
	err := run(t.Context(), io.Discard, []string{
		"MONACO_ENV=test", "DATABASE_URL=postgres://localhost/monaco", "NATS_URL=nats://localhost:4222",
		"OTEL_EXPORTER_OTLP_ENDPOINT=collector:4318",
	}, openapi.Spec, noop.NewMeterProvider())
	if errs.CodeOf(err) != errs.CodeInvalidInput || !strings.Contains(err.Error(), "observability.Setup") {
		t.Fatalf("run = %v, want invalid_input from observability.Setup", err)
	}
}

func TestRun_reportsAnAddressItCannotListenOn(t *testing.T) {
	t.Parallel()
	err := run(
		t.Context(),
		io.Discard,
		bootEnv(t, "MONACO_HTTP_ADDR=256.0.0.1:1"),
		openapi.Spec,
		noop.NewMeterProvider(),
	)
	if err == nil || !strings.Contains(err.Error(), "listen on 256.0.0.1:1") {
		t.Fatalf("run = %v, want a listen error", err)
	}
}

func TestRun_refusesToBootWithAnUnparsableSpec(t *testing.T) {
	t.Parallel()
	err := run(t.Context(), io.Discard, bootEnv(t), []byte("openapi: [unclosed"), noop.NewMeterProvider())
	if errs.CodeOf(err) != errs.CodeInvalidConfig || !strings.HasPrefix(err.Error(), "ratelimit.Load: ") {
		t.Fatalf("run = %v, want invalid_config from ratelimit.Load", err)
	}
}

func TestRun_refusesToBootWithAMalformedRateLimit(t *testing.T) {
	t.Parallel()
	const anchor = "      operationId: postSystemPing\n"
	spec := bytes.Replace(openapi.Spec, []byte(anchor),
		[]byte(anchor+"      x-rate-limit: {ip: {rate: 0, per: 1m, burst: 1}}\n"), 1)
	if bytes.Equal(spec, openapi.Spec) {
		t.Fatalf("anchor %q is not in api/openapi.yaml", anchor)
	}
	err := run(t.Context(), io.Discard, bootEnv(t), spec, noop.NewMeterProvider())
	if errs.CodeOf(err) != errs.CodeInvalidConfig || !strings.HasPrefix(err.Error(), "ratelimit.Load: ") {
		t.Fatalf("run = %v, want invalid_config from ratelimit.Load", err)
	}
}

func TestRun_refusesToBootWhenTheRateLimitCountersCannotBeCreated(t *testing.T) {
	t.Parallel()
	err := run(t.Context(), io.Discard, bootEnv(t), openapi.Spec, testkit.FailingGauges{Prefix: "monaco_ratelimit_"})
	if errs.CodeOf(err) != errs.CodeInternal || !strings.HasPrefix(err.Error(), "ratelimit.New: ") {
		t.Fatalf("run = %v, want internal from ratelimit.New", err)
	}
}

func TestRun_cancelledDuringBootStopsCleanly(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err := run(ctx, io.Discard, []string{
		"MONACO_ENV=test", "DATABASE_URL=postgres://localhost/monaco", "NATS_URL=" + testkit.NATSURL(),
		"MONACO_HTTP_ADDR=127.0.0.1:0", "MONACO_WORKER_HEALTH_ADDR=127.0.0.1:0", "MONACO_DEV_TOKEN_KEY=test-only",
	}, openapi.Spec, noop.NewMeterProvider())
	if err != nil {
		t.Fatalf("run with a cancelled context = %v, want nil: a stop during boot is a clean stop", err)
	}
}

func TestRun_aShutdownFailureAfterACancelIsReported(t *testing.T) {
	t.Parallel()
	url := testkit.StandaloneNATS(t)
	conn, err := bus.Connect(t.Context(), config.NATS{URL: url}, bus.ProcessMonacoctl)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Apply(t.Context()); err != nil {
		t.Fatal(err)
	}
	conn.Close(t.Context())

	ctx, cancel := context.WithCancel(t.Context())
	stop := time.AfterFunc(2*time.Second, cancel)
	defer stop.Stop()
	err = run(ctx, io.Discard, []string{
		"MONACO_ENV=test",
		"DATABASE_URL=" + testkit.DB(t).Config().ConnString(),
		"NATS_URL=" + url,
		"MONACO_HTTP_ADDR=127.0.0.1:0",
		"MONACO_WORKER_HEALTH_ADDR=127.0.0.1:0",
		"OTEL_EXPORTER_OTLP_ENDPOINT=http://127.0.0.1:1",
		"MONACO_TIMEOUT_SHUTDOWN=1s",
		"MONACO_DEV_TOKEN_KEY=test-only",
	}, openapi.Spec, noop.NewMeterProvider())
	if errs.CodeOf(err) != errs.CodeUpstreamUnavailable || errors.Is(err, context.Canceled) {
		t.Fatalf("run = %v, want the telemetry flush failure reported after the cancel", err)
	}
}

func TestBootErr_onlyOurOwnCancelIsACleanStop(t *testing.T) {
	t.Parallel()
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	foreign := fmt.Errorf("pool: %w", context.Canceled)
	if err := bootErr(cancelled, foreign); err != nil {
		t.Fatalf("bootErr(cancelled ctx, wrapped Canceled) = %v, want nil", err)
	}
	if err := bootErr(t.Context(), foreign); !errors.Is(err, context.Canceled) {
		t.Fatalf(
			"bootErr(live ctx, foreign Canceled) = %v, want the error kept: nobody asked this process to stop",
			err,
		)
	}
	if err := bootErr(cancelled, io.ErrUnexpectedEOF); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("bootErr(cancelled ctx, other error) = %v, want the error kept", err)
	}
}

func TestRun_refusesToBootWithoutADevTokenKeyOutsideProduction(t *testing.T) {
	t.Parallel()
	err := run(t.Context(), io.Discard, bootEnv(t, "MONACO_HTTP_ADDR=127.0.0.1:0", "MONACO_DEV_TOKEN_KEY="),
		openapi.Spec, noop.NewMeterProvider())
	if errs.CodeOf(err) != errs.CodeInvalidInput || !strings.Contains(err.Error(), "auth.NewDevVerifier") {
		t.Fatalf("run = %v, want invalid_input from auth.NewDevVerifier", err)
	}
}

func TestRun_refusesToBootWhenTheRelayBacklogGaugesCannotBeExported(t *testing.T) {
	t.Parallel()
	err := run(t.Context(), io.Discard, bootEnv(t, "MONACO_HTTP_ADDR=127.0.0.1:0"), openapi.Spec,
		testkit.FailingGauges{Prefix: "monaco_events_"})
	if errs.CodeOf(err) != errs.CodeInternal || !strings.HasPrefix(err.Error(), "bus.Relay.ExportBacklogGauges: ") {
		t.Fatalf("run = %v, want internal from bus.Relay.ExportBacklogGauges", err)
	}
}

func TestRun_refusesToBootWithAnUnknownFaultpoint(t *testing.T) {
	t.Parallel()
	err := run(t.Context(), io.Discard, []string{
		"MONACO_ENV=test", "DATABASE_URL=postgres://localhost/monaco", "NATS_URL=nats://localhost:4222",
		"MONACO_FAULTPOINT=after-everything",
	}, openapi.Spec, noop.NewMeterProvider())
	if err == nil || err.Error() != "faultpoint.Configure: invalid_input" {
		t.Fatalf("run = %v, want invalid_input from faultpoint.Configure", err)
	}
}
