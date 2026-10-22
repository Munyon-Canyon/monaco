package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const stagedUDID = "7B30D45E-62FD-42E2-871A-787B19D38CCF"

func stageAndCheck(t *testing.T, files map[string]string, paths ...string) (string, error) {
	t.Helper()
	dir := t.TempDir()
	run := func(name string, args ...string) string {
		cmd := exec.Command(name, args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil && name == "git" {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	run("git", "init", "-q")
	for rel, body := range files {
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run("git", "add", "-A")
	cmd := exec.Command("bash", append([]string{filepath.Join(repoRoot(t), "scripts", "githooks", "check-staged-udid.sh")}, paths...)...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestCheckStagedUDID_refusesAUDIDInScope(t *testing.T) {
	t.Parallel()
	for _, rel := range []string{".cursor/rules/a.mdc", "docs/how-to/sim.md", "AGENTS.md", "apps/mobile/AGENTS.md"} {
		out, err := stageAndCheck(t, map[string]string{rel: "- Gold sim (UDID `" + stagedUDID + "`)\n"}, rel)
		if err == nil || !strings.Contains(out, rel) || !strings.Contains(out, stagedUDID) {
			t.Errorf("%s: err=%v out=%q", rel, err, out)
		}
	}
}

func TestCheckStagedUDID_allowsOutOfScopeFilesAndPlainText(t *testing.T) {
	t.Parallel()
	files := map[string]string{
		"docs/legacy/old.md":      stagedUDID + "\n",
		"apps/mobile/notes.md":    stagedUDID + "\n",
		"docs/how-to/clean.md":    "Run scripts/gold-sim-udid.sh.\nlower 7b30d45e-62fd-42e2-871a-787b19d38ccf\n",
		".cursor/rules/clean.mdc": "no id here\n",
	}
	out, err := stageAndCheck(t, files, "docs/legacy/old.md", "apps/mobile/notes.md", "docs/how-to/clean.md", ".cursor/rules/clean.mdc")
	if err != nil {
		t.Fatalf("err=%v out=%q", err, out)
	}
}

func TestCheckStagedUDID_readsPathsFromStdinAndIgnoresRemovedLines(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	dir := t.TempDir()
	git := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	path := filepath.Join(dir, "docs.md")
	git("init", "-q")
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	doc := filepath.Join(dir, "docs", "sim.md")
	if err := os.WriteFile(doc, []byte(stagedUDID+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", "-A")
	git("-c", "user.email=t@t", "-c", "user.name=t", "commit", "-q", "-m", "seed")
	if err := os.WriteFile(doc, []byte("resolved at run time\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", "-A")
	_ = path
	cmd := exec.Command("bash", filepath.Join(root, "scripts", "githooks", "check-staged-udid.sh"))
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader("docs/sim.md\n\n")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("removing a UDID must pass: err=%v out=%q", err, out)
	}
	if err := os.WriteFile(doc, []byte("again "+stagedUDID+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", "-A")
	cmd = exec.Command("bash", filepath.Join(root, "scripts", "githooks", "check-staged-udid.sh"))
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader("docs/sim.md\n")
	if out, err := cmd.CombinedOutput(); err == nil {
		t.Fatalf("adding a UDID must fail: out=%q", out)
	}
}
