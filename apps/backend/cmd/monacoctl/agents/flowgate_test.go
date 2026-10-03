package agents

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/tools/flows"
)

func gateGit(heads ...string) map[string]string {
	out := map[string]string{}
	for _, head := range heads {
		out["show "+head+":"+flowsFile] = flows.Header + "\n" +
			"00\tPing\tsystem\tGET /p\tRecordPing\t\t\tok\tbuilt\tdocs/f.md\n" +
			"01\tSign in\tidentity\tGET /s\tSignIn\t\t\tok\tbuilt\tdocs/f.md\n"
		out["ls-tree --name-only "+head+" "+flows.AppDir+"/"] = flows.AppDir + "/00.tsv\n"
		out["show "+head+":"+flows.AppDir+"/00.tsv"] = flows.AppHeader + "\n00\tSystemPing\tbuilt\tdocs/f.md\n"
	}
	out["merge-base origin/fb b2-oid"] = "base\n"
	out["diff --name-only base..origin/fb"] = ""
	return out
}

func gateStack(t *testing.T, f *fixture, files map[int][]File, extra ...*stackPR) *stackGH {
	t.Helper()
	s := newStackGH(t, f, append([]*stackPR{green(t, 1, "b1", "fb"), green(t, 2, "b2", "b1")}, extra...)...)
	s.gitOut = gateGit("b2-oid", "q2-oid")
	for n, fs := range files {
		f.hub.on(list("/pulls/"+strconv.Itoa(n)+"/files?"), fs)
	}
	f.owner(t, Record{Ticket: 40, Worktree: "/w/40", State: Done})
	return s
}

func (s *stackGH) gitRan(prefix string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.ContainsFunc(s.gitCalls, func(c string) bool { return strings.HasPrefix(c, prefix) })
}

func TestFlowGate_aStackThatTouchesNoFlowNeverDiffsAgainstStaging(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	s := gateStack(t, f, map[int][]File{1: {{Filename: "README.md"}}, 2: {{Filename: "docs/index.md"}}})
	if code, stdout, stderr := f.agents(
		t,
		"land-stack",
		"2",
	); code != 0 ||
		!strings.HasPrefix(stdout, "queued #1 #2\n") {
		t.Fatalf("%d %q %q", code, stdout, stderr)
	}
	if s.gitRan("diff") || s.gitRan("merge-base") || s.gitRan("log") {
		t.Fatalf("a stack with no flow ran git %q", s.gitCalls)
	}
}

func TestFlowGate_refusesAFlowThatChangedOnStagingUntilTheStackIsRestacked(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	s := gateStack(t, f, map[int][]File{2: {{Filename: "packages/flows/app/00.tsv"}}})
	s.gitOut["diff --name-only base..origin/fb"] = "apps/backend/internal/modules/system/http.go\nREADME.md\n"
	s.gitOut["log -1 --format=%s base..origin/fb -- apps/backend/internal/modules/system/http.go"] = "Record a ping note (#77)\n"
	code, stdout, stderr := f.agents(t, "land-stack", "2")
	want := "not landing #2: flow 00 changed on staging since this stack's base (#77). " +
		"Restack with gt and rerun stage 1, then run land-stack again"
	if code == 0 || !strings.Contains(stderr, want) || s.prs[1].labeled("merge-queue") || f.owned(t).Queued != nil {
		t.Fatalf("%d %q %q", code, stdout, stderr)
	}

	s.gitOut["diff --name-only base..origin/fb"] = "apps/backend/internal/modules/identity/http.go\n"
	if code, stdout, stderr := f.agents(t, "land-stack", "2"); code != 0 || !s.prs[2].labeled("merge-queue") {
		t.Fatalf("a change to another flow on staging: %d %q %q", code, stdout, stderr)
	}
}

func TestFlowGate_namesTheChangedRowOfFlowsTSV(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	s := gateStack(t, f, map[int][]File{
		1: {{Filename: flowsFile, Patch: "@@ -2 +2 @@\n-00\tPing\n+00\tPing!\n"}},
	})
	s.gitOut["diff --name-only base..origin/fb"] = flowsFile + "\n"
	s.gitOut["diff -U0 base..origin/fb -- "+flowsFile] = "@@ -2 +2 @@\n-00\tPing\n+00\tPong\n"
	s.gitOut["log -1 --format=%s base..origin/fb -- "+flowsFile] = "Rename the ping (#78)\n"
	if code, _, stderr := f.agents(t, "land-stack", "2"); code == 0 ||
		!strings.Contains(stderr, "flow 00 changed on staging since this stack's base (#78)") {
		t.Fatalf("%d %q", code, stderr)
	}
	s.gitOut["diff -U0 base..origin/fb -- "+flowsFile] = "@@ -3 +3 @@\n-01\tSign in\n+01\tLog in\n"
	if code, stdout, stderr := f.agents(t, "land-stack", "2"); code != 0 {
		t.Fatalf("another row changed: %d %q %q", code, stdout, stderr)
	}
}

func TestFlowGate_refusesAFlowInAnotherQueuedStack(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	q1, q2 := green(t, 6, "q1", "fb"), green(t, 7, "q2", "q1")
	labeled(q1, "merge-queue")
	labeled(q2, "merge-queue")
	s := gateStack(t, f, map[int][]File{
		2: {{Filename: "packages/flows/app/00.tsv"}},
		7: {{Filename: "packages/mobile-core/Sources/MonacoSystem/Flow00SystemPingModel.swift"}},
	}, q1, q2, green(t, 9, "unqueued", "fb"))
	f.hub.on(list("/pulls/9/files?"), []File{{Filename: "packages/flows/app/00.tsv"}})
	code, stdout, stderr := f.agents(t, "land-stack", "2")
	want := "not landing #2: flow 00 is in queued stack #7. Wait for #7 to land, then run land-stack again"
	if code == 0 || !strings.Contains(stderr, want) || s.prs[2].labeled("merge-queue") {
		t.Fatalf("%d %q %q", code, stdout, stderr)
	}

	f.hub.on(list("/pulls/7/files?"), []File{{Filename: "apps/backend/internal/modules/identity/app.go"}})
	if code, stdout, stderr := f.agents(t, "land-stack", "2"); code != 0 || !s.prs[2].labeled("merge-queue") {
		t.Fatalf("a queued stack on flow 01 only: %d %q %q", code, stdout, stderr)
	}
}

