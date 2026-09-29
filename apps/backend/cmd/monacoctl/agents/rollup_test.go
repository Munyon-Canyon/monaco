package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
)

func checkAt(minute int) time.Time {
	return time.Date(2026, 9, 29, 11, minute, 0, 0, time.UTC)
}

func ciOK(conclusion string, minute int) string {
	if conclusion == "" {
		return `{"name":"ci / ci-ok","status":"IN_PROGRESS"}`
	}
	return fmt.Sprintf(`{"name":"ci / ci-ok","status":"COMPLETED","conclusion":%q,"completedAt":%q,"databaseId":%d}`,
		conclusion, checkAt(minute).Format(time.RFC3339), 100+minute)
}

func verifyAt(state string, minute int) string {
	return fmt.Sprintf(`{"context":"verify","state":%q,"createdAt":%q}`, state, checkAt(minute).Format(time.RFC3339))
}

func firstPage(oid string, runs ...string) string {
	for i := range 50 {
		runs = append(runs, fmt.Sprintf(`{"name":"ci / job %d","status":"COMPLETED","conclusion":"SUCCESS"}`, i))
	}
	return fmt.Sprintf(`{"oid":%q,"statusCheckRollup":{"contexts":{"pageInfo":{"hasNextPage":true,"endCursor":"NTA"},`+
		`"nodes":[%s]}}}`, oid, strings.Join(runs, ","))
}

func lastPage(runs ...string) string {
	return `{"data":{"repository":{"c0":{"statusCheckRollup":{"contexts":{"pageInfo":{"hasNextPage":false},` +
		`"nodes":[` + strings.Join(runs, ",") + `]}}}}}}`
}

func TestLatest_keepsTheNewestRunOfEachCheck(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		runs []string
		want []string
	}{
		{"no rollup", nil, nil},
		{"an older failure then a newer success", []string{ciOK("FAILURE", 1), ciOK("SUCCESS", 2)}, []string{"SUCCESS"}},
		{"a newer success then an older failure", []string{ciOK("SUCCESS", 2), ciOK("FAILURE", 1)}, []string{"SUCCESS"}},
		{"a run still going beats a finished one", []string{ciOK("", 0), ciOK("SUCCESS", 9)}, []string{""}},
		{"a finished run after a run still going", []string{ciOK("SUCCESS", 9), ciOK("", 0)}, []string{""}},
		{"a status context by when it was set", []string{verifyAt("SUCCESS", 5), verifyAt("PENDING", 1)}, []string{"SUCCESS"}},
		{
			"other checks keep their first-seen order",
			[]string{verifyAt("SUCCESS", 1), ciOK("FAILURE", 1), `{"name":"ci / Plan","conclusion":"SKIPPED"}`, ciOK("SUCCESS", 3)},
			[]string{"SUCCESS", "SUCCESS", "SKIPPED"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var c gqlCommit
			if tc.runs != nil {
				raw := `{"statusCheckRollup":{"contexts":{"nodes":[` + strings.Join(tc.runs, ",") + `]}}}`
				if err := json.Unmarshal([]byte(raw), &c); err != nil {
					t.Fatal(err)
				}
			}
			latest := c.latest()
			got := make([]string, 0, len(latest))
			for _, x := range latest {
				got = append(got, x.Conclusion+x.State)
			}
			if !slices.Equal(got, tc.want) {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestReadAllChecks_pagesEachTruncatedRollupUntilItEnds(t *testing.T) {
	t.Parallel()
	var long, short, none gqlCommit
	for c, raw := range map[*gqlCommit]string{
		&long:  firstPage("a1", ciOK("FAILURE", 1)),
		&short: `{"oid":"b2","statusCheckRollup":{"contexts":{"nodes":[` + ciOK("SUCCESS", 1) + `]}}}`,
		&none:  `{"oid":"c3","statusCheckRollup":null}`,
	} {
		if err := json.Unmarshal([]byte(raw), c); err != nil {
			t.Fatal(err)
		}
	}
	var queries []string
	query := func(_ context.Context, q string, data any) error {
		queries = append(queries, q)
		page := map[int]string{
			1: `{"repository":{"c0":{"statusCheckRollup":{"contexts":{"pageInfo":{"hasNextPage":true,"endCursor":"MTAw"},` +
				`"nodes":[` + ciOK("SUCCESS", 2) + `]}}}}}`,
			2: `{"repository":{"c0":{"statusCheckRollup":{"contexts":{"nodes":[` + verifyAt("SUCCESS", 3) + `]}}}}}`,
		}[len(queries)]
		return json.Unmarshal([]byte(page), data)
	}
	if err := readAllChecks(context.Background(), query, []*gqlCommit{&none, &long, &short}); err != nil {
		t.Fatal(err)
	}
	if len(queries) != 2 || !strings.HasPrefix(queries[0], repoQuery+`c0: object(oid:"a1")`) ||
		!strings.Contains(queries[0], `contexts(first:100,after:"NTA")`) || strings.Contains(queries[0], "c1:") ||
		!strings.Contains(queries[1], `after:"MTAw"`) || !strings.HasSuffix(queries[1], "}}") {
		t.Fatalf("queries:\n%s", strings.Join(queries, "\n"))
	}
	got := long.latest()
	if n := len(long.StatusCheckRollup.Contexts.Nodes); n != 53 || got[0].Conclusion != "SUCCESS" ||
		got[len(got)-1].State != "SUCCESS" || len(short.StatusCheckRollup.Contexts.Nodes) != 1 {
		t.Fatalf("read %d contexts; latest %+v", n, got)
	}
}

func TestReadAllChecks_failsWhenAPageDoesNot(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, page, want string
		err              error
	}{
		{"query error", "", "boom", errors.New("boom")},
		{"commit gone", `{"repository":{"c0":null}}`, "commit a1 lost its checks while they were paged", nil},
		{"rollup gone", `{"repository":{"c0":{"statusCheckRollup":null}}}`, "commit a1 lost its checks while they were paged", nil},
		{
			"an empty page that does not move the cursor",
			`{"repository":{"c0":{"statusCheckRollup":{"contexts":{"pageInfo":{"hasNextPage":true,"endCursor":"NTA"},"nodes":[]}}}}}`,
			"commit a1 returned an empty page of checks without moving past cursor NTA",
			nil,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var c gqlCommit
			if err := json.Unmarshal([]byte(firstPage("a1")), &c); err != nil {
				t.Fatal(err)
			}
			query := func(_ context.Context, _ string, data any) error {
				if tc.err != nil {
					return tc.err
				}
				return json.Unmarshal([]byte(tc.page), data)
			}
			if err := readAllChecks(context.Background(), query, []*gqlCommit{&c}); cliText(err) != tc.want &&
				fmt.Sprint(err) != tc.want {
				t.Fatalf("err %v", err)
			}
		})
	}
}
