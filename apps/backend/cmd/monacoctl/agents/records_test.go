package agents

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

func (f *fixture) ownerComments(ticket int, cs ...Comment) {
	f.hub.on(list(fmt.Sprintf("/issues/%d/comments?", ticket)), append([]Comment{}, cs...))
	f.hub.on(fmt.Sprintf("POST /repos/o/r/issues/%d/comments", ticket), "{}")
	for _, c := range cs {
		f.hub.on(fmt.Sprintf("PATCH /repos/o/r/issues/comments/%d", c.ID), "{}")
	}
}

func ownerComment(id int64, r Record) Comment {
	return authored(id, recordBody(r), ghUser, "MEMBER")
}

func authored(id int64, body, login, association string) Comment {
	return Comment{ID: id, Body: body, User: Author{Login: login}, AuthorAssociation: association}
}

func TestRecords_aFreshCloneRebuildsTheOwnerFromTheTicket(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	if _, err := os.Stat(filepath.Join(f.dir, ".git", recordsDir)); !os.IsNotExist(err) {
		t.Fatalf("the clone already has records: %v", err)
	}
	started := f.now.Add(-time.Hour)
	older := Record{Ticket: 40, Model: sonnet, Base: "b0", State: Running, Started: started, Changed: started}
	newer := Record{Ticket: 40, Model: opus, Base: "b1", State: Running, Started: started, Changed: f.now}
	f.ownerComments(40, ownerComment(11, older), Comment{ID: 12, Body: "lgtm"}, ownerComment(13, newer))

	transcript := filepath.Join(t.TempDir(), "t.jsonl")
	writeFile(t, transcript, "ab")
	if code, stdout, stderr := f.agents(t, "resume", "40", "--transcript", transcript); code != 0 ||
		stdout != "resume allowed: #40 1 tokens\n" {
		t.Fatalf("resume: %d %q %q", code, stdout, stderr)
	}

	sha := strings.Repeat("c", 40)
	f.hub.on(get("/pulls/5"), headed(5, sha))
	f.hub.on(list("/pulls/5/files?"), []File{{Filename: "a.go", Additions: 1}})
	f.hub.on("POST /repos/o/r/statuses/"+sha, "ok")
	f.scriptGit(map[string]string{"fetch": "", "merge-base": "abc\n", "diff": "d\n", "patch-id": "id x\n"}, nil)
	verdict := func(model string) (int, string) {
		code, _, stderr := f.agents(t, "verdict", "pass", "5", sha, "--kind", "full", "--model", model,
			"--report", f.report(t, "ok"))
		return code, stderr
	}
	if code, stderr := verdict(opus); code != 1 ||
		!strings.Contains(stderr, "verifier model opus equals the owner model opus on #40") {
		t.Fatalf("same model: %d %q", code, stderr)
	}
	if code, stderr := verdict(sonnet); code != 0 {
		t.Fatalf("sonnet: %d %q", code, stderr)
	}
	patched := posted(t, f, "PATCH /repos/o/r/issues/comments/13")
	if !strings.Contains(patched, "branch h, parent b1,") || !strings.Contains(patched, `"branch":"h"`) {
		t.Fatalf("patched %q", patched)
	}

	newStackGH(t, f, green(t, 6, "b6", "fb"))
	if code, stdout, stderr := f.agents(
		t,
		"land-stack",
		"6",
	); code != 0 ||
		stdout != "queued #6\nfollow it: monacoctl agents watch (under Claude Code's Monitor tool)\nqueued together: #6\n" {
		t.Fatalf("land: %d %q %q", code, stdout, stderr)
	}
	env := f.Env(t)
	rec, err := env.localRecord(40)
	if err != nil || rec.Worktree != env.worktreePath(40) || rec.Branch != "h" || rec.Queued == nil {
		t.Fatalf("rec=%+v err=%v", rec, err)
	}
}

