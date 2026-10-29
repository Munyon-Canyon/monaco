package events

import "github.com/google/uuid"

const TypeCommentPollVoted Type = "comment.poll_voted"

type CommentPollVoted struct {
	V            int       `json:"v"`
	CommentID    uuid.UUID `json:"comment_id"`
	FeedObjectID uuid.UUID `json:"feed_object_id"`
	UserID       uuid.UUID `json:"user_id"        pii:"true"`
	Position     int       `json:"position"`
}

func (CommentPollVoted) Type() Type { return TypeCommentPollVoted }

func (CommentPollVoted) AggregateType() string { return commentAggregate }

func (e CommentPollVoted) AggregateID() uuid.UUID { return e.CommentID }
