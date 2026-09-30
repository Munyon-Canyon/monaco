package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestMain(m *testing.M) {
	testkit.Main(m, testkit.NoDB(), testkit.WithChild(main))
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
    rules:
      # BEGIN GENERATED exclusions
      - path: stale
      # END GENERATED exclusions
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
	b, err := os.ReadFile(filepath.Join(root, outFile))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestGenerate_writesTwoSortedWallsPerModuleBetweenMarkers(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"go.mod":                               "module example.com/app\n\ngo 1.25\n",
		".golangci.base.yml":                   testConfig,
		"internal/modules/gamma/module.go":     "package gamma\n",
		"internal/modules/beta/module.go":      "package beta\n",
		"internal/modules/alpha/module.go":     "package alpha\n",
		"internal/modules/README-not-a-dir.md": "x\n",
	})

	if err := generate(root); err != nil {
		t.Fatal(err)
	}

	want := header + `linters:
  settings:
    depguard:
      rules:
        domain:
          files: ["**/domain/**"]
        # BEGIN GENERATED depguard
        module-alpha: { list-mode: lax, files: ["**/internal/modules/alpha/**", "!**/internal/modules/alpha/adapters/**", "!**/internal/modules/alpha/domain/**"], allow: ["example.com/app/internal/modules/alpha$", "example.com/app/internal/modules/alpha/", "example.com/app/internal/modules/beta$", "example.com/app/internal/modules/gamma$"], deny: [{ pkg: "example.com/app/internal/modules/", desc: "modules never import each other; send an event or use a query port" }] }
        module-alpha-inner: { list-mode: lax, files: ["**/internal/modules/alpha/adapters/**", "**/internal/modules/alpha/domain/**"], allow: ["example.com/app/internal/modules/alpha$", "example.com/app/internal/modules/alpha/"], deny: [{ pkg: "example.com/app/internal/modules/", desc: "domain and adapters import no other module; the module root and app read a query port" }] }
        module-beta: { list-mode: lax, files: ["**/internal/modules/beta/**", "!**/internal/modules/beta/adapters/**", "!**/internal/modules/beta/domain/**"], allow: ["example.com/app/internal/modules/beta$", "example.com/app/internal/modules/beta/", "example.com/app/internal/modules/alpha$", "example.com/app/internal/modules/gamma$"], deny: [{ pkg: "example.com/app/internal/modules/", desc: "modules never import each other; send an event or use a query port" }] }
        module-beta-inner: { list-mode: lax, files: ["**/internal/modules/beta/adapters/**", "**/internal/modules/beta/domain/**"], allow: ["example.com/app/internal/modules/beta$", "example.com/app/internal/modules/beta/"], deny: [{ pkg: "example.com/app/internal/modules/", desc: "domain and adapters import no other module; the module root and app read a query port" }] }
        module-gamma: { list-mode: lax, files: ["**/internal/modules/gamma/**", "!**/internal/modules/gamma/adapters/**", "!**/internal/modules/gamma/domain/**"], allow: ["example.com/app/internal/modules/gamma$", "example.com/app/internal/modules/gamma/", "example.com/app/internal/modules/alpha$", "example.com/app/internal/modules/beta$"], deny: [{ pkg: "example.com/app/internal/modules/", desc: "modules never import each other; send an event or use a query port" }] }
        module-gamma-inner: { list-mode: lax, files: ["**/internal/modules/gamma/adapters/**", "**/internal/modules/gamma/domain/**"], allow: ["example.com/app/internal/modules/gamma$", "example.com/app/internal/modules/gamma/"], deny: [{ pkg: "example.com/app/internal/modules/", desc: "domain and adapters import no other module; the module root and app read a query port" }] }
        # END GENERATED depguard
  exclusions:
    presets: [comments]
    rules:
      # BEGIN GENERATED exclusions
      # END GENERATED exclusions
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
		"go.mod":             "module example.com/app\n",
		".golangci.base.yml": testConfig,
	})

	if err := generate(root); err != nil {
		t.Fatal(err)
	}

	want := header + `linters:
  settings:
    depguard:
      rules:
        domain:
          files: ["**/domain/**"]
        # BEGIN GENERATED depguard
        # END GENERATED depguard
  exclusions:
    presets: [comments]
    rules:
      # BEGIN GENERATED exclusions
      # END GENERATED exclusions
