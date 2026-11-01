package events

func identityRegistrations() []Registration {
	return []Registration{
		Register[UserCreated](TypeUserCreated, 1),
		Register[UserAuthStateChanged](TypeUserAuthStateChanged, 1),
		Register[UserProfileUpdated](TypeUserProfileUpdated, 1),
		Register[UserAccountStatusChanged](TypeUserAccountStatusChanged, 1),
		Register[UserDeleted](TypeUserDeleted, 1),
		Register[UserSessionsRevoked](TypeUserSessionsRevoked, 1),
		Register[UserNudgeDue](TypeUserNudgeDue, 1),
	}
}