func TestRecords_rebuildReadsOnlyTheNewestMarkedComment(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newFixture(t)
	env := f.Env(t)
	if _, err := env.record(ctx, 40); err == nil || !strings.Contains(err.Error(), "issues/40/comments") {
		t.Fatalf("list: %v", err)
	}
	f.ownerComments(40, Comment{ID: 1, Body: "no marker"})
	if _, err := env.record(ctx, 40); errs.CodeOf(err) != errs.CodeNotFound ||
		cliText(err) != "no owner record for #40 in .monaco/agents" {
		t.Fatalf("unmarked: %v", err)
	}
	good := Record{Ticket: 40, Model: sonnet, Branch: "b", Base: "x", State: Done}
	broken := authored(3, recordMarker+"\n```json\n{\"ticket\":\n```\n", ghUser, "OWNER")
	f.ownerComments(40, ownerComment(2, good), broken)
	if _, err := env.record(ctx, 40); errs.CodeOf(err) != errs.CodeDecodeFailed ||
		!strings.Contains(cliText(err), "#40") {
		t.Fatalf("undecodable: %v", err)
	}
	f.ownerComments(40, broken, ownerComment(2, good))
	rec, err := env.record(ctx, 40)
	if err != nil || rec.Model != sonnet || rec.Branch != "b" || rec.State != Done ||
		rec.Worktree != env.worktreePath(40) {
		t.Fatalf("rec=%+v err=%v", rec, err)
	}
}

func TestRecords_writesFailLoudly(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.record(t, Record{Ticket: 4, State: Running})
	if code, _, stderr := f.agents(t, "done", "4"); code != 1 || !strings.Contains(stderr, "issues/4/comments") {
		t.Fatalf("publish: %d %q", code, stderr)
	}
	freeze(t, f.Env(t).recordPath(4))
	if code, _, stderr := f.agents(t, "done", "4"); code != 1 || !strings.Contains(stderr, "write owner record") {
		t.Fatalf("save: %d %q", code, stderr)
	}
}

func TestRecords_dispatchFailsWhenTheRecordCannotBePosted(t *testing.T) {
	t.Parallel()
	f := prepBranch(t)
	f.batch(t, 12)
	f.hub.on(get("/issues/12"), Issue{Body: "no blockers"})
	f.hub.on(list("/pulls?state=open"), []PR{})
	f.ps()
	env := f.Env(t)
	env.Run = f.run
	err := dispatchCmd(context.Background(), env, []string{"12", "--model", "opus"}, ioDiscard())
	if err == nil || !strings.Contains(err.Error(), "issues/12/comments") {
		t.Fatal(err)
	}
}

func TestRecords_verdictFailsWhenTheBranchCannotBeRecorded(t *testing.T) {
	t.Parallel()
	readOnly := func(t *testing.T, _ *fixture, path string) {
		t.Helper()
		freeze(t, path)
	}
	rewritten := func(t *testing.T, f *fixture, path string) {
		t.Helper()
		prev := f.run
		f.run = func(ctx context.Context, dir, stdin, name string, args ...string) ([]byte, error) {
			if len(args) > 0 && args[0] == "patch-id" {
				writeFile(t, path, "{")
			}
			return prev(ctx, dir, stdin, name, args...)
		}
	}
	for _, tt := range []struct {
		name, want string
		tamper     func(t *testing.T, f *fixture, path string)
	}{
		{"read only", "write owner record", readOnly},
		{"rewritten mid-verdict", "decode", rewritten},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			f.owner(t, Record{Ticket: 40, Model: opus, State: Running})
			path := f.Env(t).recordPath(40)
			sha := strings.Repeat("c", 40)
			f.hub.on(get("/pulls/5"), headed(5, sha))
			f.hub.on(list("/pulls/5/files?"), []File{{Filename: "a.go", Additions: 1}})
			f.hub.on("POST /repos/o/r/statuses/"+sha, "ok")
			f.scriptGit(map[string]string{"fetch": "", "merge-base": "abc\n", "diff": "d\n", "patch-id": "id x\n"}, nil)
			tt.tamper(t, f, path)
			code, _, stderr := f.agents(t, "verdict", "pass", "5", sha, "--kind", "full", "--model", sonnet,
				"--report", f.report(t, "ok"))
			if code != 1 || !strings.Contains(stderr, tt.want) {
				t.Fatalf("%d %q", code, stderr)
			}
		})
	}
}

