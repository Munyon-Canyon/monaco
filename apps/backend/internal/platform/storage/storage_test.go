package storage

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

type handlerTrip struct{ h http.Handler }

func (p handlerTrip) RoundTrip(r *http.Request) (*http.Response, error) {
	rec := httptest.NewRecorder()
	p.h.ServeHTTP(rec, r)
	if err := r.Context().Err(); err != nil {
		return nil, err
	}
	return rec.Result(), nil
}

type downTrip struct{}

func (downTrip) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errs.New(errs.CodeUpstreamUnavailable, "storage.down")
}

type badListTrip struct{ h http.Handler }

func (t badListTrip) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/object/list/") {
		return &http.Response{
			StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("not json")), Header: make(http.Header),
		}, nil
	}
	return handlerTrip(t).RoundTrip(r)
}

func configured(url, key string, opts ...httpclient.Option) (*Client, error) {
	return New(config.Config{
		Supabase: config.Supabase{URL: url, ServiceRoleKey: key},
		Timeouts: config.Timeouts{Storage: time.Second},
	}, opts...)
}

func liveClient(t *testing.T) (*Client, http.Handler) {
	t.Helper()
	h := fakes.New()
	c, err := configured("http://127.0.0.1:9", "service-role", httpclient.WithTransport(handlerTrip{h}))
	if err != nil {
		t.Fatal(err)
	}
	return c, h
}

func mustCode(t *testing.T, err error, want errs.Code) {
	t.Helper()
	if errs.CodeOf(err) != want {
		t.Fatalf("code = %v, err = %v, want %v", errs.CodeOf(err), err, want)
	}
}

func readPublic(t *testing.T, h http.Handler, raw string) (int, string, []byte) {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, raw, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	body, err := io.ReadAll(rec.Result().Body)
	if err != nil {
		t.Fatal(err)
	}
	return rec.Code, rec.Header().Get("Content-Type"), body
}

