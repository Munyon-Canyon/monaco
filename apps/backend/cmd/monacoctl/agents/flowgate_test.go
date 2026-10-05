package agents

import (
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/tools/flows"
)

func gateGit(heads ...string) map[string]string {
	out := map[string]string{}
	for _, head := range heads {
		out["ls-tree --name-only "+head+" "+flows.Dir+"/"] = flows.Dir + "/00.tsv\n" + flows.Dir + "/01.tsv\n"
		out["show "+head+":"+flows.Dir+"/00.tsv"] = flows.Header + "\n" +
			"00\tPing\tsystem\tGET /p\tRecordPing\t\t\tok\tbuilt\tdocs/f.md\n"
		out["show "+head+":"+flows.Dir+"/01.tsv"] = flows.Header + "\n" +
			"01\tSign in\tidentity\tGET /s\tSignIn\t\t\tok\tbuilt\tdocs/f.md\n"
		out["show "+head+":"+flows.SpecPath] = gateSpec + "  /s:\n    get: {}\n"
	}
	out["merge-base origin/fb b2-oid"] = "base\n"
	out["diff --name-only base..origin/fb"] = ""
	out["rev-parse origin/fb"] = "tip\n"
	return out
}

const (
	dispatchRoute = "POST /repos/" + testRepo + "/actions/workflows/flows-verify.yml/dispatches"
	pendingRoute  = "POST /repos/" + testRepo + "/statuses/b2-oid"
)

func (f *fixture) flowsVerify(statuses ...GHStatus) {
	f.hub.on(list("/commits/b2-oid/statuses?"), append([]GHStatus{}, statuses...))
	f.hub.on(dispatchRoute, "")
	f.hub.on(pendingRoute, "{}")
}

func heldFor(t *testing.T, f *fixture, s *stackGH, want string) {
	t.Helper()
	code, stdout, stderr := f.agents(t, "land-stack", "2")
	if code != 0 || !strings.Contains(stdout, want) || s.prs[2].labeled("merge-queue") || f.owned(t).Armed == nil {
		t.Fatalf("want the stack armed on %q: %d %q %q", want, code, stdout, stderr)
	}
}

const gateSpec = "paths:\n  /p:\n    get: {}\n"

