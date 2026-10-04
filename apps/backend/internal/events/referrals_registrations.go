package events

func referralRegistrations() []Registration {
	return []Registration{
		Register[ReferralAttributed](TypeReferralAttributed, 1),
		Register[ReferralQualified](TypeReferralQualified, 1),
	}
}
