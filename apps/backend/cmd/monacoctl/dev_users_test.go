package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

func TestDevToken_poolReusesItsUser(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	cfg := devNewUserConfig(t, pool, nil)
	run := func() (string, string) {
		var stdout, stderr bytes.Buffer
		if code := devCmd(
			cfg,
			[]string{"token", "--user", "new", "--pool", "browse-host"},
			&stdout,
			&stderr,
		); code != 0 {
			t.Fatalf("exit %d, stderr %q", code, stderr.String())
		}
		var id, handle, wallet string
		if _, err := fmt.Sscanf(stderr.String(), "dev user %s @%s wallet %s\n", &id, &handle, &wallet); err != nil {
			t.Fatalf("stderr %q: %v", stderr.String(), err)
		}
		return id, handle
	}
	id, handle := run()
	again, _ := run()
	var users int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM users`).Scan(&users); err != nil ||
		again != id || handle != "dev_browse_host" || users != 1 {
		t.Fatalf("ids %s then %s, handle %s, %d users, %v", id, again, handle, users, err)
	}
}

func TestDevToken_poolNeedsUserNewAndAValidName(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	if code := devCmd(devConfig(config.EnvLocal), []string{"token", "--user", devUserV7, "--pool", "x"}, &stdout,
		&stderr); code != 2 || stderr.String() != devUsage+"\n" {
		t.Fatalf("--pool with an id: exit %d stderr %q", code, stderr.String())
	}
	cfg := devNewUserConfig(t, testkit.DB(t), nil)
	stderr.Reset()
	if code := devCmd(cfg, []string{"token", "--user", "new", "--pool", "Bad_Name"}, &stdout, &stderr); code != 1 ||
		!strings.Contains(stderr.String(), "invalid_input") || stdout.Len() != 0 {
		t.Fatalf("bad pool: exit %d stderr %q", code, stderr.String())
	}
}

func TestDevUserDelete_removesTheUserEverywhere(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	cfg := devNewUserConfig(t, pool, nil)
	_, id, _ := runDevTokenNew(t, cfg)
	var stdout, stderr bytes.Buffer
	if code := devCmd(cfg, []string{"user", "delete", id}, &stdout, &stderr); code != 0 ||
		stdout.String() != "deleted dev user "+id+"\n" {
		t.Fatalf("exit %d stdout %q stderr %q", code, stdout.String(), stderr.String())
	}
	assertNoDevRows(t, pool, 0)
	stdout.Reset()
	stderr.Reset()
	if code := devCmd(cfg, []string{"user", "delete", id}, &stdout, &stderr); code != 1 ||
		!strings.HasPrefix(stderr.String(), "monacoctl dev user delete: ") {
		t.Fatalf("second delete: exit %d stderr %q", code, stderr.String())
	}
}

func TestDevUserDelete_refusesBadArguments(t *testing.T) {
	t.Parallel()
	cfg := devConfig(config.EnvLocal)
	for name, args := range map[string][]string{
		"no id": {"user", "delete"}, "wrong verb": {"user", "drop", devUserV7}, "extra": {"user", "delete", devUserV7, "x"},
	} {
		var stdout, stderr bytes.Buffer
		if code := devCmd(cfg, args, &stdout, &stderr); code != 2 || stderr.String() != devUsage+"\n" {
			t.Errorf("%s: exit %d stderr %q", name, code, stderr.String())
		}
	}
	var stdout, stderr bytes.Buffer
	if code := devCmd(cfg, []string{"user", "delete", devUserV4}, &stdout, &stderr); code != 2 ||
		stderr.String() != devUserSubjectLine+"\n" {
		t.Errorf("v4 id: exit %d stderr %q", code, stderr.String())
	}
	if code := devCmd(cfg, []string{"user", "delete", devUserV7}, &stdout, &stderr); code != 1 {
		t.Errorf("no database: exit %d", code)
	}
}

type privyStub struct {
	mu      sync.Mutex
	deleted []string
	status  int
}

func (p *privyStub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.status != 0 {
		w.WriteHeader(p.status)
		return
	}
	if r.Method == http.MethodDelete {
		p.deleted = append(p.deleted, strings.TrimPrefix(r.URL.Path, "/privy/v1/users/"))
		w.WriteHeader(http.StatusNoContent)
		return
	}
	old := clock.Real{}.Now().Add(-48 * time.Hour).Unix()
	fresh := clock.Real{}.Now().Add(-time.Hour).Unix()
	_, _ = io.WriteString(w, fmt.Sprintf(`{"data":[
		{"id":"did:privy:old-dev","created_at":%[1]d,"linked_accounts":[{"type":"email","address":"dev-ab@example.com"}]},
		{"id":"did:privy:fresh-dev","created_at":%[2]d,"linked_accounts":[{"type":"email","address":"dev-cd@example.com"}]},
		{"id":"did:privy:real","created_at":%[1]d,"linked_accounts":[{"type":"email","address":"person@gmail.com"}]},
		{"id":"did:privy:linked-dev","created_at":%[1]d,"linked_accounts":[{"type":"email","address":"dev-ef@example.com"},
			{"type":"phone","number":"+14155550100"}]},
		{"id":"did:privy:no-email","created_at":%[1]d,"linked_accounts":[]}
	]}`, old, fresh))
}

func prunePrivy(t *testing.T, stub *privyStub) config.Config {
	t.Helper()
	srv := httptest.NewServer(stub)
	t.Cleanup(srv.Close)
	cfg := devConfig(config.EnvLocal)
	cfg.Privy = config.Privy{
		AppID: "app", AppSecret: "secret", BaseURL: srv.URL + "/privy", VerificationKey: fakes.PrivyVerificationKey(),
	}
	cfg.Timeouts.Privy = 5 * time.Second
	return cfg
}

func TestDevUsersPrune_listsOldDevOnlyUsersAndDeletesOnlyWithApply(t *testing.T) {
	t.Parallel()
	stub := &privyStub{}
	cfg := prunePrivy(t, stub)
	var stdout, stderr bytes.Buffer
	if code := devCmd(cfg, []string{"users", "prune", "--dry-run"}, &stdout, &stderr); code != 0 {
		t.Fatalf("dry run exit %d stderr %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "did:privy:old-dev dev-ab@example.com") ||
		!strings.HasSuffix(stdout.String(), "would delete 1 of 5 Privy users\n") || len(stub.deleted) != 0 {
		t.Fatalf("dry run stdout %q, deleted %v", stdout.String(), stub.deleted)
	}
	stdout.Reset()
	if code := devCmd(cfg, []string{"users", "prune", "--apply"}, &stdout, &stderr); code != 0 ||
		!strings.HasSuffix(stdout.String(), "deleted 1 of 5 Privy users\n") ||
		fmt.Sprint(stub.deleted) != "[did:privy:old-dev]" {
		t.Fatalf("apply stdout %q, deleted %v, stderr %q", stdout.String(), stub.deleted, stderr.String())
	}
}

func TestDevUsersPrune_refusesDeployedEnvs(t *testing.T) {
	t.Parallel()
	for _, env := range []config.Env{config.EnvStaging, config.EnvProduction} {
		stub := &privyStub{}
		cfg := prunePrivy(t, stub)
		cfg.Env = env
		var stdout, stderr bytes.Buffer
		code := devCmd(cfg, []string{"users", "prune", "--apply"}, &stdout, &stderr)
		want := "monacoctl dev users prune: refused with MONACO_ENV=" + string(env) + "\n"
		if code != 1 || stderr.String() != want || stdout.Len() != 0 || len(stub.deleted) != 0 {
			t.Errorf(
				"%s: exit %d stderr %q stdout %q deleted %v",
				env,
				code,
				stderr.String(),
				stdout.String(),
				stub.deleted,
			)
		}
	}
}

func TestDevUsersPrune_failsOnBadArgumentsAndPrivyErrors(t *testing.T) {
	t.Parallel()
	cfg := prunePrivy(t, &privyStub{})
	for name, args := range map[string][]string{
		"no verb": {"users"}, "wrong verb": {"users", "list"}, "neither": {"users", "prune"},
		"both": {"users", "prune", "--dry-run", "--apply"}, "extra": {"users", "prune", "--apply", "x"},
		"bad flag": {"users", "prune", "--nope"},
	} {
		var stdout, stderr bytes.Buffer
		if code := devCmd(cfg, args, &stdout, &stderr); code != 2 || stderr.String() != devUsage+"\n" {
			t.Errorf("%s: exit %d stderr %q", name, code, stderr.String())
		}
	}
	var stdout, stderr bytes.Buffer
	down := prunePrivy(t, &privyStub{status: http.StatusBadGateway})
	if code := devCmd(down, []string{"users", "prune", "--dry-run"}, &stdout, &stderr); code != 1 ||
		!strings.HasPrefix(stderr.String(), "monacoctl dev users prune: privy.ListUsers: ") {
		t.Errorf("list failure: exit %d stderr %q", code, stderr.String())
	}
	noKey := prunePrivy(t, &privyStub{})
	noKey.Privy.VerificationKey = ""
	stderr.Reset()
	if code := devCmd(noKey, []string{"users", "prune", "--dry-run"}, &stdout, &stderr); code != 1 ||
		!strings.Contains(stderr.String(), "privy.New") {
		t.Errorf("no key: exit %d stderr %q", code, stderr.String())
	}
}

func TestDevUsersPrune_stopsWhenADeleteFails(t *testing.T) {
	t.Parallel()
	stub := &privyStub{}
	cfg := prunePrivy(t, stub)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		stub.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	cfg.Privy.BaseURL = srv.URL + "/privy"
	var stdout, stderr bytes.Buffer
	if code := devCmd(cfg, []string{"users", "prune", "--apply"}, &stdout, &stderr); code != 1 ||
		!strings.Contains(stderr.String(), "privy.DeleteUser") {
		t.Errorf("exit %d stderr %q", code, stderr.String())
	}
}
