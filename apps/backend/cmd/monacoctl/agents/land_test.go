package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
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
	if name == "git" && !s.repo && (args[0] == "fetch" || args[0] == "merge-tree") {
		s.mu.Lock()
		defer s.mu.Unlock()
		return nil, s.git[args[0]]
	}
	if name != "gh" && name != "gt" {
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
		stackOf(t, 1, "b1", "fb", "SUCCESS", "FAILURE"),
		stackOf(t, 2, "b2", "b1", "SUCCESS", ""),
		stackOf(t, 3, "b3", "b2", "pending", "SUCCESS"),
		stackOf(t, 4, "b4", "b3", "", "PENDING"),
		withFormat(stackOf(t, 5, "b5", "b4", "SUCCESS", "SUCCESS"), ""),
		withFormat(stackOf(t, 6, "b6", "b5", "SUCCESS", "SUCCESS"), "FAILURE"),
		withFormat(stackOf(t, 7, "b7", "b6", "SUCCESS", "SUCCESS"), "SUCCESS"),
	)
	f.owner(t, Record{Ticket: 40, Worktree: "/w/40", State: Done})
	code, stdout, stderr := f.agents(t, "land-stack", "7")
	want := "not landing #7; waiting on #1 (verify failure), #2 (verify missing), #3 (stage 1 pending), " +
		"#4 (stage 1 missing, verify pending), #5 (PR format pending), #6 (PR format failure)\n"
	if code != 0 || stdout != want || stderr != "" {
		t.Fatalf("%d %q %q", code, stdout, stderr)
	}
	if calls := s.lines(); len(calls) != 0 {
		t.Fatalf("a refusal ran %v", calls)
	}
	if f.owned(t).Queued != nil {
		t.Fatal("a refusal marked the stack queued")
	}
}

