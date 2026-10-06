package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"math"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/apns"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/concurrency"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

const (
	opPush   = "notify.Push"
	maxSends = 32
)

type Message struct {
	Title      string
	Body       string
	Data       map[string]string
	CollapseID string
}

type Kind[E events.Event] interface {
	Name() string
	Recipients(ctx context.Context, e E) ([]ids.UserID, error)
	Render(ctx context.Context, e E, to ids.UserID) (Message, error)
}

type Notification struct {
	Kind    string
	UserID  ids.UserID
	Message Message
}

type Notify[E events.Event] struct {
	Pusher *Pusher
	Kinds  []Kind[E]
}

func (n Notify[E]) Handle(ctx context.Context, d bus.Delivery, e E) error {
	recipients := make([][]ids.UserID, len(n.Kinds))
	var everyone []ids.UserID
	for i, k := range n.Kinds {
		to, err := k.Recipients(ctx, e)
		if err != nil {
			return err
		}
		recipients[i] = to
		everyone = append(everyone, to...)
	}
	eligible, err := n.Pusher.eligible(ctx, d.EventID, everyone)
	if err != nil {
		return err
	}
	var notes []Notification
	for i, k := range n.Kinds {
		for _, to := range recipients[i] {
			if !eligible[to] {
				continue
			}
			msg, err := k.Render(ctx, e, to)
			if err != nil {
				return err
			}
			notes = append(notes, Notification{Kind: k.Name(), UserID: to, Message: msg})
		}
	}
	return n.Pusher.Push(ctx, d, notes)
}

type Pusher struct {
	uow    *db.UnitOfWork
	users  Users
	sender apns.Sender
	ids    ids.Generator
	clock  clock.Clock
}

func NewPusher(uow *db.UnitOfWork, users Users, sender apns.Sender, g ids.Generator, c clock.Clock) *Pusher {
	return &Pusher{uow: uow, users: users, sender: sender, ids: g, clock: c}
}

func (p *Pusher) eligible(ctx context.Context, event ids.EventID, users []ids.UserID) (map[ids.UserID]bool, error) {
	eligible := map[ids.UserID]bool{}
	if len(users) == 0 {
		return eligible, nil
	}
	causing, err := p.causingUser(ctx, event)
	if err != nil {
		return nil, err
	}
	var distinct []ids.UserID
	for _, u := range users {
		if _, seen := eligible[u]; !seen {
			eligible[u] = false
			distinct = append(distinct, u)
		}
	}
	cards, err := p.users.UsersByID(ctx, distinct)
	if err != nil {
		return nil, err
	}
	for id, card := range cards {
		eligible[id] = !card.Deleted && id.String() != causing
	}
	return eligible, nil
}

func (p *Pusher) causingUser(ctx context.Context, event ids.EventID) (string, error) {
	actor, err := sqlc.New(p.uow.Reads()).EventActor(ctx, event.UUID())
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return "", errs.Wrap(err, errs.CodeInternal, opPush)
	case err != nil:
		return "", errs.Wrap(err, errs.CodeDBUnavailable, opPush)
	case actor.ActorType != string(auth.ActorUser):
		return "", nil
	}
	return actor.ActorID, nil
}

func (p *Pusher) Push(ctx context.Context, d bus.Delivery, notes []Notification) error {
	if err := p.write(ctx, d, notes); err != nil {
		return err
	}
	if err := p.send(ctx, d.EventID); err != nil {
		return err
	}
	return p.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		_, err := d.Record(ctx, tx)
		return err
	})
}

func (p *Pusher) write(ctx context.Context, d bus.Delivery, notes []Notification) error {
	if len(notes) == 0 {
		return nil
	}
	return p.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		q := sqlc.New(tx.Queries())
		var broadcast uuid.UUID
		if len(notes) > 1 {
			id, err := q.InsertBroadcast(ctx, sqlc.InsertBroadcastParams{
				ID: p.ids.NewV7(), Kind: notes[0].Kind, SourceEventID: d.EventID.UUID(),
				RecipientCount: int32(min(len(notes), math.MaxInt32)), CreatedAt: d.At,
			})
			switch {
			case errors.Is(err, sql.ErrNoRows):
				return nil
			case err != nil:
				return err
			}
			broadcast = id
		}
		for _, n := range notes {
			data, _ := json.Marshal(n.Message.Data)
			_, err := q.InsertNotification(ctx, sqlc.InsertNotificationParams{
				ID: p.ids.NewV7(), BroadcastID: broadcast, UserID: n.UserID.UUID(), Kind: n.Kind,
				SourceEventID: d.EventID.UUID(), Title: n.Message.Title, Body: n.Message.Body, Data: data,
				CollapseID: n.Message.CollapseID, State: string(domain.NotificationPending), CreatedAt: d.At,
			})
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
		}
		return nil
	})
}

type job struct {
	at   int
	kind string
	push apns.Push
}

type sent struct {
	at      int
	outcome apns.Outcome
	err     error
}

type pendingRow struct {
	row  sqlc.UndeliveredForEventRow
	jobs []job
}

