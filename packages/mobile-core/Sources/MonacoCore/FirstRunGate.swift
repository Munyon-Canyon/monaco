import Foundation

public enum AccountRestriction: Equatable, Sendable {
    case banned
}

public enum FirstRunDestination: Equatable, Sendable {
    case session
    case restricted(AccountRestriction)
    case handle
    case phone
    case socials
    case findFriends
    case app(restricted: Bool)
}

public enum FirstRunGate {
    public static func contactsPromptSeenKey(userID: String) -> String {
        "contactsPromptSeen.\(userID)"
    }

    public static func contactsPromptSeen(userID: String, in defaults: UserDefaults = .standard) -> Bool {
        defaults.bool(forKey: contactsPromptSeenKey(userID: userID))
    }

    public static func markContactsPromptSeen(userID: String, in defaults: UserDefaults = .standard) {
        defaults.set(true, forKey: contactsPromptSeenKey(userID: userID))
    }

    public static func destination(
        for profile: SessionProfile?, onboardingCursor: OnboardingCursor, contactsPromptSeen: Bool = true,
        connectX: Bool = true
    ) -> FirstRunDestination {
        let onboardingCursor = connectX || onboardingCursor != .socials ? onboardingCursor : .finished
        let next = route(for: profile, onboardingCursor: onboardingCursor)
        guard case .app = next, onboardingCursor == .finished, profile?.phoneLinked == true, !contactsPromptSeen
        else { return next }
        return .findFriends
    }

    private static func route(
        for profile: SessionProfile?, onboardingCursor: OnboardingCursor
    ) -> FirstRunDestination {
        guard let profile else { return .session }
        if profile.accountStatus == .banned { return .restricted(.banned) }
        if profile.handle == nil { return .handle }
        if profile.authState == .created { return .phone }
        if onboardingCursor == .socials,
            profile.authState == .awaitingSocials || profile.authState == .awaitingPhone
        {
            return .socials
        }
        return .app(restricted: profile.accountStatus == .suspended)
    }
}