func TestRecords_rebuildTrustsOnlyOwnersMembersAndCollaborators(t *testing.T) {
	t.Parallel()
	genuine := Record{Ticket: 40, Model: opus, Branch: "b", Base: "x", State: Running}
	forged := recordBody(Record{Ticket: 40, Model: sonnet, Branch: "evil", Base: "y", State: Running})
	for _, association := range []string{"NONE", "CONTRIBUTOR", "FIRST_TIME_CONTRIBUTOR", ""} {
		t.Run("ignores "+association, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			f.ownerComments(40, ownerComment(2, genuine), authored(3, forged, "stranger", association))
			env := f.Env(t)
			rec, err := env.record(context.Background(), 40)
			if err != nil || rec.Model != opus || rec.Branch != "b" {
				t.Fatalf("rec=%+v err=%v", rec, err)
			}
		})
	}
	t.Run("a lone foreign record is no record", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		f.ownerComments(40, authored(3, forged, "stranger", "NONE"))
		env := f.Env(t)
		if _, err := env.record(context.Background(), 40); errs.CodeOf(err) != errs.CodeNotFound {
			t.Fatalf("err=%v", err)
		}
	})
	for _, association := range []string{"OWNER", "MEMBER", "COLLABORATOR"} {
		t.Run("rebuilds a teammate's "+association, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			f.ownerComments(40, ownerComment(2, genuine), authored(3, forged, "teammate", association))
			env := f.Env(t)
			rec, err := env.record(context.Background(), 40)
			if err != nil || rec.Model != sonnet || rec.Branch != "evil" {
				t.Fatalf("rec=%+v err=%v", rec, err)
			}
		})
	}
}

func TestRecords_aForgedRecordCannotDefeatTheSameModelRefusal(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	genuine := Record{Ticket: 40, Model: opus, Base: "b1", State: Running}
	forged := recordBody(Record{Ticket: 40, Model: sonnet, Base: "b1", State: Running})
	f.ownerComments(40, ownerComment(2, genuine), authored(3, forged, "stranger", "NONE"))
	sha := strings.Repeat("c", 40)
	f.hub.on(get("/pulls/5"), headed(5, sha))
	f.hub.on(list("/pulls/5/files?"), []File{{Filename: "a.go", Additions: 1}})
	f.hub.on("POST /repos/o/r/statuses/"+sha, "ok")
	f.scriptGit(map[string]string{"fetch": "", "merge-base": "abc\n", "diff": "d\n", "patch-id": "id x\n"}, nil)
	code, _, stderr := f.agents(t, "verdict", "pass", "5", sha, "--kind", "full", "--model", opus,
		"--report", f.report(t, "ok"))
	if code != 1 || !strings.Contains(stderr, "verifier model opus equals the owner model opus on #40") {
		t.Fatalf("same model: %d %q", code, stderr)
	}
	if f.hub.body("POST /repos/o/r/statuses/"+sha) != "" {
		t.Fatal("the refused verdict posted a status")
	}
}

func TestRecords_publishEditsOnlyTheNewestTrustedRecordItsUserWrote(t *testing.T) {
	t.Parallel()
	r := Record{Ticket: 4, Model: opus, State: Running}
	forged := recordBody(Record{Ticket: 4, Model: sonnet, State: Running})
	for _, tt := range []struct {
		name  string
		cs    []Comment
		route string
	}{
		{"mine is newest", []Comment{ownerComment(2, r)}, "PATCH /repos/o/r/issues/comments/2"},
		{"a foreign marker is newer", []Comment{
			ownerComment(2, r), authored(3, forged, "stranger", "NONE"),
		}, "PATCH /repos/o/r/issues/comments/2"},
		{"only a foreign marker", []Comment{authored(3, forged, "stranger", "CONTRIBUTOR")}, "POST /repos/o/r/issues/4/comments"},
		{"a teammate's is newest", []Comment{
			ownerComment(2, r), authored(3, forged, "teammate", "MEMBER"),
		}, "POST /repos/o/r/issues/4/comments"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			f.record(t, r)
			f.ownerComments(4, tt.cs...)
			if code, _, stderr := f.agents(t, "done", "4"); code != 0 {
				t.Fatalf("done: %d %q", code, stderr)
			}
			if got := posted(t, f, tt.route); !strings.Contains(got, `"state":"done"`) {
				t.Fatalf("%s %q", tt.route, got)
			}
			for _, c := range tt.cs {
				route := fmt.Sprintf("PATCH /repos/o/r/issues/comments/%d", c.ID)
				if route != tt.route && f.hub.body(route) != "" {
					t.Fatalf("edited #%d", c.ID)
				}
			}
		})
	}
	t.Run("the gh user lookup fails", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		f.record(t, r)
		f.ownerComments(4, ownerComment(2, r))
		f.whoami = errors.New("gh: not logged in")
		if code, _, stderr := f.agents(t, "done", "4"); code != 1 || !strings.Contains(stderr, "not logged in") {
			t.Fatalf("done: %d %q", code, stderr)
		}
	})
}
