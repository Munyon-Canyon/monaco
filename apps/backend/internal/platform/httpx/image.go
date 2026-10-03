package httpx

import (
	"errors"
	"io"
	"mime/multipart"
	"net/http"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

const MaxImageBytes = 2 << 20

type Image struct {
	ContentType string
	Ext         string
	Body        []byte
}

func ReadImage(reader *multipart.Reader, field, op string) (Image, error) {
	if reader == nil {
		return Image{}, errs.New(errs.CodePhotoInvalid, op)
	}
	part, err := reader.NextPart()
	if err != nil || part.FormName() != field {
		return Image{}, errs.New(errs.CodePhotoInvalid, op)
	}
	body, err := io.ReadAll(io.LimitReader(part, MaxImageBytes+1))
	if err != nil || len(body) == 0 || len(body) > MaxImageBytes {
		return Image{}, errs.New(errs.CodePhotoInvalid, op)
	}
	if _, err := reader.NextPart(); !errors.Is(err, io.EOF) {
		return Image{}, errs.New(errs.CodePhotoInvalid, op)
	}
	switch contentType := http.DetectContentType(body); contentType {
	case "image/jpeg":
		return Image{ContentType: contentType, Ext: "jpg", Body: body}, nil
	case "image/png":
		return Image{ContentType: contentType, Ext: "png", Body: body}, nil
	case "image/webp":
		return Image{ContentType: contentType, Ext: "webp", Body: body}, nil
	default:
		return Image{}, errs.New(errs.CodePhotoInvalid, op)
	}
}