func TestLandStack_labelsEveryPRBottomToTopAndKeepsTheirBases(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	s := newStackGH(t, f,
		green(t, 1, "b1", "fb"), green(t, 2, "b2", "b1"), green(t, 3, "b3", "b2"),
		green(t, 7, "other", "fb"), green(t, 8, "above-other", "other"),
	)
	f.owner(t, Record{Ticket: 40, Worktree: "/w/40", State: Done})
	code, stdout, stderr := f.agents(t, "land-stack", "3")
	if code != 0 ||
		stdout != "queued #1 #2 #3\nfollow it: monacoctl agents watch (under Claude Code's Monitor tool)\nqueued together: #1 #2 #3\n" {
		t.Fatalf("%d %q %q", code, stdout, stderr)
	}
	want := []string{
		"POST /repos/o/r/issues/1/labels",
		"POST /repos/o/r/issues/2/labels",
		"POST /repos/o/r/issues/3/labels",
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
	if q := f.owned(t).Queued; q == nil || q.Top != 3 || !slices.Equal(q.PRs, []int{1, 2, 3}) {
		t.Fatalf("queued %+v", q)
	}
	if got := posted(t, f, "POST /repos/o/r/issues/40/comments"); !strings.Contains(
		got, `"queued":{"top":3,"prs":[1,2,3]}`,
	) {
		t.Fatalf("published %q", got)
	}
	code, stdout, _ = f.agents(t, "land-stack", "3")
	if code != 0 || stdout != "#3 is queued in the Graphite merge queue\n" ||
		len(f.hub.callsContaining("/labels")) != len(want) {
		t.Fatalf("second call: %d %q %v", code, stdout, f.hub.callsContaining("/labels"))
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
			"not landing #2; waiting on #1 (stage 1 pending)\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			paged := pagedStack(t, 1, "b1", "fb", ciOK("FAILURE", 1), ciOK("SUCCESS", 2))
			s := newStackGH(t, f, paged, green(t, 2, "b2", "b1"))
			s.pages = map[string]string{"b1": tc.rest}
			f.owner(t, Record{Ticket: 40, Worktree: "/w/40", State: Done})
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
			f.owner(t, Record{Ticket: 40, Worktree: "/w/40", State: Done})
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
	f.owner(t, Record{Ticket: 40, Worktree: "/w/40"})
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
	f.owner(t, Record{Ticket: 40, Worktree: "/w/40"})
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
	want := []string{"POST /repos/o/r/issues/1/labels", "POST /repos/o/r/issues/2/labels"}
	if got := f.hub.callsContaining("/labels"); !slices.Equal(got, want) || len(f.waited) != 0 {
		t.Fatalf("labels %v, waited %v", got, f.waited)
	}
	if got := strings.TrimSpace(gitOut(t, m.worktree, "rev-parse", "origin/fb")); got != m.tip {
		t.Fatalf("checked against origin/fb %s, want the moved tip %s", got, m.tip)
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
	if got := f.hub.callsContaining("/labels"); len(got) != 0 || f.owned(t).Queued != nil {
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
			f.owner(t, Record{Ticket: 40, Worktree: "/w/40", State: Done})
			code, stdout, stderr := f.agents(t, "land-stack", "2")
			if code != 1 || stdout != "" || !strings.Contains(stderr, step+": boom") ||
				!strings.Contains(stderr, "the stack is not marked queued") {
				t.Fatalf("%d %q %q", code, stdout, stderr)
			}
			if got := f.hub.callsContaining("/labels"); len(got) != 0 || f.owned(t).Queued != nil {
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
		gone       bool
	}{
		{
			name: "every PR merged",
			edit: func(prs map[int]*stackPR) {
				prs[2].State, prs[3].State, prs[3].MergeCommit.OID = "MERGED", "MERGED", oid
			},
			stdout: "#3 merged as abcdef0; gt sync ran in WT\n",
			calls:  []string{"gt sync --no-interactive --delete-all --no-restack"},
		},
		{
			name: "every PR merged, in a clone without the worktree",
			edit: func(prs map[int]*stackPR) {
				prs[2].State, prs[3].State, prs[3].MergeCommit.OID = "MERGED", "MERGED", oid
			},
			stdout: "#3 merged as abcdef0; no worktree at WT, skipped gt sync\n",
			gone:   true,
		},
		{
			name:   "every PR closed by the Graphite fast-forward",
			edit:   func(prs map[int]*stackPR) { prs[2].State, prs[3].State = "CLOSED", "CLOSED" },
			stdout: "#3 merged as b3-oid; gt sync ran in WT\n",
			calls:  []string{"gt sync --no-interactive --delete-all --no-restack"},
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
			if tt.gone {
				wt = filepath.Join(wt, "gone")
			}
			f.owner(t, Record{Ticket: 40, Worktree: wt, Queued: &Queue{Top: 3, PRs: []int{1, 2, 3}}})
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
			if queued := f.owned(t).Queued != nil; queued != tt.stillQueue {
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
			rec: &Record{Ticket: 40, Queued: &Queue{Top: 7, PRs: []int{7}}},
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
			rec := Record{Ticket: 40, Worktree: "/w/40"}
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
			if rec.Ticket == 40 && rec.Queued == nil && f.owned(t).Queued != nil {
				t.Fatal("a failed land marked the stack queued")
			}
		})
	}
}

func TestLandStack_settleFailures(t *testing.T) {
	t.Parallel()
	t.Run("gt sync", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		bottom, top := green(t, 1, "b1", "fb"), green(t, 2, "b2", "fb")
		bottom.State, top.State = "MERGED", "MERGED"
		s := newStackGH(t, f, bottom, top)
		s.fail = "gt sync"
		wt := t.TempDir()
		f.owner(t, Record{Ticket: 40, Worktree: wt, Queued: &Queue{Top: 2, PRs: []int{1, 2}}})
		code, stdout, stderr := f.agents(t, "land-stack", "2")
		if want := "#2 merged as b2-oid; gt sync failed in " + wt + ": "; code != 0 || !strings.Contains(stdout, want) {
			t.Fatalf("%d %q %q", code, stdout, stderr)
		}
		if f.owned(t).Queued != nil {
			t.Fatal("a failed gt sync kept the queued mark of a landed stack")
		}
	})
	t.Run("the queue drafts cannot be read", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		newStackGH(t, f, green(t, 2, "b2", "fb"))
		delete(f.hub.routes, graphqlRoute)
		f.owner(t, Record{Ticket: 40, Queued: &Queue{Top: 2, PRs: []int{2}}})
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
		f.owner(t, Record{Ticket: 40, Queued: &Queue{Top: 2, PRs: []int{1, 2}}})
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
	f.owner(t, Record{Ticket: 40, Worktree: "/w/40", State: Done, Queued: &Queue{Top: 3, PRs: []int{1, 2, 3}}})
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
		"POST /repos/o/r/issues/1/labels",
		"POST /repos/o/r/issues/2/labels",
		"POST /repos/o/r/issues/3/labels",
	}
	if got := f.hub.callsContaining("/labels"); !slices.Equal(got, want) {
		t.Fatalf("calls %v", got)
	}
	if q := f.owned(t).Queued; q == nil || q.Top != 3 || !slices.Equal(q.PRs, []int{1, 2, 3}) {
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
	want := []string{"POST /repos/o/r/issues/2/labels", "POST /repos/o/r/issues/3/labels"}
	if got := f.hub.callsContaining("/labels"); !slices.Equal(got, want) {
		t.Fatalf("calls %v", got)
	}
	if q := f.owned(t).Queued; q == nil || !slices.Equal(q.PRs, []int{2, 3}) {
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
			name: "a lower PR's verify is red",
			edit: func(prs map[int]*stackPR) {
				prs[2].Commits.Nodes[0].Commit.StatusCheckRollup.Contexts.Nodes[1].State = "FAILURE"
			},
			out: "#3 left the Graphite merge queue; relanding its stack\nnot landing #3; waiting on #2 (verify failure)\n",
		},
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
			if f.owned(t).Queued != nil {
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
			f.owner(t, Record{Ticket: 40, Worktree: "/w/40", State: Done})
			if code, stdout, stderr := f.agents(t, "land-stack", "3"); code != 0 || stdout != tc.out {
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
	if f.owned(t).Queued != nil {
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
	f.owner(t, Record{Ticket: 40, State: Exited, Queued: &Queue{Top: 3, PRs: []int{1, 2, 3}}})
	f.record(t, Record{Ticket: 41, State: Exited, Queued: &Queue{Top: 5, PRs: []int{5}}})
	f.record(t, Record{Ticket: 42, State: Exited, Queued: &Queue{Top: 6, PRs: []int{8, 6}}})
	f.record(t, Record{Ticket: 43, State: Exited, Queued: &Queue{Top: 8, PRs: []int{8}}})
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
		if err != nil || (r.Queued != nil) != queued {
			t.Fatalf("#%d queued %+v %v", ticket, r.Queued, err)
		}
	}
	if calls := s.lines(); len(calls) != 0 {
		t.Fatalf("watch ran %v", calls)
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
	if f.owned(t).Queued != nil {
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
	if f.owned(t).Queued != nil {
		t.Fatal("kept the queued mark")
	}
}

func TestWatchOnce_waitsAMinuteBeforeEjectingAStackGraphiteJustUnlabeled(t *testing.T) {
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
				`{"nodes":[{"__typename":"UnlabeledEvent","createdAt":%q,"label":{"name":"merge-queue"}},`+
					`{"__typename":"UnlabeledEvent","createdAt":%q,"label":{"name":"large-pr"}}]}`,
				f.now.Add(-tc.ago).Format(time.RFC3339),
				f.now.Format(time.RFC3339),
			)
			if err := json.Unmarshal([]byte(raw), &s.prs[2].TimelineItems); err != nil {
				t.Fatal(err)
			}
			f.noFailures()
			code, stdout, stderr := f.agents(t, "watch", "--once")
			if code != 0 || strings.Contains(stdout, "unqueued: #40") != tc.ejected ||
				(f.owned(t).Queued == nil) != tc.ejected {
				t.Fatalf("%d %q %q", code, stdout, stderr)
			}
		})
	}
}

func TestLandStack_aSkippedPRFormatRunAfterASuccessDoesNotBlockARelanding(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	s := newStackGH(t, f,
		withFormat(withFormat(green(t, 1, "b1", "fb"), "SUCCESS"), "SKIPPED"),
		withFormat(withFormat(green(t, 2, "b2", "b1"), "CANCELLED"), "SKIPPED"),
	)
	f.owner(t, Record{Ticket: 40, Worktree: "/w/40", State: Done})
	code, stdout, stderr := f.agents(t, "land-stack", "2")
	if code != 0 || stdout != "not landing #2; waiting on #2 (PR format skipped)\n" || stderr != "" {
		t.Fatalf("%d %q %q", code, stdout, stderr)
	}
	if calls := s.lines(); len(calls) != 0 {
		t.Fatalf("a refusal ran %v", calls)
	}
}
