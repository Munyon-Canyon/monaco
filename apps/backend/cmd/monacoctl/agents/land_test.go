package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type ghCall struct {
	dir, stdin, line string
}

type stackGH struct {
	t     *testing.T
	mu    sync.Mutex
	prs   map[int]*stackPR
	pages map[string]string
	calls []ghCall
	fail  string
	raw   string
	gtLog string
	git   map[string]error
	repo  bool

	denied bool
	gql    int

	gitOut   map[string]string
	gitCalls []string
	gitFail  string
	openFail int
	opens    int
}

func newStackGH(t *testing.T, f *fixture, prs ...*stackPR) *stackGH {
	t.Helper()
	s := &stackGH{t: t, prs: map[int]*stackPR{}, git: map[string]error{}}
	nums := make([]string, 0, len(prs))
	for _, p := range prs {
		s.prs[p.Number] = p
		nums = append(nums, strconv.Itoa(p.Number))
		f.hub.on(fmt.Sprintf("POST /repos/%s/issues/%d/labels", testRepo, p.Number), "[]")
		f.hub.on(fmt.Sprintf("DELETE /repos/%s/issues/%d/labels/merge-queue", testRepo, p.Number), "[]")
		f.hub.on(fmt.Sprintf("DELETE /repos/%s/issues/%d/labels/ship-it", testRepo, p.Number), "[]")
		f.hub.on(list(fmt.Sprintf("/pulls/%d/files?", p.Number)), "[]")
	}
	f.hub.on(list("/pulls?state=open"), []PR{{Number: 900, Title: "[Graphite MQ] Draft PR GROUP:x (PRs " +
		strings.Join(nums, ", ") + ")", Head: Ref{Ref: "gtmq_x"}}})
	f.hub.on(graphqlRoute, draftData(nil))
	f.hub.hook = s.onLabel
	f.run = s.run
	return s
}

func (s *stackGH) onLabel(method, path, body string, status int) {
	if status >= 300 {
		return
	}
	m := regexp.MustCompile(`/issues/(\d+)/labels`).FindStringSubmatch(path)
	if m == nil {
		return
	}
	n, _ := strconv.Atoi(m[1])
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.prs[n]
	if p == nil {
		return
	}
	switch method {
	case http.MethodPost:
		var payload struct {
			Labels []string `json:"labels"`
		}
		if json.Unmarshal([]byte(body), &payload) == nil {
			for _, label := range payload.Labels {
				labeled(p, label)
			}
		}
	case http.MethodDelete:
		p.Labels.Nodes = nil
	}
}

func stackOf(t *testing.T, n int, head, base, stage1, verify string) *stackPR {
	t.Helper()
	var contexts []string
	switch stage1 {
	case "":
	case "pending":
		contexts = append(contexts, `{"name":"ci / ci-ok","status":"IN_PROGRESS"}`)
	default:
		contexts = append(contexts, fmt.Sprintf(`{"name":"ci / ci-ok","status":"COMPLETED","conclusion":%q}`, stage1))
	}
	if verify != "" {
		contexts = append(contexts, fmt.Sprintf(`{"context":"verify","state":%q}`, verify))
	}
	raw := fmt.Sprintf(`{"number":%d,"state":"OPEN","baseRefName":%q,"headRefName":%q,"headRefOid":"%s-oid",`+
		`"body":"Part of #40\n\n## TLDR\nx","commits":{"nodes":[{"commit":{"statusCheckRollup":`+
		`{"contexts":{"nodes":[%s]}}}}]}}`, n, base, head, head, strings.Join(contexts, ","))
	var p stackPR
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		t.Fatal(err)
	}
	return &p
}

func withFormat(p *stackPR, conclusion string) *stackPR {
	rollup := &p.Commits.Nodes[0].Commit.StatusCheckRollup.Contexts
	rollup.Nodes = append(rollup.Nodes, gqlContext{Name: formatCheck, Conclusion: conclusion})
	return p
}

func green(t *testing.T, n int, head, base string) *stackPR {
	t.Helper()
	return stackOf(t, n, head, base, "SUCCESS", "SUCCESS")
}

var (
	aliasRE = regexp.MustCompile(`p(\d+): pullRequest`)
	pageRE  = regexp.MustCompile(`c0: object\(oid:"(\w+)"\)`)
)

func (s *stackGH) run(ctx context.Context, dir, stdin, name string, args ...string) ([]byte, error) {
	if name == "git" && s.gitOut != nil {
		line := strings.Join(args, " ")
		s.mu.Lock()
		defer s.mu.Unlock()
		s.gitCalls = append(s.gitCalls, line)
		if s.gitFail != "" && strings.HasPrefix(line, s.gitFail) {
			return nil, errors.New("git broke")
		}
		if out, ok := s.gitOut[line]; ok || args[0] == "fetch" || args[0] == "merge-tree" {
			return []byte(out), nil
		}
		return nil, errors.New("unexpected git " + line)
	}
	if name == "git" && !s.repo && (args[0] == "fetch" || args[0] == "merge-tree") {
		s.mu.Lock()
		defer s.mu.Unlock()
		return nil, s.git[args[0]]
	}
	if name != "gh" && name != "gt" && name != "bash" && name != "go" {
		return hostless(ctx, dir, stdin, name, args...)
	}
	line := name + " " + strings.Join(args, " ")
	s.mu.Lock()
	defer s.mu.Unlock()
	if args[0] != "api" && args[0] != "log" {
		s.calls = append(s.calls, ghCall{dir, stdin, line})
	}
	if s.fail != "" && strings.HasPrefix(line, s.fail) {
		return nil, errors.New(s.fail + ": boom")
	}
	if name == "bash" || name == "go" {
		return nil, nil
	}
	if name == "gt" {
		return []byte(s.gtLog), nil
	}
	if args[0] == "api" {
		s.gql++
		if s.denied {
			return nil, errors.New("HTTP 403: Resource not accessible by integration")
		}
		return s.graphql(args[3])
	}
	n, _ := strconv.Atoi(args[2])
	switch args[1] + " " + args[3] {
	case "edit --add-label":
		labeled(s.prs[n], args[4])
	case "edit --remove-label":
		s.prs[n].Labels.Nodes = nil
	}
	return nil, nil
}

func labeled(p *stackPR, label string) *stackPR {
	if !p.labeled(label) {
		p.Labels.Nodes = append(p.Labels.Nodes, gqlName{label})
	}
	return p
}

func (s *stackGH) graphql(query string) ([]byte, error) {
	if s.raw != "" {
		return []byte(s.raw), nil
	}
	if m := pageRE.FindStringSubmatch(query); m != nil {
		return []byte(s.pages[m[1]]), nil
	}
	repo := map[string]any{}
	if strings.Contains(query, "open: pullRequests(states:OPEN") {
		if s.opens++; s.opens == s.openFail {
			return nil, errors.New("open pulls broke")
		}
		var open []*stackPR
		for _, n := range slices.Sorted(maps.Keys(s.prs)) {
			if s.prs[n].State == "OPEN" {
				open = append(open, s.prs[n])
			}
		}
		repo["open"] = map[string]any{"nodes": open}
	}
	for _, m := range aliasRE.FindAllStringSubmatch(query, -1) {
		n, _ := strconv.Atoi(m[1])
		if p, ok := s.prs[n]; ok {
			repo["p"+m[1]] = p
		} else {
			repo["p"+m[1]] = nil
		}
	}
	return json.Marshal(map[string]any{"data": map[string]any{"repository": repo}})
}

func (s *stackGH) lines() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.calls))
	for i, c := range s.calls {
		out[i] = c.line
	}
	return out
}

