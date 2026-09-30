package lint_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type finding struct {
	Linter string `json:"FromLinter"`
	Text   string `json:"Text"`
}

const stubRequires = `
require (
	github.com/google/uuid v1.0.0
	github.com/jackc/pgx/v5 v5.0.0
)

replace (
	github.com/google/uuid => ./stubs/uuid
	github.com/jackc/pgx/v5 => ./stubs/pgx
)
`

func requireGolangciLint(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("runs golangci-lint once per fixture; CI runs it without -short, outside the 10 s package budget")
	}
	if _, err := exec.LookPath("golangci-lint"); err == nil {
		return
	}
	if os.Getenv("CI") != "" {
		t.Fatal("golangci-lint must be on PATH in CI")
	}
	t.Skip("golangci-lint is not on PATH; run just install to prove the lint rules locally")
}

func fixtureGoMod(t *testing.T) []byte {
	t.Helper()
	backendMod, err := os.ReadFile(filepath.Join(backendRoot, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	var mod strings.Builder
	for line := range strings.Lines(string(backendMod)) {
		if strings.HasPrefix(line, "module ") || strings.HasPrefix(line, "go ") {
			mod.WriteString(line)
		}
	}
	mod.WriteString(stubRequires)
	return []byte(mod.String())
}

func fixtureModule(t *testing.T, fixture string) string {
	t.Helper()
	dir := t.TempDir()
	cfg, err := os.ReadFile(filepath.Join(backendRoot, ".golangci.base.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".golangci.base.yml"), cfg, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), fixtureGoMod(t), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.CopyFS(filepath.Join(dir, "stubs"), os.DirFS(filepath.Join("testdata", "stubs"))); err != nil {
		t.Fatal(err)
	}
	if err := os.CopyFS(dir, os.DirFS(filepath.Join("testdata", fixture))); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.CommandContext(t.Context(), genGolangci, dir).CombinedOutput(); err != nil {
		t.Fatalf("gen-golangci: %v\n%s", err, out)
	}
	return dir
}

func golangciFindings(t *testing.T, dir string) []finding {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "golangci-lint", "run", "--allow-parallel-runners", "--show-stats=false",
		"--output.json.path=stdout", "--output.text.path=stderr", "./...",
	)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	var exit *exec.ExitError
	if err != nil && (!errors.As(err, &exit) || exit.ExitCode() != 1) {
		t.Fatalf("golangci-lint: %v\n%s", err, stderr.String())
	}
	var report struct {
		Issues []finding `json:"Issues"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("golangci-lint json: %v\n%s\n%s", err, stdout.String(), stderr.String())
	}
	return report.Issues
}

func nogoFindings(t *testing.T, dir string) []finding {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), nogoBin, "./...")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	var exit *exec.ExitError
	switch {
	case err == nil:
		return nil
	case errors.As(err, &exit) && exit.ExitCode() == 3:
		var fs []finding
		for line := range strings.Lines(strings.TrimSpace(string(out))) {
			fs = append(fs, finding{Linter: "nogo", Text: line})
		}
		return fs
	default:
		t.Fatalf("nogo: %v\n%s", err, out)
		return nil
	}
}

func TestLintRules_eachViolationFailsWithItsLinter(t *testing.T) {
	t.Parallel()
	requireGolangciLint(t)
	for _, tc := range []struct {
		fixture, linter, text string
	}{
		{"domain-imports-pgx", "depguard", "domain is pure"},
		{"app-imports-net-http", "depguard", "use cases are transport-free"},
		{"module-imports-module", "depguard", "modules never import each other"},
		{"module-imports-module-app", "depguard", "modules never import each other"},
		{"adapters-import-a-query-port", "depguard", "domain and adapters import no other module"},
		{"domain-imports-a-query-port", "depguard", "domain and adapters import no other module"},
		{"module-imports-boundary", "depguard", "only platform, testkit and cmd log Warn and Error"},
		{"float-in-domain", "forbidigo", "money is integer base units"},
		{"float-in-money", "forbidigo", "money is integer base units"},
		{"time-now-in-app", "forbidigo", "inject platform/clock.Clock"},
		{"errors-new-in-module", "forbidigo", "use errs.New"},
		{"slog-error-in-module", "forbidigo", "boundaries log them once"},
		{"fmt-println", "forbidigo", "use slog"},
		{"http-default-client-in-module", "forbidigo", "outbound HTTP goes through platform/httpclient"},
		{"math-rand-global", "forbidigo", "inject the random source"},
		{"uuid-new-outside-platform", "forbidigo", "inject the id generator"},
		{"time-sleep-in-test", "forbidigo", "use synctest.Test, testkit.Eventually or testkit.AssertNoRedelivery"},
		{"pgx-begin-outside-db", "forbidigo", "open transactions through platform/db"},
		{"context-background-in-module", "forbidigo", "only main, tests and bus roots"},
		{"os-getenv-in-module", "forbidigo", "read config through platform/config"},
		{"go-statement-in-module", "nogo", "bare go statement"},
		{"testmain-outside-main-test", "nogo", "TestMain belongs in main_test.go"},
		{"testmain-custom-body", "nogo", "TestMain body must be exactly testkit.Main(m, opts...)"},
		{"testwait-fixed-wait", "nogo", "fixed wait <-time.After"},
	} {
		t.Run(tc.fixture, func(t *testing.T) {
			t.Parallel()
			dir := fixtureModule(t, filepath.Join("violations", tc.fixture))
			got := append(golangciFindings(t, dir), nogoFindings(t, dir)...)
			for _, f := range got {
				if f.Linter == tc.linter && strings.Contains(f.Text, tc.text) {
					return
				}
			}
			t.Fatalf("want a %s finding containing %q, got %+v", tc.linter, tc.text, got)
		})
	}
}

func TestLintRules_cleanTreeHasNoFindings(t *testing.T) {
	t.Parallel()
	requireGolangciLint(t)
	dir := fixtureModule(t, "clean")
	if got := append(golangciFindings(t, dir), nogoFindings(t, dir)...); len(got) != 0 {
		t.Fatalf("want no findings on exported, comment-free code and allowed platform/cmd uses, got %+v", got)
	}
}
