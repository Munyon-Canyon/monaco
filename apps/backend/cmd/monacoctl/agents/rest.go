package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
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
	return strings.Contains(query, "statusCheckRollup") && !strings.Contains(query, "pullRequest")
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
	ClosedAt       *time.Time `json:"closed_at"`
	MergedAt       *time.Time `json:"merged_at"`
	MergeCommitSHA *string    `json:"merge_commit_sha"`
	Mergeable      *bool      `json:"mergeable"`
	MergeableState string     `json:"mergeable_state"`
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

var (
	errNoRESTMapping = errors.New("no REST mapping for query")
	errMergeNoPull   = errors.New("merge query names no pull request")
)

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
	case strings.Contains(query, "potentialMergeCommit"):
		return "merge"
	case strings.Contains(query, "open: pullRequests"):
		return "open"
	case strings.Contains(query, "fragment pr on PullRequest"):
		return "stack"
	case strings.Contains(query, "drafts:"):
		return "drafts"
	default:
		return "query"
	}
}

func (env *Env) restPayload(ctx context.Context, query string) (any, error) {
	switch {
	case strings.Contains(query, "potentialMergeCommit"):
		return env.restMergePayload(ctx, query)
	case strings.Contains(query, "open: pullRequests"):
		return env.restOpenPayload(ctx)
	case strings.Contains(query, "fragment pr on PullRequest"):
		return env.restStackPayload(ctx, query)
	case strings.Contains(query, "drafts:"):
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
	return p.asStack(), nil
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

func (env *Env) restMergePayload(ctx context.Context, query string) (any, error) {
	nums := mergeNumbers(query)
	if len(nums) == 0 {
		return nil, errMergeNoPull
	}
	p, err := env.GitHub.restPull(ctx, nums[0])
	if err != nil {
		return nil, err
	}
	mergeable, parents, err := env.mergeParents(ctx, p)
	if err != nil {
		return nil, err
	}
	var commit any
	if parents != nil {
		commit = map[string]any{"parents": map[string]any{"nodes": parents}}
	}
	return map[string]any{"repository": map[string]any{"pullRequest": map[string]any{
		"mergeable": mergeable, "baseRefOid": p.Base.SHA, "potentialMergeCommit": commit,
	}}}, nil
}

func mergeNumbers(query string) []int {
	matches := regexp.MustCompile(`pullRequest\(number:(\d+)\)`).FindAllStringSubmatch(query, -1)
	nums := make([]int, 0, len(matches))
	for _, m := range matches {
		n, _ := strconv.Atoi(m[1])
		nums = append(nums, n)
	}
	return nums
}

func (env *Env) mergeParents(ctx context.Context, p restPull) (string, []gqlOID, error) {
	if p.MergeableState == "dirty" {
		return "CONFLICTING", nil, nil
	}
	if p.Mergeable == nil || !*p.Mergeable || p.MergeCommitSHA == nil || *p.MergeCommitSHA == "" {
		return "UNKNOWN", nil, nil
	}
	var commit struct {
		Parents []struct {
			SHA string `json:"sha"`
		} `json:"parents"`
	}
	if err := env.GitHub.call(
		ctx, http.MethodGet, env.GitHub.repo("/commits/%s", *p.MergeCommitSHA), "", nil, &commit,
	); err != nil {
		return "", nil, err
	}
	parents := make([]gqlOID, 0, len(commit.Parents))
	for _, parent := range commit.Parents {
		parents = append(parents, gqlOID{OID: parent.SHA})
	}
	return "MERGEABLE", parents, nil
}

func (env *Env) restDraftPayload(ctx context.Context) (any, error) {
	pulls, err := env.GitHub.openPullsREST(ctx)
	if err != nil {
		return nil, err
	}
	return env.draftNodes(pulls), nil
}

func (env *Env) draftNodes(pulls []restPull) any {
	nodes := make([]queueDraft, 0, len(pulls))
	for _, p := range pulls {
		if !strings.HasPrefix(p.Head.Ref, draftPrefix) {
			continue
		}
		nodes = append(nodes, queueDraft{
			Number: p.Number, State: p.graphState(), Title: p.Title, Body: p.Body,
			HeadRefName: p.Head.Ref, UpdatedAt: p.UpdatedAt,
		})
	}
	return map[string]any{"repository": map[string]any{"drafts": map[string]any{"nodes": nodes}}}
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
