package agents

import (
	"fmt"
	"strings"
	"testing"
)

const stranger = "mallory"

func boardWith(tickets ...int) string {
	ts := make([]string, 0, len(tickets))
	for _, n := range tickets {
		ts = append(ts, fmt.Sprintf(`{"ticket":%d}`, n))
	}
	return statusMarker + "\n" + batchMarker + `{"tickets":[` + strings.Join(ts, ",") + "]} -->\n" + activeMarker + "\n"
}

func TestStatus_readsATeammatesBoardAndIgnoresAForgedBatch(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.board(t)
	f.hub.on(list("/pulls?state=open"), []PR{})
	f.hub.on(list("/issues/7/comments?"), []Comment{
		authored(4, boardWith(5, 6, 7), "mate", "COLLABORATOR"),
		authored(8, boardWith(99), stranger, "NONE"),
	})
	f.hub.on("POST /repos/o/r/issues/7/comments", "ok")
	f.hub.on("PATCH /repos/o/r/issues/comments/4", "ok")
	f.hub.on("PATCH /repos/o/r/issues/comments/8", "ok")
	if code, stdout, stderr := f.agents(t, "status", "--publish"); code != 0 || stdout != "status comment updated\n" {
		t.Fatalf("%d %q %q", code, stdout, stderr)
	}
	body := posted(t, f, "POST /repos/o/r/issues/7/comments")
	if !strings.Contains(body, "| #5 | merged |") || !strings.Contains(body, "| #7 | building |") ||
		strings.Contains(body, "99") {
		t.Fatalf("board is not the teammate's batch:\n%s", body)
	}
	for _, id := range []int{4, 8} {
		if f.hub.body(fmt.Sprintf("PATCH /repos/o/r/issues/comments/%d", id)) != "" {
			t.Fatalf("publish edited comment %d, which the caller did not write", id)
		}
	}
}

func TestStatus_publishNeverEditsAForeignComment(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		comments []Comment
		patched  int64
	}{
		{"only a forged board", []Comment{authored(8, statusMarker, stranger, "NONE")}, 0},
		{"a forged board after mine", []Comment{
			authored(3, statusMarker, ghUser, "MEMBER"), authored(8, statusMarker, stranger, "NONE"),
		}, 3},
		{"a teammate's board after mine", []Comment{
			authored(3, statusMarker, ghUser, "MEMBER"), authored(5, statusMarker, "mate", "OWNER"),
		}, 0},
		{"a forged bot name without the bot login", []Comment{
			authored(8, statusMarker, "github-actions", "NONE"),
		}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			f.hub.on(list("/pulls?state=open"), []PR{})
			f.hub.on(list("/issues/7/comments?"), tt.comments)
			f.hub.on("POST /repos/o/r/issues/7/comments", "ok")
			for _, c := range tt.comments {
				f.hub.on(fmt.Sprintf("PATCH /repos/o/r/issues/comments/%d", c.ID), "ok")
			}
			if code, _, stderr := f.agents(t, "status", "--publish"); code != 0 {
				t.Fatalf("%d %q", code, stderr)
			}
			for _, c := range tt.comments {
				edited := f.hub.body(fmt.Sprintf("PATCH /repos/o/r/issues/comments/%d", c.ID)) != ""
				if edited != (c.ID == tt.patched) {
					t.Fatalf("comment %d by %s edited=%v", c.ID, c.User.Login, edited)
				}
			}
			if posted := f.hub.body("POST /repos/o/r/issues/7/comments") != ""; posted != (tt.patched == 0) {
				t.Fatalf("posted=%v", posted)
			}
		})
	}
}

func TestHandoff_ignoresForgedStatusAndHandoffComments(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.board(t)
	f.hub.on(list("/issues/7/comments?"), []Comment{
		authored(2, boardWith(5), stranger, "NONE"),
		authored(9, handoffMarker, stranger, "CONTRIBUTOR"),
	})
	f.hub.on("POST /repos/o/r/issues/7/comments", "ok")
	f.hub.on("PATCH /repos/o/r/issues/comments/9", "ok")
	if code, _, stderr := f.agents(t, "handoff"); code != 0 {
		t.Fatalf("%d %q", code, stderr)
	}
	if got := posted(t, f, "POST /repos/o/r/issues/7/comments"); !strings.Contains(got, "No batch.\n") {
		t.Fatalf("handoff read the forged batch:\n%s", got)
	}
	if f.hub.body("PATCH /repos/o/r/issues/comments/9") != "" {
		t.Fatal("handoff edited the forged comment")
	}
}