func scriptFail(t *testing.T, h http.Handler, route string, status int) {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/_script", bytes.NewBufferString(
		`{"route":"`+route+`","action":"fail","status":`+itoa(status)+`}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("script = %d %s", rec.Code, rec.Body.String())
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [4]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func TestPutAndDeletePrefix_roundTripsTheObject(t *testing.T) {
	t.Parallel()
	c, h := liveClient(t)
	body := []byte{0x89, 'P', 'N', 'G'}
	public, err := c.Put(t.Context(), "avatars", "user/Az9-_.png", "image/png", body)
	if err != nil {
		t.Fatal(err)
	}
	wantURL := "http://127.0.0.1:9/storage/v1/object/public/avatars/user/Az9-_.png"
	if public != wantURL {
		t.Fatalf("public url = %s", public)
	}
	status, ctype, got := readPublic(t, h, public)
	if status != http.StatusOK || ctype != "image/png" || !bytes.Equal(got, body) {
		t.Fatalf("public get = %d %s %q", status, ctype, got)
	}
	if _, err := c.Put(t.Context(), "avatars", "other/keep.png", "image/png", body); err != nil {
		t.Fatal(err)
	}
	if err := c.DeletePrefix(t.Context(), "avatars", "user"); err != nil {
		t.Fatal(err)
	}
	if status, _, _ := readPublic(t, h, public); status != http.StatusNotFound {
		t.Fatalf("deleted object status = %d", status)
	}
	kept := "http://127.0.0.1:9/storage/v1/object/public/avatars/other/keep.png"
	if status, _, got := readPublic(t, h, kept); status != http.StatusOK || !bytes.Equal(got, body) {
		t.Fatalf("kept object = %d %q", status, got)
	}
}

func TestProfilePhotos_storesAndDeletesAUsersPhotos(t *testing.T) {
	t.Parallel()
	c, h := liveClient(t)
	userID, err := ids.ParseUserID("019c1da5-8000-7000-8000-000000000001")
	if err != nil {
		t.Fatal(err)
	}
	photos := ProfilePhotos{Storage: c}
	url, err := photos.Put(t.Context(), userID.String()+"/photo.png", "image/png", []byte{1})
	if err != nil {
		t.Fatal(err)
	}
	if err := photos.DeleteAll(t.Context(), userID); err != nil {
		t.Fatal(err)
	}
	if status, _, _ := readPublic(t, h, url); status != http.StatusNotFound {
		t.Fatalf("photo status = %d, want 404", status)
	}
}

func TestDeletePrefix_pagesAndSkipsNestedObjects(t *testing.T) {
	t.Parallel()
	c, h := liveClient(t)
	for i := range 1001 {
		if _, err := c.Put(t.Context(), "avatars", "user/"+itoa(i)+".png", "image/png", []byte{1}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := c.Put(t.Context(), "avatars", "user/nested/keep.png", "image/png", []byte{1}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Put(t.Context(), "avatars", "other/keep.png", "image/png", []byte{1}); err != nil {
		t.Fatal(err)
	}
	if err := c.DeletePrefix(t.Context(), "avatars", "user"); err != nil {
		t.Fatal(err)
	}
	nested := "http://127.0.0.1:9/storage/v1/object/public/avatars/user/nested/keep.png"
	if status, _, _ := readPublic(t, h, nested); status != http.StatusOK {
		t.Fatal(status)
	}
	for _, name := range []string{"0.png", "999.png", "1000.png"} {
		url := "http://127.0.0.1:9/storage/v1/object/public/avatars/user/" + name
		if status, _, _ := readPublic(t, h, url); status != http.StatusNotFound {
			t.Fatal(status)
		}
	}
	other := "http://127.0.0.1:9/storage/v1/object/public/avatars/other/keep.png"
	if status, _, _ := readPublic(t, h, other); status != http.StatusOK {
		t.Fatal(status)
	}
}

func TestDeletePrefix_emptyListAndDeleteFailure(t *testing.T) {
	t.Parallel()
	c, h := liveClient(t)
	if err := c.DeletePrefix(t.Context(), "avatars", "nobody"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Put(t.Context(), "avatars", "user/a.png", "image/png", []byte{1}); err != nil {
		t.Fatal(err)
	}
	scriptFail(t, h, "/storage/v1/object/avatars", http.StatusBadGateway)
	mustCode(t, c.DeletePrefix(t.Context(), "avatars", "user"), errs.CodeUpstreamUnavailable)
	if _, err := c.Put(t.Context(), "avatars", "user/b.png", "image/png", []byte{1}); err != nil {
		t.Fatal(err)
	}
	scriptFail(t, h, "/storage/v1/object/avatars", http.StatusBadRequest)
	mustCode(t, c.DeletePrefix(t.Context(), "avatars", "user"), errs.CodeUpstreamUnavailable)
}

func TestDeletePrefix_rejectsInvalidListJSON(t *testing.T) {
	t.Parallel()
	c, err := configured("http://127.0.0.1:9", "service-role", httpclient.WithTransport(badListTrip{fakes.New()}))
	if err != nil {
		t.Fatal(err)
	}
	mustCode(t, c.DeletePrefix(t.Context(), "avatars", "user"), errs.CodeUpstreamUnavailable)
}

func TestPut_reportsStatusAndTransportFailures(t *testing.T) {
	t.Parallel()
	c, h := liveClient(t)
	scriptFail(t, h, "/storage/v1/object/avatars", http.StatusBadRequest)
	_, err := c.Put(t.Context(), "avatars", "user/a.png", "image/png", []byte{1})
	mustCode(t, err, errs.CodeUpstreamUnavailable)

	scriptFail(t, h, "/storage/v1/object/avatars", http.StatusBadGateway)
	_, err = c.Put(t.Context(), "avatars", "user/a.png", "image/png", []byte{1})
	mustCode(t, err, errs.CodeUpstreamUnavailable)

	down, err := configured("http://127.0.0.1:9", "service-role", httpclient.WithTransport(downTrip{}))
	if err != nil {
		t.Fatal(err)
	}
	_, err = down.Put(t.Context(), "avatars", "user/a.png", "image/png", []byte{1})
	mustCode(t, err, errs.CodeUpstreamUnavailable)
}

func TestDeletePrefix_reportsStatusAndTransportFailures(t *testing.T) {
	t.Parallel()
	c, h := liveClient(t)
	scriptFail(t, h, "/storage/v1/object/list", http.StatusBadRequest)
	mustCode(t, c.DeletePrefix(t.Context(), "avatars", "user"), errs.CodeUpstreamUnavailable)

	scriptFail(t, h, "/storage/v1/object/list", http.StatusBadGateway)
	mustCode(t, c.DeletePrefix(t.Context(), "avatars", "user"), errs.CodeUpstreamUnavailable)

	down, err := configured("http://127.0.0.1:9", "service-role", httpclient.WithTransport(downTrip{}))
	if err != nil {
		t.Fatal(err)
	}
	mustCode(t, down.DeletePrefix(t.Context(), "avatars", "user"), errs.CodeUpstreamUnavailable)
}

func TestNew_disabledWhenUnsetAndRejectsAPartialConfig(t *testing.T) {
	t.Parallel()
	c, err := New(config.Config{})
	if err != nil || c == nil || c.http != nil {
		t.Fatalf("disabled = %#v %v", c, err)
	}
	rejects := []config.Config{
		{Supabase: config.Supabase{URL: "https://x.supabase.co"}},
		{Supabase: config.Supabase{ServiceRoleKey: "k"}},
		{Supabase: config.Supabase{URL: "https://x.supabase.co", ServiceRoleKey: "k"}},
		{
			Supabase: config.Supabase{URL: "https://x.supabase.co", ServiceRoleKey: "k"},
			Timeouts: config.Timeouts{Storage: -time.Second},
		},
		{
			Supabase: config.Supabase{URL: "http://example.com", ServiceRoleKey: "k"},
			Timeouts: config.Timeouts{Storage: time.Second},
		},
		{
			Supabase: config.Supabase{URL: "http://8.8.8.8", ServiceRoleKey: "k"},
			Timeouts: config.Timeouts{Storage: time.Second},
		},
		{
			Supabase: config.Supabase{URL: "ftp://127.0.0.1", ServiceRoleKey: "k"},
			Timeouts: config.Timeouts{Storage: time.Second},
		},
		{
			Supabase: config.Supabase{URL: "https://", ServiceRoleKey: "k"},
			Timeouts: config.Timeouts{Storage: time.Second},
		},
		{
			Supabase: config.Supabase{URL: "http://[", ServiceRoleKey: "k"},
			Timeouts: config.Timeouts{Storage: time.Second},
		},
		{
			Supabase: config.Supabase{URL: "supabase.co", ServiceRoleKey: "k"},
			Timeouts: config.Timeouts{Storage: time.Second},
		},
	}
	for _, cfg := range rejects {
		if _, err := New(cfg); errs.CodeOf(err) != errs.CodeInvalidConfig {
			t.Fatalf("New(%+v) = %v", cfg.Supabase.URL, err)
		}
	}
}

func TestNew_acceptsHttpsLocalhostAndLoopback(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{
		"https://x.supabase.co/",
		"http://localhost:9",
		"http://127.0.0.1:9",
		"http://[::1]:9",
	} {
		c, err := configured(raw, "k")
		if err != nil || c.http == nil || c.base == raw && raw[len(raw)-1] == '/' {
			t.Fatalf("New(%s) = %v base %q", raw, err, c.base)
		}
	}
}

func TestPutAndDeletePrefix_rejectBadPathsAndAnUnconfiguredClient(t *testing.T) {
	t.Parallel()
	c, _ := liveClient(t)
	badPut := []struct{ bucket, key, ctype string }{
		{"avatars", "user/a.png", ""},
		{"", "user/a.png", "image/png"},
		{".", "user/a.png", "image/png"},
		{"..", "user/a.png", "image/png"},
		{"bad bucket", "user/a.png", "image/png"},
		{"avatars", "", "image/png"},
		{"avatars", "/user/a.png", "image/png"},
		{"avatars", "user/a.png/", "image/png"},
		{"avatars", "user//a.png", "image/png"},
		{"avatars", "user/../a.png", "image/png"},
		{"avatars", "user/./a.png", "image/png"},
		{"avatars", "user/{.png", "image/png"},
	}
	for _, tc := range badPut {
		_, err := c.Put(t.Context(), tc.bucket, tc.key, tc.ctype, []byte{1})
		mustCode(t, err, errs.CodeInvalidInput)
	}
	_, err := c.Put(t.Context(), "avatars", "user/a.png", "image/png", nil)
	mustCode(t, err, errs.CodeInvalidInput)
	for _, prefix := range []string{"", "/user", "user/", "user//x", "user/../x", "."} {
		mustCode(t, c.DeletePrefix(t.Context(), "avatars", prefix), errs.CodeInvalidInput)
	}
	mustCode(t, c.DeletePrefix(t.Context(), "", "user"), errs.CodeInvalidInput)

	off, err := New(config.Config{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = off.Put(t.Context(), "avatars", "user/a.png", "image/png", []byte{1})
	mustCode(t, err, errs.CodeUpstreamUnavailable)
	mustCode(t, off.DeletePrefix(t.Context(), "avatars", "user"), errs.CodeUpstreamUnavailable)
	var nilClient *Client
	_, err = nilClient.Put(context.Background(), "avatars", "user/a.png", "image/png", []byte{1})
	mustCode(t, err, errs.CodeUpstreamUnavailable)
	mustCode(t, nilClient.DeletePrefix(context.Background(), "avatars", "user"), errs.CodeUpstreamUnavailable)
}

func TestPutAndDeletePrefix_rejectARequestThatCannotBeBuilt(t *testing.T) {
	t.Parallel()
	httpClient := httpclient.New("storage", httpclient.WithTimeout(time.Second))
	c := &Client{http: httpClient, newRequest: func(context.Context, string, string, io.Reader) (*http.Request, error) {
		return nil, errs.New(errs.CodeInvalidInput, "storage.build")
	}}
	_, err := c.Put(context.Background(), "avatars", "user/a.png", "image/png", []byte{1})
	mustCode(t, err, errs.CodeInvalidInput)
	mustCode(t, c.DeletePrefix(context.Background(), "avatars", "user"), errs.CodeInvalidInput)
}
