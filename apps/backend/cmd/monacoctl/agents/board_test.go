package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

const graphqlRoute = "POST /graphql"

func (f *fixture) at(d time.Duration) string { return f.now.Add(d).Format(time.RFC3339) }

func (f *fixture) board(t *testing.T) {
	t.Helper()
	merged := fmt.Sprintf(`{"number":11,"body":"Part of #5","createdAt":%q,"state":"MERGED","mergedAt":%q,`+
		`"commits":{"nodes":[{"commit":{"committedDate":%q,"statusCheckRollup":{"contexts":{"nodes":[`+
		`{"name":"ci / ci-ok","status":"COMPLETED","conclusion":"SUCCESS","completedAt":%q},`+
		`{"name":"ci / Plan","status":"COMPLETED","conclusion":"FAILURE","completedAt":%q},`+
		`{"context":"verify","state":"SUCCESS","createdAt":%q}]}}}}]},`+
		`"timelineItems":{"nodes":[{"__typename":"LabeledEvent","createdAt":%q,"label":{"name":"merge-queue"}},`+
		`{"__typename":"LabeledEvent","createdAt":%q,"label":{"name":"large-pr"}},`+
		`{"__typename":"UnlabeledEvent","createdAt":%q,"label":{"name":"merge-queue"}}]}}`,
		f.at(-110*time.Minute), f.at(-30*time.Minute), f.at(-115*time.Minute), f.at(-90*time.Minute),
		f.at(-90*time.Minute), f.at(-80*time.Minute), f.at(-70*time.Minute), f.at(-60*time.Minute),
		f.at(-30*time.Minute))
	queued := fmt.Sprintf(`{"number":12,"body":"Closes #6","createdAt":%q,"state":"OPEN",`+
		`"labels":{"nodes":[{"name":"large-pr"},{"name":"merge-queue"}]},`+
		`"commits":{"nodes":[{"commit":{"committedDate":%q,"statusCheckRollup":null}}]}}`,
		f.at(-20*time.Minute), f.at(-25*time.Minute))
	landed := fmt.Sprintf(`{"number":15,"body":"Part of #6","createdAt":%q,"state":"CLOSED","closedAt":%q,`+
		`"headRefOid":"h15"}`, f.at(-15*time.Minute), f.at(-10*time.Minute))
	f.hub.on(graphqlRoute, `{"data":{"repository":{"t5":{"timelineItems":{"nodes":[{"source":`+merged+`}]}},`+
		`"t6":{"timelineItems":{"nodes":[{"source":{}},{"source":`+queued+`},{"source":`+queued+`},`+
		`{"source":{"number":13,"body":"Part of #6","state":"CLOSED","headRefOid":"h13"}},`+
		`{"source":{"number":14,"body":"Part of #9"}},{"source":`+landed+`}]}},`+
		`"t7":{"timelineItems":{"nodes":[]}}}}}`)
	f.hub.on(get("/compare/fb...h13"), `{"status":"diverged"}`)
	f.hub.on(list("/commits?sha=fb&since=2026-09-27T10:50:00Z"),
		`[{"commit":{"message":"Land the thing (#15)\n\nCloses #6"}},{"commit":{"message":"Other (#150)"}}]`)
}

func TestViews_readsEachTicketsPRsFromOneQuery(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.board(t)
	b := Batch{Tickets: []BatchTicket{{Ticket: 5}, {Ticket: 6}, {Ticket: 7}}}
	views, err := f.Env(t).views(context.Background(), b)
	if err != nil {
		t.Fatal(err)
	}
	sent := f.hub.body(graphqlRoute)
	for _, want := range []string{`t5: issue(number:5)`, `t7: issue(number:7)`, `"owner":"o"`, `"name":"r"`} {
		if !strings.Contains(sent, want) {
			t.Fatalf("query lacks %s: %s", want, sent)
		}
	}
	v6 := views[1].PRs
	got := fmt.Sprint(views[0].PRs, len(v6), v6[0].InQueue, v6[1].Number, v6[1].Merged, len(views[2].PRs))
	want := fmt.Sprint([]ticketPR{{
		Number: 11, Opened: f.now.Add(-110 * time.Minute), Merged: f.now.Add(-30 * time.Minute),
		Head: f.now.Add(-115 * time.Minute), Stage1: "success", Stage1At: f.now.Add(-90 * time.Minute),
		Verify: "success", VerifyAt: f.now.Add(-80 * time.Minute),
		Queued: []queueEvent{{true, f.now.Add(-70 * time.Minute)}, {false, f.now.Add(-30 * time.Minute)}},
	}}, 2, true, 15, f.now.Add(-10*time.Minute), 0)
	if got != want {
		t.Fatalf("views\n got %s\nwant %s", got, want)
	}
}

