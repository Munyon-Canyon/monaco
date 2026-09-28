package agents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyPlan_classifiesTheDiff(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		files []File
		want  string
	}{
		{
			"test-only",
			[]File{
				{Filename: "a_test.go", Additions: 900},
				{Filename: "x/testdata/y.json", Additions: 5},
				{Filename: "testdata/z.json", Additions: 1},
			},
			"#5: root-check, verifier sonnet\nreason: test-only\nnon-test lines: 0 in 0 files\n",
		},
		{
			"small",
			[]File{{Filename: "a.go", Additions: 30, Deletions: 19}, {Filename: "a_test.go", Additions: 400}},
			"#5: root-check, verifier sonnet\nreason: under 50 non-test lines\nnon-test lines: 49 in 1 files\n",
		},
		{
			"large",
			[]File{{Filename: "a.go", Additions: 30, Deletions: 10}, {Filename: "b.md", Additions: 10}},
			"#5: full, verifier sonnet\nreason: 50 non-test lines\nnon-test lines: 50 in 2 files\n",
		},
		{
			"money",
			[]File{
				{Filename: "apps/backend/internal/modules/treasury/fund.go", Additions: 1},
				{Filename: "apps/backend/internal/platform/db/uow.go", Additions: 1},
			},
			"#5: full, verifier opus\nreason: apps/backend/internal/modules/treasury/fund.go is in modules/treasury/\nnon-test lines: 2 in 2 files\n",
		},
		{
			"concurrency",
			[]File{
				{
					Filename:  "apps/backend/internal/x/w.go",
					Additions: 2,
					Patch:     "@@ -1 +1,2 @@\n-a\n+\tgo func() { done <- true }()\n",
				},
			},
			"#5: full, verifier opus\nreason: apps/backend/internal/x/w.go adds concurrency code (go func)\nnon-test lines: 2 in 1 files\n",
		},
		{
			"sensitive test",
			[]File{
				{Filename: "apps/backend/internal/platform/bus/relay_test.go", Additions: 3, Patch: "+ sync.WaitGroup"},
			},
			"#5: root-check, verifier sonnet\nreason: test-only\nnon-test lines: 0 in 0 files\n",
		},
		{
			"removed concurrency",
			[]File{{Filename: "w.go", Deletions: 1, Patch: "-\tvar mu sync.Mutex\n"}},
			"#5: root-check, verifier sonnet\nreason: under 50 non-test lines\nnon-test lines: 1 in 1 files\n",
		},
	}
	for _, c := range cases {
		f := newFixture(t)
		f.hub.on(get("/pulls/5"), pr(5, "h", "fb", "Part of #40"))
		f.hub.on(list("/pulls/5/files?"), c.files)
		code, stdout, stderr := f.agents(t, "verify-plan", "#5")
		want := c.want + "owner: unknown (no owner record for #40 in .monaco/agents)\n"
		if code != 0 || stdout != want || stderr != "" {
			t.Errorf("%s: code=%d stderr=%q\n got %q\nwant %q", c.name, code, stderr, stdout, want)
		}
	}
}

func TestVerifyPlan_neverPicksTheOwnersModel(t *testing.T) {
	t.Parallel()
	cases := map[string]struct{ owner, file, want string }{
		"opus owner, plain diff":      {opus, "a.go", "verifier sonnet"},
		"sonnet owner, plain diff":    {sonnet, "a.go", "verifier opus"},
		"opus owner, sensitive diff":  {opus, "apps/backend/internal/platform/bus/relay.go", "verifier sonnet"},
		"haiku owner, sensitive diff": {"haiku", "apps/backend/internal/platform/money/u.go", "verifier opus"},
	}
	for name, c := range cases {
		f := newFixture(t)
		f.owner(t, Record{Ticket: 40, Model: c.owner, State: Running})
		f.hub.on(get("/pulls/5"), pr(5, "h", "fb", "Closes #40"))
		f.hub.on(list("/pulls/5/files?"), []File{{Filename: c.file, Additions: 60}})
		code, stdout, _ := f.agents(t, "verify-plan", "5")
		if code != 0 || !strings.Contains(stdout, c.want) || !strings.HasSuffix(stdout, "owner: #40 "+c.owner+"\n") {
			t.Errorf("%s: code=%d stdout=%q", name, code, stdout)
		}
	}
}

