package gen_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/tools/gen"
)

func at(t *testing.T, stamp string) time.Time {
	t.Helper()
	ts, err := time.Parse("20060102150405", stamp)
	if err != nil {
		t.Fatal(err)
	}
	return ts
}

func TestNextPrefix_isNowUnlessTheNewestPrefixIsNotBehindIt(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		names []string
		now   string
		want  string
	}{
		"no migrations":              {nil, "20261002101530", "20261002101530"},
		"clock ahead of the newest":  {[]string{"20261002090000_a.sql", "20261001000000_b.sql"}, "20261002101530", "20261002101530"},
		"clock behind the newest":    {[]string{"20261002120003_referrals.sql", "20261001000000_b.sql"}, "20261002101530", "20261002120004"},
		"clock equal to the newest":  {[]string{"20261002101530_a.sql"}, "20261002101530", "20261002101531"},
		"carries across a minute":    {[]string{"20261002105959_a.sql"}, "20261002100000", "20261002110000"},
		"ignores files off the form": {[]string{"atlas.sum", "0003_x.sql"}, "20261002101530", "20261002101530"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := gen.NextPrefix(tc.names, at(t, tc.now)); got != tc.want {
				t.Fatalf("NextPrefix = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestNextPrefix_dropsSubSecondPrecisionAndConvertsToUTC(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 2, 5, 15, 30, 999_000_000, time.FixedZone("x", -5*3600))
	if got, want := gen.NextPrefix(nil, now), "20261002101530"; got != want {
		t.Fatalf("NextPrefix = %s, want %s", got, want)
	}
}

func TestRebasePlan_movesOnlyTheBranchsFilesAboveStagingInTheirOrder(t *testing.T) {
	t.Parallel()
	staging := []string{"20261001000000_a.sql", "20261002120003_referrals.sql"}
	for name, tc := range map[string]struct {
		onDisk []string
		now    string
		want   [][2]string
	}{
		"clock behind staging": {
			[]string{"20261002120002_social_b.sql", "20261002090000_social_a.sql", "20261001000000_a.sql", "20261002120003_referrals.sql"},
			"20261002101530",
			[][2]string{
				{"20261002090000_social_a.sql", "20261002120004_social_a.sql"},
				{"20261002120002_social_b.sql", "20261002120005_social_b.sql"},
			},
		},
		"clock ahead of staging": {
			[]string{"20261002090000_x.sql", "20261002090001_y.sql", "20261001000000_a.sql", "20261002120003_referrals.sql"},
			"20261002130000",
			[][2]string{
				{"20261002090000_x.sql", "20261002130000_x.sql"},
				{"20261002090001_y.sql", "20261002130001_y.sql"},
			},
		},
		"already above staging": {
			[]string{"20261002120004_x.sql", "20261001000000_a.sql", "20261002120003_referrals.sql"},
			"20261002130000",
			nil,
		},
		"own files share a suffix across staging's newest": {
			[]string{"20261002090000_social_x.sql", "20261002120004_social_x.sql", "20261001000000_a.sql", "20261002120003_referrals.sql"},
			"20261002101530",
			[][2]string{
				{"20261002090000_social_x.sql", "20261002120005_social_x.sql"},
				{"20261002120004_social_x.sql", "20261002120006_social_x.sql"},
			},
		},
		"no own files": {staging, "20261002130000", nil},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := gen.RebasePlan(tc.onDisk, staging, at(t, tc.now))
			if !slices.Equal(got, tc.want) {
				t.Fatalf("RebasePlan = %v, want %v", got, tc.want)
			}
		})
	}
}

func migrator(t *testing.T, files map[string]string, staging []string, now string) (gen.Migrator, *int) {
	t.Helper()
	files["go.mod"] = "module example.com/app\n"
	files["internal/modules/social/module.go"] = "package social\n"
	hashes := 0
	return gen.Migrator{
		Dir:     tree(t, files),
		Now:     func() time.Time { return at(t, now) },
		Staging: func(context.Context) ([]string, error) { return staging, nil },
		Hash:    func(context.Context) error { hashes++; return nil },
	}, &hashes
}

func listed(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(dir, "migrations"))
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

func TestMigrator_createWritesTheNewestPlusOneSecondWhenTheClockIsBehind(t *testing.T) {
	t.Parallel()
	m, hashes := migrator(t, map[string]string{"migrations/20261002120003_referrals.sql": "x"}, nil, "20261002101530")
	touched, err := m.Run(context.Background(), []string{"social", "follows"})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"migrations/20261002120004_social_follows.sql"}; !slices.Equal(touched, want) {
		t.Fatalf("touched = %v, want %v", touched, want)
	}
	if *hashes != 1 {
		t.Fatalf("atlas migrate hash ran %d times, want 1", *hashes)
	}
}

