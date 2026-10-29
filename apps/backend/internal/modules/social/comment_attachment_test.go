package social_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/socialapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const gifID = "xT9IgDEI1iZyb2wqo8"

func gifMedia(path string) api.GifMedia {
	return api.GifMedia{Url: "https://media.giphy.com/media/" + gifID + "/" + path, Width: 200, Height: 150}
}

func gifRequest(body *string, parent *uuid.UUID) *api.CreateCommentRequest {
	still := gifMedia("200_s.gif")
	still.Url += "?cid=abc&ep=v1_gifs"
	return &api.CreateCommentRequest{
		Body: body, ParentCommentId: parent,
		Attachment: &api.CommentAttachmentInput{
			Kind: api.CommentAttachmentInputKindGif,
			Gif: api.GifInput{
				Id: gifID, Title: "happy dance", Still: still, Preview: gifMedia("200w.mp4"),
				Original: gifMedia("giphy.gif"),
			},
		},
	}
}

func (f quoteThread) rows(t *testing.T) (comments, attachments int) {
	t.Helper()
	err := f.pool.QueryRow(t.Context(), `SELECT
		(SELECT count(*) FROM feed_comments WHERE feed_object_id = $1),
		(SELECT count(*) FROM comment_attachments WHERE feed_object_id = $1)`, f.item).Scan(&comments, &attachments)
	if err != nil {
		t.Fatal(err)
	}
	return comments, attachments
}

func (f quoteThread) gif(t *testing.T, who ids.UserID, body *string, answering uuid.UUID) api.Comment {
	t.Helper()
	var parent *uuid.UUID
	if answering != uuid.Nil {
		parent = &answering
	}
	c, err := f.post(t, f.item, who, gifRequest(body, parent))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func gifOf(t *testing.T, c api.Comment) api.GifAttachment {
	t.Helper()
	if c.Attachment == nil {
		t.Fatalf("comment %+v has no attachment", c)
	}
	return *c.Attachment
}

func wantGif(t *testing.T, name string, c api.Comment) {
	t.Helper()
	gif := gifOf(t, c)
	if gif.Kind != api.GifAttachmentKindGif || gif.Id != gifID || gif.Title != "happy dance" ||
		gif.Still != gifMedia("200_s.gif") || gif.Preview != gifMedia("200w.mp4") ||
		gif.Original != gifMedia("giphy.gif") {
		t.Errorf("%s gif = %+v, want the posted one without its query string", name, gif)
	}
	if c.Body != nil || c.BodyDisplay != "GIF" {
		t.Errorf("%s = %+v, want no body and body_display GIF", name, c)
	}
}

func TestGifReply_isStoredWithoutABodyAndReadBackWithTheQueryDropped(t *testing.T) {
	t.Parallel()
	f := newQuoteThread(t)
	created := f.gif(t, f.nora, nil, uuid.Nil)
	if created.Body != nil || created.BodyDisplay != "GIF" {
		t.Fatalf("created = %+v, want no body and body_display GIF", created)
	}
	for name, c := range map[string]api.Comment{"created": created, "listed": f.page(t, f.author).Items[0].Comment} {
		wantGif(t, name, c)
	}
	if comments, attachments := f.rows(t); comments != 1 || attachments != 1 {
		t.Fatalf("rows = %d comments, %d attachments, want 1 and 1", comments, attachments)
	}
}

func TestGifReply_keepsTheTextBesideTheGif(t *testing.T) {
	t.Parallel()
	f := newQuoteThread(t)
	created := f.gif(t, f.nora, ptr("lol"), uuid.Nil)
	listed := f.page(t, f.author).Items[0].Comment
	for name, c := range map[string]api.Comment{"created": created, "listed": listed} {
		if c.Body == nil || *c.Body != "lol" || c.BodyDisplay != "lol" || c.Attachment == nil {
			t.Errorf("%s = %+v, want the text and the gif", name, c)
		}
	}
}

func TestGifReply_aTextReplyHasNoAttachment(t *testing.T) {
	t.Parallel()
	f := newQuoteThread(t)
	f.say(t, f.nora, "plain", uuid.Nil)
	if got := f.page(t, f.author).Items[0].Comment; got.Attachment != nil {
		t.Fatalf("listed = %+v, want no attachment", got)
	}
}

func TestGifReply_aBadGifWritesNothing(t *testing.T) {
	t.Parallel()
	f := newQuoteThread(t)
	req := gifRequest(nil, nil)
	req.Attachment.Gif.Preview.Url = "http://media.giphy.com/media/" + gifID + "/200w.mp4"
	_, err := f.post(t, f.item, f.nora, req)
	wantCode(t, err, errs.CodeInvalidInput)
	if comments, attachments := f.rows(t); comments != 0 || attachments != 0 {
		t.Fatalf("rows = %d comments, %d attachments, want none", comments, attachments)
	}
}

func TestGifReply_refusesAReplyWithNeitherTextNorAttachmentAndAnUnknownKind(t *testing.T) {
	t.Parallel()
	f := newQuoteThread(t)
	unknown := gifRequest(nil, nil)
	unknown.Attachment.Kind = "photo"
	for name, req := range map[string]*api.CreateCommentRequest{
		"empty": {}, "blank text": {Body: ptr(" ")}, "unknown kind": unknown,
	} {
		_, err := f.post(t, f.item, f.nora, req)
		if errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Errorf("%s: error = %v, want invalid_input", name, err)
		}
	}
	if comments, _ := f.rows(t); comments != 0 {
		t.Fatalf("comments = %d, want none", comments)
	}
}

