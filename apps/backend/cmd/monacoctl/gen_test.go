package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/tools/flows"
	codegen "github.com/monaco/monaco/apps/backend/internal/tools/gen"
)

const staleSwift = "@testable import MonacoAPI\n// stale\n"

func writeSpec(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ErrorCodeCases.gen.swift")
	if err := os.WriteFile(path, []byte(staleSwift), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func readSpec(t *testing.T, path string) ([]byte, error) {
	t.Helper()
	dir, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dir.Close() }()
	return dir.ReadFile(filepath.Base(path))
}

func TestGenErrors_replacesTheWholeFileWithTheGeneratedSchema(t *testing.T) {
	t.Parallel()
	path := writeSpec(t)
	var stdout, stderr bytes.Buffer
	if code := run(nil, tools(nil), nil, []string{"gen", "errors", path}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr.String())
	}
	got, err := readSpec(t, path)
	if err != nil {
		t.Fatal(err)
	}
	var cases strings.Builder
	for _, code := range errs.All() {
		cases.WriteString("        case ." + swiftCaseName(code) + ": true\n")
	}
	want := "@testable import MonacoAPI\n\nextension Components.Schemas.ErrorCode {\n    var isListed: Bool {\n" +
		"        switch self {\n" + cases.String() + "        }\n    }\n}\n"
	if string(got) != want {
		t.Fatalf("swift =\n%s\nwant\n%s", got, want)
	}
	if want := fmt.Sprintf("wrote %d error codes to %s\n", len(errs.All()), path); stdout.String() != want {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestGenErrors_writesTheSchemaTheBundlerMerges(t *testing.T) {
	t.Parallel()
	dir := writeSpecDir(t, map[string]string{"base.yaml": bundleBase})
	bundle, err := bundleOpenAPI(dir, errs.All())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(bundle), "\n  # api/spec/error_codes.yaml\n    ErrorCode:\n      type: string\n") ||
		!strings.HasSuffix(string(bundle), "\n        - x_not_linked\n") ||
		strings.Contains(string(bundle), "monacoctl gen errors from") {
		t.Fatalf("bundle =\n%s", bundle)
	}
	stale := writeSpecDir(t, map[string]string{
		"base.yaml": bundleBase, "error_codes.yaml": renderErrorCodes(errs.All()),
	})
	if _, err := bundleOpenAPI(stale, errs.All()); err == nil ||
		!strings.Contains(err.Error(), "schema ErrorCode is defined in both error_codes.yaml and error_codes.yaml") {
		t.Fatalf("a spec dir with its own ErrorCode: err = %v", err)
	}
}

func TestGenErrors_isIdempotent(t *testing.T) {
	t.Parallel()
	path := writeSpec(t)
	var out bytes.Buffer
	var specs [2][]byte
	for i := range specs {
		if code := gen([]string{"errors", path}, &out, &out); code != 0 {
			t.Fatalf("run %d: exit code = %d, output = %q", i, code, out.String())
		}
		var err error
		if specs[i], err = readSpec(t, path); err != nil {
			t.Fatal(err)
		}
	}
	if first, second := specs[0], specs[1]; !bytes.Equal(first, second) {
		t.Fatalf("second run changed the swift list:\n%s\nvs\n%s", first, second)
	}
}

func TestGenErrors_runsWithoutBootConfig(t *testing.T) {
	t.Parallel()
	path := writeSpec(t)
	var stdout, stderr bytes.Buffer
	if code := run(commands(), tools(nil), []string{"MONACO_FOO=1"}, []string{"gen", "errors", path}, &stdout,
		&stderr); code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr.String())
	}
}

func TestGenErrors_missingDirOrUnwritableSpecExits1(t *testing.T) {
	t.Parallel()
	var stderr bytes.Buffer
	missing := filepath.Join(t.TempDir(), "gone", "ErrorCodeCases.gen.swift")
	if code := gen([]string{"errors", missing}, &stderr, &stderr); code != 1 ||
		!strings.Contains(stderr.String(), "monacoctl.writeSwiftCases: invalid_input") {
		t.Fatalf("gen errors %s = %d %q", missing, code, stderr.String())
	}
	if os.Geteuid() == 0 {
		t.Skip("root writes through file modes")
	}
	path := writeSpec(t)
	if err := os.Chmod(path, 0o400); err != nil {
		t.Fatal(err)
	}
	stderr.Reset()
	if code := gen([]string{"errors", path}, &stderr, &stderr); code != 1 ||
		!strings.Contains(stderr.String(), "monacoctl.writeSwiftCases: internal") {
		t.Fatalf("read-only swift list: gen = %d %q", code, stderr.String())
	}
}

