package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

func (env *Env) useREST() bool { return env.GitHub.rest.Load() }

func (env *Env) markREST() { env.GitHub.rest.Store(true) }

func graphqlCLIDenied(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "403") || strings.Contains(msg, "not permitted")
}

func graphqlHTTPDenied(err error) bool {
	var hs httpStatusError
	if !errors.As(err, &hs) {
		return false
	}
	return strings.Contains(hs.path, "/graphql") && strings.HasPrefix(hs.status, "403")
}

func checkPageQuery(query string) bool {
	return strings.Contains(query, "object(oid:") && strings.Contains(query, "statusCheckRollup")
}

func (env *Env) graphQL(ctx context.Context, query string, out any) error {
	if env.useREST() && !checkPageQuery(query) {
		return env.restQuery(ctx, query, out)
	}
	err := env.GitHub.graphql(ctx, query, out)
	if !graphqlHTTPDenied(err) {
		return err
	}
	env.markREST()
	if checkPageQuery(query) {
		return err
	}
	return env.restQuery(ctx, query, out)
}

func (env *Env) addLabel(ctx context.Context, n int) error {
	body := map[string][]string{"labels": {env.Config.QueueLabel}}
	return env.GitHub.call(ctx, http.MethodPost, env.GitHub.repo("/issues/%d/labels", n), "", body, nil)
}

func (env *Env) removeLabel(ctx context.Context, n int) error {
	path := env.GitHub.repo("/issues/%d/labels/%s", n, url.PathEscape(env.Config.QueueLabel))
	err := env.GitHub.call(ctx, http.MethodDelete, path, "", nil, nil)
	var hs httpStatusError
	if errors.As(err, &hs) && strings.HasPrefix(hs.status, "404") {
		return nil
	}
	return err
}

type restRef struct {
	Ref string `json:"ref"`
	SHA string `json:"sha"`
}

type restPull struct {
	Number         int        `json:"number"`
	Title          string     `json:"title"`
	Body           string     `json:"body"`
	State          string     `json:"state"`
	Draft          bool       `json:"draft"`
	CreatedAt      time.Time  `json:"created_at"`
	ClosedAt       *time.Time `json:"closed_at"`
	MergedAt       *time.Time `json:"merged_at"`
	MergeCommitSHA *string    `json:"merge_commit_sha"`
	UpdatedAt      time.Time  `json:"updated_at"`
	Head           restRef    `json:"head"`
	Base           restRef    `json:"base"`
	Labels         []gqlName  `json:"labels"`
}

func (p restPull) graphState() string {
	if p.MergedAt != nil && !p.MergedAt.IsZero() {
		return "MERGED"
	}
	return strings.ToUpper(p.State)
}

func (p restPull) asStack() stackPR {
	sp := stackPR{}
	sp.Number = p.Number
	sp.Body = p.Body
	sp.State = p.graphState()
	sp.HeadOID = p.Head.SHA
	sp.Base = p.Base.Ref
	sp.Head = p.Head.Ref
	if p.ClosedAt != nil {
		sp.ClosedAt = *p.ClosedAt
	}
	if p.MergeCommitSHA != nil {
		sp.MergeCommit.OID = *p.MergeCommitSHA
	}
	sp.Labels.Nodes = append(sp.Labels.Nodes, p.Labels...)
	sp.Commits.Nodes = append(sp.Commits.Nodes, struct {
		Commit gqlCommit `json:"commit"`
	}{Commit: gqlCommit{OID: p.Head.SHA}})
	return sp
}

func (g *GitHub) restPull(ctx context.Context, n int) (restPull, error) {
	var p restPull
	err := g.call(ctx, http.MethodGet, g.repo("/pulls/%d", n), "", nil, &p)
	return p, err
}

func (g *GitHub) openPullsREST(ctx context.Context) ([]restPull, error) {
	return pages[restPull](ctx, g, g.repo("/pulls?state=open"))
}

var errNoRESTMapping = errors.New("no REST mapping for query")

func (env *Env) restQuery(ctx context.Context, query string, out any) error {
	payload, err := env.restPayload(ctx, query)
	if err != nil {
		return err
	}
	encode := json.Marshal
	if env.GitHub.encode != nil {
		encode = env.GitHub.encode
	}
	raw, err := encode(payload)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("decode REST %s: %w", queryKind(query), err)
	}
	return nil
}

