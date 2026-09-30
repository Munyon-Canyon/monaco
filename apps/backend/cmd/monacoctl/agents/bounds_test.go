package agents

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
	"testing"
)

func TestBounds_killsDispatchSurvivors(t *testing.T) {
	t.Parallel()
	env := newFixture(t).Env(t)
	for _, ancErr := range []bool{true, false} {
		env.Run = func(_ context.Context, _, _, name string, args ...string) ([]byte, error) {
			line := strings.Join(args, " ")
			switch {
			case name == "git" && strings.Contains(line, "--verify"):
				return []byte("ok"), nil
			case name == "git" && strings.Contains(line, "--is-ancestor") && ancErr:
				return nil, errors.New("behind")
			case name == "git" && strings.Contains(line, "merge-tree"):
				return nil, errors.New("tree")
			default:
				return nil, nil
			}
		}
		behind, files, err := env.mergeTree(context.Background(), headed(1, "abc"))
		if err == nil || files != nil || behind != ancErr {
			t.Fatalf("%v %v %v %v", ancErr, behind, files, err)
		}
	}
	var buf strings.Builder
	err := writeRebase(&buf, pr(1, "h", "fb", ""), Record{}, false, make([]string, maxLines))
	if err != nil || !strings.Contains(buf.String(), "and 5 more files") {
		t.Fatal(buf.String(), err)
	}
	f := prepBranch(t)
	env = f.Env(t)
	f.hub.on(get("/issues/4"), Issue{Body: "**Milestone:** M7 · **Blocked by:** #8 #9 · **Touches:** `a`"})
	f.hub.on(get("/issues/8"), Issue{State: "closed", StateReason: "completed"})
	f.hub.on(get("/issues/9"), Issue{State: "open"})
	f.hub.on(list("/pulls?state=closed"), []PR{})
	if err = env.blockersClear(context.Background(), 4); err == nil || !strings.Contains(cliText(err), "#9") {
		t.Fatal(err)
	}
	if !closes("Closes #1 and closes #8", 8) || closes("Closes #1", 8) {
		t.Fatal("closes")
	}
	side := commitFile(t, f.dir, "side.go", "x\n")
	git(t, f.dir, "update-ref", "refs/remotes/origin/fb", "HEAD~1")
	when := f.now
	for _, sha := range []string{side, "not-a-sha"} {
		f.hub.on(list("/pulls?state=closed"), []PR{{
			MergedAt: &when, Base: Ref{Ref: "fb"}, Body: "Closes #8", MergeCommitSHA: sha,
		}})
		err = env.issueBlocker(context.Background(), 8)
		gotMiss := err != nil && strings.Contains(cliText(err), "not merged into")
		if gotMiss != (sha == side) {
			t.Fatal(sha, err)
		}
	}
	self := strconv.Itoa(os.Getpid())
	for _, line := range []string{strings.Repeat("9", 20) + " other", "1 other", "2 other"} {
		env.Run = func(_ context.Context, _, _, name string, args ...string) ([]byte, error) {
			if name == "ps" && len(args) > 0 && args[len(args)-1] == self {
				return []byte(line + "\n"), nil
			}
			if name == "ps" {
				return []byte("0 claude\n"), nil
			}
			return nil, errors.New("no")
		}
		got, pidErr := env.claudePID(context.Background())
		wantPID := strings.HasPrefix(line, "2")
		if (pidErr == nil) != wantPID || (pidErr == nil && got != 2) ||
			(pidErr != nil && !strings.Contains(cliText(pidErr), "no claude process above pid")) {
			t.Fatalf("%q %d %v", line, got, pidErr)
		}
	}
}
