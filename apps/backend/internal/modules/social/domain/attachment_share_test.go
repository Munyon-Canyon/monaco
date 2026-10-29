package domain_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestCommentInput_textNamesTheAttachmentWhenThereIsNoBody(t *testing.T) {
	t.Parallel()
	for kind, want := range map[domain.AttachmentKind]domain.CommentBody{
		domain.AttachmentGIF: "GIF", domain.AttachmentThesisQuote: "Quoted the thesis", domain.AttachmentStock: "Shared a stock",
	} {
		if got := (domain.CommentInput{Attachment: &domain.Attachment{Kind: kind}}).Text(); got != want {
			t.Errorf("%s text = %q, want %q", kind, got, want)
		}
	}
	if got := (domain.Attachment{Kind: "poll"}).Display(); got != "" {
		t.Errorf("unknown kind display = %q, want empty", got)
	}
	if got := (domain.CommentInput{}).Text(); got != "" {
		t.Errorf("empty input text = %q, want empty", got)
	}
}

func TestDecodeAttachment_roundTripsEachKindAndRefusesTheRest(t *testing.T) {
	t.Parallel()
	id := testkit.NewIDs(1).NewV7()
	quote := domain.Attachment{
		Kind:   domain.AttachmentThesisQuote,
		Thesis: &domain.ThesisQuote{ProposalID: id, Text: "x", ProposerName: "Ada"},
	}
	stock := domain.Attachment{
		Kind:  domain.AttachmentStock,
		Stock: &domain.StockSnapshot{AssetID: id, Symbol: "AAPL", IsPreIPO: true},
	}
	for _, att := range []domain.Attachment{quote, stock} {
		got := domain.DecodeAttachment(string(att.Kind), att.Payload())
		if got == nil || got.Kind != att.Kind || (got.Thesis == nil) != (att.Thesis == nil) ||
			(got.Stock == nil) != (att.Stock == nil) {
			t.Errorf("decode %s = %+v, want the same kind back", att.Kind, got)
		}
	}
	if got := domain.DecodeAttachment("poll", []byte("{}")); got != nil {
		t.Errorf("unknown kind decoded to %+v", got)
	}
	for _, kind := range []domain.AttachmentKind{domain.AttachmentGIF, domain.AttachmentThesisQuote, domain.AttachmentStock} {
		if got := domain.DecodeAttachment(string(kind), []byte("{")); got != nil {
			t.Errorf("%s with a broken payload decoded to %+v", kind, got)
		}
	}
}

func TestParseCommentInput_acceptsAStockAndAThesisQuoteAndRefusesBadRanges(t *testing.T) {
	t.Parallel()
	asset := testkit.NewIDs(1).NewV7()
	in, err := domain.ParseCommentInput(nil, &domain.AttachmentInput{Kind: "stock", AssetID: asset})
	if err != nil || in.Attachment.Stock.AssetID != asset {
		t.Fatalf("stock = %+v, %v", in, err)
	}
	start, end, neg := 2, 5, -1
	in, err = domain.ParseCommentInput(nil, &domain.AttachmentInput{Kind: "thesis_quote", Start: &start, End: &end})
	if err != nil || in.Attachment.Range.Start != &start {
		t.Fatalf("quote = %+v, %v", in, err)
	}
	if _, err := domain.ParseCommentInput(
		nil,
		&domain.AttachmentInput{Kind: "thesis_quote", Start: &start},
	); err != nil {
		t.Fatalf("open-ended quote: %v", err)
	}
	for name, a := range map[string]domain.AttachmentInput{
		"negative start": {Kind: "thesis_quote", Start: &neg}, "negative end": {Kind: "thesis_quote", End: &neg},
		"start after end": {Kind: "thesis_quote", Start: &end, End: &start},
		"gif without gif": {Kind: "gif"},
	} {
		if _, err := domain.ParseCommentInput(nil, &a); err == nil {
			t.Errorf("%s: error = nil", name)
		}
	}
}

func TestQuoteRange_cutsByRunes(t *testing.T) {
	t.Parallel()
	one, three, nine := 1, 3, 9
	for name, tc := range map[string]struct {
		r    domain.QuoteRange
		want string
		ok   bool
	}{
		"whole": {domain.QuoteRange{}, "aéb—c", true}, "middle": {domain.QuoteRange{Start: &one, End: &three}, "éb", true},
		"tail": {domain.QuoteRange{Start: &three}, "—c", true}, "past the end": {domain.QuoteRange{End: &nine}, "", false},
		"empty": {domain.QuoteRange{Start: &three, End: &three}, "", false},
	} {
		got, ok := tc.r.Cut("aéb—c")
		if got != tc.want || ok != tc.ok {
			t.Errorf("%s = %q, %v, want %q, %v", name, got, ok, tc.want, tc.ok)
		}
	}
}
