package events

func identityRegistrations() []Registration {
	return []Registration{
		Register[UserCreated](TypeUserCreated, 1),
		Register[UserAuthStateChanged](TypeUserAuthStateChanged, 1),
		Register[UserProfileUpdated](TypeUserProfileUpdated, 1),
	}
}
