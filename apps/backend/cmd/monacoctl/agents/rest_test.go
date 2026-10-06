package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

func restPullBody(n int, head string) string {
	return fmt.Sprintf(`{"number":%d,"state":"open","body":"Part of #40\n\n## TLDR\nx",`+
		`"head":{"ref":%q,"sha":%q},"base":{"ref":"fb","sha":"base-sha"},`+
		`"merge_commit_sha":"merge-sha","labels":[]}`, n, head, head+"-oid")
}

func serveChecks(f *fixture, sha string) {
	f.hub.on(get("/commits/"+sha+"/check-runs?per_page=100&filter=all&page=1"),
		`{"check_runs":[{"id":7,"name":"ci / ci-ok","status":"completed","conclusion":"success",`+
			`"completed_at":"2026-09-29T11:03:00Z","details_url":"https://example.test/run"}]}`)
	f.hub.on(get("/commits/"+sha+"/status"),
		`{"statuses":[{"context":"verify","state":"success","created_at":"2026-09-29T11:04:00Z"}]}`)
}

func serveQuietPull(f *fixture, n int) {
	f.hub.on(get(fmt.Sprintf("/pulls/%d", n)), fmt.Sprintf(
		`{"number":%d,"state":"open","body":"Part of #40","head":{"ref":"b%d","sha":"sha%d"},`+
			`"base":{"ref":"fb","sha":"base"},"labels":[]}`, n, n, n))
	f.hub.on(get(fmt.Sprintf("/commits/sha%d/check-runs?per_page=100&filter=all&page=1", n)), `{}`)
	f.hub.on(get(fmt.Sprintf("/commits/sha%d/status", n)), `{}`)
	f.hub.on(list(fmt.Sprintf("/issues/%d/events?", n)), `[]`)
}

func wantErr(t *testing.T, err error, sub string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), sub) {
		t.Fatalf("%v", err)
	}
}

func TestLandStack_labelsThroughRESTWhenGraphQLIsForbidden(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	s := newStackGH(t, f, green(t, 5, "b5", "fb"))
	s.denied = true
	body := restPullBody(5, "b5")
	f.hub.on(get("/pulls/5"), body)
	f.hub.on(list("/issues/5/events?"), `[]`)
	f.hub.on(list("/pulls?state=open"), "["+body+"]")
	serveChecks(f, "b5-oid")
	f.owner(t, Record{Ticket: 40, Worktree: f.dir})
	code, stdout, stderr := f.agents(t, "land-stack", "5")
	want := "queued #5\nfollow it: monacoctl agents watch (under Claude Code's Monitor tool)\n" +
		"no Graphite draft holds #5 after 3m0s; run land-stack again if it stays that way\n"
	posts := f.hub.callsContaining("POST /repos/o/r/issues/5/labels")
	if code != 0 || stdout != want || s.gql != 1 || len(posts) != 1 ||
		!strings.Contains(f.hub.body(posts[0]), `"merge-queue"`) {
		t.Fatalf("%d %q %q graphql %d posts %v", code, stdout, stderr, s.gql, posts)
	}
}

func TestLandStack_returnsAGraphQLErrorThatIsNotForbidden(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	s := newStackGH(t, f, green(t, 1, "b1", "fb"), green(t, 2, "b2", "b1"))
	s.fail = "gh api graphql"
	f.owner(t, Record{Ticket: 40, Worktree: f.dir, State: Done})
	code, _, stderr := f.agents(t, "land-stack", "2")
	if code != 1 || !strings.Contains(stderr, "boom") || len(f.hub.callsContaining("/pulls")) != 0 ||
		len(f.hub.callsContaining("/commits/")) != 0 {
		t.Fatalf("%d %q rest %v", code, stderr, f.hub.callsContaining("/pulls"))
	}
}

func draftList(page int) string {
	return get(fmt.Sprintf("/pulls?state=open&per_page=100&page=%d", page))
}

func closedDraftList() string {
	return get("/pulls?state=closed&sort=updated&direction=desc&per_page=30")
}

func openPullPage(n int) string {
	rows := make([]string, n)
	for i := range rows {
		rows[i] = fmt.Sprintf(`{"number":%d,"state":"open","title":"old","head":{"ref":"feature"}}`, i+1)
	}
	return "[" + strings.Join(rows, ",") + "]"
}