func (f *fixture) owned(t *testing.T) Record {
	t.Helper()
	r, err := f.Env(t).localRecord(40)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestLandStack_refusesNamingEveryPRItWaitsOnAndChangesNothing(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	s := newStackGH(t, f,
		stackOf(t, 1, "b1", "fb", "FAILURE", "FAILURE"),
		stackOf(t, 2, "b2", "b1", "SUCCESS", ""),
		stackOf(t, 3, "b3", "b2", "pending", "SUCCESS"),
		stackOf(t, 4, "b4", "b3", "", "PENDING"),
	)
	f.owner(t, Record{Ticket: 40, Worktree: f.dir, State: Done})
	code, stdout, stderr := f.agents(t, "land-stack", "4")
	want := "not landing #4; waiting on #1 (stage 1 failure), #3 (stage 1 pending), #4 (stage 1 missing)\n"
	if code != 0 || stdout != want || stderr != "" {
		t.Fatalf("%d %q %q", code, stdout, stderr)
	}
	if calls := s.lines(); len(calls) != 0 {
		t.Fatalf("a refusal ran %v", calls)
	}
	if len(f.owned(t).Queued) > 0 {
		t.Fatal("a refusal marked the stack queued")
	}
}

func TestLandStack_labelsEveryPRAndKeepsTheirBases(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	s := newStackGH(t, f,
		green(t, 1, "b1", "fb"), green(t, 2, "b2", "b1"), green(t, 3, "b3", "b2"),
		green(t, 7, "other", "fb"), green(t, 8, "above-other", "other"),
	)
	f.owner(t, Record{Ticket: 40, Worktree: f.dir, State: Done})
	code, stdout, stderr := f.agents(t, "land-stack", "3")
	if code != 0 ||
		stdout != "queued #1 #2 #3\nfollow it: monacoctl agents watch (under Claude Code's Monitor tool)\nqueued together: #1 #2 #3\n" {
		t.Fatalf("%d %q %q", code, stdout, stderr)
	}
	want := []string{
		"POST /repos/o/r/issues/3/labels",
		"POST /repos/o/r/issues/2/labels",
		"POST /repos/o/r/issues/1/labels",
	}
	if got := f.hub.callsContaining("/labels"); !slices.Equal(got, want) {
		t.Fatalf("calls:\n%s", strings.Join(got, "\n"))
	}
	if s.prs[2].Base != "b1" || s.prs[3].Base != "b2" || s.prs[3].Body != "Part of #40\n\n## TLDR\nx" {
		t.Fatalf("bases %s %s, body %q", s.prs[2].Base, s.prs[3].Base, s.prs[3].Body)
	}
	if s.prs[7].labeled("merge-queue") || s.prs[8].labeled("merge-queue") {
		t.Fatal("labelled a PR outside the stack")
	}
	if q := f.owned(t).Queued; len(q) != 1 || q[0].Top != 3 || !slices.Equal(q[0].PRs, []int{1, 2, 3}) {
		t.Fatalf("queued %+v", q)
	}
	if got := posted(t, f, "POST /repos/o/r/issues/40/comments"); !strings.Contains(
		got, `"queued":{"top":3,"prs":[1,2,3],"at":"2026-09-27T12:00:00Z"},"queues":[{"top":3,`,
	) {
		t.Fatalf("published %q", got)
	}
	code, stdout, _ = f.agents(t, "land-stack", "3")
	if code != 0 || stdout != "#3 is queued in the Graphite merge queue\n" ||
		len(f.hub.callsContaining("/labels")) != len(want) {
		t.Fatalf("second call: %d %q %v", code, stdout, f.hub.callsContaining("/labels"))
	}
}

func TestLandStack_labelsTheTopFirstSoGraphiteQueuesTheStackTogether(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	newStackGH(t, f, green(t, 1, "b1", "fb"), green(t, 2, "b2", "b1"), green(t, 3, "b3", "b2"))
	f.owner(t, Record{Ticket: 40, Worktree: f.dir, State: Done})
	if code, _, stderr := f.agents(t, "land-stack", "3"); code != 0 {
		t.Fatalf("%d %q", code, stderr)
	}
	got := f.hub.callsContaining("/labels")
	if len(got) == 0 || got[0] != "POST /repos/o/r/issues/3/labels" {
		t.Fatalf("the first label went to %v, want the top PR #3 so the bottom is never labeled alone", got)
	}
}

func TestLandStack_queueingAgainClearsTheLastSettlement(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	newStackGH(t, f, green(t, 1, "b1", "fb"))
	last := &Settlement{Top: 1, PRs: []int{1}, Outcome: outcomeEjected, Detail: "stack #1 ejected", At: f.now}
	f.owner(t, Record{Ticket: 40, Worktree: f.dir, State: Done, Settled: last})
	if code, stdout, stderr := f.agents(t, "land-stack", "1"); code != 0 || f.owned(t).Settled != nil {
		t.Fatalf("%d %q %q %+v", code, stdout, stderr, f.owned(t).Settled)
	}
}

func pagedStack(t *testing.T, n int, head, base string, first ...string) *stackPR {
	t.Helper()
	raw := fmt.Sprintf(`{"number":%d,"state":"OPEN","baseRefName":%q,"headRefName":%q,`+
		`"body":"Part of #40\n\n## TLDR\nx","commits":{"nodes":[{"commit":%s}]}}`, n, base, head, firstPage(head, first...))
	var p stackPR
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		t.Fatal(err)
	}
	return &p
}

func TestLandStack_readsEveryCheckAndTheNewestRunOfEach(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, rest, stdout string
	}{
		{
			"the newest ci-ok and verify sit past the first page",
			lastPage(ciOK("SUCCESS", 3), verifyAt("SUCCESS", 4)),
			"queued #1 #2\nfollow it: monacoctl agents watch (under Claude Code's Monitor tool)\nqueued together: #1 #2\n",
		},
		{
			"a newer ci-ok failed",
			lastPage(ciOK("FAILURE", 3), verifyAt("SUCCESS", 4)),
			"not landing #2; waiting on #1 (stage 1 failure)\n",
		},
		{
			"a rerun of ci-ok is still going",
			lastPage(ciOK("", 0), verifyAt("SUCCESS", 4)),
			"armed #2; agents watch lands it once stage 1 passes (waiting on #1 (stage 1 pending))\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			paged := pagedStack(t, 1, "b1", "fb", ciOK("FAILURE", 1), ciOK("SUCCESS", 2))
			s := newStackGH(t, f, paged, green(t, 2, "b2", "b1"))
			s.pages = map[string]string{"b1": tc.rest}
			f.owner(t, Record{Ticket: 40, Worktree: f.dir, State: Done})
			if code, stdout, stderr := f.agents(t, "land-stack", "2"); code != 0 || stdout != tc.stdout {
				t.Fatalf("%d %q %q", code, stdout, stderr)
			}
		})
	}
}

func TestLandStack_failsWhenPagingChecksFails(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, lost string
		other      func(t *testing.T) *stackPR
	}{
		{"the top's checks", "b1", func(t *testing.T) *stackPR { t.Helper(); return green(t, 7, "b7", "fb") }},
		{"another open PR's checks", "b7", func(t *testing.T) *stackPR { t.Helper(); return pagedStack(t, 7, "b7", "fb") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			top := pagedStack(t, 1, "b1", "fb")
			if tc.lost == "b7" {
				top = green(t, 1, "b1", "fb")
			}
			s := newStackGH(t, f, top, tc.other(t))
			s.pages = map[string]string{tc.lost: `{"data":{"repository":{"c0":null}}}`}
			f.owner(t, Record{Ticket: 40, Worktree: f.dir, State: Done})
			code, _, stderr := f.agents(t, "land-stack", "1")
			if code != 1 || !strings.Contains(stderr, "commit "+tc.lost+" lost its checks") || len(s.lines()) != 0 {
				t.Fatalf("%d %q %v", code, stderr, s.lines())
			}
		})
	}
}

func TestLandStack_aSinglePRGetsOnlyTheLabel(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	s := newStackGH(t, f, green(t, 5, "b5", "fb"))
	f.owner(t, Record{Ticket: 40, Worktree: f.dir})
	if code, stdout, stderr := f.agents(
		t,
		"land-stack",
		"5",
	); code != 0 ||
		stdout != "queued #5\nfollow it: monacoctl agents watch (under Claude Code's Monitor tool)\nqueued together: #5\n" {
		t.Fatalf("%d %q %q", code, stdout, stderr)
	}
	want := []string{"POST /repos/o/r/issues/5/labels"}
	if got := f.hub.callsContaining("/labels"); !slices.Equal(got, want) {
		t.Fatalf("calls %v", got)
	}
	if !s.prs[5].labeled("merge-queue") {
		t.Fatal("#5 lacks the queue label")
	}
}

func TestLandStack_labelsWithTheConfiguredQueueLabel(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	labelled := strings.Replace(testConfig, "[batch]", "queue_label = \"ship-it\"\n[batch]", 1)
	writeFile(t, filepath.Join(f.dir, configPath), labelled)
	newStackGH(t, f, green(t, 5, "b5", "fb"))
	f.owner(t, Record{Ticket: 40, Worktree: f.dir})
	if code, stdout, stderr := f.agents(
		t,
		"land-stack",
		"5",
	); code != 0 ||
		stdout != "queued #5\nfollow it: monacoctl agents watch (under Claude Code's Monitor tool)\nqueued together: #5\n" {
		t.Fatalf("%d %q %q", code, stdout, stderr)
	}
	if got := f.hub.callsContaining("/labels"); !slices.Equal(got, []string{"POST /repos/o/r/issues/5/labels"}) ||
		!strings.Contains(f.hub.body("POST /repos/o/r/issues/5/labels"), `"ship-it"`) {
		t.Fatalf("calls %v body %s", got, f.hub.body("POST /repos/o/r/issues/5/labels"))
	}
	code, stdout, _ := f.agents(t, "land-stack", "5")
	if code != 0 || stdout != "#5 is queued in the Graphite merge queue\n" {
		t.Fatalf("second call: %d %q", code, stdout)
	}
}

type movedTrunk struct {
	worktree, head, tip string
}

