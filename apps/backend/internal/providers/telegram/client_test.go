package telegram_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/providers/telegram"
)

const (
	botToken = "123456:ABC-secret"
	chatID   = "-1001234567890"
)

type sent struct {
	method      string
	path        string
	contentType string
	body        string
}

func serve(t *testing.T, status int, body string, requests chan<- sent) *telegram.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		if requests != nil {
			requests <- sent{
				method: r.Method, path: r.URL.Path, contentType: r.Header.Get("Content-Type"), body: string(raw),
			}
		}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	hc := telegram.HTTP(srv.URL)
	t.Cleanup(hc.CloseIdleConnections)
	return telegram.New(hc, botToken, chatID)
}

func detail(err error, key string) string {
	for _, a := range errs.Detail(err) {
		if a.Key == key {
			return a.Value.String()
		}
	}
	return ""
}

func TestSend_PostsPlainTextToTheChat(t *testing.T) {
	t.Parallel()
	requests := make(chan sent, 1)
	c := serve(t, http.StatusOK, `{"ok": true, "result": {}}`, requests)

	if err := c.Send(t.Context(), "@maria · score 7.5\nhttps://x.com/maria/status/1"); err != nil {
		t.Fatal(err)
	}

	got := <-requests
	if got.method != http.MethodPost || got.path != "/bot"+botToken+"/sendMessage" ||
		got.contentType != "application/json" {
		t.Fatalf("request = %+v, want a JSON POST of sendMessage", got)
	}
	var body struct {
		ChatID  string `json:"chat_id"`
		Text    string `json:"text"`
		Mode    string `json:"parse_mode"`
		Preview struct {
			Disabled bool `json:"is_disabled"`
		} `json:"link_preview_options"`
	}
	if err := json.Unmarshal([]byte(got.body), &body); err != nil {
		t.Fatal(err)
	}
	if body.ChatID != chatID || body.Text != "@maria · score 7.5\nhttps://x.com/maria/status/1" ||
		body.Mode != "" || !body.Preview.Disabled {
		t.Errorf("body = %+v, want the chat, the text, no parse mode and no link preview", body)
	}
}

func TestSend_NamesTheFailure(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		status      int
		body        string
		code        errs.Code
		description string
		retryAfter  string
	}{
		{
			"chat not found", http.StatusBadRequest, `{"ok": false, "description": "Bad Request: chat not found"}`,
			errs.CodeUpstreamUnavailable, "Bad Request: chat not found", "0",
		},
		{
			"flood wait", http.StatusTooManyRequests,
			`{"ok": false, "description": "Too Many Requests", "parameters": {"retry_after": 7}}`,
			errs.CodeRateLimited, "Too Many Requests", "7",
		},
		{
			"a token echoed back", http.StatusUnauthorized, `{"description": "bad token ` + botToken + `"}`,
			errs.CodeUpstreamUnavailable, "bad token ***", "0",
		},
		{"an html error page", http.StatusInternalServerError, `<html>`, errs.CodeUpstreamUnavailable, "", "0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := serve(t, tt.status, tt.body, nil)

			err := c.Send(t.Context(), "hello")

			if errs.CodeOf(err) != tt.code {
				t.Fatalf("err = %v, want code %s", err, tt.code)
			}
			if got := detail(err, "description"); got != tt.description {
				t.Errorf("description = %q, want %q", got, tt.description)
			}
			if got := detail(err, "retry_after"); got != tt.retryAfter {
				t.Errorf("retry_after = %q, want %q", got, tt.retryAfter)
			}
		})
	}
}

func TestSend_NeverPutsTheTokenInAnError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.NotFoundHandler())
	hc := telegram.HTTP(srv.URL)
	t.Cleanup(hc.CloseIdleConnections)
	srv.Close()

	err := telegram.New(hc, botToken, chatID).Send(t.Context(), "hello")

	if errs.CodeOf(err) != errs.CodeUpstreamUnavailable {
		t.Fatalf("err = %v, want upstream_unavailable", err)
	}
	if strings.Contains(err.Error(), botToken) || !strings.Contains(detail(err, "cause"), "***") {
		t.Errorf("error %q leaks the token or hides where the call failed", err)
	}
}

func TestSend_RefusesATokenThatCannotFormAURL(t *testing.T) {
	t.Parallel()
	hc := telegram.HTTP("http://127.0.0.1:1")
	t.Cleanup(hc.CloseIdleConnections)
	token := "123:ABC\nsecret"

	err := telegram.New(hc, token, chatID).Send(t.Context(), "hello")

	if errs.CodeOf(err) != errs.CodeInvalidInput || strings.Contains(err.Error(), "secret") {
		t.Fatalf("err = %v, want invalid_input without the token", err)
	}
}