func TestGifReply_quotedAloneItReadsAsGifWithAnAttachmentFlag(t *testing.T) {
	t.Parallel()
	f := newQuoteThread(t)
	gif := f.gif(t, f.nora, nil, uuid.Nil)
	reply := f.say(t, f.gone, "same", gif.Id)
	want := api.CommentQuote{
		CommentId: gif.Id, RootCommentId: gif.Id, AuthorDisplayName: "Mia", Excerpt: ptr("GIF"), HasAttachment: true,
	}
	listed := f.page(t, f.nora).Items[1].Comment
	for name, c := range map[string]api.Comment{"created": reply, "listed": listed} {
		if c.ReplyTo == nil || !sameQuote(*c.ReplyTo, want) {
			t.Errorf("%s quote = %+v, want %+v", name, c.ReplyTo, want)
		}
	}
	text := f.say(t, f.author, "words", uuid.Nil)
	answer := f.say(t, f.nora, "re", text.Id)
	if answer.ReplyTo == nil || answer.ReplyTo.HasAttachment {
		t.Fatalf("quote of a text reply = %+v, want has_attachment false", answer.ReplyTo)
	}
}

func sameQuote(got, want api.CommentQuote) bool {
	return got.CommentId == want.CommentId && got.RootCommentId == want.RootCommentId &&
		got.AuthorDisplayName == want.AuthorDisplayName && got.Excerpt != nil && want.Excerpt != nil &&
		*got.Excerpt == *want.Excerpt && got.HasAttachment == want.HasAttachment &&
		got.IsDeleted == want.IsDeleted && got.IsFolded == want.IsFolded
}

func TestGifReply_deletedItDropsItsAttachmentButTheRowStays(t *testing.T) {
	t.Parallel()
	f := newQuoteThread(t)
	gif := f.gif(t, f.nora, nil, uuid.Nil)
	f.say(t, f.gone, "same", gif.Id)
	req := api.DeleteFeedCommentRequestObject{CommentId: gif.Id}
	_, err := f.routes.DeleteFeedComment(asUser(t.Context(), f.nora), req)
	if err != nil {
		t.Fatal(err)
	}
	page := f.page(t, f.author)
	got := page.Items[0].Comment
	if got.Attachment != nil || got.BodyDisplay != "Comment deleted" || got.Body != nil {
		t.Fatalf("deleted = %+v, want no attachment", got)
	}
	quote := page.Items[1].Comment.ReplyTo
	if quote == nil || quote.HasAttachment || quote.Excerpt != nil || !quote.IsDeleted {
		t.Fatalf("quote of the deleted gif = %+v, want no attachment flag and no excerpt", quote)
	}
	if _, attachments := f.rows(t); attachments != 1 {
		t.Fatalf("attachments = %d, want the row kept", attachments)
	}
}

func TestGifReply_aFailedAttachmentInsertIsInternalAndWritesNothing(t *testing.T) {
	t.Parallel()
	f := newQuoteThread(t)
	name := "fail_attachment_" + strings.ReplaceAll(f.item.String(), "-", "")
	ctx := t.Context()
	_, err := f.pool.Exec(ctx, `CREATE FUNCTION `+name+`() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
			IF NEW.feed_object_id = '`+f.item.String()+`' THEN RAISE EXCEPTION 'attachment refused'; END IF;
			RETURN NEW;
		END $$`)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = f.pool.Exec(context.WithoutCancel(ctx), `DROP TRIGGER IF EXISTS `+name+` ON comment_attachments`)
		_, _ = f.pool.Exec(context.WithoutCancel(ctx), `DROP FUNCTION IF EXISTS `+name+`()`)
	})
	if _, err := f.pool.Exec(ctx, `CREATE TRIGGER `+name+` BEFORE INSERT ON comment_attachments
		FOR EACH ROW EXECUTE FUNCTION `+name+`()`); err != nil {
		t.Fatal(err)
	}
	_, err = f.post(t, f.item, f.nora, gifRequest(nil, nil))
	wantCode(t, err, errs.CodeInternal)
	if comments, attachments := f.rows(t); comments != 0 || attachments != 0 {
		t.Fatalf("rows = %d comments, %d attachments, want none", comments, attachments)
	}
}
