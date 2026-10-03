package adapters

import (
	"bytes"
	"mime/multipart"
	"testing"
)

func TestPhoto_rejectsMalformedMultipartForms(t *testing.T) {
	t.Parallel()
	for name, write := range map[string]func(*multipart.Writer){
		"empty": func(*multipart.Writer) {},
		"wrong field": func(w *multipart.Writer) {
			part, _ := w.CreateFormFile("other", "photo")
			_, _ = part.Write([]byte("x"))
		},
		"empty photo": func(w *multipart.Writer) { _, _ = w.CreateFormFile("photo", "photo") },
		"multiple parts": func(w *multipart.Writer) {
			part, _ := w.CreateFormFile("photo", "photo")
			_, _ = part.Write([]byte("x"))
			_, _ = w.CreateFormField("extra")
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var form bytes.Buffer
			writer := multipart.NewWriter(&form)
			write(writer)
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			if _, _, _, err := photo(multipart.NewReader(&form, writer.Boundary())); err == nil {
				t.Fatal("photo accepted malformed form")
			}
		})
	}
}