func forbidDequeue(t *testing.T) (*fixture, *Env) {
	t.Helper()
	f := newFixture(t)
	_, env := dequeueStack(t, f)
	f.hub.status[graphqlRoute] = http.StatusForbidden
	f.hub.on(graphqlRoute, "forbidden")
	f.hub.on(closedDraftList(), `[]`)
	serveQuietPull(f, 1)
	serveQuietPull(f, 2)
	return f, env
}

func TestDequeue_readsDraftsFromRESTWhenGraphQLIsForbidden(t *testing.T) {
	t.Parallel()
	hold := `[{"number":90,"title":"(PRs 1, 2)","state":"open","head":{"ref":"gtmq_hold"}}]`
	heldMsg := "Graphite still holds #2; remove it from the queue in the Graphite app, then rerun"
	closed := `[{"number":90,"title":"(PRs 1, 2)","state":"closed","head":{"ref":"gtmq_hold"}}]`
	unrelated := `[{"number":9,"title":"nope","head":{"ref":"feature"}},` +
		`{"number":90,"title":"(PRs 8)","head":{"ref":"gtmq_x"}}]`
	for _, tc := range []struct {
		body string
		held bool
	}{{unrelated, false}, {hold, true}, {closed, false}} {
		f, env := forbidDequeue(t)
		f.hub.on(draftList(1), tc.body)
		err := dequeueCmd(t.Context(), env, []string{"2"}, &strings.Builder{})
		if len(f.hub.callsContaining("POST /graphql")) != 1 ||
			tc.held && cliText(err) != heldMsg || !tc.held && err != nil {
			t.Fatal(err)
		}
	}
	f, env := forbidDequeue(t)
	f.hub.on(draftList(1), openPullPage(100))
	f.hub.on(draftList(2), hold)
	err := dequeueCmd(t.Context(), env, []string{"2"}, &strings.Builder{})
	if cliText(err) != heldMsg || len(f.hub.callsContaining("page=2")) == 0 {
		t.Fatal(err)
	}
	f, env = forbidDequeue(t)
	f.hub.on(draftList(1), openPullPage(100))
	f.hub.status[draftList(2)] = http.StatusInternalServerError
	f.hub.on(draftList(2), "boom")
	var out strings.Builder
	err = dequeueCmd(t.Context(), env, []string{"2"}, &out)
	if err == nil || !strings.Contains(err.Error(), "boom") || strings.Contains(out.String(), "safe to push") ||
		len(f.hub.callsContaining("page=2")) == 0 {
		t.Fatalf("%v %q", err, out.String())
	}
}

func mustCommit(t *testing.T, raw string) *gqlCommit {
	t.Helper()
	var c gqlCommit
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		t.Fatal(err)
	}
	return &c
}

func TestReadChecks_restFallback(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	env := f.Env(t)
	env.Run = func(_ context.Context, _, _, name string, args ...string) ([]byte, error) {
		if name == "gh" && len(args) > 3 && checkPageQuery(args[3]) {
			return nil, errors.New("HTTP 403")
		}
		return nil, errors.New("unexpected " + name)
	}
	c := mustCommit(t, firstPage("b1", ciOK("FAILURE", 1)))
	serveChecks(f, "b1")
	if err := env.readChecks(t.Context(), []*gqlCommit{c}, env.graphqlGH); err != nil {
		t.Fatal(err)
	}
	latest := c.latest()
	if !env.useREST() || len(latest) != 2 || latest[0].Conclusion != "SUCCESS" || latest[0].DatabaseID != 7 ||
		latest[0].DetailsURL != "https://example.test/run" || latest[1].State != "SUCCESS" ||
		c.StatusCheckRollup.Contexts.PageInfo.HasNextPage {
		t.Fatalf("%+v", latest)
	}
	other := newFixture(t)
	env = other.Env(t)
	env.Run = func(context.Context, string, string, string, ...string) ([]byte, error) {
		return nil, errors.New("gh api graphql: boom")
	}
	err := env.readChecks(t.Context(), []*gqlCommit{mustCommit(t, firstPage("b1"))}, env.graphqlGH)
	if err == nil || !strings.Contains(err.Error(), "boom") || env.useREST() ||
		len(other.hub.callsContaining("/commits/")) != 0 {
		t.Fatalf("%v", err)
	}
	env.markREST()
	serveChecks(other, "sha")
	called := false
	sha := &gqlCommit{OID: "sha"}
	err = env.readChecks(t.Context(), []*gqlCommit{sha}, func(context.Context, string, any) error {
		called = true
		return errors.New("nope")
	})
	if err != nil || called || sha.latest()[0].Conclusion != "SUCCESS" {
		t.Fatalf("%v %v", err, called)
	}
}

