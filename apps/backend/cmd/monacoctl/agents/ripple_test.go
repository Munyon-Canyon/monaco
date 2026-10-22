package agents

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func rippleOfFixture(touches []string, files map[string]string) []string {
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	return ripple(rippleInput{
		touches: touches,
		files:   names,
		read: func(name string) ([]byte, error) {
			body, ok := files[name]
			if !ok {
				return nil, fs.ErrNotExist
			}
			return []byte(body), nil
		},
	})
}

func TestRipple_eachGeneratorFeedsItsOutputs(t *testing.T) {
	t.Parallel()
	const be = "apps/backend/"
	tests := []struct {
		name    string
		touches []string
		files   map[string]string
		want    []string
	}{
		{
			"error codes",
			[]string{be + "internal/errs/codes_multichain.go"},
			nil,
			[]string{
				be + "api/openapi.yaml",
				be + "internal/errs/testdata/codes_multichain.golden",
				"docs/reference/errors.md",
				"packages/mobile-core/Sources/MonacoAPI/openapi.yaml",
				"packages/mobile-core/Tests/MonacoAPITests/ErrorCodeCases.gen.swift",
			},
		},
		{
			"api spec",
			[]string{be + "api/spec/cabal.yaml"},
			map[string]string{
				be + "internal/platform/httpx/api/cabalapi/mount.gen.go": "",
				be + "internal/platform/httpx/api/api.gen.go":            "",
				be + "internal/platform/httpx/mount.go":                  "",
			},
			[]string{
				be + "api/openapi.yaml",
				be + "internal/platform/httpx/api/api.gen.go",
				be + "internal/platform/httpx/api/cabalapi/mount.gen.go",
				"packages/mobile-core/Sources/MonacoAPI/openapi.yaml",
			},
		},
		{
			"log messages",
			[]string{be + "internal/platform/observability/msgs_market.go"},
			nil,
			[]string{be + "internal/platform/observability/msgs_market.gen.go", "docs/reference/logs.md"},
		},
		{
			"event registrations",
			[]string{be + "internal/events/cabal_registrations.go"},
			nil,
			[]string{"docs/reference/events.md"},
		},
		{
			"module queries",
			[]string{be + "queries/cabal/members.sql"},
			map[string]string{
				be + "internal/modules/cabal/sqlc/members.sql.gen.go": "",
				be + "internal/modules/cabal/sqlc/db.gen.go":          "",
				be + "internal/modules/admin/sqlc/db.gen.go":          "",
			},
			[]string{
				be + "internal/modules/cabal/sqlc/db.gen.go",
				be + "internal/modules/cabal/sqlc/members.sql.gen.go",
			},
		},
		{
			"platform queries",
			[]string{be + "queries/platform/jobs.sql"},
			map[string]string{be + "internal/platform/db/sqlc/jobs.sql.gen.go": ""},
			[]string{be + "internal/platform/db/sqlc/jobs.sql.gen.go"},
		},
		{
			"own package",
			[]string{be + "internal/x/x.go"},
			map[string]string{
				be + "internal/x/x.go":                  "package x\n\nimport \"os\"\n\ntype Kind int\n\nconst Limit = 1\n\nvar Mode, quiet = os.Args, 0\n\nfunc Parse() {}\nfunc hidden() {}\n",
				be + "internal/x/x_test.go":             "package x\n",
				be + "internal/x/other_test.go":         "package x\n\nvar _ = Parse\n",
				be + "internal/x/unrelated_test.go":     "package x\n\nvar _ = hidden\n",
				be + "internal/x/testdata/x.golden":     "",
				be + "internal/x/testdata/other.golden": "",
				be + "internal/y/y_test.go":             "package y\n\nvar _ = Parse\n",
			},
			[]string{
				be + "internal/x/other_test.go",
				be + "internal/x/testdata/x.golden",
				be + "internal/x/x_test.go",
			},
		},
		{"unrelated file", []string{"docs/how-to/a.md"}, nil, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := rippleOfFixture(tc.touches, tc.files); !slices.Equal(got, tc.want) {
				t.Fatalf("ripple =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(tc.want, "\n"))
			}
		})
	}
}

func TestRipple_aGlobTouchesTrackedFiles(t *testing.T) {
	t.Parallel()
	got := rippleOfFixture(
		[]string{"apps/backend/internal/errs/codes_*.go"},
		map[string]string{
			"apps/backend/internal/errs/codes_a.go": "",
			"apps/backend/internal/errs/codes_b.go": "",
		},
	)
	for _, want := range []string{
		"apps/backend/internal/errs/testdata/codes_a.golden",
		"apps/backend/internal/errs/testdata/codes_b.golden",
	} {
		if !slices.Contains(got, want) {
			t.Fatalf("missing %s in %v", want, got)
		}
	}
}

