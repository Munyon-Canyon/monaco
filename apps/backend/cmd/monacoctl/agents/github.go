package agents

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

var errGitHubStatus = errors.New("github status")

const (
	pageSize       = 100
	errorBodyLimit = 1024
)

type GitHub struct {
	API    string
	Repo   string
	Token  func(ctx context.Context) (string, error)
	HTTP   *http.Client
	rest   atomic.Bool
	encode func(any) ([]byte, error)
}
type Ref struct {
	Ref string `json:"ref"`
	SHA string `json:"sha"`
}
type PR struct {
	Number         int        `json:"number"`
	Title          string     `json:"title"`
	Body           string     `json:"body"`
	State          string     `json:"state"`
	UpdatedAt      time.Time  `json:"updated_at"`
	MergedAt       *time.Time `json:"merged_at"`
	ClosedAt       *time.Time `json:"closed_at"`
	MergeCommitSHA string     `json:"merge_commit_sha"`
	Head           Ref        `json:"head"`
	Base           Ref        `json:"base"`
}
type Issue struct {
	Number      int       `json:"number"`
	Body        string    `json:"body"`
	State       string    `json:"state"`
	StateReason string    `json:"state_reason"`
	UpdatedAt   time.Time `json:"updated_at"`
	PullRequest *struct{} `json:"pull_request"`
}
type File struct {
	Filename  string `json:"filename"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
	Patch     string `json:"patch"`
}

func (g *GitHub) PR(ctx context.Context, n int) (PR, error) {
	var pr PR
	err := g.call(ctx, http.MethodGet, g.repo("/pulls/%d", n), "", nil, &pr)
	return pr, err
}

func (g *GitHub) Issue(ctx context.Context, n int) (Issue, error) {
	var is Issue
	err := g.call(ctx, http.MethodGet, g.repo("/issues/%d", n), "", nil, &is)
	return is, err
}

func (g *GitHub) PRs(ctx context.Context, query string) ([]PR, error) {
	return pages[PR](ctx, g, g.repo("/pulls?%s", query))
}

func (g *GitHub) Files(ctx context.Context, n int) ([]File, error) {
	return pages[File](ctx, g, g.repo("/pulls/%d/files?", n))
}

func (g *GitHub) graphql(ctx context.Context, query string, out any) error {
	owner, name, _ := strings.Cut(g.Repo, "/")
	resp := struct {
		Data   any `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}{Data: out}
	body := map[string]any{"query": query, "variables": map[string]string{"owner": owner, "name": name}}
	if err := g.call(ctx, http.MethodPost, "/graphql", "", body, &resp); err != nil {
		return err
	}
	if len(resp.Errors) > 0 {
		return detailErr(errs.CodeUpstreamUnavailable, "monacoctl.agents.graphql", "graphql: "+resp.Errors[0].Message)
	}
	return nil
}

func (g *GitHub) repo(format string, a ...any) string {
	return "/repos/" + g.Repo + fmt.Sprintf(format, a...)
}

func pages[T any](ctx context.Context, g *GitHub, path string) ([]T, error) {
	var all []T
	for page := 1; ; page++ {
		var batch []T
		if err := g.call(
			ctx,
			http.MethodGet,
			fmt.Sprintf("%s&per_page=%d&page=%d", path, pageSize, page),
			"",
			nil,
			&batch,
		); err != nil {
			return nil, err
		}
		all = append(all, batch...)
		if len(batch) < pageSize {
			return all, nil
		}
	}
}

func (g *GitHub) call(ctx context.Context, method, path, auth string, body, out any) error {
	if auth == "" {
		t, err := g.Token(ctx)
		if err != nil {
			return fmt.Errorf("github token: %w", err)
		}
		auth = "token " + t
	}
	payload, err := encodeBody(method, path, body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, g.API+path, payload)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	req.Header.Set("Authorization", auth)
	req.Header.Set("Accept", "application/vnd.github+json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := g.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= http.StatusMultipleChoices {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, errorBodyLimit))
		return httpStatusError{method, path, resp.Status, string(bytes.TrimSpace(data))}
	}
	switch raw := out.(type) {
	case nil:
		return nil
	case *[]byte:
		if *raw, err = io.ReadAll(resp.Body); err != nil {
			return fmt.Errorf("read %s %s: %w", method, path, err)
		}
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode %s %s: %w", method, path, err)
	}
	return nil
}

func encodeBody(method, path string, body any) (io.Reader, error) {
	if body == nil {
		return bytes.NewReader(nil), nil
	}
	b, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encode %s %s: %w", method, path, err)
	}
	return bytes.NewReader(b), nil
}

type httpStatusError struct {
	method string
	path   string
	status string
	body   string
}

func (e httpStatusError) Error() string {
	return e.method + " " + e.path + ": " + e.status + ": " + e.body
}

func (e httpStatusError) Unwrap() error { return errGitHubStatus }

type Run struct {
	ID         int64     `json:"id"`
	WorkflowID int64     `json:"workflow_id"`
	Name       string    `json:"name"`
	Status     string    `json:"status"`
	Conclusion string    `json:"conclusion"`
	Attempt    int       `json:"run_attempt"`
	URL        string    `json:"html_url"`
	CreatedAt  time.Time `json:"created_at"`
}

func newestPerWorkflow(runs []Run) []Run {
	newest := map[int64]Run{}
	for _, r := range runs {
		cur, ok := newest[r.WorkflowID]
		if !ok || r.CreatedAt.After(cur.CreatedAt) || (r.CreatedAt.Equal(cur.CreatedAt) && r.ID > cur.ID) {
			newest[r.WorkflowID] = r
		}
	}
	out := make([]Run, 0, len(newest))
	for _, r := range runs {
		if newest[r.WorkflowID].ID == r.ID {
			out = append(out, r)
		}
	}
	return out
}

func (r Run) done() bool { return r.Status == "completed" }

func (r Run) broken() bool {
	return r.done() && (r.Conclusion == "cancelled" || r.Conclusion == "failure")
}

func (g *GitHub) Runs(ctx context.Context, sha string) ([]Run, error) {
	var all []Run
	for page := 1; ; page++ {
		var batch struct {
			Runs []Run `json:"workflow_runs"`
		}
		path := g.repo("/actions/runs?head_sha=%s&per_page=%d&page=%d", sha, pageSize, page)
		if err := g.call(ctx, http.MethodGet, path, "", nil, &batch); err != nil {
			return nil, err
		}
		all = append(all, batch.Runs...)
		if len(batch.Runs) < pageSize {
			return all, nil
		}
	}
}

func (g *GitHub) Rerun(ctx context.Context, id int64) error {
	err := g.call(ctx, http.MethodPost, g.repo("/actions/runs/%d/rerun", id), "", nil, nil)
	if err == nil {
		return nil
	}
	return g.call(ctx, http.MethodPost, g.repo("/actions/runs/%d/rerun-failed-jobs", id), "", nil, nil)
}
