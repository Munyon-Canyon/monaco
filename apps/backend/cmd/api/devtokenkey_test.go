package main

import (
	"errors"
	"io"
	"os/exec"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/metric/noop"

	openapi "github.com/monaco/monaco/apps/backend/api"
	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestRun_refusesToBootInStagingWithThePlaceholderDevTokenKey(t *testing.T) {
	t.Parallel()
	err := run(t.Context(), io.Discard, append([]string{
		"MONACO_ENV=staging", "DATABASE_URL=postgres://localhost/monaco", "NATS_URL=nats://localhost:4222",
		"MONACO_DEV_TOKEN_KEY=" + auth.PlaceholderDevTokenKey,
	}, testkit.APNsEnv()...), openapi.Spec, noop.NewMeterProvider())
	if errs.CodeOf(err) != errs.CodeInvalidConfig || !strings.HasPrefix(err.Error(), "auth.CheckDevTokenKey: ") {
		t.Fatalf("run = %v, want invalid_config from auth.CheckDevTokenKey", err)
	}
}

func TestMain_exitsOneAndLogsWhyThePlaceholderDevTokenKeyIsRefusedWithoutEchoingIt(t *testing.T) {
	t.Parallel()
	out, err := testkit.MainCommand(t, append([]string{
		"MONACO_ENV=staging", "DATABASE_URL=postgres://localhost/monaco", "NATS_URL=nats://localhost:4222",
		"MONACO_DEV_TOKEN_KEY=" + auth.PlaceholderDevTokenKey,
	}, testkit.APNsEnv()...)).CombinedOutput()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 1 {
		t.Fatalf("api with the placeholder key in staging = %v\n%s", err, out)
	}
	for _, want := range []string{
		`"msg":"boot.stopped"`, `"code":"invalid_config"`, "placeholder dev token key",
	} {
		if !strings.Contains(string(out), want) {
			t.Errorf("boot log lacks %s:\n%s", want, out)
		}
	}
	if strings.Contains(string(out), auth.PlaceholderDevTokenKey) {
		t.Fatalf("boot log echoes the key:\n%s", out)
	}
}
