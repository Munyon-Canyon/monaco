package agents

import (
	"io/fs"
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
				be + "internal/x/x.go":                  "package x\n\nfunc Parse() {}\nfunc hidden() {}\n",
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