func TestViews_failsOnHTTPAndGraphQLErrors(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	b := Batch{Tickets: []BatchTicket{{Ticket: 5}}}
	if _, err := f.Env(t).views(context.Background(), b); err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatal(err)
	}
	f.hub.on(graphqlRoute, `{"data":null,"errors":[{"message":"rate limited"}]}`)
	if _, err := f.Env(t).views(context.Background(), b); cliText(err) != "graphql: rate limited" {
		t.Fatal(err)
	}
	if len(f.hub.callsContaining("/timeline")) != 0 || len(f.hub.callsContaining("/events")) != 0 {
		t.Fatal(f.hub.callsContaining("/issues/"))
	}
	if v, err := f.Env(t).views(context.Background(), Batch{}); v != nil || err != nil {
		t.Fatal(v, err)
	}
}

func TestViews_readsEveryCheckAndTheNewestRunOfEach(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.hub.on(graphqlRoute, `{"data":{"repository":{"t5":{"timelineItems":{"nodes":[{"source":`+
		`{"number":11,"body":"Part of #5","state":"OPEN","commits":{"nodes":[{"commit":`+
		firstPage("h11", ciOK("FAILURE", 1), verifyAt("PENDING", 1))+`}]}}}]}}}}}`)
	f.hub.onQuery(`object(oid:\"h11\")`, lastPage(ciOK("SUCCESS", 3), verifyAt("SUCCESS", 4)))
	b := Batch{Tickets: []BatchTicket{{Ticket: 5}}}
	views, err := f.Env(t).views(context.Background(), b)
	if err != nil {
		t.Fatal(err)
	}
	p := views[0].PRs[0]
	if p.Stage1 != "success" || !p.Stage1At.Equal(checkAt(3)) || p.Verify != "success" ||
		views[0].state() != "verifying" {
		t.Fatalf("pr %+v", p)
	}
	f.hub.onQuery(`object(oid:\"h11\")`, `{"data":null,"errors":[{"message":"rate limited"}]}`)
	if _, err := f.Env(t).views(context.Background(), b); cliText(err) != "graphql: rate limited" {
		t.Fatal(err)
	}
}

