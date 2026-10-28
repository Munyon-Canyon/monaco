package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func seedFixture(t *testing.T) (primary, lane string) {
	t.Helper()
	primary = t.TempDir()
	lane = filepath.Join(primary, ".worktrees", "lane")
	for _, args := range [][]string{
		{"init", "-q"},
		{"-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "-q", "--allow-empty", "-m", "init"},
		{"worktree", "add", "-q", "--detach", lane},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = primary
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return primary, lane
}

func runSeed(t *testing.T, lane string) string {
	t.Helper()
	out, err := exec.Command(filepath.Join(repoRoot(t), "scripts", "seed-lane-caches.sh"), lane).CombinedOutput()
	if err != nil {
		t.Fatalf("seed-lane-caches.sh must exit 0: %v\n%s", err, out)
	}
	return string(out)
}

func TestSeedLaneCaches_clonesTheCachesIntoANewLane(t *testing.T) {
	primary, lane := seedFixture(t)
	if err := os.MkdirAll(filepath.Join(primary, "packages", "mobile-core", ".build"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(primary, "packages", "mobile-core", ".build", "build.db"), "swiftpm")
	if err := os.MkdirAll(filepath.Join(primary, ".build", "DerivedData", "Build"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(lane, "packages", "mobile-core"), 0o755); err != nil {
		t.Fatal(err)
	}
	out := runSeed(t, lane)
	if runtime.GOOS != "darwin" {
		if strings.Contains(out, "seeded") || strings.Count(out, "skipped") != 2 {
			t.Fatalf("off macOS the script only skips:\n%s", out)
		}
		return
	}
	for _, want := range []string{"seeded packages/mobile-core/.build from ", "seeded .build/DerivedData from "} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in\n%s", want, out)
		}
	}
	got, err := os.ReadFile(filepath.Join(lane, "packages", "mobile-core", ".build", "build.db"))
	if err != nil || string(got) != "swiftpm" {
		t.Fatalf("clone content: %q %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(lane, ".build", "DerivedData", "Build")); err != nil {
		t.Fatal(err)
	}
}

func TestSeedLaneCaches_skipsWhenTheDestinationExists(t *testing.T) {
	primary, lane := seedFixture(t)
	if err := os.MkdirAll(filepath.Join(primary, "packages", "mobile-core", ".build"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(primary, "packages", "mobile-core", ".build", "build.db"), "swiftpm")
	if err := os.MkdirAll(filepath.Join(lane, "packages", "mobile-core", ".build"), 0o755); err != nil {
		t.Fatal(err)
	}
	out := runSeed(t, lane)
	want := "skipped packages/mobile-core/.build: destination exists"
	if runtime.GOOS != "darwin" {
		want = "skipped packages/mobile-core/.build: cp -c needs APFS on macOS"
	}
	if !strings.Contains(out, want) {
		t.Fatalf("got\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(lane, "packages", "mobile-core", ".build", "build.db")); !os.IsNotExist(err) {
		t.Fatalf("destination was overwritten: %v", err)
	}
}

func TestSeedLaneCaches_skipsWhenThePrimaryHasNoCache(t *testing.T) {
	_, lane := seedFixture(t)
	out := runSeed(t, lane)
	if strings.Contains(out, "seeded") || strings.Count(out, "skipped") != 2 {
		t.Fatalf("got\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(lane, ".build")); !os.IsNotExist(err) {
		t.Fatalf("created a destination without a source: %v", err)
	}
}