func newMovedTrunk(t *testing.T, trunkFile string) movedTrunk {
	t.Helper()
	root := t.TempDir()
	origin, author := filepath.Join(root, "origin.git"), filepath.Join(root, "author")
	m := movedTrunk{worktree: filepath.Join(root, "worktree")}
	commit := func(file, content string) string {
		writeFile(t, filepath.Join(author, file), content)
		git(t, author, "add", ".")
		git(t, author, "commit", "-q", "-m", file)
		return strings.TrimSpace(gitOut(t, author, "rev-parse", "HEAD"))
	}
	git(t, root, "init", "-q", "--bare", "--template=", "-b", "fb", origin)
	git(t, root, "init", "-q", "--template=", "-b", "fb", author)
	commit("a.txt", "base\n")
	git(t, author, "remote", "add", "origin", origin)
	git(t, author, "push", "-q", "origin", "fb")
	git(t, author, "checkout", "-q", "-b", "b1")
	m.head = commit("b.txt", "pr\n")
	git(t, author, "push", "-q", "origin", "b1")
	git(t, root, "clone", "-q", origin, m.worktree)
	git(t, author, "checkout", "-q", "fb")
	m.tip = commit(trunkFile, "trunk\n")
	git(t, author, "push", "-q", "origin", "fb")
	return m
}

func movedStack(t *testing.T, f *fixture, trunkFile string) movedTrunk {
	t.Helper()
	m := newMovedTrunk(t, trunkFile)
	bottom := green(t, 1, "b1", "fb")
	bottom.HeadOID = m.head
	s := newStackGH(t, f, bottom, green(t, 2, "b2", "b1"))
	s.repo = true
	f.owner(t, Record{Ticket: 40, Worktree: m.worktree, State: Done})
	return m
}

func TestLandStack_queuesOnTheFirstRunWhenTheBottomMergesCleanlyOntoAMovedTrunk(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	m := movedStack(t, f, "c.txt")
	code, stdout, stderr := f.agents(t, "land-stack", "2")
	if code != 0 ||
		stdout != "queued #1 #2\nfollow it: monacoctl agents watch (under Claude Code's Monitor tool)\nqueued together: #1 #2\n" {
		t.Fatalf("%d %q %q", code, stdout, stderr)
	}
	want := []string{"POST /repos/o/r/issues/2/labels", "POST /repos/o/r/issues/1/labels"}
	if got := f.hub.callsContaining("/labels"); !slices.Equal(got, want) || len(f.waited) != 0 {
		t.Fatalf("labels %v, waited %v", got, f.waited)
	}
	if got := strings.TrimSpace(gitOut(t, m.worktree, "rev-parse", "origin/fb")); got != m.tip {
		t.Fatalf("checked against origin/fb %s, want the moved tip %s", got, m.tip)
	}
}

func TestLandStack_runsGitOnThisMachineWhenTheRecordsWorktreeIsElsewhere(t *testing.T) {
	t.Parallel()
	for _, checkedOut := range []bool{false, true} {
		t.Run(fmt.Sprintf("top branch checked out %v", checkedOut), func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			m := movedStack(t, f, "c.txt")
			git(t, f.dir, "init", "-q", "--template=", "-b", "fb")
			git(t, f.dir, "remote", "add", "origin", filepath.Join(filepath.Dir(m.worktree), "origin.git"))
			root := filepath.Dir(f.Env(t).Common)
			want := root
			if checkedOut {
				git(t, f.dir, "fetch", "-q", "origin", "b1")
				want = filepath.Join(root, ".worktrees", "b2")
				git(t, f.dir, "worktree", "add", "-q", "-b", "b2", want, "FETCH_HEAD")
			}
			gone := "/elsewhere/.worktrees/603"
			f.owner(t, Record{Ticket: 40, Worktree: gone, State: Done})
			code, stdout, stderr := f.agents(t, "land-stack", "2")
			if code != 0 || stdout != "record 40's worktree "+gone+" is not on this machine; using "+want+"\n"+
				"queued #1 #2\nfollow it: monacoctl agents watch (under Claude Code's Monitor tool)\nqueued together: #1 #2\n" {
				t.Fatalf("%d %q %q", code, stdout, stderr)
			}
			if got := strings.TrimSpace(gitOut(t, want, "rev-parse", "origin/fb")); got != m.tip {
				t.Fatalf("checked against origin/fb %s, want the moved tip %s", got, m.tip)
			}
			if got := f.owned(t).Worktree; got != gone {
				t.Fatalf("record worktree %q, want %q kept", got, gone)
			}
		})
	}
}

func TestLandStack_queuesAStackWhoseVerifyIsRedOrMissing(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	newStackGH(t, f, stackOf(t, 1, "b1", "fb", "SUCCESS", "FAILURE"), stackOf(t, 2, "b2", "b1", "SUCCESS", ""))
	f.owner(t, Record{Ticket: 40, Worktree: f.dir, State: Done})
	code, stdout, stderr := f.agents(t, "land-stack", "2")
	if code != 0 ||
		stdout != "queued #1 #2\nfollow it: monacoctl agents watch (under Claude Code's Monitor tool)\nqueued together: #1 #2\n" {
		t.Fatalf("%d %q %q", code, stdout, stderr)
	}
	want := []string{"POST /repos/o/r/issues/2/labels", "POST /repos/o/r/issues/1/labels"}
	if got := f.hub.callsContaining("/labels"); !slices.Equal(got, want) {
		t.Fatalf("labels %v", got)
	}
}

func TestLandStack_refusesABottomThatConflictsWithTheTrunkTip(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	movedStack(t, f, "b.txt")
	code, stdout, stderr := f.agents(t, "land-stack", "2")
	if code != 1 || stdout != "" || !strings.Contains(stderr, "#1 conflicts with fb. Fix the conflicts") ||
		!strings.Contains(stderr, "then run land-stack 2") {
		t.Fatalf("%d %q %q", code, stdout, stderr)
	}
	if got := f.hub.callsContaining("/labels"); len(got) != 0 || len(f.owned(t).Queued) > 0 {
		t.Fatalf("labelled %v, queued %+v", got, f.owned(t).Queued)
	}
}

func TestLandStack_failsWithoutLabelsWhenGitCannotCheckTheMerge(t *testing.T) {
	t.Parallel()
	for _, step := range []string{"fetch", "merge-tree"} {
		t.Run(step, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			s := newStackGH(t, f, green(t, 1, "b1", "fb"), green(t, 2, "b2", "b1"))
			s.git[step] = errors.New(step + ": boom")
			f.owner(t, Record{Ticket: 40, Worktree: f.dir, State: Done})
			code, stdout, stderr := f.agents(t, "land-stack", "2")
			if code != 1 || stdout != "" || !strings.Contains(stderr, step+": boom") ||
				!strings.Contains(stderr, "the stack is not marked queued") {
				t.Fatalf("%d %q %q", code, stdout, stderr)
			}
			if got := f.hub.callsContaining("/labels"); len(got) != 0 || len(f.owned(t).Queued) > 0 {
				t.Fatalf("labelled %v, queued %+v", got, f.owned(t).Queued)
			}
		})
	}
}

func TestLandStack_settlesTheQueuedStack(t *testing.T) {
	t.Parallel()
	const oid = "abcdef0123456789abcdef0123456789abcdef01"
	for _, tt := range []struct {
		name       string
		edit       func(prs map[int]*stackPR)
		stdout     string
		calls      []string
		stillQueue bool
	}{
		{
			name: "every PR merged",
			edit: func(prs map[int]*stackPR) {
				prs[2].State, prs[3].State, prs[3].MergeCommit.OID = "MERGED", "MERGED", oid
			},
			stdout: "#3 merged as abcdef0\n",
		},
		{
			name:   "every PR closed by the Graphite fast-forward",
			edit:   func(prs map[int]*stackPR) { prs[2].State, prs[3].State = "CLOSED", "CLOSED" },
			stdout: "#3 merged as b3-oid\n",
		},
		{
			name:       "a lower PR closed by the queue while the top waits",
			edit:       func(prs map[int]*stackPR) { prs[2].State = "CLOSED" },
			stdout:     "#3 is queued in the Graphite merge queue\n",
			stillQueue: true,
		},
		{
			name:       "the open PRs carry the label",
			edit:       func(map[int]*stackPR) {},
			stdout:     "#3 is queued in the Graphite merge queue\n",
			stillQueue: true,
		},
		{
			name:       "the top merged before a lower PR",
			edit:       func(prs map[int]*stackPR) { prs[3].State = "MERGED" },
			stdout:     "#3 is queued in the Graphite merge queue\n",
			stillQueue: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			merged := green(t, 1, "b1", "fb")
			merged.State = "MERGED"
			s := newStackGH(t, f, merged,
				labeled(green(t, 2, "b2", "fb"), "merge-queue"), labeled(green(t, 3, "b3", "b2"), "merge-queue"))
			tt.edit(s.prs)
			f.hub.on(get("/compare/fb...b2-oid"), `{"status":"identical"}`)
			f.hub.on(get("/compare/fb...b3-oid"), `{"status":"behind"}`)
			wt := t.TempDir()
			f.owner(t, Record{Ticket: 40, Worktree: wt, Queued: Queues{{Top: 3, PRs: []int{1, 2, 3}}}})
			code, stdout, stderr := f.agents(t, "land-stack", "3")
			if code != 0 || stdout != strings.ReplaceAll(tt.stdout, "WT", wt) {
				t.Fatalf("%d %q %q", code, stdout, stderr)
			}
			if got := s.lines(); !slices.Equal(got, tt.calls) {
				t.Fatalf("calls %v", got)
			}
			for _, c := range s.calls {
				if strings.HasPrefix(c.line, "gt ") && c.dir != wt {
					t.Fatalf("gt ran in %q", c.dir)
				}
			}
			if queued := len(f.owned(t).Queued) > 0; queued != tt.stillQueue {
				t.Fatalf("queued = %v", queued)
			}
		})
	}
}