func gateStack(t *testing.T, f *fixture, files map[int][]File, extra ...*stackPR) *stackGH {
	t.Helper()
	s := newStackGH(t, f, append([]*stackPR{green(t, 1, "b1", "fb"), green(t, 2, "b2", "b1")}, extra...)...)
	s.gitOut = gateGit("b2-oid", "q2-oid")
	for n, fs := range files {
		f.hub.on(list("/pulls/"+strconv.Itoa(n)+"/files?"), fs)
	}
	f.owner(t, Record{Ticket: 40, Worktree: f.dir, State: Done})
	f.flowsVerify()
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

func TestFlowGate_startsFlowsVerifyForAFlowThatChangedOnStagingAndArms(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	s := gateStack(t, f, map[int][]File{2: {{Filename: "packages/flows/app/00.tsv"}}})
	s.gitOut["diff --name-only base..origin/fb"] = "apps/backend/internal/modules/system/http.go\nREADME.md\n"
	s.gitOut["log -1 --format=%s base..origin/fb -- apps/backend/internal/modules/system/http.go"] = "Record a ping note (#77)\n"
	heldFor(t, f, s, pingOnlyHold)
	if got := f.hub.body(
		dispatchRoute,
	); got != `{"inputs":{"base_sha":"tip","flows":"00","head_sha":"b2-oid","pr":"2"},"ref":"fb"}` {
		t.Fatalf("dispatched %q", got)
	}
	if got := f.hub.body(
		pendingRoute,
	); got != `{"context":"flows-verify","description":"flows 00 on staging tip","state":"pending"}` {
		t.Fatalf("status %q", got)
	}
	if s.prs[1].labeled("merge-queue") || f.owned(t).Queued != nil {
		t.Fatal("a held stack was queued")
	}

	s.gitOut["diff --name-only base..origin/fb"] = "apps/backend/internal/modules/identity/http.go\n"
	if code, stdout, stderr := f.agents(t, "land-stack", "2"); code != 0 || !s.prs[2].labeled("merge-queue") {
		t.Fatalf("a change to another flow on staging: %d %q %q", code, stdout, stderr)
	}
}

func TestFlowGate_namesTheChangedBackendFlowFile(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	s := gateStack(t, f, map[int][]File{1: {{Filename: flows.Dir + "/00.tsv"}}})
	s.gitOut["diff --name-only base..origin/fb"] = flows.Dir + "/00.tsv\n"
	s.gitOut["log -1 --format=%s base..origin/fb -- "+flows.Dir+"/00.tsv"] = "Rename the ping (#78)\n"
	heldFor(t, f, s, "flow 00 changed on staging since this stack's base (#78)")
	s.gitOut["diff --name-only base..origin/fb"] = flows.Dir + "/01.tsv\n"
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
	want := "not landing #2: flow 00 is in queued stack #7. Wait for #7 to land, then restack with gt (it changes these flows on staging) and run land-stack again"
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
		{"list the backend flow files", "ls-tree --name-only b2-oid " + flows.Dir, "", 0},
		{"read a backend flow file", "show b2-oid:" + flows.Dir + "/00.tsv", "", 0},
		{"fetch staging", "fetch --no-tags origin fb", "", 0},
		{"merge-base", "merge-base", "", 0},
		{"diff staging", "diff --name-only", "", 0},
		{"log the change", "log -1", "", 0},
		{"fetch a queued stack head", "fetch --no-tags origin q1", "", 0},
		{"list the open PRs a second time", "", "", 2},
		{"list a PR's files", "", "/pulls/2/files?", 0},
		{"list a queued PR's files", "", "/pulls/6/files?", 0},
		{"read the staging tip", "rev-parse origin/fb", "", 0},
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
				s.gitOut["diff --name-only base..origin/fb"] = flows.Dir + "/00.tsv\n"
			}
			s.gitOut["log -1 --format=%s base..origin/fb -- "+flows.Dir+"/00.tsv"] = "a hand-pushed commit\n"
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

func TestFlowGate_aSpecChangeGatesOnlyTheFlowsWhoseRoutesChanged(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	q1, q2 := green(t, 6, "q1", "fb"), green(t, 7, "q2", "q1")
	labeled(q1, "merge-queue")
	labeled(q2, "merge-queue")
	s := gateStack(t, f, map[int][]File{
		2: {{Filename: flows.SpecPath}, {Filename: "apps/backend/api/spec/identity.yaml"}},
		7: {{Filename: "apps/backend/internal/modules/identity/app.go"}},
	}, q1, q2)
	s.gitOut["show base:"+flows.SpecPath] = gateSpec
	want := "not landing #2: flow 01 is in queued stack #7"
	if code, stdout, stderr := f.agents(t, "land-stack", "2"); code == 0 || !strings.Contains(stderr, want) {
		t.Fatalf("a queued stack on flow 01: %d %q %q", code, stdout, stderr)
	}

	f.hub.on(list("/pulls/7/files?"), []File{
		{Filename: "packages/mobile-core/Sources/MonacoSystem/Flow00SystemPingModel.swift"},
	})
	if code, stdout, stderr := f.agents(t, "land-stack", "2"); code != 0 || !s.prs[2].labeled("merge-queue") {
		t.Fatalf("a queued stack on flow 00 only: %d %q %q", code, stdout, stderr)
	}
}

func TestFlowGate_specReadFailuresLabelNothing(t *testing.T) {
	t.Parallel()
	for _, gitFail := range []string{
		"merge-base origin/fb b2-oid",
		"show base:" + flows.SpecPath,
		"show b2-oid:" + flows.SpecPath,
		"merge-base base origin/fb",
	} {
		t.Run(gitFail, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			s := gateStack(t, f, map[int][]File{2: {{Filename: flows.SpecPath}}})
			s.gitOut["show base:"+flows.SpecPath] = gateSpec
			s.gitOut["diff --name-only base..origin/fb"] = flows.SpecPath + "\n"
			s.gitOut["merge-base base origin/fb"] = "base\n"
			s.gitFail = gitFail
			code, _, stderr := f.agents(t, "land-stack", "2")
			if code == 0 || s.prs[2].labeled("merge-queue") || !strings.Contains(stderr, "broke") {
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
	heldFor(t, f, s, "flow 00 changed on staging since this stack's base (a hand-pushed commit)")
}

func TestFlowGate_armsAPendingStackOnStage1AndFlowsVerify(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	s := armedStack(t, f)
	s.gitOut = gateGit("b2-oid")
	f.flowsVerify()
	f.hub.on(list("/pulls/2/files?"), []File{{Filename: "packages/flows/app/00.tsv"}})
	s.gitOut["diff --name-only base..origin/fb"] = "packages/flows/app/00.tsv\n"
	s.gitOut["log -1 --format=%s base..origin/fb -- packages/flows/app/00.tsv"] = "Edit flow 00 (#77)\n"
	heldFor(t, f, s, "(waiting on #2 (stage 1 pending), flows-verify of flows 00 on staging tip: "+
		"flow 00 changed on staging since this stack's base (#77))\n")
}

func movedArmedStack(t *testing.T) (*fixture, *stackGH) {
	t.Helper()
	f := newFixture(t)
	s := armedStack(t, f)
	s.gitOut = gateGit("b2-oid")
	f.flowsVerify()
	f.hub.on(list("/pulls/2/files?"), []File{{Filename: "packages/flows/app/00.tsv"}})
	if code, stdout, stderr := f.agents(t, "land-stack", "2"); code != 0 || f.owned(t).Armed == nil {
		t.Fatalf("arm: %d %q %q", code, stdout, stderr)
	}
	*s.prs[2] = *green(t, 2, "b2", "b1")
	s.gitOut["diff --name-only base..origin/fb"] = "packages/flows/app/00.tsv\n"
	s.gitOut["log -1 --format=%s base..origin/fb -- packages/flows/app/00.tsv"] = "Edit flow 00 (#77)\n"
	f.noFailures()
	return f, s
}

func flowsStatus(f *fixture, state, staging string, age time.Duration) GHStatus {
	return GHStatus{
		State: state, Context: "flows-verify", Description: "flows 00 on staging " + staging,
		TargetURL: "https://run/1", CreatedAt: f.now.Add(-age),
	}
}

func TestFlowGate_watchStartsFlowsVerifyOnceAndKeepsTheStackArmed(t *testing.T) {
	t.Parallel()
	f, s := movedArmedStack(t)
	before := len(f.hub.callsContaining("/dispatches"))
	code, stdout, stderr := f.agents(t, "watch", "--once")
	want := "armed stack #2 started flows-verify of flows 00 on staging tip: " +
		"flow 00 changed on staging since this stack's base (#77)"
	if code != 0 || !strings.Contains(stdout, want) || s.prs[2].labeled("merge-queue") || f.owned(t).Armed == nil ||
		len(f.hub.callsContaining("/dispatches")) != before+1 {
		t.Fatalf("%d %q %q", code, stdout, stderr)
	}

	f.flowsVerify(flowsStatus(f, "pending", "tip", 10*time.Minute))
	code, stdout, stderr = f.agents(t, "watch", "--once")
	if code != 0 || strings.Contains(stdout, "armed stack #2") || f.owned(t).Armed == nil ||
		len(f.hub.callsContaining("/dispatches")) != before+1 {
		t.Fatalf("a running flows-verify: %d %q %q", code, stdout, stderr)
	}
}

func TestFlowGate_watchRestartsFlowsVerifyWhenStagingMovesOrTheRunGoesQuiet(t *testing.T) {
	t.Parallel()
	for name, status := range map[string]func(*fixture) GHStatus{
		"staging moved": func(f *fixture) GHStatus { return flowsStatus(f, "success", "older-tip", time.Minute) },
		"another flow set": func(*fixture) GHStatus {
			return GHStatus{State: "success", Context: "flows-verify", Description: "flows 01 on staging tip"}
		},
		"a stale pending": func(f *fixture) GHStatus { return flowsStatus(f, "pending", "tip", 31*time.Minute) },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f, s := movedArmedStack(t)
			f.flowsVerify(status(f))
			code, stdout, stderr := f.agents(t, "watch", "--once")
			if code != 0 ||
				!strings.Contains(stdout, "armed stack #2 started flows-verify of flows 00 on staging tip") ||
				s.prs[2].labeled("merge-queue") {
				t.Fatalf("%d %q %q", code, stdout, stderr)
			}
		})
	}
}

func TestFlowGate_watchLandsTheStackOnceFlowsVerifyPasses(t *testing.T) {
	t.Parallel()
	f, s := movedArmedStack(t)
	f.flowsVerify(flowsStatus(f, "success", "tip", time.Minute), flowsStatus(f, "failure", "older-tip", time.Hour))
	code, stdout, stderr := f.agents(t, "watch", "--once")
	if code != 0 || !strings.Contains(stdout, "armed stack #2 landing") || !s.prs[2].labeled("merge-queue") {
		t.Fatalf("%d %q %q", code, stdout, stderr)
	}
}

func TestFlowGate_watchDisarmsTheStackWhenFlowsVerifyFails(t *testing.T) {
	t.Parallel()
	for _, state := range []string{"failure", "error"} {
		t.Run(state, func(t *testing.T) {
			t.Parallel()
			f, s := movedArmedStack(t)
			f.flowsVerify(flowsStatus(f, state, "tip", time.Minute))
			code, stdout, stderr := f.agents(t, "watch", "--once")
			want := "armed stack #2 disarmed: not landing #2: flow 00 changed on staging since this stack's base (#77), " +
				"and flows-verify failed on staging tip (https://run/1). " +
				"Restack with gt and rerun stage 1, then run land-stack again"
			if code != 0 || !strings.Contains(stdout, want) || s.prs[2].labeled("merge-queue") ||
				f.owned(t).Armed != nil {
				t.Fatalf("%d %q %q", code, stdout, stderr)
			}
		})
	}
}

func TestFlowGate_aFailedFlowsVerifyCallLeavesTheStackUnarmed(t *testing.T) {
	t.Parallel()
	for _, route := range []string{list("/commits/b2-oid/statuses?"), dispatchRoute, pendingRoute} {
		t.Run(route, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			s := gateStack(t, f, map[int][]File{2: {{Filename: "packages/flows/app/00.tsv"}}})
			s.gitOut["diff --name-only base..origin/fb"] = "packages/flows/app/00.tsv\n"
			s.gitOut["log -1 --format=%s base..origin/fb -- packages/flows/app/00.tsv"] = "Edit flow 00 (#77)\n"
			f.hub.status[route] = http.StatusInternalServerError
			code, stdout, stderr := f.agents(t, "land-stack", "2")
			if code == 0 || !strings.Contains(stderr, "500") || f.owned(t).Armed != nil ||
				s.prs[2].labeled("merge-queue") {
				t.Fatalf("%d %q %q", code, stdout, stderr)
			}
		})
	}
}

func TestFlowGate_aFlowSetTooLongForAStatusIsRefusedForARestack(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	s := gateStack(t, f, map[int][]File{2: {{Filename: "packages/flows/app/00.tsv"}}})
	s.gitOut["rev-parse origin/fb"] = strings.Repeat("f", 140) + "\n"
	s.gitOut["diff --name-only base..origin/fb"] = "packages/flows/app/00.tsv\n"
	s.gitOut["log -1 --format=%s base..origin/fb -- packages/flows/app/00.tsv"] = "Edit flow 00 (#77)\n"
	code, stdout, stderr := f.agents(t, "land-stack", "2")
	want := "not landing #2: flow 00 changed on staging since this stack's base (#77). " +
		"Restack with gt and rerun stage 1, then run land-stack again"
	if code == 0 || !strings.Contains(stderr, want) || len(f.hub.callsContaining("/dispatches")) != 0 {
		t.Fatalf("%d %q %q", code, stdout, stderr)
	}
}

const (
	pingModule   = "apps/backend/internal/modules/system/http.go"
	signInModule = "apps/backend/internal/modules/identity/http.go"
	pingOnlyHold = "(waiting on flows-verify of flows 00 on staging tip: " +
		"flow 00 changed on staging since this stack's base (#77))\n"
)

func problemSpec(pingSummary string, codes ...string) string {
	route := func(summary string) string {
		return "    get:\n      summary: " + summary +
			"\n      responses:\n        default: {$ref: \"#/components/responses/Problem\"}\n"
	}
	return "paths:\n  /p:\n" + route(pingSummary) + "  /s:\n" + route("Sign in.") +
		"components:\n  responses:\n    Problem:\n      content:\n        application/problem+json:\n" +
		"          schema: {$ref: \"#/components/schemas/Problem\"}\n" +
		"  schemas:\n    Problem:\n      type: object\n" +
		"      properties:\n        code: {$ref: \"#/components/schemas/ErrorCode\"}\n" +
		"    ErrorCode:\n      type: string\n      enum:\n        - " + strings.Join(codes, "\n        - ") + "\n"
}

func pingRow(outcomes string) string {
	return flows.Header + "\n00\tPing\tsystem\tGET /p\tRecordPing\t\t\t" + outcomes + "\tbuilt\tdocs/f.md\n"
}

func errorCodeStack(t *testing.T, pingSummary string) (*fixture, *stackGH) {
	t.Helper()
	f := newFixture(t)
	s := gateStack(t, f, map[int][]File{2: {
		{Filename: flows.SpecPath},
		{Filename: "apps/backend/internal/errs/codes_treasury.go"},
		{Filename: "apps/backend/internal/errs/testdata/codes_treasury.golden"},
		{Filename: "apps/backend/internal/platform/httpx/api/api.gen.go"},
	}})
	headCodes := []string{"not_found", "price_unavailable", "rpc_unavailable"}
	s.gitOut["show base:"+flows.SpecPath] = problemSpec("Ping.", "not_found")
	s.gitOut["show b2-oid:"+flows.SpecPath] = problemSpec(pingSummary, headCodes...)
	s.gitOut["diff --name-only base..origin/fb"] = pingModule + "\n" + signInModule + "\n"
	s.gitOut["log -1 --format=%s base..origin/fb -- "+pingModule] = "Record a ping note (#77)\n"
	s.gitOut["log -1 --format=%s base..origin/fb -- "+signInModule] = "Rename sign in (#78)\n"
	return f, s
}

func TestFlowGate_aNewErrorCodeAloneWaitsForNoFlowThatMovedOnStaging(t *testing.T) {
	t.Parallel()
	f, s := errorCodeStack(t, "Ping.")
	if code, stdout, stderr := f.agents(t, "land-stack", "2"); code != 0 || !s.prs[2].labeled("merge-queue") {
		t.Fatalf("%d %q %q", code, stdout, stderr)
	}
}

func TestFlowGate_aStackThatChangesARouteStillWaitsForTheFlowOfThatRoute(t *testing.T) {
	t.Parallel()
	f, s := errorCodeStack(t, "Ping again.")
	heldFor(t, f, s, pingOnlyHold)
}

func TestFlowGate_aNewErrorCodeWaitsForTheFlowsThatListItAsAnOutcome(t *testing.T) {
	t.Parallel()
	f, s := errorCodeStack(t, "Ping.")
	for _, listed := range []string{"PriceUnavailable", "RPCUnavailable"} {
		s.gitOut["show b2-oid:"+flows.Dir+"/00.tsv"] = pingRow("ok;" + listed)
		heldFor(t, f, s, pingOnlyHold)
	}

	s.gitOut["show b2-oid:"+flows.Dir+"/00.tsv"] = pingRow("ok;InvalidInput")
	if code, stdout, stderr := f.agents(t, "land-stack", "2"); code != 0 || !s.prs[2].labeled("merge-queue") {
		t.Fatalf("a flow that lists another code: %d %q %q", code, stdout, stderr)
	}
}

func TestFlowGate_aCodeAddedOnStagingMovesOnlyTheFlowsThatListIt(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	s := gateStack(t, f, map[int][]File{2: {{Filename: "packages/flows/app/00.tsv"}}})
	s.gitOut["merge-base base origin/fb"] = "base\n"
	s.gitOut["diff --name-only base..origin/fb"] = "README.md\n" + flows.SpecPath + "\n"
	s.gitOut["show base:"+flows.SpecPath] = problemSpec("Ping.", "not_found")
	s.gitOut["show origin/fb:"+flows.SpecPath] = problemSpec("Ping.", "not_found", "price_unavailable")
	s.gitOut["log -1 --format=%s base..origin/fb -- "+flows.SpecPath] = "Add price_unavailable (#80)\n"
	s.gitOut["show b2-oid:"+flows.Dir+"/00.tsv"] = pingRow("ok;PriceUnavailable")
	heldFor(t, f, s, "flow 00 changed on staging since this stack's base (#80)")

	s.gitOut["show b2-oid:"+flows.Dir+"/00.tsv"] = pingRow("ok")
	if code, stdout, stderr := f.agents(t, "land-stack", "2"); code != 0 || !s.prs[2].labeled("merge-queue") {
		t.Fatalf("a flow that lists no new code: %d %q %q", code, stdout, stderr)
	}
}

func TestFlowGate_aSpecThatDoesNotParseCountsEveryRouteAsChanged(t *testing.T) {
	t.Parallel()
	f, s := errorCodeStack(t, "Ping.")
	s.gitOut["show b2-oid:"+flows.SpecPath] = "paths: [\n"
	heldFor(t, f, s, "flows-verify of flows 00 01 on staging tip: "+
		"flow 00 changed on staging since this stack's base (#77); "+
		"flow 01 changed on staging since this stack's base (#78)")
}

func TestFlowGate_aTestOnlyChangeOnStagingLike2743MovesNoFlow(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	s := gateStack(t, f, map[int][]File{2: {{Filename: "apps/backend/internal/modules/identity/http.go"}}})
	s.gitOut["diff --name-only base..origin/fb"] = "apps/backend/internal/modules/identity/http_test.go\n" +
		"apps/backend/internal/modules/identity/verifier.go\n"
	s.gitOut["log -1 --format=%s base..origin/fb -- apps/backend/internal/modules/identity/verifier.go"] = "Verify (#79)\n"
	heldFor(t, f, s, "flow 01 changed on staging since this stack's base (#79)")

	s.gitOut["diff --name-only base..origin/fb"] = strings.Join([]string{
		"apps/backend/internal/modules/identity/availability_test.go",
		"apps/backend/internal/modules/identity/delete_me_http_test.go",
		"apps/backend/internal/modules/identity/http_test.go",
		"apps/backend/internal/modules/identity/main_test.go",
		"apps/backend/internal/modules/identity/verifier_test.go",
	}, "\n") + "\n"
	if code, stdout, stderr := f.agents(t, "land-stack", "2"); code != 0 || !s.prs[2].labeled("merge-queue") {
		t.Fatalf("%d %q %q", code, stdout, stderr)
	}
}