func TestGraphQL_switchesOnlyOnARefusal(t *testing.T) {
	t.Parallel()
	var c gqlCommit
	if err := json.Unmarshal([]byte(firstPage("b1")), &c); err != nil {
		t.Fatal(err)
	}
	page, _ := nextChecks([]*gqlCommit{&c})
	f := newFixture(t)
	env := f.Env(t)
	f.hub.status[graphqlRoute] = http.StatusForbidden
	f.hub.on(graphqlRoute, "no")
	err := env.graphQL(t.Context(), page, &struct{}{})
	if !graphqlHTTPDenied(err) || !env.useREST() {
		t.Fatalf("%v", err)
	}
	f = newFixture(t)
	env = f.Env(t)
	f.hub.status[graphqlRoute] = http.StatusInternalServerError
	f.hub.on(graphqlRoute, "rate limited")
	err = env.graphQL(t.Context(), page, &struct{}{})
	if err == nil || env.useREST() || len(f.hub.callsContaining("/pulls")) != 0 {
		t.Fatalf("%v", err)
	}
	f = newFixture(t)
	env = f.Env(t)
	env.markREST()
	f.hub.on(get("/pulls/1"), restPullBody(1, "b1"))
	f.hub.on(list("/issues/1/events?"), `[]`)
	var data struct {
		Repository map[string]*stackPR `json:"repository"`
	}
	q := repoQuery + "p1: pullRequest(number:1){...pr} }}\nfragment pr on PullRequest{number}"
	if err = env.graphQL(t.Context(), q, &data); err != nil || data.Repository["p1"] == nil ||
		data.Repository["p1"].Number != 1 || len(f.hub.callsContaining("POST /graphql")) != 0 {
		t.Fatalf("%v %+v", err, data.Repository["p1"])
	}
}