func TestLandStack_failures(t *testing.T) {
	t.Parallel()
	stack := func(t *testing.T) []*stackPR {
		t.Helper()
		return []*stackPR{green(t, 1, "b1", "fb"), green(t, 2, "b2", "b1")}
	}
	for _, tt := range []struct {
		name   string
		args   []string
		prs    func(t *testing.T) []*stackPR
		rec    *Record
		fail   string
		raw    string
		code   int
		stderr string
	}{
		{name: "usage", args: []string{}, code: 2, stderr: "usage: monacoctl agents land-stack <top-pr>"},
		{name: "not a PR", args: []string{"9"}, prs: stack, code: 1, stderr: "#9 is not a PR"},
		{name: "graphql fails", args: []string{"2"}, prs: stack, fail: "gh api", code: 1, stderr: "gh api: boom"},
		{name: "open list fails", args: []string{"2"}, prs: stack, fail: "gh api graphql -f query=" + repoQuery + "open:", code: 1, stderr: "boom"},
		{name: "graphql garbage", args: []string{"2"}, prs: stack, raw: "{", code: 1, stderr: "decode gh api graphql"},
		{
			name: "no ticket", args: []string{"2"}, code: 1, stderr: `#2 links no ticket`,
			prs: func(t *testing.T) []*stackPR {
				t.Helper()
				p := green(t, 2, "b2", "fb")
				p.Body = "## TLDR"
				return []*stackPR{p}
			},
		},
		{name: "no owner record", args: []string{"2"}, prs: stack, rec: &Record{}, code: 1, stderr: "no owner record for #40"},
		{
			name: "another top queued", args: []string{"2"}, prs: stack, code: 1, stderr: "#40 already has #7 queued",
			rec: &Record{Ticket: 40, Queued: Queues{{Top: 7, PRs: []int{2, 7}}}},
		},
		{
			name: "top not open", args: []string{"2"}, code: 1, stderr: "#2 is not an open PR",
			prs: func(t *testing.T) []*stackPR {
				t.Helper()
				p := green(t, 2, "b2", "fb")
				p.State = "MERGED"
				return []*stackPR{p}
			},
		},
		{
			name: "orphan base", args: []string{"2"}, code: 1, stderr: "#2's base gone is neither fb nor an open PR",
			prs: func(t *testing.T) []*stackPR {
				t.Helper()
				return []*stackPR{green(t, 2, "b2", "gone")}
			},
		},
		{
			name: "base cycle", args: []string{"2"}, code: 1, stderr: "is neither fb nor an open PR",
			prs: func(t *testing.T) []*stackPR {
				t.Helper()
				return []*stackPR{green(t, 1, "b1", "b2"), green(t, 2, "b2", "b1")}
			},
		},
		{name: "bottom label fails", args: []string{"2"}, prs: stack, fail: "label 1", code: 1, stderr: "not marked queued"},
		{name: "top label fails", args: []string{"2"}, prs: stack, fail: "label 2", code: 1, stderr: "run land-stack again"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			var prs []*stackPR
			if tt.prs != nil {
				prs = tt.prs(t)
			}
			s := newStackGH(t, f, prs...)
			s.fail, s.raw = tt.fail, tt.raw
			if n, ok := strings.CutPrefix(tt.fail, "label "); ok {
				route := "POST /repos/o/r/issues/" + n + "/labels"
				f.hub.status[route] = http.StatusInternalServerError
				f.hub.on(route, "boom")
				s.fail = ""
			}
			f.ownerComments(40)
			rec := Record{Ticket: 40, Worktree: f.dir}
			if tt.rec != nil {
				rec = *tt.rec
			}
			if rec.Ticket != 0 {
				f.owner(t, rec)
			}
			code, _, stderr := f.agents(t, append([]string{"land-stack"}, tt.args...)...)
			if code != tt.code || !strings.Contains(stderr, tt.stderr) {
				t.Fatalf("%d %q", code, stderr)
			}
			if rec.Ticket == 40 && len(rec.Queued) == 0 && len(f.owned(t).Queued) > 0 {
				t.Fatal("a failed land marked the stack queued")
			}
		})
	}
}

func TestLandStack_settleFailures(t *testing.T) {
	t.Parallel()
	t.Run("the queue drafts cannot be read", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		newStackGH(t, f, green(t, 2, "b2", "fb"))
		delete(f.hub.routes, graphqlRoute)
		f.owner(t, Record{Ticket: 40, Queued: Queues{{Top: 2, PRs: []int{2}}}})
		if code, _, stderr := f.agents(t, "land-stack", "2"); code != 1 || !strings.Contains(stderr, "/graphql") {
			t.Fatalf("%d %q", code, stderr)
		}
	})
	t.Run("queued PR vanished", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		top := green(t, 2, "b2", "fb")
		top.State = "MERGED"
		newStackGH(t, f, top)
		f.owner(t, Record{Ticket: 40, Queued: Queues{{Top: 2, PRs: []int{1, 2}}}})
		if code, _, stderr := f.agents(t, "land-stack", "2"); code != 1 || !strings.Contains(stderr, "#1 is not a PR") {
			t.Fatalf("%d %q", code, stderr)
		}
	})
}

func TestLandStack_unwritableRecordFailsAfterQueueing(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	newStackGH(t, f, green(t, 5, "b5", "fb"))
	f.owner(t, Record{Ticket: 40})
	freeze(t, f.Env(t).recordPath(40))
	if code, _, stderr := f.agents(t, "land-stack", "5"); code != 1 || !strings.Contains(stderr, "write owner record") {
		t.Fatalf("%d %q", code, stderr)
	}
}

func TestPRRefs(t *testing.T) {
	t.Parallel()
	if got := prRefs([]int{1, 22, 3}); got != "#1 #22 #3" {
		t.Fatalf("prRefs = %q", got)
	}
}

func ejectedStack(t *testing.T, f *fixture) *stackGH {
	t.Helper()
	s := newStackGH(t, f, green(t, 1, "b1", "fb"), green(t, 2, "b2", "b1"), green(t, 3, "b3", "b2"))
	f.owner(t, Record{Ticket: 40, Worktree: f.dir, State: Done, Queued: Queues{{Top: 3, PRs: []int{1, 2, 3}}}})
	return s
}

func TestLandStack_relandsAnEjectedStackWholeInOneCall(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ejectedStack(t, f)
	code, stdout, stderr := f.agents(t, "land-stack", "3")
	if code != 0 ||
		stdout != "#3 left the Graphite merge queue; relanding its stack\nqueued #1 #2 #3\nfollow it: monacoctl agents watch (under Claude Code's Monitor tool)\nqueued together: #1 #2 #3\n" {
		t.Fatalf("%d %q %q", code, stdout, stderr)
	}
	want := []string{
		"POST /repos/o/r/issues/3/labels",
		"POST /repos/o/r/issues/2/labels",
		"POST /repos/o/r/issues/1/labels",
	}
	if got := f.hub.callsContaining("/labels"); !slices.Equal(got, want) {
		t.Fatalf("calls %v", got)
	}
	if q := f.owned(t).Queued; len(q) != 1 || q[0].Top != 3 || !slices.Equal(q[0].PRs, []int{1, 2, 3}) {
		t.Fatalf("queued %+v", q)
	}
}

func TestLandStack_relandsWhatIsLeftWhenOnePRLostTheLabel(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	s := ejectedStack(t, f)
	s.prs[1].State = "MERGED"
	s.prs[2].Base = "fb"
	labeled(s.prs[3], "merge-queue")
	code, stdout, stderr := f.agents(t, "land-stack", "3")
	if code != 0 ||
		stdout != "#3 left the Graphite merge queue; relanding its stack\nqueued #2 #3\nfollow it: monacoctl agents watch (under Claude Code's Monitor tool)\nqueued together: #2 #3\n" {
		t.Fatalf("%d %q %q", code, stdout, stderr)
	}
	want := []string{"POST /repos/o/r/issues/3/labels", "POST /repos/o/r/issues/2/labels"}
	if got := f.hub.callsContaining("/labels"); !slices.Equal(got, want) {
		t.Fatalf("calls %v", got)
	}
	if q := f.owned(t).Queued; len(q) != 1 || !slices.Equal(q[0].PRs, []int{2, 3}) {
		t.Fatalf("queued %+v", q)
	}
}

