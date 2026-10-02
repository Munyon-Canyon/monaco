package gen_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/tools/gen"
)

const (
	okMain   = "package main\n\nfunc main() {}\n"
	failMain = "package main\n\nimport \"os\"\n\nfunc main() { os.Exit(3) }\n"
	sqlcYAML = "version: \"2\"\nsql:\n  - engine: postgresql\n    queries: queries/platform\n  # BEGIN GENERATED modules\n  # END GENERATED modules\n"
)

func stubBackend(t *testing.T, overrides map[string]string) string {
	t.Helper()
	files := map[string]string{
		"apps/backend/go.mod":                       "module example.com/app\n\ngo 1.25\n",
		"apps/backend/CHANGELOG.md":                 changelog,
		"apps/backend/sqlc.yaml":                    sqlcYAML,
		"apps/backend/scripts/gen-golangci/main.go": okMain,
		"apps/backend/scripts/gen-registry/main.go": okMain,
		".bin/sqlc": "#!/bin/sh\nexit 0\n",
	}
	for rel, body := range overrides {
		if body == "" {
			delete(files, rel)
			continue
		}
		files[rel] = body
	}
	repo := tree(t, files)
	if err := os.Chmod(filepath.Join(repo, ".bin", "sqlc"), 0o700); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return filepath.Join(repo, "apps", "backend")
}

func run(t *testing.T, root, kind string, args ...string) ([]string, error) {
	t.Helper()
	g, ok := gen.Find(kind)
	if !ok {
		t.Fatalf("no generator %q", kind)
	}
	return g.Run(t.Context(), root, args)
}

func TestRun_regeneratesAfterAModuleAndSyncsSqlcForItsQueries(t *testing.T) {
	t.Parallel()
	root := stubBackend(t, nil)
	if _, err := run(t, root, "module", "wallets"); err != nil {
		t.Fatal(err)
	}
	if got := read(t, root, "sqlc.yaml"); got != sqlcYAML {
		t.Fatalf("a module with no queries changed sqlc.yaml:\n%s", got)
	}
	if _, err := run(t, root, "query", "wallets", "GetWallet"); err != nil {
		t.Fatal(err)
	}
	got := read(t, root, "sqlc.yaml")
	for _, want := range []string{
		"  # BEGIN GENERATED modules\n  - engine: postgresql\n    schema: migrations\n    queries: queries/wallets\n",
		"      out: internal/modules/wallets/sqlc\n",
		"          go_type: time.Time\n  # END GENERATED modules\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("sqlc.yaml lacks %q:\n%s", want, got)
		}
	}
	if _, err := run(t, root, "query", "wallets", "ListWallets"); err != nil {
		t.Fatal(err)
	}
	if again := read(t, root, "sqlc.yaml"); again != got {
		t.Fatalf("a second query changed sqlc.yaml:\n%s", again)
	}
}

func TestRun_namesTheStepThatFailed(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		overrides map[string]string
		args      []string
		want      string
	}{
		"gen-golangci fails": {
			map[string]string{"apps/backend/scripts/gen-golangci/main.go": failMain}, []string{"module", "ledger"}, "gen-golangci",
		},
		"gen-registry fails": {
			map[string]string{"apps/backend/scripts/gen-registry/main.go": failMain}, []string{"module", "ledger"}, "gen-registry",
		},
		"sqlc.yaml missing": {map[string]string{"apps/backend/sqlc.yaml": ""}, []string{"module", "ledger"}, "sqlc.yaml"},
		"sqlc.yaml unmarked": {
			map[string]string{"apps/backend/sqlc.yaml": "version: \"2\"\n"}, []string{"module", "ledger"}, "# BEGIN GENERATED modules",
		},
		"sqlc fails": {
			map[string]string{".bin/sqlc": "#!/bin/sh\necho broken query >&2\nexit 1\n"},
			[]string{"query", "wallets", "GetWallet"},
			"broken query",
		},
		"query regenerate fails": {
			map[string]string{"apps/backend/scripts/gen-registry/main.go": failMain},
			[]string{"query", "wallets", "GetWallet"},
			"gen-registry",
		},
		"no go.mod":    {map[string]string{"apps/backend/go.mod": ""}, []string{"provider", "quotes"}, "gen.modulePath"},
		"no changelog": {map[string]string{"apps/backend/CHANGELOG.md": ""}, []string{"module", "ledger"}, "CHANGELOG.md"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := stubBackend(t, tc.overrides)
			if tc.args[0] == "query" {
				if _, err := gen.Apply(root, "module", "wallets"); err != nil {
					t.Fatal(err)
				}
			}
			_, err := run(t, root, tc.args[0], tc.args[1:]...)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestRegenerate_failsWhenModulesIsNotADirectory(t *testing.T) {
	t.Parallel()
	root := stubBackend(t, map[string]string{"apps/backend/internal/modules": "x"})
	if err := gen.Regenerate(t.Context(), root); err == nil || !strings.Contains(err.Error(), "gen.queryModules") {
		t.Fatalf("err = %v, want gen.queryModules", err)
	}
	if err := gen.Regenerate(
		t.Context(),
		filepath.Join(root, "missing"),
	); err == nil ||
		!strings.Contains(err.Error(), "gen.syncSqlc") {
		t.Fatalf("err = %v, want gen.syncSqlc", err)
	}
}

func TestRun_failsOnAMissingRootAndOnUnwritableTargets(t *testing.T) {
	t.Parallel()
	root := stubBackend(t, nil)
	if _, err := run(
		t,
		filepath.Join(root, "missing"),
		"provider",
		"quotes",
	); err == nil ||
		!strings.Contains(err.Error(), "gen.write") {
		t.Fatalf("missing root err = %v, want gen.write", err)
	}
	internal := filepath.Join(root, "internal", "providers")
	if err := os.MkdirAll(internal, 0o750); err != nil {
		t.Fatal(err)
	}
	freeze(t, internal)
	if _, err := run(t, root, "provider", "quotes"); err == nil || !strings.Contains(err.Error(), "gen.writeFile") {
		t.Fatalf("read-only internal/ err = %v, want gen.writeFile", err)
	}
	freeze(t, filepath.Join(root, "CHANGELOG.md"))
	if _, err := run(t, root, "module", "ledger"); err == nil || !strings.Contains(err.Error(), "gen.writeFile") {
		t.Fatalf("read-only CHANGELOG.md err = %v, want gen.writeFile", err)
	}
}

func TestConsumer_refusesAModuleGoThatDoesNotParse(t *testing.T) {
	t.Parallel()
	root := withModule(t)
	if err := os.WriteFile(
		filepath.Join(root, "internal/modules/wallets/module.go"),
		[]byte("package wallets\n\nfunc {"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := gen.Apply(
		root,
		"consumer",
		"wallets",
		"onDeposit",
	); err == nil ||
		!strings.Contains(err.Error(), "parse:") {
		t.Fatalf("err = %v, want a parse error", err)
	}
}