func TestRESTStackAndDrafts(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	env := f.Env(t)
	closed := time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)
	f.hub.on(get("/pulls/1"), `{"number":1,"state":"closed","closed_at":"`+closed.Format(time.RFC3339)+
		`","merge_commit_sha":"abc","body":"Part of #40","head":{"ref":"b1","sha":"sha"},`+
		`"base":{"ref":"fb","sha":"base"},"labels":[{"name":"merge-queue"}]}`)
	events := list("/issues/1/events?")
	f.hub.on(events, `[{"event":"labeled","created_at":"2026-01-01T00:00:00Z","label":{"name":"other"}},`+
		`{"event":"unlabeled","created_at":"2026-01-02T00:00:00Z","label":{"name":"merge-queue"}}]`)
	var data struct {
		Repository map[string]*stackPR `json:"repository"`
	}
	q := repoQuery + "p1: pullRequest(number:1){...pr} p9: pullRequest(number:9){...pr} }}\n" +
		"fragment pr on PullRequest{number}"
	if err := env.restQuery(t.Context(), q, &data); err != nil {
		t.Fatal(err)
	}
	p := data.Repository["p1"]
	if p == nil || data.Repository["p9"] != nil || !p.ClosedAt.Equal(closed) || p.MergeCommit.OID != "abc" ||
		p.State != "CLOSED" || !p.labeled("merge-queue") || p.HeadOID != "sha" ||
		len(p.TimelineItems.Nodes) != 1 || p.TimelineItems.Nodes[0].Label.Name != "merge-queue" {
		t.Fatalf("%+v", p)
	}
	f.hub.status[events] = http.StatusInternalServerError
	f.hub.on(events, "no events")
	_, err := env.restStackPayload(t.Context(), q)
	wantErr(t, err, "no events")
	f.hub.status[events] = 0
	f.hub.on(events, `[]`)
	f.hub.status[get("/pulls/1")] = http.StatusInternalServerError
	f.hub.on(get("/pulls/1"), "boom")
	_, err = env.restStackPayload(t.Context(), q)
	wantErr(t, err, "boom")
	f.hub.status[list("/pulls?state=open")] = http.StatusInternalServerError
	f.hub.on(list("/pulls?state=open"), "nope")
	_, err = env.restOpenPayload(t.Context())
	wantErr(t, err, "nope")
	merged := "2026-01-02T00:00:00Z"
	route := draftList(1)
	f.hub.on(closedDraftList(), `[]`)
	f.hub.status[route] = 0
	f.hub.on(route, `[{"number":4,"title":"keep","state":"closed","merged_at":"`+merged+
		`","head":{"ref":"gtmq_a"},"updated_at":"`+merged+`","closed_at":"`+merged+
		`"},{"number":5,"title":"old","state":"closed","head":{"ref":"gtmq_b"}},`+
		`{"number":6,"title":"drop","head":{"ref":"feature"}}]`)
	var drafts struct {
		Repository struct {
			Drafts struct {
				Nodes []queueDraft `json:"nodes"`
			} `json:"drafts"`
		} `json:"repository"`
	}
	if err := env.restQuery(t.Context(), draftsQuery, &drafts); err != nil {
		t.Fatal(err)
	}
	nodes := drafts.Repository.Drafts.Nodes
	if len(nodes) != 2 || nodes[0].State != "MERGED" || nodes[1].State != "CLOSED" || nodes[0].HeadRefName != "gtmq_a" {
		t.Fatalf("%+v", nodes)
	}
	if nodes[0].ClosedAt.Format(time.RFC3339) != merged || !nodes[1].ClosedAt.IsZero() {
		t.Fatalf("closedAt %v and %v, want %s and none", nodes[0].ClosedAt, nodes[1].ClosedAt, merged)
	}
	f.hub.status[route] = http.StatusInternalServerError
	f.hub.on(route, "boom")
	_, err = env.restDraftPayload(t.Context())
	wantErr(t, err, "boom")
	f.hub.status[route] = 0
	f.hub.on(route, `[]`)
	f.hub.status[closedDraftList()] = http.StatusInternalServerError
	f.hub.on(closedDraftList(), "closed boom")
	_, err = env.restDraftPayload(t.Context())
	wantErr(t, err, "closed boom")
}

func TestRESTMode_holdsAGraphiteTakenPRUntilAClosedDraftRanItThenEjectsIt(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, closed, want string }{
		{"no draft yet", `[]`, prTaken},
		{"a closed draft ran the PR since", `[{"number":90,"title":"(PRs 1)","state":"closed",` +
			`"head":{"ref":"gtmq_a"},"updated_at":"2026-09-27T11:59:00Z","closed_at":"2026-09-27T11:59:00Z"}]`, prEjected},
		{"a closed draft ran it before Graphite took it", `[{"number":90,"title":"(PRs 1)","state":"closed",` +
			`"head":{"ref":"gtmq_a"},"updated_at":"2026-09-27T11:57:00Z","closed_at":"2026-09-27T11:57:00Z"}]`, prTaken},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			env := f.Env(t)
			env.markREST()
			serveQuietPull(f, 1)
			f.hub.on(list("/issues/1/events?"), fmt.Sprintf(
				`[{"event":"unlabeled","created_at":%q,"label":{"name":"merge-queue"},"actor":{"login":%q}}]`,
				f.now.Add(-2*time.Minute).Format(time.RFC3339), graphiteBot))
			f.hub.on(draftList(1), `[]`)
			f.hub.on(closedDraftList(), tc.closed)
			prs, err := env.stackPulls(t.Context(), []int{1})
			if err != nil {
				t.Fatal(err)
			}
			drafts, err := env.queueDrafts(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if got := env.queueState(prs[0], false, drafts); got != tc.want {
				t.Fatalf("%q, want %q (drafts %+v)", got, tc.want, drafts)
			}
		})
	}
}

