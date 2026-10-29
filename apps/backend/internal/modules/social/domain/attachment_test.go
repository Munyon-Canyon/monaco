package domain_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
)

func gifMedia(path string) domain.Media {
	return domain.Media{URL: "https://media.giphy.com/media/xT9IgDEI1iZyb2wqo8/" + path, Width: 200, Height: 150}
}

func gifInput() domain.GifInput {
	return domain.GifInput{
		ID: "xT9IgDEI1iZyb2wqo8", Title: "happy dance",
		Still: gifMedia("200_s.gif"), Preview: gifMedia("200w.mp4"), Original: gifMedia("giphy.gif"),
	}
}

func reasonOf(t *testing.T, err error) string {
	t.Helper()
	var e *errs.Error
	if !errors.As(err, &e) || e.Code != errs.CodeInvalidInput {
		t.Fatalf("error = %v, want invalid_input", err)
	}
	for _, a := range e.Attrs {
		if a.Key == "reason" {
			return a.Value.String()
		}
	}
	return ""
}

func TestParseGif_keepsAGiphyGifAndDropsEveryQueryString(t *testing.T) {
	t.Parallel()
	in := gifInput()
	in.Still.URL += "?cid=abc&ep=v1_gifs#frag"
	in.Original.URL = "https://i.giphy.com/xT9IgDEI1iZyb2wqo8/giphy.gif?x=1"
	got, err := domain.ParseGif(in)
	if err != nil {
		t.Fatal(err)
	}
	if got.Still.URL != "https://media.giphy.com/media/xT9IgDEI1iZyb2wqo8/200_s.gif" ||
		got.Original.URL != "https://i.giphy.com/xT9IgDEI1iZyb2wqo8/giphy.gif" ||
		got.Preview != in.Preview || got.ID != in.ID || got.Title != in.Title {
		t.Fatalf("gif = %+v", got)
	}
}

func TestParseGif_refusesWhatIsNotGiphysOwnMedia(t *testing.T) {
	t.Parallel()
	tests := map[string]func(*domain.GifInput){
		"empty id":            func(g *domain.GifInput) { g.ID = "" },
		"id with a dash":      func(g *domain.GifInput) { g.ID = "xT9-Igd" },
		"65 character id":     func(g *domain.GifInput) { g.ID = strings.Repeat("a", 65) },
		"141 rune title":      func(g *domain.GifInput) { g.Title = strings.Repeat("é", 141) },
		"http url":            func(g *domain.GifInput) { g.Still.URL = strings.Replace(g.Still.URL, "https", "http", 1) },
		"unparseable url":     func(g *domain.GifInput) { g.Still.URL = "https://media.giphy.com/%zz" },
		"other host":          func(g *domain.GifInput) { g.Preview.URL = "https://evil.example.com/media/xT9IgDEI1iZyb2wqo8/a.gif" },
		"host with a suffix":  func(g *domain.GifInput) { g.Preview.URL = "https://media.giphy.com.evil.io/xT9IgDEI1iZyb2wqo8/a.gif" },
		"host with userinfo":  func(g *domain.GifInput) { g.Preview.URL = "https://u@media.giphy.com/media/xT9IgDEI1iZyb2wqo8/a.gif" },
		"media5 host":         func(g *domain.GifInput) { g.Preview.URL = "https://media5.giphy.com/media/xT9IgDEI1iZyb2wqo8/a.gif" },
		"id missing in path":  func(g *domain.GifInput) { g.Original.URL = "https://media.giphy.com/media/other/giphy.gif" },
		"id only in query":    func(g *domain.GifInput) { g.Original.URL = "https://media.giphy.com/a.gif?q=/xT9IgDEI1iZyb2wqo8/" },
		"zero width":          func(g *domain.GifInput) { g.Still.Width = 0 },
		"zero height":         func(g *domain.GifInput) { g.Preview.Height = 0 },
		"width over 2000":     func(g *domain.GifInput) { g.Original.Width = 2001 },
		"height over 2000":    func(g *domain.GifInput) { g.Original.Height = 2001 },
		"negative height":     func(g *domain.GifInput) { g.Still.Height = -1 },
		"relative url":        func(g *domain.GifInput) { g.Still.URL = "/media/xT9IgDEI1iZyb2wqo8/a.gif" },
		"empty url":           func(g *domain.GifInput) { g.Still.URL = "" },
		"data url":            func(g *domain.GifInput) { g.Still.URL = "data:image/gif;base64,R0lGOD" },
		"javascript scheme":   func(g *domain.GifInput) { g.Still.URL = "javascript://media.giphy.com/xT9IgDEI1iZyb2wqo8/" },
		"port on the host":    func(g *domain.GifInput) { g.Still.URL = "https://media.giphy.com:8443/media/xT9IgDEI1iZyb2wqo8/a.gif" },
		"id as the last part": func(g *domain.GifInput) { g.Still.URL = "https://media.giphy.com/media/xT9IgDEI1iZyb2wqo8" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			in := gifInput()
			mutate(&in)
			_, err := domain.ParseGif(in)
			if got := reasonOf(t, err); got != "invalid_gif" {
				t.Fatalf("reason = %q, want invalid_gif", got)
			}
		})
	}
}

