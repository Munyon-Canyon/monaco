package agents

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResume_allowsATranscriptAtTheCap(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	commitFixture(t, f)
	path := filepath.Join(t.TempDir(), "owner.jsonl")
	writeFile(t, path, strings.Repeat("a", resumeTokenCap*4))
	f.owner(t, Record{Ticket: 40, Model: opus, Worktree: f.dir, Base: "abc", State: Running})
	code, stdout, stderr := f.agents(t, "resume", "40", "--transcript", path)
	if code != 0 || stderr != "" || stdout != "resume allowed: #40 250000 tokens\n" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if _, err := os.Stat(filepath.Join(f.dir, ".git", "pstack")); !os.IsNotExist(err) {
		t.Fatalf("success wrote a log: %v", err)
	}
}

func TestResume_refusesPastTheCapAndPrintsAFreshOwner(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	sha := commitFixture(t, f)
	writeFile(t, filepath.Join(f.dir, "dirty.txt"), "x")
	path := filepath.Join(t.TempDir(), "owner.jsonl")
	writeFile(t, path, strings.Repeat("a", resumeTokenCap*4+1))
	f.owner(t, Record{Ticket: 40, Model: sonnet, Worktree: f.dir, Base: "parent", State: Running})
	env := f.Env(t)
	if err := env.saveVerdict(Verdict{PR: 1, State: "failure", Description: "one"}); err != nil {
		t.Fatal(err)
	}
	if err := env.saveVerdict(Verdict{PR: 2, State: "failure", Description: "two"}); err != nil {
		t.Fatal(err)
	}
	if err := env.saveVerdict(Verdict{PR: 3, State: "failure", Description: "three"}); err != nil {
		t.Fatal(err)
	}
	if err := env.saveVerdict(Verdict{PR: 4, State: "failure", Description: "four"}); err != nil {
		t.Fatal(err)
	}
	if err := env.saveVerdict(Verdict{PR: 5, State: "success", Description: "passed"}); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(env.Common, recordsDir, "verdicts", "notes.txt"), "skip")
	writeFile(t, filepath.Join(env.Common, recordsDir, "verdicts", "x.json"), "{")
	code, stdout, stderr := f.agents(t, "resume", "--transcript", path, "#40")
	if code != 1 || !strings.Contains(stderr, "transcript is over 250k tokens") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	for _, want := range []string{
		"fresh owner\n",
		"ticket: 40\n",
		"worktree: " + f.dir + "\n",
		"parent: parent\n",
		"brief: docs/agents/owner.md\n",
		"tokens: 250001\n",
		"trail:\n",
		"cfg\n",
		"branch:\n" + sha + " main\n",
		"dirty.txt",
		"findings:\n#1 one\n#2 two\n#3 three\nand 1 more\n",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout missing %q\n%s", want, stdout)
		}
	}
	if strings.Contains(stdout, "passed") || strings.Contains(stdout, "four") {
		t.Fatalf("stdout leaked a closed or capped finding:\n%s", stdout)
	}
	logPath := filepath.Join(f.dir, ".git", "pstack", "ms", "logs", "resume.log")
	body, err := os.ReadFile(logPath)
	if err != nil || !strings.Contains(string(body), "fresh owner") || !strings.Contains(string(body), "250k") {
		t.Fatalf("log %q: %v", body, err)
	}
	if _, err := os.Stat(filepath.Join(f.dir, "pstack")); !os.IsNotExist(err) {
		t.Fatalf("log landed in the worktree: %v", err)
	}
}

