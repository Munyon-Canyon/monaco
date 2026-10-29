package domain

import (
	"strings"
	"time"
)

type CatchUpKind string

const (
	CatchUpWaitingVote CatchUpKind = "waiting_vote"
	CatchUpReply       CatchUpKind = "reply"
	CatchUpMention     CatchUpKind = "mention"
	CatchUpInvite      CatchUpKind = "invite"
	CatchUpResult      CatchUpKind = "result"
)

const (
	ItemKeyProposalPrefix = "proposal:"
	ItemKeyCommentPrefix  = "comment:"
	ItemKeyChatPrefix     = "chat:"
	ItemKeyInvitePrefix   = "invite:"
)

const (
	CatchUpWindow = 30 * 24 * time.Hour
	CatchUpDay    = 24 * time.Hour
)

func CatchUpKindOf(notificationKind string) (CatchUpKind, bool) {
	switch notificationKind {
	case "comment_reply", "post_reply", "chat_thread_reply":
		return CatchUpReply, true
	case "chat_mention", "comment_mention":
		return CatchUpMention, true
	case "proposal_result":
		return CatchUpResult, true
	}
	return "", false
}

func CatchUpNotificationKinds() []string {
	return []string{
		"comment_reply", "post_reply", "chat_thread_reply", "chat_mention", "comment_mention", "proposal_result",
	}
}

func ItemKeyID(key, prefix string) (string, bool) {
	return strings.CutPrefix(key, prefix)
}

type CatchUpPerson struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	PhotoURL string `json:"photo_url"`
}

type CatchUpCabal struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	PictureURL string `json:"picture_url"`
}

type CatchUpProposal struct {
	ID           string        `json:"id"`
	Kind         string        `json:"kind"`
	Symbol       string        `json:"symbol"`
	AssetName    string        `json:"asset_name"`
	AssetLogoURL string        `json:"asset_logo_url"`
	USDCMicros   *uint64       `json:"usdc_micros"`
	TokenAmount  *uint64       `json:"token_amount"`
	Proposer     CatchUpPerson `json:"proposer"`
	CreatedAt    time.Time     `json:"created_at"`
	ExpiresAt    time.Time     `json:"expires_at"`
	Status       string        `json:"status"`
	Yes          int           `json:"yes"`
	No           int           `json:"no"`
	Voted        int           `json:"voted"`
	Eligible     int           `json:"eligible"`
	ReplyCount   int           `json:"reply_count"`
	Outcome      string        `json:"outcome,omitempty"`
	YourChoice   string        `json:"your_choice,omitempty"`
	Filled       *bool         `json:"filled,omitempty"`
}

type CatchUpPost struct {
	FeedItemID string `json:"feed_item_id"`
	Kind       string `json:"kind"`
	Label      string `json:"label"`
	ActorName  string `json:"actor_name"`
}

type CatchUpComment struct {
	FeedItemID      string        `json:"feed_item_id"`
	CommentID       string        `json:"comment_id"`
	ParentCommentID string        `json:"parent_comment_id,omitempty"`
	RootCommentID   string        `json:"root_comment_id"`
	ProposalID      string        `json:"proposal_id,omitempty"`
	CabalID         string        `json:"cabal_id,omitempty"`
	Author          CatchUpPerson `json:"author"`
	Body            string        `json:"body"`
	CreatedAt       time.Time     `json:"created_at"`
}

type CatchUpQuote struct {
	AuthorName string `json:"author_name"`
	IsYou      bool   `json:"is_you"`
	Text       string `json:"text"`
}

type CatchUpInviteInfo struct {
	ID             string `json:"id"`
	MemberCount    int    `json:"member_count"`
	TopHoldingName string `json:"top_holding_name,omitempty"`
}

type CatchUpItem struct {
	Key           string             `json:"key"`
	Kind          CatchUpKind        `json:"kind"`
	Unread        bool               `json:"unread"`
	At            time.Time          `json:"at"`
	ClearedReason string             `json:"cleared_reason,omitempty"`
	Actor         *CatchUpPerson     `json:"actor,omitempty"`
	Cabal         *CatchUpCabal      `json:"cabal,omitempty"`
	Proposal      *CatchUpProposal   `json:"proposal,omitempty"`
	Post          *CatchUpPost       `json:"post,omitempty"`
	Comment       *CatchUpComment    `json:"comment,omitempty"`
	Quote         *CatchUpQuote      `json:"quote,omitempty"`
	Invite        *CatchUpInviteInfo `json:"invite,omitempty"`
}

type CatchUp struct {
	Count   int
	Cabals  []CatchUpCabal
	Waiting []CatchUpItem
	Replies []CatchUpItem
	Invites []CatchUpItem
	Results []CatchUpItem
	Cleared []CatchUpItem
}
