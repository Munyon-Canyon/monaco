package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestCaldate_reportsCalendarDatesComparedWithTheClockOrStoredByRepos(t *testing.T) {
	t.Parallel()
	dir := writeTree(t, map[string]string{
		"go.mod": "module github.com/monaco/monaco/apps/backend\n\ngo 1.25\n",
		"internal/testkit/clock.go": "package testkit\n\nimport \"time\"\n\ntype Clock struct{}\n\n" +
			"func NewClock(time.Time) *Clock { return &Clock{} }\n\nfunc (c *Clock) Set(time.Time) {}\n",
		"internal/modules/cabal/repo/repo.go": "package repo\n\nimport \"time\"\n\ntype Repo struct{}\n\n" +
			"func (Repo) Stamp(at time.Time) {}\n",
		"internal/platform/db/db.go": "package db\n\nimport \"time\"\n\ntype Tx struct{}\n\n" +
			"func (Tx) Touch(at time.Time, n int) {}\n",
		"internal/prod/p.go": "package prod\n\nimport \"time\"\n\nvar Seed = time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)\n\n" +
			"func Fresh() bool { return Seed.After(time.Now()) }\n",
		"internal/f/f_test.go": "package f\n\nimport (\n\t\"testing\"\n\t\"time\"\n\n" +
			"\t\"github.com/monaco/monaco/apps/backend/internal/modules/cabal/repo\"\n" +
			"\t\"github.com/monaco/monaco/apps/backend/internal/platform/db\"\n" +
			"\t\"github.com/monaco/monaco/apps/backend/internal/testkit\"\n)\n\n" +
			"func TestExpiry(t *testing.T) {\n" +
			"\texpires := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)\n" +
			"\tif time.Now().After(expires) {\n\t\tt.Fatal(\"expired\")\n\t}\n}\n\n" +
			"func TestStoredAndCompared(t *testing.T) {\n" +
			"\tstamped := time.Date(2027, 1, 2, 0, 0, 0, 0, time.UTC)\n" +
			"\trepo.Repo{}.Stamp(stamped.Add(time.Hour))\n" +
			"\t_ = time.Since(time.Date(2028, 5, 6, 0, 0, 0, 0, time.UTC))\n" +
			"\t_ = time.Date(2029, 7, 8, 0, 0, 0, 0, time.UTC) == time.Now()\n}\n\n" +
			"func TestClocked(t *testing.T) {\n" +
			"\tstart := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)\n" +
			"\tc := testkit.NewClock(start)\n\tc.Set(time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC))\n" +
			"\t_ = start.Before(time.Now())\n}\n\n" +
			"func pair(time.Time) (int, error) { return 0, nil }\n\n" +
			"type holder struct{ at time.Time }\n\n" +
			"func TestShapes(t *testing.T) {\n" +
			"\tn, _ := pair(time.Now())\n\t_ = n\n\tvar h holder\n\th.at = time.Now()\n" +
			"\tnow := time.Now()\n\t_ = time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC).After(now)\n" +
			"\tyear := 2031\n\t_ = time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC).After(now)\n" +
			"\tdb.Tx{}.Touch(time.Date(2032, 1, 1, 0, 0, 0, 0, time.UTC), 1)\n}\n\n" +
			"func TestSentinel(t *testing.T) {\n" +
			"\t_ = time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC).Before(time.Now())\n" +
			"\t_ = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Hour)\n" +
			"\trepo.Repo{}.Stamp(time.Now())\n}\n",
	})
	code, stderr := nogoStderr(t, dir, "-caldate")
	f := filepath.Join(dir, "internal/f/f_test.go")
	src, err := os.ReadFile(f)
	if err != nil {
		t.Fatal(err)
	}
	msg := " compared with the clock; stamp it relative to time.Now() or a testkit clock"
	dates := []string{
		"time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)",
		"time.Date(2027, 1, 2, 0, 0, 0, 0, time.UTC)",
		"time.Date(2028, 5, 6, 0, 0, 0, 0, time.UTC)",
		"time.Date(2029, 7, 8, 0, 0, 0, 0, time.UTC)",
		"time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)",
		"time.Date(2032, 1, 1, 0, 0, 0, 0, time.UTC)",
	}
	want := make([]string, 0, len(dates))
	for _, date := range dates {
		offset := strings.Index(string(src), date)
		line := 1 + strings.Count(string(src[:offset]), "\n")
		col := offset - strings.LastIndex(string(src[:offset]), "\n")
		want = append(want, fmt.Sprintf("%s:%d:%d: calendar date %s%s", f, line, col, date, msg))
	}
	got := strings.Split(strings.TrimSpace(stderr), "\n")
	slices.Sort(got)
	slices.Sort(want)
	if code != 3 || !slices.Equal(got, want) {
		t.Fatalf("nogo = %d\n%s\nwant 3 and\n%s", code, strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}
