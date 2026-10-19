package domain

type Category string

const (
	CategoryDecisionRequests   Category = "decision_requests"
	CategoryRepliesAndMentions Category = "replies_and_mentions"
)

func CategoryOf(kind string) (Category, bool) {
	switch kind {
	case "proposal_created", "proposal_passed", "trade_filled", "trade_failed", "trade_blocked",
		"cabal_paused", "cabal_resumed":
		return CategoryDecisionRequests, true
	case "comment_reply", "chat_mention", "chat_thread_reply":
		return CategoryRepliesAndMentions, true
	}
	return "", false
}

func AlwaysOnKinds() []string {
	return []string{
		"deposit_credited", "new_follower", "notify.follow_digest", "nudge",
		"cabal_access_requested", "cabal_access_approved", "test",
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
	switch c {
	case CategoryDecisionRequests:
		return p.DecisionRequests
	case CategoryRepliesAndMentions:
		return p.RepliesAndMentions
	}
	return true
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
