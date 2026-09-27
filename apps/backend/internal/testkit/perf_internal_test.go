package testkit

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type recordingT struct {
	fatal, logged string
}

func (r *recordingT) Helper() {}

func (r *recordingT) Fatalf(format string, args ...any) { r.fatal = fmt.Sprintf(format, args...) }

func (r *recordingT) Logf(format string, args ...any) { r.logged = fmt.Sprintf(format, args...) }

func TestCompareBaseline(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "perf", "baseline.json")
	rec := &recordingT{}
	compareBaseline(rec, path, false, "queries", "uow.Do", 4)
	if !strings.Contains(rec.fatal, `no queries baseline for "uow.Do"`) || !strings.Contains(rec.fatal, "measured 4") {
		t.Fatalf("missing baseline: fatal = %q", rec.fatal)
	}

	compareBaseline(&recordingT{}, path, true, "queries", "uow.Do", 4)
	compareBaseline(&recordingT{}, path, true, "allocs", "problem", 30)
	data, err := os.ReadFile(path)
	want := "{\n  \"allocs\": {\n    \"problem\": 30\n  },\n  \"queries\": {\n    \"uow.Do\": 4\n  }\n}\n"
	if err != nil || string(data) != want {
		t.Fatalf("baseline file = %q, %v; want %q", data, err, want)
	}

	for _, tc := range []struct {
		kind          string
		got           int64
		fatal, logged string
	}{
		{"queries", 4, "", ""},
		{"queries", 5, `testkit: queries for "uow.Do" = 5, baseline 4. An increase needs ` + path + " updated in the same PR (-testkit.perf-update)", ""},
		{"queries", 3, "", `testkit: queries for "uow.Do" = 3, below the baseline 4; lower it with -testkit.perf-update`},
		{"allocs", 31, `testkit: allocs for "problem" = 31, baseline 30. An increase needs ` + path + " updated in the same PR (-testkit.perf-update)", ""},
	} {
		name := "uow.Do"
		if tc.kind == "allocs" {
			name = "problem"
		}
		rec := &recordingT{}
		compareBaseline(rec, path, false, tc.kind, name, tc.got)
		if rec.fatal != tc.fatal || rec.logged != tc.logged {
			t.Fatalf("%s %d: fatal=%q logged=%q", tc.kind, tc.got, rec.fatal, rec.logged)
		}
	}
}

func TestCompareBaselineReportsAnUnreadableFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{bad: "testkit: parse " + bad, dir: "testkit: read " + dir} {
		rec := &recordingT{}
		compareBaseline(rec, path, false, "allocs", "x", 1)
		if !strings.HasPrefix(rec.fatal, want) {
			t.Fatalf("fatal = %q, want prefix %q", rec.fatal, want)
		}
	}
	rec := &recordingT{}
	compareBaseline(rec, filepath.Join(bad, "nested.json"), true, "allocs", "x", 1)
	if !strings.HasPrefix(rec.fatal, "testkit: read ") {
		t.Fatalf("fatal = %q, want a read error through a file used as a directory", rec.fatal)
	}
}

func TestReadBaselineFillsMissingSections(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "baseline.json")
	if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	rec := &recordingT{}
	compareBaseline(rec, path, true, "queries", "q", 2)
	compareBaseline(rec, path, true, "allocs", "a", 1)
	if rec.fatal != "" {
		t.Fatalf("fatal = %q", rec.fatal)
	}
}