func TestMigrator_createUsesTheClockWhenItIsAhead(t *testing.T) {
	t.Parallel()
	m, _ := migrator(t, map[string]string{"migrations/20261002120003_referrals.sql": "x"}, nil, "20261002130000")
	touched, err := m.Run(context.Background(), []string{"social", "follows"})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"migrations/20261002130000_social_follows.sql"}; !slices.Equal(touched, want) {
		t.Fatalf("touched = %v, want %v", touched, want)
	}
}

func TestMigrator_createRefusesBadArgumentsWithoutWriting(t *testing.T) {
	t.Parallel()
	for name, args := range map[string][]string{
		"no arguments":   nil,
		"one argument":   {"social"},
		"unknown flag":   {"--now", "x"},
		"unknown module": {"nope", "follows"},
		"bad name":       {"social", "Follows"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			files := map[string]string{"migrations/20261002120003_referrals.sql": "x"}
			m, hashes := migrator(t, files, nil, "20261002130000")
			if _, err := m.Run(context.Background(), args); err == nil {
				t.Fatal("Run succeeded")
			}
			if got := listed(t, m.Dir); len(got) != 1 || *hashes != 0 {
				t.Fatalf("files = %v, hashes = %d; want the tree untouched", got, *hashes)
			}
		})
	}
}

