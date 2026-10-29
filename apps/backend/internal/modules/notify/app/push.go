package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"maps"
	"math"
	"slices"
	"time"

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
	day      = 24 * time.Hour

	defaultSound = "default"
)

type Message struct {
	Title      string
	Body       string
	Data       map[string]string
	CollapseID string

	Presentation Presentation
}

type Kind[E events.Event] interface {
	Name() string
	Recipients(ctx context.Context, e E) ([]ids.UserID, error)
	Render(ctx context.Context, e E, to ids.UserID) (Message, error)
}

type Capped interface {
	DailyCap() int
}

type Keyed[E events.Event] interface {
	SourceKey(e E) uuid.UUID
}

type ItemKeyed[E events.Event] interface {
	ItemKey(e E) string
}

type Notification struct {
	Kind    string
	UserID  ids.UserID
	Message Message
	Cap     int
	ItemKey string
	Muted   bool
	source  ids.EventID
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
	prefs, err := n.Pusher.preferences(ctx, everyone)
	if err != nil {
		return err
	}
	sources := n.sources(d.EventID, e)
	notes, err := n.notes(ctx, e, recipients, sources, func(i int, to ids.UserID) (keep, muted bool) {
		category, categorised := domain.CategoryOf(n.Kinds[i].Name())
		return eligible[to], categorised && !prefs[to].Enabled(category)
	})
	if err != nil {
		return err
	}
	return n.Pusher.Push(ctx, d, distinct(sources), notes)
}

func (n Notify[E]) sources(event ids.EventID, e E) []ids.EventID {
	sources := make([]ids.EventID, len(n.Kinds))
	for i, k := range n.Kinds {
		sources[i] = event
		if keyed, ok := k.(Keyed[E]); ok {
			sources[i] = ids.EventIDFrom(keyed.SourceKey(e))
		}
	}
	return sources
}

func (n Notify[E]) notes(
	ctx context.Context,
	e E,
	recipients [][]ids.UserID,
	sources []ids.EventID,
	wanted func(kind int, to ids.UserID) (keep, muted bool),
) ([]Notification, error) {
	var notes []Notification
	for i, k := range n.Kinds {
		limit := 0
		if c, ok := k.(Capped); ok {
			limit = c.DailyCap()
		}
		key := ""
		if keyed, ok := k.(ItemKeyed[E]); ok {
			key = keyed.ItemKey(e)
		}
		for _, to := range recipients[i] {
			keep, muted := wanted(i, to)
			if !keep {
				continue
			}
			msg, err := k.Render(ctx, e, to)
			if err != nil {
				return nil, err
			}
			notes = append(notes, Notification{
				Kind: k.Name(), UserID: to, Message: msg, Cap: limit, ItemKey: key, Muted: muted, source: sources[i],
			})
		}
	}
	return notes, nil
}

type Hints interface {
	PublishHint(ctx context.Context, key string, payload []byte)
}

type Pusher struct {
	uow    *db.UnitOfWork
	users  Users
	sender apns.Sender
	ids    ids.Generator
	clock  clock.Clock
	hints  Hints
}

func NewPusher(uow *db.UnitOfWork, users Users, sender apns.Sender, g ids.Generator, c clock.Clock) *Pusher {
	return &Pusher{uow: uow, users: users, sender: sender, ids: g, clock: c}
}

func (p *Pusher) WithHints(h Hints) *Pusher {
	p.hints = h
	return p
}

func (p *Pusher) eligible(ctx context.Context, event ids.EventID, users []ids.UserID) (map[ids.UserID]bool, error) {
	if len(users) == 0 {
		return map[ids.UserID]bool{}, nil
	}
	causing, err := p.causingUser(ctx, event)
	if err != nil {
		return nil, err
	}
	return p.live(ctx, users, causing)
}

