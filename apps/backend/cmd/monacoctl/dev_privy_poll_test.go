package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

func TestDevPrivyTransfer_pollsAPendingActionUntilItSucceeds(t *testing.T) {
	t.Parallel()
	var reads atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			_, _ = io.WriteString(w, `{"id":"a1","status":"pending"}`)
			return
		}
		status := "pending"
		if reads.Add(1) > 2 {
			status = "succeeded"
		}
		_, _ = io.WriteString(w, `{"id":"a1","status":"`+status+`"}`)
	}))
	t.Cleanup(srv.Close)
	cfg := config.Config{Env: config.EnvLocal, Privy: config.Privy{
		AppID: "app-fixture", AppSecret: "secret", BaseURL: srv.URL,
		VerificationKey:         fakes.PrivyVerificationKey(),
		AuthorizationPrivateKey: fakes.PrivyAuthorizationKeyConfig(),
		AuthorizationKeyID:      fakes.PrivyAuthorizationKeyID,
	}, Timeouts: config.Timeouts{Privy: 5 * time.Second}}
	var stdout, stderr bytes.Buffer
	code := devCmd(cfg, []string{
		"privy-transfer", "--wallet", "w", "--to", "0x00000000000000000000000000000000000000aa", "--usdc", "1",
		"--key", "k", "--poll", "1ms",
	}, &stdout, &stderr)
	if code != 0 || reads.Load() != 3 || strings.Count(stdout.String(), "status ") != 2 {
		t.Fatalf("exit %d after %d reads, stdout %s stderr %s", code, reads.Load(), stdout.String(), stderr.String())
	}
}
