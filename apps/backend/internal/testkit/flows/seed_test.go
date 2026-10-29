package flows_test

import (
	"context"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
	toolflows "github.com/monaco/monaco/apps/backend/internal/tools/flows"
)

func appBuiltRows(t *testing.T, repo fs.FS) []toolflows.AppRow {
	t.Helper()
	backend, problems, err := toolflows.ReadAll(repo)
	if err != nil {
		t.Fatal(err)
	}
	app, appProblems := toolflows.ReadApp(repo)
	if len(problems)+len(appProblems) > 0 {
		t.Fatalf("fixture does not parse: %v %v", problems, appProblems)
	}
	return slices.DeleteFunc(app, func(row toolflows.AppRow) bool {
		return !row.Status.AtLeastBuilt() ||
			!slices.ContainsFunc(backend, func(f toolflows.Flow) bool { return f.ID == row.ID })
	})
}

func seedProblems(t *testing.T, repo fs.FS, scripts, seeds []string) []string {
	t.Helper()
	var out []string
	for _, row := range appBuiltRows(t, repo) {
		owns := regexp.MustCompile(`^F` + regexp.QuoteMeta(row.ID) + `[A-Z]`)
		for _, name := range scripts {
			if owns.MatchString(name) && !slices.Contains(seeds, name) {
				out = append(out, fmt.Sprintf(
					"flow %s is app %s but script %s has no seeder in Seeds()", row.ID, row.Status, name))
			}
		}
	}
	for _, name := range seeds {
		if !slices.Contains(scripts, name) {
			out = append(out, fmt.Sprintf("seeder %s matches no script in Scripts()", name))
		}
	}
	return out
}

func TestSeeds_coverEveryScriptOfAnAppBuiltOrVerifiedFlow(t *testing.T) {
	t.Parallel()
	scripts := slices.Sorted(maps.Keys(flows.Scripts()))
	seeds := slices.Sorted(maps.Keys(flows.Seeds()))
	if problems := seedProblems(t, os.DirFS("../../../../.."), scripts, seeds); len(problems) > 0 {
		t.Fatal(strings.Join(problems, "\n"))
	}
}

func TestSeeds_problemsNameTheGap(t *testing.T) {
	t.Parallel()
	const row = "00\tPing\tsystem\tPOST /v1/system/pings\tRecordPing\tsystem.pinged\tsystem.echo\tok;Unauthorized\tverified\td.md#a\n"
	repo := func(status string) fstest.MapFS {
		return fstest.MapFS{
			toolflows.Dir + "/00.tsv": {Data: []byte(toolflows.Header + "\n" + row)},
			"packages/flows/app/00.tsv": {
				Data: []byte(toolflows.AppHeader + "\n00\tSystemPing\t" + status + "\td.md#a\n"),
			},
		}
	}
	scripts := []string{"F00RecordPingOK", "F00RecordPingUnauthorized", "F01OpenSessionOK"}
	for _, tc := range []struct {
		name, status string
		seeds        []string
		want         []string
	}{
		{"every script seeded", "verified", []string{"F00RecordPingOK", "F00RecordPingUnauthorized"}, nil},
		{
			"a verified flow missing a seeder", "verified",
			[]string{"F00RecordPingOK"},
			[]string{"flow 00 is app verified but script F00RecordPingUnauthorized has no seeder in Seeds()"},
		},
		{
			"a built flow missing a seeder", "built",
			[]string{"F00RecordPingUnauthorized"},
			[]string{"flow 00 is app built but script F00RecordPingOK has no seeder in Seeds()"},
		},
		{"a planned flow needs no seeder", "planned", nil, nil},
		{
			"a seeder without a script", "planned",
			[]string{"F00RecordPingGone"},
			[]string{"seeder F00RecordPingGone matches no script in Scripts()"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := seedProblems(t, repo(tc.status), scripts, tc.seeds); !slices.Equal(got, tc.want) {
				t.Fatalf("problems = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSeeds_anonymousSeedHasNoToken(t *testing.T) {
	t.Parallel()
	got, err := flows.Seeds()["F00RecordPingUnauthorized"](context.Background(), flows.SeedEnv{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Token != "" || got.UserID != "" || got.IDs == nil {
		t.Fatalf("anonymous seed = %+v, want no token, no user and an empty ids map", got)
	}
}

func TestSeedsWithoutFakesNeedAnAbsolutePrivyURL(t *testing.T) {
	t.Parallel()
	seed := flows.Seeds()["F19aResolveCallsOK"]
	if seed == nil {
		t.Fatal("no F19aResolveCallsOK seeder")
	}
	base := []string{
		"MONACO_ENV=test", "DATABASE_URL=postgres://u:p@127.0.0.1:1/db", "NATS_URL=nats://127.0.0.1:1",
		"MONACO_DEV_TOKEN_KEY=k", "PRIVY_APP_ID=a", "PRIVY_APP_SECRET=s",
	}
	for name, environ := range map[string][]string{
		"relative": append(slices.Clone(base), "PRIVY_BASE_URL=/privy"),
		"empty":    append(slices.Clone(base), "PRIVY_BASE_URL="),
		"missing":  {},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := seed(context.Background(), flows.SeedEnv{Environ: environ})
			if err == nil {
				t.Fatal("seed succeeded without a fakes server or a real Privy URL")
			}
		})
	}
	_, err := seed(context.Background(), flows.SeedEnv{Environ: append(slices.Clone(base), "PRIVY_BASE_URL=/privy")})
	if err == nil || !strings.Contains(err.Error(), "MONACO_FAKES_URL") {
		t.Fatalf("err = %v, want a message naming MONACO_FAKES_URL", err)
	}
}