func TestLandStack_relandChecksEveryPRInTheStack(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		edit func(prs map[int]*stackPR)
		code int
		out  string
	}{
		{
			name: "a lower PR's stage 1 is red",
			edit: func(prs map[int]*stackPR) {
				prs[1].Commits.Nodes[0].Commit.StatusCheckRollup.Contexts.Nodes[0].Conclusion = "FAILURE"
			},
			out: "#3 left the Graphite merge queue; relanding its stack\nnot landing #3; waiting on #1 (stage 1 failure)\n",
		},
		{
			name: "a lower PR closed",
			edit: func(prs map[int]*stackPR) { prs[1].State = "CLOSED" },
			code: 1,
			out:  "#2's base b1 is neither fb nor an open PR",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			f.hub.on(get("/compare/fb...b1-oid"), `{"status":"diverged"}`)
			s := ejectedStack(t, f)
			tc.edit(s.prs)
			code, stdout, stderr := f.agents(t, "land-stack", "3")
			got := stdout
			if tc.code != 0 {
				got = stderr
			}
			if code != tc.code || !strings.Contains(got, tc.out) || len(s.lines()) != 0 {
				t.Fatalf("%d %q %q %v", code, stdout, stderr, s.lines())
			}
			if len(f.owned(t).Queued) > 0 {
				t.Fatal("the ejected stack kept its queued mark")
			}
		})
	}
}

func TestLandStack_readsTheStackFromGraphite(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, gtLog, fail, out string
	}{
		{
			name:  "gt names the lower PRs",
			gtLog: "◯  fb\n◯  b1\n◯  b2 (needs restack)\n◉  b3\n◯  b4\n",
			out:   "queued #1 #2 #3\nfollow it: monacoctl agents watch (under Claude Code's Monitor tool)\nqueued together: #1 #2 #3\n",
		},
		{
			name:  "gt lists the trunk, whose own PR targets main",
			gtLog: "◯  fb\n◯  b0\n◯  b1\n◉  b3\n",
			out:   "queued #1 #3\nfollow it: monacoctl agents watch (under Claude Code's Monitor tool)\nqueued together: #1 #3\n",
		},
		{name: "gt names only the top", gtLog: "◯  fb\n◉  b3\n", out: "queued #3\nfollow it: monacoctl agents watch (under Claude Code's Monitor tool)\nqueued together: #3\n"},
		{name: "gt is on another stack", gtLog: "◯  fb\n◯  b1\n◉  b7\n", out: "queued #3\nfollow it: monacoctl agents watch (under Claude Code's Monitor tool)\nqueued together: #3\n"},
		{
			name: "gt fails",
			fail: "gt log",
			out:  "gt log in /w/40 failed (gt log: boom); landing the GitHub base chain\nqueued #3\nfollow it: monacoctl agents watch (under Claude Code's Monitor tool)\nqueued together: #3\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			s := newStackGH(t, f,
				green(t, 1, "b1", "fb"), green(t, 2, "b2", "b1"), green(t, 3, "b3", "fb"), green(t, 7, "b7", "fb"),
				stackOf(t, 9, "fb", "main", "SUCCESS", ""),
			)
			s.gtLog, s.fail = tc.gtLog, tc.fail
			f.owner(t, Record{Ticket: 40, Worktree: f.dir, State: Done})
			want := strings.ReplaceAll(tc.out, "/w/40", f.dir)
			if code, stdout, stderr := f.agents(t, "land-stack", "3"); code != 0 || stdout != want {
				t.Fatalf("%d %q %q", code, stdout, stderr)
			}
		})
	}
}

func TestLandStack_aClosedEjectedTopIsUnmarkedAndRefused(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	s := ejectedStack(t, f)
	s.prs[3].State = "CLOSED"
	f.hub.on(get("/compare/fb...b3-oid"), `{"status":"diverged"}`)
	code, stdout, stderr := f.agents(t, "land-stack", "3")
	if code != 1 || stdout != "#3 left the Graphite merge queue; relanding its stack\n" ||
		!strings.Contains(stderr, "#3 is not an open PR") {
		t.Fatalf("%d %q %q", code, stdout, stderr)
	}
	if len(f.owned(t).Queued) > 0 {
		t.Fatal("kept the queued mark")
	}
}

func TestLandStack_anUnwritableRecordStopsTheReland(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	s := ejectedStack(t, f)
	freeze(t, f.Env(t).recordPath(40))
	code, _, stderr := f.agents(t, "land-stack", "3")
	if code != 1 || !strings.Contains(stderr, "write owner record") || len(s.lines()) != 0 {
		t.Fatalf("%d %q %v", code, stderr, s.lines())
	}
}

func TestWatch_clearsTheQueuedMarkOfAnEjectedStack(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	queued := labeled(green(t, 5, "b5", "fb"), "merge-queue")
	half := labeled(green(t, 6, "b6", "fb"), "merge-queue")
	merged := green(t, 8, "b8", "fb")
	merged.State = "MERGED"
	s := ejectedStack(t, f)
	labeled(s.prs[1], "merge-queue")
	labeled(s.prs[3], "merge-queue")
	for _, p := range []*stackPR{queued, half, merged} {
		s.prs[p.Number] = p
	}
	f.owner(t, Record{Ticket: 40, State: Exited, Queued: Queues{{Top: 3, PRs: []int{1, 2, 3}}}})
	f.record(t, Record{Ticket: 41, State: Exited, Queued: Queues{{Top: 5, PRs: []int{5}}}})
	f.record(t, Record{Ticket: 42, State: Exited, Queued: Queues{{Top: 6, PRs: []int{8, 6}}}})
	f.record(t, Record{Ticket: 43, State: Exited, Queued: Queues{{Top: 8, PRs: []int{8}}}})
	f.record(t, Record{Ticket: 44, State: Exited})
	f.noFailures()
	code, stdout, stderr := f.agents(t, "watch", "--once")
	want := "unqueued: #40; #3 left the Graphite merge queue. Fix the stack with gt modify and gt submit --stack --draft, then run land-stack 3\n"
	if code != 0 || stdout != want {
		t.Fatalf("%d %q %q", code, stdout, stderr)
	}
	env := f.Env(t)
	for ticket, queued := range map[int]bool{40: false, 41: true, 42: true, 43: true} {
		r, err := env.localRecord(ticket)
		if err != nil || (len(r.Queued) > 0) != queued {
			t.Fatalf("#%d queued %+v %v", ticket, r.Queued, err)
		}
	}
	if calls := s.lines(); len(calls) != 0 {
		t.Fatalf("watch ran %v", calls)
	}
	if s.prs[1].labeled("merge-queue") || s.prs[3].labeled("merge-queue") || !s.prs[5].labeled("merge-queue") {
		t.Fatalf("labels after the ejection: #1 %v, #3 %v, #5 %v", s.prs[1].Labels, s.prs[3].Labels, s.prs[5].Labels)
	}
	if r := f.owned(t).Settled; r == nil || r.Detail != "stack #3 ejected: #2 left the Graphite merge queue" {
		t.Fatalf("settled %+v", r)
	}
	if got := posted(t, f, "POST /repos/o/r/issues/40/comments"); !strings.Contains(got, `"queued":null`) {
		t.Fatalf("published %q", got)
	}
}

func TestWatch_unqueueFailures(t *testing.T) {
	t.Parallel()
	t.Run("the stack cannot be read", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		s := ejectedStack(t, f)
		s.fail = "gh api"
		if code, _, stderr := f.agents(t, "watch", "--once"); code != 1 || !strings.Contains(stderr, "gh api: boom") {
			t.Fatalf("%d %q", code, stderr)
		}
	})
	t.Run("the queue drafts cannot be read", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		ejectedStack(t, f)
		delete(f.hub.routes, graphqlRoute)
		if code, _, stderr := f.agents(t, "watch", "--once"); code != 1 || !strings.Contains(stderr, "/graphql") {
			t.Fatalf("%d %q", code, stderr)
		}
	})
	t.Run("Graphite keeps holding part of the stack", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		ejectedStack(t, f)
		f.hub.on(
			graphqlRoute,
			draftData([]string{queueDraftNode(90, "[Graphite MQ] Draft PR GROUP:x (PRs 1)", noRollup)}),
		)
		code, stdout, stderr := f.agents(t, "watch", "--once")
		const want = "stack #3 left queued: an open Graphite draft still tests #1\n"
		if code != 0 || stdout != want || stderr != "" || len(f.owned(t).Queued) == 0 ||
			slices.Contains(f.waited, dequeueEvery) {
			t.Fatalf("%d %q %q queued %+v waited %v", code, stdout, stderr, f.owned(t).Queued, f.waited)
		}
	})
	t.Run("the record cannot be written", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		ejectedStack(t, f)
		freeze(t, f.Env(t).recordPath(40))
		if code, _, stderr := f.agents(
			t,
			"watch",
			"--once",
		); code != 1 ||
			!strings.Contains(stderr, "write owner record") {
			t.Fatalf("%d %q", code, stderr)
		}
	})
}

func TestLandStack_aClosedPROutsideTheTrunkWithoutTheLabelIsEjected(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	s := ejectedStack(t, f)
	s.prs[1].State = "CLOSED"
	labeled(s.prs[2], "merge-queue")
	labeled(s.prs[3], "merge-queue")
	f.hub.on(get("/compare/fb...b1-oid"), `{"status":"diverged"}`)
	f.noFailures()
	code, stdout, stderr := f.agents(t, "watch", "--once")
	if code != 0 || !strings.HasPrefix(stdout, "unqueued: #40; #3 left the Graphite merge queue.") {
		t.Fatalf("%d %q %q", code, stdout, stderr)
	}
	if len(f.owned(t).Queued) > 0 {
		t.Fatal("kept the queued mark")
	}
}

