package testkit

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestChildCoverDir_isolatesEachChildAndMergesItsFilesIntoTheSharedDir(t *testing.T) {
	t.Parallel()
	shared := t.TempDir()
	t.Cleanup(func() {
		entries, err := os.ReadDir(shared)
		if err != nil {
			t.Fatal(err)
		}
		got := make([]string, 0, len(entries))
		for _, e := range entries {
			got = append(got, e.Name())
		}
		if want := []string{"covcounters.aa.1.1", "covmeta.aa"}; !slices.Equal(got, want) {
			t.Fatalf("shared GOCOVERDIR holds %q after the children's cleanup, want %q", got, want)
		}
	})
	names := []string{"covmeta.aa", "covcounters.aa.1.1"}
	dirs := make([]string, 0, len(names))
	for _, name := range names {
		dir := childCoverDir(t, shared)
		if dir == shared || slices.Contains(dirs, dir) {
			t.Fatalf("child GOCOVERDIR %q is shared with %q or %q", dir, shared, dirs)
		}
		dirs = append(dirs, dir)
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestMainCommand_passesGOCOVERDIROnlyWhenThisBinaryIsInstrumented(t *testing.T) {
	t.Parallel()
	env := os.Environ()
	cmd := MainCommand(t, env, "-h")
	var added []string
	for _, kv := range cmd.Env[len(env):] {
		if strings.HasPrefix(kv, "GOCOVERDIR=") {
			added = append(added, kv)
		}
	}
	want := coverDirFlag() != "" && testing.CoverMode() != ""
	if (len(added) == 1) != want || len(added) > 1 {
		t.Fatalf("MainCommand added GOCOVERDIR entries %q; this binary covermode %q, gocoverdir flag %q",
			added, testing.CoverMode(), coverDirFlag())
	}
	if want && !strings.HasPrefix(added[0], "GOCOVERDIR="+coverDirFlag()+string(filepath.Separator)+"child") {
		t.Fatalf("child GOCOVERDIR %q is not a child dir of %q", added[0], coverDirFlag())
	}
}
