package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const anchor = "**Milestone:** M7 · **Blocked by:** none · **Touches:** `a/**`"

func TestBatch_defersEachOffenderByName(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		bodies []string
		want   string
	}{
		{
			"no touches line",
			[]string{anchor, "**Milestone:** M7 · **Blocked by:** none"},
			"deferred #2: no Touches line\n",
		},
		{
			"blocker in the same batch",
			[]string{anchor, "**Blocked by:** #1 · **Touches:** `b/**`"},
			"deferred #2: blocked by #1 in the same batch\n",
		},
		{
			"blocker not merged",
			[]string{anchor, "**Blocked by:** #8 (open) · **Touches:** `b/**`"},
			"deferred #2: blocker #8 is not merged into fb\n",
		},
		{
			"second blocker not merged",
			[]string{anchor, "**Blocked by:** #9, #8 · **Touches:** `b/**`"},
			"deferred #2: blocker #8 is not merged into fb\n",
		},
		{
			"second blocker in the same batch",
			[]string{anchor, "**Blocked by:** #9 and #1 · **Touches:** `b/**`"},
			"deferred #2: blocked by #1 in the same batch\n",
		},
		{
			"blocker without a number",
			[]string{anchor, "**Blocked by:** the retro · **Touches:** `b/**`"},
			"deferred #2: Blocked by line has no issue numbers\n",
		},
		{
			"touches overlap",
			[]string{anchor, "**Blocked by:** nothing, start here · **Touches:** `c.go`, `a/b/*.go`"},
			"deferred #2: Touches a/b/*.go overlaps #1 a/**\n",
		},
		{
			"over the batch size",
			[]string{anchor, "**Touches:** `b/**`", "**Touches:** `c/**`"},
			"deferred #3: batch is full at 2\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := prepBranch(t)
			f.hub.on(get("/issues/8"), Issue{State: "open"})
			f.hub.on(get("/issues/9"), Issue{PullRequest: &struct{}{}})
			merged := f.now
			f.hub.on(get("/pulls/9"), PR{MergedAt: &merged, MergeCommitSHA: f.head(t)})
			f.hub.on(list("/pulls?state=closed"), []PR{})
			args := []string{"batch"}
			for i, body := range tc.bodies {
				n := i + 1
				f.hub.on(get("/issues/"+strconv.Itoa(n)), Issue{Number: n, Body: body})
				args = append(args, strconv.Itoa(n))
			}
			code, stdout, stderr := f.agents(t, args...)
			if code != 0 || stderr != "" || !strings.HasPrefix(stdout, "batch: #1") ||
				!strings.HasSuffix(stdout, tc.want) {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
			}
		})
	}
}

func TestBatch_writesTheAcceptedTicketsWithTheirGlobs(t *testing.T) {
	t.Parallel()
	f := prepBranch(t)
	f.hub.on(get("/issues/5"), Issue{Body: anchor})
	f.hub.on(get("/issues/6"), Issue{Body: "**Blocked by:** #9 · **Touches:** `b/x.go`, `cmd/ci*.go`"})
	f.hub.on(get("/issues/9"), Issue{PullRequest: &struct{}{}})
	merged := f.now
	f.hub.on(get("/pulls/9"), PR{MergedAt: &merged, MergeCommitSHA: f.head(t)})
	code, stdout, stderr := f.agents(t, "batch", "5", "6")
	path := f.Env(t).batchPath()
	if code != 0 || stderr != "" || stdout != "batch: #5 #6 (2 of 2) in "+path+"\n" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got Batch
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	seen := make([]any, 0, 2*len(got.Tickets))
	for _, tk := range got.Tickets {
		seen = append(seen, tk.Ticket, tk.Touches)
	}
	want := `[5 [a/**] 6 [b/x.go cmd/ci*.go]]`
	if !got.Created.Equal(f.now) || fmt.Sprint(seen) != want || strings.Contains(string(data), "dispatched") {
		t.Fatalf("batch = %+v", got)
	}
}

