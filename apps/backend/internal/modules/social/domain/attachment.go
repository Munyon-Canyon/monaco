package domain

import (
	"encoding/json"
	"log/slog"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

type AttachmentKind string

const (
	AttachmentGIF         AttachmentKind = "gif"
	AttachmentThesisQuote AttachmentKind = "thesis_quote"
	AttachmentStock       AttachmentKind = "stock"

	GifDisplay         CommentBody = "GIF"
	ThesisQuoteDisplay CommentBody = "Quoted the thesis"
	StockDisplay       CommentBody = "Shared a stock"

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

type ThesisQuote struct {
	ProposalID   uuid.UUID `json:"proposal_id"`
	Text         string    `json:"text"`
	ProposerName string    `json:"proposer_name"`
}

type StockSnapshot struct {
	AssetID  uuid.UUID `json:"asset_id"`
	Symbol   string    `json:"symbol"`
	Name     string    `json:"name"`
	LogoURL  string    `json:"logo_url"`
	IsPreIPO bool      `json:"is_pre_ipo"`
}

type QuoteRange struct {
	Start *int
	End   *int
}

func (r QuoteRange) Cut(thesis string) (string, bool) {
	runes := []rune(thesis)
	start, end := 0, len(runes)
	if r.Start != nil {
		start = *r.Start
	}
	if r.End != nil {
		end = *r.End
	}
	if start < 0 || start >= end || end > len(runes) {
		return "", false
	}
	return string(runes[start:end]), true
}

type Attachment struct {
	Kind   AttachmentKind
	GIF    *GifAttachment
	Thesis *ThesisQuote
	Stock  *StockSnapshot
	Poll   *Poll
	Range  QuoteRange
}

type GifInput = GifAttachment

type AttachmentInput struct {
	Kind    string
	GIF     *GifInput
	Poll    *PollInput
	Start   *int
	End     *int
	AssetID uuid.UUID
}

type CommentInput struct {
	Body       *CommentBody
	Attachment *Attachment
}

func (a Attachment) Display() CommentBody {
	switch a.Kind {
	case AttachmentThesisQuote:
		return ThesisQuoteDisplay
	case AttachmentStock:
		return StockDisplay
	case AttachmentGIF:
		return GifDisplay
	case AttachmentPoll:
		return CommentBody(a.Poll.Question)
	}
	return ""
}

func (c CommentInput) Text() CommentBody {
	switch {
	case c.Body != nil:
		return *c.Body
	case c.Attachment != nil:
		return c.Attachment.Display()
	}
	return ""
}

type pollPayload struct {
	Question string    `json:"question"`
	ClosesAt time.Time `json:"closes_at"`
}

func (a Attachment) Payload(now time.Time) []byte {
	var payload []byte
	switch a.Kind {
	case AttachmentThesisQuote:
		payload, _ = json.Marshal(a.Thesis)
	case AttachmentStock:
		payload, _ = json.Marshal(a.Stock)
	case AttachmentGIF:
		payload, _ = json.Marshal(a.GIF)
	case AttachmentPoll:
		payload, _ = json.Marshal(pollPayload{Question: a.Poll.Question, ClosesAt: now.Add(a.Poll.ClosesIn).UTC()})
	}
	return payload
}

func DecodeAttachment(kind string, payload []byte) *Attachment {
	att := Attachment{Kind: AttachmentKind(kind)}
	var target any
	switch att.Kind {
	case AttachmentPoll:
		var poll pollPayload
		if json.Unmarshal(payload, &poll) != nil {
			return nil
		}
		att.Poll = &Poll{Question: poll.Question, ClosesAt: poll.ClosesAt}
		return &att
	case AttachmentGIF:
		att.GIF = &GifAttachment{}
		target = att.GIF
	case AttachmentThesisQuote:
		att.Thesis = &ThesisQuote{}
		target = att.Thesis
	case AttachmentStock:
		att.Stock = &StockSnapshot{}
		target = att.Stock
	default:
		return nil
	}
	if json.Unmarshal(payload, target) != nil {
		return nil
	}
	return &att
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
	switch AttachmentKind(in.Kind) {
	case AttachmentGIF:
		if in.GIF == nil {
			break
		}
		gif, err := ParseGif(*in.GIF)
		if err != nil {
			return Attachment{}, err
		}
		return Attachment{Kind: AttachmentGIF, GIF: &gif}, nil
	case AttachmentThesisQuote:
		if !validRange(in.Start, in.End) {
			return Attachment{}, errs.New(errs.CodeInvalidInput, op, slog.String("reason", "invalid_range"))
		}
		return Attachment{Kind: AttachmentThesisQuote, Range: QuoteRange{Start: in.Start, End: in.End}}, nil
	case AttachmentStock:
		return Attachment{Kind: AttachmentStock, Stock: &StockSnapshot{AssetID: in.AssetID}}, nil
	case AttachmentPoll:
		if in.Poll == nil {
			break
		}
		poll, err := ParsePoll(in.Poll.Question, in.Poll.Options, in.Poll.ClosesIn)
		if err != nil {
			return Attachment{}, err
		}
		return Attachment{Kind: AttachmentPoll, Poll: &poll}, nil
	}
	return Attachment{}, errs.New(errs.CodeInvalidInput, op, slog.String("reason", "unknown_kind"))
}

func validRange(start, end *int) bool {
	switch {
	case start != nil && *start < 0, end != nil && *end < 0:
		return false
	case start != nil && end != nil:
		return *start < *end
	}
	return true
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
