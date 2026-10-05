package domain_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
)

func TestSubjectName_prefersTheDisplayNameOverTheHandle(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ display, handle, want string }{
		{"Quillen", "quill", "Quillen"},
		{"", "quill", "quill"},
		{"", "", ""},
	} {
		if got := domain.SubjectName(tc.display, tc.handle); got != tc.want {
			t.Errorf("SubjectName(%q, %q) = %q, want %q", tc.display, tc.handle, got, tc.want)
		}
	}
}