func queryKind(query string) string {
	switch {
	case strings.Contains(query, "CROSS_REFERENCED_EVENT"):
		return "timeline"
	case strings.Contains(query, "fragment pr on PullRequest"):
		return "stack"
	case strings.Contains(query, "open: pullRequests"):
		return "open"
	case strings.Contains(query, "UNLABELED_EVENT"):
		return "watch"
	case strings.Contains(query, "drafts:"):
		return "drafts"
	default:
		return "query"
	}
}

func (env *Env) restPayload(ctx context.Context, query string) (any, error) {
	switch queryKind(query) {
	case "watch":
		return env.restWatchPayload(ctx)
	case "timeline":
		return env.restTimelinePayload(ctx, query)
	case "open":
		return env.restOpenPayload(ctx)
	case "stack":
		return env.restStackPayload(ctx, query)
	case "drafts":
		return env.restDraftPayload(ctx)
	default:
		return nil, errNoRESTMapping
	}
}

func (env *Env) restStackPayload(ctx context.Context, query string) (any, error) {
	repo := map[string]*stackPR{}
	for _, m := range regexp.MustCompile(`p(\d+): pullRequest`).FindAllStringSubmatch(query, -1) {
		n, _ := strconv.Atoi(m[1])
		p, err := env.stackFromREST(ctx, n)
		if err != nil {
			var hs httpStatusError
			if errors.As(err, &hs) && strings.HasPrefix(hs.status, "404") {
				repo["p"+m[1]] = nil
				continue
			}
			return nil, err
		}
		copied := p
		repo["p"+m[1]] = &copied
	}
	return map[string]any{"repository": repo}, nil
}

func (env *Env) stackFromREST(ctx context.Context, n int) (stackPR, error) {
	p, err := env.GitHub.restPull(ctx, n)
	if err != nil {
		return stackPR{}, err
	}
	events, err := env.GitHub.issueEvents(ctx, n)
	if err != nil {
		return stackPR{}, err
	}
	sp := p.asStack()
	for _, e := range events {
		if e.Event != "unlabeled" {
			continue
		}
		sp.TimelineItems.Nodes = append(sp.TimelineItems.Nodes, struct {
			Typename  string    `json:"__typename"`
			CreatedAt time.Time `json:"createdAt"`
			Label     gqlName   `json:"label"`
			Actor     gqlActor  `json:"actor"`
		}{Typename: "UnlabeledEvent", CreatedAt: e.Created, Label: e.Label, Actor: e.Actor})
	}
	return sp, nil
}

func (env *Env) restOpenPayload(ctx context.Context) (any, error) {
	pulls, err := env.GitHub.openPullsREST(ctx)
	if err != nil {
		return nil, err
	}
	nodes := make([]stackPR, 0, len(pulls))
	for _, p := range pulls {
		nodes = append(nodes, p.asStack())
	}
	return map[string]any{"repository": map[string]any{"open": map[string]any{"nodes": nodes}}}, nil
}

func (env *Env) restDraftPayload(ctx context.Context) (any, error) {
	open, err := env.GitHub.openPullsREST(ctx)
	if err != nil {
		return nil, err
	}
	var closed []restPull
	path := env.GitHub.repo("/pulls?state=closed&sort=updated&direction=desc&per_page=30")
	if err := env.GitHub.call(ctx, http.MethodGet, path, "", nil, &closed); err != nil {
		return nil, err
	}
	return map[string]any{"repository": map[string]any{
		"drafts": map[string]any{"nodes": draftsFrom(open)},
		"closed": map[string]any{"nodes": draftsFrom(closed)},
	}}, nil
}

type restEvent struct {
	Event   string    `json:"event"`
	Created time.Time `json:"created_at"`
	Label   gqlName   `json:"label"`
	Actor   gqlActor  `json:"actor"`
}

type restTimeline struct {
	Event  string `json:"event"`
	Source struct {
		Issue struct {
			Number        int       `json:"number"`
			RepositoryURL string    `json:"repository_url"`
			Pull          *struct{} `json:"pull_request"`
		} `json:"issue"`
	} `json:"source"`
}

func (g *GitHub) issueEvents(ctx context.Context, n int) ([]restEvent, error) {
	return pages[restEvent](ctx, g, g.repo("/issues/%d/events?", n))
}

func (g *GitHub) issueTimeline(ctx context.Context, n int) ([]restTimeline, error) {
	return pages[restTimeline](ctx, g, g.repo("/issues/%d/timeline?", n))
}

