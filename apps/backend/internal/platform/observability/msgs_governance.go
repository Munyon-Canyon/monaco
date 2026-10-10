package observability

var GovernanceBanVoidSkipped = Msg{
	Name: "governance.ban_void.skipped", Required: []string{"proposal_id", "user_id", "code"},
}
