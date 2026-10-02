package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
)

const (
	opPut          = "storage.Put"
	opDeletePrefix = "storage.DeletePrefix"
	objectPrefix   = "/storage/v1/object/"
	publicPrefix   = "/storage/v1/object/public/"
	listPrefix     = "/storage/v1/object/list/"
	listLimit      = 1000
)

type Client struct {
	base       string
	key        string
	http       *httpclient.Client
	newRequest func(context.Context, string, string, io.Reader) (*http.Request, error)
}

func New(cfg config.Config, opts ...httpclient.Option) (*Client, error) {
	base := strings.TrimRight(cfg.Supabase.URL, "/")
	key := cfg.Supabase.ServiceRoleKey
	if base == "" && key == "" {
		return &Client{}, nil
	}
	if base == "" || key == "" || cfg.Timeouts.Storage <= 0 || !reachable(base) {
		return nil, errs.New(errs.CodeInvalidConfig, "storage.New", slog.String("reason", "supabase storage config"))
	}
	client := httpclient.New("storage", append([]httpclient.Option{
		httpclient.WithBaseURL(base),
		httpclient.WithTimeout(cfg.Timeouts.Storage),
	}, opts...)...)
	return &Client{base: base, key: key, http: client, newRequest: http.NewRequestWithContext}, nil
}

func reachable(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return false
	}
	if u.Scheme == "https" {
		return true
	}
	ip := net.ParseIP(u.Hostname())
	return u.Scheme == "http" && (u.Hostname() == "localhost" || ip != nil && ip.IsLoopback())
}

func (c *Client) Put(ctx context.Context, bucket, key, contentType string, body []byte) (string, error) {
	if err := c.enabled(opPut); err != nil {
		return "", err
	}
	if contentType == "" || len(body) == 0 || !validPath(bucket, key) {
		return "", errs.New(errs.CodeInvalidInput, opPut, slog.String("bucket", bucket), slog.String("key", key))
	}
	req, err := c.newRequest(ctx, http.MethodPost, objectPrefix+bucket+"/"+escapeKey(key), bytes.NewReader(body))
	if err != nil {
		return "", errs.Wrap(err, errs.CodeInvalidInput, opPut)
	}
	req.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(body)), nil }
	req.Header.Set("Content-Type", contentType)
	c.authorize(req)
	resp, err := c.http.Do(ctx, req)
	if err != nil {
		return "", err
	}
	defer closeBody(resp)
	if resp.StatusCode/100 != 2 {
		return "", errs.New(errs.CodeUpstreamUnavailable, opPut, slog.Int("status", resp.StatusCode))
	}
	return c.base + publicPrefix + bucket + "/" + escapeKey(key), nil
}

func (c *Client) DeletePrefix(ctx context.Context, bucket, prefix string) error {
	if err := c.enabled(opDeletePrefix); err != nil {
		return err
	}
	if !validPath(bucket, prefix) {
		return errs.New(errs.CodeInvalidInput, opDeletePrefix, slog.String("bucket", bucket),
			slog.String("prefix", prefix))
	}
	names := []string{}
	for offset := 0; ; offset += listLimit {
		page, count, err := c.list(ctx, bucket, prefix, offset)
		if err != nil {
			return err
		}
		names = append(names, page...)
		if count < listLimit {
			break
		}
	}
	for start := 0; start < len(names); start += listLimit {
		if err := c.delete(ctx, bucket, names[start:min(start+listLimit, len(names))]); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) list(ctx context.Context, bucket, prefix string, offset int) ([]string, int, error) {
	payload, _ := json.Marshal(map[string]any{"prefix": prefix, "limit": listLimit, "offset": offset})
	resp, err := c.request(ctx, http.MethodPost, listPrefix+bucket, payload)
	if err != nil {
		return nil, 0, err
	}
	defer closeBody(resp)
	if resp.StatusCode/100 != 2 {
		return nil, 0, errs.New(errs.CodeUpstreamUnavailable, opDeletePrefix, slog.Int("status", resp.StatusCode))
	}
	var rows []struct {
		Name string  `json:"name"`
		ID   *string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rows); err != nil {
		return nil, 0, errs.Wrap(err, errs.CodeUpstreamUnavailable, opDeletePrefix)
	}
	names := make([]string, 0, len(rows))
	for _, row := range rows {
		if row.ID != nil {
			names = append(names, prefix+"/"+row.Name)
		}
	}
	return names, len(rows), nil
}

func (c *Client) delete(ctx context.Context, bucket string, names []string) error {
	payload, _ := json.Marshal(map[string]any{"prefixes": names})
	resp, err := c.request(ctx, http.MethodDelete, objectPrefix+bucket, payload)
	if err != nil {
		return err
	}
	defer closeBody(resp)
	if resp.StatusCode/100 != 2 {
		return errs.New(errs.CodeUpstreamUnavailable, opDeletePrefix, slog.Int("status", resp.StatusCode))
	}
	return nil
}

func (c *Client) request(ctx context.Context, method, path string, payload []byte) (*http.Response, error) {
	req, err := c.newRequest(ctx, method, path, bytes.NewReader(payload))
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInvalidInput, opDeletePrefix)
	}
	req.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(payload)), nil }
	req.Header.Set("Content-Type", "application/json")
	c.authorize(req)
	return c.http.Do(ctx, req)
}

func (c *Client) enabled(op string) error {
	if c == nil || c.http == nil {
		return errs.New(errs.CodeUpstreamUnavailable, op, slog.String("reason", "not_configured"))
	}
	return nil
}

func (c *Client) authorize(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+c.key)
	req.Header.Set("apikey", c.key)
}

func closeBody(resp *http.Response) {
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
	_ = resp.Body.Close()
}

func validPath(bucket, key string) bool {
	return segment(bucket) && key != "" && !strings.HasPrefix(key, "/") && !strings.HasSuffix(key, "/") &&
		!strings.Contains(key, "//") && everySegment(key)
}

func everySegment(key string) bool {
	for _, part := range strings.Split(key, "/") {
		if !segment(part) {
			return false
		}
	}
	return true
}

func segment(part string) bool {
	if part == "" || part == "." || part == ".." {
		return false
	}
	for _, r := range part {
		if !allowedRune(r) {
			return false
		}
	}
	return true
}

func allowedRune(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.'
}

func escapeKey(key string) string {
	parts := strings.Split(key, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}