func TestRESTQueryErrors(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	env := f.Env(t)
	if queryKind("zzz") != "query" || queryKind(draftsQuery) != "drafts" ||
		queryKind("open: pullRequests") != "open" ||
		queryKind("fragment pr on PullRequest") != "stack" ||
		queryKind(failureQuery("")) != "watch" || queryKind(ticketQuery([]int{1})) != "timeline" ||
		queryKind(repoQuery+"open: pullRequests("+openPage("")+"){nodes{"+stackFields+"}}}}") != "open" ||
		queryKind(repoQuery+"}}\nfragment pr on PullRequest{"+stackFields+"}") != "stack" {
		t.Fatal(queryKind("zzz"))
	}
	var n int
	wantErr(t, env.restQuery(t.Context(), "nope", &n), "no REST mapping")
	f.hub.on(get("/pulls/1"), restPullBody(1, "b1"))
	f.hub.on(list("/issues/1/events?"), `[]`)
	q := repoQuery + "p1: pullRequest(number:1){...pr} }}\nfragment pr on PullRequest{" + stackFields + "}"
	wantErr(t, env.restQuery(t.Context(), q, &n), "decode REST stack")
	env.GitHub.encode = func(any) ([]byte, error) { return nil, errors.New("encode") }
	var data any
	wantErr(t, env.restQuery(t.Context(), q, &data), "encode")
}

func TestLabels_postAndDelete(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	env := f.Env(t)
	route := "POST /repos/o/r/issues/5/labels"
	f.hub.on(route, "[]")
	if err := env.addLabel(t.Context(), 5); err != nil || !strings.Contains(f.hub.body(route), `"merge-queue"`) {
		t.Fatal(err)
	}
	f.hub.status[route] = http.StatusInternalServerError
	f.hub.on(route, "boom")
	wantErr(t, env.addLabel(t.Context(), 5), "boom")
	if err := env.removeLabel(t.Context(), 8); err != nil {
		t.Fatal(err)
	}
	del := "DELETE /repos/o/r/issues/3/labels/merge-queue"
	f.hub.on(del, "[]")
	if err := env.removeLabel(t.Context(), 3); err != nil {
		t.Fatal(err)
	}
	f.hub.status[del] = http.StatusInternalServerError
	f.hub.on(del, "boom")
	wantErr(t, env.removeLabel(t.Context(), 3), "boom")
}

func TestCheckContexts_pagesAndErrors(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	env := f.Env(t)
	if nodes, err := env.GitHub.checkContexts(t.Context(), ""); err != nil || nodes != nil {
		t.Fatalf("%v %v", nodes, err)
	}
	if err := env.restFillChecks(t.Context(), []*gqlCommit{{}}); err != nil {
		t.Fatal(err)
	}
	runs := make([]map[string]any, pageSize)
	for i := range runs {
		runs[i] = map[string]any{"id": i + 1, "name": "ci / job", "status": "completed", "conclusion": "success"}
	}
	page1, err := json.Marshal(map[string]any{"check_runs": runs})
	if err != nil {
		t.Fatal(err)
	}
	sha := "abc"
	f.hub.on(get("/commits/"+sha+"/check-runs?per_page=100&filter=all&page=1"), string(page1))
	f.hub.on(get("/commits/"+sha+"/check-runs?per_page=100&filter=all&page=2"),
		`{"check_runs":[{"name":"ci / last","status":"in_progress"}]}`)
	f.hub.on(get("/commits/"+sha+"/status"),
		`{"statuses":[{"context":"verify","state":"pending","created_at":"2026-01-02T00:00:00Z"}]}`)
	nodes, err := env.GitHub.checkContexts(t.Context(), sha)
	if err != nil || len(nodes) != pageSize+2 || nodes[0].Status != "COMPLETED" ||
		nodes[pageSize].Status != "IN_PROGRESS" || nodes[pageSize+1].State != "PENDING" {
		t.Fatalf("%d %v", len(nodes), err)
	}
	f.hub.status[get("/commits/bad/check-runs?per_page=100&filter=all&page=1")] = http.StatusInternalServerError
	f.hub.on(get("/commits/bad/check-runs?per_page=100&filter=all&page=1"), "boom")
	wantErr(t, env.restFillChecks(t.Context(), []*gqlCommit{{OID: "bad"}}), "boom")
	f.hub.on(get("/commits/nostatus/check-runs?per_page=100&filter=all&page=1"), `{"check_runs":[]}`)
	f.hub.status[get("/commits/nostatus/status")] = http.StatusInternalServerError
	f.hub.on(get("/commits/nostatus/status"), "boom")
	_, err = env.GitHub.checkContexts(t.Context(), "nostatus")
	wantErr(t, err, "boom")
}