func TestLandStack_aClosedPRInTheTrunkIsNotEjected(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	s := ejectedStack(t, f)
	s.prs[1].State = "CLOSED"
	labeled(s.prs[2], "merge-queue")
	labeled(s.prs[3], "merge-queue")
	f.hub.on(get("/compare/fb...b1-oid"), `{"status":"behind"}`)
	code, stdout, stderr := f.agents(t, "land-stack", "3")
	if code != 0 || stdout != "#3 is queued in the Graphite merge queue\n" || len(s.lines()) != 0 {
		t.Fatalf("%d %q %q %v", code, stdout, stderr, s.lines())
	}
	f.hub.on(get("/compare/fb...b1-oid"), `{"message":"boom"`)
	code, _, stderr = f.agents(t, "land-stack", "3")
	if code != 1 || !strings.Contains(stderr, "compare b1-oid with fb") {
		t.Fatalf("a failed compare: %d %q", code, stderr)
	}
}

func TestLandStack_aFailedCompareStopsSettleAndWatch(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	s := ejectedStack(t, f)
	s.prs[1].State = "CLOSED"
	labeled(s.prs[1], "merge-queue")
	labeled(s.prs[2], "merge-queue")
	labeled(s.prs[3], "merge-queue")
	f.hub.on(get("/compare/fb...b1-oid"), `{"message":"boom"`)
	code, _, stderr := f.agents(t, "land-stack", "3")
	if code != 1 || !strings.Contains(stderr, "compare b1-oid with fb") {
		t.Fatalf("settle on a failed compare: %d %q", code, stderr)
	}
	s.prs[1].Labels.Nodes = nil
	code, _, stderr = f.agents(t, "watch", "--once")
	if code != 1 || !strings.Contains(stderr, "compare b1-oid with fb") {
		t.Fatalf("watch on a failed compare: %d %q", code, stderr)
	}
}

func TestLandStack_aPRClosedByHandIsEjectedEvenWithTheLabel(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	s := ejectedStack(t, f)
	for _, n := range []int{1, 2, 3} {
		labeled(s.prs[n], "merge-queue")
	}
	s.prs[2].State = "CLOSED"
	f.hub.on(get("/compare/fb...b2-oid"), `{"status":"diverged"}`)
	f.noFailures()
	code, stdout, stderr := f.agents(t, "watch", "--once")
	if code != 0 || !strings.HasPrefix(stdout, "unqueued: #40; #3 left the Graphite merge queue.") {
		t.Fatalf("%d %q %q", code, stdout, stderr)
	}
	if len(f.owned(t).Queued) > 0 {
		t.Fatal("kept the queued mark")
	}
}

func TestWatchOnce_waitsAMinuteBeforeEjectingAStackWhoseLabelAPersonRemoved(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		ago     time.Duration
		ejected bool
	}{
		{"removed 30 seconds ago", 30 * time.Second, false},
		{"removed two minutes ago", 2 * time.Minute, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			s := ejectedStack(t, f)
			labeled(s.prs[1], "merge-queue")
			labeled(s.prs[3], "merge-queue")
			raw := fmt.Sprintf(
				`{"nodes":[{"__typename":"UnlabeledEvent","createdAt":%q,"label":{"name":"merge-queue"},`+
					`"actor":{"login":"logan"}},{"__typename":"UnlabeledEvent","createdAt":%q,"label":{"name":"large-pr"}}]}`,
				f.now.Add(-tc.ago).Format(time.RFC3339),
				f.now.Format(time.RFC3339),
			)
			if err := json.Unmarshal([]byte(raw), &s.prs[2].TimelineItems); err != nil {
				t.Fatal(err)
			}
			f.noFailures()
			code, stdout, stderr := f.agents(t, "watch", "--once")
			if code != 0 || strings.Contains(stdout, "unqueued: #40") != tc.ejected ||
				(len(f.owned(t).Queued) == 0) != tc.ejected {
				t.Fatalf("%d %q %q", code, stdout, stderr)
			}
		})
	}
}

func setUnlabels(t *testing.T, p *stackPR, events ...string) {
	t.Helper()
	p.Labels.Nodes = nil
	if err := json.Unmarshal([]byte(`{"nodes":[`+strings.Join(events, ",")+`]}`), &p.TimelineItems); err != nil {
		t.Fatal(err)
	}
}

func closedDraftOf(t *testing.T, n int, prs string, closed time.Time) string {
	t.Helper()
	node := closedDraft(n, noRollup)
	for _, swap := range [][2]string{
		{"(PRs 1, 2)", "(PRs " + prs + ")"},
		{"2026-09-27T11:59:00Z", closed.Format(time.RFC3339)},
	} {
		if !strings.Contains(node, swap[0]) {
			t.Fatalf("closedDraft no longer carries %q", swap[0])
		}
		node = strings.ReplaceAll(node, swap[0], swap[1])
	}
	return node
}

func closedDraftData(closed ...string) string {
	return `{"data":{"repository":{"pullRequests":{"nodes":[]},"drafts":{"nodes":[]},` +
		`"closed":{"nodes":[` + strings.Join(closed, ",") + `]}}}}`
}

func TestWatchOnce_holdsAStackGraphiteTookBeforeItIsEjected(t *testing.T) {
	t.Parallel()
	const graphiteGraphQL = "graphite-app"
	type removal struct {
		label, by string
		ago       time.Duration
	}
	queueLabel := func(by string, ago time.Duration) removal { return removal{"merge-queue", by, ago} }
	for _, tc := range []struct {
		name        string
		removals    []removal
		untouched   []int
		draftClosed time.Duration
		draftPRs    string
		ejected     bool
	}{
		{name: "Graphite took the label two minutes ago", removals: []removal{queueLabel(graphiteGraphQL, 2*time.Minute)}},
		{name: "Graphite's REST login", removals: []removal{queueLabel(graphiteBot, 2*time.Minute)}},
		{
			name: "a later removal of another label does not change who took it",
			removals: []removal{
				queueLabel(graphiteGraphQL, 2*time.Minute), {"large-pr", "logan", time.Minute},
			},
		},
		{name: "Graphite took it just inside the hold", removals: []removal{queueLabel(graphiteGraphQL, takenFor-time.Second)}},
		{
			name: "Graphite took it a whole hold ago", ejected: true,
			removals: []removal{queueLabel(graphiteGraphQL, takenFor)},
		},
		{
			name: "Graphite took two PRs and has not drafted them while the third is ejected", ejected: true,
			removals: []removal{queueLabel(graphiteGraphQL, 2*time.Minute)}, untouched: []int{2},
		},
		{
			name: "a person removed it two minutes ago", ejected: true,
			removals: []removal{queueLabel("logan", 2*time.Minute)},
		},
		{
			name:     "a person removed it after Graphite took it earlier, so the person's removal counts",
			removals: []removal{queueLabel(graphiteGraphQL, 5*time.Minute), queueLabel("logan", 2*time.Minute)},
			ejected:  true,
		},
		{
			name:     "Graphite took it after a person removed it earlier, so the take counts",
			removals: []removal{queueLabel("logan", 3*time.Hour), queueLabel(graphiteGraphQL, 2*time.Minute)},
		},
		{
			name:     "a draft that ran the PRs since closed",
			removals: []removal{queueLabel(graphiteGraphQL, 2*time.Minute)}, draftClosed: time.Minute, draftPRs: "1, 2, 3",
			ejected: true,
		},
		{
			name:     "the closed draft ran before Graphite took it",
			removals: []removal{queueLabel(graphiteGraphQL, 2*time.Minute)}, draftClosed: 3 * time.Minute, draftPRs: "1, 2",
		},
		{
			name:     "a closed draft that ran other PRs does not end the hold",
			removals: []removal{queueLabel(graphiteGraphQL, 2*time.Minute)}, draftClosed: time.Minute, draftPRs: "8, 9",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			s := ejectedStack(t, f)
			events := make([]string, len(tc.removals))
			for i, r := range tc.removals {
				events[i] = unlabel(f.now.Add(-r.ago), r.label, r.by)
			}
			for _, n := range []int{1, 2, 3} {
				if !slices.Contains(tc.untouched, n) {
					setUnlabels(t, s.prs[n], events...)
				}
			}
			if tc.draftClosed != 0 {
				f.hub.on(graphqlRoute, closedDraftData(closedDraftOf(t, 90, tc.draftPRs, f.now.Add(-tc.draftClosed))))
			} else {
				f.noFailures()
			}
			code, stdout, stderr := f.agents(t, "watch", "--once")
			if code != 0 || strings.Contains(stdout, "unqueued: #40") != tc.ejected ||
				(len(f.owned(t).Queued) == 0) != tc.ejected {
				t.Fatalf("%d %q %q", code, stdout, stderr)
			}
			if released := len(f.hub.callsContaining("/labels")) != 0; released != tc.ejected {
				t.Fatalf("labels released = %v, want %v", released, tc.ejected)
			}
		})
	}
}

