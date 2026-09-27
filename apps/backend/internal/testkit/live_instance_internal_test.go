package testkit

import (
	"context"
	"crypto/rand"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestDropStaleLeavesAnotherRunsLiveInstance(t *testing.T) {
	t.Parallel()
	s := current.Load()
	DB(t)
	ctx := context.Background()
	live := s.template.Database + instanceMarker + "live" + strings.ToLower(rand.Text()[:8])
	ident := pgx.Identifier{live}.Sanitize()
	if _, err := s.admin.Exec(ctx, "CREATE DATABASE "+ident); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := s.admin.Exec(context.Background(), "DROP DATABASE IF EXISTS "+ident+" WITH (FORCE)"); err != nil {
			t.Error(err)
		}
	})
	cfg := s.admin.Config().ConnConfig.Copy()
	cfg.Database = live
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(ctx) }()
	stale := "testdb_tpl_testkitfixture_" + strings.ToLower(rand.Text()[:8])
	dropped, err := s.dropStale(ctx, time.Now().Add(time.Hour), stale, s.template.Database)
	if err != nil || !exists(t, s, live) {
		t.Fatalf("dropStale dropped %v, %v; another run's live instance %s must survive", dropped, err, live)
	}
}
