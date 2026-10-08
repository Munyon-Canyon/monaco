package bucket_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/bucket"
)

func TestParse(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		raw  string
		want bucket.Size
		code errs.Code
	}{
		"day":     {raw: "day", want: bucket.Day},
		"week":    {raw: "week", want: bucket.Week},
		"month":   {raw: "month", code: errs.CodeInvalidInput},
		"empty":   {raw: "", code: errs.CodeInvalidInput},
		"capital": {raw: "Day", code: errs.CodeInvalidInput},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := bucket.Parse(tt.raw)
			if tt.code != "" {
				if errs.CodeOf(err) != tt.code || err == nil {
					t.Fatalf("Parse(%q) error = %v, want code %s", tt.raw, err, tt.code)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("Parse(%q) = %q, %v, want %q", tt.raw, got, err, tt.want)
			}
		})
	}
}