func (g *GitHub) committedAt(ctx context.Context, sha string) (time.Time, error) {
	if sha == "" {
		return time.Time{}, nil
	}
	var commit struct {
		Commit struct {
			Committer struct {
				Date time.Time `json:"date"`
			} `json:"committer"`
		} `json:"commit"`
	}
	err := g.call(ctx, http.MethodGet, g.repo("/commits/%s", sha), "", nil, &commit)
	return commit.Commit.Committer.Date, err
}

func (env *Env) restWatchPayload(ctx context.Context) (any, error) {
	pulls, err := env.GitHub.openPullsREST(ctx)
	if err != nil {
		return nil, err
	}
	nodes := make([]map[string]any, 0, len(pulls))
	for _, p := range pulls {
		node, err := env.watchNode(ctx, p)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}
	var listed []restPull
	path := env.GitHub.repo("/pulls?state=all&sort=updated&direction=desc&per_page=30")
	if err := env.GitHub.call(ctx, http.MethodGet, path, "", nil, &listed); err != nil {
		return nil, err
	}
	drafts := draftsFrom(listed)
	slices.SortFunc(drafts, func(a, b queueDraft) int { return a.UpdatedAt.Compare(b.UpdatedAt) })
	return map[string]any{"repository": map[string]any{
		"pullRequests": map[string]any{"nodes": nodes},
		"drafts":       map[string]any{"nodes": drafts},
	}}, nil
}

func (env *Env) watchNode(ctx context.Context, p restPull) (map[string]any, error) {
	events, err := env.GitHub.issueEvents(ctx, p.Number)
	if err != nil {
		return nil, err
	}
	var timeline []any
	for _, e := range events {
		if e.Event != "unlabeled" {
			continue
		}
		timeline = append(timeline, map[string]any{
			"createdAt": e.Created, "label": e.Label, "actor": e.Actor,
		})
	}
	return map[string]any{
		"number": p.Number, "body": p.Body, "headRefName": p.Head.Ref, "baseRefName": p.Base.Ref,
		"headRefOid": p.Head.SHA, "isDraft": p.Draft, "labels": map[string]any{"nodes": p.Labels},
		"commits": map[string]any{"nodes": []any{
			map[string]any{"commit": map[string]any{"oid": p.Head.SHA}},
		}},
		"timelineItems": map[string]any{"nodes": timeline},
	}, nil
}

func (env *Env) restTimelinePayload(ctx context.Context, query string) (any, error) {
	repo := map[string]any{}
	for _, m := range regexp.MustCompile(`issue\(number:(\d+)\)`).FindAllStringSubmatch(query, -1) {
		n, _ := strconv.Atoi(m[1])
		nodes, err := env.crossRefs(ctx, n)
		if err != nil {
			return nil, err
		}
		repo["t"+m[1]] = map[string]any{"timelineItems": map[string]any{"nodes": nodes}}
	}
	return map[string]any{"repository": repo}, nil
}

func (env *Env) crossRefs(ctx context.Context, ticket int) ([]any, error) {
	events, err := env.GitHub.issueTimeline(ctx, ticket)
	if err != nil {
		return nil, err
	}
	var nodes []any
	for _, e := range events {
		if e.Event != "cross-referenced" || e.Source.Issue.Pull == nil {
			continue
		}
		if e.Source.Issue.RepositoryURL != "" &&
			e.Source.Issue.RepositoryURL != "https://api.github.com/repos/"+env.GitHub.Repo {
			continue
		}
		pr, err := env.pullAsGQL(ctx, e.Source.Issue.Number)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, map[string]any{"source": pr})
	}
	return nodes, nil
}

func (env *Env) pullAsGQL(ctx context.Context, n int) (gqlPR, error) {
	p, err := env.GitHub.restPull(ctx, n)
	if err != nil {
		return gqlPR{}, err
	}
	events, err := env.GitHub.issueEvents(ctx, n)
	if err != nil {
		return gqlPR{}, err
	}
	when, err := env.GitHub.committedAt(ctx, p.Head.SHA)
	if err != nil {
		return gqlPR{}, err
	}
	return p.asGQL(events, when), nil
}

