package cabal_test

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/storage"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

const pngImage = "\x89PNG\r\n\x1a\ncabal"

type refusedStore struct{ t *testing.T }

func (s refusedStore) Put(context.Context, string, string, []byte) (string, error) {
	s.t.Error("the picture was uploaded")
	return "", nil
}

func supabase(t *testing.T, handler http.Handler) (storage.ProfilePhotos, string) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	client, err := storage.New(config.Config{
		Supabase: config.Supabase{URL: srv.URL, ServiceRoleKey: "service-role"},
		Timeouts: config.Timeouts{Storage: time.Second},
	})
	if err != nil {
		t.Fatal(err)
	}
	return storage.ProfilePhotos{Storage: client}, srv.URL
}

func (f accessFixture) pictures(store app.PictureStore, users app.UserCards) adapters.HTTP {
	h := f.routes(users)
	h.Picture = app.NewSetCabalPictureHandler(f.uow, f.pool, f.ids, f.clock, store)
	return h
}

func pictureForm(t *testing.T, body []byte) *multipart.Reader {
	t.Helper()
	var form bytes.Buffer
	writer := multipart.NewWriter(&form)
	part, err := writer.CreateFormFile("picture", "cabal.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return multipart.NewReader(&form, writer.Boundary())
}

func (f accessFixture) put(
	t *testing.T, h adapters.HTTP, actor ids.UserID, c ids.CabalID, body []byte,
) (api.PutCabalPictureResponseObject, error) {
	t.Helper()
	return h.PutCabalPicture(as(t.Context(), actor), api.PutCabalPictureRequestObject{
		Id: c.UUID(), Body: pictureForm(t, body),
	})
}

func (f accessFixture) pictureURL(t *testing.T, c ids.CabalID) *string {
	t.Helper()
	var url *string
	if err := f.pool.QueryRow(t.Context(), `SELECT picture_url FROM cabals WHERE id = $1`, c.UUID()).
		Scan(&url); err != nil {
		t.Fatal(err)
	}
	return url
}

func pictureChanges(t *testing.T, f accessFixture) []string {
	t.Helper()
	updates := f.updates(t)
	out := make([]string, 0, len(updates))
	for _, e := range updates {
		if e.Changes.PictureURL == nil || e.Changes.Name != nil || e.Changes.SlippageBps != nil {
			t.Fatalf("cabal.updated changes = %+v, want only picture_url", e.Changes)
		}
		out = append(out, *e.Changes.PictureURL)
	}
	return out
}

func (f accessFixture) clearTwice(t *testing.T, h adapters.HTTP, c testkit.SeededCabal) {
	t.Helper()
	for range 2 {
		res, err := h.DeleteCabalPicture(
			as(t.Context(), c.Creator.ID),
			api.DeleteCabalPictureRequestObject{Id: c.ID.UUID()},
		)
		if cleared, ok := res.(api.DeleteCabalPicture200JSONResponse); err != nil || !ok || cleared.PictureUrl != nil {
			t.Fatalf("DeleteCabalPicture = %+v, %v; want the cabal with no picture", res, err)
		}
	}
}

func TestUpdateCabal_PictureOk(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool)
	store, base := supabase(t, fakes.New())
	h := f.pictures(store, nil)
	res, err := f.put(t, h, c.Creator.ID, c.ID, []byte(pngImage))
	got, ok := res.(api.PutCabalPicture200JSONResponse)
	if err != nil || !ok || got.PictureUrl == nil {
		t.Fatalf("PutCabalPicture = %+v, %v; want the cabal with its picture", res, err)
	}
	url := *got.PictureUrl
	prefix := base + "/storage/v1/object/public/avatars/cabals/" + c.ID.String() + "/"
	if !strings.HasPrefix(url, prefix) || !strings.HasSuffix(url, ".png") {
		t.Fatalf("picture_url = %q, want %s<random>.png", url, prefix)
	}
	f.clearTwice(t, h, c)
	if now := f.pictureURL(t, c.ID); now != nil {
		t.Fatalf("stored picture_url = %q, want NULL", *now)
	}
	if changes := pictureChanges(t, f); len(changes) != 2 || changes[0] != url || changes[1] != "" {
		t.Fatalf("picture changes = %q, want the upload then one clear", changes)
	}
}

