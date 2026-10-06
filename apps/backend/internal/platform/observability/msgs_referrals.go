package observability

var ReferralsAttributed = Msg{
	Name:     "referrals.attributed",
	Required: []string{"referral_id", "code_kind", "source"},
}