func (p *Pusher) send(ctx context.Context, event ids.EventID) error {
	rows, jobs, err := p.plan(ctx, event)
	if err != nil {
		return err
	}
	results := p.sendAll(ctx, jobs)
	for _, r := range rows {
		if err := p.settle(ctx, event, r, results); err != nil {
			return err
		}
	}
	return eventError(results)
}

func (p *Pusher) plan(ctx context.Context, event ids.EventID) ([]pendingRow, []job, error) {
	q := sqlc.New(p.uow.Reads())
	undelivered, err := q.UndeliveredForEvent(ctx, event.UUID())
	if err != nil {
		return nil, nil, errs.Wrap(err, errs.CodeDBUnavailable, opPush)
	}
	rows := make([]pendingRow, len(undelivered))
	var jobs []job
	for i, row := range undelivered {
		tokens, err := q.ActiveTokensForUser(ctx, row.UserID)
		if err != nil {
			return nil, nil, errs.Wrap(err, errs.CodeDBUnavailable, opPush)
		}
		var data map[string]string
		if err := json.Unmarshal(row.Data, &data); err != nil {
			return nil, nil, errs.Wrap(err, errs.CodeDecodeFailed, opPush)
		}
		rows[i].row = row
		for _, t := range tokens {
			j := job{at: len(jobs), kind: row.Kind, push: apns.Push{
				UserID: ids.UserIDFrom(row.UserID), Token: t.Token, Environment: apns.Environment(t.Environment),
				CollapseID: row.CollapseID, Title: row.Title, Body: row.Body, Data: data,
			}}
			rows[i].jobs = append(rows[i].jobs, j)
			jobs = append(jobs, j)
		}
	}
	return rows, jobs, nil
}

func (p *Pusher) sendAll(ctx context.Context, jobs []job) []sent {
	results := make([]sent, len(jobs))
	if len(jobs) == 0 {
		return results
	}
	out, errc := concurrency.Pool(ctx, min(maxSends, len(jobs)), concurrency.Feed(ctx, jobs), p.sendOne)
	for out != nil || errc != nil {
		select {
		case s, ok := <-out:
			if !ok {
				out = nil
				continue
			}
			results[s.at] = s
		case _, ok := <-errc:
			if !ok {
				errc = nil
			}
		}
	}
	return results
}

func (p *Pusher) sendOne(ctx context.Context, j job) (sent, error) {
	res, err := p.sender.Send(ctx, j.push)
	s := sent{at: j.at, err: err}
	if err == nil {
		observability.Info(ctx, observability.NotifyPushResult, slog.String("user_id", j.push.UserID.String()),
			slog.String("kind", j.kind), slog.Int("status", res.Status), slog.String("reason", res.Reason))
		s.outcome = apns.Classify(res)
	}
	return s, nil
}

func (p *Pusher) settle(ctx context.Context, event ids.EventID, r pendingRow, results []sent) error {
	state, dead := rowState(r, results)
	if state == domain.NotificationPending && len(dead) == 0 {
		return nil
	}
	now := p.clock.Now()
	return p.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		q := sqlc.New(tx.Queries())
		for _, token := range dead {
			if _, err := q.DisableToken(ctx, sqlc.DisableTokenParams{Token: token, DisabledAt: now}); err != nil {
				return err
			}
		}
		switch state {
		case domain.NotificationDelivered:
			n, err := q.MarkDelivered(ctx, sqlc.MarkDeliveredParams{ID: r.row.ID, DeliveredAt: now})
			if err != nil || n != 1 {
				return err
			}
			return tx.Events.Append(ctx, events.NotificationSent{
				V: 1, NotificationID: r.row.ID, UserID: r.row.UserID, Kind: r.row.Kind, SourceEventID: event.UUID(),
			})
		case domain.NotificationNoDevice:
			_, err := q.MarkNoDevice(ctx, r.row.ID)
			return err
		case domain.NotificationPending, domain.NotificationBatched:
		}
		return nil
	})
}

func rowState(r pendingRow, results []sent) (domain.NotificationState, []string) {
	delivered := false
	var dead []string
	for _, j := range r.jobs {
		switch results[j.at].outcome {
		case apns.Delivered:
			delivered = true
		case apns.TokenDead:
			dead = append(dead, j.push.Token)
		case apns.Retry, apns.AuthFailed, apns.Rejected:
		}
	}
	switch {
	case delivered:
		return domain.NotificationDelivered, dead
	case len(dead) == len(r.jobs):
		return domain.NotificationNoDevice, dead
	}
	return domain.NotificationPending, dead
}

func eventError(results []sent) error {
	var first error
	retry := false
	for _, s := range results {
		switch s.outcome {
		case apns.AuthFailed:
			return errs.New(errs.CodeAPNSAuthFailed, opPush)
		case apns.Retry:
			retry = true
		case apns.Delivered, apns.TokenDead, apns.Rejected:
		}
		if first == nil {
			first = s.err
		}
	}
	switch {
	case first != nil:
		return first
	case retry:
		return errs.New(errs.CodeAPNSUnavailable, opPush)
	}
	return nil
}
