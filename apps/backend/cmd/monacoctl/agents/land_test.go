package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
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
}

func newStackGH(t *testing.T, f *fixture, prs ...*stackPR) *stackGH {
	t.Helper()
	s := &stackGH{t: t, prs: map[int]*stackPR{}}
	for _, p := range prs {
		s.prs[p.Number] = p
	}
	f.run = s.run
	return s
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
	raw := fmt.Sprintf(`{"number":%d,"state":"OPEN","baseRefName":%q,"headRefName":%q,`+
		`"body":"Part of #40\n\n## TLDR\nx","commits":{"nodes":[{"commit":{"statusCheckRollup":`+
		`{"contexts":{"nodes":[%s]}}}}]}}`, n, base, head, strings.Join(contexts, ","))
	var p stackPR
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		t.Fatal(err)
	}
	return &p
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
		return s.graphql(args[3])
	}
	n, _ := strconv.Atoi(args[2])
	p := s.prs[n]
	switch args[1] + " " + args[3] {
	case "edit --base":
		p.Base = args[4]
	case "edit --body-file":
		p.Body = stdin
	case "merge --auto":
		p.AutoMerge = &struct{}{}
	case "close --comment":
		p.State = "CLOSED"
	}
	return nil, nil
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
	)
	f.owner(t, Record{Ticket: 40, Worktree: "/w/40", State: Done})
	code, stdout, stderr := f.agents(t, "land-stack", "4")
	want := "not landing #4; waiting on #1 (verify failure), #2 (verify missing), #3 (stage 1 pending), " +
		"#4 (stage 1 missing, verify pending)\n"
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

