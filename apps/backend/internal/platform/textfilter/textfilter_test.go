package textfilter_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/textfilter"
)

func TestAllowed_refusesListedWordsHoweverTheyAreDisguised(t *testing.T) {
	t.Parallel()
	for _, text := range []string{
		"kys", "KYS", "ky5", "ky$", "kyyyys", "re7ard", "re+ard", "ret@rd", "k1llyourself", "k!llyourself", "killy0urself", "kÿs", "go kys now", "you are a r3t4rd", "KILLYOURSELF", "hello, kys.",
	} {
		if textfilter.Allowed(text) {
			t.Errorf("Allowed(%q) = true, want false", text)
		}
	}
}

func TestAllowed_passesOrdinaryTextAndWordsThatMerelyContainAListedOne(t *testing.T) {
	t.Parallel()
	for _, text := range []string{
		"", "   ", "Alpha Cabal", "skyscraper", "Scunthorpe", "cocoon", "retarded flights", "$AAPL to the moon",
		"12345", "Zoë", "日本語",
	} {
		if !textfilter.Allowed(text) {
			t.Errorf("Allowed(%q) = false, want true", text)
		}
	}
}

func TestCheck_returnsTheContentCodeOnlyForRefusedText(t *testing.T) {
	t.Parallel()
	if err := textfilter.Check("test.Check", "fine"); err != nil {
		t.Fatalf("Check(fine) = %v, want nil", err)
	}
	err := textfilter.Check("test.Check", "kys")
	if got := errs.CodeOf(err); got != errs.CodeContentNotAllowed {
		t.Fatalf("Check(kys) code = %s, want %s", got, errs.CodeContentNotAllowed)
	}
}