func TestBatch_refusesBadInputAndFailures(t *testing.T) {
	t.Parallel()
	f := prepBranch(t)
	if code, _, stderr := f.agents(t, "batch"); code != 2 || !strings.Contains(stderr, "batch <issue>") {
		t.Fatalf("usage: %d %q", code, stderr)
	}
	if code, _, stderr := f.agents(t, "batch", "x"); code != 2 {
		t.Fatalf("int: %d %q", code, stderr)
	}
	if code, _, stderr := f.agents(t, "batch", "3", "3"); code != 1 || !strings.Contains(stderr, "#3 is listed twice") {
		t.Fatalf("twice: %d %q", code, stderr)
	}
	if code, _, stderr := f.agents(t, "batch", "3"); code != 1 || !strings.Contains(stderr, "issues/3") {
		t.Fatalf("issue: %d %q", code, stderr)
	}
	f.hub.on(get("/issues/3"), Issue{Body: "no header"})
	code, stdout, stderr := f.agents(t, "batch", "3")
	if code != 1 || stdout != "deferred #3: no Touches line\n" || !strings.Contains(stderr, "no ticket is ready") {
		t.Fatalf("none: %d %q %q", code, stdout, stderr)
	}
	if _, err := os.Stat(f.Env(t).batchPath()); !os.IsNotExist(err) {
		t.Fatalf("batch written: %v", err)
	}
	f.hub.on(get("/issues/3"), Issue{Body: "**Blocked by:** #8 · **Touches:** `a/**`"})
	if code, _, stderr := f.agents(t, "batch", "3"); code != 1 || !strings.Contains(stderr, "issues/8") {
		t.Fatalf("blocker: %d %q", code, stderr)
	}
	f.hub.on(get("/issues/3"), Issue{Body: anchor})
	writeFile(t, filepath.Join(f.dir, ".git", "pstack", "ms", "batch.json", "x"), "")
	if code, _, stderr := f.agents(t, "batch", "3"); code != 1 || !strings.Contains(stderr, "write batch") {
		t.Fatalf("write: %d %q", code, stderr)
	}
	writeFile(t, filepath.Join(f.dir, ".git", "pstack", "ms2"), "")
	env := f.Env(t)
	env.Config.Milestone = "ms2"
	if err := env.saveBatch(Batch{}); err == nil || !strings.Contains(err.Error(), "write batch") {
		t.Fatal(err)
	}
}

func TestGlobsOverlap(t *testing.T) {
	t.Parallel()
	cases := []struct {
		a, b string
		want bool
	}{
		{"a/**", "a/b/c.go", true},
		{"a/*.go", "a/b.go", true},
		{"a/b.go", "a/*.go", true},
		{"a/*.go", "a/b.py", false},
		{"**/x.go", "a/b/x.go", true},
		{"**/b", "a", false},
		{"a/**", "b/**", false},
		{"a/**", "**", true},
		{"cmd/ci*.go", "cmd/c*.go", true},
		{"cmd/ci*.go", "cmd/x*.go", false},
		{"cmd/*_test.go", "cmd/*.py", false},
		{"a", "a/b", false},
		{"a/[", "a/b", false},
	}
	for _, tc := range cases {
		if got := globsOverlap(strings.Split(tc.a, "/"), strings.Split(tc.b, "/")); got != tc.want {
			t.Errorf("globsOverlap(%q, %q) = %v", tc.a, tc.b, got)
		}
	}
}

func TestDispatch_refusesATicketOutsideTheBatch(t *testing.T) {
	t.Parallel()
	f := prepBranch(t)
	path := f.Env(t).batchPath()
	if code, _, stderr := f.agents(t, "dispatch", "12", "--model", "opus"); code != 1 ||
		!strings.Contains(stderr, "no batch at "+path) {
		t.Fatalf("missing: %d %q", code, stderr)
	}
	f.batch(t, 4)
	if code, _, stderr := f.agents(t, "dispatch", "12", "--model", "opus"); code != 1 ||
		!strings.Contains(stderr, "#12 is not in "+path+"; pass --urgent") {
		t.Fatalf("outside: %d %q", code, stderr)
	}
	writeFile(t, path, "{")
	if code, _, stderr := f.agents(t, "dispatch", "4", "--model", "opus"); code != 1 ||
		!strings.Contains(stderr, "decode") {
		t.Fatalf("decode: %d %q", code, stderr)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(path, "x"), "")
	if code, _, stderr := f.agents(t, "dispatch", "4", "--model", "opus"); code != 1 ||
		!strings.Contains(stderr, "read batch") {
		t.Fatalf("read: %d %q", code, stderr)
	}
}

func TestDispatch_urgentSkipsTheBatchAndLogsToTracking(t *testing.T) {
	t.Parallel()
	f := prepBranch(t)
	f.hub.on(get("/issues/12"), Issue{Body: "no blockers"})
	f.hub.on(list("/pulls?state=open"), []PR{})
	f.ps()
	code, stdout, stderr := f.agents(t, "dispatch", "12", "--model", "opus", "--urgent", "--dry-run")
	if code != 0 || stderr != "" || !strings.Contains(stdout, "dry-run: would log the urgent dispatch to #7") {
		t.Fatalf("dry: %d %q %q", code, stdout, stderr)
	}
	env := f.Env(t)
	env.Run = f.run
	args := []string{"12", "--model", "opus", "--urgent"}
	if err := dispatchCmd(context.Background(), env, args, ioDiscard()); err == nil ||
		!strings.Contains(err.Error(), "issues/7/comments") {
		t.Fatal(err)
	}
	f.hub.on("POST /repos/o/r/issues/7/comments", "ok")
	if err := dispatchCmd(context.Background(), env, args, ioDiscard()); err != nil {
		t.Fatal(err)
	}
	logged := f.hub.body("POST /repos/o/r/issues/7/comments")
	if !strings.Contains(logged, "#12 dispatched outside batch.json at 2026-09-27T12:00:00Z") {
		t.Fatal(logged)
	}
	if rec, err := env.record(12); err != nil || rec.State != Running {
		t.Fatalf("rec=%+v err=%v", rec, err)
	}
}