func (p restPull) asGQL(events []restEvent, committed time.Time) gqlPR {
	g := gqlPR{
		Number: p.Number, Body: p.Body, CreatedAt: p.CreatedAt, State: p.graphState(), HeadOID: p.Head.SHA,
	}
	if p.MergedAt != nil {
		g.MergedAt = *p.MergedAt
	}
	if p.ClosedAt != nil {
		g.ClosedAt = *p.ClosedAt
	}
	g.Labels.Nodes = append(g.Labels.Nodes, p.Labels...)
	g.Commits.Nodes = append(g.Commits.Nodes, struct {
		Commit gqlCommit `json:"commit"`
	}{Commit: gqlCommit{OID: p.Head.SHA, CommittedDate: committed}})
	for _, e := range events {
		if kind := eventKind(e.Event); kind != "" {
			g.TimelineItems.Nodes = append(g.TimelineItems.Nodes, struct {
				Typename  string    `json:"__typename"`
				CreatedAt time.Time `json:"createdAt"`
				Label     gqlName   `json:"label"`
				Actor     gqlActor  `json:"actor"`
			}{Typename: kind, CreatedAt: e.Created, Label: e.Label, Actor: e.Actor})
		}
	}
	return g
}

func eventKind(event string) string {
	switch event {
	case "labeled":
		return "LabeledEvent"
	case "unlabeled":
		return "UnlabeledEvent"
	default:
		return ""
	}
}

func draftsFrom(pulls []restPull) []queueDraft {
	nodes := make([]queueDraft, 0, len(pulls))
	for _, p := range pulls {
		if !strings.HasPrefix(p.Head.Ref, draftPrefix) {
			continue
		}
		d := queueDraft{
			Number: p.Number, State: p.graphState(), Title: p.Title, Body: p.Body,
			HeadRefName: p.Head.Ref, HeadRefOID: p.Head.SHA, UpdatedAt: p.UpdatedAt,
		}
		if p.ClosedAt != nil {
			d.ClosedAt = *p.ClosedAt
		}
		d.Commits.Nodes = append(d.Commits.Nodes, struct {
			Commit gqlCommit `json:"commit"`
		}{Commit: gqlCommit{OID: p.Head.SHA}})
		nodes = append(nodes, d)
	}
	return nodes
}

func (env *Env) restFillChecks(ctx context.Context, commits []*gqlCommit) error {
	for _, c := range commits {
		nodes, err := env.GitHub.checkContexts(ctx, c.OID)
		if err != nil {
			return err
		}
		if c.StatusCheckRollup == nil {
			c.StatusCheckRollup = &gqlRollup{}
		}
		c.StatusCheckRollup.Contexts.Nodes = nodes
		c.StatusCheckRollup.Contexts.PageInfo.HasNextPage = false
		c.StatusCheckRollup.Contexts.PageInfo.EndCursor = ""
	}
	return nil
}

func (g *GitHub) checkContexts(ctx context.Context, sha string) ([]gqlContext, error) {
	if sha == "" {
		return nil, nil
	}
	var out []gqlContext
	for page := 1; ; page++ {
		var body struct {
			CheckRuns []struct {
				ID          int64     `json:"id"`
				Name        string    `json:"name"`
				Status      string    `json:"status"`
				Conclusion  string    `json:"conclusion"`
				CompletedAt time.Time `json:"completed_at"`
				DetailsURL  string    `json:"details_url"`
			} `json:"check_runs"`
		}
		path := g.repo("/commits/%s/check-runs?per_page=%d&filter=all&page=%d", sha, pageSize, page)
		if err := g.call(ctx, http.MethodGet, path, "", nil, &body); err != nil {
			return nil, err
		}
		for _, run := range body.CheckRuns {
			out = append(out, gqlContext{
				Name: run.Name, Status: strings.ToUpper(run.Status), Conclusion: strings.ToUpper(run.Conclusion),
				CompletedAt: run.CompletedAt, DatabaseID: run.ID, DetailsURL: run.DetailsURL,
			})
		}
		if len(body.CheckRuns) < pageSize {
			break
		}
	}
	var status struct {
		Statuses []struct {
			Context   string    `json:"context"`
			State     string    `json:"state"`
			CreatedAt time.Time `json:"created_at"`
		} `json:"statuses"`
	}
	if err := g.call(ctx, http.MethodGet, g.repo("/commits/%s/status", sha), "", nil, &status); err != nil {
		return nil, err
	}
	for _, s := range status.Statuses {
		out = append(out, gqlContext{Context: s.Context, State: strings.ToUpper(s.State), CreatedAt: s.CreatedAt})
	}
	return out, nil
}
