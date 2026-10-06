package domain_test

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestParseCommentBody(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		raw  string
		want string
		bad  bool
	}{
		{"plain", "nice", "nice", false},
		{"trimmed", " \n nice \t", "nice", false},
		{"1000 multi-byte scalars", strings.Repeat("é", 1000), strings.Repeat("é", 1000), false},
		{"empty", "", "", true},
		{"only whitespace", " \n\t ", "", true},
		{"1001 scalars", strings.Repeat("🙂", 1001), "", true},
		{"invalid UTF-8", "nice\xff", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := domain.ParseCommentBody(tt.raw)
			if tt.bad {
				if errs.CodeOf(err) != errs.CodeInvalidInput {
					t.Fatalf("error = %v, want invalid_input", err)
				}
				return
			}
			if err != nil || got.String() != tt.want {
				t.Fatalf("ParseCommentBody = %q, %v, want %q", got, err, tt.want)
			}
		})
	}
}

func TestCommentBody_Excerpt(t *testing.T) {
	t.Parallel()
	short := domain.CommentBody("gm")
	if got := short.Excerpt(); got != "gm" {
		t.Fatalf("short excerpt = %q", got)
	}
	exact := domain.CommentBody(strings.Repeat("é", domain.CommentExcerptMax))
	if got := exact.Excerpt(); got != string(exact) {
		t.Fatalf("exact excerpt = %q", got)
	}
	long := domain.CommentBody(strings.Repeat("é", domain.CommentExcerptMax+1))
	if got := long.Excerpt(); got != strings.Repeat("é", domain.CommentExcerptMax) {
		t.Fatalf("long excerpt = %q", got)
	}
}

func TestPlaceReply(t *testing.T) {
	t.Parallel()
	g := testkit.NewIDs(622)
	top, reply, author := g.NewV7(), g.NewV7(), g.NewV7()
	got := domain.PlaceReply(domain.CommentTarget{ID: top, AuthorID: author})
	if got.ParentID != top || got.ReplyToID != uuid.Nil {
		t.Fatalf("reply to a top-level comment = %+v, want parent %s and no reply-to", got, top)
	}
	got = domain.PlaceReply(domain.CommentTarget{ID: reply, AuthorID: author, ParentID: top})
	if got.ParentID != top || got.ReplyToID != author {
		t.Fatalf("reply to a reply = %+v, want parent %s and reply-to %s", got, top, author)
	}
}
