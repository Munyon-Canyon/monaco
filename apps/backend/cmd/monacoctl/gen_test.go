package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/tools/flows"
)

const staleSpec = `components:
  schemas:
    ErrorCode:
      type: string
      enum:
        # BEGIN GENERATED ErrorCode
        - gone
        # END GENERATED ErrorCode
    Other:
      type: string
`

func writeSpec(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "openapi.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
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

func TestGenErrors_writesEveryCodeBetweenTheMarkersAtTheMarkerIndent(t *testing.T) {
	t.Parallel()
	path := writeSpec(t, staleSpec)
	var stdout, stderr bytes.Buffer
	if code := run(nil, tools(nil), nil, []string{"gen", "errors", path}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr.String())
	}
	got, err := readSpec(t, path)
	if err != nil {
		t.Fatal(err)
	}
	var enum strings.Builder
	for _, code := range errs.All() {
		enum.WriteString("        - " + string(code) + "\n")
	}
	want := strings.Replace(staleSpec, "        - gone\n", enum.String(), 1)
	if string(got) != want {
		t.Fatalf("spec =\n%s\nwant\n%s", got, want)
	}
	if want := fmt.Sprintf("wrote %d error codes to %s\n", len(errs.All()), path); stdout.String() != want {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestGenErrors_isIdempotent(t *testing.T) {
	t.Parallel()
	path := writeSpec(t, staleSpec)
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
		t.Fatalf("second run changed the spec:\n%s\nvs\n%s", first, second)
	}
}

func TestGenErrors_runsWithoutBootConfig(t *testing.T) {
	t.Parallel()
	path := writeSpec(t, staleSpec)
	var stdout, stderr bytes.Buffer
	if code := run(commands(), tools(nil), []string{"MONACO_FOO=1"}, []string{"gen", "errors", path}, &stdout,
		&stderr); code != 0 {
		t.Fatalf("exit code = %d, stderr = %q", code, stderr.String())
	}
}

func TestGenErrors_refusesSpecsWithoutExactlyOneMarkerPair(t *testing.T) {
	t.Parallel()
	for name, body := range map[string]string{
		"no markers":       "enum: []\n",
		"end before begin": "# END GENERATED ErrorCode\n# BEGIN GENERATED ErrorCode\n",
		"two begins":       "# BEGIN GENERATED ErrorCode\n# BEGIN GENERATED ErrorCode\n# END GENERATED ErrorCode\n",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			path := writeSpec(t, body)
			var stdout, stderr bytes.Buffer
			code := gen([]string{"errors", path}, &stdout, &stderr)
			if code != 1 ||
				!strings.HasPrefix(stderr.String(), "monacoctl: monacoctl.spliceErrorCodes: invalid_input") {
				t.Fatalf("gen = %d %q, want 1 and invalid_input", code, stderr.String())
			}
			if got, err := readSpec(t, path); err != nil || string(got) != body {
				t.Fatalf("spec changed to %q (%v)", got, err)
			}
		})
	}
}

func TestGenErrors_unreadableOrUnwritableSpecExits1(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	var stderr bytes.Buffer
	for _, missing := range []string{filepath.Join(dir, "missing.yaml"), filepath.Join(dir, "gone", "openapi.yaml")} {
		stderr.Reset()
		if code := gen([]string{"errors", missing}, &stderr, &stderr); code != 1 ||
			!strings.Contains(stderr.String(), "monacoctl.writeErrorCodes: invalid_input") {
			t.Fatalf("gen errors %s = %d %q", missing, code, stderr.String())
		}
	}
	if os.Geteuid() == 0 {
		t.Skip("root writes through file modes")
	}
	path := writeSpec(t, staleSpec)
	if err := os.Chmod(path, 0o400); err != nil {
		t.Fatal(err)
	}
	stderr.Reset()
	if code := gen([]string{"errors", path}, &stderr, &stderr); code != 1 ||
		!strings.Contains(stderr.String(), "monacoctl.writeErrorCodes: internal") {
		t.Fatalf("read-only spec: gen = %d %q", code, stderr.String())
	}
}

func TestCommittedSpecListsEveryErrorCode(t *testing.T) {
	t.Parallel()
	spec, err := os.ReadFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := spliceErrorCodes(string(spec), errs.All())
	if err != nil {
		t.Fatal(err)
	}
	if fresh != string(spec) {
		t.Fatal("api/openapi.yaml ErrorCode enum differs from errs.All(); run go generate ./cmd/monacoctl")
	}
}

func TestGen_otherArgsPrintUsage(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{nil, {"errors"}, {"events", "x"}, {"errors", "a", "b"}} {
		var stdout, stderr bytes.Buffer
		if code := gen(args, &stdout, &stderr); code != 2 ||
			!strings.HasPrefix(stderr.String(), "usage: monacoctl gen errors <openapi.yaml>\n") ||
			!strings.Contains(stderr.String(), "\n       monacoctl gen module <name>\n") {
			t.Fatalf("gen %v = %d %q, want 2 and usage", args, code, stderr.String())
		}
	}
}

func TestGen_scaffoldsIntoTheWorkingDirAndPrintsWhatItWrote(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
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
		) || !strings.Contains(stderr.String(), "flows.tsv") {
		t.Fatalf("gen flow without flows.tsv = %d %q %q, want 1 and the error", code, stdout.String(), stderr.String())
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
	dir, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dir.Close() }()
	row := "01a\tSet handle\tidentity\tPOST /v1/me/handle\tSetHandle\t\t\tok;InvalidInput;crash:before-commit\tplanned\tdocs/x.md\n"
	if err := dir.MkdirAll("internal/modules/identity", 0o750); err != nil {
		t.Fatal(err)
	}
	for rel, body := range map[string]string{
		"go.mod":                              "module example.com/app\n",
		flows.File:                            flows.Header + "\n" + row,
		"internal/modules/identity/module.go": "package identity\n",
	} {
		if err := dir.WriteFile(rel, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
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