func TestTicketState(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	ago := func(m int) time.Time { return now.Add(-time.Duration(m) * time.Minute) }
	green := ticketPR{Number: 1, Head: ago(60), Stage1: "success"}
	merged := ticketPR{Number: 2, Merged: ago(5)}
	with := func(p ticketPR, edit func(*ticketPR)) ticketPR { edit(&p); return p }
	tests := []struct {
		name string
		prs  []ticketPR
		want string
	}{
		{"no PR yet", nil, "building"},
		{"every PR merged, even after a queue removal", []ticketPR{
			merged, with(merged, func(p *ticketPR) { p.Queued = []queueEvent{{false, ago(5)}} }),
		}, "merged"},
		{"carrying the queue label", []ticketPR{merged, with(green, func(p *ticketPR) { p.InQueue = true })}, "queued"},
		{"removed with no push since", []ticketPR{
			with(green, func(p *ticketPR) { p.Queued = []queueEvent{{true, ago(30)}, {false, ago(10)}} }),
		}, "ejected"},
		{"pushed after the removal", []ticketPR{
			with(green, func(p *ticketPR) { p.Queued = []queueEvent{{false, ago(70)}}; p.Stage1 = "pending" }),
		}, "stage 1"},
		{
			"queued again",
			[]ticketPR{with(green, func(p *ticketPR) { p.Queued = []queueEvent{{true, ago(5)}} })},
			"verifying",
		},
		{"stage 1 red", []ticketPR{with(green, func(p *ticketPR) { p.Stage1 = "failure" })}, "stage 1"},
		{"one stacked PR still running stage 1", []ticketPR{green, {Number: 3}}, "stage 1"},
		{"stage 1 green, verdict pending", []ticketPR{green, merged}, "verifying"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := (ticketView{PRs: tt.prs}).state(); got != tt.want {
				t.Fatalf("state = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNote_readsStage1AndVerify(t *testing.T) {
	t.Parallel()
	var p ticketPR
	p.note(gqlContext{Name: stage1Check, Status: "IN_PROGRESS"})
	p.note(gqlContext{Context: verifyContext, State: "FAILURE"})
	if p.Stage1 != "pending" || p.Verify != "failure" {
		t.Fatalf("%+v", p)
	}
}

func TestElapsedAndSpan(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		view ticketView
		want string
	}{
		{ticketView{}, "-"},
		{ticketView{Dispatched: now.Add(-5 * time.Minute)}, "5m"},
		{
			ticketView{Dispatched: now.Add(-65 * time.Minute), PRs: []ticketPR{{Merged: now.Add(-3 * time.Minute)}}},
			"1h02m",
		},
		{
			ticketView{
				Dispatched: now.Add(-3 * time.Hour),
				PRs:        []ticketPR{{Merged: now}, {Merged: now.Add(-time.Hour)}},
			},
			"3h00m",
		},
		{ticketView{Dispatched: now.Add(-3 * time.Hour), PRs: []ticketPR{{Merged: now}, {}}}, "3h00m"},
	}
	for _, tt := range tests {
		if got := tt.view.elapsed(now); got != tt.want {
			t.Fatalf("elapsed = %q, want %q", got, tt.want)
		}
	}
}

func (f *fixture) record(t *testing.T, r Record) {
	t.Helper()
	if err := f.Env(t).saveRecord(r); err != nil {
		t.Fatal(err)
	}
}

func posted(t *testing.T, f *fixture, route string) string {
	t.Helper()
	var p struct {
		Body string `json:"body"`
	}
	if err := json.Unmarshal([]byte(f.hub.body(route)), &p); err != nil {
		t.Fatal(route, err)
	}
	return p.Body
}

func TestStatus_publishesTheBatchAndCIReadsItBack(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.batch(t, 5, 6, 7)
	f.record(t, Record{Ticket: 5, State: Done, Started: f.now.Add(-2 * time.Hour)})
	f.board(t)
	f.hub.on(list("/pulls?state=open"), []PR{})
	f.hub.on(list("/issues/7/comments?"), []Comment{})
	f.hub.on("POST /repos/o/r/issues/7/comments", "ok")
	if code, stdout, stderr := f.agents(t, "status", "--publish"); code != 0 || stdout != "status comment updated\n" {
		t.Fatalf("local: %d %q %q", code, stdout, stderr)
	}
	body := posted(t, f, "POST /repos/o/r/issues/7/comments")
	rows := "| ticket | state | since dispatch |\n| --- | --- | --- |\n" +
		"| #5 | merged | 1h30m |\n| #6 | queued | - |\n| #7 | building | - |\n"
	if !strings.Contains(body, "|\n\n"+rows+batchMarker) || !strings.HasSuffix(body, " -->\n"+activeMarker+"\n") {
		t.Fatalf("body:\n%s", body)
	}

	ci := newFixture(t)
	ci.env = append(ci.env, "GITHUB_ACTIONS=true")
	ci.board(t)
	ci.hub.on(list("/pulls?state=open"), []PR{})
	ci.hub.on(list("/issues/7/comments?"), []Comment{authored(4, body, actionsBot, "NONE")})
	if code, stdout, stderr := ci.agents(
		t,
		"status",
		"--publish",
	); code != 0 ||
		stdout != "status comment unchanged\n" {
		t.Fatalf("ci: %d %q %q", code, stdout, stderr)
	}

	settled := strings.Replace(body, `{"ticket":6`, `{"ticket":5`, 1)
	settled = strings.Replace(settled, `{"ticket":7`, `{"ticket":5`, 1)
	ci.hub.on(list("/issues/7/comments?"), []Comment{authored(4, settled, actionsBot, "NONE")})
	ci.hub.on("PATCH /repos/o/r/issues/comments/4", "ok")
	if code, _, stderr := ci.agents(t, "status", "--publish"); code != 0 {
		t.Fatalf("settled: %d %q", code, stderr)
	}
	if got := posted(t, ci, "PATCH /repos/o/r/issues/comments/4"); strings.Contains(got, activeMarker) ||
		!strings.Contains(got, "| #5 | merged | 1h30m |\n| #5 | merged | - |\n") {
		t.Fatalf("settled body:\n%s", got)
	}
}

func TestStatus_batchFailures(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.hub.on(list("/pulls?state=open"), []PR{})
	env := f.Env(t)
	f.batch(t, 5)
	f.hub.on(list("/issues/7/comments?"), []Comment{})
	if code, _, stderr := f.agents(t, "status", "--publish"); code != 1 || !strings.Contains(stderr, "graphql") {
		t.Fatalf("cmd: %d %q", code, stderr)
	}
	if _, err := env.statusBody(context.Background(), ""); err == nil || !strings.Contains(err.Error(), "graphql") {
		t.Fatal(err)
	}
	writeFile(t, env.recordPath(9), "{")
	if _, err := env.statusBody(context.Background(), ""); err == nil || !strings.Contains(err.Error(), "9.json") {
		t.Fatal(err)
	}
	writeFile(t, env.batchPath(), "{")
	if _, err := env.statusBody(context.Background(), ""); err == nil || !strings.Contains(err.Error(), "batch.json") {
		t.Fatal(err)
	}
	for body, want := range map[string]int{
		"":                    0,
		batchMarker + "{ -->": 0,
		batchMarker + `{"tickets":[{"ticket":3}]} -->`: 1,
	} {
		if got := publishedBatch(body); len(got.Tickets) != want {
			t.Fatalf("%q -> %+v", body, got)
		}
	}
}

func TestViews_failsWhenAClosedPRsCompareFails(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.board(t)
	f.hub.on(get("/compare/fb...h13"), `{"message":"boom"`)
	b := Batch{Tickets: []BatchTicket{{Ticket: 6}}}
	_, err := f.Env(t).views(context.Background(), b)
	if err == nil || !strings.Contains(cliText(err), "compare h13 with fb") {
		t.Fatalf("views on a failed compare: %v", err)
	}
}
