package agents

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestLoad_setsGitHubTimeoutToThirtySeconds(t *testing.T) {
	t.Parallel()
	env := newFixture(t).Env(t)
	if env.GitHub.HTTP.Timeout != 30*time.Second {
		t.Fatalf("timeout=%s", env.GitHub.HTTP.Timeout)
	}
}

func TestGitHub_treatsStatus300AsAnError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusMultipleChoices)
		_, _ = io.WriteString(w, `{"message":"multiple"}`)
	}))
	t.Cleanup(srv.Close)
	gh := &GitHub{
		API:   srv.URL,
		Repo:  testRepo,
		Token: func(context.Context) (string, error) { return "t", nil },
		HTTP:  srv.Client(),
	}
	_, err := gh.PR(context.Background(), 1)
	if err == nil || !strings.Contains(err.Error(), "300 Multiple Choices") {
		t.Fatalf("err=%v", err)
	}
}
