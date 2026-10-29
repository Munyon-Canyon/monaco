package domain

import (
	"encoding/json"
	"log/slog"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

type AttachmentKind string

const (
	AttachmentGIF AttachmentKind = "gif"

	GifDisplay CommentBody = "GIF"

	gifTitleMax = 140
	gifSizeMax  = 2000
)

var gifIDPattern = regexp.MustCompile(`^[A-Za-z0-9]{1,64}$`)

func isGiphyHost(host string) bool {
	switch host {
	case "media.giphy.com", "media0.giphy.com", "media1.giphy.com", "media2.giphy.com", "media3.giphy.com",
		"media4.giphy.com", "i.giphy.com":
		return true
	}
	return false
}

type Media struct {
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

type GifAttachment struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Still    Media  `json:"still"`
	Preview  Media  `json:"preview"`
	Original Media  `json:"original"`
}

type Attachment struct {
	Kind AttachmentKind
	GIF  *GifAttachment
}

type GifInput = GifAttachment

type AttachmentInput struct {
	Kind string
	GIF  *GifInput
}

type CommentInput struct {
	Body       *CommentBody
	Attachment *Attachment
}

func (c CommentInput) Text() CommentBody {
	if c.Body != nil {
		return *c.Body
	}
	return GifDisplay
}

func (a Attachment) Payload() []byte {
	payload, _ := json.Marshal(a.GIF)
	return payload
}

func DecodeAttachment(kind string, payload []byte) *Attachment {
	var gif GifAttachment
	if AttachmentKind(kind) != AttachmentGIF || json.Unmarshal(payload, &gif) != nil {
		return nil
	}
	return &Attachment{Kind: AttachmentGIF, GIF: &gif}
}

func ParseCommentInput(body *string, att *AttachmentInput) (CommentInput, error) {
	const op = "social.ParseCommentInput"
	var in CommentInput
	if body != nil && (strings.TrimSpace(*body) != "" || att == nil) {
		parsed, err := ParseCommentBody(*body)
		if err != nil {
			return CommentInput{}, err
		}
		in.Body = &parsed
	}
	if att != nil {
		parsed, err := parseAttachment(*att)
		if err != nil {
			return CommentInput{}, err
		}
		in.Attachment = &parsed
	}
	if in.Body == nil && in.Attachment == nil {
		return CommentInput{}, errs.New(errs.CodeInvalidInput, op, slog.String("reason", "empty_reply"))
	}
	return in, nil
}

func parseAttachment(in AttachmentInput) (Attachment, error) {
	const op = "social.ParseAttachment"
	if AttachmentKind(in.Kind) != AttachmentGIF || in.GIF == nil {
		return Attachment{}, errs.New(errs.CodeInvalidInput, op, slog.String("reason", "unknown_kind"))
	}
	gif, err := ParseGif(*in.GIF)
	if err != nil {
		return Attachment{}, err
	}
	return Attachment{Kind: AttachmentGIF, GIF: &gif}, nil
}

func ParseGif(in GifInput) (GifAttachment, error) {
	const op = "social.ParseGif"
	invalid := func(why string) error {
		return errs.New(errs.CodeInvalidInput, op, slog.String("reason", "invalid_gif"), slog.String("why", why))
	}
	if !gifIDPattern.MatchString(in.ID) {
		return GifAttachment{}, invalid("id")
	}
	if utf8.RuneCountInString(in.Title) > gifTitleMax {
		return GifAttachment{}, invalid("title")
	}
	out := GifAttachment{ID: in.ID, Title: in.Title}
	for _, m := range []struct {
		in  Media
		out *Media
	}{{in.Still, &out.Still}, {in.Preview, &out.Preview}, {in.Original, &out.Original}} {
		media, why := parseMedia(m.in, in.ID)
		if why != "" {
			return GifAttachment{}, invalid(why)
		}
		*m.out = media
	}
	return out, nil
}

func parseMedia(in Media, id string) (Media, string) {
	if in.Width < 1 || in.Width > gifSizeMax || in.Height < 1 || in.Height > gifSizeMax {
		return Media{}, "size"
	}
	u, err := url.Parse(in.URL)
	switch {
	case err != nil || u.Scheme != "https":
		return Media{}, "scheme"
	case u.User != nil || !isGiphyHost(u.Host):
		return Media{}, "host"
	case !strings.Contains(u.Path, "/"+id+"/"):
		return Media{}, "path"
	}
	u.RawQuery, u.Fragment = "", ""
	return Media{URL: u.String(), Width: in.Width, Height: in.Height}, ""
}