func TestLandStack_setsTheBodyThenPointsTheStackAtTheFeatureBranchThenQueuesOnlyTheTop(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	top := green(t, 3, "b3", "b2")
	top.Body = "Lands stack: #9\n\nPart of #40\n\n## TLDR\nx"
	s := newStackGH(t, f,
		green(t, 1, "b1", "fb"), green(t, 2, "b2", "b1"), top,
		green(t, 7, "other", "fb"), green(t, 8, "above-other", "other"),
	)
	f.owner(t, Record{Ticket: 40, Worktree: "/w/40", State: Done})
	code, stdout, stderr := f.agents(t, "land-stack", "3")
	if code != 0 || stdout != "queued #3. Lands stack: #1 #2 #3\n" {
		t.Fatalf("%d %q %q", code, stdout, stderr)
	}
	want := []string{
		"gh pr edit 3 --body-file - -R o/r",
		"gh pr edit 2 --base fb -R o/r",
		"gh pr edit 3 --base fb -R o/r",
		"gh pr merge 3 --auto -R o/r",
	}
	if got := s.lines(); !slices.Equal(got, want) {
		t.Fatalf("calls:\n%s", strings.Join(got, "\n"))
	}
	if got := s.prs[3].Body; got != "Lands stack: #1 #2 #3\n\nPart of #40\n\n## TLDR\nx" {
		t.Fatalf("body %q", got)
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
	if code != 0 || stdout != "#3 waits for its checks, then enters the queue\n" || len(s.lines()) != len(want) {
		t.Fatalf("second call: %d %q %v", code, stdout, s.lines())
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
			"queued #2. Lands stack: #1 #2\n",
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

func TestLandStack_aSinglePRSkipsTheBaseEdits(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	s := newStackGH(t, f, green(t, 5, "b5", "fb"))
	f.owner(t, Record{Ticket: 40, Worktree: "/w/40"})
	if code, stdout, stderr := f.agents(t, "land-stack", "5"); code != 0 || stdout != "queued #5. Lands stack: #5\n" {
		t.Fatalf("%d %q %q", code, stdout, stderr)
	}
	want := []string{"gh pr edit 5 --body-file - -R o/r", "gh pr merge 5 --auto -R o/r"}
	if got := s.lines(); !slices.Equal(got, want) {
		t.Fatalf("calls %v", got)
	}
	if got := s.prs[5].Body; got != "Lands stack: #5\n\nPart of #40\n\n## TLDR\nx" {
		t.Fatalf("body %q", got)
	}
}

func TestLandStack_settlesTheQueuedStack(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name       string
		top        func(p *stackPR)
		stdout     string
		calls      []string
		stillQueue bool
		gone       bool
	}{
		{
			name: "merged",
			top: func(p *stackPR) {
				p.State, p.MergeCommit.OID = "MERGED", "abcdef0123456789abcdef0123456789abcdef01"
			},
			stdout: "closed #2: Landed in #3 (abcdef0)\n#3 merged as abcdef0; gt sync ran in WT\n",
			calls: []string{
				"gh pr close 2 --comment Landed in #3 (abcdef0) -R o/r",
				"gt sync --no-interactive --delete-all --no-restack",
			},
		},
		{
			name: "merged in a clone without the worktree",
			top: func(p *stackPR) {
				p.State, p.MergeCommit.OID = "MERGED", "abcdef0123456789abcdef0123456789abcdef01"
			},
			stdout: "closed #2: Landed in #3 (abcdef0)\n#3 merged as abcdef0; no worktree at WT, skipped gt sync\n",
			calls:  []string{"gh pr close 2 --comment Landed in #3 (abcdef0) -R o/r"},
			gone:   true,
		},
		{
			name: "in the queue",
			top: func(p *stackPR) {
				p.InQueue = true
				p.MergeQueueEntry = &struct {
					Position int `json:"position"`
				}{Position: 2}
			},
			stdout:     "#3 is queued at position 2\n",
			stillQueue: true,
		},
		{
			name:       "in the queue before GitHub gives it a position",
			top:        func(p *stackPR) { p.InQueue = true },
			stdout:     "#3 is queued at position 0\n",
			stillQueue: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			merged, top := green(t, 1, "b1", "fb"), green(t, 3, "b3", "fb")
			merged.State = "MERGED"
			tt.top(top)
			s := newStackGH(t, f, merged, green(t, 2, "b2", "fb"), top)
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
		{name: "base edit fails", args: []string{"2"}, prs: stack, fail: "gh pr edit 2 --base", code: 1, stderr: "gt submit --stack"},
		{name: "body edit fails", args: []string{"2"}, prs: stack, fail: "gh pr edit 2 --body-file", code: 1, stderr: "not marked queued"},
		{name: "merge fails", args: []string{"2"}, prs: stack, fail: "gh pr merge", code: 1, stderr: "restore the bases"},
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
		})
	}
}

func TestLandStack_settleFailures(t *testing.T) {
	t.Parallel()
	for _, fail := range []string{"gh pr close", "gt sync"} {
		t.Run(fail, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			top := green(t, 2, "b2", "fb")
			top.State = "MERGED"
			s := newStackGH(t, f, green(t, 1, "b1", "fb"), top)
			s.fail = fail
			f.owner(t, Record{Ticket: 40, Worktree: t.TempDir(), Queued: &Queue{Top: 2, PRs: []int{1, 2}}})
			if code, _, stderr := f.agents(
				t,
				"land-stack",
				"2",
			); code != 1 ||
				!strings.Contains(stderr, fail+": boom") {
				t.Fatalf("%d %q", code, stderr)
			}
			if f.owned(t).Queued == nil {
				t.Fatal("a failed settle cleared the mark")
			}
		})
	}
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
	path := f.Env(t).recordPath(40)
	if err := os.Chmod(path, 0o400); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := f.agents(t, "land-stack", "5"); code != 1 || !strings.Contains(stderr, "write owner record") {
		t.Fatalf("%d %q", code, stderr)
	}
}

