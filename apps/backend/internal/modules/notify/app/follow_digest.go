package app

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

const (
	followDigestInterval = time.Hour
	followDigestLag      = time.Hour
	followDigestKind     = "follow_digest"
	opFollowDigest       = "notify.FollowDigest"
)

type FollowDigest struct{ pusher *Pusher }

func NewFollowDigest(p *Pusher) *FollowDigest { return &FollowDigest{pusher: p} }

func (*FollowDigest) Name() string { return "notify.follow_digest" }

func (*FollowDigest) Interval() time.Duration { return followDigestInterval }

func (p *FollowDigest) Tick(ctx context.Context) (poller.Report, error) {
	now := p.pusher.clock.Now()
	until := now.UTC().Add(-followDigestLag).Truncate(day)
	since := until.Add(-day)
	rows, err := sqlc.New(p.pusher.uow.Reads()).BatchedFollowCounts(ctx,
		sqlc.BatchedFollowCountsParams{Since: since, Until: until})
	if err != nil {
		return poller.Report{}, errs.Wrap(err, errs.CodeDBUnavailable, opFollowDigest)
	}
	report := poller.Report{Scanned: len(rows)}
	cards, err := p.cards(ctx, rows)
	if err != nil {
		return report, err
	}
	var failed []error
	for _, row := range rows {
		followee := ids.UserIDFrom(row.UserID)
		if card, ok := cards[followee]; !ok || card.Deleted {
			continue
		}
		written, err := p.digest(ctx, now, since, followee, row.Followers)
		report.Changed += written
		failed = append(failed, err)
	}
	return report, errors.Join(failed...)
}

func (p *FollowDigest) cards(
	ctx context.Context, rows []sqlc.BatchedFollowCountsRow,
) (map[ids.UserID]identity.UserCard, error) {
	followees := make([]ids.UserID, len(rows))
	for i, row := range rows {
		followees[i] = ids.UserIDFrom(row.UserID)
	}
	cards := make(map[ids.UserID]identity.UserCard, len(rows))
	for chunk := range slices.Chunk(followees, identity.MaxUsersByID) {
		got, err := p.pusher.users.UsersByID(ctx, chunk)
		if err != nil {
			return nil, err
		}
		maps.Copy(cards, got)
	}
	return cards, nil
}

func (p *FollowDigest) digest(
	ctx context.Context, now, since time.Time, followee ids.UserID, followers int64,
) (int, error) {
	key := fmt.Sprintf("%s:%s:%s", p.Name(), followee, since.Format(time.DateOnly))
	source := ids.EventIDFrom(uuid.NewSHA1(uuid.NameSpaceOID, []byte(key)))
	note := Notification{Kind: followDigestKind, UserID: followee, Message: followDigestMessage(followers)}
	written, err := p.pusher.write(ctx, bus.Delivery{Handler: p.Name(), EventID: source, At: now}, []Notification{note})
	if err != nil {
		return 0, err
	}
	return written, p.pusher.send(ctx, source)
}

func followDigestMessage(followers int64) Message {
	body := "1 more person followed you"
	if followers > 1 {
		body = fmt.Sprintf("%d more people followed you", followers)
	}
	return Message{
		Title:      "New followers",
		Body:       body,
		Data:       map[string]string{"kind": followDigestKind},
		CollapseID: "follow-digest",
	}
}
