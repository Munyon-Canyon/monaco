package domain_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/identity/domain"
)

func TestDevSuffix(t *testing.T) {
	t.Parallel()
	cases := []struct {
		email, suffix string
		ok            bool
	}{
		{"dev-1a2b3c4d@example.com", "1a2b3c4d", true},
		{domain.DevEmail("ab"), "ab", true},
		{"dev-@example.com", "", false},
		{"dev-a@b@example.com", "", false},
		{"dev-ab@example.org", "", false},
		{"qa-ab@example.com", "", false},
		{"devab@example.com", "", false},
		{"", "", false},
	}
	for _, c := range cases {
		if suffix, ok := domain.DevSuffix(c.email); suffix != c.suffix || ok != c.ok {
			t.Errorf("DevSuffix(%q) = %q, %v, want %q, %v", c.email, suffix, ok, c.suffix, c.ok)
		}
	}
}

func TestDevXAccount(t *testing.T) {
	t.Parallel()
	if got := domain.DevXAccount("ab", ""); got != (domain.XAccount{UserID: "dev:ab", Username: "dev_x_ab"}) {
		t.Fatalf("default = %+v", got)
	}
	if got := domain.DevXAccount("ab", "qa_x"); got != (domain.XAccount{UserID: "dev:ab", Username: "qa_x"}) {
		t.Fatalf("named = %+v", got)
	}
}
