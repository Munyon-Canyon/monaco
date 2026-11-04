package adapters

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func TestWirePreview_SetsThePhotoOnlyWhenPresent(t *testing.T) {
	t.Parallel()
	got := *wirePreview([]app.MemberPreview{
		{UserID: ids.UserIDFrom(ids.Real{}.NewV7()), DisplayName: "A", PhotoURL: "https://x/p.jpg"},
		{UserID: ids.UserIDFrom(ids.Real{}.NewV7()), DisplayName: "B"},
	})
	if len(got) != 2 || got[0].PhotoUrl == nil || *got[0].PhotoUrl != "https://x/p.jpg" || got[1].PhotoUrl != nil {
		t.Fatalf("wirePreview = %+v, want the photo on the first only", got)
	}
}
