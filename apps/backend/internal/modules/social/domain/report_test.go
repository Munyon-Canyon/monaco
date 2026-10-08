package domain_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
)

func TestReportKind_Valid(t *testing.T) {
	t.Parallel()
	for kind, want := range map[domain.ReportKind]bool{
		domain.ReportMessage: true, domain.ReportComment: true, domain.ReportUser: true, domain.ReportCabal: true,
		"": false, "proposal": false, "Message": false,
	} {
		if got := kind.Valid(); got != want {
			t.Errorf("ReportKind(%q).Valid() = %v, want %v", kind, got, want)
		}
	}
}

func TestReportReason_Valid(t *testing.T) {
	t.Parallel()
	for reason, want := range map[domain.ReportReason]bool{
		domain.ReasonSpam: true, domain.ReasonAbuse: true, domain.ReasonOther: true,
		"": false, "harassment": false, "Spam": false,
	} {
		if got := reason.Valid(); got != want {
			t.Errorf("ReportReason(%q).Valid() = %v, want %v", reason, got, want)
		}
	}
}
