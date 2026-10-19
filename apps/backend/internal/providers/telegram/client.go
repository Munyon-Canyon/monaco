package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
)

const (
	DefaultBaseURL = "https://api.telegram.org"
	maxBody        = 1 << 20
	timeout        = 15 * time.Second
	masked         = "***"
)

type Client struct {
	http  *httpclient.Client
	token string
	chat  string
}

func New(c *httpclient.Client, token, chatID string) *Client {
	return &Client{http: c, token: token, chat: chatID}
}

func HTTP(base string, opts ...httpclient.Option) *httpclient.Client {
	return httpclient.New("telegram", append([]httpclient.Option{
		httpclient.WithBaseURL(base),
		httpclient.WithTimeout(timeout),
		httpclient.WithReturned(http.StatusTooManyRequests),
	}, opts...)...)
}

type message struct {
	ChatID  string  `json:"chat_id"`
	Text    string  `json:"text"`
	Preview preview `json:"link_preview_options"`
}

type preview struct {
	Disabled bool `json:"is_disabled"`
}

type failure struct {
	Description string `json:"description"`
	Parameters  struct {
		RetryAfter int `json:"retry_after"`
	} `json:"parameters"`
}

func (c *Client) Send(ctx context.Context, text string) error {
	const op = "telegram.Client.Send"
	body, _ := json.Marshal(message{ChatID: c.chat, Text: text, Preview: preview{Disabled: true}})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/bot"+c.token+"/sendMessage", bytes.NewReader(body))
	if err != nil {
		return errs.New(errs.CodeInvalidInput, op)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(ctx, req)
	if err != nil {
		return errs.New(errs.CodeOf(err), op, slog.String("cause", c.redact(err.Error())))
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	var f failure
	_ = json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(&f)
	code := errs.CodeUpstreamUnavailable
	if resp.StatusCode == http.StatusTooManyRequests {
		code = errs.CodeRateLimited
	}
	return errs.New(code, op, slog.Int("status", resp.StatusCode),
		slog.String("description", c.redact(f.Description)), slog.Int("retry_after", f.Parameters.RetryAfter))
}

func (c *Client) redact(s string) string { return strings.ReplaceAll(s, c.token, masked) }