func TestGraphQLDenialHelpers(t *testing.T) {
	t.Parallel()
	if graphqlCLIDenied(nil) || graphqlCLIDenied(errors.New("boom")) || !graphqlCLIDenied(errors.New("HTTP 403")) ||
		!graphqlCLIDenied(errors.New("not permitted")) {
		t.Fatal("cli")
	}
	ok := httpStatusError{path: "/graphql", status: "403 Forbidden"}
	if graphqlHTTPDenied(nil) || graphqlHTTPDenied(errors.New("403")) ||
		graphqlHTTPDenied(httpStatusError{path: "/pulls/1", status: "403 Forbidden"}) ||
		graphqlHTTPDenied(httpStatusError{path: "/graphql", status: "500 Internal Server Error"}) ||
		!graphqlHTTPDenied(
			ok,
		) || !checkPageQuery(`object(oid:"x"){statusCheckRollup`) ||
		checkPageQuery("pullRequest statusCheckRollup") || checkPageQuery(ticketQuery([]int{1})) {
		t.Fatal("http")
	}
}

func TestWatch_readsFailuresThroughRESTWhenGraphQLIsForbidden(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.hub.status[graphqlRoute] = http.StatusForbidden
	f.hub.on(graphqlRoute, "forbidden")
	open := list("/pulls?state=open")
	f.hub.on(open, `[{"number":5,"body":"Part of #40","head":{"ref":"b5","sha":"sha5"},`+
		`"base":{"ref":"fb"},"labels":[{"name":"x"}]}]`)
	events := list("/issues/5/events?")
	f.hub.on(events, `[{"event":"labeled","label":{"name":"other"}},{"event":"unlabeled",`+
		`"created_at":"2026-03-01T00:00:00Z","label":{"name":"merge-queue"},"actor":{"login":"someone"}}]`)
	drafts := get("/pulls?state=all&sort=updated&direction=desc&per_page=30")
	f.hub.on(drafts, `[{"number":2,"state":"closed","title":"(PRs 5)","head":{"ref":"gtmq_b"},`+
		`"updated_at":"2026-02-02T00:00:00Z"},{"number":1,"state":"closed","title":"(PRs 5)",`+
		`"head":{"ref":"gtmq_a"},"updated_at":"2026-01-01T00:00:00Z"}]`)
	f.hub.on(get("/commits/sha5/check-runs?per_page=100&filter=all&page=1"),
		`{"check_runs":[{"name":"ci / ci-ok","status":"completed","conclusion":"failure"}]}`)
	f.hub.on(get("/commits/sha5/status"), `{"statuses":[]}`)
	env := f.Env(t)
	var watched struct {
		Repository struct {
			PullRequests struct {
				Nodes []watchPR `json:"nodes"`
			} `json:"pullRequests"`
			Drafts struct {
				Nodes []queueDraft `json:"nodes"`
			} `json:"drafts"`
		} `json:"repository"`
	}
	if err := env.restQuery(t.Context(), failureQuery(""), &watched); err != nil {
		t.Fatal(err)
	}
	prs, got := watched.Repository.PullRequests.Nodes, watched.Repository.Drafts.Nodes
	actor := ""
	if len(prs) == 1 && len(prs[0].TimelineItems.Nodes) == 1 {
		actor = prs[0].TimelineItems.Nodes[0].Actor.Login
	}
	if actor != "someone" || len(got) != 2 || got[0].HeadRefName != "gtmq_a" || got[1].HeadRefName != "gtmq_b" {
		t.Fatalf("%+v %+v", prs, got)
	}
	code, stdout, stderr := f.agents(t, "watch", "--once")
	if !strings.Contains(stdout, "#5 stage 1 is red") || len(f.hub.callsContaining("POST /graphql")) != 1 {
		t.Fatalf("%d %q %q gql %v", code, stdout, stderr, f.hub.callsContaining("POST /graphql"))
	}
	f.hub.status[open] = http.StatusInternalServerError
	f.hub.on(open, "boom")
	_, err := env.restWatchPayload(t.Context())
	wantErr(t, err, "boom")
	f.hub.status[open] = 0
	f.hub.on(open, `[{"number":5,"head":{"ref":"b5","sha":"sha5"},"base":{"ref":"fb"}}]`)
	f.hub.status[events] = http.StatusInternalServerError
	f.hub.on(events, "boom")
	_, err = env.restWatchPayload(t.Context())
	wantErr(t, err, "boom")
	f.hub.status[events] = 0
	f.hub.on(events, `[]`)
	f.hub.status[drafts] = http.StatusInternalServerError
	f.hub.on(drafts, "boom")
	_, err = env.restWatchPayload(t.Context())
	wantErr(t, err, "boom")
}