func TestLandsBody(t *testing.T) {
	t.Parallel()
	for body, want := range map[string]string{
		"## TLDR\nx":                      "L\n\n## TLDR\nx",
		"Lands stack: #1\n## TLDR\nx":     "L\n\n## TLDR\nx",
		"Lands stack: #1\n\n\n## TLDR\nx": "L\n\n## TLDR\nx",
		"":                                "L\n\n",
	} {
		if got := landsBody("L", body); got != want {
			t.Errorf("landsBody(%q) = %q, want %q", body, got, want)
		}
	}
}

func ejectedStack(t *testing.T, f *fixture, lands string) *stackGH {
	t.Helper()
	top := green(t, 3, "b3", "b2")
	top.Base = "fb"
	top.Body = lands + "\n\nPart of #40\n\n## TLDR\nx"
	s := newStackGH(t, f, green(t, 1, "b1", "fb"), green(t, 2, "b2", "fb"), top)
	f.owner(t, Record{Ticket: 40, Worktree: "/w/40", State: Done, Queued: &Queue{Top: 3, PRs: []int{1, 2, 3}}})
	return s
}

func TestLandStack_relandsAnEjectedStackWholeInOneCall(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	s := ejectedStack(t, f, "Lands stack: #1 #2 #3")
	code, stdout, stderr := f.agents(t, "land-stack", "3")
	if code != 0 || stdout != "#3 left the queue; relanding its stack\nqueued #3. Lands stack: #1 #2 #3\n" {
		t.Fatalf("%d %q %q", code, stdout, stderr)
	}
	want := []string{"gh pr edit 3 --body-file - -R o/r", "gh pr merge 3 --auto -R o/r"}
	if got := s.lines(); !slices.Equal(got, want) {
		t.Fatalf("calls %v", got)
	}
	if q := f.owned(t).Queued; q == nil || q.Top != 3 || !slices.Equal(q.PRs, []int{1, 2, 3}) {
		t.Fatalf("queued %+v", q)
	}
}