func TestCommittedSpecListsEveryErrorCode(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var spec struct {
		Components struct {
			Schemas struct {
				ErrorCode struct {
					Enum []errs.Code `yaml:"enum"`
				} `yaml:"ErrorCode"`
			} `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(raw, &spec); err != nil {
		t.Fatal(err)
	}
	if got := spec.Components.Schemas.ErrorCode.Enum; !slices.Equal(got, errs.All()) {
		t.Fatalf("api/openapi.yaml ErrorCode enum = %v, want errs.All() = %v; run go generate ./api", got, errs.All())
	}
}

func TestGenErrors_writesTheSwiftCaseListWhenGivenAPath(t *testing.T) {
	t.Parallel()
	swift := filepath.Join(t.TempDir(), "ErrorCodeCases.gen.swift")
	var out bytes.Buffer
	if code := gen([]string{"errors", swift}, &out, &out); code != 0 {
		t.Fatalf("exit code = %d, output = %q", code, out.String())
	}
	got, err := readSpec(t, swift)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"\n        case ._internal: true\n", "\n        case .idempotencyInFlight: true\n", "\n        case .xNotLinked: true\n",
	} {
		if !strings.Contains(string(got), want) {
			t.Errorf("swift list lacks %q:\n%s", want, got)
		}
	}
	if n := strings.Count(string(got), "        case ."); n != len(errs.All()) {
		t.Errorf("swift list has %d case lines, want one per code (%d)", n, len(errs.All()))
	}
	for _, line := range strings.Split(string(got), "\n") {
		if len(line) > 120 {
			t.Errorf("line over 120 columns: %q", line)
		}
	}
}

func TestGenErrors_unwritableSwiftPathExits1(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	var stderr bytes.Buffer
	if code := gen([]string{"errors", dir}, &stderr, &stderr); code != 1 ||
		!strings.Contains(stderr.String(), "monacoctl.writeSwiftCases: internal") {
		t.Fatalf("path is a directory: gen = %d %q", code, stderr.String())
	}
}

func TestCommittedSwiftCaseListMatchesErrs(t *testing.T) {
	t.Parallel()
	got, err := os.ReadFile("../../../../packages/mobile-core/Tests/MonacoAPITests/ErrorCodeCases.gen.swift")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != renderSwiftCases(errs.All()) {
		t.Fatal("ErrorCodeCases.gen.swift differs from errs.All(); run go generate ./cmd/monacoctl")
	}
}

func TestGen_otherArgsPrintUsage(t *testing.T) {
	t.Parallel()
	const wantUsage = "usage: monacoctl gen errors <ErrorCodeCases.gen.swift>\n"
	for _, args := range [][]string{nil, {"errors"}, {"events", "x"}, {"errors", "a", "b"}} {
		var stdout, stderr bytes.Buffer
		if code := gen(args, &stdout, &stderr); code != 2 ||
			!strings.HasPrefix(stderr.String(), wantUsage) ||
			!strings.Contains(stderr.String(), "\n       monacoctl gen module <name>\n") {
			t.Fatalf("gen %v = %d %q, want 2 and usage", args, code, stderr.String())
		}
	}
}

func TestGen_scaffoldsIntoTheWorkingDirAndPrintsWhatItWrote(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "apps", "backend")
	if err := os.MkdirAll(root, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/app\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gen := toolGen(toolEnv{wd: root})
	var stdout, stderr bytes.Buffer
	if code := gen([]string{"provider", "quotes"}, &stdout, &stderr); code != 0 {
		t.Fatalf("gen provider = %d, stderr %q", code, stderr.String())
	}
	if !strings.HasPrefix(stdout.String(), "internal/providers/quotes/client.go\n") ||
		strings.Count(stdout.String(), "\n") != 6 {
		t.Fatalf("stdout = %q, want the six written files", stdout.String())
	}
	stdout.Reset()
	if code := gen([]string{"flow", "1"}, &stdout, &stderr); code != 1 || stdout.Len() != 0 ||
		!strings.HasPrefix(
			stderr.String(),
			"monacoctl: gen.planFlow",
		) || !strings.Contains(stderr.String(), flows.Dir) {
		t.Fatalf("gen flow without flow files = %d %q %q, want 1 and the error", code, stdout.String(), stderr.String())
	}
	stderr.Reset()
	if code := gen(
		[]string{"nope"},
		&stdout,
		&stderr,
	); code != 2 ||
		!strings.Contains(stderr.String(), "monacoctl gen flow <id>") {
		t.Fatalf("gen nope = %d %q, want 2 and usage", code, stderr.String())
	}
}

func TestGen_flowWithALetterSuffixWritesItsTestFileIntoTheModule(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	row := "01a\tSet handle\tidentity\tPOST /v1/me/handle\tSetHandle\t\t\tok;InvalidInput;crash:before-commit\tplanned\tdocs/x.md\n"
	for rel, body := range map[string]string{
		"apps/backend/go.mod":                              "module example.com/app\n",
		"apps/backend/internal/modules/identity/module.go": "package identity\n",
		flows.Dir + "/01a.tsv":                             flows.Header + "\n" + row,
	} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(repo, rel)), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(repo, rel), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	dir, err := os.OpenRoot(filepath.Join(repo, "apps", "backend"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dir.Close() }()
	var stdout, stderr bytes.Buffer
	if code := toolGen(toolEnv{wd: dir.Name()})([]string{"flow", "01a"}, &stdout, &stderr); code != 0 {
		t.Fatalf("gen flow 01a = %d, stderr %q", code, stderr.String())
	}
	const written = "internal/modules/identity/flow01a_test.go"
	if stdout.String() != written+"\n" {
		t.Fatalf("stdout = %q, want %q", stdout.String(), written+"\n")
	}
	src, err := dir.ReadFile(written)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"TestFlow01a_SetHandle_OK", "TestFlow01a_SetHandle_InvalidInput", "TestFlow01a_SetHandle_CrashBeforeCommit",
	} {
		want := "func " + name + "(t *testing.T) {\n\tt.Parallel()\n\tt.Fatal(\"not implemented\")\n}"
		if !strings.Contains(string(src), want) {
			t.Errorf("%s lacks a failing %s:\n%s", written, name, src)
		}
	}
	if got := strings.Count(string(src), "func Test"); got != 3 {
		t.Errorf("%s has %d tests, want 3", written, got)
	}
}

func TestGen_migrationRoutesToTheMigratorAndReportsItsFiles(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for rel, body := range map[string]string{
		"go.mod":                            "module example.com/app\n",
		"internal/modules/social/module.go": "package social\n",
		"migrations/atlas.sum":              "",
	} {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var stdout, stderr bytes.Buffer
	if code := toolGen(toolEnv{wd: root})([]string{"migration", "social"}, &stdout, &stderr); code != 1 ||
		!strings.Contains(stderr.String(), "gen migration <module> <name>") {
		t.Fatalf("gen migration social = %d %q, want 1 and usage", code, stderr.String())
	}
	stderr.Reset()
	files := codegen.Migrator{
		Dir:     root,
		Now:     func() time.Time { return time.Date(2026, 10, 2, 13, 0, 0, 0, time.UTC) },
		Staging: func(context.Context) ([]string, error) { return nil, nil },
		Hash:    func(context.Context) error { return nil },
	}
	if code := migrate(files, []string{"social", "follows"}, &stdout, &stderr); code != 0 ||
		stdout.String() != "migrations/20261002130000_social_follows.sql\n" {
		t.Fatalf("migrate = %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	}
	if code := gen([]string{"nope"}, &stdout, &stderr); code != 2 ||
		!strings.Contains(stderr.String(), "gen migration --rebase") {
		t.Fatalf("usage lacks the migration line: %q", stderr.String())
	}
}
