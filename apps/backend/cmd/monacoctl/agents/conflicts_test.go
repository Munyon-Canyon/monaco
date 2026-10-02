package agents

import (
	"context"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

func TestMergeTree_cleanMergeReportsNoFiles(t *testing.T) {
	t.Parallel()
	behind, files, err := runMerge(t, "tree-id\n", nil)
	if err != nil || behind || files != nil {
		t.Fatalf("behind %v files %v err %v", behind, files, err)
	}
}

func TestMergeTree_conflictReportsThePathsAfterTheTreeID(t *testing.T) {
	t.Parallel()
	_, files, err := runMerge(t, "tree-id\na.go\nb.go\n", exitOne(t))
	if err != nil || !slices.Equal(files, []string{"a.go", "b.go"}) {
		t.Fatalf("files %v err %v", files, err)
	}
}

func TestMergeTree_otherExitStatusIsAnError(t *testing.T) {
	t.Parallel()
	_, files, err := runMerge(t, "tree-id\na.go\n", exitTwo(t))
	if err == nil || files != nil {
		t.Fatalf("files %v err %v", files, err)
	}
}

func runMerge(t *testing.T, out string, mergeErr error) (bool, []string, error) {
	t.Helper()
	env := newFixture(t).Env(t)
	saw := false
	env.Run = func(_ context.Context, _, _, name string, args ...string) ([]byte, error) {
		line := strings.Join(args, " ")
		switch {
		case name == "git" && strings.Contains(line, "--verify"):
			return []byte("ok"), nil
		case name == "git" && strings.Contains(line, "merge-tree"):
			saw = strings.Contains(line, "--no-messages")
			return []byte(out), mergeErr
		default:
			return nil, nil
		}
	}
	behind, files, err := env.mergeTree(context.Background(), headed(1, "abc"))
	if !saw {
		t.Fatal("merge-tree ran without --no-messages")
	}
	return behind, files, err
}

func exitOne(t *testing.T) error {
	t.Helper()
	err := exec.CommandContext(t.Context(), "/bin/sh", "-c", "exit 1").Run()
	if err == nil {
		t.Fatal("exit 1")
	}
	return err
}

func exitTwo(t *testing.T) error {
	t.Helper()
	err := exec.CommandContext(t.Context(), "/bin/sh", "-c", "exit 2").Run()
	if err == nil {
		t.Fatal("exit 2")
	}
	return err
}
