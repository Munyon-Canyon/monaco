package gen_test

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/platform/lint/comments"
	"github.com/monaco/monaco/apps/backend/internal/tools/gen"
)

type step struct {
	kind string
	args []string
}

func scenario() []step {
	return []step{
		{"module", []string{"example"}},
		{"command", []string{"example", "DoThing"}},
		{"query", []string{"example", "GetThing"}},
		{"consumer", []string{"example", "onThing"}},
		{"provider", []string{"example"}},
		{"flow", []string{"1"}},
	}
}

func notImplemented() []string {
	return []string{
		"TestDoThing_appendsItsEvent",
		"TestOnThing_convergesUnderChaos",
		"TestFlow1_DoThing_OK",
		"TestFlow1_DoThing_invalid_input",
		"TestFlow1_DoThing_CrashBeforeCommit",
	}
}

func generatedDirs() []string {
	return []string{
		"internal/modules/example",
		"queries/example",
		"internal/providers/example",
		"internal/testkit/fakes/testdata/fakes/example",
	}
}

func ownedByExample(rel string) bool {
	return slices.ContainsFunc(generatedDirs(), func(dir string) bool { return strings.HasPrefix(rel, dir+"/") }) ||
		slices.Contains([]string{
			"api/spec/example.yaml",
			"cmd/api/module_example.gen.go",
			"cmd/worker/module_example.gen.go",
			"cmd/monacoctl/module_example.gen.go",
			".golangci.yml",
			"sqlc.yaml",
			"CHANGELOG.md",
		}, rel)
}

func requireTools(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("copies the backend and builds, lints and tests it; CI runs it without -short")
	}
	for _, tool := range []string{"golangci-lint", "sqlc"} {
		if _, err := exec.LookPath(tool); err == nil {
			continue
		}
		if _, err := os.Stat(filepath.Join("..", "..", "..", "..", "..", ".bin", tool)); err == nil {
			continue
		}
		if _, ci := os.LookupEnv("CI"); ci {
			t.Fatal(tool + " must be on PATH in CI")
		}
		t.Skip(tool + " is not on PATH; run just install to prove the generators locally")
	}
}

func copyBackend(t *testing.T) string {
	t.Helper()
	src, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	repo := t.TempDir()
	root := filepath.Join(repo, "apps", "backend")
	if err := os.CopyFS(root, os.DirFS(src)); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(src, "..", "..", ".bin")
	if _, err := os.Stat(bin); err == nil {
		if err := os.Symlink(bin, filepath.Join(repo, ".bin")); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func snapshot(t *testing.T, root string) map[string][sha256.Size]byte {
	t.Helper()
	out := map[string][sha256.Size]byte{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		out[filepath.ToSlash(rel)] = sha256.Sum256(b)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func changed(before, after map[string][sha256.Size]byte) []string {
	var out []string
	for _, rel := range slices.Sorted(maps.Keys(after)) {
		if old, ok := before[rel]; !ok || old != after[rel] {
			out = append(out, rel)
		}
	}
	for _, rel := range slices.Sorted(maps.Keys(before)) {
		if _, ok := after[rel]; !ok {
			out = append(out, rel+" (removed)")
		}
	}
	return out
}

func must(t *testing.T, root, name string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), name, args...)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, out)
	}
}

func addFlow(t *testing.T, root string) {
	t.Helper()
	row := "1\tExample does a thing\texample\tPOST /v1/example\tDoThing\tsystem.pinged\t\t" +
		"ok;invalid_input;crash:before-commit\tplanned\tdocs/architecture/backend-platform.md\n"
	f, err := os.OpenFile(filepath.Join(root, "flows.tsv"), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(row); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func runScenario(t *testing.T, root string) {
	t.Helper()
	for _, s := range scenario() {
		g, ok := gen.Find(s.kind)
		if !ok {
			t.Fatalf("no generator %q", s.kind)
		}
		if _, err := g.Run(t.Context(), root, s.args); err != nil {
			t.Fatalf("gen %s %v: %v", s.kind, s.args, err)
		}
	}
}

func assertLintClean(t *testing.T, root string, pkgs []string) {
	t.Helper()
	must(t, root, "golangci-lint", append([]string{"run", "--allow-parallel-runners"}, pkgs...)...)
	for _, pkg := range pkgs {
		var findings bytes.Buffer
		if code := comments.Run([]string{filepath.Join(root, pkg)}, &findings, &findings); code != 0 {
			t.Errorf("generated code has comments:\n%s", findings.String())
		}
	}
}

func TestGolden_everyGeneratorEmitsCodeThatBuildsLintsCleanAndIsReversible(t *testing.T) {
	t.Parallel()
	requireTools(t)
	root := copyBackend(t)
	addFlow(t, root)
	before := snapshot(t, root)
	runScenario(t, root)
	for _, rel := range changed(before, snapshot(t, root)) {
		if !ownedByExample(rel) {
			t.Errorf("the generators touched %s, outside the example module's files", rel)
		}
	}

	must(t, root, "go", "build", "-trimpath", "./...")
	pkgs := []string{"./internal/modules/example/...", "./internal/providers/example/..."}
	assertLintClean(t, root, pkgs)
	assertOnlyNotImplementedFailures(t, root, pkgs)

	for _, dir := range generatedDirs() {
		if err := os.RemoveAll(filepath.Join(root, dir)); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Remove(filepath.Join(root, "api", "spec", "example.yaml")); err != nil {
		t.Fatal(err)
	}
	if err := gen.Regenerate(t.Context(), root); err != nil {
		t.Fatal(err)
	}
	if left := changed(before, snapshot(t, root)); !slices.Equal(left, []string{"CHANGELOG.md"}) {
		t.Fatalf("after removing the module the tree differs in %v, want only the CHANGELOG.md stub", left)
	}
}

type testEvent struct {
	Action  string `json:"Action"`
	Package string `json:"Package"`
	Test    string `json:"Test"`
	Output  string `json:"Output"`
}

func assertOnlyNotImplementedFailures(t *testing.T, root string, pkgs []string) {
	t.Helper()
	cmd := exec.CommandContext(
		t.Context(),
		"go",
		append([]string{"test", "-tags", "faultpoints", "-json", "-count=1"}, pkgs...)...)
	cmd.Dir = root
	out, _ := cmd.Output()
	output := map[string]*strings.Builder{}
	var failed []string
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 0, 1<<20), 1<<24)
	for sc.Scan() {
		var ev testEvent
		if err := json.Unmarshal(sc.Bytes(), &ev); err != nil {
			t.Fatalf("go test -json: %v\n%s", err, out)
		}
		key := ev.Package + " " + ev.Test
		if output[key] == nil {
			output[key] = &strings.Builder{}
		}
		output[key].WriteString(ev.Output)
		if ev.Action == "fail" {
			failed = append(failed, key)
		}
	}
	var tests []string
	for _, key := range failed {
		pkg, test, _ := strings.Cut(key, " ")
		if test != "" {
			tests = append(tests, test)
		}
		switch {
		case test != "" && !strings.Contains(output[key].String(), "not implemented"):
			t.Errorf("%s failed for a reason other than not implemented:\n%s", test, output[key].String())
		case test == "" && !slices.ContainsFunc(failed, func(k string) bool { return strings.HasPrefix(k, pkg+" Test") }):
			t.Errorf("%s failed without a failing test:\n%s", pkg, output[key].String())
		}
	}
	slices.Sort(tests)
	if want := slices.Sorted(slices.Values(notImplemented())); !slices.Equal(tests, want) {
		t.Errorf("failing generated tests = %v, want exactly %v", tests, want)
	}
}