func TestUpdateCabal_PictureRejectsAGIFAndA3MBFile(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool)
	h := f.pictures(refusedStore{t}, nil)
	big := append(bytes.Clone([]byte(pngImage)), make([]byte, 3<<20)...)
	for _, body := range [][]byte{[]byte("GIF89a\x01\x00\x01\x00"), big} {
		_, err := f.put(t, h, c.Creator.ID, c.ID, body)
		wantErr(t, err, errs.CodePhotoInvalid)
	}
	if url := f.pictureURL(t, c.ID); url != nil || len(f.updates(t)) != 0 {
		t.Fatalf("picture_url = %v with %d events, want nothing written", url, len(f.updates(t)))
	}
}

func TestUpdateCabal_PictureStorageUnavailable(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool)
	down, _ := supabase(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	for _, store := range []app.PictureStore{down, nil} {
		_, err := f.put(t, f.pictures(store, nil), c.Creator.ID, c.ID, []byte(pngImage))
		wantErr(t, err, errs.CodeStorageUnavailable)
	}
	if url := f.pictureURL(t, c.ID); url != nil || len(f.updates(t)) != 0 {
		t.Fatalf("picture_url = %v with %d events, want nothing written", url, len(f.updates(t)))
	}
}

func TestUpdateCabal_PictureIsCreatorOnly(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithMembers(2))
	banned := testkit.NewCabal(t, f.pool)
	f.exec(t, `UPDATE cabals SET status = 'banned' WHERE id = $1`, banned.ID.UUID())
	h := f.pictures(refusedStore{t}, nil)
	for _, tt := range []struct {
		actor ids.UserID
		cabal ids.CabalID
		want  errs.Code
	}{
		{c.Members[1].ID, c.ID, errs.CodeNotCabalCreator},
		{banned.Creator.ID, banned.ID, errs.CodeCabalBanned},
		{c.Creator.ID, ids.CabalIDFrom(ids.Real{}.NewV7()), errs.CodeCabalNotFound},
	} {
		_, err := f.put(t, h, tt.actor, tt.cabal, []byte(pngImage))
		wantErr(t, err, tt.want)
		_, err = h.DeleteCabalPicture(
			as(t.Context(), tt.actor),
			api.DeleteCabalPictureRequestObject{Id: tt.cabal.UUID()},
		)
		wantErr(t, err, tt.want)
	}
	_, err := h.DeleteCabalPicture(t.Context(), api.DeleteCabalPictureRequestObject{Id: c.ID.UUID()})
	wantErr(t, err, errs.CodeUnauthorized)
	if len(f.updates(t)) != 0 {
		t.Fatalf("events = %d, want none", len(f.updates(t)))
	}
}

func TestUpdateCabal_PictureWrapsStoreFailuresAsInternal(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool)
	store, _ := supabase(t, fakes.New())
	_, err := f.put(t, f.pictures(store, failCards{}), c.Creator.ID, c.ID, []byte(pngImage))
	wantErr(t, err, errs.CodeInternal)
	f.exec(t, `UPDATE cabals SET picture_url = NULL WHERE id = $1`, c.ID.UUID())
	f.exec(t, `ALTER TABLE cabals ADD CONSTRAINT no_picture CHECK (picture_url IS NULL OR picture_url = '')`)
	_, err = f.put(t, f.pictures(store, nil), c.Creator.ID, c.ID, []byte(pngImage))
	wantErr(t, err, errs.CodeInternal)
	if changes := pictureChanges(t, f); len(changes) != 1 {
		t.Fatalf("picture changes = %q, want only the first upload, whose read failed after commit", changes)
	}
}