`
	if got := readConfig(t, root); got != want {
		t.Fatalf("config after generate:\n%s\nwant:\n%s", got, want)
	}
}

func TestGenerate_failsWithoutMarkersOrModuleLine(t *testing.T) {
	t.Parallel()
	for name, files := range map[string]map[string]string{
		"no markers":     {"go.mod": "module example.com/app\n", ".golangci.base.yml": "linters: {}\n"},
		"no module line": {"go.mod": "go 1.25\n", ".golangci.base.yml": testConfig},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := writeTree(t, files)
			if err := generate(root); err == nil {
				t.Fatal("generate succeeded, want an error")
			}
			if _, err := os.Stat(filepath.Join(root, outFile)); !os.IsNotExist(err) {
				t.Fatalf("a failed generate wrote %s: %v", outFile, err)
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
		"missing go.mod":    {map[string]string{".golangci.base.yml": testConfig}, false, "read go.mod: "},
		"modules is a file": {map[string]string{"go.mod": "module m\n", "internal/modules": "x"}, false, "list modules: "},
		"missing config":    {map[string]string{"go.mod": "module m\n"}, false, "read config: "},
		"read-only config": {
			map[string]string{"go.mod": "module m\n", ".golangci.base.yml": testConfig, outFile: "stale\n"},
			true, "write config: ",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := writeTree(t, tc.files)
			if tc.readOnly {
				if err := os.Chmod(filepath.Join(root, outFile), 0o400); err != nil {
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
		".golangci.base.yml":               testConfig,
		"internal/modules/alpha/module.go": "package alpha\n",
	})
	cmd := testkit.MainCommand(t, nil)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil || len(out) != 0 ||
		!strings.Contains(readConfig(t, root), "module-alpha:") || strings.Contains(readConfig(t, root), "stale-rule") {
		t.Fatalf("gen-golangci in %s = %v %q\n%s", root, err, out, readConfig(t, root))
	}

	missing := filepath.Join(root, "missing")
	failing := testkit.MainCommand(t, nil, missing)
	out, _ := failing.CombinedOutput()
	if code := failing.ProcessState.ExitCode(); code != 1 ||
		!strings.HasPrefix(string(out), "gen-golangci: read go.mod: ") {
		t.Fatalf("gen-golangci %s = %d %q", missing, code, out)
	}
}

func TestGenerate_addsEachModulesLintExclusionsWithItsReason(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"go.mod": "module example.com/app\n",
		".golangci.base.yml": "rules:\n  # BEGIN GENERATED depguard\n  # END GENERATED depguard\n" +
			"exclusions:\n  # BEGIN GENERATED exclusions\n  # END GENERATED exclusions\n",
		"internal/modules/beta/lint.yml": "- path: ^internal/modules/beta/adapters/privy\\.go$\n" +
			"  linters: [gosec]\n  text: G404\n  reason: Privy's SDK wants math/rand for jitter\n",
		"internal/modules/alpha/lint.yml": "- path: ^internal/modules/alpha/app/'quoted'\\.go$\n" +
			"  linters: [funlen, cyclop]\n  reason: one table-driven switch\n",
	})
	if err := generate(root); err != nil {
		t.Fatal(err)
	}
	got := readConfig(t, root)
	want := "exclusions:\n  # BEGIN GENERATED exclusions\n" +
		"  - { path: '^internal/modules/alpha/app/''quoted''\\.go$', linters: [funlen, cyclop] } " +
		"# alpha: one table-driven switch\n" +
		"  - { path: '^internal/modules/beta/adapters/privy\\.go$', linters: [gosec], text: 'G404' } " +
		"# beta: Privy's SDK wants math/rand for jitter\n" +
		"  # END GENERATED exclusions\n"
	if !strings.HasSuffix(got, want) {
		t.Fatalf("config:\n%s\nwant it to end with:\n%s", got, want)
	}
}

func TestGenerate_rejectsAModuleExclusionThatReachesOutsideItOrHasNoReason(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct{ lint, want string }{
		"other module path": {
			"- {path: ^internal/modules/beta/x\\.go$, linters: [gosec], reason: r}\n",
			`rule 1 needs a path starting "^internal/modules/alpha/", linters and a one-line reason`,
		},
		"no reason":       {"- {path: ^internal/modules/alpha/x\\.go$, linters: [gosec]}\n", "rule 1 needs"},
		"no linters":      {"- {path: ^internal/modules/alpha/x\\.go$, reason: r}\n", "rule 1 needs"},
		"two-line reason": {"- {path: ^internal/modules/alpha/x\\.go$, linters: [gosec], reason: \"a\\nb\"}\n", "rule 1 needs"},
		"unknown field":   {"- {path-except: x, linters: [gosec], reason: r}\n", "field path-except not found"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := writeTree(t, map[string]string{
				"go.mod":                          "module m\n",
				".golangci.base.yml":              testConfig,
				"internal/modules/alpha/lint.yml": tc.lint,
			})
			if err := generate(root); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("generate = %v, want an error containing %q", err, tc.want)
			}
		})
	}
}

func TestGenerate_namesAModuleLintFileItCannotRead(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"go.mod":                               "module m\n",
		".golangci.base.yml":                   testConfig,
		"internal/modules/alpha/lint.yml/keep": "x",
	})
	if err := generate(root); err == nil || !strings.HasPrefix(err.Error(), "read "+root) {
		t.Fatalf("generate = %v, want a read error naming the lint file", err)
	}
}

func TestRepoConfig_isWhatTheGeneratorWrites(t *testing.T) {
	t.Parallel()
	want, err := render("../..")
	if err != nil {
		t.Fatal(err)
	}
	if got := readConfig(t, "../.."); got != string(want) {
		t.Fatalf("%s differs from what scripts/gen-golangci renders; edit %s and run go generate ./...",
			outFile, baseFile)
	}
}
