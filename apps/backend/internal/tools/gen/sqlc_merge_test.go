package gen_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const anchoredSqlcYAML = `version: "2"
sql:
  - engine: postgresql
    schema: migrations
    queries: queries/platform
    gen:
      go: &common
        package: sqlc
        out: internal/platform/db/sqlc
        sql_package: pgx/v5
        emit_interface: false
        omit_unused_structs: true
  # BEGIN GENERATED modules
  # END GENERATED modules
`

func git(t *testing.T, repo string, args ...string) {
	t.Helper()
	full := append([]string{"-C", repo, "-c", "user.name=t", "-c", "user.email=t@example.com"}, args...)
	if out, err := exec.CommandContext(t.Context(), "git", full...).CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func writeIn(t *testing.T, root, rel, body string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestSqlc_aSharedSettingChangeAndANewModuleMergeToTheGeneratorsOutput(t *testing.T) {
	t.Parallel()
	root := stubBackend(t, map[string]string{
		"apps/backend/sqlc.yaml":                        anchoredSqlcYAML,
		"apps/backend/internal/modules/alpha/module.go": "package alpha\n",
		"apps/backend/queries/alpha/get_alpha.sql":      "-- name: GetAlpha :one\nSELECT 1;\n",
	})
	repo := filepath.Dir(filepath.Dir(root))
	if _, err := run(t, root, "sqlc"); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "init", "-q", "-b", "base")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-q", "-m", "base")

	git(t, repo, "checkout", "-q", "-b", "shared-setting")
	yaml := read(t, root, "sqlc.yaml")
	const flag = "        omit_unused_structs: true\n"
	writeIn(t, root, "sqlc.yaml", strings.Replace(yaml, flag, flag+"        emit_exact_table_names: true\n", 1))
	git(t, repo, "commit", "-q", "-am", "shared setting")

	git(t, repo, "checkout", "-q", "base")
	git(t, repo, "checkout", "-q", "-b", "new-module")
	writeIn(t, root, "internal/modules/wallets/module.go", "package wallets\n")
	writeIn(t, root, "queries/wallets/get_wallet.sql", "-- name: GetWallet :one\nSELECT 1;\n")
	if _, err := run(t, root, "sqlc"); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-q", "-m", "new module")

	git(t, repo, "merge", "-q", "--no-edit", "shared-setting")
	merged := read(t, root, "sqlc.yaml")
	if _, err := run(t, root, "sqlc"); err != nil {
		t.Fatal(err)
	}
	if regenerated := read(t, root, "sqlc.yaml"); regenerated != merged {
		t.Fatalf("git's merge differs from monacoctl gen sqlc:\n--- merged\n%s\n--- generated\n%s", merged, regenerated)
	}
	for _, want := range []string{"emit_exact_table_names: true", "queries: queries/wallets,"} {
		if !strings.Contains(merged, want) {
			t.Errorf("merged sqlc.yaml lacks %q:\n%s", want, merged)
		}
	}
}
