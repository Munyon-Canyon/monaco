package x_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/providers/x"
)

const bearer = "bearer-secret"

const searchBody = `{
  "data": [
    {"id": "1002", "text": "Which app lets friends pool money for stocks?", "author_id": "u2",
     "created_at": "2026-10-18T09:58:00.000Z",
     "entities": {"urls": [{"expanded_url": "https://x.com/maria/status/1001/photo/1"}]}},
    {"id": "1001", "text": "wrote this up https://t.co/abc", "author_id": "u1",
     "created_at": "2026-10-18T09:55:00.000Z",
     "entities": {"urls": [{"expanded_url": "https://example.com/blog"}]}},
    {"id": "1000", "text": "author missing from includes", "author_id": "u9",
     "created_at": "2026-10-18T09:50:00.000Z"}
  ],
  "includes": {"users": [
    {"id": "u1", "username": "pablo", "public_metrics": {"followers_count": 1200}},
    {"id": "u2", "username": "maria", "public_metrics": {"followers_count": 80}}
  ]},
  "meta": {"newest_id": "1002", "result_count": 3}
}`

type sent struct {
	method string
	path   string
	query  url.Values
	auth   string
}

func serve(t *testing.T, h http.HandlerFunc) *x.Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	hc := x.HTTP(srv.URL)
	t.Cleanup(hc.CloseIdleConnections)
	return x.New(hc, bearer)
}

func reply(t *testing.T, status int, body string, requests chan<- sent) *x.Client {
	t.Helper()
	return serve(t, func(w http.ResponseWriter, r *http.Request) {
		if requests != nil {
			requests <- sent{method: r.Method, path: r.URL.Path, query: r.URL.Query(), auth: r.Header.Get("Authorization")}
		}
		w.Header().Set("x-rate-limit-reset", "1792300000")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	})
}

func detail(err error, key string) string {
	for _, a := range errs.Detail(err) {
		if a.Key == key {
			return a.Value.String()
		}
	}
	return ""
}

func TestSearch_AsksForRecentPostsWithAuthors(t *testing.T) {
	t.Parallel()
	requests := make(chan sent, 1)
	c := reply(t, http.StatusOK, searchBody, requests)
	start := time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)

	if _, err := c.Search(t.Context(), x.SearchRequest{Query: `"tokenized stocks" lang:en`, Start: start}); err != nil {
		t.Fatal(err)
	}

	got := <-requests
	if got.method != http.MethodGet || got.path != "/2/tweets/search/recent" || got.auth != "Bearer "+bearer {
		t.Fatalf("request = %+v, want an authenticated GET of recent search", got)
	}
	want := map[string]string{
		"query": `"tokenized stocks" lang:en`, "start_time": "2026-10-18T09:00:00Z", "expansions": "author_id",
		"user.fields": "username,public_metrics", "tweet.fields": "created_at,author_id,entities", "max_results": "100",
	}
	for key, value := range want {
		if got.query.Get(key) != value {
			t.Errorf("query %s = %q, want %q", key, got.query.Get(key), value)
		}
	}
	if got.query.Has("since_id") {
		t.Errorf("since_id sent with start_time: %v", got.query)
	}
}

func TestSearch_ReadsPostsWithTheirAuthors(t *testing.T) {
	t.Parallel()
	c := reply(t, http.StatusOK, searchBody, nil)

	page, err := c.Search(t.Context(), x.SearchRequest{Query: "xStocks", SinceID: "1"})
	if err != nil {
		t.Fatal(err)
	}

	want := []x.Post{
		{
			ID: "1002", Text: "Which app lets friends pool money for stocks?", Handle: "maria", Followers: 80,
			CreatedAt: time.Date(2026, 10, 18, 9, 58, 0, 0, time.UTC),
		},
		{
			ID: "1001", Text: "wrote this up https://t.co/abc", Handle: "pablo", Followers: 1200, Link: true,
			CreatedAt: time.Date(2026, 10, 18, 9, 55, 0, 0, time.UTC),
		},
		{
			ID: "1000", Text: "author missing from includes",
			CreatedAt: time.Date(2026, 10, 18, 9, 50, 0, 0, time.UTC),
		},
	}
	if page.Newest != "1002" || !slices.Equal(page.Posts, want) {
		t.Fatalf("page = %+v, want newest 1002 and posts %+v", page, want)
	}
}

func TestSearch_SinceIDReplacesStartTime(t *testing.T) {
	t.Parallel()
	requests := make(chan sent, 1)
	c := reply(t, http.StatusOK, `{"meta": {"result_count": 0}}`, requests)
	start := time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)

	page, err := c.Search(t.Context(), x.SearchRequest{Query: "xStocks", SinceID: "1002", Start: start})
	if err != nil {
		t.Fatal(err)
	}

	got := <-requests
	if got.query.Get("since_id") != "1002" || got.query.Has("start_time") {
		t.Errorf("query = %v, want since_id alone", got.query)
	}
	if len(page.Posts) != 0 || page.Newest != "" {
		t.Errorf("page = %+v, want it empty", page)
	}
}

