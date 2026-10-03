package httpx_test

import (
	"bytes"
	"mime/multipart"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
)

func imageForm(t *testing.T, write func(*multipart.Writer)) *multipart.Reader {
	t.Helper()
	var form bytes.Buffer
	writer := multipart.NewWriter(&form)
	write(writer)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return multipart.NewReader(&form, writer.Boundary())
}

func imagePart(field string, body []byte) func(*multipart.Writer) {
	return func(w *multipart.Writer) {
		part, _ := w.CreateFormFile(field, "image")
		_, _ = part.Write(body)
	}
}

func TestReadImage_acceptsJPEGPNGAndWebPByTheirBytes(t *testing.T) {
	t.Parallel()
	png := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0}, httpx.MaxImageBytes-8)...)
	for _, tt := range []struct {
		body             []byte
		contentType, ext string
	}{
		{[]byte("\xff\xd8\xff\xe0photo"), "image/jpeg", "jpg"},
		{png, "image/png", "png"},
		{[]byte("RIFF\x00\x00\x00\x00WEBPVP8 "), "image/webp", "webp"},
	} {
		got, err := httpx.ReadImage(imageForm(t, imagePart("picture", tt.body)), "picture", "test.ReadImage")
		if err != nil || got.ContentType != tt.contentType || got.Ext != tt.ext || !bytes.Equal(got.Body, tt.body) {
			t.Errorf("ReadImage(%s) = %s %s %d bytes, %v", tt.contentType, got.ContentType, got.Ext, len(got.Body), err)
		}
	}
}

func TestReadImage_refusesAnythingButOneSmallImage(t *testing.T) {
	t.Parallel()
	tooBig := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0}, httpx.MaxImageBytes-7)...)
	for name, write := range map[string]func(*multipart.Writer){
		"empty":       func(*multipart.Writer) {},
		"wrong field": imagePart("other", []byte("\x89PNG\r\n\x1a\n")),
		"empty image": func(w *multipart.Writer) { _, _ = w.CreateFormFile("picture", "image") },
		"multiple parts": func(w *multipart.Writer) {
			imagePart("picture", []byte("\x89PNG\r\n\x1a\n"))(w)
			_, _ = w.CreateFormField("extra")
		},
		"a gif":              imagePart("picture", []byte("GIF89a\x01\x00\x01\x00")),
		"one byte over 2 MB": imagePart("picture", tooBig),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := httpx.ReadImage(imageForm(t, write), "picture", "test.ReadImage")
			if errs.CodeOf(err) != errs.CodePhotoInvalid {
				t.Fatalf("err = %v, want photo_invalid", err)
			}
		})
	}
	if _, err := httpx.ReadImage(nil, "picture", "test.ReadImage"); errs.CodeOf(err) != errs.CodePhotoInvalid {
		t.Fatalf("nil reader err = %v, want photo_invalid", err)
	}
}
