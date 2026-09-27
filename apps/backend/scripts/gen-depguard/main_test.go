package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestMain(m *testing.M) {
	testkit.RunMain(m, main)
}

const testConfig = `linters:
  settings:
    depguard:
      rules:
        domain:
          files: ["**/domain/**"]
        # BEGIN GENERATED depguard
        stale-rule:
          files: ["gone"]
        # END GENERATED depguard
  exclusions:
    presets: [comments]
`

func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, body := range files {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func readConfig(t *testing.T, root string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, ".golangci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestGenerate_writesOneSortedWallPerModuleBetweenMarkers(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"go.mod":                               "module example.com/app\n\ngo 1.25\n",
		".golangci.yml":                        testConfig,
		"internal/modules/beta/module.go":      "package beta\n",
		"internal/modules/alpha/module.go":     "package alpha\n",
		"internal/modules/README-not-a-dir.md": "x\n",
	})

	if err := generate(root); err != nil {
		t.Fatal(err)
	}

	want := `linters:
  settings:
    depguard:
      rules:
        domain:
          files: ["**/domain/**"]
        # BEGIN GENERATED depguard
        module-alpha:
          list-mode: lax
          files: ["**/internal/modules/alpha/**"]
          allow: ["example.com/app/internal/modules/alpha$", "example.com/app/internal/modules/alpha/"]
          deny:
            - pkg: "example.com/app/internal/modules/"
              desc: "modules never import each other; send an event or use a query port"
        module-beta:
          list-mode: lax
          files: ["**/internal/modules/beta/**"]
          allow: ["example.com/app/internal/modules/beta$", "example.com/app/internal/modules/beta/"]
          deny:
            - pkg: "example.com/app/internal/modules/"
              desc: "modules never import each other; send an event or use a query port"
        # END GENERATED depguard
  exclusions:
    presets: [comments]
`
	if got := readConfig(t, root); got != want {
		t.Fatalf("config after generate:\n%s\nwant:\n%s", got, want)
	}
	if err := generate(root); err != nil {
		t.Fatal(err)
	}
	if got := readConfig(t, root); got != want {
		t.Fatalf("second generate changed the config:\n%s", got)
	}
}

func TestGenerate_emptiesTheBlockWhenThereAreNoModules(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"go.mod":        "module example.com/app\n",
		".golangci.yml": testConfig,
	})

	if err := generate(root); err != nil {
		t.Fatal(err)
	}

	want := `linters:
  settings:
    depguard:
      rules:
        domain:
          files: ["**/domain/**"]
        # BEGIN GENERATED depguard
        # END GENERATED depguard
  exclusions:
    presets: [comments]
`
	if got := readConfig(t, root); got != want {
		t.Fatalf("config after generate:\n%s\nwant:\n%s", got, want)
	}
}

func TestGenerate_failsWithoutMarkersOrModuleLine(t *testing.T) {
	t.Parallel()
	for name, files := range map[string]map[string]string{
		"no markers":     {"go.mod": "module example.com/app\n", ".golangci.yml": "linters: {}\n"},
		"no module line": {"go.mod": "go 1.25\n", ".golangci.yml": testConfig},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := writeTree(t, files)
			before := readConfig(t, root)
			if err := generate(root); err == nil {
				t.Fatal("generate succeeded, want an error")
			}
			if readConfig(t, root) != before {
				t.Fatal("a failed generate rewrote the config")
			}
		})
	}
}

func TestGenerate_namesTheInputItCannotReadOrWrite(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		files    map[string]string
		readOnly bool
		want     string
	}{
		"missing go.mod":    {map[string]string{".golangci.yml": testConfig}, false, "read go.mod: "},
		"modules is a file": {map[string]string{"go.mod": "module m\n", "internal/modules": "x"}, false, "list modules: "},
		"missing config":    {map[string]string{"go.mod": "module m\n"}, false, "read config: "},
		"read-only config": {
			map[string]string{"go.mod": "module m\n", ".golangci.yml": testConfig}, true, "write config: ",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := writeTree(t, tc.files)
			if tc.readOnly {
				if err := os.Chmod(filepath.Join(root, ".golangci.yml"), 0o400); err != nil {
					t.Fatal(err)
				}
			}
			if err := generate(root); err == nil || !strings.HasPrefix(err.Error(), tc.want) {
				t.Fatalf("generate = %v, want an error starting %q", err, tc.want)
			}
		})
	}
}

func TestMain_generatesInTheWorkingDirOrExitsOneNamingTheError(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"go.mod":                           "module example.com/app\n",
		".golangci.yml":                    testConfig,
		"internal/modules/alpha/module.go": "package alpha\n",
	})
	cmd := testkit.MainCommand(t, nil)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil || len(out) != 0 ||
		!strings.Contains(readConfig(t, root), "module-alpha:") || strings.Contains(readConfig(t, root), "stale-rule") {
		t.Fatalf("gen-depguard in %s = %v %q\n%s", root, err, out, readConfig(t, root))
	}

	missing := filepath.Join(root, "missing")
	failing := testkit.MainCommand(t, nil, missing)
	out, _ := failing.CombinedOutput()
	if code := failing.ProcessState.ExitCode(); code != 1 ||
		!strings.HasPrefix(string(out), "gen-depguard: read go.mod: ") {
		t.Fatalf("gen-depguard %s = %d %q", missing, code, out)
	}
}
