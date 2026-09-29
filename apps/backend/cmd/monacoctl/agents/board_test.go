package agents

import (
	"context"
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
		`"timelineItems":{"nodes":[{"__typename":"AddedToMergeQueueEvent","createdAt":%q},`+
		`{"__typename":"RemovedFromMergeQueueEvent","createdAt":%q}]}}`,
		f.at(-110*time.Minute), f.at(-30*time.Minute), f.at(-115*time.Minute), f.at(-90*time.Minute),
		f.at(-90*time.Minute), f.at(-80*time.Minute), f.at(-70*time.Minute), f.at(-30*time.Minute))
	queued := fmt.Sprintf(`{"number":12,"body":"Closes #6","createdAt":%q,"state":"OPEN",`+
		`"mergeQueueEntry":{"position":2},"commits":{"nodes":[{"commit":{"committedDate":%q,"statusCheckRollup":null}}]}}`,
		f.at(-20*time.Minute), f.at(-25*time.Minute))
	f.hub.on(graphqlRoute, `{"data":{"repository":{"t5":{"timelineItems":{"nodes":[{"source":`+merged+`}]}},`+
		`"t6":{"timelineItems":{"nodes":[{"source":{}},{"source":`+queued+`},{"source":`+queued+`},`+
		`{"source":{"number":13,"body":"Part of #6","state":"CLOSED"}},{"source":{"number":14,"body":"Part of #9"}}]}},`+
		`"t7":{"timelineItems":{"nodes":[]}}}}}`)
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
	got := fmt.Sprint(views[0].PRs, len(views[1].PRs), views[1].PRs[0].Queue, len(views[2].PRs))
	want := fmt.Sprint([]ticketPR{{
		Number: 11, Opened: f.now.Add(-110 * time.Minute), Merged: f.now.Add(-30 * time.Minute),
		Head: f.now.Add(-115 * time.Minute), Stage1: "success", Stage1At: f.now.Add(-90 * time.Minute),
		Verify: "success", VerifyAt: f.now.Add(-80 * time.Minute),
		Queued: []queueEvent{{true, f.now.Add(-70 * time.Minute)}, {false, f.now.Add(-30 * time.Minute)}},
	}}, 1, 2, 0)
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
	if v, err := f.Env(t).views(context.Background(), Batch{}); v != nil || err != nil {
		t.Fatal(v, err)
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

func (f *fixture) record(t *testing.T, r Record) {
	t.Helper()
	if err := f.Env(t).saveRecord(r); err != nil {
		t.Fatal(err)
	}
}