func TestSearch_OnlyReadsRecentSearch(t *testing.T) {
	t.Parallel()
	requests := make(chan sent, 4)
	c := reply(t, http.StatusOK, searchBody, requests)

	for range 3 {
		if _, err := c.Search(t.Context(), x.SearchRequest{Query: "xStocks", SinceID: "1"}); err != nil {
			t.Fatal(err)
		}
	}

	close(requests)
	for got := range requests {
		if got.method != http.MethodGet || got.path != "/2/tweets/search/recent" {
			t.Fatalf("request = %s %s, want GET /2/tweets/search/recent", got.method, got.path)
		}
	}
}

func TestSearch_NamesTheFailure(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		status int
		body   string
		code   errs.Code
		title  string
	}{
		{
			"rate limited", http.StatusTooManyRequests, `{"title": "Too Many Requests", "detail": "slow down"}`,
			errs.CodeRateLimited, "Too Many Requests",
		},
		{
			"credits depleted", http.StatusPaymentRequired, `{"title": "CreditsDepleted", "detail": "add credits"}`,
			errs.CodeUpstreamUnavailable, "CreditsDepleted",
		},
		{"an html error page", http.StatusBadGateway, `<html>bad gateway</html>`, errs.CodeUpstreamUnavailable, ""},
		{"a body that is not a page", http.StatusOK, `{"data": "nope"}`, errs.CodeDecodeFailed, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := reply(t, tt.status, tt.body, nil)

			_, err := c.Search(t.Context(), x.SearchRequest{Query: "xStocks", SinceID: "1"})

			if errs.CodeOf(err) != tt.code {
				t.Fatalf("err = %v, want code %s", err, tt.code)
			}
			if got := detail(err, "title"); got != tt.title {
				t.Errorf("title = %q, want %q", got, tt.title)
			}
			if tt.status != http.StatusOK && detail(err, "status") != strconv.Itoa(tt.status) {
				t.Errorf("status = %q, want %d", detail(err, "status"), tt.status)
			}
		})
	}
}

func TestSearch_ReportsWhenTheRateLimitResets(t *testing.T) {
	t.Parallel()
	c := reply(t, http.StatusTooManyRequests, `{}`, nil)

	_, err := c.Search(t.Context(), x.SearchRequest{Query: "xStocks", SinceID: "1"})

	if got := detail(err, "rate_limit_reset"); got != "1792300000" {
		t.Fatalf("rate_limit_reset = %q, want the header value", got)
	}
}

func TestSearch_FailsWhenXIsUnreachable(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.NotFoundHandler())
	hc := x.HTTP(srv.URL)
	t.Cleanup(hc.CloseIdleConnections)
	srv.Close()

	_, err := x.New(hc, bearer).Search(t.Context(), x.SearchRequest{Query: "xStocks", SinceID: "1"})

	if errs.CodeOf(err) != errs.CodeUpstreamUnavailable {
		t.Fatalf("err = %v, want upstream_unavailable", err)
	}
}

func TestSearch_FailsWhenTheBodyIsCutShort(t *testing.T) {
	t.Parallel()
	c := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "500")
		_, _ = io.WriteString(w, `{"data":`)
	})

	_, err := c.Search(t.Context(), x.SearchRequest{Query: "xStocks", SinceID: "1"})

	if errs.CodeOf(err) != errs.CodeUpstreamUnavailable {
		t.Fatalf("err = %v, want upstream_unavailable", err)
	}
}

func TestDecode_RejectsGarbage(t *testing.T) {
	t.Parallel()
	if _, err := x.Decode([]byte("not json")); errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("err = %v, want decode_failed", err)
	}
}

func TestDecode_FlagsOnlyLinksOutsideX(t *testing.T) {
	t.Parallel()
	tests := []struct {
		expanded string
		link     bool
	}{
		{"https://example.com/post", true},
		{"https://x.com/maria/status/1", false},
		{"https://twitter.com/maria/status/1", false},
		{"https://pic.x.com/abc", false},
		{"https://www.twitter.com/maria", false},
		{"https://notx.com/post", true},
		{"https://example.com/?next=x.com", true},
		{"http://[::1", true},
	}
	for _, tt := range tests {
		t.Run(tt.expanded, func(t *testing.T) {
			t.Parallel()
			raw, err := jsonPost(tt.expanded)
			if err != nil {
				t.Fatal(err)
			}
			page, err := x.Decode(raw)
			if err != nil {
				t.Fatal(err)
			}
			if got := page.Posts[0].Link; got != tt.link {
				t.Errorf("Link = %t, want %t", got, tt.link)
			}
		})
	}
}

func TestPost_URLFallsBackToTheGenericForm(t *testing.T) {
	t.Parallel()
	if got := (x.Post{ID: "7", Handle: "maria"}).URL(); got != "https://x.com/maria/status/7" {
		t.Errorf("URL = %s", got)
	}
	if got := (x.Post{ID: "7"}).URL(); got != "https://x.com/i/status/7" {
		t.Errorf("URL without a handle = %s", got)
	}
}

func jsonPost(expanded string) ([]byte, error) {
	return json.Marshal(map[string]any{"data": []any{map[string]any{
		"id": "1", "text": "t", "author_id": "u1",
		"entities": map[string]any{"urls": []any{map[string]string{"expanded_url": expanded}}},
	}}})
}