func TestWatchOnce_releasesTheEjectedPROfAStackGraphiteHoldsInPart(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	s := ejectedStack(t, f)
	took := unlabel(f.now.Add(-2*time.Minute), "merge-queue", graphiteApp)
	setUnlabels(t, s.prs[1], took)
	setUnlabels(t, s.prs[3], took)
	s.prs[2].State = "CLOSED"
	labeled(s.prs[2], "merge-queue")
	f.hub.on(get("/compare/fb...b2-oid"), `{"status":"diverged"}`)
	f.noFailures()
	code, stdout, stderr := f.agents(t, "watch", "--once")
	if code != 0 || !strings.HasPrefix(stdout, "unqueued: #40; #3 left the Graphite merge queue.") {
		t.Fatalf("%d %q %q", code, stdout, stderr)
	}
	if s.prs[2].labeled("merge-queue") || len(f.owned(t).Queued) > 0 {
		t.Fatalf("the ejected PR kept its label %v, or the stack kept its mark %+v", s.prs[2].Labels, f.owned(t).Queued)
	}
}

func TestWatchOnce_removesTheLabelsOfAStackAnOpenDraftStillTests(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	s := ejectedStack(t, f)
	labeled(s.prs[1], "merge-queue")
	labeled(s.prs[3], "merge-queue")
	f.hub.on(
		graphqlRoute,
		draftData([]string{queueDraftNode(90, "[Graphite MQ] Draft PR GROUP:x (PRs 1, 3)", noRollup)}),
	)
	code, stdout, stderr := f.agents(t, "watch", "--once")
	const want = "stack #3 left queued: an open Graphite draft still tests #1\n"
	if code != 0 || stdout != want || stderr != "" {
		t.Fatalf("%d %q %q", code, stdout, stderr)
	}
	if r := f.owned(t); len(r.Queued) == 0 || r.Settled != nil || len(f.waited) != 0 {
		t.Fatalf("queued %+v settled %+v waited %v", r.Queued, r.Settled, f.waited)
	}
	released := []string{
		"DELETE /repos/o/r/issues/1/labels/merge-queue",
		"DELETE /repos/o/r/issues/3/labels/merge-queue",
	}
	if got := f.hub.callsContaining("/labels"); !slices.Equal(got, released) {
		t.Fatalf("label calls %v, want %v", got, released)
	}
}

func TestWatchOnce_reportsALabelItCannotRemoveFromAStackAnOpenDraftStillTests(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	s := ejectedStack(t, f)
	labeled(s.prs[1], "merge-queue")
	route := "DELETE /repos/o/r/issues/1/labels/merge-queue"
	f.hub.status[route] = http.StatusInternalServerError
	f.hub.on(route, "boom")
	f.hub.on(
		graphqlRoute,
		draftData([]string{queueDraftNode(90, "[Graphite MQ] Draft PR GROUP:x (PRs 1)", noRollup)}),
	)
	code, stdout, stderr := f.agents(t, "watch", "--once")
	if code != 1 || !strings.Contains(stderr, "boom") || strings.Contains(stdout, "left queued") ||
		len(f.owned(t).Queued) == 0 {
		t.Fatalf("%d %q %q queued %+v", code, stdout, stderr, f.owned(t).Queued)
	}
}

func TestLandStack_doesNotRelabelAStackGraphiteTookWhileItWaitsForADraft(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	s := ejectedStack(t, f)
	for _, n := range []int{1, 2, 3} {
		setUnlabels(t, s.prs[n], unlabel(f.now.Add(-2*time.Minute), "merge-queue", "graphite-app"))
	}
	code, stdout, stderr := f.agents(t, "land-stack", "3")
	if code != 0 || stdout != "#3 is queued in the Graphite merge queue\n" || len(f.owned(t).Queued) == 0 {
		t.Fatalf("%d %q %q queued %+v", code, stdout, stderr, f.owned(t).Queued)
	}
	if got := f.hub.callsContaining("/labels"); len(got) != 0 {
		t.Fatalf("land-stack relabelled a stack Graphite holds: %v", got)
	}
}

func TestLandStack_queuesAStackWhosePRFormatIsRedOrPending(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	newStackGH(t, f,
		withFormat(green(t, 1, "b1", "fb"), "FAILURE"),
		withFormat(green(t, 2, "b2", "b1"), ""),
	)
	f.owner(t, Record{Ticket: 40, Worktree: f.dir, State: Done})
	code, stdout, stderr := f.agents(t, "land-stack", "2")
	if code != 0 ||
		stdout != "queued #1 #2\nfollow it: monacoctl agents watch (under Claude Code's Monitor tool)\nqueued together: #1 #2\n" {
		t.Fatalf("%d %q %q", code, stdout, stderr)
	}
	want := []string{"POST /repos/o/r/issues/2/labels", "POST /repos/o/r/issues/1/labels"}
	if got := f.hub.callsContaining("/labels"); !slices.Equal(got, want) {
		t.Fatalf("labels %v", got)
	}
}

func armedStack(t *testing.T, f *fixture) *stackGH {
	t.Helper()
	s := newStackGH(t, f, green(t, 1, "b1", "fb"), stackOf(t, 2, "b2", "b1", "pending", "SUCCESS"))
	f.owner(t, Record{Ticket: 40, State: Exited, Worktree: f.dir})
	return s
}

func TestLandStack_armsAStackWhoseStage1IsPendingAndLabelsNothing(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	s := armedStack(t, f)
	code, stdout, stderr := f.agents(t, "land-stack", "2")
	want := "armed #2; agents watch lands it once stage 1 passes (waiting on #2 (stage 1 pending))\n"
	if code != 0 || stdout != want || stderr != "" {
		t.Fatalf("%d %q %q", code, stdout, stderr)
	}
	if got := f.hub.callsContaining("/labels"); len(got) != 0 {
		t.Fatalf("arming labelled %v", got)
	}
	r := f.owned(t)
	if len(r.Queued) > 0 || len(r.Armed) == 0 || r.Armed[0].Top != 2 || !slices.Equal(r.Armed[0].PRs, []int{1, 2}) ||
		!r.Armed[0].At.Equal(f.now) {
		t.Fatalf("queued %+v armed %+v", r.Queued, r.Armed)
	}
	got := posted(t, f, "POST /repos/o/r/issues/40/comments")
	if !strings.Contains(got, `"armed":{"top":2,"prs":[1,2]`) {
		t.Fatalf("published %q", got)
	}
	*s.prs[2] = *green(t, 2, "b2", "b1")
	if code, stdout, _ := f.agents(t, "land-stack", "2"); code != 0 || !strings.HasPrefix(stdout, "queued #1 #2\n") {
		t.Fatalf("%d %q", code, stdout)
	}
	if r := f.owned(t); len(r.Armed) > 0 || len(r.Queued) == 0 {
		t.Fatalf("queued %+v armed %+v", r.Queued, r.Armed)
	}
}

func TestLandStack_doesNotArmAStackWithAFailedCheck(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	newStackGH(t, f, stackOf(t, 1, "b1", "fb", "FAILURE", "SUCCESS"), stackOf(t, 2, "b2", "b1", "pending", "SUCCESS"))
	f.owner(t, Record{Ticket: 40, State: Exited, Worktree: f.dir})
	code, stdout, _ := f.agents(t, "land-stack", "2")
	if code != 0 || stdout != "not landing #2; waiting on #1 (stage 1 failure), #2 (stage 1 pending)\n" {
		t.Fatalf("%d %q", code, stdout)
	}
	if len(f.owned(t).Armed) > 0 {
		t.Fatal("armed a stack with a failed check")
	}
}

func TestWatchOnce_landsAnArmedStackThatWentGreen(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	s := armedStack(t, f)
	if code, _, _ := f.agents(t, "land-stack", "2"); code != 0 {
		t.Fatal(code)
	}
	*s.prs[2] = *green(t, 2, "b2", "b1")
	f.noFailures()
	code, stdout, stderr := f.agents(t, "watch", "--once")
	if code != 0 || !strings.Contains(stdout, "armed stack #2 landing\nqueued #1 #2\n") {
		t.Fatalf("%d %q %q", code, stdout, stderr)
	}
	if r := f.owned(t); len(r.Armed) > 0 || len(r.Queued) == 0 || r.Queued[0].Top != 2 {
		t.Fatalf("queued %+v armed %+v", r.Queued, r.Armed)
	}
}

func requeuedDuringRelease(t *testing.T, at int, relabel bool) (*fixture, *stackGH, Queue) {
	t.Helper()
	f := newFixture(t)
	s := queuedStack(t, f, f.dir)
	s.prs[2].Labels.Nodes = nil
	again := Queue{Top: 2, PRs: []int{1, 2}, At: f.now.Add(time.Hour)}
	f.onWait = func(n int) {
		if n != at {
			return
		}
		f.owner(t, Record{Ticket: 40, State: Exited, Worktree: f.dir, Queued: Queues{again}})
		if relabel {
			labeled(s.prs[1], "merge-queue")
			labeled(s.prs[2], "merge-queue")
		}
	}
	return f, s, again
}

