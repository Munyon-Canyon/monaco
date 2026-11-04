package domain_test

import (
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func TestParseChatBody(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		raw  string
		want string
		code errs.Code
	}{
		{"plain", "gm", "gm", ""},
		{"trimmed", " \n\tgm frens \n", "gm frens", ""},
		{"2000 multi-byte scalars", strings.Repeat("é", 2000), strings.Repeat("é", 2000), ""},
		{"2000 scalars after trimming", "  " + strings.Repeat("a", 2000) + "  ", strings.Repeat("a", 2000), ""},
		{"empty", "", "", errs.CodeChatBodyInvalid},
		{"only whitespace", " \n\t ", "", errs.CodeChatBodyInvalid},
		{"2001 scalars", strings.Repeat("🙂", 2001), "", errs.CodeChatBodyInvalid},
		{"invalid UTF-8", "gm\xff", "", errs.CodeChatBodyInvalid},
		{"listed word", "go kys", "", errs.CodeContentNotAllowed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := domain.ParseChatBody(tt.raw)
			if code := errs.CodeOf(err); tt.code != "" && (err == nil || code != tt.code) {
				t.Fatalf("ParseChatBody = %q, %v, want code %s", got, err, tt.code)
			}
			if tt.code == "" && (err != nil || got.String() != tt.want) {
				t.Fatalf("ParseChatBody = %q, %v, want %q", got, err, tt.want)
			}
		})
	}
}

func TestValidateReply(t *testing.T) {
	t.Parallel()
	cabal := ids.CabalIDFrom(ids.Real{}.NewV7())
	other := ids.CabalIDFrom(ids.Real{}.NewV7())
	tests := []struct {
		name   string
		parent domain.ChatParent
		want   errs.Code
	}{
		{"top-level parent in the cabal", domain.ChatParent{CabalID: cabal}, ""},
		{"parent in another cabal", domain.ChatParent{CabalID: other}, errs.CodeChatParentNotFound},
		{"deleted parent", domain.ChatParent{CabalID: cabal, Deleted: true}, errs.CodeChatParentNotFound},
		{
			"parent is a reply",
			domain.ChatParent{CabalID: cabal, ParentID: ids.Real{}.NewV7()},
			errs.CodeChatParentIsReply,
		},
		{
			"reply in another cabal",
			domain.ChatParent{CabalID: other, ParentID: ids.Real{}.NewV7()},
			errs.CodeChatParentNotFound,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := domain.ValidateReply(cabal, tt.parent)
			if tt.want == "" {
				if err != nil {
					t.Fatalf("ValidateReply = %v, want nil", err)
				}
				return
			}
			if got := errs.CodeOf(err); err == nil || got != tt.want {
				t.Fatalf("ValidateReply = %v (code %s), want %s", err, got, tt.want)
			}
		})
	}
}

func TestParseChatInput(t *testing.T) {
	t.Parallel()
	proposal := ids.Real{}.NewV7()
	stock := &domain.AttachmentInput{Kind: "stock", AssetID: proposal}
	quote := &domain.AttachmentInput{Kind: "thesis_quote", ProposalID: proposal}
	text := func(s string) *string { return &s }
	tests := []struct {
		name string
		body *string
		att  *domain.AttachmentInput
		text string
		code errs.Code
	}{
		{"text", text(" gm "), nil, "gm", ""},
		{"stock alone reads as its display text", nil, stock, "Shared a stock", ""},
		{"blank text beside an attachment is dropped", text("  "), stock, "Shared a stock", ""},
		{"text beside an attachment", text("look"), stock, "look", ""},
		{"thesis quote", nil, quote, "Quoted the thesis", ""},
		{"nothing", nil, nil, "", errs.CodeChatBodyInvalid},
		{"blank text", text(" "), nil, "", errs.CodeChatBodyInvalid},
		{"bad text beside an attachment", text(strings.Repeat("a", 2001)), stock, "", errs.CodeChatBodyInvalid},
		{"poll", nil, &domain.AttachmentInput{Kind: "poll"}, "", errs.CodeInvalidInput},
		{"quote without a proposal", nil, &domain.AttachmentInput{Kind: "thesis_quote"}, "", errs.CodeInvalidInput},
		{"unknown kind", nil, &domain.AttachmentInput{Kind: "photo"}, "", errs.CodeInvalidInput},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := domain.ParseChatInput(tt.body, tt.att)
			if tt.code != "" {
				if errs.CodeOf(err) != tt.code {
					t.Fatalf("ParseChatInput error = %v, want %s", err, tt.code)
				}
				return
			}
			if err != nil || got.Text().String() != tt.text {
				t.Fatalf("ParseChatInput = %q, %v, want %q", got.Text(), err, tt.text)
			}
		})
	}
	if got := (domain.ChatInput{}).Stored(); got != "" {
		t.Fatalf("empty input stored body = %q, want empty", got)
	}
	if got := (domain.ChatInput{}).Text(); got != "" {
		t.Fatalf("empty input text = %q, want empty", got)
	}
}