func (p *Pusher) live(ctx context.Context, users []ids.UserID, causing string) (map[ids.UserID]bool, error) {
	eligible := map[ids.UserID]bool{}
	var distinct []ids.UserID
	for _, u := range users {
		if _, seen := eligible[u]; !seen {
			eligible[u] = false
			distinct = append(distinct, u)
		}
	}
	if len(distinct) == 0 {
		return eligible, nil
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

func (p *Pusher) wanted(ctx context.Context, kind string, users []ids.UserID) ([]ids.UserID, error) {
	live, err := p.live(ctx, users, "")
	if err != nil {
		return nil, err
	}
	prefs, err := p.preferences(ctx, users)
	if err != nil {
		return nil, err
	}
	category, categorised := domain.CategoryOf(kind)
	return slices.DeleteFunc(slices.Clone(users), func(u ids.UserID) bool {
		return !live[u] || categorised && !prefs[u].Enabled(category)
	}), nil
}

func (p *Pusher) preferences(ctx context.Context, users []ids.UserID) (map[ids.UserID]domain.Preferences, error) {
	if len(users) == 0 {
		return map[ids.UserID]domain.Preferences{}, nil
	}
	seen := make(map[ids.UserID]bool, len(users))
	distinct := make([]uuid.UUID, 0, len(users))
	for _, u := range users {
		if !seen[u] {
			seen[u] = true
			distinct = append(distinct, u.UUID())
		}
	}
	rows, err := sqlc.New(p.uow.Reads()).GetPreferences(ctx, distinct)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeDBUnavailable, opPush)
	}
	enabled := make(map[ids.UserID]map[domain.Category]bool, len(rows))
	for _, r := range rows {
		u := ids.UserIDFrom(r.UserID)
		if enabled[u] == nil {
			enabled[u] = map[domain.Category]bool{}
		}
		enabled[u][domain.Category(r.Category)] = r.Enabled
	}
	prefs := make(map[ids.UserID]domain.Preferences, len(distinct))
	for u := range seen {
		prefs[u] = domain.PreferencesFrom(enabled[u])
	}
	return prefs, nil
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

func (p *Pusher) Push(ctx context.Context, d bus.Delivery, sources []ids.EventID, notes []Notification) error {
	for _, source := range sources {
		keyed := d
		keyed.EventID = source
		if _, err := p.write(ctx, keyed, notesOf(notes, source)); err != nil {
			return err
		}
	}
	var sendErr error
	for _, source := range sources {
		if err := p.send(ctx, source); err != nil && sendErr == nil {
			sendErr = err
		}
	}
	denied := errs.CodeOf(sendErr) == errs.CodeAPNSAuthFailed
	if sendErr != nil && !denied {
		return sendErr
	}
	if err := p.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		if denied {
			_, err := d.RecordAs(ctx, tx, string(errs.CodeAPNSAuthFailed))
			return err
		}
		_, err := d.Record(ctx, tx)
		return err
	}); err != nil {
		return err
	}
	return sendErr
}

func notesOf(notes []Notification, source ids.EventID) []Notification {
	return slices.DeleteFunc(slices.Clone(notes), func(n Notification) bool { return n.source != source })
}

func distinct(sources []ids.EventID) []ids.EventID {
	var out []ids.EventID
	for _, s := range sources {
		if !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	return out
}

func (p *Pusher) write(ctx context.Context, d bus.Delivery, notes []Notification) (int, error) {
	return p.writeWith(ctx, d, notes, nil)
}

func (p *Pusher) writeWith(
	ctx context.Context, d bus.Delivery, notes []Notification, also func(context.Context, *sqlc.Queries) error,
) (int, error) {
	if len(notes) == 0 && also == nil {
		return 0, nil
	}
	written := 0
	err := p.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		written = 0
		q := sqlc.New(tx.Queries())
		if len(notes) > 0 {
			var err error
			if written, err = p.insertBatch(ctx, q, notes, d); err != nil {
				return err
			}
			p.publishCatchUp(tx, notes)
		}
		if also == nil {
			return nil
		}
		return also(ctx, q)
	})
	return written, err
}

func (p *Pusher) publishCatchUp(tx db.Tx, notes []Notification) {
	if p.hints == nil {
		return
	}
	var users []ids.UserID
	for _, n := range notes {
		if n.ItemKey != "" && !slices.Contains(users, n.UserID) {
			users = append(users, n.UserID)
		}
	}
	if len(users) == 0 {
		return
	}
	tx.AfterCommit(func(ctx context.Context) {
		for _, u := range users {
			p.hints.PublishHint(ctx, events.UserCatchUpChangedHint(u), nil)
		}
	})
}

