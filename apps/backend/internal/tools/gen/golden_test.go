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
	}
}

func ownedByExample(rel string) bool {
	return strings.HasPrefix(rel, "internal/modules/example/") ||
		strings.HasPrefix(rel, "queries/example/") ||
		slices.Contains([]string{
			"cmd/api/module_example.gen.go",
			"cmd/worker/module_example.gen.go",
			"cmd/monacoctl/module_example.gen.go",
			".golangci.yml",
			"CHANGELOG.md",
		}, rel)
}

func requireTools(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("copies the backend and builds, lints and tests it; CI runs it without -short")
	}
	if _, err := exec.LookPath("golangci-lint"); err == nil {
		return
	}
	if _, ci := os.LookupEnv("CI"); ci {
		t.Fatal("golangci-lint must be on PATH in CI")
	}
	t.Skip("golangci-lint is not on PATH; run just install to prove the generators locally")
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

func TestGolden_everyGeneratorEmitsCodeThatBuildsLintsCleanAndIsReversible(t *testing.T) {
	t.Parallel()
	requireTools(t)
	root := copyBackend(t)
	before := snapshot(t, root)
	for _, s := range scenario() {
		g, ok := gen.Find(s.kind)
		if !ok {
			t.Fatalf("no generator %q", s.kind)
		}
		if _, err := g.Run(t.Context(), root, s.args); err != nil {
			t.Fatalf("gen %s %v: %v", s.kind, s.args, err)
		}
	}
	for _, rel := range changed(before, snapshot(t, root)) {
		if !ownedByExample(rel) {
			t.Errorf("the generators touched %s, outside the example module's files", rel)
		}
	}

	must(t, root, "go", "build", "-trimpath", "./...")
	must(t, root, "golangci-lint", "run", "--allow-parallel-runners", "./internal/modules/example/...")
	var findings bytes.Buffer
	if code := comments.Run(
		[]string{filepath.Join(root, "internal", "modules", "example") + "/..."},
		&findings,
		&findings,
	); code != 0 {
		t.Errorf("generated code has comments:\n%s", findings.String())
	}
	assertOnlyNotImplementedFailures(t, root)

	for _, dir := range []string{filepath.Join("internal", "modules", "example"), filepath.Join("queries", "example")} {
		if err := os.RemoveAll(filepath.Join(root, dir)); err != nil {
			t.Fatal(err)
		}
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

func assertOnlyNotImplementedFailures(t *testing.T, root string) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "go", "test", "-json", "-count=1", "./internal/modules/example/...")
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
	for _, key := range failed {
		pkg, test, _ := strings.Cut(key, " ")
		switch {
		case test != "" && !strings.Contains(output[key].String(), "not implemented"):
			t.Errorf("%s failed for a reason other than not implemented:\n%s", test, output[key].String())
		case test == "" && !slices.ContainsFunc(failed, func(k string) bool { return strings.HasPrefix(k, pkg+" Test") }):
			t.Errorf("%s failed without a failing test:\n%s", pkg, output[key].String())
		}
	}
}
