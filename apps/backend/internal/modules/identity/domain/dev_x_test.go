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

func TestValidDevPool(t *testing.T) {
	t.Parallel()
	for name, want := range map[string]bool{
		"browse-host": true, "a": true, "vote-host-2": true, "sixteen-chars-ok": true,
		"": false, "Upper": false, "has_underscore": false, "-lead": false, "trail-": false, "a--b": false,
		"seventeen-chars-x": false, "0a1b2c3d": false, "0a1b2c3de": true, "0a1b2c3g": true,
	} {
		if got := domain.ValidDevPool(name); got != want {
			t.Errorf("ValidDevPool(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestDevHandle(t *testing.T) {
	t.Parallel()
	if got := domain.DevHandle("browse-other-1"); got != "dev_browse_other_1" {
		t.Fatalf("DevHandle = %q", got)
	}
	if got := domain.DevHandle("0a1b2c3d"); got != "dev_0a1b2c3d" {
		t.Fatalf("DevHandle(throwaway) = %q", got)
	}
}