func TestFlowGate_failuresLabelNothing(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, gitFail, files string
		openFail             int
	}{
		{"fetch the stack head", "fetch --no-tags origin b2", "", 0},
		{"read flows.tsv", "show b2-oid:" + flowsFile, "", 0},
		{"list the app files", "ls-tree", "", 0},
		{"read an app file", "show b2-oid:packages/flows/app/00.tsv", "", 0},
		{"fetch staging", "fetch --no-tags origin fb", "", 0},
		{"merge-base", "merge-base", "", 0},
		{"diff staging", "diff --name-only", "", 0},
		{"diff flows.tsv rows", "diff -U0", "", 0},
		{"log the change", "log -1", "", 0},
		{"fetch a queued stack head", "fetch --no-tags origin q1", "", 0},
		{"list the open PRs a second time", "", "", 2},
		{"list a PR's files", "", "/pulls/2/files?", 0},
		{"list a queued PR's files", "", "/pulls/6/files?", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			q1 := labeled(green(t, 6, "q1", "fb"), "merge-queue")
			s := gateStack(
				t,
				f,
				map[int][]File{2: {{Filename: "packages/flows/app/00.tsv"}}, 6: {{Filename: "README.md"}}},
				q1,
			)
			s.gitOut = gateGit("b2-oid", "q1-oid")
			if tc.gitFail == "" || strings.HasSuffix(tc.gitFail, "q1") {
				s.gitOut["diff --name-only base..origin/fb"] = ""
			} else {
				s.gitOut["diff --name-only base..origin/fb"] = flowsFile + "\n"
			}
			s.gitOut["diff -U0 base..origin/fb -- "+flowsFile] = "+00\tPing\n"
			s.gitOut["log -1 --format=%s base..origin/fb -- "+flowsFile] = "a hand-pushed commit\n"
			s.gitFail, s.openFail = tc.gitFail, tc.openFail
			if tc.files != "" {
				f.hub.mu.Lock()
				delete(f.hub.routes, list(tc.files))
				f.hub.mu.Unlock()
			}
			code, _, stderr := f.agents(t, "land-stack", "2")
			if code == 0 || s.prs[2].labeled("merge-queue") ||
				!strings.Contains(stderr, "broke") && !strings.Contains(stderr, "404") {
				t.Fatalf("%d %q", code, stderr)
			}
		})
	}
}

func TestFlowGate_namesACommitWithNoPRNumberBySubject(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	s := gateStack(t, f, map[int][]File{2: {{Filename: "packages/flows/app/00.tsv"}}})
	s.gitOut["diff --name-only base..origin/fb"] = "packages/flows/app/00.tsv\n"
	s.gitOut["log -1 --format=%s base..origin/fb -- packages/flows/app/00.tsv"] = "a hand-pushed commit\n"
	if code, _, stderr := f.agents(t, "land-stack", "2"); code == 0 ||
		!strings.Contains(stderr, "flow 00 changed on staging since this stack's base (a hand-pushed commit)") {
		t.Fatalf("%d %q", code, stderr)
	}
}

func TestFlowGate_refusesAPendingStackInsteadOfArmingIt(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	s := armedStack(t, f)
	s.gitOut = gateGit("b2-oid")
	f.hub.on(list("/pulls/2/files?"), []File{{Filename: "packages/flows/app/00.tsv"}})
	s.gitOut["diff --name-only base..origin/fb"] = "packages/flows/app/00.tsv\n"
	s.gitOut["log -1 --format=%s base..origin/fb -- packages/flows/app/00.tsv"] = "Edit flow 00 (#77)\n"
	code, stdout, stderr := f.agents(t, "land-stack", "2")
	if code == 0 || !strings.Contains(stderr, "flow 00 changed on staging since this stack's base (#77)") ||
		f.owned(t).Armed != nil {
		t.Fatalf("%d %q %q", code, stdout, stderr)
	}
}

func TestFlowGate_disarmsAnArmedStackWhoseFlowMovedBeforeItWentGreen(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	s := armedStack(t, f)
	s.gitOut = gateGit("b2-oid")
	f.hub.on(list("/pulls/2/files?"), []File{{Filename: "packages/flows/app/00.tsv"}})
	if code, stdout, stderr := f.agents(t, "land-stack", "2"); code != 0 || f.owned(t).Armed == nil {
		t.Fatalf("arm: %d %q %q", code, stdout, stderr)
	}
	*s.prs[2] = *green(t, 2, "b2", "b1")
	s.gitOut["diff --name-only base..origin/fb"] = "packages/flows/app/00.tsv\n"
	s.gitOut["log -1 --format=%s base..origin/fb -- packages/flows/app/00.tsv"] = "Edit flow 00 (#77)\n"
	f.noFailures()
	code, stdout, stderr := f.agents(t, "watch", "--once")
	if code != 0 || !strings.Contains(stdout, "armed stack #2 disarmed") ||
		!strings.Contains(stdout, "flow 00 changed on staging since this stack's base (#77)") ||
		s.prs[2].labeled("merge-queue") {
		t.Fatalf("%d %q %q", code, stdout, stderr)
	}
}
