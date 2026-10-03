package adapters

import (
	"bytes"
	"mime/multipart"
	"testing"
)

func TestPhoto_rejectsMalformedMultipartForms(t *testing.T) {
	t.Parallel()
	for _, body := range []string{
		"--b\r\nContent-Disposition: form-data; name=\"other\"\r\n\r\nx\r\n--b--\r\n",
		"--b\r\nContent-Disposition: form-data; name=\"photo\"\r\n\r\n\r\n--b--\r\n",
		"--b\r\nContent-Disposition: form-data; name=\"photo\"\r\n\r\nx\r\n--b\r\nContent-Disposition: form-data; name=\"extra\"\r\n\r\n\r\n--b--\r\n",
	} {
		if _, _, _, err := photo(multipart.NewReader(bytes.NewBufferString(body), "b")); err == nil {
			t.Fatal("photo accepted malformed form")
		}
	}
}
