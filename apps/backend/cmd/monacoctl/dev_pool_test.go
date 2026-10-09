package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestDevTokenNewUser_poolReusesItsUser(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	cfg := devNewUserConfig(t, pool, nil)
	run := func() (string, string) {
		var stdout, stderr bytes.Buffer
		if code := devTokenNewUser(cfg, "browse-host", time.Hour, &stdout, &stderr); code != 0 {
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

func TestDevTokenNewUser_poolRefusesABadName(t *testing.T) {
	t.Parallel()
	cfg := devNewUserConfig(t, testkit.DB(t), nil)
	var stdout, stderr bytes.Buffer
	if code := devTokenNewUser(cfg, "Bad_Name", time.Hour, &stdout, &stderr); code != 1 ||
		!strings.Contains(stderr.String(), "invalid_input") || stdout.Len() != 0 {
		t.Fatalf("exit %d stdout %q stderr %q", code, stdout.String(), stderr.String())
	}
}

func TestDevToken_poolNeedsUserNewAndTheLocalDevDatabase(t *testing.T) {
	t.Parallel()
	local := config.Config{
		Env: config.EnvLocal, DB: config.DB{URL: "postgres://postgres@localhost:1/monaco"},
		Auth: config.Auth{DevTokenKey: "test-key"},
	}
	var stdout, stderr bytes.Buffer
	if code := devCmd(local, []string{"token", "--user", devUserV7, "--pool", "x"}, &stdout, &stderr); code != 2 ||
		stderr.String() != devUsage+"\n" {
		t.Fatalf("--pool with an id: exit %d stderr %q", code, stderr.String())
	}
	for name, cfg := range map[string]config.Config{
		"test env":    {Env: config.EnvTest, DB: local.DB, Auth: local.Auth},
		"remote host": {Env: config.EnvLocal, DB: config.DB{URL: "postgres://u@db.supabase.co:5432/monaco"}, Auth: local.Auth},
		"staging":     {Env: config.EnvStaging, DB: local.DB, Auth: local.Auth},
	} {
		stderr.Reset()
		if code := devCmd(cfg, []string{"token", "--user", "new", "--pool", "x"}, &stdout, &stderr); code != 1 ||
			stderr.String() != devPoolRefused+"\n" || stdout.Len() != 0 {
			t.Errorf("%s: exit %d stdout %q stderr %q", name, code, stdout.String(), stderr.String())
		}
	}
	stderr.Reset()
	if code := devCmd(local, []string{"token", "--user", "new", "--pool", "x"}, &stdout, &stderr); code != 1 ||
		!strings.Contains(stderr.String(), "db.Open") {
		t.Errorf("local dev database that is down: exit %d stderr %q", code, stderr.String())
	}
}
