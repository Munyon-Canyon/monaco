package x

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
)

const (
	DefaultBaseURL = "https://api.x.com"
	searchPath     = "/2/tweets/search/recent"
	pageSize       = "100"
	maxBody        = 8 << 20
	timeout        = 15 * time.Second
)

type Post struct {
	ID        string
	Text      string
	Handle    string
	Followers int
	CreatedAt time.Time
	Link      bool
}

func (p Post) URL() string {
	handle := p.Handle
	if handle == "" {
		handle = "i"
	}
	return "https://x.com/" + handle + "/status/" + p.ID
}

type SearchRequest struct {
	Query   string
	SinceID string
	Start   time.Time
}

type Page struct {
	Posts  []Post
	Newest string
}

type Client struct {
	http  *httpclient.Client
	token string
}

func New(c *httpclient.Client, token string) *Client { return &Client{http: c, token: token} }

func HTTP(base string, opts ...httpclient.Option) *httpclient.Client {
	return httpclient.New("x", append([]httpclient.Option{
		httpclient.WithBaseURL(base),
		httpclient.WithTimeout(timeout),
		httpclient.WithReturned(http.StatusTooManyRequests),
	}, opts...)...)
}

func (c *Client) Search(ctx context.Context, r SearchRequest) (Page, error) {
	const op = "x.Client.Search"
	q := url.Values{
		"query":        {r.Query},
		"max_results":  {pageSize},
		"tweet.fields": {"created_at,author_id,entities"},
		"expansions":   {"author_id"},
		"user.fields":  {"username,public_metrics"},
	}
	if r.SinceID != "" {
		q.Set("since_id", r.SinceID)
	} else {
		q.Set("start_time", r.Start.UTC().Format(time.RFC3339))
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, searchPath+"?"+q.Encode(), nil)
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.http.Do(ctx, req)
	if err != nil {
		return Page{}, errs.Wrap(err, errs.CodeOf(err), op)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return Page{}, errs.Wrap(err, errs.CodeUpstreamUnavailable, op)
	}
	if resp.StatusCode != http.StatusOK {
		return Page{}, rejected(op, resp, body)
	}
	return Decode(body)
}

type problem struct {
	Title  string `json:"title"`
	Detail string `json:"detail"`
}

func rejected(op string, resp *http.Response, body []byte) error {
	var p problem
	_ = json.Unmarshal(body, &p)
	code := errs.CodeUpstreamUnavailable
	if resp.StatusCode == http.StatusTooManyRequests {
		code = errs.CodeRateLimited
	}
	return errs.New(code, op,
		slog.Int("status", resp.StatusCode), slog.String("title", p.Title), slog.String("detail", p.Detail),
		slog.String("rate_limit_reset", resp.Header.Get("x-rate-limit-reset")))
}

type searchWire struct {
	Data     []postWire `json:"data"`
	Includes struct {
		Users []userWire `json:"users"`
	} `json:"includes"`
	Meta struct {
		NewestID string `json:"newest_id"`
	} `json:"meta"`
}

type postWire struct {
	ID        string    `json:"id"`
	Text      string    `json:"text"`
	AuthorID  string    `json:"author_id"`
	CreatedAt time.Time `json:"created_at"`
	Entities  struct {
		URLs []struct {
			Expanded string `json:"expanded_url"`
		} `json:"urls"`
	} `json:"entities"`
}

type userWire struct {
	ID            string `json:"id"`
	Username      string `json:"username"`
	PublicMetrics struct {
		Followers int `json:"followers_count"`
	} `json:"public_metrics"`
}

func Decode(raw []byte) (Page, error) {
	var w searchWire
	if err := json.Unmarshal(raw, &w); err != nil {
		return Page{}, errs.Wrap(err, errs.CodeDecodeFailed, "x.Decode")
	}
	authors := make(map[string]userWire, len(w.Includes.Users))
	for _, u := range w.Includes.Users {
		authors[u.ID] = u
	}
	posts := make([]Post, len(w.Data))
	for i, d := range w.Data {
		a := authors[d.AuthorID]
		posts[i] = Post{
			ID: d.ID, Text: d.Text, Handle: a.Username, Followers: a.PublicMetrics.Followers,
			CreatedAt: d.CreatedAt, Link: hasOutboundLink(d),
		}
	}
	return Page{Posts: posts, Newest: w.Meta.NewestID}, nil
}

func hasOutboundLink(p postWire) bool {
	for _, link := range p.Entities.URLs {
		u, err := url.Parse(link.Expanded)
		if err != nil || !ownHost(strings.ToLower(u.Hostname())) {
			return true
		}
	}
	return false
}

func ownHost(host string) bool {
	for _, own := range [...]string{"x.com", "twitter.com"} {
		if host == own || strings.HasSuffix(host, "."+own) {
			return true
		}
	}
	return false
}