func (p *Pusher) insertBatch(ctx context.Context, q *sqlc.Queries, notes []Notification, d bus.Delivery) (int, error) {
	var broadcast uuid.UUID
	if len(notes) > 1 {
		id, err := q.InsertBroadcast(ctx, sqlc.InsertBroadcastParams{
			ID: p.ids.NewV7(), Kind: notes[0].Kind, SourceEventID: d.EventID.UUID(),
			RecipientCount: int32(min(len(notes), math.MaxInt32)), CreatedAt: d.At,
		})
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return 0, nil
		case err != nil:
			return 0, err
		}
		broadcast = id
	}
	return p.insertAll(ctx, q, notes, broadcast, d)
}

func (p *Pusher) insertAll(
	ctx context.Context, q *sqlc.Queries, notes []Notification, broadcast uuid.UUID, d bus.Delivery,
) (int, error) {
	written := 0
	for _, n := range notes {
		state, err := stateFor(ctx, q, n, d.At)
		if err != nil {
			return 0, err
		}
		params, err := insertParams(p.ids.NewV7(), broadcast, n, state, d)
		if err != nil {
			return 0, err
		}
		_, err = q.InsertNotification(ctx, params)
		switch {
		case errors.Is(err, sql.ErrNoRows):
		case err != nil:
			return 0, err
		default:
			written++
		}
	}
	return written, nil
}

func insertParams(
	id, broadcast uuid.UUID, n Notification, state domain.NotificationState, d bus.Delivery,
) (sqlc.InsertNotificationParams, error) {
	data, _ := json.Marshal(n.Message.Data)
	params := sqlc.InsertNotificationParams{
		ID: id, BroadcastID: broadcast, UserID: n.UserID.UUID(), Kind: n.Kind,
		SourceEventID: d.EventID.UUID(), Title: n.Message.Title, Body: n.Message.Body, Data: data,
		CollapseID: n.Message.CollapseID, State: string(state), CreatedAt: d.At,
		ItemKey: n.ItemKey,
	}
	p := n.Message.Presentation
	if !p.set() {
		return params, nil
	}
	if err := p.check(n.Message.Data); err != nil {
		return params, err
	}
	params.ThreadID, params.Category, params.InterruptionLevel, params.Presentation =
		p.ThreadID, p.Category, p.InterruptionLevel, p.encode()
	return params, nil
}

func stateFor(
	ctx context.Context, q *sqlc.Queries, n Notification, at time.Time,
) (domain.NotificationState, error) {
	if n.Muted {
		return domain.NotificationMuted, nil
	}
	if n.Cap == 0 {
		return domain.NotificationPending, nil
	}
	today, err := q.CountKindSince(ctx, sqlc.CountKindSinceParams{
		UserID: n.UserID.UUID(), Kind: n.Kind, CreatedAt: at.UTC().Truncate(day),
	})
	switch {
	case err != nil:
		return "", err
	case today >= int64(n.Cap):
		return domain.NotificationBatched, nil
	}
	return domain.NotificationPending, nil
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
		pres, err := decodePresentation(row.ThreadID, row.Category, row.InterruptionLevel, row.Presentation)
		if err != nil {
			return nil, nil, err
		}
		if pres.set() {
			data = withPresentation(data, pres)
		}
		for _, t := range tokens {
			j := job{at: len(jobs), kind: row.Kind, push: apns.Push{
				UserID: ids.UserIDFrom(row.UserID), Token: t.Token, Environment: apns.Environment(t.Environment),
				CollapseID: row.CollapseID, Title: row.Title, Body: row.Body, Data: data,
			}}
			if pres.set() {
				j.push.ThreadID, j.push.Category = pres.ThreadID, pres.Category
				j.push.InterruptionLevel, j.push.MutableContent = pres.InterruptionLevel, pres.mutable()
				j.push.Sound = defaultSound
			}
			rows[i].jobs = append(rows[i].jobs, j)
			jobs = append(jobs, j)
		}
	}
	return rows, jobs, nil
}

func withPresentation(data map[string]string, p Presentation) map[string]string {
	merged := make(map[string]string, len(data))
	maps.Copy(merged, data)
	maps.Copy(merged, p.data())
	return merged
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
		case domain.NotificationPending, domain.NotificationBatched, domain.NotificationMuted:
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
