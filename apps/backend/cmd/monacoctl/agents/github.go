package agents

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	pageSize       = 100
	errorBodyLimit = 1024
)

type GitHub struct {
	API   string
	Repo  string
	Token func(ctx context.Context) (string, error)
	HTTP  *http.Client
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
	resp, err := g.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= http.StatusMultipleChoices {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, errorBodyLimit))
		return fmt.Errorf("%s %s: %s: %s", method, path, resp.Status, bytes.TrimSpace(data))
	}
	if out == nil {
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
