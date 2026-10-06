package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const (
	apiBundle = "apps/backend/api/openapi.yaml"
	apiBase   = "apps/backend/internal/platform/httpx/api/api.gen.go"
	apiModule = "apps/backend/internal/platform/httpx/api/rankingapi/api.gen.go"
)

func TestGeneratedAPIFiles_stayOnGitsTextMerge(t *testing.T) {
	root := repoRoot(t)
	for _, path := range []string{apiBundle, apiBase, apiModule} {
		cmd := exec.Command("git", "check-attr", "merge", "diff", "binary", "--", path)
		cmd.Dir = root
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("git check-attr %s: %v", path, err)
		}
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if !strings.HasSuffix(line, ": unspecified") {
				t.Errorf("%s: %q, want the default text merge", path, line)
			}
		}
	}
}

func TestRestackRegen_settlesASameModuleConflictInTheAPIFiles(t *testing.T) {
	dir := t.TempDir()
	attrs, err := os.ReadFile(filepath.Join(repoRoot(t), ".gitattributes"))
	if err != nil {
		t.Fatal(err)
	}
	gen := "mkdir -p apps/backend/internal/platform/httpx/api/rankingapi apps/backend/api src && " +
		"sort src/*.txt > " + apiBundle + " && sort -r src/*.txt > " + apiModule
	commit := func(msg, src, body string) {
		t.Helper()
		writeTestFile(t, filepath.Join(dir, "src", src), body)
		regenRun(t, dir, "sh", "-c", gen)
		regenRun(t, dir, "git", "add", "-A")
		regenRun(t, dir, "git", "commit", "-q", "-m", msg)
	}
	regenRun(t, dir, "git", "init", "-q", "-b", "main")
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(dir, ".gitattributes"), string(attrs))
	commit("base", "base.txt", "m\n")
	regenRun(t, dir, "git", "checkout", "-q", "-b", "feature")
	commit("feature route", "feature.txt", "a\n")
	regenRun(t, dir, "git", "checkout", "-q", "main")
	commit("main route", "main.txt", "b\n")
	regenRun(t, dir, "git", "checkout", "-q", "feature")

	gt := filepath.Join(t.TempDir(), "gt")
	writeExecutable(t, gt, "#!/bin/sh\ncase \"$1\" in restack) git rebase main ;; continue) git rebase --continue ;; esac\n")
	cmd := exec.Command(filepath.Join(repoRoot(t), "scripts", "restack-regen.sh"))
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), regenGitEnv...)
	cmd.Env = append(cmd.Env, "GT="+gt, "GENERATE="+gen)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("restack-regen on the default globs: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "regenerating after a stop on") {
		t.Fatalf("the restack never stopped on a generated file, so the test proves nothing:\n%s", out)
	}
	for path, want := range map[string]string{apiBundle: "a\nb\nm\n", apiModule: "m\nb\na\n"} {
		got, err := os.ReadFile(filepath.Join(dir, path))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != want {
			t.Errorf("%s = %q, want %q", path, got, want)
		}
	}
}
