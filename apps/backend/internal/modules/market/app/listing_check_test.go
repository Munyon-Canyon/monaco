package app

import (
	"log/slog"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

func TestListingCheckReasonCarriesTheErrorDetail(t *testing.T) {
	t.Parallel()
	c := ListingCheck{Err: errs.New(errs.CodeDecodeFailed, "market.verify", slog.String("symbol", "AAPLc"),
		slog.String("got", "AAPLx"))}
	got := c.Reason()
	if !strings.HasPrefix(got, c.Err.Error()) || !strings.Contains(got, " symbol=AAPLc") ||
		!strings.Contains(got, " got=AAPLx") {
		t.Fatalf("Reason() = %q", got)
	}
}