func TestVerifyPlan_reportsAMissingTicketAndBadRecords(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.hub.on(get("/pulls/5"), pr(5, "h", "fb", "no ticket"))
	f.hub.on(list("/pulls/5/files?"), []File{})
	code, stdout, _ := f.agents(t, "verify-plan", "5")
	if code != 0 || !strings.HasSuffix(stdout, "owner: unknown (the PR body names no Part of or Closes ticket)\n") {
		t.Fatalf("code=%d stdout=%q", code, stdout)
	}
	if code, _, stderr := f.agents(t, "verify-plan"); code != 2 || !strings.Contains(stderr, "verify-plan <pr>") {
		t.Fatalf("usage: code=%d stderr=%q", code, stderr)
	}
	if code, _, stderr := f.agents(t, "verify-plan", "0"); code != 2 {
		t.Fatalf("zero: code=%d stderr=%q", code, stderr)
	}
	f.hub.on(get("/pulls/6"), pr(6, "h", "fb", "Part of #41"))
	f.hub.on(list("/pulls/6/files?"), []File{})
	writeFile(t, f.Env(t).recordPath(41), "{")
	if code, _, stderr := f.agents(t, "verify-plan", "6"); code != 1 || !strings.Contains(stderr, "decode ") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
}

func TestVerifyPlan_failsWhenGitHubFails(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	if code, _, stderr := f.agents(t, "verify-plan", "5"); code != 1 || !strings.Contains(stderr, "pulls/5: 404") {
		t.Fatalf("pr: code=%d stderr=%q", code, stderr)
	}
	f.hub.on(get("/pulls/5"), pr(5, "h", "fb", ""))
	if code, _, stderr := f.agents(t, "verify-plan", "5"); code != 1 || !strings.Contains(stderr, "pulls/5/files") {
		t.Fatalf("files: code=%d stderr=%q", code, stderr)
	}
}

func TestRecords_skipsOtherFilesAndReportsUnreadableOnes(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	env := f.Env(t)
	if rs, err := env.records(); err != nil || len(rs) != 0 {
		t.Fatalf("empty: %v %v", rs, err)
	}
	f.owner(t, Record{Ticket: 3, Model: opus})
	writeFile(t, env.recordPath(3)+".bak", "")
	writeFile(t, filepath.Join(env.Common, recordsDir, "12"), "")
	if err := os.Mkdir(filepath.Join(env.Common, recordsDir, "verdicts"), 0o750); err != nil {
		t.Fatal(err)
	}
	rs, err := env.records()
	if err != nil || len(rs) != 1 || rs[0].Ticket != 3 {
		t.Fatalf("records = %v, %v", rs, err)
	}
	if err := os.Mkdir(env.recordPath(9), 0o750); err != nil {
		t.Fatal(err)
	}
	if _, err := env.records(); err == nil || !strings.Contains(err.Error(), "read owner record") {
		t.Fatalf("unreadable: %v", err)
	}
}

func TestRecords_writeAndListFailuresAreReported(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	env := f.Env(t)
	writeFile(t, filepath.Join(env.Common, ".monaco", "agents"), "not a dir")
	if err := env.saveRecord(Record{Ticket: 1}); err == nil || !strings.Contains(err.Error(), "write owner record") {
		t.Fatalf("mkdir: %v", err)
	}
	if _, err := env.records(); err == nil || !strings.Contains(err.Error(), "list owner records") {
		t.Fatalf("list: %v", err)
	}
	g := newFixture(t).Env(t)
	writeFile(t, g.recordPath(2)+"/x", "")
	if err := g.saveRecord(Record{Ticket: 2}); err == nil || !strings.Contains(err.Error(), "write owner record") {
		t.Fatalf("write: %v", err)
	}
}

func TestMain_aWorktreeReadsItsOwnConfigAndTheSharedRecords(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	git(t, f.dir, "add", configPath)
	git(t, f.dir, "commit", "-q", "-m", "c")
	f.owner(t, Record{Ticket: 40, Model: opus})
	wt := filepath.Join(f.dir, ".worktrees", "9")
	git(t, f.dir, "worktree", "add", "-q", "--detach", wt)
	writeFile(t, filepath.Join(f.dir, configPath), "broken")
	f.hub.on(get("/pulls/5"), pr(5, "h", "fb", "Part of #40"))
	f.hub.on(list("/pulls/5/files?"), []File{})
	f.dir = wt
	code, stdout, stderr := f.agents(t, "verify-plan", "5")
	if code != 0 || !strings.HasSuffix(stdout, "owner: #40 opus\n") || stderr != "" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func (f *fixture) owner(t *testing.T, r Record) {
	t.Helper()
	if err := f.Env(t).saveRecord(r); err != nil {
		t.Fatal(err)
	}
}
