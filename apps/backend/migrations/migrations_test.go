package migrations_test

import (
	"cmp"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/migrations"
)

func TestLatest_isTheHighestVersionPrefixOnDisk(t *testing.T) {
	t.Parallel()
	want := ""
	for _, name := range sqlFiles(t) {
		version, _, _ := strings.Cut(name, "_")
		want = max(want, version)
	}
	if got := migrations.Latest(); got != want || got == "" {
		t.Fatalf("Latest() = %q, want %q from the migrations directory", got, want)
	}
}

func TestFiles_haveUniqueTimestampPrefixesInTheOrderGitAddedThem(t *testing.T) {
	t.Parallel()
	names := sqlFiles(t)
	if err := checkNames(names, addOrder(t, names)); err != nil {
		t.Fatal(err)
	}
}

func TestCheckNames_rejectsSequenceNumbersDuplicatesAndOutOfOrderAdds(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		names []string
		added map[string]int
		want  string
	}{
		"sequence number": {
			[]string{"0003_x.sql"},
			nil,
			`0003_x.sql: want YYYYMMDDHHMMSS_name.sql`,
		},
		"impossible date": {
			[]string{"20261399000000_x.sql"},
			nil,
			`20261399000000_x.sql: prefix is not a timestamp`,
		},
		"duplicate prefix": {
			[]string{"20260927140427_a.sql", "20260927140427_b.sql"},
			nil,
			`20260927140427_b.sql: prefix 20260927140427 is also used by 20260927140427_a.sql`,
		},
		"added after a newer migration": {
			[]string{"20260101000000_late.sql", "20260927140427_early.sql"},
			map[string]int{"20260927140427_early.sql": 0, "20260101000000_late.sql": 1},
			"20260101000000_late.sql: added after 20260927140427_early.sql but sorts before it; " +
				"rename it with a current timestamp",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			err := checkNames(tc.names, tc.added)
			if err == nil || err.Error() != tc.want {
				t.Fatalf("checkNames = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestCheckNames_acceptsFilesAddedTogetherInAnyOrderAndUncommittedFilesLast(t *testing.T) {
	t.Parallel()
	names := []string{"20260102000000_b.sql", "20260101000000_a.sql", "20260103000000_new.sql"}
	added := map[string]int{"20260102000000_b.sql": 0, "20260101000000_a.sql": 0}
	if err := checkNames(names, added); err != nil {
		t.Fatal(err)
	}
}

func sqlFiles(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	return names
}

func addOrder(t *testing.T, names []string) map[string]int {
	t.Helper()
	out, err := exec.CommandContext(t.Context(), "git", "log", "--first-parent", "--reverse", "--no-renames",
		"--diff-filter=A", "--name-only", "--relative", "--format=%x00", "--", ".").Output()
	if err != nil {
		t.Fatalf("git log: %v", err)
	}
	added := map[string]int{}
	for i, commit := range strings.Split(string(out), "\x00") {
		for _, path := range strings.Fields(commit) {
			if slices.Contains(names, path) {
				added[path] = i
			}
		}
	}
	return added
}

func checkNames(names []string, added map[string]int) error {
	pattern := regexp.MustCompile(`^(\d{14})_[a-z0-9_]+\.sql$`)
	byPrefix := map[string]string{}
	for _, name := range slices.Sorted(slices.Values(names)) {
		m := pattern.FindStringSubmatch(name)
		if m == nil {
			return fmt.Errorf("%s: want YYYYMMDDHHMMSS_name.sql", name)
		}
		if _, err := time.Parse("20060102150405", m[1]); err != nil {
			return fmt.Errorf("%s: prefix is not a timestamp", name)
		}
		if other, dup := byPrefix[m[1]]; dup {
			return fmt.Errorf("%s: prefix %s is also used by %s", name, m[1], other)
		}
		byPrefix[m[1]] = name
	}
	order := func(name string) int {
		if i, ok := added[name]; ok {
			return i
		}
		return len(names) + len(added)
	}
	sorted := slices.SortedFunc(slices.Values(names), func(a, b string) int {
		return cmp.Or(cmp.Compare(order(a), order(b)), cmp.Compare(a, b))
	})
	newest := ""
	for i, name := range sorted {
		if i > 0 && order(sorted[i-1]) < order(name) {
			newest = max(newest, sorted[i-1])
		}
		if name < newest {
			return fmt.Errorf(
				"%s: added after %s but sorts before it; rename it with a current timestamp",
				name,
				newest,
			)
		}
	}
	return nil
}
