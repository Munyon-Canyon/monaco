package agents

import (
	"path/filepath"
	"strings"
	"testing"
)

func trackedFixture(t *testing.T, extra string, branches ...string) *fixture {
	t.Helper()
	f := liveFixture(t, branches...)
	writeFile(t, filepath.Join(f.dir, configPath),
		strings.Replace(testConfig, "feature_branch = \"fb-checkpoint-1\"\n", "", 1)+extra)
	return f
}

func TestStatus_publishesEachFeatureToItsOwnTrackingIssue(t *testing.T) {
	t.Parallel()
	f := trackedFixture(t, "[features.leaderboards]\ntracking = 31\n",
		"leaderboards-checkpoint-2", "following-checkpoint-5", "orphan-checkpoint-1")
	f.openStack(strings.Repeat("e", 40),
		pr(1, "l1", "leaderboards-checkpoint-2", "Part of #100"),
		pr(2, "f1", "following-checkpoint-5", "Closes #200"),
		pr(3, "o1", "orphan-checkpoint-1", "Closes #300"))
	f.hub.on(get("/issues/200"), Issue{Number: 200, Body: "**Milestone:** M9 · **Tracking:** #41"})
	f.hub.on(get("/issues/300"), Issue{Number: 300, Body: "**Milestone:** none"})
	for _, n := range []string{"7", "31", "41"} {
		f.hub.on(list("/issues/"+n+"/comments?"), []Comment{})
		f.hub.on("POST /repos/o/r/issues/"+n+"/comments", "ok")
	}
	code, stdout, stderr := f.agents(t, "status", "--publish")
	want := "status comment updated on #7\n" +
		"status comment updated on #41\n" +
		"status comment updated on #31\n"
	if code != 0 || !strings.HasSuffix(stdout, want) ||
		!strings.Contains(stdout, "skipped orphan-checkpoint-1: orphan-checkpoint-1 has no tracking issue; "+
			"add [features.orphan] with tracking = <issue> to .monaco/agents.toml\n") {
		t.Fatalf("code=%d stderr=%q stdout:\n%s", code, stderr, stdout)
	}
	leaderboards, following := posted(t, f, "POST /repos/o/r/issues/31/comments"),
		posted(t, f, "POST /repos/o/r/issues/41/comments")
	if !strings.Contains(leaderboards, "| #1 | leaderboards-checkpoint-2 |") ||
		strings.Contains(leaderboards, "| #2 |") ||
		!strings.Contains(following, "| #2 | following-checkpoint-5 |") ||
		strings.Contains(following, "| #1 |") {
		t.Fatalf("leaderboards:\n%s\nfollowing:\n%s", leaderboards, following)
	}
	if fb := posted(t, f, "POST /repos/o/r/issues/7/comments"); strings.Contains(fb, "| #") {
		t.Fatalf("fb has no open stack:\n%s", fb)
	}
}

func TestBatch_defersATicketOnAnotherFeatureBranchAndShowsOnlyOnItsBoard(t *testing.T) {
	t.Parallel()
	f := trackedFixture(t, "[features.leaderboards]\ntracking = 31\n", "leaderboards-checkpoint-2")
	f.hub.on(get("/issues/51"), Issue{Number: 51, Body: "**Base branch:** `leaderboards-checkpoint-2` · " +
		"**Blocked by:** none · **Touches:** `a/**`"})
	f.hub.on(get("/issues/52"), Issue{Number: 52, Body: "**Base branch:** `fb-checkpoint-1` · " +
		"**Blocked by:** none · **Touches:** `b/**`"})
	code, stdout, stderr := f.agents(t, "batch", "51", "52")
	if code != 0 ||
		!strings.Contains(
			stdout,
			"deferred #52: lands on fb-checkpoint-1, not the batch's leaderboards-checkpoint-2\n",
		) {
		t.Fatalf("code=%d stderr=%q stdout:\n%s", code, stderr, stdout)
	}
	b, _, err := loadBatch(f.Env(t).batchPath())
	if err != nil || b.Branch != "leaderboards-checkpoint-2" || len(b.Tickets) != 1 {
		t.Fatalf("batch %+v %v", b, err)
	}
	f.hub.onQuery("t51: issue", `{"data":{"repository":{"t51":{"timelineItems":{"nodes":[]}}}}}`)
	f.openStack(strings.Repeat("f", 40))
	env := f.Env(t)
	for trunks, want := range map[string]bool{"leaderboards-checkpoint-2": true, "fb-checkpoint-1": false} {
		body, err := env.statusBody(t.Context(), nil, []string{trunks}, "")
		if err != nil || strings.Contains(body, "| #51 |") != want {
			t.Errorf("%s: board shown %v, want %v:\n%s %v", trunks, !want, want, body, err)
		}
	}
}

func TestHandoff_postsToTheBatchFeaturesTrackingIssueAndNeedsABranchWhenAmbiguous(t *testing.T) {
	t.Parallel()
	f := trackedFixture(t, "[features.leaderboards]\ntracking = 31\n", "leaderboards-checkpoint-2")
	if code, _, stderr := f.agents(t, "handoff"); code != 1 ||
		!strings.Contains(
			stderr,
			"the batch names no feature branch and fb-checkpoint-1, leaderboards-checkpoint-2 are live",
		) {
		t.Fatalf("ambiguous: %d %q", code, stderr)
	}
	f.hub.on(list("/issues/31/comments?"), []Comment{})
	f.hub.on("POST /repos/o/r/issues/31/comments", "ok")
	if code, stdout, stderr := f.agents(t, "handoff", "--branch", "leaderboards-checkpoint-2"); code != 0 ||
		stdout != "handoff posted to #31\n" ||
		!strings.Contains(
			posted(t, f, "POST /repos/o/r/issues/31/comments"),
			"Feature branch `leaderboards-checkpoint-2`.",
		) {
		t.Fatalf("--branch: %d %q %q", code, stdout, stderr)
	}
	if err := f.Env(t).saveBatch(Batch{Created: f.now, Branch: "leaderboards-checkpoint-2"}); err != nil {
		t.Fatal(err)
	}
	if code, stdout, stderr := f.agents(t, "handoff"); code != 0 || stdout != "handoff posted to #31\n" {
		t.Fatalf("batch branch: %d %q %q", code, stdout, stderr)
	}
}

func TestDispatch_logsAnUrgentTicketToItsTrackingHeaderWhenTheFeatureHasNoEntry(t *testing.T) {
	t.Parallel()
	f := trackedFixture(t, "", "leaderboards-checkpoint-2")
	f.hub.on(get("/issues/71"), Issue{Number: 71, Body: "**Base branch:** `leaderboards-checkpoint-2` · " +
		"**Tracking:** #45 · **Touches:** `a`"})
	f.hub.on(list("/pulls?state=open"), []PR{})
	f.ps()
	code, stdout, stderr := f.agents(t, "dispatch", "71", "--model", "opus", "--dry-run", "--urgent")
	if code != 0 || !strings.Contains(stdout, "dry-run: would log the urgent dispatch to #45\n") {
		t.Fatalf("code=%d stderr=%q stdout:\n%s", code, stderr, stdout)
	}
}
