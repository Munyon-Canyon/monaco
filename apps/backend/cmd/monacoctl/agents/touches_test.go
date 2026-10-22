package agents

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

const (
	parseBefore = "package x\n\nfunc Parse() {}\n"
	parseAfter  = "package x\n\nfunc Parse(a int) {}\n"
)

func touchesHarness(t *testing.T, touches ...string) *checkHarness {
	t.Helper()
	h := newCheckHarness(t)
	h.base(t, map[string]string{
		"apps/backend/internal/x/x.go":           parseBefore,
		"apps/backend/internal/y/y_test.go":      "package y\n\nvar _ = x.Parse\n",
		"apps/backend/internal/z/z_test.go":      "package z\n\nvar _ = 1\n",
		"apps/backend/internal/w/testdata/a.txt": "Parse\n",
		"apps/backend/internal/v/v_test.go":      "package v\n\nvar _ = x.Parse\n",
		"docs/reference/errors.md":               "old\n",
	})
	h.affected = "./internal/y\n./internal/z\n"
	h.excluded = "./internal/w\n"
	h.owner(t, Record{Ticket: 1, Worktree: h.work, Touches: touches})
	return h
}

func TestCheckTouches_acceptsFixturesInImportersThatNameAChangedIdentifier(t *testing.T) {
	t.Parallel()
	h := touchesHarness(t, "apps/backend/internal/x/x.go", "apps/backend/internal/errs/codes_zz.go")
	h.commit(t, map[string]string{
		"apps/backend/internal/x/x.go":           parseAfter,
		"apps/backend/internal/y/y_test.go":      "package y\n\nvar _ = x.Parse(1)\n",
		"apps/backend/internal/w/testdata/a.txt": "Parse(1)\n",
		"docs/reference/errors.md":               "new\n",
	})
	code, stdout, stderr := h.check(t)
	if code != 0 {
		t.Fatalf("check: %d %q %q", code, stdout, stderr)
	}
	for _, want := range []string{
		"accepted by ripple: apps/backend/internal/y/y_test.go (fixture naming Parse)",
		"accepted by ripple: apps/backend/internal/w/testdata/a.txt (fixture naming Parse)",
		"accepted by ripple: docs/reference/errors.md (generated from apps/backend/internal/errs/codes_zz.go)",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout lacks %q:\n%s", want, stdout)
		}
	}
}

func TestCheckTouches_stillFailsAnUnrelatedFixtureEdit(t *testing.T) {
	t.Parallel()
	h := touchesHarness(t, "apps/backend/internal/x/x.go")
	h.commit(t, map[string]string{
		"apps/backend/internal/x/x.go":      parseAfter,
		"apps/backend/internal/z/z_test.go": "package z\n\nvar _ = 2\n",
		"apps/backend/internal/v/v_test.go": "package v\n\nvar _ = x.Parse(1)\n",
		"README.md":                         "hi\n",
	})
	code, stdout, _ := h.check(t)
	want := "outside Touches: README.md, apps/backend/internal/v/v_test.go, apps/backend/internal/z/z_test.go"
	if code == 0 || !strings.Contains(stdout, want) {
		t.Fatalf("check: %d\n%s", code, stdout)
	}
	if strings.Contains(stdout, "accepted by ripple") {
		t.Fatalf("accepted an unrelated edit:\n%s", stdout)
	}
}

func TestCheckTouches_aChangeInsideTouchesPrintsNothingExtra(t *testing.T) {
	t.Parallel()
	h := touchesHarness(t, "apps/backend/internal/x/**")
	h.commit(t, map[string]string{"apps/backend/internal/x/x.go": parseAfter})
	code, stdout, _ := h.check(t)
	if code != 0 || strings.Contains(stdout, "accepted by ripple") || !strings.Contains(stdout, "touches") {
		t.Fatalf("check: %d\n%s", code, stdout)
	}
}

func TestCheckTouches_aRecordWithoutTouchesAddsNoRow(t *testing.T) {
	t.Parallel()
	h := touchesHarness(t)
	h.commit(t, map[string]string{"README.md": "hi\n"})
	code, stdout, _ := h.check(t)
	if code != 0 || strings.Contains(stdout, "touches") {
		t.Fatalf("check: %d\n%s", code, stdout)
	}
}

func TestCheckTouches_reportsWhatItCouldNotRead(t *testing.T) {
	t.Parallel()
	cases := map[string]func(*testing.T, *checkHarness){
		"affected": func(_ *testing.T, h *checkHarness) { h.affectedErr = errors.New("no go list") },
		"excluded": func(_ *testing.T, h *checkHarness) { h.excludedErr = errors.New("no go list") },
		"tracked files": func(_ *testing.T, h *checkHarness) {
			h.replies = []reply{{prefix: "git ls-files", err: errors.New("no git")}}
		},
		"the diff": func(_ *testing.T, h *checkHarness) {
			h.replies = []reply{{prefix: "git diff -U0", err: errors.New("no git")}}
		},
		"owner records": func(t *testing.T, h *checkHarness) {
			t.Helper()
			writeFile(t, h.Env(t).recordPath(1), "{")
		},
	}
	for name, arrange := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := touchesHarness(t, "apps/backend/internal/x/x.go")
			h.commit(t, map[string]string{"README.md": "hi\n"})
			arrange(t, h)
			if code, stdout, stderr := h.check(t); code == 0 {
				t.Fatalf("a failure to read %s passed:\n%s%s", name, stdout, stderr)
			}
		})
	}
}

func TestChangedIdentifiers_readTopLevelDeclarationsAndMethods(t *testing.T) {
	t.Parallel()
	diff := strings.Join([]string{
		"-func Parse() {}", "+func Parse(a int) {}",
		"+func (r *Reader) Read[T any]() {}", "-type Kind int", "+const Limit = 1", "-var Mode = 1",
		"+func hidden() {}", "+\tfunc Indented() {}", " func Context() {}", "+++ b/x.go",
	}, "\n")
	want := []string{"Kind", "Limit", "Mode", "Parse", "Read"}
	if got := changedIdentifiers(diff); !slices.Equal(got, want) {
		t.Fatalf("changedIdentifiers = %v, want %v", got, want)
	}
}

func TestCheckTouches_aNarrowEditNeedsNoPackageLookup(t *testing.T) {
	t.Parallel()
	h := touchesHarness(t, "apps/backend/internal/errs/codes_zz.go")
	h.affectedErr = errors.New("no go list")
	h.commit(t, map[string]string{"docs/reference/errors.md": "new\n"})
	code, stdout, _ := h.check(t)
	if code != 0 || !strings.Contains(stdout, "accepted by ripple: docs/reference/errors.md") {
		t.Fatalf("check: %d\n%s", code, stdout)
	}
}

func TestWideTier_skipsAFileItCannotRead(t *testing.T) {
	t.Parallel()
	read := func(name string) ([]byte, error) {
		if name == "p/a_test.go" {
			return nil, errors.New("gone")
		}
		return []byte("Parse"), nil
	}
	got := wideTier([]string{"p/a_test.go", "p/b_test.go", "p/notes.md"}, []string{"p"}, []string{"Parse"}, read)
	if len(got) != 1 || got["p/b_test.go"] != "fixture naming Parse" {
		t.Fatalf("wideTier = %v", got)
	}
}
