package gen_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/tools/gen"
)

func requireOneWalletsBlock(t *testing.T, got string) {
	t.Helper()
	want := "  # BEGIN GENERATED modules\n  - {engine: postgresql, schema: migrations, queries: queries/wallets, gen: {go: {<<: *common, package: sqlc, out: internal/modules/wallets/sqlc, output_db_file_name: db.gen.go, output_models_file_name: models.gen.go, output_files_suffix: .gen}}}\n  # END GENERATED modules\n"
	if !strings.Contains(got, want) {
		t.Errorf("sqlc.yaml lacks %q:\n%s", want, got)
	}
	if n := strings.Count(got, "queries: queries/wallets,"); n != 1 {
		t.Errorf("sqlc.yaml has %d wallets blocks, want 1:\n%s", n, got)
	}
}

func TestSqlc_writesTheBlockOfAModuleWhoseFirstQueryWasWrittenByHand(t *testing.T) {
	t.Parallel()
	const mergedBlock = "  - {engine: postgresql, queries: queries/wallets}\n  - {engine: postgresql, queries: queries/wallets}\n"
	for name, start := range map[string]string{
		"no block yet":        sqlcYAML,
		"a hand-merged block": strings.Replace(sqlcYAML, "  # END GENERATED", mergedBlock+"  # END GENERATED", 1),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := stubBackend(t, map[string]string{
				"apps/backend/sqlc.yaml":                          start,
				"apps/backend/internal/modules/wallets/module.go": "package wallets\n",
				"apps/backend/queries/wallets/get_wallet.sql":     "-- name: GetWallet :one\nSELECT 1;\n",
			})
			if _, err := run(t, root, "sqlc"); err != nil {
				t.Fatal(err)
			}
			got := read(t, root, "sqlc.yaml")
			requireOneWalletsBlock(t, got)
			if _, err := run(t, root, "sqlc"); err != nil {
				t.Fatal(err)
			}
			if again := read(t, root, "sqlc.yaml"); again != got {
				t.Fatalf("a second run changed sqlc.yaml:\n%s", again)
			}
		})
	}
}

func TestSqlc_takesNoArgumentsAndNamesAConfigWithoutMarkers(t *testing.T) {
	t.Parallel()
	g, _ := gen.Find("sqlc")
	if got := g.Usage(); got != "gen sqlc" {
		t.Fatalf("Usage() = %q, want %q", got, "gen sqlc")
	}
	_, err := run(t, stubBackend(t, nil), "sqlc", "wallets")
	if err == nil || !strings.Contains(err.Error(), "usage: gen sqlc") {
		t.Fatalf("err = %v, want the usage line", err)
	}
	unmarked := stubBackend(t, map[string]string{"apps/backend/sqlc.yaml": "version: \"2\"\n"})
	_, err = run(t, unmarked, "sqlc")
	if err == nil || !strings.Contains(err.Error(), "# BEGIN GENERATED modules") {
		t.Fatalf("err = %v, want the missing marker named", err)
	}
}

func TestGenerateSqlcAndHashMigrationsNameTheInstallScriptWhenTheToolIsMissing(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	dir := filepath.Join(t.TempDir(), "apps", "backend")
	for tool, run := range map[string]func(context.Context, string) error{
		"sqlc": gen.GenerateSqlc, "atlas": gen.HashMigrations,
	} {
		if err := run(
			t.Context(),
			dir,
		); err == nil ||
			!strings.Contains(err.Error(), "run scripts/install-"+tool+".sh") {
			t.Errorf("%s missing: err = %v, want the install script", tool, err)
		}
	}
}

func TestGenerateSqlcSyncsTheConfigBeforeItRunsSqlc(t *testing.T) {
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "sqlc"), []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	if err := gen.GenerateSqlc(t.Context(), t.TempDir()); err == nil || !strings.Contains(err.Error(), "gen.SyncSqlc") {
		t.Fatalf("GenerateSqlc without sqlc.yaml = %v, want the sync error", err)
	}
}
