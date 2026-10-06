package social_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/social"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/adapters"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/socialapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

type chatRoutes struct {
	chatFixture
	users  *fakes.Identity
	routes adapters.HTTP
}

func newChatRoutes(t *testing.T) chatRoutes {
	t.Helper()
	f := chatRoutes{chatFixture: newChatFixture(t)}
	f.users = fakes.NewIdentity([]identity.UserCard{
		{ID: f.member(0), Handle: "kai", DisplayName: "Kai", PhotoURL: "https://cdn.example.com/kai.jpg"},
		{ID: f.member(1), Handle: "gone", DisplayName: "Gone", Deleted: true},
	}, nil)
	deps := module.Deps{Pool: f.pool, UoW: f.deps.UoW, IDs: f.deps.IDs, Clock: f.clock}
	f.routes = social.HTTPOf(social.New(deps, social.WithUsers(f.users)))
	return f
}

func (f chatRoutes) postAs(
	t *testing.T, author ids.UserID, body api.PostChatMessageRequest,
) (api.ChatMessage, error) {
	t.Helper()
	res, err := f.routes.PostChatMessage(asUser(t.Context(), author), api.PostChatMessageRequestObject{
		Id: f.cabal.ID.UUID(), Body: &body,
	})
	if err != nil {
		return api.ChatMessage{}, err
	}
	created, ok := res.(api.PostChatMessage201JSONResponse)
	if !ok {
		t.Fatalf("response = %#v", res)
	}
	return api.ChatMessage(created), nil
}

func (f chatRoutes) mustPost(t *testing.T, author ids.UserID, body api.PostChatMessageRequest) api.ChatMessage {
	t.Helper()
	m, err := f.postAs(t, author, body)
	if err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(time.Second)
	return m
}

func (f chatRoutes) remove(t *testing.T, caller ids.UserID, message uuid.UUID) error {
	t.Helper()
	res, err := f.routes.DeleteChatMessage(asUser(t.Context(), caller), api.DeleteChatMessageRequestObject{
		Id: f.cabal.ID.UUID(), MessageId: message,
	})
	if err == nil {
		if _, ok := res.(api.DeleteChatMessage204Response); !ok {
			t.Fatalf("response = %#v", res)
		}
	}
	return err
}

func TestChatRoutes_postRendersTheStoredMessageWithItsAuthor(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	got := f.mustPost(t, f.member(0), api.PostChatMessageRequest{Body: " gm "})
	want := api.ChatMessage{
		Id: got.Id, Body: ptr("gm"), CreatedAt: f.now,
		Author: api.ChatAuthor{
			Id: f.member(0).UUID(), Handle: ptr("kai"), DisplayName: "Kai",
			PhotoUrl: ptr("https://cdn.example.com/kai.jpg"),
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("posted = %+v, want %+v", got, want)
	}
	reply := f.mustPost(t, f.member(2), api.PostChatMessageRequest{
		Body: "wagmi", ParentId: &got.Id, AlsoInChannel: ptr(true),
	})
	if reply.ParentId == nil || *reply.ParentId != got.Id || !reply.AlsoInChannel {
		t.Fatalf("reply = %+v, want a reply to %s shown in the channel", reply, got.Id)
	}
	if a := reply.Author; a.Id != f.member(2).UUID() || a.Handle != nil || a.DisplayName != "" {
		t.Fatalf("author with no card = %+v, want an empty name", a)
	}
}

func TestPostChatMessage_refusesABadRequestBeforeTheCommand(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	tests := []struct {
		name string
		body *api.PostChatMessageRequest
		want errs.Code
	}{
		{"no body", nil, errs.CodeInvalidInput},
		{"blank text", &api.PostChatMessageRequest{Body: "  "}, errs.CodeChatBodyInvalid},
		{
			"also in channel without a parent",
			&api.PostChatMessageRequest{Body: "gm", AlsoInChannel: ptr(true)},
			errs.CodeInvalidInput,
		},
	}
	for _, tt := range tests {
		_, err := f.routes.PostChatMessage(asUser(t.Context(), f.member(0)), api.PostChatMessageRequestObject{
			Id: f.cabal.ID.UUID(), Body: tt.body,
		})
		if errs.CodeOf(err) != tt.want {
			t.Errorf("%s: err = %v, want %s", tt.name, err, tt.want)
		}
	}
	if _, err := f.postAs(t, f.outsider, api.PostChatMessageRequest{Body: "gm"}); errs.CodeOf(err) !=
		errs.CodeNotCabalMember {
		t.Fatalf("outsider: err = %v, want not_cabal_member", err)
	}
	if n := f.count(t, `SELECT count(*) FROM cabal_messages`); n != 0 {
		t.Fatalf("messages = %d, want 0", n)
	}
}

func TestChatRoutes_requireASignedInUser(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	ctx := t.Context()
	id := f.cabal.ID.UUID()
	_, postErr := f.routes.PostChatMessage(ctx, api.PostChatMessageRequestObject{Id: id})
	_, deleteErr := f.routes.DeleteChatMessage(ctx, api.DeleteChatMessageRequestObject{Id: id})
	for _, err := range []error{postErr, deleteErr} {
		if errs.CodeOf(err) != errs.CodeUnauthorized {
			t.Errorf("err = %v, want unauthorized", err)
		}
	}
}

func TestChatRoutes_failWhenADependencyFails(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	m := f.mustPost(t, f.member(0), api.PostChatMessageRequest{Body: "gm"})
	if err := f.remove(t, f.member(1), m.Id); errs.CodeOf(err) != errs.CodeChatMessageNotOwned {
		t.Fatalf("delete by another member: err = %v, want chat_message_not_owned", err)
	}
	f.users.Fail("UsersByID", errs.New(errs.CodeUpstreamUnavailable, "test"))
	if _, err := f.postAs(t, f.member(0), api.PostChatMessageRequest{Body: "gm"}); errs.CodeOf(err) !=
		errs.CodeUpstreamUnavailable {
		t.Fatalf("post: err = %v, want the author lookup failure", err)
	}
}

func TestChatRoutes_deleteSoftDeletesTheCallersMessage(t *testing.T) {
	t.Parallel()
	f := newChatRoutes(t)
	m := f.mustPost(t, f.member(0), api.PostChatMessageRequest{Body: "gm"})
	for range 2 {
		if err := f.remove(t, f.member(0), m.Id); err != nil {
			t.Fatal(err)
		}
	}
	if n := f.count(t, `SELECT count(*) FROM cabal_messages WHERE id = $1 AND deleted_at IS NOT NULL`, m.Id); n != 1 {
		t.Fatalf("deleted rows = %d, want 1", n)
	}
	if err := f.remove(t, f.member(0), ids.Real{}.NewV7()); errs.CodeOf(err) != errs.CodeChatMessageNotFound {
		t.Fatalf("unknown message: err = %v, want chat_message_not_found", err)
	}
}