func TestStatus_readsTimelinesThroughRESTWhenGraphQLIsForbidden(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.hub.status[graphqlRoute] = http.StatusForbidden
	f.hub.on(graphqlRoute, "forbidden")
	f.batch(t, 5)
	f.hub.on(list("/pulls?state=open"), `[]`)
	f.hub.on(list("/issues/7/comments?"), `[]`)
	f.hub.on("POST /repos/o/r/issues/7/comments", "ok")
	timeline := list("/issues/5/timeline?")
	f.hub.on(timeline, `[{"event":"commented"},{"event":"cross-referenced","source":{"issue":{"number":12}}},`+
		`{"event":"cross-referenced","source":{"issue":{"number":99,"repository_url":"https://api.github.com/repos/o/other","pull_request":{}}}},`+
		`{"event":"cross-referenced","source":{"issue":{"number":11,"repository_url":"https://api.github.com/repos/o/r","pull_request":{}}}}]`)
	f.hub.on(get("/pulls/11"), `{"number":11,"state":"closed","created_at":"2026-01-01T00:00:00Z",`+
		`"merged_at":"2026-01-03T00:00:00Z","closed_at":"2026-01-03T00:00:00Z","body":"Part of #5",`+
		`"head":{"ref":"b11","sha":"h11"},`+
		`"base":{"ref":"fb"},"labels":[{"name":"merge-queue"}]}`)
	f.hub.on(list("/issues/11/events?"), `[{"event":"labeled","created_at":"2026-01-02T00:00:00Z",`+
		`"label":{"name":"merge-queue"}},{"event":"unlabeled","created_at":"2026-01-04T00:00:00Z",`+
		`"label":{"name":"merge-queue"}},{"event":"closed"}]`)
	f.hub.on(get("/commits/h11"), `{"commit":{"committer":{"date":"2026-01-02T03:00:00Z"}}}`)
	f.hub.on(get("/commits/h11/check-runs?per_page=100&filter=all&page=1"),
		`{"check_runs":[{"name":"ci / ci-ok","status":"completed","conclusion":"success"}]}`)
	f.hub.on(get("/commits/h11/status"), `{"statuses":[]}`)
	env := f.Env(t)
	empty, err := env.restTimelinePayload(t.Context(), "CROSS_REFERENCED_EVENT")
	repo, _ := empty.(map[string]any)
	inner, _ := repo["repository"].(map[string]any)
	if err != nil || len(inner) != 0 {
		t.Fatalf("%v %v", empty, err)
	}
	views, err := env.views(t.Context(), Batch{Tickets: []BatchTicket{{Ticket: 5}}})
	if err != nil {
		t.Fatal(err)
	}
	p := views[0].PRs[0]
	head := time.Date(2026, 1, 2, 3, 0, 0, 0, time.UTC)
	queuedAt := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	removedAt := time.Date(2026, 1, 4, 0, 0, 0, 0, time.UTC)
	if !p.Head.Equal(head) || !p.InQueue || len(p.Queued) != 2 || !p.Queued[0].Added || p.Queued[1].Added ||
		!p.Queued[0].At.Equal(queuedAt) || !p.Queued[1].At.Equal(removedAt) || views[0].state() != "merged" {
		t.Fatalf("%+v %s", p, views[0].state())
	}
	code, stdout, stderr := f.agents(t, "status", "--publish")
	body := posted(t, f, "POST /repos/o/r/issues/7/comments")
	if code != 0 || stdout != "status comment updated\n" || !strings.Contains(body, "| #5 | merged |") ||
		len(f.hub.callsContaining("POST /graphql")) != 2 {
		t.Fatalf("%d %q %q\n%s", code, stdout, stderr, body)
	}
	when, err := env.GitHub.committedAt(t.Context(), "")
	if err != nil || !when.IsZero() {
		t.Fatal(when, err)
	}
	f.hub.status[timeline] = http.StatusInternalServerError
	f.hub.on(timeline, "boom")
	_, err = env.restTimelinePayload(t.Context(), ticketQuery([]int{5}))
	wantErr(t, err, "boom")
	f.hub.status[timeline] = 0
	f.hub.on(timeline, `[{"event":"cross-referenced","source":{"issue":{"number":11,"pull_request":{}}}}]`)
	f.hub.status[get("/pulls/11")] = http.StatusInternalServerError
	f.hub.on(get("/pulls/11"), "boom")
	_, err = env.restTimelinePayload(t.Context(), ticketQuery([]int{5}))
	wantErr(t, err, "boom")
	f.hub.status[get("/pulls/11")] = 0
	f.hub.on(get("/pulls/11"), `{"number":11,"body":"Part of #5","head":{"sha":"h11"}}`)
	ev := list("/issues/11/events?")
	f.hub.status[ev] = http.StatusInternalServerError
	f.hub.on(ev, "boom")
	_, err = env.restTimelinePayload(t.Context(), ticketQuery([]int{5}))
	wantErr(t, err, "boom")
	f.hub.status[ev] = 0
	f.hub.on(ev, `[]`)
	f.hub.status[get("/commits/h11")] = http.StatusInternalServerError
	f.hub.on(get("/commits/h11"), "boom")
	_, err = env.restTimelinePayload(t.Context(), ticketQuery([]int{5}))
	wantErr(t, err, "boom")
}