func TestEjectStack_leavesAStackQueuedWhenLandStackQueuesItAgainDuringTheRelease(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		at      int
		relabel bool
	}{
		{"between release passes", 1, true},
		{"after the last pass, before it concludes", dequeueChecks, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f, s, again := requeuedDuringRelease(t, tc.at, tc.relabel)
			prs := []stackPR{*s.prs[1], *s.prs[2]}
			line, requeued, err := f.Env(t).ejectStack(t.Context(), f.owned(t), f.owned(t).Queued[0], prs[1], prs, nil)
			if err != nil || !requeued || line != "stack #2 was re-queued during its release; left it queued" {
				t.Fatalf("ejectStack = %q, %v, %v", line, requeued, err)
			}
			r := f.owned(t)
			if len(r.Queued) == 0 || !r.Queued[0].At.Equal(again.At) || r.Settled != nil {
				t.Fatalf("record queued %+v settled %+v, want the new queue kept", r.Queued, r.Settled)
			}
			if tc.relabel && (!s.prs[1].labeled("merge-queue") || !s.prs[2].labeled("merge-queue")) {
				t.Fatalf("the release removed the labels the new land-stack added: %v %v",
					s.prs[1].Labels, s.prs[2].Labels)
			}
		})
	}
}

func TestEjectStack_reportsAnOwnerRecordItCannotReread(t *testing.T) {
	t.Parallel()
	for _, at := range []int{0, dequeueChecks} {
		f := newFixture(t)
		s := queuedStack(t, f, f.dir)
		s.prs[2].Labels.Nodes = nil
		env := f.Env(t)
		rec := f.owned(t)
		gone := func() {
			if err := os.Remove(env.recordPath(40)); err != nil {
				t.Fatal(err)
			}
		}
		if at == 0 {
			gone()
		} else {
			f.onWait = func(n int) {
				if n == at {
					gone()
				}
			}
		}
		prs := []stackPR{*s.prs[1], *s.prs[2]}
		if _, _, err := env.ejectStack(t.Context(), rec, rec.Queued[0], prs[1], prs, nil); err == nil {
			t.Fatalf("record removed at wait %d: ejectStack returned no error", at)
		}
	}
}

func TestWatchOnce_leavesAStackQueuedAgainDuringItsReleaseAlone(t *testing.T) {
	t.Parallel()
	f, s, again := requeuedDuringRelease(t, 1, true)
	f.noFailures()
	code, stdout, stderr := f.agents(t, "watch", "--once")
	if code != 0 || !strings.Contains(stdout, "stack #2 was re-queued during its release; left it queued\n") ||
		strings.Contains(stdout, "unqueued:") {
		t.Fatalf("%d %q %q", code, stdout, stderr)
	}
	if r := f.owned(t); len(r.Queued) == 0 || !r.Queued[0].At.Equal(again.At) {
		t.Fatalf("queued %+v, want the new queue kept", r.Queued)
	}
	if !s.prs[2].labeled("merge-queue") {
		t.Fatal("the release removed the label the new land-stack added")
	}
}

func TestLandStack_namesAConflictAnywhereInTheStackInsteadOfStage1Missing(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	newStackGH(t, f,
		conflicted(t, stackOf(t, 1, "b1", "fb", "", "")),
		conflicted(t, stackOf(t, 2, "b2", "b1", "SUCCESS", "SUCCESS")),
		stackOf(t, 3, "b3", "b2", "", ""),
	)
	f.owner(t, Record{Ticket: 40, Worktree: f.dir, State: Exited})
	code, stdout, stderr := f.agents(t, "land-stack", "3")
	want := "not landing #3; waiting on " +
		"#1 conflicts with fb; GitHub runs no CI until it is resolved: restack with gt and resubmit, " +
		"#2 conflicts with b1; GitHub runs no CI until it is resolved: restack with gt and resubmit, " +
		"#3 (stage 1 missing)\n"
	if code != 0 || stdout != want || stderr != "" {
		t.Fatalf("%d %q %q", code, stdout, stderr)
	}
	if r := f.owned(t); len(r.Armed) > 0 || len(r.Queued) > 0 {
		t.Fatalf("queued %+v armed %+v", r.Queued, r.Armed)
	}
}

func conflicted(t *testing.T, p *stackPR) *stackPR {
	t.Helper()
	if err := json.Unmarshal([]byte(`{"mergeable":"CONFLICTING"}`), p); err != nil {
		t.Fatal(err)
	}
	return p
}

func independentStacks(t *testing.T, f *fixture, rec Record) *stackGH {
	t.Helper()
	s := newStackGH(t, f,
		labeled(green(t, 1, "b1", "fb"), "merge-queue"), labeled(green(t, 2, "b2", "b1"), "merge-queue"),
		stackOf(t, 3, "b3", "fb", "pending", "SUCCESS"))
	rec.Ticket, rec.State, rec.Worktree = 40, Exited, f.dir
	f.owner(t, rec)
	f.noFailures()
	return s
}

func TestLandStack_armsAndQueuesASecondStackWhileTheFirstIsQueued(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	s := independentStacks(t, f, Record{Queued: Queues{{Top: 2, PRs: []int{1, 2}, At: f.now}}})
	code, stdout, stderr := f.agents(t, "land-stack", "3")
	if code != 0 || !strings.HasPrefix(stdout, "armed #3;") {
		t.Fatalf("%d %q %q", code, stdout, stderr)
	}
	if r := f.owned(t); len(r.Queued) != 1 || r.Queued[0].Top != 2 || len(r.Armed) != 1 || r.Armed[0].Top != 3 {
		t.Fatalf("queued %+v armed %+v", r.Queued, r.Armed)
	}
	*s.prs[3] = *green(t, 3, "b3", "fb")
	code, stdout, stderr = f.agents(t, "watch", "--once")
	if code != 0 || !strings.Contains(stdout, "armed stack #3 landing\nqueued #3\n") {
		t.Fatalf("%d %q %q", code, stdout, stderr)
	}
	r := f.owned(t)
	if len(r.Armed) != 0 || len(r.Queued) != 2 || r.Queued.find(2) == nil || r.Queued.find(3) == nil {
		t.Fatalf("queued %+v armed %+v", r.Queued, r.Armed)
	}
}

func TestLandStack_settlingOneQueuedStackLeavesTheOtherQueued(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	s := independentStacks(t, f, Record{Queued: Queues{
		{Top: 2, PRs: []int{1, 2}, At: f.now}, {Top: 3, PRs: []int{3}, At: f.now},
	}})
	labeled(s.prs[3], "merge-queue")
	for _, n := range []int{1, 2} {
		s.prs[n].State = "MERGED"
	}
	s.prs[2].MergeCommit.OID = "abcdef0123456789abcdef0123456789abcdef01"
	code, stdout, stderr := f.agents(t, "land-stack", "2")
	if code != 0 || stdout != "#2 merged as abcdef0\n" {
		t.Fatalf("%d %q %q", code, stdout, stderr)
	}
	r := f.owned(t)
	if len(r.Queued) != 1 || r.Queued[0].Top != 3 || r.Settled == nil || r.Settled.Top != 2 {
		t.Fatalf("queued %+v settled %+v", r.Queued, r.Settled)
	}
}

func TestLandStack_refusesAStackWhoseTopIsInsideAnotherQueuedStack(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	independentStacks(t, f, Record{Queued: Queues{{Top: 2, PRs: []int{1, 2}, At: f.now}}})
	code, _, stderr := f.agents(t, "land-stack", "1")
	if code != 1 || !strings.Contains(stderr, "#40 already has #2 queued") {
		t.Fatalf("%d %q", code, stderr)
	}
}

func TestWatch_warnsAndHoldsNothingForStacksAnOlderWriterLeftStale(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	s := newStackGH(t, f, green(t, 1, "b1", "fb"), green(t, 2, "b2", "b1"))
	f.noFailures()
	f.owner(t, Record{Ticket: 40, State: Exited, Worktree: f.dir})
	writeFile(t, f.Env(t).recordPath(40), `{"ticket":40,"state":"exited","worktree":"`+f.dir+`","armed":null,`+
		`"arms":[{"top":2,"prs":[1,2],"at":"2026-09-27T12:00:00Z"},{"top":7,"prs":[7],"at":"2026-09-27T12:00:00Z"}]}`)
	code, stdout, stderr := f.agents(t, "watch", "--once")
	want := "#40 record was rewritten by an older monacoctl; queued/armed lists may be stale; " +
		"rerun land-stack for #2 #7 with a current monacoctl"
	if code != 0 || !strings.Contains(stdout, want) || strings.Contains(stdout, "landing") {
		t.Fatalf("%d %q %q", code, stdout, stderr)
	}
	if calls := f.hub.callsContaining("/labels"); len(calls) != 0 || s.prs[2].labeled("merge-queue") {
		t.Fatalf("labels %v", calls)
	}
}

func TestWatchStream_warnsOnceAboutQueuesAnOlderWriterLeftStale(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	queuedStack(t, f, f.dir)
	writeFile(t, f.Env(t).recordPath(40), `{"ticket":40,"state":"exited","worktree":"`+f.dir+`","queued":null,`+
		`"queues":[{"top":2,"prs":[1,2],"at":"2026-09-27T12:00:00Z"}]}`)
	got := streamRounds(t, f, 2, func(int) {})
	line := "#40 record was rewritten by an older monacoctl; queued/armed lists may be stale; " +
		"rerun land-stack for #2 with a current monacoctl\n"
	if strings.Count(got, line) != 1 || strings.Contains(got, "#1 queued") {
		t.Fatalf("stream %q", got)
	}
}
