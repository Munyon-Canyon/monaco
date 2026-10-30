package verify

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

func (s *Stack) applySchema(ctx context.Context) error {
	dir := filepath.Join(s.opts.Dir, "migrations")
	cached, err := schemaDumpPath(s.opts.SchemaCache, dir)
	if err != nil {
		return err
	}
	if dump, err := os.ReadFile(cached); cached != "" && err == nil {
		return restoreSchema(ctx, s.DBURL, string(dump))
	}
	if err := migrate(ctx, s.opts.Atlas, dir, s.DBURL); err != nil {
		return err
	}
	if cached == "" || s.container == "" {
		return nil
	}
	dump, err := s.opts.Docker.run(ctx, "exec", s.container,
		"pg_dump", "--inserts", "--no-owner", "--no-privileges", "-U", "monaco", "monaco")
	if err != nil {
		return err
	}
	return writeAtomic(cached, withoutMetaCommands(dump))
}

func schemaDumpPath(cacheDir, migrations string) (string, error) {
	if cacheDir == "" {
		return "", nil
	}
	sum, err := os.ReadFile(filepath.Join(migrations, "atlas.sum"))
	if err != nil {
		return "", fmt.Errorf("schema cache key: %w", err)
	}
	key := sha256.Sum256(append([]byte(PostgresImage+"\n"), sum...))
	return filepath.Join(cacheDir, hex.EncodeToString(key[:8])+".sql"), nil
}

func withoutMetaCommands(dump string) string {
	lines := strings.Split(dump, "\n")
	kept := lines[:0]
	for _, line := range lines {
		if !strings.HasPrefix(line, `\`) {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "\n")
}

func restoreSchema(ctx context.Context, url, dump string) error {
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		return fmt.Errorf("restore cached schema: %w", err)
	}
	defer func() { _ = conn.Close(context.WithoutCancel(ctx)) }()
	if _, err := conn.Exec(ctx, dump); err != nil {
		return fmt.Errorf("restore cached schema: %w", err)
	}
	return nil
}

func writeAtomic(path, body string) error {
	tmp := path + ".tmp-" + strconv.Itoa(os.Getpid())
	err := os.MkdirAll(filepath.Dir(path), 0o750)
	if err == nil {
		err = os.WriteFile(tmp, []byte(body), 0o600)
	}
	if err == nil {
		err = os.Rename(tmp, path)
	}
	if err != nil {
		return fmt.Errorf("cache schema dump: %w", err)
	}
	return nil
}

func SchemaCacheDir() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "monaco", "verify-schema")
}