func TestOpenPRs_pageUntilGitHubSaysDone(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	env := f.Env(t)
	f.hub.onQuery("UNLABELED_EVENT", `{"data":{"repository":{"pullRequests":{`+
		`"pageInfo":{"hasNextPage":true,"endCursor":"CUR1"},"nodes":[{"number":1}]},`+
		`"drafts":{"nodes":[{"number":9,"headRefName":"gtmq_a"}]}}}}`)
	f.hub.onQuery("CUR1", `{"data":{"repository":{"pullRequests":{`+
		`"pageInfo":{"hasNextPage":false,"endCursor":"CUR2"},"nodes":[{"number":2}]}}}}`)
	got, err := env.watchData(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	gql := f.hub.callsContaining("POST /graphql")
	if len(got.prs) != 2 || got.prs[0].Number != 1 || got.prs[1].Number != 2 ||
		len(got.drafts) != 1 || got.drafts[0].Number != 9 || len(gql) != 2 {
		t.Fatalf("%+v %v", got, gql)
	}

	var queries []string
	env.Run = func(_ context.Context, _, _, _ string, args ...string) ([]byte, error) {
		q := args[3]
		queries = append(queries, q)
		if strings.Contains(q, `after:"C1"`) {
			return []byte(`{"data":{"repository":{"open":{"pageInfo":{"hasNextPage":false},` +
				`"nodes":[{"number":4}]}}}}`), nil
		}
		return []byte(`{"data":{"repository":{"open":{"pageInfo":{"hasNextPage":true,"endCursor":"C1"},` +
			`"nodes":[{"number":3}]}}}}`), nil
	}
	open, err := env.openPulls(t.Context())
	if err != nil || len(open) != 2 || open[0].Number != 3 || open[1].Number != 4 || len(queries) != 2 ||
		!strings.Contains(queries[0], "first:20)") {
		t.Fatalf("%v %+v %q", err, open, queries)
	}
}