func TestParseGif_acceptsTheLimits(t *testing.T) {
	t.Parallel()
	in := gifInput()
	in.ID = strings.Repeat("a", 64)
	in.Title = strings.Repeat("é", 140)
	for _, m := range []*domain.Media{&in.Still, &in.Preview, &in.Original} {
		m.URL = "https://media4.giphy.com/media/" + in.ID + "/x.gif"
		m.Width, m.Height = 2000, 1
	}
	if _, err := domain.ParseGif(in); err != nil {
		t.Fatal(err)
	}
}

func TestParseCommentInput(t *testing.T) {
	t.Parallel()
	gif := gifInput()
	text, blank := "nice", "  \n "
	tests := []struct {
		name   string
		body   *string
		att    *domain.AttachmentInput
		reason string
		text   string
	}{
		{"text only", &text, nil, "", "nice"},
		{"gif only", nil, &domain.AttachmentInput{Kind: "gif", GIF: &gif}, "", "GIF"},
		{"blank text beside a gif", &blank, &domain.AttachmentInput{Kind: "gif", GIF: &gif}, "", "GIF"},
		{"text beside a gif", &text, &domain.AttachmentInput{Kind: "gif", GIF: &gif}, "", "nice"},
		{"neither", nil, nil, "empty_reply", ""},
		{"blank text and no attachment", &blank, nil, "", ""},
		{"unknown kind", nil, &domain.AttachmentInput{Kind: "photo", GIF: &gif}, "unknown_kind", ""},
		{"gif kind without a gif", nil, &domain.AttachmentInput{Kind: "gif"}, "unknown_kind", ""},
		{"bad gif", nil, &domain.AttachmentInput{Kind: "gif", GIF: &domain.GifInput{}}, "invalid_gif", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := domain.ParseCommentInput(tt.body, tt.att)
			switch {
			case tt.name == "blank text and no attachment":
				if errs.CodeOf(err) != errs.CodeInvalidInput {
					t.Fatalf("error = %v, want invalid_input", err)
				}
			case tt.reason != "":
				if r := reasonOf(t, err); r != tt.reason {
					t.Fatalf("reason = %q, want %q", r, tt.reason)
				}
			case err != nil || got.Text().String() != tt.text:
				t.Fatalf("ParseCommentInput = %+v, %v, want text %q", got, err, tt.text)
			}
		})
	}
}

func TestDecodeAttachment(t *testing.T) {
	t.Parallel()
	gif := gifInput()
	att := &domain.Attachment{Kind: domain.AttachmentGIF, GIF: &gif}
	got := domain.DecodeAttachment("gif", att.Payload(time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)))
	if got == nil || got.GIF == nil || *got.GIF != gif {
		t.Fatalf("decoded = %+v, want %+v", got, gif)
	}
	for name, c := range map[string]struct {
		kind    string
		payload []byte
	}{
		"no row":       {"", nil},
		"unknown kind": {"photo", []byte(`{}`)},
		"bad payload":  {"gif", []byte(`not json`)},
	} {
		if got := domain.DecodeAttachment(c.kind, c.payload); got != nil {
			t.Errorf("%s: decoded = %+v, want nil", name, got)
		}
	}
}