func TestRippleCmd_printsThePathsOfATicket(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.hub.on(get("/issues/5"), Issue{
		Number: 5,
		Body:   "**Touches:** `apps/backend/internal/errs/codes_zz.go`",
	})
	code, stdout, stderr := f.agents(t, "ripple", "5")
	if code != 0 || stderr != "" || !strings.Contains(stdout, "apps/backend/internal/errs/testdata/codes_zz.golden\n") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if code, _, _ := f.agents(t, "ripple"); code == 0 {
		t.Fatal("ripple without a ticket succeeded")
	}
}

func TestRippleOf_readsTrackedFilesFromTheTree(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	writeFile(t, filepath.Join(f.dir, "pkg", "a.go"), "package pkg\n\nfunc Run() {}\n")
	writeFile(t, filepath.Join(f.dir, "pkg", "b_test.go"), "package pkg\n\nvar _ = Run\n")
	writeFile(t, filepath.Join(f.dir, "pkg", "c_test.go"), "package pkg\n")
	git(t, f.dir, "add", "pkg")
	got, err := f.Env(t).rippleOf(t.Context(), f.dir, []string{"pkg/a.go"})
	if err != nil || !slices.Equal(got, []string{"pkg/b_test.go"}) {
		t.Fatalf("ripple = %v, %v", got, err)
	}
}

func TestRippleOf_failsWhenGitCannotListTheTree(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	env := f.Env(t)
	env.Run = func(context.Context, string, string, string, ...string) ([]byte, error) {
		return nil, errors.New("no git")
	}
	if _, err := env.rippleOf(t.Context(), f.dir, []string{"a"}); err == nil {
		t.Fatal("a git failure was swallowed")
	}
}

func TestRippleCmd_refusesABadTicketOrAMissingIssue(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.hub.on(get("/issues/6"), Issue{Number: 6, Body: "**Touches:** `a`"})
	for _, args := range [][]string{{"ripple", "x"}, {"ripple", "0"}, {"ripple", "7"}} {
		if code, _, _ := f.agents(t, args...); code == 0 {
			t.Fatalf("%v succeeded", args)
		}
	}
	env := f.Env(t)
	env.Run = func(context.Context, string, string, string, ...string) ([]byte, error) {
		return nil, errors.New("no git")
	}
	if err := rippleCmd(t.Context(), env, []string{"6"}, io.Discard); err == nil {
		t.Fatal("a git failure was swallowed")
	}
}

func TestRipple_globsFollowTheTouchesSyntax(t *testing.T) {
	t.Parallel()
	for glob, file := range map[string]string{
		"docs/**":          "docs/a/b.md",
		"docs/**/b.md":     "docs/b.md",
		"docs/*/b.md":      "docs/a/b.md",
		"docs/?.md":        "docs/a.md",
		"docs/a.md":        "docs/a.md",
		"a/**/b/**/c_?.go": "a/x/y/b/z/c_1.go",
	} {
		if !globsMatch([]string{glob}, file) {
			t.Errorf("%s did not match %s", glob, file)
		}
	}
	for glob, file := range map[string]string{
		"docs/*.md": "docs/a/b.md",
		"docs/?.md": "docs/ab.md",
		"docs/a.md": "docs/axmd",
	} {
		if globsMatch([]string{glob}, file) {
			t.Errorf("%s matched %s", glob, file)
		}
	}
}

func TestRipple_aFileWithoutExportsStillFindsItsOwnTest(t *testing.T) {
	t.Parallel()
	got := rippleOfFixture([]string{"p/a.go"}, map[string]string{
		"p/a.go":       "package p\n\nfunc hidden() {}\n",
		"p/a_test.go":  "package p\n",
		"p/b_test.go":  "package p\n\nvar _ = hidden\n",
		"p/unparsable": "",
	})
	if !slices.Equal(got, []string{"p/a_test.go"}) {
		t.Fatalf("ripple = %v", got)
	}
}

func TestDispatchTouches_refuseWhenTheIssueOrTheTreeIsUnreadable(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	env := f.Env(t)
	if _, err := env.dispatchTouches(t.Context(), 9, f.dir, io.Discard); err == nil {
		t.Fatal("a missing issue was swallowed")
	}
	if err := env.recordDispatch(t.Context(), dispatchIn{ticket: 9}, f.dir, "tip", io.Discard); err == nil {
		t.Fatal("a missing issue was swallowed by recordDispatch")
	}
	f.hub.on(get("/issues/9"), Issue{Number: 9, Body: "**Touches:** `a`"})
	env.Run = func(context.Context, string, string, string, ...string) ([]byte, error) {
		return nil, errors.New("no git")
	}
	if _, err := env.dispatchTouches(t.Context(), 9, f.dir, io.Discard); err == nil {
		t.Fatal("a git failure was swallowed")
	}
}
