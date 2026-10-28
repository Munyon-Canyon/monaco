package agents

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func attestFixture(t *testing.T, on bool) (*fixture, string, string) {
	t.Helper()
	f := newFixture(t)
	if on {
		writeFile(t, filepath.Join(f.dir, configPath), testConfig+"\n[land]\non_stage0 = true\n")
	}
	sha, tree := strings.Repeat("c", 40), strings.Repeat("d", 40)
	head := pr(5, "h", "fb", "Part of #40")
	head.Head.SHA = sha
	f.hub.on(get("/pulls/5"), head)
	f.hub.on("POST /repos/o/r/statuses/"+sha, "ok")
	f.scriptGit(map[string]string{"rev-parse": tree + "\n"}, nil)
	return f, sha, tree
}

func TestAttest_offPrintsAndPostsNothing(t *testing.T) {
	t.Parallel()
	f, _, tree := attestFixture(t, false)
	writeFile(t, f.Env(t).statePath("checks", tree), "")
	code, stdout, stderr := f.agents(t, "attest", "5")
	if code != 0 || stderr != "" || stdout != "land on stage 0 is off; nothing attested\n" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if posts := f.hub.callsContaining("POST"); len(posts) != 0 {
		t.Fatalf("posted %v", posts)
	}
}

func TestAttest_noStage0RecordFailsAndPostsNothing(t *testing.T) {
	t.Parallel()
	f, sha, tree := attestFixture(t, true)
	code, _, stderr := f.agents(t, "attest", "5")
	want := "no stage 0 record for #5 head " + sha[:12] + " (tree " + tree[:12] + "); run monacoctl agents check first"
	if code == 0 || !strings.Contains(stderr, want) {
		t.Fatalf("code=%d stderr=%q, want %q", code, stderr, want)
	}
	if posts := f.hub.callsContaining("POST"); len(posts) != 0 {
		t.Fatalf("posted %v", posts)
	}
}

func TestAttest_aRecordPostsTheTreeOnTheHead(t *testing.T) {
	t.Parallel()
	f, sha, tree := attestFixture(t, true)
	writeFile(t, f.Env(t).statePath("checks", tree), "")
	code, stdout, stderr := f.agents(t, "attest", "5")
	want := "#5 " + sha[:12] + " attested: stage 0 passed on tree " + tree[:12] + "\n"
	if code != 0 || stderr != "" || stdout != want {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	route := "POST /repos/o/r/statuses/" + sha
	wantBody := `{"context":"stage0/ok","description":"tree ` + tree + `","state":"success"}`
	if got := strings.TrimSpace(f.hub.body(route)); got != wantBody || f.hub.authOf(route) != "token tok" {
		t.Fatalf("body=%q auth=%q", got, f.hub.authOf(route))
	}
}

func TestAttest_theDescriptionCarriesTheRowsOfTheRecord(t *testing.T) {
	t.Parallel()
	f, sha, tree := attestFixture(t, true)
	record := checkRecord{Rows: []string{"go build", "go vet", "go lint", "go test -short", "swift test", "xcode"}}
	if _, err := f.Env(t).writeCheckRecord(tree, record); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := f.agents(t, "attest", "5"); code != 0 {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	wantBody := `{"context":"stage0/ok","description":"tree ` + tree + ` rows swift test,xcode","state":"success"}`
	if got := strings.TrimSpace(f.hub.body("POST /repos/o/r/statuses/" + sha)); got != wantBody {
		t.Fatalf("body=%q", got)
	}
	if n := len(checkRecord{Rows: attestedRows()}.description(tree)); n > 140 {
		t.Fatalf("description is %d bytes; GitHub accepts 140", n)
	}
}

func TestAttest_anUnreadableRecordFailsAndPostsNothing(t *testing.T) {
	t.Parallel()
	f, _, tree := attestFixture(t, true)
	if err := os.MkdirAll(f.Env(t).statePath("checks", tree), 0o750); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := f.agents(t, "attest", "5"); code == 0 || stderr == "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if posts := f.hub.callsContaining("POST"); len(posts) != 0 {
		t.Fatalf("posted %v", posts)
	}
}

func TestAttest_fetchesTheHeadBranchWhenTheCommitIsMissing(t *testing.T) {
	t.Parallel()
	f, sha, tree := attestFixture(t, true)
	writeFile(t, f.Env(t).statePath("checks", tree), "")
	var seen []string
	f.run = func(ctx context.Context, dir, stdin, name string, args ...string) ([]byte, error) {
		if name != "git" {
			return Exec(ctx, dir, stdin, name, args...)
		}
		seen = append(seen, strings.Join(args, " "))
		switch args[0] {
		case "rev-parse":
			if !slices.Contains(seen, "fetch --quiet origin h") {
				return nil, errors.New("bad object")
			}
			return []byte(tree + "\n"), nil
		case "fetch":
			return nil, nil
		}
		return Exec(ctx, dir, stdin, name, args...)
	}
	code, stdout, stderr := f.agents(t, "attest", "5")
	if code != 0 || stderr != "" || !strings.Contains(stdout, "attested") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if len(f.hub.callsContaining("POST /repos/o/r/statuses/"+sha)) != 1 {
		t.Fatalf("calls %v", f.hub.calls)
	}
}

func TestAttest_namesBothFailuresWhenTheTreeStaysMissing(t *testing.T) {
	t.Parallel()
	f, _, _ := attestFixture(t, true)
	f.scriptGit(nil, map[string]string{"rev-parse": "bad object", "fetch": "no remote"})
	code, _, stderr := f.agents(t, "attest", "5")
	if code == 0 || !strings.Contains(stderr, "bad object") || !strings.Contains(stderr, "no remote") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
}

func TestAttest_reportsAnUnreadableVerifierKeyAndARefusedPost(t *testing.T) {
	t.Parallel()
	f, sha, tree := attestFixture(t, true)
	writeFile(t, f.Env(t).statePath("checks", tree), "")
	key := filepath.Join(f.home, ".config", "monaco", "verifier.pem")
	writeFile(t, key, "not a key")
	if code, _, stderr := f.agents(t, "attest", "5"); code == 0 || stderr == "" {
		t.Fatalf("unreadable key: code=%d stderr=%q", code, stderr)
	}
	if err := os.Remove(key); err != nil {
		t.Fatal(err)
	}
	f.hub.status["POST /repos/o/r/statuses/"+sha] = 422
	if code, _, stderr := f.agents(t, "attest", "5"); code == 0 || stderr == "" {
		t.Fatalf("refused post: code=%d stderr=%q", code, stderr)
	}
}

func TestAttest_refusesBadArgumentsAndAnUnknownPR(t *testing.T) {
	t.Parallel()
	f, _, _ := attestFixture(t, true)
	for _, args := range [][]string{{"attest"}, {"attest", "x"}, {"attest", "1", "2"}} {
		if code, _, stderr := f.agents(t, args...); code == 0 || !strings.Contains(stderr, "attest <pr>") {
			t.Fatalf("%v: code=%d stderr=%q", args, code, stderr)
		}
	}
	if code, _, stderr := f.agents(t, "attest", "9"); code == 0 || stderr == "" {
		t.Fatalf("unknown PR: code=%d stderr=%q", code, stderr)
	}
}
