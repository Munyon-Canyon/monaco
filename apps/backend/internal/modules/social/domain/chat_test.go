package domain_test

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func TestNormalizeBody(t *testing.T) {
	t.Parallel()
	emoji := "👩🏽‍💻"
	body := strings.Repeat(emoji, 400)
	if got, err := domain.NormalizeBody(" \n" + body + "\t "); err != nil || got != body {
		t.Fatalf("NormalizeBody() = %q, %v", got, err)
	}
	for _, raw := range []string{" \n\t", strings.Repeat("a", domain.MaxChatBodyScalars+1)} {
		if got, err := domain.NormalizeBody(raw); got != "" || errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Errorf("NormalizeBody(%q) = %q, %v", raw, got, err)
		}
	}
}

func TestValidateReply(t *testing.T) {
	t.Parallel()
	cabal := ids.Real{}.NewV7()
	other := ids.Real{}.NewV7()
	deleted := time.Unix(1_700_000_000, 0)
	for _, parent := range []domain.ReplyParent{
		{CabalID: cabal, ParentCabalID: cabal},
		{CabalID: cabal, ParentCabalID: cabal, ParentID: other},
		{CabalID: cabal, ParentCabalID: other},
		{CabalID: cabal, ParentCabalID: cabal, DeletedAt: &deleted},
	} {
		err := domain.ValidateReply(parent)
		if parent.ParentID == uuid.Nil && parent.CabalID == parent.ParentCabalID && parent.DeletedAt == nil {
			if err != nil {
				t.Errorf("ValidateReply(%+v) = %v", parent, err)
			}
		} else if errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Errorf("ValidateReply(%+v) = %v", parent, err)
		}
	}
}
