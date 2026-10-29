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

func TestMainCommand_givesTheChildGOCOVERDIROnlyWhenThisBinaryIsInstrumented(t *testing.T) {
	t.Parallel()
	env := append(os.Environ(), "GOCOVERDIR=/inherited/from/go/test")
	cmd := MainCommand(t, env, "-h")
	var got []string
	for _, kv := range cmd.Env {
		if strings.HasPrefix(kv, "GOCOVERDIR=") {
			got = append(got, kv)
		}
	}
	instrumented := coverDirFlag() != "" && testing.CoverMode() != ""
	if instrumented {
		if len(got) != 1 ||
			!strings.HasPrefix(got[0], "GOCOVERDIR="+coverDirFlag()+string(filepath.Separator)+"child") {
			t.Fatalf(
				"instrumented binary gave the child GOCOVERDIR entries %q, want one child dir under %q",
				got,
				coverDirFlag(),
			)
		}
		return
	}
	if len(got) != 0 {
		t.Fatalf(
			"uninstrumented binary (covermode %q) gave the child GOCOVERDIR entries %q, want none",
			testing.CoverMode(),
			got,
		)
	}
}
