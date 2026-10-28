package app

import (
	"cmp"
	"context"
	"encoding/json"
	"log/slog"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const (
	SenderPerson = "person"
	SenderCabal  = "cabal"
	SenderApp    = "app"

	InterruptionActive        = "active"
	InterruptionTimeSensitive = "time-sensitive"

	threadAccount = "account"

	opPresentation = "notify.Presentation"
)

func reservedKeys() []string {
	return []string{
		"sender_kind", "sender_id", "sender_name", "sender_image_url",
		"group_id", "group_name", "group_image_url",
	}
}

type Sender struct {
	Kind     string `json:"kind,omitempty"`
	ID       string `json:"id,omitempty"`
	Name     string `json:"name,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
}

type Group struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	ImageURL string `json:"image_url,omitempty"`
}

type Presentation struct {
	ThreadID          string `json:"-"`
	Category          string `json:"-"`
	InterruptionLevel string `json:"-"`
	Sender            Sender `json:"sender"`
	Group             *Group `json:"group,omitempty"`
}

func ForCabal(view cabal.View, sender Sender) Presentation {
	return Presentation{
		ThreadID:          "cabal-" + view.ID.String(),
		InterruptionLevel: InterruptionActive,
		Sender:            sender,
		Group:             &Group{ID: view.ID.String(), Name: view.Name, ImageURL: view.PictureURL},
	}
}

func ForPerson(card identity.UserCard) Presentation {
	return Presentation{
		ThreadID:          "user-" + card.ID.String(),
		InterruptionLevel: InterruptionActive,
		Sender:            PersonSender(card),
	}
}

func ForAccount() Presentation {
	return Presentation{ThreadID: threadAccount, InterruptionLevel: InterruptionActive}
}

func forUserThread(user ids.UserID) Presentation {
	return Presentation{ThreadID: "user-" + user.String(), InterruptionLevel: InterruptionActive}
}

func CabalSender(view cabal.View) Sender {
	return Sender{Kind: SenderCabal, ID: view.ID.String(), Name: view.Name, ImageURL: view.PictureURL}
}

func PersonSender(card identity.UserCard) Sender {
	if card.Deleted {
		return Sender{Kind: SenderPerson, ID: card.ID.String(), Name: unnamedProposer}
	}
	return Sender{
		Kind: SenderPerson, ID: card.ID.String(), Name: cmp.Or(card.DisplayName, unnamedProposer),
		ImageURL: card.PhotoURL,
	}
}

func cabalPresentation(ctx context.Context, cabals Cabals, cabalID uuid.UUID) (Presentation, error) {
	view, err := cabals.Cabal(ctx, ids.CabalIDFrom(cabalID))
	if err != nil {
		return Presentation{}, err
	}
	return ForCabal(view, CabalSender(view)), nil
}

func personSender(ctx context.Context, users Users, id ids.UserID) (Sender, error) {
	cards, err := users.UsersByID(ctx, []ids.UserID{id})
	if err != nil {
		return Sender{}, err
	}
	card := cards[id]
	card.ID = id
	return PersonSender(card), nil
}

func (p Presentation) set() bool { return p.ThreadID != "" }

func (p Presentation) mutable() bool { return p.Sender.ImageURL != "" || p.Group != nil }

func (p Presentation) data() map[string]string {
	out := map[string]string{}
	put := func(key, value string) {
		if value != "" {
			out[key] = value
		}
	}
	put("sender_kind", p.Sender.Kind)
	put("sender_id", p.Sender.ID)
	put("sender_name", p.Sender.Name)
	put("sender_image_url", p.Sender.ImageURL)
	if p.Group != nil {
		put("group_id", p.Group.ID)
		put("group_name", p.Group.Name)
		put("group_image_url", p.Group.ImageURL)
	}
	return out
}

func (p Presentation) check(data map[string]string) error {
	for _, key := range reservedKeys() {
		if _, clash := data[key]; clash {
			return errs.New(errs.CodeInvalidInput, opPresentation, slog.String("reason", "data may not set "+key))
		}
	}
	return nil
}

func (p Presentation) encode() []byte {
	raw, _ := json.Marshal(p)
	return raw
}

func decodePresentation(threadID, category, level string, raw []byte) (Presentation, error) {
	var p Presentation
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &p); err != nil {
			return Presentation{}, errs.Wrap(err, errs.CodeDecodeFailed, opPresentation)
		}
	}
	p.ThreadID, p.Category, p.InterruptionLevel = threadID, category, level
	return p, nil
}