func TestResume_usesTheDefaultTranscriptAndReportsUsage(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	r := Record{Ticket: 40, Model: opus, Worktree: f.dir, Base: "abc", AgentID: "sess", State: Running}
	f.owner(t, r)
	path, err := f.Env(t).defaultTranscript(r)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, path, "ab")
	code, stdout, stderr := f.agents(t, "resume", "40")
	if code != 0 || stderr != "" || stdout != "resume allowed: #40 1 tokens\n" {
		t.Fatalf("default: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if code, _, stderr := f.agents(t, "resume"); code != 2 || !strings.Contains(stderr, "resume <ticket>") {
		t.Fatalf("usage: code=%d stderr=%q", code, stderr)
	}
	if code, _, stderr := f.agents(t, "resume", "40", "--transcript"); code != 2 {
		t.Fatalf("flag: code=%d stderr=%q", code, stderr)
	}
	if code, _, stderr := f.agents(t, "resume", "40", "--transcript", path, "--transcript", path); code != 2 {
		t.Fatalf("twice: code=%d stderr=%q", code, stderr)
	}
	if code, _, stderr := f.agents(t, "resume", "0"); code != 2 {
		t.Fatalf("zero: code=%d stderr=%q", code, stderr)
	}
	code, _, stderr = f.agents(t, "resume", "41", "--transcript", path)
	if code != 1 || !strings.Contains(stderr, "no owner record") {
		t.Fatalf("missing: code=%d stderr=%q", code, stderr)
	}
}

func TestResume_reportsAMissingTranscriptAndAnEmptyOwner(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.owner(t, Record{Ticket: 40, Worktree: f.dir, State: Running})
	missing := filepath.Join(t.TempDir(), "gone.jsonl")
	code, _, stderr := f.agents(t, "resume", "40", "--transcript", missing)
	if code != 1 || !strings.Contains(stderr, "read transcript:") {
		t.Fatalf("missing file: code=%d stderr=%q", code, stderr)
	}
	if code, _, stderr := f.agents(t, "resume", "40"); code != 1 || !strings.Contains(stderr, "pass --transcript") {
		t.Fatalf("no agent: code=%d stderr=%q", code, stderr)
	}
	f.owner(t, Record{Ticket: 40, Worktree: f.dir, AgentID: "sess", State: Running})
	f.env = []string{f.env[0], "GH_TOKEN=tok", "HOME="}
	if code, _, stderr := f.agents(t, "resume", "40"); code != 1 || !strings.Contains(stderr, "HOME is unset") {
		t.Fatalf("home: code=%d stderr=%q", code, stderr)
	}
	dir := t.TempDir()
	code, _, stderr = f.agents(t, "resume", "40", "--transcript", dir)
	if code != 1 || !strings.Contains(stderr, "read transcript:") {
		t.Fatalf("directory: code=%d stderr=%q", code, stderr)
	}
}

