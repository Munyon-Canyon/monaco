package domain

type Category string

const (
	CategoryDecisionRequests   Category = "decision_requests"
	CategoryRepliesAndMentions Category = "replies_and_mentions"
)

func CategoryOf(kind string) (Category, bool) {
	switch kind {
	case "proposal_created",
		"proposal_result",
		"vote_reminder",
		"vote_deciding",
		"close_reminder",
		"cabal_paused",
		"cabal_resumed":
		return CategoryDecisionRequests, true
	case "comment_reply", "post_reply", "comment_mention", "chat_mention", "chat_thread_reply":
		return CategoryRepliesAndMentions, true
	}
	return "", false
}

func AlwaysOnKinds() []string {
	return []string{
		"deposit_credited", "new_follower", "notify.follow_digest", "nudge",
		"cabal_access_requested", "cabal_access_approved", "weekly_ledger", "test",
	}
}

func Categories() []Category {
	return []Category{CategoryDecisionRequests, CategoryRepliesAndMentions}
}

type Preferences struct {
	DecisionRequests   bool
	RepliesAndMentions bool
}

func (p Preferences) Enabled(c Category) bool {
	if c == CategoryDecisionRequests {
		return p.DecisionRequests
	}
	return p.RepliesAndMentions
}

func PreferencesFrom(enabled map[Category]bool) Preferences {
	on := func(c Category) bool {
		v, ok := enabled[c]
		return !ok || v
	}
	return Preferences{
		DecisionRequests:   on(CategoryDecisionRequests),
		RepliesAndMentions: on(CategoryRepliesAndMentions),
	}
}
