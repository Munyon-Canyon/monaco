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
	root, folded, nora, logan, sam := g.NewV7(), g.NewV7(), g.NewV7(), g.NewV7(), g.NewV7()
	tests := []struct {
		name         string
		target       domain.CommentTarget
		replier      uuid.UUID
		threadAuthor uuid.UUID
		want         domain.CommentPlacement
	}{
		{
			"the author answers a top-level comment",
			domain.CommentTarget{ID: root, AuthorID: nora},
			logan, logan,
			domain.CommentPlacement{ParentID: root, ReplyToCommentID: root, ReplyToUserID: nora},
		},
		{
			"the author answers a folded answer",
			domain.CommentTarget{ID: folded, AuthorID: logan, ParentID: root},
			logan, logan,
			domain.CommentPlacement{ParentID: root, ReplyToCommentID: folded, ReplyToUserID: logan},
		},
		{
			"someone else answers a top-level comment",
			domain.CommentTarget{ID: root, AuthorID: nora},
			sam, logan,
			domain.CommentPlacement{ReplyToCommentID: root, ReplyToUserID: nora},
		},
		{
			"someone else answers a folded answer",
			domain.CommentTarget{ID: folded, AuthorID: logan, ParentID: root},
			sam, logan,
			domain.CommentPlacement{ReplyToCommentID: folded, ReplyToUserID: logan},
		},
		{
			"a system item has no author to fold for",
			domain.CommentTarget{ID: root, AuthorID: nora},
			sam, uuid.Nil,
			domain.CommentPlacement{ReplyToCommentID: root, ReplyToUserID: nora},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := domain.PlaceReply(tt.target, tt.replier, tt.threadAuthor); got != tt.want {
				t.Fatalf("PlaceReply = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestQuoteExcerpt(t *testing.T) {
	t.Parallel()
	body := domain.CommentBody(strings.Repeat("é", domain.CommentExcerptMax+5))
	if got := domain.QuoteExcerpt(&body, false); got == nil || *got != strings.Repeat("é", domain.CommentExcerptMax) {
		t.Fatalf("live excerpt = %v", got)
	}
	if got := domain.QuoteExcerpt(&body, true); got != nil {
		t.Fatalf("deleted excerpt = %q, want nil", *got)
	}
	if got := domain.QuoteExcerpt(nil, false); got != nil {
		t.Fatalf("missing excerpt = %q, want nil", *got)
	}
}