func TestLandStack_checksEveryPRTheLandsLineNames(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, lands string
		edit        func(prs map[int]*stackPR)
		code        int
		out         string
	}{
		{
			name:  "a lower PR's verify is red",
			lands: "Lands stack: #1 #2 #3",
			edit: func(prs map[int]*stackPR) {
				prs[2].Commits.Nodes[0].Commit.StatusCheckRollup.Contexts.Nodes[1].State = "FAILURE"
			},
			out: "#3 left the queue; relanding its stack\nnot landing #3; waiting on #2 (verify failure)\n",
		},
		{
			name:  "a lower PR's stage 1 is red",
			lands: "Lands stack: #1 #2 #3",
			edit: func(prs map[int]*stackPR) {
				prs[1].Commits.Nodes[0].Commit.StatusCheckRollup.Contexts.Nodes[0].Conclusion = "FAILURE"
			},
			out: "#3 left the queue; relanding its stack\nnot landing #3; waiting on #1 (stage 1 failure)\n",
		},
		{
			name:  "a lower PR closed",
			lands: "Lands stack: #1 #2 #3",
			edit:  func(prs map[int]*stackPR) { prs[1].State = "CLOSED" },
			code:  1,
			out:   `#1 from #3's "Lands stack:" line is closed; only open PRs reland`,
		},
		{
			name:  "a PR in the line is gone",
			lands: "Lands stack: #9 #2 #3",
			code:  1,
			out:   "#9 is not a PR",
		},
		{
			name:  "the line ends with another PR",
			lands: "Lands stack: #3 #1",
			code:  1,
			out:   `#3's "Lands stack:" line does not end with #3`,
		},
		{
			name:  "the line names something that is not a PR",
			lands: "Lands stack: #1 two #3",
			code:  1,
			out:   `"two" is not a PR in "Lands stack: #1 two #3"`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			s := ejectedStack(t, f, tc.lands)
			if tc.edit != nil {
				tc.edit(s.prs)
			}
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

func TestLandStack_withoutALandsLineReadsTheStackFromGraphite(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, gtLog, fail, out string
	}{
		{
			name:  "gt names the lower PRs",
			gtLog: "◯  fb\n◯  b1\n◯  b2 (needs restack)\n◉  b3\n◯  b4\n",
			out:   "queued #3. Lands stack: #1 #2 #3\n",
		},
		{
			name:  "gt lists the trunk, whose own PR targets main",
			gtLog: "◯  fb\n◯  b0\n◯  b1\n◉  b3\n",
			out:   "queued #3. Lands stack: #1 #3\n",
		},
		{name: "gt names only the top", gtLog: "◯  fb\n◉  b3\n", out: "queued #3. Lands stack: #3\n"},
		{name: "gt is on another stack", gtLog: "◯  fb\n◯  b1\n◉  b7\n", out: "queued #3. Lands stack: #3\n"},
		{
			name: "gt fails",
			fail: "gt log",
			out:  "gt log in /w/40 failed (gt log: boom); landing the GitHub base chain\nqueued #3. Lands stack: #3\n",
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
	s := ejectedStack(t, f, "Lands stack: #1 #2 #3")
	s.prs[3].State = "CLOSED"
	code, stdout, stderr := f.agents(t, "land-stack", "3")
	if code != 1 || stdout != "#3 left the queue; relanding its stack\n" ||
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
	s := ejectedStack(t, f, "Lands stack: #1 #2 #3")
	if err := os.Chmod(f.Env(t).recordPath(40), 0o400); err != nil {
		t.Fatal(err)
	}
	code, _, stderr := f.agents(t, "land-stack", "3")
	if code != 1 || !strings.Contains(stderr, "write owner record") || len(s.lines()) != 0 {
		t.Fatalf("%d %q %v", code, stderr, s.lines())
	}
}

func TestWatch_clearsTheQueuedMarkOfAnEjectedStack(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	queued := green(t, 5, "b5", "fb")
	queued.InQueue = true
	waiting := green(t, 6, "b6", "fb")
	waiting.AutoMerge = &struct{}{}
	merged := green(t, 8, "b8", "fb")
	merged.State = "MERGED"
	s := ejectedStack(t, f, "Lands stack: #1 #2 #3")
	for _, p := range []*stackPR{queued, waiting, merged} {
		s.prs[p.Number] = p
	}
	f.owner(t, Record{Ticket: 40, State: Exited, Queued: &Queue{Top: 3, PRs: []int{1, 2, 3}}})
	f.record(t, Record{Ticket: 41, State: Exited, Queued: &Queue{Top: 5, PRs: []int{5}}})
	f.record(t, Record{Ticket: 42, State: Exited, Queued: &Queue{Top: 6, PRs: []int{6}}})
	f.record(t, Record{Ticket: 43, State: Exited, Queued: &Queue{Top: 8, PRs: []int{8}}})
	f.record(t, Record{Ticket: 44, State: Exited})
	f.noFailures()
	code, stdout, stderr := f.agents(t, "watch")
	want := "unqueued: #40; #3 left the queue. Fix the stack with gt modify and gt submit --stack --draft, then run land-stack 3\n"
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
	t.Run("the top cannot be read", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		s := ejectedStack(t, f, "Lands stack: #1 #2 #3")
		s.fail = "gh api"
		if code, _, stderr := f.agents(t, "watch"); code != 1 || !strings.Contains(stderr, "gh api: boom") {
			t.Fatalf("%d %q", code, stderr)
		}
	})
	t.Run("the record cannot be written", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		ejectedStack(t, f, "Lands stack: #1 #2 #3")
		if err := os.Chmod(f.Env(t).recordPath(40), 0o400); err != nil {
			t.Fatal(err)
		}
		if code, _, stderr := f.agents(t, "watch"); code != 1 || !strings.Contains(stderr, "write owner record") {
			t.Fatalf("%d %q", code, stderr)
		}
	})
}
