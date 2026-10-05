package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func runDatabaseURLGuard(t *testing.T, databaseURL string) error {
	t.Helper()

	root := repoRoot(t)
	script := filepath.Join(root, "scripts", "assert-local-database-url.sh")
	cmd := exec.Command("bash", script)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "DATABASE_URL="+databaseURL)
	return cmd.Run()
}

func TestDatabaseURLGuard_rejectsHostedSupabaseUrl(t *testing.T) {
	// Arrange
	databaseURL := "postgres://user:pass@abc.supabase.co:5432/postgres"

	// Act
	err := runDatabaseURLGuard(t, databaseURL)

	// Assert
	if err == nil {
		t.Fatal("expected error for hosted Supabase DATABASE_URL")
	}
}

func TestDatabaseURLGuard_acceptsLocalhostComposeUrl(t *testing.T) {
	// Arrange
	databaseURL := "postgres://monaco:monaco@localhost:54322/monaco?sslmode=disable"

	// Act
	err := runDatabaseURLGuard(t, databaseURL)

	// Assert
	if err != nil {
		t.Fatalf("expected localhost DATABASE_URL to pass guard: %v", err)
	}
}

func TestDatabaseURLGuard_rejectsTheDurabilityOffTestDatabase(t *testing.T) {
	for _, databaseURL := range []string{
		"postgres://monaco:monaco@localhost:54323/monaco?sslmode=disable",
		"postgres://monaco:monaco@127.0.0.1:54323",
		"postgres://monaco:monaco@localhost:54326/monaco?sslmode=disable",
		"postgres://monaco:monaco@localhost:54338/monaco",
	} {
		if err := runDatabaseURLGuard(t, databaseURL); err == nil {
			t.Fatalf("expected error for DATABASE_URL on the test container: %s", databaseURL)
		}
	}
}
