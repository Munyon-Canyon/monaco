package observability

var ReferralsAttributed = Msg{
	Name:     "referrals.attributed",
	Required: []string{"referral_id", "code_kind", "source"},
}

var ReferralsClick = Msg{
	Name:     "referrals.click",
	Required: []string{"code_kind", "known"},
}