func TestResume_unavailableGitAndFindingErrorsStayInThePromptOrTheError(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	path := filepath.Join(t.TempDir(), "big.jsonl")
	writeFile(t, path, strings.Repeat("b", resumeTokenCap*4+1))
	f.owner(t, Record{Ticket: 7, Worktree: t.TempDir(), Base: "base", State: Running})
	code, stdout, stderr := f.agents(t, "resume", "7", "--transcript", path)
	if code != 1 || !strings.Contains(stdout, "trail:\nunavailable\n") ||
		!strings.Contains(stdout, "branch:\nunavailable\n") ||
		!strings.Contains(stdout, "findings:\nnone\n") || !strings.Contains(stderr, "250k") {
		t.Fatalf("unavailable: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	env := f.Env(t)
	writeFile(t, filepath.Join(env.Common, recordsDir, "verdicts"), "not a dir")
	code, _, stderr = f.agents(t, "resume", "7", "--transcript", path)
	if code != 1 || !strings.Contains(stderr, "list findings") {
		t.Fatalf("list: code=%d stderr=%q", code, stderr)
	}
}

func TestResume_gitDetailsAndAnEmptyTrail(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	commitFixture(t, f)
	env := f.Env(t)
	ctx := t.Context()
	if env.trail(ctx, f.dir) == "unavailable" || !strings.Contains(env.branchState(ctx, f.dir), "clean") {
		t.Fatalf("trail=%q branch=%q", env.trail(ctx, f.dir), env.branchState(ctx, f.dir))
	}
	env.Run = func(ctx context.Context, dir, stdin, name string, args ...string) ([]byte, error) {
		if name == "git" && len(args) > 0 && args[0] == "log" {
			return []byte("\n"), nil
		}
		if name == "git" && len(args) > 1 && args[1] == "--abbrev-ref" {
			return nil, errors.New("no branch")
		}
		return Exec(ctx, dir, stdin, name, args...)
	}
	if env.trail(ctx, f.dir) != "none" {
		t.Fatalf("empty trail: %q", env.trail(ctx, f.dir))
	}
	if env.branchState(ctx, f.dir) != "unavailable" {
		t.Fatalf("branch name: %q", env.branchState(ctx, f.dir))
	}
	env.Run = func(ctx context.Context, dir, stdin, name string, args ...string) ([]byte, error) {
		if name == "git" && len(args) > 0 && args[0] == "status" {
			return nil, errors.New("no status")
		}
		return Exec(ctx, dir, stdin, name, args...)
	}
	if env.branchState(ctx, f.dir) != "unavailable" {
		t.Fatalf("status: %q", env.branchState(ctx, f.dir))
	}
	if capBlock("a\nb\nc\nd", 3, "none") != "a\nb\nc\nand 1 more" {
		t.Fatal(capBlock("a\nb\nc\nd", 3, "none"))
	}
	if capBlock("a\nb\nc", 3, "none") != "a\nb\nc" {
		t.Fatal(capBlock("a\nb\nc", 3, "none"))
	}
	if capBlock("", 3, "none") != "none" {
		t.Fatal("empty")
	}
}

func TestResume_reportsACorruptFinding(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	path := filepath.Join(t.TempDir(), "big.jsonl")
	writeFile(t, path, strings.Repeat("c", resumeTokenCap*4+1))
	f.owner(t, Record{Ticket: 7, Worktree: f.dir, Base: "base", State: Running})
	writeFile(t, filepath.Join(f.Env(t).Common, recordsDir, "verdicts", "8.json"), "{")
	code, _, stderr := f.agents(t, "resume", "7", "--transcript", path)
	if code != 1 || !strings.Contains(stderr, "decode ") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
}

func TestFailLog_writesTheFullOutputUnderGitAndReportsABlockedPath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	env := &Env{Common: dir, Config: Config{Milestone: "m7-rest"}}
	body := strings.Repeat("line\n", 30)
	if err := logged(env, "forecast", body, errors.New("boom")); err == nil || err.Error() != "boom" {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "pstack", "m7-rest", "logs", "forecast.log"))
	if err != nil || string(got) != body+"boom\n" {
		t.Fatalf("log=%q err=%v", got, err)
	}
	if logged(nil, "forecast", body, nil) != nil {
		t.Fatal("success")
	}
	if logged(nil, "forecast", "", errors.New("x")).Error() != "x" {
		t.Fatal("no env")
	}
	blocked := filepath.Join(t.TempDir(), "notdir")
	writeFile(t, blocked, "x")
	err = logged(&Env{Common: blocked, Config: Config{Milestone: "m7-rest"}}, "forecast", body, errors.New("boom"))
	if err == nil || !strings.Contains(err.Error(), "boom") || !strings.Contains(err.Error(), "write log") {
		t.Fatal(err)
	}
	env = &Env{Common: t.TempDir(), Config: Config{Milestone: "m7-rest"}}
	if err := os.MkdirAll(filepath.Join(env.Common, "pstack", "m7-rest", "logs", "forecast.log"), 0o750); err != nil {
		t.Fatal(err)
	}
	err = env.writeFailLog("forecast", "full\n", errors.New("boom"))
	if err == nil || !strings.Contains(err.Error(), "write log") {
		t.Fatal(err)
	}
}

func TestMain_writesAFailureLogBesideGitAndNotInTheWorktree(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	code, stdout, stderr := f.agents(t, "forecast")
	if code != 1 || stdout != "" || !strings.Contains(stderr, "pulls?state=open") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	got, err := os.ReadFile(filepath.Join(f.dir, ".git", "pstack", "ms", "logs", "forecast.log"))
	if err != nil || !strings.Contains(string(got), "pulls?state=open") {
		t.Fatalf("log=%q err=%v", got, err)
	}
	if _, err := os.Stat(filepath.Join(f.dir, "pstack")); !os.IsNotExist(err) {
		t.Fatalf("product tree log: %v", err)
	}
}

func commitFixture(t *testing.T, f *fixture) string {
	t.Helper()
	git(t, f.dir, "add", configPath)
	git(t, f.dir, "commit", "-q", "-m", "cfg")
	cmd := strings.TrimSpace(gitOut(t, f.dir, "rev-parse", "HEAD"))
	return cmd
}

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := Exec(t.Context(), dir, "", "git", args...)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}
