package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func runSetupAgentEnv(t *testing.T, home string, args ...string) (string, error) {
	t.Helper()
	script := filepath.Join(repoRoot(t), "scripts", "setup-agent-env.sh")
	cmd := exec.Command("bash", append([]string{script}, args...)...)
	cmd.Env = append(os.Environ(), "HOME="+home)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func readHome(t *testing.T, home, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(home, ".claude", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestSetupAgentEnv_writesTheSheetOnceAndASecondRunChangesNothing(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	want := readRepo(t, repoRoot(t), "docs/agents/pstack-models.md")
	if out, err := runSetupAgentEnv(t, home); err != nil {
		t.Fatalf("first run: %v\n%s", err, out)
	}
	sheet := readHome(t, home, "pstack-models.md")
	claudeMD := readHome(t, home, "CLAUDE.md")
	if sheet != want {
		t.Fatalf("sheet differs from docs/agents/pstack-models.md:\n%s", sheet)
	}
	if strings.Count(claudeMD, "@~/.claude/pstack-models.md\n") != 1 {
		t.Fatalf("CLAUDE.md include: %q", claudeMD)
	}
	out, err := runSetupAgentEnv(t, home)
	if err != nil {
		t.Fatalf("second run: %v\n%s", err, out)
	}
	if strings.Contains(out, "wrote") || strings.Contains(out, "added") {
		t.Fatalf("second run changed something:\n%s", out)
	}
	if readHome(t, home, "pstack-models.md") != sheet || readHome(t, home, "CLAUDE.md") != claudeMD {
		t.Fatal("second run changed a file")
	}
}

func TestSetupAgentEnv_refusesADifferingSheetUnlessForced(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o750); err != nil {
		t.Fatal(err)
	}
	mine := "feature, refactoring: sonnet\n"
	if err := os.WriteFile(filepath.Join(home, ".claude", "pstack-models.md"), []byte(mine), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := runSetupAgentEnv(t, home)
	if err == nil {
		t.Fatalf("differing sheet without --force exited 0:\n%s", out)
	}
	if !strings.Contains(out, "-feature, refactoring: sonnet") || !strings.Contains(out, "--force") {
		t.Fatalf("refusal does not print the diff and the fix:\n%s", out)
	}
	if got := readHome(t, home, "pstack-models.md"); got != mine {
		t.Fatalf("refusal overwrote the sheet: %q", got)
	}
	if out, err := runSetupAgentEnv(t, home, "--force"); err != nil {
		t.Fatalf("--force: %v\n%s", err, out)
	}
	if got := readHome(t, home, "pstack-models.md"); got != readRepo(t, repoRoot(t), "docs/agents/pstack-models.md") {
		t.Fatalf("--force left %q", got)
	}
}

func TestSetupAgentEnv_dryRunWritesNothing(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	out, err := runSetupAgentEnv(t, home, "--dry-run")
	if err != nil {
		t.Fatalf("dry run: %v\n%s", err, out)
	}
	if !strings.Contains(out, "would write") || !strings.Contains(out, "would add") {
		t.Fatalf("dry run output:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude")); !os.IsNotExist(err) {
		t.Fatalf("dry run created ~/.claude: %v", err)
	}
}

func TestSetupAgentEnv_keepsAnExistingClaudeMD(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o750); err != nil {
		t.Fatal(err)
	}
	mine := "# my rules\n"
	if err := os.WriteFile(filepath.Join(home, ".claude", "CLAUDE.md"), []byte(mine), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := runSetupAgentEnv(t, home); err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}
	if got := readHome(t, home, "CLAUDE.md"); got != mine+"\n@~/.claude/pstack-models.md\n" {
		t.Fatalf("CLAUDE.md = %q", got)
	}
}
