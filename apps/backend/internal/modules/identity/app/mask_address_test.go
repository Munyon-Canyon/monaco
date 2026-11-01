package app_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/identity/app"
)

func TestMaskAddress_keepsTheEdgesAndHidesTheMiddle(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ in, want string }{
		{"F4nbnZw67wKUQGW46MSRwN23VAKQRhYUMEXt88GQwD9z", "F4nb...wD9z"},
		{"abcdefgh", "..."},
		{"", "..."},
	} {
		if got := app.MaskAddress(tc.in); got != tc.want {
			t.Errorf("MaskAddress(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
