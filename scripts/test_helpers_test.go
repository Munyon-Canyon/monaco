package scripts_test

import (
	"os"
	"path/filepath"
	"testing"
)

// Apple Git starts a detached auto maintenance repack after a commit,
// which races a hardlink clone of the same temp repo.
func TestMain(m *testing.M) {
	os.Setenv("GIT_CONFIG_COUNT", "1")
	os.Setenv("GIT_CONFIG_KEY_0", "maintenance.auto")
	os.Setenv("GIT_CONFIG_VALUE_0", "false")
	os.Exit(m.Run())
}

func repoRoot(t *testing.T) string {
	t.Helper()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if filepath.Base(wd) == "scripts" {
		return filepath.Dir(wd)
	}
	return wd
}