func TestMigrator_rebaseRenamesOnlyTheBranchsFilesKeepingTheirBodiesAndOrder(t *testing.T) {
	t.Parallel()
	m, hashes := migrator(t, map[string]string{
		"migrations/20261001000000_a.sql":            "a",
		"migrations/20261002120003_referrals.sql":    "r",
		"migrations/20261002090000_social_first.sql": "first",
		"migrations/20261002120002_social_next.sql":  "next",
		"migrations/atlas.sum":                       "sum",
	}, []string{"20261001000000_a.sql", "20261002120003_referrals.sql"}, "20261002101530")
	touched, err := m.Run(context.Background(), []string{"--rebase"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"migrations/20261002120004_social_first.sql",
		"migrations/20261002120005_social_next.sql",
	}
	if !slices.Equal(touched, want) {
		t.Fatalf("touched = %v, want %v", touched, want)
	}
	if got, w := listed(t, m.Dir), []string{
		"20261001000000_a.sql", "20261002120003_referrals.sql", "20261002120004_social_first.sql",
		"20261002120005_social_next.sql", "atlas.sum",
	}; !slices.Equal(got, w) {
		t.Fatalf("files = %v, want %v", got, w)
	}
	if got := read(t, m.Dir, want[0]) + read(t, m.Dir, want[1]); got != "firstnext" {
		t.Fatalf("bodies = %q", got)
	}
	if *hashes != 1 {
		t.Fatalf("atlas migrate hash ran %d times, want 1", *hashes)
	}
}

func TestMigrator_rebaseRenamesNothingButRehashesWhenTheBranchAlreadySortsAboveStaging(t *testing.T) {
	t.Parallel()
	m, hashes := migrator(t, map[string]string{
		"migrations/20261002120003_referrals.sql": "r",
		"migrations/20261002120004_social_x.sql":  "x",
	}, []string{"20261002120003_referrals.sql"}, "20261002130000")
	touched, err := m.Run(context.Background(), []string{"--rebase"})
	if err != nil || len(touched) != 0 || *hashes != 1 {
		t.Fatalf("Run = %v, %v, hashes %d; want no renames and one rehash", touched, err, *hashes)
	}
}

func TestStagingMigrations_listsTheSqlFilesOfOriginStaging(t *testing.T) {
	t.Parallel()
	dir := tree(t, map[string]string{
		"migrations/20261001000000_a.sql": "a",
		"migrations/atlas.sum":            "sum",
		"other/20261001000001_b.sql":      "b",
	})
	for _, args := range [][]string{
		{"init", "-q"},
		{"add", "."},
		{"-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "-q", "-m", "staging"},
		{"update-ref", "refs/remotes/origin/staging", "HEAD"},
	} {
		cmd := exec.CommandContext(t.Context(), "git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	got, err := gen.StagingMigrations(dir)
	if want := []string{"20261001000000_a.sql"}; err != nil || !slices.Equal(got, want) {
		t.Fatalf("StagingMigrations = %v, %v; want %v", got, err, want)
	}
}

func TestStagingMigrations_failsOutsideAGitRepoWithOriginStaging(t *testing.T) {
	t.Parallel()
	if _, err := gen.StagingMigrations(t.TempDir()); err == nil {
		t.Fatal("StagingMigrations succeeded in a directory with no origin/staging")
	}
}

func referralsOnly() map[string]string {
	return map[string]string{"migrations/20261002120003_referrals.sql": "x"}
}

func createAndRebase() [][]string { return [][]string{{"--rebase"}, {"social", "follows"}} }

func TestMigrator_runFailsWhenTheBackendOrMigrationsDirIsMissing(t *testing.T) {
	t.Parallel()
	m, _ := migrator(t, referralsOnly(), nil, "20261002130000")
	m.Dir = filepath.Join(m.Dir, "missing")
	empty, _ := migrator(t, map[string]string{}, nil, "20261002130000")
	for _, m := range []gen.Migrator{m, empty} {
		for _, args := range createAndRebase() {
			if _, err := m.Run(context.Background(), args); err == nil {
				t.Fatalf("Run %v succeeded", args)
			}
		}
	}
}

func TestMigrator_rebaseStopsWhenStagingIsUnreadable(t *testing.T) {
	t.Parallel()
	boom := errs.New(errs.CodeInternal, "test.boom")
	m, hashes := migrator(t, referralsOnly(), nil, "20261002130000")
	m.Staging = func(context.Context) ([]string, error) { return nil, boom }
	if _, err := m.Run(context.Background(), []string{"--rebase"}); !errors.Is(err, boom) || *hashes != 0 {
		t.Fatalf("Run = %v, hashes %d; want boom and no hash", err, *hashes)
	}
}

func TestMigrator_runReturnsTheHashFailure(t *testing.T) {
	t.Parallel()
	boom := errs.New(errs.CodeInternal, "test.boom")
	m, _ := migrator(t, referralsOnly(), nil, "20261002130000")
	m.Hash = func(context.Context) error { return boom }
	for _, args := range createAndRebase() {
		if _, err := m.Run(context.Background(), args); !errors.Is(err, boom) {
			t.Fatalf("Run %v = %v, want boom", args, err)
		}
	}
}

func TestMigrator_createFailsWhenTheTargetNameIsTakenByADirectory(t *testing.T) {
	t.Parallel()
	m, hashes := migrator(t, referralsOnly(), nil, "20261002130000")
	taken := filepath.Join(m.Dir, "migrations", "20261002130000_social_follows.sql")
	m.Now = func() time.Time {
		if err := os.Mkdir(taken, 0o750); err != nil {
			t.Error(err)
		}
		return at(t, "20261002130000")
	}
	if _, err := m.Run(context.Background(), []string{"social", "follows"}); err == nil || *hashes != 0 {
		t.Fatalf("Run = %v, hashes %d; want a write error and no hash", err, *hashes)
	}
}

func TestMigrator_rebaseKeepsBothBodiesWhenOwnFilesShareASuffixAcrossStagingsNewest(t *testing.T) {
	t.Parallel()
	m, _ := migrator(t, map[string]string{
		"migrations/20261002120003_referrals.sql": "r",
		"migrations/20261002090000_social_x.sql":  "lower",
		"migrations/20261002120004_social_x.sql":  "upper",
	}, []string{"20261002120003_referrals.sql"}, "20261002101530")
	if _, err := m.Run(context.Background(), []string{"--rebase"}); err != nil {
		t.Fatal(err)
	}
	if got, w := listed(t, m.Dir), []string{
		"20261002120003_referrals.sql", "20261002120005_social_x.sql", "20261002120006_social_x.sql",
	}; !slices.Equal(got, w) {
		t.Fatalf("files = %v, want %v", got, w)
	}
	lower := read(t, m.Dir, "migrations/20261002120005_social_x.sql")
	upper := read(t, m.Dir, "migrations/20261002120006_social_x.sql")
	if lower != "lower" || upper != "upper" {
		t.Fatalf("bodies = %q, %q; want lower, upper", lower, upper)
	}
}

func TestMigrator_rebaseRefusesToRenameOntoAnExistingFile(t *testing.T) {
	t.Parallel()
	m, hashes := migrator(t, map[string]string{"migrations/20261001000000_social_a.sql": "a"}, nil, "20261002130000")
	m.Staging = func(context.Context) ([]string, error) { return []string{"20261002120003_referrals.sql"}, nil }
	taken := filepath.Join(m.Dir, "migrations", "20261002130000_social_a.sql")
	m.Now = func() time.Time {
		if err := os.WriteFile(taken, []byte("other"), 0o600); err != nil {
			t.Error(err)
		}
		return at(t, "20261002130000")
	}
	touched, err := m.Run(context.Background(), []string{"--rebase"})
	if errs.CodeOf(err) != errs.CodeInternal || len(touched) != 0 || *hashes != 0 {
		t.Fatalf("Run = %v, %v, hashes %d; want CodeInternal, nothing touched, no hash", touched, err, *hashes)
	}
	src := read(t, m.Dir, "migrations/20261001000000_social_a.sql")
	dst := read(t, m.Dir, "migrations/20261002130000_social_a.sql")
	if src != "a" || dst != "other" {
		t.Fatalf("bodies = %q, %q; want both files unchanged", src, dst)
	}
}

func TestMigrator_rebaseFailsWhenARenameSourceVanishes(t *testing.T) {
	t.Parallel()
	m, hashes := migrator(t, map[string]string{"migrations/20261001000000_social_a.sql": "a"}, nil, "20261002130000")
	m.Staging = func(context.Context) ([]string, error) { return []string{"20261002120003_referrals.sql"}, nil }
	m.Now = func() time.Time {
		if err := os.Remove(filepath.Join(m.Dir, "migrations", "20261001000000_social_a.sql")); err != nil {
			t.Error(err)
		}
		return at(t, "20261002130000")
	}
	_, err := m.Run(context.Background(), []string{"--rebase"})
	if errs.CodeOf(err) != errs.CodeInternal || *hashes != 0 {
		t.Fatalf("Run = %v, hashes %d; want a CodeInternal rename error and no hash", err, *hashes)
	}
}

func fakeAtlas(t *testing.T, path, script string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	body := []byte("#!/bin/sh\n" + script + "\n")
	if err := os.WriteFile(path, body, 0o700); err != nil {
		t.Fatal(err)
	}
}

func TestAtlasHash_runsMigrateHashOnTheMigrationsDirWithThePinnedBuildFirst(t *testing.T) {
	t.Parallel()
	top := t.TempDir()
	dir := filepath.Join(top, "apps", "backend")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	fakeAtlas(t, filepath.Join(top, ".bin", "atlas"), `echo "$@" > hashed.txt`)
	if err := gen.AtlasHash(dir); err != nil {
		t.Fatal(err)
	}
	if got, want := read(t, dir, "hashed.txt"), "migrate hash --dir file://migrations\n"; got != want {
		t.Fatalf("atlas args = %q, want %q", got, want)
	}
	fakeAtlas(t, filepath.Join(top, ".bin", "atlas"), `echo "no good" >&2; exit 3`)
	if err := gen.AtlasHash(dir); err == nil || !strings.Contains(err.Error(), "no good") {
		t.Fatalf("AtlasHash = %v, want the atlas output in the error", err)
	}
}

func TestAtlasHash_fallsBackToAtlasOnPath(t *testing.T) {
	bin := t.TempDir()
	fakeAtlas(t, filepath.Join(bin, "atlas"), `echo "$@" > hashed.txt`)
	t.Setenv("PATH", bin)
	dir := t.TempDir()
	if err := gen.AtlasHash(dir); err != nil {
		t.Fatal(err)
	}
	if got := read(t, dir, "hashed.txt"); !strings.HasPrefix(got, "migrate hash") {
		t.Fatalf("atlas args = %q", got)
	}
}
